package backend_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/backend"
)

func TestAdapterFactory_CreateAdapter(t *testing.T) {
	factory := backend.NewAdapterFactory()

	t.Run("nil instance returns error", func(t *testing.T) {
		adp, err := factory.CreateAdapter(nil)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("unsupported adapter type returns error", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-backend-1",
			AdapterType: "nonexistent",
		}
		adp, err := factory.CreateAdapter(instance)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.ErrorIs(t, err, backend.ErrUnsupportedAdapterType)
	})

	t.Run("mock adapter with default config", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-backend-1",
			AdapterType: "mock",
		}
		adp, err := factory.CreateAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, adp)
		assert.Equal(t, "mock", adp.Name())
	})

	t.Run("mock adapter with custom ocloud_id", func(t *testing.T) {
		cfg := map[string]interface{}{
			"populate_sample_data": true,
			"ocloud_id":            "custom-ocloud",
		}
		cfgJSON, err := json.Marshal(cfg)
		require.NoError(t, err)

		instance := &backend.Instance{
			ID:          "test-backend-2",
			AdapterType: "mock",
			Config:      cfgJSON,
		}
		adp, err := factory.CreateAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, adp)

		// Verify the OCloudID is used via the deployment manager
		dms, dmErr := adp.ListDeploymentManagers(
			context.Background(), nil,
		)
		require.NoError(t, dmErr)
		require.Len(t, dms, 1)
		assert.Equal(t, "custom-ocloud", dms[0].OCloudID)
	})

	t.Run("mock adapter with no sample data", func(t *testing.T) {
		cfg := map[string]interface{}{
			"populate_sample_data": false,
		}
		cfgJSON, err := json.Marshal(cfg)
		require.NoError(t, err)

		instance := &backend.Instance{
			ID:          "empty-backend",
			AdapterType: "mock",
			Config:      cfgJSON,
		}
		adp, err := factory.CreateAdapter(instance)
		require.NoError(t, err)

		// No sample data — should have no resource pools
		pools, poolErr := adp.ListResourcePools(
			context.Background(), nil,
		)
		require.NoError(t, poolErr)
		assert.Empty(t, pools)
	})

	t.Run("mock adapter with invalid config JSON", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-backend-3",
			AdapterType: "mock",
			Config:      []byte(`{invalid json`),
		}
		adp, err := factory.CreateAdapter(instance)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.Contains(t, err.Error(), "parse mock adapter config")
	})

	t.Run("mock adapter uses instance ID as OCloudID when not configured", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "my-special-backend",
			AdapterType: "mock",
		}
		adp, err := factory.CreateAdapter(instance)
		require.NoError(t, err)

		dms, dmErr := adp.ListDeploymentManagers(
			context.Background(), nil,
		)
		require.NoError(t, dmErr)
		require.Len(t, dms, 1)
		assert.Equal(t, "my-special-backend", dms[0].OCloudID)
	})
}
