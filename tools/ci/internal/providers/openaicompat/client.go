// Package openaicompat implements review.LLMClient against the OpenAI Chat Completions wire
// format, which Azure OpenAI Service, xAI, and local inference servers all accept. A single
// client covers every such provider. Only the base URL, model, and credential source differ.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ci-tools/internal/config"
	"ci-tools/internal/httpguard"
	"ci-tools/internal/tokensource"
)

// Config declares the injection surface shared by every provider package. The calling binary
// resolves both members from the CI job environment.
type Config struct {
	ModelOptions config.ModelOptions
	Tokens       tokensource.Provider
}

// maxErrorBodyBytes bounds an upstream error body before the body enters an error message.
const maxErrorBodyBytes = 512

// displayNames maps a ModelOptions provider identifier to the label shown in review output.
var displayNames = map[string]string{
	"openai":       "OpenAI",
	"azure-openai": "Azure OpenAI",
	"grok":         "Grok",
	"local":        "Local",
}

// Client issues Chat Completions requests to a single configured provider endpoint.
type Client struct {
	name         string
	url          string
	modelOptions config.ModelOptions
	tokens       tokensource.Provider
	http         *http.Client
}

// New constructs a Client from Config, resolving the display name, endpoint shape, and timeout.
func New(cfg Config) (*Client, error) {
	if cfg.Tokens == nil {
		return nil, errors.New("openaicompat: tokens provider is required")
	}
	cfg.ModelOptions.Model = strings.TrimSpace(cfg.ModelOptions.Model)
	if cfg.ModelOptions.Model == "" {
		return nil, errors.New("openaicompat: model is required")
	}

	provider := strings.TrimSpace(cfg.ModelOptions.Provider)
	baseURL := strings.TrimSpace(cfg.ModelOptions.BaseURL)
	apiVersion := strings.TrimSpace(cfg.ModelOptions.APIVersion)

	switch provider {
	case "azure-openai":
		if baseURL == "" {
			return nil, errors.New("openaicompat: base_url is required for azure-openai")
		}
		if apiVersion == "" {
			return nil, errors.New("openaicompat: api_version is required for azure-openai")
		}
	case "local":
		if baseURL == "" {
			return nil, errors.New("openaicompat: base_url is required for local")
		}
	case "openai":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
	case "grok":
		if baseURL == "" {
			baseURL = "https://api.x.ai/v1"
		}
	default:
		if baseURL == "" {
			if provider == "" {
				return nil, errors.New("openaicompat: base_url is required when provider is not specified")
			}
			return nil, fmt.Errorf("openaicompat: base_url is required for provider %q", provider)
		}
	}
	cfg.ModelOptions.BaseURL = baseURL
	cfg.ModelOptions.APIVersion = apiVersion

	if cfg.ModelOptions.Timeout <= 0 {
		cfg.ModelOptions.Timeout = config.DefaultTimeout
	}

	return &Client{
		name:         resolveDisplayName(cfg.ModelOptions.Provider),
		url:          resolveEndpoint(cfg.ModelOptions),
		modelOptions: cfg.ModelOptions,
		tokens:       cfg.Tokens,
		http:         &http.Client{Timeout: cfg.ModelOptions.Timeout},
	}, nil
}

// resolveDisplayName maps a provider identifier to its label. An unknown value passes through
// verbatim, which keeps an added provider reporting a usable name.
func resolveDisplayName(provider string) string {
	if name, ok := displayNames[strings.TrimSpace(provider)]; ok {
		return name
	}
	return strings.TrimSpace(provider)
}

// resolveEndpoint builds the Chat Completions URL. Azure OpenAI addresses a named deployment
// and requires an explicit api-version, whereas other providers expose model as request body.
func resolveEndpoint(modelOptions config.ModelOptions) string {
	base := strings.TrimSuffix(strings.TrimSpace(modelOptions.BaseURL), "/")
	if modelOptions.APIVersion != "" {
		return fmt.Sprintf(
			"%s/openai/deployments/%s/chat/completions?api-version=%s",
			base, modelOptions.Model, modelOptions.APIVersion,
		)
	}
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1/chat/completions"
}

func (c *Client) Name() string { return c.name }

func isReasoningModel(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.HasPrefix(m, "grok-4.5") ||
		strings.HasPrefix(m, "grok-4.6")
}

func shouldUseMaxCompletionTokens(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.HasPrefix(m, "gpt-5")
}

func (c *Client) buildPayload(prompt string) map[string]any {
	payload := map[string]any{
		"model": c.modelOptions.Model,
		"messages": []any{
			map[string]any{"role": "user", "content": prompt},
		},
	}

	if c.modelOptions.MaxTokens > 0 {
		if shouldUseMaxCompletionTokens(c.modelOptions.Model) {
			payload["max_completion_tokens"] = c.modelOptions.MaxTokens
		} else {
			payload["max_tokens"] = c.modelOptions.MaxTokens
		}
	}

	if c.modelOptions.Temperature != nil {
		payload["temperature"] = *c.modelOptions.Temperature
	}
	if c.modelOptions.TopP != nil {
		payload["top_p"] = *c.modelOptions.TopP
	}
	if c.modelOptions.Seed != nil {
		payload["seed"] = *c.modelOptions.Seed
	}

	if c.modelOptions.ReasoningLevel != "" && c.modelOptions.ReasoningLevel != "none" {
		payload["reasoning_effort"] = c.modelOptions.ReasoningLevel
	}

	if c.modelOptions.RepetitionPenalty != nil {
		payload["repetition_penalty"] = *c.modelOptions.RepetitionPenalty
	}
	if c.modelOptions.MinP != nil {
		payload["min_p"] = *c.modelOptions.MinP
	}

	if !isReasoningModel(c.modelOptions.Model) && c.modelOptions.ReasoningLevel == "" {
		if c.modelOptions.FrequencyPenalty != nil {
			payload["frequency_penalty"] = *c.modelOptions.FrequencyPenalty
		}
		if c.modelOptions.PresencePenalty != nil {
			payload["presence_penalty"] = *c.modelOptions.PresencePenalty
		}
		if len(c.modelOptions.Stop) > 0 {
			payload["stop"] = c.modelOptions.Stop
		}
	}

	return payload
}

// truncate bounds data to at most limit bytes without splitting a multi-byte UTF-8 sequence.
func truncate(data []byte, limit int) []byte {
	return httpguard.TruncateUTF8(data, limit)
}

// Review submits the prompt and returns the message content of the first choice.
func (c *Client) Review(prompt string) (result string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.modelOptions.Timeout)
	defer cancel()

	cred, err := c.tokens.FetchCredential(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: resolve credential: %w", c.name, err)
	}

	isLocal := strings.TrimSpace(c.modelOptions.Provider) == "local"
	if !isLocal || cred.Value != "" {
		if err := httpguard.ValidateCredential(cred.Value); err != nil {
			return "", fmt.Errorf("%s: resolve credential: %w", c.name, err)
		}
	}

	payload := c.buildPayload(prompt)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	if c.modelOptions.Provider == "azure-openai" {
		if cred.Kind == tokensource.KindAPIKey {
			req.Header.Set("api-key", cred.Value)
		} else if cred.Value != "" {
			req.Header.Set("Authorization", "Bearer "+cred.Value)
		}
	} else if cred.Value != "" {
		req.Header.Set("Authorization", "Bearer "+cred.Value)
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
		return "", fmt.Errorf("%s api %d: %s", c.name, resp.StatusCode, truncate(data, maxErrorBodyBytes))
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
