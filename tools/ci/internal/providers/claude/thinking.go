package claude

import (
	"fmt"
	"slices"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"ci-tools/internal/reviewerconfig"
)

// MinThinkingBudget is the floor the Messages API enforces on a manual thinking budget.
const MinThinkingBudget = 1024

// thinkingPlan contains the request shape derived from the thinking fields of ModelOptions.
// An active plan suppresses sampling overrides, which the API rejects while thinking runs.
type thinkingPlan struct {
	config sdk.ThinkingConfigParamUnion
	effort sdk.OutputConfigEffort
	active bool
}

// planThinking MUST reject cross-generation thinking parameters to ensure Anthropic API compatibility.
func planThinking(model string, opts reviewerconfig.ModelOptions) (thinkingPlan, error) {
	spec := reviewerconfig.ResolveModelSpec("claude", model)
	adaptive := spec.Thinking.Mode == reviewerconfig.ThinkingAdaptive
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
		if !slices.Contains(spec.Thinking.AllowedLevels, level) {
			return thinkingPlan{}, fmt.Errorf(
				"claude: reasoning_level %q is unsupported; want low, medium, high, xhigh, or max", level)
		}
		return thinkingPlan{
			config: sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}},
			effort: sdk.OutputConfigEffort(level),
			active: true,
		}, nil
	default:
		if spec.Thinking.DefaultOmitted {
			return thinkingPlan{}, nil
		}
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
			"claude: thinking_budget %d must remain below max_tokens %d", budget, maxTokens)
	}
	enabled := sdk.ThinkingConfigEnabledParam{BudgetTokens: int64(budget)}
	return thinkingPlan{
		config: sdk.ThinkingConfigParamUnion{OfEnabled: &enabled},
		active: true,
	}, nil
}
