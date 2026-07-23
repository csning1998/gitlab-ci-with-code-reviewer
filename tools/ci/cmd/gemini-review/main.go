// Binary entrypoint for Gemini code review operations.
// Executable binaries are isolated per provider to maintain distinct execution footprints
// and enforce independent environment validation within dedicated CI pipeline jobs.
package main

import (
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/gemini"
	"ci-tools/internal/review"
)

func main() {
	cfg := config.Load()
	if cfg.GeminiToken == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'GEMINI_MR_REVIEWER' is missing.")
		os.Exit(1)
	}
	if cfg.GeminiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'GEMINI_API_KEY' is missing.")
		os.Exit(1)
	}
	notify := review.NotifyConfig{
		SlackWebhookURL: cfg.SlackWebhookURL,
		ProjectURL:      cfg.ProjectURL,
		ProjectName:     cfg.ProjectName,
		MRIID:           cfg.MRIID,
	}
	if err := review.RunOnMR(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.GeminiToken,
		gemini.New(cfg.GeminiModel, cfg.GeminiKey), notify); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
