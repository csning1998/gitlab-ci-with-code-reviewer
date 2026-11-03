// Binary entrypoint for deterministic merge request labeling.
// Operates independently of LLM reviewer jobs to ensure label application across all merge request pipelines.
package main

import (
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/labeler"
)

func main() {
	cfg := config.LoadEnvFile()
	if cfg.GitLabToken == "" {
		fmt.Fprintf(os.Stderr, "Error: need %s or %s to read and label the MR.\n", config.ClaudeReviewTokenEnvName, config.GeminiReviewTokenEnvName)
		os.Exit(1)
	}
	if err := labeler.ExecuteLabeling(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.GitLabToken); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
