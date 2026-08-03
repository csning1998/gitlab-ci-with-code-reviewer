package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	cases := []struct {
		name       string
		latest     string
		subject    string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"missing latest", "", "fix: x", 1, "", "--latest and --subject are required"},
		{"missing subject", "1.4.2", "", 1, "", "--latest and --subject are required"},
		{"feat triggers minor bump", "1.4.2", "feat(x): y", 0, "1.5.0", ""},
		{"ci type yields no release", "1.4.2", "ci(release): add automated semver tagging", 0, "NONE", ""},
		{"malformed latest surfaces an error", "bogus", "fix: x", 1, "", "is not in MAJOR.MINOR.PATCH form"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(c.latest, c.subject, &stdout, &stderr)

			if code != c.wantCode {
				t.Errorf("run(...) code = %d, want %d", code, c.wantCode)
			}
			if c.wantStdout != "" && strings.TrimSpace(stdout.String()) != c.wantStdout {
				t.Errorf("run(...) stdout = %q, want %q", stdout.String(), c.wantStdout)
			}
			if c.wantStderr != "" && !strings.Contains(stderr.String(), c.wantStderr) {
				t.Errorf("run(...) stderr = %q, want it to contain %q", stderr.String(), c.wantStderr)
			}
		})
	}
}

// TestMainSubprocess validates the execution path of the main function by re-invoking
// the test executable within a distinct child process configured with specific command-line
// arguments. This mechanism isolates calls to os.Exit, thereby permitting safe assertion
// without disrupting the execution environment of the parent test runner.
func TestMainSubprocess(t *testing.T) {
	if os.Getenv("BE_MR_SEMVER_RESOLVER") == "1" {
		os.Args = []string{"mr-semver-resolver", "--latest=1.4.2", "--subject=feat(x): y"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainSubprocess")
	cmd.Env = append(os.Environ(), "BE_MR_SEMVER_RESOLVER=1")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess exited with error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "1.5.0" {
		t.Errorf("subprocess stdout = %q, want %q", got, "1.5.0")
	}
}
