package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
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

const (
	selfPath                      = "/personal_access_tokens/self"
	tagsPath                      = "/projects/123/repository/tags"
	insufficientGranularScopeBody = `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following user permissions: [Personal Access Token: Read]."}`
)

// newGitLabStub answers the self endpoint with selfStatus and selfBody, answers the tag read with tagStatus,
// and records the request paths in order.
func newGitLabStub(t *testing.T, selfStatus int, selfBody string, tagStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case selfPath:
			w.WriteHeader(selfStatus)
			_, _ = w.Write([]byte(selfBody))
		case tagsPath:
			w.WriteHeader(tagStatus)
			if tagStatus == http.StatusOK {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &paths
}

func TestExecuteVerifyTagToken_Success(t *testing.T) {
	server, paths := newGitLabStub(t, http.StatusForbidden, insufficientGranularScopeBody, http.StatusOK)

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, "123", "token", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeVerifyTagToken(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if got, want := strings.Join(*paths, ","), selfPath+","+tagsPath; got != want {
		t.Errorf("request paths = %q, want %q (the legacy token check precedes the tag read)", got, want)
	}
}

func TestExecuteVerifyTagToken_LegacyTokenRejectedBeforeTagRead(t *testing.T) {
	server, paths := newGitLabStub(t, http.StatusOK, `{"active":true,"scopes":["api"]}`, http.StatusOK)

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, "123", "token", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "legacy scopes") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to report the prohibited legacy scopes", stderr.String())
	}
	for _, p := range *paths {
		if p == tagsPath {
			t.Errorf("request paths = %q, want no tag read after a legacy token", *paths)
		}
	}
}

func TestExecuteVerifyTagToken_NoTagReadAccess(t *testing.T) {
	server, _ := newGitLabStub(t, http.StatusForbidden, insufficientGranularScopeBody, http.StatusForbidden)

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

	server, _ := newGitLabStub(t, http.StatusForbidden, insufficientGranularScopeBody, http.StatusOK)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		"BE_VERIFY_TAG_TOKEN=1",
		"TAG_PUSH_TOKEN=token",
		"TEST_API_URL="+server.URL,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}
}
