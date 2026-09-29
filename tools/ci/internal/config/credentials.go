package config

import (
	"os"
	"strings"
)

// gitLabTokenEnvNames lists the merge request API credentials in precedence order. The neutral
// name serves the unified reviewer. The provider-specific names remain accepted.
var gitLabTokenEnvNames = []string{"REVIEW_MR_REVIEWER", "CLAUDE_MR_REVIEWER", "GEMINI_MR_REVIEWER"}

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
