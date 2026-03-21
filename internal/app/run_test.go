package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"dns-update/internal/config"
	"dns-update/internal/egress"
	"dns-update/internal/provider"
	"dns-update/internal/retry"
)

func TestNewSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
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
				Timeout: time.Second,
			},
			Provider: config.ProviderConfig{
				Type:    "cloudflare",
				Timeout: time.Second,
				Cloudflare: &config.CloudflareConfig{
					ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
					APITokenFile: tokenPath,
					BaseURL:      mustURL(t, "https://api.cloudflare.com/client/v4/"),
				},
			},
		},
		testLogger(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runner == nil {
		t.Fatal("New() = nil, want non-nil")
	}
	if diff := cmp.Diff(provider.RecordOptions{Proxy: boolPtr(false)}, runner.desiredOptions); diff != "" {
		t.Fatalf("runner.desiredOptions mismatch (-want +got):\n%s", diff)
	}
}

func TestNewRejectsMissingProviderSecret(t *testing.T) {
	t.Parallel()

	_, err := New(
		config.Config{
			Record: config.RecordConfig{
				Name:       "host.example.com.",
				Zone:       "example.com.",
				TTLSeconds: 300,
			},
			Provider: config.ProviderConfig{
				Type: "cloudflare",
				Cloudflare: &config.CloudflareConfig{
					ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
					APITokenFile: "/tmp/does-not-exist",
					BaseURL:      mustURL(t, "https://api.cloudflare.com/client/v4/"),
				},
			},
		},
		testLogger(),
	)
	if err == nil {
		t.Fatal("New() error = nil, want non-nil")
	}
}

func TestRunNoop(t *testing.T) {
	t.Parallel()

	runner := testRunner(t, providerState(
		record("a1", provider.RecordTypeA, "198.51.100.10"),
		record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
	))

	if err := runner.Run(context.Background(), false, false); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if runner.provider.(*fakeProvider).applyCalls != 0 {
		t.Fatal("Apply() was called, want no calls")
	}
}

func TestRunForcePushNoopApplies(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
			providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
	}

	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, true); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fake.applyCalls, 1; got != want {
		t.Fatalf("Apply() calls = %d, want %d", got, want)
	}
	if got, want := fake.readCalls, 2; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
}

func TestRunForcePushDryRunSkipsApply(t *testing.T) {
	t.Parallel()

	buffer := new(bytes.Buffer)
	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
	}

	runner := testRunnerWithLogger(t, fake, slog.LevelInfo, buffer)
	if err := runner.Run(context.Background(), true, true); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fake.applyCalls, 0; got != want {
		t.Fatalf("Apply() calls = %d, want %d", got, want)
	}
	if got, want := fake.readCalls, 1; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
	got := buffer.String()
	if !strings.Contains(got, "dry run: planned provider operations") {
		t.Fatalf("logger output = %q, want dry-run message", got)
	}
	if !strings.Contains(got, "update A") || !strings.Contains(got, "update AAAA") {
		t.Fatalf("logger output = %q, want forced update summaries", got)
	}
}

func TestRunForcePushNoopWithoutManagedAddresses(t *testing.T) {
	t.Parallel()

	buffer := new(bytes.Buffer)
	fake := &fakeProvider{
		readStates: []provider.State{providerState()},
	}

	runner := &Runner{
		cfg:            testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober:         &fakeProber{},
		provider:       fake,
		desiredOptions: provider.RecordOptions{Proxy: boolPtr(false)},
		logger:         slog.New(slog.NewTextHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug})),
		retries:        testRetryPolicy(),
	}

	if err := runner.Run(context.Background(), false, true); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fake.applyCalls, 0; got != want {
		t.Fatalf("Apply() calls = %d, want %d", got, want)
	}
	if got, want := fake.readCalls, 1; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
	if got := buffer.String(); !strings.Contains(got, "records already match current egress IPs") {
		t.Fatalf("logger output = %q, want noop debug message", got)
	}
}

