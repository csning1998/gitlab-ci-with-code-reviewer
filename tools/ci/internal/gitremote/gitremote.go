// Package gitremote manages origin remote configuration for authenticated Git push operations.
package gitremote

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// Name is the target remote name ("origin"). go-git file transport rejects push operations for non-origin remotes.
const Name = "origin"

// Set updates the origin remote URL in repository config. It updates the remote in-place
// to avoid transient missing-remote errors and overwrites pre-existing clone URLs.
func Set(repo *git.Repository, remoteURL string) (*git.Remote, error) {
	cfg, err := repo.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to read repository configuration: %w", err)
	}

	remoteConfig := &config.RemoteConfig{Name: Name, URLs: []string{remoteURL}}
	if err := remoteConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid remote configuration: %w", err)
	}

	cfg.Remotes[Name] = remoteConfig
	if err := repo.Storer.SetConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to persist remote configuration: %w", err)
	}

	return git.NewRemote(repo.Storer, remoteConfig), nil
}
