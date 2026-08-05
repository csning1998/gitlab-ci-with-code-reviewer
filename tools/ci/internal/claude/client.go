package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Client encapsulates Anthropic Messages API operations for a configured model.
type Client struct {
	client    sdk.Client
	model     sdk.Model
	maxTokens int64
	timeout   time.Duration
}

func New(model, apiKey string, maxTokens int, timeout time.Duration) *Client {
	return &Client{
		client:    sdk.NewClient(option.WithAPIKey(apiKey)),
		model:     sdk.Model(model),
		maxTokens: int64(maxTokens),
		timeout:   timeout,
	}
}

func (c *Client) Name() string { return "Claude" }

// Review executes a streaming request to the Claude Messages API guarded by c.timeout,
// mitigating HTTP request timeouts on large review payloads.
//
// Adaptive thinking is disabled so max_tokens applies only to visible response text.
// Sonnet 5 enables adaptive thinking by default; thinking tokens share the max_tokens
// budget with text and can exhaust that budget on large review prompts, yielding an
// empty TextBlock and a downstream JSON parse failure.
func (c *Client) Review(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	disabled := sdk.NewThinkingConfigDisabledParam()
	stream := c.client.Messages.NewStreaming(ctx, sdk.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Thinking: sdk.ThinkingConfigParamUnion{
			OfDisabled: &disabled,
		},
		Messages: []sdk.MessageParam{
			sdk.NewUserMessage(sdk.NewTextBlock(prompt)),
		},
	})
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
