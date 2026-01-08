// Package helpers provides utilities for integration tests.
package helpers

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewTestHTTPClient creates an HTTP client configured for integration tests.
// The client includes proper timeouts, connection limits, and TLS configuration
// to ensure reliable and production-representative testing.
//
// Configuration:
//   - Request timeout: 30s (prevents hanging tests)
//   - TLS handshake timeout: 10s
//   - Idle connection timeout: 90s
//   - Max idle connections: 10 per host
//
// This client should be used instead of http.DefaultClient in all integration tests
// to comply with security best practices and project standards.
func NewTestHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			// TLS configuration for mTLS testing
			// skipcq: GO-S1020 - InsecureSkipVerify may be needed for test environments
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
				// InsecureSkipVerify can be set true for test environments
				// In production tests with real certificates, this should be false
				InsecureSkipVerify: false, //nolint:gosec // Configurable for test environments
			},
		},
	}
}

// NewTestHTTPClientWithTimeout creates an HTTP client with a custom timeout.
// Use this when specific tests require different timeout values.
func NewTestHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	client := NewTestHTTPClient()
	client.Timeout = timeout
	return client
}
