package versiontag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestLoadConfig_Success(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: ""
    dir: "."
  - name: reviewer
    dir: tools/ci
    tag_prefix: "reviewer-"
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(%q) returned an unexpected error: %v", path, err)
	}
	if len(cfg.Modules) != 2 {
		t.Fatalf("LoadConfig(%q) returned %d modules, want 2", path, len(cfg.Modules))
	}
	if cfg.Modules[1].Prefix() != "reviewer-" {
		t.Errorf("Modules[1].Prefix() = %q, want %q", cfg.Modules[1].Prefix(), "reviewer-")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.yml")); err == nil {
		t.Error("LoadConfig(...) on a missing file succeeded unexpectedly; expected an error")
	}
}

func TestLoadConfig_NoModules(t *testing.T) {
	path := writeConfig(t, "modules: []\n")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) with zero modules succeeded unexpectedly; expected an error")
	}
}

func TestLoadConfig_ModuleMissingDir(t *testing.T) {
	path := writeConfig(t, "modules:\n  - name: reviewer\n")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) with a module missing dir succeeded unexpectedly; expected an error")
	}
}

func TestModule_Prefix(t *testing.T) {
	explicitEmpty := ""
	explicitCustom := "gopls/v"

	cases := []struct {
		name   string
		module Module
		want   string
	}{
		{"empty name defaults to no prefix", Module{Name: "", Dir: "."}, ""},
		{"non-empty name defaults to hyphenated prefix", Module{Name: "reviewer", Dir: "tools/ci"}, "reviewer-"},
		{"explicit override wins over name default", Module{Name: "gopls", Dir: "gopls", TagPrefix: &explicitCustom}, "gopls/v"},
		{"explicit empty override suppresses the hyphenated default", Module{Name: "reviewer", Dir: "tools/ci", TagPrefix: &explicitEmpty}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.module.Prefix(); got != c.want {
				t.Errorf("Prefix() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestLatestTag_NoMatch(t *testing.T) {
	repo := newEmptyRepo(t)

	tag, version, err := LatestTag(repo, "reviewer-")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "reviewer-0.0.0" || version != "0.0.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"reviewer-0.0.0\", \"0.0.0\")", tag, version)
	}
}

func TestLatestTag_PicksMaxVersionAmongMatchingPrefix(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	for _, tag := range []string{"reviewer-1.0.0", "reviewer-1.2.0", "reviewer-1.1.5", "reviewer-v1.3.0", "other-9.9.9", "reviewer-not-a-version"} {
		mustTag(t, repo, tag, sha)
	}

	tag, version, err := LatestTag(repo, "reviewer-")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "reviewer-v1.3.0" || version != "1.3.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"reviewer-v1.3.0\", \"1.3.0\")", tag, version)
	}
}

func TestDirChanged(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "a1",
		"dirB/file.txt": "b1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/file.txt": "a2",
		"dirB/file.txt": "b1",
	})

	changedA, err := DirChanged(repo, firstSHA, secondSHA, "dirA")
	if err != nil {
		t.Fatalf("DirChanged(..., \"dirA\") returned an unexpected error: %v", err)
	}
	if !changedA {
		t.Error("DirChanged(..., \"dirA\") = false, want true")
	}

	changedB, err := DirChanged(repo, firstSHA, secondSHA, "dirB")
	if err != nil {
		t.Fatalf("DirChanged(..., \"dirB\") returned an unexpected error: %v", err)
	}
	if changedB {
		t.Error("DirChanged(..., \"dirB\") = true, want false")
	}

	changedRoot, err := DirChanged(repo, firstSHA, secondSHA, ".")
	if err != nil {
		t.Fatalf("DirChanged(..., \".\") returned an unexpected error: %v", err)
	}
	if !changedRoot {
		t.Error("DirChanged(..., \".\") = false, want true")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "modules: [unclosed_bracket")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) with invalid YAML succeeded unexpectedly; expected an error")
	}
}

func TestLatestTag_EmptyPrefix(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	for _, tag := range []string{"v1.0.0", "v1.2.0", "v1.1.5", "not-semver"} {
		mustTag(t, repo, tag, sha)
	}

	tag, version, err := LatestTag(repo, "")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "v1.2.0" || version != "1.2.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"v1.2.0\", \"1.2.0\")", tag, version)
	}
}

func TestLatestTag_SemverComparison(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	tags := []string{
		"reviewer-0.9.9",
		"reviewer-0.10.0",
		"reviewer-1.2.9",
		"reviewer-1.2.10",
		"reviewer-1.9.9",
		"reviewer-2.0.0",
	}
	for _, tag := range tags {
		mustTag(t, repo, tag, sha)
	}

	tag, version, err := LatestTag(repo, "reviewer-")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "reviewer-2.0.0" || version != "2.0.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"reviewer-2.0.0\", \"2.0.0\")", tag, version)
	}
}

func TestDirChanged_InvalidSHA(t *testing.T) {
	repo, validSHA := newRepoWithCommit(t)
	invalidSHA := "0000000000000000000000000000000000000000"

	if _, err := DirChanged(repo, invalidSHA, validSHA, "."); err == nil {
		t.Error("DirChanged(...) with invalid parentSHA succeeded unexpectedly; expected an error")
	}
	if _, err := DirChanged(repo, validSHA, invalidSHA, "."); err == nil {
		t.Error("DirChanged(...) with invalid target SHA succeeded unexpectedly; expected an error")
	}
}

