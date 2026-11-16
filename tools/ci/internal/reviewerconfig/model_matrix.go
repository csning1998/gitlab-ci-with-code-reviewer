package reviewerconfig

// ProviderPolicy defines parameter validation and wire capability bounds per provider.
type ProviderPolicy struct {
	AllowCustomModels        bool
	MaxTemperature           float64
	SupportsTopK             bool
	SupportsThinkingBudget   bool
	SupportsFrequencyPenalty bool
	SupportsPresencePenalty  bool
	SupportsRepetition       bool
	SupportsMinP             bool
	SupportsGoogleSearch     bool
	SupportsWebSearch        bool
}

// ThinkingMode classifies how a model configures extended reasoning.
type ThinkingMode uint8

const (
	ThinkingManualBudget ThinkingMode = iota
	ThinkingLevel
	ThinkingAdaptive
)

// ThinkingSpec describes a model's reasoning parameter contract.
type ThinkingSpec struct {
	Mode           ThinkingMode
	DefaultOmitted bool
	AllowedLevels  []string
}

// SamplingSpec describes sampling and token-limit constraints for a model.
type SamplingSpec struct {
	RejectAtConstruction       bool
	OmitInPayload              bool
	SuppressWhenThinkingActive bool
	OmitPenaltiesWhenReasoning bool
	UseMaxCompletionTokens     bool
}

// ModelSpec declaratively describes one approved model, its aliases, and runtime behavior.
type ModelSpec struct {
	Provider             string
	CanonicalID          string
	Aliases              []string
	Thinking             ThinkingSpec
	Sampling             SamplingSpec
	DisallowGoogleSearch bool
}

// Provider identifiers of the two platforms which publish an identical model catalogue.
// A reverse lookup of a shared identifier requires the api-version to pick the owner.
const (
	providerOpenAI      = "openai"
	providerAzureOpenAI = "azure-openai"
)

// Approved Claude model identifiers.
const (
	ModelClaude_Fable_5_2          = "claude-fable-5-2"
	ModelClaude_Fable_5_1          = "claude-fable-5-1"
	ModelClaude_Fable_5            = "claude-fable-5"
	ModelClaude_Haiku_5_5          = "claude-haiku-5-5"
	ModelClaude_Opus_5_5           = "claude-opus-5-5"
	ModelClaude_Sonnet_5_5         = "claude-sonnet-5-5"
	ModelClaude_Opus_5             = "claude-opus-5"
	ModelClaude_Sonnet_5           = "claude-sonnet-5"
	ModelClaude_Opus_4_8           = "claude-opus-4-8"
	ModelClaude_Opus_4_7           = "claude-opus-4-7"
	ModelClaude_Opus_4_6           = "claude-opus-4-6"
	ModelClaude_Sonnet_4_6         = "claude-sonnet-4-6"
	ModelClaude_Opus_4_5_20251101  = "claude-opus-4-5-20251101"
	ModelClaude_Haiku_4_5_20251001 = "claude-haiku-4-5-20251001"
)

// Approved Gemini and Gemma model identifiers served via the Gemini API.
const (
	ModelGemini_4_Hydrogen      = "gemini-4-hydrogen"
	ModelGemini_4_Carbon        = "gemini-4-carbon"
	ModelGemini_4_Argon         = "gemini-4-argon"
	ModelGemini_3_8_Flash       = "gemini-3.8-flash"
	ModelGemini_3_6_Flash       = "gemini-3.6-flash"
	ModelGemini_3_5_Flash_Lite  = "gemini-3.5-flash-lite"
	ModelGemini_3_1_Pro_Preview = "gemini-3.1-pro-preview"
	ModelGemini_3_Flash_Preview = "gemini-3-flash-preview"
	ModelGemma_4_31B_IT         = "gemma-4-31b-it"
	ModelGemma_4_26B_A4B_IT     = "gemma-4-26b-a4b-it"
)

