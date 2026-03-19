package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	cloudflareapi "github.com/cloudflare/cloudflare-go/v6"
	"github.com/google/go-cmp/cmp"

	"dns-update/internal/httpclient"
	"dns-update/internal/provider"
	"dns-update/internal/retry"
)

func TestReadState(t *testing.T) {
	t.Parallel()

	t.Run("filters exact name and preserves record semantics", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if page := r.URL.Query().Get("page"); page == "2" {
				writeJSON(t, w, map[string]any{
					"success": true,
					"result":  []map[string]any{},
				})
				return
			}
			if got, want := r.Header.Get("Authorization"), "Bearer secret"; got != want {
				t.Fatalf("Authorization header = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("User-Agent"), httpclient.UserAgent; got != want {
				t.Fatalf("User-Agent header = %q, want %q", got, want)
			}
			if got, want := r.URL.Path, "/client/v4/zones/023e105f4ecef8ad9ca31a8372d0c353/dns_records"; got != want {
				t.Fatalf("path = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("name.exact"), "host.example.com"; got != want {
				t.Fatalf("query name.exact = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("match"), "all"; got != want {
				t.Fatalf("query match = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("per_page"), "100"; got != want {
				t.Fatalf("query per_page = %q, want %q", got, want)
			}
			writeJSON(t, w, map[string]any{
				"success": true,
				"result": []map[string]any{
					{
						"id":      "a1",
						"name":    "host.example.com",
						"type":    "A",
						"content": "198.51.100.10",
						"ttl":     300,
						"proxied": false,
					},
					{
						"id":      "a2",
						"name":    "other.example.com",
						"type":    "A",
						"content": "198.51.100.11",
						"ttl":     300,
						"proxied": false,
					},
				},
				"result_info": map[string]any{
					"page":        1,
					"per_page":    100,
					"count":       2,
					"total_count": 2,
					"total_pages": 1,
				},
			})
		}))
		defer server.Close()

		client := testClient(t, server)
		state, err := client.ReadState(context.Background(), "host.example.com.")
		if err != nil {
			t.Fatalf("ReadState() error = %v", err)
		}

		want := provider.State{
			Name: "host.example.com.",
			Records: []provider.Record{{
				ID:         "a1",
				Name:       "host.example.com.",
				Type:       provider.RecordTypeA,
				Content:    "198.51.100.10",
				TTLSeconds: 300,
				Options:    provider.RecordOptions{Proxy: boolPtr(false)},
			}},
		}
		if diff := cmp.Diff(want, state); diff != "" {
			t.Fatalf("ReadState() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("classifies retryable errors", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(t, w, map[string]any{
				"success": false,
				"errors":  []map[string]any{{"code": 1000, "message": "boom"}},
			})
		}))
		defer server.Close()

		client := testClient(t, server)
		_, err := client.ReadState(context.Background(), "host.example.com.")
		if err == nil {
			t.Fatal("ReadState() error = nil, want non-nil")
		}
		delay, ok := retry.After(err)
		if !ok {
			t.Fatalf("After() ok = false, want true: %v", err)
		}
		if got, want := delay, 3*time.Second; got != want {
			t.Fatalf("After() = %v, want %v", got, want)
		}
	})

	t.Run("rejects redirects", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/client/v4/redirected", http.StatusFound)
		}))
		defer server.Close()

		client := testClient(t, server)
		_, err := client.ReadState(context.Background(), "host.example.com.")
		if err == nil {
			t.Fatal("ReadState() error = nil, want redirect error")
		}
		if _, ok := retry.After(err); ok {
			t.Fatalf("After() ok = true, want false: %v", err)
		}
	})

	t.Run("honors cancelled contexts", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("server should not be called for a cancelled context")
		}))
		defer server.Close()

		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errors.New("boom"))

		client := testClient(t, server)
		_, err := client.ReadState(ctx, "host.example.com.")
		if err == nil {
			t.Fatal("ReadState() error = nil, want context error")
		}
		if _, ok := retry.After(err); ok {
			t.Fatalf("After() ok = true, want false: %v", err)
		}
	})
}