func TestRunNoopLogsOnlyAtDebug(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		prober        *fakeProber
		providerState provider.State
	}{
		{
			name: "both A and AAAA",
			prober: &fakeProber{
				ipv4: mustAddr(t, "198.51.100.10"),
				ipv6: mustAddr(t, "2001:db8::10"),
			},
			providerState: providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
		{
			name: "A only",
			prober: &fakeProber{
				ipv4: mustAddr(t, "198.51.100.10"),
			},
			providerState: providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
			),
		},
		{
			name: "AAAA only",
			prober: &fakeProber{
				ipv6: mustAddr(t, "2001:db8::10"),
			},
			providerState: providerState(
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			infoBuffer := new(bytes.Buffer)
			infoRunner := &Runner{
				cfg:            testConfig(t, "http://example.com/4", "http://example.com/6"),
				prober:         tc.prober,
				provider:       &fakeProvider{readStates: []provider.State{tc.providerState}},
				desiredOptions: provider.RecordOptions{Proxy: boolPtr(false)},
				logger:         slog.New(slog.NewTextHandler(infoBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})),
				retries:        testRetryPolicy(),
			}
			if err := infoRunner.Run(context.Background(), false, false); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if infoBuffer.Len() != 0 {
				t.Fatalf("info logger output = %q, want empty", infoBuffer.String())
			}

			debugBuffer := new(bytes.Buffer)
			debugRunner := &Runner{
				cfg:            testConfig(t, "http://example.com/4", "http://example.com/6"),
				prober:         tc.prober,
				provider:       &fakeProvider{readStates: []provider.State{tc.providerState}},
				desiredOptions: provider.RecordOptions{Proxy: boolPtr(false)},
				logger:         slog.New(slog.NewTextHandler(debugBuffer, &slog.HandlerOptions{Level: slog.LevelDebug})),
				retries:        testRetryPolicy(),
			}
			if err := debugRunner.Run(context.Background(), false, false); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := debugBuffer.String(); !strings.Contains(got, "records already match current egress IPs") {
				t.Fatalf("debug logger output = %q, want noop message", got)
			}
		})
	}
}

func TestRunDryRunSkipsApply(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
		},
	}

	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), true, false); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fake.applyCalls != 0 {
		t.Fatal("Apply() was called during dry run")
	}
	if got, want := fake.readCalls, 1; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
}

func TestRunDryRunLogsInfo(t *testing.T) {
	t.Parallel()

	buffer := new(bytes.Buffer)
	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
		},
	}

	runner := testRunnerWithLogger(t, fake, slog.LevelInfo, buffer)
	if err := runner.Run(context.Background(), true, false); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := buffer.String(); !strings.Contains(got, "dry run: planned provider operations") {
		t.Fatalf("logger output = %q, want dry-run message", got)
	}
	if got := buffer.String(); strings.Contains(got, "evaluated DNS state") {
		t.Fatalf("logger output = %q, want no debug state line", got)
	}
}

func TestRunApplyAndVerify(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
			providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
	}

	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, false); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fake.applyCalls, 1; got != want {
		t.Fatalf("Apply() calls = %d, want %d", got, want)
	}
	if got, want := fake.readCalls, 2; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
}

func TestRunApplyLogsOnlyUpdateAtInfo(t *testing.T) {
	t.Parallel()

	buffer := new(bytes.Buffer)
	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
			providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
	}

	runner := testRunnerWithLogger(t, fake, slog.LevelInfo, buffer)
	if err := runner.Run(context.Background(), false, false); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := buffer.String()
	if !strings.Contains(got, "applied DNS update") {
		t.Fatalf("logger output = %q, want applied update message", got)
	}
	if strings.Contains(got, "verified DNS state after update") {
		t.Fatalf("logger output = %q, want no debug verification message", got)
	}
}

func TestRunBothProbesError(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		cfg: testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober: &fakeProber{
			ipv4Err: errors.New("bad-response"),
			ipv6Err: errors.New("bad-response"),
		},
		provider: &fakeProvider{
			readStates: []provider.State{providerState()},
		},
		logger:  testLogger(),
		retries: testRetryPolicy(),
	}

	err := runner.Run(context.Background(), false, false)
	if err == nil {
		t.Fatal("Run() error = nil, want probe failure")
	}
	if !strings.Contains(err.Error(), "all egress probes failed") {
		t.Fatalf("Run() error = %q, want message containing \"all egress probes failed\"", err)
	}
}

func TestCollectIPv4ProbeError(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		cfg: testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober: &fakeProber{
			ipv4Err: errors.New("bad-response"),
			ipv6:    mustAddr(t, "2001:db8::10"),
		},
		provider: &fakeProvider{
			readStates: []provider.State{providerState()},
		},
		logger: testLogger(),
	}

	observed, _, err := runner.collect(context.Background())
	if err != nil {
		t.Fatalf("collect() error = %v, want nil (IPv6 probe succeeded)", err)
	}
	if observed.IPv4 != nil {
		t.Fatalf("collect() observed.IPv4 = %v, want nil", observed.IPv4)
	}
	if observed.IPv6 == nil {
		t.Fatal("collect() observed.IPv6 = nil, want non-nil")
	}
}

