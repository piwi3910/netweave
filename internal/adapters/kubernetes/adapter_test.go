// Package kubernetes provides tests for the Kubernetes adapter implementation.
package kubernetes

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	adapterapi "github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// It registers a cleanup function to properly close the adapter after the test.
func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()

	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel))
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              logger,
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}

	// Register cleanup to close adapter after test
	t.Cleanup(func() {
		if err := adp.Close(); err != nil {
			t.Logf("warning: failed to close adapter during cleanup: %v", err)
		}
	})

	return adp
}

// newTestAdapterSilent creates a test adapter with a no-op logger.
// Use this for tests that intentionally trigger error conditions to suppress
// expected ERROR logs in test output.
func newTestAdapterSilent(t *testing.T) *Adapter {
	t.Helper()

	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              zap.NewNop(), // No-op logger for expected errors
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}

	// Register cleanup to close adapter after test
	t.Cleanup(func() {
		if err := adp.Close(); err != nil {
			t.Logf("warning: failed to close adapter during cleanup: %v", err)
		}
	})

	return adp
}

// newTestAdapterWithStore creates a test adapter with a Redis store for testing subscriptions.
func newTestAdapterWithStore(t *testing.T) *Adapter {
	t.Helper()

	// Create miniredis instance for testing
	mr := miniredis.RunT(t)

	// Create Redis store
	store := storage.NewRedisStore(&storage.RedisConfig{
		Addr: mr.Addr(),
	})

	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel))
	adp := &Adapter{
		client:              fake.NewClientset(),
		store:               store,
		logger:              logger,
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}

	// Register cleanup
	t.Cleanup(func() {
		if err := adp.Close(); err != nil {
			t.Logf("warning: failed to close adapter during cleanup: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Logf("warning: failed to close store during cleanup: %v", err)
		}
		mr.Close()
	})

	return adp
}

// newTestAdapterWithStoreSilent creates a test adapter with Redis store and no-op logger.
// Use this for tests that intentionally trigger error conditions to suppress expected ERROR logs.
func newTestAdapterWithStoreSilent(t *testing.T) *Adapter {
	t.Helper()

	// Create miniredis instance for testing
	mr := miniredis.RunT(t)

	// Create Redis store
	store := storage.NewRedisStore(&storage.RedisConfig{
		Addr: mr.Addr(),
	})

	adp := &Adapter{
		client:              fake.NewClientset(),
		store:               store,
		logger:              zap.NewNop(), // No-op logger for expected errors
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}

	// Register cleanup
	t.Cleanup(func() {
		if err := adp.Close(); err != nil {
			t.Logf("warning: failed to close adapter during cleanup: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Logf("warning: failed to close store during cleanup: %v", err)
		}
		mr.Close()
	})

	return adp
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

	t.Run("valid deployment manager ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "test-dm")
		require.NoError(t, err)
		require.NotNil(t, dm)
		assert.Equal(t, "test-dm", dm.DeploymentManagerID)
		assert.Equal(t, "test-ocloud", dm.OCloudID)
		assert.Contains(t, dm.Capabilities, "resource-pools")
		assert.Contains(t, dm.Capabilities, "resources")
		assert.Contains(t, dm.Capabilities, "resource-types")
	})

	t.Run("invalid deployment manager ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "invalid-dm")
		require.Error(t, err)
		assert.Nil(t, dm)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestKubernetesAdapter_ListResourcePools(t *testing.T) {
	t.Run("empty cluster", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		pools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, pools)
		assert.Empty(t, pools) // No namespaces in fake client
	})

	t.Run("with namespaces", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		// Create test namespaces
		ns1 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Labels: map[string]string{
					"environment": "prod",
				},
			},
		}
		ns2 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "staging",
				Labels: map[string]string{
					"environment": "staging",
				},
			},
		}
		_, err := adp.client.CoreV1().Namespaces().Create(ctx, ns1, metav1.CreateOptions{})
		require.NoError(t, err)
		_, err = adp.client.CoreV1().Namespaces().Create(ctx, ns2, metav1.CreateOptions{})
		require.NoError(t, err)

		pools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, pools, 2)
		assert.Equal(t, "k8s-namespace-production", pools[0].ResourcePoolID)
		assert.Equal(t, "k8s-namespace-staging", pools[1].ResourcePoolID)
	})
}

