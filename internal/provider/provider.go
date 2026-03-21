package provider

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// RecordType identifies a DNS record type relevant to this application.
type RecordType string

const (
	// RecordTypeA identifies an IPv4 address record.
	RecordTypeA RecordType = "A"
	// RecordTypeAAAA identifies an IPv6 address record.
	RecordTypeAAAA RecordType = "AAAA"
	// RecordTypeCNAME identifies a canonical-name alias record.
	RecordTypeCNAME RecordType = "CNAME"
)

// Record represents a DNS record returned by a provider API.
type Record struct {
	ID         string
	Name       string
	Type       RecordType
	Content    string
	TTLSeconds uint32
	Options    RecordOptions
}

// State is the current provider-side DNS state for a single hostname.
type State struct {
	Name    string
	Records []Record
}

// RecordOptions holds provider-agnostic record behaviors supported by this tool.
type RecordOptions struct {
	// Proxy requests proxying when the provider supports it.
	Proxy *bool
}

// DesiredState is the target DNS state derived from the current egress IPs.
type DesiredState struct {
	Name       string
	TTLSeconds uint32
	IPv4       *netip.Addr
	IPv6       *netip.Addr
	Options    RecordOptions
}

// RecordSelection identifies which managed address-record families to target.
type RecordSelection uint8

const (
	// RecordSelectionNone selects no address-record families.
	RecordSelectionNone RecordSelection = iota
	// RecordSelectionA selects only A records.
	RecordSelectionA
	// RecordSelectionAAAA selects only AAAA records.
	RecordSelectionAAAA
	// RecordSelectionBoth selects both A and AAAA records.
	RecordSelectionBoth
)

// String returns the stable CLI-facing name for the selection.
func (s RecordSelection) String() string {
	switch s {
	case RecordSelectionA:
		return "a"
	case RecordSelectionAAAA:
		return "aaaa"
	case RecordSelectionBoth:
		return "both"
	default:
		return ""
	}
}

// Includes reports whether the selection targets the given record type.
func (s RecordSelection) Includes(recordType RecordType) bool {
	switch s {
	case RecordSelectionA:
		return recordType == RecordTypeA
	case RecordSelectionAAAA:
		return recordType == RecordTypeAAAA
	case RecordSelectionBoth:
		return recordType == RecordTypeA || recordType == RecordTypeAAAA
	default:
		return false
	}
}

// OperationKind identifies a single planned DNS mutation.
type OperationKind string

const (
	// OperationCreate creates a missing record.
	OperationCreate OperationKind = "create"
	// OperationUpdate updates an existing record in place.
	OperationUpdate OperationKind = "update"
	// OperationDelete removes an existing record.
	OperationDelete OperationKind = "delete"
)

// Operation is one provider-agnostic DNS mutation.
type Operation struct {
	Kind    OperationKind
	Current Record
	Desired Record
}

// Plan is the provider-agnostic set of changes needed to reach the target state.
type Plan struct {
	Operations []Operation
}

// Provider is implemented by a concrete DNS backend.
type Provider interface {
	ReadState(ctx context.Context, name string) (State, error)
	Apply(ctx context.Context, plan Plan) error
}

// IsNoop reports whether the plan is empty.
func (p Plan) IsNoop() bool {
	return len(p.Operations) == 0
}

// Summaries returns stable human-readable descriptions of the plan.
func (p Plan) Summaries() []string {
	summaries := make([]string, 0, len(p.Operations))
	for _, operation := range p.Operations {
		switch operation.Kind {
		case OperationCreate:
			summaries = append(summaries, fmt.Sprintf(
				"create %s %s -> %s ttl=%d%s",
				operation.Desired.Type,
				operation.Desired.Name,
				operation.Desired.Content,
				operation.Desired.TTLSeconds,
				formatOptions(operation.Desired.Options),
			))
		case OperationUpdate:
			summaries = append(summaries, fmt.Sprintf(
				"update %s %s id=%s %s -> %s ttl=%d -> %d proxy=%s -> %s",
				operation.Current.Type,
				operation.Current.Name,
				operation.Current.ID,
				operation.Current.Content,
				operation.Desired.Content,
				operation.Current.TTLSeconds,
				operation.Desired.TTLSeconds,
				formatProxyValue(operation.Current.Options.Proxy),
				formatProxyValue(operation.Desired.Options.Proxy),
			))
		case OperationDelete:
			summaries = append(summaries, fmt.Sprintf(
				"delete %s %s id=%s content=%s",
				operation.Current.Type,
				operation.Current.Name,
				operation.Current.ID,
				operation.Current.Content,
			))
		}
	}
	sort.Strings(summaries)
	return summaries
}

