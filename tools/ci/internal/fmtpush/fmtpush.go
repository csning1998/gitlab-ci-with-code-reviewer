// Package fmtpush stages, commits, and pushes automated formatting changes made to a merge
// request's source branch. It consolidates the git remote setup, dirty-check, commit, and push
// sequence that was previously duplicated within five separate CI/CD component templates
// (lang-go.yml, lang-python.yml, lang-typescript.yml, iac-terraform.yml, iac-packer.yml), three of
// which lacked the fallback-to-create-remote defense the other two carried.
package fmtpush

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"ci-tools/internal/gitremote"
)

// HasChanges reports whether the working tree at repoPath differs from HEAD, replacing the
// `git diff --quiet` dirty-check duplicated across the five templates named above. Unlike
// `git diff`, which ignores untracked paths, this also reports true for untracked files, matching
// `git status` rather than `git diff`; the formatters driving every caller only rewrite files
// already tracked in the repository, so this distinction has no observable effect in practice.
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
