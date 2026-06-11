package main

import (
	"fmt"
	"os"

	"ci-tools/internal/anthropic"
	"ci-tools/internal/config"
	"ci-tools/internal/review"
)

func main() {
	cfg := config.Load()
	if cfg.ClaudeToken == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'CLAUDE_MR_REVIEWER' is missing.")
		os.Exit(1)
	}
	if cfg.AnthropicKey == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'CLAUDE_API_KEY' is missing.")
		os.Exit(1)
	}
	if err := review.RunOnMR(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.ClaudeToken,
		anthropic.New(cfg.AnthropicModel, cfg.AnthropicKey)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
