// Package gitremote configures the "origin" remote repository reference for authenticated push operations,
// establishing a unified execution sequence shared by internal/gittag for tag pushes and internal/fmtpush
// for formatting-commit pushes.
package gitremote

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// Name defines the sole remote identifier upon which this package operates, given that the file transport
// mechanism within go-git rejects push operations for remotes configured under alternative identifiers.
const Name = "origin"

// Set updates the configuration of the Name remote via an atomic read-modify-write operation, thereby
// eliminating the failure window inherent in consecutive deletion and creation operations during which
// the repository might otherwise remain devoid of a Name remote configuration. This procedure overwrites
// pre-existing configurations for Name to accommodate execution environments (e.g. GitLab CI working
// directories) in which a default origin remote targets a distinct URL.
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
