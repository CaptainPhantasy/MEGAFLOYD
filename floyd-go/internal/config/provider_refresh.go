package config

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/legacy-ai/floyd/internal/discover"
)

type ProviderRefreshChange struct {
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName"`
	Added        int    `json:"added"`
	Removed      int    `json:"removed"`
}

type ProviderRefreshResult struct {
	Changed   []ProviderRefreshChange  `json:"changed"`
	Unchanged []string                 `json:"unchanged"`
	Failed    []ProviderRefreshFailure `json:"failed"`
}

type ProviderRefreshFailure struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

func (s *ConfigStore) RefreshProviderModels(ctx context.Context, providerID string, oauthOnly bool) (ProviderRefreshResult, error) {
	result := ProviderRefreshResult{Changed: []ProviderRefreshChange{}, Unchanged: []string{}, Failed: []ProviderRefreshFailure{}}
	providers := s.Config().Providers.Copy()
	if providerID != "" {
		if _, ok := providers[providerID]; !ok {
			return result, fmt.Errorf("provider is not configured")
		}
	}
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		p := providers[id]
		if providerID != "" && id != providerID || p.Disable || oauthOnly && p.OAuthToken == nil {
			continue
		}
		if p.Source != nil {
			result.Failed = append(result.Failed, ProviderRefreshFailure{id, "Refresh this registry through its configured source"})
			continue
		}
		bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := func() error {
			if p.OAuthToken != nil && p.OAuthToken.IsExpired() {
				if err := s.RefreshOAuthToken(bounded, ScopeGlobal, id); err != nil {
					return err
				}
				p, _ = s.Config().Providers.Get(id)
			}
			if id == "hyper" {
				if err := s.RefetchHyperProvider(bounded); err != nil {
					return err
				}
				after, _ := s.Config().Providers.Get(id)
				if reflect.DeepEqual(p.Models, after.Models) {
					result.Unchanged = append(result.Unchanged, id)
				} else {
					result.Changed = append(result.Changed, ProviderRefreshChange{id, p.Name, len(after.Models), len(p.Models)})
				}
				return nil
			}
			if p.BaseURL == "" {
				return fmt.Errorf("provider has no models endpoint")
			}
			models, err := discover.DiscoverModels(bounded, discover.Config{ID: id, BaseURL: p.BaseURL, APIKey: p.APIKey, ExtraHeaders: p.ExtraHeaders, ExistingModels: p.Models}, s.Resolver())
			if err != nil {
				return err
			}
			if len(models) == 0 {
				return fmt.Errorf("provider returned no models")
			}
			if enrich := discover.GetEnricher(string(p.Type)); enrich != nil {
				models, err = enrich.EnrichModels(bounded, discover.Config{ID: id, BaseURL: p.BaseURL, APIKey: p.APIKey, ExtraHeaders: p.ExtraHeaders, ExistingModels: p.Models}, s.Resolver(), models)
				if err != nil {
					return err
				}
			}
			if reflect.DeepEqual(p.Models, models) {
				result.Unchanged = append(result.Unchanged, id)
				return nil
			}
			before := map[string]bool{}
			after := map[string]bool{}
			for _, m := range p.Models {
				before[m.ID] = true
			}
			for _, m := range models {
				after[m.ID] = true
			}
			added, removed := 0, 0
			for id := range after {
				if !before[id] {
					added++
				}
			}
			for id := range before {
				if !after[id] {
					removed++
				}
			}
			if err := s.update(ScopeGlobal, func(cfg *Config) map[string]any {
				current, _ := cfg.Providers.Get(id)
				current.Models = models
				cfg.Providers.Set(id, current)
				return map[string]any{"providers." + id + ".models": models}
			}); err != nil {
				return err
			}
			result.Changed = append(result.Changed, ProviderRefreshChange{id, p.Name, added, removed})
			return nil
		}()
		cancel()
		if err != nil {
			result.Failed = append(result.Failed, ProviderRefreshFailure{id, err.Error()})
		}
		if ctx.Err() != nil {
			break
		}
	}
	return result, nil
}
