package app

import (
	"context"
	"strings"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/message"
)

// ConfigureOutputReview keeps credentials in memory for literal redaction.
// It neither loads an external behavior policy nor changes provider settings.
func ConfigureOutputReview(ctx context.Context, messages message.Service, cfg *config.Config) error {
	if !cfg.OutputReviewEnabled() {
		return nil
	}
	var secrets []string
	add := func(value string) {
		if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "$") ||
			strings.HasPrefix(value, "company-auth://") {
			return
		}
		secrets = append(secrets, value)
	}
	if cfg.Providers != nil {
		for _, provider := range cfg.Providers.Copy() {
			add(provider.APIKey)
			if provider.OAuthToken != nil {
				add(provider.OAuthToken.AccessToken)
				add(provider.OAuthToken.RefreshToken)
			}
		}
	}
	return message.EnableOutputReview(ctx, messages, message.ReviewConfig{
		SecretValues: secrets,
	})
}
