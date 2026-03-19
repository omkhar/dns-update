package netutil

import (
	"net"
	"strings"
)

// IsLoopbackHost reports whether host is a loopback address or the name "localhost".
func IsLoopbackHost(host string) bool {
	normalized := strings.TrimSpace(strings.ToLower(host))
	if normalized == "" {
		return false
	}
	if normalized == "localhost" {
		return true
	}
	address := net.ParseIP(normalized)
	return address != nil && address.IsLoopback()
}
