package tokensource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	projectNumberPattern  = regexp.MustCompile(`^[0-9]+$`)
	gcpProviderPattern    = regexp.MustCompile(`^projects/[0-9]+/locations/[a-z0-9-]+/workloadIdentityPools/[a-z0-9-]+/providers/[a-z0-9-]+$`)
	serviceAccountPattern = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
)

// GoogleWIFConfig specifies parameters for Google Cloud Workload Identity Federation.
type GoogleWIFConfig struct {
	ProjectID                string
	ProjectNumber            string
	WorkloadIdentityProvider string
	ServiceAccount           string
	IDToken                  string
	STSEndpoint              string
	IAMCredentialsEndpoint   string
	Scopes                   []string
	HTTPClient               *http.Client
}

// GoogleWIF holds state and cached access token for GCP STS and IAM Credentials token exchange.
type GoogleWIF struct {
	cfg        GoogleWIFConfig
	httpClient *http.Client
	mu         sync.Mutex
	cachedTok  string
	expiresAt  time.Time
}

// NewGoogleWIF constructs a GoogleWIF token source after validating required configuration fields.
func NewGoogleWIF(cfg GoogleWIFConfig) (*GoogleWIF, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, newGoogleRequiredFieldError(FieldProjectID)
	}

	if strings.TrimSpace(cfg.ProjectNumber) == "" {
		return nil, newGoogleRequiredFieldError(FieldProjectNumber)
	}
	if !projectNumberPattern.MatchString(strings.TrimSpace(cfg.ProjectNumber)) {
		return nil, newGoogleInvalidFormatFieldError(FieldProjectNumber)
	}

	if strings.TrimSpace(cfg.WorkloadIdentityProvider) == "" {
		return nil, newGoogleRequiredFieldError(FieldWorkloadIdentityProvider)
	}
	if !gcpProviderPattern.MatchString(strings.TrimSpace(cfg.WorkloadIdentityProvider)) {
		return nil, newGoogleInvalidFormatFieldError(FieldWorkloadIdentityProvider)
	}

	if strings.TrimSpace(cfg.ServiceAccount) == "" {
		return nil, newGoogleRequiredFieldError(FieldServiceAccount)
	}
	if !serviceAccountPattern.MatchString(strings.TrimSpace(cfg.ServiceAccount)) {
		return nil, newGoogleInvalidFormatFieldError(FieldServiceAccount)
	}

	if strings.TrimSpace(cfg.IDToken) == "" {
		return nil, newGoogleRequiredFieldError(FieldIDToken)
	}
	if !validateIDToken(strings.TrimSpace(cfg.IDToken)) {
		return nil, newGoogleInvalidFormatFieldError(FieldIDToken)
	}

	trimmedCfg := cfg
	trimmedCfg.ProjectID = strings.TrimSpace(cfg.ProjectID)
	trimmedCfg.ProjectNumber = strings.TrimSpace(cfg.ProjectNumber)
	trimmedCfg.WorkloadIdentityProvider = strings.TrimSpace(cfg.WorkloadIdentityProvider)
	trimmedCfg.ServiceAccount = strings.TrimSpace(cfg.ServiceAccount)
	trimmedCfg.IDToken = strings.TrimSpace(cfg.IDToken)

	if trimmedCfg.STSEndpoint == "" {
		trimmedCfg.STSEndpoint = "https://sts.googleapis.com/v1/token"
	}
	if trimmedCfg.IAMCredentialsEndpoint == "" {
		trimmedCfg.IAMCredentialsEndpoint = fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken", trimmedCfg.ServiceAccount)
	}
	if len(trimmedCfg.Scopes) == 0 {
		trimmedCfg.Scopes = []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/generative-language",
		}
	}

	client := trimmedCfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &GoogleWIF{
		cfg:        trimmedCfg,
		httpClient: client,
	}, nil
}

// Config returns a copy of the underlying GoogleWIFConfig.
func (g *GoogleWIF) Config() GoogleWIFConfig {
	return g.cfg
}

// FetchCredential exchanges the GitLab ID token for a Google Cloud service account access token.
func (g *GoogleWIF) FetchCredential(ctx context.Context) (Credential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}

	if g.cachedTok != "" && time.Now().Before(g.expiresAt) {
		return Credential{Value: g.cachedTok, Kind: KindBearer}, nil
	}

	stsToken, err := g.exchangeSTS(ctx)
	if err != nil {
		return Credential{}, err
	}

	saToken, expiry, err := g.generateAccessToken(ctx, stsToken)
	if err != nil {
		return Credential{}, err
	}

	g.cachedTok = saToken
	if !expiry.IsZero() && expiry.After(time.Now().Add(60*time.Second)) {
		g.expiresAt = expiry.Add(-60 * time.Second)
	} else if !expiry.IsZero() {
		g.expiresAt = expiry
	} else {
		g.expiresAt = time.Now().Add(3500 * time.Second)
	}

	return Credential{Value: saToken, Kind: KindBearer}, nil
}

func (g *GoogleWIF) exchangeSTS(ctx context.Context) (string, error) {
	audience := g.cfg.WorkloadIdentityProvider
	if !strings.HasPrefix(audience, "//iam.googleapis.com/") {
		audience = "//iam.googleapis.com/" + audience
	}

	stsPayload := map[string]string{
		"audience":           audience,
		"grantType":          "urn:ietf:params:oauth:grant-type:token-exchange",
		"requestedTokenType": "urn:ietf:params:oauth:token-type:access_token",
		"subjectTokenType":   "urn:ietf:params:oauth:token-type:jwt",
		"subjectToken":       g.cfg.IDToken,
		"scope":              strings.Join(g.cfg.Scopes, " "),
	}

	reqBody, err := json.Marshal(stsPayload)
	if err != nil {
		return "", fmt.Errorf("google wif: marshal sts payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.STSEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("google wif: create sts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("google wif: sts token request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("google wif: read sts response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google wif: sts exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var stsResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &stsResp); err != nil {
		return "", fmt.Errorf("google wif: unmarshal sts response: %w", err)
	}
	if stsResp.AccessToken == "" {
		return "", errors.New("google wif: sts response missing access_token")
	}

	return stsResp.AccessToken, nil
}

func (g *GoogleWIF) generateAccessToken(ctx context.Context, stsToken string) (string, time.Time, error) {
	iamPayload := map[string]any{
		"scope":    g.cfg.Scopes,
		"lifetime": "3600s",
	}

	reqBody, err := json.Marshal(iamPayload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("google wif: marshal iam payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.IAMCredentialsEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("google wif: create iam request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+stsToken)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("google wif: iam credentials request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("google wif: read iam response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("google wif: iam generateAccessToken failed with status %d: %s", resp.StatusCode, string(body))
	}

	var iamResp struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  string `json:"expireTime"`
	}
	if err := json.Unmarshal(body, &iamResp); err != nil {
		return "", time.Time{}, fmt.Errorf("google wif: unmarshal iam response: %w", err)
	}
	if iamResp.AccessToken == "" {
		return "", time.Time{}, errors.New("google wif: iam response missing accessToken")
	}

	var expiry time.Time
	if iamResp.ExpireTime != "" {
		t, err := time.Parse(time.RFC3339, iamResp.ExpireTime)
		if err == nil {
			expiry = t
		}
	}

	return iamResp.AccessToken, expiry, nil
}
