package config

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"dns-update/internal/netutil"
	"dns-update/internal/securefile"
)

// Load reads, validates, and normalizes configuration from path.
func Load(path string) (Config, error) {
	return LoadWithOptions(LoadOptions{
		Path:         path,
		ExplicitPath: true,
	})
}

// LoadWithOptions reads configuration from file, applies the supported env overrides,
// and returns the validated runtime config.
func LoadWithOptions(options LoadOptions) (Config, error) {
	workingDir, err := resolveWorkingDir(options.WorkingDir)
	if err != nil {
		return Config{}, err
	}

	raw := fileConfig{}
	baseDir := workingDir

	if options.Path != "" || options.ExplicitPath {
		fileConfig, fileBaseDir, err := loadFileConfig(options.Path, options.ExplicitPath)
		if err != nil {
			return Config{}, err
		}
		raw = fileConfig
		if fileBaseDir != "" {
			baseDir = fileBaseDir
		}
	}

	raw = applyEnvironment(raw, options.Env, workingDir)
	return normalize(raw, baseDir)
}

func normalize(raw fileConfig, baseDir string) (Config, error) {
	recordConfig, err := normalizeRecordConfig(raw.Record)
	if err != nil {
		return Config{}, err
	}

	probeConfig, err := normalizeProbeConfig(raw.Probe)
	if err != nil {
		return Config{}, err
	}

	providerConfig, err := normalizeTopLevelProviderConfig(raw.Provider, baseDir, recordConfig.TTLSeconds)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Record:   recordConfig,
		Probe:    probeConfig,
		Provider: providerConfig,
	}, nil
}

func normalizeRecordConfig(raw fileRecordConfig) (RecordConfig, error) {
	recordName, err := normalizeFQDN(raw.Name)
	if err != nil {
		return RecordConfig{}, fmt.Errorf("record.name: %w", err)
	}

	zoneName, err := normalizeFQDN(raw.Zone)
	if err != nil {
		return RecordConfig{}, fmt.Errorf("record.zone: %w", err)
	}

	if !isNameWithinZone(recordName, zoneName) {
		return RecordConfig{}, fmt.Errorf("record.name %q must be within record.zone %q", recordName, zoneName)
	}
	if raw.TTLSeconds == 0 {
		return RecordConfig{}, errors.New("record.ttl_seconds must be greater than zero")
	}

	return RecordConfig{
		Name:       recordName,
		Zone:       zoneName,
		TTLSeconds: raw.TTLSeconds,
	}, nil
}

func normalizeProbeConfig(raw fileProbeConfig) (ProbeConfig, error) {
	ipv4URL, err := parseProbeURL(raw.IPv4URL, defaultProbeIPv4URL, raw.AllowInsecureHTTP)
	if err != nil {
		return ProbeConfig{}, fmt.Errorf("probe.ipv4_url: %w", err)
	}

	ipv6URL, err := parseProbeURL(raw.IPv6URL, defaultProbeIPv6URL, raw.AllowInsecureHTTP)
	if err != nil {
		return ProbeConfig{}, fmt.Errorf("probe.ipv6_url: %w", err)
	}

	timeout, err := parseDurationOrDefault(raw.Timeout, defaultTimeout)
	if err != nil {
		return ProbeConfig{}, fmt.Errorf("probe.timeout: %w", err)
	}

	return ProbeConfig{
		IPv4URL:           ipv4URL,
		IPv6URL:           ipv6URL,
		Timeout:           timeout,
		AllowInsecureHTTP: raw.AllowInsecureHTTP,
	}, nil
}

func normalizeTopLevelProviderConfig(raw fileProviderConfig, baseDir string, ttlSeconds uint32) (ProviderConfig, error) {
	timeout, err := parseDurationOrDefault(raw.Timeout, defaultTimeout)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("provider.timeout: %w", err)
	}

	providerType := strings.TrimSpace(strings.ToLower(raw.Type))
	if providerType == "" {
		return ProviderConfig{}, errors.New("provider.type is required")
	}

	return normalizeProviderConfig(providerType, timeout, raw.Cloudflare, baseDir, ttlSeconds)
}

func normalizeFQDN(value string) (string, error) {
	name := strings.TrimSpace(strings.ToLower(value))
	if name == "" {
		return "", errors.New("value is required")
	}
	if strings.ContainsAny(name, " \t\r\n") {
		return "", errors.New("must not contain whitespace")
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	if len(name) > 253 {
		return "", errors.New("must be 253 bytes or less")
	}

	trimmed := strings.TrimSuffix(name, ".")
	labels := strings.Split(trimmed, ".")
	for _, label := range labels {
		if label == "" {
			return "", errors.New("contains an empty label")
		}
		if len(label) > 63 {
			return "", fmt.Errorf("label %q exceeds 63 bytes", label)
		}
		if label == "*" {
			continue
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				continue
			}
			return "", fmt.Errorf("label %q contains unsupported characters", label)
		}
	}

	return name, nil
}

