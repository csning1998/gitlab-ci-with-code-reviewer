// Binary entrypoint for Claude code review operations.
// Executable binaries are isolated per provider to maintain distinct execution footprints
// and enforce independent environment validation within dedicated CI pipeline jobs.
package main

import (
	"fmt"
	"os"

	"ci-tools/internal/claude"
	"ci-tools/internal/config"
	"ci-tools/internal/review"
)

func main() {
	cfg := config.Load()
	if cfg.ClaudeToken == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'CLAUDE_MR_REVIEWER' is missing.")
		os.Exit(1)
	}
	if cfg.ClaudeKey == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'CLAUDE_API_KEY' is missing.")
		os.Exit(1)
	}
	if err := review.RunOnMR(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.ClaudeToken,
		claude.New(cfg.ClaudeModel, cfg.ClaudeKey)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
