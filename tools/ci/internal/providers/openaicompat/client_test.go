package openaicompat

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
	"unicode/utf8"

	"ci-tools/internal/config"
	"ci-tools/internal/testutil"
	"ci-tools/internal/tokensource"
)

func mustNewClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New(...) returned an unexpected error: %v", err)
	}
	return c
}

// stubCapturingClient serves a fixed completion response and records the decoded request body.
func stubCapturingClient(t *testing.T, opts config.ModelOptions, gotBody *map[string]any) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
	}))
	t.Cleanup(server.Close)

	opts.BaseURL = server.URL
	return mustNewClient(t, Config{ModelOptions: opts, Tokens: tokensource.Static("test-token")})
}

// stubRespondingClient serves a predetermined status code and payload response.
func stubRespondingClient(t *testing.T, statusCode int, responseBody string, tokens tokensource.Provider) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)

	return mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{
			Provider:  "openai",
			Model:     "test-model",
			MaxTokens: 512,
			Timeout:   5 * time.Second,
			BaseURL:   server.URL,
		},
		Tokens: tokens,
	})
}

func TestResolveEndpoint_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		opts config.ModelOptions
		want string
	}{
		{
			name: "standard base URL without trailing slash",
			opts: config.ModelOptions{BaseURL: "https://api.x.ai", Model: "grok-4.6"},
			want: "https://api.x.ai/v1/chat/completions",
		},
		{
			name: "standard base URL with trailing slash",
			opts: config.ModelOptions{BaseURL: "https://api.x.ai/", Model: "grok-4.6"},
			want: "https://api.x.ai/v1/chat/completions",
		},
		{
			name: "standard base URL ending with /v1",
			opts: config.ModelOptions{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"},
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "standard base URL ending with /v1/",
			opts: config.ModelOptions{BaseURL: "https://api.openai.com/v1/", Model: "gpt-4o"},
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "local base URL without trailing slash",
			opts: config.ModelOptions{BaseURL: "http://localhost:8000", Model: "llama-3.3-70b"},
			want: "http://localhost:8000/v1/chat/completions",
		},
		{
			name: "local base URL with trailing slash",
			opts: config.ModelOptions{BaseURL: "http://localhost:8000/", Model: "llama-3.3-70b"},
			want: "http://localhost:8000/v1/chat/completions",
		},
		{
			name: "local base URL ending with /v1",
			opts: config.ModelOptions{BaseURL: "http://localhost:8000/v1", Model: "llama-3.3-70b"},
			want: "http://localhost:8000/v1/chat/completions",
		},
		{
			name: "local base URL ending with /v1/",
			opts: config.ModelOptions{BaseURL: "http://localhost:8000/v1/", Model: "llama-3.3-70b"},
			want: "http://localhost:8000/v1/chat/completions",
		},
		{
			name: "azure shape without trailing slash",
			opts: config.ModelOptions{
				BaseURL:    "https://example.openai.azure.com",
				Model:      "gpt-deployment",
				APIVersion: "2026-01-01",
			},
			want: "https://example.openai.azure.com/openai/deployments/gpt-deployment/chat/completions?api-version=2026-01-01",
		},
		{
			name: "azure shape with trailing slash",
			opts: config.ModelOptions{
				BaseURL:    "https://example.openai.azure.com/",
				Model:      "gpt-deployment",
				APIVersion: "2026-01-01",
			},
			want: "https://example.openai.azure.com/openai/deployments/gpt-deployment/chat/completions?api-version=2026-01-01",
		},
		{
			name: "subpath base URL without trailing slash",
			opts: config.ModelOptions{BaseURL: "https://gateway.internal:8443/custom/prefix", Model: "gpt-4o"},
			want: "https://gateway.internal:8443/custom/prefix/v1/chat/completions",
		},
		{
			name: "subpath base URL with trailing slash",
			opts: config.ModelOptions{BaseURL: "https://gateway.internal:8443/custom/prefix/", Model: "gpt-4o"},
			want: "https://gateway.internal:8443/custom/prefix/v1/chat/completions",
		},
		{
			name: "subpath base URL ending with /v1",
			opts: config.ModelOptions{BaseURL: "https://gateway.internal:8443/custom/prefix/v1", Model: "gpt-4o"},
			want: "https://gateway.internal:8443/custom/prefix/v1/chat/completions",
		},
		{
			name: "subpath base URL ending with /v1/",
			opts: config.ModelOptions{BaseURL: "https://gateway.internal:8443/custom/prefix/v1/", Model: "gpt-4o"},
			want: "https://gateway.internal:8443/custom/prefix/v1/chat/completions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveEndpoint(tc.opts); got != tc.want {
				t.Errorf("resolveEndpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestName_DerivesFromProvider(t *testing.T) {
	tests := []struct {
		provider   string
		baseURL    string
		apiVersion string
		want       string
	}{
		{provider: "openai", want: "OpenAI"},
		{provider: "azure-openai", baseURL: "https://example.openai.azure.com", apiVersion: "2026-01-01", want: "Azure OpenAI"},
		{provider: "grok", want: "Grok"},
		{provider: "local", baseURL: "http://localhost:8000", want: "Local"},
		{provider: "custom-gateway", baseURL: "https://llm.internal.net", want: "custom-gateway"},
		{provider: "  deepseek  ", baseURL: "https://api.deepseek.com", want: "deepseek"},
		{provider: "", baseURL: "https://api.openai.com", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			c := mustNewClient(t, Config{
				ModelOptions: config.ModelOptions{
					Provider:   tc.provider,
					Model:      "test-model",
					BaseURL:    tc.baseURL,
					APIVersion: tc.apiVersion,
				},
				Tokens: tokensource.Static("test-token"),
			})
			if got := c.Name(); got != tc.want {
				t.Errorf("Name() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNew_ConfigurationRejection(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantError string
	}{
		{
			name:      "rejects nil tokens",
			cfg:       Config{ModelOptions: config.ModelOptions{Provider: "grok", Model: "grok-4.6"}},
			wantError: "tokens",
		},
		{
			name: "rejects empty model",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "grok"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "model",
		},
		{
			name: "rejects azure-openai with empty base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "azure-openai", Model: "gpt-deployment", APIVersion: "2024-02-15-preview"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects azure-openai with whitespace base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "azure-openai", Model: "gpt-deployment", BaseURL: "   ", APIVersion: "2024-02-15-preview"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects azure-openai with empty api_version",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "azure-openai", Model: "gpt-deployment", BaseURL: "https://example.openai.azure.com"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "api_version",
		},
		{
			name: "rejects azure-openai with whitespace api_version",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "azure-openai", Model: "gpt-deployment", BaseURL: "https://example.openai.azure.com", APIVersion: "   "},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "api_version",
		},
		{
			name: "rejects local with empty base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "local", Model: "gemma4"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects local with whitespace base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "local", Model: "gemma4", BaseURL: "   "},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects unknown provider with empty base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "custom-gateway", Model: "custom-model"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects unknown provider with whitespace base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "custom-gateway", Model: "custom-model", BaseURL: "   "},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects empty provider with empty base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "", Model: "custom-model"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
		{
			name: "rejects empty provider with whitespace base_url",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "", Model: "custom-model", BaseURL: "   "},
				Tokens:       tokensource.Static("test-token"),
			},
			wantError: "base_url",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("New(...) error = %v, want error containing %q", err, tc.wantError)
			}
		})
	}
}

func TestNew_ConfigurationDefaults(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantURL string
	}{
		{
			name: "defaults timeout when unset",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "openai", Model: "gpt-4o"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantURL: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "defaults openai base_url to official endpoint",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "openai", Model: "gpt-4o"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantURL: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "defaults openai whitespace base_url to official endpoint",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "openai", Model: "gpt-4o", BaseURL: "   "},
				Tokens:       tokensource.Static("test-token"),
			},
			wantURL: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "defaults grok base_url to official endpoint",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "grok", Model: "grok-4"},
				Tokens:       tokensource.Static("test-token"),
			},
			wantURL: "https://api.x.ai/v1/chat/completions",
		},
		{
			name: "defaults grok whitespace base_url to official endpoint",
			cfg: Config{
				ModelOptions: config.ModelOptions{Provider: "grok", Model: "grok-4", BaseURL: "   "},
				Tokens:       tokensource.Static("test-token"),
			},
			wantURL: "https://api.x.ai/v1/chat/completions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(tc.cfg)
			if err != nil {
				t.Fatalf("New(...) returned unexpected error: %v", err)
			}
			if c.modelOptions.Timeout != config.DefaultTimeout {
				t.Errorf("Timeout = %v, want default %v", c.modelOptions.Timeout, config.DefaultTimeout)
			}
			if c.url != tc.wantURL {
				t.Errorf("url = %q, want %q", c.url, tc.wantURL)
			}
		})
	}
}

