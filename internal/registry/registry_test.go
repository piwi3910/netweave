package registry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/registry"
)

func TestNew(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := zap.NewNop()
		reg := registry.New(logger)
		assert.NotNil(t, reg)
	})

	t.Run("with nil logger", func(t *testing.T) {
		reg := registry.New(nil)
		assert.NotNil(t, reg)
	})
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name    string
		plugin  *registry.Plugin
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid plugin",
			plugin: &registry.Plugin{
				Category: registry.CategoryIMS,
				Name:     "kubernetes",
				Version:  "1.0.0",
				Priority: 100,
				Status:   registry.StatusActive,
				Instance: &mockAdapter{},
			},
			wantErr: false,
		},
		{
			name:    "nil plugin",
			plugin:  nil,
			wantErr: true,
			errMsg:  "plugin cannot be nil",
		},
		{
			name: "empty name",
			plugin: &registry.Plugin{
				Category: registry.CategoryIMS,
				Name:     "",
				Instance: &mockAdapter{},
			},
			wantErr: true,
			errMsg:  "plugin name cannot be empty",
		},
		{
			name: "empty category",
			plugin: &registry.Plugin{
				Category: "",
				Name:     "test",
				Instance: &mockAdapter{},
			},
			wantErr: true,
			errMsg:  "plugin category cannot be empty",
		},
		{
			name: "nil instance",
			plugin: &registry.Plugin{
				Category: registry.CategoryIMS,
				Name:     "test",
				Instance: nil,
			},
			wantErr: true,
			errMsg:  "plugin instance cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := registry.New(zap.NewNop())
			err := reg.Register(tt.plugin)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugin := &registry.Plugin{
		Category: registry.CategoryIMS,
		Name:     "kubernetes",
		Version:  "1.0.0",
		Status:   registry.StatusActive,
		Instance: &mockAdapter{},
	}

	// First registration should succeed
	err := reg.Register(plugin)
	require.NoError(t, err)

	// Second registration should fail
	err = reg.Register(plugin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Get(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugin := &registry.Plugin{
		Category: registry.CategoryIMS,
		Name:     "kubernetes",
		Version:  "1.0.0",
		Status:   registry.StatusActive,
		Instance: &mockAdapter{},
	}

	err := reg.Register(plugin)
	require.NoError(t, err)

	t.Run("existing plugin", func(t *testing.T) {
		retrieved, err := reg.Get(registry.CategoryIMS, "kubernetes")
		require.NoError(t, err)
		assert.Equal(t, plugin.Name, retrieved.Name)
		assert.Equal(t, plugin.Version, retrieved.Version)
	})

	t.Run("non-existent plugin", func(t *testing.T) {
		_, err := reg.Get(registry.CategoryIMS, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("non-existent category", func(t *testing.T) {
		_, err := reg.Get(registry.CategoryObservability, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no plugins registered")
	})
}

func TestRegistry_Unregister(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugin := &registry.Plugin{
		Category: registry.CategoryIMS,
		Name:     "kubernetes",
		Version:  "1.0.0",
		Status:   registry.StatusActive,
		Instance: &mockAdapter{},
	}

	err := reg.Register(plugin)
	require.NoError(t, err)

	t.Run("unregister existing", func(t *testing.T) {
		err := reg.Unregister(registry.CategoryIMS, "kubernetes")
		require.NoError(t, err)

		// Should not be retrievable anymore
		_, err = reg.Get(registry.CategoryIMS, "kubernetes")
		require.Error(t, err)
	})

	t.Run("unregister non-existent", func(t *testing.T) {
		err := reg.Unregister(registry.CategoryIMS, "nonexistent")
		require.Error(t, err)
	})
}

func TestRegistry_List(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugins := []*registry.Plugin{
		{
			Category: registry.CategoryIMS,
			Name:     "kubernetes",
			Version:  "1.0.0",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
		{
			Category: registry.CategoryIMS,
			Name:     "openstack",
			Version:  "2.0.0",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
		{
			Category: registry.CategoryDMS,
			Name:     "helm",
			Version:  "3.0.0",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
	}

	for _, p := range plugins {
		err := reg.Register(p)
		require.NoError(t, err)
	}

	t.Run("list IMS plugins", func(t *testing.T) {
		imsList := reg.List(registry.CategoryIMS)
		assert.Len(t, imsList, 2)
	})

	t.Run("list DMS plugins", func(t *testing.T) {
		dmsList := reg.List(registry.CategoryDMS)
		assert.Len(t, dmsList, 1)
	})

	t.Run("list empty category", func(t *testing.T) {
		smoList := reg.List(registry.CategorySMO)
		assert.Len(t, smoList, 0)
	})
}

func TestRegistry_ListAll(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugins := []*registry.Plugin{
		{
			Category: registry.CategoryIMS,
			Name:     "kubernetes",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
		{
			Category: registry.CategoryDMS,
			Name:     "helm",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
		{
			Category: registry.CategorySMO,
			Name:     "onap",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
	}

	for _, p := range plugins {
		err := reg.Register(p)
		require.NoError(t, err)
	}

	allPlugins := reg.ListAll()
	assert.Len(t, allPlugins, 3)
	assert.Len(t, allPlugins[registry.CategoryIMS], 1)
	assert.Len(t, allPlugins[registry.CategoryDMS], 1)
	assert.Len(t, allPlugins[registry.CategorySMO], 1)
}

func TestRegistry_UpdateStatus(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugin := &registry.Plugin{
		Category: registry.CategoryIMS,
		Name:     "kubernetes",
		Status:   registry.StatusActive,
		Instance: &mockAdapter{},
	}

	err := reg.Register(plugin)
	require.NoError(t, err)

	t.Run("update to unhealthy", func(t *testing.T) {
		err := reg.UpdateStatus(registry.CategoryIMS, "kubernetes", registry.StatusUnhealthy)
		require.NoError(t, err)

		retrieved, err := reg.Get(registry.CategoryIMS, "kubernetes")
		require.NoError(t, err)
		assert.Equal(t, registry.StatusUnhealthy, retrieved.Status)
	})

	t.Run("update non-existent", func(t *testing.T) {
		err := reg.UpdateStatus(registry.CategoryIMS, "nonexistent", registry.StatusActive)
		require.Error(t, err)
	})
}

func TestRegistry_SelectPlugin(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugins := []*registry.Plugin{
		{
			Category:     registry.CategoryIMS,
			Name:         "kubernetes",
			Priority:     100,
			Status:       registry.StatusActive,
			Capabilities: []string{"nodes", "machines"},
			Instance:     &mockAdapter{},
		},
		{
			Category:     registry.CategoryIMS,
			Name:         "openstack",
			Priority:     80,
			Status:       registry.StatusActive,
			Capabilities: []string{"nodes", "instances"},
			Instance:     &mockAdapter{},
		},
		{
			Category:     registry.CategoryIMS,
			Name:         "vmware",
			Priority:     90,
			Status:       registry.StatusDisabled,
			Capabilities: []string{"nodes", "vms"},
			Instance:     &mockAdapter{},
		},
	}

	for _, p := range plugins {
		err := reg.Register(p)
		require.NoError(t, err)
	}

	t.Run("select highest priority", func(t *testing.T) {
		plugin, err := reg.SelectPlugin(registry.CategoryIMS, nil)
		require.NoError(t, err)
		assert.Equal(t, "kubernetes", plugin.Name)
		assert.Equal(t, 100, plugin.Priority)
	})

	t.Run("select by capability", func(t *testing.T) {
		plugin, err := reg.SelectPlugin(registry.CategoryIMS, map[string]interface{}{
			"capability": "instances",
		})
		require.NoError(t, err)
		assert.Equal(t, "openstack", plugin.Name)
	})

	t.Run("select by name", func(t *testing.T) {
		plugin, err := reg.SelectPlugin(registry.CategoryIMS, map[string]interface{}{
			"name": "openstack",
		})
		require.NoError(t, err)
		assert.Equal(t, "openstack", plugin.Name)
	})

	t.Run("no matching plugin", func(t *testing.T) {
		_, err := reg.SelectPlugin(registry.CategoryIMS, map[string]interface{}{
			"capability": "nonexistent",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no active plugin found")
	})

	t.Run("empty category", func(t *testing.T) {
		_, err := reg.SelectPlugin(registry.CategoryObservability, nil)
		require.Error(t, err)
	})
}

func TestRegistry_Stats(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugins := []*registry.Plugin{
		{
			Category: registry.CategoryIMS,
			Name:     "kubernetes",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
		{
			Category: registry.CategoryIMS,
			Name:     "openstack",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
		{
			Category: registry.CategoryDMS,
			Name:     "helm",
			Status:   registry.StatusDisabled,
			Instance: &mockAdapter{},
		},
	}

	for _, p := range plugins {
		err := reg.Register(p)
		require.NoError(t, err)
	}

	stats := reg.Stats()
	assert.Equal(t, 3, stats["total_plugins"])

	byCategory := stats["by_category"].(map[string]int)
	assert.Equal(t, 2, byCategory["ims"])
	assert.Equal(t, 1, byCategory["dms"])

	byStatus := stats["by_status"].(map[string]int)
	assert.Equal(t, 2, byStatus["active"])
	assert.Equal(t, 1, byStatus["disabled"])
}

func TestRegistry_HealthCheck(t *testing.T) {
	reg := registry.New(zap.NewNop())

	plugins := []*registry.Plugin{
		{
			Category: registry.CategoryIMS,
			Name:     "healthy",
			Status:   registry.StatusActive,
			Instance: &mockHealthyAdapter{},
		},
		{
			Category: registry.CategoryIMS,
			Name:     "unhealthy",
			Status:   registry.StatusActive,
			Instance: &mockUnhealthyAdapter{},
		},
		{
			Category: registry.CategoryIMS,
			Name:     "no-health",
			Status:   registry.StatusActive,
			Instance: &mockAdapter{},
		},
	}

	for _, p := range plugins {
		err := reg.Register(p)
		require.NoError(t, err)
	}

	ctx := context.Background()
	results := reg.HealthCheck(ctx)

	assert.NoError(t, results["ims/healthy"])
	assert.Error(t, results["ims/unhealthy"])
	assert.NoError(t, results["ims/no-health"])
}

// Mock adapters for testing

type mockAdapter struct{}

type mockHealthyAdapter struct{}

func (m *mockHealthyAdapter) Health(_ context.Context) error {
	return nil
}

type mockUnhealthyAdapter struct{}

func (m *mockUnhealthyAdapter) Health(_ context.Context) error {
	return assert.AnError
}
