package vmware_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/vmware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"
)

// newTestVMwareAdapter creates a VMware adapter suitable for unit testing.
// It uses direct struct construction with a nop logger and empty subscription map.
func newTestVMwareAdapter() *vmware.Adapter {
	return vmware.NewTestAdapter()
}

// --- Config Validation Tests ---

func TestValidateVMwareConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *vmware.Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing vCenterURL",
			config: &vmware.Config{
				Username:   "admin",
				Password:   "pass",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "vCenterURL is required",
		},
		{
			name: "missing username",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Password:   "pass",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name: "missing password",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "password is required",
		},
		{
			name: "missing datacenter",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "pass",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "datacenter is required",
		},
		{
			name: "missing oCloudID",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "pass",
				Datacenter: "DC1",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "invalid poolMode",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "pass",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
				PoolMode:   "invalid",
			},
			wantErr: true,
			errMsg:  "poolMode must be 'cluster' or 'pool'",
		},
		{
			name: "valid poolMode cluster",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "pass",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
				PoolMode:   "cluster",
			},
			// Will fail at connection, but validation passes
			wantErr: true,
			errMsg:  "vCenter",
		},
		{
			name: "valid poolMode pool",
			config: &vmware.Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "pass",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
				PoolMode:   "pool",
			},
			wantErr: true,
			errMsg:  "vCenter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := vmware.New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adp)
			}
		})
	}
}

// --- Metadata Tests ---

func TestVMwareAdapter_Name(t *testing.T) {
	adp := newTestVMwareAdapter()
	assert.Equal(t, "vmware", adp.Name())
}

func TestVMwareAdapter_Version(t *testing.T) {
	adp := newTestVMwareAdapter()
	// With nil client, should return default version
	version := adp.Version()
	assert.NotEmpty(t, version)
	assert.Equal(t, "vsphere-7.0", version)
}

func TestVMwareAdapter_Capabilities(t *testing.T) {
	adp := newTestVMwareAdapter()
	caps := adp.Capabilities()

	assert.Len(t, caps, 6)
	assert.Contains(t, caps, adapter.CapabilityResourcePools)
	assert.Contains(t, caps, adapter.CapabilityResources)
	assert.Contains(t, caps, adapter.CapabilityResourceTypes)
	assert.Contains(t, caps, adapter.CapabilityDeploymentManagers)
	assert.Contains(t, caps, adapter.CapabilitySubscriptions)
	assert.Contains(t, caps, adapter.CapabilityHealthChecks)
}

// --- ID Generation Tests ---

