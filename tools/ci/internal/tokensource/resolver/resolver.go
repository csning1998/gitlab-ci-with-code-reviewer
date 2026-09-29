package resolver

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"ci-tools/internal/tokensource"
	"ci-tools/internal/tokensource/azurewif"
	"ci-tools/internal/tokensource/claudewif"
	"ci-tools/internal/tokensource/gcpwif"
)

// ClaudeWIFConfig specifies parameters for Anthropic Claude Workload Identity Federation.
type ClaudeWIFConfig = claudewif.ClaudeWIFConfig

// AzureWIFConfig specifies parameters for Microsoft Azure Workload Identity Federation.
type AzureWIFConfig = azurewif.AzureWIFConfig

// GoogleWIFConfig specifies parameters for Google Cloud Workload Identity Federation.
type GoogleWIFConfig = gcpwif.GoogleWIFConfig

// Resolve selects the credential source for provider. A declared native WIF configuration
// takes precedence for the matching provider. A declared VAULT_ADDR selects Workload Identity Federation
// via Vault. Every other case uses the static provider credential.
func Resolve(provider string) (tokensource.Provider, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "claude":
		cfg := DeriveClaudeWIFConfig()
		if hasClaudeWIFSignal(cfg) {
			return newClaudeWIFProvider(cfg)
		}
	case "azure-openai", "azure_openai":
		cfg := DeriveAzureWIFConfig()
		if hasAzureWIFSignal(cfg) {
			return newAzureWIFProvider(cfg)
		}
	case "gemini", "google":
		cfg := DeriveGoogleWIFConfig()
		if hasGoogleWIFSignal(cfg) {
			return newGoogleWIFProvider(cfg)
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

// ResolveTokenProvider is an alias for Resolve.
func ResolveTokenProvider(provider string) (tokensource.Provider, error) {
	return Resolve(provider)
}

// hasClaudeWIFSignal checks whether ANTHROPIC_FEDERATION_RULE_ID is configured.
func hasClaudeWIFSignal(cfg claudewif.ClaudeWIFConfig) bool {
	return cfg.FederationRuleID != ""
}

// hasAzureWIFSignal checks whether AZURE_CLIENT_ID is configured.
func hasAzureWIFSignal(cfg azurewif.AzureWIFConfig) bool {
	return cfg.ClientID != ""
}

// hasGoogleWIFSignal checks whether GCP_WORKLOAD_IDENTITY_PROVIDER is configured.
func hasGoogleWIFSignal(cfg gcpwif.GoogleWIFConfig) bool {
	return cfg.WorkloadIdentityProvider != ""
}

// newClaudeWIFProvider builds the Anthropic Claude WIF credential source.
func newClaudeWIFProvider(cfg claudewif.ClaudeWIFConfig) (tokensource.Provider, error) {
	wif, err := claudewif.NewClaudeWIF(cfg)
	if err != nil {
		return nil, mapClaudeWIFValidationError(err)
	}
	return wif, nil
}

// newAzureWIFProvider builds the Azure WIF credential source.
func newAzureWIFProvider(cfg azurewif.AzureWIFConfig) (tokensource.Provider, error) {
	wif, err := azurewif.NewAzureWIF(cfg)
	if err != nil {
		return nil, mapAzureWIFValidationError(err)
	}
	return wif, nil
}

// newGoogleWIFProvider builds the Google Cloud WIF credential source.
func newGoogleWIFProvider(cfg gcpwif.GoogleWIFConfig) (tokensource.Provider, error) {
	wif, err := gcpwif.NewGoogleWIF(cfg)
	if err != nil {
		return nil, mapGoogleWIFValidationError(err)
	}
	return wif, nil
}

var claudeWIFFieldEnvNames = map[tokensource.Field]string{
	tokensource.FieldFederationRuleID: "ANTHROPIC_FEDERATION_RULE_ID",
	tokensource.FieldOrganizationID:   "ANTHROPIC_ORGANIZATION_ID",
	tokensource.FieldServiceAccountID: "ANTHROPIC_SERVICE_ACCOUNT_ID",
	tokensource.FieldIDToken:          "ANTHROPIC_ID_TOKEN",
	tokensource.FieldWorkspaceID:      "ANTHROPIC_WORKSPACE_ID",
}

func mapClaudeWIFValidationError(err error) error {
	var fieldErr *tokensource.FieldError
	if errors.As(err, &fieldErr) {
		if envName, ok := claudeWIFFieldEnvNames[fieldErr.Field]; ok {
			if fieldErr.Reason == tokensource.ReasonRequired {
				return fmt.Errorf("missing %s: %w", envName, err)
			}
			return fmt.Errorf("invalid %s: %w", envName, err)
		}
	}
	return fmt.Errorf("invalid claude wif configuration: %w", err)
}

var azureWIFFieldEnvNames = map[tokensource.Field]string{
	tokensource.FieldTenantID:       "AZURE_TENANT_ID",
	tokensource.FieldClientID:       "AZURE_CLIENT_ID",
	tokensource.FieldOpenAIEndpoint: "AZURE_OPENAI_ENDPOINT",
	tokensource.FieldIDToken:        "AZURE_ID_TOKEN",
}

var googleWIFFieldEnvNames = map[tokensource.Field]string{
	tokensource.FieldProjectID:                "GCP_PROJECT_ID",
	tokensource.FieldProjectNumber:            "GCP_PROJECT_NUMBER",
	tokensource.FieldWorkloadIdentityProvider: "GCP_WORKLOAD_IDENTITY_PROVIDER",
	tokensource.FieldServiceAccount:           "GCP_SERVICE_ACCOUNT",
	tokensource.FieldIDToken:                  "GCP_ID_TOKEN",
}

func mapAzureWIFValidationError(err error) error {
	var fieldErr *tokensource.FieldError
	if errors.As(err, &fieldErr) {
		if envName, ok := azureWIFFieldEnvNames[fieldErr.Field]; ok {
			if fieldErr.Reason == tokensource.ReasonRequired {
				return fmt.Errorf("missing %s: %w", envName, err)
			}
			return fmt.Errorf("invalid %s: %w", envName, err)
		}
	}
	return fmt.Errorf("invalid azure wif configuration: %w", err)
}

func mapGoogleWIFValidationError(err error) error {
	var fieldErr *tokensource.FieldError
	if errors.As(err, &fieldErr) {
		if envName, ok := googleWIFFieldEnvNames[fieldErr.Field]; ok {
			if fieldErr.Reason == tokensource.ReasonRequired {
				return fmt.Errorf("missing %s: %w", envName, err)
			}
			return fmt.Errorf("invalid %s: %w", envName, err)
		}
	}
	return fmt.Errorf("invalid google wif configuration: %w", err)
}

// newFederationProvider builds the Vault backed credential source.
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

// DeriveVaultSecretField derives the KV-v2 field holding the credential of provider.
func DeriveVaultSecretField(provider string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(provider), "-", "_"))
	if normalized == "" {
		return ""
	}
	return normalized + "_api_key"
}

