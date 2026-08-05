package review

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ci-tools/internal/gitlab"
)

var errBoom = errors.New("boom")

func TestExtractJSONArray_PlainArray(t *testing.T) {
	arr, err := extractJSONArray(`[{"file":"a.go"}]`)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("extractJSONArray(...) returned %d elements, want 1", len(arr))
	}
}

func TestExtractJSONArray_EmptyArray(t *testing.T) {
	arr, err := extractJSONArray(`[]`)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 0 {
		t.Errorf("extractJSONArray(...) returned %d elements, want 0", len(arr))
	}
}

func TestExtractJSONArray_MarkdownCodeFence(t *testing.T) {
	raw := "```json\n[{\"file\":\"a.go\"}]\n```"
	arr, err := extractJSONArray(raw)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("extractJSONArray(...) returned %d elements, want 1", len(arr))
	}
}

func TestExtractJSONArray_ConversationalPrefixText(t *testing.T) {
	raw := "Here is the review:\n[{\"file\":\"a.go\"}]"
	arr, err := extractJSONArray(raw)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("extractJSONArray(...) returned %d elements, want 1", len(arr))
	}
}

func TestExtractJSONArray_TrailingCommaTolerated(t *testing.T) {
	raw := `[{"file":"a.go"},]`
	arr, err := extractJSONArray(raw)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("extractJSONArray(...) returned %d elements, want 1", len(arr))
	}
}

func TestExtractJSONArray_TrailingCommaBeforeClosingBrace(t *testing.T) {
	raw := `[{"file":"a.go",}]`
	arr, err := extractJSONArray(raw)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("extractJSONArray(...) returned %d elements, want 1", len(arr))
	}
}

func TestExtractJSONArray_NoArrayPresent(t *testing.T) {
	if _, err := extractJSONArray("no json here"); err == nil {
		t.Error("extractJSONArray(...) succeeded unexpectedly on prose without any array; want an error")
	}
}

func TestExtractJSONArray_EmptyString(t *testing.T) {
	if _, err := extractJSONArray(""); err == nil {
		t.Error("extractJSONArray(\"\") succeeded unexpectedly; want an error")
	}
}

func TestExtractJSONArray_UnclosedArray(t *testing.T) {
	if _, err := extractJSONArray(`[{"file":"a.go"}`); err == nil {
		t.Error("extractJSONArray(...) succeeded unexpectedly on an unclosed array; want an error")
	}
}

