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

func TestResolveDeclaredOptions_ProviderPrecedence(t *testing.T) {
	// An explicit provider MUST win over derivation. Derivation MUST run only when the merged provider is blank.
	tests := []struct {
		name         string
		body         string
		wantProvider string
		wantErr      string
	}{
		{
			name:         "defaults provider is inherited by an entry that omits it",
			body:         "defaults:\n  provider: gemini\nmodels:\n  k:\n    model: gemini-3.5-flash\n",
			wantProvider: "gemini",
		},
		{
			name:    "inherited defaults provider rejects a model it does not publish",
			body:    "defaults:\n  provider: gemini\nmodels:\n  k:\n    model: claude-sonnet-5\n",
			wantErr: `for provider "gemini"`,
		},
		{
			name:         "entry provider overrides defaults provider",
			body:         "defaults:\n  provider: gemini\nmodels:\n  k:\n    provider: claude\n    model: claude-sonnet-5\n",
			wantProvider: "claude",
		},
		{
			name:         "blank defaults provider falls through to derivation",
			body:         "defaults:\n  provider: \"  \"\nmodels:\n  k:\n    model: claude-sonnet-5\n",
			wantProvider: "claude",
		},
		{
			name:         "blank entry provider inherits defaults provider",
			body:         "defaults:\n  provider: gemini\nmodels:\n  k:\n    provider: \"  \"\n    model: gemini-3.5-flash\n",
			wantProvider: "gemini",
		},
		{
			name:    "unknown defaults provider is rejected",
			body:    "defaults:\n  provider: nonesuch\nmodels:\n  k:\n    model: claude-sonnet-5\n",
			wantErr: `unknown provider "nonesuch"`,
		},
		{
			name:         "defaults model is inherited when the entry omits it",
			body:         "defaults:\n  model: claude-sonnet-5\nmodels:\n  k:\n    max_tokens: 512\n",
			wantProvider: "claude",
		},
		{
			name:         "api version does not reroute a non OpenAI model",
			body:         "defaults:\n  api_version: \"2026-01-01\"\nmodels:\n  k:\n    model: claude-sonnet-5\n",
			wantProvider: "claude",
		},
		{
			name:         "explicit openai provider wins over api version routing",
			body:         "defaults:\n  provider: openai\n  api_version: \"2026-01-01\"\nmodels:\n  k:\n    model: gpt-4o\n",
			wantProvider: "openai",
		},
		{
			name:         "api version routes a shared OpenAI model to azure",
			body:         "defaults:\n  api_version: \"2026-01-01\"\nmodels:\n  k:\n    model: gpt-4o\n",
			wantProvider: "azure-openai",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := ResolveDeclaredOptions(writeDeclaration(t, tc.body), "k")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ResolveDeclaredOptions(...) error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveDeclaredOptions(...) returned an unexpected error: %v", err)
			}
			if opts.Provider != tc.wantProvider {
				t.Errorf("Provider = %q, want %q", opts.Provider, tc.wantProvider)
			}
		})
	}
}