func TestApply(t *testing.T) {
	t.Parallel()

	t.Run("sends the expected batch request", func(t *testing.T) {
		t.Parallel()

		var requestBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got, want := r.URL.Path, "/client/v4/zones/023e105f4ecef8ad9ca31a8372d0c353/dns_records/batch"; got != want {
				t.Fatalf("path = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("User-Agent"), httpclient.UserAgent; got != want {
				t.Fatalf("User-Agent header = %q, want %q", got, want)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if err := json.Unmarshal(body, &requestBody); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			writeJSON(t, w, map[string]any{
				"success": true,
				"result":  map[string]any{},
			})
		}))
		defer server.Close()

		client := testClient(t, server)
		plan := provider.Plan{
			Operations: []provider.Operation{
				{
					Kind: provider.OperationDelete,
					Current: provider.Record{
						ID:      "old",
						Name:    "host.example.com.",
						Type:    provider.RecordTypeA,
						Content: "198.51.100.10",
					},
				},
				{
					Kind: provider.OperationUpdate,
					Current: provider.Record{
						ID:      "existing",
						Name:    "host.example.com.",
						Type:    provider.RecordTypeA,
						Content: "198.51.100.10",
					},
					Desired: provider.Record{
						Name:       "host.example.com.",
						Type:       provider.RecordTypeA,
						Content:    "198.51.100.20",
						TTLSeconds: 300,
						Options:    provider.RecordOptions{Proxy: boolPtr(false)},
					},
				},
				{
					Kind: provider.OperationCreate,
					Desired: provider.Record{
						Name:       "host.example.com.",
						Type:       provider.RecordTypeAAAA,
						Content:    "2001:db8::10",
						TTLSeconds: 300,
						Options:    provider.RecordOptions{Proxy: boolPtr(false)},
					},
				},
			},
		}
		if err := client.Apply(context.Background(), plan); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		if got, want := sliceLen(requestBody["deletes"]), 1; got != want {
			t.Fatalf("deletes length = %d, want %d", got, want)
		}
		if got := mapAt(requestBody, "deletes", 0)["id"]; got != "old" {
			t.Fatalf("deletes[0].id = %v, want old", got)
		}

		patch := mapAt(requestBody, "patches", 0)
		if got := patch["id"]; got != "existing" {
			t.Fatalf("patches[0].id = %v, want existing", got)
		}
		if got := patch["name"]; got != "host.example.com" {
			t.Fatalf("patches[0].name = %v, want host.example.com", got)
		}
		if got := patch["content"]; got != "198.51.100.20" {
			t.Fatalf("patches[0].content = %v, want 198.51.100.20", got)
		}
		if got := patch["ttl"]; got != float64(300) {
			t.Fatalf("patches[0].ttl = %v, want 300", got)
		}
		if got := patch["type"]; got != "A" {
			t.Fatalf("patches[0].type = %v, want A", got)
		}
		if got := patch["proxied"]; got != false {
			t.Fatalf("patches[0].proxied = %v, want false", got)
		}

		post := mapAt(requestBody, "posts", 0)
		if got := post["name"]; got != "host.example.com" {
			t.Fatalf("posts[0].name = %v, want host.example.com", got)
		}
		if got := post["content"]; got != "2001:db8::10" {
			t.Fatalf("posts[0].content = %v, want 2001:db8::10", got)
		}
		if got := post["ttl"]; got != float64(300) {
			t.Fatalf("posts[0].ttl = %v, want 300", got)
		}
		if got := post["type"]; got != "AAAA" {
			t.Fatalf("posts[0].type = %v, want AAAA", got)
		}
		if got := post["proxied"]; got != false {
			t.Fatalf("posts[0].proxied = %v, want false", got)
		}
	})

	t.Run("does not retry SDK requests", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(t, w, map[string]any{
				"success": false,
				"errors":  []map[string]any{{"message": "boom"}},
			})
		}))
		defer server.Close()

		client := testClient(t, server)
		err := client.Apply(context.Background(), provider.Plan{
			Operations: []provider.Operation{{
				Kind: provider.OperationDelete,
				Current: provider.Record{
					ID:      "old",
					Name:    "host.example.com.",
					Type:    provider.RecordTypeA,
					Content: "198.51.100.10",
				},
			}},
		})
		if err == nil {
			t.Fatal("Apply() error = nil, want error")
		}
		if got, want := calls.Load(), int32(1); got != want {
			t.Fatalf("request count = %d, want %d", got, want)
		}
	})

	t.Run("rejects unsupported operations", func(t *testing.T) {
		t.Parallel()

		client := mustClient(t, Config{
			ZoneID:   "zone",
			APIToken: "token",
			BaseURL:  mustURL(t, "https://api.cloudflare.com/client/v4/"),
			Timeout:  time.Second,
		})
		err := client.Apply(context.Background(), provider.Plan{
			Operations: []provider.Operation{{Kind: provider.OperationKind("bad")}},
		})
		if err == nil {
			t.Fatal("Apply() error = nil, want unsupported operation error")
		}
	})

	t.Run("ignores noop plans", func(t *testing.T) {
		t.Parallel()

		client := mustClient(t, Config{
			ZoneID:   "zone",
			APIToken: "token",
			BaseURL:  mustURL(t, "https://api.cloudflare.com/client/v4/"),
			Timeout:  time.Second,
		})
		if err := client.Apply(context.Background(), provider.Plan{}); err != nil {
			t.Fatalf("Apply(noop) error = %v", err)
		}
	})
}