func TestExtractJSONArray_BracketInsideProseBeforeRealArray(t *testing.T) {
	// The first "[" candidate (inside "list [of stuff]") fails to decode as a JSON array, so
	// extraction must continue scanning for a later "[" that succeeds.
	raw := `Findings list [of stuff] follow: [{"file":"a.go"}]`
	arr, err := extractJSONArray(raw)
	if err != nil {
		t.Fatalf("extractJSONArray(...) returned an unexpected error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("extractJSONArray(...) returned %d elements, want 1", len(arr))
	}
}

func TestExtractJSONArray_ErrorMessageTruncatesLongInput(t *testing.T) {
	raw := strings.Repeat("x", 5000)
	_, err := extractJSONArray(raw)
	if err == nil {
		t.Fatal("extractJSONArray(...) succeeded unexpectedly; want an error")
	}
	if len(err.Error()) > 400 {
		t.Errorf("extractJSONArray(...) error length = %d, want the raw input truncated to roughly 200 chars", len(err.Error()))
	}
}

func TestFormatMRIntent_EmptyTitleAndDescription(t *testing.T) {
	if got := formatMRIntent("", "", 100); got != "" {
		t.Errorf("formatMRIntent(\"\", \"\", 100) = %q, want empty string", got)
	}
}

func TestFormatMRIntent_TitleOnly(t *testing.T) {
	got := formatMRIntent("feat: add thing", "", 100)
	if !strings.Contains(got, "Title: feat: add thing") {
		t.Errorf("formatMRIntent(...) = %q, want it to contain the title", got)
	}
	if strings.Contains(got, "Description:") {
		t.Errorf("formatMRIntent(...) = %q, want no Description section for an empty description", got)
	}
}

func TestFormatMRIntent_TruncatesLongDescription(t *testing.T) {
	description := strings.Repeat("a", 50)
	// Budget must exceed the truncation marker length so the marker fits verbatim.
	got := formatMRIntent("", description, 30)
	if !strings.Contains(got, "... [truncated]") {
		t.Errorf("formatMRIntent(...) = %q, want a truncation marker", got)
	}
	if strings.Contains(got, strings.Repeat("a", 50)) {
		t.Errorf("formatMRIntent(...) = %q, want the description shortened below maxRunes", got)
	}
}

func TestFormatMRIntent_MaxRunesShorterThanMarker_DoesNotPanic(t *testing.T) {
	// maxRunes below the marker length previously sliced with a negative index.
	got := formatMRIntent("", strings.Repeat("a", 50), 5)
	if got == "" {
		t.Fatal("formatMRIntent(...) = empty, want a non-empty intent block")
	}
	if strings.Contains(got, strings.Repeat("a", 50)) {
		t.Errorf("formatMRIntent(...) = %q, want the long description truncated", got)
	}
}

func TestFormatMRIntent_DescriptionExactlyAtLimit_NotTruncated(t *testing.T) {
	description := strings.Repeat("a", 10)
	got := formatMRIntent("", description, 10)
	if strings.Contains(got, "[truncated]") {
		t.Errorf("formatMRIntent(...) = %q, want no truncation when the description is exactly at maxRunes", got)
	}
}

func TestFormatMRIntent_CJKRuneCountingNotByteCounting(t *testing.T) {
	// Each CJK character occupies 3 bytes in UTF-8; truncation must operate on rune counts so a
	// budget of 25 runes is not interpreted as 25 bytes (which would split a multi-byte rune).
	description := strings.Repeat("測", 40)
	got := formatMRIntent("", description, 25)
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("formatMRIntent(...) = %q, want a truncation marker", got)
	}
	if strings.Contains(got, strings.Repeat("測", 40)) {
		t.Errorf("formatMRIntent(...) = %q, want the description truncated", got)
	}
	// 40 CJK runes are 120 UTF-8 bytes; a byte-oriented cut at 25 would corrupt the string.
	if !strings.Contains(got, "測") {
		t.Errorf("formatMRIntent(...) = %q, want intact CJK runes after rune-based truncation", got)
	}
}

func TestFormatMRIntent_WhitespaceOnlyFieldsTreatedAsEmpty(t *testing.T) {
	if got := formatMRIntent("   ", "\t\n", 100); got != "" {
		t.Errorf("formatMRIntent(\"   \", \"\\t\\n\", 100) = %q, want empty string after trimming", got)
	}
}

func TestBuildCommentBody_NoSuggestion(t *testing.T) {
	got := buildCommentBody("something is wrong", "", 1, 1)
	if got != "something is wrong" {
		t.Errorf("buildCommentBody(...) = %q, want the description verbatim with no suggestion", got)
	}
}

func TestBuildCommentBody_WithSuggestion(t *testing.T) {
	got := buildCommentBody("fix this", "return nil", 1, 1)
	if !strings.Contains(got, "```suggestion") {
		t.Errorf("buildCommentBody(...) = %q, want a \"suggestion\" fenced code block", got)
	}
	if !strings.Contains(got, "return nil") {
		t.Errorf("buildCommentBody(...) = %q, want it to contain the suggested replacement", got)
	}
}

func TestBuildPosition_ExactEndLineMatch(t *testing.T) {
	n := 5
	info := fileInfo{oldPath: "a.go", lines: map[int]linePos{5: {newLine: &n}}}
	pos := buildPosition(gitlab.DiffRefs{BaseSha: "b", StartSha: "s", HeadSha: "h"}, "a.go", info, 5, 5)
	if pos == nil {
		t.Fatal("buildPosition(...) = nil, want a populated position map")
	}
	if pos["new_line"] != 5 {
		t.Errorf("pos[\"new_line\"] = %v, want 5", pos["new_line"])
	}
	if pos["base_sha"] != "b" || pos["start_sha"] != "s" || pos["head_sha"] != "h" {
		t.Errorf("pos = %v, want the diff refs propagated", pos)
	}
}

