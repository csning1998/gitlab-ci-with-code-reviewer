package reviewerconfig

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	canonicalRegistry   = map[string]map[string]struct{}{}
	aliasRegistry       = map[string]map[string]string{}
	specByProviderModel = map[string]map[string]ModelSpec{}
	specByGlobalID      = map[string]ModelSpec{}
)

func init() {
	for _, spec := range modelCatalog {
		if canonicalRegistry[spec.Provider] == nil {
			canonicalRegistry[spec.Provider] = map[string]struct{}{}
		}
		if aliasRegistry[spec.Provider] == nil {
			aliasRegistry[spec.Provider] = map[string]string{}
		}
		if specByProviderModel[spec.Provider] == nil {
			specByProviderModel[spec.Provider] = map[string]ModelSpec{}
		}

		canonicalRegistry[spec.Provider][spec.CanonicalID] = struct{}{}
		specByProviderModel[spec.Provider][spec.CanonicalID] = spec
		specByGlobalID[strings.ToLower(spec.CanonicalID)] = spec

		for _, alias := range spec.Aliases {
			aliasRegistry[spec.Provider][alias] = spec.CanonicalID
			specByProviderModel[spec.Provider][alias] = spec
			specByGlobalID[strings.ToLower(alias)] = spec
		}
	}
}

// LookupProviderPolicy returns the capability policy declared for provider.
func LookupProviderPolicy(provider string) (ProviderPolicy, bool) {
	policy, ok := providerPolicies[strings.TrimSpace(provider)]
	return policy, ok
}

// LookupModelSpec returns the declared ModelSpec for a cataloged canonical or alias model.
func LookupModelSpec(provider, model string) (ModelSpec, bool) {
	trimmedProvider := strings.TrimSpace(provider)
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		return ModelSpec{}, false
	}

	if trimmedProvider != "" {
		if specs, ok := specByProviderModel[trimmedProvider]; ok {
			if spec, found := specs[trimmedModel]; found {
				return spec, true
			}
		}
		return ModelSpec{}, false
	}

	spec, ok := specByGlobalID[strings.ToLower(trimmedModel)]
	return spec, ok
}

// ResolveModelSpec resolves the runtime ModelSpec for provider and model, falling back to
// generation prefix defaults when a direct caller uses an uncataloged or custom model.
func ResolveModelSpec(provider, model string) ModelSpec {
	if spec, ok := LookupModelSpec(provider, model); ok {
		return spec
	}

	trimmedProvider := strings.TrimSpace(provider)
	lowerModel := strings.ToLower(strings.TrimSpace(model))
	for _, fb := range prefixFallbacks {
		if fb.provider != "" && trimmedProvider != "" && fb.provider != trimmedProvider {
			continue
		}
		if strings.HasPrefix(lowerModel, fb.prefix) {
			return fb.spec
		}
	}
	return ModelSpec{Provider: trimmedProvider, CanonicalID: strings.TrimSpace(model)}
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
