package main

import "testing"

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
