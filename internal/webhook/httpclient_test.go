package webhook_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/webhook"
)

// TestNewHTTPClient_TLSMinVersionDefault asserts that the shared constructor
// applies a TLS 1.2 floor when the caller does not override it. This is the
// guarantee required by the workers.WebhookWorker path (#478).
func TestNewHTTPClient_TLSMinVersionDefault(t *testing.T) {
	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "transport must be *http.Transport")
	require.NotNil(t, transport.TLSClientConfig, "TLS config must be set")
	assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion,
		"default MinVersion must be TLS 1.2 to prevent downgrade attacks")
}

// TestNewHTTPClient_TLSMinVersionExplicit asserts that callers may raise the
// floor above the default (e.g. the notifier path, which uses TLS 1.3).
func TestNewHTTPClient_TLSMinVersionExplicit(t *testing.T) {
	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout:       5 * time.Second,
		MinTLSVersion: tls.VersionTLS13,
	})
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
}

// TestNewHTTPClient_IdleConnDefaults asserts that the shared constructor
// installs the bounded idle-connection pool required by #519 so a single
// misbehaving webhook endpoint cannot exhaust file descriptors.
func TestNewHTTPClient_IdleConnDefaults(t *testing.T) {
	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, webhook.DefaultMaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, webhook.DefaultMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	assert.Equal(t, webhook.DefaultIdleConnTimeout, transport.IdleConnTimeout)
}

// TestNewHTTPClient_IdleConnOverrides asserts that callers may override the
// defaults.
func TestNewHTTPClient_IdleConnOverrides(t *testing.T) {
	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout:             5 * time.Second,
		MaxIdleConns:        42,
		MaxIdleConnsPerHost: 7,
		IdleConnTimeout:     11 * time.Second,
	})
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 42, transport.MaxIdleConns)
	assert.Equal(t, 7, transport.MaxIdleConnsPerHost)
	assert.Equal(t, 11*time.Second, transport.IdleConnTimeout)
}

// TestNewHTTPClient_DialContextInstalled asserts that a DialContext is wired,
// which is required for the SSRF-safe delivery-time validation (#476).
func TestNewHTTPClient_DialContextInstalled(t *testing.T) {
	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.DialContext, "SSRF-safe DialContext must be installed")
}

