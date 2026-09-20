package tokensource_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ci-tools/internal/tokensource"
)

type mockServerParams struct {
	loginToken   string
	loginStatus  int
	loginErrors  []string
	secretPath   string
	secretStatus int
	secretData   map[string]any
	secretErrors []string
	delay        time.Duration
}

func handleMockLogin(w http.ResponseWriter, r *http.Request, p mockServerParams) bool {
	if r.URL.Path != "/v1/auth/gitlab-saas-jwt/login" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	if p.loginStatus != 0 {
		w.WriteHeader(p.loginStatus)
	}
	if len(p.loginErrors) > 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": p.loginErrors})
		return true
	}
	if p.loginToken != "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth": map[string]any{"client_token": p.loginToken},
		})
		return true
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{}})
	return true
}

func handleMockSecret(w http.ResponseWriter, r *http.Request, p mockServerParams) bool {
	expectedPath := p.secretPath
	if expectedPath == "" {
		expectedPath = "/v1/secret/data/ci/credentials"
	}
	if r.URL.Path != expectedPath {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	if p.secretStatus != 0 {
		w.WriteHeader(p.secretStatus)
	}
	if len(p.secretErrors) > 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": p.secretErrors})
		return true
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{"data": p.secretData},
	})
	return true
}

func newMockVaultKVServer(t *testing.T, p mockServerParams) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.delay > 0 {
			time.Sleep(p.delay)
		}
		if handleMockLogin(w, r, p) {
			return
		}
		if handleMockSecret(w, r, p) {
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func newDefaultVaultKVConfig(vaultAddr string) tokensource.VaultKVConfig {
	return tokensource.VaultKVConfig{
		VaultAddr:   vaultAddr,
		Role:        "ci-reviewer",
		JWT:         "valid-jwt",
		MountPath:   "secret",
		SecretPath:  "ci/credentials",
		SecretField: "api_key",
	}
}

// TestStatic_TokenBehavior covers Static token provider behaviors for valid and empty values.
func TestStatic_TokenBehavior(t *testing.T) {
	tests := []struct {
		name      string
		staticVal tokensource.Static
		wantToken string
		wantErr   error
	}{
		{
			name:      "configured static credential",
			staticVal: "mock-static-token",
			wantToken: "mock-static-token",
			wantErr:   nil,
		},
		{
			name:      "empty static credential",
			staticVal: "",
			wantToken: "",
			wantErr:   tokensource.ErrEmptyStatic,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.staticVal.Token(context.Background())
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Token() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.wantToken {
				t.Errorf("Token() = %q, want %q", got, tc.wantToken)
			}
		})
	}
}

// TestVaultKV_ConfigValidationBehavior covers parameter validation behaviors upon NewVaultKV construction.
func TestVaultKV_ConfigValidationBehavior(t *testing.T) {
	valid := newDefaultVaultKVConfig("https://vault.example.com")

	tests := []struct {
		name      string
		mutate    func(*tokensource.VaultKVConfig)
		wantError bool
	}{
		{
			name:      "valid configuration",
			mutate:    func(c *tokensource.VaultKVConfig) {},
			wantError: false,
		},
		{
			name: "empty VaultAddr",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.VaultAddr = ""
			},
			wantError: true,
		},
		{
			name: "empty Role",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.Role = ""
			},
			wantError: true,
		},
		{
			name: "empty JWT",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.JWT = ""
			},
			wantError: true,
		},
		{
			name: "empty MountPath",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.MountPath = ""
			},
			wantError: true,
		},
		{
			name: "empty SecretPath",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.SecretPath = ""
			},
			wantError: true,
		},
		{
			name: "empty SecretField",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.SecretField = ""
			},
			wantError: true,
		},
		{
			name: "nonexistent CACertPath",
			mutate: func(c *tokensource.VaultKVConfig) {
				c.CACertPath = "/nonexistent/ca.pem"
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			_, err := tokensource.NewVaultKV(cfg)
			if (err != nil) != tc.wantError {
				t.Errorf("NewVaultKV() error = %v, wantError = %v", err, tc.wantError)
			}
		})
	}
}

// TestVaultKV_StandardRetrieval covers standard GitLab SaaS JWT login and secret retrieval.
func TestVaultKV_StandardRetrieval(t *testing.T) {
	server := newMockVaultKVServer(t, mockServerParams{
		loginToken: "vault-client-token-abc",
		secretData: map[string]any{"api_key": "mock-secret-payload-value"},
	})

	cfg := newDefaultVaultKVConfig(server.URL)
	cfg.JWT = "mock-gitlab-jwt"
	vk, err := tokensource.NewVaultKV(cfg)
	if err != nil {
		t.Fatalf("NewVaultKV() error = %v", err)
	}

	token, err := vk.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "mock-secret-payload-value" {
		t.Errorf("Token() = %q, want %q", token, "mock-secret-payload-value")
	}
}

