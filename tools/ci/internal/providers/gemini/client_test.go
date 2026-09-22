package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

type tokenFunc func(context.Context) (string, error)

func (f tokenFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

// hostRewriteTransport redirects only the production API host to the fixture server. Every
// other host, such as a redirect target, is contacted directly.
type hostRewriteTransport struct {
	target string
}

func (rt hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "generativelanguage.googleapis.com" {
		return http.DefaultTransport.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	targetURL, err := http.NewRequest(req.Method, rt.target, req.Body)
	if err != nil {
		return nil, err
	}
	clone.URL.Scheme = targetURL.URL.Scheme
	clone.URL.Host = targetURL.URL.Host
	clone.Host = targetURL.URL.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func newBoundaryClient(t *testing.T, timeout time.Duration, tokens tokensource.Provider, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", Timeout: timeout},
		Tokens:       tokens,
	})
	client.http.Transport = hostRewriteTransport{target: server.URL}
	return client
}

func echoPromptHandler(seen *sync.Map) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get("x-goog-api-key"), struct{}{})
		var payload struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		text := ""
		if len(payload.Contents) > 0 && len(payload.Contents[0].Parts) > 0 {
			text = payload.Contents[0].Parts[0].Text
		}
		reply, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": text}}}}}})
		_, _ = w.Write(reply)
	}
}

func TestReview_ConcurrentCallsKeepPromptsAndCredentialsSeparate(t *testing.T) {
	const callers = 48

	var issued atomic.Int64
	tokens := tokenFunc(func(context.Context) (string, error) {
		return fmt.Sprintf("key-%d", issued.Add(1)), nil
	})
	var seen sync.Map
	client := newBoundaryClient(t, 10*time.Second, tokens, echoPromptHandler(&seen))

	start := make(chan struct{})
	results := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = client.Review(fmt.Sprintf("prompt-%d", i))
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Errorf("caller %d returned an unexpected error: %v", i, errs[i])
			continue
		}
		if want := fmt.Sprintf("prompt-%d", i); results[i] != want {
			t.Errorf("caller %d received %q, want %q", i, results[i], want)
		}
	}

	distinct := 0
	seen.Range(func(_, _ any) bool { distinct++; return true })
	if distinct != callers {
		t.Errorf("server observed %d distinct credentials, want %d", distinct, callers)
	}
}

func TestReview_TimeoutBoundsSlowServer(t *testing.T) {
	release := make(chan struct{})
	client := newBoundaryClient(t, 150*time.Millisecond, tokensource.Static("key"), func(http.ResponseWriter, *http.Request) {
		<-release
	})
	t.Cleanup(func() { close(release) })

	start := time.Now()
	if _, err := client.Review("prompt"); err == nil {
		t.Fatal("Review succeeded unexpectedly against a stalled server")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Review took %v, want a bound near the 150ms timeout", elapsed)
	}
}

func TestReview_MalformedCredentialNeverReachesServer(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "whitespace only", token: "   "},
		{name: "carriage return line feed injection", token: "abc\r\nX-Injected: 1"},
		{name: "nul byte", token: "abc\x00def"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reached atomic.Bool
			client := newBoundaryClient(t, 5*time.Second,
				tokenFunc(func(context.Context) (string, error) { return tc.token, nil }),
				func(w http.ResponseWriter, r *http.Request) {
					reached.Store(true)
					if r.Header.Get("X-Injected") != "" {
						t.Error("server observed an injected header")
					}
					_, _ = w.Write([]byte(`{"candidates":[]}`))
				})

			_, err := client.Review("prompt")
			if err == nil && reached.Load() {
				t.Errorf("Review sent a request with credential %q", tc.token)
			}
		})
	}
}

// The API key travels in a custom header, which the standard library forwards on a redirect to
// another host. The client MUST NOT forward the key to that host.
func TestReview_CrossHostRedirectDoesNotForwardCredential(t *testing.T) {
	var leaked atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("x-goog-api-key"))
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	t.Cleanup(target.Close)
	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)

	client := newBoundaryClient(t, 5*time.Second, tokensource.Static("secret-key"), func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL+r.URL.Path, http.StatusTemporaryRedirect)
	})

	_, _ = client.Review("prompt")

	if got, _ := leaked.Load().(string); got != "" {
		t.Errorf("redirect target received x-goog-api-key %q", got)
	}
}

func TestReview_OversizedErrorBodyIsBounded(t *testing.T) {
	client := newBoundaryClient(t, 10*time.Second, tokensource.Static("key"), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 5<<20)))
	})

	_, err := client.Review("prompt")
	if err == nil {
		t.Fatal("Review succeeded unexpectedly")
	}
	if len(err.Error()) > 8192 {
		t.Errorf("error message holds %d bytes, want a bounded excerpt", len(err.Error()))
	}
}

func TestReview_ResponseBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "null candidates", body: `{"candidates":null}`},
		{name: "candidate without content", body: `{"candidates":[{}]}`},
		{name: "null part text", body: `{"candidates":[{"content":{"parts":[{"text":null}]}}]}`},
		{name: "part without text", body: `{"candidates":[{"content":{"parts":[{"inlineData":{}}]}}]}`},
		{name: "text as number", body: `{"candidates":[{"content":{"parts":[{"text":5}]}}]}`, wantErr: true},
		{name: "parts as object", body: `{"candidates":[{"content":{"parts":{}}}]}`, wantErr: true},
		{name: "unicode across parts", body: `{"candidates":[{"content":{"parts":[{"text":"設"},{"text":"定"}]}}]}`, want: "設定"},
		{name: "thought parts are excluded", body: `{"candidates":[{"content":{"parts":[{"text":"a","thought":true},{"text":"b"}]}}]}`, want: "b"},
		{name: "trailing garbage", body: `{"candidates":[]} tail`, wantErr: true},
		{name: "only whitespace", body: "  \n", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newBoundaryClient(t, 5*time.Second, tokensource.Static("key"), func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})

			got, err := client.Review("prompt")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Review succeeded unexpectedly with %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Review returned an unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("Review = %q, want %q", got, tc.want)
			}
		})
	}
}
