package review

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"ci-tools/internal/gate"
	"ci-tools/internal/gitlab"
)

// LLMClient defines the interface for language model client integrations.
type LLMClient interface {
	Name() string
	Review(prompt string) (string, error)
}

// DefaultMaxTotalDiff establishes the fallback upper bound on combined diff characters.
const DefaultMaxTotalDiff = 300000

// ResolveMaxTotalDiff parses MAX_TOTAL_DIFF from the environment or returns DefaultMaxTotalDiff.
func ResolveMaxTotalDiff() int {
	val := strings.TrimSpace(os.Getenv("MAX_TOTAL_DIFF"))
	if val == "" {
		return DefaultMaxTotalDiff
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return DefaultMaxTotalDiff
	}
	return n
}

// DefaultPromptTemplate specifies default review instructions, domain checks,
// and JSON schema requirements when a custom prompt configuration is omitted.
const DefaultPromptTemplate = `Perform automated code review on the provided merge request diff.

Review Context:
- Annotated diff of modified files follows below.
- File boundaries are marked by: === File: <path> ===
- Added lines carry line numbers formatted as [L   N].
- Deleted lines carry empty line markers [     ].

Merge Request Intent:
- An optional "=== Merge Request Intent ===" section precedes the diff containing author title and description.
- Treat author intent and explicit trade-offs as authoritative context.
- Suppress findings on intentional trade-offs unless flawed by factual errors or security risks.

Finding Quality Requirements:
- Omit findings where analysis indicates zero risk, existing mitigation, or intended behavior. Do not emit mitigated findings with caveats.

Review Focus & Domains:
- Core: Logic bugs, security vulnerabilities, performance bottlenecks, architectural flaws, and maintainability defects.
- Vue Components: Reactivity flaws, lifecycle bugs, prop validation errors, and XSS vulnerabilities (e.g., unsafe v-html usage).
- TypeScript: Type safety violations, implicit any usage, and unsafe type assertions.
- Python (2.7 through 3.15): Python 2/3 porting defects (str/bytes confusion, integer division changes), mutable default arguments, unsound type hints against configured checker strictness, unsafe deserialization (pickle, yaml.load without SafeLoader), async/await anti-patterns, GIL-atomicity assumptions unsafe under free-threaded builds, and, for NumPy/pandas/SciPy code, unintended broadcasting, chained-assignment mutation, and floating-point precision loss in statistical aggregation.
- C / C++: Memory safety defects (buffer overflows, use-after-free, double free), undefined behavior, standard dialect compliance violations (C89 through C23 / C++11 through C++26 against configured standard flags), pointer arithmetic errors, and unchecked POSIX/system call returns.
- JVM Ecosystem (Java, Kotlin, Scala, Groovy): Target level/class file format compatibility violations (language feature usage exceeding specified target version/class major floor), concurrency defects, resource leaks (unclosed streams/AutoCloseable), unsafe reflection, and insecure deserialization.
- C# / .NET: Target Framework Moniker (TFM) and language version incompatibilities, async/await anti-patterns (deadlocks, unobserved task exceptions), IDisposable leaks, improper unsafe code blocks, and allocation hot-paths.
- Rust: Soundness violations in unsafe blocks, panicking pathways in production paths, lifetime/ownership mismatches, unchecked unwrap/expect calls, and concurrency data races.
- Go: Goroutine leaks, channel deadlocks, data races, unhandled error returns, improper unsafe.Pointer conversions, and missing Context propagation.
- Infrastructure (HCL, YAML, Dockerfile): Unbounded resources, insecure security contexts, embedded credentials, and misconfigurations.

Mathematical Correctness:
- Verify mathematical, statistical, and algorithmic computations against documented specifications (docstrings, comments, or MR intent) or standard definitions.
- Report sign errors, summation or index off-by-one errors, invalid distributional or numerical assumptions, and formula deviations as defects, independent of code style.

Security Labeling Policy:
- Set "security": true strictly for: (1) Credential or secret exposure (API keys, tokens, passwords); (2) Injection vulnerabilities (XSS, SQLi, command injection, path traversal); (3) Memory-safety defects (memory leaks, use-after-free, buffer overflows).
- Clear "security" flag for general string-handling, validation, or encoding issues lacking direct exploit vectors in the above categories.

Output Format Requirements:
- Emit a raw JSON array exclusively. Exclude markdown code blocks, backticks, or outer text wrappers.
- Emit an empty array [] when no actionable defects exist.

JSON Element Schema:
{
    "file": "<exact file path from the === File: <path> === header>",
    "start_line": <integer, starting line number [L N] of the problematic range>,
    "end_line": <integer, ending line number [L N] of the problematic range; equal to start_line for single-line issues>,
    "description": "<concise markdown detailing the defect and technical impact>",
    "suggestion": "<optional: exact replacement lines for start_line..end_line maintaining indentation; omit when inapplicable>",
    "security": <boolean, true strictly per Security Labeling Policy; false or omitted otherwise>
}`

