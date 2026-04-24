package config

import (
	"strings"
	"testing"

	"dns-update/internal/netutil"
)

func FuzzNormalizeFQDN(f *testing.F) {
	seeds := []string{
		"",
		"example.com",
		"Example.COM.",
		"*.example.com",
		"bad label",
		strings.Repeat("a", 64) + ".example.com",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		normalized, err := normalizeFQDN(input)
		if err != nil {
			return
		}

		if !strings.HasSuffix(normalized, ".") {
			t.Fatalf("normalizeFQDN(%q) = %q, want trailing dot", input, normalized)
		}
		if normalized != strings.ToLower(normalized) {
			t.Fatalf("normalizeFQDN(%q) = %q, want lowercase", input, normalized)
		}
		if strings.ContainsAny(normalized, " \t\r\n") {
			t.Fatalf("normalizeFQDN(%q) = %q, want no whitespace", input, normalized)
		}

		renormalized, err := normalizeFQDN(normalized)
		if err != nil {
			t.Fatalf("normalizeFQDN(%q) second pass error = %v", normalized, err)
		}
		if renormalized != normalized {
			t.Fatalf("normalizeFQDN(%q) second pass = %q, want %q", normalized, renormalized, normalized)
		}
	})
}

func FuzzParseProbeURL(f *testing.F) {
	f.Add("", false)
	f.Add(defaultProbeIPv4URL, false)
	f.Add("https://4.ip.omsab.net/path", false)
	f.Add("http://127.0.0.1/", true)
	f.Add("http://localhost/", true)
	f.Add("http://example.com/", true)
	f.Add("ftp://4.ip.omsab.net/", false)

	f.Fuzz(func(t *testing.T, raw string, allowHTTP bool) {
		if len(raw) > 512 {
			t.Skip()
		}
		value := strings.TrimSpace(raw)
		lowerValue := strings.ToLower(value)
		if value != "" &&
			!strings.HasPrefix(lowerValue, "http://") &&
			!strings.HasPrefix(lowerValue, "https://") &&
			!strings.HasPrefix(lowerValue, "ftp://") {
			t.Skip()
		}

		parsed, err := parseProbeURL(raw, defaultProbeIPv4URL, allowHTTP)
		if err != nil {
			t.Skip()
		}

		if parsed == nil {
			t.Fatalf("parseProbeURL(%q, %t) = nil, want URL", raw, allowHTTP)
		}
		if parsed.Host == "" {
			t.Fatalf("parseProbeURL(%q, %t) host is empty", raw, allowHTTP)
		}
		if parsed.User != nil {
			t.Fatalf("parseProbeURL(%q, %t) userinfo = %v, want nil", raw, allowHTTP, parsed.User)
		}
		if parsed.Fragment != "" {
			t.Fatalf("parseProbeURL(%q, %t) fragment = %q, want empty", raw, allowHTTP, parsed.Fragment)
		}
		if parsed.RawQuery != "" {
			t.Fatalf("parseProbeURL(%q, %t) raw query = %q, want empty", raw, allowHTTP, parsed.RawQuery)
		}

		switch parsed.Scheme {
		case "https":
		case "http":
			if !allowHTTP {
				t.Fatalf("parseProbeURL(%q, %t) returned http URL with allowHTTP disabled", raw, allowHTTP)
			}
			if !netutil.IsLoopbackHost(parsed.Hostname()) {
				t.Fatalf("parseProbeURL(%q, %t) returned non-loopback http host %q", raw, allowHTTP, parsed.Hostname())
			}
		default:
			t.Fatalf("parseProbeURL(%q, %t) scheme = %q, want http or https", raw, allowHTTP, parsed.Scheme)
		}
	})
}
