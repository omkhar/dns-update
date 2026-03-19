package app

import (
	"net/netip"
	"net/url"
	"testing"
)

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", value, err)
	}
	return parsed
}

func mustAddr(t *testing.T, value string) *netip.Addr {
	t.Helper()

	addr, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("netip.ParseAddr(%q) error = %v", value, err)
	}
	return &addr
}
