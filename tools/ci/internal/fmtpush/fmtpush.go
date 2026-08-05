// Package fmtpush stages, commits, and pushes formatting revisions to a remote merge request branch.
package fmtpush

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"ci-tools/internal/gitremote"
)

// DetectWorktreeModifications reports whether the repository working tree contains
// modifications or untracked files relative to the index and HEAD.
func DetectWorktreeModifications(repoPath string) (bool, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return false, fmt.Errorf("failed to open repository at path %q: %w", repoPath, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to retrieve working tree: %w", err)
	}
	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to read working tree status: %w", err)
	}
	return !status.IsClean(), nil
}

// CommitAndPush stages working tree modifications, creates a commit from local HEAD with message,
// and pushes to branch on remoteURL using basic authentication.
// Non-fast-forward pushes fail directly without attempting content merges.
func CommitAndPush(repoPath, message, branch, remoteURL, username, password string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at path %q: %w", repoPath, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to retrieve working tree: %w", err)
	}

	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("failed to stage working tree changes: %w", err)
	}

	if _, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "GitLab CI", Email: "gitlab-ci@noreply"},
	}); err != nil {
		return fmt.Errorf("failed to create commit %q: %w", message, err)
	}

	remote, err := gitremote.ConfigureOriginRemoteURL(repo, remoteURL)
	if err != nil {
		return fmt.Errorf("failed to configure push remote for URL %q: %w", remoteURL, err)
	}

	refSpec := config.RefSpec(fmt.Sprintf("HEAD:refs/heads/%s", branch))
	err = remote.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{refSpec},
		Auth: &http.BasicAuth{
			Username: username,
			Password: password,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push branch %q to remote destination %q: %w", branch, remoteURL, err)
	}

	return nil
}
