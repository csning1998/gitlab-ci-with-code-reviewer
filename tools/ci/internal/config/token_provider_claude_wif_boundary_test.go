package config

import (
	"strings"
	"testing"
)

func TestResolveTokenProvider_ClaudeWIF_LengthFormatInjectionAndLeak(t *testing.T) {
	tests := []struct {
		name       string
		setupEnv   func(t *testing.T)
		wantSubstr string
		secret     string
	}{
		{
			name: "federation rule id too short with static key",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "fdrl_")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			wantSubstr: "ANTHROPIC_FEDERATION_RULE_ID",
			secret:     "fdrl_",
		},
		{
			name: "organization id too long",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "abcdef01-2345-4678-89ab-cdef01234567supersecret")
			},
			wantSubstr: "ANTHROPIC_ORGANIZATION_ID",
			secret:     "supersecret",
		},
		{
			name: "service account id path characters with vault",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				setVaultEnv(t)
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "svac_123/../supersecret")
			},
			wantSubstr: "ANTHROPIC_SERVICE_ACCOUNT_ID",
			secret:     "supersecret",
		},
		{
			name: "id token too short",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_ID_TOKEN", "jwt_tok")
			},
			wantSubstr: "ANTHROPIC_ID_TOKEN",
			secret:     "jwt_tok",
		},
		{
			name: "id token too long",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_ID_TOKEN", strings.Repeat("e", 4086)+"supersecret")
			},
			wantSubstr: "ANTHROPIC_ID_TOKEN",
			secret:     "supersecret",
		},
		{
			name: "id token carriage return injection with vault and static key",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				setVaultEnv(t)
				t.Setenv("CLAUDE_API_KEY", "static-key")
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature\r\nX-Injected: supersecret")
			},
			wantSubstr: "ANTHROPIC_ID_TOKEN",
			secret:     "supersecret",
		},
		{
			name: "workspace wrong prefix",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_WORKSPACE_ID", "org_optional")
			},
			wantSubstr: "ANTHROPIC_WORKSPACE_ID",
			secret:     "org_optional",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			_, err := ResolveTokenProvider("claude")
			if err == nil {
				t.Fatalf("ResolveTokenProvider(...) succeeded unexpectedly when %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("ResolveTokenProvider(...) error = %q, want error containing %q", err.Error(), tc.wantSubstr)
			}
			if tc.secret != "" && strings.Contains(err.Error(), tc.secret) {
				t.Errorf("ResolveTokenProvider(...) error %q contains secret material", err)
			}
		})
	}
}
