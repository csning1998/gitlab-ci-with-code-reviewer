package tokensource_test

import (
	"strings"
	"testing"

	"ci-tools/internal/tokensource"
)

func TestNewClaudeWIF_LengthFormatInjectionAndLeak(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tokensource.ClaudeWIFConfig
		wantErr bool
	}{
		{
			name: "federation rule id suffix at limit",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "fdrl_" + strings.Repeat("a", 64)
			}),
			wantErr: false,
		},
		{
			name: "id token at length limit",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = strings.Repeat("a", 8)
			}),
			wantErr: false,
		},
		{
			name: "workspace at suffix limit",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = "wrkspc_" + strings.Repeat("a", 64)
			}),
			wantErr: false,
		},
		{
			name: "federation rule id prefix only",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "fdrl_"
			}),
			wantErr: true,
		},
		{
			name: "federation rule id suffix too long",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "fdrl_" + strings.Repeat("a", 54) + "supersecret"
			}),
			wantErr: true,
		},
		{
			name: "organization id truncated uuid",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = "abcdef01-2345-4678-89ab"
			}),
			wantErr: true,
		},
		{
			name: "organization id trailing content",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = "abcdef01-2345-4678-89ab-cdef01234567supersecret"
			}),
			wantErr: true,
		},
		{
			name: "service account id prefix only",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.ServiceAccountID = "svac_"
			}),
			wantErr: true,
		},
		{
			name: "service account id suffix too long",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.ServiceAccountID = "svac_" + strings.Repeat("c", 54) + "supersecret"
			}),
			wantErr: true,
		},
		{
			name: "workspace prefix only",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = "wrkspc_"
			}),
			wantErr: true,
		},
		{
			name: "workspace suffix too long",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = "wrkspc_" + strings.Repeat("d", 54) + "supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token too short",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = "jwt_tok"
			}),
			wantErr: true,
		},
		{
			name: "id token too long",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = strings.Repeat("e", 4086) + "supersecret"
			}),
			wantErr: true,
		},
		{
			name: "federation rule id missing prefix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "123456789"
			}),
			wantErr: true,
		},
		{
			name: "federation rule id wrong prefix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "org_123456"
			}),
			wantErr: true,
		},
		{
			name: "federation rule id path characters",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "fdrl_123/../supersecret"
			}),
			wantErr: true,
		},
		{
			name: "organization id wrong prefix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = "svac_123456"
			}),
			wantErr: true,
		},
		{
			name: "service account id wrong prefix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.ServiceAccountID = "wrk_123456"
			}),
			wantErr: true,
		},
		{
			name: "workspace wrong prefix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = "org_optional"
			}),
			wantErr: true,
		},
		{
			name: "id token disallowed character",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = "jwt_token+supersecret"
			}),
			wantErr: true,
		},
		{
			name: "federation rule id carriage return injection",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "fdrl_123\r\nX-Injected: supersecret"
			}),
			wantErr: true,
		},
		{
			name: "organization id nul injection",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = "abcdef01-2345-4678-89ab-cdef01234567\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token carriage return injection",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = "jwt_token\r\nX-Injected: supersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token line feed injection",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = "jwt_token\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "id token nul injection",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = "jwt_token\x00supersecret"
			}),
			wantErr: true,
		},
		{
			name: "workspace carriage return injection",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = "wrkspc_123\r\nsupersecret"
			}),
			wantErr: true,
		},
		{
			name: "federation rule id multi byte suffix at byte limit",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.FederationRuleID = "fdrl_" + strings.Repeat("é", 32)
			}),
			wantErr: true,
		},
		{
			name: "organization id multi byte suffix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.OrganizationID = "éécdef01-2345-4678-89ab-cdef01234567"
			}),
			wantErr: true,
		},
		{
			name: "service account id multi byte suffix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.ServiceAccountID = "svac_é"
			}),
			wantErr: true,
		},
		{
			name: "workspace multi byte suffix",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.WorkspaceID = "wrkspc_é"
			}),
			wantErr: true,
		},
		{
			name: "id token multi byte content at byte minimum",
			cfg: withTestClaudeWIFConfig(func(c *tokensource.ClaudeWIFConfig) {
				c.IDToken = strings.Repeat("é", 4)
			}),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokensource.NewClaudeWIF(tc.cfg)
			assertBoundaryResult(t, tc.cfg, tc.wantErr, got, err)
		})
	}
}

func assertBoundaryResult(t *testing.T, cfg tokensource.ClaudeWIFConfig, wantErr bool, got *tokensource.ClaudeWIF, err error) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Fatalf("NewClaudeWIF() error = %v, wantErr = %v", err, wantErr)
	}
	if err != nil {
		assertBoundaryErrorSanitized(t, cfg, err)
		return
	}
	assertBoundarySuccess(t, cfg, got)
}

func assertBoundaryErrorSanitized(t *testing.T, cfg tokensource.ClaudeWIFConfig, err error) {
	t.Helper()
	assertClaudeWIFErrorOmitsValues(t, err, cfg)
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("NewClaudeWIF() error %q contains secret material", err)
	}
}

func assertBoundarySuccess(t *testing.T, cfg tokensource.ClaudeWIFConfig, got *tokensource.ClaudeWIF) {
	t.Helper()
	if got == nil {
		t.Fatal("NewClaudeWIF() returned nil")
	}
	if got.Config() != cfg {
		t.Errorf("Config() = %+v, want %+v", got.Config(), cfg)
	}
}

func assertClaudeWIFErrorOmitsValues(t *testing.T, err error, cfg tokensource.ClaudeWIFConfig) {
	t.Helper()
	for _, value := range []string{
		cfg.FederationRuleID,
		cfg.OrganizationID,
		cfg.ServiceAccountID,
		cfg.IDToken,
		cfg.WorkspaceID,
	} {
		if len(value) < 8 || strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("NewClaudeWIF() error %q contains a configured field value", err)
		}
	}
}
