package vmware

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new VMwareAdapter.
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
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
			config: &Config{
				Username:   "admin",
				Password:   "password",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "vCenterURL is required",
		},
		{
			name: "missing username",
			config: &Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Password:   "password",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name: "missing password",
			config: &Config{
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
			config: &Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "password",
				OCloudID:   "ocloud-1",
			},
			wantErr: true,
			errMsg:  "datacenter is required",
		},
		{
			name: "missing oCloudID",
			config: &Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "password",
				Datacenter: "DC1",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "invalid pool mode",
			config: &Config{
				VCenterURL: "https://vcenter.example.com/sdk",
				Username:   "admin",
				Password:   "password",
				Datacenter: "DC1",
				OCloudID:   "ocloud-1",
				PoolMode:   "invalid",
			},
			wantErr: true,
			errMsg:  "poolMode must be 'cluster' or 'pool'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adp)
				if adp != nil {
					defer adp.Close()
				}
			}
		})
	}
}

// TestMetadata tests metadata methods.
func TestMetadata(t *testing.T) {
	adp := &VMwareAdapter{
		logger: zap.NewNop(),
	}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "vmware", adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		// Without a client, it returns the default version
		assert.Equal(t, "vsphere-7.0", adp.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := adp.Capabilities()
		assert.NotEmpty(t, caps)
		assert.Len(t, caps, 6)

		// Verify specific capabilities
		assert.Contains(t, caps, adapter.CapabilityResourcePools)
		assert.Contains(t, caps, adapter.CapabilityResources)
		assert.Contains(t, caps, adapter.CapabilityResourceTypes)
		assert.Contains(t, caps, adapter.CapabilityDeploymentManagers)
		assert.Contains(t, caps, adapter.CapabilitySubscriptions)
		assert.Contains(t, caps, adapter.CapabilityHealthChecks)
	})
}

// NOTE: TestMatchesFilter and TestApplyPagination tests moved to internal/adapter/helpers_test.go
// These shared helper functions are now tested in the common adapter package.

