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
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// tagRequest records parameters captured from a single POST /projects/:id/repository/tags API call.
type tagRequest struct {
	projectPath string
	escapedPath string
	tagName     string
	ref         string
	method      string
	token       string
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
			method:      r.Method,
			token:       r.Header.Get("PRIVATE-TOKEN"),
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

func TestExecuteAutoTag_MissingConfig_AppliesWholeRepositoryDefaults(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	missingConfig := filepath.Join(t.TempDir(), "does-not-exist.yml")
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, missingConfig, sha, server.URL, "123", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Applying whole-repository defaults") {
		t.Errorf("executeAutoTag(...) stdout = %q, want it to mention the applied defaults", stdout.String())
	}
	assertTagCreated(t, server, "0.1.0", sha)
}

func TestExecuteAutoTag_MalformedConfig_Fails(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "chore: init")
	malformedConfig := writeConfig(t, `
modules:
  - name: "root"
    dir: ""
`)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, malformedConfig, sha, "https://example.invalid/api/v4", "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "has an empty dir") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the empty dir validation failure", stderr.String())
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

// TestExecuteAutoTag_CommitMessageClassification exercises single-commit, single-module runs
// where the only varying input is the commit subject. The bump-type mapping itself
// (feat/fix/perf/chore/exclamation mark) is unit tested in semver.TestDetermineBump; each case
// here instead asserts that executeAutoTag wires that classification through to the correct
// tag name, tag suppression, or stdout message on a real repository and mocked GitLab API.
func TestExecuteAutoTag_CommitMessageClassification(t *testing.T) {
	cases := []struct {
		name          string
		message       string
		wantTag       string // empty means no tag should be created
		wantTagAbsent string // optional: a specific tag name that must not be created
		wantStdoutHas string // optional substring
	}{
		{
			name:    "feat on a root commit triggers a minor bump",
			message: "feat: initial release",
			wantTag: "0.1.0",
		},
		{
			name:    "fix with scope and exclamation mark triggers a major bump",
			message: "fix(api)!: breaking fix",
			wantTag: "1.0.0",
		},
		{
			name:    "perf triggers a patch bump",
			message: "perf: reduce allocations",
			wantTag: "0.0.1",
		},
		{
			name:          "chore does not trigger a release",
			message:       "chore: bump dependency",
			wantStdoutHas: "<repository>: commit subject does not trigger a release. Skipping.",
		},
		{
			name:          "BREAKING CHANGE text confined to the commit body does not force a major bump",
			message:       "feat: add endpoint\n\nBREAKING CHANGE: remove v1",
			wantTag:       "0.1.0",
			wantTagAbsent: "1.0.0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, c.message)
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

			if c.wantTag != "" {
				assertTagCreated(t, server, c.wantTag, sha)
			} else {
				assertNoTagsCreated(t, server)
			}
			if c.wantTagAbsent != "" {
				assertTagNotCreated(t, server, c.wantTagAbsent)
			}
			if c.wantStdoutHas != "" && !strings.Contains(stdout.String(), c.wantStdoutHas) {
				t.Errorf("executeAutoTag(...) stdout = %q, want it to contain %q", stdout.String(), c.wantStdoutHas)
			}
		})
	}
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

