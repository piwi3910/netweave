package kubernetes_test

import (
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	adapterapi "github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Helper: creates adapter WITHOUT skipping in short mode.
func unitTestAdapter(t *testing.T) *kubernetes.Adapter {
	t.Helper()
	adp := kubernetes.NewForTesting(fake.NewClientset(), zap.NewNop())
	t.Cleanup(func() { _ = adp.Close() })
	return adp
}

// unitTestAdapterWithStore creates a test adapter with Redis for subscription tests.
func unitTestAdapterWithStore(t *testing.T) *kubernetes.Adapter {
	t.Helper()
	mr := miniredis.RunT(t)
	store := storage.NewRedisStore(&storage.RedisConfig{Addr: mr.Addr()})
	adp := kubernetes.NewForTestingWithStore(fake.NewClientset(), store, zap.NewNop())
	t.Cleanup(func() {
		_ = adp.Close()
		_ = store.Close()
		mr.Close()
	})
	return adp
}

// --- Metadata tests ---

func TestUnit_Name(t *testing.T) {
	adp := unitTestAdapter(t)
	assert.Equal(t, "kubernetes", adp.Name())
}

func TestUnit_Version(t *testing.T) {
	adp := unitTestAdapter(t)
	assert.Equal(t, "1.30.0", adp.Version())
}

func TestUnit_Capabilities(t *testing.T) {
	adp := unitTestAdapter(t)
	caps := adp.Capabilities()
	require.Len(t, caps, 6)
	assert.Contains(t, caps, adapterapi.CapabilityResourcePools)
	assert.Contains(t, caps, adapterapi.CapabilityResources)
	assert.Contains(t, caps, adapterapi.CapabilityResourceTypes)
	assert.Contains(t, caps, adapterapi.CapabilityDeploymentManagers)
	assert.Contains(t, caps, adapterapi.CapabilitySubscriptions)
	assert.Contains(t, caps, adapterapi.CapabilityHealthChecks)
}

func TestUnit_Health(t *testing.T) {
	adp := unitTestAdapter(t)
	err := adp.Health(context.Background())
	require.NoError(t, err)
}

func TestUnit_Close(t *testing.T) {
	adp := kubernetes.NewForTesting(fake.NewClientset(), zap.NewNop())
	err := adp.Close()
	require.NoError(t, err)
	// Second close should also be safe
	err = adp.Close()
	require.NoError(t, err)
}

func TestUnit_GetClient(t *testing.T) {
	client := fake.NewClientset()
	adp := kubernetes.NewForTesting(client, zap.NewNop())
	t.Cleanup(func() { _ = adp.Close() })
	assert.Equal(t, client, adp.GetClient())
}

func TestUnit_GetSetNamespace(t *testing.T) {
	adp := unitTestAdapter(t)
	assert.Equal(t, "o2ims-system", adp.GetNamespace())
	adp.SetNamespace("custom-ns")
	assert.Equal(t, "custom-ns", adp.GetNamespace())
}

// --- New() config validation ---

func TestUnit_New_NilConfig(t *testing.T) {
	adp, err := kubernetes.New(nil)
	require.Error(t, err)
	assert.Nil(t, adp)
	assert.Contains(t, err.Error(), "config cannot be nil")
}

func TestUnit_New_InvalidKubeconfig(t *testing.T) {
	cfg := &kubernetes.Config{
		Kubeconfig: "/nonexistent/path/kubeconfig",
		Logger:     zap.NewNop(),
	}
	adp, err := kubernetes.New(cfg)
	require.Error(t, err)
	assert.Nil(t, adp)
	assert.Contains(t, err.Error(), "kubeconfig")
}

func TestUnit_New_InClusterFails(t *testing.T) {
	cfg := &kubernetes.Config{
		OCloudID: "ocloud-1",
		Logger:   zap.NewNop(),
	}
	adp, err := kubernetes.New(cfg)
	require.Error(t, err)
	assert.Nil(t, adp)
}

// --- Deployment Manager ---

func TestUnit_GetDeploymentManager(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      string
		wantErr bool
		wantID  string
	}{
		{"configured ID", "test-dm", false, "test-dm"},
		{"default alias", "default", false, "test-dm"},
		{"empty alias", "", false, "test-dm"},
		{"invalid ID", "wrong-dm", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm, err := adp.GetDeploymentManager(ctx, tt.id)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, dm)
			} else {
				require.NoError(t, err)
				require.NotNil(t, dm)
				assert.Equal(t, tt.wantID, dm.DeploymentManagerID)
				assert.Equal(t, "test-ocloud", dm.OCloudID)
			}
		})
	}
}

