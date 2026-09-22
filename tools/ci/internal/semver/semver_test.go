package semver

import (
	"fmt"
	"math"
	"testing"
)

func TestDetermineBump(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		want    Bump
	}{
		{"feat", "feat(modules): add kvm-provisioning and vault-provisioning module packages", BumpMinor},
		{"feat with no scope", "feat: add retry logic", BumpMinor},
		{"fix", "fix(ci): resolve module publish job collisions", BumpPatch},
		{"perf", "perf(reviewer): reduce token usage", BumpPatch},
		{"ci type yields no release", "ci(release): add automated semver tagging", BumpNone},
		{"refactor type yields no release", "refactor(ci): consolidate includes", BumpNone},
		{"chore type yields no release", "chore: bump dependency", BumpNone},
		{"adhoc type yields no release", "adhoc: patch runner config on the fly", BumpNone},
		{"feat with exclamation mark", "feat(api)!: change auth flow", BumpMajor},
		{"fix with exclamation mark and scope", "fix(api)!: remove deprecated field", BumpMajor},
		{"subject without a Conventional Commit header", "bump go.mod dependencies", BumpNone},
		{
			"a subject merely describing the convention in prose does not itself trigger a major bump",
			"ci: add automated Semantic Version tagging",
			BumpNone,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetermineBump(c.subject)
			if got != c.want {
				t.Errorf("DetermineBump(%q) = %q, want %q", c.subject, got, c.want)
			}
		})
	}
}

func TestNextVersion(t *testing.T) {
	okCases := []struct {
		name   string
		latest string
		bump   Bump
		want   string
	}{
		{"minor bump", "1.4.2", BumpMinor, "1.5.0"},
		{"patch bump", "1.4.2", BumpPatch, "1.4.3"},
		{"major bump resets minor and patch", "1.4.2", BumpMajor, "2.0.0"},
		{"patch bump from zero", "0.0.0", BumpPatch, "0.0.1"},
	}

	for _, c := range okCases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NextVersion(c.latest, c.bump)
			if err != nil {
				t.Fatalf("NextVersion(%q, %q) returned error: %v", c.latest, c.bump, err)
			}
			if got != c.want {
				t.Errorf("NextVersion(%q, %q) = %q, want %q", c.latest, c.bump, got, c.want)
			}
		})
	}

	errCases := []struct {
		name   string
		latest string
		bump   Bump
	}{
		{"not three segments", "1.2", BumpPatch},
		{"non-numeric segment", "1.x.2", BumpPatch},
		{"unstructured input", "bogus", BumpPatch},
		{"bump none is not a valid target", "1.4.2", BumpNone},
	}

	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NextVersion(c.latest, c.bump); err == nil {
				t.Errorf("NextVersion(%q, %q) expected an error, got none", c.latest, c.bump)
			}
		})
	}
}

func TestParseVersion_RejectsNonCanonicalInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "two components", input: "1.2"},
		{name: "four components", input: "1.2.3.4"},
		{name: "empty middle component", input: "1..3"},
		{name: "negative major", input: "-1.0.0"},
		{name: "negative minor", input: "1.-1.0"},
		{name: "negative patch", input: "1.0.-1"},
		{name: "plus signed major", input: "+1.0.0"},
		{name: "plus signed patch", input: "1.0.+3"},
		{name: "leading space", input: " 1.0.0"},
		{name: "trailing space", input: "1.0.0 "},
		{name: "trailing newline", input: "1.0.0\n"},
		{name: "prerelease suffix", input: "1.2.3-rc1"},
		{name: "build metadata suffix", input: "1.2.3+build.5"},
		{name: "hexadecimal component", input: "0x1.0.0"},
		{name: "underscore digit separator", input: "1_0.0.0"},
		{name: "fullwidth digits", input: "１.０.０"},
		{name: "int64 overflow major", input: "9223372036854775808.0.0"},
		{name: "int64 overflow patch", input: "0.0.99999999999999999999"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			major, minor, patch, err := ParseVersion(tc.input)
			if err == nil {
				t.Fatalf("ParseVersion(%q) = %d.%d.%d, want a rejection", tc.input, major, minor, patch)
			}
		})
	}
}

func TestDetermineBump_SubjectBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		want    Bump
	}{
		{name: "empty subject", subject: "", want: BumpNone},
		{name: "uppercase type", subject: "FEAT: x", want: BumpNone},
		{name: "capitalized type", subject: "Feat: x", want: BumpNone},
		{name: "leading space is trimmed", subject: " feat: x", want: BumpMinor},
		{name: "leading byte order mark", subject: "\ufeff" + "feat: x", want: BumpNone},
		{name: "fullwidth colon", subject: "feat： x", want: BumpNone},
		{name: "fullwidth type letters", subject: "ｆｅａｔ: x", want: BumpNone},
		{name: "bang before scope", subject: "feat!(api): x", want: BumpNone},
		{name: "two scopes", subject: "feat(a)(b): x", want: BumpNone},
		{name: "scope containing closing paren", subject: "feat(a)b): x", want: BumpNone},
		{name: "empty scope", subject: "feat(): x", want: BumpMinor},
		{name: "bang after empty scope", subject: "feat()!: x", want: BumpMajor},
		{name: "bang without type", subject: "!: x", want: BumpNone},
		{name: "type only", subject: "feat", want: BumpNone},
		{name: "colon only", subject: ":", want: BumpNone},
		{name: "revert wrapping feat", subject: "revert: feat: x", want: BumpNone},
		{name: "multiline keeps first line semantics", subject: "chore: x\nfeat!: y", want: BumpNone},
		{name: "breaking chore", subject: "chore!: x", want: BumpMajor},
		{name: "breaking docs with scope", subject: "docs(readme)!: x", want: BumpMajor},
		{name: "perf patch", subject: "perf(core): x", want: BumpPatch},
		{name: "scope with spaces", subject: "fix(api gateway): x", want: BumpPatch},
		{name: "scope with unicode", subject: "fix(\u8a2d\u5b9a): x", want: BumpPatch},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetermineBump(tc.subject); got != tc.want {
				t.Errorf("DetermineBump(%q) = %q, want %q", tc.subject, got, tc.want)
			}
		})
	}
}

func TestNextVersion_RejectsIntegerOverflow(t *testing.T) {
	tests := []struct {
		name   string
		latest string
		bump   Bump
	}{
		{name: "patch at max", latest: fmt.Sprintf("0.0.%d", math.MaxInt), bump: BumpPatch},
		{name: "minor at max", latest: fmt.Sprintf("0.%d.0", math.MaxInt), bump: BumpMinor},
		{name: "major at max", latest: fmt.Sprintf("%d.0.0", math.MaxInt), bump: BumpMajor},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextVersion(tc.latest, tc.bump)
			if err == nil {
				t.Fatalf("NextVersion(%q, %q) = %q, want a rejection instead of a wrapped negative component", tc.latest, tc.bump, got)
			}
		})
	}
}

func FuzzParseVersionRoundTrip(f *testing.F) {
	for _, seed := range []string{"0.0.0", "1.2.3", "-1.0.0", "+1.0.0", "1.2", "a.b.c", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, version string) {
		major, minor, patch, err := ParseVersion(version)
		if err != nil {
			return
		}
		if major < 0 || minor < 0 || patch < 0 {
			t.Fatalf("ParseVersion(%q) accepted negative components %d.%d.%d", version, major, minor, patch)
		}
		if canonical := fmt.Sprintf("%d.%d.%d", major, minor, patch); canonical != version {
			t.Fatalf("ParseVersion(%q) accepted a non canonical spelling of %q", version, canonical)
		}
	})
}
