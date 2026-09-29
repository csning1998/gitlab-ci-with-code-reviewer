package tokensource_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ci-tools/internal/tokensource"
	"ci-tools/internal/tokensource/azurewif"
	"ci-tools/internal/tokensource/claudewif"
	"ci-tools/internal/tokensource/gcpwif"
)

func newTestClaudeWIF(t *testing.T) *claudewif.ClaudeWIF {
	t.Helper()
	src, err := claudewif.NewClaudeWIF(claudewif.ClaudeWIFConfig{
		FederationRuleID: "fdrl_123",
		OrganizationID:   "abcdef01-2345-4678-89ab-cdef01234567",
		ServiceAccountID: "svac_123",
		IDToken:          "jwt_token",
	})
	if err != nil {
		t.Fatalf("NewClaudeWIF() error = %v", err)
	}
	return src
}

func newTestAzureWIF(t *testing.T) *azurewif.AzureWIF {
	t.Helper()
	src, err := azurewif.NewAzureWIF(azurewif.AzureWIFConfig{
		TenantID:       "663bcc2a-0747-4e40-bc5d-6d8c4450e1f3",
		ClientID:       "11111111-2222-3333-4444-555555555555",
		OpenAIEndpoint: "https://oai-csning1998-lab.openai.azure.com/",
		IDToken:        "jwt.token.here",
	})
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}
	return src
}

func newTestGoogleWIF(t *testing.T) *gcpwif.GoogleWIF {
	t.Helper()
	src, err := gcpwif.NewGoogleWIF(gcpwif.GoogleWIFConfig{
		ProjectID:                "test-gcp-project",
		ProjectNumber:            "123456789012",
		WorkloadIdentityProvider: "projects/123456789012/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider",
		ServiceAccount:           "sa-p-example-app@test-gcp-project.iam.gserviceaccount.com",
		IDToken:                  "jwt.token.gcp.test",
	})
	if err != nil {
		t.Fatalf("NewGoogleWIF() error = %v", err)
	}
	return src
}

func TestStatic_FetchCredential(t *testing.T) {
	got, err := tokensource.Static("static-credential").FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	want := tokensource.Credential{
		Value: "static-credential",
		Kind:  tokensource.KindAPIKey,
	}
	if got != want {
		t.Errorf("FetchCredential() = %+v, want %+v", got, want)
	}

	_, err = tokensource.Static("").FetchCredential(context.Background())
	if !errors.Is(err, tokensource.ErrEmptyStatic) {
		t.Errorf("FetchCredential() error = %v, want ErrEmptyStatic", err)
	}
}

func TestStatic_Token(t *testing.T) {
	got, err := tokensource.Static("static-credential").Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != "static-credential" {
		t.Errorf("Token() = %q, want %q", got, "static-credential")
	}

	_, err = tokensource.Static("").Token(context.Background())
	if !errors.Is(err, tokensource.ErrEmptyStatic) {
		t.Errorf("Token() error = %v, want ErrEmptyStatic", err)
	}
}

var validTestVaultKVConfig = tokensource.VaultKVConfig{
	VaultAddr:   "https://vault.example.com",
	Role:        "ci-role",
	JWT:         "jwt-token",
	MountPath:   "secret",
	SecretPath:  "ci/credentials",
	SecretField: "api_key",
}

func newTestVaultKV(t *testing.T) *tokensource.VaultKV {
	t.Helper()
	vk, err := tokensource.NewVaultKV(validTestVaultKVConfig)
	if err != nil {
		t.Fatalf("NewVaultKV() unexpected error = %v", err)
	}
	return vk
}

func TestNewVaultKV_Delegation(t *testing.T) {
	_, err := tokensource.NewVaultKV(tokensource.VaultKVConfig{})
	if err == nil {
		t.Fatal("NewVaultKV() with empty config expected validation error, got nil")
	}

	vk := newTestVaultKV(t)
	if vk == nil {
		t.Fatal("NewVaultKV() returned nil instance")
	}
}

func TestStatic_ConcurrentCallsReturnTheSameCredential(t *testing.T) {
	const callers = 128

	source := tokensource.Static("shared-credential")
	results := make([]tokensource.Credential, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = source.FetchCredential(context.Background())
		}(i)
	}
	wg.Wait()

	want := tokensource.Credential{
		Value: "shared-credential",
		Kind:  tokensource.KindAPIKey,
	}
	for i := range callers {
		if errs[i] != nil || results[i] != want {
			t.Errorf("caller %d: FetchCredential() = %+v, %v", i, results[i], errs[i])
		}
	}
}

func TestModeDescription(t *testing.T) {
	tests := []struct {
		name     string
		provider tokensource.Provider
		want     string
	}{
		{
			name:     "nil provider describes none",
			provider: nil,
			want:     "Mode: None",
		},
		{
			name:     "static token source describes legacy mode",
			provider: tokensource.Static("api-key"),
			want:     "Mode: Legacy Token",
		},
		{
			name:     "claude wif token source describes claude native federation mode",
			provider: newTestClaudeWIF(t),
			want:     "Mode: Workload Identity Federation (Claude Native)",
		},
		{
			name:     "vault kv token source describes vault federation mode",
			provider: newTestVaultKV(t),
			want:     "Mode: Workload Identity Federation (Vault)",
		},
		{
			name:     "azure wif token source describes azure federation mode",
			provider: newTestAzureWIF(t),
			want:     "Mode: Workload Identity Federation (Azure OpenAI Native)",
		},
		{
			name:     "google wif token source describes google federation mode",
			provider: newTestGoogleWIF(t),
			want:     "Mode: Workload Identity Federation (Google Cloud Native)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokensource.ModeDescription(tc.provider); got != tc.want {
				t.Errorf("ModeDescription() = %q, want %q", got, tc.want)
			}
		})
	}
}
