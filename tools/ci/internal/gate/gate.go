package gate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const defaultMaxRunes = 5000

// MaxRunes reads the MAX_DESCRIPTION_CHARS limit, falling back to the default.
func MaxRunes() (int, error) {
	v := strings.TrimSpace(os.Getenv("MAX_DESCRIPTION_CHARS"))
	if v == "" {
		return defaultMaxRunes, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid MAX_DESCRIPTION_CHARS %q: want a positive integer", v)
	}
	return n, nil
}

// CheckLength fails when the description exceeds max. It counts runes, not bytes,
// so CJK text is measured accurately. The caller passes the full description from
// the GitLab API, not CI_MERGE_REQUEST_DESCRIPTION, which GitLab caps at 2700.
func CheckLength(description string, max int) error {
	if n := utf8.RuneCountInString(description); n > max {
		return fmt.Errorf("MR description is %d characters; the limit is %d. Shorten it and update the MR", n, max)
	}
	return nil
}
