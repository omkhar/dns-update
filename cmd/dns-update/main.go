package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"dns-update/internal/app"
	"dns-update/internal/config"
)

type runner interface {
	Run(ctx context.Context, dryRun bool, forcePush bool) error
}

type dependencies struct {
	loadConfig    func(options config.LoadOptions) (config.Config, error)
	newRunner     func(cfg config.Config, logger *slog.Logger) (runner, error)
	notifyContext func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc)
	lookupEnv     func(string) (string, bool)
}

var exitFunc = os.Exit

func main() {
	exitFunc(run(os.Args[1:], os.Stdout, os.Stderr, productionDependencies()))
}

func run(args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {
	flags, values := newFlagSet(stderr)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	runtimeOptions, err := resolveRuntimeOptions(flags, *values, deps.lookupEnv)
	if err != nil {
		writeError(stderr, "failed to parse flags", err)
		return 2
	}

	appLogger := newLogger(stdout, runtimeOptions.verbose)

	cfg, err := deps.loadConfig(config.LoadOptions{
		Path:         runtimeOptions.configPath,
		ExplicitPath: runtimeOptions.explicitConfigPath,
	})
	if err != nil {
		writeError(stderr, "failed to load config", err)
		return 1
	}
	if handled, err := handleIntrospection(stdout, cfg, runtimeOptions); handled {
		if err != nil {
			writeError(stderr, "failed to print config", err)
			return 1
		}
		return 0
	}

	ctx, stop := deps.notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if runtimeOptions.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runtimeOptions.timeout)
		defer cancel()
	}

	appRunner, err := deps.newRunner(cfg, appLogger)
	if err != nil {
		writeError(stderr, "failed to initialize app", err)
		return 1
	}
	if err := appRunner.Run(ctx, runtimeOptions.dryRun, runtimeOptions.forcePush); err != nil {
		writeError(stderr, "dns update run failed", err)
		return 1
	}

	return 0
}

func newLogger(stderr io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

func writeError(stderr io.Writer, message string, err error) {
	_, _ = fmt.Fprintf(stderr, "dns-update: %s: %v\n", message, err)
}

func productionDependencies() dependencies {
	return dependencies{
		loadConfig: config.LoadWithOptions,
		newRunner: func(cfg config.Config, logger *slog.Logger) (runner, error) {
			return app.New(cfg, logger)
		},
		notifyContext: signal.NotifyContext,
		lookupEnv:     os.LookupEnv,
	}
}
