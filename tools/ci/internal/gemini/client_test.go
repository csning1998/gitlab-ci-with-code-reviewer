package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// redirectTransport rewrites every outbound request to target a local httptest server instead of
// the real Gemini endpoint, since Client's URL is a fixed constant with no injection point.
type redirectTransport struct {
	target string
	base   http.RoundTripper
}

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	targetURL, err := http.NewRequest(req.Method, rt.target, req.Body)
	if err != nil {
		return nil, err
	}
	clone.URL = targetURL.URL
	clone.Host = targetURL.URL.Host
	return rt.base.RoundTrip(clone)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c := New("gemini-test-model", "test-api-key")
	c.http = &http.Client{Transport: redirectTransport{target: server.URL, base: http.DefaultTransport}}
	return c
}

func TestNew_BuildsModelSpecificURL(t *testing.T) {
	c := New("gemini-2.5-pro", "key")
	want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent"
	if c.url != want {
		t.Errorf("url = %q, want %q", c.url, want)
	}
}

func TestName_ReturnsGemini(t *testing.T) {
	if got := (&Client{}).Name(); got != "Gemini" {
		t.Errorf("Name() = %q, want %q", got, "Gemini")
	}
}

func TestRedactedString_StringMasksValue(t *testing.T) {
	s := redactedString("super-secret-key")
	if s.String() != "[REDACTED]" {
		t.Errorf("String() = %q, want \"[REDACTED]\"", s.String())
	}
	if s.value() != "super-secret-key" {
		t.Errorf("value() = %q, want the raw underlying key", s.value())
	}
}

func TestReview_Success_ConcatenatesParts(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-api-key" {
			t.Errorf("x-goog-api-key header = %q, want %q", got, "test-api-key")
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"["},{"text":"]"}]}}]}`))
	})

	got, err := c.Review("review this")
	if err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("Review(...) = %q, want the concatenation of all parts, \"[]\"", got)
	}
}

func TestReview_RequestBodyStructure(t *testing.T) {
	var captured map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	})

	if _, err := c.Review("the prompt text"); err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	genConfig, _ := captured["generationConfig"].(map[string]any)
	if genConfig["responseMimeType"] != "application/json" {
		t.Errorf("generationConfig = %v, want responseMimeType = application/json", genConfig)
	}
	contents, _ := captured["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("contents = %v, want exactly one content entry", contents)
	}
}

func TestReview_NoCandidates_ReturnsEmptyString(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	})

	got, err := c.Review("prompt")
	if err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("Review(...) = %q, want empty string when no candidates are returned", got)
	}
}

func TestReview_CandidateWithNoParts(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[]}}]}`))
	})

	got, err := c.Review("prompt")
	if err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("Review(...) = %q, want empty string for a candidate with zero parts", got)
	}
}

func TestReview_HTTPErrorStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	})

	_, err := c.Review("prompt")
	if err == nil {
		t.Fatal("Review(...) succeeded unexpectedly against a 429 response; want an error")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("Review(...) error = %q, want the response body included", err.Error())
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("Review(...) error = %q, want the status code included", err.Error())
	}
}

func TestReview_MalformedJSONResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})

	if _, err := c.Review("prompt"); err == nil {
		t.Error("Review(...) succeeded unexpectedly on a malformed JSON response; want an error")
	}
}

func TestReview_EmptyResponseBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if _, err := c.Review("prompt"); err == nil {
		t.Error("Review(...) succeeded unexpectedly on an empty response body; want a JSON parse error")
	}
}

func TestReview_UnreachableHost(t *testing.T) {
	c := New("model", "key")
	c.http = &http.Client{Transport: redirectTransport{target: "http://127.0.0.1:1", base: http.DefaultTransport}}

	if _, err := c.Review("prompt"); err == nil {
		t.Error("Review(...) succeeded unexpectedly against an unreachable host; want an error")
	}
}

func TestReview_MultipleCandidates_OnlyFirstUsed(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"first"}]}},{"content":{"parts":[{"text":"second"}]}}]}`))
	})

	got, err := c.Review("prompt")
	if err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	if got != "first" {
		t.Errorf("Review(...) = %q, want only the first candidate's text", got)
	}
}
