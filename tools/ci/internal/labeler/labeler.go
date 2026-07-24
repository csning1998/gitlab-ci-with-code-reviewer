// Package labeler determines and applies deterministic merge request labels (type::*,
// breaking-change, area::*) based on Conventional Commit metadata and changed file paths.
package labeler

import (
	"fmt"
	"regexp"
	"strings"

	"ci-tools/internal/gitlab"
)

// commitTypeToLabel maps Conventional Commit types to corresponding group labels.
// Unmapped types (build, chore, ci, revert, style) default to type::ad-hoc.
var commitTypeToLabel = map[string]string{
	"feat":     "type::feature",
	"fix":      "type::fix",
	"docs":     "type::documentation",
	"refactor": "type::refactor",
	"test":     "type::test",
	"perf":     "type::enhancement",
	"build":    "type::ad-hoc",
	"chore":    "type::ad-hoc",
	"ci":       "type::ad-hoc",
	"revert":   "type::ad-hoc",
	"style":    "type::ad-hoc",
}

// conventionalHeaderRe parses Conventional Commit headers for type, optional scope, and breaking change indicators.
var conventionalHeaderRe = regexp.MustCompile(`^([a-z]+)(\([^)]*\))?(!)?:\s`)

// breakingChangeFooterRe matches the Conventional Commits BREAKING CHANGE footer convention.
var breakingChangeFooterRe = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:`)

type areaRule struct {
	label   string
	pattern *regexp.Regexp
}

// areaRules maps file paths to area::* labels via path pattern heuristics.
var areaRules = []areaRule{
	{"area::infrastructure", regexp.MustCompile(`(^|/)terraform/|\.tf$|\.tfvars$|\.hcl$|\.tofu$`)},
	{"area::CI", regexp.MustCompile(`(^|/)\.gitlab-ci\.yml$|(^|/)templates/.*\.ya?ml$|(^|/)tools/ci/`)},
	{"area::frontend", regexp.MustCompile(`(^|/)frontend/`)},
	{"area::backend", regexp.MustCompile(`(^|/)backend/`)},
	{"area::observability", regexp.MustCompile(`(^|/)(grafana|prometheus|monitoring|observability)/`)},
}

// commitTypeLabel extracts the type::* label from the merge request title, returning "" if unmapped or invalid.
func commitTypeLabel(title string) string {
	m := conventionalHeaderRe.FindStringSubmatch(strings.TrimSpace(title))
	if m == nil {
		return ""
	}
	return commitTypeToLabel[m[1]]
}

// isBreakingChange identifies breaking changes via title header "!" markers or description footers.
func isBreakingChange(title, description string) bool {
	m := conventionalHeaderRe.FindStringSubmatch(strings.TrimSpace(title))
	if m != nil && m[3] == "!" {
		return true
	}
	return breakingChangeFooterRe.MatchString(description)
}

// resolveAreaLabels derives deduplicated area::* labels from changed file paths.
func resolveAreaLabels(changes []gitlab.Change) []string {
	seen := map[string]bool{}
	var labels []string
	for _, ch := range changes {
		path := ch.NewPath
		if path == "" {
			path = ch.OldPath
		}
		for _, rule := range areaRules {
			if !seen[rule.label] && rule.pattern.MatchString(path) {
				seen[rule.label] = true
				labels = append(labels, rule.label)
			}
		}
	}
	return labels
}

// Labeler applies deterministic merge request labels via an injected GitLab client.
type Labeler struct {
	gitlab *gitlab.Client
}

func New(gl *gitlab.Client) *Labeler {
	return &Labeler{gitlab: gl}
}

// RunOnMR is the shared entrypoint for the mr-labeler binary.
func RunOnMR(apiURL, projectID, mriid, token string) error {
	return New(gitlab.New(apiURL, projectID, mriid, token)).Run()
}

func (l *Labeler) Run() error {
	mr, err := l.gitlab.FetchMR()
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}

	var labels []string
	if lbl := commitTypeLabel(mr.Title); lbl != "" {
		labels = append(labels, lbl)
	}
	if isBreakingChange(mr.Title, mr.Description) {
		labels = append(labels, "breaking-change")
	}
	labels = append(labels, resolveAreaLabels(mr.Changes)...)

	if len(labels) == 0 {
		fmt.Println("No deterministic labels matched.")
		return nil
	}
	if _, err := l.gitlab.AddLabels(labels); err != nil {
		return fmt.Errorf("apply labels: %w", err)
	}
	fmt.Printf("Labels applied: %s\n", strings.Join(labels, ", "))
	return nil
}
