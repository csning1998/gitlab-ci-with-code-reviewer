package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ci-tools/internal/config"
	"ci-tools/internal/testutil"
	"ci-tools/internal/tokensource"
)

// redirectTransport rewrites every outbound request to target a local httptest server.
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

func mustNewClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New(...) returned an unexpected error: %v", err)
	}
	return c
}

// stubCapturingClient records the decoded generateContent request body.
func stubCapturingClient(t *testing.T, opts config.ModelOptions, gotBody *map[string]any) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, gotBody)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"[]"}]}}]}`))
	}))
	t.Cleanup(server.Close)

	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	c := mustNewClient(t, Config{ModelOptions: opts, Tokens: tokensource.Static("test-api-key")})
	c.http = &http.Client{Transport: redirectTransport{target: server.URL, base: http.DefaultTransport}}
	return c
}

// stubRespondingClient serves a predetermined status code and payload response.
func stubRespondingClient(t *testing.T, statusCode int, responseBody string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)

	c := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Model: "gemini-test-model", Timeout: 5 * time.Second},
		Tokens:       tokensource.Static("test-api-key"),
	})
	c.http = &http.Client{Transport: redirectTransport{target: server.URL, base: http.DefaultTransport}}
	return c
}

func TestName_ReturnsGemini(t *testing.T) {
	if got := (&Client{}).Name(); got != "Gemini" {
		t.Errorf("Name() = %q, want %q", got, "Gemini")
	}
}

func TestNew_ConfigurationValidation(t *testing.T) {
	t.Run("valid model URL generation", func(t *testing.T) {
		c, err := New(Config{
			ModelOptions: config.ModelOptions{Model: "gemini-2.5-pro"},
			Tokens:       tokensource.Static("key"),
		})
		if err != nil {
			t.Fatalf("New(...) returned unexpected error: %v", err)
		}
		wantURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent"
		if c.url != wantURL {
			t.Errorf("url = %q, want %q", c.url, wantURL)
		}
	})

	t.Run("rejects invalid configuration", func(t *testing.T) {
		tests := []struct {
			name     string
			cfg      Config
			wantWord string
		}{
			{
				name:     "rejects nil tokens",
				cfg:      Config{ModelOptions: config.ModelOptions{Model: "gemini-2.5-pro"}},
				wantWord: "tokens",
			},
			{
				name:     "rejects empty model",
				cfg:      Config{Tokens: tokensource.Static("key")},
				wantWord: "model",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := New(tc.cfg)
				if err == nil || !strings.Contains(err.Error(), tc.wantWord) {
					t.Fatalf("New(...) error = %v, want error containing %q", err, tc.wantWord)
				}
			})
		}
	})
}

func TestNew_TimeoutResolution(t *testing.T) {
	cCustom := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Model: "gemini-2.5-pro", Timeout: 42 * time.Second},
		Tokens:       tokensource.Static("key"),
	})
	if cCustom.http.Timeout != 42*time.Second {
		t.Errorf("http.Timeout = %v, want 42s", cCustom.http.Timeout)
	}

	cDefault := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Model: "gemini-2.5-pro"},
		Tokens:       tokensource.Static("key"),
	})
	if cDefault.http.Timeout != config.DefaultTimeout {
		t.Errorf("http.Timeout = %v, want %v", cDefault.http.Timeout, config.DefaultTimeout)
	}
}

func TestReview_ReportsCredentialFailureBeforeRequest(t *testing.T) {
	var reached bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	t.Cleanup(server.Close)

	c := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Model: "gemini-2.5-pro"},
		Tokens:       testutil.FailingTokenProvider{},
	})
	c.http = &http.Client{Transport: redirectTransport{target: server.URL, base: http.DefaultTransport}}

	_, err := c.Review("prompt")
	if err == nil || !strings.Contains(err.Error(), "resolve credential") {
		t.Errorf("Review(...) error = %v, want credential resolution error", err)
	}
	if reached {
		t.Error("endpoint MUST NOT be contacted when credential resolution fails")
	}
}

func TestReview_ResponseHandling_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantResult string
		wantError  string
	}{
		{
			name:       "concatenates multiple text parts",
			status:     http.StatusOK,
			body:       `{"candidates":[{"content":{"parts":[{"text":"["},{"text":"]"}]}}]}`,
			wantResult: "[]",
		},
		{
			name:       "empty candidates list yields empty string",
			status:     http.StatusOK,
			body:       `{"candidates":[]}`,
			wantResult: "",
		},
		{
			name:       "candidate with zero parts yields empty string",
			status:     http.StatusOK,
			body:       `{"candidates":[{"content":{"parts":[]}}]}`,
			wantResult: "",
		},
		{
			name:       "multiple candidates picks first choice only",
			status:     http.StatusOK,
			body:       `{"candidates":[{"content":{"parts":[{"text":"first"}]}},{"content":{"parts":[{"text":"second"}]}}]}`,
			wantResult: "first",
		},
		{
			name:      "upstream HTTP 429 reports error",
			status:    http.StatusTooManyRequests,
			body:      `{"error":{"message":"rate limited"}}`,
			wantError: "rate limited",
		},
		{
			name:      "malformed JSON reports error",
			status:    http.StatusOK,
			body:      `not json`,
			wantError: "invalid character",
		},
		{
			name:      "empty response body reports error",
			status:    http.StatusOK,
			body:      "",
			wantError: "unexpected end of JSON input",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := stubRespondingClient(t, tc.status, tc.body)
			got, err := c.Review("prompt")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("Review(...) error = %v, want error containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Review(...) returned unexpected error: %v", err)
			}
			if got != tc.wantResult {
				t.Errorf("Review(...) = %q, want %q", got, tc.wantResult)
			}
		})
	}
}

func TestReview_UnreachableHost(t *testing.T) {
	c := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Model: "model", Timeout: 5 * time.Second},
		Tokens:       tokensource.Static("key"),
	})
	c.http = &http.Client{Transport: redirectTransport{target: "http://127.0.0.1:1", base: http.DefaultTransport}}

	if _, err := c.Review("prompt"); err == nil {
		t.Error("Review(...) MUST report an error when endpoint is unreachable")
	}
}

func TestReview_InjectsGenerationConfig(t *testing.T) {
	var gotBody map[string]any
	c := stubCapturingClient(t, config.ModelOptions{
		Provider:         "gemini",
		Model:            "gemini-2.5-flash",
		MaxTokens:        2048,
		Temperature:      testutil.Ptr(0.4),
		TopP:             testutil.Ptr(0.9),
		TopK:             testutil.Ptr(64),
		Stop:             []string{"END"},
		Seed:             testutil.Ptr(int64(42)),
		N:                testutil.Ptr(1),
		FrequencyPenalty: testutil.Ptr(0.5),
		PresencePenalty:  testutil.Ptr(0.25),
	}, &gotBody)

	if _, err := c.Review("prompt"); err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	genConfig, _ := gotBody["generationConfig"].(map[string]any)
	wants := map[string]any{
		"responseMimeType": "application/json",
		"temperature":      0.4,
		"topP":             0.9,
		"topK":             float64(64),
		"maxOutputTokens":  float64(2048),
		"seed":             float64(42),
		"candidateCount":   float64(1),
		"frequencyPenalty": 0.5,
		"presencePenalty":  0.25,
	}
	for field, want := range wants {
		if genConfig[field] != want {
			t.Errorf("generationConfig[%q] = %v, want %v", field, genConfig[field], want)
		}
	}
	stop, _ := genConfig["stopSequences"].([]any)
	if len(stop) != 1 || stop[0] != "END" {
		t.Errorf("stopSequences = %v, want [END]", genConfig["stopSequences"])
	}
}

func TestReview_OmitsGenerationFieldsWhenUnset(t *testing.T) {
	var gotBody map[string]any
	c := stubCapturingClient(t, config.ModelOptions{Model: "gemini-2.5-flash"}, &gotBody)

	if _, err := c.Review("prompt"); err != nil {
		t.Fatalf("Review(...) returned an unexpected error: %v", err)
	}
	genConfig, _ := gotBody["generationConfig"].(map[string]any)
	for _, field := range []string{
		"temperature", "topP", "topK", "maxOutputTokens", "stopSequences",
		"seed", "candidateCount", "frequencyPenalty", "presencePenalty", "thinkingConfig",
	} {
		if _, present := genConfig[field]; present {
			t.Errorf("generationConfig[%q] must be omitted when unset", field)
		}
	}
}

func TestReview_ThinkingGeneration_TableDriven(t *testing.T) {
	t.Run("rejects invalid thinking configuration", func(t *testing.T) {
		tests := []struct {
			name      string
			opts      config.ModelOptions
			wantError string
		}{
			{
				name: "gemini 3.6 rejects thinkingBudget",
				opts: config.ModelOptions{
					Model:          "gemini-3.6-flash",
					ThinkingBudget: testutil.Ptr(4096),
				},
				wantError: "thinking_budget",
			},
			{
				name: "gemini 2.5 rejects reasoningLevel",
				opts: config.ModelOptions{
					Model:          "gemini-2.5-pro",
					ReasoningLevel: "high",
				},
				wantError: "reasoning_level",
			},
			{
				name: "gemma 4 rejects medium level",
				opts: config.ModelOptions{
					Model:          "gemma-4-31b-it",
					ReasoningLevel: "medium",
				},
				wantError: "reasoning_level",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := New(Config{ModelOptions: tc.opts, Tokens: tokensource.Static("key")})
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("New(...) error = %v, want error containing %q", err, tc.wantError)
				}
			})
		}
	})

	t.Run("generates valid thinking payload", func(t *testing.T) {
		tests := []struct {
			name      string
			opts      config.ModelOptions
			wantKey   string
			wantValue any
			unwantKey string
		}{
			{
				name: "gemini 2.5 sends thinkingBudget",
				opts: config.ModelOptions{
					Model:          "gemini-2.5-pro",
					ThinkingBudget: testutil.Ptr(4096),
				},
				wantKey:   "thinkingBudget",
				wantValue: float64(4096),
				unwantKey: "thinkingLevel",
			},
			{
				name: "gemini 3.6 sends thinkingLevel",
				opts: config.ModelOptions{
					Model:          "gemini-3.6-flash",
					ReasoningLevel: "high",
				},
				wantKey:   "thinkingLevel",
				wantValue: "high",
				unwantKey: "thinkingBudget",
			},
			{
				name: "gemma 4 accepts high level",
				opts: config.ModelOptions{
					Model:          "gemma-4-31b-it",
					ReasoningLevel: "high",
				},
				wantKey:   "thinkingLevel",
				wantValue: "high",
				unwantKey: "thinkingBudget",
			},
			{
				name: "gemma 4 accepts minimal level",
				opts: config.ModelOptions{
					Model:          "gemma-4-31b-it",
					ReasoningLevel: "minimal",
				},
				wantKey:   "thinkingLevel",
				wantValue: "minimal",
				unwantKey: "thinkingBudget",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				var gotBody map[string]any
				c := stubCapturingClient(t, tc.opts, &gotBody)
				if _, err := c.Review("prompt"); err != nil {
					t.Fatalf("Review(...) returned unexpected error: %v", err)
				}
				assertThinkingConfigField(t, gotBody, tc.wantKey, tc.wantValue, tc.unwantKey)
			})
		}
	})
}

func assertThinkingConfigField(t *testing.T, body map[string]any, wantKey string, wantValue any, unwantKey string) {
	t.Helper()
	genConfig, _ := body["generationConfig"].(map[string]any)
	thinking, _ := genConfig["thinkingConfig"].(map[string]any)
	if thinking[wantKey] != wantValue {
		t.Errorf("thinkingConfig[%q] = %v, want %v", wantKey, thinking[wantKey], wantValue)
	}
	if unwantKey != "" {
		if _, present := thinking[unwantKey]; present {
			t.Errorf("thinkingConfig[%q] MUST NOT be present", unwantKey)
		}
	}
}
