package fmtpush

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func newRepoWithCommit(t *testing.T) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
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
	// GitLab CI checks out commit hashes directly, placing HEAD in detached state.
	// CommitAndPush resolves HEAD when operating under detached HEAD state.
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: hash}); err != nil {
		t.Fatalf("Checkout(%q) failed: %v", hash, err)
	}

	return dir, hash.String()
}

// seedRemoteBranch initializes a bare repository at remoteDir and commits an isolated
// history to branch, creating a non-fast-forward target for local pushes.
//
// Seed parameters must differ from newRepoWithCommit defaults ("main.go", "package main\n",
// "initial commit", test/test@example.com). Matching tree objects, commit messages, and
// author attributes generated within the same second yield identical commit hashes,
// converting divergent history into a linear ancestor and invalidating test constraints.
func seedRemoteBranch(t *testing.T, remoteDir, branch string) {
	t.Helper()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	seedDir := t.TempDir()
	seedRepo, err := git.PlainInit(seedDir, false)
	if err != nil {
		t.Fatalf("PlainInit(%q) failed: %v", seedDir, err)
	}
	seedWorktree, err := seedRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	seedFilePath := filepath.Join(seedDir, "unrelated.txt")
	if err := os.WriteFile(seedFilePath, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", seedFilePath, err)
	}
	if _, err := seedWorktree.Add("unrelated.txt"); err != nil {
		t.Fatalf("Add(unrelated.txt) failed: %v", err)
	}
	sha, err := seedWorktree.Commit("seed: unrelated history", &git.CommitOptions{
		Author: &object.Signature{Name: "seed", Email: "seed@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	remote, err := seedRepo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
	if err != nil {
		t.Fatalf("failed to configure seed remote: %v", err)
	}
	refSpec := config.RefSpec(fmt.Sprintf("%s:refs/heads/%s", sha, branch))
	if err := remote.Push(&git.PushOptions{RefSpecs: []config.RefSpec{refSpec}}); err != nil {
		t.Fatalf("failed to seed remote branch %q: %v", branch, err)
	}
}

func TestHasChanges_Clean(t *testing.T) {
	dir, _ := newRepoWithCommit(t)

	changed, err := HasChanges(dir)
	if err != nil {
		t.Fatalf("HasChanges(...) returned an unexpected error: %v", err)
	}
	if changed {
		t.Error("HasChanges(...) = true, want false on a clean working tree")
	}
}

func TestHasChanges_ModifiedTrackedFile(t *testing.T) {
	dir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}

	changed, err := HasChanges(dir)
	if err != nil {
		t.Fatalf("HasChanges(...) returned an unexpected error: %v", err)
	}
	if !changed {
		t.Error("HasChanges(...) = false, want true after modifying a tracked file")
	}
}

func TestHasChanges_MissingRepo(t *testing.T) {
	if _, err := HasChanges(t.TempDir()); err == nil {
		t.Error("HasChanges(...) on a non-repository succeeded unexpectedly; expected an error")
	}
}

func TestHasChanges_UntrackedFile(t *testing.T) {
	dir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}

	// HasChanges evaluates working tree status matching git status insteaf of git diff;
	// unstaged and untracked files mark the tree as modified.
	changed, err := HasChanges(dir)
	if err != nil {
		t.Fatalf("HasChanges(...) returned an unexpected error: %v", err)
	}
	if !changed {
		t.Error("HasChanges(...) = false, want true with an untracked file present")
	}
}

func TestHasChanges_StagedButUncommitted(t *testing.T) {
	dir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to open repository at path %q: %v", dir, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if _, err := worktree.Add("main.go"); err != nil {
		t.Fatalf("Add(main.go) failed: %v", err)
	}

	changed, err := HasChanges(dir)
	if err != nil {
		t.Fatalf("HasChanges(...) returned an unexpected error: %v", err)
	}
	if !changed {
		t.Error("HasChanges(...) = false, want true for a staged but uncommitted change")
	}
}

func TestHasChanges_DeletedTrackedFile(t *testing.T) {
	dir, _ := newRepoWithCommit(t)
	if err := os.Remove(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("Remove(...) failed: %v", err)
	}

	changed, err := HasChanges(dir)
	if err != nil {
		t.Fatalf("HasChanges(...) returned an unexpected error: %v", err)
	}
	if !changed {
		t.Error("HasChanges(...) = false, want true after deleting a tracked file")
	}
}

func TestHasChanges_BareRepository(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", dir, err)
	}

	if _, err := HasChanges(dir); err == nil {
		t.Error("HasChanges(...) on a bare repository succeeded unexpectedly; expected an error")
	}
}

