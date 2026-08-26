package tokensource

import (
	"context"
	"errors"
	"testing"
)

func TestStatic_ReturnsConfiguredValue(t *testing.T) {
	got, err := Static("xai-key").Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != "xai-key" {
		t.Errorf("Token() = %q, want %q", got, "xai-key")
	}
}

func TestStatic_RejectsEmptyValue(t *testing.T) {
	_, err := Static("").Token(context.Background())
	if !errors.Is(err, errEmptyStatic) {
		t.Errorf("Token() error = %v, want errEmptyStatic", err)
	}
}
