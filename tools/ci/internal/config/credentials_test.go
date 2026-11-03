package config

import (
	"testing"
)

// Token values which identify the variable a resolver read.
const (
	reviewTokenValue  = "review-token"
	claudeTokenValue  = "claude-token"
	geminiTokenValue  = "gemini-token"
	fileTokenValue    = "file-token"
	processTokenValue = "process-token"
)

// clearCredentialEnv unsets every variable the GitLab credential resolvers read.
func clearCredentialEnv(t *testing.T) {
	t.Helper()
	for _, name := range gitLabTokenEnvNames {
		t.Setenv(name, "")
	}
}

func TestResolveGitLabToken_PrefersNeutralVariable(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv(ReviewTokenEnvName, reviewTokenValue)
	t.Setenv(ClaudeReviewTokenEnvName, claudeTokenValue)

	if got := ResolveGitLabToken(); got != reviewTokenValue {
		t.Errorf("ResolveGitLabToken() = %q, want %q", got, reviewTokenValue)
	}
}

func TestResolveGitLabToken_FallsBackToProviderVariables(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "claude fallback",
			env:  map[string]string{ClaudeReviewTokenEnvName: claudeTokenValue},
			want: claudeTokenValue,
		},
		{
			name: "gemini fallback",
			env:  map[string]string{GeminiReviewTokenEnvName: geminiTokenValue},
			want: geminiTokenValue,
		},
		{
			name: "claude precedes gemini",
			env:  map[string]string{ClaudeReviewTokenEnvName: claudeTokenValue, GeminiReviewTokenEnvName: geminiTokenValue},
			want: claudeTokenValue,
		},
		{
			name: "none configured",
			env:  map[string]string{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearCredentialEnv(t)
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			if got := ResolveGitLabToken(); got != tc.want {
				t.Errorf("ResolveGitLabToken() = %q, want %q", got, tc.want)
			}
		})
	}
}
