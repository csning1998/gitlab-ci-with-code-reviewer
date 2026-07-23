package config

import (
	"fmt"
	"os"
	"strings"
)

// Config aggregates environment configuration for CI tool executables.
type Config struct {
	APIURL          string
	ProjectID       string
	MRIID           string
	GeminiToken     string
	ClaudeToken     string
	GeminiModel     string
	GeminiKey       string
	ClaudeModel     string
	ClaudeKey       string
	SlackWebhookURL string
	ProjectURL      string
	ProjectName     string
}

// Load populates Config from process environment variables.
// Essential GitLab CI variables trigger immediate failure if missing, whereas provider keys and
// integration endpoints are deferred to individual command binaries to accommodate modular job execution.
func Load() Config {
	return Config{
		APIURL:          require("CI_API_V4_URL", ""),
		ProjectID:       require("CI_PROJECT_ID", ""),
		MRIID:           require("CI_MERGE_REQUEST_IID", ""),
		GeminiToken:     env("GEMINI_MR_REVIEWER", ""),
		ClaudeToken:     env("CLAUDE_MR_REVIEWER", ""),
		GeminiModel:     env("GEMINI_MODEL", defaultGeminiModel),
		GeminiKey:       env("GEMINI_API_KEY", ""),
		ClaudeModel:     env("CLAUDE_MODEL", defaultClaudeModel),
		ClaudeKey:       env("CLAUDE_API_KEY", ""),
		SlackWebhookURL: env("SLACK_REVIEW_WEBHOOK_URL", ""),
		ProjectURL:      env("CI_PROJECT_URL", ""),
		ProjectName:     env("CI_PROJECT_NAME", ""),
	}
}

func require(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		v = def
	}
	if v == "" {
		fmt.Printf("Error: Required environment variable '%s' is missing.\n", name)
		os.Exit(1)
	}
	return v
}

func env(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}
