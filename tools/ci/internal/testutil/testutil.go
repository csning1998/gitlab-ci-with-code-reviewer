package testutil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ci-tools/internal/gitlabtoken"
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

// NewStubToken returns raw as a FineGrainedToken through gitlabtoken.New against a stub self
// endpoint, since no other constructor exists.
func NewStubToken(t testing.TB, raw string) gitlabtoken.FineGrainedToken {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, InsufficientGranularScopeBody)
	}))
	defer server.Close()

	token, err := gitlabtoken.New(context.Background(), server.URL, raw)
	if err != nil {
		t.Fatalf("gitlabtoken.New(...) against the stub self endpoint failed: %v", err)
	}
	return token
}

// NewConnectionCountingServer answers every request with status and body, and counts the TCP
// connections which clients open against the server.
func NewConnectionCountingServer(t testing.TB, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	return server, &connections
}

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
