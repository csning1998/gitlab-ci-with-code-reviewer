package review

import (
	"strings"
	"testing"
)

func TestParseDiff_SingleHunk_AddOnly(t *testing.T) {
	diff := "@@ -1,2 +1,3 @@\n context\n+added\n context2\n"
	lines := parseDiff(diff)

	if len(lines) != 4 {
		t.Fatalf("parseDiff(...) returned %d lines, want 4", len(lines))
	}
	if lines[0].prefix != "@@" {
		t.Errorf("lines[0].prefix = %q, want \"@@\"", lines[0].prefix)
	}
	if lines[2].prefix != "+" || lines[2].content != "added" || *lines[2].newLine != 2 {
		t.Errorf("lines[2] = %+v, want an addition at new line 2", lines[2])
	}
}

func TestParseDiff_AdditionsAndDeletions_LineNumberTracking(t *testing.T) {
	// A deletion consumes only the old line counter, and a subsequent addition consumes only the
	// new line counter, so the two must diverge across a hunk mixing both.
	diff := "@@ -10,2 +10,2 @@\n-removed line\n+added line\n context\n"
	lines := parseDiff(diff)

	if len(lines) != 4 {
		t.Fatalf("parseDiff(...) returned %d lines, want 4", len(lines))
	}
	del := lines[1]
	if del.prefix != "-" || del.newLine != nil || *del.oldLine != 10 {
		t.Errorf("deletion line = %+v, want oldLine=10 and newLine=nil", del)
	}
	add := lines[2]
	if add.prefix != "+" || add.oldLine != nil || *add.newLine != 10 {
		t.Errorf("addition line = %+v, want newLine=10 and oldLine=nil", add)
	}
	ctx := lines[3]
	if ctx.prefix != " " || *ctx.newLine != 11 || *ctx.oldLine != 11 {
		t.Errorf("context line = %+v, want newLine=11 and oldLine=11", ctx)
	}
}

func TestParseDiff_MultipleHunks_ResetsLineCounters(t *testing.T) {
	diff := "@@ -1,1 +1,1 @@\n context1\n@@ -50,1 +52,1 @@\n context2\n"
	lines := parseDiff(diff)

	if len(lines) != 4 {
		t.Fatalf("parseDiff(...) returned %d lines, want 4", len(lines))
	}
	if *lines[1].newLine != 1 {
		t.Errorf("first hunk context newLine = %d, want 1", *lines[1].newLine)
	}
	if *lines[3].newLine != 52 {
		t.Errorf("second hunk context newLine = %d, want 52", *lines[3].newLine)
	}
}

func TestParseDiff_EmptyDiff(t *testing.T) {
	if lines := parseDiff(""); len(lines) != 0 {
		t.Errorf("parseDiff(\"\") returned %d lines, want 0", len(lines))
	}
}

func TestParseDiff_NoHunkHeader_ContentIgnored(t *testing.T) {
	// Content lines preceding the first "@@" hunk header (e.g. a "diff --git" preamble or binary
	// file marker) have no established line counters and must be dropped rather than panicking on
	// a nil pointer dereference.
	diff := "diff --git a/file b/file\nindex abc..def 100644\n--- a/file\n+++ b/file\n"
	lines := parseDiff(diff)
	if len(lines) != 0 {
		t.Errorf("parseDiff(...) returned %d lines, want 0 lines when no hunk header is present", len(lines))
	}
}

func TestParseDiff_NoNewlineAtEOFMarker_Ignored(t *testing.T) {
	diff := "@@ -1,1 +1,1 @@\n+added\n\\ No newline at end of file\n"
	lines := parseDiff(diff)
	if len(lines) != 2 {
		t.Fatalf("parseDiff(...) returned %d lines, want 2 (hunk header + addition)", len(lines))
	}
}

func TestParseDiff_EmptyContextLine(t *testing.T) {
	// A bare " " prefix line with zero-length remainder (a fully blank line in the file) must not
	// panic when slicing line[1:] to extract content.
	diff := "@@ -1,1 +1,1 @@\n\n"
	lines := parseDiff(diff)
	if len(lines) != 2 {
		t.Fatalf("parseDiff(...) returned %d lines, want 2", len(lines))
	}
	if lines[1].content != "" {
		t.Errorf("lines[1].content = %q, want empty string for a blank context line", lines[1].content)
	}
}

