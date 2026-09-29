package claudewif

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"ci-tools/internal/tokensource"
)

// ClaudeWIFConfig specifies parameters for Anthropic Claude Workload Identity Federation.
type ClaudeWIFConfig struct {
	FederationRuleID string
	OrganizationID   string
	ServiceAccountID string
	IDToken          string
	WorkspaceID      string
}

// ClaudeWIF holds parameters for Anthropic Claude Workload Identity Federation.
type ClaudeWIF struct {
	cfg ClaudeWIFConfig
}

// isSafeIDRune checks whether r is an ASCII alphanumeric character, underscore, or hyphen.
func isSafeIDRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
}

// isJWTRune checks whether r is a valid JWT character (alphanumeric, dot, underscore, or hyphen).
func isJWTRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

// validatePrefixedID checks that id has prefix followed by 1..64 alphanumeric or underscore/hyphen characters.
func validatePrefixedID(id, prefix string) bool {
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	suffix := id[len(prefix):]
	if len(suffix) == 0 || len(suffix) > 64 {
		return false
	}
	for _, r := range suffix {
		if !isSafeIDRune(r) {
			return false
		}
	}
	return true
}

var organizationIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// isValidOrganizationID checks that id is a lowercase UUID.
func isValidOrganizationID(id string) bool {
	return organizationIDPattern.MatchString(id)
}

// isValidWorkspaceID checks that id is a wrkspc_ tagged identifier or the literal default.
func isValidWorkspaceID(id string) bool {
	return id == "default" || validatePrefixedID(id, "wrkspc_")
}

// validateIDToken checks that token length is within 8..4096 and contains only base64url/JWT characters.
func validateIDToken(token string) bool {
	if len(token) < 8 || len(token) > 4096 {
		return false
	}
	for _, r := range token {
		if !isJWTRune(r) {
			return false
		}
	}
	return true
}

// NewClaudeWIF constructs a ClaudeWIF provider after validating required fields.
func NewClaudeWIF(cfg ClaudeWIFConfig) (*ClaudeWIF, error) {
	if strings.TrimSpace(cfg.FederationRuleID) == "" {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldFederationRuleID, Reason: tokensource.ReasonRequired}
	}
	if !validatePrefixedID(cfg.FederationRuleID, "fdrl_") {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldFederationRuleID, Reason: tokensource.ReasonInvalidFormat}
	}
	if strings.TrimSpace(cfg.OrganizationID) == "" {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldOrganizationID, Reason: tokensource.ReasonRequired}
	}
	if !isValidOrganizationID(cfg.OrganizationID) {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldOrganizationID, Reason: tokensource.ReasonInvalidFormat}
	}
	if strings.TrimSpace(cfg.ServiceAccountID) == "" {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldServiceAccountID, Reason: tokensource.ReasonRequired}
	}
	if !validatePrefixedID(cfg.ServiceAccountID, "svac_") {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldServiceAccountID, Reason: tokensource.ReasonInvalidFormat}
	}
	if strings.TrimSpace(cfg.IDToken) == "" {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldIDToken, Reason: tokensource.ReasonRequired}
	}
	if !validateIDToken(cfg.IDToken) {
		return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldIDToken, Reason: tokensource.ReasonInvalidFormat}
	}
	if strings.TrimSpace(cfg.WorkspaceID) != "" {
		if !isValidWorkspaceID(cfg.WorkspaceID) {
			return nil, &tokensource.FieldError{Provider: "claude", Field: tokensource.FieldWorkspaceID, Reason: tokensource.ReasonInvalidFormat}
		}
	}
	return &ClaudeWIF{cfg: cfg}, nil
}

// Config returns the underlying ClaudeWIFConfig.
func (c *ClaudeWIF) Config() ClaudeWIFConfig {
	return c.cfg
}

// FetchCredential returns the ID token with KindBearer.
func (c *ClaudeWIF) FetchCredential(_ context.Context) (tokensource.Credential, error) {
	if strings.TrimSpace(c.cfg.IDToken) == "" {
		return tokensource.Credential{}, errors.New("claude wif: id token is empty")
	}
	return tokensource.Credential{Value: c.cfg.IDToken, Kind: tokensource.KindBearer}, nil
}

// ModeDescription returns the Anthropic Claude native workload identity federation mode label.
func (c *ClaudeWIF) ModeDescription() string {
	return "Mode: Workload Identity Federation (Claude Native)"
}