func TestCollectIPv6ProbeError(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		cfg: testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober: &fakeProber{
			ipv4:    mustAddr(t, "198.51.100.10"),
			ipv6Err: errors.New("bad-response"),
		},
		provider: &fakeProvider{
			readStates: []provider.State{providerState()},
		},
		logger: testLogger(),
	}

	observed, _, err := runner.collect(context.Background())
	if err != nil {
		t.Fatalf("collect() error = %v, want nil (IPv4 probe succeeded)", err)
	}
	if observed.IPv4 == nil {
		t.Fatal("collect() observed.IPv4 = nil, want non-nil")
	}
	if observed.IPv6 != nil {
		t.Fatalf("collect() observed.IPv6 = %v, want nil", observed.IPv6)
	}
}

func TestRunReadStateError(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readErrors: []error{errors.New("boom")},
	}
	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want read-state failure")
	}
	if got, want := fake.readCalls, 1; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
}

func TestRunRetriesRetryableErrorThenSucceeds(t *testing.T) {
	t.Parallel()

	sleepCalls := 0
	fake := &fakeProvider{
		readErrors: []error{
			retry.Mark(errors.New("boom"), 2*time.Second),
		},
		readStates: []provider.State{
			providerState(),
			providerState(
				record("a1", provider.RecordTypeA, "198.51.100.10"),
				record("aaaa1", provider.RecordTypeAAAA, "2001:db8::10"),
			),
		},
	}

	runner := testRunnerWithProvider(t, fake)
	runner.retries = retry.Policy{
		MaxAttempts:   3,
		InitialDelay:  time.Second,
		MaxDelay:      5 * time.Second,
		RandomFloat64: func() float64 { return 0 },
		Sleep: func(context.Context, time.Duration) error {
			sleepCalls++
			return nil
		},
	}

	if err := runner.Run(context.Background(), false, false); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if got, want := fake.readCalls, 2; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
	if got, want := sleepCalls, 1; got != want {
		t.Fatalf("sleep calls = %d, want %d", got, want)
	}
}

func TestRunStopsAfterRetryLimit(t *testing.T) {
	t.Parallel()

	sleepCalls := 0
	fake := &fakeProvider{
		readErrors: []error{
			retry.Mark(errors.New("boom"), 0),
			retry.Mark(errors.New("boom"), 0),
		},
	}

	runner := testRunnerWithProvider(t, fake)
	runner.retries = retry.Policy{
		MaxAttempts:   2,
		InitialDelay:  time.Second,
		MaxDelay:      time.Second,
		RandomFloat64: func() float64 { return 0 },
		Sleep: func(context.Context, time.Duration) error {
			sleepCalls++
			return nil
		},
	}

	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want exhausted retry error")
	}
	if got, want := fake.readCalls, 2; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
	if got, want := sleepCalls, 1; got != want {
		t.Fatalf("sleep calls = %d, want %d", got, want)
	}
}

func TestRunRetryWaitError(t *testing.T) {
	t.Parallel()

	sleepCalls := 0
	fake := &fakeProvider{
		readErrors: []error{
			retry.Mark(errors.New("boom"), 0),
		},
	}

	runner := testRunnerWithProvider(t, fake)
	runner.retries = retry.Policy{
		RandomFloat64: func() float64 { return 0 },
		Sleep: func(context.Context, time.Duration) error {
			sleepCalls++
			return errors.New("wait failed")
		},
	}

	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want wait failure")
	}
	if got, want := sleepCalls, 1; got != want {
		t.Fatalf("sleep calls = %d, want %d", got, want)
	}
	if got, want := fake.readCalls, 1; got != want {
		t.Fatalf("ReadState() calls = %d, want %d", got, want)
	}
}

func TestRunBuildPlanError(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("c1", provider.RecordTypeCNAME, "other.example.com.")),
		},
	}
	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want build-plan failure")
	}
}

func TestRunApplyError(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
		},
		applyErr: errors.New("boom"),
	}
	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want apply failure")
	}
}

func TestRunVerifyReadError(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
		},
		readErrors: []error{nil, errors.New("boom")},
	}
	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want verify read failure")
	}
}

func TestRunVerifyError(t *testing.T) {
	t.Parallel()

	fake := &fakeProvider{
		readStates: []provider.State{
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
			providerState(record("a1", provider.RecordTypeA, "198.51.100.10")),
		},
	}
	runner := testRunnerWithProvider(t, fake)
	if err := runner.Run(context.Background(), false, false); err == nil {
		t.Fatal("Run() error = nil, want verify failure")
	}
}