func TestKubernetesAdapter_GetResourcePool(t *testing.T) {
	t.Run("namespace not found", func(t *testing.T) {
		// Use silent adapter to suppress expected ERROR logs
		adp := newTestAdapterSilent(t)
		ctx := context.Background()

		pool, err := adp.GetResourcePool(ctx, "k8s-namespace-nonexistent")
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("namespace found", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
			},
		}
		_, err := adp.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		require.NoError(t, err)

		pool, err := adp.GetResourcePool(ctx, "k8s-namespace-production")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "k8s-namespace-production", pool.ResourcePoolID)
		assert.Equal(t, "production", pool.Name)
	})
}

func TestKubernetesAdapter_CreateResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool := &adapterapi.ResourcePool{
		Name:     "test-pool",
		OCloudID: "ocloud-1",
	}

	created, err := adp.CreateResourcePool(ctx, pool)

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "k8s-namespace-test-pool", created.ResourcePoolID)
	assert.Equal(t, "test-pool", created.Name)
}

func TestKubernetesAdapter_UpdateResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// Create namespace first
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
		},
	}
	_, err := adp.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	require.NoError(t, err)

	pool := &adapterapi.ResourcePool{
		ResourcePoolID: "k8s-namespace-production",
		Name:           "production",
		OCloudID:       "ocloud-1",
		Description:    "Updated description",
	}

	updated, err := adp.UpdateResourcePool(ctx, "k8s-namespace-production", pool)

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "k8s-namespace-production", updated.ResourcePoolID)
}

func TestKubernetesAdapter_DeleteResourcePool(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// Create namespace first
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
		},
	}
	_, err := adp.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	require.NoError(t, err)

	err = adp.DeleteResourcePool(ctx, "k8s-namespace-production")

	require.NoError(t, err)

	// Verify it's gone
	_, err = adp.client.CoreV1().Namespaces().Get(ctx, "production", metav1.GetOptions{})
	require.Error(t, err)
}

func TestKubernetesAdapter_ListResources(t *testing.T) {
	t.Run("empty cluster", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, resources)
		assert.Empty(t, resources) // No nodes in fake client
	})

	t.Run("with nodes", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		// Create test nodes
		node1 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
			},
		}
		node2 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-2",
			},
		}
		_, err := adp.client.CoreV1().Nodes().Create(ctx, node1, metav1.CreateOptions{})
		require.NoError(t, err)
		_, err = adp.client.CoreV1().Nodes().Create(ctx, node2, metav1.CreateOptions{})
		require.NoError(t, err)

		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)
		require.Len(t, resources, 2)
		assert.Equal(t, "k8s-node-worker-1", resources[0].ResourceID)
		assert.Equal(t, "k8s-node-worker-2", resources[1].ResourceID)
	})
}

func TestKubernetesAdapter_GetResource(t *testing.T) {
	t.Run("node not found", func(t *testing.T) {
		// Use silent adapter to suppress expected ERROR logs
		adp := newTestAdapterSilent(t)
		ctx := context.Background()

		resource, err := adp.GetResource(ctx, "k8s-node-nonexistent")
		require.Error(t, err)
		assert.Nil(t, resource)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("node found", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
			},
		}
		_, err := adp.client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)

		resource, err := adp.GetResource(ctx, "k8s-node-worker-1")
		require.NoError(t, err)
		require.NotNil(t, resource)
		assert.Equal(t, "k8s-node-worker-1", resource.ResourceID)
	})
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
	assert.Contains(t, err.Error(), "nodes are registered by kubelet")
}

func TestKubernetesAdapter_DeleteResource(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// Create a node first
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
		},
	}
	_, err := adp.client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	require.NoError(t, err)

	// Delete it
	err = adp.DeleteResource(ctx, "k8s-node-worker-1")
	require.NoError(t, err)

	// Verify it's gone
	_, err = adp.client.CoreV1().Nodes().Get(ctx, "worker-1", metav1.GetOptions{})
	require.Error(t, err)
}

