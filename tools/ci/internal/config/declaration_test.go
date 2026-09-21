package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeDeclaration materializes a reviewer declaration inside an isolated directory and returns
// the file path.
func writeDeclaration(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "reviewer.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write declaration: %v", err)
	}
	return path
}

func TestResolveDeclaredOptions_MergesDefaults(t *testing.T) {
	path := writeDeclaration(t, `
defaults:
  max_tokens: 8192
  timeout: 5m
models:
  fast-reviewer:
    provider: gemini
    model: gemini-3.6-flash
`)

	opts, err := ResolveDeclaredOptions(path, "fast-reviewer")
	if err != nil {
		t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
	}
	if opts.Provider != "gemini" {
		t.Errorf("Provider = %q, want %q", opts.Provider, "gemini")
	}
	if opts.Model != "gemini-3.6-flash" {
		t.Errorf("Model = %q, want %q", opts.Model, "gemini-3.6-flash")
	}
	if opts.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want the inherited 8192", opts.MaxTokens)
	}
	if opts.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want the inherited %v", opts.Timeout, 5*time.Minute)
	}
}

func TestResolveDeclaredOptions_NamedEntryOverridesDefaults(t *testing.T) {
	path := writeDeclaration(t, `
defaults:
  max_tokens: 8192
  temperature: 0.1
models:
  deep-reviewer:
    provider: grok
    model: grok-4.6
    max_tokens: 4096
    reasoning_level: xhigh
`)

	opts, err := ResolveDeclaredOptions(path, "deep-reviewer")
	if err != nil {
		t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
	}
	if opts.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want the entry override 4096", opts.MaxTokens)
	}
	if opts.ReasoningLevel != "xhigh" {
		t.Errorf("ReasoningLevel = %q, want %q", opts.ReasoningLevel, "xhigh")
	}
	if opts.Temperature == nil || *opts.Temperature != 0.1 {
		t.Errorf("Temperature = %v, want the inherited 0.1", opts.Temperature)
	}
}

func TestResolveDeclaredOptions_SlotIndirection(t *testing.T) {
	path := writeDeclaration(t, `
models:
  primary-reviewer:
    provider: claude
    model: claude-sonnet-5
  secondary-reviewer:
    provider: gemini
    model: gemini-2.5-flash
slots:
  primary: primary-reviewer
  secondary: secondary-reviewer
`)

	tests := []struct {
		slot      string
		wantModel string
	}{
		{slot: "primary", wantModel: "claude-sonnet-5"},
		{slot: "secondary", wantModel: "gemini-2.5-flash"},
	}

	for _, tc := range tests {
		t.Run(tc.slot, func(t *testing.T) {
			opts, err := ResolveDeclaredOptions(path, tc.slot)
			if err != nil {
				t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
			}
			if opts.Model != tc.wantModel {
				t.Errorf("Model = %q, want %q", opts.Model, tc.wantModel)
			}
		})
	}
}

func TestResolveDeclaredOptions_MultipleEntriesResolveIndependently(t *testing.T) {
	// A declaration carries an unbounded number of named models. Each key MUST resolve without
	// interference from any sibling entry.
	path := writeDeclaration(t, `
defaults:
  max_tokens: 2048
models:
  a:
    model: claude-opus-4-6
  b:
    model: gemini-3.5-flash
    max_tokens: 1024
  c:
    model: grok-4.5
  d:
    model: gpt-4o
  e:
    model: llama-3.3-70b
`)

	wantProviders := map[string]string{
		"a": "claude",
		"b": "gemini",
		"c": "grok",
		"d": "openai",
		"e": "local",
	}
	for key, wantProvider := range wantProviders {
		t.Run(key, func(t *testing.T) {
			opts, err := ResolveDeclaredOptions(path, key)
			if err != nil {
				t.Fatalf("ResolveDeclaredOptions(..., %q) returned an unexpected error: %v", key, err)
			}
			if opts.Provider != wantProvider {
				t.Errorf("Provider = %q, want %q", opts.Provider, wantProvider)
			}
		})
	}

	opts, err := ResolveDeclaredOptions(path, "b")
	if err != nil {
		t.Fatalf("ResolveDeclaredOptions(..., \"b\") returned an unexpected error: %v", err)
	}
	if opts.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d, want the entry override 1024", opts.MaxTokens)
	}
}

