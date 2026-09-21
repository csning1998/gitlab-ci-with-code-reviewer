package tokensource

import (
	"gitlab.com/csning1998-lab/parent-group-governance/tools/governance/pkg/tokensource"
)

// Provider yields the credential sent in the Authorization header of a provider request.
type Provider = tokensource.Provider

// Static holds a credential supplied verbatim by the environment.
type Static = tokensource.Static

// ErrEmptyStatic is returned when a Static credential is empty.
var ErrEmptyStatic = tokensource.ErrEmptyStatic

// DefaultAuthMountPath specifies the default Vault JWT auth backend mount path.
const DefaultAuthMountPath = tokensource.DefaultAuthMountPath

// VaultKVConfig specifies parameters for exchanging a JWT for a secret from Vault.
type VaultKVConfig = tokensource.VaultKVConfig

// VaultKV resolves credentials from a HashiCorp Vault KV-v2 secret engine via JWT authentication.
type VaultKV = tokensource.VaultKV

// NewVaultKV constructs a VaultKV token source with parameter validation and optional CA cert pool configuration.
func NewVaultKV(cfg VaultKVConfig) (*VaultKV, error) {
	return tokensource.NewVaultKV(cfg)
}
