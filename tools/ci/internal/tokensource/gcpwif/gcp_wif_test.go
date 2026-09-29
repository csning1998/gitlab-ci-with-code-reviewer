package gcpwif_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ci-tools/internal/tokensource"
	"ci-tools/internal/tokensource/gcpwif"
)

var validTestGoogleWIFConfig = gcpwif.GoogleWIFConfig{
	ProjectID:                "test-gcp-project",
	ProjectNumber:            "123456789012",
	WorkloadIdentityProvider: "projects/123456789012/locations/global/workloadIdentityPools/gitlab-pool/providers/gitlab-provider",
	ServiceAccount:           "sa-p-example-app@test-gcp-project.iam.gserviceaccount.com",
	IDToken:                  "jwt.token.gcp.test",
}

func withTestGoogleWIFConfig(mutate func(c *gcpwif.GoogleWIFConfig)) gcpwif.GoogleWIFConfig {
	cfg := validTestGoogleWIFConfig
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func newTestGoogleWIF(t *testing.T) *gcpwif.GoogleWIF {
	t.Helper()
	src, err := gcpwif.NewGoogleWIF(validTestGoogleWIFConfig)
	if err != nil {
		t.Fatalf("NewGoogleWIF() error = %v", err)
	}
	return src
}

func assertGoogleWIFExpectedError(t *testing.T, wantErr string, got *gcpwif.GoogleWIF, err error) {
	t.Helper()
	if err == nil || err.Error() != wantErr {
		t.Fatalf("NewGoogleWIF() error = %v, want %q", err, wantErr)
	}
	if got != nil {
		t.Fatalf("NewGoogleWIF() = %#v, want nil", got)
	}
}

func assertGoogleWIFSuccess(t *testing.T, got *gcpwif.GoogleWIF, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("NewGoogleWIF() unexpected error = %v", err)
	}
	if got == nil {
		t.Fatal("NewGoogleWIF() returned nil")
	}
}

func TestNewGoogleWIF_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     gcpwif.GoogleWIFConfig
		wantErr string
	}{
		{
			name: "valid configuration",
			cfg:  validTestGoogleWIFConfig,
		},
		{
			name: "missing project id",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectID = ""
			}),
			wantErr: "google wif: project id is required",
		},
		{
			name: "missing project number",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectNumber = ""
			}),
			wantErr: "google wif: project number is required",
		},
		{
			name: "invalid project number",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectNumber = "not-a-number"
			}),
			wantErr: "google wif: project number format is invalid",
		},
		{
			name: "missing workload identity provider",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = ""
			}),
			wantErr: "google wif: workload identity provider is required",
		},
		{
			name: "invalid workload identity provider format",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = "invalid/provider/path"
			}),
			wantErr: "google wif: workload identity provider format is invalid",
		},
		{
			name: "missing service account",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ServiceAccount = ""
			}),
			wantErr: "google wif: service account is required",
		},
		{
			name: "invalid service account format",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ServiceAccount = "not-an-email"
			}),
			wantErr: "google wif: service account format is invalid",
		},
		{
			name: "missing id token",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = ""
			}),
			wantErr: "google wif: id token is required",
		},
		{
			name: "invalid id token",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = "short"
			}),
			wantErr: "google wif: id token format is invalid",
		},
		{
			name: "whitespace project id",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectID = "  \t\n "
			}),
			wantErr: "google wif: project id is required",
		},
		{
			name: "whitespace project number",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ProjectNumber = " \t "
			}),
			wantErr: "google wif: project number is required",
		},
		{
			name: "whitespace workload identity provider",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.WorkloadIdentityProvider = "  \n\t "
			}),
			wantErr: "google wif: workload identity provider is required",
		},
		{
			name: "whitespace service account",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.ServiceAccount = " \t "
			}),
			wantErr: "google wif: service account is required",
		},
		{
			name: "whitespace id token",
			cfg: withTestGoogleWIFConfig(func(c *gcpwif.GoogleWIFConfig) {
				c.IDToken = " \n\t "
			}),
			wantErr: "google wif: id token is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gcpwif.NewGoogleWIF(tc.cfg)
			if tc.wantErr != "" {
				assertGoogleWIFExpectedError(t, tc.wantErr, got, err)
				return
			}
			assertGoogleWIFSuccess(t, got, err)
		})
	}
}

func newMockGoogleWIFServer(t *testing.T, stsCalls, iamCalls *int32, expectedIDToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/token"):
			handleMockSTSRequest(t, w, r, stsCalls, expectedIDToken)
		case strings.Contains(r.URL.Path, ":generateAccessToken"):
			handleMockIAMRequest(t, w, r, iamCalls)
		default:
			t.Fatalf("Unexpected request to %s", r.URL.Path)
		}
	}))
}

