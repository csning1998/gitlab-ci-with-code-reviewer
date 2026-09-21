package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// DefaultDeclarationPath is the project-level reviewer declaration consulted when the caller
// supplies no explicit path.
const DefaultDeclarationPath = ".gitlab/reviewer.yml"

// ResolveDeclaredOptions resolves key against a reviewer declaration. The key names a slot
// binding first and a declared model second, which lets one declaration serve any number of
// models. An absent file or an unbound key reports ErrSlotNotConfigured.
func ResolveDeclaredOptions(path, key string) (ModelOptions, error) {
	declarationPath := strings.TrimSpace(path)
	if declarationPath == "" {
		declarationPath = DefaultDeclarationPath
	}
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return ModelOptions{}, ErrSlotNotConfigured
	}

	decl, err := ParseConfigFile(declarationPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ModelOptions{}, ErrSlotNotConfigured
		}
		return ModelOptions{}, err
	}

	entry, err := lookupDeclaredEntry(decl, trimmedKey)
	if err != nil {
		return ModelOptions{}, err
	}

	opts := mergeModelOptions(decl.Defaults, entry)
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if strings.TrimSpace(opts.Provider) == "" {
		provider, err := ResolveProvider(opts.Model, opts.APIVersion)
		if err != nil {
			return ModelOptions{}, err
		}
		opts.Provider = provider
	}

	canonicalModel, err := NormalizeModel(opts.Provider, opts.Model)
	if err != nil {
		return ModelOptions{}, err
	}
	opts.Model = canonicalModel

	if err := Validate(opts); err != nil {
		return ModelOptions{}, err
	}
	return opts, nil
}

// lookupDeclaredEntry resolves key through the slot bindings before the model table. A slot name
// and a model name therefore share one lookup surface. A dangling slot binding MUST NOT return
// ErrSlotNotConfigured, which lets the caller hide the binding behind a raw model id fallback.
func lookupDeclaredEntry(decl *ConfigDeclaration, key string) (ModelOptions, error) {
	if name, bound := decl.Slots[key]; bound {
		boundName := strings.TrimSpace(name)
		entry, found := decl.Models[boundName]
		if !found {
			return ModelOptions{}, fmt.Errorf("slot %q is bound to undeclared model %q", key, boundName)
		}
		return entry, nil
	}
	entry, found := decl.Models[key]
	if !found {
		return ModelOptions{}, ErrSlotNotConfigured
	}
	return entry, nil
}

// mergeModelOptions overlays entry onto base. A field left at its zero value in entry inherits
// the value declared under defaults.
func mergeModelOptions(base, entry ModelOptions) ModelOptions {
	merged := base

	mergeStringField(&merged.Provider, entry.Provider)
	mergeStringField(&merged.Model, entry.Model)
	mergeStringField(&merged.ReasoningLevel, entry.ReasoningLevel)
	mergeStringField(&merged.ThinkingType, entry.ThinkingType)
	mergeStringField(&merged.ResponseMIMEType, entry.ResponseMIMEType)
	mergeStringField(&merged.Verbosity, entry.Verbosity)
	mergeStringField(&merged.BaseURL, entry.BaseURL)
	mergeStringField(&merged.APIVersion, entry.APIVersion)
	mergeStringField(&merged.ServiceTier, entry.ServiceTier)
	mergeStringField(&merged.PromptCacheKey, entry.PromptCacheKey)
	mergeStringField(&merged.SafetyIdentifier, entry.SafetyIdentifier)
	mergeStringField(&merged.MediaResolution, entry.MediaResolution)
	mergeStringField(&merged.Prompt, entry.Prompt)
	mergeStringField(&merged.PromptFile, entry.PromptFile)

	if entry.Timeout != 0 {
		merged.Timeout = entry.Timeout
	}
	if entry.MaxTokens != 0 {
		merged.MaxTokens = entry.MaxTokens
	}
	if entry.Temperature != nil {
		merged.Temperature = entry.Temperature
	}
	if entry.TopP != nil {
		merged.TopP = entry.TopP
	}
	if entry.TopK != nil {
		merged.TopK = entry.TopK
	}
	if entry.FrequencyPenalty != nil {
		merged.FrequencyPenalty = entry.FrequencyPenalty
	}
	if entry.PresencePenalty != nil {
		merged.PresencePenalty = entry.PresencePenalty
	}
	if entry.RepetitionPenalty != nil {
		merged.RepetitionPenalty = entry.RepetitionPenalty
	}
	if entry.MinP != nil {
		merged.MinP = entry.MinP
	}
	if entry.Seed != nil {
		merged.Seed = entry.Seed
	}
	if entry.N != nil {
		merged.N = entry.N
	}
	if entry.ThinkingBudget != nil {
		merged.ThinkingBudget = entry.ThinkingBudget
	}
	if entry.IncludeThoughts != nil {
		merged.IncludeThoughts = entry.IncludeThoughts
	}
	if entry.Logprobs != nil {
		merged.Logprobs = entry.Logprobs
	}
	if entry.TopLogprobs != nil {
		merged.TopLogprobs = entry.TopLogprobs
	}
	if len(entry.Stop) > 0 {
		merged.Stop = entry.Stop
	}
	if len(entry.LogitBias) > 0 {
		merged.LogitBias = entry.LogitBias
	}
	if len(entry.ResponseSchema) > 0 {
		merged.ResponseSchema = entry.ResponseSchema
	}
	if len(entry.SafetySettings) > 0 {
		merged.SafetySettings = entry.SafetySettings
	}

	return merged
}

// mergeStringField overwrites target when the entry declares a non-empty value.
func mergeStringField(target *string, value string) {
	if strings.TrimSpace(value) != "" {
		*target = value
	}
}
