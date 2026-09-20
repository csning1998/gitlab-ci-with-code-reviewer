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

func executeGeminiReviewSubprocess(t *testing.T, extraEnv ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestGeminiReviewSubprocessHelper")
	baseEnv := []string{
		"BE_GEMINI_REVIEW_CMD=1",
		"CI_API_V4_URL=https://gitlab.example.com/api/v4",
		"CI_PROJECT_ID=1",
		"CI_MERGE_REQUEST_IID=2",
		"GEMINI_MR_REVIEWER=mock-gitlab-token",
		"GEMINI_API_KEY=mock-gemini-key",
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

// TestGeminiReviewSubprocessHelper is re-executed in isolation to observe main() process exit behaviors.
func TestGeminiReviewSubprocessHelper(t *testing.T) {
	if os.Getenv("BE_GEMINI_REVIEW_CMD") != "1" {
		t.Skip("only runs as a re-executed subprocess")
	}
	main()
}

func TestGeminiReview_MissingGeminiToken_ExitsNonzero(t *testing.T) {
	code, _, stderr := executeGeminiReviewSubprocess(t, "GEMINI_MR_REVIEWER=")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "GEMINI_MR_REVIEWER") {
		t.Errorf("stderr = %q, want mention of GEMINI_MR_REVIEWER", stderr)
	}
}

func TestGeminiReview_MissingGeminiKey_ExitsNonzero(t *testing.T) {
	code, _, stderr := executeGeminiReviewSubprocess(t, "GEMINI_API_KEY=")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "GEMINI_API_KEY") {
		t.Errorf("stderr = %q, want mention of GEMINI_API_KEY", stderr)
	}
}

func TestGeminiReview_EmptyChanges_SucceedsWithoutLLMCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"feat: test","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`))
	}))
	t.Cleanup(server.Close)

	code, _, stderr := executeGeminiReviewSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestGeminiReview_GitLabError_ExitsNonzero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	code, _, stderr := executeGeminiReviewSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if len(stderr) == 0 {
		t.Error("stderr is empty, want error output on review failure")
	}
}
