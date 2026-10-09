package reviewerconfig

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Provider identifiers of the two platforms which publish an identical model catalogue.
// A reverse lookup of a shared identifier requires the api-version to pick the owner.
const (
	providerOpenAI      = "openai"
	providerAzureOpenAI = "azure-openai"
)

var openAICanonicalModels = map[string]struct{}{
	"gpt-4o":                 {},
	"gpt-4o-mini":            {}, // Deprecated with retirement on April 14, 2027 on Azure.
	"gpt-4-turbo":            {},
	"gpt-4.1":                {}, // Deprecated with retirement on April 14, 2027 on Azure.
	"gpt-4.1-mini":           {}, // Deprecated with retirement on April 14, 2027 on Azure.
	"gpt-4.1-nano":           {}, // Deprecated with retirement on April 14, 2027 on Azure.
	"gpt-5":                  {},
	"gpt-5.1":                {}, // Deprecated with OpenAI shutdown on April 1, 2027 and Azure retirement on May 15, 2027.
	"gpt-5-mini":             {},
	"gpt-5-nano":             {},
	"gpt-5.2":                {}, // Retires on June 8, 2027 on Azure.
	"gpt-5.3-codex":          {}, // Deprecated with OpenAI shutdown on April 1, 2027 and Azure retirement on August 24, 2027.
	"gpt-5.4":                {}, // Retires on September 2, 2027 on Azure.
	"gpt-5.4-mini":           {}, // Retires on September 21, 2027 on Azure.
	"gpt-5.4-nano":           {}, // Deprecated with OpenAI shutdown on April 1, 2027 and Azure retirement on September 21, 2027.
	"gpt-5.4-pro":            {}, // Retires on September 7, 2027 on Azure.
	"gpt-5.5":                {}, // Retires on October 26, 2027 on Azure.
	"gpt-5.6-sol":            {}, // Retires on January 11, 2028 on Azure.
	"gpt-5.6-terra":          {}, // Retires on January 11, 2028 on Azure.
	"gpt-5.6-luna":           {}, // Retires on January 11, 2028 on Azure.
	"gpt-6-astra":            {},
	"o1":                     {},
	"o1-mini":                {},
	"o1-preview":             {},
	"o3":                     {},
	"o3-mini":                {},
	"o3-pro":                 {},
	"o4-mini":                {}, // Deprecated with retirement on November 19, 2026 on Azure.
	"gpt-4o-2024-11-20":      {}, // Legacy with retirement on April 14, 2027 on Azure.
	"gpt-4o-2024-08-06":      {}, // Deprecated with retirement on April 14, 2027 on Azure.
	"gpt-4o-2024-05-13":      {}, // Deprecated with retirement on December 9, 2026 on Azure.
	"gpt-4-turbo-2024-04-09": {},
	"gpt-5-2025-08-07":       {}, // Deprecated with OpenAI shutdown on December 11, 2026 and Azure retirement on February 9, 2027.
	"gpt-5-mini-2025-08-07":  {}, // Deprecated with OpenAI shutdown on December 11, 2026 and Azure retirement on February 9, 2027.
	"gpt-5-nano-2025-08-07":  {}, // Deprecated with OpenAI shutdown on December 11, 2026 and Azure retirement on February 9, 2027.
	"gpt-5-pro-2025-10-06":   {}, // Deprecated with OpenAI shutdown on December 11, 2026 and Azure retirement on April 7, 2027.
	"o1-2024-12-17":          {}, // Deprecated with retirement on November 19, 2026 on Azure.
	"o1-mini-2024-09-12":     {},
	"o1-preview-2024-09-12":  {},
	"o3-2025-04-16":          {}, // Deprecated with Azure retirement on November 19, 2026 and OpenAI shutdown on December 11, 2026.
	"o3-pro-2025-06-10":      {}, // Deprecated with Azure retirement on November 19, 2026 and OpenAI shutdown on December 11, 2026.
	"o3-mini-2025-01-31":     {}, // Deprecated with retirement on November 19, 2026 on Azure.
}

