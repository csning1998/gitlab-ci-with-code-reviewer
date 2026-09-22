//go:build unix

package config

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Opening a FIFO for reading blocks until a writer opens the other end. readBoundedDeclarationFile
// bounds that wait under declarationReadTimeout instead of hanging forever.
func TestParseConfigFile_FIFOWithNoWriterTimesOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reviewer.yml")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, err := ParseConfigFile(path)
		done <- result{err}
	}()
	t.Cleanup(func() {
		w, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			_ = w.Close()
		}
	})

	select {
	case r := <-done:
		if r.err == nil {
			t.Fatal("ParseConfigFile succeeded unexpectedly on a FIFO with no writer; want a read-timeout error")
		}
		if !strings.Contains(r.err.Error(), "exceeded") {
			t.Errorf("ParseConfigFile error = %q, want it to name the read timeout", r.err.Error())
		}
	case <-time.After(declarationReadTimeout + 3*time.Second):
		t.Fatal("ParseConfigFile did not return within declarationReadTimeout plus margin; want the bound enforced")
	}
}
