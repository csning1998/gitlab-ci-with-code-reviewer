package claude

import (
	"fmt"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"ci-tools/internal/config"
)

// MinThinkingBudget is the floor the Messages API enforces on a manual thinking budget.
const MinThinkingBudget = 1024

// adaptiveThinkingModels lists the canonical models which accept adaptive thinking with an
// effort level. Every other approved model takes a manual budget instead.
var adaptiveThinkingModels = map[string]bool{
	"claude-fable-5.1":  true,
	"claude-fable-5":    true,
	"claude-opus-5":     true,
	"claude-sonnet-5":   true,
	"claude-opus-4-8":   true,
	"claude-opus-4-7":   true,
	"claude-opus-4-6":   true,
	"claude-sonnet-4-6": true,
}

// adaptiveEfforts lists the effort levels the Messages API accepts on output_config.
var adaptiveEfforts = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

// thinkingPlan holds the request shape derived from the thinking fields of ModelOptions.
// An active plan suppresses sampling overrides, which the API rejects while thinking runs.
type thinkingPlan struct {
	config sdk.ThinkingConfigParamUnion
	effort sdk.OutputConfigEffort
	active bool
}

// planThinking MUST reject cross-generation thinking parameters to ensure Anthropic API compatibility.
func planThinking(model string, opts config.ModelOptions) (thinkingPlan, error) {
	adaptive := adaptiveThinkingModels[model]
	level := strings.TrimSpace(opts.ReasoningLevel)
	budget := 0
	if opts.ThinkingBudget != nil {
		budget = *opts.ThinkingBudget
	}

	if budget > 0 && adaptive {
		return thinkingPlan{}, fmt.Errorf(
			"claude: model %q uses adaptive thinking and rejects thinking_budget; set reasoning_level instead", model)
	}
	if level != "" && level != "none" && !adaptive {
		return thinkingPlan{}, fmt.Errorf(
			"claude: model %q predates adaptive thinking and rejects reasoning_level; set thinking_budget instead", model)
	}

	switch {
	case budget > 0:
		return planManualThinking(model, budget, opts.MaxTokens)
	case level != "" && level != "none":
		if !adaptiveEfforts[level] {
			return thinkingPlan{}, fmt.Errorf(
				"claude: reasoning_level %q is unsupported; want low, medium, high, xhigh, or max", level)
		}
		return thinkingPlan{
			config: sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}},
			effort: sdk.OutputConfigEffort(level),
			active: true,
		}, nil
	default:
		disabled := sdk.NewThinkingConfigDisabledParam()
		return thinkingPlan{config: sdk.ThinkingConfigParamUnion{OfDisabled: &disabled}}, nil
	}
}

// planManualThinking MUST enforce budget token limits to satisfy Messages API bounds.
func planManualThinking(model string, budget, maxTokens int) (thinkingPlan, error) {
	if budget < MinThinkingBudget {
		return thinkingPlan{}, fmt.Errorf(
			"claude: thinking_budget %d is below the %d floor for model %q", budget, MinThinkingBudget, model)
	}
	if maxTokens > 0 && budget >= maxTokens {
		return thinkingPlan{}, fmt.Errorf(
			"claude: thinking_budget %d must stay below max_tokens %d", budget, maxTokens)
	}
	enabled := sdk.ThinkingConfigEnabledParam{BudgetTokens: int64(budget)}
	return thinkingPlan{
		config: sdk.ThinkingConfigParamUnion{OfEnabled: &enabled},
		active: true,
	}, nil
}
