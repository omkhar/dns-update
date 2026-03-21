package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dns-update/internal/config"
)

func TestRunSuccessDryRun(t *testing.T) {
	t.Parallel()

	var (
		gotOptions config.LoadOptions
		gotDryRun  bool
		gotForce   bool
		ran        bool
	)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-config", "/tmp/config.json", "-dry-run", "-verbose"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(options config.LoadOptions) (config.Config, error) {
				gotOptions = options
				return config.Config{}, nil
			},
			newRunner: func(_ config.Config, _ *slog.Logger) (runner, error) {
				return runnerFunc(func(_ context.Context, dryRun bool, forcePush bool) error {
					ran = true
					gotDryRun = dryRun
					gotForce = forcePush
					return nil
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if got, want := gotOptions.Path, "/tmp/config.json"; got != want {
		t.Fatalf("loadConfig path = %q, want %q", got, want)
	}
	if !gotOptions.ExplicitPath {
		t.Fatal("loadConfig explicitPath = false, want true")
	}
	if !ran {
		t.Fatal("runner was not invoked")
	}
	if !gotDryRun {
		t.Fatal("dryRun = false, want true")
	}
	if gotForce {
		t.Fatal("forcePush = true, want false")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunSuccessForcePush(t *testing.T) {
	t.Parallel()

	var gotForcePush bool

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-force-push"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(options config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(_ config.Config, _ *slog.Logger) (runner, error) {
				return runnerFunc(func(_ context.Context, dryRun bool, forcePush bool) error {
					if dryRun {
						t.Fatal("dryRun = true, want false")
					}
					gotForcePush = forcePush
					return nil
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if !gotForcePush {
		t.Fatal("forcePush = false, want true")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUsesRuntimeEnvWhenFlagsAreUnset(t *testing.T) {
	t.Parallel()

	var gotOptions config.LoadOptions

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(options config.LoadOptions) (config.Config, error) {
				gotOptions = options
				return config.Config{}, nil
			},
			newRunner: func(_ config.Config, logger *slog.Logger) (runner, error) {
				return runnerFunc(func(ctx context.Context, dryRun bool, forcePush bool) error {
					if !dryRun {
						t.Fatal("dryRun = false, want true")
					}
					deadline, ok := ctx.Deadline()
					if !ok {
						t.Fatal("ctx.Deadline() ok = false, want true")
					}
					remaining := time.Until(deadline)
					if remaining <= 0 || remaining > 2*time.Second {
						t.Fatalf("time until deadline = %v, want value in (0s, 2s]", remaining)
					}
					logger.Debug("verbose log")
					return nil
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(map[string]string{
				envConfigPath: "/tmp/env-config.json",
				envDryRun:     "true",
				envVerbose:    "true",
				envTimeout:    "2s",
			}),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if got, want := gotOptions.Path, "/tmp/env-config.json"; got != want {
		t.Fatalf("loadConfig path = %q, want %q", got, want)
	}
	if !gotOptions.ExplicitPath {
		t.Fatal("loadConfig explicitPath = false, want true")
	}
	if !strings.Contains(stdout.String(), "verbose log") {
		t.Fatalf("stdout = %q, want verbose log", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunFlagsOverrideRuntimeEnv(t *testing.T) {
	t.Parallel()

	var gotOptions config.LoadOptions

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{
			"-config", "/tmp/cli-config.json",
			"-dry-run=false",
			"-verbose",
			"-timeout", "1s",
		},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(options config.LoadOptions) (config.Config, error) {
				gotOptions = options
				return config.Config{}, nil
			},
			newRunner: func(_ config.Config, logger *slog.Logger) (runner, error) {
				return runnerFunc(func(ctx context.Context, dryRun bool, forcePush bool) error {
					if dryRun {
						t.Fatal("dryRun = true, want false")
					}
					deadline, ok := ctx.Deadline()
					if !ok {
						t.Fatal("ctx.Deadline() ok = false, want true")
					}
					remaining := time.Until(deadline)
					if remaining <= 0 || remaining > time.Second {
						t.Fatalf("time until deadline = %v, want value in (0s, 1s]", remaining)
					}
					logger.Debug("verbose log")
					return nil
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(map[string]string{
				envConfigPath: "/tmp/env-config.json",
				envDryRun:     "true",
				envVerbose:    "false",
				envTimeout:    "5s",
			}),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if got, want := gotOptions.Path, "/tmp/cli-config.json"; got != want {
		t.Fatalf("loadConfig path = %q, want %q", got, want)
	}
	if !gotOptions.ExplicitPath {
		t.Fatal("loadConfig explicitPath = false, want true")
	}
	if !strings.Contains(stdout.String(), "verbose log") {
		t.Fatalf("stdout = %q, want verbose log", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunRejectsInvalidRuntimeEnv(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				t.Fatal("loadConfig should not be called")
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(map[string]string{
				envTimeout: "bad",
			}),
		},
	)

	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), envTimeout) {
		t.Fatalf("stderr = %q, want env parse error", stderr.String())
	}
}

func TestRunRejectsInvalidDryRunEnv(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				t.Fatal("loadConfig should not be called")
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(map[string]string{
				envDryRun: "bad",
			}),
		},
	)

	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), envDryRun) {
		t.Fatalf("stderr = %q, want env parse error", stderr.String())
	}
}

func TestRunRejectsInvalidVerboseEnv(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				t.Fatal("loadConfig should not be called")
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(map[string]string{
				envVerbose: "bad",
			}),
		},
	)

	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), envVerbose) {
		t.Fatalf("stderr = %q, want env parse error", stderr.String())
	}
}

func TestRunRejectsRemovedConfigOverrideFlag(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-record-ttl-seconds", "4294967296"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				t.Fatal("loadConfig should not be called")
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q, want unknown flag error", stderr.String())
	}
	if strings.Contains(stderr.String(), "failed to parse flags") {
		t.Fatalf("stderr = %q, want no duplicate parse logger output", stderr.String())
	}
}

func TestRunTimeoutFlagSetsDeadline(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run(
		[]string{"-timeout", "3s"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				return runnerFunc(func(ctx context.Context, dryRun bool, forcePush bool) error {
					if dryRun {
						t.Fatal("dryRun = true, want false")
					}
					if forcePush {
						t.Fatal("forcePush = true, want false")
					}
					deadline, ok := ctx.Deadline()
					if !ok {
						t.Fatal("ctx.Deadline() ok = false, want true")
					}
					remaining := time.Until(deadline)
					if remaining <= 0 || remaining > 3*time.Second {
						t.Fatalf("time until deadline = %v, want value in (0s, 3s]", remaining)
					}
					return nil
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunTimeoutExceeded(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run(
		[]string{"-timeout", "1ms"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				return runnerFunc(func(ctx context.Context, dryRun bool, forcePush bool) error {
					if dryRun {
						t.Fatal("dryRun = true, want false")
					}
					if forcePush {
						t.Fatal("forcePush = true, want false")
					}
					<-ctx.Done()
					return context.Cause(ctx)
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 1; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "context deadline exceeded") {
		t.Fatalf("stderr = %q, want timeout error", stderr.String())
	}
}

func TestRunRejectsNegativeTimeout(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-timeout", "-1s"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				t.Fatal("loadConfig should not be called")
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "timeout must be greater than or equal to 0") {
		t.Fatalf("stderr = %q, want timeout validation error", stderr.String())
	}
}

func TestRunFlagParseError(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"-unknown"}, stdout, stderr, noOpDependencies())
	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q, want parse error", stderr.String())
	}
	if strings.Contains(stderr.String(), "failed to parse flags") {
		t.Fatalf("stderr = %q, want no duplicate parse logger output", stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run([]string{"-h"}, stdout, stderr, noOpDependencies())
	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Print planned changes without applying them.") {
		t.Fatalf("stderr = %q, want help output", stderr.String())
	}
	if !strings.Contains(stderr.String(), "-force-push") {
		t.Fatalf("stderr = %q, want force-push help output", stderr.String())
	}
}

func TestRunConfigLoadError(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, errors.New("boom")
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 1; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "failed to load config") {
		t.Fatalf("stderr = %q, want config load error", stderr.String())
	}
}

func TestRunValidateConfig(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-validate-config"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if got, want := stdout.String(), "config is valid\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunRejectsConflictingIntrospectionFlags(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-validate-config", "-print-effective-config"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				t.Fatal("loadConfig should not be called")
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 2; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "mutually exclusive") {
		t.Fatalf("stderr = %q, want conflicting introspection error", got)
	}
}

func TestRunPrintEffectiveConfig(t *testing.T) {
	t.Parallel()

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	ipv4URL, err := url.Parse("https://4.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	ipv6URL, err := url.Parse("https://6.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-config", "/tmp/config.json", "-dry-run", "-force-push", "-verbose", "-timeout", "3s", "-print-effective-config"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{
					SourcePath: "/tmp/config.json",
					Record: config.RecordConfig{
						Name:       "host.example.com.",
						Zone:       "example.com.",
						TTLSeconds: 300,
					},
					Probe: config.ProbeConfig{
						IPv4URL:           ipv4URL,
						IPv6URL:           ipv6URL,
						Timeout:           11 * time.Second,
						AllowInsecureHTTP: false,
					},
					Provider: config.ProviderConfig{
						Type:    "cloudflare",
						Timeout: 12 * time.Second,
						Cloudflare: &config.CloudflareConfig{
							ZoneID:       "023e105f4ecef8ad9ca31a8372d0c353",
							APITokenFile: "/tmp/cloudflare.token",
							BaseURL:      baseURL,
							Proxied:      true,
						},
					},
				}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	output := stdout.String()
	for _, expected := range []string{
		`"config_path": "/tmp/config.json"`,
		`"dry_run": true`,
		`"force_push": true`,
		`"verbose": true`,
		`"timeout": "3s"`,
		`"name": "host.example.com."`,
		`"api_token_file": "/tmp/cloudflare.token"`,
		`"base_url": "https://api.cloudflare.com/client/v4/"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stdout = %q, want substring %q", output, expected)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunPrintEffectiveConfigError(t *testing.T) {
	t.Parallel()

	ipv4URL, err := url.Parse("https://4.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	ipv6URL, err := url.Parse("https://6.ip.omsab.net/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	stderr := new(bytes.Buffer)
	exitCode := run(
		[]string{"-print-effective-config"},
		failingWriter{err: errors.New("boom")},
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{
					Record: config.RecordConfig{
						Name:       "host.example.com.",
						Zone:       "example.com.",
						TTLSeconds: 300,
					},
					Probe: config.ProbeConfig{
						IPv4URL: ipv4URL,
						IPv6URL: ipv6URL,
						Timeout: time.Second,
					},
					Provider: config.ProviderConfig{
						Type:    "cloudflare",
						Timeout: time.Second,
					},
				}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				t.Fatal("newRunner should not be called")
				return nil, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				t.Fatal("notifyContext should not be called")
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 1; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if got := stderr.String(); !strings.Contains(got, "failed to print config") {
		t.Fatalf("stderr = %q, want print config error", got)
	}
}

func TestPrintEffectiveConfigWithoutCloudflare(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	err := printEffectiveConfig(stdout, config.Config{
		Record: config.RecordConfig{
			Name:       "host.example.com.",
			Zone:       "example.com.",
			TTLSeconds: 300,
		},
		Provider: config.ProviderConfig{
			Type:    "test",
			Timeout: time.Second,
		},
	}, runtimeOptions{})
	if err != nil {
		t.Fatalf("printEffectiveConfig() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"cloudflare": {`) {
		t.Fatalf("stdout = %q, want cloudflare object", got)
	}
}

func TestRunRunnerConstructionError(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				return nil, errors.New("boom")
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 1; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "failed to initialize app") {
		t.Fatalf("stderr = %q, want runner construction error", stderr.String())
	}
}

func TestRunRunnerError(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(config.Config, *slog.Logger) (runner, error) {
				return runnerFunc(func(context.Context, bool, bool) error {
					return errors.New("boom")
				}), nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 1; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "dns update run failed") {
		t.Fatalf("stderr = %q, want runner error", stderr.String())
	}
}

func TestRunVerboseRunnerLogsReachStdout(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run(
		[]string{"-verbose"},
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(_ config.Config, logger *slog.Logger) (runner, error) {
				return loggerRunner{logger: logger}, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if !strings.Contains(stdout.String(), "verbose log") {
		t.Fatalf("stdout = %q, want verbose log", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunNonVerboseDebugLogsStaySilent(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run(
		nil,
		stdout,
		stderr,
		dependencies{
			loadConfig: func(config.LoadOptions) (config.Config, error) {
				return config.Config{}, nil
			},
			newRunner: func(_ config.Config, logger *slog.Logger) (runner, error) {
				return loggerRunner{logger: logger}, nil
			},
			notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				return parent, func() {}
			},
			lookupEnv: envLookup(nil),
		},
	)

	if got, want := exitCode, 0; got != want {
		t.Fatalf("run() exitCode = %d, want %d", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestProductionDependencies(t *testing.T) {
	t.Parallel()

	deps := productionDependencies()
	if deps.loadConfig == nil || deps.newRunner == nil || deps.notifyContext == nil || deps.lookupEnv == nil {
		t.Fatalf("productionDependencies() = %#v, want all hooks populated", deps)
	}
}

func TestProductionDependenciesNewRunner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	baseURL, err := url.Parse("https://api.cloudflare.com/client/v4/")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	deps := productionDependencies()
	runner, err := deps.newRunner(
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
					BaseURL:      baseURL,
				},
			},
		},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("newRunner() error = %v", err)
	}
	if runner == nil {
		t.Fatal("newRunner() = nil, want non-nil")
	}
}

func TestMainCallsExitFunc(t *testing.T) {
	originalArgs := os.Args
	originalExit := exitFunc
	defer func() {
		os.Args = originalArgs
		exitFunc = originalExit
	}()

	os.Args = []string{"dns-update", "-h"}
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	main()

	if got, want := exitCode, 0; got != want {
		t.Fatalf("main() exitCode = %d, want %d", got, want)
	}
}

type runnerFunc func(ctx context.Context, dryRun bool, forcePush bool) error

func (f runnerFunc) Run(ctx context.Context, dryRun bool, forcePush bool) error {
	return f(ctx, dryRun, forcePush)
}

type loggerRunner struct {
	logger *slog.Logger
}

func (r loggerRunner) Run(context.Context, bool, bool) error {
	r.logger.Debug("verbose log")
	return nil
}

func noOpDependencies() dependencies {
	return dependencies{
		loadConfig: func(config.LoadOptions) (config.Config, error) {
			return config.Config{}, nil
		},
		newRunner: func(config.Config, *slog.Logger) (runner, error) {
			return runnerFunc(func(context.Context, bool, bool) error { return nil }), nil
		},
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return parent, func() {}
		},
		lookupEnv: envLookup(nil),
	}
}

func envLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if values == nil {
			return "", false
		}
		value, ok := values[name]
		return value, ok
	}
}
