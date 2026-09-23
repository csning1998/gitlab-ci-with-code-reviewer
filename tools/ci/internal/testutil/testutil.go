package testutil

import (
	"context"
	"errors"

	"ci-tools/internal/tokensource"
)

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
