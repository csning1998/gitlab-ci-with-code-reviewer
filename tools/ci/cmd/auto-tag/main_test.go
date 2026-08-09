package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// tagRequest records parameters captured from a single POST /projects/:id/repository/tags API call.
type tagRequest struct {
	projectPath string
	escapedPath string
	tagName     string
	ref         string
}

// tagAPIServer implements a mock GitLab tag creation endpoint that logs requests and returns a fixed status code.
type tagAPIServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []tagRequest
	status   int
}

func newTagAPIServer(t *testing.T, status int) *tagAPIServer {
	t.Helper()
	s := &tagAPIServer{status: status}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, tagRequest{
			projectPath: r.URL.Path,
			escapedPath: r.URL.EscapedPath(),
			tagName:     r.URL.Query().Get("tag_name"),
			ref:         r.URL.Query().Get("ref"),
		})
		s.mu.Unlock()
		w.WriteHeader(s.status)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *tagAPIServer) recorded() []tagRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tagRequest(nil), s.requests...)
}

func assertTagCreated(t *testing.T, s *tagAPIServer, tagName, wantRef string) {
	t.Helper()
	for _, req := range s.recorded() {
		if req.tagName == tagName {
			if req.ref != wantRef {
				t.Errorf("tag %q created for ref %q, want %q", tagName, req.ref, wantRef)
			}
			return
		}
	}
	t.Errorf("no tag creation request recorded for %q; recorded = %+v", tagName, s.recorded())
}

func assertTagNotCreated(t *testing.T, s *tagAPIServer, tagName string) {
	t.Helper()
	for _, req := range s.recorded() {
		if req.tagName == tagName {
			t.Errorf("tag %q unexpectedly created", tagName)
		}
	}
}

func assertNoTagsCreated(t *testing.T, s *tagAPIServer) {
	t.Helper()
	if got := s.recorded(); len(got) != 0 {
		t.Errorf("recorded %d tag creation requests, want 0: %+v", len(got), got)
	}
}

func TestExecuteAutoTag_MissingArguments(t *testing.T) {
	cases := []struct {
		name       string
		sha        string
		apiBaseURL string
		projectID  string
		password   string
		wantErr    string
	}{
		{"missing sha", "", "https://example.invalid/api/v4", "123", "token", "--sha, --api-url, and --project-id are required"},
		{"missing api-url", "abc", "", "123", "token", "--sha, --api-url, and --project-id are required"},
		{"missing project-id", "abc", "https://example.invalid/api/v4", "", "token", "--sha, --api-url, and --project-id are required"},
		{"missing password", "abc", "https://example.invalid/api/v4", "123", "", "TAG_PUSH_TOKEN must be set"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := executeAutoTag(".", ".gitlab/versioning.yml", c.sha, c.apiBaseURL, c.projectID, c.password, io.Discard, &stderr)
			if code != 1 {
				t.Errorf("executeAutoTag(...) code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), c.wantErr) {
				t.Errorf("executeAutoTag(...) stderr = %q, want it to contain %q", stderr.String(), c.wantErr)
			}
		})
	}
}

func TestExecuteAutoTag_ConfigLoadFails(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "chore: init")
	missingConfig := filepath.Join(t.TempDir(), "does-not-exist.yml")

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, missingConfig, sha, "https://example.invalid/api/v4", "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to read versioning config") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the config read failure", stderr.String())
	}
}

func TestExecuteAutoTag_RepoOpenFails(t *testing.T) {
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)

	var stderr bytes.Buffer
	code := executeAutoTag(t.TempDir(), config, "abc123", "https://example.invalid/api/v4", "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to open repository") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the open failure", stderr.String())
	}
}

func TestExecuteAutoTag_RootCommit_SingleModule_TagsAndPushes(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	assertTagCreated(t, server, "0.1.0", sha)
}

func TestExecuteAutoTag_BumpNone_NoTagPushed(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "chore: bump dependency")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_MultiModule_OnlyChangedModuleTagged(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a1",
		"modB/file.txt": "b1",
	}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a2",
	}, "feat: change module A")

	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
  - name: modB
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, secondSHA, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	assertTagCreated(t, server, "modA-0.1.0", secondSHA)
	assertTagNotCreated(t, server, "modB-0.1.0")
}

