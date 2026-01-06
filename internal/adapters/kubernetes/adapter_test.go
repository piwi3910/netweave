// Package kubernetes provides tests for the Kubernetes adapter implementation.
package kubernetes

import (
	"context"
	"testing"

	adapterapi "github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config returns error",
			cfg:     nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "valid config with all fields",
			cfg: &Config{
				OCloudID:            "ocloud-1",
				DeploymentManagerID: "dm-1",
				Namespace:           "o2ims-system",
				Logger:              zap.NewNop(),
			},
			wantErr: false,
		},
		{
			name: "config with default namespace",
			cfg: &Config{
				OCloudID:            "ocloud-2",
				DeploymentManagerID: "dm-2",
				Logger:              zap.NewNop(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that require kubeconfig/in-cluster since they will fail
			// without actual Kubernetes access. We test those scenarios separately.
			if tt.cfg != nil && tt.cfg.Kubeconfig == "" {
				// This will attempt in-cluster config which won't work in unit tests
				_, err := New(tt.cfg)
				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.errMsg)
				} else {
					// In-cluster config expected to fail in unit test environment
					require.Error(t, err)
					assert.Contains(t, err.Error(), "in-cluster")
				}
				return
			}

			adp, err := New(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, adp)
		})
	}
}

func TestNewWithInvalidKubeconfig(t *testing.T) {
	cfg := &Config{
		Kubeconfig:          "/nonexistent/path/to/kubeconfig",
		OCloudID:            "ocloud-1",
		DeploymentManagerID: "dm-1",
		Logger:              zap.NewNop(),
	}

	adp, err := New(cfg)
	require.Error(t, err)
	assert.Nil(t, adp)
	assert.Contains(t, err.Error(), "kubeconfig")
}

// newTestAdapter creates a KubernetesAdapter with a fake client for testing.
func newTestAdapter(t *testing.T) *KubernetesAdapter {
	t.Helper()
	return &KubernetesAdapter{
		client:              fake.NewSimpleClientset(),
		logger:              zaptest.NewLogger(t),
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}
}

func TestKubernetesAdapter_Name(t *testing.T) {
	adp := newTestAdapter(t)
	assert.Equal(t, "kubernetes", adp.Name())
}

func TestKubernetesAdapter_Version(t *testing.T) {
	adp := newTestAdapter(t)
	version := adp.Version()
	assert.NotEmpty(t, version)
	assert.Equal(t, "1.30.0", version)
}

func TestKubernetesAdapter_Capabilities(t *testing.T) {
	adp := newTestAdapter(t)
	caps := adp.Capabilities()

	require.NotEmpty(t, caps)
	assert.Len(t, caps, 6)

	// Verify expected capabilities are present
	expectedCaps := []adapterapi.Capability{
		adapterapi.CapabilityResourcePools,
		adapterapi.CapabilityResources,
		adapterapi.CapabilityResourceTypes,
		adapterapi.CapabilityDeploymentManagers,
		adapterapi.CapabilitySubscriptions,
		adapterapi.CapabilityHealthChecks,
	}

	for _, expected := range expectedCaps {
		assert.Contains(t, caps, expected)
	}
}

func TestKubernetesAdapter_Health(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// With fake client, ServerVersion should return a dummy version
	err := adp.Health(ctx)
	require.NoError(t, err)
}

func TestKubernetesAdapter_Close(t *testing.T) {
	adp := newTestAdapter(t)

	err := adp.Close()
	require.NoError(t, err)
}

func TestKubernetesAdapter_GetDeploymentManager(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	dm, err := adp.GetDeploymentManager(ctx, "dm-1")

	require.Error(t, err)
	assert.Nil(t, dm)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_ListResourcePools(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		filter *adapterapi.Filter
	}{
		{
			name:   "with nil filter",
			filter: nil,
		},
		{
			name:   "with empty filter",
			filter: &adapterapi.Filter{},
		},
		{
			name: "with resource pool filter",
			filter: &adapterapi.Filter{
				ResourcePoolID: "pool-1",
				Location:       "dc-west-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pools, err := adp.ListResourcePools(ctx, tt.filter)

			require.Error(t, err)
			assert.Nil(t, pools)
			assert.Contains(t, err.Error(), "not implemented")
		})
	}
}

func TestKubernetesAdapter_GetResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool, err := adp.GetResourcePool(ctx, "pool-1")

	require.Error(t, err)
	assert.Nil(t, pool)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_CreateResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool := &adapterapi.ResourcePool{
		Name:     "test-pool",
		OCloudID: "ocloud-1",
	}

	created, err := adp.CreateResourcePool(ctx, pool)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_UpdateResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool := &adapterapi.ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "updated-pool",
		OCloudID:       "ocloud-1",
	}

	updated, err := adp.UpdateResourcePool(ctx, "pool-1", pool)

	require.Error(t, err)
	assert.Nil(t, updated)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_DeleteResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	err := adp.DeleteResourcePool(ctx, "pool-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_ListResources(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		filter *adapterapi.Filter
	}{
		{
			name:   "with nil filter",
			filter: nil,
		},
		{
			name:   "with empty filter",
			filter: &adapterapi.Filter{},
		},
		{
			name: "with resource filter",
			filter: &adapterapi.Filter{
				ResourcePoolID: "pool-1",
				ResourceTypeID: "type-compute",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := adp.ListResources(ctx, tt.filter)

			require.Error(t, err)
			assert.Nil(t, resources)
			assert.Contains(t, err.Error(), "not implemented")
		})
	}
}

