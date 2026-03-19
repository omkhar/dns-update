package app

import (
	"fmt"

	"dns-update/internal/config"
	"dns-update/internal/provider"
	"dns-update/internal/provider/cloudflare"
	"dns-update/internal/securefile"
)

func newProvider(cfg config.ProviderConfig) (provider.Provider, provider.RecordOptions, error) {
	switch cfg.Type {
	case "cloudflare":
		if cfg.Cloudflare == nil {
			return nil, provider.RecordOptions{}, fmt.Errorf("provider.cloudflare is required when provider.type=%q", cfg.Type)
		}

		token, err := securefile.ReadSingleToken(cfg.Cloudflare.APITokenFile)
		if err != nil {
			return nil, provider.RecordOptions{}, fmt.Errorf("read Cloudflare API token file %q: %w", cfg.Cloudflare.APITokenFile, err)
		}

		cloudflareConfig := cloudflare.Config{
			ZoneID:   cfg.Cloudflare.ZoneID,
			APIToken: token,
			BaseURL:  cfg.Cloudflare.BaseURL,
			Timeout:  cfg.Timeout,
			Proxied:  cfg.Cloudflare.Proxied,
		}
		client, err := cloudflare.New(cloudflareConfig)
		if err != nil {
			return nil, provider.RecordOptions{}, fmt.Errorf("build Cloudflare provider: %w", err)
		}

		return client, cloudflareConfig.RecordOptions(), nil
	default:
		return nil, provider.RecordOptions{}, fmt.Errorf("unsupported provider.type %q", cfg.Type)
	}
}