// canonicalRegistry maps provider names to the set of supported canonical model identifiers.
var canonicalRegistry = map[string]map[string]struct{}{
	"claude": {
		"claude-fable-5.1":           {}, // Retires not sooner than September 1, 2027.
		"claude-fable-5":             {}, // Retires not sooner than June 9, 2027.
		"claude-opus-5-5":            {}, // Retires not sooner than September 22, 2027.
		"claude-sonnet-5-5":          {}, // Retires not sooner than September 28, 2027.
		"claude-opus-5":              {}, // Retires not sooner than July 24, 2027.
		"claude-sonnet-5":            {}, // Retires not sooner than June 30, 2027.
		"claude-opus-4-8":            {}, // Retires not sooner than May 28, 2027.
		"claude-opus-4-7":            {}, // Retires not sooner than April 16, 2027.
		"claude-opus-4-6":            {}, // Retires not sooner than February 5, 2027.
		"claude-sonnet-4-6":          {}, // Retires not sooner than February 17, 2027.
		"claude-sonnet-4-5-20250929": {}, // Deprecated with retirement on November 30, 2026.
		"claude-opus-4-5-20251101":   {}, // Retires not sooner than November 24, 2026.
		"claude-haiku-4-5-20251001":  {}, // Retires not sooner than October 15, 2026.
	},
	"gemini": {
		"gemini-3.8-flash":       {}, // Active with no announced retirement date.
		"gemini-3.7-flash":       {}, // Retires on January 28, 2027.
		"gemini-3.6-flash":       {}, // Retires on November 19, 2026.
		"gemini-3.5-flash":       {}, // Retires on May 19, 2027 or later.
		"gemini-3.5-flash-lite":  {}, // Retires on July 21, 2027 or later.
		"gemini-3.1-flash-lite":  {}, // Retires on May 7, 2027 or later.
		"gemini-3.1-pro-preview": {},
		"gemini-3-flash-preview": {},
		"gemini-2.5-pro":         {}, // Retires on October 20, 2026.
		"gemini-2.5-flash":       {}, // Retires on October 20, 2026.
		"gemini-2.5-flash-lite":  {}, // Retires on October 20, 2026.
		"gemma-4-31b-it":         {},
		"gemma-4-26b-a4b-it":     {},
	},
	"openai":       openAICanonicalModels,
	"azure-openai": openAICanonicalModels,
	"grok": {
		"grok-4.7":              {}, // Active with no announced retirement date.
		"grok-4.6":              {}, // Active with no announced retirement date.
		"grok-4.5":              {}, // Active with no announced retirement date.
		"grok-4.3":              {}, // Active with no announced retirement date.
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
		"claude-opus-5.5":   "claude-opus-5-5",
		"claude-sonnet-5.5": "claude-sonnet-5-5",
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
		"grok-4.7-latest":            "grok-4.7",
		"grok-4-7":                   "grok-4.7",
		"grok-4.6-latest":            "grok-4.6",
		"grok-4.20-multi-agent-0309": "grok-4.20-multi-agent",
	},
}

// NormalizeModel validates and maps a model string to its canonical identifier.
// A model absent from the compile-time allowlist produces an error.
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

// ResolveProvider identifies which provider publishes model, reading the same compile-time
// allowlist as NormalizeModel. An api-version routes a shared OpenAI identifier to Azure.
func ResolveProvider(model, apiVersion string) (string, error) {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		return "", errors.New("mandatory field 'model' is required")
	}

	owners := findModelOwners(trimmedModel)
	switch {
	case len(owners) == 0:
		return "", fmt.Errorf("unsupported or unapproved model %q", trimmedModel)
	case len(owners) == 1:
		return owners[0], nil
	case isOpenAIFamily(owners):
		if strings.TrimSpace(apiVersion) != "" {
			return providerAzureOpenAI, nil
		}
		return providerOpenAI, nil
	}
	return "", fmt.Errorf("ambiguous model %q across providers %s", trimmedModel, strings.Join(owners, ", "))
}

// findModelOwners returns every provider which publishes model as a canonical identifier or
// as an alias, sorted for deterministic reporting.
func findModelOwners(model string) []string {
	seen := map[string]struct{}{}
	for provider, canonicalSet := range canonicalRegistry {
		if _, ok := canonicalSet[model]; ok {
			seen[provider] = struct{}{}
		}
	}
	for provider, aliases := range aliasRegistry {
		if _, ok := aliases[model]; ok {
			seen[provider] = struct{}{}
		}
	}

	owners := make([]string, 0, len(seen))
	for provider := range seen {
		owners = append(owners, provider)
	}
	sort.Strings(owners)
	return owners
}

// isOpenAIFamily reports whether owners is exactly the OpenAI and Azure OpenAI pair.
func isOpenAIFamily(owners []string) bool {
	return len(owners) == 2 && owners[0] == providerAzureOpenAI && owners[1] == providerOpenAI
}
