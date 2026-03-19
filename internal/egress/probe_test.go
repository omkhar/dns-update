package egress

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"dns-update/internal/httpclient"
	"dns-update/internal/retry"
)

func TestNewProber(t *testing.T) {
	t.Parallel()

	prober := NewProber(2 * time.Second)
	if prober == nil || prober.ipv4Client == nil || prober.ipv6Client == nil {
		t.Fatalf("NewProber() = %#v, want initialized clients", prober)
	}
	if prober.ipv4Client.Transport == nil || prober.ipv6Client.Transport == nil {
		t.Fatalf("NewProber() = %#v, want initialized transports", prober)
	}
}

func TestLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if got, want := r.Header.Get("User-Agent"), httpclient.UserAgent; got != want {
				t.Fatalf("User-Agent = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Accept"), "text/plain"; got != want {
				t.Fatalf("Accept = %q, want %q", got, want)
			}
			_, _ = io.WriteString(w, "ip=198.51.100.10")
		case "/status":
			http.Error(w, "nope", http.StatusBadGateway)
		case "/rate-limited":
			w.Header().Set("Retry-After", "2")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case "/not-found":
			http.NotFound(w, r)
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prober := NewProber(time.Second)

	tests := []struct {
		name      string
		endpoint  string
		family    Family
		want      string
		wantError string
		wantRetry bool
		wantDelay time.Duration
	}{
		{
			name:     "success",
			endpoint: server.URL + "/ok",
			family:   IPv4,
			want:     "198.51.100.10",
		},
		{
			name:      "status error",
			endpoint:  server.URL + "/status",
			family:    IPv4,
			wantError: "unexpected HTTP status",
			wantRetry: true,
		},
		{
			name:      "rate limited",
			endpoint:  server.URL + "/rate-limited",
			family:    IPv4,
			wantError: "unexpected HTTP status 429",
			wantRetry: true,
			wantDelay: 2 * time.Second,
		},
		{
			name:      "redirect blocked",
			endpoint:  server.URL + "/redirect",
			family:    IPv4,
			wantError: "redirects are not allowed",
		},
		{
			name:      "non-retryable status",
			endpoint:  server.URL + "/not-found",
			family:    IPv4,
			wantError: "unexpected HTTP status 404",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			endpoint, err := url.Parse(test.endpoint)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}

			address, err := prober.Lookup(context.Background(), endpoint, test.family)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Lookup() error = %v, want substring %q", err, test.wantError)
				}
				delay, ok := retry.After(err)
				if ok != test.wantRetry {
					t.Fatalf("After() ok = %t, want %t", ok, test.wantRetry)
				}
				if delay != test.wantDelay {
					t.Fatalf("After() delay = %v, want %v", delay, test.wantDelay)
				}
				return
			}
			if err != nil {
				t.Fatalf("Lookup() error = %v", err)
			}
			if address == nil || address.String() != test.want {
				t.Fatalf("Lookup() address = %v, want %q", address, test.want)
			}
		})
	}
}

func TestLookupErrors(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  *url.URL
		prober    *Prober
		prepare   func()
		wantError string
	}{
		{
			name:     "request build error",
			endpoint: mustURL(t, "http://example.com"),
			prober: &Prober{
				ipv4Client: httpclient.NewWithNetwork(time.Second, "tcp4"),
				ipv6Client: httpclient.NewWithNetwork(time.Second, "tcp6"),
				newRequest: func(context.Context, string, string, io.Reader) (*http.Request, error) {
					return nil, errors.New("boom")
				},
			},
			wantError: "build request",
		},
		{
			name:     "transport error",
			endpoint: mustURL(t, "http://example.com"),
			prober: &Prober{
				ipv4Client: &http.Client{
					Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return nil, errors.New("boom")
					}),
				},
			},
			wantError: "perform request",
			prepare:   nil,
		},
		{
			name:     "body read error",
			endpoint: mustURL(t, "http://example.com"),
			prober: &Prober{
				ipv4Client: &http.Client{
					Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(errorReader{}),
						}, nil
					}),
				},
			},
			wantError: "read response body",
		},
		{
			name:     "parse error",
			endpoint: mustURL(t, "http://example.com"),
			prober: &Prober{
				ipv4Client: &http.Client{
					Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader("bad")),
						}, nil
					}),
				},
			},
			wantError: "response must begin with ip=",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if test.prepare != nil {
				test.prepare()
			}

			_, err := test.prober.Lookup(context.Background(), test.endpoint, IPv4)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Lookup() error = %v, want substring %q", err, test.wantError)
			}
			_, ok := retry.After(err)
			if test.name == "transport error" && !ok {
				t.Fatal("After(transport error) ok = false, want true")
			}
			if test.name != "transport error" && ok {
				t.Fatalf("After(%s) ok = true, want false", test.name)
			}
		})
	}
}

