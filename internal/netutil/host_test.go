package netutil_test

import (
	"testing"

	"dns-update/internal/netutil"
)

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		{"", false},
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"1.2.3.4", false},
		{"not-an-ip", false},
	}

	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			t.Parallel()
			if got := netutil.IsLoopbackHost(test.host); got != test.want {
				t.Fatalf("IsLoopbackHost(%q) = %v, want %v", test.host, got, test.want)
			}
		})
	}
}
