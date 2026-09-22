package claude

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
	"ci-tools/internal/tokensource"
)

type tokenFunc func(context.Context) (string, error)

func (f tokenFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

func buildSSEStream(text string, complete bool) string {
	quoted, _ := json.Marshal(text)
	var b strings.Builder
	b.WriteString("event: message_start\n")
	b.WriteString(`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}` + "\n\n")
	b.WriteString("event: content_block_start\n")
	b.WriteString(`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n")
	b.WriteString("event: content_block_delta\n")
	fmt.Fprintf(&b, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`+"\n\n", quoted)
	if !complete {
		return b.String()
	}
	b.WriteString("event: content_block_stop\n")
	b.WriteString(`data: {"type":"content_block_stop","index":0}` + "\n\n")
	b.WriteString("event: message_delta\n")
	b.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}` + "\n\n")
	b.WriteString("event: message_stop\n")
	b.WriteString(`data: {"type":"message_stop"}` + "\n\n")
	return b.String()
}

func newBoundaryClient(t *testing.T, timeout time.Duration, tokens tokensource.Provider, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Provider: "claude", Model: "claude-sonnet-5", BaseURL: server.URL, Timeout: timeout},
		Tokens:       tokens,
	})
}

func echoPromptHandler(seen *sync.Map) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get("X-Api-Key"), struct{}{})
		var payload struct {
			Messages []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		text := ""
		if len(payload.Messages) > 0 && len(payload.Messages[0].Content) > 0 {
			text = payload.Messages[0].Content[0].Text
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(buildSSEStream(text, true)))
	}
}

func TestReview_ConcurrentCallsKeepPromptsAndCredentialsSeparate(t *testing.T) {
	const callers = 32

	var issued atomic.Int64
	tokens := tokenFunc(func(context.Context) (string, error) {
		return fmt.Sprintf("key-%d", issued.Add(1)), nil
	})
	var seen sync.Map
	client := newBoundaryClient(t, 20*time.Second, tokens, echoPromptHandler(&seen))

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

func TestReview_TimeoutBoundsStalledStream(t *testing.T) {
	release := make(chan struct{})
	client := newBoundaryClient(t, 300*time.Millisecond, tokensource.Static("key"), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(buildSSEStream("[", false)))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
	})
	t.Cleanup(func() { close(release) })

	start := time.Now()
	if _, err := client.Review("prompt"); err == nil {
		t.Fatal("Review succeeded unexpectedly against a stalled stream")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Review took %v, want a bound near the 300ms timeout", elapsed)
	}
}

func TestReview_TruncatedStreamIsAnError(t *testing.T) {
	client := newBoundaryClient(t, 10*time.Second, tokensource.Static("key"), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(buildSSEStream(`[{"file":"a.go"`, false)))
	})

	got, err := client.Review("prompt")
	if err == nil {
		t.Errorf("Review returned %q from a stream which ended before message_stop, want an error", got)
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
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte(buildSSEStream("[]", true)))
				})

			_, err := client.Review("prompt")
			if err == nil && reached.Load() {
				t.Errorf("Review sent a request with credential %q", tc.token)
			}
		})
	}
}

// The standard library forwards the X-Api-Key header on a redirect to another host. The client
// MUST NOT forward the key to that host.
func TestReview_CrossHostRedirectDoesNotForwardCredential(t *testing.T) {
	var leaked atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("X-Api-Key"))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(buildSSEStream("[]", true)))
	}))
	t.Cleanup(target.Close)
	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)

	client := newBoundaryClient(t, 10*time.Second, tokensource.Static("secret-key"), func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL+r.URL.Path, http.StatusTemporaryRedirect)
	})

	_, _ = client.Review("prompt")

	if got, _ := leaked.Load().(string); got != "" {
		t.Errorf("redirect target received X-Api-Key %q", got)
	}
}

func TestReview_HostileEnvironmentDoesNotRedirectRequests(t *testing.T) {
	var hijacked atomic.Bool
	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacked.Store(true)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(buildSSEStream("[]", true)))
	}))
	t.Cleanup(hostile.Close)
	t.Setenv("ANTHROPIC_BASE_URL", hostile.URL)
	t.Setenv("ANTHROPIC_API_KEY", "ambient-key")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "ambient-token")

	var sawAmbient atomic.Bool
	intended := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "explicit-key" || r.Header.Get("Authorization") != "" {
			sawAmbient.Store(true)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(buildSSEStream("[]", true)))
	}))
	t.Cleanup(intended.Close)

	client := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Provider: "claude", Model: "claude-sonnet-5", BaseURL: intended.URL, Timeout: 10 * time.Second},
		Tokens:       tokensource.Static("explicit-key"),
	})
	if _, err := client.Review("prompt"); err != nil {
		t.Fatalf("Review returned an unexpected error: %v", err)
	}
	if hijacked.Load() {
		t.Error("ANTHROPIC_BASE_URL redirected the request away from the configured endpoint")
	}
	if sawAmbient.Load() {
		t.Error("an ambient ANTHROPIC_* credential accompanied or replaced the configured key")
	}
}

func TestReview_EmptyBaseURLIgnoresAmbientEndpoint(t *testing.T) {
	var hijacked atomic.Bool
	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacked.Store(true)
		http.Error(w, "unexpected", http.StatusTeapot)
	}))
	t.Cleanup(hostile.Close)
	t.Setenv("ANTHROPIC_BASE_URL", hostile.URL)

	client := mustNewClient(t, Config{
		ModelOptions: config.ModelOptions{Provider: "claude", Model: "claude-sonnet-5", Timeout: 2 * time.Second},
		Tokens:       tokensource.Static("key"),
	})
	_, _ = client.Review("prompt")

	if hijacked.Load() {
		t.Error("an ambient ANTHROPIC_BASE_URL captured a request whose credential was configured explicitly")
	}
}

func TestReview_ResponseBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		stream  string
		want    string
		wantErr bool
	}{
		{name: "unicode text", stream: buildSSEStream("設定 \U0001F600", true), want: "設定 \U0001F600"},
		{name: "embedded nul", stream: buildSSEStream("a\x00b", true), want: "a\x00b"},
		{name: "empty text block", stream: buildSSEStream("", true), wantErr: true},
		{name: "not an event stream", stream: `{"type":"error"}`, wantErr: true},
		{name: "empty body", stream: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newBoundaryClient(t, 10*time.Second, tokensource.Static("key"), func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(tc.stream))
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

func TestReview_ErrorStatusBoundaries(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 529} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := newBoundaryClient(t, 10*time.Second, tokensource.Static("key"), func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
			})

			if _, err := client.Review("prompt"); err == nil {
				t.Errorf("status %d succeeded unexpectedly", status)
			}
		})
	}
}