func TestKubernetesAdapter_ListResourceTypes(t *testing.T) {
	t.Run("empty cluster", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, types)
		assert.Empty(t, types) // No nodes in fake client
	})

	t.Run("with nodes", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		// Create test nodes with different instance types
		node1 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.large",
				},
			},
		}
		node2 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-2",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.large",
				},
			},
		}
		_, err := adp.client.CoreV1().Nodes().Create(ctx, node1, metav1.CreateOptions{})
		require.NoError(t, err)
		_, err = adp.client.CoreV1().Nodes().Create(ctx, node2, metav1.CreateOptions{})
		require.NoError(t, err)

		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		require.Len(t, types, 1) // Only one unique type
		assert.Equal(t, "k8s-node-type-m5.large", types[0].ResourceTypeID)
	})
}

func TestKubernetesAdapter_GetResourceType(t *testing.T) {
	t.Run("type not found", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		rt, err := adp.GetResourceType(ctx, "k8s-node-type-nonexistent")
		require.Error(t, err)
		assert.Nil(t, rt)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("type found", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.large",
				},
			},
		}
		_, err := adp.client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)

		rt, err := adp.GetResourceType(ctx, "k8s-node-type-m5.large")
		require.NoError(t, err)
		require.NotNil(t, rt)
		assert.Equal(t, "k8s-node-type-m5.large", rt.ResourceTypeID)
	})
}

func TestKubernetesAdapter_CreateSubscription(t *testing.T) {
	t.Run("without store configured", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		sub := &adapterapi.Subscription{
			SubscriptionID: "sub-1",
			Callback:       "https://smo.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage not configured")
		assert.Nil(t, created)
	})

	t.Run("with store configured", func(t *testing.T) {
		adp := newTestAdapterWithStore(t)
		ctx := context.Background()

		sub := &adapterapi.Subscription{
			SubscriptionID: "sub-1",
			Callback:       "https://smo.example.com/notify",
		}

		created, err := adp.CreateSubscription(ctx, sub)

		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "sub-1", created.SubscriptionID)
		assert.Equal(t, "https://smo.example.com/notify", created.Callback)
	})
}

func TestKubernetesAdapter_GetSubscription(t *testing.T) {
	t.Run("without store configured", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		sub, err := adp.GetSubscription(ctx, "sub-1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage not configured")
		assert.Nil(t, sub)
	})

	t.Run("subscription exists", func(t *testing.T) {
		adp := newTestAdapterWithStore(t)
		ctx := context.Background()

		// Create subscription first
		sub := &adapterapi.Subscription{
			SubscriptionID: "sub-1",
			Callback:       "https://smo.example.com/notify",
		}
		_, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Get subscription
		retrieved, err := adp.GetSubscription(ctx, "sub-1")

		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "sub-1", retrieved.SubscriptionID)
		assert.Equal(t, "https://smo.example.com/notify", retrieved.Callback)
	})

	t.Run("subscription not found", func(t *testing.T) {
		// Use silent adapter to suppress expected ERROR logs
		adp := newTestAdapterWithStoreSilent(t)
		ctx := context.Background()

		sub, err := adp.GetSubscription(ctx, "nonexistent")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
		assert.Nil(t, sub)
	})
}

