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

// SelfPath is the GitLab endpoint which reports the type and scopes of the calling token.
const SelfPath = "/personal_access_tokens/self"

// LegacyTokenSelfBody is the 200 body which /personal_access_tokens/self returns to a legacy token.
const LegacyTokenSelfBody = `{"active":true,"scopes":["api"]}`

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

// HandleFineGrainedSelf answers /personal_access_tokens/self as GitLab.com answers a fine-grained
// token lacking Personal Access Token Read, and passes every other request to next.
func HandleFineGrainedSelf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == SelfPath {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, InsufficientGranularScopeBody)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewLegacyTokenServer answers /personal_access_tokens/self as GitLab answers a legacy token, and
// counts every other request, which a command MUST NOT send after the rejection.
func NewLegacyTokenServer(t testing.TB) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var otherRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == SelfPath {
			_, _ = io.WriteString(w, LegacyTokenSelfBody)
			return
		}
		otherRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, &otherRequests
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
