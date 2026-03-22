package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"
)

const maxResponseHeaderBytes = 8 << 10

// ErrRedirectNotAllowed is returned when an outbound request attempts to follow a redirect.
var ErrRedirectNotAllowed = errors.New("redirects are not allowed")

// UserAgent is the product user-agent used for outbound HTTP requests.
const UserAgent = "dns-update/1.3.8"

func dialContext(timeout time.Duration, forcedNetwork string) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		if forcedNetwork != "" {
			network = forcedNetwork
		}
		return dialer.DialContext(ctx, network, address)
	}
}

// New returns a conservative HTTP client for untrusted network responses.
func New(timeout time.Duration) *http.Client {
	return NewWithNetwork(timeout, "")
}

// NewWithNetwork returns a conservative HTTP client that optionally forces a specific dial network.
func NewWithNetwork(timeout time.Duration, forcedNetwork string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                  nil,
			ForceAttemptHTTP2:      true,
			MaxIdleConns:           4,
			MaxIdleConnsPerHost:    2,
			MaxConnsPerHost:        4,
			IdleConnTimeout:        30 * time.Second,
			ResponseHeaderTimeout:  timeout,
			ExpectContinueTimeout:  time.Second,
			TLSHandshakeTimeout:    timeout,
			MaxResponseHeaderBytes: maxResponseHeaderBytes,
			DisableCompression:     true,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
			DialContext: dialContext(timeout, forcedNetwork),
		},
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirectNotAllowed
		},
	}
}
