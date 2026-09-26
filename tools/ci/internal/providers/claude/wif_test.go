package claude

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ci-tools/internal/config"
	"ci-tools/internal/tokensource"
)

var validClaudeWIFConfig = tokensource.ClaudeWIFConfig{
	FederationRuleID: "fdrl_123456",
	OrganizationID:   "abcdef01-2345-4678-89ab-cdef01234567",
	ServiceAccountID: "svac_789012",
	IDToken:          "header.payload.signature",
	WorkspaceID:      "wrkspc_123456",
}

func mustNewClaudeWIF(t *testing.T, cfg tokensource.ClaudeWIFConfig) *tokensource.ClaudeWIF {
	t.Helper()
	wif, err := tokensource.NewClaudeWIF(cfg)
	if err != nil {
		t.Fatalf("NewClaudeWIF(...) error = %v", err)
	}
	return wif
}

type mockAnthropicServer struct {
	server       *httptest.Server
	oauthCalls   atomic.Int64
	messageCalls atomic.Int64
	lastAuth     string
	lastAPIKey   string
	oauthStatus  int
}

func newMockAnthropicServer(t *testing.T) *mockAnthropicServer {
	t.Helper()
	mock := &mockAnthropicServer{oauthStatus: http.StatusOK}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		mock.oauthCalls.Add(1)
		if mock.oauthStatus != http.StatusOK {
			w.WriteHeader(mock.oauthStatus)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token exchange failed"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"exchanged-bearer-token-123","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		mock.messageCalls.Add(1)
		mock.lastAuth = r.Header.Get("Authorization")
		mock.lastAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(streamBody))
	})

	mock.server = httptest.NewServer(mux)
	t.Cleanup(mock.server.Close)
	return mock
}

func (m *mockAnthropicServer) newClient(t *testing.T, tokens tokensource.Provider) *Client {
	t.Helper()
	return mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{
			Model:   "claude-sonnet-5",
			BaseURL: m.server.URL,
			Timeout: 10 * time.Second,
		},
		Tokens: tokens,
	})
}

func TestClaude_WIF_SuccessfulTokenExchangeAndReview(t *testing.T) {
	mock := newMockAnthropicServer(t)
	client := mock.newClient(t, mustNewClaudeWIF(t, validClaudeWIFConfig))

	got, err := client.Review("test prompt")
	if err != nil {
		t.Fatalf("Review(...) returned unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("Review(...) = %q, want %q", got, "[]")
	}
	if mock.oauthCalls.Load() != 1 {
		t.Errorf("oauthCalls = %d, want 1", mock.oauthCalls.Load())
	}
	if mock.messageCalls.Load() != 1 {
		t.Errorf("messageCalls = %d, want 1", mock.messageCalls.Load())
	}
	if mock.lastAuth != "Bearer exchanged-bearer-token-123" {
		t.Errorf("Authorization header = %q, want bearer token", mock.lastAuth)
	}
	if mock.lastAPIKey != "" {
		t.Errorf("x-api-key header = %q, want empty in WIF mode", mock.lastAPIKey)
	}
}

func TestClaude_WIF_CachesTokenAcrossMultipleCalls(t *testing.T) {
	mock := newMockAnthropicServer(t)
	client := mock.newClient(t, mustNewClaudeWIF(t, validClaudeWIFConfig))

	for range 3 {
		got, err := client.Review("test prompt")
		if err != nil {
			t.Fatalf("Review(...) error = %v", err)
		}
		if got != "[]" {
			t.Errorf("Review(...) = %q, want %q", got, "[]")
		}
	}
	if mock.oauthCalls.Load() != 1 {
		t.Errorf("oauthCalls = %d, want 1 (token cached)", mock.oauthCalls.Load())
	}
	if mock.messageCalls.Load() != 3 {
		t.Errorf("messageCalls = %d, want 3", mock.messageCalls.Load())
	}
}

func TestClaude_WIF_WithoutEnvironmentDefaults_IgnoresAnthropicAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "stale-env-api-key")

	mock := newMockAnthropicServer(t)
	client := mock.newClient(t, mustNewClaudeWIF(t, validClaudeWIFConfig))

	got, err := client.Review("test prompt")
	if err != nil {
		t.Fatalf("Review(...) error = %v", err)
	}
	if got != "[]" {
		t.Errorf("Review(...) = %q, want %q", got, "[]")
	}
	if mock.lastAPIKey == "stale-env-api-key" {
		t.Errorf("x-api-key leaked stale ANTHROPIC_API_KEY from environment")
	}
	if mock.lastAuth != "Bearer exchanged-bearer-token-123" {
		t.Errorf("Authorization header = %q, want bearer token", mock.lastAuth)
	}
}

func TestClaude_WIF_ExchangeFailureReturnsWrappedError(t *testing.T) {
	mock := newMockAnthropicServer(t)
	mock.oauthStatus = http.StatusUnauthorized
	client := mock.newClient(t, mustNewClaudeWIF(t, validClaudeWIFConfig))

	_, err := client.Review("test prompt")
	if err == nil {
		t.Fatal("Review(...) succeeded unexpectedly when oauth exchange fails")
	}
}

func TestClaude_NonWIF_UsesAPIKeyHeader(t *testing.T) {
	mock := newMockAnthropicServer(t)
	client := mock.newClient(t, tokensource.Static("static-api-key"))

	got, err := client.Review("test prompt")
	if err != nil {
		t.Fatalf("Review(...) error = %v", err)
	}
	if got != "[]" {
		t.Errorf("Review(...) = %q, want %q", got, "[]")
	}
	if mock.oauthCalls.Load() != 0 {
		t.Errorf("oauthCalls = %d, want 0 in non-WIF mode", mock.oauthCalls.Load())
	}
	if mock.lastAPIKey != "static-api-key" {
		t.Errorf("x-api-key = %q, want %q", mock.lastAPIKey, "static-api-key")
	}
}