func TestGenerateVMProfileID_Unit(t *testing.T) {
	tests := []struct {
		name     string
		cpus     int32
		memoryMB int64
		want     string
	}{
		{
			name:     "small profile",
			cpus:     1,
			memoryMB: 1024,
			want:     "vmware-profile-1cpu-1024MB",
		},
		{
			name:     "medium profile",
			cpus:     4,
			memoryMB: 8192,
			want:     "vmware-profile-4cpu-8192MB",
		},
		{
			name:     "large profile",
			cpus:     16,
			memoryMB: 32768,
			want:     "vmware-profile-16cpu-32768MB",
		},
		{
			name:     "zero values",
			cpus:     0,
			memoryMB: 0,
			want:     "vmware-profile-0cpu-0MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vmware.GenerateVMProfileID(tt.cpus, tt.memoryMB)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateVMID_Unit(t *testing.T) {
	tests := []struct {
		name          string
		vmName        string
		clusterOrPool string
		want          string
	}{
		{
			name:          "standard vm",
			vmName:        "web-server-1",
			clusterOrPool: "cluster-1",
			want:          "vmware-vm-cluster-1-web-server-1",
		},
		{
			name:          "empty values",
			vmName:        "",
			clusterOrPool: "",
			want:          "vmware-vm--",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vmware.GenerateVMID(tt.vmName, tt.clusterOrPool)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateClusterPoolID(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		want        string
	}{
		{
			name:        "standard cluster",
			clusterName: "production-cluster",
			want:        "vmware-cluster-production-cluster",
		},
		{
			name:        "empty cluster",
			clusterName: "",
			want:        "vmware-cluster-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vmware.GenerateClusterPoolID(tt.clusterName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateResourcePoolID(t *testing.T) {
	tests := []struct {
		name        string
		poolName    string
		clusterName string
		want        string
	}{
		{
			name:        "standard pool",
			poolName:    "pool-1",
			clusterName: "cluster-1",
			want:        "vmware-pool-cluster-1-pool-1",
		},
		{
			name:        "empty pool",
			poolName:    "",
			clusterName: "",
			want:        "vmware-pool--",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vmware.GenerateResourcePoolID(tt.poolName, tt.clusterName)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Subscription Tests ---

func TestVMwareAdapter_CreateSubscription(t *testing.T) {
	tests := []struct {
		name    string
		sub     *adapter.Subscription
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid subscription",
			sub: &adapter.Subscription{
				Callback:               "https://callback.example.com/notify",
				ConsumerSubscriptionID: "consumer-sub-1",
				Filter: &adapter.SubscriptionFilter{
					ResourcePoolID: "pool-1",
				},
			},
			wantErr: false,
		},
		{
			name: "subscription with custom ID",
			sub: &adapter.Subscription{
				SubscriptionID: "custom-sub-id",
				Callback:       "https://callback.example.com/notify",
			},
			wantErr: false,
		},
		{
			name: "missing callback URL",
			sub: &adapter.Subscription{
				Callback: "",
			},
			wantErr: true,
			errMsg:  "callback URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := newTestVMwareAdapter()
			ctx := context.Background()

			created, err := adp.CreateSubscription(ctx, tt.sub)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, created)
			} else {
				require.NoError(t, err)
				require.NotNil(t, created)
				assert.NotEmpty(t, created.SubscriptionID)
				assert.Equal(t, tt.sub.Callback, created.Callback)
				assert.Equal(t, tt.sub.ConsumerSubscriptionID, created.ConsumerSubscriptionID)

				if tt.sub.SubscriptionID != "" {
					assert.Equal(t, tt.sub.SubscriptionID, created.SubscriptionID)
				} else {
					assert.Contains(t, created.SubscriptionID, "vmware-sub-")
				}
			}
		})
	}
}

func TestVMwareAdapter_GetSubscription(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	// Create a subscription first
	sub := &adapter.Subscription{
		SubscriptionID: "test-sub-get",
		Callback:       "https://callback.example.com/notify",
	}
	created, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	t.Run("get existing subscription", func(t *testing.T) {
		retrieved, getErr := adp.GetSubscription(ctx, created.SubscriptionID)
		require.NoError(t, getErr)
		assert.Equal(t, created.SubscriptionID, retrieved.SubscriptionID)
		assert.Equal(t, created.Callback, retrieved.Callback)
	})

	t.Run("get non-existent subscription", func(t *testing.T) {
		_, getErr := adp.GetSubscription(ctx, "nonexistent-id")
		require.Error(t, getErr)
		assert.Contains(t, getErr.Error(), "subscription not found")
	})
}

func TestVMwareAdapter_UpdateSubscription(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	// Create a subscription first
	sub := &adapter.Subscription{
		SubscriptionID:         "test-sub-update",
		Callback:               "https://callback.example.com/original",
		ConsumerSubscriptionID: "consumer-1",
	}
	_, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	t.Run("update existing subscription", func(t *testing.T) {
		updated, updateErr := adp.UpdateSubscription(ctx, "test-sub-update", &adapter.Subscription{
			Callback:               "https://callback.example.com/updated",
			ConsumerSubscriptionID: "consumer-2",
			Filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "compute",
			},
		})
		require.NoError(t, updateErr)
		require.NotNil(t, updated)
		assert.Equal(t, "test-sub-update", updated.SubscriptionID)
		assert.Equal(t, "https://callback.example.com/updated", updated.Callback)
		assert.Equal(t, "consumer-2", updated.ConsumerSubscriptionID)
		require.NotNil(t, updated.Filter)
		assert.Equal(t, "compute", updated.Filter.ResourceTypeID)
	})

	t.Run("update persists changes", func(t *testing.T) {
		fetched, getErr := adp.GetSubscription(ctx, "test-sub-update")
		require.NoError(t, getErr)
		assert.Equal(t, "https://callback.example.com/updated", fetched.Callback)
	})

	t.Run("update non-existent subscription", func(t *testing.T) {
		_, updateErr := adp.UpdateSubscription(ctx, "nonexistent-id", &adapter.Subscription{
			Callback: "https://callback.example.com/updated",
		})
		require.Error(t, updateErr)
		assert.Contains(t, updateErr.Error(), "subscription not found")
	})

	t.Run("update with empty callback", func(t *testing.T) {
		_, updateErr := adp.UpdateSubscription(ctx, "test-sub-update", &adapter.Subscription{
			Callback: "",
		})
		require.Error(t, updateErr)
		assert.Contains(t, updateErr.Error(), "callback URL is required")
	})
}

func TestVMwareAdapter_DeleteSubscription(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	// Create a subscription first
	sub := &adapter.Subscription{
		SubscriptionID: "test-sub-delete",
		Callback:       "https://callback.example.com/notify",
	}
	_, err := adp.CreateSubscription(ctx, sub)
	require.NoError(t, err)

	t.Run("delete existing subscription", func(t *testing.T) {
		deleteErr := adp.DeleteSubscription(ctx, "test-sub-delete")
		require.NoError(t, deleteErr)

		// Verify it's gone
		_, getErr := adp.GetSubscription(ctx, "test-sub-delete")
		require.Error(t, getErr)
		assert.Contains(t, getErr.Error(), "subscription not found")
	})

	t.Run("delete non-existent subscription", func(t *testing.T) {
		deleteErr := adp.DeleteSubscription(ctx, "nonexistent-id")
		require.Error(t, deleteErr)
		assert.Contains(t, deleteErr.Error(), "subscription not found")
	})
}

func TestVMwareAdapter_ListSubscriptions(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	t.Run("list empty subscriptions", func(t *testing.T) {
		subs := adp.ListSubscriptions()
		assert.Empty(t, subs)
	})

	t.Run("list multiple subscriptions", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			sub := &adapter.Subscription{
				Callback: "https://callback.example.com/notify",
			}
			_, err := adp.CreateSubscription(ctx, sub)
			require.NoError(t, err)
		}

		subs := adp.ListSubscriptions()
		assert.Len(t, subs, 3)

		for _, sub := range subs {
			assert.NotEmpty(t, sub.SubscriptionID)
			assert.NotEmpty(t, sub.Callback)
		}
	})
}

// --- Resource Type Tests ---

func TestVMwareAdapter_CreateResourceType(t *testing.T) {
	adp := newTestVMwareAdapter()

	tests := []struct {
		name     string
		cpuCount int32
		memoryMB int64
		wantID   string
		wantName string
	}{
		{
			name:     "small profile",
			cpuCount: 1,
			memoryMB: 1024,
			wantID:   "vmware-profile-1cpu-1024MB",
			wantName: "VM-1cpu-1GB",
		},
		{
			name:     "medium profile",
			cpuCount: 4,
			memoryMB: 4096,
			wantID:   "vmware-profile-4cpu-4096MB",
			wantName: "VM-4cpu-4GB",
		},
		{
			name:     "large profile",
			cpuCount: 8,
			memoryMB: 16384,
			wantID:   "vmware-profile-8cpu-16384MB",
			wantName: "VM-8cpu-16GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := adp.CreateResourceType(tt.cpuCount, tt.memoryMB)

			assert.Equal(t, tt.wantID, rt.ResourceTypeID)
			assert.Equal(t, tt.wantName, rt.Name)
			assert.Equal(t, "VMware", rt.Vendor)
			assert.Equal(t, "vSphere", rt.Version)
			assert.Equal(t, "compute", rt.ResourceClass)
			assert.Equal(t, "virtual", rt.ResourceKind)
			assert.Contains(t, rt.Description, "VMware VM Profile")
			assert.NotNil(t, rt.Extensions)
			assert.Equal(t, tt.cpuCount, rt.Extensions["vmware.numCpu"])
			assert.Equal(t, tt.memoryMB, rt.Extensions["vmware.memorySizeMB"])
		})
	}
}

func TestVMwareAdapter_GetDefaultResourceTypes(t *testing.T) {
	adp := newTestVMwareAdapter()

	resourceTypes := adp.GetDefaultResourceTypes()

	assert.Len(t, resourceTypes, 10)

	// Verify first and last profiles
	assert.Equal(t, "vmware-profile-1cpu-1024MB", resourceTypes[0].ResourceTypeID)
	assert.Equal(t, "vmware-profile-32cpu-65536MB", resourceTypes[9].ResourceTypeID)

	// Verify all have required fields
	for _, rt := range resourceTypes {
		assert.NotEmpty(t, rt.ResourceTypeID)
		assert.NotEmpty(t, rt.Name)
		assert.Equal(t, "VMware", rt.Vendor)
		assert.Equal(t, "compute", rt.ResourceClass)
		assert.Equal(t, "virtual", rt.ResourceKind)
	}
}

// --- Not-Implemented Operations Tests ---

func TestVMwareAdapter_CreateResourcePool_NotImplemented(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	pool, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{
		Name: "test-pool",
	})

	require.Error(t, err)
	assert.Nil(t, pool)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestVMwareAdapter_UpdateResourcePool_NotImplemented(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	pool, err := adp.UpdateResourcePool(ctx, "pool-id", &adapter.ResourcePool{
		Name: "updated-pool",
	})

	require.Error(t, err)
	assert.Nil(t, pool)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestVMwareAdapter_DeleteResourcePool_NotImplemented(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	err := adp.DeleteResourcePool(ctx, "pool-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestVMwareAdapter_CreateResource_NotImplemented(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	resource, err := adp.CreateResource(ctx, &adapter.Resource{
		ResourceTypeID: "vmware-profile-1cpu-1024MB",
	})

	require.Error(t, err)
	assert.Nil(t, resource)
	assert.Contains(t, err.Error(), "additional configuration")
}

// --- Test Helper Exports ---

func TestVMwareAdapter_TestValidateVMResourceID(t *testing.T) {
	adp := newTestVMwareAdapter()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid resource ID",
			id:      "vmware-vm-cluster1-myvm",
			wantErr: false,
		},
		{
			name:    "invalid prefix",
			id:      "invalid-vm-cluster1-myvm",
			wantErr: true,
		},
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adp.TestValidateVMResourceID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid resource ID format")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVMwareAdapter_TestBuildVMAnnotation(t *testing.T) {
	adp := newTestVMwareAdapter()

	tests := []struct {
		name     string
		resource *adapter.Resource
		want     string
	}{
		{
			name: "description only",
			resource: &adapter.Resource{
				Description: "My production VM",
			},
			want: "My production VM",
		},
		{
			name: "description with custom attributes",
			resource: &adapter.Resource{
				Description: "Web server",
				Extensions: map[string]interface{}{
					"vmware.customAttributes": map[string]string{
						"env":  "production",
						"team": "platform",
					},
				},
			},
			want: "Web server\n\nCustom Attributes:",
		},
		{
			name: "empty description no extensions",
			resource: &adapter.Resource{
				Description: "",
			},
			want: "",
		},
		{
			name: "extensions but no custom attributes",
			resource: &adapter.Resource{
				Description: "Test VM",
				Extensions: map[string]interface{}{
					"vmware.name": "test-vm",
				},
			},
			want: "Test VM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildVMAnnotation(tt.resource)
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestVMwareAdapter_TestExtractCustomAttributes(t *testing.T) {
	adp := newTestVMwareAdapter()

	tests := []struct {
		name       string
		extensions map[string]interface{}
		wantLen    int
	}{
		{
			name: "valid custom attributes",
			extensions: map[string]interface{}{
				"vmware.customAttributes": map[string]string{
					"env":  "production",
					"team": "platform",
				},
			},
			wantLen: 2,
		},
		{
			name: "no custom attributes key",
			extensions: map[string]interface{}{
				"vmware.name": "test-vm",
			},
			wantLen: 0,
		},
		{
			name: "wrong type for custom attributes",
			extensions: map[string]interface{}{
				"vmware.customAttributes": "not a map",
			},
			wantLen: 0,
		},
		{
			name:       "empty extensions",
			extensions: map[string]interface{}{},
			wantLen:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestExtractCustomAttributes(tt.extensions)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestVMwareAdapter_TestGetVMDescription(t *testing.T) {
	adp := newTestVMwareAdapter()

	tests := []struct {
		name       string
		vmName     string
		annotation string
		want       string
	}{
		{
			name:       "with annotation",
			vmName:     "test-vm",
			annotation: "Production web server",
			want:       "Production web server",
		},
		{
			name:       "without annotation falls back to vmName",
			vmName:     "test-vm",
			annotation: "",
			want:       "test-vm",
		},
		{
			name:       "empty vmName empty annotation",
			vmName:     "",
			annotation: "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestGetVMDescription(tt.vmName, tt.annotation)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Close Tests ---

func TestVMwareAdapter_Close(t *testing.T) {
	t.Run("close with nil client", func(t *testing.T) {
		adp := newTestVMwareAdapter()

		// Add some subscriptions that should be cleared
		adp.ExportSubscriptions()["sub-1"] = &adapter.Subscription{SubscriptionID: "sub-1"}
		adp.ExportSubscriptions()["sub-2"] = &adapter.Subscription{SubscriptionID: "sub-2"}

		err := adp.Close()
		assert.NoError(t, err)

		// Verify subscriptions are cleared
		assert.Empty(t, adp.ExportSubscriptions())
	})
}

// --- GetResource Invalid ID Tests ---

func TestVMwareAdapter_GetResource_InvalidID(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	tests := []struct {
		name string
		id   string
	}{
		{
			name: "invalid prefix",
			id:   "invalid-prefix-myvm",
		},
		{
			name: "empty id",
			id:   "",
		},
		{
			name: "completely wrong format",
			id:   "just-some-string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, err := adp.GetResource(ctx, tt.id)
			require.Error(t, err)
			assert.Nil(t, resource)
			assert.Contains(t, err.Error(), "invalid resource ID format")
		})
	}
}

// --- Concurrency Tests ---

func TestVMwareAdapter_SubscriptionConcurrency(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	t.Run("concurrent creates", func(t *testing.T) {
		const numGoroutines = 20

		var wg sync.WaitGroup
		ids := make(chan string, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				sub := &adapter.Subscription{
					Callback: "https://callback.example.com/notify",
				}
				created, err := adp.CreateSubscription(ctx, sub)
				if err == nil {
					ids <- created.SubscriptionID
				}
			}(i)
		}

		wg.Wait()
		close(ids)

		// All subscriptions should have unique IDs
		uniqueIDs := make(map[string]bool)
		for id := range ids {
			uniqueIDs[id] = true
		}
		assert.Equal(t, numGoroutines, len(uniqueIDs))
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		readAdp := newTestVMwareAdapter()

		// Create initial subscriptions
		for i := 0; i < 5; i++ {
			_, err := readAdp.CreateSubscription(ctx, &adapter.Subscription{
				Callback: "https://callback.example.com/notify",
			})
			require.NoError(t, err)
		}

		var wg sync.WaitGroup
		const readers = 10

		// Concurrent reads
		for i := 0; i < readers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				subs := readAdp.ListSubscriptions()
				assert.NotNil(t, subs)
			}()
		}

		wg.Wait()
	})
}

// --- Subscription Filter Tests ---

func TestVMwareAdapter_SubscriptionWithFilters(t *testing.T) {
	tests := []struct {
		name   string
		filter *adapter.SubscriptionFilter
	}{
		{
			name: "filter by resource pool",
			filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-123",
			},
		},
		{
			name: "filter by resource type",
			filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "type-456",
			},
		},
		{
			name: "filter by resource ID",
			filter: &adapter.SubscriptionFilter{
				ResourceID: "resource-789",
			},
		},
		{
			name: "combined filters",
			filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-123",
				ResourceTypeID: "type-456",
				ResourceID:     "resource-789",
			},
		},
		{
			name:   "nil filter",
			filter: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := newTestVMwareAdapter()
			ctx := context.Background()

			sub := &adapter.Subscription{
				Callback: "https://callback.example.com/notify",
				Filter:   tt.filter,
			}

			created, err := adp.CreateSubscription(ctx, sub)
			require.NoError(t, err)
			assert.NotEmpty(t, created.SubscriptionID)

			if tt.filter != nil {
				require.NotNil(t, created.Filter)
				assert.Equal(t, tt.filter.ResourcePoolID, created.Filter.ResourcePoolID)
				assert.Equal(t, tt.filter.ResourceTypeID, created.Filter.ResourceTypeID)
				assert.Equal(t, tt.filter.ResourceID, created.Filter.ResourceID)
			}

			// Cleanup
			err = adp.DeleteSubscription(ctx, created.SubscriptionID)
			require.NoError(t, err)
		})
	}
}

// --- Exported Internal Function Tests ---

// TestExportAddVMConfigInfo tests the addVMConfigInfo helper.
func TestExportAddVMConfigInfo(t *testing.T) {
	ext := make(map[string]interface{})
	config := types.VirtualMachineConfigSummary{
		GuestFullName: "Ubuntu 22.04",
		GuestId:       "ubuntu64",
		NumCpu:        4,
		MemorySizeMB:  8192,
		Uuid:          "uuid-1234",
		InstanceUuid:  "inst-5678",
		Template:      false,
	}

	vmware.ExportAddVMConfigInfo(ext, config)

	assert.Equal(t, "Ubuntu 22.04", ext["vmware.guestFullName"])
	assert.Equal(t, "ubuntu64", ext["vmware.guestId"])
	assert.Equal(t, int32(4), ext["vmware.numCpu"])
	assert.Equal(t, int32(8192), ext["vmware.memorySizeMB"])
	assert.Equal(t, "uuid-1234", ext["vmware.uuid"])
	assert.Equal(t, "inst-5678", ext["vmware.instanceUuid"])
	assert.Equal(t, false, ext["vmware.template"])
}

// TestExportAddVMRuntimeInfo tests the addVMRuntimeInfo helper.
func TestExportAddVMRuntimeInfo(t *testing.T) {
	t.Run("basic runtime info", func(t *testing.T) {
		ext := make(map[string]interface{})
		runtime := types.VirtualMachineRuntimeInfo{
			PowerState:      types.VirtualMachinePowerStatePoweredOn,
			ConnectionState: types.VirtualMachineConnectionStateConnected,
		}

		vmware.ExportAddVMRuntimeInfo(ext, runtime)

		assert.Equal(t, "poweredOn", ext["vmware.powerState"])
		assert.Equal(t, "connected", ext["vmware.connectionState"])
	})

	t.Run("runtime with host", func(t *testing.T) {
		ext := make(map[string]interface{})
		hostRef := types.ManagedObjectReference{Value: "host-42"}
		runtime := types.VirtualMachineRuntimeInfo{
			PowerState:      types.VirtualMachinePowerStatePoweredOff,
			ConnectionState: types.VirtualMachineConnectionStateDisconnected,
			Host:            &hostRef,
		}

		vmware.ExportAddVMRuntimeInfo(ext, runtime)

		assert.Equal(t, "poweredOff", ext["vmware.powerState"])
		assert.Equal(t, "host-42", ext["vmware.host"])
	})

	t.Run("runtime with boot time", func(t *testing.T) {
		ext := make(map[string]interface{})
		bootTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		runtime := types.VirtualMachineRuntimeInfo{
			PowerState:      types.VirtualMachinePowerStatePoweredOn,
			ConnectionState: types.VirtualMachineConnectionStateConnected,
			BootTime:        &bootTime,
		}

		vmware.ExportAddVMRuntimeInfo(ext, runtime)

		assert.NotEmpty(t, ext["vmware.bootTime"])
	})
}

// TestExportAddVMQuickStats tests the addVMQuickStats helper.
func TestExportAddVMQuickStats(t *testing.T) {
	ext := make(map[string]interface{})
	stats := types.VirtualMachineQuickStats{
		OverallCpuUsage:  2400,
		GuestMemoryUsage: 4096,
		HostMemoryUsage:  6144,
		UptimeSeconds:    86400,
	}

	vmware.ExportAddVMQuickStats(ext, stats)

	assert.Equal(t, int32(2400), ext["vmware.overallCpuUsage"])
	assert.Equal(t, int32(4096), ext["vmware.guestMemoryUsage"])
	assert.Equal(t, int32(6144), ext["vmware.hostMemoryUsage"])
	assert.Equal(t, int32(86400), ext["vmware.uptimeSeconds"])
}

// TestExportAddVMGuestInfo tests the addVMGuestInfo helper.
func TestExportAddVMGuestInfo(t *testing.T) {
	t.Run("nil guest info", func(t *testing.T) {
		ext := make(map[string]interface{})
		vmware.ExportAddVMGuestInfo(ext, nil)
		// Should not add any keys
		assert.Empty(t, ext)
	})

	t.Run("basic guest info", func(t *testing.T) {
		ext := make(map[string]interface{})
		guest := &types.GuestInfo{
			GuestState:   "running",
			ToolsStatus:  types.VirtualMachineToolsStatusToolsOk,
			ToolsVersion: "12345",
			HostName:     "web-server",
			IpAddress:    "192.168.1.100",
		}

		vmware.ExportAddVMGuestInfo(ext, guest)

		assert.Equal(t, "running", ext["vmware.guestState"])
		assert.Equal(t, "toolsOk", ext["vmware.guestToolsStatus"])
		assert.Equal(t, "12345", ext["vmware.guestToolsVersion"])
		assert.Equal(t, "web-server", ext["vmware.hostName"])
		assert.Equal(t, "192.168.1.100", ext["vmware.ipAddress"])
	})

	t.Run("guest info with network interfaces", func(t *testing.T) {
		ext := make(map[string]interface{})
		guest := &types.GuestInfo{
			GuestState: "running",
			Net: []types.GuestNicInfo{
				{
					Network:    "VM Network",
					Connected:  true,
					MacAddress: "00:50:56:ab:cd:ef",
					IpAddress:  []string{"10.0.0.1", "fd00::1"},
				},
				{
					Network:    "Management",
					Connected:  false,
					MacAddress: "00:50:56:12:34:56",
				},
			},
		}

		vmware.ExportAddVMGuestInfo(ext, guest)

		nics, ok := ext["vmware.networkInterfaces"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, nics, 2)

		assert.Equal(t, "VM Network", nics[0]["network"])
		assert.Equal(t, true, nics[0]["connected"])
		assert.Equal(t, "00:50:56:ab:cd:ef", nics[0]["macAddress"])

		ips, hasIPs := nics[0]["ipAddresses"]
		require.True(t, hasIPs)
		assert.Equal(t, []string{"10.0.0.1", "fd00::1"}, ips)

		// Second NIC has no IPs
		_, hasIPs2 := nics[1]["ipAddresses"]
		assert.False(t, hasIPs2)
	})
}

// TestExportAddVMStorageInfo tests the addVMStorageInfo helper.
func TestExportAddVMStorageInfo(t *testing.T) {
	t.Run("nil storage", func(t *testing.T) {
		ext := make(map[string]interface{})
		vmware.ExportAddVMStorageInfo(ext, nil)
		assert.Empty(t, ext)
	})

	t.Run("with storage info", func(t *testing.T) {
		ext := make(map[string]interface{})
		storage := &types.VirtualMachineStorageSummary{
			Committed:   1073741824,
			Uncommitted: 2147483648,
			Unshared:    536870912,
		}

		vmware.ExportAddVMStorageInfo(ext, storage)

		assert.Equal(t, int64(1073741824), ext["vmware.committed"])
		assert.Equal(t, int64(2147483648), ext["vmware.uncommitted"])
		assert.Equal(t, int64(536870912), ext["vmware.unshared"])
	})
}

// TestExportBuildVMExtensions tests the full buildVMExtensions function.
func TestExportBuildVMExtensions(t *testing.T) {
	vm := &mo.VirtualMachine{
		ManagedEntity: mo.ManagedEntity{},
	}
	vm.Summary.Config = types.VirtualMachineConfigSummary{
		GuestFullName: "Ubuntu 22.04",
		GuestId:       "ubuntu64",
		NumCpu:        2,
		MemorySizeMB:  4096,
		Uuid:          "uuid-abc",
	}
	vm.Summary.Runtime = types.VirtualMachineRuntimeInfo{
		PowerState:      types.VirtualMachinePowerStatePoweredOn,
		ConnectionState: types.VirtualMachineConnectionStateConnected,
	}
	vm.Summary.QuickStats = types.VirtualMachineQuickStats{
		OverallCpuUsage: 1200,
		UptimeSeconds:   3600,
	}

	ext := vmware.ExportBuildVMExtensions(vm, "test-vm", "DC1")

	assert.Equal(t, "test-vm", ext["vmware.name"])
	assert.Equal(t, "DC1", ext["vmware.datacenter"])
	assert.Equal(t, "Ubuntu 22.04", ext["vmware.guestFullName"])
	assert.Equal(t, "poweredOn", ext["vmware.powerState"])
	assert.Equal(t, int32(1200), ext["vmware.overallCpuUsage"])
}

// TestExportDetermineVMResourcePoolID tests pool ID determination.
func TestExportDetermineVMResourcePoolID(t *testing.T) {
	t.Run("cluster mode returns cluster pool ID", func(t *testing.T) {
		adp := vmware.NewTestAdapter()
		// Set pool mode to cluster via Config defaults
		// Since poolMode is unexported, use the adapter as-is (defaults to "")
		// The Export function should handle this

		vm := &mo.VirtualMachine{}

		// With default pool mode (non-cluster), it should use resource pool ref
		poolID := adp.ExportDetermineVMResourcePoolID(vm)
		// When vm.ResourcePool is nil, it defaults
		assert.Contains(t, poolID, "vmware-pool-")
	})

	t.Run("with resource pool reference", func(t *testing.T) {
		adp := vmware.NewTestAdapter()

		rpRef := types.ManagedObjectReference{Value: "resgroup-42"}
		vm := &mo.VirtualMachine{
			ManagedEntity: mo.ManagedEntity{},
		}
		vm.ResourcePool = &rpRef

		poolID := adp.ExportDetermineVMResourcePoolID(vm)
		assert.Contains(t, poolID, "vmware-pool-")
		assert.Contains(t, poolID, "resgroup-42")
	})
}

// TestExportExtractTenantIDFromVM tests tenant ID extraction.
func TestExportExtractTenantIDFromVM(t *testing.T) {
	adp := newTestVMwareAdapter()

	t.Run("nil config returns empty", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Config = nil
		result := adp.ExportExtractTenantIDFromVM(vm)
		assert.Empty(t, result)
	})

	t.Run("nil extra config returns empty", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Config = &types.VirtualMachineConfigInfo{}
		result := adp.ExportExtractTenantIDFromVM(vm)
		assert.Empty(t, result)
	})

	t.Run("extra config with tenant ID", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Config = &types.VirtualMachineConfigInfo{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "o2ims.io/tenant-id", Value: "tenant-abc"},
			},
		}
		result := adp.ExportExtractTenantIDFromVM(vm)
		assert.Equal(t, "tenant-abc", result)
	})

	t.Run("extra config without tenant ID", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Config = &types.VirtualMachineConfigInfo{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "other-key", Value: "other-value"},
			},
		}
		result := adp.ExportExtractTenantIDFromVM(vm)
		assert.Empty(t, result)
	})

	t.Run("extra config with wrong value type", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Config = &types.VirtualMachineConfigInfo{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "o2ims.io/tenant-id", Value: 12345},
			},
		}
		result := adp.ExportExtractTenantIDFromVM(vm)
		assert.Empty(t, result)
	})
}