// Approved OpenAI and Azure OpenAI GPT-series model identifiers.
const (
	ModelGPT_4o                 = "gpt-4o"
	ModelGPT_4_Turbo            = "gpt-4-turbo"
	ModelGPT_4_Turbo_2024_04_09 = "gpt-4-turbo-2024-04-09"
	ModelGPT_5                  = "gpt-5"
	ModelGPT_5_Mini             = "gpt-5-mini"
	ModelGPT_5_Nano             = "gpt-5-nano"
	ModelGPT_5_2                = "gpt-5.2"
	ModelGPT_5_4                = "gpt-5.4"
	ModelGPT_5_4_Mini           = "gpt-5.4-mini"
	ModelGPT_5_4_Pro            = "gpt-5.4-pro"
	ModelGPT_5_5                = "gpt-5.5"
	ModelGPT_5_6_Sol            = "gpt-5.6-sol"
	ModelGPT_5_6_Terra          = "gpt-5.6-terra"
	ModelGPT_5_6_Luna           = "gpt-5.6-luna"
	ModelGPT_6_Astra            = "gpt-6-astra"
)

// Approved OpenAI and Azure OpenAI o-series reasoning model identifiers.
const (
	ModelO1                    = "o1"
	ModelO1_Mini               = "o1-mini"
	ModelO1_Mini_2024_09_12    = "o1-mini-2024-09-12"
	ModelO1_Preview            = "o1-preview"
	ModelO1_Preview_2024_09_12 = "o1-preview-2024-09-12"
	ModelO3                    = "o3"
	ModelO3_Mini               = "o3-mini"
	ModelO3_Pro                = "o3-pro"
)

// Approved xAI Grok model identifiers.
const (
	ModelGrok_4_7              = "grok-4.7"
	ModelGrok_4_6              = "grok-4.6"
	ModelGrok_4_5              = "grok-4.5"
	ModelGrok_4_3              = "grok-4.3"
	ModelGrok_4_20_Multi_Agent = "grok-4.20-multi-agent"
)

// Approved local inference model identifiers.
const (
	ModelGemma_2_9B      = "gemma-2-9b"
	ModelGemma_2_27B     = "gemma-2-27b"
	ModelGemma_4_31B     = "gemma-4-31b"
	ModelGemma_4_26B_A4B = "gemma-4-26b-a4b"
	ModelLlama_3_3_70B   = "llama-3.3-70b"
	ModelLlama_3_1_8B    = "llama-3.1-8b"
	ModelCodeLlama_70B   = "codellama-70b"
	ModelMistral_Large   = "mistral-large"
)

var providerPolicies = map[string]ProviderPolicy{
	"claude": {
		MaxTemperature:         1.0,
		SupportsTopK:           true,
		SupportsThinkingBudget: true,
		SupportsWebSearch:      true,
	},
	"gemini": {
		MaxTemperature:           2.0,
		SupportsTopK:             true,
		SupportsThinkingBudget:   true,
		SupportsFrequencyPenalty: true,
		SupportsPresencePenalty:  true,
		SupportsGoogleSearch:     true,
	},
	providerOpenAI: {
		MaxTemperature:           2.0,
		SupportsFrequencyPenalty: true,
		SupportsPresencePenalty:  true,
	},
	providerAzureOpenAI: {
		MaxTemperature:           2.0,
		SupportsFrequencyPenalty: true,
		SupportsPresencePenalty:  true,
	},
	"grok": {
		MaxTemperature:           2.0,
		SupportsFrequencyPenalty: true,
		SupportsPresencePenalty:  true,
	},
	"local": {
		AllowCustomModels:        true,
		MaxTemperature:           2.0,
		SupportsTopK:             true,
		SupportsThinkingBudget:   true,
		SupportsFrequencyPenalty: true,
		SupportsPresencePenalty:  true,
		SupportsRepetition:       true,
		SupportsMinP:             true,
	},
}

