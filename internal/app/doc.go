// Package app coordinates probes, DNS reads, plan construction, and updates.
//
// It owns one reconciliation cycle of the dns-update service and keeps the
// orchestration logic separate from provider-specific implementations.
package app
