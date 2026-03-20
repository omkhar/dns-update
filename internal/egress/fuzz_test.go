package egress

import (
	"strings"
	"testing"
)

func FuzzParseResponse(f *testing.F) {
	f.Add("ip=203.0.113.10\n", false)
	f.Add("ip=2001:db8::1\n", true)
	f.Add("ip=none\n", false)
	f.Add("bad", false)

	f.Fuzz(func(t *testing.T, body string, wantIPv6 bool) {
		family := IPv4
		if wantIPv6 {
			family = IPv6
		}

		address, err := parseResponse([]byte(body), family)
		if err != nil {
			return
		}

		if address == nil {
			value := strings.TrimSpace(body)
			if !strings.HasPrefix(value, "ip=") || strings.TrimSpace(strings.TrimPrefix(value, "ip=")) != "none" {
				t.Fatalf("parseResponse(%q, %d) = nil, want address or explicit none", body, family)
			}
			return
		}

		if !address.IsValid() {
			t.Fatalf("parseResponse(%q, %d) returned invalid address %v", body, family, address)
		}
		if family == IPv4 && !address.Is4() {
			t.Fatalf("parseResponse(%q, %d) returned non-IPv4 address %v", body, family, address)
		}
		if family == IPv6 && (!address.Is6() || address.Is4In6()) {
			t.Fatalf("parseResponse(%q, %d) returned non-IPv6 address %v", body, family, address)
		}
	})
}
