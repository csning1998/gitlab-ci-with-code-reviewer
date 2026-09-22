package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	yaml "go.yaml.in/yaml/v4"
)

// ErrSlotNotConfigured indicates that a model slot was queried without an underlying configuration.
var ErrSlotNotConfigured = errors.New("model slot is not configured")

// DefaultTimeout establishes fallback HTTP transport duration when unspecified.
const DefaultTimeout = 10 * time.Minute

// SafetySetting defines threshold parameters for provider moderation filters.
type SafetySetting struct {
	Category  string `json:"category" yaml:"category" jsonschema:"required,description=Harm category identifier"`
	Threshold string `json:"threshold" yaml:"threshold" jsonschema:"required,description=Block threshold identifier"`
}

// ModelOptions encapsulates unified inference configuration across providers.
type ModelOptions struct {
	// Mandatory core fields
	Provider string        `json:"provider" yaml:"provider" jsonschema:"required,enum=claude,enum=gemini,enum=openai,enum=azure-openai,enum=grok,enum=local"`
	Model    string        `json:"model" yaml:"model" jsonschema:"required,description=Target model or deployment name"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout" jsonschema:"description=HTTP transport timeout"`

	// Optional decoding and sampling fields
	MaxTokens         int            `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty" jsonschema:"minimum=1"`
	Temperature       *float64       `json:"temperature,omitempty" yaml:"temperature,omitempty" jsonschema:"minimum=0,maximum=2,description=Sampling temperature (0.0 to 1.0 for Claude; 0.0 to 2.0 for other providers)"`
	TopP              *float64       `json:"top_p,omitempty" yaml:"top_p,omitempty" jsonschema:"minimum=0,maximum=1"`
	TopK              *int           `json:"top_k,omitempty" yaml:"top_k,omitempty" jsonschema:"minimum=1"`
	Stop              []string       `json:"stop,omitempty" yaml:"stop,omitempty"`
	FrequencyPenalty  *float64       `json:"frequency_penalty,omitempty" yaml:"frequency_penalty,omitempty" jsonschema:"minimum=-2,maximum=2"`
	PresencePenalty   *float64       `json:"presence_penalty,omitempty" yaml:"presence_penalty,omitempty" jsonschema:"minimum=-2,maximum=2"`
	RepetitionPenalty *float64       `json:"repetition_penalty,omitempty" yaml:"repetition_penalty,omitempty" jsonschema:"minimum=0"`
	MinP              *float64       `json:"min_p,omitempty" yaml:"min_p,omitempty" jsonschema:"minimum=0,maximum=1"`
	Seed              *int64         `json:"seed,omitempty" yaml:"seed,omitempty"`
	N                 *int           `json:"n,omitempty" yaml:"n,omitempty" jsonschema:"minimum=1"`
	LogitBias         map[string]int `json:"logit_bias,omitempty" yaml:"logit_bias,omitempty"`

	// Optional reasoning fields
	ThinkingBudget  *int   `json:"thinking_budget,omitempty" yaml:"thinking_budget,omitempty" jsonschema:"minimum=0"`
	ReasoningLevel  string `json:"reasoning_level,omitempty" yaml:"reasoning_level,omitempty" jsonschema:"enum=none,enum=minimal,enum=low,enum=medium,enum=high,enum=xhigh,enum=max"`
	ThinkingType    string `json:"thinking_type,omitempty" yaml:"thinking_type,omitempty" jsonschema:"enum=disabled,enum=enabled,enum=adaptive"`
	IncludeThoughts *bool  `json:"include_thoughts,omitempty" yaml:"include_thoughts,omitempty"`

	// Optional structured output and debug fields
	ResponseMIMEType string          `json:"response_mime_type,omitempty" yaml:"response_mime_type,omitempty"`
	ResponseSchema   json.RawMessage `json:"response_schema,omitempty" yaml:"response_schema,omitempty"`
	Verbosity        string          `json:"verbosity,omitempty" yaml:"verbosity,omitempty" jsonschema:"enum=low,enum=medium,enum=high"`
	Logprobs         *bool           `json:"logprobs,omitempty" yaml:"logprobs,omitempty"`
	TopLogprobs      *int            `json:"top_logprobs,omitempty" yaml:"top_logprobs,omitempty" jsonschema:"minimum=0,maximum=20"`

	// Optional platform, endpoint, and safety fields
	BaseURL          string          `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	APIVersion       string          `json:"api_version,omitempty" yaml:"api_version,omitempty"`
	ServiceTier      string          `json:"service_tier,omitempty" yaml:"service_tier,omitempty"`
	PromptCacheKey   string          `json:"prompt_cache_key,omitempty" yaml:"prompt_cache_key,omitempty"`
	SafetyIdentifier string          `json:"safety_identifier,omitempty" yaml:"safety_identifier,omitempty"`
	SafetySettings   []SafetySetting `json:"safety_settings,omitempty" yaml:"safety_settings,omitempty"`
	MediaResolution  string          `json:"media_resolution,omitempty" yaml:"media_resolution,omitempty" jsonschema:"enum=MEDIA_RESOLUTION_LOW,enum=MEDIA_RESOLUTION_MEDIUM,enum=MEDIA_RESOLUTION_HIGH"`

	// Optional review prompt customization fields
	Prompt     string `json:"prompt,omitempty" yaml:"prompt,omitempty" jsonschema:"description=Custom review prompt instructions"`
	PromptFile string `json:"prompt_file,omitempty" yaml:"prompt_file,omitempty" jsonschema:"description=File path to custom review prompt instructions"`
}

// ConfigDeclaration defines declarative YAML configuration structure (.gitlab/reviewer.yml).
type ConfigDeclaration struct {
	Defaults ModelOptions            `json:"defaults,omitempty" yaml:"defaults,omitempty" jsonschema:"description=Global default options"`
	Models   map[string]ModelOptions `json:"models,omitempty" yaml:"models,omitempty" jsonschema:"description=Named model configurations"`
	Slots    map[string]string       `json:"slots,omitempty" yaml:"slots,omitempty" jsonschema:"description=Slot to named model bindings"`
}

// GenerateJSONSchema generates standard JSON schema definition directly from ConfigDeclaration.
func GenerateJSONSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            false,
	}
	schema := reflector.Reflect(&ConfigDeclaration{})
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal json schema: %w", err)
	}
	return data, nil
}

// ParseConfigFile unmarshals a YAML reviewer declaration file.
func ParseConfigFile(path string) (*ConfigDeclaration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Strict decoding rejects a misspelled key, since a lenient decoder drops its parameter silently.
	var decl ConfigDeclaration
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&decl); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}
	if err := rejectFractionalIntegers(data); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}

	return &decl, nil
}

// integerFieldNames lists the YAML keys of ModelOptions which hold an integer. The decoder
// truncates a fractional scalar for these fields without an error.
var integerFieldNames = map[string]bool{
	"max_tokens": true, "top_k": true, "n": true, "seed": true, "thinking_budget": true, "top_logprobs": true,
}

// rejectFractionalIntegers reports an integer field which carries a float scalar such as 1.5.
func rejectFractionalIntegers(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 {
		return nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		switch top.Content[i].Value {
		case "defaults":
			if err := checkIntegerFields(top.Content[i+1]); err != nil {
				return err
			}
		case "models":
			models := top.Content[i+1]
			for j := 0; j+1 < len(models.Content); j += 2 {
				if err := checkIntegerFields(models.Content[j+1]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkIntegerFields(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Value == "<<" {
			if err := checkIntegerFields(value); err != nil {
				return err
			}
			continue
		}
		if integerFieldNames[key.Value] && value.Kind == yaml.ScalarNode && value.ShortTag() == "!!float" {
			return fmt.Errorf("line %d: field %q must be an integer, got %q", value.Line, key.Value, value.Value)
		}
	}
	return nil
}

// ResolveModelOptions reads environment variables for a given provider and slot.
func ResolveModelOptions(provider string, slot string) (ModelOptions, error) {
	prefix := strings.ToUpper(provider)
	isSecondary := strings.EqualFold(slot, "secondary")

	var modelKey string
	if isSecondary {
		modelKey = prefix + "_MODEL_SECONDARY"
	} else {
		modelKey = prefix + "_MODEL"
	}

	modelVal := strings.TrimSpace(os.Getenv(modelKey))
	if modelVal == "" {
		return ModelOptions{}, ErrSlotNotConfigured
	}

	opts := ModelOptions{
		Provider: provider,
		Model:    modelVal,
		Timeout:  DefaultTimeout,
	}

	// Resolve max tokens
	maxTokensStr := lookupSlotEnv(prefix, "MAX_TOKENS", isSecondary)
	if maxTokensStr != "" {
		if n, err := strconv.Atoi(maxTokensStr); err == nil && n > 0 {
			opts.MaxTokens = n
		}
	}

	// Resolve temperature
	tempStr := lookupSlotEnv(prefix, "TEMPERATURE", isSecondary)
	if tempStr != "" {
		if f, err := strconv.ParseFloat(tempStr, 64); err == nil {
			opts.Temperature = &f
		}
	}

	// Resolve top_p
	topPStr := lookupSlotEnv(prefix, "TOP_P", isSecondary)
	if topPStr != "" {
		if f, err := strconv.ParseFloat(topPStr, 64); err == nil {
			opts.TopP = &f
		}
	}

	// Resolve top_k
	topKStr := lookupSlotEnv(prefix, "TOP_K", isSecondary)
	if topKStr != "" {
		if n, err := strconv.Atoi(topKStr); err == nil {
			opts.TopK = &n
		}
	}

	// Resolve thinking budget
	thinkingBudgetStr := lookupSlotEnv(prefix, "THINKING_BUDGET", isSecondary)
	if thinkingBudgetStr != "" {
		if n, err := strconv.Atoi(thinkingBudgetStr); err == nil {
			opts.ThinkingBudget = &n
		}
	}

	// Resolve reasoning effort
	reasoningEffortStr := lookupSlotEnv(prefix, "REASONING_EFFORT", isSecondary)
	if reasoningEffortStr != "" {
		opts.ReasoningLevel = reasoningEffortStr
	}

	// Resolve frequency penalty
	freqPenaltyStr := lookupSlotEnv(prefix, "FREQUENCY_PENALTY", isSecondary)
	if freqPenaltyStr != "" {
		if f, err := strconv.ParseFloat(freqPenaltyStr, 64); err == nil {
			opts.FrequencyPenalty = &f
		}
	}

	// Resolve presence penalty
	presPenaltyStr := lookupSlotEnv(prefix, "PRESENCE_PENALTY", isSecondary)
	if presPenaltyStr != "" {
		if f, err := strconv.ParseFloat(presPenaltyStr, 64); err == nil {
			opts.PresencePenalty = &f
		}
	}

	// Resolve prompt overrides
	promptVal := lookupSlotEnv(prefix, "PROMPT", isSecondary)
	if promptVal == "" {
		promptVal = strings.TrimSpace(os.Getenv("REVIEWER_PROMPT"))
	}
	opts.Prompt = promptVal

	// Resolve prompt file overrides
	promptFileVal := lookupSlotEnv(prefix, "PROMPT_FILE", isSecondary)
	if promptFileVal == "" {
		promptFileVal = strings.TrimSpace(os.Getenv("REVIEWER_PROMPT_FILE"))
	}
	opts.PromptFile = promptFileVal

	normalizedModel, err := NormalizeModel(provider, opts.Model)
	if err != nil {
		return ModelOptions{}, err
	}
	opts.Model = normalizedModel

	if err := Validate(opts); err != nil {
		return ModelOptions{}, err
	}

	return opts, nil
}

// validateNumericBounds enforces the numeric bounds which the generated JSON schema advertises.
// A NaN compares false against every bound, and the JSON encoder rejects a NaN value.
func validateNumericBounds(opts ModelOptions) error {
	floats := []struct {
		name     string
		value    *float64
		min, max float64
	}{
		{"temperature", opts.Temperature, 0, 2},
		{"top_p", opts.TopP, 0, 1},
		{"frequency_penalty", opts.FrequencyPenalty, -2, 2},
		{"presence_penalty", opts.PresencePenalty, -2, 2},
		{"repetition_penalty", opts.RepetitionPenalty, 0, math.MaxFloat64},
		{"min_p", opts.MinP, 0, 1},
	}
	for _, f := range floats {
		if f.value == nil {
			continue
		}
		if math.IsNaN(*f.value) || math.IsInf(*f.value, 0) {
			return fmt.Errorf("%s must be a finite number", f.name)
		}
		if *f.value < f.min || *f.value > f.max {
			return fmt.Errorf("%s %v out of range [%v, %v]", f.name, *f.value, f.min, f.max)
		}
	}

	ints := []struct {
		name     string
		value    *int
		min, max int
	}{
		{"top_k", opts.TopK, 1, math.MaxInt},
		{"n", opts.N, 1, math.MaxInt},
		{"top_logprobs", opts.TopLogprobs, 0, 20},
	}
	for _, i := range ints {
		if i.value != nil && (*i.value < i.min || *i.value > i.max) {
			return fmt.Errorf("%s %d out of range [%d, %d]", i.name, *i.value, i.min, i.max)
		}
	}

	if opts.MaxTokens < 0 {
		return fmt.Errorf("max_tokens %d must not be negative", opts.MaxTokens)
	}
	return nil
}

func lookupSlotEnv(prefix, param string, isSecondary bool) string {
	if isSecondary {
		if v := strings.TrimSpace(os.Getenv(prefix + "_" + param + "_SECONDARY")); v != "" {
			return v
		}
	}
	return strings.TrimSpace(os.Getenv(prefix + "_" + param))
}

// Validate executes static compatibility and range checks on ModelOptions.
func Validate(opts ModelOptions) error {
	if opts.Model == "" {
		return errors.New("mandatory field 'model' is required")
	}

	if opts.Provider != "" {
		if _, err := NormalizeModel(opts.Provider, opts.Model); err != nil {
			return err
		}
	}

	if opts.Temperature != nil {
		if *opts.Temperature < 0.0 || *opts.Temperature > 2.0 {
			return fmt.Errorf("temperature %v out of range [0.0, 2.0]", *opts.Temperature)
		}
		if opts.Provider == "claude" && *opts.Temperature > 1.0 {
			return fmt.Errorf("temperature %v exceeds claude limit of 1.0", *opts.Temperature)
		}
	}

	if opts.TopP != nil && (*opts.TopP < 0.0 || *opts.TopP > 1.0) {
		return fmt.Errorf("top_p %v out of range [0.0, 1.0]", *opts.TopP)
	}

	if err := validateNumericBounds(opts); err != nil {
		return err
	}

	if opts.ThinkingBudget != nil && *opts.ThinkingBudget < 0 {
		return errors.New("thinking_budget must be non-negative")
	}

	if strings.TrimSpace(opts.Prompt) != "" && strings.TrimSpace(opts.PromptFile) != "" {
		return errors.New("cannot configure both 'prompt' and 'prompt_file'")
	}

	if opts.ReasoningLevel != "" {
		switch opts.ReasoningLevel {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("invalid reasoning_level %q", opts.ReasoningLevel)
		}
	}

	if opts.ThinkingType != "" {
		switch opts.ThinkingType {
		case "disabled", "enabled", "adaptive":
		default:
			return fmt.Errorf("invalid thinking_type %q", opts.ThinkingType)
		}
	}

	if opts.Verbosity != "" {
		switch opts.Verbosity {
		case "low", "medium", "high":
		default:
			return fmt.Errorf("invalid verbosity %q", opts.Verbosity)
		}
	}

	if opts.MediaResolution != "" {
		switch opts.MediaResolution {
		case "MEDIA_RESOLUTION_LOW", "MEDIA_RESOLUTION_MEDIUM", "MEDIA_RESOLUTION_HIGH":
		default:
			return fmt.Errorf("invalid media_resolution %q", opts.MediaResolution)
		}
	}

	switch opts.Provider {
	case "claude":
		if opts.FrequencyPenalty != nil || opts.PresencePenalty != nil {
			return errors.New("frequency_penalty and presence_penalty are not supported by claude")
		}
		if opts.RepetitionPenalty != nil || opts.MinP != nil {
			return errors.New("repetition_penalty and min_p are not supported by claude")
		}
	case "gemini":
		if opts.RepetitionPenalty != nil || opts.MinP != nil {
			return errors.New("repetition_penalty and min_p are not supported by gemini")
		}
	case "openai", "azure-openai", "grok":
		if opts.TopK != nil {
			return fmt.Errorf("top_k is not supported by %s", opts.Provider)
		}
		if opts.ThinkingBudget != nil {
			return fmt.Errorf("thinking_budget is not supported by %s; use reasoning_level", opts.Provider)
		}
		if opts.RepetitionPenalty != nil || opts.MinP != nil {
			return fmt.Errorf("repetition_penalty and min_p are not supported by %s", opts.Provider)
		}
	}

	return nil
}
