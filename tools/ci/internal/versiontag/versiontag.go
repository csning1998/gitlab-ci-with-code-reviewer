// Package versiontag parses repository module configuration files to determine
// module semantic version tags and directory modification status between commits.
package versiontag

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"go.yaml.in/yaml/v4"

	"ci-tools/internal/semver"
)

// Module defines a versioned directory scope within a repository. Dir is the root-relative
// path where file changes trigger version increments ("." for repository root). TagPrefix
// is prepended to tag names; if nil, it defaults to "<Name>-" for named modules or "" if empty.
type Module struct {
	Name      string  `yaml:"name"`
	Dir       string  `yaml:"dir"`
	TagPrefix *string `yaml:"tag_prefix"`
}

// Prefix resolves the effective tag prefix applicable to the target module instance.
func (m Module) Prefix() string {
	if m.TagPrefix != nil {
		return *m.TagPrefix
	}
	if m.Name == "" {
		return ""
	}
	return m.Name + "-"
}

// Config represents the repository-local versioning specification consumed by the cmd/auto-tag binary.
type Config struct {
	Modules []Module `yaml:"modules"`
}

// LoadConfig retrieves and parses the versioning specification from the specified file path.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read versioning config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse versioning config %q: %w", path, err)
	}
	if len(cfg.Modules) == 0 {
		return Config{}, fmt.Errorf("versioning config %q declares no modules", path)
	}
	for i, m := range cfg.Modules {
		if m.Dir == "" {
			return Config{}, fmt.Errorf("versioning config %q: module at index %d has an empty dir", path, i)
		}
	}

	return cfg, nil
}

// LatestTag returns the highest semver tag and parsed MAJOR.MINOR.PATCH version matching
// prefix. Tags are ordered by semantic version instead of commit ancestry. Returns
// "<prefix>0.0.0" and "0.0.0" if no tag matches.
func LatestTag(repo *git.Repository, prefix string) (tag, version string, err error) {
	iter, err := repo.Tags()
	if err != nil {
		return "", "", fmt.Errorf("failed to list tags: %w", err)
	}
	defer iter.Close()

	bestMajor, bestMinor, bestPatch := -1, -1, -1
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		if !strings.HasPrefix(name, prefix) {
			return nil
		}

		candidate := strings.TrimPrefix(strings.TrimPrefix(name[len(prefix):], "v"), "V")
		major, minor, patch, parseErr := semver.ParseVersion(candidate)
		if parseErr != nil {
			// Ignore non-semver tag suffixes instead of failing.
			return nil
		}

		if compareVersions(major, minor, patch, bestMajor, bestMinor, bestPatch) > 0 {
			bestMajor, bestMinor, bestPatch = major, minor, patch
			tag = name
		}
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to iterate tags: %w", err)
	}

	if tag == "" {
		return prefix + "0.0.0", "0.0.0", nil
	}
	return tag, fmt.Sprintf("%d.%d.%d", bestMajor, bestMinor, bestPatch), nil
}

func compareVersions(majorA, minorA, patchA, majorB, minorB, patchB int) int {
	if majorA != majorB {
		return majorA - majorB
	}
	if minorA != minorB {
		return minorA - minorB
	}
	return patchA - patchB
}

// DirChanged reports whether any files under dir were modified between parentSHA and sha commit
// trees. Passing "." or "" matches changes anywhere in the repository.
func DirChanged(repo *git.Repository, parentSHA, sha, dir string) (bool, error) {
	fromTree, err := treeAt(repo, parentSHA)
	if err != nil {
		return false, err
	}
	toTree, err := treeAt(repo, sha)
	if err != nil {
		return false, err
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return false, fmt.Errorf("failed to diff commits %q and %q: %w", parentSHA, sha, err)
	}

	prefix := strings.TrimSuffix(dir, "/")
	for _, change := range changes {
		path := change.To.Name
		if path == "" {
			path = change.From.Name
		}
		if pathUnderDir(path, prefix) {
			return true, nil
		}
	}
	return false, nil
}

func pathUnderDir(path, dir string) bool {
	if dir == "" || dir == "." {
		return true
	}
	return path == dir || strings.HasPrefix(path, dir+"/")
}

func treeAt(repo *git.Repository, sha string) (*object.Tree, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve commit %q: %w", sha, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tree for commit %q: %w", sha, err)
	}
	return tree, nil
}
