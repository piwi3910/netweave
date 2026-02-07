package service_test

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/cli/service"
)

func TestNewInsecureTLSClient(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{
			name:    "10 second timeout",
			timeout: 10 * time.Second,
		},
		{
			name:    "30 second timeout",
			timeout: 30 * time.Second,
		},
		{
			name:    "zero timeout",
			timeout: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := service.NewInsecureTLSClient(tt.timeout)
			require.NotNil(t, client)

			assert.Equal(t, tt.timeout, client.Timeout)

			transport, ok := client.Transport.(*http.Transport)
			require.True(t, ok, "Transport should be *http.Transport")
			require.NotNil(t, transport.TLSClientConfig)

			assert.Equal(t,
				uint16(tls.VersionTLS12),
				transport.TLSClientConfig.MinVersion,
				"should enforce TLS 1.2 minimum",
			)
			assert.True(t,
				transport.TLSClientConfig.InsecureSkipVerify,
				"should skip certificate verification for port-forward connections",
			)
		})
	}
}
