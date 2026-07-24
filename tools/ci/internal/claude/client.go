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
}

func New(model, apiKey string, maxTokens int) *Client {
	return &Client{
		client:    sdk.NewClient(option.WithAPIKey(apiKey)),
		model:     sdk.Model(model),
		maxTokens: int64(maxTokens),
	}
}

func (c *Client) Name() string { return "Claude" }

// Review executes a streaming request to the Claude Messages API with a 3-minute timeout guard,
// mitigating HTTP request timeouts on large review payloads.
func (c *Client) Review(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	stream := c.client.Messages.NewStreaming(ctx, sdk.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
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
	var sb strings.Builder
	for _, block := range message.Content {
		if t, ok := block.AsAny().(sdk.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String(), nil
}