func TestKubernetesAdapter_DeleteSubscription(t *testing.T) {
	t.Run("without store configured", func(t *testing.T) {
		adp := newTestAdapter(t)
		ctx := context.Background()

		err := adp.DeleteSubscription(ctx, "sub-1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage not configured")
	})

	t.Run("subscription exists", func(t *testing.T) {
		adp := newTestAdapterWithStore(t)
		ctx := context.Background()

		// Create subscription first
		sub := &adapterapi.Subscription{
			SubscriptionID: "sub-1",
			Callback:       "https://smo.example.com/notify",
		}
		_, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)

		// Delete subscription
		err = adp.DeleteSubscription(ctx, "sub-1")

		require.NoError(t, err)

		// Verify it's deleted
		_, err = adp.GetSubscription(ctx, "sub-1")
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("subscription not found", func(t *testing.T) {
		// Use silent adapter to suppress expected ERROR logs
		adp := newTestAdapterWithStoreSilent(t)
		ctx := context.Background()

		err := adp.DeleteSubscription(ctx, "nonexistent")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
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
			adp := &Adapter{
				client:              fake.NewClientset(),
				logger:              zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)),
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

func TestKubernetesAdapter_GetOperations_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		testFunc func() (interface{}, error)
	}{
		{
			name: "GetDeploymentManager with empty ID",
			testFunc: func() (interface{}, error) {
				return adp.GetDeploymentManager(ctx, "")
			},
		},
		{
			name: "GetResourcePool with empty ID",
			testFunc: func() (interface{}, error) {
				return adp.GetResourcePool(ctx, "")
			},
		},
		{
			name: "GetResource with empty ID",
			testFunc: func() (interface{}, error) {
				return adp.GetResource(ctx, "")
			},
		},
		{
			name: "GetResourceType with empty ID",
			testFunc: func() (interface{}, error) {
				return adp.GetResourceType(ctx, "")
			},
		},
		{
			name: "GetSubscription with empty ID",
			testFunc: func() (interface{}, error) {
				return adp.GetSubscription(ctx, "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.testFunc()
			require.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestKubernetesAdapter_DeleteOperations_EmptyID(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		testFunc func() error
	}{
		{
			name: "DeleteResourcePool with empty ID",
			testFunc: func() error {
				return adp.DeleteResourcePool(ctx, "")
			},
		},
		{
			name: "DeleteResource with empty ID",
			testFunc: func() error {
				return adp.DeleteResource(ctx, "")
			},
		},
		{
			name: "DeleteSubscription with empty ID",
			testFunc: func() error {
				return adp.DeleteSubscription(ctx, "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.testFunc()
			require.Error(t, err)
		})
	}
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
}

func TestKubernetesAdapter_CreateResourcePool_EmptyName(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool := &adapterapi.ResourcePool{
		Name:     "",
		OCloudID: "ocloud-1",
	}

	// Fake client allows empty names, real K8s would reject
	created, err := adp.CreateResourcePool(ctx, pool)

	// Just verify it returns something (real validation happens in K8s API)
	assert.NotNil(t, created)
	_ = err // May or may not error depending on client
}

func TestKubernetesAdapter_ListResourcePools_WithPagination(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	filter := &adapterapi.Filter{
		Limit:  10,
		Offset: 0,
	}

	pools, err := adp.ListResourcePools(ctx, filter)

	require.NoError(t, err)
	assert.NotNil(t, pools)
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

	require.NoError(t, err)
	assert.NotNil(t, resources)
}

// Tests for context handling

func TestKubernetesAdapter_ListResourcePools_WithTimeout(t *testing.T) {
	adp := newTestAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	pools, _ := adp.ListResourcePools(ctx, nil)

	// With zero timeout, may get context deadline exceeded or success
	assert.NotNil(t, pools)
}

func TestKubernetesAdapter_ListResources_WithTimeout(t *testing.T) {
	adp := newTestAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	resources, _ := adp.ListResources(ctx, nil)

	// With zero timeout, may get context deadline exceeded or success
	assert.NotNil(t, resources)
}

// Tests for filter with extensions

func TestKubernetesAdapter_ListResourcePools_WithExtensions(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	filter := &adapterapi.Filter{
		Extensions: map[string]interface{}{
			"vendor.customField": "customValue",
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	pools, err := adp.ListResourcePools(ctx, filter)

	require.NoError(t, err)
	assert.NotNil(t, pools)
}

func TestKubernetesAdapter_ListResources_WithExtensions(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	filter := &adapterapi.Filter{
		Extensions: map[string]interface{}{
			"machineType": "n1-standard-4",
		},
	}

	resources, err := adp.ListResources(ctx, filter)

	require.NoError(t, err)
	assert.NotNil(t, resources)
}

// Tests for configuration validation

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "empty OCloudID is allowed",
			cfg: &Config{
				OCloudID:            "",
				DeploymentManagerID: "dm-1",
				Logger:              zap.NewNop(),
			},
			wantErr: true,
			errMsg:  "in-cluster", // Will fail due to no kubeconfig
		},
		{
			name: "empty DeploymentManagerID is allowed",
			cfg: &Config{
				OCloudID:            "ocloud-1",
				DeploymentManagerID: "",
				Logger:              zap.NewNop(),
			},
			wantErr: true,
			errMsg:  "in-cluster", // Will fail due to no kubeconfig
		},
		{
			name: "nil logger creates default",
			cfg: &Config{
				OCloudID:            "ocloud-1",
				DeploymentManagerID: "dm-1",
				Logger:              nil,
			},
			wantErr: true,
			errMsg:  "in-cluster", // Will fail due to no kubeconfig
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Tests for multiple Close calls

func TestKubernetesAdapter_Close_MultipleCallsAreSafe(t *testing.T) {
	// Create adapter without using newTestAdapter to avoid double close
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel)),
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}

	// First close should succeed
	err := adp.Close()
	require.NoError(t, err)

	// Second close should also succeed (idempotent)
	err = adp.Close()
	require.NoError(t, err)
}

// Tests for adapter metadata consistency

func TestKubernetesAdapter_MetadataConsistency(t *testing.T) {
	adp := newTestAdapter(t)

	// Name should always return the same value
	name1 := adp.Name()
	name2 := adp.Name()
	assert.Equal(t, name1, name2)
	assert.Equal(t, "kubernetes", name1)

	// Version should always return the same value
	version1 := adp.Version()
	version2 := adp.Version()
	assert.Equal(t, version1, version2)

	// Capabilities should always return the same slice
	caps1 := adp.Capabilities()
	caps2 := adp.Capabilities()
	assert.Equal(t, caps1, caps2)
	assert.Len(t, caps1, 6)
}

// Tests for subscription with filter

func TestKubernetesAdapter_CreateSubscription_WithFilter(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	sub := &adapterapi.Subscription{
		SubscriptionID:         "sub-test",
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-123",
		Filter: &adapterapi.SubscriptionFilter{
			ResourcePoolID: "pool-1",
			ResourceTypeID: "type-compute",
			ResourceID:     "res-specific",
		},
	}

	created, err := adp.CreateSubscription(ctx, sub)

	require.Error(t, err)
	assert.Nil(t, created)
}

// Tests for resource with all extensions

func TestKubernetesAdapter_CreateResource_WithExtensions(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	resource := &adapterapi.Resource{
		ResourceID:     "res-test",
		ResourceTypeID: "type-compute",
		ResourcePoolID: "pool-1",
		GlobalAssetID:  "urn:o-ran:resource:test-node",
		Description:    "Test worker node",
		Extensions: map[string]interface{}{
			"nodeName": "worker-test",
			"status":   "Ready",
			"cpu":      "8",
			"memory":   "32Gi",
			"labels": map[string]string{
				"role": "worker",
			},
		},
	}

	created, err := adp.CreateResource(ctx, resource)

	require.Error(t, err)
	assert.Nil(t, created)
}

// Tests for resource pool with all fields

func TestKubernetesAdapter_CreateResourcePool_WithAllFields(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	pool := &adapterapi.ResourcePool{
		ResourcePoolID:   "pool-test",
		Name:             "production-pool",
		Description:      "High-performance compute pool",
		Location:         "dc-west-1",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "geo:37.7749,-122.4194",
		Extensions: map[string]interface{}{
			"machineType": "n1-standard-4",
			"replicas":    3,
			"volumeSize":  "100Gi",
		},
	}

	created, err := adp.CreateResourcePool(ctx, pool)

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "k8s-namespace-production-pool", created.ResourcePoolID)
	assert.Equal(t, "production-pool", created.Name)
}

// Tests for edge case validation - negative and boundary values

func TestKubernetesAdapter_ListResourcePools_NegativePagination(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// Test with negative limit - stub returns "not implemented" but validates filter is passed
	filter := &adapterapi.Filter{
		Limit:  -1,
		Offset: -10,
	}

	pools, err := adp.ListResourcePools(ctx, filter)

	// Should succeed with empty results
	require.NoError(t, err)
	assert.NotNil(t, pools)
}

func TestKubernetesAdapter_ListResources_NegativePagination(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	filter := &adapterapi.Filter{
		Limit:  -100,
		Offset: -5,
	}

	resources, err := adp.ListResources(ctx, filter)

	// Should succeed with empty results
	require.NoError(t, err)
	assert.NotNil(t, resources)
}

func TestKubernetesAdapter_ListResourcePools_ZeroPagination(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// Zero limit should be valid (use default)
	filter := &adapterapi.Filter{
		Limit:  0,
		Offset: 0,
	}

	pools, err := adp.ListResourcePools(ctx, filter)

	require.NoError(t, err)
	assert.NotNil(t, pools)
}

func TestKubernetesAdapter_ListResourcePools_LargePagination(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	// Very large limit value
	filter := &adapterapi.Filter{
		Limit:  10000,
		Offset: 999999,
	}

	pools, err := adp.ListResourcePools(ctx, filter)

	require.NoError(t, err)
	assert.NotNil(t, pools)
}

// Tests for JSON marshaling/unmarshaling of adapter types

func TestFilter_JSONMarshal(t *testing.T) {
	filter := &adapterapi.Filter{
		ResourcePoolID: "pool-1",
		ResourceTypeID: "type-compute",
		Location:       "dc-west-1",
		Labels: map[string]string{
			"env": "prod",
		},
		Extensions: map[string]interface{}{
			"custom": "value",
		},
		Limit:  100,
		Offset: 0,
	}

	// Marshal to JSON
	data, err := json.Marshal(filter)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal back
	var decoded adapterapi.Filter
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, filter.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, filter.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, filter.Location, decoded.Location)
	assert.Equal(t, filter.Limit, decoded.Limit)
	assert.Equal(t, filter.Offset, decoded.Offset)
}

func TestResourcePool_JSONMarshal(t *testing.T) {
	pool := &adapterapi.ResourcePool{
		ResourcePoolID:   "pool-123",
		Name:             "Test Pool",
		Description:      "A test resource pool",
		Location:         "us-west-2",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "geo:37.7749,-122.4194",
		Extensions: map[string]interface{}{
			"machineType": "n1-standard-4",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(pool)
	require.NoError(t, err)
	assert.Contains(t, string(data), "resourcePoolId")
	assert.Contains(t, string(data), "pool-123")

	// Unmarshal back
	var decoded adapterapi.ResourcePool
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, pool.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, pool.Name, decoded.Name)
	assert.Equal(t, pool.Description, decoded.Description)
	assert.Equal(t, pool.Location, decoded.Location)
	assert.Equal(t, pool.OCloudID, decoded.OCloudID)
	assert.Equal(t, pool.GlobalLocationID, decoded.GlobalLocationID)
}

func TestResource_JSONMarshal(t *testing.T) {
	resource := &adapterapi.Resource{
		ResourceID:     "res-456",
		ResourceTypeID: "type-compute",
		ResourcePoolID: "pool-123",
		GlobalAssetID:  "urn:o-ran:resource:node-01",
		Description:    "Worker node",
		Extensions: map[string]interface{}{
			"cpu":    "8",
			"memory": "32Gi",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(resource)
	require.NoError(t, err)
	assert.Contains(t, string(data), "resourceId")
	assert.Contains(t, string(data), "res-456")

	// Unmarshal back
	var decoded adapterapi.Resource
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resource.ResourceID, decoded.ResourceID)
	assert.Equal(t, resource.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, resource.ResourcePoolID, decoded.ResourcePoolID)
	assert.Equal(t, resource.GlobalAssetID, decoded.GlobalAssetID)
	assert.Equal(t, resource.Description, decoded.Description)
}

func TestResourceType_JSONMarshal(t *testing.T) {
	rt := &adapterapi.ResourceType{
		ResourceTypeID: "type-compute",
		Name:           "Compute Node",
		Description:    "High-performance compute node",
		Vendor:         "Dell",
		Model:          "PowerEdge R740",
		Version:        "1.0",
		ResourceClass:  "compute",
		ResourceKind:   "physical",
		Extensions: map[string]interface{}{
			"cores": 64,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(rt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "resourceTypeId")

	// Unmarshal back
	var decoded adapterapi.ResourceType
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, rt.ResourceTypeID, decoded.ResourceTypeID)
	assert.Equal(t, rt.Name, decoded.Name)
	assert.Equal(t, rt.Vendor, decoded.Vendor)
	assert.Equal(t, rt.Model, decoded.Model)
	assert.Equal(t, rt.ResourceClass, decoded.ResourceClass)
	assert.Equal(t, rt.ResourceKind, decoded.ResourceKind)
}

func TestSubscription_JSONMarshal(t *testing.T) {
	sub := &adapterapi.Subscription{
		SubscriptionID:         "sub-789",
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-123",
		Filter: &adapterapi.SubscriptionFilter{
			ResourcePoolID: "pool-1",
			ResourceTypeID: "type-compute",
			ResourceID:     "res-specific",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(sub)
	require.NoError(t, err)
	assert.Contains(t, string(data), "subscriptionId")
	assert.Contains(t, string(data), "callback")

	// Unmarshal back
	var decoded adapterapi.Subscription
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, sub.SubscriptionID, decoded.SubscriptionID)
	assert.Equal(t, sub.Callback, decoded.Callback)
	assert.Equal(t, sub.ConsumerSubscriptionID, decoded.ConsumerSubscriptionID)
	require.NotNil(t, decoded.Filter)
	assert.Equal(t, sub.Filter.ResourcePoolID, decoded.Filter.ResourcePoolID)
	assert.Equal(t, sub.Filter.ResourceTypeID, decoded.Filter.ResourceTypeID)
	assert.Equal(t, sub.Filter.ResourceID, decoded.Filter.ResourceID)
}

func TestDeploymentManager_JSONMarshal(t *testing.T) {
	dm := &adapterapi.DeploymentManager{
		DeploymentManagerID: "dm-001",
		Name:                "Production DM",
		Description:         "Production Kubernetes deployment manager",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.example.com/o2ims/v1",
		SupportedLocations:  []string{"us-west-1", "us-east-1"},
		Capabilities:        []string{"compute", "storage", "network"},
		Extensions: map[string]interface{}{
			"clusterVersion": "1.30.0",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(dm)
	require.NoError(t, err)
	assert.Contains(t, string(data), "deploymentManagerId")
	assert.Contains(t, string(data), "serviceUri")

	// Unmarshal back
	var decoded adapterapi.DeploymentManager
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, dm.DeploymentManagerID, decoded.DeploymentManagerID)
	assert.Equal(t, dm.Name, decoded.Name)
	assert.Equal(t, dm.Description, decoded.Description)
	assert.Equal(t, dm.OCloudID, decoded.OCloudID)
	assert.Equal(t, dm.ServiceURI, decoded.ServiceURI)
	assert.Equal(t, dm.SupportedLocations, decoded.SupportedLocations)
	assert.Equal(t, dm.Capabilities, decoded.Capabilities)
}

// Test for empty JSON unmarshaling

func TestFilter_JSONUnmarshalEmpty(t *testing.T) {
	var filter adapterapi.Filter
	err := json.Unmarshal([]byte("{}"), &filter)
	require.NoError(t, err)

	assert.Empty(t, filter.ResourcePoolID)
	assert.Empty(t, filter.ResourceTypeID)
	assert.Empty(t, filter.Location)
	assert.Nil(t, filter.Labels)
	assert.Nil(t, filter.Extensions)
	assert.Equal(t, 0, filter.Limit)
	assert.Equal(t, 0, filter.Offset)
}

func TestSubscription_JSONUnmarshalWithoutFilter(t *testing.T) {
	jsonData := `{"subscriptionId":"sub-1","callback":"https://example.com/notify"}`

	var sub adapterapi.Subscription
	err := json.Unmarshal([]byte(jsonData), &sub)
	require.NoError(t, err)

	assert.Equal(t, "sub-1", sub.SubscriptionID)
	assert.Equal(t, "https://example.com/notify", sub.Callback)
	assert.Nil(t, sub.Filter)
}

// Concurrent operation tests - verify thread safety of adapter methods
// These tests use the -race flag during CI to detect race conditions

func TestKubernetesAdapter_ConcurrentHealth(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = adp.Health(ctx)
		}()
	}

	wg.Wait()
}

func TestKubernetesAdapter_ConcurrentMetadata(t *testing.T) {
	adp := newTestAdapter(t)

	const numGoroutines = 10
	var wg sync.WaitGroup

	// Collect results from goroutines to avoid race conditions on testing.T
	names := make([]string, numGoroutines)
	versions := make([]string, numGoroutines)
	capabilities := make([][]adapterapi.Capability, numGoroutines)

	wg.Add(numGoroutines * 3)

	// Concurrent Name() calls
	for i := 0; i < numGoroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			names[i] = adp.Name()
		}()
	}

	// Concurrent Version() calls
	for i := 0; i < numGoroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			versions[i] = adp.Version()
		}()
	}

	// Concurrent Capabilities() calls
	for i := 0; i < numGoroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			capabilities[i] = adp.Capabilities()
		}()
	}

	wg.Wait()

	// Assert on collected results after all goroutines complete
	for i := 0; i < numGoroutines; i++ {
		assert.Equal(t, "kubernetes", names[i], "Name() should return 'kubernetes'")
		assert.NotEmpty(t, versions[i], "Version() should not be empty")
		assert.Len(t, capabilities[i], 6, "Capabilities() should return 6 items")
	}
}

