package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockDMSAdapter implements adapter.DMSAdapter for testing.
type mockDMSAdapter struct {
	name         string
	version      string
	capabilities []adapter.Capability
	healthy      bool
	healthErr    error
	closed       bool
}

func newMockDMSAdapter(name string) *mockDMSAdapter {
	return &mockDMSAdapter{
		name:         name,
		version:      "1.0.0",
		capabilities: []adapter.Capability{adapter.CapabilityDeploymentLifecycle},
		healthy:      true,
	}
}

func (m *mockDMSAdapter) Name() string                       { return m.name }
func (m *mockDMSAdapter) Version() string                    { return m.version }
func (m *mockDMSAdapter) Capabilities() []adapter.Capability { return m.capabilities }

func (m *mockDMSAdapter) ListDeploymentPackages(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentPackage, error) {
	return nil, nil
}

func (m *mockDMSAdapter) GetDeploymentPackage(_ context.Context, _ string) (*adapter.DeploymentPackage, error) {
	return nil, nil
}

func (m *mockDMSAdapter) UploadDeploymentPackage(_ context.Context, _ *adapter.DeploymentPackageUpload) (*adapter.DeploymentPackage, error) {
	return nil, nil
}

func (m *mockDMSAdapter) DeleteDeploymentPackage(_ context.Context, _ string) error {
	return nil
}

func (m *mockDMSAdapter) ListDeployments(_ context.Context, _ *adapter.Filter) ([]*adapter.Deployment, error) {
	return nil, nil
}

func (m *mockDMSAdapter) GetDeployment(_ context.Context, _ string) (*adapter.Deployment, error) {
	return nil, nil
}

func (m *mockDMSAdapter) CreateDeployment(_ context.Context, _ *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	return nil, nil
}

func (m *mockDMSAdapter) UpdateDeployment(_ context.Context, _ string, _ *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	return nil, nil
}

func (m *mockDMSAdapter) DeleteDeployment(_ context.Context, _ string) error {
	return nil
}

func (m *mockDMSAdapter) ScaleDeployment(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockDMSAdapter) RollbackDeployment(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockDMSAdapter) GetDeploymentStatus(_ context.Context, _ string) (*adapter.DeploymentStatusDetail, error) {
	return nil, nil
}

func (m *mockDMSAdapter) GetDeploymentHistory(_ context.Context, _ string) (*adapter.DeploymentHistory, error) {
	return nil, nil
}

func (m *mockDMSAdapter) GetDeploymentLogs(_ context.Context, _ string, _ *adapter.LogOptions) ([]byte, error) {
	return nil, nil
}

func (m *mockDMSAdapter) SupportsRollback() bool { return true }
func (m *mockDMSAdapter) SupportsScaling() bool  { return true }
func (m *mockDMSAdapter) SupportsGitOps() bool   { return false }

func (m *mockDMSAdapter) Health(_ context.Context) error {
	if !m.healthy {
		return m.healthErr
	}
	return nil
}

func (m *mockDMSAdapter) Close() error {
	m.closed = true
	return nil
}

func TestRegistry_Register(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("test-adapter")

	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, false)
	require.NoError(t, err)

	// Verify registration.
	adp := reg.Get("test-adapter")
	assert.NotNil(t, adp)
	assert.Equal(t, "test-adapter", adp.Name())
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("test-adapter")

	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, false)
	require.NoError(t, err)

	// Try to register again with the same name.
	err = reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_RegisterDefault(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("default-adapter")

	err := reg.Register(context.Background(), "default-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	// Verify it's set as default.
	defaultAdp := reg.GetDefault()
	assert.NotNil(t, defaultAdp)
	assert.Equal(t, "default-adapter", defaultAdp.Name())
}

func TestRegistry_Unregister(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("test-adapter")

	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, false)
	require.NoError(t, err)

	err = reg.Unregister("test-adapter")
	require.NoError(t, err)

	// Verify it's removed.
	adp := reg.Get("test-adapter")
	assert.Nil(t, adp)
	assert.True(t, mockAdp.closed)
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	err := reg.Unregister("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_Get(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("test-adapter")

	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, false)
	require.NoError(t, err)

	// Get existing adapter.
	adp := reg.Get("test-adapter")
	assert.NotNil(t, adp)

	// Get non-existent adapter.
	adp = reg.Get("nonexistent")
	assert.Nil(t, adp)
}

