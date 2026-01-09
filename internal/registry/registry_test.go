package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
)

// mockAdapter is a test implementation of the adapter interface.
type mockAdapter struct {
	mu           sync.RWMutex
	name         string
	version      string
	capabilities []adapter.Capability
	healthy      bool
	healthError  error
	closed       bool
}

func (m *mockAdapter) Name() string {
	return m.name
}

func (m *mockAdapter) Version() string {
	return m.version
}

func (m *mockAdapter) Capabilities() []adapter.Capability {
	return m.capabilities
}

func (m *mockAdapter) Health(_ context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.healthError != nil {
		return m.healthError
	}
	if !m.healthy {
		return errors.New("unhealthy")
	}
	return nil
}

func (m *mockAdapter) Close() error {
	m.closed = true
	return nil
}

// SetHealth safely sets the health status with proper locking.
func (m *mockAdapter) SetHealth(healthy bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
	m.healthError = err
}

// errNotImplemented is returned by stub methods not used in tests.
var errNotImplemented = errors.New("method not implemented in mock")

// Implement remaining adapter.Adapter methods.
func (m *mockAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetResourcePool(_ context.Context, _ string) (*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) CreateResourcePool(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) UpdateResourcePool(
	_ context.Context,
	_ string,
	_ *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) DeleteResourcePool(_ context.Context, _ string) error {
	return errNotImplemented
}

func (m *mockAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetResource(_ context.Context, _ string) (*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) CreateResource(_ context.Context, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) UpdateResource(_ context.Context, _ string, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) DeleteResource(_ context.Context, _ string) error {
	return errNotImplemented
}

func (m *mockAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetResourceType(_ context.Context, _ string) (*adapter.ResourceType, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) CreateSubscription(_ context.Context, _ *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return errNotImplemented
}

func TestNewRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "with nil config uses defaults",
			config: nil,
		},
		{
			name: "with custom config",
			config: &Config{
				HealthCheckInterval: 10 * time.Second,
				HealthCheckTimeout:  2 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(logger, tt.config)
			assert.NotNil(t, reg)
			assert.NotNil(t, reg.plugins)
			assert.NotNil(t, reg.meta)
		})
	}
}

