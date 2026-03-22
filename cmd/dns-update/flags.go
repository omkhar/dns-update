package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"dns-update/internal/provider"
)

const (
	envConfigPath = "DNS_UPDATE_CONFIG"
	envDryRun     = "DNS_UPDATE_DRY_RUN"
	envVerbose    = "DNS_UPDATE_VERBOSE"
	envTimeout    = "DNS_UPDATE_TIMEOUT"
)

type cliFlagValues struct {
	configPath           string
	deleteSelection      provider.RecordSelection
	dryRun               bool
	forcePush            bool
	validateConfig       bool
	printEffectiveConfig bool
	verbose              bool
	timeout              time.Duration
}

type introspectionMode uint8

const (
	introspectionModeNone introspectionMode = iota
	introspectionModeValidateConfig
	introspectionModePrintEffectiveConfig
)

type runtimeOptions struct {
	configPath         string
	deleteSelection    provider.RecordSelection
	explicitConfigPath bool
	dryRun             bool
	forcePush          bool
	introspectionMode  introspectionMode
	verbose            bool
	timeout            time.Duration
}

func newFlagSet(stderr io.Writer) (*flag.FlagSet, *cliFlagValues) {
	values := &cliFlagValues{}

	flags := flag.NewFlagSet("dns-update", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&values.configPath, "config", "", "Path to the JSON configuration file.")
	flags.Var(newDeleteFlagValue(&values.deleteSelection), "delete", "Delete managed DNS records instead of reconciling to observed egress IPs. Use a, aaaa, or both; bare -delete deletes both.")
	flags.BoolVar(&values.dryRun, "dry-run", false, "Print planned changes without applying them.")
	flags.BoolVar(&values.forcePush, "force-push", false, "Force a provider update even when the observed DNS state already matches.")
	flags.BoolVar(&values.validateConfig, "validate-config", false, "Load and validate the assembled configuration, print a success message, and exit.")
	flags.BoolVar(&values.printEffectiveConfig, "print-effective-config", false, "Load and validate the assembled configuration, print it as JSON, and exit.")
	flags.BoolVar(&values.verbose, "verbose", false, "Enable debug logging.")
	flags.DurationVar(&values.timeout, "timeout", 0, "Maximum total runtime for one reconciliation cycle. 0 disables the global timeout.")

	return flags, values
}

func resolveRuntimeOptions(flags *flag.FlagSet, values cliFlagValues, lookupEnv func(string) (string, bool)) (runtimeOptions, error) {
	setFlags := visitedFlags(flags)
	configPath, explicitConfigPath := resolveConfigPath(values.configPath, setFlags["config"], lookupEnv)

	dryRun, err := resolveBoolFlag(values.dryRun, setFlags["dry-run"], lookupEnv, envDryRun, false)
	if err != nil {
		return runtimeOptions{}, err
	}

	verbose, err := resolveBoolFlag(values.verbose, setFlags["verbose"], lookupEnv, envVerbose, false)
	if err != nil {
		return runtimeOptions{}, err
	}

	timeout, err := resolveDurationFlag(values.timeout, setFlags["timeout"], lookupEnv, envTimeout, 0)
	if err != nil {
		return runtimeOptions{}, err
	}
	if timeout < 0 {
		return runtimeOptions{}, errors.New("timeout must be greater than or equal to 0")
	}
	if values.deleteSelection != provider.RecordSelectionNone && values.forcePush {
		return runtimeOptions{}, errors.New("delete and force-push are mutually exclusive")
	}
	if values.validateConfig && values.printEffectiveConfig {
		return runtimeOptions{}, errors.New("validate-config and print-effective-config are mutually exclusive")
	}

	mode := introspectionModeNone
	if values.validateConfig {
		mode = introspectionModeValidateConfig
	}
	if values.printEffectiveConfig {
		mode = introspectionModePrintEffectiveConfig
	}

	return runtimeOptions{
		configPath:         configPath,
		deleteSelection:    values.deleteSelection,
		explicitConfigPath: explicitConfigPath,
		dryRun:             dryRun,
		forcePush:          values.forcePush,
		introspectionMode:  mode,
		verbose:            verbose,
		timeout:            timeout,
	}, nil
}

func visitedFlags(flags *flag.FlagSet) map[string]bool {
	visited := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func resolveConfigPath(flagValue string, set bool, lookupEnv func(string) (string, bool)) (string, bool) {
	if set {
		return flagValue, true
	}
	if value, ok := lookupEnv(envConfigPath); ok {
		return value, true
	}
	return "", false
}

func resolveBoolFlag(flagValue bool, set bool, lookupEnv func(string) (string, bool), envName string, defaultValue bool) (bool, error) {
	if set {
		return flagValue, nil
	}
	rawValue, ok := lookupEnv(envName)
	if !ok {
		return defaultValue, nil
	}
	parsedValue, err := parseBoolValue(strings.TrimSpace(rawValue))
	if err != nil {
		return false, fmt.Errorf("%s: parse bool: %w", envName, err)
	}
	return parsedValue, nil
}

func resolveDurationFlag(flagValue time.Duration, set bool, lookupEnv func(string) (string, bool), envName string, defaultValue time.Duration) (time.Duration, error) {
	if set {
		return flagValue, nil
	}
	rawValue, ok := lookupEnv(envName)
	if !ok {
		return defaultValue, nil
	}
	parsedValue, err := time.ParseDuration(strings.TrimSpace(rawValue))
	if err != nil {
		return 0, fmt.Errorf("%s: parse duration: %w", envName, err)
	}
	return parsedValue, nil
}

func parseBoolValue(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "1", "t", "true", "y", "yes":
		return true, nil
	case "0", "f", "false", "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid syntax")
	}
}

type deleteFlagValue struct {
	selection *provider.RecordSelection
}

func newDeleteFlagValue(selection *provider.RecordSelection) *deleteFlagValue {
	return &deleteFlagValue{selection: selection}
}

func (v *deleteFlagValue) String() string {
	if v == nil || v.selection == nil {
		return ""
	}
	return v.selection.String()
}

func (v *deleteFlagValue) Set(value string) error {
	selection, err := parseDeleteSelection(value)
	if err != nil {
		return err
	}
	*v.selection = selection
	return nil
}

func (*deleteFlagValue) IsBoolFlag() bool {
	return true
}

func parseDeleteSelection(value string) (provider.RecordSelection, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true", "both":
		return provider.RecordSelectionBoth, nil
	case "a":
		return provider.RecordSelectionA, nil
	case "aaaa":
		return provider.RecordSelectionAAAA, nil
	case "false", "none":
		return provider.RecordSelectionNone, nil
	default:
		return provider.RecordSelectionNone, fmt.Errorf("invalid delete value %q (want a, aaaa, or both)", value)
	}
}
