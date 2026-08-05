// Package versiontag parses a repository-local module declaration to derive, for each module,
// the current semantic version and the directory modification state between two specified commits.
// This implementation supersedes the comma-separated modules and v_prefix CI/CD component inputs,
// the encoding of which cannot express per-module tag separators, version prefixes, or coexisting
// prefixed and unprefixed tag schemes within a single repository—three requirements identified through
// a cross-ecosystem survey of Git tagging conventions.
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

// Module specifies an independently versioned artifact within a repository. Dir represents the path,
// relative to the repository root, within which file modifications trigger version incrementation;
// "." designates repository-wide scope. TagPrefix explicitly defines the prefix prepended to the
// resolved version string; when set to nil, this field defaults to "<Name>-" for non-empty Name
// attributes, or an empty string when Name remains unspecified.
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

// LatestTag determines the highest semantic version matching the specified prefix, returning both
// the complete tag name and the extracted MAJOR.MINOR.PATCH tuple. Evaluation relies on parsed
// semantic version ordering rather than commit-graph proximity; because all pipeline tags originate
// from sequential executions on the default branch, semantic version ordering and graph proximity
// remain equivalent without requiring commit-ancestry graph traversal. If no matching tag exists,
// the function returns a synthetic "<prefix>0.0.0" tag alongside the fallback version string "0.0.0",
// thereby preserving backward compatibility with initial module release behavior.
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
			// Reference names matching the prefix do not unconditionally represent valid release tags;
			// non-semver suffixes are ignored rather than processed as fatal errors.
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

// DirChanged evaluates whether file modifications exist within the specified directory between
// the commit trees referenced by parentSHA and sha. The directory path "." evaluates to true
// for all file paths, reproducing the unconditional tagging behavior exhibited when
// module scoping is omitted.
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
