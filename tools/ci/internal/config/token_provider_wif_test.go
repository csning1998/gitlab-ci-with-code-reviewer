package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
)

func TestDeriveClaudeWIFConfig_ValueBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func(t *testing.T)
		want     tokensource.ClaudeWIFConfig
	}{
		{
			name:     "unset",
			setupEnv: func(t *testing.T) {},
			want:     tokensource.ClaudeWIFConfig{},
		},
		{
			name: "surrounding whitespace trimmed",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "  fdrl_123456\n")
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "\tabcdef01-2345-4678-89ab-cdef01234567  ")
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "  svac_789012\t")
				t.Setenv("ANTHROPIC_ID_TOKEN", "\nheader.payload.signature  ")
				t.Setenv("ANTHROPIC_WORKSPACE_ID", "  wrkspc_optional_123\n")
			},
			want: tokensource.ClaudeWIFConfig{
				FederationRuleID: "fdrl_123456",
				OrganizationID:   "abcdef01-2345-4678-89ab-cdef01234567",
				ServiceAccountID: "svac_789012",
				IDToken:          "header.payload.signature",
				WorkspaceID:      "wrkspc_optional_123",
			},
		},
		{
			name: "whitespace only becomes empty",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", " \t")
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "\n")
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "\u00a0")
				t.Setenv("ANTHROPIC_ID_TOKEN", "  ")
				t.Setenv("ANTHROPIC_WORKSPACE_ID", " \t\n")
			},
			want: tokensource.ClaudeWIFConfig{},
		},
		{
			name: "workspace omitted",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
			},
			want: defaultTestClaudeWIFConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			if got := DeriveClaudeWIFConfig(); got != tc.want {
				t.Errorf("DeriveClaudeWIFConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_ProviderSpellings(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{name: "lower", provider: "claude"},
		{name: "mixed", provider: "Claude"},
		{name: "upper", provider: "CLAUDE"},
		{name: "padded", provider: "  claude\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setClaudeWIFEnv(t)

			provider, err := ResolveTokenProvider(tc.provider)
			if err != nil {
				t.Fatalf("ResolveTokenProvider(%q) returned an unexpected error: %v", tc.provider, err)
			}
			wif, ok := provider.(*tokensource.ClaudeWIF)
			if !ok {
				t.Fatalf("provider has type %T, want *tokensource.ClaudeWIF", provider)
			}
			if got := wif.Config(); got != defaultTestClaudeWIFConfig {
				t.Errorf("Config() = %+v, want %+v", got, defaultTestClaudeWIFConfig)
			}
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_PartialConfigurationDoesNotFallBack(t *testing.T) {
	tests := []struct {
		name       string
		setupEnv   func(t *testing.T)
		wantSubstr string
	}{
		{
			name: "rule only with static key",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "fdrl_123456")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			wantSubstr: "ANTHROPIC_ORGANIZATION_ID",
		},
		{
			name: "rule only with vault",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "fdrl_123456")
				setVaultEnv(t)
			},
			wantSubstr: "ANTHROPIC_ORGANIZATION_ID",
		},
		{
			name: "rule and organization with static key",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "fdrl_123456")
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "abcdef01-2345-4678-89ab-cdef01234567")
				t.Setenv("CLAUDE_API_KEY", "static-key")
				t.Setenv("REVIEW_API_KEY", "neutral-key")
			},
			wantSubstr: "ANTHROPIC_SERVICE_ACCOUNT_ID",
		},
		{
			name: "three fields with vault",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_ID_TOKEN", "")
				setVaultEnv(t)
			},
			wantSubstr: "ANTHROPIC_ID_TOKEN",
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
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_InactiveSignalsKeepStatic(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		setupEnv func(t *testing.T)
		want     string
	}{
		{
			name:     "whitespace required fields",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", " \t\n")
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "   ")
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "\u00a0")
				t.Setenv("ANTHROPIC_ID_TOKEN", "\t")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "workspace alone",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_WORKSPACE_ID", "wrkspc_123")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "id token alone from CI template keeps static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "both CI template id tokens keep static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature")
				t.Setenv("VAULT_ID_TOKEN", "header.payload.signature")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "id token alone prefers the neutral review key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature")
				t.Setenv("CLAUDE_API_KEY", "static-key")
				t.Setenv("REVIEW_API_KEY", "neutral-key")
			},
			want: "neutral-key",
		},
		{
			name:     "organization alone keeps static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "abcdef01-2345-4678-89ab-cdef01234567")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "service account alone keeps static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "svac_789012")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "organization and service account without rule keep static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "abcdef01-2345-4678-89ab-cdef01234567")
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "svac_789012")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "every field except rule keeps static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "whitespace rule with every other field keeps static key",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "  ")
				t.Setenv("CLAUDE_API_KEY", "static-key")
			},
			want: "static-key",
		},
		{
			name:     "openai with CI template id token keeps its static key",
			provider: "openai",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature")
				t.Setenv("OPENAI_API_KEY", "openai-key")
			},
			want: "openai-key",
		},
		{
			name:     "grok with CI template id token keeps its static key",
			provider: "grok",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature")
				t.Setenv("GROK_API_KEY", "grok-key")
			},
			want: "grok-key",
		},
		{
			name:     "azure-openai keeps its static key",
			provider: "azure-openai",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("AZURE_OPENAI_API_KEY", "azure-key")
			},
			want: "azure-key",
		},
		{
			name:     "prefixed provider name keeps its static key",
			provider: "claude-sonnet",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				t.Setenv("CLAUDE_SONNET_API_KEY", "sonnet-key")
			},
			want: "sonnet-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			provider, err := ResolveTokenProvider(tc.provider)
			if err != nil {
				t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
			}
			static, ok := provider.(tokensource.Static)
			if !ok {
				t.Fatalf("provider has type %T, want tokensource.Static", provider)
			}
			if string(static) != tc.want {
				t.Errorf("static credential = %q, want %q", string(static), tc.want)
			}
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_InactiveSignalsKeepVault(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		setupEnv func(t *testing.T)
	}{
		{
			name:     "whitespace required fields",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", " \t\n")
				t.Setenv("ANTHROPIC_ORGANIZATION_ID", "   ")
				t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "\u00a0")
				t.Setenv("ANTHROPIC_ID_TOKEN", "\t")
				setVaultEnv(t)
			},
		},
		{
			name:     "workspace alone",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_WORKSPACE_ID", "wrkspc_123")
				setVaultEnv(t)
			},
		},
		{
			name:     "id token alone from CI template keeps vault",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_ID_TOKEN", "header.payload.signature")
				setVaultEnv(t)
			},
		},
		{
			name:     "gemini keeps vault",
			provider: "gemini",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
				setVaultEnv(t)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			provider, err := ResolveTokenProvider(tc.provider)
			if err != nil {
				t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
			}
			if _, ok := provider.(*tokensource.VaultKV); !ok {
				t.Fatalf("provider has type %T, want *tokensource.VaultKV", provider)
			}
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_InactiveSignalsNameStaticVariable(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		setupEnv   func(t *testing.T)
		wantSubstr string
	}{
		{
			name:     "workspace alone",
			provider: "claude",
			setupEnv: func(t *testing.T) {
				t.Setenv("ANTHROPIC_WORKSPACE_ID", "wrkspc_123")
			},
			wantSubstr: "CLAUDE_API_KEY",
		},
		{
			name:     "grok without a key",
			provider: "grok",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
			},
			wantSubstr: "GROK_API_KEY",
		},
		{
			name:     "empty provider",
			provider: "",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
			},
			wantSubstr: "required environment variable",
		},
		{
			name:     "whitespace provider",
			provider: "   ",
			setupEnv: func(t *testing.T) {
				setClaudeWIFEnv(t)
			},
			wantSubstr: "required environment variable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			_, err := ResolveTokenProvider(tc.provider)
			if err == nil {
				t.Fatalf("ResolveTokenProvider(...) succeeded unexpectedly when %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("ResolveTokenProvider(...) error = %q, want error containing %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_TrimsValuesIntoCredential(t *testing.T) {
	clearTokenProviderEnv(t)
	t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "  fdrl_123456\n")
	t.Setenv("ANTHROPIC_ORGANIZATION_ID", "\tabcdef01-2345-4678-89ab-cdef01234567  ")
	t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "  svac_789012\t")
	t.Setenv("ANTHROPIC_ID_TOKEN", " \theader.payload.signature\n")
	t.Setenv("ANTHROPIC_WORKSPACE_ID", "  wrkspc_optional_123\n")
	t.Setenv("CLAUDE_API_KEY", "static-key")
	t.Setenv("REVIEW_API_KEY", "neutral-key")
	setVaultEnv(t)

	provider, err := ResolveTokenProvider("Claude")
	if err != nil {
		t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
	}
	wif, ok := provider.(*tokensource.ClaudeWIF)
	if !ok {
		t.Fatalf("provider has type %T, want *tokensource.ClaudeWIF", provider)
	}
	want := defaultTestClaudeWIFConfig
	want.WorkspaceID = "wrkspc_optional_123"
	if got := wif.Config(); got != want {
		t.Errorf("Config() = %+v, want %+v", got, want)
	}
	cred, err := wif.FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	if cred.Value != want.IDToken || cred.Kind != tokensource.KindBearer {
		t.Errorf("FetchCredential() = %+v, want Value %q Kind %v", cred, want.IDToken, tokensource.KindBearer)
	}
}

func TestResolveTokenProvider_ClaudeWIF_MissingRequiredFieldReportsFieldError(t *testing.T) {
	tests := []struct {
		name      string
		envName   string
		wantField tokensource.Field
	}{
		{name: "organization", envName: "ANTHROPIC_ORGANIZATION_ID", wantField: tokensource.FieldOrganizationID},
		{name: "service account", envName: "ANTHROPIC_SERVICE_ACCOUNT_ID", wantField: tokensource.FieldServiceAccountID},
		{name: "id token", envName: "ANTHROPIC_ID_TOKEN", wantField: tokensource.FieldIDToken},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setClaudeWIFEnv(t)
			t.Setenv(tc.envName, "")

			_, err := ResolveTokenProvider("claude")
			var fieldErr *tokensource.FieldError
			if !errors.As(err, &fieldErr) {
				t.Fatalf("ResolveTokenProvider(...) error = %v, want a wrapped *tokensource.FieldError", err)
			}
			if fieldErr.Field != tc.wantField {
				t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, tc.wantField)
			}
			if want := "missing " + tc.envName; !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want error containing %q", err.Error(), want)
			}
			if strings.Contains(err.Error(), "invalid") {
				t.Errorf("error = %q, a missing field must not be reported as invalid", err.Error())
			}
		})
	}
}

