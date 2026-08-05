package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newRepoWithCommit initializes a repository containing a single tracked file and checks out
// a detached HEAD state, reproducing the environment of a GitLab CI job where the "HEAD"
// push refspec resolves exclusively against a detached HEAD.
func newRepoWithCommit(t *testing.T) string {
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
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", filePath, err)
	}
	if _, err := worktree.Add("main.go"); err != nil {
		t.Fatalf("Add(main.go) failed: %v", err)
	}
	hash, err := worktree.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: hash}); err != nil {
		t.Fatalf("Checkout(%q) failed: %v", hash, err)
	}
	return dir
}

func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", dir, err)
	}
	return dir
}

func TestRun_MissingArguments(t *testing.T) {
	cases := []struct {
		name      string
		message   string
		branch    string
		remoteURL string
		username  string
		password  string
		wantErr   string
	}{
		{"missing message", "", "feature", "https://example.invalid/repo.git", "gitlab-ci-token", "token", "--message, --branch, --remote-url, and --username are required"},
		{"missing branch", "style: fmt", "", "https://example.invalid/repo.git", "gitlab-ci-token", "token", "--message, --branch, --remote-url, and --username are required"},
		{"missing remote-url", "style: fmt", "feature", "", "gitlab-ci-token", "token", "--message, --branch, --remote-url, and --username are required"},
		{"missing username", "style: fmt", "feature", "https://example.invalid/repo.git", "", "token", "--message, --branch, --remote-url, and --username are required"},
		{"missing password", "style: fmt", "feature", "https://example.invalid/repo.git", "gitlab-ci-token", "", "CI_JOB_TOKEN must be set"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := executeFormatCommitPush(".", c.message, c.branch, c.remoteURL, c.username, c.password, io.Discard, &stderr)
			if code != 1 {
				t.Errorf("executeFormatCommitPush(...) code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), c.wantErr) {
				t.Errorf("executeFormatCommitPush(...) stderr = %q, want it to contain %q", stderr.String(), c.wantErr)
			}
		})
	}
}

