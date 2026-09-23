package tokensource

import (
	"context"

	"gitlab.com/csning1998-lab/parent-group-governance/tools/governance/pkg/tokensource"
)

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