func isNameWithinZone(name string, zone string) bool {
	normalizedName := strings.TrimSuffix(name, ".")
	normalizedZone := strings.TrimSuffix(zone, ".")
	if normalizedName == normalizedZone {
		return true
	}
	return strings.HasSuffix(normalizedName, "."+normalizedZone)
}

func parseProbeURL(raw string, fallback string, allowHTTP bool) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = fallback
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	switch parsed.Scheme {
	case "https":
	case "http":
		if !allowHTTP {
			return nil, errors.New("http is disabled; set probe.allow_insecure_http to true to allow it")
		}
	default:
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if err := validateURLStructure(parsed, fallback); err != nil {
		return nil, err
	}
	if parsed.Scheme == "http" && !netutil.IsLoopbackHost(parsed.Hostname()) {
		return nil, errors.New("http probe URLs must use loopback or localhost")
	}

	return parsed, nil
}

func parseDurationOrDefault(raw string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, errors.New("must be greater than zero")
	}
	return duration, nil
}

func normalizeProviderConfig(providerType string, timeout time.Duration, raw fileCloudflareConfig, baseDir string, ttlSeconds uint32) (ProviderConfig, error) {
	switch providerType {
	case "cloudflare":
		if err := validateCloudflareTTL(ttlSeconds); err != nil {
			return ProviderConfig{}, fmt.Errorf("record.ttl_seconds: %w", err)
		}

		zoneID := strings.TrimSpace(raw.ZoneID)
		if zoneID == "" {
			return ProviderConfig{}, errors.New("provider.cloudflare.zone_id is required")
		}
		if zoneID != cloudflareZoneIDExample && !cloudflareZoneIDPattern.MatchString(zoneID) {
			return ProviderConfig{}, errors.New("provider.cloudflare.zone_id must be a 32-character hexadecimal Cloudflare zone ID")
		}
		if cloudflareZoneIDPattern.MatchString(zoneID) {
			zoneID = strings.ToLower(zoneID)
		}

		if strings.TrimSpace(raw.APITokenFile) == "" {
			return ProviderConfig{}, errors.New("provider.cloudflare.api_token_file is required")
		}
		apiTokenFile := raw.APITokenFile
		if !filepath.IsAbs(apiTokenFile) {
			apiTokenFile = filepath.Join(baseDir, apiTokenFile)
		}
		if err := validateSecretFile(apiTokenFile); err != nil {
			return ProviderConfig{}, fmt.Errorf("provider.cloudflare.api_token_file: %w", err)
		}

		baseURL, err := parseHTTPSURL(raw.BaseURL, defaultCloudflareBaseURL)
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("provider.cloudflare.base_url: %w", err)
		}

		return ProviderConfig{
			Type:    providerType,
			Timeout: timeout,
			Cloudflare: &CloudflareConfig{
				ZoneID:       zoneID,
				APITokenFile: apiTokenFile,
				BaseURL:      baseURL,
				Proxied:      raw.Proxied,
			},
		}, nil
	default:
		return ProviderConfig{}, fmt.Errorf("provider.type %q is not supported", providerType)
	}
}

func validateSecretFile(path string) error {
	return securefile.Validate(path)
}

func parseHTTPSURL(raw string, fallback string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = fallback
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("must use https")
	}
	if err := validateURLStructure(parsed, fallback); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed, nil
}

func validateURLStructure(parsed *url.URL, fallback string) error {
	if parsed.Host == "" {
		return errors.New("host is required")
	}
	if parsed.User != nil {
		return errors.New("userinfo is not allowed")
	}
	if parsed.Fragment != "" {
		return errors.New("fragments are not allowed")
	}
	if parsed.RawQuery != "" {
		return errors.New("query parameters are not allowed")
	}
	return validateTrustedHost(parsed, fallback)
}

func validateTrustedHost(parsed *url.URL, fallback string) error {
	fallbackURL, err := url.Parse(fallback)
	if err != nil {
		return fmt.Errorf("parse fallback URL: %w", err)
	}

	host := strings.ToLower(parsed.Hostname())
	allowedHost := strings.ToLower(fallbackURL.Hostname())
	if host == allowedHost || netutil.IsLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("host must be %q or loopback/localhost", allowedHost)
}

func validateCloudflareTTL(ttlSeconds uint32) error {
	switch {
	case ttlSeconds == 1:
		return nil
	case ttlSeconds >= 30 && ttlSeconds <= 86400:
		return nil
	default:
		return errors.New("for Cloudflare, ttl_seconds must be 1 (automatic) or between 30 and 86400 seconds")
	}
}
