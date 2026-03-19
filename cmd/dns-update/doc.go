// Package main provides the dns-update command.
//
// The binary reads configuration from flags, environment variables, and config
// files, then runs one reconciliation cycle to align A and AAAA records with
// the host's current egress addresses.
package main