func handleMockSTSRequest(t *testing.T, w http.ResponseWriter, r *http.Request, calls *int32, expectedIDToken string) {
	t.Helper()
	atomic.AddInt32(calls, 1)
	if r.Method != http.MethodPost {
		t.Errorf("STS Method = %s, want POST", r.Method)
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("Decode STS body error = %v", err)
	}
	if body["grantType"] != "urn:ietf:params:oauth:grant-type:token-exchange" {
		t.Errorf("STS grantType = %q, want token-exchange", body["grantType"])
	}
	if body["subjectToken"] != expectedIDToken {
		t.Errorf("STS subjectToken = %q, want %q", body["subjectToken"], expectedIDToken)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "sts-federated-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

func handleMockIAMRequest(t *testing.T, w http.ResponseWriter, r *http.Request, calls *int32) {
	t.Helper()
	atomic.AddInt32(calls, 1)
	if r.Method != http.MethodPost {
		t.Errorf("IAM Method = %s, want POST", r.Method)
	}
	if authHeader := r.Header.Get("Authorization"); authHeader != "Bearer sts-federated-token" {
		t.Errorf("IAM Authorization = %q, want Bearer sts-federated-token", authHeader)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accessToken": "gcp-sa-oauth2-access-token",
		"expireTime":  time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
}

func assertGoogleFetchCredentialReturns(t *testing.T, src tokensource.Provider, want tokensource.Credential) {
	t.Helper()
	cred, err := src.FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	if cred != want {
		t.Errorf("FetchCredential() = %+v, want %+v", cred, want)
	}
}

func TestGoogleWIF_FetchCredential_ExchangesSTSAndIAMToken(t *testing.T) {
	var stsCalls int32
	var iamCalls int32

	server := newMockGoogleWIFServer(t, &stsCalls, &iamCalls, validTestGoogleWIFConfig.IDToken)
	defer server.Close()

	cfg := validTestGoogleWIFConfig
	cfg.STSEndpoint = server.URL + "/v1/token"
	cfg.IAMCredentialsEndpoint = server.URL + "/v1/projects/-/serviceAccounts/" + cfg.ServiceAccount + ":generateAccessToken"

	src, err := gcpwif.NewGoogleWIF(cfg)
	if err != nil {
		t.Fatalf("NewGoogleWIF() error = %v", err)
	}

	want := tokensource.Credential{
		Value: "gcp-sa-oauth2-access-token",
		Kind:  tokensource.KindBearer,
	}

	assertGoogleFetchCredentialReturns(t, src, want)
	assertGoogleFetchCredentialReturns(t, src, want) // cache hit

	if sts := atomic.LoadInt32(&stsCalls); sts != 1 {
		t.Errorf("STS calls = %d, want 1 (cached)", sts)
	}
	if iam := atomic.LoadInt32(&iamCalls); iam != 1 {
		t.Errorf("IAM calls = %d, want 1 (cached)", iam)
	}
}

func TestGoogleWIF_ConcurrentFetchCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/v1/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sts-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "concurrent-gcp-sa-token",
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

	const callers = 64
	var wg sync.WaitGroup
	results := make([]tokensource.Credential, callers)
	errs := make([]error, callers)

	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = src.FetchCredential(context.Background())
		}(i)
	}
	wg.Wait()

	want := tokensource.Credential{
		Value: "concurrent-gcp-sa-token",
		Kind:  tokensource.KindBearer,
	}
	for i := range callers {
		if errs[i] != nil || results[i] != want {
			t.Errorf("caller %d: FetchCredential() = %+v, %v", i, results[i], errs[i])
		}
	}
}

func TestGoogleWIF_FetchCredential_CanceledContextReturnsErrorEvenOnCacheHit(t *testing.T) {
	holdServer := make(chan struct{})
	serverStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case serverStarted <- struct{}{}:
		default:
		}
		<-holdServer
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/v1/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sts-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": "cached-valid-sa-token",
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

	// Goroutine 1 starts and acquires the lock while executing the slow server call.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = src.FetchCredential(context.Background())
	}()

	<-serverStarted

	// Goroutine 2 enters FetchCredential with an active context and blocks on g.mu.Lock().
	ctx2, cancel2 := context.WithCancel(context.Background())

	var cred2 tokensource.Credential
	var err2 error
	wg.Add(1)
	go func() {
		defer wg.Done()
		cred2, err2 = src.FetchCredential(ctx2)
	}()

	// Give goroutine 2 a moment to block on the mutex, then cancel its context.
	time.Sleep(10 * time.Millisecond)
	cancel2()

	// Unblock server so goroutine 1 finishes, caches the token, and unlocks the mutex.
	close(holdServer)
	wg.Wait()

	// Goroutine 2 acquired the lock after goroutine 1 cached the token, but its context was canceled.
	if err2 == nil || !errors.Is(err2, context.Canceled) {
		t.Errorf("FetchCredential(ctx2) error = %v, want context.Canceled", err2)
	}
	if cred2.Value != "" {
		t.Errorf("FetchCredential(ctx2) returned credential = %+v, want empty", cred2)
	}
}

func TestGoogleWIF_TokenExpirationAndRefresh(t *testing.T) {
	var stsCalls int32
	var iamCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/v1/token") {
			atomic.AddInt32(&stsCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sts-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		} else {
			call := atomic.AddInt32(&iamCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken": fmt.Sprintf("gcp-token-%d", call),
				// Token expired 1 second ago to force refresh
				"expireTime": time.Now().Add(-1 * time.Second).Format(time.RFC3339),
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

	want1 := tokensource.Credential{Value: "gcp-token-1", Kind: tokensource.KindBearer}
	assertGoogleFetchCredentialReturns(t, src, want1)

	want2 := tokensource.Credential{Value: "gcp-token-2", Kind: tokensource.KindBearer}
	assertGoogleFetchCredentialReturns(t, src, want2)

	if sts := atomic.LoadInt32(&stsCalls); sts != 2 {
		t.Errorf("STS calls = %d, want 2 (refreshed)", sts)
	}
	if iam := atomic.LoadInt32(&iamCalls); iam != 2 {
		t.Errorf("IAM calls = %d, want 2 (refreshed)", iam)
	}
}

func TestGoogleWIF_ConfigIsIndependentOfLaterMutation(t *testing.T) {
	src := newTestGoogleWIF(t)
	mutated := src.Config()
	mutated.IDToken = "mutated-token"
	if len(mutated.Scopes) > 0 {
		mutated.Scopes[0] = "mutated-scope"
	}

	if got := src.Config(); got.IDToken == "mutated-token" {
		t.Errorf("Config() IDToken mutated = %q, want original", got.IDToken)
	}
}
