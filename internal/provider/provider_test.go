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
					Options:    RecordOptions{Proxy: boolPtr(true)},
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

func TestBuildSingleAddressPlanNoop(t *testing.T) {
	t.Parallel()

	ipv4 := mustAddr(t, "198.51.100.10")
	ipv6 := mustAddr(t, "2001:db8::10")

	plan, err := BuildSingleAddressPlan(
		State{
			Name: "host.example.com.",
			Records: []Record{
				{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
				{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: ipv6.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
			},
		},
		DesiredState{
			Name:       "host.example.com.",
			TTLSeconds: 300,
			IPv4:       &ipv4,
			IPv6:       &ipv6,
			Options:    RecordOptions{Proxy: boolPtr(false)},
		},
	)
	if err != nil {
		t.Fatalf("BuildSingleAddressPlan() error = %v", err)
	}
	if diff := cmp.Diff(Plan{}, plan); diff != "" {
		t.Fatalf("BuildSingleAddressPlan() mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildSingleAddressPlanVariants(t *testing.T) {
	t.Parallel()

	currentIPv4 := mustAddr(t, "198.51.100.10")
	desiredIPv4 := mustAddr(t, "198.51.100.20")

	t.Run("delete duplicate and update", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildSingleAddressPlan(
			State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: currentIPv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
					{ID: "a2", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.11", TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
				},
			},
			DesiredState{
				Name:       "host.example.com.",
				TTLSeconds: 300,
				IPv4:       &desiredIPv4,
				Options:    RecordOptions{Proxy: boolPtr(false)},
			},
		)
		if err != nil {
			t.Fatalf("BuildSingleAddressPlan() error = %v", err)
		}
		if got, want := len(plan.Operations), 2; got != want {
			t.Fatalf("len(plan.Operations) = %d, want %d", got, want)
		}
	})

	t.Run("create missing record", func(t *testing.T) {
		t.Parallel()

		plan, err := BuildSingleAddressPlan(
			State{Name: "host.example.com."},
			DesiredState{
				Name:       "host.example.com.",
				TTLSeconds: 300,
				IPv4:       &desiredIPv4,
				Options:    RecordOptions{Proxy: boolPtr(false)},
			},
		)
		if err != nil {
			t.Fatalf("BuildSingleAddressPlan() error = %v", err)
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

		plan, err := BuildSingleAddressPlan(
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
		)
		if err != nil {
			t.Fatalf("BuildSingleAddressPlan() error = %v", err)
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

		_, err := BuildSingleAddressPlan(
			State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "c1", Name: "host.example.com.", Type: RecordTypeCNAME, Content: "other.example.com.", TTLSeconds: 300},
				},
			},
			DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &desiredIPv4, Options: RecordOptions{Proxy: boolPtr(false)}},
		)
		if err == nil {
			t.Fatal("BuildSingleAddressPlan() error = nil, want CNAME rejection")
		}
	})
}

func TestVerifySingleAddressState(t *testing.T) {
	t.Parallel()

	ipv4 := mustAddr(t, "198.51.100.10")
	ipv6 := mustAddr(t, "2001:db8::10")

	if err := VerifySingleAddressState(
		State{
			Name: "host.example.com.",
			Records: []Record{
				{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
				{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: ipv6.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
			},
		},
		DesiredState{
			Name:       "host.example.com.",
			TTLSeconds: 300,
			IPv4:       &ipv4,
			IPv6:       &ipv6,
			Options:    RecordOptions{Proxy: boolPtr(false)},
		},
	); err != nil {
		t.Fatalf("VerifySingleAddressState() error = %v", err)
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
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: boolPtr(false)}},
		},
		{
			name:    "missing ipv4",
			state:   State{Name: "host.example.com."},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: boolPtr(false)}},
		},
		{
			name: "duplicate ipv4",
			state: State{
				Name: "host.example.com.",
				Records: []Record{
					{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
					{ID: "a2", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.11", TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
				},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: boolPtr(false)}},
		},
		{
			name: "ttl mismatch",
			state: State{
				Name:    "host.example.com.",
				Records: []Record{{ID: "a1", Name: "host.example.com.", Type: RecordTypeA, Content: ipv4.String(), TTLSeconds: 600, Options: RecordOptions{Proxy: boolPtr(false)}}},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv4: &ipv4, Options: RecordOptions{Proxy: boolPtr(false)}},
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
				Records: []Record{{ID: "aaaa1", Name: "host.example.com.", Type: RecordTypeAAAA, Content: "2001:db8::20", TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}}},
			},
			desired: DesiredState{Name: "host.example.com.", TTLSeconds: 300, IPv6: &ipv6, Options: RecordOptions{Proxy: boolPtr(false)}},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := VerifySingleAddressState(test.state, test.desired); err == nil {
				t.Fatal("VerifySingleAddressState() error = nil, want non-nil")
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	if !recordOptionsEqual(RecordOptions{}, RecordOptions{}) {
		t.Fatal("recordOptionsEqual({}, {}) = false, want true")
	}
	if recordOptionsEqual(RecordOptions{Proxy: boolPtr(true)}, RecordOptions{}) {
		t.Fatal("recordOptionsEqual(non-zero, zero) = true, want false")
	}
	if recordOptionsEqual(RecordOptions{Proxy: boolPtr(true)}, RecordOptions{Proxy: boolPtr(false)}) {
		t.Fatal("recordOptionsEqual(true, false) = true, want false")
	}
	if !recordOptionsEqual(RecordOptions{Proxy: boolPtr(true)}, RecordOptions{Proxy: boolPtr(true)}) {
		t.Fatal("recordOptionsEqual(true, true) = false, want true")
	}

	if got := cloneRecordOptions(RecordOptions{}); got.Proxy != nil {
		t.Fatalf("cloneRecordOptions(zero) = %+v, want zero value", got)
	}
	if got := cloneRecordOptions(RecordOptions{Proxy: boolPtr(true)}); got.Proxy == nil || !*got.Proxy {
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
	if got := formatOptions(RecordOptions{Proxy: boolPtr(true)}); got != " proxy=true" {
		t.Fatalf("formatOptions(true) = %q, want proxy string", got)
	}
	if got := formatProxyValue(nil); got != "unset" {
		t.Fatalf("formatProxyValue(nil) = %q, want unset", got)
	}
	if got := formatProxyValue(boolPtr(false)); got != "false" {
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
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(true)}},
		Record{Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.10", TTLSeconds: 300, Options: RecordOptions{Proxy: boolPtr(false)}},
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
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1", Options: RecordOptions{Proxy: boolPtr(true)}},
			{ID: "a", Name: "host.example.com.", Type: RecordTypeA, Content: "198.51.100.1", Options: RecordOptions{Proxy: boolPtr(false)}},
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
			{Kind: OperationCreate, Desired: Record{Content: "x", Options: RecordOptions{Proxy: boolPtr(true)}}},
			{Kind: OperationCreate, Desired: Record{Content: "x", Options: RecordOptions{Proxy: boolPtr(false)}}},
		}
		sortOperations(ops)
		// " proxy=false" < " proxy=true"
		if got := ops[0].Desired.Options.Proxy; got == nil || *got != false {
			t.Fatalf("sortOperations options: expected proxy=false first")
		}
	})
}

func boolPtr(value bool) *bool {
	return &value
}

func mustAddr(t *testing.T, value string) netip.Addr {
	t.Helper()

	address, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("netip.ParseAddr(%q) error = %v", value, err)
	}
	return address
}
