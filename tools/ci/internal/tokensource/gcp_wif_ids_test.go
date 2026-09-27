package tokensource_test

import (
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
)

func TestNewGoogleWIF_IdentifierFormats(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *tokensource.GoogleWIFConfig)
		wantErr bool
	}{
		{
			name: "project id uppercase",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ProjectID = "TEST-GCP-PROJECT"
			},
			wantErr: true,
		},
		{
			name: "project id too short",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ProjectID = "abc"
			},
			wantErr: true,
		},
		{
			name: "project id too long",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ProjectID = strings.Repeat("a", 31)
			},
			wantErr: true,
		},
		{
			name: "project number too short",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ProjectNumber = "12345"
			},
			wantErr: true,
		},
		{
			name: "project number too long",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ProjectNumber = strings.Repeat("1", 21)
			},
			wantErr: true,
		},
		{
			name: "project number negative",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ProjectNumber = "-123456789012"
			},
			wantErr: true,
		},
		{
			name: "workload identity provider wrong location",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = "projects/123456789012/locations/us-central1/workloadIdentityPools/gitlab-pool/providers/gitlab-provider"
			},
			wantErr: true,
		},
		{
			name: "workload identity provider project number mismatch",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = "projects/999999999999/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider"
			},
			wantErr: true,
		},
		{
			name: "service account wrong domain",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ServiceAccount = "sa@gmail.com"
			},
			wantErr: true,
		},
		{
			name: "service account uppercase",
			mutate: func(c *tokensource.GoogleWIFConfig) {
				c.ServiceAccount = "SA-P-EXAMPLE-APP@test-gcp-project.iam.gserviceaccount.com"
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := withTestGoogleWIFConfig(tc.mutate)
			got, err := tokensource.NewGoogleWIF(cfg)
			assertGoogleWIFBoundaryResult(t, cfg, tc.wantErr, got, err)
		})
	}
}
