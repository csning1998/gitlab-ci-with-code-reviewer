// Package fmtpush stages, commits, and pushes code formatting changes to a merge request source branch.
package fmtpush

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"ci-tools/internal/gitremote"
)

// HasChanges checks if the repository at repoPath has uncommitted changes or untracked files
// using working tree status (matching git status).
func HasChanges(repoPath string) (bool, error) {
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

// CommitAndPush stages every change in the working tree at repoPath, commits it as message
// authored by "GitLab CI", and pushes the resulting commit to branch on the remote identified by
// remoteURL, using username and password as HTTP basic authentication credentials.
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

	remote, err := gitremote.Set(repo, remoteURL)
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
