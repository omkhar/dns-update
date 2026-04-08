package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"dns-update/internal/config"
	"dns-update/internal/provider"
)

func TestRunWithLiveCloudflareAndProbeStubs(t *testing.T) {
	t.Parallel()

	ipv4Probe := newLoopbackProbeServer(t, "tcp4", "127.0.0.1:0", "ip=198.51.100.10\n")
	ipv6Probe := newLoopbackProbeServer(t, "tcp6", "[::1]:0", "ip=2001:db8::10\n")
	cloudflare := newCloudflareStubServer(t)

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner, err := New(
		config.Config{
			Record: config.RecordConfig{
				Name:       "host.example.com.",
				Zone:       "example.com.",
				TTLSeconds: 300,
			},
			Probe: config.ProbeConfig{
				IPv4URL: mustURL(t, ipv4Probe.URL),
				IPv6URL: mustURL(t, ipv6Probe.URL),
				Timeout: time.Second,
			},
			Provider: config.ProviderConfig{
				Type:    "cloudflare",
				Timeout: time.Second,
				Cloudflare: &config.CloudflareConfig{
					ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
					APITokenFile: tokenPath,
					BaseURL:      mustURL(t, cloudflare.URL()+"/client/v4/"),
				},
			},
		},
		testLogger(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := runner.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := cloudflare.batchCalls(), 1; got != want {
		t.Fatalf("batchCalls() = %d, want %d", got, want)
	}

	got := cloudflare.records()
	want := []provider.Record{
		record("record-1", provider.RecordTypeA, "198.51.100.10"),
		record("record-2", provider.RecordTypeAAAA, "2001:db8::10"),
	}
	for i := range want {
		want[i].Name = "host.example.com."
		want[i].TTLSeconds = 300
		want[i].Options = provider.RecordOptions{Proxy: new(false)}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("records mismatch (-want +got):\n%s", diff)
	}
}

func newLoopbackProbeServer(t *testing.T, network string, address string, response string) *httptest.Server {
	t.Helper()

	listener, err := net.Listen(network, address)
	if err != nil {
		if network == "tcp6" {
			t.Skipf("IPv6 loopback is unavailable: %v", err)
		}
		t.Fatalf("net.Listen(%q, %q) error = %v", network, address, err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodGet; got != want {
			t.Fatalf("probe method = %q, want %q", got, want)
		}
		if _, err := io.WriteString(w, response); err != nil {
			t.Fatalf("probe WriteString() error = %v", err)
		}
	}))
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}

type cloudflareStubServer struct {
	t      *testing.T
	server *httptest.Server

	mu         sync.Mutex
	nextID     int
	batchCount int
	recordsSet []provider.Record
}

func newCloudflareStubServer(t *testing.T) *cloudflareStubServer {
	t.Helper()

	stub := &cloudflareStubServer{t: t}
	server := httptest.NewServer(http.HandlerFunc(stub.serveHTTP))
	t.Cleanup(server.Close)
	stub.server = server
	return stub
}

func (s *cloudflareStubServer) URL() string {
	return s.server.URL
}

func (s *cloudflareStubServer) batchCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batchCount
}

func (s *cloudflareStubServer) records() []provider.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]provider.Record, len(s.recordsSet))
	copy(records, s.recordsSet)
	return records
}

func (s *cloudflareStubServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.t.Helper()

	if got, want := r.Header.Get("Authorization"), "Bearer secret"; got != want {
		s.t.Fatalf("Authorization header = %q, want %q", got, want)
	}

	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/dns_records"):
		if page := r.URL.Query().Get("page"); page == "2" {
			s.writeJSON(w, map[string]any{
				"success": true,
				"result":  []map[string]any{},
			})
			return
		}
		s.writeJSON(w, map[string]any{
			"success": true,
			"result":  s.listResponse(),
			"result_info": map[string]any{
				"page":        1,
				"per_page":    100,
				"count":       len(s.records()),
				"total_count": len(s.records()),
				"total_pages": 1,
			},
		})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dns_records/batch"):
		s.applyBatch(r.Body)
		s.writeJSON(w, map[string]any{
			"success": true,
			"result":  map[string]any{},
		})
	default:
		http.NotFound(w, r)
	}
}

func (s *cloudflareStubServer) listResponse() []map[string]any {
	records := s.records()
	response := make([]map[string]any, 0, len(records))
	for _, record := range records {
		response = append(response, map[string]any{
			"id":      record.ID,
			"name":    strings.TrimSuffix(record.Name, "."),
			"type":    string(record.Type),
			"content": record.Content,
			"ttl":     record.TTLSeconds,
			"proxied": false,
		})
	}
	return response
}

func (s *cloudflareStubServer) applyBatch(body io.ReadCloser) {
	s.t.Helper()
	defer func() {
		_ = body.Close()
	}()

	var request struct {
		Posts []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Content string `json:"content"`
			TTL     uint32 `json:"ttl"`
			Proxied *bool  `json:"proxied"`
		} `json:"posts"`
	}
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		s.t.Fatalf("Decode() error = %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.batchCount++
	s.recordsSet = s.recordsSet[:0]
	for _, post := range request.Posts {
		s.nextID++
		record := provider.Record{
			ID:         "record-" + strconv.Itoa(s.nextID),
			Name:       post.Name + ".",
			Type:       provider.RecordType(post.Type),
			Content:    post.Content,
			TTLSeconds: post.TTL,
			Options:    provider.RecordOptions{Proxy: new(false)},
		}
		if post.Proxied != nil {
			record.Options.Proxy = new(*post.Proxied)
		}
		s.recordsSet = append(s.recordsSet, record)
	}
}

func (s *cloudflareStubServer) writeJSON(w http.ResponseWriter, payload any) {
	s.t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.t.Fatalf("Encode() error = %v", err)
	}
}
