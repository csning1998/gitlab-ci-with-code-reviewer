package gitlabtoken_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ci-tools/internal/gitlabtoken"
	"ci-tools/internal/testutil"
)

const (
	selfPath        = "/personal_access_tokens/self"
	legacyTokenSelf = `{"active":true,"scopes":["api"]}`
)

// newSelfServer answers every request with status and body.
func newSelfServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNew_FineGrainedTokenWithoutSelfReadPasses(t *testing.T) {
	var gotMethod, gotPath, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, testutil.InsufficientGranularScopeBody)
	}))
	t.Cleanup(server.Close)

	token, err := gitlabtoken.New(context.Background(), server.URL, testutil.SecretGitLabToken)
	if err != nil {
		t.Fatalf("New(...) returned an unexpected error: %v", err)
	}
	if token.Value() != testutil.SecretGitLabToken {
		t.Errorf("New(...).Value() = %q, want %q", token.Value(), testutil.SecretGitLabToken)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("request method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != selfPath {
		t.Errorf("request path = %q, want %q", gotPath, selfPath)
	}
	if gotToken != testutil.SecretGitLabToken {
		t.Errorf("PRIVATE-TOKEN header = %q, want %q", gotToken, testutil.SecretGitLabToken)
	}
}

func TestNew_FineGrainedTokenWithSelfReadPasses(t *testing.T) {
	server := newSelfServer(t, http.StatusOK, `{"active":true,"scopes":[],"granular_scopes":[{"access":"selected_memberships","permissions":["read_repository_tag"]}]}`)

	token, err := gitlabtoken.New(context.Background(), server.URL, testutil.GitLabToken)
	if err != nil {
		t.Fatalf("New(...) returned an unexpected error: %v", err)
	}
	if token.Value() != testutil.GitLabToken {
		t.Errorf("New(...).Value() = %q, want %q", token.Value(), testutil.GitLabToken)
	}
}

func TestNew_EmptyTokenRejectedWithoutRequest(t *testing.T) {
	var requested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requested.Store(true)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, testutil.InsufficientGranularScopeBody)
	}))
	t.Cleanup(server.Close)

	token, err := gitlabtoken.New(context.Background(), server.URL, "")
	if err == nil {
		t.Fatal("New(...) accepted an empty token")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("New(...) error = %q, want it to report the empty token", err.Error())
	}
	if token.Value() != "" {
		t.Errorf("New(...).Value() = %q, want empty on error", token.Value())
	}
	if requested.Load() {
		t.Error("New(...) contacted the API for an empty token")
	}
}

// Any token which answers the self endpoint without granular scopes is a legacy token, whatever its scopes.
func TestNew_LegacyTokenRejected(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "api", body: legacyTokenSelf},
		{name: "read_api", body: `{"active":true,"scopes":["read_api"]}`},
		{name: "read_repository", body: `{"active":true,"scopes":["read_repository"]}`},
		{name: "empty granular scopes", body: `{"active":true,"scopes":["api"],"granular_scopes":[]}`},
		{name: "null granular scopes", body: `{"active":true,"scopes":["api"],"granular_scopes":null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newSelfServer(t, http.StatusOK, tc.body)

			token, err := gitlabtoken.New(context.Background(), server.URL, testutil.GitLabToken)
			if err == nil {
				t.Fatal("New(...) accepted a legacy token")
			}
			if !strings.Contains(err.Error(), "legacy scopes") || !strings.Contains(err.Error(), "prohibited") {
				t.Errorf("New(...) error = %q, want it to state that legacy scopes are prohibited", err.Error())
			}
			if token.Value() != "" {
				t.Errorf("New(...).Value() = %q, want empty on error", token.Value())
			}
		})
	}
}

// A 403 without the granular scope error, such as insufficient_scope from a legacy token, MUST fail closed.
func TestNew_ForbiddenWithoutGranularScopeErrorRejected(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "insufficient scope", body: `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token."}`},
		{name: "plain forbidden", body: `{"message":"403 Forbidden"}`},
		{name: "empty body", body: ``},
		{name: "malformed body", body: `insufficient_granular_scope`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newSelfServer(t, http.StatusForbidden, tc.body)

			token, err := gitlabtoken.New(context.Background(), server.URL, testutil.GitLabToken)
			if err == nil {
				t.Fatal("New(...) accepted a 403 response without the granular scope error")
			}
			if !strings.Contains(err.Error(), "status 403") {
				t.Errorf("New(...) error = %q, want it to report status 403", err.Error())
			}
			if token.Value() != "" {
				t.Errorf("New(...).Value() = %q, want empty on error", token.Value())
			}
		})
	}
}

