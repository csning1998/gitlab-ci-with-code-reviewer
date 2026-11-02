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

// executeVerifyTagToken validates input parameters and delegates scope verification to gitlabapi.VerifyScope.
//
// An empty token exits zero because unprotected source branch pipelines withhold protected variables.
func executeVerifyTagToken(apiBaseURL, requiredScope, token string, stdout, stderr io.Writer) int {
	if apiBaseURL == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --api-url is required.")
		return 1
	}
	if token == "" {
		_, _ = fmt.Fprintln(stdout, "Warning: TAG_PUSH_TOKEN unavailable in current pipeline context; skipping scope verification.")
		return 0
	}

	if err := gitlabapi.VerifyScope(apiBaseURL, token, requiredScope); err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	return 0
}

func main() {
	apiBaseURL := flag.String("api-url", "", "GitLab API root, e.g. https://gitlab.com/api/v4")
	scope := flag.String("scope", "api", "token scope required by auto-tag")
	flag.Parse()

	os.Exit(executeVerifyTagToken(*apiBaseURL, *scope, os.Getenv("TAG_PUSH_TOKEN"), os.Stdout, os.Stderr))
}
