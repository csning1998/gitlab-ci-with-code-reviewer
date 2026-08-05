package claude

import (
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

func TestExtractTextConcatenatesTextBlocksOnly(t *testing.T) {
	t.Parallel()

	blocks := mustContentBlocks(t,
		`{"type":"text","text":"[{\"file\":\"a.go\"}]"}`,
		`{"type":"text","text":",[]"}`,
	)
	got := extractText(blocks)
	if got != `[{"file":"a.go"}],[]` {
		t.Fatalf("extractText = %q, want concatenated text blocks", got)
	}
}

func TestExtractTextEmptyWhenNoTextBlocks(t *testing.T) {
	t.Parallel()

	if got := extractText(nil); got != "" {
		t.Fatalf("extractText(nil) = %q, want empty", got)
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