// BuildSingleAddressPlan reconciles A and AAAA records so that each family has
// either exactly one desired record or no records.
func BuildSingleAddressPlan(current State, desired DesiredState) (Plan, error) {
	if targets := current.CNAMETargets(); len(targets) > 0 {
		return Plan{}, fmt.Errorf(
			"managed name %q has CNAME records: %s",
			desired.Name,
			strings.Join(targets, ", "),
		)
	}

	var operations []Operation
	operations = append(operations, buildTypePlan(current.ByType(RecordTypeA), desired.Name, RecordTypeA, desired.IPv4, desired.TTLSeconds, desired.Options)...)
	operations = append(operations, buildTypePlan(current.ByType(RecordTypeAAAA), desired.Name, RecordTypeAAAA, desired.IPv6, desired.TTLSeconds, desired.Options)...)
	return Plan{Operations: operations}, nil
}

// BuildDeletePlan removes only the selected A/AAAA record families and leaves
// all other provider records untouched.
func BuildDeletePlan(current State, selection RecordSelection) (Plan, error) {
	if selection == RecordSelectionNone {
		return Plan{}, fmt.Errorf("delete selection must target at least one record family")
	}

	var operations []Operation
	for _, recordType := range []RecordType{RecordTypeA, RecordTypeAAAA} {
		if !selection.Includes(recordType) {
			continue
		}
		for _, record := range current.ByType(recordType) {
			operations = append(operations, Operation{
				Kind:    OperationDelete,
				Current: record,
			})
		}
	}

	sortOperations(operations)
	return Plan{Operations: operations}, nil
}

// VerifySingleAddressState checks that the provider-side state exactly matches
// the desired A/AAAA state and that no conflicting CNAME exists.
func VerifySingleAddressState(state State, desired DesiredState) error {
	if targets := state.CNAMETargets(); len(targets) > 0 {
		return fmt.Errorf("managed name %q still has CNAME records: %s", desired.Name, strings.Join(targets, ", "))
	}

	if err := verifyTypeState(state.ByType(RecordTypeA), RecordTypeA, desired.IPv4, desired.TTLSeconds, desired.Options); err != nil {
		return err
	}
	if err := verifyTypeState(state.ByType(RecordTypeAAAA), RecordTypeAAAA, desired.IPv6, desired.TTLSeconds, desired.Options); err != nil {
		return err
	}
	return nil
}

