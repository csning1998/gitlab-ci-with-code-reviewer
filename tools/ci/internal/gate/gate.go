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

// CheckLength validates that description length does not exceed max.
// Evaluation relies on UTF-8 rune counts rather than byte lengths to ensure accurate measurement
// of multibyte CJK text. The input must be retrieved directly from the GitLab API rather than
// CI_MERGE_REQUEST_DESCRIPTION, which is hard-capped at 2700 characters by GitLab CI.
func CheckLength(description string, max int) error {
	if n := utf8.RuneCountInString(description); n > max {
		return fmt.Errorf("MR description is %d characters; the limit is %d. Shorten it and update the MR", n, max)
	}
	return nil
}
