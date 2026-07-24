package review

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"ci-tools/internal/gate"
	"ci-tools/internal/gitlab"
)

// LLMClient defines the interface for language model client integrations.
type LLMClient interface {
	Name() string
	Review(prompt string) (string, error)
}

const maxTotalDiff = 300000

// promptTemplate defines the system prompt instructing the LLM on review priorities,
// file-type inspection rules, strict raw JSON response schema requirements, and treating
// author merge request intent as authoritative context to minimize false-positive findings.
const promptTemplate = `You are an expert software engineer reviewing a merge request.

Review Context:
- Below is the annotated diff for changed files.
- Each file section starts with: === File: <path> ===
- New file lines are prefixed with [L   N].
- Removed lines are prefixed with [     ].

Merge Request Intent:
- A "=== Merge Request Intent ===" section may precede the diff containing the author's title and description.
- Treat the author's stated intent and design trade-offs as authoritative context.
- Do not raise concerns regarding intended trade-offs unless the reasoning contains factual errors or security risks.

Review Focus & Domains:
- Core: Bugs, security vulnerabilities, performance bottlenecks, architectural flaws, and code maintainability.
- Vue Components: Reactivity pitfalls, lifecycle issues, prop validation, and XSS vulnerabilities (e.g., unsafe v-html usage).
- TypeScript: Type safety enforcement, implicit any types, and unsafe type assertions.
- Infrastructure (HCL, YAML, Dockerfile): Resource constraints, security contexts, embedded secrets, and misconfigurations.

Output Format Requirements:
- Return ONLY a raw JSON array without markdown code blocks, backticks, or conversational text wrappers.
- If no significant issues are found across all files, return an empty array: []

JSON Element Schema:
{
    "file": "<exact file path from the === File: <path> === header>",
    "start_line": <integer, starting line number [L N] of the problematic range>,
    "end_line": <integer, ending line number [L N] of the problematic range; equal to start_line for single-line issues>,
    "description": "<concise markdown explanation of the defect and its technical impact>",
    "suggestion": "<optional: exact replacement lines for start_line..end_line preserving indentation; omit if no direct code replacement applies>",
    "security": <boolean, true only if this specific finding is a security vulnerability; omit or false otherwise>
}`

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
}

func New(gl *gitlab.Client, llm LLMClient) *Reviewer {
	return &Reviewer{gitlab: gl, llm: llm}
}

// RunOnMR serves as the common execution entrypoint for reviewer executables,
// encapsulating client initialization and execution workflow.
func RunOnMR(apiURL, projectID, mriid, token string, llm LLMClient) error {
	return New(gitlab.New(apiURL, projectID, mriid, token), llm).Run()
}

func (r *Reviewer) Run() error {
	maxDesc, err := gate.MaxRunes()
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

	raw, err := r.llm.Review(promptTemplate + "\n\n" + formatMRIntent(mr.Title, mr.Description, maxDesc) + combined)
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
		if r.deliver(mr.DiffRefs, fileMeta, c) {
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

// deliver posts a single review finding, attempting inline discussion placement
// before falling back to a general merge request note if line-anchoring is unavailable.
func (r *Reviewer) deliver(refs gitlab.DiffRefs, fileMeta map[string]fileInfo, c Comment) bool {
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

	body := buildBody(description, suggestion, start, end)
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
		description = string(r[:maxRunes-len([]rune(truncMarker))]) + truncMarker
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

func buildBody(description, suggestion string, start, end int) string {
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
		case shouldSkip(newPath):
			fmt.Printf("Skip %s (lock/binary/generated)\n", newPath)
			skipped++
			continue
		case strings.TrimSpace(ch.Diff) == "":
			fmt.Printf("Skip %s (empty diff)\n", newPath)
			skipped++
			continue
		case total+len(ch.Diff) > maxTotalDiff:
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
