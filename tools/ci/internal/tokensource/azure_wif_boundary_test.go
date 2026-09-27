package tokensource_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
)

func TestNewAzureWIF_LengthFormatInjectionAndLeak(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tokensource.AzureWIFConfig
		wantErr bool
	}{
		{
			name: "tenant id uppercase uuid",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = "663BCC2A-0747-4E40-BC5D-6D8C4450E1F3"
			}),
			wantErr: true,
		},
		{
			name: "tenant id missing dashes",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = "663bcc2a07474e40bc5d6d8c4450e1f3"
			}),
			wantErr: true,
		},
		{
			name: "tenant id non hex",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = "663bcc2a-0747-4e40-bc5d-6d8c4450e1fz"
			}),
			wantErr: true,
		},
		{
			name: "tenant id too long",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = validTestAzureWIFConfig.TenantID + "-supersecret"
			}),
			wantErr: true,
		},
		{
			name: "tenant id nul injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = validTestAzureWIFConfig.TenantID + "\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "tenant id crlf injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = validTestAzureWIFConfig.TenantID + "\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "client id uppercase uuid",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = "11111111-2222-3333-4444-55555555555A"
			}),
			wantErr: true,
		},
		{
			name: "client id missing dashes",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = "11111111222233334444555555555555"
			}),
			wantErr: true,
		},
		{
			name: "client id non hex",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = "11111111-2222-3333-4444-55555555555g"
			}),
			wantErr: true,
		},
		{
			name: "client id too long",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = validTestAzureWIFConfig.ClientID + "-supersecret"
			}),
			wantErr: true,
		},
		{
			name: "client id nul injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = validTestAzureWIFConfig.ClientID + "\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "client id crlf injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = validTestAzureWIFConfig.ClientID + "\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token too short",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = "jwt_tok"
			}),
			wantErr: true,
		},
		{
			name: "id token too long",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = strings.Repeat("e", 4086) + "supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token disallowed character",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = "jwt_token+supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token crlf injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = "jwt_token\r\nX-Injected: supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token nul injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = "jwt_token\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "openai endpoint malformed url",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = "://bad-url"
			}),
			wantErr: true,
		},
		{
			name: "openai endpoint non https scheme",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = "ftp://oai-csning1998-lab.openai.azure.com/"
			}),
			wantErr: true,
		},
		{
			name: "openai endpoint userinfo injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = "https://user:pass@oai-csning1998-lab.openai.azure.com/"
			}),
			wantErr: true,
		},
		{
			name: "openai endpoint crlf injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = "https://oai-csning1998-lab.openai.azure.com/\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "openai endpoint nul injection",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = "https://oai-csning1998-lab.openai.azure.com/\x00supersecret"
			}),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokensource.NewAzureWIF(tc.cfg)
			assertAzureWIFBoundaryResult(t, tc.cfg, tc.wantErr, got, err)
		})
	}
}

func assertAzureWIFBoundaryResult(t *testing.T, cfg tokensource.AzureWIFConfig, wantErr bool, got *tokensource.AzureWIF, err error) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Fatalf("NewAzureWIF() error = %v, wantErr = %v", err, wantErr)
	}
	if err != nil {
		assertAzureWIFBoundaryErrorSanitized(t, cfg, err)
		return
	}
	assertAzureWIFBoundarySuccess(t, cfg, got)
}

func assertAzureWIFBoundaryErrorSanitized(t *testing.T, cfg tokensource.AzureWIFConfig, err error) {
	t.Helper()
	assertAzureWIFErrorOmitsValues(t, err, cfg)
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("NewAzureWIF() error %q contains secret material", err)
	}
}

func assertAzureWIFBoundarySuccess(t *testing.T, cfg tokensource.AzureWIFConfig, got *tokensource.AzureWIF) {
	t.Helper()
	if got == nil {
		t.Fatal("NewAzureWIF() returned nil")
	}
	if got.Config().TenantID != cfg.TenantID || got.Config().ClientID != cfg.ClientID {
		t.Errorf("Config() = %+v, want %+v", got.Config(), cfg)
	}
}

func assertAzureWIFErrorOmitsValues(t *testing.T, err error, cfg tokensource.AzureWIFConfig) {
	t.Helper()
	for _, value := range []string{
		cfg.TenantID,
		cfg.ClientID,
		cfg.IDToken,
		cfg.OpenAIEndpoint,
	} {
		if len(value) < 8 || strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("NewAzureWIF() error %q contains a configured field value", err)
		}
	}
}

func TestAzureWIF_FetchCredential_AdverseHttp(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "http 400 oauth invalid client error",
			statusCode: http.StatusBadRequest,
			body:       `{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret."}`,
		},
		{
			name:       "http 401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":"unauthorized_client"}`,
		},
		{
			name:       "http 403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"error":"access_denied"}`,
		},
		{
			name:       "http 429 rate limit",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":"slow_down"}`,
		},
		{
			name:       "http 500 internal server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":"server_error"}`,
		},
		{
			name:       "empty response body",
			statusCode: http.StatusOK,
			body:       "",
		},
		{
			name:       "malformed json response",
			statusCode: http.StatusOK,
			body:       `{"token_type": "Bearer", "access_tok`,
		},
		{
			name:       "html error response",
			statusCode: http.StatusBadGateway,
			body:       `<html><body>502 Bad Gateway</body></html>`,
		},
		{
			name:       "missing access token",
			statusCode: http.StatusOK,
			body:       `{"token_type":"Bearer","expires_in":3600}`,
		},
		{
			name:       "negative expires in",
			statusCode: http.StatusOK,
			body:       `{"token_type":"Bearer","access_token":"tok","expires_in":-1}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			cfg := validTestAzureWIFConfig
			cfg.TokenEndpoint = server.URL + "/token"

			src, err := tokensource.NewAzureWIF(cfg)
			if err != nil {
				t.Fatalf("NewAzureWIF() error = %v", err)
			}

			_, err = src.FetchCredential(context.Background())
			if err == nil {
				t.Fatalf("FetchCredential() expected error for %s, got nil", tc.name)
			}
			assertAzureWIFErrorOmitsValues(t, err, cfg)
		})
	}
}

func TestAzureWIF_FetchCredential_NetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := server.URL + "/token"
	server.Close() // immediately closed to simulate connection failure

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = endpoint

	src, err := tokensource.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	_, err = src.FetchCredential(context.Background())
	if err == nil {
		t.Fatal("FetchCredential() expected network failure error, got nil")
	}
}
