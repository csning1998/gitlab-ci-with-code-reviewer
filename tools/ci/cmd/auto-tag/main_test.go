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
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestExecuteAutoTag_MissingArguments(t *testing.T) {
	cases := []struct {
		name      string
		sha       string
		remoteURL string
		username  string
		password  string
		wantErr   string
	}{
		{"missing sha", "", "https://example.invalid/repo.git", "gitlab-ci-token", "token", "--sha, --remote-url, and --username are required"},
		{"missing remote-url", "abc", "", "gitlab-ci-token", "token", "--sha, --remote-url, and --username are required"},
		{"missing username", "abc", "https://example.invalid/repo.git", "", "token", "--sha, --remote-url, and --username are required"},
		{"missing password", "abc", "https://example.invalid/repo.git", "gitlab-ci-token", "", "TAG_PUSH_TOKEN must be set"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := executeAutoTag(".", ".gitlab/versioning.yml", c.sha, c.remoteURL, c.username, c.password, io.Discard, &stderr)
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
	code := executeAutoTag(repoDir, missingConfig, sha, "https://example.invalid/repo.git", "gitlab-ci-token", "unused", io.Discard, &stderr)
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
	code := executeAutoTag(t.TempDir(), config, "abc123", "https://example.invalid/repo.git", "gitlab-ci-token", "unused", io.Discard, &stderr)
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	assertRemoteTag(t, remoteDir, "0.1.0", sha)
}

func TestExecuteAutoTag_BumpNone_NoTagPushed(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "chore: bump dependency")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	assertNoTags(t, remoteDir)
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, secondSHA, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	assertRemoteTag(t, remoteDir, "modA-0.1.0", secondSHA)
	assertTagAbsent(t, remoteDir, "modB-0.1.0")
}

func TestExecuteAutoTag_InvalidSHA_CommitResolveFails(t *testing.T) {
	repoDir, _ := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	remoteDir := newBareRemote(t)
	nonExistentSHA := strings.Repeat("f", 40)

	var stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, nonExistentSHA, remoteDir, "gitlab-ci-token", "unused", io.Discard, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to resolve commit") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the commit resolution failure", stderr.String())
	}
	assertNoTags(t, remoteDir)
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// A root commit has no parent tree to diff against, so every declared module is treated
	// as changed even though neither modA nor modB exists within the committed tree.
	assertRemoteTag(t, remoteDir, "modA-0.1.0", sha)
	assertRemoteTag(t, remoteDir, "modB-0.1.0", sha)
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, secondSHA, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `modA: no changes under "modA". Skipping.`) {
		t.Errorf("executeAutoTag(...) stdout = %q, want it to report modA as skipped", stdout.String())
	}
	if !strings.Contains(stdout.String(), `modB: no changes under "modB". Skipping.`) {
		t.Errorf("executeAutoTag(...) stdout = %q, want it to report modB as skipped", stdout.String())
	}
	assertNoTags(t, remoteDir)
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, secondSHA, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	assertRemoteTag(t, remoteDir, "modA-0.0.1", secondSHA)
	assertRemoteTag(t, remoteDir, "modB-0.0.1", secondSHA)
}

func TestExecuteAutoTag_VersionProgression_AcrossSequentialCommits(t *testing.T) {
	repoDir, repo := initRepo(t)
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	remoteDir := newBareRemote(t)

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
		code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
		if code != 0 {
			t.Fatalf("executeAutoTag(%q) code = %d, want 0, stderr = %q", step.message, code, stderr.String())
		}
		assertRemoteTag(t, remoteDir, step.wantTag, sha)
	}
}

func TestExecuteAutoTag_BreakingChangeWithScope_MajorBump(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "fix(api)!: breaking fix")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// A "!" following the type or scope forces a major bump regardless of the "fix" type,
	// which would otherwise only warrant a patch release.
	assertRemoteTag(t, remoteDir, "1.0.0", sha)
}