func TestExecuteAutoTag_ConfigPathIsDirectory_DoesNotApplyDefaults(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	server := newTagAPIServer(t, http.StatusCreated)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, t.TempDir(), sha, server.URL, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1 (a directory is not a missing config file)", code)
	}
	if strings.Contains(stderr.String(), "Applying whole-repository defaults") {
		t.Errorf("executeAutoTag(...) stderr = %q, must not apply defaults when the path exists as a directory", stderr.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_InvalidYAML_DoesNotApplyDefaults(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	malformedConfig := writeConfig(t, "modules: [unclosed")
	server := newTagAPIServer(t, http.StatusCreated)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, malformedConfig, sha, server.URL, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to parse versioning config") {
		t.Errorf("executeAutoTag(...) stderr = %q, want a YAML parse error", stderr.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_EmptyModules_Fails(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	emptyModules := writeConfig(t, "modules: []")
	server := newTagAPIServer(t, http.StatusCreated)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, emptyModules, sha, server.URL, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "declares no modules") {
		t.Errorf("executeAutoTag(...) stderr = %q, want the empty-modules validation failure", stderr.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_MissingConfig_NonRootCommitTagsWholeRepository(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{"README.md": "v1\n"}, "chore: init")
	sha := commitFiles(t, repo, map[string]string{"README.md": "v2\n"}, "feat: follow-up")
	missingConfig := filepath.Join(t.TempDir(), "does-not-exist.yml")
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, missingConfig, sha, server.URL, "123", "token-value", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), missingConfig) {
		t.Errorf("executeAutoTag(...) stdout = %q, want the missing config path %q", stdout.String(), missingConfig)
	}
	if !strings.Contains(stdout.String(), "<repository>: bumping") {
		t.Errorf("executeAutoTag(...) stdout = %q, want the empty-name module label <repository>", stdout.String())
	}
	assertTagCreated(t, server, "0.1.0", sha)
}

func TestExecuteAutoTag_SendsPrivateTokenAndPOST(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)
	const token = "glpat-auto-tag-token"

	code := executeAutoTag(repoDir, config, sha, server.URL, "123", token, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	requests := server.recorded()
	if len(requests) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(requests))
	}
	if requests[0].method != http.MethodPost {
		t.Errorf("request method = %q, want POST", requests[0].method)
	}
	if requests[0].token != token {
		t.Errorf("PRIVATE-TOKEN = %q, want %q", requests[0].token, token)
	}
}

func TestExecuteAutoTag_MirrorsTagLocallyOnSuccess(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	assertLocalTag(t, repoDir, "0.1.0", sha)
}

func TestExecuteAutoTag_CustomTagPrefix(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"svc/file.txt": "1"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: svc
    dir: svc
    tag_prefix: "svc/v"
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	assertTagCreated(t, server, "svc/v0.1.0", sha)
	assertTagNotCreated(t, server, "svc-0.1.0")
}

func TestExecuteAutoTag_SiblingDirectoryPrefixNotMatched(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt":       "a1",
		"modA-extra/file.txt": "e1",
	}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{
		"modA-extra/file.txt": "e2",
	}, "feat: change sibling directory")

	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, secondSHA, server.URL, "123", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_FirstModuleSkippedSecondTagged(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a1",
		"modB/file.txt": "b1",
	}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{
		"modB/file.txt": "b2",
	}, "fix: change module B")

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
	assertTagNotCreated(t, server, "modA-0.0.1")
	assertTagCreated(t, server, "modB-0.0.1", secondSHA)
}

func TestExecuteAutoTag_FileDeletionInModule_Tags(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{
		"modA/file.txt": "a1",
	}, "chore: init")

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if _, err := wt.Remove("modA/file.txt"); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}
	sha, err := wt.Commit("fix: remove module file", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, sha.String(), server.URL, "123", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	assertTagCreated(t, server, "modA-0.0.1", sha.String())
}

func TestExecuteAutoTag_ModuleDirTrailingSlash(t *testing.T) {
	repoDir, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{"modA/file.txt": "a1"}, "chore: init")
	secondSHA := commitFiles(t, repo, map[string]string{"modA/file.txt": "a2"}, "feat: change module A")

	config := writeConfig(t, `
modules:
  - name: modA
    dir: "modA/"
`)
	server := newTagAPIServer(t, http.StatusCreated)

	code := executeAutoTag(repoDir, config, secondSHA, server.URL, "123", "unused", io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	assertTagCreated(t, server, "modA-0.1.0", secondSHA)
}

func TestExecuteAutoTag_APIStatusOK_NotCreated_Fails(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusOK)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1 (tag creation requires HTTP 201)", code)
	}
	if !strings.Contains(stderr.String(), "GitLab API returned status 200") {
		t.Errorf("executeAutoTag(...) stderr = %q, want status 200 in the error", stderr.String())
	}
}

func TestExecuteAutoTag_WhitespaceSHA_FailsBeforeAPI(t *testing.T) {
	repoDir, _ := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, "   ", server.URL, "123", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to resolve commit") {
		t.Errorf("executeAutoTag(...) stderr = %q, want a commit resolution failure", stderr.String())
	}
	assertNoTagsCreated(t, server)
}