func TestReview_ReportsCredentialFailureBeforeRequest(t *testing.T) {
	var reached bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	t.Cleanup(server.Close)

	c := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Provider: "openai", Model: "gpt-4o", BaseURL: server.URL},
		Tokens:       testutil.FailingTokenProvider{},
	})

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
			name:       "valid single choice response",
			status:     http.StatusOK,
			body:       `{"choices":[{"message":{"content":"[]"}}]}`,
			wantResult: "[]",
		},
		{
			name:      "upstream HTTP 429 reports error",
			status:    http.StatusTooManyRequests,
			body:      `{"error":"rate limited"}`,
			wantError: "429",
		},
		{
			name:      "empty choices array reports error",
			status:    http.StatusOK,
			body:      `{"choices":[]}`,
			wantError: "no choices",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := stubRespondingClient(t, tc.status, tc.body, tokensource.Static("test-token"))
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

func TestReview_InjectsOptionalParametersWhenConfigured(t *testing.T) {
	var gotBody map[string]any
	c := stubCapturingClient(t, config.ModelOptions{
		Provider:         "openai",
		Model:            "gpt-4o",
		Temperature:      testutil.Ptr(0.7),
		TopP:             testutil.Ptr(0.95),
		Seed:             testutil.Ptr(int64(42)),
		FrequencyPenalty: testutil.Ptr(0.5),
		PresencePenalty:  testutil.Ptr(0.5),
		Stop:             []string{"\n\n"},
		MaxTokens:        1024,
	}, &gotBody)

	if _, err := c.Review("prompt"); err != nil {
		t.Fatalf("Review() error = %v", err)
	}

	if gotBody["model"] != "gpt-4o" {
		t.Errorf("model = %v, want gpt-4o", gotBody["model"])
	}
	if gotBody["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", gotBody["temperature"])
	}
	if gotBody["top_p"] != 0.95 {
		t.Errorf("top_p = %v, want 0.95", gotBody["top_p"])
	}
	if gotBody["seed"] != float64(42) {
		t.Errorf("seed = %v, want 42", gotBody["seed"])
	}
	if gotBody["frequency_penalty"] != 0.5 {
		t.Errorf("frequency_penalty = %v, want 0.5", gotBody["frequency_penalty"])
	}
	if gotBody["presence_penalty"] != 0.5 {
		t.Errorf("presence_penalty = %v, want 0.5", gotBody["presence_penalty"])
	}
	if gotBody["max_tokens"] != float64(1024) {
		t.Errorf("max_tokens = %v, want 1024", gotBody["max_tokens"])
	}
	if _, present := gotBody["max_completion_tokens"]; present {
		t.Error("max_completion_tokens must not be present for gpt-4o")
	}
	if _, present := gotBody["response_format"]; present {
		t.Error("response_format must be omitted; json_object mode rejects a top-level array")
	}
}

