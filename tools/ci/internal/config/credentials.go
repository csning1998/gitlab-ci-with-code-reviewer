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

// ResolveProviderAPIKey returns the credential for provider. REVIEW_API_KEY applies to every
// provider and takes precedence over the provider-specific variable.
func ResolveProviderAPIKey(provider string) string {
	if value := strings.TrimSpace(os.Getenv("REVIEW_API_KEY")); value != "" {
		return value
	}
	name := DeriveProviderAPIKeyEnv(provider)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(name))
}

// DeriveProviderAPIKeyEnv derives the provider-specific credential variable name, mapping
// azure-openai to AZURE_OPENAI_API_KEY. An empty provider yields an empty name.
func DeriveProviderAPIKeyEnv(provider string) string {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(provider), "-", "_"))
	if normalized == "" {
		return ""
	}
	return normalized + "_API_KEY"
}
