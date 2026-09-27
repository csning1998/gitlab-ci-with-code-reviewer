package tokensource_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ci-tools/internal/tokensource"
)

var validTestAzureWIFConfig = tokensource.AzureWIFConfig{
	TenantID:       "663bcc2a-0747-4e40-bc5d-6d8c4450e1f3",
	ClientID:       "11111111-2222-3333-4444-555555555555",
	OpenAIEndpoint: "https://oai-csning1998-lab.openai.azure.com/",
	IDToken:        "jwt.token.here",
}

func withTestAzureWIFConfig(mutate func(c *tokensource.AzureWIFConfig)) tokensource.AzureWIFConfig {
	cfg := validTestAzureWIFConfig
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func assertAzureWIFExpectedError(t *testing.T, wantErr string, got *tokensource.AzureWIF, err error) {
	t.Helper()
	if err == nil || err.Error() != wantErr {
		t.Fatalf("NewAzureWIF() error = %v, want %q", err, wantErr)
	}
	if got != nil {
		t.Fatalf("NewAzureWIF() = %#v, want nil", got)
	}
}

func assertAzureWIFSuccess(t *testing.T, got *tokensource.AzureWIF, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("NewAzureWIF() unexpected error = %v", err)
	}
	if got == nil {
		t.Fatal("NewAzureWIF() returned nil")
	}
}

func TestNewAzureWIF_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tokensource.AzureWIFConfig
		wantErr string
	}{
		{
			name: "valid configuration",
			cfg:  validTestAzureWIFConfig,
		},
		{
			name: "missing tenant id",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = ""
			}),
			wantErr: "azure wif: tenant id is required",
		},
		{
			name: "invalid tenant id format",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.TenantID = "invalid-uuid"
			}),
			wantErr: "azure wif: tenant id format is invalid",
		},
		{
			name: "missing client id",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = ""
			}),
			wantErr: "azure wif: client id is required",
		},
		{
			name: "invalid client id format",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.ClientID = "invalid-uuid"
			}),
			wantErr: "azure wif: client id format is invalid",
		},
		{
			name: "missing id token",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = ""
			}),
			wantErr: "azure wif: id token is required",
		},
		{
			name: "invalid id token",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.IDToken = "short"
			}),
			wantErr: "azure wif: id token format is invalid",
		},
		{
			name: "missing openai endpoint",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = ""
			}),
			wantErr: "azure wif: openai endpoint is required",
		},
		{
			name: "insecure openai endpoint",
			cfg: withTestAzureWIFConfig(func(c *tokensource.AzureWIFConfig) {
				c.OpenAIEndpoint = "http://oai-csning1998-lab.openai.azure.com/"
			}),
			wantErr: "azure wif: openai endpoint format is invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokensource.NewAzureWIF(tc.cfg)
			if tc.wantErr != "" {
				assertAzureWIFExpectedError(t, tc.wantErr, got, err)
				return
			}
			assertAzureWIFSuccess(t, got, err)
		})
	}
}

func newMockEntraIDServer(t *testing.T, requestCount *int32, expectedConfig tokensource.AzureWIFConfig) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(requestCount, 1)
		verifyEntraIDRequest(t, r, expectedConfig)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":   "Bearer",
			"expires_in":   3600,
			"access_token": "mocked-azure-openai-access-token",
		})
	}))
}

func verifyEntraIDRequest(t *testing.T, r *http.Request, expectedConfig tokensource.AzureWIFConfig) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("Method = %s, want POST", r.Method)
	}
	if !strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
		t.Errorf("Path = %s, want suffix /oauth2/v2.0/token", r.URL.Path)
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error = %v", err)
	}

	assertFormValue(t, r, "grant_type", "client_credentials")
	assertFormValue(t, r, "client_id", expectedConfig.ClientID)
	assertFormValue(t, r, "client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	assertFormValue(t, r, "client_assertion", expectedConfig.IDToken)
	assertFormValue(t, r, "scope", "https://cognitiveservices.azure.com/.default")
}

