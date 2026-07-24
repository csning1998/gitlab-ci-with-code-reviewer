package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config aggregates environment configuration for CI tool executables.
type Config struct {
	APIURL      string
	ProjectID   string
	MRIID       string
	GeminiToken string
	ClaudeToken string
	// GitLabToken holds whichever of GeminiToken/ClaudeToken is configured, for binaries that
	// only need GitLab API access (mr-gate, mr-labeler) and are indifferent to LLM provider.
	GitLabToken     string
	GeminiModel     string
	GeminiKey       string
	ClaudeModel     string
	ClaudeKey       string
	ClaudeMaxTokens int
}

// Load populates Config from process environment variables.
// Essential GitLab CI variables trigger immediate failure if missing, whereas provider keys
// are deferred to individual command binaries to accommodate modular job execution.
func Load() Config {
	claudeToken := env("CLAUDE_MR_REVIEWER", "")
	geminiToken := env("GEMINI_MR_REVIEWER", "")
	gitlabToken := claudeToken
	if gitlabToken == "" {
		gitlabToken = geminiToken
	}
	return Config{
		APIURL:          require("CI_API_V4_URL", ""),
		ProjectID:       require("CI_PROJECT_ID", ""),
		MRIID:           require("CI_MERGE_REQUEST_IID", ""),
		GeminiToken:     geminiToken,
		ClaudeToken:     claudeToken,
		GitLabToken:     gitlabToken,
		GeminiModel:     env("GEMINI_MODEL", defaultGeminiModel),
		GeminiKey:       env("GEMINI_API_KEY", ""),
		ClaudeModel:     env("CLAUDE_MODEL", defaultClaudeModel),
		ClaudeKey:       env("CLAUDE_API_KEY", ""),
		ClaudeMaxTokens: envInt("CLAUDE_MAX_TOKENS", defaultClaudeMaxTokens),
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

func envInt(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Printf("Error: '%s' must be an integer, got %q.\n", name, v)
		os.Exit(1)
	}
	return n
}
