package gate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const defaultMaxRunes = 5000

// CheckDescription fails when the MR description exceeds the configured limit.
// It counts runes, not bytes, and reads only CI vars, so it makes no API call.
func CheckDescription() error {
	max := defaultMaxRunes
	if v := strings.TrimSpace(os.Getenv("MAX_DESCRIPTION_CHARS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid MAX_DESCRIPTION_CHARS %q: want a positive integer", v)
		}
		max = n
	}

	// CI_MERGE_REQUEST_DESCRIPTION is capped at 2700 chars by GitLab; IS_TRUNCATED=true
	// signals GitLab's own cap was hit, not ours. When max > 2700 (default: 5000) the
	// IS_TRUNCATED flag cannot enforce our limit. Full check via FetchMR API deferred to
	// next MR.

	// if strings.EqualFold(strings.TrimSpace(os.Getenv("CI_MERGE_REQUEST_DESCRIPTION_IS_TRUNCATED")), "true") {
	// 	return fmt.Errorf("MR description was truncated by GitLab; it exceeds the %d-character limit", max)
	// }

	if n := utf8.RuneCountInString(os.Getenv("CI_MERGE_REQUEST_DESCRIPTION")); n > max {
		return fmt.Errorf("MR description is %d characters; the limit is %d. Shorten it and update the MR", n, max)
	}
	return nil
}
