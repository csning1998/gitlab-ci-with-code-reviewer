// Command verify-tag-token validates that TAG_PUSH_TOKEN can read the repository tags of the project.
// The read-only check runs in MR pipelines before merge. Tag creation waits until after merge
// because a squash merge assigns a new commit SHA.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"ci-tools/internal/gitlabapi"
)

// executeVerifyTagToken validates the inputs and delegates the tag read to gitlabapi.VerifyTagAccess.
//
// An empty token exits zero because unprotected source branch pipelines withhold protected variables.
func executeVerifyTagToken(apiBaseURL, projectID, token string, stdout, stderr io.Writer) int {
	if apiBaseURL == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --api-url is required.")
		return 1
	}
	if projectID == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --project-id is required.")
		return 1
	}
	if token == "" {
		_, _ = fmt.Fprintln(stdout, "Warning: TAG_PUSH_TOKEN is unavailable in the current pipeline context. Tag access verification is skipped.")
		return 0
	}

	if err := gitlabapi.VerifyTagAccess(apiBaseURL, projectID, token); err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	return 0
}

func main() {
	apiBaseURL := flag.String("api-url", "", "GitLab API root, e.g. https://gitlab.com/api/v4")
	projectID := flag.String("project-id", "", "numeric ID or URL-encoded path of the project, e.g. $CI_PROJECT_ID")
	flag.Parse()

	os.Exit(executeVerifyTagToken(*apiBaseURL, *projectID, os.Getenv("TAG_PUSH_TOKEN"), os.Stdout, os.Stderr))
}