func assertFormValue(t *testing.T, r *http.Request, key, want string) {
	t.Helper()
	if got := r.FormValue(key); got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}

func assertAzureFetchCredentialReturns(t *testing.T, src tokensource.Provider, want tokensource.Credential) {
	t.Helper()
	cred, err := src.FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	if cred != want {
		t.Errorf("FetchCredential() = %+v, want %+v", cred, want)
	}
}

func TestAzureWIF_FetchCredential_ExchangesTokenWithEntraID(t *testing.T) {
	var requestCount int32
	server := newMockEntraIDServer(t, &requestCount, validTestAzureWIFConfig)
	defer server.Close()

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = server.URL + "/" + cfg.TenantID + "/oauth2/v2.0/token"

	src, err := tokensource.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	want := tokensource.Credential{
		Value: "mocked-azure-openai-access-token",
		Kind:  tokensource.KindBearer,
	}

	assertAzureFetchCredentialReturns(t, src, want)
	assertAzureFetchCredentialReturns(t, src, want) // cache hit

	if count := atomic.LoadInt32(&requestCount); count != 1 {
		t.Errorf("Token exchange request count = %d, want 1 (cached)", count)
	}
}

func TestAzureWIF_ConcurrentFetchCredential(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":   "Bearer",
			"expires_in":   3600,
			"access_token": "concurrent-azure-token",
		})
	}))
	defer server.Close()

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = server.URL + "/token"

	src, err := tokensource.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
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
		Value: "concurrent-azure-token",
		Kind:  tokensource.KindBearer,
	}
	for i := range callers {
		if errs[i] != nil || results[i] != want {
			t.Errorf("caller %d: FetchCredential() = %+v, %v", i, results[i], errs[i])
		}
	}
}

func TestAzureWIF_ContextCanceledWhileWaitingForLock(t *testing.T) {
	var requestCount int32
	unblockServer := make(chan struct{})
	serverStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		select {
		case serverStarted <- struct{}{}:
		default:
		}
		<-unblockServer
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = server.URL + "/token"

	src, err := tokensource.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	// Goroutine 1 starts and acquires the lock, entering the slow server call.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = src.FetchCredential(context.Background())
	}()

	// Wait until the server has received request 1 (holding the lock in FetchCredential).
	<-serverStarted

	// Goroutine 2 attempts to fetch with a context that gets canceled while blocked on the lock.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2() // cancel immediately before lock can be acquired

	cred2, err2 := src.FetchCredential(ctx2)
	if err2 == nil || !errors.Is(err2, context.Canceled) {
		t.Errorf("FetchCredential(canceledCtx) error = %v, want context.Canceled", err2)
	}
	if cred2.Value != "" {
		t.Errorf("FetchCredential(canceledCtx) returned non-empty credential = %+v", cred2)
	}

	// Unblock server for goroutine 1.
	close(unblockServer)
	wg.Wait()

	// Goroutine 2 MUST NOT have triggered a second network request.
	if count := atomic.LoadInt32(&requestCount); count != 1 {
		t.Errorf("requestCount = %d, want 1 (canceled caller must not fire network request after lock)", count)
	}
}

func TestAzureWIF_FetchCredential_CanceledContextReturnsErrorEvenOnCacheHit(t *testing.T) {
	holdServer := make(chan struct{})
	serverStarted := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case serverStarted <- struct{}{}:
		default:
		}
		<-holdServer
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":   "Bearer",
			"expires_in":   3600,
			"access_token": "cached-valid-token",
		})
	}))
	defer server.Close()

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = server.URL + "/token"

	src, err := tokensource.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	// Goroutine 1 starts and acquires the lock while executing the slow server call.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = src.FetchCredential(context.Background())
	}()

	<-serverStarted

	// Goroutine 2 enters FetchCredential with an active context, passes line 103, and blocks on a.mu.Lock().
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