func TestBuildPosition_EndLineMissingFallsBackToStart(t *testing.T) {
	n := 3
	info := fileInfo{lines: map[int]linePos{3: {newLine: &n}}}
	// end=9 does not exist in the diff's line map, but start=3 does, so the position must
	// resolve to the start line instead of failing outright.
	pos := buildPosition(gitlab.DiffRefs{}, "a.go", info, 3, 9)
	if pos == nil {
		t.Fatal("buildPosition(...) = nil, want it to fall back to the start line")
	}
	if pos["new_line"] != 3 {
		t.Errorf("pos[\"new_line\"] = %v, want 3", pos["new_line"])
	}
}

func TestBuildPosition_NeitherLineFound_ReturnsNil(t *testing.T) {
	info := fileInfo{lines: map[int]linePos{}}
	if pos := buildPosition(gitlab.DiffRefs{}, "a.go", info, 3, 9); pos != nil {
		t.Errorf("buildPosition(...) = %v, want nil when neither line exists in the diff", pos)
	}
}

func TestBuildPosition_DeletionOnlyLine_OmitsNewLineKey(t *testing.T) {
	o := 7
	info := fileInfo{lines: map[int]linePos{7: {oldLine: &o}}}
	pos := buildPosition(gitlab.DiffRefs{}, "a.go", info, 7, 7)
	if pos == nil {
		t.Fatal("buildPosition(...) = nil, want a populated position map")
	}
	if _, ok := pos["new_line"]; ok {
		t.Errorf("pos = %v, want no \"new_line\" key for a pure deletion", pos)
	}
	if pos["old_line"] != 7 {
		t.Errorf("pos[\"old_line\"] = %v, want 7", pos["old_line"])
	}
}

func TestBuildCombinedDiff_SkipsLockFilesAndTracksSkipCount(t *testing.T) {
	changes := []gitlab.Change{
		{NewPath: "go.sum", Diff: "@@ -1,1 +1,1 @@\n+x\n"},
		{NewPath: "main.go", Diff: "@@ -1,1 +1,1 @@\n+x\n"},
	}
	combined, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 1 {
		t.Errorf("buildCombinedDiff(...) skipped = %d, want 1", skipped)
	}
	if len(meta) != 1 {
		t.Errorf("buildCombinedDiff(...) fileMeta has %d entries, want 1", len(meta))
	}
	if strings.Contains(combined, "go.sum") {
		t.Errorf("buildCombinedDiff(...) combined = %q, want the skipped file excluded", combined)
	}
}

func TestBuildCombinedDiff_SkipsEmptyDiff(t *testing.T) {
	changes := []gitlab.Change{{NewPath: "main.go", Diff: "   "}}
	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 1 || len(meta) != 0 {
		t.Errorf("buildCombinedDiff(...) = (meta=%v, skipped=%d), want an empty-diff file skipped", meta, skipped)
	}
}

func TestBuildCombinedDiff_TotalDiffLimitReached(t *testing.T) {
	changes := []gitlab.Change{
		{NewPath: "big.go", Diff: strings.Repeat("x", maxTotalDiff+1)},
	}
	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 1 || len(meta) != 0 {
		t.Errorf("buildCombinedDiff(...) = (meta=%v, skipped=%d), want the file skipped once the total diff budget is exceeded", meta, skipped)
	}
}

func TestBuildCombinedDiff_RenamedFile_FallsBackToOldPath(t *testing.T) {
	changes := []gitlab.Change{
		{NewPath: "", OldPath: "old_name.go", Diff: "@@ -1,1 +0,0 @@\n-x\n"},
	}
	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 0 {
		t.Fatalf("buildCombinedDiff(...) skipped = %d, want 0", skipped)
	}
	if _, ok := meta["old_name.go"]; !ok {
		t.Errorf("buildCombinedDiff(...) fileMeta = %v, want an entry keyed by the old path", meta)
	}
}

func TestBuildCombinedDiff_BothPathsEmpty_UsesUnknownPlaceholder(t *testing.T) {
	changes := []gitlab.Change{{Diff: "@@ -1,1 +1,1 @@\n+x\n"}}
	combined, meta, _ := buildCombinedDiff(changes, "TestLLM")
	if _, ok := meta["unknown"]; !ok {
		t.Errorf("buildCombinedDiff(...) fileMeta = %v, want an \"unknown\" placeholder entry", meta)
	}
	if !strings.Contains(combined, "=== File: unknown ===") {
		t.Errorf("buildCombinedDiff(...) combined = %q, want the unknown placeholder in the section header", combined)
	}
}

