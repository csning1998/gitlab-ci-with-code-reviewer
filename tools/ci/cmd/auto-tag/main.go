// Command auto-tag evaluates Conventional Commit subjects to publish Semantic Version
// tags across configured repository modules via the GitLab REST API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	ciconfig "ci-tools/internal/config"
	"ci-tools/internal/gitlabapi"
	"ci-tools/internal/gitlabtoken"
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

// autoTagRequest holds the inputs of one auto-tag run.
type autoTagRequest struct {
	repoPath   string
	configPath string
	sha        string
	apiBaseURL string
	projectID  string
	password   string
}

// validate rejects a request which lacks a flag or the tag push token.
func (r autoTagRequest) validate() error {
	if r.sha == "" || r.apiBaseURL == "" || r.projectID == "" {
		return errors.New("--sha, --api-url, and --project-id are required")
	}
	if r.password == "" {
		return fmt.Errorf("%s must be set in the environment", ciconfig.TagPushTokenEnvName)
	}
	return nil
}

// tagRun carries the state shared by the module evaluations of one commit.
type tagRun struct {
	req       autoTagRequest
	repo      *git.Repository
	bump      semver.Bump
	parent    string
	hasParent bool
	stdout    io.Writer
	stderr    io.Writer
	// The token check runs before the first tag creation alone, which spares the self request on runs without a release.
	token gitlabtoken.FineGrainedToken
}

// loadVersioningConfig falls back to whole-repository tagging when the file is missing.
// Parse errors remain fatal to prevent masking invalid configurations.
func loadVersioningConfig(configPath string, stdout io.Writer) (versiontag.Config, error) {
	cfg, err := versiontag.LoadConfig(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		_, _ = fmt.Fprintf(stdout, "No versioning config at %q. Applying whole-repository defaults.\n", configPath)
		return versiontag.DefaultConfig(), nil
	}
	return cfg, err
}

// newTagRun opens the repository and resolves the release bump and the parent of the commit.
func newTagRun(req autoTagRequest, stdout, stderr io.Writer) (*tagRun, error) {
	repo, err := git.PlainOpen(req.repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository at path %q: %w", req.repoPath, err)
	}
	subject, err := resolveCommitSubject(repo, req.sha)
	if err != nil {
		return nil, err
	}
	parent, hasParent, err := resolveFirstParentSHA(repo, req.sha)
	if err != nil {
		return nil, err
	}
	return &tagRun{
		req:       req,
		repo:      repo,
		bump:      semver.DetermineBump(subject),
		parent:    parent,
		hasParent: hasParent,
		stdout:    stdout,
		stderr:    stderr,
	}, nil
}

// moduleChanged reports whether the commit touches the module directory.
// Every module counts as changed on a root commit, because a root commit lacks a parent tree for the diff.
func (r *tagRun) moduleChanged(mod versiontag.Module) (bool, error) {
	if !r.hasParent {
		return true, nil
	}
	return versiontag.DetectDirectoryTreeChanges(r.repo, r.parent, r.req.sha, mod.Dir)
}

// ensureToken classifies the tag push token once, before the first tag creation.
func (r *tagRun) ensureToken(ctx context.Context) error {
	if !r.token.IsZero() {
		return nil
	}
	token, err := gitlabtoken.New(ctx, r.req.apiBaseURL, r.req.password)
	if err != nil {
		return fmt.Errorf("failed to confirm that %s is a fine-grained personal access token: %w", ciconfig.TagPushTokenEnvName, err)
	}
	r.token = token
	return nil
}

// tagModule publishes the next version tag of one module when the commit changes the module and triggers a release.
func (r *tagRun) tagModule(ctx context.Context, mod versiontag.Module) error {
	label := mod.Name
	if label == "" {
		label = "<repository>"
	}
	changed, err := r.moduleChanged(mod)
	if err != nil {
		return err
	}
	if !changed {
		_, _ = fmt.Fprintf(r.stdout, "%s: no changes under %q. Skipping.\n", label, mod.Dir)
		return nil
	}

	prefix := mod.Prefix()
	latestTag, latestVersion, err := versiontag.LatestTag(r.repo, prefix)
	if err != nil {
		return err
	}
	if r.bump == semver.BumpNone {
		_, _ = fmt.Fprintf(r.stdout, "%s: commit subject does not trigger a release. Skipping.\n", label)
		return nil
	}
	nextVersion, err := semver.NextVersion(latestVersion, r.bump)
	if err != nil {
		return err
	}
	newTag := prefix + nextVersion

	if err := r.ensureToken(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(r.stdout, "%s: bumping %s to %s for %s.\n", label, latestTag, newTag, r.req.sha)
	if err := gitlabapi.CreateTag(ctx, r.req.apiBaseURL, r.req.projectID, newTag, r.req.sha, r.token); err != nil {
		return err
	}
	// Mirror tag locally to supply updated ref state for sequential module version evaluation.
	// Remote REST API creation is authoritative, and a failed local ref write does not stop the release.
	if err := gittag.CreateTag(r.req.repoPath, newTag, r.req.sha); err != nil {
		_, _ = fmt.Fprintln(r.stderr, "Warning:", err)
	}
	return nil
}

// executeAutoTag processes module version bumps and publishes tags for a given commit.
// Logic is decoupled from flags and process termination to permit direct unit testing.
func executeAutoTag(ctx context.Context, req autoTagRequest, stdout, stderr io.Writer) int {
	if err := req.validate(); err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	cfg, err := loadVersioningConfig(req.configPath, stdout)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	run, err := newTagRun(req, stdout, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	for _, mod := range cfg.Modules {
		if err := run.tagModule(ctx, mod); err != nil {
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
	apiBaseURL := flag.String("api-url", "", "GitLab API root, e.g. https://gitlab.com/api/v4")
	projectID := flag.String("project-id", "", "GitLab project ID owning the tags")
	flag.Parse()

	req := autoTagRequest{
		repoPath:   *repo,
		configPath: *config,
		sha:        *sha,
		apiBaseURL: *apiBaseURL,
		projectID:  *projectID,
		password:   os.Getenv(ciconfig.TagPushTokenEnvName),
	}
	os.Exit(executeAutoTag(context.Background(), req, os.Stdout, os.Stderr))
}
