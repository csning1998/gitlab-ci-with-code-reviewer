package config

import (
	"os"
	"strings"
)

// Names of the environment variables which carry GitLab tokens.
const (
	ReviewTokenEnvName       = "REVIEW_MR_REVIEWER"
	ClaudeReviewTokenEnvName = "CLAUDE_MR_REVIEWER"
	GeminiReviewTokenEnvName = "GEMINI_MR_REVIEWER"
	TagPushTokenEnvName      = "TAG_PUSH_TOKEN"
	JobTokenEnvName          = "CI_JOB_TOKEN"
)

// gitLabTokenEnvNames lists the merge request API credentials in precedence order. The neutral
// name serves the unified reviewer. The provider-specific names remain accepted.
var gitLabTokenEnvNames = []string{ReviewTokenEnvName, ClaudeReviewTokenEnvName, GeminiReviewTokenEnvName}

// ResolveGitLabToken returns the first configured merge request API credential, or an empty
// string when no candidate variable carries a value.
func ResolveGitLabToken() string {
	for _, name := range gitLabTokenEnvNames {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}