func TestNewErrors(t *testing.T) {
	t.Parallel()

	baseURL := mustURL(t, "https://api.cloudflare.com/client/v4/")

	tests := []struct {
		name   string
		config Config
	}{
		{name: "missing zone", config: Config{APIToken: "token", BaseURL: baseURL, Timeout: time.Second}},
		{name: "missing token", config: Config{ZoneID: "zone", BaseURL: baseURL, Timeout: time.Second}},
		{name: "missing base URL", config: Config{ZoneID: "zone", APIToken: "token", Timeout: time.Second}},
		{name: "missing base URL host", config: Config{ZoneID: "zone", APIToken: "token", BaseURL: mustURL(t, "https:///client/v4/"), Timeout: time.Second}},
		{name: "base URL userinfo", config: Config{ZoneID: "zone", APIToken: "token", BaseURL: mustURL(t, "https://user:pass@api.cloudflare.com/client/v4/"), Timeout: time.Second}},
		{name: "base URL query", config: Config{ZoneID: "zone", APIToken: "token", BaseURL: mustURL(t, "https://api.cloudflare.com/client/v4/?x=1"), Timeout: time.Second}},
		{name: "base URL fragment", config: Config{ZoneID: "zone", APIToken: "token", BaseURL: mustURL(t, "https://api.cloudflare.com/client/v4/#frag"), Timeout: time.Second}},
		{name: "base URL non-Cloudflare host", config: Config{ZoneID: "zone", APIToken: "token", BaseURL: mustURL(t, "https://example.com/client/v4/"), Timeout: time.Second}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.config); err == nil {
				t.Fatal("New() error = nil, want non-nil")
			}
		})
	}
}

