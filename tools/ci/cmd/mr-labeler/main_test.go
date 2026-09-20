package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func executeMRLabelerSubprocess(t *testing.T, extraEnv ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestMRLabelerSubprocessHelper")
	baseEnv := []string{
		"BE_MR_LABELER_CMD=1",
		"CI_API_V4_URL=https://gitlab.example.com/api/v4",
		"CI_PROJECT_ID=1",
		"CI_MERGE_REQUEST_IID=2",
		"CLAUDE_MR_REVIEWER=mock-gitlab-token",
	}
	cmd.Env = append(os.Environ(), append(baseEnv, extraEnv...)...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("subprocess failed with unexpected error: %v", err)
	}
	return exitErr.ExitCode(), outBuf.String(), errBuf.String()
}

// TestMRLabelerSubprocessHelper is re-executed in isolation to observe main() process exit behaviors.
func TestMRLabelerSubprocessHelper(t *testing.T) {
	if os.Getenv("BE_MR_LABELER_CMD") != "1" {
		t.Skip("only runs as a re-executed subprocess")
	}
	main()
}

func TestMRLabeler_MissingTokens_ExitsNonzero(t *testing.T) {
	code, _, stderr := executeMRLabelerSubprocess(t, "CLAUDE_MR_REVIEWER=", "GEMINI_MR_REVIEWER=")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "need CLAUDE_MR_REVIEWER or GEMINI_MR_REVIEWER") {
		t.Errorf("stderr = %q, want mention of required tokens", stderr)
	}
}

func TestMRLabeler_ExecuteLabelingSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`[{"new_path":"main.go","diff":"@@ -0,0 +1,1 @@\n+package main\n"}]`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/discussions") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(`{"title":"feat: test","description":"","labels":[]}`))
	}))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRLabelerSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestMRLabeler_ExecuteLabelingFails_ExitsNonzero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRLabelerSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if len(stderr) == 0 {
		t.Error("stderr is empty, want error message on failure")
	}
}

func TestMRLabeler_FallbackToGeminiToken_Succeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "gemini-only-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/discussions") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"feat: test","labels":[]}`))
	}))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRLabelerSubprocess(t,
		"CI_API_V4_URL="+server.URL,
		"CLAUDE_MR_REVIEWER=",
		"GEMINI_MR_REVIEWER=gemini-only-token",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}