func TestBuildCombinedDiff_LineMapAggregatesAcrossMultipleHunks(t *testing.T) {
	// A single file's diff commonly carries several discontiguous hunks; fileMeta's line index
	// must accumulate entries from every hunk, not just retain whichever hunk was parsed last.
	diff := "@@ -1,1 +1,1 @@\n+top of file\n@@ -50,1 +50,1 @@\n+bottom of file\n"
	changes := []gitlab.Change{{NewPath: "main.go", OldPath: "main.go", Diff: diff}}

	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 0 {
		t.Fatalf("buildCombinedDiff(...) skipped = %d, want 0", skipped)
	}
	info, ok := meta["main.go"]
	if !ok {
		t.Fatal("buildCombinedDiff(...) fileMeta missing \"main.go\"")
	}
	if _, ok := info.lines[1]; !ok {
		t.Errorf("fileMeta[\"main.go\"].lines = %v, want an entry for line 1 from the first hunk", info.lines)
	}
	if _, ok := info.lines[50]; !ok {
		t.Errorf("fileMeta[\"main.go\"].lines = %v, want an entry for line 50 from the second hunk", info.lines)
	}
}

func TestBuildCombinedDiff_RenameOnlyDiff_NonEmptyDiffFieldButNoHunk(t *testing.T) {
	// A pure rename's Diff field is non-empty (it carries rename metadata), so the empty-diff skip
	// check does not trigger; the file is still queued for review, but its line index ends up
	// empty since parseDiff finds no "@@" hunk to anchor against.
	diff := "diff --git a/old_name.go b/new_name.go\nsimilarity index 100%\nrename from old_name.go\nrename to new_name.go\n"
	changes := []gitlab.Change{{NewPath: "new_name.go", OldPath: "old_name.go", Diff: diff}}

	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 0 {
		t.Fatalf("buildCombinedDiff(...) skipped = %d, want 0: a non-empty (metadata-only) diff must not be treated as empty", skipped)
	}
	info, ok := meta["new_name.go"]
	if !ok {
		t.Fatal("buildCombinedDiff(...) fileMeta missing \"new_name.go\"")
	}
	if len(info.lines) != 0 {
		t.Errorf("fileMeta[\"new_name.go\"].lines = %v, want empty for a rename with no content hunks", info.lines)
	}
}

func TestBuildCombinedDiff_ExactlyAtTotalDiffLimit_NotSkipped(t *testing.T) {
	changes := []gitlab.Change{
		{NewPath: "big.go", Diff: strings.Repeat("x", maxTotalDiff)},
	}
	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 0 {
		t.Errorf("buildCombinedDiff(...) skipped = %d, want 0 when the diff lands exactly at, not over, the budget", skipped)
	}
	if _, ok := meta["big.go"]; !ok {
		t.Error("buildCombinedDiff(...) fileMeta missing \"big.go\" at the exact budget boundary")
	}
}

func TestBuildCombinedDiff_SecondFileConsumesRemainingBudget(t *testing.T) {
	// The running total accumulates across files within the same call; a second file that would
	// individually fit under maxTotalDiff can still be skipped once the first file has already
	// consumed most of the shared budget.
	changes := []gitlab.Change{
		{NewPath: "first.go", Diff: strings.Repeat("x", maxTotalDiff-10)},
		{NewPath: "second.go", Diff: strings.Repeat("y", 20)},
	}
	_, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if skipped != 1 {
		t.Errorf("buildCombinedDiff(...) skipped = %d, want 1 (second.go pushed over the shared budget)", skipped)
	}
	if _, ok := meta["first.go"]; !ok {
		t.Error("buildCombinedDiff(...) fileMeta missing \"first.go\"")
	}
	if _, ok := meta["second.go"]; ok {
		t.Error("buildCombinedDiff(...) fileMeta unexpectedly contains \"second.go\" past the shared budget")
	}
}

