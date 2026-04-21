package server

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ValidateCallback validates a subscription callback URL at registration time.
// It performs early validation to provide fast failure before calling the
// adapter. Includes SSRF protection to prevent callbacks to localhost and
// private IP ranges.
//
// Defense-in-depth at delivery time: the DNS-rebinding TOCTOU gap between
// this check and the actual webhook delivery is closed by the SSRF-safe
// DialContext installed on the shared HTTP client — see
// internal/webhook.NewSSRFSafeDialContext. That dialer re-resolves the
// hostname on every delivery attempt, rejects the connect if any resolved
// IP falls into the banned set (loopback, private, link-local, cloud
// metadata), and pins the TCP connect to the allow-listed IP so the OS
// does not re-resolve between the check and the handshake.
func (s *Server) ValidateCallback(ctx context.Context, sub *adapter.Subscription) error {
	if sub == nil {
		return fmt.Errorf("subscription cannot be nil")
	}

	if sub.Callback == "" {
		return fmt.Errorf("callback URL is required")
	}

	// Parse URL to validate format
	parsedURL, err := url.Parse(sub.Callback)
	if err != nil {
		return fmt.Errorf("invalid callback URL format: %w", err)
	}

	// Validate scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("callback URL must use http or https scheme")
	}

	// Validate host
	if parsedURL.Host == "" {
		return fmt.Errorf("callback URL must have a valid host")
	}

	// SSRF Protection: Block localhost and private IP ranges
	// Skip SSRF protection if disabled in config (for testing only)
	if !s.config.SecurityCfg().DisableSSRFProtection {
		if err := ValidateCallbackHost(ctx, parsedURL.Hostname()); err != nil {
			return err
		}
	}

	return nil
}

// ValidateCallbackHost validates that the callback host is not localhost or a private IP address.
// This prevents SSRF (Server-Side Request Forgery) attacks.
func ValidateCallbackHost(ctx context.Context, hostname string) error {
	// Block localhost variations
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return fmt.Errorf("callback URL cannot be localhost")
	}

	// Attempt to resolve hostname to IP
	// If DNS lookup fails, we allow it - the actual webhook delivery will fail naturally
	// This prevents blocking valid hostnames that are temporarily unresolvable
	resolver := &net.Resolver{}
	ips, _ := resolver.LookupIPAddr(ctx, hostname)
	if len(ips) == 0 {
		// No IPs resolved (possibly due to DNS failure), allow it
		return nil
	}

	// Check if any resolved IP is in a private range
	for _, ipAddr := range ips {
		if IsPrivateIP(ipAddr.IP) {
			return fmt.Errorf("callback URL cannot be a private IP address")
		}
	}

	return nil
}
