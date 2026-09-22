// Package gittag creates lightweight Git tags in a local repository to mirror remote tag state.
package gittag

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// CreateTag generates a lightweight Git tag specified by tagName at the commit identified by
// commitSHA within the repository at repoPath. Exactly one of the concurrent callers which name
// one tag succeeds.
func CreateTag(repoPath, tagName, commitSHA string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at path %q: %w", repoPath, err)
	}

	hash := plumbing.NewHash(commitSHA)
	if hash.String() != commitSHA {
		return fmt.Errorf("invalid commit SHA %q", commitSHA)
	}
	if err := createTagReference(repo, tagName, hash); err != nil {
		return fmt.Errorf("failed to create tag %q at commit %q: %w", tagName, commitSHA, err)
	}

	return nil
}

// createTagReference writes refs/tags/<tagName> through an exclusive create. The check of
// go-git followed by an unconditional write leaves a window in which two writers both succeed.
func createTagReference(repo *git.Repository, tagName string, hash plumbing.Hash) error {
	name := plumbing.NewTagReferenceName(tagName)
	if err := name.Validate(); err != nil {
		return err
	}

	_, err := repo.Storer.Reference(name)
	switch {
	case err == nil:
		return git.ErrTagExists
	case !errors.Is(err, plumbing.ErrReferenceNotFound):
		return err
	}

	storage, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return fmt.Errorf("repository storage %T does not support exclusive reference creation", repo.Storer)
	}
	dotGit := storage.Filesystem()

	file, err := dotGit.OpenFile(name.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if errors.Is(err, os.ErrExist) {
		return git.ErrTagExists
	}
	if err != nil {
		return err
	}

	_, writeErr := fmt.Fprintln(file, hash.String())
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		// An empty reference file would block every later writer of the same name.
		_ = dotGit.Remove(name.String())
		return errors.Join(writeErr, closeErr)
	}
	return nil
}
