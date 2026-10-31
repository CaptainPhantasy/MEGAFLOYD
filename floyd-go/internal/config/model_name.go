package config

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveModel finds a configured model by ID, display name, or a unique
// partial name. A provider/name pair disambiguates models from different
// providers. It never switches accounts or fetches a different catalog.
func ResolveModel(cfg *Config, name string) (SelectedModel, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if cfg == nil || cfg.Providers == nil || name == "" {
		return SelectedModel{}, fmt.Errorf("enter a configured model name or provider/model")
	}
	query, providerScope := name, ""
	if prefix, rest, qualified := strings.Cut(name, "/"); qualified {
		for providerID := range cfg.Providers.Seq2() {
			if strings.EqualFold(prefix, providerID) {
				query, providerScope = rest, prefix
				break
			}
		}
	}
	if query == "" {
		return SelectedModel{}, fmt.Errorf("enter a configured model name after the provider prefix")
	}
	var exact, partial []SelectedModel
	for providerID, provider := range cfg.Providers.Seq2() {
		if providerScope != "" && !strings.EqualFold(providerScope, providerID) {
			continue
		}
		for _, model := range provider.Models {
			id, label := strings.ToLower(model.ID), strings.ToLower(model.Name)
			qualified := strings.ToLower(providerID + "/" + model.ID)
			selected := SelectedModel{Provider: providerID, Model: model.ID, MaxTokens: model.DefaultMaxTokens, ReasoningEffort: model.DefaultReasoningEffort}
			if query == id || query == label || name == qualified {
				exact = append(exact, selected)
			} else if strings.Contains(id, query) || (label != "" && strings.Contains(label, query)) {
				partial = append(partial, selected)
			}
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = partial
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return SelectedModel{}, fmt.Errorf("no configured model matches %q; use /provider to see available models", name)
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match.Provider+"/"+match.Model)
	}
	sort.Strings(names)
	return SelectedModel{}, fmt.Errorf("model %q is ambiguous; choose one of: %s", name, strings.Join(names, ", "))
}