func TestBatchHelpers(t *testing.T) {
	t.Parallel()

	recordA := provider.Record{
		Name:       "host.example.com.",
		Type:       provider.RecordTypeA,
		Content:    "198.51.100.10",
		TTLSeconds: 300,
		Options:    provider.RecordOptions{Proxy: boolPtr(true)},
	}
	recordAAAA := provider.Record{
		Name:       "host.example.com.",
		Type:       provider.RecordTypeAAAA,
		Content:    "2001:db8::10",
		TTLSeconds: 600,
	}
	recordAAAAProxied := provider.Record{
		Name:       "host.example.com.",
		Type:       provider.RecordTypeAAAA,
		Content:    "2001:db8::20",
		TTLSeconds: 600,
		Options:    provider.RecordOptions{Proxy: boolPtr(true)},
	}

	if diff := cmp.Diff(provider.RecordOptions{Proxy: boolPtr(false)}, Config{Proxied: false}.RecordOptions()); diff != "" {
		t.Fatalf("Config.RecordOptions() mismatch (-want +got):\n%s", diff)
	}

	postA, err := toRecordPost(recordA)
	if err != nil {
		t.Fatalf("toRecordPost(A) error = %v", err)
	}
	if got := marshalJSONMap(t, postA); got["type"] != "A" || got["proxied"] != true {
		t.Fatalf("toRecordPost(A) JSON = %#v, want type A and proxied true", got)
	}

	postAAAA, err := toRecordPost(recordAAAA)
	if err != nil {
		t.Fatalf("toRecordPost(AAAA) error = %v", err)
	}
	if got := marshalJSONMap(t, postAAAA); got["type"] != "AAAA" || got["proxied"] != nil {
		t.Fatalf("toRecordPost(AAAA) JSON = %#v, want type AAAA without proxied", got)
	}

	patchA, err := toBatchPatch("record-a", recordA)
	if err != nil {
		t.Fatalf("toBatchPatch(A) error = %v", err)
	}
	if got := marshalJSONMap(t, patchA); got["id"] != "record-a" || got["type"] != "A" {
		t.Fatalf("toBatchPatch(A) JSON = %#v, want id record-a and type A", got)
	}

	patchAAAA, err := toBatchPatch("record-aaaa", recordAAAA)
	if err != nil {
		t.Fatalf("toBatchPatch(AAAA) error = %v", err)
	}
	if got := marshalJSONMap(t, patchAAAA); got["id"] != "record-aaaa" || got["type"] != "AAAA" {
		t.Fatalf("toBatchPatch(AAAA) JSON = %#v, want id record-aaaa and type AAAA", got)
	}

	patchAAAAProxied, err := toBatchPatch("record-aaaa-proxied", recordAAAAProxied)
	if err != nil {
		t.Fatalf("toBatchPatch(AAAA proxied) error = %v", err)
	}
	if got := marshalJSONMap(t, patchAAAAProxied); got["proxied"] != true {
		t.Fatalf("toBatchPatch(AAAA proxied) JSON = %#v, want proxied true", got)
	}

	plan := provider.Plan{
		Operations: []provider.Operation{
			{
				Kind: provider.OperationDelete,
				Current: provider.Record{
					ID: "old",
				},
			},
			{
				Kind:    provider.OperationUpdate,
				Current: provider.Record{ID: "existing"},
				Desired: recordA,
			},
			{
				Kind:    provider.OperationCreate,
				Desired: recordAAAA,
			},
		},
	}
	params, err := buildBatchParams("zone-id", plan)
	if err != nil {
		t.Fatalf("buildBatchParams() error = %v", err)
	}
	if got, want := params.ZoneID.Value, "zone-id"; got != want {
		t.Fatalf("ZoneID = %q, want %q", got, want)
	}

	empty, err := buildBatchParams("zone-id", provider.Plan{})
	if err != nil {
		t.Fatalf("buildBatchParams(empty) error = %v", err)
	}
	if got := marshalJSONMap(t, empty); len(got) != 0 {
		t.Fatalf("buildBatchParams(empty) JSON = %#v, want empty object", got)
	}

	if _, err := buildBatchParams("zone-id", provider.Plan{
		Operations: []provider.Operation{{Kind: provider.OperationKind("bad")}},
	}); err == nil {
		t.Fatal("buildBatchParams(bad kind) error = nil, want non-nil")
	}
	if _, err := buildBatchParams("zone-id", provider.Plan{
		Operations: []provider.Operation{{
			Kind:    provider.OperationUpdate,
			Current: provider.Record{ID: "existing"},
			Desired: provider.Record{Type: provider.RecordType("TXT")},
		}},
	}); err == nil {
		t.Fatal("buildBatchParams(invalid update) error = nil, want non-nil")
	}
	if _, err := buildBatchParams("zone-id", provider.Plan{
		Operations: []provider.Operation{{
			Kind:    provider.OperationCreate,
			Desired: provider.Record{Type: provider.RecordType("TXT")},
		}},
	}); err == nil {
		t.Fatal("buildBatchParams(invalid create) error = nil, want non-nil")
	}
	if _, err := toRecordPost(provider.Record{Type: provider.RecordType("TXT")}); err == nil {
		t.Fatal("toRecordPost(unsupported) error = nil, want non-nil")
	}
	if _, err := toBatchPatch("record", provider.Record{Type: provider.RecordType("TXT")}); err == nil {
		t.Fatal("toBatchPatch(unsupported) error = nil, want non-nil")
	}
}