func TestRun_NoChanges_SkipsPush(t *testing.T) {
	repoDir := newRepoWithCommit(t)
	// An invalid remote path verifies that no push operation occurs. When DetectWorktreeModifications detects
	// a clean working tree, execution must exit before invoking CommitAndPush.
	unreachableRemote := filepath.Join(t.TempDir(), "does-not-exist.git")

	var stdout, stderr bytes.Buffer
	code := executeFormatCommitPush(repoDir, "style: fmt", "feature-branch", unreachableRemote, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeFormatCommitPush(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No formatting changes detected") {
		t.Errorf("executeFormatCommitPush(...) stdout = %q, want it to mention no formatting changes", stdout.String())
	}
}

func TestRun_RepoOpenFails(t *testing.T) {
	var stderr bytes.Buffer
	code := executeFormatCommitPush(t.TempDir(), "style: fmt", "feature-branch", "https://example.invalid/repo.git", "gitlab-ci-token", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeFormatCommitPush(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to open repository") {
		t.Errorf("executeFormatCommitPush(...) stderr = %q, want it to mention the open failure", stderr.String())
	}
}

func TestRun_BareRepository_HasChangesFails(t *testing.T) {
	bareDir := t.TempDir()
	if _, err := git.PlainInit(bareDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", bareDir, err)
	}

	var stderr bytes.Buffer
	code := executeFormatCommitPush(bareDir, "style: fmt", "feature-branch", "https://example.invalid/repo.git", "gitlab-ci-token", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeFormatCommitPush(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to retrieve working tree") {
		t.Errorf("executeFormatCommitPush(...) stderr = %q, want it to mention the missing working tree", stderr.String())
	}
}

func TestRun_PushFails_WithChangesPresent(t *testing.T) {
	repoDir := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	// Staging a file modification forces execution through CommitAndPush, verifying that
	// remote network or path failures are correctly returned.
	unreachableRemote := filepath.Join(t.TempDir(), "does-not-exist.git")

	var stderr bytes.Buffer
	code := executeFormatCommitPush(repoDir, "style: gofmt", "feature-branch", unreachableRemote, "gitlab-ci-token", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeFormatCommitPush(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to push branch") {
		t.Errorf("executeFormatCommitPush(...) stderr = %q, want it to mention the push failure", stderr.String())
	}
}

func TestRun_UntrackedFileOnly_CommitsAndPushes(t *testing.T) {
	repoDir := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoDir, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := newBareRemote(t)

	// DetectWorktreeModifications evaluates to true for untracked files when tracked files remain unchanged.
	// This executes the untracked file handling path through executeFormatCommitPush().
	var stdout, stderr bytes.Buffer
	code := executeFormatCommitPush(repoDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeFormatCommitPush(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference("refs/heads/feature-branch", true)
	if err != nil {
		t.Fatalf("failed to locate branch \"feature-branch\" within remote repository: %v", err)
	}
	commit, err := remote.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("failed to resolve pushed commit: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("failed to resolve pushed commit tree: %v", err)
	}
	if _, err := tree.File("new.go"); err != nil {
		t.Errorf("pushed commit tree does not contain the untracked file \"new.go\": %v", err)
	}
}

func TestRun_Success_ReportsPushedMessage(t *testing.T) {
	repoDir := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeFormatCommitPush(repoDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeFormatCommitPush(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	wantMessage := `Pushed formatting commit "style: gofmt" to feature-branch.`
	if !strings.Contains(stdout.String(), wantMessage) {
		t.Errorf("executeFormatCommitPush(...) stdout = %q, want it to contain %q", stdout.String(), wantMessage)
	}
}

func TestRun_Success(t *testing.T) {
	repoDir := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeFormatCommitPush(repoDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeFormatCommitPush(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference("refs/heads/feature-branch", true)
	if err != nil {
		t.Fatalf("failed to locate branch \"feature-branch\" within remote repository: %v", err)
	}
	commit, err := remote.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("failed to resolve pushed commit: %v", err)
	}
	if commit.Message != "style: gofmt" {
		t.Errorf("pushed commit message = %q, want %q", commit.Message, "style: gofmt")
	}
}

// TestMainSubprocess executes main() within a child process. Re-executing the compiled test
// binary prevents os.Exit inside main() from terminating the parent test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_FMT_COMMIT_PUSHER") == "1" {
		os.Args = []string{
			"fmt-commit-pusher",
			"--repo=" + os.Getenv("TEST_REPO_DIR"),
			"--message=style: gofmt",
			"--branch=feature-branch",
			"--remote-url=" + os.Getenv("TEST_REMOTE_URL"),
		}
		main()
		return
	}

	repoDir := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := newBareRemote(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		"BE_FMT_COMMIT_PUSHER=1",
		"CI_JOB_TOKEN=unused",
		"TEST_REPO_DIR="+repoDir,
		"TEST_REMOTE_URL="+remoteDir,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	if _, err := remote.Reference("refs/heads/feature-branch", true); err != nil {
		t.Fatalf("failed to locate branch \"feature-branch\" within remote repository: %v", err)
	}
}

// TestMainSubprocess_MissingArguments verifies that main() forwards non-zero exit codes from
// executeFormatCommitPush() to os.Exit. Direct calls to executeFormatCommitPush() in unit tests cannot validate this process exit behavior.
func TestMainSubprocess_MissingArguments(t *testing.T) {
	if os.Getenv("BE_FMT_COMMIT_PUSHER_MISSING_ARGS") == "1" {
		os.Args = []string{
			"fmt-commit-pusher",
			"--repo=" + os.Getenv("TEST_REPO_DIR"),
			"--branch=feature-branch",
			"--remote-url=https://example.invalid/repo.git",
		}
		main()
		return
	}

	repoDir := newRepoWithCommit(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess_MissingArguments")
	cmd.Env = append(os.Environ(),
		"BE_FMT_COMMIT_PUSHER_MISSING_ARGS=1",
		"CI_JOB_TOKEN=unused",
		"TEST_REPO_DIR="+repoDir,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("subprocess exited successfully; want a nonzero exit code for a missing --message flag")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("subprocess failed for a reason other than a nonzero exit: %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("subprocess exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "--message, --branch, --remote-url, and --username are required") {
		t.Errorf("subprocess stderr = %q, want it to mention the missing required flags", stderr.String())
	}
}

// TestMainSubprocess_BranchFromEnvVar verifies that flag parsing falls back to the
// CI_MERGE_REQUEST_SOURCE_BRANCH_NAME environment variable when --branch is omitted.
func TestMainSubprocess_BranchFromEnvVar(t *testing.T) {
	if os.Getenv("BE_FMT_COMMIT_PUSHER_BRANCH_ENV") == "1" {
		os.Args = []string{
			"fmt-commit-pusher",
			"--repo=" + os.Getenv("TEST_REPO_DIR"),
			"--message=style: gofmt",
			"--remote-url=" + os.Getenv("TEST_REMOTE_URL"),
		}
		main()
		return
	}

	repoDir := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := newBareRemote(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess_BranchFromEnvVar")
	cmd.Env = append(os.Environ(),
		"BE_FMT_COMMIT_PUSHER_BRANCH_ENV=1",
		"CI_JOB_TOKEN=unused",
		"CI_MERGE_REQUEST_SOURCE_BRANCH_NAME=release/from-env",
		"TEST_REPO_DIR="+repoDir,
		"TEST_REMOTE_URL="+remoteDir,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	if _, err := remote.Reference("refs/heads/release/from-env", true); err != nil {
		t.Fatalf("failed to locate branch \"release/from-env\" within remote repository: %v", err)
	}
}