func TestExecuteAutoTag_NamedModuleStdoutUsesName(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"modA/file.txt": "a1"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: modA
    dir: modA
`)
	server := newTagAPIServer(t, http.StatusCreated)

	var stdout bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, server.URL, "123", "unused", &stdout, io.Discard)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "modA: bumping") {
		t.Errorf("executeAutoTag(...) stdout = %q, want the module name as the log label", stdout.String())
	}
	if strings.Contains(stdout.String(), "<repository>") {
		t.Errorf("executeAutoTag(...) stdout = %q, named modules must not fall back to <repository>", stdout.String())
	}
}

func TestResolveCommitSubject_UsesFirstLineOnly(t *testing.T) {
	_, repo := initRepo(t)
	sha := commitFiles(t, repo, map[string]string{"README.md": "test\n"}, "feat: subject\n\nfix: body must be ignored")

	got, err := resolveCommitSubject(repo, sha)
	if err != nil {
		t.Fatalf("resolveCommitSubject(...) error = %v", err)
	}
	if got != "feat: subject" {
		t.Errorf("resolveCommitSubject(...) = %q, want the first line only", got)
	}
}

func TestResolveCommitSubject_InvalidSHA(t *testing.T) {
	_, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{"README.md": "test\n"}, "feat: initial")

	_, err := resolveCommitSubject(repo, strings.Repeat("0", 40))
	if err == nil {
		t.Fatal("resolveCommitSubject(...) succeeded for a missing SHA; want an error")
	}
	if !strings.Contains(err.Error(), "failed to resolve commit") {
		t.Errorf("resolveCommitSubject(...) error = %q, want a wrapped resolve failure", err.Error())
	}
}

func TestResolveFirstParentSHA_RootAndLinearHistory(t *testing.T) {
	_, repo := initRepo(t)
	root := commitFiles(t, repo, map[string]string{"a.txt": "1"}, "root")
	child := commitFiles(t, repo, map[string]string{"a.txt": "2"}, "child")

	parent, hasParent, err := resolveFirstParentSHA(repo, root)
	if err != nil {
		t.Fatalf("resolveFirstParentSHA(root) error = %v", err)
	}
	if hasParent || parent != "" {
		t.Errorf("resolveFirstParentSHA(root) = (%q, %v), want (\"\", false)", parent, hasParent)
	}

	parent, hasParent, err = resolveFirstParentSHA(repo, child)
	if err != nil {
		t.Fatalf("resolveFirstParentSHA(child) error = %v", err)
	}
	if !hasParent || parent != root {
		t.Errorf("resolveFirstParentSHA(child) = (%q, %v), want (%q, true)", parent, hasParent, root)
	}
}

func TestResolveFirstParentSHA_MergeCommitUsesFirstParent(t *testing.T) {
	_, repo := initRepo(t)
	root := commitFiles(t, repo, map[string]string{"a.txt": "root"}, "root")
	firstParent := commitFiles(t, repo, map[string]string{"a.txt": "main"}, "mainline")

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	sidePath := filepath.Join(wt.Filesystem.Root(), "side.txt")
	if err := os.WriteFile(sidePath, []byte("side"), 0o644); err != nil {
		t.Fatalf("WriteFile(side.txt) failed: %v", err)
	}
	if _, err := wt.Add("side.txt"); err != nil {
		t.Fatalf("Add(side.txt) failed: %v", err)
	}
	secondParent, err := wt.Commit("side branch", &git.CommitOptions{
		Author:  &object.Signature{Name: "test", Email: "test@example.com"},
		Parents: []plumbing.Hash{plumbing.NewHash(root)},
	})
	if err != nil {
		t.Fatalf("side commit failed: %v", err)
	}

	merge, err := wt.Commit("merge", &git.CommitOptions{
		Author:  &object.Signature{Name: "test", Email: "test@example.com"},
		Parents: []plumbing.Hash{plumbing.NewHash(firstParent), secondParent},
	})
	if err != nil {
		t.Fatalf("merge commit failed: %v", err)
	}

	got, hasParent, err := resolveFirstParentSHA(repo, merge.String())
	if err != nil {
		t.Fatalf("resolveFirstParentSHA(merge) error = %v", err)
	}
	if !hasParent {
		t.Fatal("resolveFirstParentSHA(merge) hasParent = false, want true")
	}
	if got != firstParent {
		t.Errorf("resolveFirstParentSHA(merge) = %q, want first parent %q (second parent is %q)", got, firstParent, secondParent.String())
	}
}

func TestResolveFirstParentSHA_InvalidSHA(t *testing.T) {
	_, repo := initRepo(t)
	commitFiles(t, repo, map[string]string{"a.txt": "1"}, "root")

	_, _, err := resolveFirstParentSHA(repo, strings.Repeat("0", 40))
	if err == nil {
		t.Fatal("resolveFirstParentSHA(...) succeeded for a missing SHA; want an error")
	}
}

func TestMainSubprocess_MissingToken_ExitsOne(t *testing.T) {
	if os.Getenv("BE_AUTO_TAG_MISSING_TOKEN") == "1" {
		os.Args = []string{
			"auto-tag",
			"--sha=abc",
			"--api-url=http://example.invalid",
			"--project-id=123",
		}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_MissingToken_ExitsOne$")
	cmd.Env = append(envWithout(os.Environ(), "TAG_PUSH_TOKEN"), "BE_AUTO_TAG_MISSING_TOKEN=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("subprocess exited 0; want exit 1 when TAG_PUSH_TOKEN is unset")
	}
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("subprocess error = %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "TAG_PUSH_TOKEN must be set") {
		t.Errorf("subprocess stderr = %q, want the missing-token error", stderr.String())
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

func assertLocalTag(t *testing.T, repoDir, tagName, wantSHA string) {
	t.Helper()
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatalf("PlainOpen(%q) failed: %v", repoDir, err)
	}
	ref, err := repo.Tag(tagName)
	if err != nil {
		t.Fatalf("local tag %q missing: %v", tagName, err)
	}
	if got := ref.Hash().String(); got != wantSHA {
		t.Errorf("local tag %q hash = %q, want %q", tagName, got, wantSHA)
	}
}

func envWithout(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
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

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess$")
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