// DeriveClaudeWIFConfig reads the Anthropic Claude federation parameters from the job environment.
func DeriveClaudeWIFConfig() claudewif.ClaudeWIFConfig {
	return claudewif.ClaudeWIFConfig{
		FederationRuleID: strings.TrimSpace(os.Getenv("ANTHROPIC_FEDERATION_RULE_ID")),
		OrganizationID:   strings.TrimSpace(os.Getenv("ANTHROPIC_ORGANIZATION_ID")),
		ServiceAccountID: strings.TrimSpace(os.Getenv("ANTHROPIC_SERVICE_ACCOUNT_ID")),
		IDToken:          strings.TrimSpace(os.Getenv("ANTHROPIC_ID_TOKEN")),
		WorkspaceID:      strings.TrimSpace(os.Getenv("ANTHROPIC_WORKSPACE_ID")),
	}
}

// DeriveAzureWIFConfig reads the Microsoft Azure federation parameters from the job environment.
func DeriveAzureWIFConfig() azurewif.AzureWIFConfig {
	return azurewif.AzureWIFConfig{
		TenantID:       strings.TrimSpace(os.Getenv("AZURE_TENANT_ID")),
		ClientID:       strings.TrimSpace(os.Getenv("AZURE_CLIENT_ID")),
		OpenAIEndpoint: strings.TrimSpace(os.Getenv("AZURE_OPENAI_ENDPOINT")),
		IDToken:        strings.TrimSpace(os.Getenv("AZURE_ID_TOKEN")),
	}
}

// DeriveGoogleWIFConfig reads the Google Cloud federation parameters from the job environment.
func DeriveGoogleWIFConfig() gcpwif.GoogleWIFConfig {
	return gcpwif.GoogleWIFConfig{
		ProjectID:                strings.TrimSpace(os.Getenv("GCP_PROJECT_ID")),
		ProjectNumber:            strings.TrimSpace(os.Getenv("GCP_PROJECT_NUMBER")),
		WorkloadIdentityProvider: strings.TrimSpace(os.Getenv("GCP_WORKLOAD_IDENTITY_PROVIDER")),
		ServiceAccount:           strings.TrimSpace(os.Getenv("GCP_SERVICE_ACCOUNT")),
		IDToken:                  strings.TrimSpace(os.Getenv("GCP_ID_TOKEN")),
	}
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

func lookupEnv(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}
