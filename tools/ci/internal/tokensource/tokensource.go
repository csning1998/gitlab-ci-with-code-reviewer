package tokensource

import (
	"context"
	"fmt"

	govtokensource "gitlab.com/csning1998-lab/parent-group-governance/tools/governance/pkg/tokensource"
)

// Field identifies a configuration field in a TokenSource configuration.
type Field string

const (
	FieldFederationRuleID         Field = "federation rule id"
	FieldOrganizationID           Field = "organization id"
	FieldServiceAccountID         Field = "service account id"
	FieldIDToken                  Field = "id token"
	FieldWorkspaceID              Field = "workspace id"
	FieldTenantID                 Field = "tenant id"
	FieldClientID                 Field = "client id"
	FieldOpenAIEndpoint           Field = "openai endpoint"
	FieldProjectID                Field = "project id"
	FieldProjectNumber            Field = "project number"
	FieldWorkloadIdentityProvider Field = "workload identity provider"
	FieldServiceAccount           Field = "service account"
)

const (
	ReasonRequired      = "is required"
	ReasonInvalidFormat = "format is invalid"
)

// FieldError records an invalid or missing configuration field.
type FieldError struct {
	Provider string
	Field    Field
	Reason   string
}

func (e *FieldError) Error() string {
	provider := e.Provider
	if provider == "" {
		provider = "claude"
	}
	return fmt.Sprintf("%s wif: %s %s", provider, e.Field, e.Reason)
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
var ErrEmptyStatic = govtokensource.ErrEmptyStatic

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

// ModeDescription returns the legacy token mode label.
func (s Static) ModeDescription() string {
	return "Mode: Legacy Token"
}

// DefaultAuthMountPath specifies the default Vault JWT auth backend mount path.
const DefaultAuthMountPath = govtokensource.DefaultAuthMountPath

// VaultKVConfig specifies parameters for exchanging a JWT for a secret from Vault.
type VaultKVConfig = govtokensource.VaultKVConfig

// VaultKV resolves credentials from a HashiCorp Vault KV-v2 secret engine via JWT authentication.
type VaultKV struct {
	upstream *govtokensource.VaultKV
}

// NewVaultKV constructs a VaultKV token source with parameter validation and optional CA cert pool configuration.
func NewVaultKV(cfg VaultKVConfig) (*VaultKV, error) {
	vk, err := govtokensource.NewVaultKV(cfg)
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

// ModeDescription returns the Vault workload identity federation mode label.
func (v *VaultKV) ModeDescription() string {
	return "Mode: Workload Identity Federation (Vault)"
}

// ModeDescriptionProvider provides a mode description string.
type ModeDescriptionProvider interface {
	ModeDescription() string
}

// ModeDescription returns a human-readable mode label for the resolved token provider.
func ModeDescription(p Provider) string {
	if p == nil {
		return "Mode: None"
	}
	if mdp, ok := p.(ModeDescriptionProvider); ok {
		return mdp.ModeDescription()
	}
	return "Mode: Legacy Token"
}
