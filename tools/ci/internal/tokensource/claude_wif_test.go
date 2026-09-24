package tokensource_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"ci-tools/internal/tokensource"
)

var validTestClaudeWIFConfig = tokensource.ClaudeWIFConfig{
	FederationRuleID: "fdrl_123",
	OrganizationID:   "org_123",
	ServiceAccountID: "svac_123",
	IDToken:          "jwt_token",
	WorkspaceID:      "wrk_123",
}

func withTestClaudeWIFConfig(mutate func(c *tokensource.ClaudeWIFConfig)) tokensource.ClaudeWIFConfig {
	cfg := validTestClaudeWIFConfig
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func newTestClaudeWIF(t *testing.T) *tokensource.ClaudeWIF {
	t.Helper()
	src, err := tokensource.NewClaudeWIF(validTestClaudeWIFConfig)
	if err != nil {
		t.Fatalf("NewClaudeWIF() error = %v", err)
	}
	return src
}

func TestNewClaudeWIF_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tokensource.ClaudeWIFConfig
		wantErr string
	}{
		{
			name: "required fields without workspace",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = ""
			}),
		},
		{
			name: "optional workspace included",
			cfg:  validTestClaudeWIFConfig,
		},
		{
			name: "long id token",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = strings.Repeat("a", 4096)
			}),
		},
		{
			name: "missing federation rule id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = ""
			}),
			wantErr: "claude wif: federation rule id is required",
		},
		{
			name: "whitespace federation rule id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = " \t\n"
			}),
			wantErr: "claude wif: federation rule id is required",
		},
		{
			name: "nbsp federation rule id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "\u00a0"
			}),
			wantErr: "claude wif: federation rule id is required",
		},
		{
			name: "missing organization id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = ""
			}),
			wantErr: "claude wif: organization id is required",
		},
		{
			name: "whitespace organization id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = "   "
			}),
			wantErr: "claude wif: organization id is required",
		},
		{
			name: "missing service account id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.ServiceAccountID = ""
			}),
			wantErr: "claude wif: service account id is required",
		},
		{
			name: "whitespace service account id",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.ServiceAccountID = " \t\n "
			}),
			wantErr: "claude wif: service account id is required",
		},
		{
			name: "missing id token",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = ""
			}),
			wantErr: "claude wif: id token is required",
		},
		{
			name: "whitespace id token",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = "  "
			}),
			wantErr: "claude wif: id token is required",
		},
		{
			name:    "empty config reports federation rule id",
			cfg:     tokensource.ClaudeWIFConfig{},
			wantErr: "claude wif: federation rule id is required",
		},
		{
			name: "workspace alone reports federation rule id",
			cfg: tokensource.ClaudeWIFConfig{
				WorkspaceID: "wrk_123",
			},
			wantErr: "claude wif: federation rule id is required",
		},
		{
			name: "organization id precedes later fields",
			cfg: tokensource.ClaudeWIFConfig{
				FederationRuleID: "fdrl_123",
			},
			wantErr: "claude wif: organization id is required",
		},
		{
			name: "service account id precedes id token",
			cfg: tokensource.ClaudeWIFConfig{
				FederationRuleID: "fdrl_123",
				OrganizationID:   "org_123",
			},
			wantErr: "claude wif: service account id is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokensource.NewClaudeWIF(tc.cfg)
			if tc.wantErr != "" {
				assertClaudeWIFExpectedError(t, tc.cfg, tc.wantErr, got, err)
				return
			}
			assertClaudeWIFSuccess(t, tc.cfg, got, err)
		})
	}
}

func assertClaudeWIFSuccess(t *testing.T, cfg tokensource.ClaudeWIFConfig, got *tokensource.ClaudeWIF, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("NewClaudeWIF() error = %v", err)
	}
	if got == nil {
		t.Fatal("NewClaudeWIF() returned nil")
	}
	if got.Config() != cfg {
		t.Errorf("Config() = %+v, want %+v", got.Config(), cfg)
	}
	cred, err := got.FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	if cred.Value != cfg.IDToken || cred.Kind != tokensource.KindBearer {
		t.Errorf("FetchCredential() = %+v, want Value %q Kind %v", cred, cfg.IDToken, tokensource.KindBearer)
	}
}

func assertClaudeWIFExpectedError(t *testing.T, cfg tokensource.ClaudeWIFConfig, wantErr string, got *tokensource.ClaudeWIF, err error) {
	t.Helper()
	if err == nil || err.Error() != wantErr {
		t.Fatalf("NewClaudeWIF() error = %v, want %q", err, wantErr)
	}
	if got != nil {
		t.Fatalf("NewClaudeWIF() = %#v, want nil", got)
	}
	assertClaudeWIFErrorOmitsValues(t, err, cfg)
}

func TestClaudeWIF_FetchCredentialIgnoresCanceledContext(t *testing.T) {
	src := newTestClaudeWIF(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cred, err := src.FetchCredential(ctx)
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	want := tokensource.Credential{
		Value: validTestClaudeWIFConfig.IDToken,
		Kind:  tokensource.KindBearer,
	}
	if cred != want {
		t.Errorf("FetchCredential() = %+v, want %+v", cred, want)
	}
}

func TestClaudeWIF_ConfigIsIndependentOfLaterMutation(t *testing.T) {
	src := newTestClaudeWIF(t)

	mutated := src.Config()
	mutated.IDToken = "replaced"
	mutated.WorkspaceID = "replaced"

	cred, err := src.FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	want := tokensource.Credential{
		Value: validTestClaudeWIFConfig.IDToken,
		Kind:  tokensource.KindBearer,
	}
	if cred != want {
		t.Errorf("FetchCredential() = %+v, want %+v", cred, want)
	}
	if got := src.Config(); got != validTestClaudeWIFConfig {
		t.Errorf("Config() = %+v, want %+v", got, validTestClaudeWIFConfig)
	}
}

func TestClaudeWIF_ConcurrentFetchCredential(t *testing.T) {
	const callers = 128

	src := newTestClaudeWIF(t)
	results := make([]tokensource.Credential, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = src.FetchCredential(context.Background())
		}(i)
	}
	wg.Wait()

	want := tokensource.Credential{
		Value: validTestClaudeWIFConfig.IDToken,
		Kind:  tokensource.KindBearer,
	}
	for i := range callers {
		if errs[i] != nil || results[i] != want {
			t.Errorf("caller %d: FetchCredential() = %+v, %v", i, results[i], errs[i])
		}
	}
}
