// Package provider defines provider-agnostic DNS state and plans.
//
// Concrete backends adapt their APIs to this shared model so reconciliation
// logic can remain independent of any single DNS provider.
package provider
