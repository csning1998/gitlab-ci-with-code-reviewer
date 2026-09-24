package tokensource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitlab.com/csning1998-lab/parent-group-governance/tools/governance/pkg/tokensource"
)

// Field identifies a configuration field in a TokenSource configuration.
type Field string

const (
	FieldFederationRuleID Field = "federation rule id"
	FieldOrganizationID   Field = "organization id"
	FieldServiceAccountID Field = "service account id"
	FieldIDToken          Field = "id token"
	FieldWorkspaceID      Field = "workspace id"
)

const (
	ReasonRequired      = "is required"
	ReasonInvalidFormat = "format is invalid"
)

// FieldError records an invalid or missing configuration field.
type FieldError struct {
	Field  Field
	Reason string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("claude wif: %s %s", e.Field, e.Reason)
}

func newRequiredFieldError(f Field) error {
	return &FieldError{Field: f, Reason: ReasonRequired}
}

func newInvalidFormatFieldError(f Field) error {
	return &FieldError{Field: f, Reason: ReasonInvalidFormat}
}

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
		return nil, newRequiredFieldError(FieldFederationRuleID)
	}
	if !validatePrefixedID(cfg.FederationRuleID, "fdrl_") {
		return nil, newInvalidFormatFieldError(FieldFederationRuleID)
	}
	if strings.TrimSpace(cfg.OrganizationID) == "" {
		return nil, newRequiredFieldError(FieldOrganizationID)
	}
	if !validatePrefixedID(cfg.OrganizationID, "org_") {
		return nil, newInvalidFormatFieldError(FieldOrganizationID)
	}
	if strings.TrimSpace(cfg.ServiceAccountID) == "" {
		return nil, newRequiredFieldError(FieldServiceAccountID)
	}
	if !validatePrefixedID(cfg.ServiceAccountID, "svac_") {
		return nil, newInvalidFormatFieldError(FieldServiceAccountID)
	}
	if strings.TrimSpace(cfg.IDToken) == "" {
		return nil, newRequiredFieldError(FieldIDToken)
	}
	if !validateIDToken(cfg.IDToken) {
		return nil, newInvalidFormatFieldError(FieldIDToken)
	}
	if strings.TrimSpace(cfg.WorkspaceID) != "" {
		if !validatePrefixedID(cfg.WorkspaceID, "wrk_") {
			return nil, newInvalidFormatFieldError(FieldWorkspaceID)
		}
	}
	return &ClaudeWIF{cfg: cfg}, nil
}

// Config returns the underlying ClaudeWIFConfig.
func (c *ClaudeWIF) Config() ClaudeWIFConfig {
	return c.cfg
}

// FetchCredential returns the ID token with KindBearer.
func (c *ClaudeWIF) FetchCredential(_ context.Context) (Credential, error) {
	if strings.TrimSpace(c.cfg.IDToken) == "" {
		return Credential{}, errors.New("claude wif: id token is empty")
	}
	return Credential{Value: c.cfg.IDToken, Kind: KindBearer}, nil
}

// CredentialKind classifies the credential transported in the request.
type CredentialKind string

const (
	KindAPIKey CredentialKind = "api_key" // KindAPIKey indicates a static provider API key.
	KindBearer CredentialKind = "bearer"  // KindBearer indicates a bearer access token.
)

// Credential holds a resolved credential and its kind.
type Credential struct {
	Value string
	Kind  CredentialKind
}

// Provider yields the credential sent in the appropriate header of a provider request.
type Provider interface {
	FetchCredential(ctx context.Context) (Credential, error)
}

// Static holds a credential supplied verbatim by the environment.
type Static string

// ErrEmptyStatic is returned when a Static credential is empty.
var ErrEmptyStatic = tokensource.ErrEmptyStatic

// FetchCredential returns the static credential with KindAPIKey.
func (s Static) FetchCredential(ctx context.Context) (Credential, error) {
	if s == "" {
		return Credential{}, ErrEmptyStatic
	}
	return Credential{Value: string(s), Kind: KindAPIKey}, nil
}

// Token returns the static credential string.
func (s Static) Token(ctx context.Context) (string, error) {
	if s == "" {
		return "", ErrEmptyStatic
	}
	return string(s), nil
}

// DefaultAuthMountPath specifies the default Vault JWT auth backend mount path.
const DefaultAuthMountPath = tokensource.DefaultAuthMountPath

// VaultKVConfig specifies parameters for exchanging a JWT for a secret from Vault.
type VaultKVConfig = tokensource.VaultKVConfig

// VaultKV resolves credentials from a HashiCorp Vault KV-v2 secret engine via JWT authentication.
type VaultKV struct {
	upstream *tokensource.VaultKV
}

// NewVaultKV constructs a VaultKV token source with parameter validation and optional CA cert pool configuration.
func NewVaultKV(cfg VaultKVConfig) (*VaultKV, error) {
	vk, err := tokensource.NewVaultKV(cfg)
	if err != nil {
		return nil, err
	}
	return &VaultKV{upstream: vk}, nil
}

// FetchCredential exchanges a JWT for a secret from Vault, tagged with KindAPIKey.
func (v *VaultKV) FetchCredential(ctx context.Context) (Credential, error) {
	tok, err := v.upstream.Token(ctx)
	if err != nil {
		return Credential{}, err
	}
	return Credential{Value: tok, Kind: KindAPIKey}, nil
}

// Token delegates to the upstream VaultKV Token method.
func (v *VaultKV) Token(ctx context.Context) (string, error) {
	return v.upstream.Token(ctx)
}
