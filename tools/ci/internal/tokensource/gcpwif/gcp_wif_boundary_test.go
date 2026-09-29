package gcpwif_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ci-tools/internal/tokensource/gcpwif"
)

func TestNewGoogleWIF_LengthFormatInjectionAndLeak(t *testing.T) {
	tests := []struct {
		name    string
		cfg     gcpwif.GoogleWIFConfig
		wantErr bool
	}{
		{
			name: "project id crlf injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectID = "test-project\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "project id nul injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectID = "test-project\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "project number crlf injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectNumber = "123456789012\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "project number nul injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectNumber = "123456789012\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "workload identity provider crlf injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = validTestGoogleWIFConfig.WorkloadIdentityProvider + "\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "workload identity provider nul injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = validTestGoogleWIFConfig.WorkloadIdentityProvider + "\x00supersecret"
			}),
			wantErr: true,
		},

		{
			name: "service account crlf injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ServiceAccount = validTestGoogleWIFConfig.ServiceAccount + "\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "service account nul injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ServiceAccount = validTestGoogleWIFConfig.ServiceAccount + "\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token too short",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = "jwt_tok"
			}),
			wantErr: true,
		},
		{
			name: "id token too long",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = strings.Repeat("e", 4086) + "supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token disallowed character",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = "jwt_token+supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token crlf injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = "jwt_token\r\nX-Injected: supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token nul injection",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = "jwt_token\x00supersecret"
			}),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gcpwif.NewGoogleWIF(tc.cfg)
			assertGoogleWIFBoundaryResult(t, tc.cfg, tc.wantErr, got, err)
		})
	}
}

func assertGoogleWIFBoundaryResult(t *testing.T, cfg gcpwif.GoogleWIFConfig, wantErr bool, got *gcpwif.GoogleWIF, err error) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Fatalf("NewGoogleWIF() error = %v, wantErr = %v", err, wantErr)
	}
	if err != nil {
		assertGoogleWIFBoundaryErrorSanitized(t, cfg, err)
		return
	}
	assertGoogleWIFBoundarySuccess(t, cfg, got)
}

func assertGoogleWIFBoundaryErrorSanitized(t *testing.T, cfg gcpwif.GoogleWIFConfig, err error) {
	t.Helper()
	assertGoogleWIFErrorOmitsValues(t, err, cfg)
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("NewGoogleWIF() error %q contains secret material", err)
	}
}

func assertGoogleWIFBoundarySuccess(t *testing.T, cfg gcpwif.GoogleWIFConfig, got *gcpwif.GoogleWIF) {
	t.Helper()
	if got == nil {
		t.Fatal("NewGoogleWIF() returned nil")
	}
	if got.Config().ProjectID != cfg.ProjectID || got.Config().ProjectNumber != cfg.ProjectNumber {
		t.Errorf("Config() = %+v, want %+v", got.Config(), cfg)
	}
}

func assertGoogleWIFErrorOmitsValues(t *testing.T, err error, cfg gcpwif.GoogleWIFConfig) {
	t.Helper()
	for _, value := range []string{
		cfg.ProjectID,
		cfg.ProjectNumber,
		cfg.WorkloadIdentityProvider,
		cfg.ServiceAccount,
		cfg.IDToken,
	} {
		if len(value) < 8 || strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("NewGoogleWIF() error %q contains a configured field value", err)
		}
	}
}

