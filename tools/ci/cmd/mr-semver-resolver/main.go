// Command-line utility that derives the target Semantic Version tag from a Conventional Commit
// subject relative to a specified baseline version. Outputs NONE if the commit type triggers
// no release, delegating publishing logic to the invoking workflow.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"ci-tools/internal/semver"
)

// run evaluates the provided latest and subject parameters to determine the resulting exit
// code and output stream, decoupling execution logic from flag parsing and os.Exit to
// facilitate direct unit testing. latest is accepted with or without a leading v regardless
// of vPrefix, and vPrefix controls only whether the printed result carries one, since the
// choice belongs to the calling project rather than to this tool.
func run(latest, subject string, vPrefix bool, stdout, stderr io.Writer) int {
	if latest == "" || subject == "" {
		_, _ = fmt.Fprintln(stderr, "Error: --latest and --subject are required.")
		return 1
	}

	bump := semver.DetermineBump(subject)
	if bump == semver.BumpNone {
		_, _ = fmt.Fprintln(stdout, "NONE")
		return 0
	}

	trimmedLatest := strings.TrimPrefix(strings.TrimPrefix(latest, "v"), "V")
	next, err := semver.NextVersion(trimmedLatest, bump)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if vPrefix {
		next = "v" + next
	}
	_, _ = fmt.Fprintln(stdout, next)
	return 0
}

func main() {
	latest := flag.String("latest", "", "current latest Semantic Version tag, e.g. X.Y.Z or vX.Y.Z")
	subject := flag.String("subject", "", "commit subject line")
	vPrefix := flag.Bool("v-prefix", false, "prepend v to the printed version, e.g. for a Go module consumer")
	flag.Parse()

	os.Exit(run(*latest, *subject, *vPrefix, os.Stdout, os.Stderr))
}