func TestReview_OmitsOptionalParametersWhenNil(t *testing.T) {
	var gotBody map[string]any
	c := stubCapturingClient(t, config.ModelOptions{Provider: "openai", Model: "gpt-4o"}, &gotBody)

	if _, err := c.Review("prompt"); err != nil {
		t.Fatalf("Review() error = %v", err)
	}

	for _, field := range []string{
		"temperature", "top_p", "seed", "frequency_penalty", "presence_penalty",
		"stop", "reasoning_effort", "max_tokens", "max_completion_tokens",
		"repetition_penalty", "min_p",
	} {
		if _, present := gotBody[field]; present {
			t.Errorf("field %q must be omitted when unset", field)
		}
	}
}

func TestReview_ReasoningModel_FiltersConflictingParameters(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		reasoningLevel string
	}{
		{name: "OpenAI o3-mini", model: "o3-mini", reasoningLevel: "high"},
		{name: "OpenAI o1", model: "o1", reasoningLevel: "medium"},
		{name: "xAI grok-4.5", model: "grok-4.5", reasoningLevel: ""},
		{name: "xAI grok-4.6", model: "grok-4.6", reasoningLevel: "xhigh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			c := stubCapturingClient(t, config.ModelOptions{
				Model:            tc.model,
				ReasoningLevel:   tc.reasoningLevel,
				FrequencyPenalty: testutil.Ptr(0.5),
				PresencePenalty:  testutil.Ptr(0.5),
				Stop:             []string{"END"},
				MaxTokens:        2048,
			}, &gotBody)

			if _, err := c.Review("prompt"); err != nil {
				t.Fatalf("Review() error = %v", err)
			}

			assertOmittedKeys(t, gotBody, "frequency_penalty", "presence_penalty", "stop")
			if tc.reasoningLevel != "" {
				assertBodyField(t, gotBody, "reasoning_effort", tc.reasoningLevel)
			}
		})
	}
}

