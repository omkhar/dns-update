package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	cloudflareapi "github.com/cloudflare/cloudflare-go/v6"
	cfdns "github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"

	"dns-update/internal/httpclient"
	"dns-update/internal/netutil"
	"dns-update/internal/provider"
)

const recordsPerPage = 100

// Client implements the provider.Provider interface for Cloudflare.
type Client struct {
	zoneID  string
	records *cfdns.RecordService
}

// Config defines the Cloudflare client settings.
type Config struct {
	ZoneID   string
	APIToken string
	BaseURL  *url.URL
	Timeout  time.Duration
	Proxied  bool
}

// New returns a Cloudflare-backed provider.
func New(config Config) (*Client, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	sdk := cloudflareapi.NewClient(
		option.WithAPIToken(config.APIToken),
		option.WithBaseURL(config.BaseURL.String()),
		option.WithHTTPClient(httpclient.New(config.Timeout)),
		option.WithHeader("User-Agent", httpclient.UserAgent),
		option.WithMaxRetries(0),
	)

	return &Client{
		zoneID:  config.ZoneID,
		records: sdk.DNS.Records,
	}, nil
}

func (c Config) validate() error {
	switch {
	case c.ZoneID == "":
		return errors.New("zone ID is required")
	case c.APIToken == "":
		return errors.New("API token is required")
	case c.BaseURL == nil:
		return errors.New("base URL is required")
	case c.BaseURL.Host == "":
		return errors.New("base URL host is required")
	case c.BaseURL.User != nil:
		return errors.New("base URL userinfo is not allowed")
	case c.BaseURL.RawQuery != "":
		return errors.New("base URL query parameters are not allowed")
	case c.BaseURL.Fragment != "":
		return errors.New("base URL fragments are not allowed")
	case !isAllowedBaseURL(c.BaseURL):
		return errors.New("base URL host must be api.cloudflare.com or loopback/localhost")
	default:
		return nil
	}
}

func isAllowedBaseURL(baseURL *url.URL) bool {
	host := strings.TrimSpace(strings.ToLower(baseURL.Hostname()))
	if host == "api.cloudflare.com" && baseURL.Scheme == "https" {
		return true
	}
	return netutil.IsLoopbackHost(host)
}

// ReadState returns the current Cloudflare DNS records for name.
func (c *Client) ReadState(ctx context.Context, name string) (provider.State, error) {
	records, err := c.listRecords(ctx, normalizeAPIName(name))
	if err != nil {
		return provider.State{}, normalizeRequestError(ctx, "list Cloudflare DNS records", err)
	}

	normalizedName := normalizeAPIName(name)
	state := provider.State{
		Name:    name,
		Records: make([]provider.Record, 0, len(records)),
	}
	for _, record := range records {
		if normalizeAPIName(record.Name) != normalizedName {
			continue
		}
		state.Records = append(state.Records, provider.Record{
			ID:         record.ID,
			Name:       normalizeProviderName(record.Name),
			Type:       provider.RecordType(record.Type),
			Content:    record.Content,
			TTLSeconds: uint32(record.TTL),
			Options:    recordOptions(record.Proxied),
		})
	}
	return state, nil
}

// Apply executes plan through the Cloudflare batch DNS records endpoint.
func (c *Client) Apply(ctx context.Context, plan provider.Plan) error {
	if plan.IsNoop() {
		return nil
	}

	params, err := buildBatchParams(c.zoneID, plan)
	if err != nil {
		return fmt.Errorf("build Cloudflare batch request: %w", err)
	}

	if _, err := c.records.Batch(ctx, params); err != nil {
		return normalizeRequestError(ctx, "apply Cloudflare batch update", err)
	}
	return nil
}

func (c *Client) listRecords(ctx context.Context, name string) ([]cfdns.RecordResponse, error) {
	pager := c.records.ListAutoPaging(ctx, cfdns.RecordListParams{
		ZoneID: cloudflareapi.F(c.zoneID),
		Match:  cloudflareapi.F(cfdns.RecordListParamsMatchAll),
		Name: cloudflareapi.F(cfdns.RecordListParamsName{
			Exact: cloudflareapi.F(name),
		}),
		PerPage: cloudflareapi.Float(recordsPerPage),
	})

	records := make([]cfdns.RecordResponse, 0, 4)
	for pager.Next() {
		records = append(records, pager.Current())
	}
	if err := pager.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
