// Package httpclient constructs hardened HTTP clients for probes and APIs.
//
// The clients use bounded timeouts, explicit transport settings, and a fixed
// user agent so outbound requests behave consistently and predictably.
package httpclient