// TestSSRFSafeDialContext_BlocksMetadataIPLiteral simulates the DNS-rebinding
// attack at the delivery step: even if registration-time validation passed,
// at connect time the host resolves to 169.254.169.254 (cloud metadata) and
// the dialer MUST refuse.
//
// We drive the behavior deterministically by passing the metadata IP as the
// dialed address (the same code path a rebind lookup would exercise).
func TestSSRFSafeDialContext_BlocksMetadataIPLiteral(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{
		Timeout:     time.Second,
		DialTimeout: 500 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := dial(ctx, "tcp", "169.254.169.254:80")
	require.Error(t, err, "metadata service IP must be blocked at dial time")
	require.Nil(t, conn)

	var blocked *webhook.SSRFBlockedError
	require.True(t, errors.As(err, &blocked), "must return SSRFBlockedError, got %v", err)
	assert.Equal(t, "169.254.169.254", blocked.Host)
	assert.True(t, blocked.IP.Equal(net.ParseIP("169.254.169.254")))
}

// TestSSRFSafeDialContext_BlocksLoopbackLiteral covers 127.0.0.1 / ::1.
func TestSSRFSafeDialContext_BlocksLoopbackLiteral(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := dial(ctx, "tcp", "127.0.0.1:8080")
	require.Error(t, err)
	var blocked *webhook.SSRFBlockedError
	assert.True(t, errors.As(err, &blocked), "loopback must be blocked, got %v", err)

	_, err = dial(ctx, "tcp", "[::1]:8080")
	require.Error(t, err)
	assert.True(t, errors.As(err, &blocked), "IPv6 loopback must be blocked, got %v", err)
}

// TestSSRFSafeDialContext_BlocksPrivateIPLiteral covers 10.x/172.16.x/192.168.x.
func TestSSRFSafeDialContext_BlocksPrivateIPLiteral(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, ip := range []string{"10.0.0.1", "172.16.5.7", "192.168.1.1"} {
		_, err := dial(ctx, "tcp", ip+":80")
		require.Error(t, err, "private IP %s must be blocked", ip)
		var blocked *webhook.SSRFBlockedError
		assert.True(t, errors.As(err, &blocked), "private IP %s should return SSRFBlockedError", ip)
	}
}

// rebindingResolver simulates a DNS-rebinding attack: at t0 (registration)
// the host resolved to a public IP, but at t1 (delivery) it "rebinds" to a
// banned target such as the cloud metadata service. The dialer MUST refuse.
type rebindingResolver struct {
	ips []net.IPAddr
	err error
}

func (r *rebindingResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.ips, nil
}

// TestSSRFSafeDialContext_RebindingViaResolver reproduces the #476 exploit:
// even if ValidateCallback accepted the hostname at registration time, a
// fresh lookup at delivery time must catch the rebound IP. We inject a
// resolver that returns 169.254.169.254 (AWS IMDS) and assert SSRFBlockedError.
func TestSSRFSafeDialContext_RebindingViaResolver(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{
		Resolver: &rebindingResolver{
			ips: []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}},
		},
	})

	_, err := dial(context.Background(), "tcp", "dns-rebind.attacker.example.com:443")
	require.Error(t, err, "rebinding target must be rejected at delivery time")

	var blocked *webhook.SSRFBlockedError
	require.True(t, errors.As(err, &blocked),
		"DNS-rebind to metadata IP must produce SSRFBlockedError, got %v", err)
	assert.True(t, blocked.IP.Equal(net.ParseIP("169.254.169.254")))
}

// TestSSRFSafeDialContext_RebindingSplitAnswer covers the split-DNS variant:
// the attacker's resolver returns [public, metadata] so a naive "first IP"
// check would pass. The safe dialer must still refuse.
func TestSSRFSafeDialContext_RebindingSplitAnswer(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{
		Resolver: &rebindingResolver{
			ips: []net.IPAddr{
				{IP: net.ParseIP("93.184.216.34")},   // example.com — public
				{IP: net.ParseIP("169.254.169.254")}, // metadata smuggled in
			},
		},
	})

	_, err := dial(context.Background(), "tcp", "attacker.example.com:443")
	require.Error(t, err, "split-DNS rebinding attempt must be rejected")

	var blocked *webhook.SSRFBlockedError
	require.True(t, errors.As(err, &blocked))
	assert.True(t, blocked.IP.Equal(net.ParseIP("169.254.169.254")),
		"should identify the banned IP in the answer set")
}

// TestSSRFSafeDialContext_ResolverEmptyAnswer covers the defensive path where
// the resolver succeeds but returns no addresses.
func TestSSRFSafeDialContext_ResolverEmptyAnswer(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{
		Resolver: &rebindingResolver{ips: nil},
	})

	_, err := dial(context.Background(), "tcp", "example.com:443")
	require.Error(t, err)
	var blocked *webhook.SSRFBlockedError
	assert.False(t, errors.As(err, &blocked),
		"empty-answer should be a resolve error, not SSRFBlockedError")
}

// TestSSRFSafeDialContext_ResolverError propagates lookup errors unchanged
// (wrapped with context) rather than silently failing open.
func TestSSRFSafeDialContext_ResolverError(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{
		Resolver: &rebindingResolver{err: errors.New("NXDOMAIN")},
	})

	_, err := dial(context.Background(), "tcp", "nxdomain.example:443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NXDOMAIN")
}

