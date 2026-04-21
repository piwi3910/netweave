package callbackurl_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/security/callbackurl"
)

// stubResolver returns a fixed set of IPs or a fixed error, bypassing real DNS.
type stubResolver struct {
	ips []net.IPAddr
	err error
}

func (s *stubResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	return s.ips, s.err
}

// TestValidate covers the full matrix of accepted and rejected callback URLs.
func TestValidate(t *testing.T) {
	publicIP := net.ParseIP("93.184.216.34") // example.com
	privateIP := net.ParseIP("10.1.2.3")
	metaIP := net.ParseIP("169.254.169.254")

	tests := []struct {
		name     string
		rawURL   string
		resolver callbackurl.Resolver
		wantErr  error
	}{
		{
			name:     "happy path public https",
			rawURL:   "https://smo.example.com/notify",
			resolver: &stubResolver{ips: []net.IPAddr{{IP: publicIP}}},
		},
		{
			name:     "happy path public http",
			rawURL:   "http://smo.example.com/notify",
			resolver: &stubResolver{ips: []net.IPAddr{{IP: publicIP}}},
		},
		{
			name:    "empty URL",
			rawURL:  "",
			wantErr: callbackurl.ErrEmptyURL,
		},
		{
			name:    "file scheme",
			rawURL:  "file:///etc/passwd",
			wantErr: callbackurl.ErrInvalidScheme,
		},
		{
			name:    "gopher scheme",
			rawURL:  "gopher://example.com:70/",
			wantErr: callbackurl.ErrInvalidScheme,
		},
		{
			name:    "ftp scheme",
			rawURL:  "ftp://example.com/",
			wantErr: callbackurl.ErrInvalidScheme,
		},
		{
			name:    "javascript scheme",
			rawURL:  "javascript:alert(1)",
			wantErr: callbackurl.ErrInvalidScheme,
		},
		{
			name:    "missing host",
			rawURL:  "https:///path",
			wantErr: callbackurl.ErrMissingHost,
		},
		{
			name:    "literal loopback hostname",
			rawURL:  "https://localhost/notify",
			wantErr: callbackurl.ErrLoopbackHost,
		},
		{
			name:    "literal 127.0.0.1",
			rawURL:  "https://127.0.0.1/notify",
			wantErr: callbackurl.ErrPrivateAddress,
		},
		{
			name:    "literal IPv6 loopback",
			rawURL:  "https://[::1]/notify",
			wantErr: callbackurl.ErrPrivateAddress,
		},
		{
			name:    "literal RFC1918 10.x",
			rawURL:  "https://10.1.2.3/notify",
			wantErr: callbackurl.ErrPrivateAddress,
		},
		{
			name:    "literal RFC1918 192.168.x",
			rawURL:  "https://192.168.0.5/notify",
			wantErr: callbackurl.ErrPrivateAddress,
		},
		{
			name:    "literal cloud metadata IP",
			rawURL:  "https://169.254.169.254/latest/meta-data/",
			wantErr: callbackurl.ErrPrivateAddress,
		},
		{
			name:     "DNS rebind attempt - any resolved IP banned rejects",
			rawURL:   "https://rebind.attacker.example/notify",
			resolver: &stubResolver{ips: []net.IPAddr{{IP: publicIP}, {IP: metaIP}}},
			wantErr:  callbackurl.ErrPrivateAddress,
		},
		{
			name:     "hostname resolves to private IP",
			rawURL:   "https://intranet.example.com/notify",
			resolver: &stubResolver{ips: []net.IPAddr{{IP: privateIP}}},
			wantErr:  callbackurl.ErrPrivateAddress,
		},
		{
			name:     "DNS error is treated as pass-through",
			rawURL:   "https://temporarily-down.example.com/notify",
			resolver: &stubResolver{err: errors.New("dns timeout")},
		},
		{
			name:     "no IPs returned is treated as pass-through",
			rawURL:   "https://nxdomain.example.com/notify",
			resolver: &stubResolver{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := callbackurl.Validate(context.Background(), tt.rawURL, callbackurl.Options{
				Resolver: tt.resolver,
			})
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestValidate_AllowPrivateNetworks confirms the test-only escape hatch works.
func TestValidate_AllowPrivateNetworks(t *testing.T) {
	err := callbackurl.Validate(context.Background(), "https://127.0.0.1:8080/notify", callbackurl.Options{
		AllowPrivateNetworks: true,
	})
	require.NoError(t, err)
}

// TestValidate_MalformedURL exercises the url.Parse failure path.
func TestValidate_MalformedURL(t *testing.T) {
	// %zz is an invalid percent-escape that url.Parse rejects.
	err := callbackurl.Validate(context.Background(), "https://example.com/%zz", callbackurl.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid callback URL format")
}
