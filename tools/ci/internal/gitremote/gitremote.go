// Package gitremote manages origin remote configuration for authenticated Git push operations.
package gitremote

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// Name is the target remote name ("origin"). go-git file transport rejects push operations for non-origin remotes.
const Name = "origin"

// configLockName follows the lock file protocol of the Git command line, which serializes
// every writer of the repository configuration.
const configLockName = "config.lock"

// configLockTimeout bounds the wait for a competing writer. A lock left by a crashed writer
// fails the call after this delay.
const configLockTimeout = 5 * time.Second

// ConfigureOriginRemoteURL updates the origin remote URL in repository config in place, which
// avoids transient missing remote errors. A pre-existing clone URL is overwritten.
func ConfigureOriginRemoteURL(repo *git.Repository, remoteURL string) (*git.Remote, error) {
	if err := validateRemoteURL(remoteURL); err != nil {
		return nil, err
	}

	remoteConfig := &config.RemoteConfig{Name: Name, URLs: []string{remoteURL}}
	if err := remoteConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid remote configuration: %w", err)
	}

	err := updateConfig(repo, func(cfg *config.Config) {
		cfg.Remotes[Name] = remoteConfig
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist remote configuration: %w", err)
	}

	return git.NewRemote(repo.Storer, remoteConfig), nil
}

// validateRemoteURL rejects a URL which the configuration file cannot carry verbatim.
func validateRemoteURL(remoteURL string) error {
	if strings.TrimSpace(remoteURL) != remoteURL {
		return fmt.Errorf("remote URL %q has leading or trailing whitespace", remoteURL)
	}
	if strings.IndexFunc(remoteURL, unicode.IsControl) >= 0 {
		return fmt.Errorf("remote URL %q holds a control character", remoteURL)
	}
	return nil
}

// fallbackConfigMu serializes updateConfig writers for a storer other than *filesystem.Storage,
// since such a storer holds configuration in process memory with no file to lock.
var fallbackConfigMu sync.Mutex

// updateConfig applies mutate to the repository configuration under config.lock and replaces
// the file by rename. A writer which bypasses config.lock is not serialized.
func updateConfig(repo *git.Repository, mutate func(*config.Config)) error {
	storage, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		fallbackConfigMu.Lock()
		defer fallbackConfigMu.Unlock()
		cfg, err := repo.Config()
		if err != nil {
			return fmt.Errorf("failed to read repository configuration: %w", err)
		}
		mutate(cfg)
		return repo.Storer.SetConfig(cfg)
	}
	dotGit := storage.Filesystem()

	var lock io.WriteCloser
	deadline := time.Now().Add(configLockTimeout)
	for lock == nil {
		file, err := dotGit.OpenFile(configLockName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		switch {
		case err == nil:
			lock = file
		case !errors.Is(err, os.ErrExist):
			return err
		case time.Now().After(deadline):
			return fmt.Errorf("%s is held by another writer", configLockName)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	committed := false
	defer func() {
		if !committed {
			_ = dotGit.Remove(configLockName)
		}
	}()

	cfg, err := repo.Config()
	if err != nil {
		_ = lock.Close()
		return fmt.Errorf("failed to read repository configuration: %w", err)
	}
	mutate(cfg)
	data, err := cfg.Marshal()
	if err != nil {
		_ = lock.Close()
		return fmt.Errorf("failed to encode repository configuration: %w", err)
	}
	if _, err := lock.Write(data); err != nil {
		_ = lock.Close()
		return err
	}
	if err := lock.Close(); err != nil {
		return err
	}
	if err := dotGit.Rename(configLockName, "config"); err != nil {
		return err
	}
	committed = true
	return nil
}
