package app

import (
	"context"
	"errors"
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

// RunOptions controls one reconciliation or delete cycle.
type RunOptions struct {
	DryRun    bool
	ForcePush bool
	Delete    provider.RecordSelection
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

// Run performs one egress-to-DNS reconciliation or delete cycle.
func (r *Runner) Run(ctx context.Context, options RunOptions) error {
	for attempt := 1; ; attempt++ {
		err := r.runOnce(ctx, options)
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

func (r *Runner) runOnce(ctx context.Context, options RunOptions) error {
	if options.Delete != provider.RecordSelectionNone {
		return r.runDeleteOnce(ctx, options)
	}
	return r.runReconcileOnce(ctx, options)
}

func (r *Runner) runReconcileOnce(ctx context.Context, options RunOptions) error {
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
		if options.ForcePush {
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

	if options.DryRun {
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

func (r *Runner) runDeleteOnce(ctx context.Context, options RunOptions) error {
	current, err := r.provider.ReadState(ctx, r.cfg.Record.Name)
	if err != nil {
		return fmt.Errorf("read current provider state: %w", err)
	}

	plan, err := provider.BuildDeletePlan(current, options.Delete)
	if err != nil {
		return err
	}
	if r.logger.Enabled(ctx, slog.LevelDebug) {
		r.logDeleteState(options.Delete, current, plan)
	}
	if plan.IsNoop() {
		r.logger.Debug("selected DNS records already absent", "delete", options.Delete.String())
		return nil
	}

	if options.DryRun {
		r.logger.Info("dry run: planned provider delete operations", "operations", strings.Join(plan.Summaries(), "; "))
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
	if err := provider.VerifyDeletedTypes(verified, options.Delete); err != nil {
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

	if err := collectProbeError(ipv4Err, ipv6Err); err != nil {
		return observedState{}, provider.State{}, err
	}

	return observed, current, nil
}

func collectProbeError(ipv4Err error, ipv6Err error) error {
	if ipv4Err == nil && ipv6Err == nil {
		return nil
	}
	if ipv4Err != nil && ipv6Err != nil {
		combined := fmt.Errorf(
			"all egress probes failed: %w",
			errors.Join(
				fmt.Errorf("IPv4 probe: %w", ipv4Err),
				fmt.Errorf("IPv6 probe: %w", ipv6Err),
			),
		)

		ipv4RetryAfter, ipv4Retryable := retry.After(ipv4Err)
		ipv6RetryAfter, ipv6Retryable := retry.After(ipv6Err)
		if ipv4Retryable && ipv6Retryable {
			retryAfter := ipv4RetryAfter
			if ipv6RetryAfter > retryAfter {
				retryAfter = ipv6RetryAfter
			}
			return retry.Mark(combined, retryAfter)
		}
		return combined
	}
	if ipv4Err != nil {
		return fmt.Errorf("IPv4 egress probe failed: %w", ipv4Err)
	}
	return fmt.Errorf("IPv6 egress probe failed: %w", ipv6Err)
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

func (r *Runner) logDeleteState(selection provider.RecordSelection, current provider.State, plan provider.Plan) {
	r.logger.Debug(
		"evaluated DNS delete state",
		"delete", selection.String(),
		"current_ipv4", formatRecords(current.ByType(provider.RecordTypeA)),
		"current_ipv6", formatRecords(current.ByType(provider.RecordTypeAAAA)),
		"current_cname", formatRecords(current.ByType(provider.RecordTypeCNAME)),
		"operations", strings.Join(plan.Summaries(), "; "),
	)
}
