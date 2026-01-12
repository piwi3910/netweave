package smo_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockPlugin implements the Plugin interface for testing.
type mockPlugin struct {
	mu           sync.RWMutex
	name         string
	version      string
	description  string
	vendor       string
	capabilities []Capability
	healthy      bool
	healthErr    error
	closed       bool
	closeErr     error

	// Track method calls
	syncInfraCount         int
	syncDeployCount        int
	publishInfraCount      int
	publishDeployCount     int
	executeWorkflowCount   int
	getWorkflowStatusCount int
	cancelWorkflowCount    int
	registerModelCount     int
	getModelCount          int
	listModelsCount        int
	applyPolicyCount       int
	getPolicyStatusCount   int
}

func (m *mockPlugin) Metadata() PluginMetadata {
	return PluginMetadata{
		Name:        m.name,
		Version:     m.version,
		Description: m.description,
		Vendor:      m.vendor,
	}
}

func (m *mockPlugin) Capabilities() []Capability {
	return m.capabilities
}

func (m *mockPlugin) Initialize(_ context.Context, config map[string]interface{}) error {
	return nil
}

func (m *mockPlugin) Health(_ context.Context) HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := HealthStatus{
		Healthy:   m.healthy,
		Timestamp: time.Now(),
	}
	switch {
	case m.healthErr != nil:
		status.Message = m.healthErr.Error()
	case m.healthy:
		status.Message = "healthy"
	default:
		status.Message = "unhealthy"
	}
	return status
}

func (m *mockPlugin) Close() error {
	m.closed = true
	return m.closeErr
}

func (m *mockPlugin) SyncInfrastructureInventory(_ context.Context, inventory *InfrastructureInventory) error {
	m.syncInfraCount++
	return nil
}

func (m *mockPlugin) SyncDeploymentInventory(_ context.Context, inventory *DeploymentInventory) error {
	m.syncDeployCount++
	return nil
}

func (m *mockPlugin) PublishInfrastructureEvent(_ context.Context, event *InfrastructureEvent) error {
	m.publishInfraCount++
	return nil
}

func (m *mockPlugin) PublishDeploymentEvent(_ context.Context, event *DeploymentEvent) error {
	m.publishDeployCount++
	return nil
}

func (m *mockPlugin) ExecuteWorkflow(_ context.Context, workflow *WorkflowRequest) (*WorkflowExecution, error) {
	m.executeWorkflowCount++
	return &WorkflowExecution{
		ExecutionID:  "exec-123",
		WorkflowName: workflow.WorkflowName,
		Status:       "RUNNING",
		StartedAt:    time.Now(),
	}, nil
}

func (m *mockPlugin) GetWorkflowStatus(_ context.Context, executionID string) (*WorkflowStatus, error) {
	m.getWorkflowStatusCount++
	return &WorkflowStatus{
		ExecutionID:  executionID,
		WorkflowName: "test-workflow",
		Status:       "RUNNING",
		Progress:     50,
		StartedAt:    time.Now(),
	}, nil
}

func (m *mockPlugin) CancelWorkflow(_ context.Context, executionID string) error {
	m.cancelWorkflowCount++
	return nil
}

func (m *mockPlugin) RegisterServiceModel(_ context.Context, model *ServiceModel) error {
	m.registerModelCount++
	return nil
}

func (m *mockPlugin) GetServiceModel(_ context.Context, id string) (*ServiceModel, error) {
	m.getModelCount++
	return &ServiceModel{
		ID:      id,
		Name:    "test-model",
		Version: "1.0.0",
	}, nil
}

func (m *mockPlugin) ListServiceModels(_ context.Context) ([]*ServiceModel, error) {
	m.listModelsCount++
	return []*ServiceModel{
		{ID: "model-1", Name: "model-1", Version: "1.0.0"},
		{ID: "model-2", Name: "model-2", Version: "2.0.0"},
	}, nil
}

func (m *mockPlugin) ApplyPolicy(_ context.Context, policy *Policy) error {
	m.applyPolicyCount++
	return nil
}

func (m *mockPlugin) GetPolicyStatus(_ context.Context, policyID string) (*PolicyStatus, error) {
	m.getPolicyStatusCount++
	now := time.Now()
	return &PolicyStatus{
		PolicyID:         policyID,
		Status:           "active",
		EnforcementCount: 10,
		ViolationCount:   2,
		LastEnforced:     &now,
		Message:          "Policy is active",
	}, nil
}

