// Package tokensource resolves the bearer credential presented to an LLM provider at request
// time. Federated providers mint a token per job, whereas providers without any federation
// mechanism supply a static value; both are consumed through the same interface.
package tokensource

import (
	"context"
	"errors"
)

// Provider yields the credential sent in the Authorization header of a provider request.
type Provider interface {
	Token(ctx context.Context) (string, error)
}

// Static holds a credential supplied verbatim by the environment, applicable to providers
// offering no workload identity federation.
type Static string

var errEmptyStatic = errors.New("tokensource: static credential is empty")

func (s Static) Token(context.Context) (string, error) {
	if s == "" {
		return "", errEmptyStatic
	}
	return string(s), nil
}