func TestReview_OutputTokens_RoutesCorrectField(t *testing.T) {
	tests := []struct {
		model     string
		wantField string
	}{
		{model: "o1", wantField: "max_completion_tokens"},
		{model: "o1-mini", wantField: "max_completion_tokens"},
		{model: "o3-mini", wantField: "max_completion_tokens"},
		{model: "o4-mini", wantField: "max_completion_tokens"},
		{model: "gpt-5", wantField: "max_completion_tokens"},
		{model: "gpt-5.4-mini", wantField: "max_completion_tokens"},
		{model: "gpt-5.6-sol", wantField: "max_completion_tokens"},
		{model: "gpt-4o", wantField: "max_tokens"},
		{model: "gpt-4.1", wantField: "max_tokens"},
		{model: "grok-4", wantField: "max_tokens"},
		{model: "local-vllm", wantField: "max_tokens"},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			var gotBody map[string]any
			c := stubCapturingClient(t, config.ModelOptions{Model: tc.model, MaxTokens: 1500}, &gotBody)

			if _, err := c.Review("prompt"); err != nil {
				t.Fatalf("Review() error = %v", err)
			}

			if gotBody[tc.wantField] != float64(1500) {
				t.Errorf("model %q: expected field %q to be 1500, got %v", tc.model, tc.wantField, gotBody[tc.wantField])
			}
			unwanted := "max_tokens"
			if tc.wantField == "max_tokens" {
				unwanted = "max_completion_tokens"
			}
			if _, present := gotBody[unwanted]; present {
				t.Errorf("model %q: field %q must not be present", tc.model, unwanted)
			}
		})
	}
}

func TestReview_OpenSourceInferenceParameters(t *testing.T) {
	var gotBody map[string]any
	c := stubCapturingClient(t, config.ModelOptions{
		Provider:          "local",
		Model:             "meta-llama-3",
		RepetitionPenalty: testutil.Ptr(1.15),
		MinP:              testutil.Ptr(0.05),
	}, &gotBody)

	if _, err := c.Review("prompt"); err != nil {
		t.Fatalf("Review() error = %v", err)
	}

	if gotBody["repetition_penalty"] != 1.15 {
		t.Errorf("repetition_penalty = %v, want 1.15", gotBody["repetition_penalty"])
	}
	if gotBody["min_p"] != 0.05 {
		t.Errorf("min_p = %v, want 0.05", gotBody["min_p"])
	}
}

func assertOmittedKeys(t *testing.T, body map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, present := body[key]; present {
			t.Errorf("field %q MUST be omitted", key)
		}
	}
}

func assertBodyField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	if got := body[key]; got != want {
		t.Errorf("body[%q] = %v, want %v", key, got, want)
	}
}

type tokenFunc func(context.Context) (string, error)

