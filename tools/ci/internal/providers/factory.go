// Package providers selects the review.LLMClient implementation which serves a resolved
// ModelOptions value. The four OpenAI-compatible providers share one implementation.
package providers

import (
	"fmt"

	"ci-tools/internal/config"
	"ci-tools/internal/providers/claude"
	"ci-tools/internal/providers/gemini"
	"ci-tools/internal/providers/openaicompat"
	"ci-tools/internal/review"
	"ci-tools/internal/tokensource"
)

// New constructs the client which implements the wire format of opts.Provider. Construction
// errors raised by the selected provider package pass through unchanged.
func New(opts config.ModelOptions, tokens tokensource.Provider) (review.LLMClient, error) {
	switch opts.Provider {
	case "claude":
		client, err := claude.New(claude.Config{ModelOptions: opts, Tokens: tokens})
		if err != nil {
			return nil, err
		}
		return client, nil
	case "gemini":
		client, err := gemini.New(gemini.Config{ModelOptions: opts, Tokens: tokens})
		if err != nil {
			return nil, err
		}
		return client, nil
	case "openai", "azure-openai", "grok", "local":
		client, err := openaicompat.New(openaicompat.Config{ModelOptions: opts, Tokens: tokens})
		if err != nil {
			return nil, err
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
}
