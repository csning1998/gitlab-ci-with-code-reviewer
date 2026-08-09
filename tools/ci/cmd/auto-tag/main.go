// Command auto-tag evaluates Conventional Commit subjects to publish Semantic Version
// tags across configured repository modules via the GitLab REST API.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"ci-tools/internal/gitlabapi"
	"ci-tools/internal/gittag"
	"ci-tools/internal/semver"
	"ci-tools/internal/versiontag"
)

// resolveCommitSubject returns the subject line of a commit SHA for Conventional Commit header parsing.
func resolveCommitSubject(repo *git.Repository, sha string) (string, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", fmt.Errorf("failed to resolve commit %q: %w", sha, err)
	}
	subject, _, _ := strings.Cut(commit.Message, "\n")
	return subject, nil
}

// resolveFirstParentSHA locates the first parent SHA of a commit to establish baseline trees for directory diffs.
func resolveFirstParentSHA(repo *git.Repository, sha string) (string, bool, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve commit %q: %w", sha, err)
	}
	if len(commit.ParentHashes) == 0 {
		return "", false, nil
	}
	return commit.ParentHashes[0].String(), true, nil
}

// executeAutoTag processes module version bumps and publishes tags for a given commit.
// Logic is decoupled from flags and process termination to permit direct unit testing.
func executeAutoTag(repoPath, configPath, sha, apiBaseURL, projectID, password string, stdout, stderr io.Writer) int {
	if sha == "" || apiBaseURL == "" || projectID == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --sha, --api-url, and --project-id are required.")
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

	subject, err := resolveCommitSubject(repo, sha)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	bump := semver.DetermineBump(subject)

	parent, hasParent, err := resolveFirstParentSHA(repo, sha)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	for _, mod := range cfg.Modules {
		label := mod.Name
		if label == "" {
			label = "<repository>"
		}

		// Root commits have no parent tree for diff evaluation. Mark all modules changed to guarantee initial tag coverage.
		changed := true
		if hasParent {
			changed, err = versiontag.DetectDirectoryTreeChanges(repo, parent, sha, mod.Dir)
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
		if err := gitlabapi.CreateTag(apiBaseURL, projectID, newTag, sha, password); err != nil {
			_, _ = fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		// Mirror tag locally to supply updated ref state for sequential module version evaluation.
		// Remote REST API creation is authoritative; local ref writes are non-fatal to release execution.
		if err := gittag.CreateTag(repoPath, newTag, sha); err != nil {
			_, _ = fmt.Fprintln(stderr, "Warning:", err)
		}
	}

	return 0
}

func main() {
	repo := flag.String("repo", ".", "path to the local repository")
	config := flag.String("config", ".gitlab/versioning.yml", "path to the versioning configuration file")
	sha := flag.String("sha", "", "commit SHA to evaluate and tag")
	apiBaseURL := flag.String("api-url", "", "GitLab API root, e.g. https://gitlab.com/api/v4")
	projectID := flag.String("project-id", "", "GitLab project ID owning the tags")
	flag.Parse()

	os.Exit(executeAutoTag(*repo, *config, *sha, *apiBaseURL, *projectID, os.Getenv("TAG_PUSH_TOKEN"), os.Stdout, os.Stderr))
}
