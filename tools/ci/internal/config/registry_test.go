package config

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

type resolveProviderCase struct {
	name       string
	model      string
	apiVersion string
	want       string
	wantErr    bool
}

func TestResolveProvider_Matrix(t *testing.T) {
	tests := []resolveProviderCase{
		// Canonical unique lookups
		{name: "claude sonnet", model: "claude-sonnet-5", want: "claude"},
		{name: "claude dated snapshot", model: "claude-opus-4-5-20251101", want: "claude"},
		{name: "claude alias dot notation", model: "claude-opus-4.7", want: "claude"},
		{name: "gemini flash", model: "gemini-2.5-flash", want: "gemini"},
		{name: "gemini pro alias", model: "gemini-3-pro", want: "gemini"},
		{name: "gemma on gemini", model: "gemma-4-31b-it", want: "gemini"},
		{name: "grok canonical", model: "grok-4.6", want: "grok"},
		{name: "grok alias latest", model: "grok-4.6-latest", want: "grok"},
		{name: "local gemma", model: "gemma-4-31b", want: "local"},
		{name: "local llama", model: "llama-3.3-70b", want: "local"},
		{name: "local codellama", model: "codellama-70b", want: "local"},

		// OpenAI family routing
		{name: "openai default without api version", model: "gpt-4o", want: "openai"},
		{name: "openai mini without api version", model: "gpt-5.4-mini", want: "openai"},
		{name: "openai alias without api version", model: "gpt-5.6", want: "openai"},
		{name: "openai o3 without api version", model: "o3-mini", want: "openai"},
		{name: "openai o4-mini without api version", model: "o4-mini-2025-04-16", want: "openai"},
		{name: "azure with api version", model: "gpt-4o", apiVersion: "2026-01-01", want: "azure-openai"},
		{name: "azure gpt-5 with api version", model: "gpt-5", apiVersion: "2026-01-01", want: "azure-openai"},
		{name: "azure o1 with api version", model: "o1", apiVersion: "2026-01-01", want: "azure-openai"},
		{name: "non openai model ignores api version", model: "claude-sonnet-5", apiVersion: "2026-01-01", want: "claude"},

		// Whitespace handling
		{name: "surrounding whitespace in model", model: "  grok-4.5  ", want: "grok"},
		{name: "surrounding whitespace in api version", model: "gpt-4o", apiVersion: "  2026-01-01  ", want: "azure-openai"},

		// Unapproved or invalid models
		{name: "unapproved deepseek", model: "deepseek-r1", wantErr: true},
		{name: "unapproved qwen", model: "qwen-2.5-coder", wantErr: true},
		{name: "outdated claude snapshot", model: "claude-3-opus-20240229", wantErr: true},
		{name: "outdated gemini snapshot", model: "gemini-1.5-pro", wantErr: true},
		{name: "empty model", model: "", wantErr: true},
		{name: "whitespace only model", model: "   ", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertResolveProviderCase(t, tc)
		})
	}
}

// assertResolveProviderCase fails the test unless ResolveProvider(tc.model, tc.apiVersion)
// matches tc.want, or, when tc.wantErr, unless the returned error names the rejected model.
func assertResolveProviderCase(t *testing.T, tc resolveProviderCase) {
	t.Helper()

	got, err := ResolveProvider(tc.model, tc.apiVersion)
	if tc.wantErr {
		if err == nil {
			t.Fatalf("ResolveProvider(%q, %q) succeeded unexpectedly; want error", tc.model, tc.apiVersion)
		}
		if trimmed := strings.TrimSpace(tc.model); trimmed != "" && !strings.Contains(err.Error(), trimmed) {
			t.Errorf("ResolveProvider(%q, %q) error = %q, want the rejected model named", tc.model, tc.apiVersion, err.Error())
		}
		return
	}
	if err != nil {
		t.Fatalf("ResolveProvider(%q, %q) returned unexpected error: %v", tc.model, tc.apiVersion, err)
	}
	if got != tc.want {
		t.Errorf("ResolveProvider(%q, %q) = %q, want %q", tc.model, tc.apiVersion, got, tc.want)
	}
}

type normalizeModelCase struct {
	name     string
	provider string
	model    string
	want     string
	wantErr  bool
}

