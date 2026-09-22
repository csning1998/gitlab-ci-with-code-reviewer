//go:build unix

package review

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A named pipe reports size zero to os.Stat, which defeats the size check performed before the
// read. The read MUST NOT return more than MaxPromptSizeBytes.
func TestResolvePrompt_NamedPipeCannotBypassSizeLimit(t *testing.T) {
	pipePath := filepath.Join(t.TempDir(), "prompt.fifo")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Skipf("named pipes unavailable: %v", err)
	}

	writer, err := os.OpenFile(pipePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open pipe for writing: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	oversized := bytes.Repeat([]byte("a"), MaxPromptSizeBytes+4096)
	go func() {
		_, _ = writer.Write(oversized)
		_ = writer.Close()
	}()

	prompt, err := ResolvePrompt("", pipePath)
	if err == nil {
		t.Fatalf("ResolvePrompt returned %d bytes from a named pipe, want a rejection", len(prompt))
	}
}
