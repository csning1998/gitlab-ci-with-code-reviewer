// Package gittag creates lightweight Git tags in a local repository to mirror remote tag state.
package gittag

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// CreateTag generates a lightweight Git tag specified by tagName at the commit identified
// by commitSHA within the repository located at repoPath.
func CreateTag(repoPath, tagName, commitSHA string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at path %q: %w", repoPath, err)
	}

	hash := plumbing.NewHash(commitSHA)
	if hash.String() != commitSHA {
		return fmt.Errorf("invalid commit SHA %q", commitSHA)
	}
	if _, err := repo.CreateTag(tagName, hash, nil); err != nil {
		return fmt.Errorf("failed to create tag %q at commit %q: %w", tagName, commitSHA, err)
	}

	return nil
}
