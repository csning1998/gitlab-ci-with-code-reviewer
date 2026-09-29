package azurewif_test

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
	"ci-tools/internal/tokensource/azurewif"
)

var validTestAzureWIFConfig = azurewif.AzureWIFConfig{
	TenantID:       "663bcc2a-0747-4e40-bc5d-6d8c4450e1f3",
	ClientID:       "11111111-2222-3333-4444-555555555555",
	OpenAIEndpoint: "https://oai-csning1998-lab.openai.azure.com/",
	IDToken:        "jwt.token.here",
}

func withTestAzureWIFConfig(mutate func(c *azurewif.AzureWIFConfig)) azurewif.AzureWIFConfig {
	cfg := validTestAzureWIFConfig
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func newTestAzureWIF(t *testing.T) *azurewif.AzureWIF {
	t.Helper()
	src, err := azurewif.NewAzureWIF(validTestAzureWIFConfig)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}
	return src
}

func assertAzureWIFExpectedError(t *testing.T, wantErr string, got *azurewif.AzureWIF, err error) {
	t.Helper()
	if err == nil || err.Error() != wantErr {
		t.Fatalf("NewAzureWIF() error = %v, want %q", err, wantErr)
	}
	if got != nil {
		t.Fatalf("NewAzureWIF() = %#v, want nil", got)
	}
}

func assertAzureWIFSuccess(t *testing.T, got *azurewif.AzureWIF, err error) {
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
		cfg     azurewif.AzureWIFConfig
		wantErr string
	}{
		{
			name: "valid configuration",
			cfg:  validTestAzureWIFConfig,
		},
		{
			name: "missing tenant id",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.TenantID = ""
			}),
			wantErr: "azure wif: tenant id is required",
		},
		{
			name: "invalid tenant id format",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.TenantID = "invalid-uuid"
			}),
			wantErr: "azure wif: tenant id format is invalid",
		},
		{
			name: "missing client id",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.ClientID = ""
			}),
			wantErr: "azure wif: client id is required",
		},
		{
			name: "invalid client id format",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.ClientID = "invalid-uuid"
			}),
			wantErr: "azure wif: client id format is invalid",
		},
		{
			name: "missing id token",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.IDToken = ""
			}),
			wantErr: "azure wif: id token is required",
		},
		{
			name: "invalid id token",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.IDToken = "short"
			}),
			wantErr: "azure wif: id token format is invalid",
		},
		{
			name: "missing openai endpoint",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.OpenAIEndpoint = ""
			}),
			wantErr: "azure wif: openai endpoint is required",
		},
		{
			name: "whitespace openai endpoint",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.OpenAIEndpoint = "   \t\n"
			}),
			wantErr: "azure wif: openai endpoint is required",
		},
		{
			name: "insecure openai endpoint",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.OpenAIEndpoint = "http://oai-csning1998-lab.openai.azure.com/"
			}),
			wantErr: "azure wif: openai endpoint format is invalid",
		},
		{
			name: "whitespace tenant id",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.TenantID = "  \t\n "
			}),
			wantErr: "azure wif: tenant id is required",
		},
		{
			name: "whitespace client id",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.ClientID = " \t "
			}),
			wantErr: "azure wif: client id is required",
		},
		{
			name: "whitespace id token",
			cfg: withTestAzureWIFConfig(func(c *azurewif.AzureWIFConfig) {
				c.IDToken = " \t\n"
			}),
			wantErr: "azure wif: id token is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := azurewif.NewAzureWIF(tc.cfg)
			if tc.wantErr != "" {
				assertAzureWIFExpectedError(t, tc.wantErr, got, err)
				return
			}
			assertAzureWIFSuccess(t, got, err)
		})
	}
}

func newMockEntraIDServer(t *testing.T, requestCount *int32, expectedConfig azurewif.AzureWIFConfig) *httptest.Server {
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

func verifyEntraIDRequest(t *testing.T, r *http.Request, expectedConfig azurewif.AzureWIFConfig) {
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

	src, err := azurewif.NewAzureWIF(cfg)
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

	src, err := azurewif.NewAzureWIF(cfg)
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

	src, err := azurewif.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = src.FetchCredential(context.Background())
	}()

	<-serverStarted

	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()

	cred2, err2 := src.FetchCredential(ctx2)
	if err2 == nil || !errors.Is(err2, context.Canceled) {
		t.Errorf("FetchCredential(canceledCtx) error = %v, want context.Canceled", err2)
	}
	if cred2.Value != "" {
		t.Errorf("FetchCredential(canceledCtx) returned non-empty credential = %+v", cred2)
	}

	close(unblockServer)
	wg.Wait()

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

	src, err := azurewif.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = src.FetchCredential(context.Background())
	}()

	<-serverStarted

	ctx2, cancel2 := context.WithCancel(context.Background())

	var cred2 tokensource.Credential
	var err2 error
	wg.Add(1)
	go func() {
		defer wg.Done()
		cred2, err2 = src.FetchCredential(ctx2)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel2()

	close(holdServer)
	wg.Wait()

	if err2 == nil || !errors.Is(err2, context.Canceled) {
		t.Errorf("FetchCredential(ctx2) error = %v, want context.Canceled", err2)
	}
	if cred2.Value != "" {
		t.Errorf("FetchCredential(ctx2) returned credential = %+v, want empty", cred2)
	}
}

func TestAzureWIF_TokenExpirationAndRefresh(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":   "Bearer",
			"expires_in":   0,
			"access_token": fmt.Sprintf("azure-token-%d", count),
		})
	}))
	defer server.Close()

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = server.URL + "/token"

	src, err := azurewif.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	want1 := tokensource.Credential{Value: "azure-token-1", Kind: tokensource.KindBearer}
	assertAzureFetchCredentialReturns(t, src, want1)

	want2 := tokensource.Credential{Value: "azure-token-2", Kind: tokensource.KindBearer}
	assertAzureFetchCredentialReturns(t, src, want2)

	if count := atomic.LoadInt32(&requestCount); count != 2 {
		t.Errorf("Token exchange request count = %d, want 2 (refreshed)", count)
	}
}

func TestAzureWIF_FetchCredential_NilContext(t *testing.T) {
	var requestCount int32
	server := newMockEntraIDServer(t, &requestCount, validTestAzureWIFConfig)
	defer server.Close()

	cfg := validTestAzureWIFConfig
	cfg.TokenEndpoint = server.URL + "/" + cfg.TenantID + "/oauth2/v2.0/token"

	src, err := azurewif.NewAzureWIF(cfg)
	if err != nil {
		t.Fatalf("NewAzureWIF() error = %v", err)
	}

	var nilCtx context.Context
	cred, err := src.FetchCredential(nilCtx)
	if err != nil {
		t.Fatalf("FetchCredential(nil) error = %v", err)
	}
	if cred.Value != "mocked-azure-openai-access-token" {
		t.Errorf("FetchCredential(nil) = %q, want mocked-azure-openai-access-token", cred.Value)
	}
}

func TestAzureWIF_ConfigIsIndependentOfLaterMutation(t *testing.T) {
	src := newTestAzureWIF(t)
	mutated := src.Config()
	mutated.IDToken = "replaced"
	mutated.TenantID = "00000000-0000-0000-0000-000000000000"

	if got := src.Config(); got != validTestAzureWIFConfig {
		t.Errorf("Config() = %+v, want %+v", got, validTestAzureWIFConfig)
	}
}