// TestExportValidateVMwareConfig tests config validation.
func TestExportValidateVMwareConfig(t *testing.T) {
	validCfg := &vmware.Config{
		VCenterURL: "https://vcenter.example.com/sdk",
		Username:   "admin",
		Password:   "secret",
		Datacenter: "DC1",
		OCloudID:   "ocloud-1",
	}

	t.Run("valid config", func(t *testing.T) {
		err := vmware.ExportValidateVMwareConfig(validCfg)
		require.NoError(t, err)
	})

	t.Run("valid config with pool mode cluster", func(t *testing.T) {
		cfg := *validCfg
		cfg.PoolMode = "cluster"
		err := vmware.ExportValidateVMwareConfig(&cfg)
		require.NoError(t, err)
	})

	t.Run("valid config with pool mode pool", func(t *testing.T) {
		cfg := *validCfg
		cfg.PoolMode = "pool"
		err := vmware.ExportValidateVMwareConfig(&cfg)
		require.NoError(t, err)
	})
}

// TestExportApplyVMwareDefaults tests default application.
func TestExportApplyVMwareDefaults(t *testing.T) {
	t.Run("all defaults", func(t *testing.T) {
		cfg := &vmware.Config{
			Datacenter: "DC1",
		}
		dmID, poolMode, timeout := vmware.ExportApplyVMwareDefaults(cfg)
		assert.Equal(t, "ocloud-vmware-DC1", dmID)
		assert.Equal(t, "cluster", poolMode)
		assert.Equal(t, 30*time.Second, timeout)
	})

	t.Run("custom values", func(t *testing.T) {
		cfg := &vmware.Config{
			Datacenter:          "DC2",
			DeploymentManagerID: "custom-dm",
			PoolMode:            "pool",
			Timeout:             60 * time.Second,
		}
		dmID, poolMode, timeout := vmware.ExportApplyVMwareDefaults(cfg)
		assert.Equal(t, "custom-dm", dmID)
		assert.Equal(t, "pool", poolMode)
		assert.Equal(t, 60*time.Second, timeout)
	})
}