func TestNormalizeModel_Matrix(t *testing.T) {
	tests := []normalizeModelCase{
		// Identity mappings (canonical models)
		{name: "claude canonical", provider: "claude", model: "claude-sonnet-5", want: "claude-sonnet-5"},
		{name: "gemini canonical", provider: "gemini", model: "gemini-2.5-flash", want: "gemini-2.5-flash"},
		{name: "openai canonical", provider: "openai", model: "gpt-4o", want: "gpt-4o"},
		{name: "azure canonical", provider: "azure-openai", model: "gpt-4o", want: "gpt-4o"},
		{name: "grok canonical", provider: "grok", model: "grok-4.6", want: "grok-4.6"},
		{name: "local canonical", provider: "local", model: "llama-3.3-70b", want: "llama-3.3-70b"},

		// Alias normalization
		{name: "claude alias dot notation", provider: "claude", model: "claude-opus-4.7", want: "claude-opus-4-7"},
		{name: "claude alias short name", provider: "claude", model: "claude-sonnet-4-5", want: "claude-sonnet-4-5-20250929"},
		{name: "gemini alias preview", provider: "gemini", model: "gemini-3-flash", want: "gemini-3-flash-preview"},
		{name: "gemini alias pro", provider: "gemini", model: "gemini-3-pro", want: "gemini-3.1-pro-preview"},
		{name: "openai alias snapshot", provider: "openai", model: "gpt-4.1-2025-04-14", want: "gpt-4.1"},
		{name: "openai alias sol", provider: "openai", model: "gpt-5.6", want: "gpt-5.6-sol"},
		{name: "grok alias latest", provider: "grok", model: "grok-4.6-latest", want: "grok-4.6"},

		// Whitespace handling
		{name: "whitespace padded provider", provider: "  claude  ", model: "claude-sonnet-5", want: "claude-sonnet-5"},
		{name: "whitespace padded model", provider: "claude", model: "  claude-sonnet-5  ", want: "claude-sonnet-5"},

		// Error cases: case mismatch, unknown provider, unknown model, cross-provider mismatch
		{name: "capitalized provider rejected", provider: "Gemini", model: "gemini-3.5-flash", wantErr: true},
		{name: "uppercase provider rejected", provider: "GEMINI", model: "gemini-3.5-flash", wantErr: true},
		{name: "model with interior space", provider: "claude", model: "claude sonnet-5", wantErr: true},
		{name: "unknown provider", provider: "unknown", model: "claude-sonnet-5", wantErr: true},
		{name: "unknown model for valid provider", provider: "claude", model: "claude-unknown-9", wantErr: true},
		{name: "cross provider model mismatch", provider: "gemini", model: "claude-sonnet-5", wantErr: true},
		{name: "empty model", provider: "claude", model: "", wantErr: true},
		{name: "empty provider", provider: "", model: "claude-sonnet-5", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertNormalizeModelCase(t, tc)
		})
	}
}

// assertNormalizeModelCase fails the test unless NormalizeModel(tc.provider, tc.model)
// matches tc.want, or, when tc.wantErr, returns a non-nil error.
func assertNormalizeModelCase(t *testing.T, tc normalizeModelCase) {
	t.Helper()

	got, err := NormalizeModel(tc.provider, tc.model)
	if tc.wantErr {
		if err == nil {
			t.Fatalf("NormalizeModel(%q, %q) succeeded unexpectedly; want error", tc.provider, tc.model)
		}
		return
	}
	if err != nil {
		t.Fatalf("NormalizeModel(%q, %q) returned unexpected error: %v", tc.provider, tc.model, err)
	}
	if got != tc.want {
		t.Errorf("NormalizeModel(%q, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
	}
}

type isModelSupportedCase struct {
	name     string
	provider string
	model    string
	want     bool
}

func TestIsModelSupported_Matrix(t *testing.T) {
	tests := []isModelSupportedCase{
		{name: "canonical model is supported", provider: "claude", model: "claude-sonnet-5", want: true},
		{name: "alias model is supported", provider: "gemini", model: "gemini-3-pro", want: true},
		{name: "unknown model is not supported", provider: "claude", model: "gpt-4o", want: false},
		{name: "unknown provider is not supported", provider: "invalid", model: "claude-sonnet-5", want: false},
		{name: "empty model is not supported", provider: "claude", model: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertIsModelSupportedCase(t, tc)
		})
	}
}

// assertIsModelSupportedCase fails the test unless IsModelSupported(tc.provider, tc.model)
// equals tc.want.
func assertIsModelSupportedCase(t *testing.T, tc isModelSupportedCase) {
	t.Helper()

	if got := IsModelSupported(tc.provider, tc.model); got != tc.want {
		t.Errorf("IsModelSupported(%q, %q) = %t, want %t", tc.provider, tc.model, got, tc.want)
	}
}

type supportedModelsCase struct {
	name        string
	provider    string
	wantNil     bool
	minCount    int
	mustContain []string
}

func TestSupportedModels_Matrix(t *testing.T) {
	tests := []supportedModelsCase{
		{
			name:        "claude models",
			provider:    "claude",
			minCount:    5,
			mustContain: []string{"claude-sonnet-5", "claude-opus-5"},
		},
		{
			name:        "gemini models",
			provider:    "gemini",
			minCount:    5,
			mustContain: []string{"gemini-2.5-flash", "gemini-3.7-flash"},
		},
		{
			name:        "openai models",
			provider:    "openai",
			minCount:    10,
			mustContain: []string{"gpt-4o", "gpt-5", "o1"},
		},
		{
			name:        "azure models",
			provider:    "azure-openai",
			minCount:    10,
			mustContain: []string{"gpt-4o", "gpt-5", "o1"},
		},
		{
			name:        "grok models",
			provider:    "grok",
			minCount:    3,
			mustContain: []string{"grok-4.5", "grok-4.6"},
		},
		{
			name:        "local models",
			provider:    "local",
			minCount:    5,
			mustContain: []string{"llama-3.3-70b", "codellama-70b"},
		},
		{
			name:     "unknown provider returns nil",
			provider: "unknown",
			wantNil:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSupportedModelsCase(t, tc)
		})
	}
}

