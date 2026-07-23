package review

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// File extensions and filenames excluded from LLM code reviews to optimize prompt token limits
// and prevent noise from auto-generated lockfiles, compiled binaries, and minified assets.
var skipExtensions = map[string]bool{
	".lock": true, ".sum": true, ".min.js": true, ".min.css": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".svg": true, ".webp": true, ".pdf": true, ".zip": true, ".tar": true,
	".gz": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".pem": true, ".crt": true,
}

var skipFilenames = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "bun.lock": true,
	"go.sum": true, "poetry.lock": true, "Pipfile.lock": true,
	"composer.lock": true, "pnpm-lock.yaml": true,
	".terraform.lock.hcl": true, "packer_manifest.json": true,
}

type diffLine struct {
	newLine *int
	oldLine *int
	prefix  string
	content string
}

type linePos struct {
	newLine *int
	oldLine *int
}

type fileInfo struct {
	oldPath string
	lines   map[int]linePos
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parseDiff parses a unified diff string into indexed diffLine structures containing line numbers
// for both new and old file revisions.
func parseDiff(diff string) []diffLine {
	var result []diffLine
	var newLine, oldLine *int

	for _, line := range strings.Split(strings.TrimSuffix(diff, "\n"), "\n") {
		if m := hunkRe.FindStringSubmatch(line); m != nil {
			o, _ := strconv.Atoi(m[1])
			n, _ := strconv.Atoi(m[2])
			oldLine, newLine = &o, &n
			result = append(result, diffLine{prefix: "@@", content: line})
			continue
		}
		if newLine == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			nl := *newLine
			result = append(result, diffLine{newLine: &nl, prefix: "+", content: line[1:]})
			*newLine++
		case strings.HasPrefix(line, "-"):
			ol := *oldLine
			result = append(result, diffLine{oldLine: &ol, prefix: "-", content: line[1:]})
			*oldLine++
		case strings.HasPrefix(line, "\\"):
			// Ignore unified diff EOF marker indicating absence of a trailing newline.
		default:
			nl, ol := *newLine, *oldLine
			content := ""
			if len(line) > 0 {
				content = line[1:]
			}
			result = append(result, diffLine{newLine: &nl, oldLine: &ol, prefix: " ", content: content})
			*newLine++
			*oldLine++
		}
	}
	return result
}

// annotateDiff formats diff lines with explicit line tags ([L N] for additions/context, [ ] for deletions)
// to establish unambiguous line reference targets for LLM response evaluation.
func annotateDiff(lines []diffLine) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		switch l.prefix {
		case "@@":
			out = append(out, l.content)
		case "+":
			out = append(out, fmt.Sprintf("[L%4d] + %s", *l.newLine, l.content))
		case "-":
			out = append(out, fmt.Sprintf("[     ] - %s", l.content))
		default:
			out = append(out, fmt.Sprintf("[L%4d]   %s", *l.newLine, l.content))
		}
	}
	return strings.Join(out, "\n")
}

func shouldSkip(path string) bool {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	return skipFilenames[base] || skipExtensions[ext]
}