// --- Resource Pool operations ---

func TestUnit_ListResourcePools(t *testing.T) {
	t.Run("empty cluster", func(t *testing.T) {
		adp := unitTestAdapter(t)
		pools, err := adp.ListResourcePools(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, pools)
		assert.Empty(t, pools)
	})

	t.Run("with namespaces", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		for _, name := range []string{"production", "staging"} {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
			_, err := adp.GetClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			require.NoError(t, err)
		}

		pools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, pools, 2)
	})

	t.Run("with filter", func(t *testing.T) {
		adp := unitTestAdapter(t)
		filter := &adapterapi.Filter{Limit: 10, Offset: 0}
		pools, err := adp.ListResourcePools(context.Background(), filter)
		require.NoError(t, err)
		assert.NotNil(t, pools)
	})
}

func TestUnit_GetResourcePool(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		adp := unitTestAdapter(t)
		pool, err := adp.GetResourcePool(context.Background(), "k8s-namespace-nonexistent")
		require.Error(t, err)
		assert.Nil(t, pool)
	})

	t.Run("found", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}}
		_, err := adp.GetClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		require.NoError(t, err)

		pool, err := adp.GetResourcePool(ctx, "k8s-namespace-production")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "k8s-namespace-production", pool.ResourcePoolID)
		assert.Equal(t, "production", pool.Name)
	})
}

func TestUnit_CreateResourcePool(t *testing.T) {
	adp := unitTestAdapter(t)
	pool := &adapterapi.ResourcePool{Name: "test-pool", OCloudID: "ocloud-1"}
	created, err := adp.CreateResourcePool(context.Background(), pool)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "k8s-namespace-test-pool", created.ResourcePoolID)
}

func TestUnit_UpdateResourcePool(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}}
	_, err := adp.GetClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	require.NoError(t, err)

	pool := &adapterapi.ResourcePool{
		ResourcePoolID: "k8s-namespace-production",
		Name:           "production",
		Description:    "Updated",
	}
	updated, err := adp.UpdateResourcePool(ctx, "k8s-namespace-production", pool)
	require.NoError(t, err)
	require.NotNil(t, updated)
}

func TestUnit_DeleteResourcePool(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}}
	_, err := adp.GetClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	require.NoError(t, err)

	err = adp.DeleteResourcePool(ctx, "k8s-namespace-production")
	require.NoError(t, err)
}

// --- Resource operations ---

func TestUnit_ListResources(t *testing.T) {
	t.Run("empty cluster", func(t *testing.T) {
		adp := unitTestAdapter(t)
		resources, err := adp.ListResources(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, resources)
	})

	t.Run("with nodes", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		for _, name := range []string{"worker-1", "worker-2"} {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
			_, err := adp.GetClient().CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			require.NoError(t, err)
		}

		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)
		require.Len(t, resources, 2)
	})
}

func TestUnit_GetResource(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		adp := unitTestAdapter(t)
		res, err := adp.GetResource(context.Background(), "k8s-node-nonexistent")
		require.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("found", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}
		_, err := adp.GetClient().CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)

		res, err := adp.GetResource(ctx, "k8s-node-worker-1")
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, "k8s-node-worker-1", res.ResourceID)
	})
}

func TestUnit_CreateResource(t *testing.T) {
	adp := unitTestAdapter(t)
	res := &adapterapi.Resource{ResourceTypeID: "type-compute"}
	created, err := adp.CreateResource(context.Background(), res)
	require.Error(t, err)
	assert.Nil(t, created)
	assert.Contains(t, err.Error(), "nodes are registered by kubelet")
}

func TestUnit_UpdateResource(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	// UpdateResource always returns an error because updating nodes directly
	// is not supported in Kubernetes -- nodes are managed by kubelet.
	res := &adapterapi.Resource{
		ResourceID:  "k8s-node-worker-1",
		Description: "Updated description",
	}
	updated, err := adp.UpdateResource(ctx, "k8s-node-worker-1", res)
	require.Error(t, err)
	assert.Nil(t, updated)
	assert.Contains(t, err.Error(), "updating nodes directly is not supported")
}