// assertSupportedModelsCase fails the test unless SupportedModels(tc.provider) is nil when
// tc.wantNil, and sorted, at least tc.minCount long, and a superset of tc.mustContain when not.
func assertSupportedModelsCase(t *testing.T, tc supportedModelsCase) {
	t.Helper()

	models := SupportedModels(tc.provider)
	if tc.wantNil {
		if models != nil {
			t.Fatalf("SupportedModels(%q) = %v, want nil", tc.provider, models)
		}
		return
	}
	if len(models) < tc.minCount {
		t.Errorf("SupportedModels(%q) returned %d models, want at least %d", tc.provider, len(models), tc.minCount)
	}
	if !sort.StringsAreSorted(models) {
		t.Errorf("SupportedModels(%q) is not sorted: %v", tc.provider, models)
	}
	assertContainsAllModels(t, tc.provider, models, tc.mustContain)
}

// assertContainsAllModels fails the test unless every entry in want is present in models.
func assertContainsAllModels(t *testing.T, provider string, models, want []string) {
	t.Helper()

	modelSet := make(map[string]struct{}, len(models))
	for _, m := range models {
		modelSet[m] = struct{}{}
	}
	for _, req := range want {
		if _, ok := modelSet[req]; !ok {
			t.Errorf("SupportedModels(%q) missing required model %q", provider, req)
		}
	}
}

func checkEachAlias(check func(provider, alias, target string) string) (failures []string) {
	for provider, aliases := range aliasRegistry {
		for alias, target := range aliases {
			if msg := check(provider, alias, target); msg != "" {
				failures = append(failures, msg)
			}
		}
	}
	return failures
}

func checkEachCanonicalModel(check func(provider, model string) string) (failures []string) {
	for provider, models := range canonicalRegistry {
		for id := range models {
			if msg := check(provider, id); msg != "" {
				failures = append(failures, msg)
			}
		}
	}
	return failures
}

// checkAliasTargetsCanonical reports a message unless target names a canonical model of p.
func checkAliasTargetsCanonical(p, a, target string) string {
	if _, ok := canonicalRegistry[p][target]; !ok {
		return fmt.Sprintf("alias %q of %q -> non-canonical %q", a, p, target)
	}
	return ""
}

// checkAliasDoesNotShadowCanonical reports a message when a itself names a canonical model of p.
func checkAliasDoesNotShadowCanonical(p, a, _ string) string {
	if _, ok := canonicalRegistry[p][a]; ok {
		return fmt.Sprintf("alias %q of %q shadows canonical", a, p)
	}
	return ""
}

// checkCanonicalIdentifierNormalized reports a message unless id is a trimmed, lowercase token.
func checkCanonicalIdentifierNormalized(p, id string) string {
	if id != strings.ToLower(strings.TrimSpace(id)) || strings.ContainsAny(id, " \t\n") {
		return fmt.Sprintf("identifier %q of %q is not normalized", id, p)
	}
	return ""
}

// checkCanonicalIdentifierSingleOwner reports a message when id belongs to more than one
// provider outside the OpenAI family, which is the sole sanctioned cross-provider overlap.
func checkCanonicalIdentifierSingleOwner(p, id string) string {
	owners := findModelOwners(id)
	if len(owners) > 1 && !isOpenAIFamily(owners) {
		return fmt.Sprintf("identifier %q of %q has ambiguous owners %v", id, p, owners)
	}
	return ""
}

// checkCanonicalModelNormalizesToSelf reports a message unless NormalizeModel(p, id) returns id
// unchanged.
func checkCanonicalModelNormalizesToSelf(p, id string) string {
	got, err := NormalizeModel(p, id)
	if err != nil || got != id {
		return fmt.Sprintf("NormalizeModel(%q, %q) = %q, %v; want self", p, id, got, err)
	}
	return ""
}

func TestRegistry_TableInvariants(t *testing.T) {
	tests := []struct {
		name  string
		check func() []string
	}{
		{
			name:  "alias targets canonical model",
			check: func() []string { return checkEachAlias(checkAliasTargetsCanonical) },
		},
		{
			name:  "alias does not shadow canonical identifier",
			check: func() []string { return checkEachAlias(checkAliasDoesNotShadowCanonical) },
		},
		{
			name:  "canonical identifiers are normalized tokens",
			check: func() []string { return checkEachCanonicalModel(checkCanonicalIdentifierNormalized) },
		},
		{
			name:  "non-OpenAI identifier has single provider owner",
			check: func() []string { return checkEachCanonicalModel(checkCanonicalIdentifierSingleOwner) },
		},
		{
			name:  "every canonical model normalizes to itself",
			check: func() []string { return checkEachCanonicalModel(checkCanonicalModelNormalizesToSelf) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if failures := tc.check(); len(failures) > 0 {
				t.Errorf("invariant %q violated:\n%s", tc.name, strings.Join(failures, "\n"))
			}
		})
	}
}
