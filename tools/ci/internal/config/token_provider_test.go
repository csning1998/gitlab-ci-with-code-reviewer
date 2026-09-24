package config

import (
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
)

// clearTokenProviderEnv unsets every variable ResolveTokenProvider reads, isolating each test
// from whatever CI or shell environment hosts the run.
func clearTokenProviderEnv(t *testing.T) {
	t.Helper()
	clearCredentialEnv(t)
	for _, name := range []string{
		"VAULT_ADDR", "VAULT_ID_TOKEN", "VAULT_AUTH_MOUNT", "VAULT_ROLE",
		"VAULT_KV_MOUNT", "VAULT_SECRET_PATH", "VAULT_SECRET_FIELD", "VAULT_CACERT",
		"ANTHROPIC_FEDERATION_RULE_ID", "ANTHROPIC_ORGANIZATION_ID",
		"ANTHROPIC_SERVICE_ACCOUNT_ID", "ANTHROPIC_ID_TOKEN", "ANTHROPIC_WORKSPACE_ID",
	} {
		t.Setenv(name, "")
	}
}

var defaultTestClaudeWIFConfig = tokensource.ClaudeWIFConfig{
	FederationRuleID: "fdrl_123456",
	OrganizationID:   "org_abcdef",
	ServiceAccountID: "svac_789012",
	IDToken:          "header.payload.signature",
}

// setClaudeWIFEnv declares a complete Claude WIF configuration for the current test.
func setClaudeWIFEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", defaultTestClaudeWIFConfig.FederationRuleID)
	t.Setenv("ANTHROPIC_ORGANIZATION_ID", defaultTestClaudeWIFConfig.OrganizationID)
	t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", defaultTestClaudeWIFConfig.ServiceAccountID)
	t.Setenv("ANTHROPIC_ID_TOKEN", defaultTestClaudeWIFConfig.IDToken)
}

// setVaultEnv declares a complete federation configuration for the current test.
func setVaultEnv(t *testing.T) {
	t.Helper()
	t.Setenv("VAULT_ADDR", "https://vault.example.com")
	t.Setenv("VAULT_ID_TOKEN", "header.payload.signature")
	t.Setenv("VAULT_ROLE", "ci-code-reviewer")
	t.Setenv("VAULT_KV_MOUNT", "secret")
	t.Setenv("VAULT_SECRET_PATH", "ci/credentials")
}

func TestResolveTokenProvider_StaticWhenVaultUnconfigured(t *testing.T) {
	clearTokenProviderEnv(t)
	t.Setenv("GROK_API_KEY", "grok-key")

	provider, err := ResolveTokenProvider("grok")
	if err != nil {
		t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
	}
	static, ok := provider.(tokensource.Static)
	if !ok {
		t.Fatalf("provider has type %T, want tokensource.Static", provider)
	}
	if string(static) != "grok-key" {
		t.Errorf("static credential = %q, want %q", string(static), "grok-key")
	}
}

func TestResolveTokenProvider_MissingStaticCredentialNamesVariable(t *testing.T) {
	clearTokenProviderEnv(t)

	_, err := ResolveTokenProvider("azure-openai")
	if err == nil || !strings.Contains(err.Error(), "AZURE_OPENAI_API_KEY") {
		t.Fatalf("ResolveTokenProvider(...) error = %v, want the derived variable named", err)
	}
}

func TestResolveTokenProvider_FederationWhenVaultConfigured(t *testing.T) {
	// A configured Vault address plus a CI identity token selects the federation path. No
	// provider API key exists in the environment.
	clearTokenProviderEnv(t)
	setVaultEnv(t)

	provider, err := ResolveTokenProvider("claude")
	if err != nil {
		t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
	}
	if _, ok := provider.(*tokensource.VaultKV); !ok {
		t.Fatalf("provider has type %T, want *tokensource.VaultKV", provider)
	}
}

func TestResolveTokenProvider_FederationPrecedesStaticCredential(t *testing.T) {
	clearTokenProviderEnv(t)
	setVaultEnv(t)
	t.Setenv("CLAUDE_API_KEY", "static-key")

	provider, err := ResolveTokenProvider("claude")
	if err != nil {
		t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
	}
	if _, ok := provider.(*tokensource.VaultKV); !ok {
		t.Fatalf("provider has type %T, want the federation path to win", provider)
	}
}

func TestResolveTokenProvider_VaultAddressWithoutIDTokenIsRejected(t *testing.T) {
	// A half configured federation MUST fail loudly. Falling back to a static key would mask
	// a broken id_tokens declaration in the CI job.
	clearTokenProviderEnv(t)
	setVaultEnv(t)
	t.Setenv("VAULT_ID_TOKEN", "")

	_, err := ResolveTokenProvider("claude")
	if err == nil || !strings.Contains(err.Error(), "VAULT_ID_TOKEN") {
		t.Fatalf("ResolveTokenProvider(...) error = %v, want the missing id token named", err)
	}
}

func TestResolveTokenProvider_IncompleteFederationConfigIsRejected(t *testing.T) {
	tests := []struct {
		name    string
		cleared string
	}{
		{name: "missing role", cleared: "VAULT_ROLE"},
		{name: "missing kv mount", cleared: "VAULT_KV_MOUNT"},
		{name: "missing secret path", cleared: "VAULT_SECRET_PATH"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setVaultEnv(t)
			t.Setenv(tc.cleared, "")

			if _, err := ResolveTokenProvider("claude"); err == nil {
				t.Fatalf("ResolveTokenProvider(...) succeeded unexpectedly without %s", tc.cleared)
			}
		})
	}
}

func TestResolveTokenProvider_AppliesDefaultMountAndField(t *testing.T) {
	// One KV secret holds one field per provider, which lets every matrix job share a single
	// VAULT_SECRET_PATH while reading its own credential.
	tests := []struct {
		provider  string
		wantField string
	}{
		{provider: "claude", wantField: "claude_api_key"},
		{provider: "gemini", wantField: "gemini_api_key"},
		{provider: "azure-openai", wantField: "azure_openai_api_key"},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setVaultEnv(t)

			cfg := DeriveVaultKVConfig(tc.provider)
			if cfg.AuthMountPath != tokensource.DefaultAuthMountPath {
				t.Errorf("AuthMountPath = %q, want the %q default", cfg.AuthMountPath, tokensource.DefaultAuthMountPath)
			}
			if cfg.SecretField != tc.wantField {
				t.Errorf("SecretField = %q, want %q", cfg.SecretField, tc.wantField)
			}
		})
	}
}

func TestResolveTokenProvider_HonoursExplicitMountAndField(t *testing.T) {
	clearTokenProviderEnv(t)
	setVaultEnv(t)
	t.Setenv("VAULT_AUTH_MOUNT", "spire-oidc-jwt")
	t.Setenv("VAULT_SECRET_FIELD", "claude_api_key")

	cfg := DeriveVaultKVConfig("claude")
	if cfg.AuthMountPath != "spire-oidc-jwt" {
		t.Errorf("AuthMountPath = %q, want %q", cfg.AuthMountPath, "spire-oidc-jwt")
	}
	if cfg.SecretField != "claude_api_key" {
		t.Errorf("SecretField = %q, want %q", cfg.SecretField, "claude_api_key")
	}
}