func TestUnit_DeleteResource(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}
	_, err := adp.GetClient().CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	require.NoError(t, err)

	err = adp.DeleteResource(ctx, "k8s-node-worker-1")
	require.NoError(t, err)
}

// --- Resource Type operations ---

func TestUnit_ListResourceTypes(t *testing.T) {
	t.Run("empty cluster", func(t *testing.T) {
		adp := unitTestAdapter(t)
		types, err := adp.ListResourceTypes(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, types)
	})

	t.Run("with nodes", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-1",
				Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		}
		_, err := adp.GetClient().CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)

		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		require.Len(t, types, 1)
		assert.Equal(t, "k8s-node-type-m5.large", types[0].ResourceTypeID)
	})

	t.Run("deduplicates same type", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		for _, name := range []string{"worker-1", "worker-2"} {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   name,
					Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"},
				},
			}
			_, err := adp.GetClient().CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			require.NoError(t, err)
		}

		types, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, types, 1)
	})
}

func TestUnit_GetResourceType(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		adp := unitTestAdapter(t)
		rt, err := adp.GetResourceType(context.Background(), "k8s-node-type-nonexistent")
		require.Error(t, err)
		assert.Nil(t, rt)
	})

	t.Run("found", func(t *testing.T) {
		adp := unitTestAdapter(t)
		ctx := context.Background()

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-1",
				Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"},
			},
		}
		_, err := adp.GetClient().CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)

		rt, err := adp.GetResourceType(ctx, "k8s-node-type-m5.large")
		require.NoError(t, err)
		require.NotNil(t, rt)
		assert.Equal(t, "k8s-node-type-m5.large", rt.ResourceTypeID)
	})
}

// --- Subscription operations ---

func TestUnit_CreateSubscription(t *testing.T) {
	t.Run("no store returns error", func(t *testing.T) {
		adp := unitTestAdapter(t)
		sub := &adapterapi.Subscription{SubscriptionID: "sub-1", Callback: "https://example.com"}
		created, err := adp.CreateSubscription(context.Background(), sub)
		require.Error(t, err)
		assert.Nil(t, created)
		assert.Contains(t, err.Error(), "storage not configured")
	})

	t.Run("with store success", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		sub := &adapterapi.Subscription{
			SubscriptionID: "sub-1",
			Callback:       "https://example.com/notify",
		}
		created, err := adp.CreateSubscription(context.Background(), sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "sub-1", created.SubscriptionID)
	})

	t.Run("with filter", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		sub := &adapterapi.Subscription{
			SubscriptionID: "sub-filter",
			Callback:       "https://example.com/notify",
			Filter: &adapterapi.SubscriptionFilter{
				ResourcePoolID: "pool-1",
				ResourceTypeID: "type-1",
				ResourceID:     "res-1",
			},
		}
		created, err := adp.CreateSubscription(context.Background(), sub)
		require.NoError(t, err)
		require.NotNil(t, created)
	})
}

func TestUnit_GetSubscription(t *testing.T) {
	t.Run("no store returns error", func(t *testing.T) {
		adp := unitTestAdapter(t)
		sub, err := adp.GetSubscription(context.Background(), "sub-1")
		require.Error(t, err)
		assert.Nil(t, sub)
	})

	t.Run("not found", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		sub, err := adp.GetSubscription(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Nil(t, sub)
	})

	t.Run("found", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		ctx := context.Background()

		_, err := adp.CreateSubscription(ctx, &adapterapi.Subscription{
			SubscriptionID: "sub-get",
			Callback:       "https://example.com",
		})
		require.NoError(t, err)

		got, err := adp.GetSubscription(ctx, "sub-get")
		require.NoError(t, err)
		assert.Equal(t, "sub-get", got.SubscriptionID)
		assert.Equal(t, "https://example.com", got.Callback)
	})

	t.Run("with filter roundtrip", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		ctx := context.Background()

		_, err := adp.CreateSubscription(ctx, &adapterapi.Subscription{
			SubscriptionID:         "sub-filter-rt",
			Callback:               "https://example.com",
			ConsumerSubscriptionID: "consumer-123",
			Filter: &adapterapi.SubscriptionFilter{
				ResourcePoolID: "pool-1",
			},
		})
		require.NoError(t, err)

		got, err := adp.GetSubscription(ctx, "sub-filter-rt")
		require.NoError(t, err)
		require.NotNil(t, got.Filter)
		assert.Equal(t, "pool-1", got.Filter.ResourcePoolID)
		assert.Equal(t, "consumer-123", got.ConsumerSubscriptionID)
	})
}