func TestRegistry_GetDefault(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// No default set yet.
	adp := reg.GetDefault()
	assert.Nil(t, adp)

	// Register a default adapter.
	mockAdp := newMockDMSAdapter("default-adapter")
	err := reg.Register(context.Background(), "default-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	adp = reg.GetDefault()
	assert.NotNil(t, adp)
	assert.Equal(t, "default-adapter", adp.Name())
}

func TestRegistry_GetMetadata(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("test-adapter")
	mockAdp.capabilities = []adapter.Capability{
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityRollback,
	}

	config := map[string]interface{}{"key": "value"}
	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, config, true)
	require.NoError(t, err)

	meta := reg.GetMetadata("test-adapter")
	assert.NotNil(t, meta)
	assert.Equal(t, "test-adapter", meta.Name)
	assert.Equal(t, "mock", meta.Type)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.True(t, meta.Enabled)
	assert.True(t, meta.Default)
	assert.True(t, meta.Healthy)
	assert.Len(t, meta.Capabilities, 2)
	assert.Equal(t, "value", meta.Config["key"])
}

func TestRegistry_GetMetadataNotFound(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	meta := reg.GetMetadata("nonexistent")
	assert.Nil(t, meta)
}

func TestRegistry_List(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// Register multiple adapters.
	for i := 0; i < 3; i++ {
		mockAdp := newMockDMSAdapter("adapter-" + string(rune('a'+i)))
		err := reg.Register(context.Background(), mockAdp.name, "mock", mockAdp, nil, false)
		require.NoError(t, err)
	}

	adapters := reg.List()
	assert.Len(t, adapters, 3)
}

func TestRegistry_ListMetadata(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// Register multiple adapters.
	for i := 0; i < 2; i++ {
		mockAdp := newMockDMSAdapter("adapter-" + string(rune('a'+i)))
		err := reg.Register(context.Background(), mockAdp.name, "mock", mockAdp, nil, false)
		require.NoError(t, err)
	}

	metadata := reg.ListMetadata()
	assert.Len(t, metadata, 2)
}

func TestRegistry_ListHealthy(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// Register healthy adapter.
	healthyAdp := newMockDMSAdapter("healthy")
	err := reg.Register(context.Background(), "healthy", "mock", healthyAdp, nil, false)
	require.NoError(t, err)

	// Register unhealthy adapter.
	unhealthyAdp := newMockDMSAdapter("unhealthy")
	unhealthyAdp.healthy = false
	unhealthyAdp.healthErr = errors.New("unhealthy")
	err = reg.Register(context.Background(), "unhealthy", "mock", unhealthyAdp, nil, false)
	require.NoError(t, err)

	healthy := reg.ListHealthy()
	assert.Len(t, healthy, 1)
	assert.Equal(t, "healthy", healthy[0].Name())
}

func TestRegistry_FindByCapability(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// Register adapter with rollback capability.
	rollbackAdp := newMockDMSAdapter("rollback-adapter")
	rollbackAdp.capabilities = []adapter.Capability{adapter.CapabilityRollback}
	err := reg.Register(context.Background(), "rollback-adapter", "mock", rollbackAdp, nil, false)
	require.NoError(t, err)

	// Register adapter without rollback capability.
	basicAdp := newMockDMSAdapter("basic-adapter")
	basicAdp.capabilities = []adapter.Capability{adapter.CapabilityDeploymentLifecycle}
	err = reg.Register(context.Background(), "basic-adapter", "mock", basicAdp, nil, false)
	require.NoError(t, err)

	// Find by rollback capability.
	adapters := reg.FindByCapability(adapter.CapabilityRollback)
	assert.Len(t, adapters, 1)
	assert.Equal(t, "rollback-adapter", adapters[0].Name())
}