func TestExecuteAutoTag_PushFails_UnreachableRemote_LocalTagPersists(t *testing.T) {
	repoDir, sha := setupRepo(t, map[string]string{"README.md": "test\n"}, "feat: initial release")
	config := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	unreachableRemote := filepath.Join(t.TempDir(), "does-not-exist.git")

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, unreachableRemote, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 1 {
		t.Errorf("executeAutoTag(...) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "failed to push tag") {
		t.Errorf("executeAutoTag(...) stderr = %q, want it to mention the push failure", stderr.String())
	}

	// CreateTag runs, and succeeds, before PushTag is attempted, so a push failure leaves
	// the tag committed to the local repository even though the remote never received it.
	assertRemoteTag(t, repoDir, "0.1.0", sha)
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// An explicit empty tag_prefix suppresses the "modA-" default that would otherwise be
	// derived from the module name.
	assertRemoteTag(t, remoteDir, "0.1.0", sha)
	assertTagAbsent(t, remoteDir, "modA-0.1.0")
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
	remoteDir := newBareRemote(t)

	var stdout, stderr bytes.Buffer
	code := executeAutoTag(repoDir, config, sha, remoteDir, "gitlab-ci-token", "unused", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executeAutoTag(...) code = %d, want 0, stderr = %q", code, stderr.String())
	}

	// Both modules resolve to the same empty tag prefix. LatestTag re-reads repository
	// tags on each iteration, so the second module detects the first tag and bumps from it
	// within the same executeAutoTag() call.
	assertRemoteTag(t, remoteDir, "0.1.0", sha)
	assertRemoteTag(t, remoteDir, "0.2.0", sha)
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

func newBareRemote(t *testing.T) string {
	t.Helper()
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}
	return remoteDir
}

func assertRemoteTag(t *testing.T, remoteDir, tagName, wantSHA string) {
	t.Helper()
	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference(plumbing.NewTagReferenceName(tagName), true)
	if err != nil {
		t.Fatalf("failed to locate reference for tag %q within remote repository: %v", tagName, err)
	}
	if ref.Hash().String() != wantSHA {
		t.Errorf("tag %q resolves to commit hash %q; expected %q", tagName, ref.Hash().String(), wantSHA)
	}
}

func assertTagAbsent(t *testing.T, remoteDir, tagName string) {
	t.Helper()
	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	if _, err := remote.Reference(plumbing.NewTagReferenceName(tagName), true); err == nil {
		t.Errorf("tag %q unexpectedly exists within remote repository", tagName)
	}
}

func assertNoTags(t *testing.T, remoteDir string) {
	t.Helper()
	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	iter, err := remote.Tags()
	if err != nil {
		t.Fatalf("Tags() failed: %v", err)
	}
	defer iter.Close()
	count := 0
	_ = iter.ForEach(func(*plumbing.Reference) error {
		count++
		return nil
	})
	if count != 0 {
		t.Errorf("remote repository has %d tags, want 0", count)
	}
}

// TestMainSubprocess exercises main itself by re-executing this test binary as a child process,
// since main calls os.Exit and would otherwise terminate the parent test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_AUTO_TAG") == "1" {
		os.Args = []string{
			"auto-tag",
			"--repo=" + os.Getenv("TEST_REPO_DIR"),
			"--config=" + os.Getenv("TEST_CONFIG_PATH"),
			"--sha=" + os.Getenv("TEST_SHA"),
			"--remote-url=" + os.Getenv("TEST_REMOTE_URL"),
			"--username=gitlab-ci-token",
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
	remoteDir := newBareRemote(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(),
		"BE_AUTO_TAG=1",
		"TAG_PUSH_TOKEN=unused",
		"TEST_REPO_DIR="+repoDir,
		"TEST_CONFIG_PATH="+config,
		"TEST_SHA="+sha,
		"TEST_REMOTE_URL="+remoteDir,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v, stderr = %q", err, stderr.String())
	}

	assertRemoteTag(t, remoteDir, "0.1.0", sha)
}
