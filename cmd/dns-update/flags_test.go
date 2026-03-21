package main

import (
	"testing"

	"dns-update/internal/provider"
)

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
		test := test
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
		test := test
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
