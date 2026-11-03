package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"ci-tools/internal/config"
	"ci-tools/internal/testutil"
)

// fallbackToken identifies the Gemini variable when the Claude variable is empty.
const fallbackToken = "gemini-only-token"

func executeMRLabelerSubprocess(t *testing.T, extraEnv ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestMRLabelerSubprocessHelper")
	baseEnv := []string{
		"BE_MR_LABELER_CMD=1",
		"CI_API_V4_URL=https://gitlab.example.com/api/v4",
		"CI_PROJECT_ID=1",
		"CI_MERGE_REQUEST_IID=2",
		config.ClaudeReviewTokenEnvName + "=" + testutil.GitLabToken,
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
	code, _, stderr := executeMRLabelerSubprocess(t, config.ClaudeReviewTokenEnvName+"=", config.GeminiReviewTokenEnvName+"=")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "need "+config.ClaudeReviewTokenEnvName+" or "+config.GeminiReviewTokenEnvName) {
		t.Errorf("stderr = %q, want mention of required tokens", stderr)
	}
}

func TestMRLabeler_LegacyTokenRejectedBeforeMRRequest(t *testing.T) {
	server, mrRequests := testutil.NewLegacyTokenServer(t)

	code, _, stderr := executeMRLabelerSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "legacy scopes") {
		t.Errorf("stderr = %q, want it to report the prohibited legacy scopes", stderr)
	}
	if got := mrRequests.Load(); got != 0 {
		t.Errorf("server received %d merge request API calls, want 0", got)
	}
}

func TestMRLabeler_ExecuteLabelingSucceeds(t *testing.T) {
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRLabelerSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestMRLabeler_ExecuteLabelingFails_ExitsNonzero(t *testing.T) {
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})))
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
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != fallbackToken {
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
	})))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRLabelerSubprocess(t,
		"CI_API_V4_URL="+server.URL,
		config.ClaudeReviewTokenEnvName+"=",
		config.GeminiReviewTokenEnvName+"="+fallbackToken,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}
