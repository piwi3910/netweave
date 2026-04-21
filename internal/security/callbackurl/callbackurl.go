// Package callbackurl centralizes validation of customer-supplied subscription
// callback URLs. It parses the URL, enforces an http/https scheme, requires a
// host, and — when DNS resolution succeeds — rejects any hostname whose
// resolved addresses fall into the SSRF-banned ranges (loopback, RFC1918
// private space, link-local including cloud metadata 169.254.169.254, CGNAT,
// multicast, IPv6 equivalents).
//
// This package is the single validation boundary for every site that accepts
// a callback URL from an untrusted API caller. It is used by:
//   - internal/server: request-time validation of new/updated subscriptions.
//   - internal/adapters/openstack: delivery-time validation on every webhook
//     send, providing defense-in-depth against stored URLs whose DNS resolution
//     may have drifted into private space since registration.
//
// The delivery-time TOCTOU gap between a validation call and the actual TCP
// connect is closed by the SSRF-safe DialContext in internal/webhook; callers
// delivering webhooks MUST use that HTTP client in addition to pre-validating
// here.
package callbackurl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/piwi3910/netweave/internal/webhook"
)

// ErrEmptyURL indicates a blank callback URL was supplied.
var ErrEmptyURL = errors.New("callback URL is required")

// ErrInvalidScheme indicates the callback URL did not use http or https.
var ErrInvalidScheme = errors.New("callback URL must use http or https scheme")

// ErrMissingHost indicates the callback URL had no host component.
var ErrMissingHost = errors.New("callback URL must have a valid host")

// ErrLoopbackHost indicates the callback hostname was a loopback literal.
var ErrLoopbackHost = errors.New("callback URL cannot target loopback")

// ErrPrivateAddress indicates the callback hostname resolved to a banned range.
var ErrPrivateAddress = errors.New("callback URL cannot target a private, loopback, link-local, or metadata IP")

// Resolver abstracts DNS lookup for testability. *net.Resolver satisfies it.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Options configures Validate behavior.
type Options struct {
	// AllowPrivateNetworks disables the SSRF IP guard. Intended ONLY for unit
	// and integration tests that exercise loopback endpoints. Production
	// callers MUST NOT set this field.
	AllowPrivateNetworks bool

	// Resolver overrides the DNS resolver. Defaults to net.DefaultResolver.
	Resolver Resolver
}

// Validate parses and validates a callback URL for use as an outbound webhook
// target. It enforces:
//
//   - non-empty URL;
//   - http or https scheme (gopher, file, ftp, javascript, etc. are rejected);
//   - non-empty host;
//   - hostname is not a loopback literal;
//   - when DNS resolves, NO resolved address falls into the SSRF-banned set
//     (loopback, RFC1918, link-local including cloud metadata, CGNAT,
//     multicast, or IPv6 equivalents).
//
// If DNS resolution returns no addresses or errors out, Validate allows the
// URL through: temporary resolution failures must not block legitimate
// endpoints, and the SSRF-safe DialContext in internal/webhook re-validates
// at connect time.
func Validate(ctx context.Context, rawURL string, opts Options) error {
	_, err := ValidateAndParse(ctx, rawURL, opts)
	return err
}

// ValidateAndParse runs the same checks as Validate and returns the parsed
// *url.URL on success. Callers that subsequently construct an *http.Request
// should prefer ValidateAndParse so the request is built from a URL value
// that static-analysis tooling can see has been validated, closing SSRF
// taint flows at the call site.
func ValidateAndParse(ctx context.Context, rawURL string, opts Options) (*url.URL, error) {
	parsed, err := parseAndCheckScheme(rawURL)
	if err != nil {
		return nil, err
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return nil, ErrMissingHost
	}

	if opts.AllowPrivateNetworks {
		return parsed, nil
	}

	if err := checkLiteralHost(hostname); err != nil {
		return nil, err
	}

	if err := checkResolvedHost(ctx, hostname, opts.Resolver); err != nil {
		return nil, err
	}
	return parsed, nil
}

// parseAndCheckScheme parses rawURL and enforces the http/https scheme and
// non-empty host requirements.
func parseAndCheckScheme(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, ErrEmptyURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid callback URL format: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrInvalidScheme
	}
	if parsed.Host == "" {
		return nil, ErrMissingHost
	}
	return parsed, nil
}

// checkLiteralHost rejects IP literals that resolve to banned ranges and
// rejects the classic loopback hostname "localhost".
func checkLiteralHost(hostname string) error {
	if hostname == "localhost" {
		return ErrLoopbackHost
	}
	if literal := net.ParseIP(hostname); literal != nil {
		if webhook.IsPrivateOrReservedIP(literal) {
			return fmt.Errorf("%w: %s", ErrPrivateAddress, literal.String())
		}
	}
	return nil
}

// checkResolvedHost performs a DNS lookup and rejects the URL if ANY resolved
// address is in a banned range. Missing/errored DNS is treated as pass-through:
// transient DNS failures must not break legitimate endpoints, and the dialer
// re-resolves and enforces the same policy at connect time.
func checkResolvedHost(ctx context.Context, hostname string, resolver Resolver) error {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return scanResolvedIPs(resolveIPs(ctx, resolver, hostname), hostname)
}

// resolveIPs performs the DNS lookup and returns the resolved addresses. DNS
// errors are intentionally coalesced into an empty slice — transient
// resolution failures must not break legitimate endpoints, and the SSRF-safe
// DialContext in internal/webhook re-resolves and enforces the policy at
// connect time.
func resolveIPs(ctx context.Context, resolver Resolver, hostname string) []net.IPAddr {
	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil
	}
	return ips
}

// scanResolvedIPs rejects the hostname if any resolved IP is in a banned
// range. An empty slice is treated as pass-through (see resolveIPs).
func scanResolvedIPs(ips []net.IPAddr, hostname string) error {
	for _, ip := range ips {
		if webhook.IsPrivateOrReservedIP(ip.IP) {
			return fmt.Errorf("%w: %s -> %s", ErrPrivateAddress, hostname, ip.IP.String())
		}
	}
	return nil
}