func TestParseDiff_HunkHeaderWithoutCountSuffix(t *testing.T) {
	// A single-line hunk omits the ",<count>" suffix entirely (e.g. "@@ -5 +5 @@").
	diff := "@@ -5 +5 @@\n context\n"
	lines := parseDiff(diff)
	if len(lines) != 2 {
		t.Fatalf("parseDiff(...) returned %d lines, want 2", len(lines))
	}
	if *lines[1].newLine != 5 {
		t.Errorf("context newLine = %d, want 5", *lines[1].newLine)
	}
}

func TestParseDiff_PureDeletion_NoAdditions(t *testing.T) {
	diff := "@@ -1,3 +0,0 @@\n-line1\n-line2\n-line3\n"
	lines := parseDiff(diff)
	if len(lines) != 4 {
		t.Fatalf("parseDiff(...) returned %d lines, want 4", len(lines))
	}
	for i := 1; i < 4; i++ {
		if lines[i].newLine != nil {
			t.Errorf("lines[%d].newLine = %v, want nil for a pure deletion hunk", i, lines[i].newLine)
		}
	}
}

func TestParseDiff_TrailingNewline_DoesNotProduceEmptyLine(t *testing.T) {
	withTrailing := parseDiff("@@ -1,1 +1,1 @@\n context\n")
	withoutTrailing := parseDiff("@@ -1,1 +1,1 @@\n context")
	if len(withTrailing) != len(withoutTrailing) {
		t.Errorf("parseDiff with vs without trailing newline produced %d vs %d lines, want equal", len(withTrailing), len(withoutTrailing))
	}
}

func TestParseDiff_LineContainingLiteralAtAtButNotAHeader(t *testing.T) {
	// A content line that merely begins with "@@ " text (e.g. an added comment) inside an already
	// open hunk must be treated as a context line, not mistaken for a new hunk header, since it
	// does not match the anchored hunk regexp.
	diff := "@@ -1,1 +1,2 @@\n context\n+// @@ not a hunk header\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3", len(lines))
	}
	if lines[2].prefix != "+" {
		t.Errorf("lines[2].prefix = %q, want \"+\" (an addition, not a hunk header)", lines[2].prefix)
	}
}

func TestAnnotateDiff_FormatsAdditionsDeletionsAndContext(t *testing.T) {
	lines := parseDiff("@@ -1,2 +1,3 @@\n context\n-removed\n+added\n")
	out := annotateDiff(lines)

	if !strings.Contains(out, "@@ -1,2 +1,3 @@") {
		t.Errorf("annotateDiff(...) output = %q, want it to preserve the hunk header verbatim", out)
	}
	if !strings.Contains(out, "[     ] - removed") {
		t.Errorf("annotateDiff(...) output = %q, want a blank line-number marker for the deletion", out)
	}
	if !strings.Contains(out, "+ added") {
		t.Errorf("annotateDiff(...) output = %q, want the addition rendered with a line number", out)
	}
}

func TestAnnotateDiff_EmptyInput(t *testing.T) {
	if out := annotateDiff(nil); out != "" {
		t.Errorf("annotateDiff(nil) = %q, want an empty string", out)
	}
}

func TestMatchesReviewExclusion_LockAndGeneratedFilenames(t *testing.T) {
	cases := []string{
		"package-lock.json", "yarn.lock", "bun.lock", "go.sum", "poetry.lock",
		"Pipfile.lock", "composer.lock", "pnpm-lock.yaml", ".terraform.lock.hcl", "packer_manifest.json",
	}
	for _, name := range cases {
		if !matchesReviewExclusion(name) {
			t.Errorf("matchesReviewExclusion(%q) = false, want true", name)
		}
		if !matchesReviewExclusion("nested/dir/" + name) {
			t.Errorf("matchesReviewExclusion(%q) = false, want true for a nested path", "nested/dir/"+name)
		}
	}
}

func TestMatchesReviewExclusion_BinaryAndGeneratedExtensions(t *testing.T) {
	cases := []string{
		"image.png", "photo.JPG", "icon.svg", "archive.zip", "cert.pem", "cert.CRT", "font.woff2",
	}
	for _, name := range cases {
		if !matchesReviewExclusion(name) {
			t.Errorf("matchesReviewExclusion(%q) = false, want true (case-insensitive extension match)", name)
		}
	}
}

