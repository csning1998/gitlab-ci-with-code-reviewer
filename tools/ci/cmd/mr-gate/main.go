package main

import (
	"context"
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/gate"
	"ci-tools/internal/gitlab"
	"ci-tools/internal/gitlabtoken"
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

	// The token check runs after the local configuration checks, which report a configuration error without a network call.
	token, err := gitlabtoken.New(context.Background(), cfg.APIURL, cfg.GitLabToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to confirm that %s or %s carries a fine-grained personal access token: %v\n", config.ClaudeReviewTokenEnvName, config.GeminiReviewTokenEnvName, err)
		os.Exit(1)
	}

	desc, err := gitlab.New(cfg.APIURL, cfg.ProjectID, cfg.MRIID, token).FetchMRDescription()
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
