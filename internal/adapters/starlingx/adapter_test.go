package starlingx_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	dmsadapter "github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/starlingx"
)

func TestNew(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name    string
		config  *starlingx.Config
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
			name: "missing endpoint",
			config: &starlingx.Config{
				KeystoneEndpoint:    "http://localhost:5000",
				Username:            "admin",
				Password:            "secret",
				OCloudID:            "test-cloud",
				DeploymentManagerID: "test-dm",
			},
			wantErr: true,
			errMsg:  "endpoint is required",
		},
		{
			name: "missing keystone endpoint",
			config: &starlingx.Config{
				Endpoint:            "http://localhost:6385",
				Username:            "admin",
				Password:            "secret",
				OCloudID:            "test-cloud",
				DeploymentManagerID: "test-dm",
			},
			wantErr: true,
			errMsg:  "keystone endpoint is required",
		},
		{
			name: "missing username",
			config: &starlingx.Config{
				Endpoint:            "http://localhost:6385",
				KeystoneEndpoint:    "http://localhost:5000",
				Password:            "secret",
				OCloudID:            "test-cloud",
				DeploymentManagerID: "test-dm",
			},
			wantErr: true,
			errMsg:  "username and password are required",
		},
		{
			name: "missing oCloudID",
			config: &starlingx.Config{
				Endpoint:            "http://localhost:6385",
				KeystoneEndpoint:    "http://localhost:5000",
				Username:            "admin",
				Password:            "secret",
				DeploymentManagerID: "test-dm",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "missing deploymentManagerID",
			config: &starlingx.Config{
				Endpoint:         "http://localhost:6385",
				KeystoneEndpoint: "http://localhost:5000",
				Username:         "admin",
				Password:         "secret",
				OCloudID:         "test-cloud",
			},
			wantErr: true,
			errMsg:  "deploymentManagerID is required",
		},
		{
			name: "valid config with defaults",
			config: &starlingx.Config{
				Endpoint:            "http://localhost:6385",
				KeystoneEndpoint:    "http://localhost:5000",
				Username:            "admin",
				Password:            "secret",
				OCloudID:            "test-cloud",
				DeploymentManagerID: "test-dm",
				Logger:              logger,
			},
			wantErr: false,
		},
		{
			name: "valid config with all fields",
			config: &starlingx.Config{
				Endpoint:            "http://localhost:6385",
				KeystoneEndpoint:    "http://localhost:5000",
				Username:            "testuser",
				Password:            "testpass",
				ProjectName:         "testproject",
				DomainName:          "TestDomain",
				Region:              "RegionOne",
				OCloudID:            "test-cloud",
				DeploymentManagerID: "test-dm",
				Logger:              logger,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := starlingx.New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, adp)
				assert.Equal(t, "starlingx", adp.Name())
				assert.Equal(t, "8.0", adp.Version())

				// Verify capabilities
				caps := adp.Capabilities()
				assert.Contains(t, caps, dmsadapter.CapabilityResourcePools)
				assert.Contains(t, caps, dmsadapter.CapabilityResources)
				assert.Contains(t, caps, dmsadapter.CapabilityResourceTypes)
				assert.Contains(t, caps, dmsadapter.CapabilityDeploymentManagers)
				assert.Contains(t, caps, dmsadapter.CapabilityHealthChecks)

				// Clean up
				err = adp.Close()
				assert.NoError(t, err)
			}
		})
	}
}

func TestAdapter_Metadata(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adp, err := starlingx.New(&starlingx.Config{
		Endpoint:            "http://localhost:6385",
		KeystoneEndpoint:    "http://localhost:5000",
		Username:            "admin",
		Password:            "secret",
		OCloudID:            "test-cloud",
		DeploymentManagerID: "test-dm",
		Logger:              logger,
	})
	require.NoError(t, err)
	defer func() {
		if closeErr := adp.Close(); closeErr != nil {
			t.Logf("failed to close adapter: %v", closeErr)
		}
	}()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "starlingx", adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.Equal(t, "8.0", adp.Version())
	})

	t.Run("Capabilities without store", func(t *testing.T) {
		caps := adp.Capabilities()
		assert.Len(t, caps, 5)
		assert.Contains(t, caps, dmsadapter.CapabilityResourcePools)
		assert.Contains(t, caps, dmsadapter.CapabilityResources)
		assert.Contains(t, caps, dmsadapter.CapabilityResourceTypes)
		assert.Contains(t, caps, dmsadapter.CapabilityDeploymentManagers)
		assert.Contains(t, caps, dmsadapter.CapabilityHealthChecks)
		assert.NotContains(t, caps, dmsadapter.CapabilitySubscriptions)
	})
}

func TestAdapter_Close(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adp, err := starlingx.New(&starlingx.Config{
		Endpoint:            "http://localhost:6385",
		KeystoneEndpoint:    "http://localhost:5000",
		Username:            "admin",
		Password:            "secret",
		OCloudID:            "test-cloud",
		DeploymentManagerID: "test-dm",
		Logger:              logger,
	})
	require.NoError(t, err)

	err = adp.Close()
	assert.NoError(t, err)
}

func TestAdapter_Health_NotImplemented(t *testing.T) {
	// This is a unit test that doesn't require a real StarlingX instance
	// Health() will fail because we can't reach the mock endpoint
	logger := zaptest.NewLogger(t)

	adp, err := starlingx.New(&starlingx.Config{
		Endpoint:            "http://localhost:9999", // Non-existent endpoint
		KeystoneEndpoint:    "http://localhost:9998", // Non-existent endpoint
		Username:            "admin",
		Password:            "secret",
		OCloudID:            "test-cloud",
		DeploymentManagerID: "test-dm",
		Logger:              logger,
	})
	require.NoError(t, err)
	defer func() {
		if closeErr := adp.Close(); closeErr != nil {
			t.Logf("failed to close adapter: %v", closeErr)
		}
	}()

	ctx := context.Background()
	err = adp.Health(ctx)
	// Should fail because we can't connect
	assert.Error(t, err)
}
