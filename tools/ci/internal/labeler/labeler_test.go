package labeler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ci-tools/internal/gitlab"
)

func TestResolveCommitTypeLabel_KnownTypes(t *testing.T) {
	cases := map[string]string{
		"feat: add thing":     "type::feature",
		"fix: correct bug":    "type::fix",
		"docs: update readme": "type::documentation",
		"refactor: simplify":  "type::refactor",
		"test: add coverage":  "type::test",
		"perf: speed up":      "type::enhancement",
		"build: bump deps":    "type::ad-hoc",
		"chore: housekeeping": "type::ad-hoc",
		"ci: tweak pipeline":  "type::ad-hoc",
		"revert: undo change": "type::ad-hoc",
		"style: reformat":     "type::ad-hoc",
	}
	for title, want := range cases {
		if got := resolveCommitTypeLabel(title); got != want {
			t.Errorf("resolveCommitTypeLabel(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestResolveCommitTypeLabel_WithScope(t *testing.T) {
	if got := resolveCommitTypeLabel("feat(api): add endpoint"); got != "type::feature" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want type::feature with a scope present", got)
	}
}

func TestResolveCommitTypeLabel_UnknownType(t *testing.T) {
	if got := resolveCommitTypeLabel("wip: unfinished work"); got != "" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want empty for an unmapped type", got)
	}
}

func TestResolveCommitTypeLabel_NoConventionalPrefix(t *testing.T) {
	if got := resolveCommitTypeLabel("Fix the thing"); got != "" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want empty when the header doesn't match the pattern", got)
	}
}

func TestResolveCommitTypeLabel_UppercaseTypeNotMatched(t *testing.T) {
	// The regexp requires a lowercase type; GitLab MR titles are free text and may not follow the
	// convention exactly.
	if got := resolveCommitTypeLabel("FEAT: add thing"); got != "" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want empty for an uppercase type", got)
	}
}

func TestResolveCommitTypeLabel_MissingSpaceAfterColon(t *testing.T) {
	if got := resolveCommitTypeLabel("feat:add thing"); got != "" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want empty when no space follows the colon", got)
	}
}

func TestResolveCommitTypeLabel_LeadingWhitespaceTrimmed(t *testing.T) {
	if got := resolveCommitTypeLabel("  feat: add thing"); got != "type::feature" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want type::feature after trimming leading whitespace", got)
	}
}

func TestResolveCommitTypeLabel_EmptyTitle(t *testing.T) {
	if got := resolveCommitTypeLabel(""); got != "" {
		t.Errorf("resolveCommitTypeLabel(\"\") = %q, want empty", got)
	}
}

func TestResolveCommitTypeLabel_EmptyScopeParens(t *testing.T) {
	if got := resolveCommitTypeLabel("feat(): add thing"); got != "type::feature" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want type::feature with empty scope parens", got)
	}
}

func TestResolveCommitTypeLabel_BreakingBangDoesNotAffectTypeMapping(t *testing.T) {
	if got := resolveCommitTypeLabel("fix!: breaking fix"); got != "type::fix" {
		t.Errorf("resolveCommitTypeLabel(...) = %q, want type::fix regardless of the breaking-change marker", got)
	}
}

func TestDetectBreakingChange_BangInTitle(t *testing.T) {
	if !detectBreakingChange("feat!: breaking change", "") {
		t.Error("detectBreakingChange(...) = false, want true for a \"!\" following the type")
	}
}

func TestDetectBreakingChange_BangAfterScope(t *testing.T) {
	if !detectBreakingChange("fix(api)!: breaking change", "") {
		t.Error("detectBreakingChange(...) = false, want true for a \"!\" following a scoped type")
	}
}

func TestDetectBreakingChange_FooterInDescription(t *testing.T) {
	if !detectBreakingChange("feat: add thing", "Some notes.\n\nBREAKING CHANGE: removes old API") {
		t.Error("detectBreakingChange(...) = false, want true when the description carries a BREAKING CHANGE footer")
	}
}

func TestDetectBreakingChange_HyphenatedFooterVariant(t *testing.T) {
	if !detectBreakingChange("feat: add thing", "BREAKING-CHANGE: removes old API") {
		t.Error("detectBreakingChange(...) = false, want true for the BREAKING-CHANGE hyphenated spelling")
	}
}

func TestDetectBreakingChange_FooterMustBeAtLineStart(t *testing.T) {
	if detectBreakingChange("feat: add thing", "This is not a BREAKING CHANGE: footer, just prose") {
		t.Error("detectBreakingChange(...) = true, want false when the marker is not anchored to a line start")
	}
}

func TestDetectBreakingChange_NeitherMarkerPresent(t *testing.T) {
	if detectBreakingChange("feat: add thing", "No breaking changes here.") {
		t.Error("detectBreakingChange(...) = true, want false with no breaking-change indicator")
	}
}

func TestDetectBreakingChange_BangWithoutConventionalType(t *testing.T) {
	if detectBreakingChange("Just a title!", "") {
		t.Error("detectBreakingChange(...) = true, want false since the title never matches the header pattern")
	}
}

func TestResolveAreaLabels_MatchesMultipleRulesDeduplicated(t *testing.T) {
	changes := []gitlab.Change{
		{NewPath: "terraform/main.tf"},
		{NewPath: "terraform/variables.tf"},
		{NewPath: "frontend/src/App.vue"},
	}
	labels := resolveAreaLabels(changes)
	if len(labels) != 2 {
		t.Fatalf("resolveAreaLabels(...) = %v, want 2 deduplicated labels", labels)
	}
	if labels[0] != "area::infrastructure" || labels[1] != "area::frontend" {
		t.Errorf("resolveAreaLabels(...) = %v, want [area::infrastructure area::frontend] in first-match order", labels)
	}
}