// TestVMwareAdapter_Version tests Version with nil client.
func TestVMwareAdapter_Version_NilClient(t *testing.T) {
	adp := newTestVMwareAdapter()
	assert.Equal(t, "vsphere-7.0", adp.Version())
}

// --- vmToResource Tests ---

func TestExportVMToResource(t *testing.T) {
	adp := newTestVMwareAdapter()

	t.Run("basic VM conversion", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Summary.Config = types.VirtualMachineConfigSummary{
			GuestFullName: "Ubuntu 22.04",
			GuestId:       "ubuntu64",
			NumCpu:        4,
			MemorySizeMB:  8192,
			Uuid:          "uuid-abc",
			InstanceUuid:  "inst-def",
			Annotation:    "Production web server",
		}
		vm.Summary.Runtime = types.VirtualMachineRuntimeInfo{
			PowerState:      types.VirtualMachinePowerStatePoweredOn,
			ConnectionState: types.VirtualMachineConnectionStateConnected,
		}

		resource := adp.ExportVMToResource(vm, "web-server-1")
		require.NotNil(t, resource)

		assert.Contains(t, resource.ResourceID, "vmware-vm-")
		assert.Contains(t, resource.ResourceID, "web-server-1")
		assert.Equal(t, "vmware-profile-4cpu-8192MB", resource.ResourceTypeID)
		assert.Contains(t, resource.GlobalAssetID, "urn:vmware:vm:")
		assert.Contains(t, resource.GlobalAssetID, "web-server-1")
		assert.Equal(t, "Production web server", resource.Description)
		assert.NotNil(t, resource.Extensions)
		assert.Equal(t, "web-server-1", resource.Extensions["vmware.name"])
	})

	t.Run("VM without annotation uses vmName as description", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Summary.Config = types.VirtualMachineConfigSummary{
			NumCpu:       2,
			MemorySizeMB: 4096,
		}
		vm.Summary.Runtime = types.VirtualMachineRuntimeInfo{
			PowerState: types.VirtualMachinePowerStatePoweredOff,
		}

		resource := adp.ExportVMToResource(vm, "test-vm")
		require.NotNil(t, resource)
		assert.Equal(t, "test-vm", resource.Description)
	})

	t.Run("VM with tenant ID in extra config", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Summary.Config = types.VirtualMachineConfigSummary{
			NumCpu:       1,
			MemorySizeMB: 1024,
		}
		vm.Config = &types.VirtualMachineConfigInfo{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "o2ims.io/tenant-id", Value: "tenant-xyz"},
			},
		}

		resource := adp.ExportVMToResource(vm, "tenant-vm")
		require.NotNil(t, resource)
		assert.Equal(t, "tenant-xyz", resource.TenantID)
	})

	t.Run("VM with resource pool reference", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Summary.Config = types.VirtualMachineConfigSummary{
			NumCpu:       2,
			MemorySizeMB: 2048,
		}
		rpRef := types.ManagedObjectReference{Value: "resgroup-99"}
		vm.ResourcePool = &rpRef

		resource := adp.ExportVMToResource(vm, "pool-vm")
		require.NotNil(t, resource)
		assert.Contains(t, resource.ResourcePoolID, "vmware-")
	})

	t.Run("VM with storage info", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Summary.Config = types.VirtualMachineConfigSummary{
			NumCpu:       1,
			MemorySizeMB: 512,
		}
		vm.Summary.Storage = &types.VirtualMachineStorageSummary{
			Committed:   1073741824,
			Uncommitted: 2147483648,
		}

		resource := adp.ExportVMToResource(vm, "storage-vm")
		require.NotNil(t, resource)
		assert.Equal(t, int64(1073741824), resource.Extensions["vmware.committed"])
		assert.Equal(t, int64(2147483648), resource.Extensions["vmware.uncommitted"])
	})

	t.Run("VM with guest info", func(t *testing.T) {
		vm := &mo.VirtualMachine{}
		vm.Summary.Config = types.VirtualMachineConfigSummary{
			NumCpu:       1,
			MemorySizeMB: 512,
		}
		vm.Guest = &types.GuestInfo{
			GuestState:   "running",
			ToolsStatus:  types.VirtualMachineToolsStatusToolsOk,
			ToolsVersion: "12345",
			HostName:     "guest-host",
			IpAddress:    "10.0.0.5",
		}

		resource := adp.ExportVMToResource(vm, "guest-vm")
		require.NotNil(t, resource)
		assert.Equal(t, "running", resource.Extensions["vmware.guestState"])
		assert.Equal(t, "guest-host", resource.Extensions["vmware.hostName"])
		assert.Equal(t, "10.0.0.5", resource.Extensions["vmware.ipAddress"])
	})
}