func TestNew_MalformedSelfBodyRejected(t *testing.T) {
	server := newSelfServer(t, http.StatusOK, `not json`)

	if _, err := gitlabtoken.New(context.Background(), server.URL, testutil.GitLabToken); err == nil {
		t.Fatal("New(...) accepted an unparsable self response")
	}
}

func TestNew_Unauthorized(t *testing.T) {
	server := newSelfServer(t, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)

	_, err := gitlabtoken.New(context.Background(), server.URL, testutil.GitLabToken)
	if err == nil {
		t.Fatal("New(...) succeeded unexpectedly on a 401 response")
	}
	if !strings.Contains(err.Error(), "status 401") || !strings.Contains(err.Error(), "revoked") {
		t.Errorf("New(...) error = %q, want it to report status 401 as an invalid or revoked token", err.Error())
	}
}

func TestNew_ErrorStatusRejected(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusNoContent, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			server := newSelfServer(t, status, ``)

			_, err := gitlabtoken.New(context.Background(), server.URL, testutil.GitLabToken)
			if err == nil {
				t.Fatal("New(...) succeeded unexpectedly")
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("status %d", status)) {
				t.Errorf("New(...) error = %q, want it to report status %d", err.Error(), status)
			}
		})
	}
}

func TestNew_UnreachableServer(t *testing.T) {
	_, err := gitlabtoken.New(context.Background(), "http://127.0.0.1:0", testutil.GitLabToken)
	if err == nil {
		t.Fatal("New(...) against an unreachable server succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "failed to reach GitLab API") {
		t.Errorf("New(...) error = %q, want it to mention the unreachable API", err.Error())
	}
}

// A redirect to a different host MUST NOT carry the PRIVATE-TOKEN header.
func TestNew_CrossHostRedirectDoesNotForwardToken(t *testing.T) {
	var leaked atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("PRIVATE-TOKEN"))
		_, _ = io.WriteString(w, legacyTokenSelf)
	}))
	t.Cleanup(target.Close)
	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	if _, err := gitlabtoken.New(context.Background(), redirector.URL, testutil.SecretGitLabToken); err == nil {
		t.Error("New(...) succeeded through a cross-host redirect")
	}
	if got, _ := leaked.Load().(string); got != "" {
		t.Errorf("redirect target received PRIVATE-TOKEN %q", got)
	}
}

func TestFineGrainedToken_ZeroValueCarriesNoToken(t *testing.T) {
	var token gitlabtoken.FineGrainedToken
	if token.Value() != "" {
		t.Errorf("zero FineGrainedToken.Value() = %q, want empty", token.Value())
	}
}

// CRLF sequences in header values enable header injection. The transport MUST drop the token before
// the request reaches the server.
func TestNew_TokenWithCRLFRejectedBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler invoked; want the request rejected client-side before it reaches the server")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, testutil.InsufficientGranularScopeBody)
	}))
	t.Cleanup(server.Close)

	token, err := gitlabtoken.New(context.Background(), server.URL, "token-with-\r\ninjected-header: evil")
	if err == nil {
		t.Fatal("New(...) with a CRLF-bearing token succeeded unexpectedly")
	}
	if token.Value() != "" {
		t.Errorf("New(...).Value() = %q, want empty on error", token.Value())
	}
}

func TestNew_CanceledContextRejected(t *testing.T) {
	server := newSelfServer(t, http.StatusForbidden, testutil.InsufficientGranularScopeBody)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	token, err := gitlabtoken.New(ctx, server.URL, testutil.GitLabToken)
	if err == nil {
		t.Fatal("New(...) succeeded with a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("New(...) error = %v, want it to wrap context.Canceled", err)
	}
	if token.Value() != "" {
		t.Errorf("New(...).Value() = %q, want empty on error", token.Value())
	}
}

// Formatting a token MUST NOT reveal the raw value, since loggers format values with these verbs.
func TestFineGrainedToken_FormattingRedactsTheRawValue(t *testing.T) {
	token := testutil.NewStubToken(t, testutil.SecretGitLabToken)

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		t.Run(verb, func(t *testing.T) {
			got := fmt.Sprintf(verb, token)
			if strings.Contains(got, testutil.SecretGitLabToken) {
				t.Errorf("fmt.Sprintf(%q, token) = %q, want the raw value redacted", verb, got)
			}
			if !strings.Contains(got, "REDACTED") {
				t.Errorf("fmt.Sprintf(%q, token) = %q, want a redaction marker", verb, got)
			}
		})
	}
}

// NewStubToken obtains the token through New, which keeps tests in other packages from bypassing
// the check.
func TestNewStubToken_ReturnsTheRawValue(t *testing.T) {
	token := testutil.NewStubToken(t, testutil.GitLabToken)
	if token.Value() != testutil.GitLabToken {
		t.Errorf("NewStubToken(...).Value() = %q, want %q", token.Value(), testutil.GitLabToken)
	}
}