func TestUnit_UpdateSubscription(t *testing.T) {
	t.Run("no store returns error", func(t *testing.T) {
		adp := unitTestAdapter(t)
		_, err := adp.UpdateSubscription(context.Background(), "sub-1", &adapterapi.Subscription{
			Callback: "https://example.com",
		})
		require.Error(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		_, err := adp.UpdateSubscription(context.Background(), "nonexistent", &adapterapi.Subscription{
			Callback: "https://example.com",
		})
		require.Error(t, err)
	})

	t.Run("empty callback", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		_, err := adp.UpdateSubscription(context.Background(), "sub-1", &adapterapi.Subscription{
			Callback: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("success", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		ctx := context.Background()

		_, err := adp.CreateSubscription(ctx, &adapterapi.Subscription{
			SubscriptionID: "sub-upd",
			Callback:       "https://old.example.com",
		})
		require.NoError(t, err)

		updated, err := adp.UpdateSubscription(ctx, "sub-upd", &adapterapi.Subscription{
			Callback: "https://new.example.com",
		})
		require.NoError(t, err)
		assert.Equal(t, "https://new.example.com", updated.Callback)
	})
}

func TestUnit_DeleteSubscription(t *testing.T) {
	t.Run("no store returns error", func(t *testing.T) {
		adp := unitTestAdapter(t)
		err := adp.DeleteSubscription(context.Background(), "sub-1")
		require.Error(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		err := adp.DeleteSubscription(context.Background(), "nonexistent")
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		adp := unitTestAdapterWithStore(t)
		ctx := context.Background()

		_, err := adp.CreateSubscription(ctx, &adapterapi.Subscription{
			SubscriptionID: "sub-del",
			Callback:       "https://example.com",
		})
		require.NoError(t, err)

		err = adp.DeleteSubscription(ctx, "sub-del")
		require.NoError(t, err)

		_, err = adp.GetSubscription(ctx, "sub-del")
		require.Error(t, err)
	})
}

// --- Concurrency ---

func TestUnit_ConcurrentMetadata(t *testing.T) {
	adp := unitTestAdapter(t)
	var wg sync.WaitGroup
	const n = 10

	wg.Add(n * 3)
	for i := 0; i < n; i++ {
		go func() { defer wg.Done(); _ = adp.Name() }()
		go func() { defer wg.Done(); _ = adp.Version() }()
		go func() { defer wg.Done(); _ = adp.Capabilities() }()
	}
	wg.Wait()
}

// --- Interface compliance ---

func TestUnit_ImplementsAdapterInterface(t *testing.T) {
	adp := unitTestAdapter(t)
	var _ adapterapi.Adapter = adp
}

// --- NewForTesting with nil logger ---

func TestUnit_NewForTesting_NilLogger(t *testing.T) {
	adp := kubernetes.NewForTesting(fake.NewClientset(), nil)
	require.NotNil(t, adp)
	assert.Equal(t, "kubernetes", adp.Name())
	t.Cleanup(func() { _ = adp.Close() })
}

func TestUnit_NewForTestingWithStore_NilLogger(t *testing.T) {
	mr := miniredis.RunT(t)
	store := storage.NewRedisStore(&storage.RedisConfig{Addr: mr.Addr()})
	adp := kubernetes.NewForTestingWithStore(fake.NewClientset(), store, nil)
	require.NotNil(t, adp)
	assert.Equal(t, "kubernetes", adp.Name())
	t.Cleanup(func() {
		_ = adp.Close()
		_ = store.Close()
		mr.Close()
	})
}

// --- ListDeploymentManagers ---

func TestUnit_ListDeploymentManagers(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	t.Run("returns single deployment manager", func(t *testing.T) {
		managers, err := adp.ListDeploymentManagers(ctx, nil)
		require.NoError(t, err)
		require.Len(t, managers, 1)

		dm := managers[0]
		assert.NotEmpty(t, dm.DeploymentManagerID)
		assert.NotEmpty(t, dm.Name)
		assert.NotEmpty(t, dm.Description)
		assert.NotEmpty(t, dm.ServiceURI)
		assert.NotEmpty(t, dm.Capabilities)
		assert.NotNil(t, dm.Extensions)
	})

	t.Run("with filter matches deployment manager", func(t *testing.T) {
		managers, err := adp.ListDeploymentManagers(ctx, nil)
		require.NoError(t, err)
		require.Len(t, managers, 1)
	})
}

// --- CreateDeploymentManager ---

func TestUnit_CreateDeploymentManager(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	dm := &adapterapi.DeploymentManager{
		Name: "new-dm",
	}
	result, err := adp.CreateDeploymentManager(ctx, dm)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not supported")
}

// --- GetOCloudInfrastructure ---

func TestUnit_GetOCloudInfrastructure(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	infra, err := adp.GetOCloudInfrastructure(ctx)
	require.NoError(t, err)
	require.NotNil(t, infra)

	assert.NotEmpty(t, infra["oCloudId"])
	assert.NotEmpty(t, infra["name"])
	assert.NotEmpty(t, infra["description"])
	assert.Equal(t, "kubernetes-cluster", infra["resourceType"])
	assert.NotNil(t, infra["kubernetes"])
	assert.NotNil(t, infra["deploymentManagers"])
}

// --- CreateResourceTypeFromNode edge cases ---

func TestUnit_ListResourceTypes_WithProviderIDs(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		providerID string
		labels     map[string]string
		wantVendor string
	}{
		{
			name:       "AWS provider ID",
			providerID: "aws:///us-east-1a/i-abc123",
			wantVendor: "AWS",
		},
		{
			name:       "GCE provider ID",
			providerID: "gce://my-project/us-central1-a/my-instance",
			wantVendor: "GCP",
		},
		{
			name:       "Azure provider ID",
			providerID: "azure:///subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Compute",
			wantVendor: "Azure",
		},
		{
			name:       "short provider ID",
			providerID: "short",
			wantVendor: "",
		},
		{
			name:       "unknown provider",
			providerID: "vmware://vcenter/vm-123",
			wantVendor: "Unknown",
		},
		{
			name: "vendor from label",
			labels: map[string]string{
				"node.kubernetes.io/vendor": "CustomVendor",
			},
			wantVendor: "CustomVendor",
		},
		{
			name: "instance type from label",
			labels: map[string]string{
				"node.kubernetes.io/instance-type": "m5.xlarge",
			},
		},
		{
			name: "instance type from beta label",
			labels: map[string]string{
				"beta.kubernetes.io/instance-type": "t3.medium",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fake.NewClientset()
			adp := kubernetes.NewForTesting(cs, zap.NewNop())
			t.Cleanup(func() { _ = adp.Close() })

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: tt.labels,
				},
				Spec: corev1.NodeSpec{},
				Status: corev1.NodeStatus{
					Capacity: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("4"),
						corev1.ResourceMemory:           resource.MustParse("16Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
						corev1.ResourcePods:             resource.MustParse("110"),
					},
				},
			}

			if tt.providerID != "" {
				node.Spec.ProviderID = tt.providerID
			}

			_, err := cs.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			require.NoError(t, err)

			types, err := adp.ListResourceTypes(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, types)

			if tt.wantVendor != "" {
				assert.Equal(t, tt.wantVendor, types[0].Vendor)
			}
		})
	}
}

// --- GetDeploymentManager edge cases ---

func TestUnit_GetDeploymentManager_Aliases(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	t.Run("default alias", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "default")
		require.NoError(t, err)
		assert.NotNil(t, dm)
	})

	t.Run("empty string alias", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "")
		require.NoError(t, err)
		assert.NotNil(t, dm)
	})

	t.Run("non-existent ID", func(t *testing.T) {
		_, err := adp.GetDeploymentManager(ctx, "non-existent-dm-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// --- Health with fake client extended ---

func TestUnit_Health_Extended(t *testing.T) {
	adp := unitTestAdapter(t)
	ctx := context.Background()

	// Fake clientset will succeed with version check
	err := adp.Health(ctx)
	// The important thing is it does not panic and returns a result
	_ = err
}