func (f tokenFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

func (f tokenFunc) FetchCredential(ctx context.Context) (tokensource.Credential, error) {
	s, err := f(ctx)
	if err != nil {
		return tokensource.Credential{}, err
	}
	return tokensource.Credential{Value: s, Kind: tokensource.KindAPIKey}, nil
}

func newBoundaryClient(t *testing.T, timeout time.Duration, tokens tokensource.Provider, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{
			Provider:  "openai",
			Model:     "gpt-4o",
			MaxTokens: 64,
			Timeout:   timeout,
			BaseURL:   server.URL,
		},
		Tokens: tokens,
	})
}

func echoPromptHandler(seen *sync.Map) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get("Authorization"), struct{}{})
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		content := ""
		if len(payload.Messages) > 0 {
			content = payload.Messages[0].Content
		}
		reply, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
		_, _ = w.Write(reply)
	}
}

// One Client serves many goroutines. Each response MUST match the prompt of its own call, and
// each call MUST send the credential issued for that call.
func TestReview_ConcurrentCallsKeepPromptsAndCredentialsSeparate(t *testing.T) {
	const callers = 48

	var issued atomic.Int64
	tokens := tokenFunc(func(context.Context) (string, error) {
		return fmt.Sprintf("token-%d", issued.Add(1)), nil
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
	client := newBoundaryClient(t, 150*time.Millisecond, tokensource.Static("token"), func(http.ResponseWriter, *http.Request) {
		<-release
	})
	t.Cleanup(func() { close(release) })

	start := time.Now()
	_, err := client.Review("prompt")
	if err == nil {
		t.Fatal("Review succeeded unexpectedly against a stalled server")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Review took %v, want a bound near the 150ms timeout", elapsed)
	}
}

func TestReview_CredentialResolutionHonoursDeadline(t *testing.T) {
	tokens := tokenFunc(func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	client := newBoundaryClient(t, 150*time.Millisecond, tokens, func(http.ResponseWriter, *http.Request) {
		t.Error("request reached the server without a credential")
	})

	start := time.Now()
	if _, err := client.Review("prompt"); err == nil {
		t.Fatal("Review succeeded unexpectedly without a credential")
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
		{name: "line feed injection", token: "abc\nX-Injected: 1"},
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
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
				})

			_, err := client.Review("prompt")
			if err == nil && reached.Load() {
				t.Errorf("Review sent a request with credential %q", tc.token)
			}
		})
	}
}

func TestReview_ResponseBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "null content", body: `{"choices":[{"message":{"content":null}}]}`},
		{name: "missing message", body: `{"choices":[{}]}`},
		{name: "empty object", body: `{}`, wantErr: true},
		{name: "null choices", body: `{"choices":null}`, wantErr: true},
		{name: "choices as object", body: `{"choices":{}}`, wantErr: true},
		{name: "content as number", body: `{"choices":[{"message":{"content":5}}]}`, wantErr: true},
		{name: "content as parts array", body: `{"choices":[{"message":{"content":[{"type":"text","text":"x"}]}}]}`, wantErr: true},
		{name: "unicode content", body: `{"choices":[{"message":{"content":"設定 😀"}}]}`, want: "設定 \U0001F600"},
		{name: "escaped nul", body: `{"choices":[{"message":{"content":"a\u0000b"}}]}`, want: "a\x00b"},
		{name: "trailing garbage", body: `{"choices":[{"message":{"content":"x"}}]} tail`, wantErr: true},
		{name: "byte order mark prefix", body: "\xef\xbb\xbf{\"choices\":[{\"message\":{\"content\":\"x\"}}]}", wantErr: true},
		{name: "only whitespace", body: "  \n", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newBoundaryClient(t, 5*time.Second, tokensource.Static("token"), func(w http.ResponseWriter, _ *http.Request) {
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

func TestReview_ErrorBodyExcerptStaysValidUTF8(t *testing.T) {
	// The excerpt limit is 512 bytes. A two byte rune straddling the limit MUST NOT be split.
	body := strings.Repeat("a", 511) + "é" + strings.Repeat("b", 100)
	client := newBoundaryClient(t, 5*time.Second, tokensource.Static("token"), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(body))
	})

	_, err := client.Review("prompt")
	if err == nil {
		t.Fatal("Review succeeded unexpectedly")
	}
	if !utf8.ValidString(err.Error()) {
		t.Errorf("error message contains an invalid UTF-8 sequence: %q", err.Error()[len(err.Error())-20:])
	}
}

func TestTruncate_LimitBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{name: "shorter than limit", input: "abc", limit: 5, want: "abc"},
		{name: "exactly limit", input: "abcde", limit: 5, want: "abcde"},
		{name: "one over limit", input: "abcdef", limit: 5, want: "abcde"},
		{name: "zero limit", input: "abc", limit: 0, want: ""},
		{name: "empty input", input: "", limit: 5, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(truncate([]byte(tc.input), tc.limit)); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.limit, got, tc.want)
			}
		})
	}
}