func TestMatchesReviewExclusion_ReviewableSourceFiles(t *testing.T) {
	cases := []string{"main.go", "handler.ts", "component.vue", "playbook.yml", "README.md"}
	for _, name := range cases {
		if matchesReviewExclusion(name) {
			t.Errorf("matchesReviewExclusion(%q) = true, want false", name)
		}
	}
}

func TestMatchesReviewExclusion_MinifiedAssetsUseCompoundExtension(t *testing.T) {
	// skipExtensions keys on filepath.Ext, which returns only the final "."-delimited segment, so
	// a compound ".min.js" suffix is not recognized as skippable by extension alone.
	if matchesReviewExclusion("app.min.js") {
		t.Error("matchesReviewExclusion(\"app.min.js\") = true, want false: filepath.Ext yields \".js\", not \".min.js\"")
	}
}

func TestMatchesReviewExclusion_NoExtension(t *testing.T) {
	if matchesReviewExclusion("Makefile") {
		t.Error("matchesReviewExclusion(\"Makefile\") = true, want false")
	}
}

func TestMatchesReviewExclusion_EmptyPath(t *testing.T) {
	if matchesReviewExclusion("") {
		t.Error("matchesReviewExclusion(\"\") = true, want false")
	}
}

// --- Real-world git diff computation edge cases ---

func TestParseDiff_HunkHeaderWithFunctionContextSuffix(t *testing.T) {
	// `git diff` on source files commonly appends the enclosing function/class signature after
	// the closing "@@" (e.g. via .gitattributes diff drivers or language-aware xfuncname patterns).
	// The regexp is not end-anchored, so the numeric groups must still resolve correctly and the
	// suffix must survive verbatim into the rendered header for LLM context.
	diff := "@@ -10,5 +10,6 @@ func doSomething() {\n context\n+added\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3", len(lines))
	}
	if *lines[1].newLine != 10 {
		t.Errorf("first content line newLine = %d, want 10", *lines[1].newLine)
	}
	if lines[0].content != "@@ -10,5 +10,6 @@ func doSomething() {" {
		t.Errorf("hunk header content = %q, want the function-context suffix preserved verbatim", lines[0].content)
	}
	annotated := annotateDiff(lines)
	if !strings.Contains(annotated, "func doSomething() {") {
		t.Errorf("annotateDiff(...) = %q, want the function-context suffix reproduced in the rendered output", annotated)
	}
}

func TestParseDiff_NewFileHunk_ZeroOldStart(t *testing.T) {
	// A newly added file's single hunk is conventionally headed "@@ -0,0 +1,N @@": the old side
	// has zero lines, so oldLine starts at 0 even though no "-" line will ever consume it.
	diff := "@@ -0,0 +1,2 @@\n+line one\n+line two\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3", len(lines))
	}
	if *lines[1].newLine != 1 || *lines[2].newLine != 2 {
		t.Errorf("addition newLines = [%d %d], want [1 2]", *lines[1].newLine, *lines[2].newLine)
	}
}

func TestParseDiff_FullFileDeletionHunk_ZeroNewStart(t *testing.T) {
	// A fully deleted file's hunk header reads "@@ -1,N +0,0 @@": the new side is empty, and every
	// content line in the hunk is a deletion, never touching the newLine counter.
	diff := "@@ -1,2 +0,0 @@\n-line one\n-line two\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3", len(lines))
	}
	if *lines[1].oldLine != 1 || *lines[2].oldLine != 2 {
		t.Errorf("deletion oldLines = [%d %d], want [1 2]", *lines[1].oldLine, *lines[2].oldLine)
	}
}

func TestParseDiff_MidFileZeroCountDeletionHunk(t *testing.T) {
	// A hunk can carry a zero new-side count mid-file (not only for whole-file deletion), e.g.
	// "@@ -12,3 +12,0 @@" when a block is removed without any replacement lines.
	diff := "@@ -12,3 +12,0 @@\n-a\n-b\n-c\n@@ -20,1 +17,1 @@\n context\n"
	lines := parseDiff(diff)
	if len(lines) != 6 {
		t.Fatalf("parseDiff(...) returned %d lines, want 6", len(lines))
	}
	// The second hunk's own declared start (17) must be honored independent of whatever the
	// first hunk's line count implied, since counters are always reset from the header, never
	// accumulated across hunks.
	if *lines[5].newLine != 17 {
		t.Errorf("second hunk context newLine = %d, want 17 (from its own header, not accumulated)", *lines[5].newLine)
	}
}