// TestGenerateIDs tests ID generation functions.
func TestGenerateIDs(t *testing.T) {
	t.Run("generateVMProfileID", func(t *testing.T) {
		tests := []struct {
			cpus     int32
			memoryMB int64
			want     string
		}{
			{4, 8192, "vmware-profile-4cpu-8192MB"},
			{2, 4096, "vmware-profile-2cpu-4096MB"},
			{8, 16384, "vmware-profile-8cpu-16384MB"},
		}

		for _, tt := range tests {
			got := generateVMProfileID(tt.cpus, tt.memoryMB)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateVMID", func(t *testing.T) {
		tests := []struct {
			vmName        string
			clusterOrPool string
			want          string
		}{
			{"my-vm", "DC1", "vmware-vm-DC1-my-vm"},
			{"web-server", "cluster-1", "vmware-vm-cluster-1-web-server"},
		}

		for _, tt := range tests {
			got := generateVMID(tt.vmName, tt.clusterOrPool)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateClusterPoolID", func(t *testing.T) {
		tests := []struct {
			clusterName string
			want        string
		}{
			{"cluster-1", "vmware-cluster-cluster-1"},
			{"prod-cluster", "vmware-cluster-prod-cluster"},
		}

		for _, tt := range tests {
			got := generateClusterPoolID(tt.clusterName)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateResourcePoolID", func(t *testing.T) {
		tests := []struct {
			poolName    string
			clusterName string
			want        string
		}{
			{"pool-1", "cluster-1", "vmware-pool-cluster-1-pool-1"},
			{"dev-pool", "prod-cluster", "vmware-pool-prod-cluster-dev-pool"},
		}

		for _, tt := range tests {
			got := generateResourcePoolID(tt.poolName, tt.clusterName)
			assert.Equal(t, tt.want, got)
		}
	})
}

// TestSubscriptions tests subscription CRUD operations.
func TestSubscriptions(t *testing.T) {
	adp := &VMwareAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}
	ctx := context.Background()

	t.Run("CreateSubscription", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback:               "https://example.com/callback",
			ConsumerSubscriptionID: "consumer-sub-1",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.NotEmpty(t, created.SubscriptionID)
		assert.Equal(t, "https://example.com/callback", created.Callback)
		assert.Equal(t, "consumer-sub-1", created.ConsumerSubscriptionID)
	})

	t.Run("CreateSubscription with ID", func(t *testing.T) {
		sub := &adapter.Subscription{
			SubscriptionID: "my-custom-id",
			Callback:       "https://example.com/callback2",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "my-custom-id", created.SubscriptionID)
	})

	t.Run("CreateSubscription without callback", func(t *testing.T) {
		sub := &adapter.Subscription{}

		_, err := adp.CreateSubscription(ctx, sub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("GetSubscription", func(t *testing.T) {
		sub, err := adp.GetSubscription(ctx, "my-custom-id")
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, "my-custom-id", sub.SubscriptionID)
	})

	t.Run("GetSubscription not found", func(t *testing.T) {
		_, err := adp.GetSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("ListSubscriptions", func(t *testing.T) {
		subs := adp.ListSubscriptions()
		assert.Len(t, subs, 2)
	})

	t.Run("DeleteSubscription", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "my-custom-id")
		require.NoError(t, err)

		_, err = adp.GetSubscription(ctx, "my-custom-id")
		require.Error(t, err)
	})

	t.Run("DeleteSubscription not found", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
}

// TestCreateResourceType tests resource type creation.
func TestCreateResourceType(t *testing.T) {
	adp := &VMwareAdapter{
		logger: zap.NewNop(),
	}

	t.Run("creates valid resource type", func(t *testing.T) {
		rt := adp.createResourceType(4, 8192)

		assert.Equal(t, "vmware-profile-4cpu-8192MB", rt.ResourceTypeID)
		assert.Equal(t, "VM-4cpu-8GB", rt.Name)
		assert.Equal(t, "VMware", rt.Vendor)
		assert.Equal(t, "compute", rt.ResourceClass)
		assert.Equal(t, "virtual", rt.ResourceKind)
		assert.Contains(t, rt.Description, "4 vCPUs")
		assert.Contains(t, rt.Description, "8 GiB RAM")
	})
}

// TestGetDefaultResourceTypes tests default resource type generation.
func TestGetDefaultResourceTypes(t *testing.T) {
	adp := &VMwareAdapter{
		logger: zap.NewNop(),
	}

	rts := adp.getDefaultResourceTypes()

	assert.Len(t, rts, 10)

	// Verify first and last profiles
	assert.Equal(t, "vmware-profile-1cpu-1024MB", rts[0].ResourceTypeID)
	assert.Equal(t, "vmware-profile-32cpu-65536MB", rts[9].ResourceTypeID)
}

// NOTE: BenchmarkMatchesFilter moved to internal/adapter/helpers_test.go

// TestVMwareAdapter_Health tests the Health function.
func TestVMwareAdapter_Health(t *testing.T) {
	adapter, err := New(&Config{
		VCenterURL: "https://vcenter.example.com",
		Username:   "test",
		Password:   "test",
		Datacenter: "DC1",
		OCloudID:   "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires VMware credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = adapter.Health(ctx)
	if err != nil {
		t.Skip("Skipping - requires VMware access")
	}
}

// TestVMwareAdapter_ListResourcePools tests the ListResourcePools function.
func TestVMwareAdapter_ListResourcePools(t *testing.T) {
	adapter, err := New(&Config{
		VCenterURL: "https://vcenter.example.com",
		Username:   "test",
		Password:   "test",
		Datacenter: "DC1",
		OCloudID:   "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires VMware credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pools, err := adapter.ListResourcePools(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires VMware access")
	}
	assert.NotNil(t, pools)
}

// TestVMwareAdapter_ListResources tests the ListResources function.
func TestVMwareAdapter_ListResources(t *testing.T) {
	adapter, err := New(&Config{
		VCenterURL: "https://vcenter.example.com",
		Username:   "test",
		Password:   "test",
		Datacenter: "DC1",
		OCloudID:   "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires VMware credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resources, err := adapter.ListResources(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires VMware access")
	}
	assert.NotNil(t, resources)
}

// TestVMwareAdapter_ListResourceTypes tests the ListResourceTypes function.
func TestVMwareAdapter_ListResourceTypes(t *testing.T) {
	adapter, err := New(&Config{
		VCenterURL: "https://vcenter.example.com",
		Username:   "test",
		Password:   "test",
		Datacenter: "DC1",
		OCloudID:   "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires VMware credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	types, err := adapter.ListResourceTypes(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires VMware access")
	}
	assert.NotNil(t, types)
}

// TestVMwareAdapter_GetDeploymentManager tests the GetDeploymentManager function.
func TestVMwareAdapter_GetDeploymentManager(t *testing.T) {
	adapter, err := New(&Config{
		VCenterURL: "https://vcenter.example.com",
		Username:   "test",
		Password:   "test",
		Datacenter: "DC1",
		OCloudID:   "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires VMware credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm, err := adapter.GetDeploymentManager(ctx, "dm-1")
	if err != nil {
		t.Skip("Skipping - requires VMware access")
	}
	assert.NotNil(t, dm)
}