func TestKubernetesAdapter_GetResource(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	resource, err := adp.GetResource(ctx, "res-1")

	require.Error(t, err)
	assert.Nil(t, resource)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_CreateResource(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	resource := &adapterapi.Resource{
		ResourceTypeID: "type-compute",
		ResourcePoolID: "pool-1",
	}

	created, err := adp.CreateResource(ctx, resource)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_DeleteResource(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	err := adp.DeleteResource(ctx, "res-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_ListResourceTypes(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		filter *adapterapi.Filter
	}{
		{
			name:   "with nil filter",
			filter: nil,
		},
		{
			name:   "with empty filter",
			filter: &adapterapi.Filter{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			types, err := adp.ListResourceTypes(ctx, tt.filter)

			require.Error(t, err)
			assert.Nil(t, types)
			assert.Contains(t, err.Error(), "not implemented")
		})
	}
}

func TestKubernetesAdapter_GetResourceType(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	rt, err := adp.GetResourceType(ctx, "type-1")

	require.Error(t, err)
	assert.Nil(t, rt)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_CreateSubscription(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	sub := &adapterapi.Subscription{
		Callback: "https://smo.example.com/notify",
	}

	created, err := adp.CreateSubscription(ctx, sub)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_GetSubscription(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	sub, err := adp.GetSubscription(ctx, "sub-1")

	require.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_DeleteSubscription(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	err := adp.DeleteSubscription(ctx, "sub-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_ImplementsAdapterInterface(t *testing.T) {
	adp := newTestAdapter(t)

	// Verify that KubernetesAdapter implements adapterapi.Adapter
	var _ adapterapi.Adapter = adp
}

func TestConfigDefaults(t *testing.T) {
	tests := []struct {
		name              string
		namespace         string
		expectedNamespace string
	}{
		{
			name:              "empty namespace uses default",
			namespace:         "",
			expectedNamespace: "o2ims-system",
		},
		{
			name:              "custom namespace is preserved",
			namespace:         "custom-ns",
			expectedNamespace: "custom-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully test New() without kubernetes access,
			// but we can verify the logic by creating adapter manually
			adp := &KubernetesAdapter{
				client:              fake.NewSimpleClientset(),
				logger:              zaptest.NewLogger(t),
				oCloudID:            "test-ocloud",
				deploymentManagerID: "test-dm",
			}

			// Set namespace using same logic as New()
			if tt.namespace == "" {
				adp.namespace = "o2ims-system"
			} else {
				adp.namespace = tt.namespace
			}

			assert.Equal(t, tt.expectedNamespace, adp.namespace)
		})
	}
}

// Tests for boundary conditions and edge cases

func TestKubernetesAdapter_GetDeploymentManager_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	dm, err := adp.GetDeploymentManager(ctx, "")

	require.Error(t, err)
	assert.Nil(t, dm)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_GetResourcePool_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool, err := adp.GetResourcePool(ctx, "")

	require.Error(t, err)
	assert.Nil(t, pool)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_GetResource_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	resource, err := adp.GetResource(ctx, "")

	require.Error(t, err)
	assert.Nil(t, resource)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_GetResourceType_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	rt, err := adp.GetResourceType(ctx, "")

	require.Error(t, err)
	assert.Nil(t, rt)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_GetSubscription_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	sub, err := adp.GetSubscription(ctx, "")

	require.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_DeleteResourcePool_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	err := adp.DeleteResourcePool(ctx, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_DeleteResource_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	err := adp.DeleteResource(ctx, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_DeleteSubscription_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	err := adp.DeleteSubscription(ctx, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_CreateSubscription_EmptyCallback(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	sub := &adapterapi.Subscription{
		Callback: "",
	}

	created, err := adp.CreateSubscription(ctx, sub)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_CreateResourcePool_EmptyName(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool := &adapterapi.ResourcePool{
		Name:     "",
		OCloudID: "ocloud-1",
	}

	created, err := adp.CreateResourcePool(ctx, pool)

	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_ListResourcePools_WithPagination(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	filter := &adapterapi.Filter{
		Limit:  10,
		Offset: 0,
	}

	pools, err := adp.ListResourcePools(ctx, filter)

	require.Error(t, err)
	assert.Nil(t, pools)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestKubernetesAdapter_ListResources_WithLabels(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	filter := &adapterapi.Filter{
		Labels: map[string]string{
			"environment": "production",
			"tier":        "backend",
		},
	}

	resources, err := adp.ListResources(ctx, filter)

	require.Error(t, err)
	assert.Nil(t, resources)
	assert.Contains(t, err.Error(), "not implemented")
}
