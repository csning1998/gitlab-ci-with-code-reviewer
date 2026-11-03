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

	"ci-tools/internal/config"
	"ci-tools/internal/testutil"
)

const (
	testProjectID           = "123"
	unreachableAPIURL       = "https://example.invalid/api/v4"
	selfPath                = "/personal_access_tokens/self"
	tagsPath                = "/projects/" + testProjectID + "/repository/tags"
	legacyTokenSelf         = `{"active":true,"scopes":["api"]}`
	subprocessEnvName       = "BE_VERIFY_TAG_TOKEN"
	subprocessAPIURLEnvName = "TEST_API_URL"
)

func TestExecuteVerifyTagToken_MissingAPIURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken("", testProjectID, testutil.GitLabToken, &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--api-url is required") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to mention the missing --api-url", stderr.String())
	}
}

func TestExecuteVerifyTagToken_MissingProjectID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(unreachableAPIURL, "", testutil.GitLabToken, &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--project-id is required") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to mention the missing --project-id", stderr.String())
	}
}

func TestExecuteVerifyTagToken_MissingToken_SkipsWithoutFailing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(unreachableAPIURL, testProjectID, "", &stdout, &stderr)
	if code != 0 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 0 (a missing token skips rather than fails)", code)
	}
	if !strings.Contains(stdout.String(), "Warning:") {
		t.Errorf("executeVerifyTagToken(...) stdout = %q, want a notice explaining the skip", stdout.String())
	}
}

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
	server, paths := newGitLabStub(t, http.StatusForbidden, testutil.InsufficientGranularScopeBody, http.StatusOK)

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, testProjectID, testutil.GitLabToken, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeVerifyTagToken(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if got, want := strings.Join(*paths, ","), selfPath+","+tagsPath; got != want {
		t.Errorf("request paths = %q, want %q (the legacy token check precedes the tag read)", got, want)
	}
}

func TestExecuteVerifyTagToken_LegacyTokenRejectedBeforeTagRead(t *testing.T) {
	server, paths := newGitLabStub(t, http.StatusOK, legacyTokenSelf, http.StatusOK)

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, testProjectID, testutil.GitLabToken, &stdout, &stderr)
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
	server, _ := newGitLabStub(t, http.StatusForbidden, testutil.InsufficientGranularScopeBody, http.StatusForbidden)

	var stdout, stderr bytes.Buffer
	code := executeVerifyTagToken(server.URL, testProjectID, testutil.GitLabToken, &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeVerifyTagToken(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "status 403") {
		t.Errorf("executeVerifyTagToken(...) stderr = %q, want it to report status 403", stderr.String())
	}
}

// TestMainSubprocess executes main in a child process to prevent os.Exit from terminating the test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv(subprocessEnvName) == "1" {
		os.Args = []string{
			"verify-tag-token",
			"--api-url=" + os.Getenv(subprocessAPIURLEnvName),
			"--project-id=" + testProjectID,
		}
		main()
		return
	}

	server, _ := newGitLabStub(t, http.StatusForbidden, testutil.InsufficientGranularScopeBody, http.StatusOK)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		subprocessEnvName+"=1",
		config.TagPushTokenEnvName+"="+testutil.GitLabToken,
		subprocessAPIURLEnvName+"="+server.URL,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}
}