// TestVaultKV_TLSWithCustomCA covers Vault communication with custom CA certificates.
func TestVaultKV_TLSWithCustomCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/gitlab-saas-jwt/login":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": "tls-token"}})
		case "/v1/secret/data/ci/tls-secret":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": map[string]any{"api_key": "secret-tls-val"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})
	certFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(ca.pem) error = %v", err)
	}

	cfg := newDefaultVaultKVConfig(server.URL)
	cfg.SecretPath = "ci/tls-secret"
	cfg.CACertPath = certFile
	vk, err := tokensource.NewVaultKV(cfg)
	if err != nil {
		t.Fatalf("NewVaultKV() error = %v", err)
	}

	token, err := vk.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "secret-tls-val" {
		t.Errorf("Token() = %q, want %q", token, "secret-tls-val")
	}
}

// TestVaultKV_CustomAuthMountPath covers custom JWT auth mount path configuration.
func TestVaultKV_CustomAuthMountPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/custom-spire-jwt/login":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": "spire-token"}})
		case "/v1/secret/data/ci/credentials":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": map[string]any{"api_key": "custom-mount-secret"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := newDefaultVaultKVConfig(server.URL)
	cfg.AuthMountPath = "custom-spire-jwt"
	vk, err := tokensource.NewVaultKV(cfg)
	if err != nil {
		t.Fatalf("NewVaultKV() error = %v", err)
	}

	token, err := vk.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "custom-mount-secret" {
		t.Errorf("Token() = %q, want %q", token, "custom-mount-secret")
	}
}

// TestVaultKV_ConcurrentAccess covers parallel token retrieval across multiple workers.
func TestVaultKV_ConcurrentAccess(t *testing.T) {
	server := newMockVaultKVServer(t, mockServerParams{
		loginToken: "concurrent-token",
		secretData: map[string]any{"api_key": "concurrent-val"},
	})

	cfg := newDefaultVaultKVConfig(server.URL)
	cfg.JWT = "jwt-val"
	vk, err := tokensource.NewVaultKV(cfg)
	if err != nil {
		t.Fatalf("NewVaultKV() error = %v", err)
	}

	const workers = 10
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			token, err := vk.Token(context.Background())
			if err != nil {
				errCh <- err
				return
			}
			if token != "concurrent-val" {
				errCh <- fmt.Errorf("unexpected token %q", token)
				return
			}
			errCh <- nil
		}()
	}

	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent Token() failed: %v", err)
		}
	}
}

// TestVaultKV_ServerErrorScenarios covers failure behaviors when Vault server returns non-200 or invalid payloads.
func TestVaultKV_ServerErrorScenarios(t *testing.T) {
	tests := []struct {
		name         string
		serverParams mockServerParams
		cfgMutate    func(*tokensource.VaultKVConfig)
	}{
		{
			name: "login failure 403",
			serverParams: mockServerParams{
				loginStatus: http.StatusForbidden,
				loginErrors: []string{"permission denied for role"},
			},
		},
		{
			name: "login response missing client_token",
			serverParams: mockServerParams{
				loginToken: "",
			},
		},
		{
			name: "secret path 404 not found",
			serverParams: mockServerParams{
				loginToken:   "valid-token",
				secretStatus: http.StatusNotFound,
				secretErrors: []string{"path not found"},
			},
		},
		{
			name: "secret payload missing requested field",
			serverParams: mockServerParams{
				loginToken: "valid-token",
				secretData: map[string]any{"other_field": "some-value"},
			},
		},
		{
			name: "secret field value is not a string",
			serverParams: mockServerParams{
				loginToken: "valid-token",
				secretData: map[string]any{"api_key": 12345},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newMockVaultKVServer(t, tc.serverParams)
			cfg := newDefaultVaultKVConfig(server.URL)
			if tc.cfgMutate != nil {
				tc.cfgMutate(&cfg)
			}

			vk, err := tokensource.NewVaultKV(cfg)
			if err != nil {
				t.Fatalf("NewVaultKV() error = %v", err)
			}

			_, err = vk.Token(context.Background())
			if err == nil {
				t.Fatalf("Token() expected error for %s, got nil", tc.name)
			}
		})
	}
}

// TestVaultKV_ContextCancellation covers handling when caller cancels context during retrieval.
func TestVaultKV_ContextCancellation(t *testing.T) {
	server := newMockVaultKVServer(t, mockServerParams{
		delay:      100 * time.Millisecond,
		loginToken: "delayed-token",
	})

	cfg := newDefaultVaultKVConfig(server.URL)
	vk, err := tokensource.NewVaultKV(cfg)
	if err != nil {
		t.Fatalf("NewVaultKV() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = vk.Token(ctx)
	if err == nil {
		t.Fatalf("Token() expected error with cancelled context, got nil")
	}
}

// Suppress unused imports for tls and x509 in root scope.
var (
	_ = tls.VersionTLS12
	_ = x509.NewCertPool
)
