package main

import (
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/gate"
	"ci-tools/internal/gitlab"
)

func main() {
	cfg := config.LoadEnvFile()
	if cfg.GitLabToken == "" {
		fmt.Fprintf(os.Stderr, "Error: need %s or %s to read the MR description.\n", config.ClaudeReviewTokenEnvName, config.GeminiReviewTokenEnvName)
		os.Exit(1)
	}

	max, err := gate.ResolveDescriptionRuneLimit()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	desc, err := gitlab.New(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.GitLabToken).FetchMRDescription()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if err := gate.ValidateDescriptionLength(desc, max); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println("MR description within limit.")
}