// MaxPromptSizeBytes defines the upper bound on custom prompt file byte length.
const MaxPromptSizeBytes = 1048576

// ResolvePrompt determines the effective review prompt instructions by evaluating
// inline overrides, external file sources, environment variables, or fallback defaults.
func ResolvePrompt(inlinePrompt, promptFile string) (string, error) {
	if trimmed := strings.TrimSpace(inlinePrompt); trimmed != "" {
		return trimmed, nil
	}

	targetFile := strings.TrimSpace(promptFile)
	if targetFile == "" {
		targetFile = strings.TrimSpace(os.Getenv("REVIEWER_PROMPT_FILE"))
	}
	if targetFile != "" {
		stat, err := os.Stat(targetFile)
		if err != nil {
			return "", fmt.Errorf("stat prompt file %q: %w", targetFile, err)
		}
		if stat.Size() > MaxPromptSizeBytes {
			return "", fmt.Errorf("prompt file %q size (%d bytes) exceeds maximum limit of %d bytes", targetFile, stat.Size(), MaxPromptSizeBytes)
		}
		data, err := os.ReadFile(targetFile)
		if err != nil {
			return "", fmt.Errorf("read prompt file %q: %w", targetFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	if envPrompt := strings.TrimSpace(os.Getenv("REVIEWER_PROMPT")); envPrompt != "" {
		return envPrompt, nil
	}

	return DefaultPromptTemplate, nil
}

// Comment represents a single code review finding emitted by an LLM provider.
// Pointer types for line numbers safely accommodate null or missing JSON attributes during unmarshaling.
type Comment struct {
	File        string `json:"file"`
	StartLine   *int   `json:"start_line"`
	EndLine     *int   `json:"end_line"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	Security    bool   `json:"security"`
}

// Reviewer orchestrates merge request evaluation workflow across GitLab API and LLM provider interfaces.
type Reviewer struct {
	gitlab *gitlab.Client
	llm    LLMClient
	prompt string
}

func New(gl *gitlab.Client, llm LLMClient) *Reviewer {
	return &Reviewer{
		gitlab: gl,
		llm:    llm,
		prompt: DefaultPromptTemplate,
	}
}

// WithPrompt overrides default review instructions with custom prompt guidelines.
func (r *Reviewer) WithPrompt(prompt string) *Reviewer {
	if strings.TrimSpace(prompt) != "" {
		r.prompt = strings.TrimSpace(prompt)
	}
	return r
}

// ExecuteCodeReview serves as the common execution entrypoint for reviewer executables,
// encapsulating client initialization and execution workflow.
func ExecuteCodeReview(apiURL, projectID, mriid, token string, llm LLMClient) error {
	return New(gitlab.New(apiURL, projectID, mriid, token), llm).Execute()
}

func (r *Reviewer) Execute() error {
	maxDesc, err := gate.ResolveDescriptionRuneLimit()
	if err != nil {
		return err
	}
	// Validate MAX_DESCRIPTION_CHARS prior to fetching MR details to fail early on configuration errors.
	mr, err := r.gitlab.FetchMR()
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}
	if len(mr.Changes) == 0 {
		fmt.Println("No code changes detected in this MR.")
		return nil
	}
	if mr.DiffRefs.BaseSha == "" {
		return fmt.Errorf("diff_refs missing from MR data")
	}

	combined, fileMeta, skipped := buildCombinedDiff(mr.Changes, r.llm.Name())
	if len(fileMeta) == 0 {
		fmt.Println("No reviewable changes after filtering.")
		return nil
	}

	prompt := r.prompt
	if prompt == "" {
		prompt = DefaultPromptTemplate
	}
	raw, err := r.llm.Review(prompt + "\n\n" + formatMRIntent(mr.Title, mr.Description, maxDesc) + combined)
	if err != nil {
		return fmt.Errorf("llm call failed: %w", err)
	}

	rawComments, err := extractJSONArray(raw)
	if err != nil {
		return err
	}
	if len(rawComments) == 0 {
		fmt.Println("LGTM -- no issues found.")
		return nil
	}

	fmt.Printf("LLM returned %d comment(s). Posting ...\n", len(rawComments))
	posted, foundSecurity := 0, false
	for _, rc := range rawComments {
		var c Comment
		if err := json.Unmarshal(rc, &c); err != nil {
			fmt.Printf("  skip: cannot parse comment (%v)\n", err)
			continue
		}
		if c.Security {
			foundSecurity = true
		}
		if r.postReviewComments(mr.DiffRefs, fileMeta, c) {
			posted++
		}
	}
	fmt.Printf("\nDone: %d comment(s) posted, %d file(s) skipped.\n", posted, skipped)

	// Deterministic labels (type::*, breaking-change, area::*) are the mr-labeler binary's
	// responsibility; only the LLM-derived security signal is applied here, since it depends
	// on this run's findings.
	if foundSecurity {
		if _, err := r.gitlab.AddLabels([]string{"security"}); err != nil {
			fmt.Printf("label assignment failed: %v\n", err)
		}
	}
	return nil
}

var trailingCommaRe = regexp.MustCompile(`,(\s*[\]}])`)

// extractJSONArray parses the LLM output into a JSON array, tolerating markdown code fences
// and surrounding conversational prose by attempting JSON decoding from candidate array start positions.
func extractJSONArray(raw string) ([]json.RawMessage, error) {
	cleaned := trailingCommaRe.ReplaceAllString(strings.TrimSpace(raw), "$1")
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &arr); err == nil {
		return arr, nil
	}
	for i := 0; i < len(cleaned); i++ {
		if cleaned[i] != '[' {
			continue
		}
		if err := json.NewDecoder(strings.NewReader(cleaned[i:])).Decode(&arr); err == nil {
			return arr, nil
		}
	}
	return nil, fmt.Errorf("parse llm response: no valid JSON array found (raw: %.200s)", cleaned)
}

// postReviewComments posts a single review finding, attempting inline discussion placement
// before falling back to a general merge request note if line-anchoring is unavailable.
func (r *Reviewer) postReviewComments(refs gitlab.DiffRefs, fileMeta map[string]fileInfo, c Comment) bool {
	file := strings.TrimSpace(c.File)
	description := strings.TrimSpace(c.Description)
	suggestion := strings.TrimSpace(c.Suggestion)

	if c.StartLine == nil {
		fmt.Println("  skip: missing start_line")
		return false
	}
	start := *c.StartLine
	end := start
	if c.EndLine != nil {
		end = *c.EndLine
	}
	if file == "" || description == "" {
		fmt.Println("  skip: missing file or description")
		return false
	}

	body := buildCommentBody(description, suggestion, start, end)
	label := fmt.Sprintf("L%d", start)
	if start != end {
		label = fmt.Sprintf("L%d-%d", start, end)
	}

	if info, ok := fileMeta[file]; ok {
		if pos := buildPosition(refs, file, info, start, end); pos != nil {
			status, err := r.gitlab.PostDiscussion(body, pos)
			if err == nil {
				fmt.Printf("  inline: %s %s (HTTP %d)\n", file, label, status)
				return true
			}
			fmt.Printf("  inline failed, falling back to note: %v\n", err)
		}
	}

	fallback := fmt.Sprintf("### Code Review -- `%s` (%s)\n\n%s", file, label, body)
	status, err := r.gitlab.PostNote(fallback)
	if err != nil {
		fmt.Printf("  note failed: %v\n", err)
		return false
	}
	fmt.Printf("  note: %s %s (HTTP %d)\n", file, label, status)
	return true
}

// formatMRIntent formats merge request title and description into an authoritative intent context block.
// Descriptions exceeding maxRunes are truncated with a notification marker as a defense-in-depth measure.
func formatMRIntent(title, description string, maxRunes int) string {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" && description == "" {
		return ""
	}
	const truncMarker = "... [truncated]"
	if r := []rune(description); len(r) > maxRunes {
		// Keep the marker within the budget when maxRunes is shorter than the marker.
		// A negative keep length would panic on the slice and leave the gate unprotected.
		markerRunes := []rune(truncMarker)
		keep := maxRunes - len(markerRunes)
		if keep < 0 {
			description = string(markerRunes[:maxRunes])
		} else {
			description = string(r[:keep]) + truncMarker
		}
	}
	var b strings.Builder
	b.WriteString("=== Merge Request Intent ===\n")
	if title != "" {
		b.WriteString("Title: ")
		b.WriteString(title)
		b.WriteByte('\n')
	}
	if description != "" {
		b.WriteString("Description:\n")
		b.WriteString(description)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func buildCommentBody(description, suggestion string, start, end int) string {
	if suggestion == "" {
		return description
	}
	header := "suggestion"
	return fmt.Sprintf("%s\n\n```%s\n%s\n```", description, header, suggestion)
}

func buildPosition(refs gitlab.DiffRefs, file string, info fileInfo, start, end int) map[string]any {
	if _, ok := info.lines[end]; !ok {
		if _, ok2 := info.lines[start]; ok2 {
			end = start
		}
	}
	anchor := end
	if _, ok := info.lines[anchor]; !ok {
		anchor = start
		if _, ok2 := info.lines[anchor]; !ok2 {
			return nil
		}
	}

	lp := info.lines[anchor]
	pos := map[string]any{
		"base_sha":      refs.BaseSha,
		"start_sha":     refs.StartSha,
		"head_sha":      refs.HeadSha,
		"position_type": "text",
		"new_path":      file,
		"old_path":      info.oldPath,
	}
	if lp.newLine != nil {
		pos["new_line"] = *lp.newLine
	}
	if lp.oldLine != nil {
		pos["old_line"] = *lp.oldLine
	}
	return pos
}

// buildCombinedDiff constructs the annotated diff payload for LLM prompt evaluation
// and generates per-file line index mappings for inline discussion placement.
func buildCombinedDiff(changes []gitlab.Change, llmName string) (string, map[string]fileInfo, int) {
	fileMeta := map[string]fileInfo{}
	var sections []string
	total, skipped := 0, 0
	maxDiff := ResolveMaxTotalDiff()

	for _, ch := range changes {
		newPath := ch.NewPath
		if newPath == "" {
			newPath = ch.OldPath
		}
		if newPath == "" {
			newPath = "unknown"
		}
		oldPath := ch.OldPath
		if oldPath == "" {
			oldPath = newPath
		}

		switch {
		case matchesReviewExclusion(newPath):
			fmt.Printf("Skip %s (lock/binary/generated)\n", newPath)
			skipped++
			continue
		case strings.TrimSpace(ch.Diff) == "":
			fmt.Printf("Skip %s (empty diff)\n", newPath)
			skipped++
			continue
		case total+len(ch.Diff) > maxDiff:
			fmt.Printf("Skip %s (total diff limit reached)\n", newPath)
			skipped++
			continue
		}

		parsed := parseDiff(ch.Diff)
		lines := map[int]linePos{}
		for _, l := range parsed {
			if l.newLine != nil {
				lines[*l.newLine] = linePos{newLine: l.newLine, oldLine: l.oldLine}
			}
		}
		fileMeta[newPath] = fileInfo{oldPath: oldPath, lines: lines}
		sections = append(sections, fmt.Sprintf("=== File: %s ===\n%s", newPath, annotateDiff(parsed)))
		total += len(ch.Diff)
		fmt.Printf("Queued %s (%d chars)\n", newPath, len(ch.Diff))
	}

	fmt.Printf("\nSending %d files (%d chars) to %s ...\n", len(fileMeta), total, llmName)
	return strings.Join(sections, "\n\n"), fileMeta, skipped
}
