package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"ci-tools/internal/httpguard"
	"ci-tools/internal/reviewerconfig"
	"ci-tools/internal/tokensource"
	"ci-tools/internal/tokensource/gcpwif"
)

// maxErrorBodyBytes bounds an upstream error body before the body enters an error message.
const maxErrorBodyBytes = 512

// Config declares the injection surface shared by every provider package. The calling binary
// resolves both members from the CI job environment.
type Config struct {
	ModelOptions reviewerconfig.ModelOptions
	Tokens       tokensource.Provider
}

// Client manages HTTP interactions with the Gemini generateContent REST endpoint for a configured model.
type Client struct {
	url              string
	model            string
	timeout          time.Duration
	tokens           tokensource.Provider
	generationConfig map[string]any
	tools            []any
	http             *http.Client
}

// New constructs a Client from Config, deriving the model-specific endpoint and falling back
// to the shared default when ModelOptions omits a timeout.
func New(cfg Config) (*Client, error) {
	if cfg.Tokens == nil {
		return nil, errors.New("gemini: tokens provider is required")
	}
	model := strings.TrimSpace(cfg.ModelOptions.Model)
	if model == "" {
		return nil, errors.New("gemini: model is required")
	}

	timeout := cfg.ModelOptions.Timeout
	if timeout <= 0 {
		timeout = reviewerconfig.DefaultTimeout
	}

	if err := validateGeneration(model, cfg.ModelOptions); err != nil {
		return nil, err
	}

	var tools []any
	if cfg.ModelOptions.GoogleSearch != nil && *cfg.ModelOptions.GoogleSearch {
		tools = []any{
			map[string]any{
				"googleSearch": map[string]any{},
			},
		}
	}

	return &Client{
		url:              resolveEndpoint(model, cfg.ModelOptions, cfg.Tokens),
		model:            model,
		timeout:          timeout,
		tokens:           cfg.Tokens,
		generationConfig: buildGenerationConfig(model, cfg.ModelOptions),
		tools:            tools,
		http:             &http.Client{Timeout: timeout, CheckRedirect: httpguard.RefuseCrossHostRedirect},
	}, nil
}

func resolveLocation() string {
	if loc := strings.TrimSpace(os.Getenv("GCP_LOCATION")); loc != "" {
		return loc
	}
	return "global"
}

func resolveProjectID(tokens tokensource.Provider) string {
	if tokens != nil {
		type gcpConfigProvider interface {
			Config() gcpwif.GoogleWIFConfig
		}
		if p, ok := tokens.(gcpConfigProvider); ok {
			if id := strings.TrimSpace(p.Config().ProjectID); id != "" {
				return id
			}
		}
	}
	return strings.TrimSpace(os.Getenv("GCP_PROJECT_ID"))
}

func isGoogleWIF(tokens tokensource.Provider) bool {
	if tokens == nil {
		return false
	}
	type gcpConfigProvider interface {
		Config() gcpwif.GoogleWIFConfig
	}
	_, ok := tokens.(gcpConfigProvider)
	return ok
}

func buildVertexAIEndpoint(location, projectID, model string) string {
	if location == "global" || location == "" {
		return fmt.Sprintf(
			"https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:generateContent",
			projectID,
			model,
		)
	}
	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		location,
		projectID,
		location,
		model,
	)
}

func resolveEndpoint(model string, opts reviewerconfig.ModelOptions, tokens tokensource.Provider) string {
	if baseURL := strings.TrimSpace(opts.BaseURL); baseURL != "" {
		base := strings.TrimSuffix(baseURL, "/")
		if strings.HasSuffix(base, ":generateContent") {
			return base
		}
		return fmt.Sprintf("%s/models/%s:generateContent", base, model)
	}

	if isGoogleWIF(tokens) {
		projectID := resolveProjectID(tokens)
		if projectID != "" {
			return buildVertexAIEndpoint(resolveLocation(), projectID, model)
		}
	}

	return fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		model,
	)
}

func (c *Client) Name() string { return "Gemini" }

// Review submits the prompt payload to the Gemini API, enforcing JSON structured response configuration
// and aggregating returned candidate text parts.
func (c *Client) Review(prompt string) (result string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cred, err := c.tokens.FetchCredential(ctx)
	if err != nil {
		return "", fmt.Errorf("gemini: resolve credential: %w", err)
	}
	if err := httpguard.ValidateCredential(cred.Value); err != nil {
		return "", fmt.Errorf("gemini: resolve credential: %w", err)
	}

	endpoint := c.url
	if cred.Kind == tokensource.KindBearer && strings.Contains(endpoint, "generativelanguage.googleapis.com") {
		projectID := resolveProjectID(c.tokens)
		if projectID != "" {
			endpoint = buildVertexAIEndpoint(resolveLocation(), projectID, c.model)
		}
	}

	payload := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt}}}},
		"generationConfig": c.generationConfig,
	}
	if len(c.tools) > 0 {
		payload["tools"] = c.tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	if cred.Kind == tokensource.KindBearer {
		req.Header.Set("Authorization", "Bearer "+cred.Value)
		if projectID := resolveProjectID(c.tokens); projectID != "" {
			req.Header.Set("x-goog-user-project", projectID)
		}
	} else {
		req.Header.Set("x-goog-api-key", cred.Value)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini api %d: %s", resp.StatusCode, httpguard.TruncateUTF8(data, maxErrorBodyBytes))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		if p.Thought {
			continue
		}
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}
