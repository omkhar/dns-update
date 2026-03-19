package app

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"dns-update/internal/config"
	providerpkg "dns-update/internal/provider"
)

func TestNewProviderSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	dnsProvider, desiredOptions, err := newProvider(config.ProviderConfig{
		Type:    "cloudflare",
		Timeout: time.Second,
		Cloudflare: &config.CloudflareConfig{
			ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
			APITokenFile: tokenPath,
			BaseURL:      baseURL,
			Proxied:      true,
		},
	})
	if err != nil {
		t.Fatalf("newProvider() error = %v", err)
	}
	if dnsProvider == nil {
		t.Fatal("newProvider() provider = nil, want non-nil")
	}
	if diff := cmp.Diff(providerpkg.RecordOptions{Proxy: boolPtr(true)}, desiredOptions); diff != "" {
		t.Fatalf("options mismatch (-want +got):\n%s", diff)
	}
}

func TestNewProviderErrors(t *testing.T) {
	t.Parallel()

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	tests := []config.ProviderConfig{
		{Type: "unsupported"},
		{Type: "cloudflare"},
		{
			Type:    "cloudflare",
			Timeout: time.Second,
			Cloudflare: &config.CloudflareConfig{
				ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
				APITokenFile: "/no/such/file",
				BaseURL:      baseURL,
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Type, func(t *testing.T) {
			t.Parallel()
			if _, _, err := newProvider(test); err == nil {
				t.Fatal("newProvider() error = nil, want non-nil")
			}
		})
	}
}

func TestNewProviderRejectsInvalidTokenFileContents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.token")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	multiPath := filepath.Join(dir, "multi.token")
	if err := os.WriteFile(multiPath, []byte("token one"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	for _, tokenPath := range []string{emptyPath, multiPath} {
		tokenPath := tokenPath
		t.Run(filepath.Base(tokenPath), func(t *testing.T) {
			t.Parallel()
			if _, _, err := newProvider(config.ProviderConfig{
				Type:    "cloudflare",
				Timeout: time.Second,
				Cloudflare: &config.CloudflareConfig{
					ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
					APITokenFile: tokenPath,
					BaseURL:      baseURL,
				},
			}); err == nil {
				t.Fatal("newProvider() error = nil, want non-nil")
			}
		})
	}
}

func TestNewProviderRejectsSymlinkTokenFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.token")
	if err := os.WriteFile(targetPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	symlinkPath := filepath.Join(dir, "cloudflare.token")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if _, _, err := newProvider(config.ProviderConfig{
		Type:    "cloudflare",
		Timeout: time.Second,
		Cloudflare: &config.CloudflareConfig{
			ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
			APITokenFile: symlinkPath,
			BaseURL:      baseURL,
		},
	}); err == nil {
		t.Fatal("newProvider() error = nil, want symlink rejection")
	}
}

func TestNewProviderRejectsInvalidCloudflareConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, _, err := newProvider(config.ProviderConfig{
		Type:    "cloudflare",
		Timeout: time.Second,
		Cloudflare: &config.CloudflareConfig{
			ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
			APITokenFile: tokenPath,
		},
	}); err == nil {
		t.Fatal("newProvider() error = nil, want invalid Cloudflare config error")
	}
}