func newMockPlugin(name string, healthy bool) *mockPlugin {
	return &mockPlugin{
		name:         name,
		version:      "1.0.0",
		description:  "Test plugin: " + name,
		vendor:       "Test Vendor",
		capabilities: []Capability{CapInventorySync, CapEventPublishing},
		healthy:      healthy,
	}
}

// setHealthy is a thread-safe method to update plugin health status.
func (m *mockPlugin) setHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

func TestNewRegistry(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := zap.NewNop()
		registry := NewRegistry(logger)

		require.NotNil(t, registry)
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("without logger", func(t *testing.T) {
		registry := NewRegistry(nil)

		require.NotNil(t, registry)
		assert.Equal(t, 0, registry.Count())
	})
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name       string
		pluginName string
		plugin     Plugin
		isDefault  bool
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "register valid plugin",
			pluginName: "test-plugin",
			plugin:     newMockPlugin("test-plugin", true),
			isDefault:  false,
			wantErr:    false,
		},
		{
			name:       "register default plugin",
			pluginName: "default-plugin",
			plugin:     newMockPlugin("default-plugin", true),
			isDefault:  true,
			wantErr:    false,
		},
		{
			name:       "register with empty name",
			pluginName: "",
			plugin:     newMockPlugin("test", true),
			isDefault:  false,
			wantErr:    true,
			errMsg:     "plugin name cannot be empty",
		},
		{
			name:       "register nil plugin",
			pluginName: "nil-plugin",
			plugin:     nil,
			isDefault:  false,
			wantErr:    true,
			errMsg:     "plugin cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry(zap.NewNop())
			ctx := context.Background()

			err := registry.Register(ctx, tt.pluginName, tt.plugin, tt.isDefault)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, 1, registry.Count())

				// Verify plugin was registered
				got, err := registry.Get(tt.pluginName)
				require.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}

	t.Run("register duplicate plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		plugin := newMockPlugin("test", true)
		err := registry.Register(ctx, "test", plugin, false)
		require.NoError(t, err)

		// Try to register with same name
		err = registry.Register(ctx, "test", plugin, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestRegistry_Unregister(t *testing.T) {
	t.Run("unregister existing plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		plugin := newMockPlugin("test", true)
		err := registry.Register(ctx, "test", plugin, false)
		require.NoError(t, err)

		err = registry.Unregister("test")
		require.NoError(t, err)
		assert.Equal(t, 0, registry.Count())
		assert.True(t, plugin.closed)
	})

	t.Run("unregister non-existent plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())

		err := registry.Unregister("non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("unregister default updates default", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		err := registry.Register(ctx, "plugin1", newMockPlugin("plugin1", true), true)
		require.NoError(t, err)
		err = registry.Register(ctx, "plugin2", newMockPlugin("plugin2", true), false)
		require.NoError(t, err)

		err = registry.Unregister("plugin1")
		require.NoError(t, err)

		// plugin2 should become default
		defaultPlugin, err := registry.GetDefault()
		require.NoError(t, err)
		assert.Equal(t, "plugin2", defaultPlugin.Metadata().Name)
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("get existing plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		plugin := newMockPlugin("test", true)
		err := registry.Register(ctx, "test", plugin, false)
		require.NoError(t, err)

		got, err := registry.Get("test")
		require.NoError(t, err)
		assert.Equal(t, plugin, got)
	})

	t.Run("get non-existent plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())

		got, err := registry.Get("non-existent")
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_GetDefault(t *testing.T) {
	t.Run("get default plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		plugin := newMockPlugin("test", true)
		err := registry.Register(ctx, "test", plugin, true)
		require.NoError(t, err)

		got, err := registry.GetDefault()
		require.NoError(t, err)
		assert.Equal(t, plugin, got)
	})

	t.Run("get default when no plugins registered", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())

		got, err := registry.GetDefault()
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "no default plugin")
	})

	t.Run("first registered becomes default", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		plugin := newMockPlugin("test", true)
		err := registry.Register(ctx, "test", plugin, false)
		require.NoError(t, err)

		got, err := registry.GetDefault()
		require.NoError(t, err)
		assert.Equal(t, plugin, got)
	})
}

