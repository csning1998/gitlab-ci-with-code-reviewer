// Package openaicompat implements review.LLMClient against the OpenAI Chat Completions wire
// format, which Azure OpenAI Service, xAI, and local inference servers all accept. A single
// client covers every such provider; only the base URL, model, and credential source differ.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ci-tools/internal/tokensource"
)

// Config declares the per-provider parameters resolved by the calling binary from the CI job
// environment. APIVersion selects the Azure OpenAI request shape; an empty value selects the
// standard shape used by every other provider.
type Config struct {
	Name       string
	BaseURL    string
	Model      string
	APIVersion string
	MaxTokens  int
	Timeout    time.Duration
	Tokens     tokensource.Provider
}

// Client issues Chat Completions requests to a single configured provider endpoint.
type Client struct {
	name      string
	url       string
	model     string
	maxTokens int
	tokens    tokensource.Provider
	http      *http.Client
}

func New(cfg Config) *Client {
	if cfg.Tokens == nil {
		panic("openaicompat: Config.Tokens must not be nil")
	}
	return &Client{
		name:      cfg.Name,
		url:       resolveEndpoint(cfg),
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		tokens:    cfg.Tokens,
		http:      &http.Client{Timeout: cfg.Timeout},
	}
}

// resolveEndpoint builds the Chat Completions URL. Azure OpenAI addresses a named deployment
// and requires an explicit api-version, whereas the other providers expose the model as a
// request body attribute under a fixed path.
func resolveEndpoint(cfg Config) string {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if cfg.APIVersion == "" {
		return base + "/v1/chat/completions"
	}
	return fmt.Sprintf(
		"%s/openai/deployments/%s/chat/completions?api-version=%s",
		base, cfg.Model, cfg.APIVersion,
	)
}

func (c *Client) Name() string { return c.name }

// truncate bounds an upstream response body before it enters an error message, preventing
// verbose provider diagnostics from propagating unbounded into CI logs.
func truncate(data []byte, limit int) []byte {
	if len(data) <= limit {
		return data
	}
	return data[:limit]
}

// Review submits the prompt and returns the first choice's message content.
//
// The response_format attribute is omitted because the reviewer prompt requires a top-level
// JSON array, which the json_object mode of this API rejects. Fence and prose tolerance is
// already provided by review.extractJSONArray.
func (c *Client) Review(prompt string) (result string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.http.Timeout)
	defer cancel()

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: resolve credential: %w", c.name, err)
	}

	payload := map[string]any{
		"model": c.model,
		"messages": []any{
			map[string]any{"role": "user", "content": prompt},
		},
	}
	if c.maxTokens > 0 {
		payload["max_tokens"] = c.maxTokens
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
	req.Header.Set("Authorization", "Bearer "+token)

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
		return "", fmt.Errorf("%s api %d: %s", c.name, resp.StatusCode, truncate(data, 512))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("%s api: response contained no choices", c.name)
	}
	return parsed.Choices[0].Message.Content, nil
}
