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
	code := executeVerifyTagToken("", "123", "token", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--api-url is required") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to mention the missing --api-url", stderr.String())
	}
}

func TestExecuteVerifyTagToken_MissingProjectID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken("https://example.invalid/api/v4", "", "token", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--project-id is required") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to mention the missing --project-id", stderr.String())
	}
}

func TestExecuteVerifyTagToken_MissingToken_SkipsWithoutFailing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken("https://example.invalid/api/v4", "123", "", &stdout, &stderr)
	if code != 0 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 0 (a missing token skips rather than fails)", code)
	}
	if !strings.Contains(stdout.String(), "Warning:") {
		t.Errorf("executeVerifyTagToken(...) stdout = %q, want a notice explaining the skip", stdout.String())
	}
}

func TestExecuteVerifyTagToken_Success(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, "123", "token", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeVerifyTagToken(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if gotPath != "/projects/123/repository/tags" {
		t.Errorf("request path = %q, want %q", gotPath, "/projects/123/repository/tags")
	}
}

func TestExecuteVerifyTagToken_NoTagReadAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, "123", "token", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "status 403") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to report status 403", stderr.String())
	}
}

// TestMainSubprocess executes main in a child process to prevent os.Exit from terminating the test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_VERIFY_TAG_TOKEN") == "1" {
		os.Args = []string{
			"verify-tag-token",
			"--api-url=" + os.Getenv("TEST_API_URL"),
			"--project-id=123",
		}
		main()
		return
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
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
