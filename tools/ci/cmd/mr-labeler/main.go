// Binary entrypoint for deterministic merge request labeling.
// Operates independently of LLM reviewer jobs to ensure label application across all merge request pipelines.
package main

import (
	"context"
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/gitlabtoken"
	"ci-tools/internal/labeler"
)

func main() {
	cfg := config.LoadEnvFile()
	if cfg.GitLabToken == "" {
		fmt.Fprintf(os.Stderr, "Error: need %s or %s to read and label the MR.\n", config.ClaudeReviewTokenEnvName, config.GeminiReviewTokenEnvName)
		os.Exit(1)
	}
	token, err := gitlabtoken.New(context.Background(), cfg.APIURL, cfg.GitLabToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to confirm that %s or %s carries a fine-grained personal access token: %v\n", config.ClaudeReviewTokenEnvName, config.GeminiReviewTokenEnvName, err)
		os.Exit(1)
	}
	if err := labeler.ExecuteLabeling(cfg.APIURL, cfg.ProjectID, cfg.MRIID, token); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
