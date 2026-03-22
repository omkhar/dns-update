package egress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"dns-update/internal/httpclient"
	"dns-update/internal/retry"
)

const (
	maxResponseBytes = 64
)

// Family identifies the expected IP address family of a probe response.
type Family int

const (
	// IPv4 selects an IPv4-only probe and response.
	IPv4 Family = 4
	// IPv6 selects an IPv6-only probe and response.
	IPv6 Family = 6
)

// Prober fetches egress IP values from remote endpoints.
type Prober struct {
	ipv4Client *http.Client
	ipv6Client *http.Client
	newRequest func(context.Context, string, string, io.Reader) (*http.Request, error)
}

// NewProber returns a probe client with conservative network settings.
func NewProber(timeout time.Duration) *Prober {
	return &Prober{
		ipv4Client: httpclient.NewWithNetwork(timeout, "tcp4"),
		ipv6Client: httpclient.NewWithNetwork(timeout, "tcp6"),
		newRequest: http.NewRequestWithContext,
	}
}

// Lookup fetches and validates the egress IP for the requested family.
func (p *Prober) Lookup(ctx context.Context, endpoint *url.URL, family Family) (*netip.Addr, error) {
	client, err := p.clientForFamily(family)
	if err != nil {
		return nil, err
	}

	request, err := p.requestBuilder()(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("User-Agent", httpclient.UserAgent)
	request.Header.Set("Accept", "text/plain")

	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, httpclient.ErrRedirectNotAllowed) || context.Cause(ctx) != nil {
			return nil, fmt.Errorf("perform request: %w", err)
		}
		return nil, retry.Mark(fmt.Errorf("perform request: %w", err), 0)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
		if retry.ShouldRetryHTTPStatus(response.StatusCode) {
			retryAfter, _ := retry.ParseRetryAfter(response.Header.Get("Retry-After"), time.Now())
			return nil, retry.Mark(err, retryAfter)
		}
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	address, err := parseResponse(body, family)
	if err != nil {
		return nil, err
	}
	return address, nil
}

func (p *Prober) clientForFamily(family Family) (*http.Client, error) {
	switch family {
	case IPv4:
		if p.ipv4Client == nil {
			return nil, errors.New("IPv4 probe client is not configured")
		}
		return p.ipv4Client, nil
	case IPv6:
		if p.ipv6Client == nil {
			return nil, errors.New("IPv6 probe client is not configured")
		}
		return p.ipv6Client, nil
	default:
		return nil, fmt.Errorf("unsupported IP family %d", family)
	}
}

func (p *Prober) requestBuilder() func(context.Context, string, string, io.Reader) (*http.Request, error) {
	if p.newRequest != nil {
		return p.newRequest
	}
	return http.NewRequestWithContext
}

func parseResponse(body []byte, family Family) (*netip.Addr, error) {
	value := strings.TrimSpace(string(body))
	if !strings.HasPrefix(value, "ip=") {
		return nil, errors.New("response must begin with ip=")
	}

	rawAddress := strings.TrimSpace(strings.TrimPrefix(value, "ip="))
	if rawAddress == "" {
		return nil, errors.New("response is missing an IP value")
	}
	if rawAddress == "none" {
		return nil, nil
	}

	address, err := netip.ParseAddr(rawAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid IP address %q: %w", rawAddress, err)
	}

	switch family {
	case IPv4:
		if !address.Is4() {
			return nil, fmt.Errorf("expected an IPv4 address, got %q", rawAddress)
		}
	case IPv6:
		if !address.Is6() || address.Is4In6() {
			return nil, fmt.Errorf("expected an IPv6 address, got %q", rawAddress)
		}
	default:
		return nil, fmt.Errorf("unsupported IP family %d", family)
	}

	return &address, nil
}