func TestResolveDeclaredOptions_SlotBoundToUndeclaredModelNamesTheBinding(t *testing.T) {
	path := writeDeclaration(t, `
models:
  declared:
    model: grok-4.6
slots:
  primary: missing
`)

	_, err := ResolveDeclaredOptions(path, "primary")
	if err == nil {
		t.Fatal("ResolveDeclaredOptions(...) succeeded unexpectedly; want a dangling binding rejected")
	}
	if errors.Is(err, ErrSlotNotConfigured) {
		t.Error("a dangling slot binding must not be reported as an unconfigured slot")
	}
	for _, want := range []string{`"primary"`, `"missing"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want %s named", err.Error(), want)
		}
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

func TestResolveDeclaredOptions_NonFiniteYAMLNumbersAreRejected(t *testing.T) {
	for _, literal := range []string{".nan", ".NaN", ".inf", "-.inf", "+.Inf"} {
		t.Run(literal, func(t *testing.T) {
			path := writeDeclaration(t, "models:\n  a:\n    model: gemini-3.5-flash\n    temperature: "+literal+"\n")
			if opts, err := ResolveDeclaredOptions(path, "a"); err == nil {
				t.Errorf("ResolveDeclaredOptions accepted temperature %v", *opts.Temperature)
			}
		})
	}
}

func TestResolveDeclaredOptions_EmptyDeclarationReportsUnconfiguredSlot(t *testing.T) {
	for name, body := range map[string]string{"empty": "", "comments": "# none\n", "empty maps": "models: {}\nslots: {}\n"} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveDeclaredOptions(writeDeclaration(t, body), "primary"); !errors.Is(err, ErrSlotNotConfigured) {
				t.Errorf("error = %v, want ErrSlotNotConfigured", err)
			}
		})
	}
}

func TestResolveDeclaredOptions_KeyBoundaries(t *testing.T) {
	path := writeDeclaration(t, "models:\n  a:\n    model: grok-4.6\n  \"a b\":\n    model: grok-4.5\nslots:\n  primary: a\n")

	tests := []struct {
		name      string
		key       string
		wantModel string
		wantErr   bool
	}{
		{name: "exact key", key: "a", wantModel: "grok-4.6"},
		{name: "padded key", key: "  a  ", wantModel: "grok-4.6"},
		{name: "key with interior space", key: "a b", wantModel: "grok-4.5"},
		{name: "uppercase key", key: "A", wantErr: true},
		{name: "slot", key: "primary", wantModel: "grok-4.6"},
		{name: "padded slot", key: "\tprimary\n", wantModel: "grok-4.6"},
		{name: "empty key", key: "", wantErr: true},
		{name: "whitespace key", key: "   ", wantErr: true},
		{name: "key with nul", key: "a\x00", wantErr: true},
		{name: "very long key", key: strings.Repeat("a", 1<<16), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := ResolveDeclaredOptions(path, tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveDeclaredOptions(%q) succeeded unexpectedly", tc.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveDeclaredOptions(%q) returned an unexpected error: %v", tc.key, err)
			}
			if opts.Model != tc.wantModel {
				t.Errorf("Model = %q, want %q", opts.Model, tc.wantModel)
			}
		})
	}
}

// A declaration replaced between two calls MUST be observed by the second call. An unreadable
// replacement MUST return a read failure.
func TestResolveDeclaredOptions_ReplacedFileIsObservedOnNextCall(t *testing.T) {
	path := writeDeclaration(t, "models:\n  a:\n    model: grok-4.6\n")

	first, err := ResolveDeclaredOptions(path, "a")
	if err != nil {
		t.Fatalf("first call returned an unexpected error: %v", err)
	}
	if first.Model != "grok-4.6" {
		t.Fatalf("first Model = %q, want grok-4.6", first.Model)
	}

	if err := os.WriteFile(path, []byte("models:\n  a:\n    model: grok-4.5\n"), 0o600); err != nil {
		t.Fatalf("rewrite declaration: %v", err)
	}
	second, err := ResolveDeclaredOptions(path, "a")
	if err != nil {
		t.Fatalf("second call returned an unexpected error: %v", err)
	}
	if second.Model != "grok-4.5" {
		t.Errorf("second Model = %q, want grok-4.5", second.Model)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove declaration: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace declaration with directory: %v", err)
	}
	if _, err := ResolveDeclaredOptions(path, "a"); err == nil || errors.Is(err, ErrSlotNotConfigured) {
		t.Errorf("error = %v, want a read failure distinct from ErrSlotNotConfigured", err)
	}
}

func TestResolveDeclaredOptions_SymlinkedDeclarationIsFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.yml")
	if err := os.WriteFile(target, []byte("models:\n  a:\n    model: grok-4.6\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "reviewer.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := ResolveDeclaredOptions(link, "a"); err != nil {
		t.Errorf("ResolveDeclaredOptions through a symlink returned an unexpected error: %v", err)
	}
}
