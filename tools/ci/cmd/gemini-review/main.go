package main

import (
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/gate"
	"ci-tools/internal/gemini"
	"ci-tools/internal/gitlab"
	"ci-tools/internal/review"
)

func main() {
	// Reject an over-long description before any GitLab or LLM API call.
	if err := gate.CheckDescription(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	cfg := config.Load()
	if cfg.GeminiToken == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'GEMINI_MR_REVIEWER' is missing.")
		os.Exit(1)
	}
	if cfg.GeminiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'GEMINI_API_KEY' is missing.")
		os.Exit(1)
	}

	gl := gitlab.New(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.GeminiToken)
	gm := gemini.New(cfg.GeminiModel, cfg.GeminiKey)

	if err := review.New(gl, gm).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
