// Command auto-tag parses commit messages and creates and pushes semantic version tags for
// repository modules configured in versioning.yml.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"ci-tools/internal/gittag"
	"ci-tools/internal/semver"
	"ci-tools/internal/versiontag"
)

// commitSubject extracts the initial line of the commit message corresponding to the specified commit hash.
func commitSubject(repo *git.Repository, sha string) (string, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", fmt.Errorf("failed to resolve commit %q: %w", sha, err)
	}
	subject, _, _ := strings.Cut(commit.Message, "\n")
	return subject, nil
}

// parentSHA returns the first parent commit hash of sha. Returns false if sha is a root commit.
func parentSHA(repo *git.Repository, sha string) (string, bool, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve commit %q: %w", sha, err)
	}
	if len(commit.ParentHashes) == 0 {
		return "", false, nil
	}
	return commit.ParentHashes[0].String(), true, nil
}

// run evaluates modules in configPath and pushes warranted tags for sha. Execution logic is
// separated from CLI flags and process exit for unit testing.
func run(repoPath, configPath, sha, remoteURL, username, password string, stdout, stderr io.Writer) int {
	if sha == "" || remoteURL == "" || username == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --sha, --remote-url, and --username are required.")
		return 1
	}
	if password == "" {
		_, _ = fmt.Fprintln(stderr, "Error: TAG_PUSH_TOKEN must be set in the environment.")
		return 1
	}

	cfg, err := versiontag.LoadConfig(configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: failed to open repository at path %q: %v\n", repoPath, err)
		return 1
	}

	subject, err := commitSubject(repo, sha)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	bump := semver.DetermineBump(subject)

	parent, hasParent, err := parentSHA(repo, sha)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	for _, mod := range cfg.Modules {
		label := mod.Name
		if label == "" {
			label = "<repository>"
		}

		// Root commits have no parent tree to diff against, so all modules are treated as changed.
		changed := true
		if hasParent {
			changed, err = versiontag.DirChanged(repo, parent, sha, mod.Dir)
			if err != nil {
				_, _ = fmt.Fprintln(stderr, "Error:", err)
				return 1
			}
		}
		if !changed {
			_, _ = fmt.Fprintf(stdout, "%s: no changes under %q. Skipping.\n", label, mod.Dir)
			continue
		}

		prefix := mod.Prefix()
		latestTag, latestVersion, err := versiontag.LatestTag(repo, prefix)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "Error:", err)
			return 1
		}

		if bump == semver.BumpNone {
			_, _ = fmt.Fprintf(stdout, "%s: commit subject does not trigger a release. Skipping.\n", label)
			continue
		}

		nextVersion, err := semver.NextVersion(latestVersion, bump)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		newTag := prefix + nextVersion

		_, _ = fmt.Fprintf(stdout, "%s: bumping %s to %s for %s.\n", label, latestTag, newTag, sha)
		if err := gittag.CreateTag(repoPath, newTag, sha); err != nil {
			_, _ = fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		if err := gittag.PushTag(repoPath, remoteURL, newTag, username, password); err != nil {
			_, _ = fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	}

	return 0
}

func main() {
	repo := flag.String("repo", ".", "path to the local repository")
	config := flag.String("config", ".gitlab/versioning.yml", "path to the versioning configuration file")
	sha := flag.String("sha", "", "commit SHA to evaluate and tag")
	remoteURL := flag.String("remote-url", "", "URL of the remote to which tags are pushed")
	username := flag.String("username", "", "HTTP basic auth username for the push")
	flag.Parse()

	os.Exit(run(*repo, *config, *sha, *remoteURL, *username, os.Getenv("TAG_PUSH_TOKEN"), os.Stdout, os.Stderr))
}
