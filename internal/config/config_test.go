package config

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestLoadRejectsInsecureHTTPByDefault(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "probe": {
    "ipv4_url": "http://4.ip.omsab.net/",
    "ipv6_url": "https://6.ip.omsab.net/",
    "timeout": "10s"
  },
  "provider": {
    "type": "cloudflare",
    "timeout": "10s",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "` + tokenFile + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "http is disabled") {
		t.Fatalf("Load() error = %v, want insecure HTTP rejection", err)
	}
}

func TestLoadRejectsGroupReadableTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "probe": {
    "timeout": "10s"
  },
  "provider": {
    "type": "cloudflare",
    "timeout": "10s",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "` + tokenFile + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "must not be accessible by group or other users") {
		t.Fatalf("Load() error = %v, want token file permission rejection", err)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "` + tokenFile + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := configView{
		RecordName:        "host.example.com.",
		RecordZone:        "example.com.",
		RecordTTLSeconds:  300,
		ProbeIPv4URL:      mustURLString(t, defaultProbeIPv4URL),
		ProbeIPv6URL:      mustURLString(t, defaultProbeIPv6URL),
		ProbeTimeout:      defaultTimeout,
		AllowInsecureHTTP: false,
		ProviderType:      "cloudflare",
		ProviderTimeout:   defaultTimeout,
		CloudflareZoneID:  "023e105f4ecef8ad9ca31a8372d0c353",
		CloudflareToken:   tokenFile,
		CloudflareBaseURL: mustURLString(t, defaultCloudflareBaseURL),
		CloudflareProxied: false,
	}
	if diff := cmp.Diff(want, viewConfig(cfg)); diff != "" {
		t.Fatalf("Load() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadAcceptsCloudflareSamplePlaceholders(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token.example")
	if err := os.WriteFile(tokenFile, []byte("CLOUDFLARE_TOKEN\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(dir, "config.example.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "CLOUDFLARE_ZONE_ID",
      "api_token_file": "` + tokenFile + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Provider.Cloudflare.ZoneID, cloudflareZoneIDExample; got != want {
		t.Fatalf("cfg.Provider.Cloudflare.ZoneID = %q, want %q", got, want)
	}
	if got, want := cfg.Provider.Cloudflare.APITokenFile, tokenFile; got != want {
		t.Fatalf("cfg.Provider.Cloudflare.APITokenFile = %q, want %q", got, want)
	}
}

func TestLoadRejectsUnsupportedCloudflareTTL(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 15
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "` + tokenFile + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "ttl_seconds") {
		t.Fatalf("Load() error = %v, want Cloudflare TTL rejection", err)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("Load() error = nil, want read error")
	}

	if err := os.WriteFile(configPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("Load() error = nil, want JSON decode error")
	}

	if err := os.WriteFile(configPath, []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("Load() error = nil, want unknown field error")
	}

	if err := os.WriteFile(configPath, []byte(`{"record":{}}
{"extra":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("Load() error = nil, want trailing JSON error")
	}

	if err := os.WriteFile(configPath, []byte(`{"record":{}}
{}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(configPath); err == nil || !strings.Contains(err.Error(), "unexpected trailing JSON data") {
		t.Fatalf("Load() error = %v, want explicit trailing JSON data error", err)
	}
}

func TestNormalizeFQDN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		want      string
		wantError bool
	}{
		{name: "success", value: "HOST.Example.com.", want: "host.example.com."},
		{name: "wildcard", value: "*.example.com.", want: "*.example.com."},
		{name: "empty", value: "", wantError: true},
		{name: "whitespace", value: "bad name.", wantError: true},
		{name: "missing dot", value: "example.com", want: "example.com."},
		{name: "empty label", value: "bad..example.", wantError: true},
		{name: "long label", value: strings.Repeat("a", 64) + ".example.", wantError: true},
		{name: "bad char", value: "bad!.example.", wantError: true},
		{name: "too long", value: strings.Repeat("a", 254) + ".", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeFQDN(test.value)
			if test.wantError {
				if err == nil {
					t.Fatal("normalizeFQDN() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeFQDN() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("normalizeFQDN() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsNameWithinZone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		zone string
		want bool
	}{
		{name: "zone apex", host: "example.com.", zone: "example.com.", want: true},
		{name: "subdomain", host: "host.example.com.", zone: "example.com.", want: true},
		{name: "sibling suffix", host: "badexample.com.", zone: "example.com.", want: false},
		{name: "other zone", host: "host.other.com.", zone: "example.com.", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isNameWithinZone(test.host, test.zone); got != test.want {
				t.Fatalf("isNameWithinZone(%q, %q) = %t, want %t", test.host, test.zone, got, test.want)
			}
		})
	}
}

func TestParseProbeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		fallback  string
		allowHTTP bool
		want      string
		wantError bool
	}{
		{name: "default", fallback: defaultProbeIPv4URL, want: defaultProbeIPv4URL},
		{name: "https loopback", raw: "https://127.0.0.1/ip", fallback: defaultProbeIPv4URL, want: "https://127.0.0.1/ip"},
		{name: "http loopback allowed", raw: "http://127.0.0.1/ip", fallback: defaultProbeIPv4URL, allowHTTP: true, want: "http://127.0.0.1/ip"},
		{name: "missing host", raw: "https:///path", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "userinfo", raw: "https://user@example.com", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "fragment", raw: "https://127.0.0.1/ip#fragment", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "query", raw: "https://127.0.0.1/ip?value=true", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "http disallowed", raw: "http://127.0.0.1/ip", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "http default host still rejected", raw: "http://4.ip.omsab.net/ip", fallback: defaultProbeIPv4URL, allowHTTP: true, wantError: true},
		{name: "remote https override rejected", raw: "https://example.com/ip", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "remote http override rejected", raw: "http://example.com/ip", fallback: defaultProbeIPv4URL, allowHTTP: true, wantError: true},
		{name: "unsupported", raw: "ftp://example.com", fallback: defaultProbeIPv4URL, wantError: true},
		{name: "parse error", raw: "%", fallback: defaultProbeIPv4URL, wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseProbeURL(test.raw, test.fallback, test.allowHTTP)
			if test.wantError {
				if err == nil {
					t.Fatal("parseProbeURL() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseProbeURL() error = %v", err)
			}
			if got.String() != test.want {
				t.Fatalf("parseProbeURL() = %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestParseDurationOrDefault(t *testing.T) {
	t.Parallel()

	if got, err := parseDurationOrDefault("", time.Second); err != nil || got != time.Second {
		t.Fatalf("parseDurationOrDefault() = %v, %v, want %v, nil", got, err, time.Second)
	}
	if _, err := parseDurationOrDefault("invalid", time.Second); err == nil {
		t.Fatal("parseDurationOrDefault() error = nil, want parse error")
	}
	if _, err := parseDurationOrDefault("0s", time.Second); err == nil {
		t.Fatal("parseDurationOrDefault() error = nil, want non-positive error")
	}
}

func TestNormalizeProviderConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := normalizeProviderConfig("cloudflare", time.Second, fileCloudflareConfig{
		ZoneID:       "023E105F4ECEF8AD9CA31A8372D0C353",
		APITokenFile: tokenFile,
		BaseURL:      "https://api.cloudflare.com/client/v4",
		Proxied:      true,
	}, dir, 300)
	if err != nil {
		t.Fatalf("normalizeProviderConfig() error = %v", err)
	}
	if got, want := cfg.Cloudflare.ZoneID, "023e105f4ecef8ad9ca31a8372d0c353"; got != want {
		t.Fatalf("cfg.Cloudflare.ZoneID = %q, want %q", got, want)
	}
	if !cfg.Cloudflare.Proxied {
		t.Fatal("cfg.Cloudflare.Proxied = false, want true")
	}

	relativeCfg, err := normalizeProviderConfig("cloudflare", time.Second, fileCloudflareConfig{
		ZoneID:       "023E105F4ECEF8AD9CA31A8372D0C353",
		APITokenFile: filepath.Base(tokenFile),
	}, dir, 300)
	if err != nil {
		t.Fatalf("normalizeProviderConfig() with relative token path error = %v", err)
	}
	if got, want := relativeCfg.Cloudflare.APITokenFile, tokenFile; got != want {
		t.Fatalf("relativeCfg.Cloudflare.APITokenFile = %q, want %q", got, want)
	}

	if _, err := normalizeProviderConfig("cloudflare", time.Second, fileCloudflareConfig{
		ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
		APITokenFile: tokenFile,
	}, dir, 2); err == nil {
		t.Fatal("normalizeProviderConfig() error = nil, want Cloudflare TTL validation error")
	}

	tests := []fileCloudflareConfig{
		{},
		{ZoneID: "bad", APITokenFile: tokenFile},
		{ZoneID: "023e105f4ecef8ad9ca31a8372d0c353"},
		{ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: filepath.Join(dir, "missing")},
		{ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile, BaseURL: "http://api.cloudflare.com/client/v4/"},
	}
	for _, raw := range tests {
		raw := raw
		t.Run(raw.ZoneID+raw.APITokenFile+raw.BaseURL, func(t *testing.T) {
			t.Parallel()
			if _, err := normalizeProviderConfig("cloudflare", time.Second, raw, dir, 300); err == nil {
				t.Fatal("normalizeProviderConfig() error = nil, want non-nil")
			}
		})
	}

	if _, err := normalizeProviderConfig("unsupported", time.Second, fileCloudflareConfig{}, dir, 300); err == nil {
		t.Fatal("normalizeProviderConfig() error = nil, want unsupported provider error")
	}
}

func TestNormalizeErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []fileConfig{
		{
			Record: fileRecordConfig{Name: "", Zone: "example.com.", TTLSeconds: 300},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record: fileRecordConfig{Name: "host.example.com.", Zone: "", TTLSeconds: 300},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record: fileRecordConfig{Name: "host.example.com.", Zone: "example.com.", TTLSeconds: 300},
			Probe:  fileProbeConfig{IPv4URL: "ftp://example.com"},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record: fileRecordConfig{Name: "host.example.com.", Zone: "example.com.", TTLSeconds: 300},
			Probe:  fileProbeConfig{IPv6URL: "ftp://example.com"},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record: fileRecordConfig{Name: "host.example.com.", Zone: "example.com.", TTLSeconds: 300},
			Probe:  fileProbeConfig{Timeout: "bad"},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record: fileRecordConfig{Name: "host.example.com.", Zone: "example.com.", TTLSeconds: 300},
			Provider: fileProviderConfig{Type: "cloudflare", Timeout: "bad", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record:   fileRecordConfig{Name: "host.example.com.", Zone: "example.com.", TTLSeconds: 300},
			Provider: fileProviderConfig{},
		},
		{
			Record: fileRecordConfig{Name: "host.other.com.", Zone: "example.com.", TTLSeconds: 300},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
		{
			Record: fileRecordConfig{Name: "host.example.com.", Zone: "example.com.", TTLSeconds: 0},
			Provider: fileProviderConfig{Type: "cloudflare", Cloudflare: fileCloudflareConfig{
				ZoneID: "023e105f4ecef8ad9ca31a8372d0c353", APITokenFile: tokenFile,
			}},
		},
	}

	for _, raw := range tests {
		raw := raw
		t.Run(raw.Record.Name+raw.Record.Zone+raw.Probe.IPv4URL+raw.Probe.IPv6URL+raw.Probe.Timeout+raw.Provider.Timeout+raw.Provider.Type, func(t *testing.T) {
			t.Parallel()
			if _, err := normalize(raw, dir); err == nil {
				t.Fatal("normalize() error = nil, want non-nil")
			}
		})
	}
}

func TestValidateSecretFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	regular := filepath.Join(dir, "secret")
	if err := os.WriteFile(regular, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := validateSecretFile(regular); err != nil {
		t.Fatalf("validateSecretFile() error = %v", err)
	}

	symlinkPath := filepath.Join(dir, "secret.link")
	if err := os.Symlink(regular, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := validateSecretFile(symlinkPath); err == nil {
		t.Fatal("validateSecretFile() error = nil, want symlink error")
	}

	subdir := filepath.Join(dir, "dir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := validateSecretFile(subdir); err == nil {
		t.Fatal("validateSecretFile() error = nil, want regular-file error")
	}

	insecure := filepath.Join(dir, "insecure")
	if err := os.WriteFile(insecure, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := validateSecretFile(insecure); err == nil {
		t.Fatal("validateSecretFile() error = nil, want permission error")
	}
}

func TestParseHTTPSURL(t *testing.T) {
	t.Parallel()

	if got, err := parseHTTPSURL("", defaultCloudflareBaseURL); err != nil || got.String() != defaultCloudflareBaseURL {
		t.Fatalf("parseHTTPSURL() = %v, %v, want %q, nil", got, err, defaultCloudflareBaseURL)
	}
	if got, err := parseHTTPSURL("https://127.0.0.1/client/v4/", defaultCloudflareBaseURL); err != nil || got.String() != "https://127.0.0.1/client/v4/" {
		t.Fatalf("parseHTTPSURL(loopback) = %v, %v, want loopback URL, nil", got, err)
	}

	tests := []string{
		"http://example.com",
		"%",
		"https:///path",
		"https://user@example.com",
		"https://example.com#fragment",
		"https://example.com/path?query=true",
		"https://api.example.com/client/v4/",
	}
	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := parseHTTPSURL(raw, defaultCloudflareBaseURL); err == nil {
				t.Fatal("parseHTTPSURL() error = nil, want non-nil")
			}
		})
	}
}

func TestValidateTrustedHostFallbackError(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse("https://127.0.0.1/test")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if err := validateTrustedHost(parsed, "%"); err == nil {
		t.Fatal("validateTrustedHost() error = nil, want non-nil")
	}
}


func TestValidateCloudflareTTL(t *testing.T) {
	t.Parallel()

	for _, ttl := range []uint32{1, 30, 86400} {
		if err := validateCloudflareTTL(ttl); err != nil {
			t.Fatalf("validateCloudflareTTL(%d) error = %v", ttl, err)
		}
	}
	if err := validateCloudflareTTL(2); err == nil {
		t.Fatal("validateCloudflareTTL() error = nil, want non-nil")
	}
}

func TestLoadWithOptionsOverridesCloudflareTokenPathFromEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileToken := filepath.Join(dir, "file.token")
	envToken := filepath.Join(dir, "env.token")
	for _, tokenFile := range []string{fileToken, envToken} {
		if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", tokenFile, err)
		}
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "` + fileToken + `"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadWithOptions(LoadOptions{
		Path:       configPath,
		WorkingDir: dir,
		Env: map[string]string{
			envProviderCloudflareAPITokenFile: filepath.Base(envToken),
		},
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if got, want := cfg.Provider.Cloudflare.APITokenFile, envToken; got != want {
		t.Fatalf("cfg.Provider.Cloudflare.APITokenFile = %q, want %q", got, want)
	}
}

func TestLoadWithOptionsResolvesRelativeCloudflareTokenPathFromEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "env.token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	configJSON := `{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
      "api_token_file": "cloudflare.token"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadWithOptions(LoadOptions{
		Path:       configPath,
		WorkingDir: dir,
		Env: map[string]string{
			envProviderCloudflareAPITokenFile: filepath.Base(tokenFile),
		},
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if got, want := cfg.Provider.Cloudflare.APITokenFile, tokenFile; got != want {
		t.Fatalf("cfg.Provider.Cloudflare.APITokenFile = %q, want %q", got, want)
	}
}

func TestLoadWithOptionsExplicitMissingConfigFails(t *testing.T) {
	t.Parallel()

	if _, err := LoadWithOptions(LoadOptions{
		Path:         filepath.Join(t.TempDir(), "missing.json"),
		ExplicitPath: true,
	}); err == nil {
		t.Fatal("LoadWithOptions() error = nil, want missing explicit config error")
	}
}

func TestLoadWithOptionsExplicitEmptyPathFails(t *testing.T) {
	t.Parallel()

	if _, err := LoadWithOptions(LoadOptions{ExplicitPath: true, Env: map[string]string{}}); err == nil {
		t.Fatal("LoadWithOptions() error = nil, want empty explicit config path error")
	}
}

func TestLoadWithOptionsFailsWhenWorkingDirectoryCannotBeResolved(t *testing.T) {
	originalGetWorkingDir := getWorkingDir
	t.Cleanup(func() {
		getWorkingDir = originalGetWorkingDir
	})
	getWorkingDir = func() (string, error) {
		return "", errors.New("boom")
	}

	_, err := LoadWithOptions(LoadOptions{})
	if err == nil || !strings.Contains(err.Error(), "resolve working directory") {
		t.Fatalf("LoadWithOptions() error = %v, want working directory error", err)
	}
}

func TestLoadFileConfigBlankPathWithoutExplicitConfig(t *testing.T) {
	t.Parallel()

	raw, baseDir, err := loadFileConfig("", false)
	if err != nil {
		t.Fatalf("loadFileConfig() error = %v", err)
	}
	if raw != (fileConfig{}) {
		t.Fatalf("loadFileConfig() raw = %#v, want zero value", raw)
	}
	if baseDir != "" {
		t.Fatalf("loadFileConfig() baseDir = %q, want empty", baseDir)
	}
}

func TestLoadFileConfigMissingDefaultPathWithoutExplicitConfig(t *testing.T) {
	t.Parallel()

	raw, baseDir, err := loadFileConfig(filepath.Join(t.TempDir(), "missing.json"), false)
	if err != nil {
		t.Fatalf("loadFileConfig() error = %v", err)
	}
	if raw != (fileConfig{}) {
		t.Fatalf("loadFileConfig() raw = %#v, want zero value", raw)
	}
	if baseDir != "" {
		t.Fatalf("loadFileConfig() baseDir = %q, want empty", baseDir)
	}
}

func TestLookupEnvAndResolvePath(t *testing.T) {
	if got, ok := lookupEnv(map[string]string{"KEY": "value"}, "KEY"); !ok || got != "value" {
		t.Fatalf("lookupEnv(map, KEY) = %q, %t, want value, true", got, ok)
	}

	t.Setenv("DNS_UPDATE_TEST_LOOKUP", "value")
	if got, ok := lookupEnv(nil, "DNS_UPDATE_TEST_LOOKUP"); !ok || got != "value" {
		t.Fatalf("lookupEnv(nil, DNS_UPDATE_TEST_LOOKUP) = %q, %t, want value, true", got, ok)
	}

	if got, want := resolvePath("", "/tmp"), ""; got != want {
		t.Fatalf("resolvePath(empty) = %q, want %q", got, want)
	}
	if got, want := resolvePath("/tmp/abs", "/base"), "/tmp/abs"; got != want {
		t.Fatalf("resolvePath(abs) = %q, want %q", got, want)
	}
	if got, want := resolvePath("rel", "/base"), "/base/rel"; got != want {
		t.Fatalf("resolvePath(rel) = %q, want %q", got, want)
	}
}

func mustURLString(t *testing.T, raw string) string {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed.String()
}

type configView struct {
	RecordName        string
	RecordZone        string
	RecordTTLSeconds  uint32
	ProbeIPv4URL      string
	ProbeIPv6URL      string
	ProbeTimeout      time.Duration
	AllowInsecureHTTP bool
	ProviderType      string
	ProviderTimeout   time.Duration
	CloudflareZoneID  string
	CloudflareToken   string
	CloudflareBaseURL string
	CloudflareProxied bool
}

func viewConfig(cfg Config) configView {
	view := configView{
		RecordName:        cfg.Record.Name,
		RecordZone:        cfg.Record.Zone,
		RecordTTLSeconds:  cfg.Record.TTLSeconds,
		ProbeIPv4URL:      cfg.Probe.IPv4URL.String(),
		ProbeIPv6URL:      cfg.Probe.IPv6URL.String(),
		ProbeTimeout:      cfg.Probe.Timeout,
		AllowInsecureHTTP: cfg.Probe.AllowInsecureHTTP,
		ProviderType:      cfg.Provider.Type,
		ProviderTimeout:   cfg.Provider.Timeout,
	}
	if cfg.Provider.Cloudflare != nil {
		view.CloudflareZoneID = cfg.Provider.Cloudflare.ZoneID
		view.CloudflareToken = cfg.Provider.Cloudflare.APITokenFile
		view.CloudflareBaseURL = cfg.Provider.Cloudflare.BaseURL.String()
		view.CloudflareProxied = cfg.Provider.Cloudflare.Proxied
	}
	return view
}
