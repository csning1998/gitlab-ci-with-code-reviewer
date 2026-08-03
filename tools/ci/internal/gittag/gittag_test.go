package gittag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newRepoWithCommit initializes a Git repository at the specified directory path
// with a single initial commit and returns the corresponding commit hash.
func newRepoWithCommit(t *testing.T, dir string) string {
	t.Helper()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to initialize repository at path %q: %v", dir, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to retrieve working tree: %v", err)
	}

	filePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(filePath, []byte("test\n"), 0o644); err != nil {
		t.Fatalf("failed to write file to path %q: %v", filePath, err)
	}
	if _, err := worktree.Add("README.md"); err != nil {
		t.Fatalf("failed to stage README.md: %v", err)
	}

	sha, err := worktree.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	return sha.String()
}

func TestCreateTag(t *testing.T) {
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	if err := CreateTag(dir, "1.4.4", sha); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to open repository at path %q: %v", dir, err)
	}

	ref, err := repo.Tag("1.4.4")
	if err != nil {
		t.Fatalf("failed to retrieve reference for tag \"1.4.4\": %v", err)
	}
	if ref.Hash().String() != sha {
		t.Errorf("tag resolves to commit hash %q; expected %q", ref.Hash().String(), sha)
	}
}

func TestCreateTag_MissingRepo(t *testing.T) {
	if err := CreateTag(t.TempDir(), "1.4.4", "0000000000000000000000000000000000000000"); err == nil {
		t.Error("CreateTag(...) on uninitialized directory succeeded unexpectedly; expected an error")
	}
}

func TestCreateTag_InvalidCommitSHA(t *testing.T) {
	dir := t.TempDir()
	newRepoWithCommit(t, dir)

	err := CreateTag(dir, "1.4.4", "not-a-valid-sha")
	if err == nil {
		t.Fatal("CreateTag(...) with a malformed commit SHA succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "invalid commit SHA") {
		t.Errorf("CreateTag(...) error = %q, want it to mention an invalid commit SHA", err.Error())
	}
}

func TestPushTag(t *testing.T) {
	sourceDir := t.TempDir()
	sha := newRepoWithCommit(t, sourceDir)
	if err := CreateTag(sourceDir, "1.4.4", sha); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}

	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("failed to initialize bare repository at path %q: %v", remoteDir, err)
	}

	if err := PushTag(sourceDir, remoteDir, "1.4.4", "unused", "unused"); err != nil {
		t.Fatalf("PushTag(...) returned an unexpected error: %v", err)
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference(plumbing.NewTagReferenceName("1.4.4"), true)
	if err != nil {
		t.Fatalf("failed to locate reference for tag \"1.4.4\" within remote repository: %v", err)
	}
	if ref.Hash().String() != sha {
		t.Errorf("pushed tag resolves to commit hash %q; expected %q", ref.Hash().String(), sha)
	}
}

func TestPushTag_UnreachableRemote(t *testing.T) {
	sourceDir := t.TempDir()
	sha := newRepoWithCommit(t, sourceDir)
	if err := CreateTag(sourceDir, "1.4.4", sha); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}
	unreachableRemote := filepath.Join(t.TempDir(), "does-not-exist.git")

	err := PushTag(sourceDir, unreachableRemote, "1.4.4", "unused", "unused")
	if err == nil {
		t.Fatal("PushTag(...) against an unreachable remote succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "failed to push tag") {
		t.Errorf("PushTag(...) error = %q, want it to mention the push failure", err.Error())
	}
}

// TestPushTag_ExistingOriginRemote validates execution within target environments
// (e.g., GitLab CI execution runners) in which an origin remote pre-exists prior
// to package invocation.
func TestPushTag_ExistingOriginRemote(t *testing.T) {
	sourceDir := t.TempDir()
	sha := newRepoWithCommit(t, sourceDir)
	if err := CreateTag(sourceDir, "1.4.4", sha); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}

	repo, err := git.PlainOpen(sourceDir)
	if err != nil {
		t.Fatalf("failed to open source repository at path %q: %v", sourceDir, err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://example.invalid/unrelated.git"},
	}); err != nil {
		t.Fatalf("failed to seed pre-existing origin remote configuration: %v", err)
	}

	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("failed to initialize bare repository at path %q: %v", remoteDir, err)
	}

	if err := PushTag(sourceDir, remoteDir, "1.4.4", "unused", "unused"); err != nil {
		t.Fatalf("PushTag(...) returned an unexpected error: %v", err)
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	if _, err := remote.Reference(plumbing.NewTagReferenceName("1.4.4"), true); err != nil {
		t.Fatalf("failed to locate reference for tag \"1.4.4\" within remote repository: %v", err)
	}
}
