// Command review posts LLM code review comments on a merge request. A single declared model
// selects the provider, which keeps the job provider agnostic and on-demand.
package main

import (
	"errors"
	"fmt"
	"os"

	"ci-tools/internal/config"
	"ci-tools/internal/providers"
	"ci-tools/internal/review"
)

// resolveOptions reads REVIEW_MODEL as a declaration key first and as a model id second. The
// declaration carries per-model parameters which a CI input cannot express.
func resolveOptions() (config.ModelOptions, error) {
	key := os.Getenv("REVIEW_MODEL")
	opts, err := config.ResolveDeclaredOptions(os.Getenv("REVIEW_CONFIG_FILE"), key)
	if err == nil {
		return opts, nil
	}
	if !errors.Is(err, config.ErrSlotNotConfigured) {
		return config.ModelOptions{}, err
	}
	return config.ResolveReviewOptions()
}

func main() {
	cfg := config.LoadEnvFile()

	gitLabToken := config.ResolveGitLabToken()
	if gitLabToken == "" {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'REVIEW_MR_REVIEWER' is missing.")
		os.Exit(1)
	}

	opts, err := resolveOptions()
	if errors.Is(err, config.ErrSlotNotConfigured) {
		fmt.Fprintln(os.Stderr, "Error: Required environment variable 'REVIEW_MODEL' is missing.")
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	tokens, err := config.ResolveTokenProvider(opts.Provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	llm, err := providers.New(opts, tokens)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if err := review.ExecuteCodeReview(cfg.APIURL, cfg.ProjectID, cfg.MRIID, gitLabToken, llm); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