// --- InitializeVMwareLogger Tests ---

func TestExportInitializeVMwareLogger(t *testing.T) {
	t.Run("nil logger creates new logger", func(t *testing.T) {
		logger, err := vmware.ExportInitializeVMwareLogger(nil)
		require.NoError(t, err)
		assert.NotNil(t, logger)
	})

	t.Run("existing logger is returned as-is", func(t *testing.T) {
		existing := zap.NewNop()
		logger, err := vmware.ExportInitializeVMwareLogger(existing)
		require.NoError(t, err)
		assert.Equal(t, existing, logger)
	})
}

// --- GetVMDescription Tests ---

func TestExportGetVMDescription(t *testing.T) {
	tests := []struct {
		name       string
		vmName     string
		annotation string
		want       string
	}{
		{
			name:       "annotation present",
			vmName:     "test-vm",
			annotation: "Custom description",
			want:       "Custom description",
		},
		{
			name:       "no annotation falls back to vmName",
			vmName:     "fallback-vm",
			annotation: "",
			want:       "fallback-vm",
		},
		{
			name:       "both empty",
			vmName:     "",
			annotation: "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vmware.ExportGetVMDescription(tt.vmName, tt.annotation)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- UpdateResource with invalid ID ---

func TestVMwareAdapter_UpdateResource_InvalidID(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	_, err := adp.UpdateResource(ctx, "invalid-id", &adapter.Resource{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource ID format")
}

// --- DeleteResource with invalid ID ---

func TestVMwareAdapter_DeleteResource_InvalidID(t *testing.T) {
	adp := newTestVMwareAdapter()
	ctx := context.Background()

	err := adp.DeleteResource(ctx, "invalid-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource ID format")
}

// --- Simulator-backed Tests ---
// These tests use govmomi/simulator to create a realistic vSphere environment
// for testing functions that require a real vCenter connection.

// withSimulator runs a test function with a simulator-backed VMware adapter.
func withSimulator(t *testing.T, f func(ctx context.Context, adp *vmware.Adapter)) {
	t.Helper()
	model := simulator.VPX()
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		adp, err := vmware.ExportNewSimulatorAdapter(ctx, c, "DC0", "cluster")
		require.NoError(t, err)
		f(ctx, adp)
	}, model)
}

func withSimulatorPoolMode(t *testing.T, poolMode string, f func(ctx context.Context, adp *vmware.Adapter)) {
	t.Helper()
	model := simulator.VPX()
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		adp, err := vmware.ExportNewSimulatorAdapter(ctx, c, "DC0", poolMode)
		require.NoError(t, err)
		f(ctx, adp)
	}, model)
}

func TestVMwareSimulator_ListResources(t *testing.T) {
	t.Run("list all VMs", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			resources, err := adp.ListResources(ctx, nil)
			require.NoError(t, err)
			// VPX model creates 2 machines by default
			assert.NotEmpty(t, resources)

			for _, r := range resources {
				assert.NotEmpty(t, r.ResourceID)
				assert.Contains(t, r.ResourceID, "vmware-vm-")
				assert.NotEmpty(t, r.ResourceTypeID)
				assert.NotEmpty(t, r.ResourcePoolID)
				assert.NotEmpty(t, r.GlobalAssetID)
				assert.NotNil(t, r.Extensions)
			}
		})
	})

	t.Run("list with pagination", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			filter := &adapter.Filter{
				Limit:  1,
				Offset: 0,
			}
			resources, err := adp.ListResources(ctx, filter)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(resources), 1)
		})
	})

	t.Run("list with tenant filter no match", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			filter := &adapter.Filter{
				TenantID: "nonexistent-tenant",
			}
			resources, err := adp.ListResources(ctx, filter)
			require.NoError(t, err)
			// No VMs should match a nonexistent tenant
			assert.Empty(t, resources)
		})
	})
}

