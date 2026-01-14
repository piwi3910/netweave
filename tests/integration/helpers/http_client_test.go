package helpers_test

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/piwi3910/netweave/tests/integration/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTestHTTPClient verifies the test HTTP client is properly configured.
func TestNewTestHTTPClient(t *testing.T) {
	client := helpers.NewTestHTTPClient()
	require.NotNil(t, client, "Client should not be nil")

	// Verify timeout configuration
	assert.Equal(t, 30*time.Second, client.Timeout,
		"Client should have 30s timeout to prevent hanging tests")

	// Verify transport configuration
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "Transport should be *http.Transport")

	assert.Equal(t, 10, transport.MaxIdleConns,
		"Should limit max idle connections")
	assert.Equal(t, 10, transport.MaxIdleConnsPerHost,
		"Should limit max idle connections per host")
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout,
		"Should have 90s idle connection timeout")
	assert.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout,
		"Should have 10s TLS handshake timeout")

	// Verify TLS configuration
	require.NotNil(t, transport.TLSClientConfig, "TLS config should be set")
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion,
		"Should enforce TLS 1.3 minimum")
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify,
		"Should validate certificates by default")
}

// TestNewTestHTTPClientWithTimeout verifies custom timeout configuration.
func TestNewTestHTTPClientWithTimeout(t *testing.T) {
	customTimeout := 60 * time.Second
	client := helpers.NewTestHTTPClientWithTimeout(customTimeout)

	require.NotNil(t, client, "Client should not be nil")
	assert.Equal(t, customTimeout, client.Timeout,
		"Client should have custom timeout")

	// Verify other transport settings remain the same
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "Transport should be *http.Transport")

	assert.Equal(t, 10, transport.MaxIdleConns,
		"Should maintain default max idle connections")
	assert.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout,
		"Should maintain default TLS handshake timeout")
}

// TestHTTPClientNotDefaultClient ensures we're not using http.DefaultClient.
func TestHTTPClientNotDefaultClient(t *testing.T) {
	client := helpers.NewTestHTTPClient()
	assert.NotEqual(t, http.DefaultClient, client,
		"Test client should not be the default client")

	// Verify we have custom configuration
	assert.NotEqual(t, http.DefaultClient.Timeout, client.Timeout,
		"Test client should have different timeout than default")
}
