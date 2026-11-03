// Package gitlabtoken admits a GitLab token into the API clients only after the token proves to be
// a fine-grained personal access token.
package gitlabtoken

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ci-tools/internal/httpguard"
)

// Prevents CI runner stalls by terminating unresponsive HTTP requests after 30s.
const requestTimeout = 30 * time.Second

// Limits error body reads to 4 KiB to prevent log buffer exhaustion from excessive API responses.
const maxResponseBodyBytes = 4096

// maxSelfResponseBytes limits the /personal_access_tokens/self response size to 64 KiB, since a fine-grained token lists every granted permission.
const maxSelfResponseBytes = 64 << 10

var httpClient = &http.Client{Timeout: requestTimeout, CheckRedirect: httpguard.RefuseCrossHostRedirect}

// redactedToken replaces the raw value whenever a FineGrainedToken is formatted.
const redactedToken = "[REDACTED]"

// FineGrainedToken is a GitLab token which passed New.
//
// The unexported field prevents other packages from constructing a token without the check.
type FineGrainedToken struct {
	value string
}

// Value returns the raw token for the PRIVATE-TOKEN header.
func (t FineGrainedToken) Value() string {
	return t.value
}

// String redacts the raw value for the %v, %+v, %s, and %q verbs.
func (t FineGrainedToken) String() string {
	return redactedToken
}

// GoString redacts the raw value for the %#v verb.
func (t FineGrainedToken) GoString() string {
	return redactedToken
}

// New returns raw as a FineGrainedToken, or an error unless raw is a fine-grained personal access token.
//
// The /personal_access_tokens/self endpoint returns HTTP 403 with error "insufficient_granular_scope"
// for a fine-grained token lacking Personal Access Token Read, and returns HTTP 200 without granular scopes
// for a legacy token.
func New(ctx context.Context, apiBaseURL, raw string) (FineGrainedToken, error) {
	if raw == "" {
		return FineGrainedToken{}, errors.New("the GitLab token is empty")
	}
	if err := rejectLegacyToken(ctx, apiBaseURL, raw); err != nil {
		return FineGrainedToken{}, err
	}
	return FineGrainedToken{value: raw}, nil
}

func rejectLegacyToken(ctx context.Context, apiBaseURL, raw string) (err error) {
	endpoint := strings.TrimRight(apiBaseURL, "/") + "/personal_access_tokens/self"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to construct request for /personal_access_tokens/self: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", raw)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach GitLab API at %q: %w", endpoint, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		var self struct {
			Scopes         []string          `json:"scopes"`
			GranularScopes []json.RawMessage `json:"granular_scopes"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxSelfResponseBytes)).Decode(&self); err != nil {
			return fmt.Errorf("failed to decode response from /personal_access_tokens/self: %w", err)
		}
		if len(self.GranularScopes) == 0 {
			return fmt.Errorf("token carries legacy scopes %v, which are prohibited; a fine-grained personal access token is required", self.Scopes)
		}
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("GitLab API returned status 401 because the token is invalid, expired, or revoked")
	case http.StatusForbidden:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		var denial struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &denial) == nil && denial.Error == "insufficient_granular_scope" {
			return nil
		}
		return fmt.Errorf("GitLab API returned status 403 from /personal_access_tokens/self: %s", body)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		return fmt.Errorf("GitLab API returned status %d from /personal_access_tokens/self: %s", resp.StatusCode, body)
	}
}