func TestVMwareSimulator_GetResource(t *testing.T) {
	t.Run("get existing resource", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			// First list to get a valid resource ID
			resources, err := adp.ListResources(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, resources)

			// Now get by ID
			resource, err := adp.GetResource(ctx, resources[0].ResourceID)
			require.NoError(t, err)
			assert.Equal(t, resources[0].ResourceID, resource.ResourceID)
		})
	})

	t.Run("get nonexistent resource", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			_, err := adp.GetResource(ctx, "vmware-vm-nonexistent-vm")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "resource not found")
		})
	})
}

func TestVMwareSimulator_ListResourcePools_ClusterMode(t *testing.T) {
	t.Run("list cluster pools", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			pools, err := adp.ListResourcePools(ctx, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, pools)

			for _, p := range pools {
				assert.NotEmpty(t, p.ResourcePoolID)
				assert.Contains(t, p.ResourcePoolID, "vmware-cluster-")
				assert.NotEmpty(t, p.Name)
				assert.NotEmpty(t, p.Description)
				assert.Equal(t, "test-ocloud", p.OCloudID)
				assert.NotNil(t, p.Extensions)
				assert.Equal(t, "Cluster", p.Extensions["vmware.type"])
			}
		})
	})

	t.Run("list with pagination", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			filter := &adapter.Filter{
				Limit:  1,
				Offset: 0,
			}
			pools, err := adp.ListResourcePools(ctx, filter)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(pools), 1)
		})
	})
}

