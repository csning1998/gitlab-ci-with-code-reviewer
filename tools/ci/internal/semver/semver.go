// Package semver derives the next Semantic Version tag from a Conventional Commit
// subject and body, following the Angular commit-analyzer release-rule convention.
package semver

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"ci-tools/internal/conventional"
)

type Bump string

const (
	BumpMajor Bump = "major"
	BumpMinor Bump = "minor"
	BumpPatch Bump = "patch"
	BumpNone  Bump = "none"
)

// DetermineBump categorizes a commit subject line into a Semantic Versioning increment level. Only
// the subject line is analyzed, because squash merge automation fills the commit body from merge
// request descriptions. An exclamation mark after the type or scope is the sole major bump marker.
func DetermineBump(subject string) Bump {
	header, ok := conventional.ParseHeader(subject)
	if !ok {
		return BumpNone
	}
	if header.Breaking {
		return BumpMajor
	}

	switch header.Type {
	case "feat":
		return BumpMinor
	case "fix", "perf":
		return BumpPatch
	default:
		return BumpNone
	}
}

// ParseVersion splits a MAJOR.MINOR.PATCH version string into its three integer components.
// Each component MUST be a canonical non negative decimal integer without sign or leading zero.
func ParseVersion(version string) (major, minor, patch int, err error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("version %q is not in MAJOR.MINOR.PATCH form", version)
	}

	major, err = parseComponent(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version in %q: %w", version, err)
	}
	minor, err = parseComponent(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version in %q: %w", version, err)
	}
	patch, err = parseComponent(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version in %q: %w", version, err)
	}

	return major, minor, patch, nil
}

// parseComponent accepts ASCII digits only, because strconv.Atoi also accepts a sign.
func parseComponent(component string) (int, error) {
	if component == "" {
		return 0, fmt.Errorf("component is empty")
	}
	for _, r := range component {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("component %q holds a non digit character", component)
		}
	}
	if len(component) > 1 && component[0] == '0' {
		return 0, fmt.Errorf("component %q has a leading zero", component)
	}
	return strconv.Atoi(component)
}

// NextVersion applies bump to a MAJOR.MINOR.PATCH version string.
func NextVersion(latest string, bump Bump) (string, error) {
	major, minor, patch, err := ParseVersion(latest)
	if err != nil {
		return "", err
	}

	switch bump {
	case BumpMajor:
		if major == math.MaxInt {
			return "", fmt.Errorf("major version of %q cannot be incremented", latest)
		}
		major++
		minor = 0
		patch = 0
	case BumpMinor:
		if minor == math.MaxInt {
			return "", fmt.Errorf("minor version of %q cannot be incremented", latest)
		}
		minor++
		patch = 0
	case BumpPatch:
		if patch == math.MaxInt {
			return "", fmt.Errorf("patch version of %q cannot be incremented", latest)
		}
		patch++
	default:
		return "", fmt.Errorf("cannot compute next version for bump %q", bump)
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}