func TestGoogleWIF_FetchCredential_AdverseHttp(t *testing.T) {
	tests := []struct {
		name          string
		stsStatus     int
		stsBody       string
		iamStatus     int
		iamBody       string
		wantIamCalled bool
	}{
		{
			name:          "sts http 400 invalid grant error",
			stsStatus:     http.StatusBadRequest,
			stsBody:       `{"error":"invalid_grant","error_description":"The provided token is expired."}`,
			wantIamCalled: false,
		},
		{
			name:          "sts http 403 forbidden",
			stsStatus:     http.StatusForbidden,
			stsBody:       `{"error":"forbidden"}`,
			wantIamCalled: false,
		},
		{
			name:          "sts http 500 internal server error",
			stsStatus:     http.StatusInternalServerError,
			stsBody:       `{"error":"server_error"}`,
			wantIamCalled: false,
		},
		{
			name:          "sts empty response body",
			stsStatus:     http.StatusOK,
			stsBody:       "",
			wantIamCalled: false,
		},
		{
			name:          "sts malformed json",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token": "sts-token", "token_type": `,
			wantIamCalled: false,
		},
		{
			name:          "sts missing access token",
			stsStatus:     http.StatusOK,
			stsBody:       `{"token_type":"Bearer","expires_in":3600}`,
			wantIamCalled: false,
		},
		{
			name:          "iam http 403 permission denied",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusForbidden,
			iamBody:       `{"error":{"code":403,"message":"Permission 'iam.serviceAccounts.getAccessToken' denied."}}`,
			wantIamCalled: true,
		},
		{
			name:          "iam http 404 service account not found",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusNotFound,
			iamBody:       `{"error":{"code":404,"message":"Service account not found."}}`,
			wantIamCalled: true,
		},
		{
			name:          "iam http 500 internal server error",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusInternalServerError,
			iamBody:       `{"error":"server_error"}`,
			wantIamCalled: true,
		},
		{
			name:          "iam empty response body",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusOK,
			iamBody:       "",
			wantIamCalled: true,
		},
		{
			name:          "iam malformed json",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusOK,
			iamBody:       `{"accessToken": "token", "expireTime": `,
			wantIamCalled: true,
		},
		{
			name:          "iam missing accessToken",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusOK,
			iamBody:       `{"expireTime":"2026-09-27T15:00:00Z"}`,
			wantIamCalled: true,
		},
		{
			name:          "iam invalid expireTime format",
			stsStatus:     http.StatusOK,
			stsBody:       `{"access_token":"sts-token","token_type":"Bearer","expires_in":3600}`,
			iamStatus:     http.StatusOK,
			iamBody:       `{"accessToken":"token","expireTime":"not-rfc3339"}`,
			wantIamCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var iamCalls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/v1/token") {
					w.WriteHeader(tc.stsStatus)
					_, _ = w.Write([]byte(tc.stsBody))
				} else {
					atomic.AddInt32(&iamCalls, 1)
					w.WriteHeader(tc.iamStatus)
					_, _ = w.Write([]byte(tc.iamBody))
				}
			}))
			defer server.Close()

			cfg := validTestGoogleWIFConfig
			cfg.STSEndpoint = server.URL + "/v1/token"
			cfg.IAMCredentialsEndpoint = server.URL + "/generateAccessToken"

			src, err := gcpwif.NewGoogleWIF(cfg)
			if err != nil {
				t.Fatalf("NewGoogleWIF() error = %v", err)
			}

			_, err = src.FetchCredential(context.Background())
			if err == nil {
				t.Fatalf("FetchCredential() expected error for %s, got nil", tc.name)
			}
			assertGoogleWIFErrorOmitsValues(t, err, cfg)

			called := atomic.LoadInt32(&iamCalls) > 0
			if called != tc.wantIamCalled {
				t.Errorf("IAM called = %v, want %v", called, tc.wantIamCalled)
			}
		})
	}
}

func TestGoogleWIF_FetchCredential_NetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := server.URL + "/v1/token"
	server.Close()

	cfg := validTestGoogleWIFConfig
	cfg.STSEndpoint = endpoint
	cfg.IAMCredentialsEndpoint = endpoint

	src, err := gcpwif.NewGoogleWIF(cfg)
	if err != nil {
		t.Fatalf("NewGoogleWIF() error = %v", err)
	}

	_, err = src.FetchCredential(context.Background())
	if err == nil {
		t.Fatal("FetchCredential() expected network failure error, got nil")
	}
}

func TestGoogleWIF_FetchCredential_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "sts-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	cfg := validTestGoogleWIFConfig
	cfg.STSEndpoint = server.URL + "/v1/token"
	cfg.IAMCredentialsEndpoint = server.URL + "/generateAccessToken"

	src, err := gcpwif.NewGoogleWIF(cfg)
	if err != nil {
		t.Fatalf("NewGoogleWIF() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = src.FetchCredential(ctx)
	if err == nil {
		t.Fatal("FetchCredential() expected error on canceled context, got nil")
	}
}

func TestGoogleWIF_FetchCredential_NilContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/v1/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sts-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "gcp-token-nil-ctx",
				"expireTime":  time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
		}
	}))
	defer server.Close()

	cfg := validTestGoogleWIFConfig
	cfg.STSEndpoint = server.URL + "/v1/token"
	cfg.IAMCredentialsEndpoint = server.URL + "/generateAccessToken"

	src, err := gcpwif.NewGoogleWIF(cfg)
	if err != nil {
		t.Fatalf("NewGoogleWIF() error = %v", err)
	}

	var nilCtx context.Context
	cred, err := src.FetchCredential(nilCtx)
	if err != nil {
		t.Fatalf("FetchCredential(nil) error = %v", err)
	}
	if cred.Value != "gcp-token-nil-ctx" {
		t.Errorf("FetchCredential(nil) = %q, want gcp-token-nil-ctx", cred.Value)
	}
}
