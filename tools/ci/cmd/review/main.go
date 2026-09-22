// Command review posts LLM code review comments on a merge request. A single declared model
// selects the provider, which keeps the job provider agnostic and on-demand.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ci-tools/internal/config"
	"ci-tools/internal/gitlab"
	"ci-tools/internal/providers"
	"ci-tools/internal/review"
)

// resolvePrompt reads the review prompt declared for the model. A prompt_file MUST stay inside
// the working directory, since the declaration comes from the repository under review.
func resolvePrompt(opts config.ModelOptions) (string, error) {
	file := strings.TrimSpace(opts.PromptFile)
	if file != "" {
		confined, err := confinePromptFile(file)
		if err != nil {
			return "", fmt.Errorf("prompt_file %q: %w", file, err)
		}
		file = confined
	}
	return review.ResolvePrompt(opts.Prompt, file)
}

// confinePromptFile resolves symlinks before the containment check, because a link inside the
// working directory can point at any file of the runner.
func confinePromptFile(path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", errors.New("path must be relative and stay inside the working directory")
	}
	workDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	inside := filepath.IsLocal(resolved)
	if filepath.IsAbs(resolved) {
		rel, err := filepath.Rel(root, resolved)
		inside = err == nil && filepath.IsLocal(rel)
	}
	if !inside {
		return "", errors.New("path resolves outside the working directory")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("path must name a regular file")
	}
	return resolved, nil
}

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

	prompt, err := resolvePrompt(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	llm, err := providers.New(opts, tokens)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	reviewer := review.New(gitlab.New(cfg.APIURL, cfg.ProjectID, cfg.MRIID, gitLabToken), llm).WithPrompt(prompt)
	if err := reviewer.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
