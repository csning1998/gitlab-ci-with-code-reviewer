package gate

import (
	"strings"
	"testing"
)

func TestResolveDescriptionRuneLimit_DefaultWhenUnset(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "")
	n, err := ResolveDescriptionRuneLimit()
	if err != nil {
		t.Fatalf("ResolveDescriptionRuneLimit() returned an unexpected error: %v", err)
	}
	if n != defaultMaxRunes {
		t.Errorf("ResolveDescriptionRuneLimit() = %d, want the default %d", n, defaultMaxRunes)
	}
}

func TestResolveDescriptionRuneLimit_ValidOverride(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "1234")
	n, err := ResolveDescriptionRuneLimit()
	if err != nil {
		t.Fatalf("ResolveDescriptionRuneLimit() returned an unexpected error: %v", err)
	}
	if n != 1234 {
		t.Errorf("ResolveDescriptionRuneLimit() = %d, want 1234", n)
	}
}

func TestResolveDescriptionRuneLimit_WhitespacePaddedValue(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "  500  ")
	n, err := ResolveDescriptionRuneLimit()
	if err != nil {
		t.Fatalf("ResolveDescriptionRuneLimit() returned an unexpected error: %v", err)
	}
	if n != 500 {
		t.Errorf("ResolveDescriptionRuneLimit() = %d, want 500", n)
	}
}

func TestResolveDescriptionRuneLimit_NonNumeric(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "abc")
	if _, err := ResolveDescriptionRuneLimit(); err == nil {
		t.Error("ResolveDescriptionRuneLimit() succeeded unexpectedly with a non-numeric value; want an error")
	}
}

func TestResolveDescriptionRuneLimit_Zero(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "0")
	if _, err := ResolveDescriptionRuneLimit(); err == nil {
		t.Error("ResolveDescriptionRuneLimit() succeeded unexpectedly with 0; want a positive-integer error")
	}
}

func TestResolveDescriptionRuneLimit_Negative(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "-5")
	if _, err := ResolveDescriptionRuneLimit(); err == nil {
		t.Error("ResolveDescriptionRuneLimit() succeeded unexpectedly with a negative value; want an error")
	}
}

func TestResolveDescriptionRuneLimit_FloatingPointValue(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "100.5")
	if _, err := ResolveDescriptionRuneLimit(); err == nil {
		t.Error("ResolveDescriptionRuneLimit() succeeded unexpectedly with a floating-point value; want an integer parse error")
	}
}

func TestResolveDescriptionRuneLimit_WhitespaceOnly(t *testing.T) {
	t.Setenv("MAX_DESCRIPTION_CHARS", "   ")
	n, err := ResolveDescriptionRuneLimit()
	if err != nil {
		t.Fatalf("ResolveDescriptionRuneLimit() returned an unexpected error: %v", err)
	}
	if n != defaultMaxRunes {
		t.Errorf("ResolveDescriptionRuneLimit() = %d, want the default %d for a whitespace-only value (trimmed to empty)", n, defaultMaxRunes)
	}
}

func TestValidateDescriptionLength_WithinLimit(t *testing.T) {
	if err := ValidateDescriptionLength("short description", 100); err != nil {
		t.Errorf("ValidateDescriptionLength(...) returned an unexpected error: %v", err)
	}
}

func TestValidateDescriptionLength_ExactlyAtLimit(t *testing.T) {
	if err := ValidateDescriptionLength(strings.Repeat("a", 10), 10); err != nil {
		t.Errorf("ValidateDescriptionLength(...) returned an unexpected error at exactly the limit: %v", err)
	}
}

func TestValidateDescriptionLength_ExceedsLimitByOne(t *testing.T) {
	if err := ValidateDescriptionLength(strings.Repeat("a", 11), 10); err == nil {
		t.Error("ValidateDescriptionLength(...) succeeded unexpectedly one rune over the limit; want an error")
	}
}

func TestValidateDescriptionLength_EmptyDescription(t *testing.T) {
	if err := ValidateDescriptionLength("", 10); err != nil {
		t.Errorf("ValidateDescriptionLength(\"\", 10) returned an unexpected error: %v", err)
	}
}

func TestValidateDescriptionLength_ZeroMaxWithNonEmptyDescription(t *testing.T) {
	if err := ValidateDescriptionLength("x", 0); err == nil {
		t.Error("ValidateDescriptionLength(\"x\", 0) succeeded unexpectedly; want an error")
	}
}

func TestValidateDescriptionLength_ZeroMaxWithEmptyDescription(t *testing.T) {
	if err := ValidateDescriptionLength("", 0); err != nil {
		t.Errorf("ValidateDescriptionLength(\"\", 0) returned an unexpected error: %v", err)
	}
}

func TestValidateDescriptionLength_CJKRuneCountingNotByteCounting(t *testing.T) {
	// Each of these 5 characters is a 3-byte UTF-8 sequence (15 bytes total), but the rune count
	// is what must be compared against the limit.
	description := strings.Repeat("測", 5)
	if err := ValidateDescriptionLength(description, 5); err != nil {
		t.Errorf("ValidateDescriptionLength(...) returned an unexpected error for 5 CJK runes against a limit of 5: %v", err)
	}
	if err := ValidateDescriptionLength(description, 4); err == nil {
		t.Error("ValidateDescriptionLength(...) succeeded unexpectedly for 5 CJK runes against a limit of 4; want an error")
	}
}

func TestValidateDescriptionLength_ErrorMessageReportsActualAndLimit(t *testing.T) {
	err := ValidateDescriptionLength(strings.Repeat("a", 15), 10)
	if err == nil {
		t.Fatal("ValidateDescriptionLength(...) succeeded unexpectedly; want an error")
	}
	if !strings.Contains(err.Error(), "15") || !strings.Contains(err.Error(), "10") {
		t.Errorf("ValidateDescriptionLength(...) error = %q, want it to report both the actual length and the limit", err.Error())
	}
}

func TestValidateDescriptionLength_EmojiSurrogatePairRuneCounting(t *testing.T) {
	// An emoji outside the Basic Multilingual Plane still counts as a single Go rune despite
	// requiring 4 UTF-8 bytes and a UTF-16 surrogate pair in other encodings.
	description := strings.Repeat("🎉", 3)
	if err := ValidateDescriptionLength(description, 3); err != nil {
		t.Errorf("ValidateDescriptionLength(...) returned an unexpected error for 3 emoji runes against a limit of 3: %v", err)
	}
}