var (
	claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

	claude5Thinking = ThinkingSpec{
		Mode:           ThinkingAdaptive,
		DefaultOmitted: true,
		AllowedLevels:  claudeEfforts,
	}
	claude4AdaptiveThinking = ThinkingSpec{
		Mode:          ThinkingAdaptive,
		AllowedLevels: claudeEfforts,
	}
	claudeManualThinking = ThinkingSpec{Mode: ThinkingManualBudget}

	gemini4StandardThinking = ThinkingSpec{
		Mode:          ThinkingLevel,
		AllowedLevels: []string{"low", "medium", "high"},
	}
	gemini4HydrogenThinking = ThinkingSpec{
		Mode:          ThinkingLevel,
		AllowedLevels: []string{"low", "medium", "high", "xhigh"},
	}
	gemini3Thinking = ThinkingSpec{
		Mode:          ThinkingLevel,
		AllowedLevels: []string{"minimal", "low", "medium", "high"},
	}
	gemma4Thinking = ThinkingSpec{
		Mode:          ThinkingLevel,
		AllowedLevels: []string{"high", "minimal"},
	}

	openAIMaxCompletionSampling = SamplingSpec{UseMaxCompletionTokens: true}
	openAIReasoningSampling     = SamplingSpec{
		OmitPenaltiesWhenReasoning: true,
		UseMaxCompletionTokens:     true,
	}
	grokReasoningSampling = SamplingSpec{OmitPenaltiesWhenReasoning: true}
)

var sharedOpenAISpecs = []ModelSpec{
	{CanonicalID: ModelGPT_4o},
	{CanonicalID: ModelGPT_4_Turbo},
	{CanonicalID: ModelGPT_5, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_Mini, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_Nano, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_2, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_4, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_4_Mini, Aliases: []string{"gpt-5.4-mini-2026-03-17"}, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_4_Pro, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_5, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_6_Sol, Aliases: []string{"gpt-5.6"}, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_6_Terra, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_5_6_Luna, Sampling: openAIMaxCompletionSampling},
	{CanonicalID: ModelGPT_6_Astra},
	{CanonicalID: ModelO1, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelO1_Mini, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelO1_Preview, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelO3, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelO3_Mini, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelO3_Pro, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelGPT_4_Turbo_2024_04_09},
	{CanonicalID: ModelO1_Mini_2024_09_12, Sampling: openAIReasoningSampling},
	{CanonicalID: ModelO1_Preview_2024_09_12, Sampling: openAIReasoningSampling},
}

var modelCatalog = buildModelCatalog()

