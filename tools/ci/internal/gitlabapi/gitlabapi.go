// Package gitlabapi provides functions to create Git tags via the GitLab REST API.
// Tags created via in-job Git pushes do not trigger tag pipelines
// (gitlab-org/gitlab#569187); API-based creation bypasses this limitation.
package gitlabapi

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Prevents CI runner stalls by terminating unresponsive HTTP requests after 30s.
const requestTimeout = 30 * time.Second

// Limits error payload reads to 4 KiB to prevent log buffer exhaustion from excessive API responses.
const maxErrorBodyBytes = 4096

var httpClient = &http.Client{Timeout: requestTimeout}

// CreateTag creates a Git tag at the specified target reference in a GitLab project.
//
// Authenticates against apiBaseURL using the PRIVATE-TOKEN header.
func CreateTag(apiBaseURL, projectID, tagName, ref, token string) (err error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/projects/%s/repository/tags", apiBaseURL, url.PathEscape(projectID)))
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return fmt.Errorf("GitLab API returned status %d creating tag %q: %s", resp.StatusCode, tagName, body)
	}

	return nil
}
