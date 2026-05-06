package provider

import (
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPlanHelpers(t *testing.T) {
	t.Parallel()

	plan := Plan{
		Operations: []Operation{
			{
				Kind: OperationDelete,
				Current: Record{
					ID:      "delete",
					Name:    "host.example.com.",
					Type:    RecordTypeA,
					Content: "198.51.100.10",
				},
			},
			{
				Kind: OperationUpdate,
				Current: Record{
					ID:      "update",
					Name:    "host.example.com.",
					Type:    RecordTypeAAAA,
					Content: "2001:db8::1",
				},
				Desired: Record{
					Name:       "host.example.com.",
					Type:       RecordTypeAAAA,
					Content:    "2001:db8::2",
					TTLSeconds: 300,
					Options:    RecordOptions{Proxy: new(true)},
				},
			},
			{
				Kind: OperationCreate,
				Desired: Record{
					Name:       "host.example.com.",
					Type:       RecordTypeA,
					Content:    "198.51.100.20",
					TTLSeconds: 300,
				},
			},
		},
	}

	if plan.IsNoop() {
		t.Fatal("IsNoop() = true, want false")
	}
	summaries := plan.Summaries()
	wantSummaries := []string{
		"create A host.example.com. -> 198.51.100.20 ttl=300",
		"delete A host.example.com. id=delete content=198.51.100.10",
		"update AAAA host.example.com. id=update 2001:db8::1 -> 2001:db8::2 ttl=0 -> 300 proxy=unset -> true",
	}
	if diff := cmp.Diff(wantSummaries, summaries); diff != "" {
		t.Fatalf("Summaries() mismatch (-want +got):\n%s", diff)
	}
	if !(Plan{}).IsNoop() {
		t.Fatal("empty plan IsNoop() = false, want true")
	}
}

func TestBuildObservedAddressPlanNoop(t *testing.T) {
	t.Parallel()

	ipv4 := mustAddr(t, "198.51.100.10")
	ipv6 := mustAddr(t, "2001:db8::10")

	plan, err := BuildObservedAddressPlan(
		State{
			Name: "host.example.com.",
			Records: []Record{
				{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
				{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: ipv6.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
			},
		},
		DesiredState{
			Name:       "host.example.com.",
			TTLSeconds: 300,
			IPv4:       &ipv4,
			IPv6:       &ipv6,
			Options:    RecordOptions{Proxy: new(false)},
		},
		ObservedFamiliesBoth,
	)
	if err != nil {
		t.Fatalf("BuildObservedAddressPlan() error = %v", err)
	}
	if diff := cmp.Diff(Plan{}, plan); diff != "" {
		t.Fatalf("BuildObservedAddressPlan() mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildObservedAddressPlanVariants(t *testing.T) {
	t.Parallel()

	currentIPv4 := mustAddr(t, "198.51.100.10")
	desiredIPv4 := mustAddr(t, "198.51.100.20")

	t.Run("delete duplicate and update", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildObservedAddressPlan(
			State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: currentIPv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
					{ID: "a2", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.11", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
				},
			},
			DesiredState{
				Name:       "host.example.com.",
				TTLSeconds: 300,
				IPv4:       &desiredIPv4,
				Options:    RecordOptions{Proxy: new(false)},
			},
			ObservedFamiliesIPv4,
		)
		if err != nil {
			t.Fatalf("BuildObservedAddressPlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 2; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
	})

	t.Run("create missing record", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildObservedAddressPlan(
			State{Name: "host.example.com."},
			DesiredState{
				Name:       "host.example.com.",
				TTLSeconds: 300,
				IPv4:       &desiredIPv4,
				Options:    RecordOptions{Proxy: new(false)},
			},
			ObservedFamiliesIPv4,
		)
		if err != nil {
			t.Fatalf("BuildObservedAddressPlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 1; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
		if got, want := plan.Operations[0].Kind, OperationCreate; got != want {
			t.Fatalf("plan.Operations[0].Kind = %q, want %q", got, want)
		}
	})

	t.Run("delete all when desired nil", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildObservedAddressPlan(
			State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: currentIPv4.String(), TTLSeconds: 300},
				},
			},
			DesiredState{
				Name:       "host.example.com.",
				TTLSeconds: 300,
			},
			ObservedFamiliesIPv4,
		)
		if err != nil {
			t.Fatalf("BuildObservedAddressPlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 1; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
		if got, want := plan.Operations[0].Kind, OperationDelete; got != want {
			t.Fatalf("plan.Operations[0].Kind = %q, want %q", got, want)
		}
	})

	t.Run("cname rejected", func(t *testing.T) {
		t.Parallel()

		_, err := BuildObservedAddressPlan(
			State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "c1", Name: "host.example.com.", Type: RecordTypeCNAME, Content: "other.example.com.", TTLSeconds: 300},
				},
			},
			DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &desiredIPv4, Options: RecordOptions{Proxy: new(false)}},
			ObservedFamiliesIPv4,
		)
		if err == nil {
			t.Fatal("BuildObservedAddressPlan() error = nil, want CNAME rejection")
		}
	})
}