func TestBuildCombinedDiff_NoReviewableFiles_EmptyResult(t *testing.T) {
	changes := []gitlab.Change{{NewPath: "go.sum", Diff: "@@ -1,1 +1,1 @@\n+x\n"}}
	combined, meta, skipped := buildCombinedDiff(changes, "TestLLM")
	if combined != "" || len(meta) != 0 || skipped != 1 {
		t.Errorf("buildCombinedDiff(...) = (%q, %v, %d), want an entirely empty, fully-skipped result", combined, meta, skipped)
	}
}

// fakeLLM is a stub LLMClient recording the prompt it received and returning a scripted response.
type fakeLLM struct {
	name     string
	response string
	err      error
	gotCall  bool
}

func (f *fakeLLM) Name() string { return f.name }
func (f *fakeLLM) Review(prompt string) (string, error) {
	f.gotCall = true
	return f.response, f.err
}

// newTestGitLabServer wires an httptest server implementing the minimal GitLab MR API surface
// FetchMR/PostDiscussion/PostNote/AddLabels depend on, recording every request path and body.
func newTestGitLabServer(t *testing.T, detail, diffs string) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("/projects/1/merge_requests/2", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(detail))
	})
	mux.HandleFunc("/projects/1/merge_requests/2/diffs", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(diffs))
	})
	mux.HandleFunc("/projects/1/merge_requests/2/discussions", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/projects/1/merge_requests/2/notes", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &calls
}

func TestReviewer_Execute_NoChanges_ReturnsNilWithoutCallingLLM(t *testing.T) {
	server, _ := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[]`,
	)
	llm := &fakeLLM{name: "Test"}
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), llm)

	if err := reviewer.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if llm.gotCall {
		t.Error("Run() invoked the LLM despite an empty change set; want it short-circuited")
	}
}

func TestReviewer_Execute_MissingDiffRefs(t *testing.T) {
	server, _ := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"","start_sha":"","head_sha":""}}`,
		`[{"new_path":"main.go","old_path":"main.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test"})

	err := reviewer.Execute()
	if err == nil {
		t.Fatal("Run() succeeded unexpectedly with an empty base_sha; want an error")
	}
	if !strings.Contains(err.Error(), "diff_refs missing") {
		t.Errorf("Run() error = %q, want it to mention missing diff_refs", err.Error())
	}
}

func TestReviewer_Execute_AllFilesFilteredOut(t *testing.T) {
	server, _ := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"go.sum","old_path":"go.sum","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	llm := &fakeLLM{name: "Test"}
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), llm)

	if err := reviewer.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if llm.gotCall {
		t.Error("Run() invoked the LLM despite every file being filtered out; want it short-circuited")
	}
}

func TestReviewer_Execute_LLMCallFails(t *testing.T) {
	server, _ := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"main.go","old_path":"main.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test", err: errBoom})

	err := reviewer.Execute()
	if err == nil {
		t.Fatal("Run() succeeded unexpectedly despite an LLM error; want it propagated")
	}
	if !strings.Contains(err.Error(), "llm call failed") {
		t.Errorf("Run() error = %q, want it to mention the LLM call failure", err.Error())
	}
}

func TestReviewer_Execute_MalformedLLMResponse(t *testing.T) {
	server, _ := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"main.go","old_path":"main.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test", response: "not json"})

	if err := reviewer.Execute(); err == nil {
		t.Fatal("Run() succeeded unexpectedly on an unparseable LLM response; want an error")
	}
}

func TestReviewer_Execute_SecurityFindingAppliesLabel(t *testing.T) {
	server, calls := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"main.go","old_path":"main.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	response := `[{"file":"main.go","start_line":1,"end_line":1,"description":"sql injection","security":true}]`
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test", response: response})

	if err := reviewer.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	var sawLabelPut bool
	for _, c := range *calls {
		if c == "PUT /projects/1/merge_requests/2" {
			sawLabelPut = true
		}
	}
	if !sawLabelPut {
		t.Errorf("calls = %v, want a PUT request applying the security label", *calls)
	}
}

func TestReviewer_Execute_UnknownFileFallsBackToNote(t *testing.T) {
	server, calls := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"main.go","old_path":"main.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	// The finding references a file absent from fileMeta (not part of this MR's diff), so deliver
	// must skip inline positioning entirely and fall back to a general note.
	response := `[{"file":"other.go","start_line":1,"end_line":1,"description":"issue"}]`
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test", response: response})

	if err := reviewer.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	var sawNote, sawDiscussion bool
	for _, c := range *calls {
		if c == "POST /projects/1/merge_requests/2/notes" {
			sawNote = true
		}
		if c == "POST /projects/1/merge_requests/2/discussions" {
			sawDiscussion = true
		}
	}
	if !sawNote {
		t.Errorf("calls = %v, want a fallback note posted", *calls)
	}
	if sawDiscussion {
		t.Errorf("calls = %v, want no inline discussion for a file outside the diff", *calls)
	}
}

func TestReviewer_Execute_RenameOnlyFile_CommentFallsBackToNote(t *testing.T) {
	// The file is present in fileMeta (queued for review) but its line index is empty since the
	// diff carries no content hunk, so buildPosition must return nil and deliver must fall back
	// to a general note rather than posting an inline discussion at a nonexistent position.
	renameDiff := "diff --git a/old_name.go b/new_name.go\\nsimilarity index 100%\\nrename from old_name.go\\nrename to new_name.go\\n"
	server, calls := newTestGitLabServer(t,
		`{"title":"chore: rename","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"new_name.go","old_path":"old_name.go","diff":"`+renameDiff+`"}]`,
	)
	response := `[{"file":"new_name.go","start_line":1,"end_line":1,"description":"consider the new name"}]`
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test", response: response})

	if err := reviewer.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	var sawNote, sawDiscussion bool
	for _, c := range *calls {
		if c == "POST /projects/1/merge_requests/2/notes" {
			sawNote = true
		}
		if c == "POST /projects/1/merge_requests/2/discussions" {
			sawDiscussion = true
		}
	}
	if !sawNote {
		t.Errorf("calls = %v, want a fallback note posted for a rename with no anchorable line", *calls)
	}
	if sawDiscussion {
		t.Errorf("calls = %v, want no inline discussion attempted when the file's line index is empty", *calls)
	}
}

