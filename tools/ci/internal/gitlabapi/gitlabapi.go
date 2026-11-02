// Package gitlabapi provides functions to create Git tags via the GitLab REST API.
// Tags created via in-job Git pushes do not trigger tag pipelines
// (gitlab-org/gitlab#569187). API-based creation bypasses this limitation.
package gitlabapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"ci-tools/internal/httpguard"
)

// Prevents CI runner stalls by terminating unresponsive HTTP requests after 30s.
const requestTimeout = 30 * time.Second

// Limits response body reads to 4 KiB to prevent log buffer exhaustion from excessive API responses.
const maxResponseBodyBytes = 4096

var httpClient = &http.Client{Timeout: requestTimeout, CheckRedirect: httpguard.RefuseCrossHostRedirect}

// CreateTag creates a Git tag at the specified target reference in a GitLab project.
//
// Authenticates against apiBaseURL using the PRIVATE-TOKEN header.
func CreateTag(apiBaseURL, projectID, tagName, ref, token string) (err error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/projects/%s/repository/tags", strings.TrimRight(apiBaseURL, "/"), url.PathEscape(projectID)))
	if err != nil {
		return fmt.Errorf("failed to construct tag creation endpoint: %w", err)
	}
	endpoint.RawQuery = url.Values{"tag_name": {tagName}, "ref": {ref}}.Encode()

	req, err := http.NewRequest(http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to construct tag creation request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach GitLab API at %q: %w", endpoint.String(), err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		return fmt.Errorf("GitLab API returned status %d creating tag %q: %s", resp.StatusCode, tagName, body)
	}

	return nil
}

// tokenSelf defines the subset of fields parsed from GET /personal_access_tokens/self.
type tokenSelf struct {
	Scopes  []string `json:"scopes"`
	Active  bool     `json:"active"`
	Revoked bool     `json:"revoked"`
}

// VerifyScope validates an active, unrevoked personal access token possessing
// requiredScope via GET /personal_access_tokens/self. The check executes no write operations.
func VerifyScope(apiBaseURL, token, requiredScope string) (err error) {
	endpoint := fmt.Sprintf("%s/personal_access_tokens/self", strings.TrimRight(apiBaseURL, "/"))

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to construct token introspection request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach GitLab API at %q: %w", endpoint, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		return fmt.Errorf("GitLab API returned status %d introspecting the token: %s", resp.StatusCode, body)
	}

	var info tokenSelf
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&info); err != nil {
		return fmt.Errorf("failed to parse token introspection response: %w", err)
	}

	if info.Revoked || !info.Active {
		return fmt.Errorf("token is revoked or inactive")
	}
	if !slices.Contains(info.Scopes, requiredScope) {
		return fmt.Errorf("token scopes %v do not include the required %q scope", info.Scopes, requiredScope)
	}

	return nil
}

// VerifyTagAccess validates that token can read the repository tags of projectID without writing.
// A tag read accepts both a legacy api-scoped token and a fine-grained Repository Tag token,
// whereas /personal_access_tokens/self rejects a fine-grained token lacking that permission.
func VerifyTagAccess(apiBaseURL, projectID, token string) (err error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/projects/%s/repository/tags", strings.TrimRight(apiBaseURL, "/"), url.PathEscape(projectID)))
	if err != nil {
		return fmt.Errorf("failed to construct tag read endpoint: %w", err)
	}
	endpoint.RawQuery = url.Values{"per_page": {"1"}}.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to construct tag read request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach GitLab API at %q: %w", endpoint.String(), err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("GitLab API returned status 401 because the token is invalid, expired, or revoked")
	case http.StatusForbidden:
		return fmt.Errorf("GitLab API returned status 403 because the token cannot read the repository tags of project %q (requires the api scope or Repository Tag Create and Read)", projectID)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		return fmt.Errorf("GitLab API returned status %d reading the repository tags: %s", resp.StatusCode, body)
	}
}
