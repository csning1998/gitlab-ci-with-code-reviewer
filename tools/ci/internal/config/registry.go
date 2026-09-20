package config

import (
	"fmt"
	"sort"
	"strings"
)

var openAICanonicalModels = map[string]struct{}{
	"gpt-4o":                 {},
	"gpt-4o-mini":            {},
	"gpt-4-turbo":            {},
	"gpt-4.1":                {},
	"gpt-4.1-mini":           {},
	"gpt-4.1-nano":           {},
	"gpt-5":                  {},
	"gpt-5-mini":             {},
	"gpt-5-nano":             {},
	"gpt-5.4":                {},
	"gpt-5.4-mini":           {},
	"gpt-5.4-nano":           {},
	"gpt-5.4-pro":            {},
	"gpt-5.6-sol":            {},
	"gpt-5.6-terra":          {},
	"gpt-5.6-luna":           {},
	"gpt-6-astra":            {},
	"o1":                     {},
	"o1-mini":                {},
	"o1-preview":             {},
	"o3":                     {},
	"o3-mini":                {},
	"o3-pro":                 {},
	"o4-mini":                {},
	"gpt-4o-2024-11-20":      {},
	"gpt-4o-2024-08-06":      {},
	"gpt-4o-2024-05-13":      {},
	"gpt-4-turbo-2024-04-09": {},
	"gpt-5-2025-08-07":       {},
	"gpt-5-mini-2025-08-07":  {},
	"gpt-5-nano-2025-08-07":  {},
	"o1-2024-12-17":          {},
	"o1-mini-2024-09-12":     {},
	"o1-preview-2024-09-12":  {},
	"o3-2025-04-16":          {},
	"o3-mini-2025-01-31":     {},
}

// canonicalRegistry maps provider names to the set of supported canonical model identifiers.
var canonicalRegistry = map[string]map[string]struct{}{
	"claude": {
		"claude-fable-5.1":           {},
		"claude-fable-5":             {},
		"claude-opus-5":              {},
		"claude-sonnet-5":            {},
		"claude-opus-4-8":            {},
		"claude-opus-4-7":            {},
		"claude-opus-4-6":            {},
		"claude-sonnet-4-6":          {},
		"claude-sonnet-4-5-20250929": {},
		"claude-opus-4-5-20251101":   {},
		"claude-haiku-4-5-20251001":  {},
	},
	"gemini": {
		"gemini-3.8-flash":       {},
		"gemini-3.7-flash":       {},
		"gemini-3.6-flash":       {},
		"gemini-3.5-flash":       {},
		"gemini-3.5-flash-lite":  {},
		"gemini-3.1-flash-lite":  {},
		"gemini-3.1-pro-preview": {},
		"gemini-3-flash-preview": {},
		"gemini-2.5-pro":         {},
		"gemini-2.5-flash":       {},
		"gemini-2.5-flash-lite":  {},
		"gemma-4-31b-it":         {},
		"gemma-4-26b-a4b-it":     {},
	},
	"openai":       openAICanonicalModels,
	"azure-openai": openAICanonicalModels,
	"grok": {
		"grok-4.6":              {},
		"grok-4.5":              {},
		"grok-4.20-multi-agent": {},
	},
	"local": {
		"gemma-2-9b":      {},
		"gemma-2-27b":     {},
		"gemma-4-31b":     {},
		"gemma-4-26b-a4b": {},
		"llama-3.3-70b":   {},
		"llama-3.1-8b":    {},
		"codellama-70b":   {},
		"mistral-large":   {},
	},
}

// aliasRegistry maps known provider-specific model aliases and snapshot formats to their canonical identifiers.
var aliasRegistry = map[string]map[string]string{
	"claude": {
		"claude-opus-4.7":   "claude-opus-4-7",
		"claude-sonnet-4-5": "claude-sonnet-4-5-20250929",
		"claude-opus-4-5":   "claude-opus-4-5-20251101",
		"claude-haiku-4-5":  "claude-haiku-4-5-20251001",
	},
	"gemini": {
		"gemini-3-flash": "gemini-3-flash-preview",
		"gemini-3-pro":   "gemini-3.1-pro-preview",
	},
	"openai": {
		"gpt-5.6":                 "gpt-5.6-sol",
		"gpt-4.1-2025-04-14":      "gpt-4.1",
		"gpt-5.4-mini-2026-03-17": "gpt-5.4-mini",
		"gpt-5.4-nano-2026-03-17": "gpt-5.4-nano",
		"o4-mini-2025-04-16":      "o4-mini",
	},
	"azure-openai": {
		"gpt-5.6":                 "gpt-5.6-sol",
		"gpt-4.1-2025-04-14":      "gpt-4.1",
		"gpt-5.4-mini-2026-03-17": "gpt-5.4-mini",
		"gpt-5.4-nano-2026-03-17": "gpt-5.4-nano",
		"o4-mini-2025-04-16":      "o4-mini",
	},
	"grok": {
		"grok-4.6-latest":            "grok-4.6",
		"grok-4.20-multi-agent-0309": "grok-4.20-multi-agent",
	},
}

// NormalizeModel validates and maps a model string to its canonical identifier.
// It returns an error if the model is not present in the compile-time allowlist.
func NormalizeModel(provider, model string) (string, error) {
	trimmedProvider := strings.TrimSpace(provider)
	trimmedModel := strings.TrimSpace(model)

	if trimmedModel == "" {
		return "", fmt.Errorf("mandatory field 'model' is required for provider %q", trimmedProvider)
	}

	canonicalSet, providerFound := canonicalRegistry[trimmedProvider]
	if !providerFound {
		return "", fmt.Errorf("unsupported or unknown provider %q", trimmedProvider)
	}

	if _, ok := canonicalSet[trimmedModel]; ok {
		return trimmedModel, nil
	}

	if aliases, ok := aliasRegistry[trimmedProvider]; ok {
		if canonical, found := aliases[trimmedModel]; found {
			return canonical, nil
		}
	}

	return "", fmt.Errorf("unsupported or unapproved model %q for provider %q", trimmedModel, trimmedProvider)
}

// IsModelSupported checks whether a model is approved (either canonical or a valid alias) for a provider.
func IsModelSupported(provider, model string) bool {
	_, err := NormalizeModel(provider, model)
	return err == nil
}

// SupportedModels returns a sorted list of canonical models approved for a provider.
func SupportedModels(provider string) []string {
	canonicalSet, ok := canonicalRegistry[strings.TrimSpace(provider)]
	if !ok {
		return nil
	}

	models := make([]string, 0, len(canonicalSet))
	for m := range canonicalSet {
		models = append(models, m)
	}
	sort.Strings(models)
	return models
}
