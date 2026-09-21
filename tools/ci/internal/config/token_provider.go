package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"ci-tools/internal/tokensource"
)

// ResolveTokenProvider selects the credential source for provider. A declared VAULT_ADDR
// selects Workload Identity Federation, in which the CI identity token is exchanged for a
// short lived Vault token. Every other case uses the static provider credential.
func ResolveTokenProvider(provider string) (tokensource.Provider, error) {
	if strings.TrimSpace(os.Getenv("VAULT_ADDR")) != "" {
		return newFederationProvider(provider)
	}

	apiKey := ResolveProviderAPIKey(provider)
	if apiKey == "" {
		return nil, fmt.Errorf("required environment variable %s is missing", DeriveProviderAPIKeyEnv(provider))
	}
	return tokensource.Static(apiKey), nil
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
