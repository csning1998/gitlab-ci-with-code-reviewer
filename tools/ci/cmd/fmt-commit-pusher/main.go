// Command fmt-commit-pusher commits and pushes formatting changes to a merge request source branch.
// This consolidates the Git remote configuration, working tree status check, commit creation, and push
// sequence previously duplicated across CI/CD templates.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"ci-tools/internal/fmtpush"
)

// run evaluates the working tree at repoPath for modifications and pushes resulting commits to branch.
// Decoupling execution logic from flag parsing, environment access, and process termination enables
// direct unit testing.
func run(repoPath, message, branch, remoteURL, username, password string, stdout, stderr io.Writer) int {
	if message == "" || branch == "" || remoteURL == "" || username == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --message, --branch, --remote-url, and --username are required.")
		return 1
	}
	if password == "" {
		_, _ = fmt.Fprintln(stderr, "Error: CI_JOB_TOKEN must be set in the environment.")
		return 1
	}

	changed, err := fmtpush.HasChanges(repoPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if !changed {
		_, _ = fmt.Fprintln(stdout, "No formatting changes detected. Skipping commit and push.")
		return 0
	}

	if err := fmtpush.CommitAndPush(repoPath, message, branch, remoteURL, username, password); err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "Pushed formatting commit %q to %s.\n", message, branch)
	return 0
}

func main() {
	repo := flag.String("repo", ".", "path to the local repository")
	message := flag.String("message", "", "commit message for the formatting change")
	branch := flag.String("branch", os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"), "branch to push the commit to")
	remoteURL := flag.String("remote-url", "", "URL of the remote to which the commit is pushed")
	username := flag.String("username", "gitlab-ci-token", "HTTP basic auth username for the push")
	flag.Parse()

	os.Exit(run(*repo, *message, *branch, *remoteURL, *username, os.Getenv("CI_JOB_TOKEN"), os.Stdout, os.Stderr))
}
