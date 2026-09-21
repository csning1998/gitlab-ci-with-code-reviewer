package config

import (
	"strings"
	"testing"
)

func TestResolveProvider_UniqueReverseLookup(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{model: "claude-sonnet-5", want: "claude"},
		{model: "claude-opus-4-5-20251101", want: "claude"},
		{model: "claude-opus-4.7", want: "claude"},
		{model: "gemini-2.5-flash", want: "gemini"},
		{model: "gemini-3-pro", want: "gemini"},
		{model: "gemma-4-31b-it", want: "gemini"},
		{model: "grok-4.6", want: "grok"},
		{model: "grok-4.6-latest", want: "grok"},
		{model: "gemma-4-31b", want: "local"},
		{model: "llama-3.3-70b", want: "local"},
		{model: "codellama-70b", want: "local"},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			got, err := ResolveProvider(tc.model, "")
			if err != nil {
				t.Fatalf("ResolveProvider(%q, \"\") returned an unexpected error: %v", tc.model, err)
			}
			if got != tc.want {
				t.Errorf("ResolveProvider(%q, \"\") = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}

func TestResolveProvider_OpenAIFamilyDefaultsToOpenAI(t *testing.T) {
	// openai and azure-openai publish an identical model set. The shared identifiers belong to
	// openai unless the caller supplies an api-version.
	for _, model := range []string{"gpt-4o", "gpt-5.4-mini", "gpt-5.6", "o3-mini", "o4-mini-2025-04-16"} {
		t.Run(model, func(t *testing.T) {
			got, err := ResolveProvider(model, "")
			if err != nil {
				t.Fatalf("ResolveProvider(%q, \"\") returned an unexpected error: %v", model, err)
			}
			if got != "openai" {
				t.Errorf("ResolveProvider(%q, \"\") = %q, want %q", model, got, "openai")
			}
		})
	}
}

func TestResolveProvider_APIVersionSelectsAzure(t *testing.T) {
	for _, model := range []string{"gpt-4o", "gpt-5", "o1"} {
		t.Run(model, func(t *testing.T) {
			got, err := ResolveProvider(model, "2026-01-01")
			if err != nil {
				t.Fatalf("ResolveProvider(%q, apiVersion) returned an unexpected error: %v", model, err)
			}
			if got != "azure-openai" {
				t.Errorf("ResolveProvider(%q, apiVersion) = %q, want %q", model, got, "azure-openai")
			}
		})
	}
}

func TestResolveProvider_APIVersionIgnoredOutsideOpenAIFamily(t *testing.T) {
	// A stray api-version MUST NOT reroute a model which only one provider publishes.
	got, err := ResolveProvider("claude-sonnet-5", "2026-01-01")
	if err != nil {
		t.Fatalf("ResolveProvider(...) returned an unexpected error: %v", err)
	}
	if got != "claude" {
		t.Errorf("ResolveProvider(\"claude-sonnet-5\", apiVersion) = %q, want %q", got, "claude")
	}
}

func TestResolveProvider_UnapprovedModelRejected(t *testing.T) {
	for _, model := range []string{"deepseek-r1", "qwen-2.5-coder", "claude-3-opus-20240229", "gemini-1.5-pro", ""} {
		t.Run(model, func(t *testing.T) {
			_, err := ResolveProvider(model, "")
			if err == nil {
				t.Fatalf("ResolveProvider(%q, \"\") succeeded unexpectedly; want a rejection", model)
			}
			if model != "" && !strings.Contains(err.Error(), model) {
				t.Errorf("ResolveProvider(%q, \"\") error = %q, want the rejected model named", model, err.Error())
			}
		})
	}
}

func TestResolveProvider_TrimsSurroundingWhitespace(t *testing.T) {
	got, err := ResolveProvider("  grok-4.5  ", "")
	if err != nil {
		t.Fatalf("ResolveProvider(...) returned an unexpected error: %v", err)
	}
	if got != "grok" {
		t.Errorf("ResolveProvider(\"  grok-4.5  \", \"\") = %q, want %q", got, "grok")
	}
}