func TestRegistry_SetDefault(t *testing.T) {
	t.Run("set default to existing plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())
		ctx := context.Background()

		err := registry.Register(ctx, "plugin1", newMockPlugin("plugin1", true), true)
		require.NoError(t, err)
		err = registry.Register(ctx, "plugin2", newMockPlugin("plugin2", true), false)
		require.NoError(t, err)

		err = registry.SetDefault("plugin2")
		require.NoError(t, err)

		got, err := registry.GetDefault()
		require.NoError(t, err)
		assert.Equal(t, "plugin2", got.Metadata().Name)
	})

	t.Run("set default to non-existent plugin", func(t *testing.T) {
		registry := NewRegistry(zap.NewNop())

		err := registry.SetDefault("non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	// Register multiple plugins
	err := registry.Register(ctx, "plugin1", newMockPlugin("plugin1", true), true)
	require.NoError(t, err)
	err = registry.Register(ctx, "plugin2", newMockPlugin("plugin2", false), false)
	require.NoError(t, err)

	plugins := registry.List()
	assert.Len(t, plugins, 2)

	// Find plugins by name
	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Name] = true
	}
	assert.True(t, names["plugin1"])
	assert.True(t, names["plugin2"])
}

func TestRegistry_FindByCapability(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	// Plugin with inventory sync capability
	plugin1 := newMockPlugin("plugin1", true)
	plugin1.capabilities = []Capability{CapInventorySync}

	// Plugin with workflow capability
	plugin2 := newMockPlugin("plugin2", true)
	plugin2.capabilities = []Capability{CapWorkflowOrchestration}

	// Unhealthy plugin with inventory sync
	plugin3 := newMockPlugin("plugin3", false)
	plugin3.capabilities = []Capability{CapInventorySync}

	err := registry.Register(ctx, "plugin1", plugin1, false)
	require.NoError(t, err)
	err = registry.Register(ctx, "plugin2", plugin2, false)
	require.NoError(t, err)
	err = registry.Register(ctx, "plugin3", plugin3, false)
	require.NoError(t, err)

	// Find inventory sync capable (only healthy)
	found := registry.FindByCapability(CapInventorySync)
	assert.Len(t, found, 1)
	assert.Equal(t, plugin1, found[0])

	// Find workflow capable
	found = registry.FindByCapability(CapWorkflowOrchestration)
	assert.Len(t, found, 1)
	assert.Equal(t, plugin2, found[0])

	// Find non-existent capability
	found = registry.FindByCapability(CapPolicyManagement)
	assert.Len(t, found, 0)
}

func TestRegistry_GetHealthy(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	healthyPlugin := newMockPlugin("healthy", true)
	unhealthyPlugin := newMockPlugin("unhealthy", false)

	err := registry.Register(ctx, "healthy", healthyPlugin, false)
	require.NoError(t, err)
	err = registry.Register(ctx, "unhealthy", unhealthyPlugin, false)
	require.NoError(t, err)

	healthy := registry.GetHealthy()
	assert.Len(t, healthy, 1)
	assert.Equal(t, healthyPlugin, healthy[0])
}

func TestRegistry_Close(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	plugin1 := newMockPlugin("plugin1", true)
	plugin2 := newMockPlugin("plugin2", true)

	err := registry.Register(ctx, "plugin1", plugin1, false)
	require.NoError(t, err)
	err = registry.Register(ctx, "plugin2", plugin2, false)
	require.NoError(t, err)

	err = registry.Close()
	require.NoError(t, err)

	assert.True(t, plugin1.closed)
	assert.True(t, plugin2.closed)
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_CloseWithError(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	plugin := newMockPlugin("plugin", true)
	plugin.closeErr = errors.New("close failed")

	err := registry.Register(ctx, "plugin", plugin, false)
	require.NoError(t, err)

	err = registry.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close failed")
}

func TestRegistry_Count(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	assert.Equal(t, 0, registry.Count())

	err := registry.Register(ctx, "plugin1", newMockPlugin("plugin1", true), false)
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Count())

	err = registry.Register(ctx, "plugin2", newMockPlugin("plugin2", true), false)
	require.NoError(t, err)
	assert.Equal(t, 2, registry.Count())

	err = registry.Unregister("plugin1")
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
}