func TestParseDiff_DoubleNoNewlineMarker_BothSidesLackTrailingNewline(t *testing.T) {
	// When the exact last line of a file is modified and neither the old nor the new revision
	// ends with a trailing newline, git emits the "\ No newline at end of file" marker twice:
	// once immediately after the removed line and once after the added line. Both occurrences
	// must be dropped without disrupting the surrounding line-number sequence.
	diff := "@@ -5,1 +5,1 @@\n-old last line\n\\ No newline at end of file\n+new last line\n\\ No newline at end of file\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3 (hunk header + deletion + addition)", len(lines))
	}
	if lines[1].prefix != "-" || lines[1].content != "old last line" {
		t.Errorf("lines[1] = %+v, want the deletion with its marker line stripped", lines[1])
	}
	if lines[2].prefix != "+" || lines[2].content != "new last line" || *lines[2].newLine != 5 {
		t.Errorf("lines[2] = %+v, want the addition at new line 5 with its marker line stripped", lines[2])
	}
}

func TestParseDiff_BackToBackHunkHeaders_NoContentBetween(t *testing.T) {
	// Nothing in the grammar forbids two hunk headers with zero content lines between them (e.g.
	// a diff tool coalescing adjacent zero-context hunks); the second header must still reset the
	// counters cleanly rather than reading a stale newLine/oldLine left by the first.
	diff := "@@ -1,0 +1,0 @@\n@@ -5,1 +5,2 @@\n context\n+added\n"
	lines := parseDiff(diff)
	if len(lines) != 4 {
		t.Fatalf("parseDiff(...) returned %d lines, want 4 (two headers + context + addition)", len(lines))
	}
	if lines[0].prefix != "@@" || lines[1].prefix != "@@" {
		t.Errorf("lines[0:2] = %+v, want two consecutive hunk-header entries", lines[0:2])
	}
	if *lines[2].newLine != 5 {
		t.Errorf("context newLine = %d, want 5 from the second header", *lines[2].newLine)
	}
}

func TestParseDiff_BinaryFileDiffPlaceholder_NoHunk(t *testing.T) {
	// GitLab represents a binary file change with prose like "Binary files a/img.png and
	// b/img.png differ" and never emits an "@@" hunk header at all.
	diff := "Binary files a/image.png and b/image.png differ\n"
	if lines := parseDiff(diff); len(lines) != 0 {
		t.Errorf("parseDiff(...) returned %d lines, want 0 for a binary-file diff placeholder", len(lines))
	}
}

func TestParseDiff_RenameOnlyDiff_NoContentHunk(t *testing.T) {
	// A pure rename (100% similarity, no content change) carries only metadata lines and never an
	// "@@" hunk, so no line can be anchored despite the Diff field being non-empty.
	diff := "diff --git a/old_name.go b/new_name.go\nsimilarity index 100%\nrename from old_name.go\nrename to new_name.go\n"
	if lines := parseDiff(diff); len(lines) != 0 {
		t.Errorf("parseDiff(...) returned %d lines, want 0 for a rename-only diff with no hunks", len(lines))
	}
}

func TestParseDiff_ModeChangeOnlyDiff_NoContentHunk(t *testing.T) {
	diff := "diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode 100755\n"
	if lines := parseDiff(diff); len(lines) != 0 {
		t.Errorf("parseDiff(...) returned %d lines, want 0 for a mode-change-only diff with no hunks", len(lines))
	}
}

func TestParseDiff_AddedBlankLine_BarePlusMarker(t *testing.T) {
	// An added blank line renders as a bare "+" with nothing after it; slicing line[1:] on a
	// length-1 string must not panic and must yield empty content.
	diff := "@@ -1,1 +1,2 @@\n context\n+\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3", len(lines))
	}
	if lines[2].prefix != "+" || lines[2].content != "" {
		t.Errorf("lines[2] = %+v, want an addition with empty content", lines[2])
	}
}

