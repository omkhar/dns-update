package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"dns-update/internal/config"
)

func handleIntrospection(stdout io.Writer, cfg config.Config, options runtimeOptions) (bool, error) {
	switch options.introspectionMode {
	case introspectionModePrintEffectiveConfig:
		return true, printEffectiveConfig(stdout, cfg, options)
	case introspectionModeValidateConfig:
		_, err := fmt.Fprintln(stdout, "config is valid")
		return true, err
	default:
		return false, nil
	}
}

func printEffectiveConfig(stdout io.Writer, cfg config.Config, options runtimeOptions) error {
	cloudflare := effectiveCloudflareConfig{}
	if cfg.Provider.Cloudflare != nil {
		baseURL := ""
		if cfg.Provider.Cloudflare.BaseURL != nil {
			baseURL = cfg.Provider.Cloudflare.BaseURL.String()
		}
		cloudflare = effectiveCloudflareConfig{
			ZoneID:       cfg.Provider.Cloudflare.ZoneID,
			APITokenFile: cfg.Provider.Cloudflare.APITokenFile,
			BaseURL:      baseURL,
			Proxied:      cfg.Provider.Cloudflare.Proxied,
		}
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(effectiveConfig{
		Runtime: effectiveRuntimeConfig{
			ConfigPath: cfg.SourcePath,
			Delete:     options.deleteSelection.String(),
			DryRun:     options.dryRun,
			ForcePush:  options.forcePush,
			Verbose:    options.verbose,
			Timeout:    options.timeout.String(),
		},
		Record: effectiveRecordConfig{
			Name:       cfg.Record.Name,
			Zone:       cfg.Record.Zone,
			TTLSeconds: cfg.Record.TTLSeconds,
		},
		Probe: effectiveProbeConfig{
			IPv4URL:           urlString(cfg.Probe.IPv4URL),
			IPv6URL:           urlString(cfg.Probe.IPv6URL),
			Timeout:           cfg.Probe.Timeout.String(),
			AllowInsecureHTTP: cfg.Probe.AllowInsecureHTTP,
		},
		Provider: effectiveProviderConfig{
			Type:       cfg.Provider.Type,
			Timeout:    cfg.Provider.Timeout.String(),
			Cloudflare: cloudflare,
		},
	})
}

func urlString(value *url.URL) string {
	if value == nil {
		return ""
	}
	return value.String()
}

type effectiveConfig struct {
	Runtime  effectiveRuntimeConfig  `json:"runtime"`
	Record   effectiveRecordConfig   `json:"record"`
	Probe    effectiveProbeConfig    `json:"probe"`
	Provider effectiveProviderConfig `json:"provider"`
}

type effectiveRuntimeConfig struct {
	ConfigPath string `json:"config_path"`
	Delete     string `json:"delete"`
	DryRun     bool   `json:"dry_run"`
	ForcePush  bool   `json:"force_push"`
	Verbose    bool   `json:"verbose"`
	Timeout    string `json:"timeout"`
}

type effectiveRecordConfig struct {
	Name       string `json:"name"`
	Zone       string `json:"zone"`
	TTLSeconds uint32 `json:"ttl_seconds"`
}

type effectiveProbeConfig struct {
	IPv4URL           string `json:"ipv4_url"`
	IPv6URL           string `json:"ipv6_url"`
	Timeout           string `json:"timeout"`
	AllowInsecureHTTP bool   `json:"allow_insecure_http"`
}

type effectiveProviderConfig struct {
	Type       string                    `json:"type"`
	Timeout    string                    `json:"timeout"`
	Cloudflare effectiveCloudflareConfig `json:"cloudflare"`
}

type effectiveCloudflareConfig struct {
	ZoneID       string `json:"zone_id"`
	APITokenFile string `json:"api_token_file"`
	BaseURL      string `json:"base_url"`
	Proxied      bool   `json:"proxied"`
}