func TestVMwareSimulator_ListResourcePools_PoolMode(t *testing.T) {
	t.Run("list resource pools in pool mode", func(t *testing.T) {
		withSimulatorPoolMode(t, "pool", func(ctx context.Context, adp *vmware.Adapter) {
			pools, err := adp.ListResourcePools(ctx, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, pools)

			for _, p := range pools {
				assert.NotEmpty(t, p.ResourcePoolID)
				assert.Contains(t, p.ResourcePoolID, "vmware-pool-")
				assert.NotNil(t, p.Extensions)
				assert.Equal(t, "ResourcePool", p.Extensions["vmware.type"])
			}
		})
	})

	t.Run("list with pagination", func(t *testing.T) {
		withSimulatorPoolMode(t, "pool", func(ctx context.Context, adp *vmware.Adapter) {
			filter := &adapter.Filter{
				Limit:  1,
				Offset: 0,
			}
			pools, err := adp.ListResourcePools(ctx, filter)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(pools), 1)
		})
	})
}

func TestVMwareSimulator_GetResourcePool(t *testing.T) {
	t.Run("get existing pool", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			pools, err := adp.ListResourcePools(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, pools)

			pool, err := adp.GetResourcePool(ctx, pools[0].ResourcePoolID)
			require.NoError(t, err)
			assert.Equal(t, pools[0].ResourcePoolID, pool.ResourcePoolID)
		})
	})

	t.Run("get nonexistent pool", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			_, err := adp.GetResourcePool(ctx, "vmware-cluster-nonexistent")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "resource pool not found")
		})
	})
}