func TestExecuteAutoTag_InvalidSHA_CommitResolveFails(t *testing.T) {
	repoDir, _ := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)
	nonExistentSHA := strings.Repeat("f", 40)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, nonExistentSHA, server.URL, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to resolve commit") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the commit resolution failure", stderr.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_RootCommit_MultiModule_AllTaggedRegardlessOfDir(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
  - name: modB
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// Root commits lack parent trees for diffing; all declared modules evaluate as changed.
	assertTagCreated(t, server, "modA-0.1.0", sha)
	assertTagCreated(t, server, "modB-0.1.0", sha)
}

func TestExecuteAutoTag_NonRootCommit_NoModuleChanged_SkipsAll(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a1",
		"modB/file.txt": "b1",
	}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{
		"unrelated/file.txt": "x",
	}, "feat: unrelated change")

	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
  - name: modB
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, secondSHA, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `modA: no changes under "modA". Skipping.`) {
		t.Errorf("executeAutoTag(...) stdout = %q, want it to report modA as skipped", stdout.String())
	}
	if !strings.Contains(stdout.String(), `modB: no changes under "modB". Skipping.`) {
		t.Errorf("executeAutoTag(...) stdout = %q, want it to report modB as skipped", stdout.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_NonRootCommit_BothModulesChanged_BothTagged(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a1",
		"modB/file.txt": "b1",
	}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a2",
		"modB/file.txt": "b2",
	}, "fix: patch both modules")

	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
  - name: modB
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, secondSHA, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	assertTagCreated(t, server, "modA-0.0.1", secondSHA)
	assertTagCreated(t, server, "modB-0.0.1", secondSHA)
}

func TestExecuteAutoTag_VersionProgression_AcrossSequentialCommits(t *testing.T) {
	repoDir, repo := initRepo(t)
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	steps := []struct {
		files   map[string]string
		message string
		wantTag string
	}{
		{map[string]string{"README.md": "v1\n"}, "feat: initial release", "0.1.0"},
		{map[string]string{"README.md": "v2\n"}, "fix: patch release", "0.1.1"},
		{map[string]string{"README.md": "v3\n"}, "feat: minor release", "0.2.0"},
		{map[string]string{"README.md": "v4\n"}, "feat!: breaking release", "1.0.0"},
	}

	for _, step := range steps {
		sha := commitFiles(t, repo, step.files, step.message)

		var stdout, stderr bytes.Buffer
		code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
		if code != 0 {
			t.Fatalf("executeAutoTag(%q) code = %d, want 0, stderr = %q", step.message, code, stderr.String())
		}
		assertTagCreated(t, server, step.wantTag, sha)
	}
}

func TestExecuteAutoTag_BreakingChangeWithScope_MajorBump(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "fix(api)!: breaking fix")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// Exclamation marks following commit types or scopes mandate major version bumps.
	assertTagCreated(t, server, "1.0.0", sha)
}

func TestExecuteAutoTag_APIFailure_ReturnsError(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusBadRequest)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "GitLab API returned status") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the API failure", stderr.String())
	}
}

