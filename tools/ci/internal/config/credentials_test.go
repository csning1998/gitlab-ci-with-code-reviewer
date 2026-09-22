package config

import (
	"testing"
)

// clearCredentialEnv unsets every variable the credential resolvers read, isolating each test
// from whatever CI or shell environment hosts the run.
func clearCredentialEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"REVIEW_MR_REVIEWER", "CLAUDE_MR_REVIEWER", "GEMINI_MR_REVIEWER",
		"REVIEW_API_KEY", "CLAUDE_API_KEY", "GEMINI_API_KEY",
		"OPENAI_API_KEY", "AZURE_OPENAI_API_KEY", "GROK_API_KEY", "LOCAL_API_KEY",
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

func TestResolveProviderAPIKey_DerivesVariableName(t *testing.T) {
	tests := []struct {
		provider string
		envName  string
	}{
		{provider: "claude", envName: "CLAUDE_API_KEY"},
		{provider: "gemini", envName: "GEMINI_API_KEY"},
		{provider: "openai", envName: "OPENAI_API_KEY"},
		{provider: "azure-openai", envName: "AZURE_OPENAI_API_KEY"},
		{provider: "grok", envName: "GROK_API_KEY"},
		{provider: "local", envName: "LOCAL_API_KEY"},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			clearCredentialEnv(t)
			t.Setenv(tc.envName, "provider-key")
			if got := ResolveProviderAPIKey(tc.provider); got != "provider-key" {
				t.Errorf("ResolveProviderAPIKey(%q) = %q, want the value of %s", tc.provider, got, tc.envName)
			}
		})
	}
}

func TestResolveProviderAPIKey_PrefersNeutralVariable(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("REVIEW_API_KEY", "review-key")
	t.Setenv("GROK_API_KEY", "grok-key")

	if got := ResolveProviderAPIKey("grok"); got != "review-key" {
		t.Errorf("ResolveProviderAPIKey(\"grok\") = %q, want %q", got, "review-key")
	}
}

func TestResolveProviderAPIKey_EmptyWhenUnset(t *testing.T) {
	clearCredentialEnv(t)
	if got := ResolveProviderAPIKey("grok"); got != "" {
		t.Errorf("ResolveProviderAPIKey(\"grok\") = %q, want an empty string", got)
	}
}

func TestDeriveProviderAPIKeyEnv_ProviderSpellings(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{provider: "claude", want: "CLAUDE_API_KEY"},
		{provider: "Claude", want: "CLAUDE_API_KEY"},
		{provider: "  grok\n", want: "GROK_API_KEY"},
		{provider: "azure-openai", want: "AZURE_OPENAI_API_KEY"},
		{provider: "azure_openai", want: "AZURE_OPENAI_API_KEY"},
		{provider: "", want: ""},
		{provider: "   ", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			if got := DeriveProviderAPIKeyEnv(tc.provider); got != tc.want {
				t.Errorf("DeriveProviderAPIKeyEnv(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}

func TestResolveProviderAPIKey_ValueBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		neutral string
		named   string
		want    string
	}{
		{name: "surrounding whitespace trimmed", named: "  key\n", want: "key"},
		{name: "whitespace only is empty", named: " \t ", want: ""},
		{name: "whitespace only neutral falls through", neutral: "  ", named: "key", want: "key"},
		{name: "neutral wins over named", neutral: "neutral", named: "named", want: "neutral"},
		{name: "interior whitespace preserved", named: "a b", want: "a b"},
		{name: "interior newline preserved for later rejection", named: "a\nb", want: "a\nb"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REVIEW_API_KEY", tc.neutral)
			t.Setenv("CLAUDE_API_KEY", tc.named)
			if got := ResolveProviderAPIKey("claude"); got != tc.want {
				t.Errorf("ResolveProviderAPIKey = %q, want %q", got, tc.want)
			}
		})
	}
}
