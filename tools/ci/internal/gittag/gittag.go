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
)

// remoteName is restricted explicitly to "origin", as the file transport mechanism within go-git
// rejects push operations for remotes configured under alternative identifiers. PushTag overwrites
// the configuration of this remote upon each invocation, as the GitLab CI working directory against
// which this package executes contains a pre-existing "origin" remote that references a distinct target URL.
const remoteName = "origin"

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

// setRemote updates the configuration of remoteName within an atomic read-modify-write sequence,
// thereby eliminating the failure window inherent in consecutive deletion and creation operations
// during which the repository might otherwise remain devoid of an "origin" remote configuration.
func setRemote(repo *git.Repository, remoteURL string) (*git.Remote, error) {
	cfg, err := repo.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to read repository configuration: %w", err)
	}

	remoteConfig := &config.RemoteConfig{Name: remoteName, URLs: []string{remoteURL}}
	if err := remoteConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid remote configuration: %w", err)
	}

	cfg.Remotes[remoteName] = remoteConfig
	if err := repo.Storer.SetConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to persist remote configuration: %w", err)
	}

	return git.NewRemote(repo.Storer, remoteConfig), nil
}

// PushTag transmits an existing tag identified by tagName from the repository located at repoPath
// to remoteURL, utilizing username and password as HTTP basic authentication credentials.
func PushTag(repoPath, remoteURL, tagName, username, password string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at path %q: %w", repoPath, err)
	}

	remote, err := setRemote(repo, remoteURL)
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
