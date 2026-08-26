package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ci-tools/internal/tokensource"
)

// failingProvider reports a credential resolution failure without contacting any endpoint.
type failingProvider struct{}

func (failingProvider) Token(context.Context) (string, error) {
	return "", errors.New("exchange rejected")
}

func newTestClient(t *testing.T, handler http.HandlerFunc, tokens tokensource.Provider) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return New(Config{
		Name:      "TestProvider",
		BaseURL:   server.URL,
		Model:     "test-model",
		MaxTokens: 512,
		Timeout:   5 * time.Second,
		Tokens:    tokens,
	})
}

func TestResolveEndpoint_StandardShape(t *testing.T) {
	got := resolveEndpoint(Config{BaseURL: "https://api.x.ai/", Model: "grok-test"})
	want := "https://api.x.ai/v1/chat/completions"
	if got != want {
		t.Errorf("resolveEndpoint() = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_AzureShape(t *testing.T) {
	got := resolveEndpoint(Config{
		BaseURL:    "https://example.openai.azure.com",
		Model:      "gpt-deployment",
		APIVersion: "2026-01-01",
	})
	want := "https://example.openai.azure.com/openai/deployments/gpt-deployment/chat/completions?api-version=2026-01-01"
	if got != want {
		t.Errorf("resolveEndpoint() = %q, want %q", got, want)
	}
}

func TestReview_SendsBearerTokenAndModel(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
	}, tokensource.Static("test-token"))

	got, err := c.Review("review this")
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if got != "[]" {
		t.Errorf("Review() = %q, want %q", got, "[]")
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotBody["model"] != "test-model" {
		t.Errorf("model = %v, want %q", gotBody["model"], "test-model")
	}
	if _, present := gotBody["response_format"]; present {
		t.Error("response_format must be omitted; json_object mode rejects a top-level array")
	}
}

func TestReview_OmitsMaxTokensWhenUnset(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
	}))
	t.Cleanup(server.Close)

	c := New(Config{
		Name:    "TestProvider",
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
		Tokens:  tokensource.Static("test-token"),
	})
	if _, err := c.Review("prompt"); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if _, present := gotBody["max_tokens"]; present {
		t.Error("max_tokens must be omitted when MaxTokens is zero")
	}
}

func TestReview_ReportsHTTPError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}, tokensource.Static("test-token"))

	_, err := c.Review("prompt")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("Review() error = %v, want an error mentioning 429", err)
	}
}

func TestReview_ReportsEmptyChoices(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}, tokensource.Static("test-token"))

	_, err := c.Review("prompt")
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("Review() error = %v, want an error mentioning no choices", err)
	}
}

func TestReview_ReportsCredentialFailureBeforeRequest(t *testing.T) {
	var reached bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}, failingProvider{})

	_, err := c.Review("prompt")
	if err == nil || !strings.Contains(err.Error(), "resolve credential") {
		t.Errorf("Review() error = %v, want a credential resolution error", err)
	}
	if reached {
		t.Error("endpoint must not be contacted when the credential cannot be resolved")
	}
}

func TestName_ReturnsConfiguredName(t *testing.T) {
	if got := New(Config{Name: "Grok", Tokens: tokensource.Static("test-token")}).Name(); got != "Grok" {
		t.Errorf("Name() = %q, want %q", got, "Grok")
	}
}

func TestNew_PanicsWhenTokensNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New() did not panic with a nil Tokens provider")
		}
	}()
	New(Config{Name: "Grok"})
}