func TestResolveTokenProvider_ClaudeWIF_InvalidFormatNamesEnvironmentVariable(t *testing.T) {
	tests := []struct {
		name      string
		envName   string
		value     string
		wantField tokensource.Field
	}{
		{name: "rule", envName: "ANTHROPIC_FEDERATION_RULE_ID", value: "123456789", wantField: tokensource.FieldFederationRuleID},
		{name: "organization", envName: "ANTHROPIC_ORGANIZATION_ID", value: "svac_123456", wantField: tokensource.FieldOrganizationID},
		{name: "service account", envName: "ANTHROPIC_SERVICE_ACCOUNT_ID", value: "wrk_123456", wantField: tokensource.FieldServiceAccountID},
		{name: "id token", envName: "ANTHROPIC_ID_TOKEN", value: "short", wantField: tokensource.FieldIDToken},
		{name: "workspace", envName: "ANTHROPIC_WORKSPACE_ID", value: "org_optional", wantField: tokensource.FieldWorkspaceID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setClaudeWIFEnv(t)
			t.Setenv(tc.envName, tc.value)

			_, err := ResolveTokenProvider("claude")
			var fieldErr *tokensource.FieldError
			if !errors.As(err, &fieldErr) {
				t.Fatalf("ResolveTokenProvider(...) error = %v, want a wrapped *tokensource.FieldError", err)
			}
			if fieldErr.Field != tc.wantField {
				t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, tc.wantField)
			}
			if want := "invalid " + tc.envName; !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want error containing %q", err.Error(), want)
			}
		})
	}
}
