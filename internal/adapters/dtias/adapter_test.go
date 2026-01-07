package dtias

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid configuration",
			config: &Config{
				Endpoint:            "https://dtias.example.com/api/v1",
				APIKey:              "test-api-key",
				OCloudID:            "ocloud-dtias-1",
				DeploymentManagerID: "ocloud-dtias-dm-1",
				Datacenter:          "dc-test-1",
				Timeout:             30 * time.Second,
				RetryAttempts:       3,
				RetryDelay:          2 * time.Second,
				Logger:              zaptest.NewLogger(t),
			},
			wantErr: false,
		},
		{
			name:    "nil configuration",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing endpoint",
			config: &Config{
				APIKey:   "test-api-key",
				OCloudID: "ocloud-dtias-1",
			},
			wantErr: true,
			errMsg:  "endpoint is required",
		},
		{
			name: "missing API key",
			config: &Config{
				Endpoint: "https://dtias.example.com/api/v1",
				OCloudID: "ocloud-dtias-1",
			},
			wantErr: true,
			errMsg:  "apiKey is required",
		},
		{
			name: "missing oCloudID",
			config: &Config{
				Endpoint: "https://dtias.example.com/api/v1",
				APIKey:   "test-api-key",
			},
			wantErr: true,
			errMsg:  "ocloudId is required",
		},
		{
			name: "configuration with defaults",
			config: &Config{
				Endpoint: "https://dtias.example.com/api/v1",
				APIKey:   "test-api-key",
				OCloudID: "ocloud-dtias-1",
				Logger:   zaptest.NewLogger(t),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adapter)
			} else {
				require.NoError(t, err)
				require.NotNil(t, adapter)

				// Verify adapter metadata
				assert.Equal(t, "dtias", adapter.Name())
				assert.Equal(t, "1.0.0", adapter.Version())
				assert.NotEmpty(t, adapter.Capabilities())

				// Verify configuration defaults were applied
				if tt.config.Timeout == 0 {
					assert.Equal(t, 30*time.Second, adapter.config.Timeout)
				}
				if tt.config.RetryAttempts == 0 {
					assert.Equal(t, 3, adapter.config.RetryAttempts)
				}
				if tt.config.RetryDelay == 0 {
					assert.Equal(t, 2*time.Second, adapter.config.RetryDelay)
				}
				if tt.config.DeploymentManagerID == "" {
					assert.NotEmpty(t, adapter.deploymentManagerID)
				}

				// Cleanup
				assert.NoError(t, adapter.Close())
			}
		})
	}
}

func TestDTIASAdapter_Name(t *testing.T) {
	adapter := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, adapter.Close())
	})

	assert.Equal(t, "dtias", adapter.Name())
}

func TestDTIASAdapter_Version(t *testing.T) {
	adapter := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, adapter.Close())
	})

	assert.Equal(t, "1.0.0", adapter.Version())
}

func TestDTIASAdapter_Capabilities(t *testing.T) {
	a := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, a.Close())
	})

	capabilities := a.Capabilities()

	// Verify expected capabilities are present
	expectedCapabilities := []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilityHealthChecks,
	}

	assert.Len(t, capabilities, len(expectedCapabilities))
	for _, expected := range expectedCapabilities {
		assert.Contains(t, capabilities, expected)
	}

	// Verify subscriptions capability is NOT present (DTIAS has no native subscriptions)
	assert.NotContains(t, capabilities, adapter.CapabilitySubscriptions)
}

func TestDTIASAdapter_Close(t *testing.T) {
	adapter := createTestAdapter(t)

	err := adapter.Close()
	assert.NoError(t, err)
}

func TestDTIASAdapter_Health(t *testing.T) {
	a := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, a.Close())
	})

	// Health check will fail without a real DTIAS backend
	// This is expected behavior for unit tests
	// Integration tests will test actual DTIAS API connectivity
	err := a.Health(context.Background())

	// We expect an error since there's no real backend
	assert.Error(t, err, "health check should fail without real backend")
}

// createTestAdapter creates a test DTIAS adapter with minimal configuration.
func createTestAdapter(t *testing.T) *DTIASAdapter {
	t.Helper()

	config := &Config{
		Endpoint:            "https://dtias.example.com/api/v1",
		APIKey:              "test-api-key",
		OCloudID:            "ocloud-test",
		DeploymentManagerID: "dm-test",
		Datacenter:          "dc-test",
		Timeout:             5 * time.Second,
		RetryAttempts:       1,
		RetryDelay:          time.Millisecond,
		Logger:              zaptest.NewLogger(t),
		InsecureSkipVerify:  true, // For testing only
	}

	adapter, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	return adapter
}