func TestBuildObservedAddressPlan(t *testing.T) {
	t.Parallel()

	desiredIPv4 := mustAddr(t, "198.51.100.20")
	desiredIPv6 := mustAddr(t, "2001:db8::20")
	current := State{
		Name: "host.example.com.",
		Records: []Record{
			{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
			{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::10", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
		},
	}
	desired := DesiredState{
		Name:       "host.example.com.",
		TTLSeconds: 300,
		IPv4:       &desiredIPv4,
		IPv6:       &desiredIPv6,
		Options:    RecordOptions{Proxy: new(false)},
	}

	t.Run("a only", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildObservedAddressPlan(current, desired, ObservedFamiliesIPv4)
		if err != nil {
			t.Fatalf("BuildObservedAddressPlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 1; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
		if got, want := plan.Operations[0].Desired.Type, RecordTypeA; got != want {
			t.Fatalf("operation type = %q, want %q", got, want)
		}
		if got, want := plan.Operations[0].Desired.Content, desiredIPv4.String(); got != want {
			t.Fatalf("operation content = %q, want %q", got, want)
		}
	})

	t.Run("aaaa only", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildObservedAddressPlan(current, desired, ObservedFamiliesIPv6)
		if err != nil {
			t.Fatalf("BuildObservedAddressPlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 1; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
		if got, want := plan.Operations[0].Desired.Type, RecordTypeAAAA; got != want {
			t.Fatalf("operation type = %q, want %q", got, want)
		}
		if got, want := plan.Operations[0].Desired.Content, desiredIPv6.String(); got != want {
			t.Fatalf("operation content = %q, want %q", got, want)
		}
	})

	t.Run("observed family required", func(t *testing.T) {
		t.Parallel()

		if _, err := BuildObservedAddressPlan(current, desired, ObservedFamiliesNone); err == nil {
			t.Fatal("BuildObservedAddressPlan() error = nil, want non-nil")
		}
	})
}

func TestBuildDeletePlan(t *testing.T) {
	t.Parallel()

	current := State{
		Name: "host.example.com.",
		Records: []Record{
			{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
			{ID: "a2", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.11", TTLSeconds: 300},
			{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::10", TTLSeconds: 300},
			{ID: "c1", Name: "host.example.com.", Type: RecordTypeCNAME, Content: "other.example.com.", TTLSeconds: 300},
		},
	}

	t.Run("delete a only", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildDeletePlan(current, RecordSelectionA)
		if err != nil {
			t.Fatalf("BuildDeletePlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 2; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
		for _, operation := range plan.Operations {
			if got, want := operation.Kind, OperationDelete; got != want {
				t.Fatalf("operation.Kind = %q, want %q", got, want)
			}
			if got, want := operation.Current.Type, RecordTypeA; got != want {
				t.Fatalf("operation.Current.Type = %q, want %q", got, want)
			}
		}
	})

	t.Run("delete aaaa only", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildDeletePlan(current, RecordSelectionAAAA)
		if err != nil {
			t.Fatalf("BuildDeletePlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 1; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
		if got, want := plan.Operations[0].Current.Type, RecordTypeAAAA; got != want {
			t.Fatalf("operation.Current.Type = %q, want %q", got, want)
		}
	})

	t.Run("delete both ignores cname", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildDeletePlan(current, RecordSelectionBoth)
		if err != nil {
			t.Fatalf("BuildDeletePlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 3; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
	})

	t.Run("noop when records already absent", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildDeletePlan(State{Name: "host.example.com."}, RecordSelectionA)
		if err != nil {
			t.Fatalf("BuildDeletePlan() error = %v", err)
		}
		if !plan.IsNoop() {
			t.Fatalf("BuildDeletePlan() = %+v, want noop", plan)
		}
	})

	t.Run("selection required", func(t *testing.T) {
		t.Parallel()

		if _, err := BuildDeletePlan(current, RecordSelectionNone); err == nil {
			t.Fatal("BuildDeletePlan() error = nil, want non-nil")
		}
	})
}

func TestRecordSelectionHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		selection  RecordSelection
		recordA    bool
		recordAAAA bool
		want       string
	}{
		{name: "none", selection: RecordSelectionNone, want: ""},
		{name: "a", selection: RecordSelectionA, recordA: true, want: "a"},
		{name: "aaaa", selection: RecordSelectionAAAA, recordAAAA: true, want: "aaaa"},
		{name: "both", selection: RecordSelectionBoth, recordA: true, recordAAAA: true, want: "both"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got, want := test.selection.String(), test.want; got != want {
				t.Fatalf("selection.String() = %q, want %q", got, want)
			}
			if got, want := test.selection.Includes(RecordTypeA), test.recordA; got != want {
				t.Fatalf("selection.Includes(A) = %t, want %t", got, want)
			}
			if got, want := test.selection.Includes(RecordTypeAAAA), test.recordAAAA; got != want {
				t.Fatalf("selection.Includes(AAAA) = %t, want %t", got, want)
			}
			if test.selection.Includes(RecordTypeCNAME) {
				t.Fatal("selection.Includes(CNAME) = true, want false")
			}
		})
	}
}

func TestObservedFamiliesHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		observed   ObservedFamilies
		recordA    bool
		recordAAAA bool
		want       string
	}{
		{name: "none", observed: ObservedFamiliesNone, want: ""},
		{name: "ipv4", observed: ObservedFamiliesIPv4, recordA: true, want: "ipv4"},
		{name: "ipv6", observed: ObservedFamiliesIPv6, recordAAAA: true, want: "ipv6"},
		{name: "both", observed: ObservedFamiliesBoth, recordA: true, recordAAAA: true, want: "both"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got, want := test.observed.String(), test.want; got != want {
				t.Fatalf("observed.String() = %q, want %q", got, want)
			}
			if got, want := test.observed.Includes(RecordTypeA), test.recordA; got != want {
				t.Fatalf("observed.Includes(A) = %t, want %t", got, want)
			}
			if got, want := test.observed.Includes(RecordTypeAAAA), test.recordAAAA; got != want {
				t.Fatalf("observed.Includes(AAAA) = %t, want %t", got, want)
			}
			if test.observed.Includes(RecordTypeCNAME) {
				t.Fatal("observed.Includes(CNAME) = true, want false")
			}
		})
	}
}

func TestVerifyObservedAddressState(t *testing.T) {
	t.Parallel()

	ipv4 := mustAddr(t, "198.51.100.10")
	ipv6 := mustAddr(t, "2001:db8::10")

	if err := VerifyObservedAddressState(
		State{
			Name: "host.example.com.",
			Records: []Record{
				{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
				{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: ipv6.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
			},
		},
		DesiredState{
			Name:       "host.example.com.",
			TTLSeconds: 300,
			IPv4:       &ipv4,
			IPv6:       &ipv6,
			Options:    RecordOptions{Proxy: new(false)},
		},
		ObservedFamiliesBoth,
	); err != nil {
		t.Fatalf("VerifyObservedAddressState() error = %v", err)
	}

	tests := []struct {
		name    string
		state   State
		desired DesiredState
	}{
		{
			name: "cname",
			state: State{
				Name:    "host.example.com.",
				Records: []Record{{ID: "c1", Name: "host.example.com.", Type: RecordTypeCNAME, Content: "other.example.com.", TTLSeconds: 300}},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: new(false)}},
		},
		{
			name:    "missing ipv4",
			state:   State{Name: "host.example.com."},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: new(false)}},
		},
		{
			name: "duplicate ipv4",
			state: State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
					{ID: "a2", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.11", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
				},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: new(false)}},
		},
		{
			name: "ttl mismatch",
			state: State{
				Name:    "host.example.com.",
				Records: []Record{{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 600, Options: RecordOptions{Proxy: new(false)}}},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: new(false)}},
		},
		{
			name: "expected none",
			state: State{
				Name:    "host.example.com.",
				Records: []Record{{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300}},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300},
		},
		{
			name: "ipv6 mismatch",
			state: State{
				Name:    "host.example.com.",
				Records: []Record{{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::20", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}}},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv6: &ipv6, Options: RecordOptions{Proxy: new(false)}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := VerifyObservedAddressState(test.state, test.desired, ObservedFamiliesBoth); err == nil {
				t.Fatal("VerifyObservedAddressState() error = nil, want non-nil")
			}
		})
	}
}

