package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// reviewEnvPrefix namespaces every declarative input of the unified reviewer entrypoint.
const reviewEnvPrefix = "REVIEW_"

// ResolveReviewOptions builds ModelOptions from the REVIEW_* namespace. The provider derives
// from the declared model, which leaves REVIEW_MODEL as the single mandatory input.
// An unset REVIEW_MODEL reports ErrSlotNotConfigured, keeping the review job optional.
func ResolveReviewOptions() (ModelOptions, error) {
	model := lookupReviewEnv("MODEL")
	if model == "" {
		return ModelOptions{}, ErrSlotNotConfigured
	}

	apiVersion := lookupReviewEnv("API_VERSION")
	provider, err := ResolveProvider(model, apiVersion)
	if err != nil {
		return ModelOptions{}, err
	}

	canonicalModel, err := NormalizeModel(provider, model)
	if err != nil {
		return ModelOptions{}, err
	}

	opts := ModelOptions{
		Provider:   provider,
		Model:      canonicalModel,
		Timeout:    DefaultTimeout,
		BaseURL:    lookupReviewEnv("BASE_URL"),
		APIVersion: apiVersion,
	}
	if err := applyReviewTunables(&opts); err != nil {
		return ModelOptions{}, err
	}

	if err := Validate(opts); err != nil {
		return ModelOptions{}, err
	}
	return opts, nil
}

// applyReviewTunables overlays the optional decoding and transport parameters. An unset
// variable defers to the model default. A variable with an unparseable value MUST return an
// error naming the variable.
func applyReviewTunables(opts *ModelOptions) error {
	if v := lookupReviewEnv("MAX_TOKENS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return newInvalidTunableError("MAX_TOKENS", v, "a positive integer")
		}
		opts.MaxTokens = n
	}
	if v := lookupReviewEnv("TIMEOUT_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return newInvalidTunableError("TIMEOUT_MINUTES", v, "a positive integer")
		}
		opts.Timeout = time.Duration(n) * time.Minute
	}
	if v := lookupReviewEnv("TEMPERATURE"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return newInvalidTunableError("TEMPERATURE", v, "a finite number")
		}
		opts.Temperature = &f
	}
	if v := lookupReviewEnv("TOP_P"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return newInvalidTunableError("TOP_P", v, "a finite number")
		}
		opts.TopP = &f
	}
	if v := lookupReviewEnv("REASONING_EFFORT"); v != "" {
		opts.ReasoningLevel = v
	}
	return nil
}

func newInvalidTunableError(param, value, want string) error {
	return fmt.Errorf("invalid %s%s %q: want %s", reviewEnvPrefix, param, value, want)
}

func lookupReviewEnv(param string) string {
	return strings.TrimSpace(os.Getenv(reviewEnvPrefix + param))
}
