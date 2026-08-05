package claude

import (
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

func TestName_ReturnsClaude(t *testing.T) {
	t.Parallel()
	if got := (&Client{}).Name(); got != "Claude" {
		t.Errorf("Name() = %q, want %q", got, "Claude")
	}
}

func TestNew_WiresModelMaxTokensAndTimeout(t *testing.T) {
	t.Parallel()
	c := New("claude-sonnet-5", "test-api-key", 8192, 10*time.Minute)
	if string(c.model) != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q", c.model, "claude-sonnet-5")
	}
	if c.maxTokens != 8192 {
		t.Errorf("maxTokens = %d, want 8192", c.maxTokens)
	}
	if c.timeout != 10*time.Minute {
		t.Errorf("timeout = %v, want %v", c.timeout, 10*time.Minute)
	}
}

func TestNew_ZeroMaxTokens(t *testing.T) {
	t.Parallel()
	c := New("claude-sonnet-5", "test-api-key", 0, 10*time.Minute)
	if c.maxTokens != 0 {
		t.Errorf("maxTokens = %d, want 0 passed through verbatim", c.maxTokens)
	}
}

func TestNew_EmptyAPIKey_DoesNotPanic(t *testing.T) {
	t.Parallel()
	// New performs no validation of apiKey; construction must succeed even with an empty key,
	// deferring authentication failures to the first actual API call.
	c := New("claude-sonnet-5", "", 1000, 10*time.Minute)
	if c == nil {
		t.Fatal("New(...) = nil, want a non-nil Client even for an empty API key")
	}
}

func TestExtractTextBlocksConcatenatesTextBlocksOnly(t *testing.T) {
	t.Parallel()

	blocks := mustContentBlocks(t,
		`{"type":"text","text":"[{\"file\":\"a.go\"}]"}`,
		`{"type":"text","text":",[]"}`,
	)
	got := extractTextBlocks(blocks)
	if got != `[{"file":"a.go"}],[]` {
		t.Fatalf("extractText = %q, want concatenated text blocks", got)
	}
}

func TestExtractTextBlocksEmptyWhenNoTextBlocks(t *testing.T) {
	t.Parallel()

	if got := extractTextBlocks(nil); got != "" {
		t.Fatalf("extractTextBlocks(nil) = %q, want empty", got)
	}
}

func mustContentBlocks(t *testing.T, raws ...string) []sdk.ContentBlockUnion {
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
