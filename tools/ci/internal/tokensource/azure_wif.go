package tokensource

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

// NewAzureWIF constructs an AzureWIF token source after validating required configuration fields.
func NewAzureWIF(cfg AzureWIFConfig) (*AzureWIF, error) {
	if strings.TrimSpace(cfg.TenantID) == "" {
		return nil, newAzureRequiredFieldError(FieldTenantID)
	}
	if !uuidPattern.MatchString(strings.TrimSpace(cfg.TenantID)) {
		return nil, newAzureInvalidFormatFieldError(FieldTenantID)
	}

	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, newAzureRequiredFieldError(FieldClientID)
	}
	if !uuidPattern.MatchString(strings.TrimSpace(cfg.ClientID)) {
		return nil, newAzureInvalidFormatFieldError(FieldClientID)
	}

	if strings.TrimSpace(cfg.IDToken) == "" {
		return nil, newAzureRequiredFieldError(FieldIDToken)
	}
	if !validateIDToken(strings.TrimSpace(cfg.IDToken)) {
		return nil, newAzureInvalidFormatFieldError(FieldIDToken)
	}

	if strings.TrimSpace(cfg.OpenAIEndpoint) == "" {
		return nil, newAzureRequiredFieldError(FieldOpenAIEndpoint)
	}
	if !strings.HasPrefix(strings.TrimSpace(cfg.OpenAIEndpoint), "https://") {
		return nil, newAzureInvalidFormatFieldError(FieldOpenAIEndpoint)
	}

	trimmedCfg := cfg
	trimmedCfg.TenantID = strings.TrimSpace(cfg.TenantID)
	trimmedCfg.ClientID = strings.TrimSpace(cfg.ClientID)
	trimmedCfg.OpenAIEndpoint = strings.TrimSpace(cfg.OpenAIEndpoint)
	trimmedCfg.IDToken = strings.TrimSpace(cfg.IDToken)

	if trimmedCfg.TokenEndpoint == "" {
		trimmedCfg.TokenEndpoint = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", trimmedCfg.TenantID)
	}
	if trimmedCfg.Scope == "" {
		trimmedCfg.Scope = "https://cognitiveservices.azure.com/.default"
	}

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
func (a *AzureWIF) FetchCredential(ctx context.Context) (Credential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}

	if a.cachedTok != "" && time.Now().Before(a.expiresAt) {
		return Credential{Value: a.cachedTok, Kind: KindBearer}, nil
	}

	data := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {a.cfg.ClientID},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {a.cfg.IDToken},
		"scope":                 {a.cfg.Scope},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return Credential{}, fmt.Errorf("azure wif: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return Credential{}, fmt.Errorf("azure wif: token exchange request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Credential{}, fmt.Errorf("azure wif: read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Credential{}, fmt.Errorf("azure wif: token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return Credential{}, fmt.Errorf("azure wif: unmarshal token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return Credential{}, errors.New("azure wif: token response missing access_token")
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

	return Credential{Value: tokenResp.AccessToken, Kind: KindBearer}, nil
}