func TestDirChanged_SubdirectoryPath(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/sub/file.txt":   "1",
		"dirA-other/file.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/sub/file.txt":   "2",
		"dirA-other/file.txt": "1",
	})

	changedSub, err := DirChanged(repo, firstSHA, secondSHA, "dirA/sub")
	if err != nil {
		t.Fatalf("DirChanged(..., \"dirA/sub\") returned an unexpected error: %v", err)
	}
	if !changedSub {
		t.Error("DirChanged(..., \"dirA/sub\") = false, want true")
	}

	changedOther, err := DirChanged(repo, firstSHA, secondSHA, "dirA-other")
	if err != nil {
		t.Fatalf("DirChanged(..., \"dirA-other\") returned an unexpected error: %v", err)
	}
	if changedOther {
		t.Error("DirChanged(..., \"dirA-other\") = true, want false")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	path := writeConfig(t, "")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) with empty file content succeeded unexpectedly; expected an error")
	}
}

func TestLoadConfig_NilModules(t *testing.T) {
	path := writeConfig(t, "other_key: value")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) missing modules key succeeded unexpectedly; expected an error")
	}
}

func TestLatestTag_UppercaseVPrefix(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	mustTag(t, repo, "reviewer-V2.1.0", sha)

	tag, version, err := LatestTag(repo, "reviewer-")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "reviewer-V2.1.0" || version != "2.1.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"reviewer-V2.1.0\", \"2.1.0\")", tag, version)
	}
}

func TestLatestTag_MalformedSemverIgnored(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	for _, tag := range []string{"reviewer-1.2", "reviewer-1.2.3.4", "reviewer-invalid", "reviewer-1.0.0"} {
		mustTag(t, repo, tag, sha)
	}

	tag, version, err := LatestTag(repo, "reviewer-")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "reviewer-1.0.0" || version != "1.0.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"reviewer-1.0.0\", \"1.0.0\")", tag, version)
	}
}

func TestLatestTag_OverlappingPrefix(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	mustTag(t, repo, "app-1.0.0", sha)
	mustTag(t, repo, "apple-2.0.0", sha)
	mustTag(t, repo, "application-3.0.0", sha)

	tag, version, err := LatestTag(repo, "app-")
	if err != nil {
		t.Fatalf("LatestTag(...) returned an unexpected error: %v", err)
	}
	if tag != "app-1.0.0" || version != "1.0.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want (\"app-1.0.0\", \"1.0.0\")", tag, version)
	}
}

func TestDirChanged_SameCommit(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	changed, err := DirChanged(repo, sha, sha, ".")
	if err != nil {
		t.Fatalf("DirChanged(...) with identical commits returned an unexpected error: %v", err)
	}
	if changed {
		t.Error("DirChanged(...) with identical commits = true, want false")
	}
}

func TestDirChanged_TrailingSlashAndEmptyDir(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "a1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/file.txt": "a2",
	})

	changedTrailing, err := DirChanged(repo, firstSHA, secondSHA, "dirA/")
	if err != nil {
		t.Fatalf("DirChanged(..., \"dirA/\") returned an unexpected error: %v", err)
	}
	if !changedTrailing {
		t.Error("DirChanged(..., \"dirA/\") = false, want true")
	}

	changedEmpty, err := DirChanged(repo, firstSHA, secondSHA, "")
	if err != nil {
		t.Fatalf("DirChanged(..., \"\") returned an unexpected error: %v", err)
	}
	if !changedEmpty {
		t.Error("DirChanged(..., \"\") = false, want true")
	}
}

func TestDirChanged_FileDeletion(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "content",
	})

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if _, err := worktree.Remove("dirA/file.txt"); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}
	secondSHA, err := worktree.Commit("delete file", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	changed, err := DirChanged(repo, firstSHA, secondSHA.String(), "dirA")
	if err != nil {
		t.Fatalf("DirChanged(...) on file deletion returned an unexpected error: %v", err)
	}
	if !changed {
		t.Error("DirChanged(...) on file deletion = false, want true")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "versioning.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write versioning config fixture: %v", err)
	}
	return path
}

func newEmptyRepo(t *testing.T) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}
	return repo
}

func newRepoWithCommit(t *testing.T) (*git.Repository, string) {
	t.Helper()
	return newRepoWithFiles(t, map[string]string{"README.md": "test\n"})
}

func newRepoWithFiles(t *testing.T, files map[string]string) (*git.Repository, string) {
	t.Helper()
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}
	sha := commitFiles(t, repo, files)
	return repo, sha
}

func addCommitWithFiles(t *testing.T, repo *git.Repository, files map[string]string) string {
	t.Helper()
	return commitFiles(t, repo, files)
}

func commitFiles(t *testing.T, repo *git.Repository, files map[string]string) string {
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

	sha, err := worktree.Commit("test commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}
	return sha.String()
}

func mustTag(t *testing.T, repo *git.Repository, tagName, sha string) {
	t.Helper()
	if _, err := repo.CreateTag(tagName, plumbing.NewHash(sha), nil); err != nil {
		t.Fatalf("CreateTag(%q) failed: %v", tagName, err)
	}
}
