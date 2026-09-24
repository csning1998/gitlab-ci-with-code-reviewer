package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"ci-tools/internal/tokensource"
)

// ClaudeWIFConfig specifies parameters for Claude Workload Identity Federation.
type ClaudeWIFConfig = tokensource.ClaudeWIFConfig

// ResolveTokenProvider selects the credential source for provider. A declared Claude WIF configuration
// takes precedence for the claude provider. A declared VAULT_ADDR selects Workload Identity Federation,
// in which the CI identity token is exchanged for a short lived Vault token. Every other case uses the
// static provider credential.
func ResolveTokenProvider(provider string) (tokensource.Provider, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "claude" {
		cfg := DeriveClaudeWIFConfig()
		if hasClaudeWIFSignal(cfg) {
			return newClaudeWIFProvider(cfg)
		}
	}

	if strings.TrimSpace(os.Getenv("VAULT_ADDR")) != "" {
		return newFederationProvider(provider)
	}

	apiKey := ResolveProviderAPIKey(provider)
	if apiKey == "" {
		return nil, fmt.Errorf("required environment variable %s is missing", DeriveProviderAPIKeyEnv(provider))
	}
	return tokensource.Static(apiKey), nil
}

// hasClaudeWIFSignal checks whether ANTHROPIC_FEDERATION_RULE_ID is configured.
// FederationRuleID is the primary project-level anchor for Anthropic WIF activation;
// ambient CI tokens alone do not activate federation mode.
func hasClaudeWIFSignal(cfg ClaudeWIFConfig) bool {
	return cfg.FederationRuleID != ""
}

// newClaudeWIFProvider builds the Anthropic Claude WIF credential source after validating required fields.
func newClaudeWIFProvider(cfg ClaudeWIFConfig) (tokensource.Provider, error) {
	wif, err := tokensource.NewClaudeWIF(cfg)
	if err != nil {
		return nil, mapWIFValidationError(err)
	}
	return wif, nil
}

var wifFieldEnvNames = map[tokensource.Field]string{
	tokensource.FieldFederationRuleID: "ANTHROPIC_FEDERATION_RULE_ID",
	tokensource.FieldOrganizationID:   "ANTHROPIC_ORGANIZATION_ID",
	tokensource.FieldServiceAccountID: "ANTHROPIC_SERVICE_ACCOUNT_ID",
	tokensource.FieldIDToken:          "ANTHROPIC_ID_TOKEN",
	tokensource.FieldWorkspaceID:      "ANTHROPIC_WORKSPACE_ID",
}

// mapWIFValidationError maps tokensource FieldError to user-facing environment variable error messages.
func mapWIFValidationError(err error) error {
	var fieldErr *tokensource.FieldError
	if errors.As(err, &fieldErr) {
		if envName, ok := wifFieldEnvNames[fieldErr.Field]; ok {
			if fieldErr.Reason == tokensource.ReasonRequired {
				return fmt.Errorf("missing %s: %w", envName, err)
			}
			return fmt.Errorf("invalid %s: %w", envName, err)
		}
	}
	return fmt.Errorf("invalid claude wif configuration: %w", err)
}

// newFederationProvider builds the Vault backed credential source. A missing identity token is
// rejected rather than silently downgraded, which keeps a broken id_tokens declaration visible.
func newFederationProvider(provider string) (tokensource.Provider, error) {
	cfg := DeriveVaultKVConfig(provider)
	if cfg.JWT == "" {
		return nil, errors.New("vault federation requires VAULT_ID_TOKEN when VAULT_ADDR is declared")
	}
	return tokensource.NewVaultKV(cfg)
}

// DeriveVaultKVConfig reads the federation parameters for provider from the job environment.
func DeriveVaultKVConfig(provider string) tokensource.VaultKVConfig {
	return tokensource.VaultKVConfig{
		VaultAddr:     lookupEnv("VAULT_ADDR", ""),
		AuthMountPath: lookupEnv("VAULT_AUTH_MOUNT", tokensource.DefaultAuthMountPath),
		Role:          lookupEnv("VAULT_ROLE", ""),
		JWT:           lookupEnv("VAULT_ID_TOKEN", ""),
		MountPath:     lookupEnv("VAULT_KV_MOUNT", ""),
		SecretPath:    lookupEnv("VAULT_SECRET_PATH", ""),
		SecretField:   lookupEnv("VAULT_SECRET_FIELD", DeriveVaultSecretField(provider)),
		CACertPath:    lookupEnv("VAULT_CACERT", ""),
	}
}

// DeriveVaultSecretField derives the KV-v2 field holding the credential of provider, mapping
// azure-openai to azure_openai_api_key. One secret therefore serves every matrix job.
func DeriveVaultSecretField(provider string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(provider), "-", "_"))
	if normalized == "" {
		return ""
	}
	return normalized + "_api_key"
}

// DeriveClaudeWIFConfig reads the Anthropic Claude federation parameters from the job environment.
func DeriveClaudeWIFConfig() ClaudeWIFConfig {
	return ClaudeWIFConfig{
		FederationRuleID: strings.TrimSpace(os.Getenv("ANTHROPIC_FEDERATION_RULE_ID")),
		OrganizationID:   strings.TrimSpace(os.Getenv("ANTHROPIC_ORGANIZATION_ID")),
		ServiceAccountID: strings.TrimSpace(os.Getenv("ANTHROPIC_SERVICE_ACCOUNT_ID")),
		IDToken:          strings.TrimSpace(os.Getenv("ANTHROPIC_ID_TOKEN")),
		WorkspaceID:      strings.TrimSpace(os.Getenv("ANTHROPIC_WORKSPACE_ID")),
	}
}