func TestLookupCanceledContextIsNotRetryable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("boom"))

	prober := &Prober{
		ipv4Client: &http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("transport should not be used")
			}),
		},
	}

	_, err := prober.Lookup(ctx, mustURL(t, "http://example.com"), IPv4)
	if err == nil {
		t.Fatal("Lookup() error = nil, want context error")
	}
	if _, ok := retry.After(err); ok {
		t.Fatal("After(canceled context) ok = true, want false")
	}
}

func TestParseResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		family    Family
		want      string
		wantError bool
	}{
		{
			name:   "ipv4 success",
			body:   "ip=203.0.113.10\n",
			family: IPv4,
			want:   "203.0.113.10",
		},
		{
			name:   "ipv6 success",
			body:   "ip=2001:db8::10",
			family: IPv6,
			want:   "2001:db8::10",
		},
		{
			name:   "none value",
			body:   "ip=none",
			family: IPv4,
		},
		{
			name:      "missing value",
			body:      "ip= ",
			family:    IPv4,
			wantError: true,
		},
		{
			name:      "invalid address",
			body:      "ip=not-an-ip",
			family:    IPv4,
			wantError: true,
		},
		{
			name:      "wrong family",
			body:      "ip=203.0.113.10",
			family:    IPv6,
			wantError: true,
		},
		{
			name:      "ipv6 given to ipv4",
			body:      "ip=2001:db8::10",
			family:    IPv4,
			wantError: true,
		},
		{
			name:      "ipv4-mapped-v6 rejected",
			body:      "ip=::ffff:203.0.113.10",
			family:    IPv6,
			wantError: true,
		},
		{
			name:      "unsupported family",
			body:      "ip=203.0.113.10",
			family:    Family(0),
			wantError: true,
		},
		{
			name:      "bad format",
			body:      "203.0.113.10",
			family:    IPv4,
			wantError: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseResponse([]byte(test.body), test.family)
			if test.wantError {
				if err == nil {
					t.Fatal("parseResponse() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseResponse() error = %v", err)
			}
			if test.want == "" {
				if got != nil {
					t.Fatalf("parseResponse() got = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("parseResponse() got = nil, want address")
			}
			if got.String() != test.want {
				t.Fatalf("parseResponse() got = %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestLookupUsesFamilySpecificClient(t *testing.T) {
	t.Parallel()

	var ipv4Requests int
	var ipv6Requests int
	prober := &Prober{
		ipv4Client: &http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				ipv4Requests++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ip=198.51.100.10")),
				}, nil
			}),
		},
		ipv6Client: &http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				ipv6Requests++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ip=2001:db8::10")),
				}, nil
			}),
		},
	}

	ipv6Address, err := prober.Lookup(context.Background(), mustURL(t, "http://example.com"), IPv6)
	if err != nil {
		t.Fatalf("Lookup(IPv6) error = %v", err)
	}
	if ipv6Address == nil || ipv6Address.String() != "2001:db8::10" {
		t.Fatalf("Lookup(IPv6) = %v, want 2001:db8::10", ipv6Address)
	}
	if ipv4Requests != 0 || ipv6Requests != 1 {
		t.Fatalf("requests after IPv6 lookup = (%d, %d), want (0, 1)", ipv4Requests, ipv6Requests)
	}

	ipv4Address, err := prober.Lookup(context.Background(), mustURL(t, "http://example.com"), IPv4)
	if err != nil {
		t.Fatalf("Lookup(IPv4) error = %v", err)
	}
	if ipv4Address == nil || ipv4Address.String() != "198.51.100.10" {
		t.Fatalf("Lookup(IPv4) = %v, want 198.51.100.10", ipv4Address)
	}
	if ipv4Requests != 1 || ipv6Requests != 1 {
		t.Fatalf("requests after IPv4 lookup = (%d, %d), want (1, 1)", ipv4Requests, ipv6Requests)
	}
}

func TestLookupFamilySelectionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prober    *Prober
		family    Family
		wantError string
	}{
		{
			name:      "missing ipv4 client",
			prober:    &Prober{},
			family:    IPv4,
			wantError: "IPv4 probe client is not configured",
		},
		{
			name:      "missing ipv6 client",
			prober:    &Prober{},
			family:    IPv6,
			wantError: "IPv6 probe client is not configured",
		},
		{
			name:      "unsupported family",
			prober:    NewProber(time.Second),
			family:    Family(0),
			wantError: "unsupported IP family",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := test.prober.Lookup(context.Background(), mustURL(t, "http://example.com"), test.family)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Lookup() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", value, err)
	}
	return parsed
}