func TestCommitAndPush_Success(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}

	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	if err := CommitAndPush(sourceDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("CommitAndPush(...) returned an unexpected error: %v", err)
	}

	changed, err := HasChanges(sourceDir)
	if err != nil {
		t.Fatalf("HasChanges(...) returned an unexpected error: %v", err)
	}
	if changed {
		t.Error("HasChanges(...) = true after CommitAndPush, want false; the working tree should have been committed")
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference(plumbing.NewBranchReferenceName("feature-branch"), true)
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
	if commit.Author.Name != "GitLab CI" || commit.Author.Email != "gitlab-ci@noreply" {
		t.Errorf("pushed commit author = %q <%s>, want \"GitLab CI\" <gitlab-ci@noreply>", commit.Author.Name, commit.Author.Email)
	}
}

func TestCommitAndPush_ExistingOriginRemote(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
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
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	if err := CommitAndPush(sourceDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("CommitAndPush(...) returned an unexpected error: %v", err)
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	if _, err := remote.Reference(plumbing.NewBranchReferenceName("feature-branch"), true); err != nil {
		t.Fatalf("failed to locate branch \"feature-branch\" within remote repository: %v", err)
	}
}

func TestCommitAndPush_UnreachableRemote(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	unreachableRemote := filepath.Join(t.TempDir(), "does-not-exist.git")

	err := CommitAndPush(sourceDir, "style: gofmt", "feature-branch", unreachableRemote, "gitlab-ci-token", "unused")
	if err == nil {
		t.Fatal("CommitAndPush(...) against an unreachable remote succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "failed to push branch") {
		t.Errorf("CommitAndPush(...) error = %q, want it to mention the push failure", err.Error())
	}
}

func TestCommitAndPush_NoChanges(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	err := CommitAndPush(sourceDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused")
	if err == nil {
		t.Fatal("CommitAndPush(...) with no working tree changes succeeded unexpectedly; expected a commit error")
	}
}

func TestCommitAndPush_StagesUntrackedFiles(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(sourceDir, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	// AddOptions{All: true} stages untracked paths alongside tracked modifications,
	// including newly created files in the commit index.
	if err := CommitAndPush(sourceDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("CommitAndPush(...) returned an unexpected error: %v", err)
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference(plumbing.NewBranchReferenceName("feature-branch"), true)
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

func TestCommitAndPush_NonFastForwardRejected(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := t.TempDir()
	seedRemoteBranch(t, remoteDir, "feature-branch")

	// The push refspec omits the "+" force modifier, causing remote rejection on non-fast-forward updates.
	err := CommitAndPush(sourceDir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused")
	if err == nil {
		t.Fatal("CommitAndPush(...) against a diverged remote branch succeeded unexpectedly; expected a non-fast-forward rejection")
	}
	if !strings.Contains(err.Error(), "failed to push branch") {
		t.Errorf("CommitAndPush(...) error = %q, want it to mention the push failure", err.Error())
	}
}

// TestCommitAndPush_ConcurrentJobsOnSameBranch_SecondPushRejectedWithoutDataLoss verifies that
// concurrent pushes to the same branch fail on non-fast-forward updates without overwriting remote commits.
func TestCommitAndPush_ConcurrentJobsOnSameBranch_SecondPushRejectedWithoutDataLoss(t *testing.T) {
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	bootstrapDir, sha := newRepoWithCommit(t)
	bootstrapRepo, err := git.PlainOpen(bootstrapDir)
	if err != nil {
		t.Fatalf("failed to open bootstrap repository at path %q: %v", bootstrapDir, err)
	}
	bootstrapRemote, err := bootstrapRepo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
	if err != nil {
		t.Fatalf("failed to configure bootstrap remote: %v", err)
	}
	refSpec := config.RefSpec(fmt.Sprintf("%s:refs/heads/feature-branch", sha))
	if err := bootstrapRemote.Push(&git.PushOptions{RefSpecs: []config.RefSpec{refSpec}}); err != nil {
		t.Fatalf("failed to bootstrap remote branch: %v", err)
	}

	// jobADir and jobBDir clone the same commit SHA, simulating concurrent CI runners executing against identical baseline history.
	jobADir := cloneDetached(t, remoteDir, "feature-branch")
	jobBDir := cloneDetached(t, remoteDir, "feature-branch")

	if err := os.WriteFile(filepath.Join(jobADir, "main.go"), []byte("package main\n\nfunc a() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	if err := CommitAndPush(jobADir, "style: gofmt (job A)", "feature-branch", remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("job A CommitAndPush(...) returned an unexpected error: %v", err)
	}

	// Job B working tree descends from the initial bootstrap commit. Pushing after
	// job A advances the remote ref causes a non-fast-forward rejection.
	if err := os.WriteFile(filepath.Join(jobBDir, "other.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	err = CommitAndPush(jobBDir, "style: gofmt (job B)", "feature-branch", remoteDir, "gitlab-ci-token", "unused")
	if err == nil {
		t.Fatal("job B CommitAndPush(...) succeeded unexpectedly; expected rejection since job A already advanced the branch")
	}
	if !strings.Contains(err.Error(), "failed to push branch") {
		t.Errorf("job B CommitAndPush(...) error = %q, want it to mention the push failure", err.Error())
	}

	// Verify remote branch tip retains job A commit hash following job B rejection.
	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference(plumbing.NewBranchReferenceName("feature-branch"), true)
	if err != nil {
		t.Fatalf("failed to locate branch \"feature-branch\" within remote repository: %v", err)
	}
	commit, err := remote.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("failed to resolve remote branch tip commit: %v", err)
	}
	if commit.Message != "style: gofmt (job A)" {
		t.Errorf("remote branch tip message = %q, want %q; job B's rejected push must not change it", commit.Message, "style: gofmt (job A)")
	}
}

// cloneDetached clones branch from remoteURL into a temporary directory and checks out
// the target commit hash in detached HEAD state.
func cloneDetached(t *testing.T, remoteURL, branch string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           remoteURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err != nil {
		t.Fatalf("PlainClone(%q) failed: %v", remoteURL, err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head() failed: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: head.Hash()}); err != nil {
		t.Fatalf("Checkout(%q) failed: %v", head.Hash(), err)
	}
	return dir
}

func TestCommitAndPush_FastForwardOntoExistingBranch(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc a() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	if err := CommitAndPush(sourceDir, "style: gofmt pass 1", "feature-branch", remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("first CommitAndPush(...) returned an unexpected error: %v", err)
	}

	// Local HEAD incorporates previous commit history, allowing subsequent pushes to
	// fast-forward the remote reference without force flags.
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc a() {}\nfunc b() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	if err := CommitAndPush(sourceDir, "style: gofmt pass 2", "feature-branch", remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("second CommitAndPush(...) returned an unexpected error: %v", err)
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	ref, err := remote.Reference(plumbing.NewBranchReferenceName("feature-branch"), true)
	if err != nil {
		t.Fatalf("failed to locate branch \"feature-branch\" within remote repository: %v", err)
	}
	commit, err := remote.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("failed to resolve pushed commit: %v", err)
	}
	if commit.Message != "style: gofmt pass 2" {
		t.Errorf("pushed commit message = %q, want %q", commit.Message, "style: gofmt pass 2")
	}
}

func TestCommitAndPush_BareRepository(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", dir, err)
	}
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	err := CommitAndPush(dir, "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused")
	if err == nil {
		t.Fatal("CommitAndPush(...) against a bare repository succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "failed to retrieve working tree") {
		t.Errorf("CommitAndPush(...) error = %q, want it to mention the missing working tree", err.Error())
	}
}

func TestCommitAndPush_MissingRepo(t *testing.T) {
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	err := CommitAndPush(t.TempDir(), "style: gofmt", "feature-branch", remoteDir, "gitlab-ci-token", "unused")
	if err == nil {
		t.Fatal("CommitAndPush(...) against a non-repository path succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "failed to open repository") {
		t.Errorf("CommitAndPush(...) error = %q, want it to mention the open failure", err.Error())
	}
}

func TestCommitAndPush_BranchNameWithSlash(t *testing.T) {
	sourceDir, _ := newRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) failed: %v", err)
	}
	remoteDir := t.TempDir()
	if _, err := git.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit(%q, bare) failed: %v", remoteDir, err)
	}

	branch := "release/1.0"
	if err := CommitAndPush(sourceDir, "style: gofmt", branch, remoteDir, "gitlab-ci-token", "unused"); err != nil {
		t.Fatalf("CommitAndPush(...) returned an unexpected error: %v", err)
	}

	remote, err := git.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("failed to open remote repository at path %q: %v", remoteDir, err)
	}
	if _, err := remote.Reference(plumbing.NewBranchReferenceName(branch), true); err != nil {
		t.Fatalf("failed to locate branch %q within remote repository: %v", branch, err)
	}
}
