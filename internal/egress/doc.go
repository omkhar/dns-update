// Package egress probes external endpoints to determine the host's current IPs.
//
// The package queries the IPv4 and IPv6 probe endpoints and validates the
// returned addresses before they are used to reconcile DNS records.
package egress