func TestExecuteAutoTag_LocalMirrorFails_JobStillSucceeds(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	// Pre-creates a directory at the target ref path to force an EISDIR write error.
	// Permission-based restrictions fail when CI containers execute as root; directory
	// collisions guarantee a local ref write failure while remaining invisible to
	// repo.Tags() version baseline calculations.
	collisionPath := filepath.Join(repoDir, ".git", "refs", "tags", "0.1.0")
	if err := os.MkdirAll(collisionPath, 0o755); err != nil {
		t.Fatalf("failed to pre-create a directory blocking the tag ref path: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0 (a local mirror failure must not fail the job), stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Warning:") {
		t.Errorf("executeAutoTag(...) stderr = %q, want a warning about the local mirror failure", stderr.String())
	}
	// Verifies remote API tag creation completes independently of local mirror failures.
	assertTagCreated(t, server, "0.1.0", sha)
}

func TestExecuteAutoTag_TagPrefixOverride_ExplicitEmptyString(t *testing.T) {
	repoDir, repo := initRepo(t)
	sha := commitFiles(t, repo, map[string]string{"modA/file.txt": "a1"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
    tag_prefix: ""
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// Empty tag_prefix overrides suppress module-name prefix derivation.
	assertTagCreated(t, server, "0.1.0", sha)
	assertTagNotCreated(t, server, "modA-0.1.0")
}

func TestExecuteAutoTag_ModulesShareSamePrefix_ChainedBumpWithinSingleRun(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
  - name: ""
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// Identical tag prefixes require local tag mirroring between module iterations to evaluate version bumps sequentially.
	assertTagCreated(t, server, "0.1.0", sha)
	assertTagCreated(t, server, "0.2.0", sha)
}

func TestExecuteAutoTag_APIFailure_StopsProcessingRemainingModules(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
  - name: modB
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusBadRequest)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	// API errors trigger immediate termination, halting subsequent module processing.
	if got := len(server.recorded()); got != 1 {
		t.Errorf("recorded %d API requests, want exactly 1 (fail-fast on the first module)", got)
	}
}

func TestExecuteAutoTag_ProjectIDWithNamespacePath_EscapedInAPICall(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, sha, server.URL, "group/subgroup/project", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}

	requests := server.recorded()
	if len(requests) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(requests))
	}
	if requests[0].escapedPath != "/projects/group%2Fsubgroup%2Fproject/repository/tags" {
		t.Errorf("request escaped path = %q, want every \"/\" in the namespace path percent-encoded", requests[0].escapedPath)
	}
}

func TestExecuteAutoTag_TagAPIUnreachable_ReturnsError(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	unreachableAPI := "http://127.0.0.1:1"

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, unreachableAPI, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to reach GitLab API") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the unreachable API", stderr.String())
	}
}

func TestExecuteAutoTag_MultiModule_ExactAPICallCount(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a1",
		"modB/file.txt": "b1",
	}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a2",
		"modB/file.txt": "b2",
	}, "fix: patch both modules")

	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
  - name: modB
    dir: modB
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, secondSHA, server.URL, "123", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	// Changed modules issue exactly one API request each.
	if got := len(server.recorded()); got != 2 {
		t.Errorf("recorded %d API requests, want exactly 2 (one per changed module)", got)
	}
}

func setupRepo(t *testing.T, files map[string]string, message string) (repoDir, sha string) {
	t.Helper()
	repoDir, repo := initRepo(t)
	sha = commitFiles(t, repo, files, message)
	return repoDir, sha
}

func initRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit(%q) failed: %v", repoDir, err)
	}
	return repoDir, repo
}

func commitFiles(t *testing.T, repo *git.Repository, files map[string]string, message string) string {
	t.Helper()
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}

	root := worktree.Filesystem.Root()
	for relPath, content := range files {
		fullPath := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", fullPath, err)
		}
		if _, err := worktree.Add(relPath); err != nil {
			t.Fatalf("Add(%q) failed: %v", relPath, err)
		}
	}

	sha, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}
	return sha.String()
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "versioning.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write versioning config fixture: %v", err)
	}
	return path
}

// TestMainSubprocess executes main in a child process to isolate os.Exit calls.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_AUTO_TAG") == "1" {
		os.Args = []string{
			"auto-tag",
			"--repo=" + os.Getenv("TEST_REPO_DIR"),
			"--config=" + os.Getenv("TEST_CONFIG_PATH"),
			"--sha=" + os.Getenv("TEST_SHA"),
			"--api-url=" + os.Getenv("TEST_API_URL"),
			"--project-id=123",
		}
		main()
		return
	}

	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		"BE_AUTO_TAG=1",
		"TAG_PUSH_TOKEN=unused",
		"TEST_REPO_DIR="+repoDir,
		"TEST_CONFIG_PATH="+config,
		"TEST_SHA="+sha,
		"TEST_API_URL="+server.URL,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}

	assertTagCreated(t, server, "0.1.0", sha)
}
