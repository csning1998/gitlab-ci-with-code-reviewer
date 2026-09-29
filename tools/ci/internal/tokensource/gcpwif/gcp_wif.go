package gcpwif

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

	"ci-tools/internal/tokensource"
)

var (
	projectIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	projectNumberPattern  = regexp.MustCompile(`^[0-9]{6,20}$`)
	gcpProviderPattern    = regexp.MustCompile(`^projects/[0-9]{6,20}/locations/global/workloadIdentityPools/[a-z0-9-]+/providers/[a-z0-9-]+$`)
	serviceAccountPattern = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z][a-z0-9-]{4,28}[a-z0-9]\.iam\.gserviceaccount\.com$`)
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

func isValidWorkloadIdentityProvider(provider, projectNumber string) bool {
	if !gcpProviderPattern.MatchString(provider) {
		return false
	}
	number, _, found := strings.Cut(strings.TrimPrefix(provider, "projects/"), "/")
	return found && number == projectNumber
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

// NewGoogleWIF constructs a GoogleWIF token source after validating required configuration fields.
func NewGoogleWIF(cfg GoogleWIFConfig) (*GoogleWIF, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldProjectID, Reason: tokensource.ReasonRequired}
	}
	if !projectIDPattern.MatchString(strings.TrimSpace(cfg.ProjectID)) {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldProjectID, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.ProjectNumber) == "" {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldProjectNumber, Reason: tokensource.ReasonRequired}
	}
	if !projectNumberPattern.MatchString(strings.TrimSpace(cfg.ProjectNumber)) {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldProjectNumber, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.WorkloadIdentityProvider) == "" {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldWorkloadIdentityProvider, Reason: tokensource.ReasonRequired}
	}
	if !isValidWorkloadIdentityProvider(strings.TrimSpace(cfg.WorkloadIdentityProvider), strings.TrimSpace(cfg.ProjectNumber)) {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldWorkloadIdentityProvider, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.ServiceAccount) == "" {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldServiceAccount, Reason: tokensource.ReasonRequired}
	}
	if !serviceAccountPattern.MatchString(strings.TrimSpace(cfg.ServiceAccount)) {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldServiceAccount, Reason: tokensource.ReasonInvalidFormat}
	}

	if strings.TrimSpace(cfg.IDToken) == "" {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldIDToken, Reason: tokensource.ReasonRequired}
	}
	if !validateIDToken(strings.TrimSpace(cfg.IDToken)) {
		return nil, &tokensource.FieldError{Provider: "google", Field: tokensource.FieldIDToken, Reason: tokensource.ReasonInvalidFormat}
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
func (g *GoogleWIF) FetchCredential(ctx context.Context) (tokensource.Credential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return tokensource.Credential{}, err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return tokensource.Credential{}, err
	}

	if g.cachedTok != "" && time.Now().Before(g.expiresAt) {
		return tokensource.Credential{Value: g.cachedTok, Kind: tokensource.KindBearer}, nil
	}

	stsToken, err := g.exchangeSTS(ctx)
	if err != nil {
		return tokensource.Credential{}, err
	}

	tok, expiresAt, err := g.generateAccessToken(ctx, stsToken)
	if err != nil {
		return tokensource.Credential{}, err
	}

	g.cachedTok = tok
	g.expiresAt = expiresAt
	return tokensource.Credential{Value: tok, Kind: tokensource.KindBearer}, nil
}

func (g *GoogleWIF) exchangeSTS(ctx context.Context) (string, error) {
	stsPayload := map[string]any{
		"audience":           "//iam.googleapis.com/" + g.cfg.WorkloadIdentityProvider,
		"grantType":          "urn:ietf:params:oauth:grant-type:token-exchange",
		"requestedTokenType": "urn:ietf:params:oauth:token-type:access_token",
		"scope":              "https://www.googleapis.com/auth/cloud-platform",
		"subjectTokenType":   "urn:ietf:params:oauth:token-type:jwt",
		"subjectToken":       g.cfg.IDToken,
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
		parsed, err := time.Parse(time.RFC3339, iamResp.ExpireTime)
		if err != nil {
			return "", time.Time{}, errors.New("google wif: iam response expireTime format is invalid")
		}
		expiry = parsed
	}

	return iamResp.AccessToken, expiry, nil
}

// ModeDescription returns the Google Cloud native workload identity federation mode label.
func (g *GoogleWIF) ModeDescription() string {
	return "Mode: Workload Identity Federation (Google Cloud Native)"
}
