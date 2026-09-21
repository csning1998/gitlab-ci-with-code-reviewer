package providers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ci-tools/internal/config"
	"ci-tools/internal/tokensource"
)

func TestNew_DispatchesByProvider(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		wantName string
	}{
		{provider: "claude", model: "claude-sonnet-5", wantName: "Claude"},
		{provider: "gemini", model: "gemini-2.5-flash", wantName: "Gemini"},
		{provider: "openai", model: "gpt-4o", wantName: "OpenAI"},
		{provider: "azure-openai", model: "gpt-4o", wantName: "Azure OpenAI"},
		{provider: "grok", model: "grok-4.6", wantName: "Grok"},
		{provider: "local", model: "llama-3.3-70b", wantName: "Local"},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			llm, err := New(
				config.ModelOptions{Provider: tc.provider, Model: tc.model},
				tokensource.Static("test-token"),
			)
			if err != nil {
				t.Fatalf("New(...) returned an unexpected error: %v", err)
			}
			if got := llm.Name(); got != tc.wantName {
				t.Errorf("Name() = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func TestNew_RejectsUnsupportedProvider(t *testing.T) {
	_, err := New(
		config.ModelOptions{Provider: "deepseek", Model: "deepseek-r1"},
		tokensource.Static("test-token"),
	)
	if err == nil || !strings.Contains(err.Error(), "deepseek") {
		t.Fatalf("New(...) error = %v, want an error naming the unsupported provider", err)
	}
}

func TestNew_PropagatesProviderConstructionError(t *testing.T) {
	tests := []struct {
		name     string
		opts     config.ModelOptions
		tokens   tokensource.Provider
		wantWord string
	}{
		{
			name:     "claude without tokens",
			opts:     config.ModelOptions{Provider: "claude", Model: "claude-sonnet-5"},
			wantWord: "tokens",
		},
		{
			name:     "gemini without model",
			opts:     config.ModelOptions{Provider: "gemini"},
			tokens:   tokensource.Static("test-token"),
			wantWord: "model",
		},
		{
			name:     "grok without model",
			opts:     config.ModelOptions{Provider: "grok"},
			tokens:   tokensource.Static("test-token"),
			wantWord: "model",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.opts, tc.tokens)
			if err == nil || !strings.Contains(err.Error(), tc.wantWord) {
				t.Fatalf("New(...) error = %v, want an error mentioning %q", err, tc.wantWord)
			}
		})
	}
}

// newVaultStub serves the JWT login and KV-v2 read endpoints of a Vault instance, recording the
// login payload for assertion.
func newVaultStub(t *testing.T, field, credential string, gotLogin *map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/login"):
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, gotLogin)
			_, _ = w.Write([]byte(`{"auth":{"client_token":"vault-client-token"}}`))
		default:
			if got := r.Header.Get("X-Vault-Token"); got != "vault-client-token" {
				t.Errorf("X-Vault-Token = %q, want the token issued by login", got)
			}
			payload := map[string]any{"data": map[string]any{"data": map[string]any{field: credential}}}
			_ = json.NewEncoder(w).Encode(payload)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNew_FederatedCredentialReachesProviderRequest(t *testing.T) {
	// The full federation chain runs end to end: the CI identity token is exchanged at the Vault
	// JWT backend, the KV-v2 field is read, and the value authorizes the provider request.
	var gotLogin map[string]string
	vault := newVaultStub(t, "grok_api_key", "federated-key", &gotLogin)

	var gotAuthorization string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
	}))
	t.Cleanup(provider.Close)

	t.Setenv("REVIEW_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	t.Setenv("VAULT_ADDR", vault.URL)
	t.Setenv("VAULT_ID_TOKEN", "header.payload.signature")
	t.Setenv("VAULT_AUTH_MOUNT", "gitlab-saas-jwt")
	t.Setenv("VAULT_ROLE", "ci-code-reviewer")
	t.Setenv("VAULT_KV_MOUNT", "secret")
	t.Setenv("VAULT_SECRET_PATH", "ci/credentials")
	t.Setenv("VAULT_SECRET_FIELD", "")
	t.Setenv("VAULT_CACERT", "")

	tokens, err := config.ResolveTokenProvider("grok")
	if err != nil {
		t.Fatalf("ResolveTokenProvider(...) returned an unexpected error: %v", err)
	}

	llm, err := New(config.ModelOptions{
		Provider: "grok",
		Model:    "grok-4.6",
		BaseURL:  provider.URL,
		Timeout:  10 * time.Second,
	}, tokens)
	if err != nil {
		t.Fatalf("New(...) returned an unexpected error: %v", err)
	}

	got, err := llm.Review("prompt")
	if err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("Review(...) = %q, want %q", got, "[]")
	}
	if gotAuthorization != "Bearer federated-key" {
		t.Errorf("Authorization = %q, want the credential read from Vault", gotAuthorization)
	}
	if gotLogin["role"] != "ci-code-reviewer" {
		t.Errorf("login role = %q, want %q", gotLogin["role"], "ci-code-reviewer")
	}
	if gotLogin["jwt"] != "header.payload.signature" {
		t.Errorf("login jwt = %q, want the CI identity token", gotLogin["jwt"])
	}
}
