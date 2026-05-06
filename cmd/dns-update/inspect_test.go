package main

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"dns-update/internal/config"
	"dns-update/internal/provider"
)

func TestHandleIntrospection(t *testing.T) {
	t.Parallel()

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	ipv4URL, err := url.Parse("https://4.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	ipv6URL, err := url.Parse("https://6.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	cfg := config.Config{
		Record: config.RecordConfig{
			Name:       "host.example.com.",
			Zone:       "example.com.",
			TTLSeconds: 300,
		},
		Probe: config.ProbeConfig{
			IPv4URL:             ipv4URL,
			IPv6URL:             ipv6URL,
			Timeout:             5 * time.Second,
			AllowPartialFailure: true,
		},
		Provider: config.ProviderConfig{
			Type:    "cloudflare",
			Timeout: 6 * time.Second,
			Cloudflare: &config.CloudflareConfig{
				ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
				APITokenFile: "/tmp/cloudflare.token",
				BaseURL:      baseURL,
				Proxied:      true,
			},
		},
	}

	t.Run("print effective config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		handled, err := handleIntrospection(&stdout, cfg, runtimeOptions{
			configPath:        "/tmp/config.json",
			deleteSelection:   provider.RecordSelectionBoth,
			dryRun:            true,
			forcePush:         true,
			introspectionMode: introspectionModePrintEffectiveConfig,
			verbose:           true,
			timeout:           3 * time.Second,
		})
		if err != nil {
			t.Fatalf("handleIntrospection() error = %v", err)
		}
		if !handled {
			t.Fatal("handleIntrospection() handled = false, want true")
		}
		if got := stdout.String(); got == "" {
			t.Fatal("handleIntrospection() output = empty, want JSON")
		}
		if got := stdout.String(); !strings.Contains(got, `"force_push": true`) {
			t.Fatalf("handleIntrospection() output = %q, want force_push field", got)
		}
		if got := stdout.String(); !strings.Contains(got, `"delete": "both"`) {
			t.Fatalf("handleIntrospection() output = %q, want delete field", got)
		}
		if got := stdout.String(); !strings.Contains(got, `"allow_partial_failure": true`) {
			t.Fatalf("handleIntrospection() output = %q, want allow_partial_failure field", got)
		}
	})

	t.Run("print effective config uses resolved source path", func(t *testing.T) {
		t.Parallel()

		cfgWithSource := cfg
		cfgWithSource.SourcePath = "/etc/dns-update/config.json"

		var stdout bytes.Buffer
		handled, err := handleIntrospection(&stdout, cfgWithSource, runtimeOptions{
			configPath:        "config.json",
			introspectionMode: introspectionModePrintEffectiveConfig,
		})
		if err != nil {
			t.Fatalf("handleIntrospection() error = %v", err)
		}
		if !handled {
			t.Fatal("handleIntrospection() handled = false, want true")
		}
		if got := stdout.String(); !strings.Contains(got, `"config_path": "/etc/dns-update/config.json"`) {
			t.Fatalf("handleIntrospection() output = %q, want resolved config path", got)
		}
	})

	t.Run("validate config", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		handled, err := handleIntrospection(&stdout, cfg, runtimeOptions{
			introspectionMode: introspectionModeValidateConfig,
		})
		if err != nil {
			t.Fatalf("handleIntrospection() error = %v", err)
		}
		if !handled {
			t.Fatal("handleIntrospection() handled = false, want true")
		}
		if got, want := stdout.String(), "config is valid\n"; got != want {
			t.Fatalf("handleIntrospection() output = %q, want %q", got, want)
		}
	})

	t.Run("not requested", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		handled, err := handleIntrospection(&stdout, cfg, runtimeOptions{})
		if err != nil {
			t.Fatalf("handleIntrospection() error = %v, want nil", err)
		}
		if handled {
			t.Fatal("handleIntrospection() handled = true, want false")
		}
		if stdout.Len() != 0 {
			t.Fatalf("handleIntrospection() output = %q, want empty", stdout.String())
		}
	})
}

func TestPrintEffectiveConfigWriterError(t *testing.T) {
	t.Parallel()

	ipv4URL, err := url.Parse("https://4.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	ipv6URL, err := url.Parse("https://6.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	errWriter := failingWriter{err: errors.New("boom")}
	err = printEffectiveConfig(errWriter, config.Config{
		Record: config.RecordConfig{
			Name:       "host.example.com.",
			Zone:       "example.com.",
			TTLSeconds: 300,
		},
		Probe: config.ProbeConfig{
			IPv4URL: ipv4URL,
			IPv6URL: ipv6URL,
			Timeout: time.Second,
		},
		Provider: config.ProviderConfig{
			Type:    "cloudflare",
			Timeout: time.Second,
		},
	}, runtimeOptions{})
	if err == nil {
		t.Fatal("printEffectiveConfig() error = nil, want non-nil")
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
