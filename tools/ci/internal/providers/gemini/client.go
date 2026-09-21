package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ci-tools/internal/config"
	"ci-tools/internal/tokensource"
)

// Config declares the injection surface shared by every provider package. The calling binary
// resolves both members from the CI job environment.
type Config struct {
	ModelOptions config.ModelOptions
	Tokens       tokensource.Provider
}

// Client manages HTTP interactions with the Gemini generateContent REST endpoint for a configured model.
type Client struct {
	url              string
	timeout          time.Duration
	tokens           tokensource.Provider
	generationConfig map[string]any
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
		timeout = config.DefaultTimeout
	}

	if err := validateGeneration(model, cfg.ModelOptions); err != nil {
		return nil, err
	}

	return &Client{
		url: fmt.Sprintf(
			"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
			model,
		),
		timeout:          timeout,
		tokens:           cfg.Tokens,
		generationConfig: buildGenerationConfig(model, cfg.ModelOptions),
		http:             &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Name() string { return "Gemini" }

// Review submits the prompt payload to the Gemini API, enforcing JSON structured response configuration
// and aggregating returned candidate text parts.
func (c *Client) Review(prompt string) (result string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("gemini: resolve credential: %w", err)
	}

	payload := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt}}}},
		"generationConfig": c.generationConfig,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", token)

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
		return "", fmt.Errorf("gemini api %d: %s", resp.StatusCode, data)
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
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
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}
