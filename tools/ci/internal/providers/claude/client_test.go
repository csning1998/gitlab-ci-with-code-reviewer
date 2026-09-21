package claude

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"ci-tools/internal/config"
	"ci-tools/internal/testutil"
	"ci-tools/internal/tokensource"
)

// streamBody provides a minimal server-sent event stream carrying one text block.
const streamBody = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"[]"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`

func mustNewClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New(...) returned an unexpected error: %v", err)
	}
	return c
}

// stubCapturingClient serves one streamed text block and records the decoded request body.
func stubCapturingClient(t *testing.T, opts config.ModelOptions, gotBody *map[string]any) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(streamBody))
	}))
	t.Cleanup(server.Close)

	opts.BaseURL = server.URL
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	return mustNewClient(t, Config{ModelOptions: opts, Tokens: tokensource.Static("test-api-key")})
}

func unmarshalContentBlocks(t *testing.T, raws ...string) []sdk.ContentBlockUnion {
	t.Helper()
	out := make([]sdk.ContentBlockUnion, 0, len(raws))
	for _, raw := range raws {
		var block sdk.ContentBlockUnion
		if err := block.UnmarshalJSON([]byte(raw)); err != nil {
			t.Fatalf("unmarshal ContentBlockUnion: %v", err)
		}
		out = append(out, block)
	}
	return out
}

func TestName_ReturnsClaude(t *testing.T) {
	t.Parallel()
	if got := (&Client{}).Name(); got != "Claude" {
		t.Errorf("Name() = %q, want Claude", got)
	}
}

func TestNew_ConfigurationValidation(t *testing.T) {
	t.Parallel()
	t.Run("rejects invalid configuration", func(t *testing.T) {
		tests := []struct {
			name     string
			cfg      Config
			wantWord string
		}{
			{
				name:     "rejects nil tokens",
				cfg:      Config{ModelOptions: config.ModelOptions{Model: "claude-sonnet-5"}},
				wantWord: "tokens",
			},
			{
				name:     "rejects empty model",
				cfg:      Config{Tokens: tokensource.Static("test-api-key")},
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

	t.Run("applies default maxTokens and timeout when unset", func(t *testing.T) {
		c, err := New(Config{
			ModelOptions: config.ModelOptions{Model: "claude-sonnet-5"},
			Tokens:       tokensource.Static("test-api-key"),
		})
		if err != nil {
			t.Fatalf("New(...) returned unexpected error: %v", err)
		}
		if c.maxTokens != DefaultMaxTokens {
			t.Errorf("maxTokens = %d, want default %d", c.maxTokens, DefaultMaxTokens)
		}
		if c.timeout != config.DefaultTimeout {
			t.Errorf("timeout = %v, want default %v", c.timeout, config.DefaultTimeout)
		}
	})
}

func TestReview_ReportsCredentialFailureBeforeRequest(t *testing.T) {
	t.Parallel()
	var reached bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	t.Cleanup(server.Close)

	c := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Model: "claude-sonnet-5", BaseURL: server.URL},
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

func TestReview_SendsResolvedCredentialAndOptions(t *testing.T) {
	t.Parallel()
	var gotKey string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(streamBody))
	}))
	t.Cleanup(server.Close)

	c := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{
			Model:     "claude-sonnet-5",
			MaxTokens: 4096,
			BaseURL:   server.URL,
			Timeout:   10 * time.Second,
		},
		Tokens: tokensource.Static("resolved-key"),
	})

	got, err := c.Review("review this")
	if err != nil {
		t.Fatalf("Review(...) returned unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("Review(...) = %q, want []", got)
	}
	if gotKey != "resolved-key" {
		t.Errorf("X-Api-Key = %q, want resolved-key", gotKey)
	}
	if gotBody["model"] != "claude-sonnet-5" {
		t.Errorf("model = %v, want claude-sonnet-5", gotBody["model"])
	}
	if gotBody["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want 4096", gotBody["max_tokens"])
	}
}

func TestExtractTextBlocks_ConcatenatesTextBlocksOnly(t *testing.T) {
	t.Parallel()
	blocks := unmarshalContentBlocks(t,
		`{"type":"text","text":"[{\"file\":\"a.go\"}]"}`,
		`{"type":"text","text":",[]"}`,
	)
	got := extractTextBlocks(blocks)
	if got != `[{"file":"a.go"}],[]` {
		t.Fatalf("extractTextBlocks = %q, want concatenated text blocks", got)
	}

	if empty := extractTextBlocks(nil); empty != "" {
		t.Fatalf("extractTextBlocks(nil) = %q, want empty string", empty)
	}
}

func TestReview_SamplingParameters_TableDriven(t *testing.T) {
	t.Parallel()
	t.Run("injects sampling parameters when configured", func(t *testing.T) {
		var gotBody map[string]any
		c := stubCapturingClient(t, config.ModelOptions{
			Provider:    "claude",
			Model:       "claude-sonnet-5",
			MaxTokens:   4096,
			Temperature: testutil.Ptr(0.3),
			TopP:        testutil.Ptr(0.9),
			TopK:        testutil.Ptr(40),
			Stop:        []string{"END"},
		}, &gotBody)

		if _, err := c.Review("prompt"); err != nil {
			t.Fatalf("Review(...) returned unexpected error: %v", err)
		}
		if gotBody["temperature"] != 0.3 {
			t.Errorf("temperature = %v, want 0.3", gotBody["temperature"])
		}
		if gotBody["top_p"] != 0.9 {
			t.Errorf("top_p = %v, want 0.9", gotBody["top_p"])
		}
		if gotBody["top_k"] != float64(40) {
			t.Errorf("top_k = %v, want 40", gotBody["top_k"])
		}
		stop, _ := gotBody["stop_sequences"].([]any)
		if len(stop) != 1 || stop[0] != "END" {
			t.Errorf("stop_sequences = %v, want [END]", gotBody["stop_sequences"])
		}
	})

	t.Run("omits sampling parameters when unset", func(t *testing.T) {
		var gotBody map[string]any
		c := stubCapturingClient(t, config.ModelOptions{Model: "claude-sonnet-5", MaxTokens: 4096}, &gotBody)

		if _, err := c.Review("prompt"); err != nil {
			t.Fatalf("Review(...) returned unexpected error: %v", err)
		}
		assertOmittedKeys(t, gotBody, "temperature", "top_p", "top_k", "stop_sequences", "output_config")
	})
}

func TestReview_ThinkingGeneration_TableDriven(t *testing.T) {
	t.Parallel()
	t.Run("rejects invalid thinking configuration", func(t *testing.T) {
		tests := []struct {
			name      string
			opts      config.ModelOptions
			wantError string
		}{
			{
				name: "rejects manual budget on adaptive generation",
				opts: config.ModelOptions{
					Model:          "claude-opus-4-7",
					MaxTokens:      8192,
					ThinkingBudget: testutil.Ptr(2048),
				},
				wantError: "adaptive",
			},
			{
				name: "rejects budget below 1024 floor",
				opts: config.ModelOptions{
					Model:          "claude-sonnet-4-5-20250929",
					MaxTokens:      8192,
					ThinkingBudget: testutil.Ptr(512),
				},
				wantError: "floor",
			},
			{
				name: "rejects budget equal or above max_tokens",
				opts: config.ModelOptions{
					Model:          "claude-sonnet-4-5-20250929",
					MaxTokens:      2048,
					ThinkingBudget: testutil.Ptr(2048),
				},
				wantError: "stay below max_tokens",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := New(Config{ModelOptions: tc.opts, Tokens: tokensource.Static("test-api-key")})
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("New(...) error = %v, want error containing %q", err, tc.wantError)
				}
			})
		}
	})

	t.Run("generates valid thinking payload", func(t *testing.T) {
		tests := []struct {
			name       string
			opts       config.ModelOptions
			assertBody func(t *testing.T, body map[string]any)
		}{
			{
				name: "defaults to disabled thinking",
				opts: config.ModelOptions{Model: "claude-sonnet-5", MaxTokens: 4096},
				assertBody: func(t *testing.T, body map[string]any) {
					assertThinkingField(t, body, "type", "disabled")
				},
			},
			{
				name: "legacy generation uses manual thinking budget",
				opts: config.ModelOptions{
					Model:          "claude-sonnet-4-5-20250929",
					MaxTokens:      8192,
					ThinkingBudget: testutil.Ptr(2048),
				},
				assertBody: func(t *testing.T, body map[string]any) {
					assertThinkingField(t, body, "type", "enabled")
					assertThinkingField(t, body, "budget_tokens", float64(2048))
				},
			},
			{
				name: "adaptive generation uses effort",
				opts: config.ModelOptions{
					Model:          "claude-opus-4-7",
					MaxTokens:      8192,
					ReasoningLevel: "xhigh",
				},
				assertBody: func(t *testing.T, body map[string]any) {
					assertThinkingField(t, body, "type", "adaptive")
					assertOutputConfigField(t, body, "effort", "xhigh")
				},
			},
			{
				name: "thinking suppresses sampling overrides",
				opts: config.ModelOptions{
					Model:          "claude-opus-4-7",
					MaxTokens:      8192,
					ReasoningLevel: "high",
					Temperature:    testutil.Ptr(0.3),
					TopP:           testutil.Ptr(0.9),
					TopK:           testutil.Ptr(40),
				},
				assertBody: func(t *testing.T, body map[string]any) {
					assertThinkingField(t, body, "type", "adaptive")
					assertOutputConfigField(t, body, "effort", "high")
					assertOmittedKeys(t, body, "temperature", "top_p", "top_k")
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				var gotBody map[string]any
				c := stubCapturingClient(t, tc.opts, &gotBody)
				if _, err := c.Review("prompt"); err != nil {
					t.Fatalf("Review(...) returned unexpected error: %v", err)
				}
				tc.assertBody(t, gotBody)
			})
		}
	})
}

func assertOmittedKeys(t *testing.T, body map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, present := body[key]; present {
			t.Errorf("field %q MUST be omitted", key)
		}
	}
}

func assertThinkingField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	thinking, _ := body["thinking"].(map[string]any)
	if got := thinking[key]; got != want {
		t.Errorf("thinking[%q] = %v, want %v", key, got, want)
	}
}

func assertOutputConfigField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	cfg, _ := body["output_config"].(map[string]any)
	if got := cfg[key]; got != want {
		t.Errorf("output_config[%q] = %v, want %v", key, got, want)
	}
}
