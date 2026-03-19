// Package retry provides bounded exponential backoff and retry decisions.
//
// It centralizes transient-failure handling so probes and provider requests use
// the same policy for delay, jitter, and retry limits.
package retry
