package testutil

import (
	"context"
	"errors"

	"ci-tools/internal/tokensource"
)

// GitLab token values for tests. A test asserts the identity of a value alone, never its format.
const (
	GitLabToken       = "token"
	SecretGitLabToken = "secret-token"
)

// InsufficientGranularScopeBody is the 403 body which GitLab.com returns to a fine-grained token
// lacking Personal Access Token Read on /personal_access_tokens/self.
const InsufficientGranularScopeBody = `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following user permissions: [Personal Access Token: Read]."}`

// Ptr returns a pointer to the value passed in.
func Ptr[T any](v T) *T {
	return &v
}

// FailingTokenProvider rejects token requests for credential failure testing.
type FailingTokenProvider struct{}

func (FailingTokenProvider) Token(context.Context) (string, error) {
	return "", errors.New("exchange rejected")
}

func (FailingTokenProvider) FetchCredential(context.Context) (tokensource.Credential, error) {
	return tokensource.Credential{}, errors.New("exchange rejected")
}
