package service_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/cli/service"
)

func TestNewTLSClient(t *testing.T) {
	caPEM := generateTestCACert(t)

	t.Run("valid CA cert", func(t *testing.T) {
		client, err := service.NewTLSClient(10*time.Second, caPEM)
		require.NoError(t, err)
		require.NotNil(t, client)

		assert.Equal(t, 10*time.Second, client.Timeout)

		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok, "Transport should be *http.Transport")
		require.NotNil(t, transport.TLSClientConfig)

		assert.Equal(t,
			uint16(tls.VersionTLS12),
			transport.TLSClientConfig.MinVersion,
			"should enforce TLS 1.2 minimum",
		)
		assert.False(t,
			transport.TLSClientConfig.InsecureSkipVerify,
			"should NOT skip certificate verification",
		)
		assert.NotNil(t, transport.TLSClientConfig.RootCAs,
			"should have custom CA pool",
		)
	})

	t.Run("invalid CA cert", func(t *testing.T) {
		client, err := service.NewTLSClient(10*time.Second, []byte("not-a-cert"))
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to parse CA certificate")
	})

	t.Run("empty CA cert", func(t *testing.T) {
		client, err := service.NewTLSClient(10*time.Second, []byte{})
		require.Error(t, err)
		assert.Nil(t, client)
	})
}

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

func generateTestCACert(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}
