package claudewif_test

import (
	"testing"

	"ci-tools/internal/tokensource/claudewif"
)

var documentedFormatClaudeWIFConfig = claudewif.ClaudeWIFConfig{
	FederationRuleID: "fdrl_TESTRULE000000000000000",
	OrganizationID:   "abcdef01-2345-4678-89ab-cdef01234567",
	ServiceAccountID: "svac_TESTACCOUNT0000000000000",
	IDToken:          "header.payload.signature",
	WorkspaceID:      "wrkspc_TESTWORKSPACE000000000",
}

func withDocumentedFormatClaudeWIFConfig(mutate func(c *claudewif.ClaudeWIFConfig)) claudewif.ClaudeWIFConfig {
	cfg := documentedFormatClaudeWIFConfig
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func TestNewClaudeWIF_OrganizationAndWorkspaceIdentifierFormats(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *claudewif.ClaudeWIFConfig)
		wantErr bool
	}{
		{name: "documented formats"},
		{
			name:    "organization uppercase uuid",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.OrganizationID = "ABCDEF01-2345-4678-89AB-CDEF01234567" },
			wantErr: true,
		},
		{
			name:    "organization legacy prefix",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.OrganizationID = "org_abcdef" },
			wantErr: true,
		},
		{
			name:    "organization uuid without dashes",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.OrganizationID = "abcdef012345467889abcdef01234567" },
			wantErr: true,
		},
		{
			name:    "organization uuid with non hex character",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.OrganizationID = "gbcdef01-2345-4678-89ab-cdef01234567" },
			wantErr: true,
		},
		{
			name: "organization uuid with trailing character",
			mutate: func(c *claudewif.ClaudeWIFConfig) {
				c.OrganizationID = documentedFormatClaudeWIFConfig.OrganizationID + "0"
			},
			wantErr: true,
		},
		{
			name:   "workspace default literal",
			mutate: func(c *claudewif.ClaudeWIFConfig) { c.WorkspaceID = "default" },
		},
		{
			name:    "workspace uuid without prefix",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.WorkspaceID = "abcdef01-2345-4678-89ab-cdef01234567" },
			wantErr: true,
		},
		{
			name:    "workspace legacy prefix",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.WorkspaceID = "ws_123456" },
			wantErr: true,
		},
		{
			name:    "workspace uppercase prefix",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.WorkspaceID = "WRKSPC_TESTWORKSPACE000000000" },
			wantErr: true,
		},
		{
			name:    "workspace organization prefix",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.WorkspaceID = "org_TESTWORKSPACE000000000" },
			wantErr: true,
		},
		{
			name:    "workspace service account prefix",
			mutate:  func(c *claudewif.ClaudeWIFConfig) { c.WorkspaceID = "svac_TESTWORKSPACE000000000" },
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := withDocumentedFormatClaudeWIFConfig(tc.mutate)
			got, err := claudewif.NewClaudeWIF(cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewClaudeWIF() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && got == nil {
				t.Fatal("NewClaudeWIF() returned nil without error")
			}
		})
	}
}