// The standard library strips Authorization when a redirect changes the host.
func TestReview_CrossHostRedirectDoesNotForwardCredential(t *testing.T) {
	var leaked atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
	}))
	t.Cleanup(target.Close)
	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)

	client := newBoundaryClient(t, 5*time.Second, tokensource.Static("secret-token"), func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL+r.URL.Path, http.StatusTemporaryRedirect)
	})

	_, _ = client.Review("prompt")

	if got, _ := leaked.Load().(string); got != "" {
		t.Errorf("redirect target received Authorization %q", got)
	}
}

func TestBuildPayload_NumericBoundariesSerialize(t *testing.T) {
	tests := []struct {
		name string
		opts config.ModelOptions
		key  string
		want any
	}{
		{name: "zero temperature is sent", opts: config.ModelOptions{Temperature: testutil.Ptr(0.0)}, key: "temperature", want: float64(0)},
		{name: "zero top p is sent", opts: config.ModelOptions{TopP: testutil.Ptr(0.0)}, key: "top_p", want: float64(0)},
		{name: "zero seed is sent", opts: config.ModelOptions{Seed: testutil.Ptr(int64(0))}, key: "seed", want: float64(0)},
		{name: "negative seed is sent", opts: config.ModelOptions{Seed: testutil.Ptr(int64(-1))}, key: "seed", want: float64(-1)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.Provider = "openai"
			tc.opts.Model = "gpt-4o"
			c := mustNewClient(t, Config{ModelOptions: tc.opts, Tokens: tokensource.Static("t")})
			raw, err := json.Marshal(c.buildPayload("prompt"))
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if got, ok := decoded[tc.key]; !ok || got != tc.want {
				t.Errorf("%s = %v (present %t), want %v", tc.key, got, ok, tc.want)
			}
		})
	}
}

type staticCredProvider struct {
	cred tokensource.Credential
}

func (s staticCredProvider) FetchCredential(context.Context) (tokensource.Credential, error) {
	return s.cred, nil
}

func (s staticCredProvider) Token(context.Context) (string, error) {
	return s.cred.Value, nil
}