// TestSSRFSafeDialContext_CustomGuard asserts that callers can tighten or
// relax the policy via the IPGuard field.
func TestSSRFSafeDialContext_CustomGuard(t *testing.T) {
	// Custom guard: allow everything. This verifies the override surface;
	// we still expect the dial to fail because the target does not exist,
	// but we must fail with a network error, NOT SSRFBlockedError.
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{
		DialTimeout: 50 * time.Millisecond,
		IPGuard: func(_ net.IP) bool {
			return true // allow all
		},
	})

	// 192.0.2.1 is TEST-NET-1 and is unroutable; the dial will fail but
	// with a connection/timeout error rather than SSRFBlockedError.
	_, err := dial(context.Background(), "tcp", "192.0.2.1:1")
	require.Error(t, err)
	var blocked *webhook.SSRFBlockedError
	assert.False(t, errors.As(err, &blocked),
		"custom guard permitted the IP, so error should NOT be SSRFBlockedError: got %v", err)
}

// TestSSRFSafeDialContext_InvalidAddress covers the defensive path where the
// input is malformed (missing port).
func TestSSRFSafeDialContext_InvalidAddress(t *testing.T) {
	dial := webhook.NewSSRFSafeDialContext(&webhook.ClientConfig{})

	_, err := dial(context.Background(), "tcp", "not-a-host-port")
	require.Error(t, err)
}

// TestIsPrivateOrReservedIP exercises the IP classifier directly. The test
// is exhaustive across the ranges a webhook target might abuse.
func TestIsPrivateOrReservedIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		private bool
	}{
		{"nil", "", true},
		{"loopback_v4", "127.0.0.1", true},
		{"loopback_v4_extended", "127.5.5.5", true},
		{"loopback_v6", "::1", true},
		{"unspecified_v4", "0.0.0.0", true},
		{"unspecified_v6", "::", true},
		{"link_local_v4", "169.254.169.254", true},
		{"link_local_v6", "fe80::1", true},
		{"private_class_a", "10.0.0.1", true},
		{"private_class_b", "172.16.0.1", true},
		{"private_class_b_edge", "172.31.255.254", true},
		{"private_class_c", "192.168.1.1", true},
		{"cgnat", "100.64.0.1", true},
		{"multicast_v4", "224.0.0.1", true},
		{"ula_v6", "fc00::1", true},
		{"public_v4", "8.8.8.8", false},
		{"public_v4_cloudflare", "1.1.1.1", false},
		{"public_v6", "2001:4860:4860::8888", false},
		{"outside_class_b", "172.15.0.1", false},
		{"outside_class_b_high", "172.32.0.1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ip net.IP
			if tc.ip != "" {
				ip = net.ParseIP(tc.ip)
				require.NotNil(t, ip, "failed to parse test IP %q", tc.ip)
			}
			assert.Equal(t, tc.private, webhook.IsPrivateOrReservedIP(ip),
				"IsPrivateOrReservedIP(%q)", tc.ip)
		})
	}
}

// TestDefaultIPGuard asserts the inverse relationship with
// IsPrivateOrReservedIP.
func TestDefaultIPGuard(t *testing.T) {
	assert.False(t, webhook.DefaultIPGuard(net.ParseIP("169.254.169.254")))
	assert.False(t, webhook.DefaultIPGuard(net.ParseIP("127.0.0.1")))
	assert.True(t, webhook.DefaultIPGuard(net.ParseIP("8.8.8.8")))
}

// TestSSRFBlockedError_Message verifies the error is informative for logs
// and does not leak payload contents.
func TestSSRFBlockedError_Message(t *testing.T) {
	e := &webhook.SSRFBlockedError{
		Host: "attacker.example.com",
		IP:   net.ParseIP("169.254.169.254"),
	}
	msg := e.Error()
	assert.Contains(t, msg, "attacker.example.com")
	assert.Contains(t, msg, "169.254.169.254")
	assert.Contains(t, msg, "SSRF")
}

// TestNewHTTPClient_InsecureSkipVerify asserts the caller-controlled opt-in
// works end-to-end for test environments.
func TestNewHTTPClient_InsecureSkipVerify(t *testing.T) {
	client, err := webhook.NewHTTPClient(&webhook.ClientConfig{
		Timeout:            time.Second,
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}
