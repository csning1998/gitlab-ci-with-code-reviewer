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

func TestExecuteVerifyTagToken_MissingAPIURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken("", "api", "token", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--api-url is required") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to mention the missing --api-url", stderr.String())
	}
}

func TestExecuteVerifyTagToken_MissingToken_SkipsWithoutFailing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken("https://example.invalid/api/v4", "api", "", &stdout, &stderr)
	if code != 0 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 0 (a missing token skips rather than fails)", code)
	}
	if !strings.Contains(stdout.String(), "Warning:") {
		t.Errorf("executeVerifyTagToken(...) stdout = %q, want a notice explaining the skip", stdout.String())
	}
}

func TestExecuteVerifyTagToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"scopes":["api"],"active":true,"revoked":false}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, "api", "token", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeVerifyTagToken(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
}

func TestExecuteVerifyTagToken_InsufficientScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"scopes":["write_repository"],"active":true,"revoked":false}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, "api", "token", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "do not include the required") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to mention the missing scope", stderr.String())
	}
}

// TestMainSubprocess executes main in a child process to prevent os.Exit from terminating the test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_VERIFY_TAG_TOKEN") == "1" {
		os.Args = []string{
			"verify-tag-token",
			"--api-url=" + os.Getenv("TEST_API_URL"),
		}
		main()
		return
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"scopes":["api"],"active":true,"revoked":false}`))
	}))
	defer server.Close()

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		"BE_VERIFY_TAG_TOKEN=1",
		"TAG_PUSH_TOKEN=unused",
		"TEST_API_URL="+server.URL,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}
}
