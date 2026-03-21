package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"strings"

	"dns-update/internal/config"
	"dns-update/internal/egress"
	"dns-update/internal/provider"
	"dns-update/internal/retry"
	"golang.org/x/sync/errgroup"
)

// Runner coordinates probes, DNS reads, comparisons, and updates.
type Runner struct {
	cfg            config.Config
	prober         prober
	provider       provider.Provider
	desiredOptions provider.RecordOptions
	logger         *slog.Logger
	retries        retry.Policy
}

type prober interface {
	Lookup(context.Context, *url.URL, egress.Family) (*netip.Addr, error)
}

// New returns a Runner.
func New(cfg config.Config, logger *slog.Logger) (*Runner, error) {
	dnsProvider, desiredOptions, err := newProvider(cfg.Provider)
	if err != nil {
		return nil, err
	}

	return &Runner{
		cfg:            cfg,
		prober:         egress.NewProber(cfg.Probe.Timeout),
		provider:       dnsProvider,
		desiredOptions: desiredOptions,
		logger:         logger,
		retries:        retry.DefaultPolicy(),
	}, nil
}

// Run performs one egress-to-DNS reconciliation cycle.
func (r *Runner) Run(ctx context.Context, dryRun bool, forcePush bool) error {
	for attempt := 1; ; attempt++ {
		err := r.runOnce(ctx, dryRun, forcePush)
		if err == nil {
			return nil
		}

		retryAfter, ok := r.retries.CanRetry(attempt, err)
		if !ok {
			return err
		}

		delay := r.retries.Delay(attempt, retryAfter)
		maxAttempts := r.retries.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = retry.DefaultMaxAttempts
		}
		r.logger.Debug(
			"transient error; retrying reconciliation",
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"delay", delay,
			"error", err,
		)
		if waitErr := r.retries.Wait(ctx, delay); waitErr != nil {
			return waitErr
		}
	}
}

func (r *Runner) runOnce(ctx context.Context, dryRun bool, forcePush bool) error {
	observed, current, err := r.collect(ctx)
	if err != nil {
		return err
	}

	desired := provider.DesiredState{
		Name:       r.cfg.Record.Name,
		TTLSeconds: r.cfg.Record.TTLSeconds,
		IPv4:       observed.IPv4,
		IPv6:       observed.IPv6,
		Options:    r.desiredOptions,
	}

	plan, err := provider.BuildSingleAddressPlan(current, desired)
	if err != nil {
		return err
	}
	if r.logger.Enabled(ctx, slog.LevelDebug) {
		r.logState(observed, current, plan)
	}

	if plan.IsNoop() {
		if forcePush {
			plan = forcePushPlan(current, desired)
			if plan.IsNoop() {
				r.logger.Debug("records already match current egress IPs")
				return nil
			}
			r.logger.Debug("forcing provider update despite unchanged DNS state")
		} else {
			r.logger.Debug("records already match current egress IPs")
			return nil
		}
	}

	if dryRun {
		r.logger.Info("dry run: planned provider operations", "operations", strings.Join(plan.Summaries(), "; "))
		return nil
	}

	if err := r.provider.Apply(ctx, plan); err != nil {
		return fmt.Errorf("apply update: %w", err)
	}
	r.logger.Info("applied DNS update", "operations", strings.Join(plan.Summaries(), "; "))

	verified, err := r.provider.ReadState(ctx, r.cfg.Record.Name)
	if err != nil {
		return fmt.Errorf("verify updated records: %w", err)
	}
	if err := provider.VerifySingleAddressState(verified, desired); err != nil {
		return err
	}

	r.logger.Debug("verified DNS state after update")
	return nil
}

func forcePushPlan(current provider.State, desired provider.DesiredState) provider.Plan {
	operations := make([]provider.Operation, 0, 2)
	operations = append(operations, forcePushOperation(current.ByType(provider.RecordTypeA), desired.Name, provider.RecordTypeA, desired.IPv4, desired.TTLSeconds, desired.Options)...)
	operations = append(operations, forcePushOperation(current.ByType(provider.RecordTypeAAAA), desired.Name, provider.RecordTypeAAAA, desired.IPv6, desired.TTLSeconds, desired.Options)...)
	return provider.Plan{Operations: operations}
}

func forcePushOperation(current []provider.Record, name string, recordType provider.RecordType, desired *netip.Addr, ttlSeconds uint32, options provider.RecordOptions) []provider.Operation {
	if desired == nil || len(current) == 0 {
		return nil
	}

	return []provider.Operation{{
		Kind:    provider.OperationUpdate,
		Current: current[0],
		Desired: provider.Record{
			Name:       name,
			Type:       recordType,
			Content:    desired.String(),
			TTLSeconds: ttlSeconds,
			Options:    options,
		},
	}}
}

type observedState struct {
	IPv4 *netip.Addr
	IPv6 *netip.Addr
}

func (r *Runner) collect(ctx context.Context) (observedState, provider.State, error) {
	group, ctx := errgroup.WithContext(ctx)

	var (
		observed observedState
		current  provider.State
		ipv4Err  error
		ipv6Err  error
	)

	group.Go(func() error {
		address, err := r.prober.Lookup(ctx, r.cfg.Probe.IPv4URL, egress.IPv4)
		ipv4Err = err
		if err == nil {
			observed.IPv4 = address
		}
		return nil
	})

	group.Go(func() error {
		address, err := r.prober.Lookup(ctx, r.cfg.Probe.IPv6URL, egress.IPv6)
		ipv6Err = err
		if err == nil {
			observed.IPv6 = address
		}
		return nil
	})

	group.Go(func() error {
		state, err := r.provider.ReadState(ctx, r.cfg.Record.Name)
		if err != nil {
			return fmt.Errorf("read current provider state: %w", err)
		}
		current = state
		return nil
	})

	if err := group.Wait(); err != nil {
		return observedState{}, provider.State{}, err
	}

	if ipv4Err != nil && ipv6Err != nil {
		return observedState{}, provider.State{}, fmt.Errorf("all egress probes failed: IPv4: %v, IPv6: %v", ipv4Err, ipv6Err)
	}

	return observed, current, nil
}

func formatDesired(address *netip.Addr) string {
	if address == nil {
		return "none"
	}
	return address.String()
}

func formatRecords(records []provider.Record) string {
	if len(records) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(records))
	for _, record := range records {
		parts = append(parts, record.Content)
	}
	return strings.Join(parts, ",")
}

func (r *Runner) logState(observed observedState, current provider.State, plan provider.Plan) {
	r.logger.Debug(
		"evaluated DNS state",
		"observed_ipv4", formatDesired(observed.IPv4),
		"observed_ipv6", formatDesired(observed.IPv6),
		"current_ipv4", formatRecords(current.ByType(provider.RecordTypeA)),
		"current_ipv6", formatRecords(current.ByType(provider.RecordTypeAAAA)),
		"current_cname", formatRecords(current.ByType(provider.RecordTypeCNAME)),
		"operations", strings.Join(plan.Summaries(), "; "),
	)
}