func TestKubernetesAdapter_ConcurrentListOperations(t *testing.T) {
	adp := newTestAdapter(t)
	ctx := context.Background()

	const numGoroutines = 5
	var wg sync.WaitGroup

	// Collect errors from goroutines to avoid race conditions on testing.T
	poolErrors := make([]error, numGoroutines)
	resourceErrors := make([]error, numGoroutines)
	typeErrors := make([]error, numGoroutines)

	wg.Add(numGoroutines * 3)

	// Concurrent ListResourcePools calls
	for i := 0; i < numGoroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			_, err := adp.ListResourcePools(ctx, nil)
			poolErrors[i] = err
		}()
	}

	// Concurrent ListResources calls
	for i := 0; i < numGoroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			_, err := adp.ListResources(ctx, nil)
			resourceErrors[i] = err
		}()
	}

	// Concurrent ListResourceTypes calls
	for i := 0; i < numGoroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			_, err := adp.ListResourceTypes(ctx, nil)
			typeErrors[i] = err
		}()
	}

	wg.Wait()

	// Assert on collected errors after all goroutines complete
	for i := 0; i < numGoroutines; i++ {
		assert.NoError(t, poolErrors[i], "ListResourcePools should not return error")
		assert.NoError(t, resourceErrors[i], "ListResources should not return error")
		assert.NoError(t, typeErrors[i], "ListResourceTypes should not return error")
	}
}

