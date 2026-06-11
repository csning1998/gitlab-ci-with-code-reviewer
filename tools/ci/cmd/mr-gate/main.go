package main

import (
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/gate"
	"ci-tools/internal/gitlab"
)

func main() {
	cfg := config.Load()
	// Either reviewer token can read the MR detail; the gate is provider-agnostic.
	token := cfg.ClaudeToken
	if token == "" {
		token = cfg.GeminiToken
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: need CLAUDE_MR_REVIEWER or GEMINI_MR_REVIEWER to read the MR description.")
		os.Exit(1)
	}

	max, err := gate.MaxRunes()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	desc, err := gitlab.New(cfg.APIURL, cfg.ProjectID, cfg.MRIID, token).FetchMRDescription()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if err := gate.CheckLength(desc, max); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println("MR description within limit.")
}
