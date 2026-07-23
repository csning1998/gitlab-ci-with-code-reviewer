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
	client sdk.Client
	model  sdk.Model
}

func New(model, apiKey string) *Client {
	return &Client{
		client: sdk.NewClient(option.WithAPIKey(apiKey)),
		model:  sdk.Model(model),
	}
}

func (c *Client) Name() string { return "Claude" }

// Review submits the prompt to the Claude Messages API using a 3-minute execution timeout guard
// to prevent indefinite process hanging in CI runner nodes, aggregating all returned text content blocks.
func (c *Client) Review(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	resp, err := c.client.Messages.New(ctx, sdk.MessageNewParams{
		Model:     c.model,
		MaxTokens: 8192,
		Messages: []sdk.MessageParam{
			sdk.NewUserMessage(sdk.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude api: %w", err)
	}
	var sb strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(sdk.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String(), nil
}