func TestRegistry_FindByType(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// Register helm adapter.
	helmAdp := newMockDMSAdapter("helm-adapter")
	err := reg.Register(context.Background(), "helm-adapter", "helm", helmAdp, nil, false)
	require.NoError(t, err)

	// Register argocd adapter.
	argoAdp := newMockDMSAdapter("argocd-adapter")
	err = reg.Register(context.Background(), "argocd-adapter", "argocd", argoAdp, nil, false)
	require.NoError(t, err)

	// Find by type.
	helmAdapters := reg.FindByType("helm")
	assert.Len(t, helmAdapters, 1)
	assert.Equal(t, "helm-adapter", helmAdapters[0].Name())
}

func TestRegistry_EnableDisable(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	mockAdp := newMockDMSAdapter("test-adapter")
	err := reg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, false)
	require.NoError(t, err)

	// Verify initially enabled.
	meta := reg.GetMetadata("test-adapter")
	assert.True(t, meta.Enabled)

	// Disable.
	err = reg.Disable("test-adapter")
	require.NoError(t, err)

	meta = reg.GetMetadata("test-adapter")
	assert.False(t, meta.Enabled)

	// Enable.
	err = reg.Enable("test-adapter")
	require.NoError(t, err)

	meta = reg.GetMetadata("test-adapter")
	assert.True(t, meta.Enabled)
}

func TestRegistry_EnableNotFound(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	err := reg.Enable("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_DisableNotFound(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	err := reg.Disable("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_SetDefault(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	// Register two adapters.
	adp1 := newMockDMSAdapter("adapter-1")
	err := reg.Register(context.Background(), "adapter-1", "mock", adp1, nil, true)
	require.NoError(t, err)

	adp2 := newMockDMSAdapter("adapter-2")
	err = reg.Register(context.Background(), "adapter-2", "mock", adp2, nil, false)
	require.NoError(t, err)

	// Verify initial default.
	assert.Equal(t, "adapter-1", reg.GetDefault().Name())

	// Change default.
	err = reg.SetDefault("adapter-2")
	require.NoError(t, err)

	assert.Equal(t, "adapter-2", reg.GetDefault().Name())

	// Verify metadata updated.
	meta1 := reg.GetMetadata("adapter-1")
	assert.False(t, meta1.Default)

	meta2 := reg.GetMetadata("adapter-2")
	assert.True(t, meta2.Default)
}

func TestRegistry_SetDefaultNotFound(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)
	defer reg.Close()

	err := reg.SetDefault("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_Close(t *testing.T) {
	logger := zap.NewNop()
	reg := NewRegistry(logger, nil)

	// Register multiple adapters.
	adapters := make([]*mockDMSAdapter, 3)
	for i := 0; i < 3; i++ {
		adapters[i] = newMockDMSAdapter("adapter-" + string(rune('a'+i)))
		err := reg.Register(context.Background(), adapters[i].name, "mock", adapters[i], nil, false)
		require.NoError(t, err)
	}

	// Close registry.
	err := reg.Close()
	require.NoError(t, err)

	// Verify all adapters are closed.
	for _, adp := range adapters {
		assert.True(t, adp.closed)
	}

	// Verify registry is empty.
	assert.Empty(t, reg.List())
}

func TestRegistry_Config(t *testing.T) {
	logger := zap.NewNop()

	config := &Config{
		HealthCheckInterval: 10 * time.Second,
		HealthCheckTimeout:  2 * time.Second,
	}

	reg := NewRegistry(logger, config)
	defer reg.Close()

	assert.Equal(t, 10*time.Second, reg.healthCheckInterval)
	assert.Equal(t, 2*time.Second, reg.healthCheckTimeout)
}

func TestRegistry_ConfigDefaults(t *testing.T) {
	logger := zap.NewNop()

	reg := NewRegistry(logger, nil)
	defer reg.Close()

	assert.Equal(t, 30*time.Second, reg.healthCheckInterval)
	assert.Equal(t, 5*time.Second, reg.healthCheckTimeout)
}