func TestCollectMultipleErrors(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		cfg: testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober: &fakeProber{
			ipv4Err: errors.New("bad-response"),
			ipv6Err: errors.New("bad-response"),
		},
		provider: &fakeProvider{
			readErrors: []error{errors.New("boom")},
		},
		logger: testLogger(),
	}

	if _, _, err := runner.collect(context.Background()); err == nil {
		t.Fatal("collect() error = nil, want non-nil")
	}
}

func TestFormatHelpers(t *testing.T) {
	t.Parallel()

	if got, want := formatDesired(nil), "none"; got != want {
		t.Fatalf("formatDesired(nil) = %q, want %q", got, want)
	}
	if got, want := formatRecords(nil), "none"; got != want {
		t.Fatalf("formatRecords(nil) = %q, want %q", got, want)
	}
}

func testRunner(t *testing.T, state provider.State) *Runner {
	t.Helper()

	return testRunnerWithProvider(t, &fakeProvider{
		readStates: []provider.State{state},
	})
}

func testRunnerWithProvider(t *testing.T, fake *fakeProvider) *Runner {
	t.Helper()

	return &Runner{
		cfg:            testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober:         defaultFakeProber(t),
		provider:       fake,
		desiredOptions: provider.RecordOptions{Proxy: boolPtr(false)},
		logger:         testLogger(),
		retries:        testRetryPolicy(),
	}
}

func testRunnerWithLogger(t *testing.T, fake *fakeProvider, level slog.Level, output io.Writer) *Runner {
	t.Helper()

	return &Runner{
		cfg:            testConfig(t, "http://example.com/4", "http://example.com/6"),
		prober:         defaultFakeProber(t),
		provider:       fake,
		desiredOptions: provider.RecordOptions{Proxy: boolPtr(false)},
		logger: slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
			Level: level,
		})),
		retries: testRetryPolicy(),
	}
}

func testRetryPolicy() retry.Policy {
	return retry.Policy{
		MaxAttempts:   5,
		InitialDelay:  time.Millisecond,
		MaxDelay:      time.Millisecond,
		RandomFloat64: func() float64 { return 0 },
		Sleep: func(context.Context, time.Duration) error {
			return nil
		},
	}
}

func testConfig(t *testing.T, ipv4URL string, ipv6URL string) config.Config {
	t.Helper()

	return config.Config{
		Record: config.RecordConfig{
			Name:       "host.example.com.",
			Zone:       "example.com.",
			TTLSeconds: 300,
		},
		Probe: config.ProbeConfig{
			IPv4URL: mustURL(t, ipv4URL),
			IPv6URL: mustURL(t, ipv6URL),
			Timeout: time.Second,
		},
		Provider: config.ProviderConfig{
			Type: "cloudflare",
			Cloudflare: &config.CloudflareConfig{
				Proxied: false,
			},
		},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeProber struct {
	ipv4    *netip.Addr
	ipv6    *netip.Addr
	ipv4Err error
	ipv6Err error
}

func (f *fakeProber) Lookup(_ context.Context, _ *url.URL, family egress.Family) (*netip.Addr, error) {
	switch family {
	case egress.IPv4:
		return f.ipv4, f.ipv4Err
	case egress.IPv6:
		return f.ipv6, f.ipv6Err
	default:
		return nil, errors.New("unsupported")
	}
}

type fakeProvider struct {
	readStates []provider.State
	readErrors []error
	applyErr   error
	readCalls  int
	applyCalls int
}

func (f *fakeProvider) ReadState(context.Context, string) (provider.State, error) {
	index := f.readCalls
	f.readCalls++

	if index < len(f.readErrors) && f.readErrors[index] != nil {
		return provider.State{}, f.readErrors[index]
	}
	if index < len(f.readStates) {
		return f.readStates[index], nil
	}
	return provider.State{}, nil
}

func (f *fakeProvider) Apply(context.Context, provider.Plan) error {
	f.applyCalls++
	return f.applyErr
}

func providerState(records ...provider.Record) provider.State {
	return provider.State{
		Name:    "host.example.com.",
		Records: records,
	}
}

func record(id string, recordType provider.RecordType, content string) provider.Record {
	return provider.Record{
		ID:         id,
		Name:       "host.example.com.",
		Type:       recordType,
		Content:    content,
		TTLSeconds: 300,
		Options:    provider.RecordOptions{Proxy: boolPtr(false)},
	}
}

func defaultFakeProber(t *testing.T) *fakeProber {
	t.Helper()

	return &fakeProber{
		ipv4: mustAddr(t, "198.51.100.10"),
		ipv6: mustAddr(t, "2001:db8::10"),
	}
}

func boolPtr(value bool) *bool {
	return &value
}
