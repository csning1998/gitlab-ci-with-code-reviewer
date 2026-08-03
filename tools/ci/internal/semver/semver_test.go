package semver

import "testing"

func TestDetermineBump(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		body    string
		want    Bump
	}{
		{"feat", "feat(modules): add kvm-provisioning and vault-provisioning module packages", "", BumpMinor},
		{"feat with no scope", "feat: add retry logic", "", BumpMinor},
		{"fix", "fix(ci): resolve module publish job collisions", "", BumpPatch},
		{"perf", "perf(reviewer): reduce token usage", "", BumpPatch},
		{"ci type yields no release", "ci(release): add automated semver tagging", "", BumpNone},
		{"refactor type yields no release", "refactor(ci): consolidate includes", "", BumpNone},
		{"chore type yields no release", "chore: bump dependency", "", BumpNone},
		{"feat with exclamation mark", "feat(api)!: change auth flow", "", BumpMajor},
		{"fix with breaking change footer", "fix(core): patch bug", "BREAKING CHANGE: removes legacy input", BumpMajor},
		{"fix with exclamation mark and scope", "fix(api)!: remove deprecated field", "", BumpMajor},
		{"subject without a Conventional Commit header", "bump go.mod dependencies", "", BumpNone},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetermineBump(c.subject, c.body)
			if got != c.want {
				t.Errorf("DetermineBump(%q, %q) = %q, want %q", c.subject, c.body, got, c.want)
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
