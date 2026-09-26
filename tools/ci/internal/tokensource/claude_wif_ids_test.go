package tokensource_test

import (
	"testing"

	"ci-tools/internal/tokensource"
)

var documentedFormatClaudeWIFConfig = tokensource.ClaudeWIFConfig{
	FederationRuleID: "fdrl_TESTRULE000000000000000",
	OrganizationID:   "abcdef01-2345-4678-89ab-cdef01234567",
	ServiceAccountID: "svac_TESTACCOUNT0000000000000",
	IDToken:          "header.payload.signature",
	WorkspaceID:      "wrkspc_TESTWORKSPACE000000000",
}

func withDocumentedFormatClaudeWIFConfig(mutate func(c *tokensource.ClaudeWIFConfig)) tokensource.ClaudeWIFConfig {
	cfg := documentedFormatClaudeWIFConfig
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func TestNewClaudeWIF_OrganizationAndWorkspaceIdentifierFormats(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *tokensource.ClaudeWIFConfig)
		wantErr bool
	}{
		{name: "documented formats"},
		{
			name:    "organization uppercase uuid",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.OrganizationID = "ABCDEF01-2345-4678-89AB-CDEF01234567" },
			wantErr: true,
		},
		{
			name:    "organization legacy prefix",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.OrganizationID = "org_abcdef" },
			wantErr: true,
		},
		{
			name:    "organization uuid without dashes",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.OrganizationID = "abcdef012345467889abcdef01234567" },
			wantErr: true,
		},
		{
			name:    "organization uuid with non hex character",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.OrganizationID = "gbcdef01-2345-4678-89ab-cdef01234567" },
			wantErr: true,
		},
		{
			name: "organization uuid with trailing character",
			mutate: func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = documentedFormatClaudeWIFConfig.OrganizationID + "0"
			},
			wantErr: true,
		},
		{
			name:   "workspace default literal",
			mutate: func(c *tokensource.ClaudeWIFConfig) { c.WorkspaceID = "default" },
		},
		{
			name:    "workspace legacy prefix",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.WorkspaceID = "wrk_optional_123" },
			wantErr: true,
		},
		{
			name:    "workspace uppercase default literal",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.WorkspaceID = "DEFAULT" },
			wantErr: true,
		},
		{
			name:    "workspace prefix only",
			mutate:  func(c *tokensource.ClaudeWIFConfig) { c.WorkspaceID = "wrkspc_" },
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := withDocumentedFormatClaudeWIFConfig(tc.mutate)
			got, err := tokensource.NewClaudeWIF(cfg)
			assertBoundaryResult(t, cfg, tc.wantErr, got, err)
		})
	}
}
