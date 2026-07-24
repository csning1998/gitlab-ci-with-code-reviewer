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
	cfg := config.Load()
	if cfg.GitLabToken == "" {
		fmt.Fprintln(os.Stderr, "Error: need CLAUDE_MR_REVIEWER or GEMINI_MR_REVIEWER to read and label the MR.")
		os.Exit(1)
	}
	if err := labeler.RunOnMR(cfg.APIURL, cfg.ProjectID, cfg.MRIID, cfg.GitLabToken); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
