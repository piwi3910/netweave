// Package webhook provides shared HTTP client construction and SSRF-safe
// dialing primitives used by every outbound webhook delivery path
// (events.WebhookNotifier and workers.WebhookWorker).
//
// A single constructor centralizes the security-sensitive transport settings
// so that TLS floor, idle-connection limits, and DNS-rebinding defenses
// remain consistent across callers.
package webhook

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultMaxIdleConns is the total idle connection pool size across hosts.
	DefaultMaxIdleConns = 100

	// DefaultMaxIdleConnsPerHost bounds per-host pooled connections so a single
	// misbehaving webhook endpoint cannot exhaust file descriptors.
	DefaultMaxIdleConnsPerHost = 10

	// DefaultIdleConnTimeout is how long idle connections are kept alive.
	DefaultIdleConnTimeout = 90 * time.Second

	// DefaultDialTimeout caps TCP handshake duration per connection attempt.
	DefaultDialTimeout = 10 * time.Second

	// DefaultKeepAlive is the keep-alive probe interval for established
	// connections.
	DefaultKeepAlive = 30 * time.Second
)

// IPGuard decides whether a resolved IP is safe to dial. Implementations must
// be safe for concurrent use.
type IPGuard func(ip net.IP) bool

// IPResolver resolves a hostname to one or more IP addresses. It is satisfied
// by *net.Resolver and by test stubs used for DNS-rebinding simulation.
type IPResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DefaultIPGuard rejects loopback, link-local, and private (RFC 1918 / ULA)
// addresses, plus the IPv4/IPv6 cloud metadata service addresses.
func DefaultIPGuard(ip net.IP) bool {
	return !IsPrivateOrReservedIP(ip)
}

// ClientConfig holds options for constructing the shared webhook HTTP client.
type ClientConfig struct {
	// Timeout caps the full request lifetime (dial + TLS + body).
	Timeout time.Duration

	// MinTLSVersion sets the minimum negotiated TLS protocol version.
	// Defaults to tls.VersionTLS12 when zero.
	MinTLSVersion uint16

	// InsecureSkipVerify disables certificate validation. ONLY use for tests.
	InsecureSkipVerify bool

	// EnableMTLS, ClientCertFile, ClientKeyFile configure client-side mTLS.
	EnableMTLS     bool
	ClientCertFile string
	ClientKeyFile  string

	// CACertFile is an optional custom root CA bundle.
	CACertFile string

	// MaxIdleConns, MaxIdleConnsPerHost, IdleConnTimeout control connection
	// pooling. Zero values fall back to package defaults.
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration

	// Resolver is used for the delivery-time DNS re-resolution that closes the
	// DNS-rebinding TOCTOU window. Defaults to net.DefaultResolver.
	Resolver IPResolver

	// IPGuard decides whether a resolved IP is safe to dial. Defaults to
	// DefaultIPGuard which rejects loopback, private, link-local, and cloud
	// metadata addresses.
	IPGuard IPGuard

	// AllowPrivateNetworks disables the SSRF IP guard entirely. Intended for
	// unit/integration tests that drive httptest.NewServer on loopback.
	// Production callers MUST NOT set this field.
	AllowPrivateNetworks bool

	// DialTimeout bounds the TCP handshake. Zero falls back to
	// DefaultDialTimeout.
	DialTimeout time.Duration

	// KeepAlive is the keep-alive probe interval for established connections.
	// Zero falls back to DefaultKeepAlive.
	KeepAlive time.Duration
}

// NewHTTPClient constructs an *http.Client sharing the webhook transport
// defaults: TLS floor, idle-connection pooling, and a DNS-rebinding-safe
// DialContext that re-resolves each host at connect time and pins the TCP
// connect to a resolved, allow-listed IP.
//
// A nil cfg is treated as a zero-value config (secure defaults applied).
func NewHTTPClient(cfg *ClientConfig) (*http.Client, error) {
	if cfg == nil {
		cfg = &ClientConfig{}
	}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = DefaultMaxIdleConns
	}
	maxIdlePerHost := cfg.MaxIdleConnsPerHost
	if maxIdlePerHost <= 0 {
		maxIdlePerHost = DefaultMaxIdleConnsPerHost
	}
	idleTimeout := cfg.IdleConnTimeout
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleConnTimeout
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsCfg,
		MaxIdleConns:        maxIdle,
		MaxIdleConnsPerHost: maxIdlePerHost,
		IdleConnTimeout:     idleTimeout,
		DialContext:         NewSSRFSafeDialContext(cfg),
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}, nil
}

