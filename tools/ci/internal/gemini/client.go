package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// redactedString enforces credential masking by overriding fmt.Stringer to return [REDACTED],
// preventing sensitive API keys from appearing in diagnostic logs while preserving raw value retrieval for HTTP headers.
type redactedString string

func (redactedString) String() string  { return "[REDACTED]" }
func (s redactedString) value() string { return string(s) }

// Client manages HTTP interactions with the Gemini generateContent REST endpoint for a configured model.
type Client struct {
	url    string
	apiKey redactedString
	http   *http.Client
}

func New(model, apiKey string) *Client {
	return &Client{
		url: fmt.Sprintf(
			"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
			model,
		),
		apiKey: redactedString(apiKey),
		http:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) Name() string { return "Gemini" }

// Review submits the prompt payload to the Gemini API, enforcing JSON structured response configuration
// and aggregating returned candidate text parts.
func (c *Client) Review(prompt string) (result string, err error) {
	payload := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt}}}},
		"generationConfig": map[string]any{"responseMimeType": "application/json"},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey.value())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini api %d: %s", resp.StatusCode, data)
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}
