package server_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
)

// TestValidateCallback tests the callback URL validation including SSRF protection.
func TestValidateCallback(t *testing.T) {
	tests := []struct {
		name    string
		sub     *adapter.Subscription
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid HTTPS callback",
			sub: &adapter.Subscription{
				Callback: "https://smo.example.com/notify",
			},
			wantErr: false,
		},
		{
			name: "valid HTTP callback",
			sub: &adapter.Subscription{
				Callback: "http://smo.example.com/notify",
			},
			wantErr: false,
		},
		{
			name: "valid callback with port",
			sub: &adapter.Subscription{
				Callback: "https://smo.example.com:8443/notify",
			},
			wantErr: false,
		},
		{
			name: "valid callback with path and query",
			sub: &adapter.Subscription{
				Callback: "https://smo.example.com/api/v1/notify?token=abc123",
			},
			wantErr: false,
		},
		{
			name:    "nil subscription",
			sub:     nil,
			wantErr: true,
			errMsg:  "subscription cannot be nil",
		},
		{
			name:    "empty callback URL",
			sub:     &adapter.Subscription{Callback: ""},
			wantErr: true,
			errMsg:  "callback URL is required",
		},
		{
			name:    "invalid URL format - no scheme",
			sub:     &adapter.Subscription{Callback: "not a url"},
			wantErr: true,
			errMsg:  "callback URL must use http or https scheme",
		},
		{
			name:    "invalid scheme - FTP",
			sub:     &adapter.Subscription{Callback: "ftp://smo.example.com/notify"},
			wantErr: true,
			errMsg:  "callback URL must use http or https scheme",
		},
		{
			name:    "invalid scheme - file",
			sub:     &adapter.Subscription{Callback: "file:///etc/passwd"},
			wantErr: true,
			errMsg:  "callback URL must use http or https scheme",
		},
		{
			name:    "no host",
			sub:     &adapter.Subscription{Callback: "https:///path"},
			wantErr: true,
			errMsg:  "callback URL must have a valid host",
		},
		{
			name:    "SSRF - localhost",
			sub:     &adapter.Subscription{Callback: "http://localhost/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be localhost",
		},
		{
			name:    "SSRF - 127.0.0.1",
			sub:     &adapter.Subscription{Callback: "http://127.0.0.1/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be localhost",
		},
		{
			name:    "SSRF - IPv6 loopback",
			sub:     &adapter.Subscription{Callback: "http://[::1]/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be localhost",
		},
		{
			name:    "SSRF - private IP 10.x.x.x",
			sub:     &adapter.Subscription{Callback: "http://10.0.0.1/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be a private IP address",
		},
		{
			name:    "SSRF - private IP 192.168.x.x",
			sub:     &adapter.Subscription{Callback: "http://192.168.1.1/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be a private IP address",
		},
		{
			name:    "SSRF - private IP 172.16.x.x",
			sub:     &adapter.Subscription{Callback: "http://172.16.0.1/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be a private IP address",
		},
		{
			name:    "SSRF - link-local 169.254.x.x",
			sub:     &adapter.Subscription{Callback: "http://169.254.1.1/admin"},
			wantErr: true,
			errMsg:  "callback URL cannot be a private IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server instance with config
			s := &Server{
				config: &config.Config{
					Security: config.SecurityConfig{
						DisableSSRFProtection: false, // Enable SSRF protection for tests
					},
				},
			}

			err := s.validateCallback(context.Background(), tt.sub)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateCallbackHost tests the hostname validation for SSRF protection.
func TestValidateCallbackHost(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid public hostname",
			hostname: "smo.example.com",
			wantErr:  false,
		},
		{
			name:     "localhost string",
			hostname: "localhost",
			wantErr:  true,
			errMsg:   "callback URL cannot be localhost",
		},
		{
			name:     "127.0.0.1",
			hostname: "127.0.0.1",
			wantErr:  true,
			errMsg:   "callback URL cannot be localhost",
		},
		{
			name:     "IPv6 loopback",
			hostname: "::1",
			wantErr:  true,
			errMsg:   "callback URL cannot be localhost",
		},
		{
			name:     "private IP 10.0.0.1",
			hostname: "10.0.0.1",
			wantErr:  true,
			errMsg:   "callback URL cannot be a private IP address",
		},
		{
			name:     "private IP 192.168.1.1",
			hostname: "192.168.1.1",
			wantErr:  true,
			errMsg:   "callback URL cannot be a private IP address",
		},
		{
			name:     "private IP 172.16.0.1",
			hostname: "172.16.0.1",
			wantErr:  true,
			errMsg:   "callback URL cannot be a private IP address",
		},
		{
			name:     "link-local 169.254.1.1",
			hostname: "169.254.1.1",
			wantErr:  true,
			errMsg:   "callback URL cannot be a private IP address",
		},
		{
			name:     "public IP 8.8.8.8",
			hostname: "8.8.8.8",
			wantErr:  false,
		},
		{
			name:     "non-existent hostname allows through",
			hostname: "this-hostname-definitely-does-not-exist-12345.example",
			wantErr:  false, // DNS lookup fails, we allow it through
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCallbackHost(context.Background(), tt.hostname)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsPrivateIP tests the private IP detection logic.
func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		isPrivate bool
	}{
		// Loopback addresses
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 loopback 127.x", "127.255.255.254", true},
		{"IPv6 loopback", "::1", true},

		// Private IPv4 ranges (RFC 1918)
		{"Private 10.0.0.0", "10.0.0.0", true},
		{"Private 10.x.x.x", "10.123.45.67", true},
		{"Private 10.255.255.255", "10.255.255.255", true},
		{"Private 172.16.0.0", "172.16.0.0", true},
		{"Private 172.16.x.x", "172.16.123.45", true},
		{"Private 172.31.255.255", "172.31.255.255", true},
		{"Private 192.168.0.0", "192.168.0.0", true},
		{"Private 192.168.x.x", "192.168.123.45", true},
		{"Private 192.168.255.255", "192.168.255.255", true},

		// Link-local
		{"Link-local 169.254.x.x", "169.254.1.1", true},

		// Public IPv4 addresses
		{"Public Google DNS", "8.8.8.8", false},
		{"Public Cloudflare DNS", "1.1.1.1", false},
		{"Public example", "93.184.216.34", false},

		// IPv6 private ranges
		{"IPv6 ULA fc00::", "fc00::1", true},
		{"IPv6 ULA fd00::", "fd00::1234", true},
		{"IPv6 link-local fe80::", "fe80::1", true},

		// Public IPv6
		{"Public IPv6 Google", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "Failed to parse IP: %s", tt.ip)

			result := isPrivateIP(ip)
			assert.Equal(t, tt.isPrivate, result,
				"IP %s should be private=%v but got %v", tt.ip, tt.isPrivate, result)
		})
	}
}
