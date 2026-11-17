package gemini

import (
	"fmt"
	"slices"
	"strings"

	"ci-tools/internal/reviewerconfig"
)

// validateGeneration MUST reject cross-generation thinking parameters at construction to prevent upstream HTTP 400 errors.
func validateGeneration(model string, opts reviewerconfig.ModelOptions) error {
	spec := reviewerconfig.ResolveModelSpec("gemini", model)
	if opts.GoogleSearch != nil && *opts.GoogleSearch && spec.DisallowGoogleSearch {
		return fmt.Errorf("gemini: model %q does not support google_search grounding", model)
	}
	level := strings.TrimSpace(opts.ReasoningLevel)
	hasBudget := opts.ThinkingBudget != nil && *opts.ThinkingBudget > 0

	if spec.Thinking.Mode == reviewerconfig.ThinkingLevel {
		if hasBudget {
			return fmt.Errorf(
				"gemini: model %q uses thinking_level and rejects thinking_budget", model)
		}
		return validateThinkingLevel(model, level, spec.Thinking.AllowedLevels)
	}

	if level != "" && level != "none" {
		return fmt.Errorf(
			"gemini: model %q uses thinking_budget and rejects reasoning_level", model)
	}
	return nil
}

// validateThinkingLevel MUST verify level against the allowed set of the model generation.
func validateThinkingLevel(model, level string, allowed []string) error {
	if level == "" || level == "none" {
		return nil
	}
	if !slices.Contains(allowed, level) {
		return fmt.Errorf(
			"gemini: model %q accepts reasoning_level %s only, got %q", model, formatLevelList(allowed), level)
	}
	return nil
}

func formatLevelList(levels []string) string {
	switch len(levels) {
	case 0:
		return ""
	case 1:
		return levels[0]
	case 2:
		return levels[0] + " or " + levels[1]
	default:
		return strings.Join(levels[:len(levels)-1], ", ") + ", or " + levels[len(levels)-1]
	}
}

// buildGenerationConfig assembles generationConfig from opts. A field left unset remains absent,
// which preserves the default the model publishes.
func buildGenerationConfig(model string, opts reviewerconfig.ModelOptions) map[string]any {
	spec := reviewerconfig.ResolveModelSpec("gemini", model)
	generationConfig := map[string]any{"responseMimeType": "application/json"}
	if mimeType := strings.TrimSpace(opts.ResponseMIMEType); mimeType != "" {
		generationConfig["responseMimeType"] = mimeType
	}

	applySamplingConfig(generationConfig, spec.Sampling.OmitInPayload, opts)
	if opts.MaxTokens > 0 {
		generationConfig["maxOutputTokens"] = opts.MaxTokens
	}
	if len(opts.Stop) > 0 {
		generationConfig["stopSequences"] = opts.Stop
	}
	if opts.Seed != nil {
		generationConfig["seed"] = *opts.Seed
	}
	if opts.N != nil {
		generationConfig["candidateCount"] = *opts.N
	}
	if opts.FrequencyPenalty != nil {
		generationConfig["frequencyPenalty"] = *opts.FrequencyPenalty
	}
	if opts.PresencePenalty != nil {
		generationConfig["presencePenalty"] = *opts.PresencePenalty
	}
	if mediaResolution := strings.TrimSpace(opts.MediaResolution); mediaResolution != "" {
		generationConfig["mediaResolution"] = mediaResolution
	}
	if thinking := buildThinkingConfig(spec.Thinking.Mode == reviewerconfig.ThinkingLevel, opts); thinking != nil {
		generationConfig["thinkingConfig"] = thinking
	}

	return generationConfig
}

// applySamplingConfig omits sampling overrides for generations governed by the matrix policy.
func applySamplingConfig(generationConfig map[string]any, omitSampling bool, opts reviewerconfig.ModelOptions) {
	if omitSampling {
		return
	}
	if opts.Temperature != nil {
		generationConfig["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		generationConfig["topP"] = *opts.TopP
	}
	if opts.TopK != nil {
		generationConfig["topK"] = *opts.TopK
	}
}

// buildThinkingConfig emits the single thinking control the generation of model accepts.
func buildThinkingConfig(usesThinkingLevel bool, opts reviewerconfig.ModelOptions) map[string]any {
	level := strings.TrimSpace(opts.ReasoningLevel)
	if usesThinkingLevel {
		if level == "" || level == "none" {
			return nil
		}
		return map[string]any{"thinkingLevel": level}
	}

	if opts.ThinkingBudget == nil {
		return nil
	}
	thinking := map[string]any{"thinkingBudget": *opts.ThinkingBudget}
	if opts.IncludeThoughts != nil {
		thinking["includeThoughts"] = *opts.IncludeThoughts
	}
	return thinking
}