func TestErrorFormattingAndNormalization(t *testing.T) {
	t.Parallel()

	if got, want := normalizeAPIName(" HOST.Example.com. "), "host.example.com"; got != want {
		t.Fatalf("normalizeAPIName() = %q, want %q", got, want)
	}
	if got, want := normalizeProviderName(" HOST.Example.com. "), "host.example.com."; got != want {
		t.Fatalf("normalizeProviderName() = %q, want %q", got, want)
	}
	if diff := cmp.Diff(provider.RecordOptions{Proxy: boolPtr(true)}, recordOptions(true)); diff != "" {
		t.Fatalf("recordOptions(true) mismatch (-want +got):\n%s", diff)
	}

	if got, want := formatAPIProblems(nil), "no error details returned"; got != want {
		t.Fatalf("formatAPIProblems(nil) = %q, want %q", got, want)
	}
	if got := formatAPIProblems([]cloudflareapi.ErrorData{{Message: "boom"}}); got != "boom" {
		t.Fatalf("formatAPIProblems() = %q, want boom", got)
	}
	if got := formatAPIProblems([]cloudflareapi.ErrorData{{Code: 12, Message: "boom"}}); got != "12: boom" {
		t.Fatalf("formatAPIProblems(with code) = %q, want 12: boom", got)
	}
	if got := normalizeRequestError(context.Background(), "list Cloudflare DNS records", nil); got != nil {
		t.Fatalf("normalizeRequestError(nil) = %v, want nil", got)
	}

	transportErr := normalizeRequestError(
		context.Background(),
		"list Cloudflare DNS records",
		&url.Error{Op: http.MethodGet, URL: "https://api.cloudflare.com/client/v4/zones/test/dns_records", Err: io.ErrUnexpectedEOF},
	)
	if _, ok := retry.After(transportErr); !ok {
		t.Fatalf("After(transport error) ok = false, want true: %v", transportErr)
	}

	localErr := normalizeRequestError(context.Background(), "list Cloudflare DNS records", errors.New("boom"))
	if _, ok := retry.After(localErr); ok {
		t.Fatalf("After(local error) ok = true, want false: %v", localErr)
	}

	apiErr := normalizeRequestError(
		context.Background(),
		"list Cloudflare DNS records",
		&cloudflareapi.Error{
			StatusCode: http.StatusBadRequest,
			Errors:     []cloudflareapi.ErrorData{{Code: 1000, Message: "boom"}},
			Request:    mustRequest(t, http.MethodGet, "https://api.cloudflare.com/client/v4/zones/test/dns_records"),
			Response: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{},
			},
		},
	)
	if _, ok := retry.After(apiErr); ok {
		t.Fatalf("After(api 400) ok = true, want false: %v", apiErr)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("boom"))
	canceledErr := normalizeRequestError(ctx, "list Cloudflare DNS records", errors.New("late boom"))
	if _, ok := retry.After(canceledErr); ok {
		t.Fatalf("After(canceled) ok = true, want false: %v", canceledErr)
	}

	if got := retryAfterFromResponse(nil); got != 0 {
		t.Fatalf("retryAfterFromResponse(nil) = %v, want 0", got)
	}
	rateLimitResponse := &http.Response{
		Header: http.Header{
			"Retry-After-Ms": []string{"1250"},
		},
	}
	if got, want := retryAfterFromResponse(rateLimitResponse), 1250*time.Millisecond; got != want {
		t.Fatalf("retryAfterFromResponse(Retry-After-Ms) = %v, want %v", got, want)
	}

	fallbackResponse := &http.Response{
		Header: http.Header{
			"Retry-After-Ms": []string{"bad"},
			"Retry-After":    []string{"2"},
		},
	}
	if got, want := retryAfterFromResponse(fallbackResponse), 2*time.Second; got != want {
		t.Fatalf("retryAfterFromResponse(fallback) = %v, want %v", got, want)
	}

	if isRetryableTransportError(nil) {
		t.Fatal("isRetryableTransportError(nil) = true, want false")
	}
	if !isRetryableTransportError(temporaryNetError{}) {
		t.Fatal("isRetryableTransportError(net.Error) = false, want true")
	}
	if isRetryableTransportError(&url.Error{Err: httpclient.ErrRedirectNotAllowed}) {
		t.Fatal("isRetryableTransportError(redirect) = true, want false")
	}
	if !isRetryableTransportError(io.EOF) {
		t.Fatal("isRetryableTransportError(io.EOF) = false, want true")
	}
}

func testClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	baseURL := mustURL(t, server.URL+"/client/v4/")
	return mustClient(t, Config{
		ZoneID:   "023e105f4ecef8ad9ca31a8372d0c353",
		APIToken: "secret",
		BaseURL:  baseURL,
		Timeout:  5 * time.Second,
	})
}

func mustClient(t *testing.T, config Config) *Client {
	t.Helper()

	client, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
}

func marshalJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()

	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return decoded
}

func mapAt(value map[string]any, key string, index int) map[string]any {
	items, ok := value[key].([]any)
	if !ok {
		return nil
	}
	if index < 0 || index >= len(items) {
		return nil
	}
	item, ok := items[index].(map[string]any)
	if !ok {
		return nil
	}
	return item
}

func sliceLen(value any) int {
	items, ok := value.([]any)
	if !ok {
		return 0
	}
	return len(items)
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", value, err)
	}
	return parsed
}

func mustRequest(t *testing.T, method string, rawURL string) *http.Request {
	t.Helper()

	request, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	return request
}

func mustAddr(t *testing.T, value string) netip.Addr {
	t.Helper()

	address, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("netip.ParseAddr(%q) error = %v", value, err)
	}
	return address
}

func TestSharedPlanAndVerify(t *testing.T) {
	t.Parallel()

	ipv4 := mustAddr(t, "198.51.100.10")
	desired := provider.DesiredState{
		Name:       "host.example.com.",
		TTLSeconds: 300,
		IPv4:       &ipv4,
		Options:    provider.RecordOptions{Proxy: boolPtr(false)},
	}
	current := provider.State{
		Name: "host.example.com.",
		Records: []provider.Record{
			{ID: "a1", Name: "host.example.com.", Type: provider.RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: provider.RecordOptions{Proxy: boolPtr(false)}},
		},
	}
	plan, err := provider.BuildSingleAddressPlan(current, desired)
	if err != nil {
		t.Fatalf("BuildSingleAddressPlan() error = %v", err)
	}
	if !plan.IsNoop() {
		t.Fatalf("BuildSingleAddressPlan() = %+v, want noop", plan)
	}
	if err := provider.VerifySingleAddressState(current, desired); err != nil {
		t.Fatalf("VerifySingleAddressState() error = %v", err)
	}
}

type temporaryNetError struct{}

func (temporaryNetError) Error() string   { return "temporary network error" }
func (temporaryNetError) Timeout() bool   { return true }
func (temporaryNetError) Temporary() bool { return true }

func boolPtr(value bool) *bool {
	return &value
}
