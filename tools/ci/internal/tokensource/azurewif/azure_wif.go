package azurewif

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"ci-tools/internal/tokensource"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AzureWIFConfig specifies parameters for Microsoft Azure Workload Identity Federation.
type AzureWIFConfig struct {
	TenantID       string
	ClientID       string
	OpenAIEndpoint string
	IDToken        string
	TokenEndpoint  string
	Scope          string
	HTTPClient     *http.Client
}

// AzureWIF holds state and cached access token for Azure Entra ID token exchange.
type AzureWIF struct {
	cfg        AzureWIFConfig
	httpClient *http.Client
	mu         sync.Mutex
	cachedTok  string
	expiresAt  time.Time
}

// isJWTRune checks whether r is a valid JWT character (alphanumeric, dot, underscore, or hyphen).
func isJWTRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

// validateIDToken checks that token length is within 8..4096 and contains only base64url/JWT characters.
func validateIDToken(token string) bool {
	if len(token) < 8 || len(token) > 4096 {
		return false
	}
	for _, r := range token {
		if !isJWTRune(r) {
			return false
		}
	}
	return true
}

// NewAzureWIF constructs an AzureWIF token source after validating required configuration fields.
func NewAzureWIF(cfg AzureWIFConfig) (*AzureWIF, error) {
	if strings.TrimSpace(cfg.TenantID) == "" {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldTenantID, Reason: tokensource.ReasonRequired}
	}
	if !uuidPattern.MatchString(strings.TrimSpace(cfg.TenantID)) {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldTenantID, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldClientID, Reason: tokensource.ReasonRequired}
	}
	if !uuidPattern.MatchString(strings.TrimSpace(cfg.ClientID)) {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldClientID, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.IDToken) == "" {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldIDToken, Reason: tokensource.ReasonRequired}
	}
	if !validateIDToken(strings.TrimSpace(cfg.IDToken)) {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldIDToken, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.OpenAIEndpoint) == "" {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldOpenAIEndpoint, Reason: tokensource.ReasonRequired}
	}
	if !isValidOpenAIEndpoint(strings.TrimSpace(cfg.OpenAIEndpoint)) {
		return nil, &tokensource.FieldError{Provider: "azure", Field: tokensource.FieldOpenAIEndpoint, Reason: tokensource.ReasonInvalidFormat}
	}

	trimmedCfg := cfg
	trimmedCfg.TenantID = strings.TrimSpace(cfg.TenantID)
	trimmedCfg.ClientID = strings.TrimSpace(cfg.ClientID)
	trimmedCfg.OpenAIEndpoint = strings.TrimSpace(cfg.OpenAIEndpoint)
	trimmedCfg.IDToken = strings.TrimSpace(cfg.IDToken)
	trimmedCfg.TokenEndpoint = strings.TrimSpace(cfg.TokenEndpoint)
	trimmedCfg.Scope = strings.TrimSpace(cfg.Scope)

	client := trimmedCfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &AzureWIF{
		cfg:        trimmedCfg,
		httpClient: client,
	}, nil
}

// Config returns a copy of the underlying AzureWIFConfig.
func (a *AzureWIF) Config() AzureWIFConfig {
	return a.cfg
}

// FetchCredential exchanges the GitLab ID token for an Azure Entra ID access token.
func (a *AzureWIF) FetchCredential(ctx context.Context) (tokensource.Credential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return tokensource.Credential{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return tokensource.Credential{}, err
	}

	if a.cachedTok != "" && time.Now().Before(a.expiresAt) {
		return tokensource.Credential{Value: a.cachedTok, Kind: tokensource.KindBearer}, nil
	}

	data := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {a.cfg.ClientID},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {a.cfg.IDToken},
		"scope":                 {a.resolveScope()},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.resolveTokenEndpoint(), strings.NewReader(data.Encode()))
	if err != nil {
		return tokensource.Credential{}, fmt.Errorf("azure wif: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return tokensource.Credential{}, fmt.Errorf("azure wif: token exchange request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokensource.Credential{}, fmt.Errorf("azure wif: read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return tokensource.Credential{}, fmt.Errorf("azure wif: token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return tokensource.Credential{}, fmt.Errorf("azure wif: unmarshal token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return tokensource.Credential{}, errors.New("azure wif: token response missing access_token")
	}
	if tokenResp.ExpiresIn < 0 {
		return tokensource.Credential{}, errors.New("azure wif: token response expires_in is invalid")
	}

	if tokenResp.ExpiresIn > 60 {
		a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)
		a.cachedTok = tokenResp.AccessToken
	} else if tokenResp.ExpiresIn > 0 {
		a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		a.cachedTok = tokenResp.AccessToken
	} else {
		a.cachedTok = ""
		a.expiresAt = time.Time{}
	}

	return tokensource.Credential{Value: tokenResp.AccessToken, Kind: tokensource.KindBearer}, nil
}

func isValidOpenAIEndpoint(raw string) bool {
	if strings.ContainsAny(raw, "\x00\r\n") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return true
}

func (a *AzureWIF) resolveTokenEndpoint() string {
	if a.cfg.TokenEndpoint != "" {
		return a.cfg.TokenEndpoint
	}
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", a.cfg.TenantID)
}

func (a *AzureWIF) resolveScope() string {
	if a.cfg.Scope != "" {
		return a.cfg.Scope
	}
	return "https://cognitiveservices.azure.com/.default"
}

// ModeDescription returns the Microsoft Azure native workload identity federation mode label.
func (a *AzureWIF) ModeDescription() string {
	return "Mode: Workload Identity Federation (Azure OpenAI Native)"
}