func TestReviewer_Execute_MissingStartLine_CommentSkipped(t *testing.T) {
	server, calls := newTestGitLabServer(t,
		`{"title":"feat: x","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		`[{"new_path":"main.go","old_path":"main.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`,
	)
	response := `[{"file":"main.go","description":"no line info"}]`
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test", response: response})

	if err := reviewer.Execute(); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	for _, c := range *calls {
		if strings.Contains(c, "notes") || strings.Contains(c, "discussions") {
			t.Errorf("calls = %v, want no comment posted when start_line is absent", *calls)
		}
	}
}

func TestReviewer_Execute_MaxDescriptionCharsInvalid_FailsBeforeFetch(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "not-a-number")
	server, calls := newTestGitLabServer(t, `{}`, `[]`)
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test"})

	if err := reviewer.Execute(); err == nil {
		t.Fatal("Run() succeeded unexpectedly with an invalid MAX_DESCRIPTION_CHARS; want an error")
	}
	if len(*calls) != 0 {
		t.Errorf("calls = %v, want no GitLab API calls when MAX_DESCRIPTION_CHARS is invalid", *calls)
	}
}

func TestReviewer_Execute_GitLabFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	reviewer := New(gitlab.New(server.URL, "1", "2", "token"), &fakeLLM{name: "Test"})

	err := reviewer.Execute()
	if err == nil {
		t.Fatal("Run() succeeded unexpectedly despite a GitLab API failure; want an error")
	}
	if !strings.Contains(err.Error(), "fetch MR changes") {
		t.Errorf("Run() error = %q, want it to mention the fetch failure", err.Error())
	}
}

// Sanity check that Comment JSON tags line up with the schema the LLM is instructed to emit.
func TestComment_JSONUnmarshal_OptionalFieldsDefaultZeroValue(t *testing.T) {
	var c Comment
	if err := json.Unmarshal([]byte(`{"file":"a.go","description":"d"}`), &c); err != nil {
		t.Fatalf("json.Unmarshal(...) returned an unexpected error: %v", err)
	}
	if c.StartLine != nil || c.EndLine != nil {
		t.Errorf("Comment = %+v, want nil StartLine/EndLine when omitted from the JSON payload", c)
	}
	if c.Security {
		t.Error("Comment.Security = true, want false when omitted from the JSON payload")
	}
}
