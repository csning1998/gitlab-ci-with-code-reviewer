package gittag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

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

func TestCreateTag_UppercaseHexSHA_RejectedAsInvalid(t *testing.T) {
	// Enforces canonical lowercase hex strings; comparison against plumbing.Hash.String()
	// rejects uppercase hex input to guarantee deterministic ref encoding.
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	err := CreateTag(dir, "1.4.4", strings.ToUpper(sha))
	if err == nil {
		t.Fatal("CreateTag(...) with an uppercase-hex SHA succeeded unexpectedly; want it rejected as invalid")
	}
	if !strings.Contains(err.Error(), "invalid commit SHA") {
		t.Errorf("CreateTag(...) error = %q, want it to mention an invalid commit SHA", err.Error())
	}
}

func TestCreateTag_SHAWithTrailingWhitespace_RejectedAsInvalid(t *testing.T) {
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	err := CreateTag(dir, "1.4.4", sha+" ")
	if err == nil {
		t.Fatal("CreateTag(...) with a whitespace-padded SHA succeeded unexpectedly; want it rejected as invalid")
	}
}

func TestCreateTag_NonExistentButWellFormedSHA_TagStillCreated(t *testing.T) {
	// Validates SHA syntax via plumbing.Hash without verifying object store presence,
	// allowing lightweight tag refs to point to unmerged or external commit hashes.
	dir := t.TempDir()
	newRepoWithCommit(t, dir)
	danglingSHA := strings.Repeat("f", 40)

	if err := CreateTag(dir, "1.4.4", danglingSHA); err != nil {
		t.Fatalf("CreateTag(...) with a well-formed but nonexistent SHA returned an unexpected error: %v", err)
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to open repository at path %q: %v", dir, err)
	}
	ref, err := repo.Tag("1.4.4")
	if err != nil {
		t.Fatalf("failed to retrieve reference for tag \"1.4.4\": %v", err)
	}
	if ref.Hash().String() != danglingSHA {
		t.Errorf("tag resolves to commit hash %q; expected the dangling hash %q", ref.Hash().String(), danglingSHA)
	}
}

func TestCreateTag_DuplicateTagName_Fails(t *testing.T) {
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	if err := CreateTag(dir, "1.4.4", sha); err != nil {
		t.Fatalf("first CreateTag(...) returned an unexpected error: %v", err)
	}

	err := CreateTag(dir, "1.4.4", sha)
	if err == nil {
		t.Fatal("second CreateTag(...) with an already-existing tag name succeeded unexpectedly; want an error")
	}
	if !strings.Contains(err.Error(), "failed to create tag") {
		t.Errorf("CreateTag(...) error = %q, want it to mention the create failure", err.Error())
	}
}

func TestCreateTag_EmptyTagName_Fails(t *testing.T) {
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	if err := CreateTag(dir, "", sha); err == nil {
		t.Error("CreateTag(...) with an empty tag name succeeded unexpectedly; want an error")
	}
}

func TestCreateTag_TagNameWithInvalidRefCharacters_Fails(t *testing.T) {
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	// Rejects spaces and tildes to comply with Git reference naming rules (git-check-ref-format).
	if err := CreateTag(dir, "invalid tag~name", sha); err == nil {
		t.Error("CreateTag(...) with an invalid ref-name character succeeded unexpectedly; want an error")
	}
}

func TestCreateTag_TagNameContainingSlash_CreatesNestedRef(t *testing.T) {
	// Constructs nested reference paths (refs/tags/release/1.0.0); slashes provide structural namespacing within ref storage.
	dir := t.TempDir()
	sha := newRepoWithCommit(t, dir)

	if err := CreateTag(dir, "release/1.0.0", sha); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error for a slash-bearing tag name: %v", err)
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to open repository at path %q: %v", dir, err)
	}
	ref, err := repo.Tag("release/1.0.0")
	if err != nil {
		t.Fatalf("failed to retrieve reference for tag \"release/1.0.0\": %v", err)
	}
	if ref.Hash().String() != sha {
		t.Errorf("tag resolves to commit hash %q; expected %q", ref.Hash().String(), sha)
	}
}

func TestCreateTag_BareRepository(t *testing.T) {
	// Reference manipulation operates solely on object/ref storage without requiring a working tree,
	// enabling tag creation on bare repositories during CI mirror checkouts.
	sourceDir := t.TempDir()
	sha := newRepoWithCommit(t, sourceDir)

	bareDir := t.TempDir()
	bareRepo, err := git.PlainInit(bareDir, true)
	if err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", bareDir, err)
	}

	sourceRepo, err := git.PlainOpen(sourceDir)
	if err != nil {
		t.Fatalf("failed to open source repository at path %q: %v", sourceDir, err)
	}
	remote, err := sourceRepo.CreateRemote(&config.RemoteConfig{Name: "bare-target", URLs: []string{bareDir}})
	if err != nil {
		t.Fatalf("failed to configure a remote pointing at the bare repository: %v", err)
	}
	refSpec := config.RefSpec(fmt.Sprintf("%s:refs/heads/main", sha))
	if err := remote.Push(&git.PushOptions{RemoteName: "bare-target", RefSpecs: []config.RefSpec{refSpec}}); err != nil {
		t.Fatalf("failed to push the source commit into the bare repository: %v", err)
	}
	_ = bareRepo

	if err := CreateTag(bareDir, "1.4.4", sha); err != nil {
		t.Fatalf("CreateTag(...) against a bare repository returned an unexpected error: %v", err)
	}

	bare, err := git.PlainOpen(bareDir)
	if err != nil {
		t.Fatalf("failed to open bare repository at path %q: %v", bareDir, err)
	}
	ref, err := bare.Tag("1.4.4")
	if err != nil {
		t.Fatalf("failed to retrieve reference for tag \"1.4.4\" in the bare repository: %v", err)
	}
	if ref.Hash().String() != sha {
		t.Errorf("tag resolves to commit hash %q; expected %q", ref.Hash().String(), sha)
	}
}
