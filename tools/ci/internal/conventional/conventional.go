// Package conventional parses the Conventional Commit header shared by release tagging and merge
// request labeling.
package conventional

import (
	"regexp"
	"strings"
)

// headerPattern captures the type, the optional scope, and the optional breaking marker. The
// colon MUST be followed by whitespace, which matches the header rule of commitlint.
var headerPattern = regexp.MustCompile(`^([a-z]+)(\([^)]*\))?(!)?:\s`)

// Header holds the parts of a Conventional Commit header which drive release and label decisions.
type Header struct {
	Type     string
	Breaking bool
}

// ParseHeader parses subject after trimming surrounding whitespace. The boolean result is false
// when subject does not open with a Conventional Commit header.
func ParseHeader(subject string) (Header, bool) {
	match := headerPattern.FindStringSubmatch(strings.TrimSpace(subject))
	if match == nil {
		return Header{}, false
	}
	return Header{Type: match[1], Breaking: match[3] == "!"}, true
}
