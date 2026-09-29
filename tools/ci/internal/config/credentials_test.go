package config

import (
	"testing"
)

// clearCredentialEnv unsets every variable the GitLab credential resolvers read.
func clearCredentialEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"REVIEW_MR_REVIEWER", "CLAUDE_MR_REVIEWER", "GEMINI_MR_REVIEWER",
	} {
		t.Setenv(name, "")
	}
}

func TestResolveGitLabToken_PrefersNeutralVariable(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("REVIEW_MR_REVIEWER", "review-token")
	t.Setenv("CLAUDE_MR_REVIEWER", "claude-token")

	if got := ResolveGitLabToken(); got != "review-token" {
		t.Errorf("ResolveGitLabToken() = %q, want %q", got, "review-token")
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
			env:  map[string]string{"CLAUDE_MR_REVIEWER": "claude-token"},
			want: "claude-token",
		},
		{
			name: "gemini fallback",
			env:  map[string]string{"GEMINI_MR_REVIEWER": "gemini-token"},
			want: "gemini-token",
		},
		{
			name: "claude precedes gemini",
			env:  map[string]string{"CLAUDE_MR_REVIEWER": "claude-token", "GEMINI_MR_REVIEWER": "gemini-token"},
			want: "claude-token",
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