// buildTLSConfig resolves the TLS settings, applying defaults and loading any
// mTLS client certificate / custom root bundle.
func buildTLSConfig(cfg *ClientConfig) (*tls.Config, error) {
	minVersion := cfg.MinTLSVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	tlsCfg := &tls.Config{
		MinVersion: minVersion,
	}
	if cfg.InsecureSkipVerify {
		// Explicitly opt-in path: callers must set this only in tests. The
		// WebhookNotifier logs a loud warning before reaching this point.
		tlsCfg.InsecureSkipVerify = true
	}

	if cfg.EnableMTLS && cfg.ClientCertFile != "" && cfg.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if cfg.CACertFile != "" {
		caCert, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

// NewSSRFSafeDialContext returns a net/http DialContext that closes the
// DNS-rebinding TOCTOU window:
//
//  1. The hostname is re-resolved at connect time using cfg.Resolver.
//  2. Every returned address is checked against cfg.IPGuard; if ANY resolved
//     IP is banned, the dial fails immediately. This is strictly more
//     conservative than "first safe IP wins" and prevents a split DNS answer
//     from smuggling an internal IP past the guard.
//  3. The TCP connect is pinned to the resolved IP (no second OS-level
//     resolution happens between the check and the connect).
//
// A nil cfg is treated as a zero-value config (secure defaults applied).
func NewSSRFSafeDialContext(cfg *ClientConfig) func(context.Context, string, string) (net.Conn, error) {
	d := resolveDialDefaults(cfg)

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", addr, err)
		}
		if literal := net.ParseIP(host); literal != nil {
			return dialLiteralHost(ctx, d.dialer, d.guard, network, host, port, literal)
		}
		return dialResolvedHost(ctx, d.resolver, d.dialer, d.guard, network, host, port)
	}
}

// dialDefaults is the resolved set of values NewSSRFSafeDialContext uses to
// build its closure. Split into a struct to keep the constructor's cognitive
// complexity low without returning bare interface values from a helper.
type dialDefaults struct {
	resolver IPResolver
	guard    IPGuard
	dialer   *net.Dialer
}

// resolveDialDefaults folds the defaulting logic out of NewSSRFSafeDialContext.
func resolveDialDefaults(cfg *ClientConfig) dialDefaults {
	if cfg == nil {
		cfg = &ClientConfig{}
	}
	resolver := cfg.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	guard := cfg.IPGuard
	if guard == nil {
		guard = DefaultIPGuard
	}
	if cfg.AllowPrivateNetworks {
		guard = func(_ net.IP) bool { return true }
	}
	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = DefaultDialTimeout
	}
	keepAlive := cfg.KeepAlive
	if keepAlive <= 0 {
		keepAlive = DefaultKeepAlive
	}
	return dialDefaults{
		resolver: resolver,
		guard:    guard,
		dialer:   &net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive},
	}
}

// dialLiteralHost validates an IP-literal host against the SSRF guard and
// dials it, skipping the resolver altogether.
func dialLiteralHost(
	ctx context.Context,
	dialer *net.Dialer,
	guard IPGuard,
	network, host, port string,
	literal net.IP,
) (net.Conn, error) {
	if !guard(literal) {
		return nil, &SSRFBlockedError{Host: host, IP: literal}
	}
	conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(literal.String(), port))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", host, err)
	}
	return conn, nil
}

// dialResolvedHost re-resolves host at connect time, rejects if any resolved
// IP is banned, and pins the TCP connect to the first allow-listed IP so the
// OS does not re-resolve between the check and the handshake.
func dialResolvedHost(
	ctx context.Context,
	resolver IPResolver,
	dialer *net.Dialer,
	guard IPGuard,
	network, host, port string,
) (net.Conn, error) {
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve %q: no addresses returned", host)
	}
	// Reject if ANY resolved IP is banned, to prevent split-DNS smuggling.
	for _, ipAddr := range ips {
		if !guard(ipAddr.IP) {
			return nil, &SSRFBlockedError{Host: host, IP: ipAddr.IP}
		}
	}
	pinned := net.JoinHostPort(ips[0].IP.String(), port)
	conn, err := dialer.DialContext(ctx, network, pinned)
	if err != nil {
		return nil, fmt.Errorf("dial %s (pinned %s): %w", host, pinned, err)
	}
	return conn, nil
}

// SSRFBlockedError is returned when the DialContext refuses to connect to an
// IP rejected by the configured IPGuard.
type SSRFBlockedError struct {
	Host string
	IP   net.IP
}

// Error implements the error interface.
func (e *SSRFBlockedError) Error() string {
	return fmt.Sprintf(
		"webhook: refusing to connect to host %q: resolved IP %s is blocked by SSRF policy",
		e.Host, e.IP.String(),
	)
}
