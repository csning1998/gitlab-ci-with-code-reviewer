package resolver

import (
	"strings"
	"testing"

	"ci-tools/internal/tokensource/gcpwif"
)

var defaultTestGoogleWIFConfig = GoogleWIFConfig{
	ProjectID:                "test-gcp-project",
	ProjectNumber:            "123456789012",
	WorkloadIdentityProvider: "projects/123456789012/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider",
	ServiceAccount:           "sa-p-example-app@test-gcp-project.iam.gserviceaccount.com",
	IDToken:                  "header.payload.signature",
}

// setGoogleWIFEnvFrom declares the Google WIF variables of cfg for the current test.
func setGoogleWIFEnvFrom(t *testing.T, cfg GoogleWIFConfig) {
	t.Helper()
	t.Setenv("GCP_PROJECT_ID", cfg.ProjectID)
	t.Setenv("GCP_PROJECT_NUMBER", cfg.ProjectNumber)
	t.Setenv("GCP_WORKLOAD_IDENTITY_PROVIDER", cfg.WorkloadIdentityProvider)
	t.Setenv("GCP_SERVICE_ACCOUNT", cfg.ServiceAccount)
	t.Setenv("GCP_ID_TOKEN", cfg.IDToken)
}

// setGoogleWIFEnv declares a complete Google WIF configuration for the current test.
func setGoogleWIFEnv(t *testing.T) {
	t.Helper()
	setGoogleWIFEnvFrom(t, defaultTestGoogleWIFConfig)
}

func TestDeriveGoogleWIFConfig_ValueBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func(t *testing.T)
		want     GoogleWIFConfig
	}{
		{
			name:     "unset",
			setupEnv: func(t *testing.T) {},
			want:     GoogleWIFConfig{},
		},
		{
			name: "surrounding whitespace trimmed",
			setupEnv: func(t *testing.T) {
				t.Setenv("GCP_PROJECT_ID", "  test-gcp-project\n")
				t.Setenv("GCP_PROJECT_NUMBER", "\t123456789012  ")
				t.Setenv("GCP_WORKLOAD_IDENTITY_PROVIDER", "  projects/123456789012/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider\t")
				t.Setenv("GCP_SERVICE_ACCOUNT", "\nsa-p-example-app@test-gcp-project.iam.gserviceaccount.com  ")
				t.Setenv("GCP_ID_TOKEN", "  header.payload.signature\n")
			},
			want: defaultTestGoogleWIFConfig,
		},
		{
			name: "whitespace only becomes empty",
			setupEnv: func(t *testing.T) {
				t.Setenv("GCP_PROJECT_ID", " \t")
				t.Setenv("GCP_PROJECT_NUMBER", "\n")
				t.Setenv("GCP_WORKLOAD_IDENTITY_PROVIDER", "  ")
				t.Setenv("GCP_SERVICE_ACCOUNT", "\u00a0")
				t.Setenv("GCP_ID_TOKEN", " \t\n")
			},
			want: GoogleWIFConfig{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			got := DeriveGoogleWIFConfig()
			if got.ProjectID != tc.want.ProjectID ||
				got.ProjectNumber != tc.want.ProjectNumber ||
				got.WorkloadIdentityProvider != tc.want.WorkloadIdentityProvider ||
				got.ServiceAccount != tc.want.ServiceAccount ||
				got.IDToken != tc.want.IDToken {
				t.Errorf("DeriveGoogleWIFConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveTokenProvider_GoogleWIF_ProviderSpellings(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{name: "gemini lower", provider: "gemini"},
		{name: "gemini mixed", provider: "Gemini"},
		{name: "google lower", provider: "google"},
		{name: "padded", provider: "  gemini\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			setGoogleWIFEnv(t)

			provider, err := ResolveTokenProvider(tc.provider)
			if err != nil {
				t.Fatalf("ResolveTokenProvider(%q) returned an unexpected error: %v", tc.provider, err)
			}
			wif, ok := provider.(*gcpwif.GoogleWIF)
			if !ok {
				t.Fatalf("provider has type %T, want *gcpwif.GoogleWIF", provider)
			}
			if got := wif.Config(); got.ProjectID != defaultTestGoogleWIFConfig.ProjectID || got.ServiceAccount != defaultTestGoogleWIFConfig.ServiceAccount {
				t.Errorf("Config() = %+v, want %+v", got, defaultTestGoogleWIFConfig)
			}
		})
	}
}

func TestResolveTokenProvider_GoogleWIF_PartialConfigurationDoesNotFallBack(t *testing.T) {
	tests := []struct {
		name       string
		setupEnv   func(t *testing.T)
		wantSubstr string
	}{
		{
			name: "provider only with static key",
			setupEnv: func(t *testing.T) {
				t.Setenv("GCP_WORKLOAD_IDENTITY_PROVIDER", defaultTestGoogleWIFConfig.WorkloadIdentityProvider)
				t.Setenv("GEMINI_API_KEY", "static-key")
			},
			wantSubstr: "GCP_PROJECT_ID",
		},
		{
			name: "provider and project id with vault",
			setupEnv: func(t *testing.T) {
				t.Setenv("GCP_WORKLOAD_IDENTITY_PROVIDER", defaultTestGoogleWIFConfig.WorkloadIdentityProvider)
				t.Setenv("GCP_PROJECT_ID", defaultTestGoogleWIFConfig.ProjectID)
				setVaultEnv(t)
			},
			wantSubstr: "GCP_PROJECT_NUMBER",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearTokenProviderEnv(t)
			tc.setupEnv(t)

			_, err := ResolveTokenProvider("gemini")
			if err == nil {
				t.Fatalf("ResolveTokenProvider(...) succeeded unexpectedly when %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("ResolveTokenProvider(...) error = %q, want error containing %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}