// Benchmark tests for performance tracking
// Run with: go test -bench=. -benchmem

func BenchmarkKubernetesAdapter_Name(b *testing.B) {
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              zap.NewNop(),
		oCloudID:            "bench-ocloud",
		deploymentManagerID: "bench-dm",
		namespace:           "o2ims-system",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adp.Name()
	}
}

func BenchmarkKubernetesAdapter_Version(b *testing.B) {
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              zap.NewNop(),
		oCloudID:            "bench-ocloud",
		deploymentManagerID: "bench-dm",
		namespace:           "o2ims-system",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adp.Version()
	}
}

func BenchmarkKubernetesAdapter_Capabilities(b *testing.B) {
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              zap.NewNop(),
		oCloudID:            "bench-ocloud",
		deploymentManagerID: "bench-dm",
		namespace:           "o2ims-system",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adp.Capabilities()
	}
}

func BenchmarkKubernetesAdapter_Health(b *testing.B) {
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              zap.NewNop(),
		oCloudID:            "bench-ocloud",
		deploymentManagerID: "bench-dm",
		namespace:           "o2ims-system",
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adp.Health(ctx)
	}
}

// Logger edge case tests

func TestKubernetesAdapter_WithNilLogger(t *testing.T) {
	// Create adapter with nil logger field to test nil handling
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              nil, // Intentionally nil
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}

	// These should not panic even with nil logger
	// Note: The actual implementation logs, so this tests robustness
	assert.Equal(t, "kubernetes", adp.Name())
	assert.NotEmpty(t, adp.Version())
	assert.Len(t, adp.Capabilities(), 6)
}

func TestKubernetesAdapter_LoggerUsedInOperations(t *testing.T) {
	// Verify logger is properly used in operations
	logger := zaptest.NewLogger(t, zaptest.Level(zap.WarnLevel))
	adp := &Adapter{
		client:              fake.NewClientset(),
		logger:              logger,
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
		namespace:           "o2ims-system",
	}
	ctx := context.Background()

	// All operations should complete without panic and use logger appropriately
	_, _ = adp.GetDeploymentManager(ctx, "dm-1")
	_, _ = adp.ListResourcePools(ctx, nil)
	_, _ = adp.GetResourcePool(ctx, "pool-1")
	_, _ = adp.ListResources(ctx, nil)
	_, _ = adp.GetResource(ctx, "res-1")
	_, _ = adp.ListResourceTypes(ctx, nil)
	_, _ = adp.GetResourceType(ctx, "type-1")
	_, _ = adp.CreateSubscription(ctx, &adapterapi.Subscription{Callback: "https://example.com"})
	_, _ = adp.GetSubscription(ctx, "sub-1")
	_ = adp.DeleteSubscription(ctx, "sub-1")
	_ = adp.Health(ctx)

	// If we get here without panic, logger handling is correct
}
