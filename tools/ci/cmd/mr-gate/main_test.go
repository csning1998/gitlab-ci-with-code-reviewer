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

func executeMRGateSubprocess(t *testing.T, extraEnv ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestMRGateSubprocessHelper")
	baseEnv := []string{
		"BE_MR_GATE_CMD=1",
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

// TestMRGateSubprocessHelper is re-executed in isolation to observe main() process exit behaviors.
func TestMRGateSubprocessHelper(t *testing.T) {
	if os.Getenv("BE_MR_GATE_CMD") != "1" {
		t.Skip("only runs as a re-executed subprocess")
	}
	main()
}

func TestMRGate_MissingTokens_ExitsNonzero(t *testing.T) {
	code, _, stderr := executeMRGateSubprocess(t, config.ClaudeReviewTokenEnvName+"=", config.GeminiReviewTokenEnvName+"=")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "need "+config.ClaudeReviewTokenEnvName+" or "+config.GeminiReviewTokenEnvName) {
		t.Errorf("stderr = %q, want mention of required tokens", stderr)
	}
}

func TestMRGate_LegacyTokenRejectedBeforeMRRequest(t *testing.T) {
	server, mrRequests := testutil.NewLegacyTokenServer(t)

	code, _, stderr := executeMRGateSubprocess(t, "CI_API_V4_URL="+server.URL)
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

func TestMRGate_InvalidRuneLimit_ExitsNonzero(t *testing.T) {
	code, _, stderr := executeMRGateSubprocess(t, "MAX_DESCRIPTION_CHARS=invalid-number")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "MAX_DESCRIPTION_CHARS") {
		t.Errorf("stderr = %q, want the invalid MAX_DESCRIPTION_CHARS named", stderr)
	}
}

func TestMRGate_FetchDescriptionFails_ExitsNonzero(t *testing.T) {
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRGateSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("stderr = %q, want mention of Error", stderr)
	}
}

func TestMRGate_DescriptionExceedsLimit_ExitsNonzero(t *testing.T) {
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"description":"` + strings.Repeat("x", 100) + `"}`))
	})))
	t.Cleanup(server.Close)

	code, _, stderr := executeMRGateSubprocess(t, "CI_API_V4_URL="+server.URL, "MAX_DESCRIPTION_CHARS=50")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("stderr = %q, want mention of Error", stderr)
	}
}

func TestMRGate_DescriptionWithinLimit_Succeeds(t *testing.T) {
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"description":"short valid description"}`))
	})))
	t.Cleanup(server.Close)

	code, stdout, stderr := executeMRGateSubprocess(t, "CI_API_V4_URL="+server.URL, "MAX_DESCRIPTION_CHARS=500")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "MR description within limit.") {
		t.Errorf("stdout = %q, want success message", stdout)
	}
}

func TestMRGate_FallbackToGeminiToken_Succeeds(t *testing.T) {
	server := httptest.NewServer(testutil.HandleFineGrainedSelf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != fallbackToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"description":"valid"}`))
	})))
	t.Cleanup(server.Close)

	code, stdout, stderr := executeMRGateSubprocess(t,
		"CI_API_V4_URL="+server.URL,
		config.ClaudeReviewTokenEnvName+"=",
		config.GeminiReviewTokenEnvName+"="+fallbackToken,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "MR description within limit.") {
		t.Errorf("stdout = %q, want success message", stdout)
	}
}
