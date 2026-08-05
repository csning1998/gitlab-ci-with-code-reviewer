package config

import (
	"bufio"
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
	GitLabToken          string
	GeminiModel          string
	GeminiKey            string
	ClaudeModel          string
	ClaudeKey            string
	ClaudeMaxTokens      int
	ClaudeTimeoutMinutes int
}

// defaultEnvFilePath is the dotenv path LoadEnvFile applies when present in the process
// working directory. Process environment variables always take precedence over file entries.
const defaultEnvFilePath = ".env"

// LoadEnvFile populates Config from the process environment after optionally applying
// KEY=VALUE pairs from defaultEnvFilePath. Existing process environment entries are never
// overwritten by the file. Essential GitLab CI variables trigger immediate failure if missing;
// provider keys are deferred to individual command binaries to accommodate modular job execution.
func LoadEnvFile() Config {
	applyEnvFile(defaultEnvFilePath)
	return loadFromProcessEnv()
}

// loadFromProcessEnv reads configuration exclusively from the process environment.
func loadFromProcessEnv() Config {
	claudeToken := lookupEnv("CLAUDE_MR_REVIEWER", "")
	geminiToken := lookupEnv("GEMINI_MR_REVIEWER", "")
	gitlabToken := claudeToken
	if gitlabToken == "" {
		gitlabToken = geminiToken
	}
	return Config{
		APIURL:               requireEnvVar("CI_API_V4_URL", ""),
		ProjectID:            requireEnvVar("CI_PROJECT_ID", ""),
		MRIID:                requireEnvVar("CI_MERGE_REQUEST_IID", ""),
		GeminiToken:          geminiToken,
		ClaudeToken:          claudeToken,
		GitLabToken:          gitlabToken,
		GeminiModel:          lookupEnv("GEMINI_MODEL", defaultGeminiModel),
		GeminiKey:            lookupEnv("GEMINI_API_KEY", ""),
		ClaudeModel:          lookupEnv("CLAUDE_MODEL", defaultClaudeModel),
		ClaudeKey:            lookupEnv("CLAUDE_API_KEY", ""),
		ClaudeMaxTokens:      lookupEnvInt("CLAUDE_MAX_TOKENS", defaultClaudeMaxTokens),
		ClaudeTimeoutMinutes: lookupEnvInt("CLAUDE_TIMEOUT_MINUTES", defaultClaudeTimeoutMinutes),
	}
}

// applyEnvFile reads path as a dotenv file and sets process environment keys that are unset
// or empty after trim. Missing files are ignored. Malformed lines are skipped.
func applyEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if strings.TrimSpace(os.Getenv(key)) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func requireEnvVar(name, def string) string {
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

func lookupEnv(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}

func lookupEnvInt(name string, def int) int {
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
