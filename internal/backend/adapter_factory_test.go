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

		// Verify the OCloudID is used via the deployment manager.
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

		// No sample data - should have no resource pools.
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

func TestAdapterFactory_CreateDMSAdapter(t *testing.T) {
	factory := backend.NewAdapterFactory()

	t.Run("nil instance returns error", func(t *testing.T) {
		adp, err := factory.CreateDMSAdapter(nil)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("unsupported DMS adapter type returns error", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-dms-1",
			AdapterType: "nonexistent-dms",
		}
		adp, err := factory.CreateDMSAdapter(instance)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.ErrorIs(t, err, backend.ErrUnsupportedAdapterType)
	})

	t.Run("mock-dms adapter with default config", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-dms-default",
			AdapterType: "mock-dms",
		}
		adp, err := factory.CreateDMSAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, adp)
		assert.Equal(t, "mock", adp.Name())
	})

	t.Run("mock-dms adapter with sample data", func(t *testing.T) {
		cfg := map[string]interface{}{
			"populate_sample_data": true,
		}
		cfgJSON, err := json.Marshal(cfg)
		require.NoError(t, err)

		instance := &backend.Instance{
			ID:          "test-dms-with-data",
			AdapterType: "mock-dms",
			Config:      cfgJSON,
		}
		adp, err := factory.CreateDMSAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, adp)

		// Should have sample packages pre-populated.
		packages, pkgErr := adp.ListDeploymentPackages(context.Background(), nil)
		require.NoError(t, pkgErr)
		assert.NotEmpty(t, packages)
	})

	t.Run("mock-dms adapter without sample data", func(t *testing.T) {
		cfg := map[string]interface{}{
			"populate_sample_data": false,
		}
		cfgJSON, err := json.Marshal(cfg)
		require.NoError(t, err)

		instance := &backend.Instance{
			ID:          "test-dms-empty",
			AdapterType: "mock-dms",
			Config:      cfgJSON,
		}
		adp, err := factory.CreateDMSAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, adp)

		// No sample data - should have no packages.
		packages, pkgErr := adp.ListDeploymentPackages(context.Background(), nil)
		require.NoError(t, pkgErr)
		assert.Empty(t, packages)
	})

	t.Run("mock-dms adapter with invalid config JSON", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-dms-bad",
			AdapterType: "mock-dms",
			Config:      []byte(`{invalid json`),
		}
		adp, err := factory.CreateDMSAdapter(instance)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.Contains(t, err.Error(), "parse mock DMS adapter config")
	})
}

func TestAdapterFactory_CreateSMOAdapter(t *testing.T) {
	factory := backend.NewAdapterFactory()

	t.Run("nil instance returns error", func(t *testing.T) {
		adp, err := factory.CreateSMOAdapter(nil)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("unsupported SMO adapter type returns error", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-smo-1",
			AdapterType: "nonexistent-smo",
		}
		adp, err := factory.CreateSMOAdapter(instance)
		require.Error(t, err)
		assert.Nil(t, adp)
		assert.ErrorIs(t, err, backend.ErrUnsupportedAdapterType)
	})

	t.Run("mock-smo adapter with default config", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-smo-default",
			AdapterType: "mock-smo",
		}
		plugin, err := factory.CreateSMOAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, plugin)

		metadata := plugin.Metadata()
		assert.Equal(t, "mock", metadata.Name)
		assert.Equal(t, "1.0.0", metadata.Version)
	})

	t.Run("mock-smo adapter with config", func(t *testing.T) {
		cfg := map[string]interface{}{
			"populate_sample_data": true,
			"simulate_workflows":   true,
		}
		cfgJSON, err := json.Marshal(cfg)
		require.NoError(t, err)

		instance := &backend.Instance{
			ID:          "test-smo-configured",
			AdapterType: "mock-smo",
			Config:      cfgJSON,
		}
		plugin, err := factory.CreateSMOAdapter(instance)
		require.NoError(t, err)
		require.NotNil(t, plugin)

		metadata := plugin.Metadata()
		assert.Equal(t, "mock", metadata.Name)
	})

	t.Run("mock-smo adapter with invalid config JSON", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-smo-bad",
			AdapterType: "mock-smo",
			Config:      []byte(`{invalid json`),
		}
		plugin, err := factory.CreateSMOAdapter(instance)
		require.Error(t, err)
		assert.Nil(t, plugin)
		assert.Contains(t, err.Error(), "parse mock SMO adapter config")
	})

	t.Run("mock-smo adapter capabilities are correct", func(t *testing.T) {
		instance := &backend.Instance{
			ID:          "test-smo-caps",
			AdapterType: "mock-smo",
		}
		plugin, err := factory.CreateSMOAdapter(instance)
		require.NoError(t, err)

		caps := plugin.Capabilities()
		assert.NotEmpty(t, caps)
	})
}