func TestRegistry_Register(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		pluginName  string
		pluginType  string
		isDefault   bool
		healthErr   error
		expectError bool
	}{
		{
			name:        "register healthy plugin",
			pluginName:  "test-plugin",
			pluginType:  "kubernetes",
			isDefault:   true,
			healthErr:   nil,
			expectError: false,
		},
		{
			name:        "register unhealthy plugin",
			pluginName:  "unhealthy-plugin",
			pluginType:  "openstack",
			isDefault:   false,
			healthErr:   errors.New("health check failed"),
			expectError: false, // Registration succeeds even if health check fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(logger, nil)

			mock := &mockAdapter{
				name:    tt.pluginName,
				version: "1.0.0",
				capabilities: []adapter.Capability{
					adapter.CapabilityResourcePools,
					adapter.CapabilityResources,
				},
				healthy:     tt.healthErr == nil,
				healthError: tt.healthErr,
			}

			err := reg.Register(ctx, tt.pluginName, tt.pluginType, mock, nil, tt.isDefault)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify plugin is registered
				plugin := reg.Get(tt.pluginName)
				assert.NotNil(t, plugin)
				assert.Equal(t, mock, plugin)

				// Verify metadata
				meta := reg.GetMetadata(tt.pluginName)
				require.NotNil(t, meta)
				assert.Equal(t, tt.pluginName, meta.Name)
				assert.Equal(t, tt.pluginType, meta.Type)
				assert.Equal(t, "1.0.0", meta.Version)
				assert.Equal(t, tt.isDefault, meta.Default)
				assert.True(t, meta.Enabled)

				// Verify default setting
				if tt.isDefault {
					defaultPlugin := reg.GetDefault()
					assert.Equal(t, mock, defaultPlugin)
				}
			}
		})
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	mock := &mockAdapter{
		name:    "test-plugin",
		version: "1.0.0",
		healthy: true,
	}

	// First registration should succeed
	err := reg.Register(ctx, "test-plugin", "kubernetes", mock, nil, false)
	assert.NoError(t, err)

	// Second registration with same name should fail
	err = reg.Register(ctx, "test-plugin", "kubernetes", mock, nil, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Unregister(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	mock := &mockAdapter{
		name:    "test-plugin",
		version: "1.0.0",
		healthy: true,
	}

	err := reg.Register(ctx, "test-plugin", "kubernetes", mock, nil, true)
	require.NoError(t, err)

	// Unregister should succeed
	err = reg.Unregister("test-plugin")
	assert.NoError(t, err)

	// Plugin should be removed
	plugin := reg.Get("test-plugin")
	assert.Nil(t, plugin)

	// Metadata should be removed
	meta := reg.GetMetadata("test-plugin")
	assert.Nil(t, meta)

	// Default should be cleared
	defaultPlugin := reg.GetDefault()
	assert.Nil(t, defaultPlugin)

	// Mock should be closed
	assert.True(t, mock.closed)
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	reg := NewRegistry(logger, nil)

	err := reg.Unregister("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_List(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	// Register multiple plugins
	for i := 0; i < 3; i++ {
		mock := &mockAdapter{
			name:    fmt.Sprintf("plugin-%d", i),
			version: "1.0.0",
			healthy: true,
		}
		err := reg.Register(ctx, mock.name, "kubernetes", mock, nil, false)
		require.NoError(t, err)
	}

	plugins := reg.List()
	assert.Len(t, plugins, 3)
}

func TestRegistry_FindByCapability(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	// Register plugin with specific capabilities
	mock1 := &mockAdapter{
		name:    "plugin-1",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityResourcePools,
			adapter.CapabilityResources,
		},
		healthy: true,
	}

	mock2 := &mockAdapter{
		name:    "plugin-2",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityResourcePools,
		},
		healthy: true,
	}

	err := reg.Register(ctx, "plugin-1", "kubernetes", mock1, nil, false)
	require.NoError(t, err)

	err = reg.Register(ctx, "plugin-2", "openstack", mock2, nil, false)
	require.NoError(t, err)

	// Find by ResourcePools capability
	plugins := reg.FindByCapability(adapter.CapabilityResourcePools)
	assert.Len(t, plugins, 2)

	// Find by Resources capability
	plugins = reg.FindByCapability(adapter.CapabilityResources)
	assert.Len(t, plugins, 1)

	// Find by non-existent capability
	plugins = reg.FindByCapability(adapter.CapabilityMetrics)
	assert.Len(t, plugins, 0)
}

func TestRegistry_FindByType(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	// Register plugins of different types
	mock1 := &mockAdapter{
		name:    "k8s-plugin",
		version: "1.0.0",
		healthy: true,
	}

	mock2 := &mockAdapter{
		name:    "openstack-plugin",
		version: "1.0.0",
		healthy: true,
	}

	err := reg.Register(ctx, "k8s-plugin", "kubernetes", mock1, nil, false)
	require.NoError(t, err)

	err = reg.Register(ctx, "openstack-plugin", "openstack", mock2, nil, false)
	require.NoError(t, err)

	// Find by type
	plugins := reg.FindByType("kubernetes")
	assert.Len(t, plugins, 1)
	assert.Equal(t, mock1, plugins[0])

	plugins = reg.FindByType("openstack")
	assert.Len(t, plugins, 1)
	assert.Equal(t, mock2, plugins[0])

	plugins = reg.FindByType("non-existent")
	assert.Len(t, plugins, 0)
}

func TestRegistry_EnableDisable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	mock := &mockAdapter{
		name:    "test-plugin",
		version: "1.0.0",
		healthy: true,
	}

	err := reg.Register(ctx, "test-plugin", "kubernetes", mock, nil, false)
	require.NoError(t, err)

	// Initially enabled
	meta := reg.GetMetadata("test-plugin")
	assert.True(t, meta.Enabled)

	// Disable
	err = reg.Disable("test-plugin")
	assert.NoError(t, err)
	meta = reg.GetMetadata("test-plugin")
	assert.False(t, meta.Enabled)

	// Enable
	err = reg.Enable("test-plugin")
	assert.NoError(t, err)
	meta = reg.GetMetadata("test-plugin")
	assert.True(t, meta.Enabled)
}

