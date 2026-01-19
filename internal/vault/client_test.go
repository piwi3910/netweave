package vault

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require a running Vault instance.
// Run: docker-compose -f docker-compose.dev.yml up -d
// Before running: go test -v ./internal/vault/...

func getTestClient(t *testing.T) *Client {
	t.Helper()

	config := &Config{
		Address: "http://localhost:8200",
		Token:   "root",
		PKIPath: "pki_int",
		Timeout: 10 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	return client
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing Address",
			config: &Config{
				Token:   "token",
				PKIPath: "pki",
			},
			wantErr: true,
			errMsg:  "address is required",
		},
		{
			name: "missing Token",
			config: &Config{
				Address: "http://localhost:8200",
				PKIPath: "pki",
			},
			wantErr: true,
			errMsg:  "token is required",
		},
		{
			name: "missing PKIPath",
			config: &Config{
				Address: "http://localhost:8200",
				Token:   "token",
			},
			wantErr: true,
			errMsg:  "PKIPath is required",
		},
		{
			name: "valid config",
			config: &Config{
				Address: "http://localhost:8200",
				Token:   "root",
				PKIPath: "pki_int",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestClient_Ping(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	err := client.Ping(ctx)
	require.NoError(t, err)
}

func TestClient_GetCAChain(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	caChain, err := client.GetCAChain(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, caChain)
	assert.Contains(t, caChain, "BEGIN CERTIFICATE")
	assert.Contains(t, caChain, "END CERTIFICATE")
}

func TestClient_GetCACertificate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	caCert, err := client.GetCACertificate(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, caCert)
	assert.Contains(t, caCert, "BEGIN CERTIFICATE")
	assert.Contains(t, caCert, "END CERTIFICATE")
}

func TestClient_GetCRL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client := getTestClient(t)
	defer client.Close()

	ctx := context.Background()
	crl, err := client.GetCRL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, crl)
	assert.Contains(t, crl, "BEGIN X509 CRL")
	assert.Contains(t, crl, "END X509 CRL")
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "http://localhost:8200", config.Address)
	assert.Equal(t, "pki_int", config.PKIPath)
	assert.Equal(t, 30*time.Second, config.Timeout)
}
