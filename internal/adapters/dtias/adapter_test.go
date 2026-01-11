package dtias

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
				Logger:              zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)),
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
			adp, err := New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, adp)

				// Verify adapter metadata
				assert.Equal(t, "dtias", adp.Name())
				assert.Equal(t, "1.0.0", adp.Version())
				assert.NotEmpty(t, adp.Capabilities())

				// Verify configuration defaults were applied
				if tt.config.Timeout == 0 {
					assert.Equal(t, 30*time.Second, adp.config.Timeout)
				}
				if tt.config.RetryAttempts == 0 {
					assert.Equal(t, 3, adp.config.RetryAttempts)
				}
				if tt.config.RetryDelay == 0 {
					assert.Equal(t, 2*time.Second, adp.config.RetryDelay)
				}
				if tt.config.DeploymentManagerID == "" {
					assert.NotEmpty(t, adp.deploymentManagerID)
				}

				// Cleanup
				assert.NoError(t, adp.Close())
			}
		})
	}
}

func TestDTIASAdapter_Name(t *testing.T) {
	adp := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, adp.Close())
	})

	assert.Equal(t, "dtias", adp.Name())
}

func TestDTIASAdapter_Version(t *testing.T) {
	adp := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, adp.Close())
	})

	assert.Equal(t, "1.0.0", adp.Version())
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
		adapter.CapabilitySubscriptions, // Polling-based implementation
	}

	assert.Len(t, capabilities, len(expectedCapabilities))
	for _, expected := range expectedCapabilities {
		assert.Contains(t, capabilities, expected)
	}
}

func TestDTIASAdapter_Close(t *testing.T) {
	adp := createTestAdapter(t)

	err := adp.Close()
	assert.NoError(t, err)
}

func TestDTIASAdapter_Health(t *testing.T) {
	// Create adapter with no-op logger to suppress expected ERROR logs
	// from intentionally failing health checks
	config := &Config{
		Endpoint:            "https://dtias.example.com/api/v1",
		APIKey:              "test-api-key",
		OCloudID:            "ocloud-test",
		DeploymentManagerID: "dm-test",
		Datacenter:          "dc-test",
		Timeout:             5 * time.Second,
		RetryAttempts:       2,
		RetryDelay:          time.Millisecond,
		Logger:              zap.NewNop(), // No-op logger for expected errors
	}

	a, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, a)

	t.Cleanup(func() {
		assert.NoError(t, a.Close())
	})

	// Health check will fail without a real DTIAS backend
	// This is expected behavior for unit tests
	// Integration tests will test actual DTIAS API connectivity
	err = a.Health(context.Background())

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
		// Use WarnLevel to suppress expected ERROR logs from intentional test failures
		Logger: zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)),
	}

	adp, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, adp)

	return adp
}

// TestSubscriptions tests subscription CRUD operations.
// DTIAS implements polling-based subscriptions stored locally.
func TestSubscriptions(t *testing.T) {
	adp := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, adp.Close())
	})

	ctx := context.Background()

	t.Run("CreateSubscription", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback:               "https://example.com/callback",
			ConsumerSubscriptionID: "consumer-sub-1",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.NotEmpty(t, created.SubscriptionID)
		assert.Equal(t, "https://example.com/callback", created.Callback)
		assert.Equal(t, "consumer-sub-1", created.ConsumerSubscriptionID)
	})

	t.Run("CreateSubscription with ID", func(t *testing.T) {
		sub := &adapter.Subscription{
			SubscriptionID: "my-custom-id",
			Callback:       "https://example.com/callback2",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "my-custom-id", created.SubscriptionID)
	})

	t.Run("CreateSubscription without callback", func(t *testing.T) {
		sub := &adapter.Subscription{}

		_, err := adp.CreateSubscription(ctx, sub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("CreateSubscription duplicate ID fails", func(t *testing.T) {
		// First create should succeed
		sub := &adapter.Subscription{
			SubscriptionID: "duplicate-test-id",
			Callback:       "https://example.com/callback",
		}
		_, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Second create with same ID should fail
		sub2 := &adapter.Subscription{
			SubscriptionID: "duplicate-test-id",
			Callback:       "https://example.com/callback2",
		}
		_, err = adp.CreateSubscription(ctx, sub2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription already exists")
		assert.True(t, errors.Is(err, adapter.ErrSubscriptionExists))
	})

	t.Run("GetSubscription", func(t *testing.T) {
		sub, err := adp.GetSubscription(ctx, "my-custom-id")
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, "my-custom-id", sub.SubscriptionID)
	})

	t.Run("GetSubscription not found", func(t *testing.T) {
		_, err := adp.GetSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("ListSubscriptions", func(t *testing.T) {
		subs := adp.ListSubscriptions()
		assert.Len(t, subs, 2)
	})

	t.Run("DeleteSubscription", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "my-custom-id")
		require.NoError(t, err)

		_, err = adp.GetSubscription(ctx, "my-custom-id")
		require.Error(t, err)
	})

	t.Run("DeleteSubscription not found", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
}

// TestGetPollingRecommendation tests polling recommendations.
func TestGetPollingRecommendation(t *testing.T) {
	adp := createTestAdapter(t)
	t.Cleanup(func() {
		assert.NoError(t, adp.Close())
	})

	rec := adp.GetPollingRecommendation()
	require.NotNil(t, rec)

	// Verify recommended intervals
	assert.Contains(t, rec.RecommendedIntervals, "resource-pools")
	assert.Contains(t, rec.RecommendedIntervals, "resources")
	assert.Contains(t, rec.RecommendedIntervals, "health-metrics")

	// Verify change detection fields
	assert.Contains(t, rec.ChangeDetectionFields, "resource-pools")
	assert.Contains(t, rec.ChangeDetectionFields, "resources")

	// Verify optimization tips
	assert.NotEmpty(t, rec.OptimizationTips)
}

// TestCloseWithSubscriptions verifies subscriptions are cleared on close.
func TestCloseWithSubscriptions(t *testing.T) {
	adp := createTestAdapter(t)
	ctx := context.Background()

	// Create some subscriptions
	_, err := adp.CreateSubscription(ctx, &adapter.Subscription{
		Callback: "https://example.com/callback1",
	})
	require.NoError(t, err)

	_, err = adp.CreateSubscription(ctx, &adapter.Subscription{
		Callback: "https://example.com/callback2",
	})
	require.NoError(t, err)

	// Verify subscriptions exist
	assert.Len(t, adp.ListSubscriptions(), 2)

	// Close adapter
	err = adp.Close()
	require.NoError(t, err)

	// Verify subscriptions are cleared
	assert.Empty(t, adp.subscriptions)
}