func buildModelCatalog() []ModelSpec {
	specs := []ModelSpec{
		// Claude
		{Provider: "claude", CanonicalID: ModelClaude_Fable_5_2, Aliases: []string{"claude-fable-5.2"}, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Fable_5_1, Aliases: []string{"claude-fable-5.1"}, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Fable_5, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Haiku_5_5, Aliases: []string{"claude-haiku-5.5"}, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Opus_5_5, Aliases: []string{"claude-opus-5.5"}, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Sonnet_5_5, Aliases: []string{"claude-sonnet-5.5"}, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Opus_5, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Sonnet_5, Thinking: claude5Thinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Opus_4_8, Thinking: claude4AdaptiveThinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Opus_4_7, Aliases: []string{"claude-opus-4.7"}, Thinking: claude4AdaptiveThinking, Sampling: SamplingSpec{RejectAtConstruction: true}},
		{Provider: "claude", CanonicalID: ModelClaude_Opus_4_6, Thinking: claude4AdaptiveThinking},
		{Provider: "claude", CanonicalID: ModelClaude_Sonnet_4_6, Thinking: claude4AdaptiveThinking},
		{Provider: "claude", CanonicalID: ModelClaude_Opus_4_5_20251101, Aliases: []string{"claude-opus-4-5"}, Thinking: claudeManualThinking},
		{Provider: "claude", CanonicalID: ModelClaude_Haiku_4_5_20251001, Aliases: []string{"claude-haiku-4-5"}, Thinking: claudeManualThinking},

		// Gemini & Gemma
		{Provider: "gemini", CanonicalID: ModelGemini_4_Hydrogen, Thinking: gemini4HydrogenThinking, Sampling: SamplingSpec{OmitInPayload: true}},
		{Provider: "gemini", CanonicalID: ModelGemini_4_Carbon, Thinking: gemini4StandardThinking, Sampling: SamplingSpec{OmitInPayload: true}},
		{Provider: "gemini", CanonicalID: ModelGemini_4_Argon, Thinking: gemini4StandardThinking, Sampling: SamplingSpec{OmitInPayload: true}},
		{Provider: "gemini", CanonicalID: ModelGemini_3_8_Flash, Thinking: gemini3Thinking},
		{Provider: "gemini", CanonicalID: ModelGemini_3_6_Flash, Thinking: gemini3Thinking},
		{Provider: "gemini", CanonicalID: ModelGemini_3_5_Flash_Lite, Thinking: gemini3Thinking},
		{Provider: "gemini", CanonicalID: ModelGemini_3_1_Pro_Preview, Aliases: []string{"gemini-3-pro"}, Thinking: gemini3Thinking},
		{Provider: "gemini", CanonicalID: ModelGemini_3_Flash_Preview, Aliases: []string{"gemini-3-flash"}, Thinking: gemini3Thinking},
		{Provider: "gemini", CanonicalID: ModelGemma_4_31B_IT, Thinking: gemma4Thinking, DisallowGoogleSearch: true},
		{Provider: "gemini", CanonicalID: ModelGemma_4_26B_A4B_IT, Thinking: gemma4Thinking, DisallowGoogleSearch: true},

		// Grok
		{Provider: "grok", CanonicalID: ModelGrok_4_7, Aliases: []string{"grok-4.7-latest", "grok-4-7"}},
		{Provider: "grok", CanonicalID: ModelGrok_4_6, Aliases: []string{"grok-4.6-latest"}, Sampling: grokReasoningSampling},
		{Provider: "grok", CanonicalID: ModelGrok_4_5, Sampling: grokReasoningSampling},
		{Provider: "grok", CanonicalID: ModelGrok_4_3},
		{Provider: "grok", CanonicalID: ModelGrok_4_20_Multi_Agent, Aliases: []string{"grok-4.20-multi-agent-0309"}},

		// Local
		{Provider: "local", CanonicalID: ModelGemma_2_9B},
		{Provider: "local", CanonicalID: ModelGemma_2_27B},
		{Provider: "local", CanonicalID: ModelGemma_4_31B},
		{Provider: "local", CanonicalID: ModelGemma_4_26B_A4B},
		{Provider: "local", CanonicalID: ModelLlama_3_3_70B},
		{Provider: "local", CanonicalID: ModelLlama_3_1_8B},
		{Provider: "local", CanonicalID: ModelCodeLlama_70B},
		{Provider: "local", CanonicalID: ModelMistral_Large},
	}

	for _, provider := range []string{providerOpenAI, providerAzureOpenAI} {
		for _, spec := range sharedOpenAISpecs {
			spec.Provider = provider
			specs = append(specs, spec)
		}
	}
	return specs
}

type prefixFallbackSpec struct {
	provider string
	prefix   string
	spec     ModelSpec
}

// prefixFallbacks resolves uncataloged direct-client models (for example retired test fixtures
// or custom local endpoints) to their generation defaults without widening the allowlist.
var prefixFallbacks = []prefixFallbackSpec{
	{provider: "gemini", prefix: "gemini-4", spec: ModelSpec{Thinking: gemini4StandardThinking, Sampling: SamplingSpec{OmitInPayload: true}}},
	{provider: "gemini", prefix: "gemini-3", spec: ModelSpec{Thinking: gemini3Thinking}},
	{provider: "gemini", prefix: "gemma-", spec: ModelSpec{Thinking: gemma4Thinking, DisallowGoogleSearch: true}},
	{prefix: "o1", spec: ModelSpec{Sampling: openAIReasoningSampling}},
	{prefix: "o3", spec: ModelSpec{Sampling: openAIReasoningSampling}},
	{prefix: "o4", spec: ModelSpec{Sampling: openAIReasoningSampling}},
	{prefix: "gpt-5", spec: ModelSpec{Sampling: openAIMaxCompletionSampling}},
	{prefix: "grok-4.5", spec: ModelSpec{Sampling: grokReasoningSampling}},
	{prefix: "grok-4.6", spec: ModelSpec{Sampling: grokReasoningSampling}},
}