func TestRegistry_SetDefault(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	mock1 := &mockAdapter{
		name:    "plugin-1",
		version: "1.0.0",
		healthy: true,
	}

	mock2 := &mockAdapter{
		name:    "plugin-2",
		version: "1.0.0",
		healthy: true,
	}

	err := reg.Register(ctx, "plugin-1", "kubernetes", mock1, nil, true)
	require.NoError(t, err)

	err = reg.Register(ctx, "plugin-2", "openstack", mock2, nil, false)
	require.NoError(t, err)

	// Initially plugin-1 is default
	defaultPlugin := reg.GetDefault()
	assert.Equal(t, mock1, defaultPlugin)
	assert.True(t, reg.GetMetadata("plugin-1").Default)
	assert.False(t, reg.GetMetadata("plugin-2").Default)

	// Change default to plugin-2
	err = reg.SetDefault("plugin-2")
	assert.NoError(t, err)

	defaultPlugin = reg.GetDefault()
	assert.Equal(t, mock2, defaultPlugin)
	assert.False(t, reg.GetMetadata("plugin-1").Default)
	assert.True(t, reg.GetMetadata("plugin-2").Default)

	// Try to set non-existent plugin as default
	err = reg.SetDefault("non-existent")
	assert.Error(t, err)
}

func TestRegistry_ListHealthy(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	// Register healthy plugin
	mock1 := &mockAdapter{
		name:    "healthy-plugin",
		version: "1.0.0",
		healthy: true,
	}

	// Register unhealthy plugin
	mock2 := &mockAdapter{
		name:        "unhealthy-plugin",
		version:     "1.0.0",
		healthy:     false,
		healthError: errors.New("health check failed"),
	}

	err := reg.Register(ctx, "healthy-plugin", "kubernetes", mock1, nil, false)
	require.NoError(t, err)

	err = reg.Register(ctx, "unhealthy-plugin", "openstack", mock2, nil, false)
	require.NoError(t, err)

	// Only healthy plugin should be returned
	healthyPlugins := reg.ListHealthy()
	assert.Len(t, healthyPlugins, 1)
	assert.Equal(t, mock1, healthyPlugins[0])
}

func TestRegistry_Close(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := NewRegistry(logger, nil)

	mock1 := &mockAdapter{
		name:    "plugin-1",
		version: "1.0.0",
		healthy: true,
	}

	mock2 := &mockAdapter{
		name:    "plugin-2",
		version: "1.0.0",
		healthy: true,
	}

	err := reg.Register(ctx, "plugin-1", "kubernetes", mock1, nil, false)
	require.NoError(t, err)

	err = reg.Register(ctx, "plugin-2", "openstack", mock2, nil, false)
	require.NoError(t, err)

	// Close registry
	err = reg.Close()
	assert.NoError(t, err)

	// All plugins should be closed
	assert.True(t, mock1.closed)
	assert.True(t, mock2.closed)

	// Registry should be empty
	assert.Len(t, reg.List(), 0)
	assert.Nil(t, reg.GetDefault())
}

func TestRegistry_HealthChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := &Config{
		HealthCheckInterval: 100 * time.Millisecond,
		HealthCheckTimeout:  50 * time.Millisecond,
	}

	reg := NewRegistry(logger, config)

	mock := &mockAdapter{
		name:    "test-plugin",
		version: "1.0.0",
		healthy: true,
	}

	err := reg.Register(ctx, "test-plugin", "kubernetes", mock, nil, false)
	require.NoError(t, err)

	// Start health checks
	reg.StartHealthChecks(ctx)

	// Wait for a health check cycle
	time.Sleep(150 * time.Millisecond)

	// Plugin should still be healthy
	meta := reg.GetMetadata("test-plugin")
	assert.True(t, meta.Healthy)

	// Make plugin unhealthy
	mock.SetHealth(false, errors.New("health check failed"))

	// Wait for another health check cycle
	time.Sleep(150 * time.Millisecond)

	// Plugin should now be unhealthy
	meta = reg.GetMetadata("test-plugin")
	assert.False(t, meta.Healthy)
	assert.NotNil(t, meta.HealthError)

	// Stop health checks
	reg.StopHealthChecks()

	// Close registry
	err = reg.Close()
	assert.NoError(t, err)
}
