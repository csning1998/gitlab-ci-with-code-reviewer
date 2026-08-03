package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newSourceRepoWithCommit initializes a Git repository within a temporary directory allocated via t.TempDir(),
// constructs an initial commit, and returns the directory path alongside the corresponding commit SHA.
func newSourceRepoWithCommit(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit(%q) failed: %v", dir, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	filePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(filePath, []byte("test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", filePath, err)
	}
	if _, err := worktree.Add("README.md"); err != nil {
		t.Fatalf("Add(README.md) failed: %v", err)
	}
	sha, err := worktree.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	return dir, sha.String()
}

func TestRun_MissingArguments(t *testing.T) {
	cases := []struct {
		name      string
		repo      string
		remoteURL string
		tag       string
		sha       string
		username  string
		password  string
		wantErr   string
	}{
		{"missing remote-url", ".", "", "1.4.4", "abc", "gitlab-ci-token", "token", "--remote-url, --tag, --sha, and --username are required"},
		{"missing tag", ".", "https://example.invalid/repo.git", "", "abc", "gitlab-ci-token", "token", "--remote-url, --tag, --sha, and --username are required"},
		{"missing sha", ".", "https://example.invalid/repo.git", "1.4.4", "", "gitlab-ci-token", "token", "--remote-url, --tag, --sha, and --username are required"},
		{"missing username", ".", "https://example.invalid/repo.git", "1.4.4", "abc", "", "token", "--remote-url, --tag, --sha, and --username are required"},
		{"missing password", ".", "https://example.invalid/repo.git", "1.4.4", "abc", "gitlab-ci-token", "", "TAG_PUSH_TOKEN must be set"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run(c.repo, c.remoteURL, c.tag, c.sha, c.username, c.password, &stderr)
			if code != 1 {
				t.Errorf("run(...) code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), c.wantErr) {
				t.Errorf("run(...) stderr = %q, want it to contain %q", stderr.String(), c.wantErr)
			}
		})
	}
}

func TestRun_Success(t *testing.T) {
	sourceDir, sha := newSourceRepoWithCommit(t)

	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	var stderr bytes.Buffer
	code := run(sourceDir, remoteDir, "1.4.4", sha, "gitlab-ci-token", "unused", &stderr)
	if code != 0 {
		t.Fatalf("run(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
}

func TestRun_CreateTagFails(t *testing.T) {
	notARepo := t.TempDir()

	var stderr bytes.Buffer
	code := run(notARepo, "https://example.invalid/repo.git", "1.4.4", "abc", "gitlab-ci-token", "unused", &stderr)
	if code != 1 {
		t.Errorf("run(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to open repository") {
		t.Errorf("run(...) stderr = %q, want it to mention the open failure", stderr.String())
	}
}

func TestRun_PushTagFails(t *testing.T) {
	sourceDir, sha := newSourceRepoWithCommit(t)
	unreachableRemote := filepath.Join(t.TempDir(), "does-not-exist.git")

	var stderr bytes.Buffer
	code := run(sourceDir, unreachableRemote, "1.4.4", sha, "gitlab-ci-token", "unused", &stderr)
	if code != 1 {
		t.Errorf("run(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to push tag") {
		t.Errorf("run(...) stderr = %q, want it to mention the push failure", stderr.String())
	}
}

// TestMainSubprocess exercises main itself by re-executing this test binary as a child
// process, since main calls os.Exit and would otherwise terminate the parent test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_GIT_TAG_PUSHER") == "1" {
		os.Args = []string{
			"git-tag-pusher",
			"--repo=" + os.Getenv("TEST_REPO_DIR"),
			"--remote-url=" + os.Getenv("TEST_REMOTE_URL"),
			"--tag=1.4.4",
			"--sha=" + os.Getenv("TEST_SHA"),
			"--username=gitlab-ci-token",
		}
		main()
		return
	}

	sourceDir, sha := newSourceRepoWithCommit(t)
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		"BE_GIT_TAG_PUSHER=1",
		"TAG_PUSH_TOKEN=unused",
		"TEST_REPO_DIR="+sourceDir,
		"TEST_REMOTE_URL="+remoteDir,
		"TEST_SHA="+sha,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}
}