// VerifyDeletedTypes checks that the selected A/AAAA record families are absent.
func VerifyDeletedTypes(state State, selection RecordSelection) error {
	if selection == RecordSelectionNone {
		return fmt.Errorf("delete selection must target at least one record family")
	}

	for _, recordType := range []RecordType{RecordTypeA, RecordTypeAAAA} {
		if !selection.Includes(recordType) {
			continue
		}
		if err := verifyTypeState(state.ByType(recordType), recordType, nil, 0, RecordOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// ByType returns a stable copy of records with the requested type.
func (s State) ByType(recordType RecordType) []Record {
	records := make([]Record, 0, len(s.Records))
	for _, record := range s.Records {
		if record.Type != recordType {
			continue
		}
		records = append(records, record)
	}
	sortRecords(records)
	return records
}

// CNAMETargets returns the current CNAME targets for the managed name.
func (s State) CNAMETargets() []string {
	records := s.ByType(RecordTypeCNAME)
	if len(records) == 0 {
		return nil
	}

	targets := make([]string, 0, len(records))
	for _, record := range records {
		targets = append(targets, record.Content)
	}
	return targets
}

func buildTypePlan(current []Record, name string, recordType RecordType, desired *netip.Addr, ttlSeconds uint32, options RecordOptions) []Operation {
	if desired == nil {
		operations := make([]Operation, 0, len(current))
		for _, record := range current {
			operations = append(operations, Operation{
				Kind:    OperationDelete,
				Current: record,
			})
		}
		return operations
	}

	desiredRecord := Record{
		Name:       name,
		Type:       recordType,
		Content:    desired.String(),
		TTLSeconds: ttlSeconds,
		Options:    cloneRecordOptions(options),
	}

	if len(current) == 0 {
		return []Operation{{
			Kind:    OperationCreate,
			Desired: desiredRecord,
		}}
	}

	keepIndex := -1
	for index, record := range current {
		if recordMatchesDesired(record, desiredRecord) {
			keepIndex = index
			break
		}
	}
	if keepIndex < 0 {
		// No existing record matches; fall back to updating the first one.
		keepIndex = 0
	}

	operations := make([]Operation, 0, len(current))
	for index, record := range current {
		if index == keepIndex {
			continue
		}
		operations = append(operations, Operation{
			Kind:    OperationDelete,
			Current: record,
		})
	}

	keep := current[keepIndex]
	if !recordMatchesDesired(keep, desiredRecord) {
		operations = append(operations, Operation{
			Kind:    OperationUpdate,
			Current: keep,
			Desired: desiredRecord,
		})
	}

	sortOperations(operations)
	return operations
}

func verifyTypeState(current []Record, recordType RecordType, desired *netip.Addr, ttlSeconds uint32, options RecordOptions) error {
	if desired == nil {
		if len(current) != 0 {
			return fmt.Errorf("%s verification failed: expected none, got %s", recordType, describeRecords(current))
		}
		return nil
	}

	if len(current) != 1 {
		return fmt.Errorf("%s verification failed: expected one record for %s, got %s", recordType, desired.String(), describeRecords(current))
	}

	expected := Record{
		Type:       recordType,
		Content:    desired.String(),
		TTLSeconds: ttlSeconds,
		Options:    cloneRecordOptions(options),
	}
	if !recordMatchesDesired(current[0], expected) {
		return fmt.Errorf("%s verification failed: expected %s ttl=%d%s, got %s", recordType, expected.Content, ttlSeconds, formatOptions(options), describeRecords(current))
	}
	return nil
}

func recordMatchesDesired(current Record, desired Record) bool {
	if desired.Name != "" && NormalizeName(current.Name) != NormalizeName(desired.Name) {
		return false
	}
	if current.Type != desired.Type {
		return false
	}
	if current.Content != desired.Content {
		return false
	}
	if current.TTLSeconds != desired.TTLSeconds {
		return false
	}
	return recordOptionsEqual(current.Options, desired.Options)
}

// NormalizeName lowercases name, trims whitespace, and strips any trailing dot.
func NormalizeName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

func cloneRecordOptions(options RecordOptions) RecordOptions {
	cloned := RecordOptions{}
	if options.Proxy != nil {
		value := *options.Proxy
		cloned.Proxy = &value
	}
	return cloned
}

func recordOptionsEqual(left RecordOptions, right RecordOptions) bool {
	switch {
	case left.Proxy == nil && right.Proxy == nil:
		return true
	case left.Proxy == nil || right.Proxy == nil:
		return false
	default:
		return *left.Proxy == *right.Proxy
	}
}

func describeRecords(records []Record) string {
	if len(records) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(records))
	for _, record := range records {
		parts = append(parts, fmt.Sprintf("%s ttl=%d%s", record.Content, record.TTLSeconds, formatOptions(record.Options)))
	}
	return strings.Join(parts, ", ")
}

func formatOptions(options RecordOptions) string {
	if options.Proxy == nil {
		return ""
	}
	return fmt.Sprintf(" proxy=%t", *options.Proxy)
}

func formatProxyValue(proxy *bool) string {
	if proxy == nil {
		return "unset"
	}
	return fmt.Sprintf("%t", *proxy)
}

func sortOperations(operations []Operation) {
	sort.Slice(operations, func(i, j int) bool {
		l, r := operations[i], operations[j]
		if l.Kind != r.Kind {
			return l.Kind < r.Kind
		}
		if l.Current.Type != r.Current.Type {
			return l.Current.Type < r.Current.Type
		}
		if l.Current.ID != r.Current.ID {
			return l.Current.ID < r.Current.ID
		}
		if l.Desired.Content != r.Desired.Content {
			return l.Desired.Content < r.Desired.Content
		}
		return formatOptions(l.Desired.Options) < formatOptions(r.Desired.Options)
	})
}

func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		l, r := records[i], records[j]
		if l.Type != r.Type {
			return l.Type < r.Type
		}
		ln, rn := NormalizeName(l.Name), NormalizeName(r.Name)
		if ln != rn {
			return ln < rn
		}
		if l.ID != r.ID {
			return l.ID < r.ID
		}
		if l.Content != r.Content {
			return l.Content < r.Content
		}
		return formatOptions(l.Options) < formatOptions(r.Options)
	})
}