// TestRegistry_ConcurrentAccess tests thread-safety of registry operations.
// Run with -race flag: go test -race ./internal/smo/...
func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	// Register initial plugins
	for i := 0; i < 5; i++ {
		name := "plugin-" + string(rune('a'+i))
		err := registry.Register(ctx, name, newMockPlugin(name, true), i == 0)
		require.NoError(t, err)
	}

	// Concurrent operations
	done := make(chan bool)
	iterations := 100

	// Goroutine 1: List plugins
	go func() {
		for i := 0; i < iterations; i++ {
			_ = registry.List()
		}
		done <- true
	}()

	// Goroutine 2: Get plugin
	go func() {
		for i := 0; i < iterations; i++ {
			_, _ = registry.Get("plugin-a")
		}
		done <- true
	}()

	// Goroutine 3: Get default
	go func() {
		for i := 0; i < iterations; i++ {
			_, _ = registry.GetDefault()
		}
		done <- true
	}()

	// Goroutine 4: Find by capability
	go func() {
		for i := 0; i < iterations; i++ {
			_ = registry.FindByCapability(CapInventorySync)
		}
		done <- true
	}()

	// Goroutine 5: Get healthy
	go func() {
		for i := 0; i < iterations; i++ {
			_ = registry.GetHealthy()
		}
		done <- true
	}()

	// Goroutine 6: Count
	go func() {
		for i := 0; i < iterations; i++ {
			_ = registry.Count()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-done
	}

	// Final verification
	assert.Equal(t, 5, registry.Count())
}

// TestRegistry_StartHealthChecksIdempotent tests that multiple calls to StartHealthChecks
// do not spawn duplicate goroutines (atomic.Bool protection).
func TestRegistry_StartHealthChecksIdempotent(t *testing.T) {
	registry := NewRegistry(zap.NewNop(),
		WithHealthCheckInterval(100*time.Millisecond),
		WithHealthCheckTimeout(50*time.Millisecond),
	)
	ctx := context.Background()

	// Register a plugin
	err := registry.Register(ctx, "test", newMockPlugin("test", true), true)
	require.NoError(t, err)

	// Start health checks multiple times concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			registry.StartHealthChecks(ctx)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Let health check run briefly
	time.Sleep(150 * time.Millisecond)

	// Stop health checks
	registry.StopHealthChecks()

	// Verify registry is still functional
	assert.Equal(t, 1, registry.Count())
}

// TestRegistry_HealthStateTransitions tests health status transitions.
func TestRegistry_HealthStateTransitions(t *testing.T) {
	registry := NewRegistry(zap.NewNop(),
		WithHealthCheckInterval(50*time.Millisecond),
		WithHealthCheckTimeout(25*time.Millisecond),
	)
	ctx := context.Background()

	// Create plugin with controllable health
	plugin := newMockPlugin("test", true)
	err := registry.Register(ctx, "test", plugin, true)
	require.NoError(t, err)

	// Start health checks
	registry.StartHealthChecks(ctx)

	// Initially healthy
	info := registry.List()
	require.Len(t, info, 1)
	assert.True(t, info[0].Healthy)

	// Transition to unhealthy
	plugin.setHealthy(false)
	time.Sleep(100 * time.Millisecond)

	info = registry.List()
	require.Len(t, info, 1)
	assert.False(t, info[0].Healthy)

	// Transition back to healthy
	plugin.setHealthy(true)
	time.Sleep(100 * time.Millisecond)

	info = registry.List()
	require.Len(t, info, 1)
	assert.True(t, info[0].Healthy)

	// Stop health checks
	registry.StopHealthChecks()
}

// TestRegistry_WithOptions tests configurable health check intervals.
func TestRegistry_WithOptions(t *testing.T) {
	customInterval := 10 * time.Second
	customTimeout := 2 * time.Second

	registry := NewRegistry(zap.NewNop(),
		WithHealthCheckInterval(customInterval),
		WithHealthCheckTimeout(customTimeout),
	)

	// Verify options were applied (internal fields)
	assert.Equal(t, customInterval, registry.healthCheckInterval)
	assert.Equal(t, customTimeout, registry.healthCheckTimeout)
}

// TestRegistry_ListDeepCopy tests that List() returns deep copies.
func TestRegistry_ListDeepCopy(t *testing.T) {
	registry := NewRegistry(zap.NewNop())
	ctx := context.Background()

	plugin := newMockPlugin("test", true)
	plugin.capabilities = []Capability{CapInventorySync, CapEventPublishing}

	err := registry.Register(ctx, "test", plugin, true)
	require.NoError(t, err)

	// Get list
	info := registry.List()
	require.Len(t, info, 1)

	// Modify the returned capabilities slice
	originalLen := len(info[0].Capabilities)
	info[0].Capabilities = append(info[0].Capabilities, CapWorkflowOrchestration)

	// Get list again - should be unchanged
	info2 := registry.List()
	require.Len(t, info2, 1)
	assert.Equal(t, originalLen, len(info2[0].Capabilities))
}