func TestVMwareSimulator_GetDeploymentManager(t *testing.T) {
	t.Run("get by configured ID", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			dm, err := adp.GetDeploymentManager(ctx, "test-dm-vmware")
			require.NoError(t, err)
			require.NotNil(t, dm)
			assert.Equal(t, "test-dm-vmware", dm.DeploymentManagerID)
			assert.Contains(t, dm.Name, "VMware vSphere")
			assert.Equal(t, "test-ocloud", dm.OCloudID)
			assert.NotEmpty(t, dm.SupportedLocations)
			assert.NotEmpty(t, dm.Capabilities)
			assert.NotNil(t, dm.Extensions)
			assert.NotEmpty(t, dm.Extensions["vmware.version"])
		})
	})

	t.Run("get by default alias", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			dm, err := adp.GetDeploymentManager(ctx, "default")
			require.NoError(t, err)
			require.NotNil(t, dm)
		})
	})

	t.Run("get by empty ID alias", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			dm, err := adp.GetDeploymentManager(ctx, "")
			require.NoError(t, err)
			require.NotNil(t, dm)
		})
	})

	t.Run("nonexistent deployment manager", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			_, err := adp.GetDeploymentManager(ctx, "nonexistent-dm")
			require.Error(t, err)
			assert.ErrorIs(t, err, adapter.ErrDeploymentManagerNotFound)
		})
	})
}

func TestVMwareSimulator_ListDeploymentManagers(t *testing.T) {
	withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
		dms, err := adp.ListDeploymentManagers(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, dms, 1)
		assert.Equal(t, "test-dm-vmware", dms[0].DeploymentManagerID)
	})
}

func TestVMwareSimulator_ListResourceTypes(t *testing.T) {
	t.Run("list resource types", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			rts, err := adp.ListResourceTypes(ctx, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, rts)

			for _, rt := range rts {
				assert.NotEmpty(t, rt.ResourceTypeID)
				assert.Contains(t, rt.ResourceTypeID, "vmware-profile-")
				assert.Equal(t, "VMware", rt.Vendor)
				assert.Equal(t, "compute", rt.ResourceClass)
				assert.Equal(t, "virtual", rt.ResourceKind)
			}
		})
	})

	t.Run("list with pagination", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			filter := &adapter.Filter{
				Limit:  1,
				Offset: 0,
			}
			rts, err := adp.ListResourceTypes(ctx, filter)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(rts), 1)
		})
	})
}

func TestVMwareSimulator_GetResourceType(t *testing.T) {
	t.Run("get existing resource type", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			rts, err := adp.ListResourceTypes(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, rts)

			rt, err := adp.GetResourceType(ctx, rts[0].ResourceTypeID)
			require.NoError(t, err)
			assert.Equal(t, rts[0].ResourceTypeID, rt.ResourceTypeID)
		})
	})

	t.Run("get nonexistent resource type", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			_, err := adp.GetResourceType(ctx, "vmware-profile-999cpu-999MB")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "resource type not found")
		})
	})
}

func TestVMwareSimulator_Health(t *testing.T) {
	withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
		err := adp.Health(ctx)
		require.NoError(t, err)
	})
}

func TestVMwareSimulator_Version(t *testing.T) {
	withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
		version := adp.Version()
		assert.NotEmpty(t, version)
		// Simulator provides a version string, not the default fallback
		assert.NotEqual(t, "", version)
	})
}

func TestVMwareSimulator_Close(t *testing.T) {
	// Note: We test Close with a simulator client to cover the logout path.
	// The simulator.Test callback already manages the session, so we only verify
	// that Close doesn't error and subscriptions are cleared.
	// Close() creates its own context.Background() internally, so we
	// capture the adapter reference and call Close outside the withSimulator
	// callback to avoid contextcheck flagging.
	var capturedAdapter *vmware.Adapter
	withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
		// Add some subscriptions
		_, err := adp.CreateSubscription(ctx, &adapter.Subscription{
			Callback: "https://example.com/notify",
		})
		require.NoError(t, err)
		capturedAdapter = adp
	})

	// Call Close outside the context-aware callback
	require.NotNil(t, capturedAdapter)
	closeErr := capturedAdapter.Close()
	assert.NoError(t, closeErr)

	// Subscriptions should be cleared
	subs := capturedAdapter.ListSubscriptions()
	assert.Empty(t, subs)
}

func TestVMwareSimulator_UpdateResource(t *testing.T) {
	t.Run("update existing resource", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			// List resources to get a valid ID
			resources, err := adp.ListResources(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, resources)

			updated, err := adp.UpdateResource(ctx, resources[0].ResourceID, &adapter.Resource{
				Description: "Updated description",
			})
			require.NoError(t, err)
			require.NotNil(t, updated)
			assert.Equal(t, resources[0].ResourceID, updated.ResourceID)
		})
	})

	t.Run("update with extensions containing custom attributes", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			resources, err := adp.ListResources(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, resources)

			updated, err := adp.UpdateResource(ctx, resources[0].ResourceID, &adapter.Resource{
				Description: "Updated with attrs",
				Extensions: map[string]interface{}{
					"vmware.customAttributes": map[string]string{
						"env":  "test",
						"team": "ops",
					},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, updated)
		})
	})

	t.Run("update nonexistent resource", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			_, err := adp.UpdateResource(ctx, "vmware-vm-DC0-nonexistent", &adapter.Resource{
				Description: "test",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "resource not found")
		})
	})
}

func TestVMwareSimulator_DeleteResource(t *testing.T) {
	t.Run("delete existing resource", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			resources, err := adp.ListResources(ctx, nil)
			require.NoError(t, err)
			require.NotEmpty(t, resources)
			initialCount := len(resources)

			err = adp.DeleteResource(ctx, resources[0].ResourceID)
			require.NoError(t, err)

			// Verify deletion
			remaining, err := adp.ListResources(ctx, nil)
			require.NoError(t, err)
			assert.Equal(t, initialCount-1, len(remaining))
		})
	})

	t.Run("delete nonexistent resource", func(t *testing.T) {
		withSimulator(t, func(ctx context.Context, adp *vmware.Adapter) {
			err := adp.DeleteResource(ctx, "vmware-vm-DC0-nonexistent")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "resource not found")
		})
	})
}
