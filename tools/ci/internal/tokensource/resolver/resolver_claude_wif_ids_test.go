package resolver

import (
	"errors"
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
	"ci-tools/internal/tokensource/claudewif"
)

var documentedFormatClaudeWIFConfig = ClaudeWIFConfig{
	FederationRuleID: "fdrl_TESTRULE000000000000000",
	OrganizationID:   "abcdef01-2345-4678-89ab-cdef01234567",
	ServiceAccountID: "svac_TESTACCOUNT0000000000000",
	IDToken:          "header.payload.signature",
	WorkspaceID:      "wrkspc_TESTWORKSPACE000000000",
}

func TestResolveTokenProvider_ClaudeWIF_DocumentedIdentifierFormats(t *testing.T) {
	clearTokenProviderEnv(t)
	setClaudeWIFEnvFrom(t, documentedFormatClaudeWIFConfig)

	provider, err := ResolveTokenProvider("claude")
	if err != nil {
		t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
	}
	wif, ok := provider.(*claudewif.ClaudeWIF)
	if !ok {
		t.Fatalf("provider has type %T, want *tokensource.ClaudeWIF", provider)
	}
	if got := wif.Config(); got != documentedFormatClaudeWIFConfig {
		t.Errorf("Config() = %+v, want %+v", got, documentedFormatClaudeWIFConfig)
	}
}

func TestResolveTokenProvider_ClaudeWIF_LegacyOrganizationPrefixNamesEnvironmentVariable(t *testing.T) {
	cfg := documentedFormatClaudeWIFConfig
	cfg.OrganizationID = "org_abcdef"
	clearTokenProviderEnv(t)
	setClaudeWIFEnvFrom(t, cfg)

	_, err := ResolveTokenProvider("claude")
	fieldErr, ok := errors.AsType[*tokensource.FieldError](err)
	if !ok || fieldErr.Field != tokensource.FieldOrganizationID {
		t.Fatalf("ResolveTokenProvider(...) error = %v, want a FieldError for the organization id", err)
	}
	if want := "invalid ANTHROPIC_ORGANIZATION_ID"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want error containing %q", err.Error(), want)
	}
}
