package config

import (
	"net/url"
	"os"
	"regexp"
	"time"
)

const (
	defaultProbeIPv4URL      = "https://4.ip.omsab.net/"
	defaultProbeIPv6URL      = "https://6.ip.omsab.net/"
	defaultTimeout           = 10 * time.Second
	defaultCloudflareBaseURL = "https://api.cloudflare.com/client/v4/"
	cloudflareZoneIDExample  = "CLOUDFLARE_ZONE_ID"
	defaultConfigFileName    = "config.json"
	systemConfigPath         = "/etc/dns-update/config.json"
)

var cloudflareZoneIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
var getWorkingDir = os.Getwd

const envProviderCloudflareAPITokenFile = "DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE"

// Config is the validated runtime configuration.
type Config struct {
	SourcePath string
	Record     RecordConfig
	Probe      ProbeConfig
	Provider   ProviderConfig
}

// RecordConfig defines the managed DNS record.
type RecordConfig struct {
	Name       string
	Zone       string
	TTLSeconds uint32
}

// ProbeConfig defines how the egress detection endpoints are queried.
type ProbeConfig struct {
	IPv4URL             *url.URL
	IPv6URL             *url.URL
	Timeout             time.Duration
	AllowInsecureHTTP   bool
	AllowPartialFailure bool
}

// ProviderConfig defines the DNS backend integration.
type ProviderConfig struct {
	Type       string
	Timeout    time.Duration
	Cloudflare *CloudflareConfig
}

// CloudflareConfig configures the Cloudflare DNS provider.
type CloudflareConfig struct {
	ZoneID       string
	APITokenFile string
	BaseURL      *url.URL
	Proxied      bool
}

// LoadOptions defines all configuration sources and their precedence controls.
type LoadOptions struct {
	Path         string
	ExplicitPath bool
	WorkingDir   string
	Env          map[string]string
}

type fileConfig struct {
	Record   fileRecordConfig   `json:"record"`
	Probe    fileProbeConfig    `json:"probe"`
	Provider fileProviderConfig `json:"provider"`
}

type fileRecordConfig struct {
	Name       string `json:"name"`
	Zone       string `json:"zone"`
	TTLSeconds uint32 `json:"ttl_seconds"`
}

type fileProbeConfig struct {
	IPv4URL             string `json:"ipv4_url"`
	IPv6URL             string `json:"ipv6_url"`
	Timeout             string `json:"timeout"`
	AllowInsecureHTTP   bool   `json:"allow_insecure_http"`
	AllowPartialFailure bool   `json:"allow_partial_failure"`
}

type fileProviderConfig struct {
	Type       string               `json:"type"`
	Timeout    string               `json:"timeout"`
	Cloudflare fileCloudflareConfig `json:"cloudflare"`
}

type fileCloudflareConfig struct {
	ZoneID       string `json:"zone_id"`
	APITokenFile string `json:"api_token_file"`
	BaseURL      string `json:"base_url"`
	Proxied      bool   `json:"proxied"`
}
