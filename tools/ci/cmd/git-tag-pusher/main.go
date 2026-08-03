// Command-line utility that generates a lightweight tag at a designated commit within an existing
// local repository and transmits the tag to a remote destination. Authentication credentials are
// retrieved from the TAG_PUSH_TOKEN environment variable rather than specified via command-line
// flags, as process arguments are exposed to arbitrary processes executing upon the host system,
// whereas environment variables mitigate credential exposure through process enumeration.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"ci-tools/internal/gittag"
)

// run creates and pushes the tag, decoupling execution logic from flag parsing, environment
// access, and os.Exit to facilitate direct unit testing.
func run(repo, remoteURL, tag, sha, username, password string, stderr io.Writer) int {
	if remoteURL == "" || tag == "" || sha == "" || username == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --remote-url, --tag, --sha, and --username are required.")
		return 1
	}
	if password == "" {
		_, _ = fmt.Fprintln(stderr, "Error: TAG_PUSH_TOKEN must be set in the environment.")
		return 1
	}

	if err := gittag.CreateTag(repo, tag, sha); err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if err := gittag.PushTag(repo, remoteURL, tag, username, password); err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	return 0
}

func main() {
	repo := flag.String("repo", ".", "path to the local repository")
	remoteURL := flag.String("remote-url", "", "URL of the remote to which the tag is pushed")
	tag := flag.String("tag", "", "tag name to create and push")
	sha := flag.String("sha", "", "commit SHA to which the tag should point")
	username := flag.String("username", "", "HTTP basic auth username for the push")
	flag.Parse()

	os.Exit(run(*repo, *remoteURL, *tag, *sha, *username, os.Getenv("TAG_PUSH_TOKEN"), os.Stderr))
}
