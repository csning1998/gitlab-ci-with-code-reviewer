package claude

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"ci-tools/internal/config"
	"ci-tools/internal/httpguard"
	"ci-tools/internal/tokensource"
)

// DefaultMaxTokens bounds the visible response when ModelOptions omits an explicit limit.
// max_tokens is a mandatory Messages API field. A zero value MUST NOT reach the wire.
const DefaultMaxTokens = 16384

// responseHeaderTimeout mirrors the SDK default bound on the wait for response headers.
const responseHeaderTimeout = 10 * time.Minute

// Config declares the injection surface shared by every provider package. The calling binary
// resolves both members from the CI job environment.
type Config struct {
	ModelOptions config.ModelOptions
	Tokens       tokensource.Provider
}

// Client encapsulates Anthropic Messages API operations for a configured model.
type Client struct {
	model        sdk.Model
	maxTokens    int64
	timeout      time.Duration
	baseURL      string
	tokens       tokensource.Provider
	http         *http.Client
	modelOptions config.ModelOptions
	thinking     thinkingPlan
}

// New constructs a Client from Config, applying fallbacks for the fields the Messages API
// requires and ModelOptions leaves optional.
func New(cfg Config) (*Client, error) {
	if cfg.Tokens == nil {
		return nil, errors.New("claude: tokens provider is required")
	}
	model := strings.TrimSpace(cfg.ModelOptions.Model)
	if model == "" {
		return nil, errors.New("claude: model is required")
	}

	maxTokens := cfg.ModelOptions.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	timeout := cfg.ModelOptions.Timeout
	if timeout <= 0 {
		timeout = config.DefaultTimeout
	}

	cfg.ModelOptions.MaxTokens = maxTokens
	plan, err := planThinking(model, cfg.ModelOptions)
	if err != nil {
		return nil, err
	}

	return &Client{
		model:        sdk.Model(model),
		maxTokens:    int64(maxTokens),
		timeout:      timeout,
		baseURL:      strings.TrimSpace(cfg.ModelOptions.BaseURL),
		tokens:       cfg.Tokens,
		http:         newHTTPClient(),
		modelOptions: cfg.ModelOptions,
		thinking:     plan,
	}, nil
}

func (c *Client) Name() string { return "Claude" }

// Review executes a streaming Messages API request guarded by c.timeout.
// Thinking stays disabled because thinking tokens share the max_tokens budget with text
// and can exhaust that budget on large prompts, yielding an empty TextBlock.
func (c *Client) Review(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("claude: resolve credential: %w", err)
	}
	if err := httpguard.ValidateCredential(token); err != nil {
		return "", fmt.Errorf("claude: resolve credential: %w", err)
	}

	client := c.newSDKClient(token)
	stream := client.Messages.NewStreaming(ctx, c.buildParams(prompt))
	message := sdk.Message{}
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("claude api: accumulate stream event: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("claude api: %w", err)
	}
	text := extractTextBlocks(message.Content)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf(
			"claude api: empty text response (stop_reason=%s input_tokens=%d output_tokens=%d content_blocks=%d)",
			message.StopReason,
			message.Usage.InputTokens,
			message.Usage.OutputTokens,
			len(message.Content),
		)
	}
	return text, nil
}

// buildParams assembles the Messages API request. Sampling overrides stay out of the payload
// while thinking is active, since the API rejects a non default value in that mode.
func (c *Client) buildParams(prompt string) sdk.MessageNewParams {
	params := sdk.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Thinking:  c.thinking.config,
		Messages: []sdk.MessageParam{
			sdk.NewUserMessage(sdk.NewTextBlock(prompt)),
		},
	}

	if c.thinking.effort != "" {
		params.OutputConfig = sdk.OutputConfigParam{Effort: c.thinking.effort}
	}
	if len(c.modelOptions.Stop) > 0 {
		params.StopSequences = c.modelOptions.Stop
	}
	if serviceTier := strings.TrimSpace(c.modelOptions.ServiceTier); serviceTier != "" {
		params.ServiceTier = sdk.MessageNewParamsServiceTier(serviceTier)
	}

	if c.thinking.active {
		return params
	}
	if c.modelOptions.Temperature != nil {
		params.Temperature = param.NewOpt(*c.modelOptions.Temperature)
	}
	if c.modelOptions.TopP != nil {
		params.TopP = param.NewOpt(*c.modelOptions.TopP)
	}
	if c.modelOptions.TopK != nil {
		params.TopK = param.NewOpt(int64(*c.modelOptions.TopK))
	}
	return params
}

// newHTTPClient builds the transport shared by every request. The SDK default client is
// replaced to attach the redirect guard, and its response header timeout is kept.
func newHTTPClient() *http.Client {
	transport := http.DefaultTransport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		clone := base.Clone()
		clone.ResponseHeaderTimeout = responseHeaderTimeout
		transport = clone
	}
	return &http.Client{Transport: transport, CheckRedirect: httpguard.RefuseCrossHostRedirect}
}

// newSDKClient builds a per-request SDK client. The resolved credential is never retained
// between calls. WithoutEnvironmentDefaults keeps ANTHROPIC_* variables from replacing the
// configured endpoint or credential. An empty baseURL selects the SDK default endpoint.
func (c *Client) newSDKClient(token string) sdk.Client {
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithHTTPClient(c.http),
		option.WithAPIKey(token),
	}
	if c.baseURL != "" {
		opts = append(opts, option.WithBaseURL(c.baseURL))
	}
	return sdk.NewClient(opts...)
}

// extractTextBlocks concatenates TextBlock payloads from a Messages API content list.
// Non-text blocks (for example thinking) are ignored by design for this reviewer.
func extractTextBlocks(blocks []sdk.ContentBlockUnion) string {
	var sb strings.Builder
	for _, block := range blocks {
		if t, ok := block.AsAny().(sdk.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}