func TestReview_HeaderDispatch_Permutations(t *testing.T) {
	tests := []struct {
		name       string
		opts       config.ModelOptions
		cred       tokensource.Credential
		wantAuth   string
		wantAPIKey string
	}{
		{
			name:       "azure-openai with api-key credential sends api-key header",
			opts:       config.ModelOptions{Provider: "azure-openai", Model: "gpt-4o", APIVersion: "2024-02-15-preview"},
			cred:       tokensource.Credential{Value: "azure-secret-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "",
			wantAPIKey: "azure-secret-key",
		},
		{
			name:       "azure-openai with bearer credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "azure-openai", Model: "gpt-4o", APIVersion: "2024-02-15-preview"},
			cred:       tokensource.Credential{Value: "azure-jwt-token", Kind: tokensource.KindBearer},
			wantAuth:   "Bearer azure-jwt-token",
			wantAPIKey: "",
		},
		{
			name:       "openai with api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "openai", Model: "gpt-4o"},
			cred:       tokensource.Credential{Value: "sk-openai-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer sk-openai-key",
			wantAPIKey: "",
		},
		{
			name:       "openai with bearer credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "openai", Model: "gpt-4o"},
			cred:       tokensource.Credential{Value: "bearer-token", Kind: tokensource.KindBearer},
			wantAuth:   "Bearer bearer-token",
			wantAPIKey: "",
		},
		{
			name:       "openai with api_version and api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "openai", Model: "gpt-4o", APIVersion: "2024-02-15"},
			cred:       tokensource.Credential{Value: "sk-openai-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer sk-openai-key",
			wantAPIKey: "",
		},
		{
			name:       "openai with api_version and bearer credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "openai", Model: "gpt-4o", APIVersion: "2024-02-15"},
			cred:       tokensource.Credential{Value: "bearer-token", Kind: tokensource.KindBearer},
			wantAuth:   "Bearer bearer-token",
			wantAPIKey: "",
		},
		{
			name:       "grok with api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "grok", Model: "grok-4"},
			cred:       tokensource.Credential{Value: "xai-api-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer xai-api-key",
			wantAPIKey: "",
		},
		{
			name:       "grok with bearer credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "grok", Model: "grok-4"},
			cred:       tokensource.Credential{Value: "xai-bearer-token", Kind: tokensource.KindBearer},
			wantAuth:   "Bearer xai-bearer-token",
			wantAPIKey: "",
		},
		{
			name:       "local with empty api-key credential sends no headers",
			opts:       config.ModelOptions{Provider: "local", Model: "llama-3.3"},
			cred:       tokensource.Credential{Value: "", Kind: tokensource.KindAPIKey},
			wantAuth:   "",
			wantAPIKey: "",
		},
		{
			name:       "local with empty bearer credential sends no headers",
			opts:       config.ModelOptions{Provider: "local", Model: "llama-3.3"},
			cred:       tokensource.Credential{Value: "", Kind: tokensource.KindBearer},
			wantAuth:   "",
			wantAPIKey: "",
		},
		{
			name:       "local with non-empty api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "local", Model: "llama-3.3"},
			cred:       tokensource.Credential{Value: "local-auth-token", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer local-auth-token",
			wantAPIKey: "",
		},
		{
			name:       "local with non-empty bearer credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "local", Model: "llama-3.3"},
			cred:       tokensource.Credential{Value: "local-bearer-token", Kind: tokensource.KindBearer},
			wantAuth:   "Bearer local-bearer-token",
			wantAPIKey: "",
		},
		{
			name:       "custom provider with api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "custom-gateway", Model: "custom-model"},
			cred:       tokensource.Credential{Value: "gateway-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer gateway-key",
			wantAPIKey: "",
		},
		{
			name:       "custom provider with bearer credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "custom-gateway", Model: "custom-model"},
			cred:       tokensource.Credential{Value: "gateway-bearer", Kind: tokensource.KindBearer},
			wantAuth:   "Bearer gateway-bearer",
			wantAPIKey: "",
		},
		{
			name:       "custom provider with api_version and api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "custom-gateway", Model: "custom-model", APIVersion: "2025-01-01"},
			cred:       tokensource.Credential{Value: "gateway-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer gateway-key",
			wantAPIKey: "",
		},
		{
			name:       "empty provider with api-key credential sends Authorization Bearer header",
			opts:       config.ModelOptions{Provider: "", Model: "custom-model"},
			cred:       tokensource.Credential{Value: "gateway-key", Kind: tokensource.KindAPIKey},
			wantAuth:   "Bearer gateway-key",
			wantAPIKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth, gotAPIKey string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotAPIKey = r.Header.Get("api-key")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
			}))
			t.Cleanup(server.Close)

			opts := tc.opts
			opts.BaseURL = server.URL
			client := mustNewClient(t, Config{
				ModelOptions: opts,
				Tokens: staticCredProvider{
					cred: tc.cred,
				},
			})

			res, err := client.Review("test prompt")
			if err != nil {
				t.Fatalf("Review() returned unexpected error: %v", err)
			}
			if res != "[]" {
				t.Errorf("Review() result = %q, want %q", res, "[]")
			}

			if gotAuth != tc.wantAuth {
				t.Errorf("Authorization header = %q, want %q", gotAuth, tc.wantAuth)
			}
			if gotAPIKey != tc.wantAPIKey {
				t.Errorf("api-key header = %q, want %q", gotAPIKey, tc.wantAPIKey)
			}
		})
	}
}
