package gitlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Change struct {
	NewPath string `json:"new_path"`
	OldPath string `json:"old_path"`
	Diff    string `json:"diff"`
}

type DiffRefs struct {
	BaseSha  string `json:"base_sha"`
	StartSha string `json:"start_sha"`
	HeadSha  string `json:"head_sha"`
}

type MRChanges struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Changes     []Change `json:"changes"`
	DiffRefs    DiffRefs `json:"diff_refs"`
}

// Client provides scoped access to GitLab API endpoints for a target merge request.
type Client struct {
	mrURL string
	token string
	http  *http.Client
}

func New(apiURL, projectID, mrIID, token string) *Client {
	return &Client{
		mrURL: fmt.Sprintf("%s/projects/%s/merge_requests/%s", apiURL, projectID, mrIID),
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// send executes HTTP operations and serves as the single point of conversion for HTTP error status codes into Go errors.
func (c *Client) send(method, url string, payload any) (status int, header http.Header, data []byte, err error) {
	var reader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	data, _ = io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return resp.StatusCode, resp.Header, data, fmt.Errorf("gitlab %s %s status %d: %s", method, url, resp.StatusCode, data)
	}
	return resp.StatusCode, resp.Header, data, nil
}

type mrDetail struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	DiffRefs    DiffRefs `json:"diff_refs"`
}

// fetchDetail GETs the MR detail (title, description, diff refs) without diffs.
func (c *Client) fetchDetail() (*mrDetail, error) {
	_, _, data, err := c.send(http.MethodGet, c.mrURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch MR detail: %w", err)
	}
	var d mrDetail
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse MR detail: %w", err)
	}
	return &d, nil
}

// FetchMRDescription retrieves the merge request description from the detail endpoint without downloading diff payloads,
// enabling full-length validation while bypassing the 2700 character environment variable truncation limit.
func (c *Client) FetchMRDescription() (string, error) {
	d, err := c.fetchDetail()
	if err != nil {
		return "", err
	}
	return d.Description, nil
}

// FetchMR queries merge request metadata and paginated file diffs using the /diffs endpoint,
// which supersedes the legacy /changes endpoint deprecated in GitLab 15.7.
func (c *Client) FetchMR() (*MRChanges, error) {
	detail, err := c.fetchDetail()
	if err != nil {
		return nil, err
	}

	_, diffsHeader, diffsData, err := c.send(http.MethodGet, c.mrURL+"/diffs?per_page=100", nil)
	if err != nil {
		return nil, fmt.Errorf("fetch MR diffs: %w", err)
	}
	if next := diffsHeader.Get("X-Next-Page"); next != "" {
		fmt.Printf("Warning: MR has more than 100 changed files; diffs from page %s onward are not reviewed.\n", next)
	}
	var changes []Change
	if err := json.Unmarshal(diffsData, &changes); err != nil {
		return nil, fmt.Errorf("parse MR diffs: %w", err)
	}

	return &MRChanges{
		Title:       detail.Title,
		Description: detail.Description,
		Changes:     changes,
		DiffRefs:    detail.DiffRefs,
	}, nil
}

func (c *Client) PostDiscussion(body string, position map[string]any) (int, error) {
	status, _, _, err := c.send(http.MethodPost, c.mrURL+"/discussions", map[string]any{"body": body, "position": position})
	return status, err
}

func (c *Client) PostNote(body string) (int, error) {
	status, _, _, err := c.send(http.MethodPost, c.mrURL+"/notes", map[string]any{"body": body})
	return status, err
}
