package main

import (
	"bytes"
	"strings"
	"testing"

	"dns-update/internal/provider"
)

func TestFlagHelpUsesSimplifiedEnglish(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	flags, _ := newFlagSet(&output)
	flags.PrintDefaults()

	help := output.String()
	if strings.Contains(help, ";") {
		t.Fatalf("flag help contains a semicolon:\n%s", help)
	}
	for _, text := range []string{
		"Set the JSON config file path.",
		"Delete managed DNS records instead of reconciliation. Use a, aaaa, or both. Bare -delete deletes both.",
		"Print planned changes without applying them.",
		"Send a provider update for an existing address record that matches an observed address.",
		"Load and validate the assembled config. Print it as JSON. Exit.",
		"Set the maximum runtime for one reconciliation or delete cycle. Use 0 to disable the limit.",
		"Load and validate the assembled config. Print a success message. Exit.",
		"Enable debug logging.",
		"Print version information. Exit.",
	} {
		if !strings.Contains(help, text) {
			t.Errorf("flag help does not contain %q:\n%s", text, help)
		}
	}
}

func TestParseBoolValue(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		raw       string
		want      bool
		wantError bool
	}{
		{name: "true", raw: "true", want: true},
		{name: "false", raw: "false", want: false},
		{name: "invalid", raw: "maybe", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBoolValue(test.raw)
			if test.wantError {
				if err == nil {
					t.Fatal("parseBoolValue() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBoolValue() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseBoolValue() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestParseDeleteSelection(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		raw       string
		want      provider.RecordSelection
		wantError bool
	}{
		{name: "bare flag true means both", raw: "true", want: provider.RecordSelectionBoth},
		{name: "empty means both", raw: "", want: provider.RecordSelectionBoth},
		{name: "both", raw: "both", want: provider.RecordSelectionBoth},
		{name: "a", raw: "a", want: provider.RecordSelectionA},
		{name: "aaaa", raw: "aaaa", want: provider.RecordSelectionAAAA},
		{name: "false disables delete", raw: "false", want: provider.RecordSelectionNone},
		{name: "invalid", raw: "ipv4", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDeleteSelection(test.raw)
			if test.wantError {
				if err == nil {
					t.Fatal("parseDeleteSelection() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDeleteSelection() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseDeleteSelection() = %v, want %v", got, test.want)
			}
		})
	}
}