func TestVerifyObservedAddressStateSelection(t *testing.T) {
	t.Parallel()

	ipv4 := mustAddr(t, "198.51.100.10")
	ipv6 := mustAddr(t, "2001:db8::10")
	state := State{
		Name: "host.example.com.",
		Records: []Record{
			{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
			{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::20", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
		},
	}
	desired := DesiredState{
		Name:       "host.example.com.",
		TTLSeconds: 300,
		IPv4:       &ipv4,
		IPv6:       &ipv6,
		Options:    RecordOptions{Proxy: new(false)},
	}

	if err := VerifyObservedAddressState(state, desired, ObservedFamiliesIPv4); err != nil {
		t.Fatalf("VerifyObservedAddressState(IPv4) error = %v", err)
	}
	if err := VerifyObservedAddressState(state, desired, ObservedFamiliesIPv6); err == nil {
		t.Fatal("VerifyObservedAddressState(IPv6) error = nil, want mismatch")
	}
	if err := VerifyObservedAddressState(state, desired, ObservedFamiliesNone); err == nil {
		t.Fatal("VerifyObservedAddressState(None) error = nil, want non-nil")
	}
}

func TestVerifyDeletedTypes(t *testing.T) {
	t.Parallel()

	if err := VerifyDeletedTypes(
		State{
			Name: "host.example.com.",
			Records: []Record{
				{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::10", TTLSeconds: 300},
				{ID: "c1", Name: "host.example.com.", Type: RecordTypeCNAME, Content: "other.example.com.", TTLSeconds: 300},
			},
		},
		RecordSelectionA,
	); err != nil {
		t.Fatalf("VerifyDeletedTypes() error = %v", err)
	}

	tests := []struct {
		name      string
		state     State
		selection RecordSelection
	}{
		{
			name:      "selection required",
			state:     State{Name: "host.example.com."},
			selection: RecordSelectionNone,
		},
		{
			name: "a still present",
			state: State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
				},
			},
			selection: RecordSelectionA,
		},
		{
			name: "aaaa still present when deleting both",
			state: State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::10", TTLSeconds: 300},
				},
			},
			selection: RecordSelectionBoth,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := VerifyDeletedTypes(test.state, test.selection); err == nil {
				t.Fatal("VerifyDeletedTypes() error = nil, want non-nil")
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	if !recordOptionsEqual(RecordOptions{}, RecordOptions{}) {
		t.Fatal("recordOptionsEqual({}, {}) = false, want true")
	}
	if recordOptionsEqual(RecordOptions{Proxy: new(true)}, RecordOptions{}) {
		t.Fatal("recordOptionsEqual(non-zero, zero) = true, want false")
	}
	if recordOptionsEqual(RecordOptions{Proxy: new(true)}, RecordOptions{Proxy: new(false)}) {
		t.Fatal("recordOptionsEqual(true, false) = true, want false")
	}
	if !recordOptionsEqual(RecordOptions{Proxy: new(true)}, RecordOptions{Proxy: new(true)}) {
		t.Fatal("recordOptionsEqual(true, true) = false, want true")
	}

	if got := cloneRecordOptions(RecordOptions{}); got.Proxy != nil {
		t.Fatalf("cloneRecordOptions(zero) = %+v, want zero value", got)
	}
	if got := cloneRecordOptions(RecordOptions{Proxy: new(true)}); got.Proxy == nil || !*got.Proxy {
		t.Fatalf("cloneRecordOptions() = %+v, want cloned proxy option", got)
	}

	if got, want := NormalizeName(" HOST.EXAMPLE.COM. "), "host.example.com"; got != want {
		t.Fatalf("NormalizeName() = %q, want %q", got, want)
	}
	if got := describeRecords(nil); got != "none" {
		t.Fatalf("describeRecords(nil) = %q, want none", got)
	}
	if got := formatOptions(RecordOptions{}); got != "" {
		t.Fatalf("formatOptions(zero) = %q, want empty", got)
	}
	if got := formatOptions(RecordOptions{Proxy: new(true)}); got != " proxy=true" {
		t.Fatalf("formatOptions(true) = %q, want proxy string", got)
	}
	if got := formatProxyValue(nil); got != "unset" {
		t.Fatalf("formatProxyValue(nil) = %q, want unset", got)
	}
	if got := formatProxyValue(new(false)); got != "false" {
		t.Fatalf("formatProxyValue(false) = %q, want false", got)
	}

	records := State{
		Name: "host.example.com.",
		Records: []Record{
			{ID: "b", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.20"},
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10"},
		},
	}.ByType(RecordTypeA)
	if got, want := records[0].ID, "a"; got != want {
		t.Fatalf("ByType() first ID = %q, want %q", got, want)
	}

	if !recordMatchesDesired(
		Record{Name: "HOST.EXAMPLE.COM.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
	) {
		t.Fatal("recordMatchesDesired() = false, want true")
	}
	if recordMatchesDesired(
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
		Record{Name: "other.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
	) {
		t.Fatal("recordMatchesDesired() = true, want false for mismatched names")
	}
	if recordMatchesDesired(
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300, Options: RecordOptions{Proxy: new(true)}},
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300, Options: RecordOptions{Proxy: new(false)}},
	) {
		t.Fatal("recordMatchesDesired() = true, want false for option mismatch")
	}
	if recordMatchesDesired(
		Record{Name: "host.example.com.", Type: RecordTypeAAAA, Content: "198.51.100.10", TTLSeconds: 300},
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
	) {
		t.Fatal("recordMatchesDesired() = true, want false for type mismatch")
	}
	if recordMatchesDesired(
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.11", TTLSeconds: 300},
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
	) {
		t.Fatal("recordMatchesDesired() = true, want false for content mismatch")
	}
	if recordMatchesDesired(
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 600},
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300},
	) {
		t.Fatal("recordMatchesDesired() = true, want false for TTL mismatch")
	}
}

func TestSortFunctions(t *testing.T) {
	t.Parallel()

	t.Run("sortRecords by type", func(t *testing.T) {
		t.Parallel()
		records := []Record{
			{ID: "1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::1"},
			{ID: "2", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1"},
		}
		sortRecords(records)
		if got, want := records[0].Type, RecordTypeA; got != want {
			t.Fatalf("sortRecords type: got %q, want %q", got, want)
		}
	})

	t.Run("sortRecords by name within same type", func(t *testing.T) {
		t.Parallel()
		records := []Record{
			{ID: "1", Name: "z.example.com.", Type: RecordTypeA, Content: "198.51.100.1"},
			{ID: "2", Name: "a.example.com.", Type: RecordTypeA, Content: "198.51.100.2"},
		}
		sortRecords(records)
		if got, want := records[0].Name, "a.example.com."; got != want {
			t.Fatalf("sortRecords name: got %q, want %q", got, want)
		}
	})

	t.Run("sortRecords by ID within same type and name", func(t *testing.T) {
		t.Parallel()
		records := []Record{
			{ID: "z", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1"},
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1"},
		}
		sortRecords(records)
		if got, want := records[0].ID, "a"; got != want {
			t.Fatalf("sortRecords ID: got %q, want %q", got, want)
		}
	})

	t.Run("sortRecords by content within same type, name, ID", func(t *testing.T) {
		t.Parallel()
		records := []Record{
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.2"},
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1"},
		}
		sortRecords(records)
		if got, want := records[0].Content, "198.51.100.1"; got != want {
			t.Fatalf("sortRecords content: got %q, want %q", got, want)
		}
	})

	t.Run("sortRecords by options within same type, name, ID, content", func(t *testing.T) {
		t.Parallel()
		records := []Record{
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1", Options: RecordOptions{Proxy: new(true)}},
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1", Options: RecordOptions{Proxy: new(false)}},
		}
		sortRecords(records)
		// " proxy=false" < " proxy=true" lexicographically
		if got := records[0].Options.Proxy; got == nil || *got != false {
			t.Fatalf("sortRecords options: expected proxy=false first")
		}
	})

	t.Run("sortOperations by Current.Type within same Kind", func(t *testing.T) {
		t.Parallel()
		ops := []Operation{
			{Kind: OperationDelete, Current: Record{Type: RecordTypeAAAA, ID: "1"}},
			{Kind: OperationDelete, Current: Record{Type: RecordTypeA, ID: "2"}},
		}
		sortOperations(ops)
		if got, want := ops[0].Current.Type, RecordTypeA; got != want {
			t.Fatalf("sortOperations type: got %q, want %q", got, want)
		}
	})

	t.Run("sortOperations by Current.ID within same Kind and Type", func(t *testing.T) {
		t.Parallel()
		ops := []Operation{
			{Kind: OperationDelete, Current: Record{Type: RecordTypeA, ID: "z"}},
			{Kind: OperationDelete, Current: Record{Type: RecordTypeA, ID: "a"}},
		}
		sortOperations(ops)
		if got, want := ops[0].Current.ID, "a"; got != want {
			t.Fatalf("sortOperations ID: got %q, want %q", got, want)
		}
	})

	t.Run("sortOperations by Desired.Content within same Kind, Type, ID", func(t *testing.T) {
		t.Parallel()
		ops := []Operation{
			{Kind: OperationCreate, Current: Record{Type: RecordTypeA, ID: ""}, Desired: Record{Content: "z"}},
			{Kind: OperationCreate, Current: Record{Type: RecordTypeA, ID: ""}, Desired: Record{Content: "a"}},
		}
		sortOperations(ops)
		if got, want := ops[0].Desired.Content, "a"; got != want {
			t.Fatalf("sortOperations content: got %q, want %q", got, want)
		}
	})

	t.Run("sortOperations by Desired.Options within same Kind, Type, ID, Content", func(t *testing.T) {
		t.Parallel()
		ops := []Operation{
			{Kind: OperationCreate, Desired: Record{Content: "x", Options: RecordOptions{Proxy: new(true)}}},
			{Kind: OperationCreate, Desired: Record{Content: "x", Options: RecordOptions{Proxy: new(false)}}},
		}
		sortOperations(ops)
		// " proxy=false" < " proxy=true"
		if got := ops[0].Desired.Options.Proxy; got == nil || *got != false {
			t.Fatalf("sortOperations options: expected proxy=false first")
		}
	})
}

func mustAddr(t *testing.T, value string) netip.Addr {
	t.Helper()

	address, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("netip.ParseAddr(%q) error = %v", value, err)
	}
	return address
}