func TestResolveDeclaredOptions_DerivesProviderWhenOmitted(t *testing.T) {
	path := writeDeclaration(t, `
models:
  azure-reviewer:
    model: gpt-4o
    base_url: https://example.openai.azure.com
    api_version: "2026-01-01"
`)

	opts, err := ResolveDeclaredOptions(path, "azure-reviewer")
	if err != nil {
		t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
	}
	if opts.Provider != "azure-openai" {
		t.Errorf("Provider = %q, want %q", opts.Provider, "azure-openai")
	}
}

func TestResolveDeclaredOptions_NormalizesAlias(t *testing.T) {
	path := writeDeclaration(t, `
models:
  aliased:
    provider: claude
    model: claude-opus-4.7
`)

	opts, err := ResolveDeclaredOptions(path, "aliased")
	if err != nil {
		t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
	}
	if opts.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want the canonical identifier %q", opts.Model, "claude-opus-4-7")
	}
}

func TestResolveDeclaredOptions_DefaultsTimeoutWhenUnset(t *testing.T) {
	path := writeDeclaration(t, `
models:
  plain:
    model: grok-4.6
`)

	opts, err := ResolveDeclaredOptions(path, "plain")
	if err != nil {
		t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
	}
	if opts.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want the %v fallback", opts.Timeout, DefaultTimeout)
	}
}

func TestResolveDeclaredOptions_MissingFileReturnsErrSlotNotConfigured(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.yml")
	if _, err := ResolveDeclaredOptions(missing, "primary"); !errors.Is(err, ErrSlotNotConfigured) {
		t.Fatalf("ResolveDeclaredOptions(...) error = %v, want ErrSlotNotConfigured", err)
	}
}

func TestResolveDeclaredOptions_UnknownKeyReturnsErrSlotNotConfigured(t *testing.T) {
	path := writeDeclaration(t, `
models:
  known:
    model: grok-4.6
`)

	if _, err := ResolveDeclaredOptions(path, "unknown"); !errors.Is(err, ErrSlotNotConfigured) {
		t.Fatalf("ResolveDeclaredOptions(...) error = %v, want ErrSlotNotConfigured", err)
	}
}

func TestResolveDeclaredOptions_MalformedYAMLReturnsError(t *testing.T) {
	path := writeDeclaration(t, "models:\n  broken: [unclosed\n")

	_, err := ResolveDeclaredOptions(path, "broken")
	if err == nil {
		t.Fatal("ResolveDeclaredOptions(...) succeeded unexpectedly on malformed YAML; want an error")
	}
	if errors.Is(err, ErrSlotNotConfigured) {
		t.Error("a malformed declaration must not be reported as an unconfigured slot")
	}
}

func TestResolveDeclaredOptions_RejectsUnapprovedModel(t *testing.T) {
	path := writeDeclaration(t, `
models:
  smuggled:
    model: deepseek-r1
`)

	_, err := ResolveDeclaredOptions(path, "smuggled")
	if err == nil || !strings.Contains(err.Error(), "deepseek-r1") {
		t.Fatalf("ResolveDeclaredOptions(...) error = %v, want the rejected model named", err)
	}
}

func TestResolveDeclaredOptions_RejectsIncompatibleField(t *testing.T) {
	path := writeDeclaration(t, `
models:
  bad-claude:
    provider: claude
    model: claude-sonnet-5
    frequency_penalty: 0.5
`)

	if _, err := ResolveDeclaredOptions(path, "bad-claude"); err == nil {
		t.Fatal("ResolveDeclaredOptions(...) succeeded unexpectedly; want the claude compatibility check enforced")
	}
}