func TestParseDiff_DeletedBlankLine_BareMinusMarker(t *testing.T) {
	diff := "@@ -1,2 +1,1 @@\n-\n context\n"
	lines := parseDiff(diff)
	if len(lines) != 3 {
		t.Fatalf("parseDiff(...) returned %d lines, want 3", len(lines))
	}
	if lines[1].prefix != "-" || lines[1].content != "" {
		t.Errorf("lines[1] = %+v, want a deletion with empty content", lines[1])
	}
}

func TestParseDiff_AddedLineContentStartingWithPlus(t *testing.T) {
	// Only the first character is the diff marker; a line adding source text that itself begins
	// with "+" (e.g. a unary-plus expression or a nested diff snippet in a doc comment) must keep
	// the remaining "+" characters as literal content.
	diff := "@@ -1,1 +1,1 @@\n+++already-plus-prefixed\n"
	lines := parseDiff(diff)
	if len(lines) != 2 {
		t.Fatalf("parseDiff(...) returned %d lines, want 2", len(lines))
	}
	if lines[1].content != "++already-plus-prefixed" {
		t.Errorf("lines[1].content = %q, want \"++already-plus-prefixed\" (only the leading marker stripped)", lines[1].content)
	}
}

func TestParseDiff_DeletedLineContentStartingWithMinus(t *testing.T) {
	// A removed YAML list item ("- foo") is itself prefixed with "-", so the diff line reads
	// "--foo"; only the outer diff marker may be stripped, not the content's own leading hyphen.
	diff := "@@ -1,1 +0,0 @@\n-- foo\n"
	lines := parseDiff(diff)
	if len(lines) != 2 {
		t.Fatalf("parseDiff(...) returned %d lines, want 2", len(lines))
	}
	if lines[1].content != "- foo" {
		t.Errorf("lines[1].content = %q, want \"- foo\" (only the outer diff marker stripped)", lines[1].content)
	}
}

func TestParseDiff_CRLFLineEndings_CarriageReturnLeaksIntoContent(t *testing.T) {
	// parseDiff splits solely on "\n"; a diff sourced from a CRLF-authored file (common on
	// Windows-originated branches) leaves a trailing "\r" attached to each split segment. This is
	// the current, documented behavior rather than a normative requirement: it does not corrupt
	// line-number bookkeeping, but it does mean rendered content carries a stray "\r".
	diff := "@@ -1,1 +1,1 @@\r\n+added\r\n"
	lines := parseDiff(diff)
	if len(lines) != 2 {
		t.Fatalf("parseDiff(...) returned %d lines, want 2", len(lines))
	}
	if lines[1].content != "added\r" {
		t.Errorf("lines[1].content = %q, want \"added\\r\" reflecting the unstripped carriage return", lines[1].content)
	}
	if *lines[1].newLine != 1 {
		t.Errorf("lines[1].newLine = %d, want 1: the stray \\r must not perturb line-number tracking", *lines[1].newLine)
	}
}

func TestParseDiff_HunkHeaderOnly_NoTrailingContentLines(t *testing.T) {
	diff := "@@ -1,1 +1,1 @@"
	lines := parseDiff(diff)
	if len(lines) != 1 || lines[0].prefix != "@@" {
		t.Fatalf("parseDiff(...) = %+v, want a single hunk-header entry and no panic", lines)
	}
}

func TestParseDiff_ContextLineImmediatelyAfterAddition_IndependentCounters(t *testing.T) {
	// After an addition (which advances only newLine) followed by a context line (which advances
	// both), the old/new line numbers must have diverged by exactly the number of pure additions
	// seen so far in the hunk, not merely mirror each other.
	diff := "@@ -10,2 +10,3 @@\n context1\n+added\n context2\n"
	lines := parseDiff(diff)
	if len(lines) != 4 {
		t.Fatalf("parseDiff(...) returned %d lines, want 4", len(lines))
	}
	ctx2 := lines[3]
	if *ctx2.newLine != 12 || *ctx2.oldLine != 11 {
		t.Errorf("second context line = newLine %d oldLine %d, want newLine=12 oldLine=11 (new side advanced one extra for the addition)", *ctx2.newLine, *ctx2.oldLine)
	}
}
