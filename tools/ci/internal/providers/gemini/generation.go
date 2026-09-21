package gemini

import (
	"fmt"
	"strings"

	"ci-tools/internal/config"
)

// gemmaThinkingLevels lists the two switch positions Gemma 4 accepts on the Gemini API.
var gemmaThinkingLevels = map[string]bool{"high": true, "minimal": true}

// thinkingLevels lists the levels the 3.x generation accepts.
var thinkingLevels = map[string]bool{"minimal": true, "low": true, "medium": true, "high": true}

// checkThinkingLevelSupport reports whether model accepts thinkingLevel over thinkingBudget.
func checkThinkingLevelSupport(model string) bool {
	return strings.HasPrefix(model, "gemini-3") || strings.HasPrefix(model, "gemma-")
}

// matchGemmaModel reports whether model represents open-weights Gemma hosted on Gemini API.
func matchGemmaModel(model string) bool {
	return strings.HasPrefix(model, "gemma-")
}

// validateGeneration MUST reject cross-generation thinking parameters at construction to prevent upstream HTTP 400 errors.
func validateGeneration(model string, opts config.ModelOptions) error {
	level := strings.TrimSpace(opts.ReasoningLevel)
	hasBudget := opts.ThinkingBudget != nil && *opts.ThinkingBudget > 0

	if checkThinkingLevelSupport(model) {
		if hasBudget {
			return fmt.Errorf(
				"gemini: model %q carries thinking_level and rejects thinking_budget", model)
		}
		return validateThinkingLevel(model, level)
	}

	if level != "" && level != "none" {
		return fmt.Errorf(
			"gemini: model %q carries thinking_budget and rejects reasoning_level", model)
	}
	return nil
}

// validateThinkingLevel MUST verify level against the allowed set of the model generation.
func validateThinkingLevel(model, level string) error {
	if level == "" || level == "none" {
		return nil
	}
	if matchGemmaModel(model) {
		if !gemmaThinkingLevels[level] {
			return fmt.Errorf(
				"gemini: model %q accepts reasoning_level high or minimal only, got %q", model, level)
		}
		return nil
	}
	if !thinkingLevels[level] {
		return fmt.Errorf(
			"gemini: reasoning_level %q is unsupported; want minimal, low, medium, or high", level)
	}
	return nil
}

// buildGenerationConfig assembles generationConfig from opts. A field left unset stays absent,
// which preserves the default the model publishes.
func buildGenerationConfig(model string, opts config.ModelOptions) map[string]any {
	generationConfig := map[string]any{"responseMimeType": "application/json"}
	if mimeType := strings.TrimSpace(opts.ResponseMIMEType); mimeType != "" {
		generationConfig["responseMimeType"] = mimeType
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
	if thinking := buildThinkingConfig(model, opts); thinking != nil {
		generationConfig["thinkingConfig"] = thinking
	}

	return generationConfig
}

// buildThinkingConfig emits the single thinking control the generation of model accepts.
func buildThinkingConfig(model string, opts config.ModelOptions) map[string]any {
	level := strings.TrimSpace(opts.ReasoningLevel)
	if checkThinkingLevelSupport(model) {
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