func TestResolveAreaLabels_CIToolsPath(t *testing.T) {
	changes := []gitlab.Change{{NewPath: "tools/ci/cmd/auto-tag/main.go"}}
	labels := resolveAreaLabels(changes)
	if len(labels) != 1 || labels[0] != "area::CI" {
		t.Errorf("resolveAreaLabels(...) = %v, want [area::CI]", labels)
	}
}

func TestResolveAreaLabels_ObservabilityDirectoryVariants(t *testing.T) {
	for _, path := range []string{"grafana/dashboard.json", "prometheus/rules.yml", "monitoring/alerts.yml", "observability/README.md"} {
		labels := resolveAreaLabels([]gitlab.Change{{NewPath: path}})
		if len(labels) != 1 || labels[0] != "area::observability" {
			t.Errorf("resolveAreaLabels(%q) = %v, want [area::observability]", path, labels)
		}
	}
}

func TestResolveAreaLabels_NoMatch(t *testing.T) {
	labels := resolveAreaLabels([]gitlab.Change{{NewPath: "README.md"}})
	if len(labels) != 0 {
		t.Errorf("resolveAreaLabels(...) = %v, want no labels", labels)
	}
}

func TestResolveAreaLabels_RenamedFile_FallsBackToOldPath(t *testing.T) {
	labels := resolveAreaLabels([]gitlab.Change{{NewPath: "", OldPath: "terraform/deleted.tf"}})
	if len(labels) != 1 || labels[0] != "area::infrastructure" {
		t.Errorf("resolveAreaLabels(...) = %v, want [area::infrastructure] via the old path for a deleted file", labels)
	}
}

func TestResolveAreaLabels_EmptyChangeSet(t *testing.T) {
	if labels := resolveAreaLabels(nil); len(labels) != 0 {
		t.Errorf("resolveAreaLabels(nil) = %v, want no labels", labels)
	}
}

func TestResolveAreaLabels_MidPathMatchNotAnchoredToRoot(t *testing.T) {
	// The "(^|/)" alternation matches a directory boundary anywhere in the path, not only at the
	// repository root, so a nested "backend/" segment still qualifies.
	labels := resolveAreaLabels([]gitlab.Change{{NewPath: "services/api/backend/handler.go"}})
	if len(labels) != 1 || labels[0] != "area::backend" {
		t.Errorf("resolveAreaLabels(...) = %v, want [area::backend] for a nested backend/ directory", labels)
	}
}

func TestResolveAreaLabels_PartialDirectoryNameNotMatched(t *testing.T) {
	// "backend-tools/" contains "backend" as a substring but not as a "/"-delimited path segment,
	// so the anchored alternation must not match it.
	labels := resolveAreaLabels([]gitlab.Change{{NewPath: "backend-tools/script.sh"}})
	if len(labels) != 0 {
		t.Errorf("resolveAreaLabels(...) = %v, want no match for a directory name that merely starts with \"backend\"", labels)
	}
}

// newTestGitLabServer wires an httptest server implementing the minimal GitLab MR API surface
// FetchMR and AddLabels depend on, recording the labels submitted via PUT.
// diffsJSON is the body of GET .../diffs (FetchMR reads paths from this endpoint, not from detail).
func newTestGitLabServer(t *testing.T, detail, diffsJSON string) (*httptest.Server, *map[string]any) {
	t.Helper()
	captured := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = w.Write([]byte(diffsJSON))
		case r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusOK)
		default:
			_, _ = w.Write([]byte(detail))
		}
	}))
	t.Cleanup(server.Close)
	return server, &captured
}

func TestLabeler_Execute_AppliesMatchedLabels(t *testing.T) {
	server, captured := newTestGitLabServer(t,
		`{"title":"feat!: add thing","description":"","diff_refs":{}}`,
		`[{"new_path":"terraform/main.tf","old_path":"terraform/main.tf","diff":""}]`,
	)
	l := New(gitlab.New(server.URL, "1", "2", "token"))

	if err := l.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	addLabels, _ := (*captured)["add_labels"].(string)
	for _, want := range []string{"type::feature", "breaking-change", "area::infrastructure"} {
		if !strings.Contains(addLabels, want) {
			t.Errorf("add_labels = %q, want it to contain %q", addLabels, want)
		}
	}
}

func TestLabeler_Execute_NoLabelsMatched_SkipsAPICall(t *testing.T) {
	server, captured := newTestGitLabServer(t,
		`{"title":"wip: unfinished","description":"","diff_refs":{}}`,
		`[{"new_path":"README.md","old_path":"README.md","diff":""}]`,
	)
	l := New(gitlab.New(server.URL, "1", "2", "token"))

	if err := l.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if len(*captured) != 0 {
		t.Errorf("captured PUT body = %v, want no AddLabels call when nothing matched", *captured)
	}
}

func TestLabeler_Execute_FetchMRFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	l := New(gitlab.New(server.URL, "1", "2", "token"))

	err := l.Execute()
	if err == nil {
		t.Fatal("Run() succeeded unexpectedly despite a GitLab API failure; want an error")
	}
	if !strings.Contains(err.Error(), "fetch MR changes") {
		t.Errorf("Run() error = %q, want it to mention the fetch failure", err.Error())
	}
}
