// Command-line utility that derives the target Semantic Version tag from a Conventional Commit
// subject relative to a specified baseline version. Outputs NONE if the commit type triggers
// no release, delegating publishing logic to the invoking workflow.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"ci-tools/internal/semver"
)

// run evaluates the provided latest and subject parameters to determine the resulting exit
// code and output stream, decoupling execution logic from flag parsing and os.Exit to
// facilitate direct unit testing.
func run(latest, subject string, stdout, stderr io.Writer) int {
	if latest == "" || subject == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --latest and --subject are required.")
		return 1
	}

	bump := semver.DetermineBump(subject)
	if bump == semver.BumpNone {
		_, _ = fmt.Fprintln(stdout, "NONE")
		return 0
	}

	next, err := semver.NextVersion(latest, bump)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, next)
	return 0
}

func main() {
	latest := flag.String("latest", "", "current latest Semantic Version tag, e.g. X.Y.Z")
	subject := flag.String("subject", "", "commit subject line")
	flag.Parse()

	os.Exit(run(*latest, *subject, os.Stdout, os.Stderr))
}
