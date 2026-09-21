package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// clearReviewEnv unsets every variable ResolveReviewOptions reads, isolating each test from
// whatever CI or shell environment hosts the run.
func clearReviewEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"REVIEW_MODEL", "REVIEW_BASE_URL", "REVIEW_API_VERSION",
		"REVIEW_MAX_TOKENS", "REVIEW_TIMEOUT_MINUTES",
		"REVIEW_TEMPERATURE", "REVIEW_TOP_P", "REVIEW_REASONING_EFFORT",
	} {
		t.Setenv(name, "")
	}
}

func TestResolveReviewOptions_UnconfiguredModelReturnsErrSlotNotConfigured(t *testing.T) {
	clearReviewEnv(t)
	_, err := ResolveReviewOptions()
	if !errors.Is(err, ErrSlotNotConfigured) {
		t.Fatalf("ResolveReviewOptions() error = %v, want ErrSlotNotConfigured", err)
	}
}

func TestResolveReviewOptions_DerivesProviderFromModel(t *testing.T) {
	tests := []struct {
		model        string
		wantProvider string
	}{
		{model: "claude-sonnet-5", wantProvider: "claude"},
		{model: "gemini-2.5-flash", wantProvider: "gemini"},
		{model: "grok-4.6", wantProvider: "grok"},
		{model: "gpt-4o", wantProvider: "openai"},
		{model: "llama-3.3-70b", wantProvider: "local"},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			clearReviewEnv(t)
			t.Setenv("REVIEW_MODEL", tc.model)

			opts, err := ResolveReviewOptions()
			if err != nil {
				t.Fatalf("ResolveReviewOptions() returned an unexpected error: %v", err)
			}
			if opts.Provider != tc.wantProvider {
				t.Errorf("Provider = %q, want %q", opts.Provider, tc.wantProvider)
			}
			if opts.Model != tc.model {
				t.Errorf("Model = %q, want %q", opts.Model, tc.model)
			}
		})
	}
}

func TestResolveReviewOptions_APIVersionSelectsAzure(t *testing.T) {
	clearReviewEnv(t)
	t.Setenv("REVIEW_MODEL", "gpt-4o")
	t.Setenv("REVIEW_BASE_URL", "https://example.openai.azure.com")
	t.Setenv("REVIEW_API_VERSION", "2026-01-01")

	opts, err := ResolveReviewOptions()
	if err != nil {
		t.Fatalf("ResolveReviewOptions() returned an unexpected error: %v", err)
	}
	if opts.Provider != "azure-openai" {
		t.Errorf("Provider = %q, want %q", opts.Provider, "azure-openai")
	}
	if opts.BaseURL != "https://example.openai.azure.com" {
		t.Errorf("BaseURL = %q, want the configured endpoint", opts.BaseURL)
	}
	if opts.APIVersion != "2026-01-01" {
		t.Errorf("APIVersion = %q, want %q", opts.APIVersion, "2026-01-01")
	}
}

func TestResolveReviewOptions_NormalizesAlias(t *testing.T) {
	clearReviewEnv(t)
	t.Setenv("REVIEW_MODEL", "claude-opus-4.7")

	opts, err := ResolveReviewOptions()
	if err != nil {
		t.Fatalf("ResolveReviewOptions() returned an unexpected error: %v", err)
	}
	if opts.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want the canonical identifier %q", opts.Model, "claude-opus-4-7")
	}
}

func TestResolveReviewOptions_RejectsUnapprovedModel(t *testing.T) {
	clearReviewEnv(t)
	t.Setenv("REVIEW_MODEL", "deepseek-r1")

	_, err := ResolveReviewOptions()
	if err == nil || !strings.Contains(err.Error(), "deepseek-r1") {
		t.Fatalf("ResolveReviewOptions() error = %v, want the rejected model named", err)
	}
}

func TestResolveReviewOptions_ParsesTunables(t *testing.T) {
	clearReviewEnv(t)
	t.Setenv("REVIEW_MODEL", "gpt-4o")
	t.Setenv("REVIEW_MAX_TOKENS", "2048")
	t.Setenv("REVIEW_TIMEOUT_MINUTES", "7")
	t.Setenv("REVIEW_TEMPERATURE", "0.4")
	t.Setenv("REVIEW_TOP_P", "0.9")
	t.Setenv("REVIEW_REASONING_EFFORT", "high")

	opts, err := ResolveReviewOptions()
	if err != nil {
		t.Fatalf("ResolveReviewOptions() returned an unexpected error: %v", err)
	}
	if opts.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", opts.MaxTokens)
	}
	if opts.Timeout != 7*time.Minute {
		t.Errorf("Timeout = %v, want %v", opts.Timeout, 7*time.Minute)
	}
	if opts.Temperature == nil || *opts.Temperature != 0.4 {
		t.Errorf("Temperature = %v, want 0.4", opts.Temperature)
	}
	if opts.TopP == nil || *opts.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", opts.TopP)
	}
	if opts.ReasoningLevel != "high" {
		t.Errorf("ReasoningLevel = %q, want %q", opts.ReasoningLevel, "high")
	}
}

func TestResolveReviewOptions_RejectsMalformedTunable(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		value string
	}{
		{name: "non numeric max tokens", env: "REVIEW_MAX_TOKENS", value: "abc"},
		{name: "non positive max tokens", env: "REVIEW_MAX_TOKENS", value: "-5"},
		{name: "unit suffixed timeout", env: "REVIEW_TIMEOUT_MINUTES", value: "10m"},
		{name: "non numeric temperature", env: "REVIEW_TEMPERATURE", value: "abc"},
		{name: "non numeric top p", env: "REVIEW_TOP_P", value: "abc"},
		{name: "not a number temperature", env: "REVIEW_TEMPERATURE", value: "NaN"},
		{name: "infinite top p", env: "REVIEW_TOP_P", value: "Inf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearReviewEnv(t)
			t.Setenv("REVIEW_MODEL", "gpt-4o")
			t.Setenv(tc.env, tc.value)

			_, err := ResolveReviewOptions()
			if err == nil || !strings.Contains(err.Error(), tc.env) {
				t.Fatalf("ResolveReviewOptions() error = %v, want the malformed variable %s named", err, tc.env)
			}
		})
	}
}

func TestResolveReviewOptions_DefaultsTimeoutWhenUnset(t *testing.T) {
	clearReviewEnv(t)
	t.Setenv("REVIEW_MODEL", "gpt-4o")

	opts, err := ResolveReviewOptions()
	if err != nil {
		t.Fatalf("ResolveReviewOptions() returned an unexpected error: %v", err)
	}
	if opts.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want the %v fallback", opts.Timeout, DefaultTimeout)
	}
}

func TestResolveReviewOptions_RejectsIncompatibleTunable(t *testing.T) {
	// Validate rejects a penalty coefficient on claude. The rejection MUST surface here rather
	// than reaching the provider endpoint.
	clearReviewEnv(t)
	t.Setenv("REVIEW_MODEL", "claude-sonnet-5")
	t.Setenv("REVIEW_TEMPERATURE", "1.8")

	if _, err := ResolveReviewOptions(); err == nil {
		t.Fatal("ResolveReviewOptions() succeeded unexpectedly; want the claude temperature ceiling enforced")
	}
}
