// Package semver derives the next Semantic Version tag from a Conventional Commit
// subject and body, following the Angular commit-analyzer release-rule convention.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Bump string

const (
	BumpMajor Bump = "major"
	BumpMinor Bump = "minor"
	BumpPatch Bump = "patch"
	BumpNone  Bump = "none"
)

var (
	breakingSubjectPattern = regexp.MustCompile(`^[a-z]+(\([^)]+\))?!:`)
	typeSubjectPattern     = regexp.MustCompile(`^([a-z]+)(\([^)]+\))?!?:`)
)

// DetermineBump classifies a commit subject and body into a target version bump level.
// Exclamation markers (`!`) in the header and `BREAKING CHANGE` footers in the body take
// precedence over default type-based bump logic.
func DetermineBump(subject, body string) Bump {
	if breakingSubjectPattern.MatchString(subject) || strings.Contains(body, "BREAKING CHANGE") {
		return BumpMajor
	}

	match := typeSubjectPattern.FindStringSubmatch(subject)
	if match == nil {
		return BumpNone
	}

	switch match[1] {
	case "feat":
		return BumpMinor
	case "fix", "perf":
		return BumpPatch
	default:
		return BumpNone
	}
}

// NextVersion applies bump to a MAJOR.MINOR.PATCH version string.
func NextVersion(latest string, bump Bump) (string, error) {
	parts := strings.Split(latest, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("latest version %q is not in MAJOR.MINOR.PATCH form", latest)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid major version in %q: %w", latest, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid minor version in %q: %w", latest, err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid patch version in %q: %w", latest, err)
	}

	switch bump {
	case BumpMajor:
		major++
		minor = 0
		patch = 0
	case BumpMinor:
		minor++
		patch = 0
	case BumpPatch:
		patch++
	default:
		return "", fmt.Errorf("cannot compute next version for bump %q", bump)
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}
