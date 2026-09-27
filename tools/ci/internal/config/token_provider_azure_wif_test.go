package config

import (
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
)

var defaultTestAzureWIFConfig = tokensource.AzureWIFConfig{
	TenantID:       "663bcc2a-0747-4e40-bc5d-6d8c4450e1f3",
	ClientID:       "11111111-2222-3333-4444-555555555555",
	OpenAIEndpoint: "https://oai-csning1998-lab.openai.azure.com/",
	IDToken:        "header.payload.signature",
}

// setAzureWIFEnvFrom declares the Azure WIF variables of cfg for the current test.
func setAzureWIFEnvFrom(t *testing.T, cfg tokensource.AzureWIFConfig) {
	t.Helper()
	t.Setenv("AZURE_TENANT_ID", cfg.TenantID)
	t.Setenv("AZURE_CLIENT_ID", cfg.ClientID)
	t.Setenv("AZURE_OPENAI_ENDPOINT", cfg.OpenAIEndpoint)
	t.Setenv("AZURE_ID_TOKEN", cfg.IDToken)
}

// setAzureWIFEnv declares a complete Azure WIF configuration for the current test.
func setAzureWIFEnv(t *testing.T) {
	t.Helper()
	setAzureWIFEnvFrom(t, defaultTestAzureWIFConfig)
}

func TestDeriveAzureWIFConfig_ValueBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func(t *testing.T)
		want     tokensource.AzureWIFConfig
	}{
		{
			name:     "unset",
			setupEnv: func(t *testing.T) {},
			want:     tokensource.AzureWIFConfig{},
		},
		{
			name: "surrounding whitespace trimmed",
			setupEnv: func(t *testing.T) {
				t.Setenv("AZURE_TENANT_ID", "  663bcc2a-0747-4e40-bc5d-6d8c4450e1f3\n")
				t.Setenv("AZURE_CLIENT_ID", "\t11111111-2222-3333-4444-555555555555  ")
				t.Setenv("AZURE_OPENAI_ENDPOINT", "  https://oai-csning1998-lab.openai.azure.com/\t")
				t.Setenv("AZURE_ID_TOKEN", "\nheader.payload.signature  ")
			},
			want: defaultTestAzureWIFConfig,
		},
		{
			name: "whitespace only becomes empty",
			setupEnv: func(t *testing.T) {
				t.Setenv("AZURE_TENANT_ID", " \t")
				t.Setenv("AZURE_CLIENT_ID", "\n")
				t.Setenv("AZURE_OPENAI_ENDPOINT", "  ")
				t.Setenv("AZURE_ID_TOKEN", " \t\n")
			},
			want: tokensource.AzureWIFConfig{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			if got := DeriveAzureWIFConfig(); got != tc.want {
				t.Errorf("DeriveAzureWIFConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveTokenProvider_AzureWIF_ProviderSpellings(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{name: "hyphen lower", provider: "azure-openai"},
		{name: "underscore lower", provider: "azure_openai"},
		{name: "mixed case", provider: "Azure-OpenAI"},
		{name: "padded", provider: "  azure-openai\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setAzureWIFEnv(t)

			provider, err := ResolveTokenProvider(tc.provider)
			if err != nil {
				t.Fatalf("ResolveTokenProvider(%q) returned an unexpected error: %v", tc.provider, err)
			}
			wif, ok := provider.(*tokensource.AzureWIF)
			if !ok {
				t.Fatalf("provider has type %T, want *tokensource.AzureWIF", provider)
			}
			if got := wif.Config(); got.ClientID != defaultTestAzureWIFConfig.ClientID || got.TenantID != defaultTestAzureWIFConfig.TenantID {
				t.Errorf("Config() = %+v, want %+v", got, defaultTestAzureWIFConfig)
			}
		})
	}
}

func TestResolveTokenProvider_AzureWIF_PartialConfigurationDoesNotFallBack(t *testing.T) {
	tests := []struct {
		name       string
		setupEnv   func(t *testing.T)
		wantSubstr string
	}{
		{
			name: "client id only with static key",
			setupEnv: func(t *testing.T) {
				t.Setenv("AZURE_CLIENT_ID", defaultTestAzureWIFConfig.ClientID)
				t.Setenv("AZURE_OPENAI_API_KEY", "static-key")
			},
			wantSubstr: "AZURE_TENANT_ID",
		},
		{
			name: "missing id token with vault",
			setupEnv: func(t *testing.T) {
				setAzureWIFEnv(t)
				t.Setenv("AZURE_ID_TOKEN", "")
				setVaultEnv(t)
			},
			wantSubstr: "AZURE_ID_TOKEN",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			_, err := ResolveTokenProvider("azure-openai")
			if err == nil {
				t.Fatalf("ResolveTokenProvider(...) succeeded unexpectedly when %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("ResolveTokenProvider(...) error = %q, want error containing %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}
