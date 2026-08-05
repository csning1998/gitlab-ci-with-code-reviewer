// Package gittag provides functionality to generate a lightweight tag at a specified commit within an existing
// local repository and transmit the tag to a remote destination. Authentication executes exclusively in memory,
// thereby eliminating credential exposure within URL strings, subprocess argument lists, or non-volatile storage.
package gittag

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"ci-tools/internal/gitremote"
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

// PushTag transmits an existing tag identified by tagName from the repository located at repoPath
// to remoteURL, utilizing username and password as HTTP basic authentication credentials.
func PushTag(repoPath, remoteURL, tagName, username, password string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at path %q: %w", repoPath, err)
	}

	remote, err := gitremote.Set(repo, remoteURL)
	if err != nil {
		return fmt.Errorf("failed to configure push remote for URL %q: %w", remoteURL, err)
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", tagName, tagName))
	err = remote.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{refSpec},
		Auth: &http.BasicAuth{
			Username: username,
			Password: password,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push tag %q to remote destination %q: %w", tagName, remoteURL, err)
	}

	return nil
}
