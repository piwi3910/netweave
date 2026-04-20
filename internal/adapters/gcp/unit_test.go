package gcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/gcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newTestGCPAdapter creates a GCP adapter suitable for unit testing.
func newTestGCPAdapter() *gcp.Adapter {
	return gcp.NewTestAdapter()
}

// --- Subscription CRUD with Full Lifecycle ---

func TestGCPSubscription_FullLifecycle(t *testing.T) {
	adp := newTestGCPAdapter()
	ctx := context.Background()

	// Create
	sub, err := adp.CreateSubscription(ctx, &adapter.Subscription{
		Callback:               "https://example.com/webhook",
		ConsumerSubscriptionID: "consumer-1",
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: "pool-1",
			ResourceTypeID: "type-1",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Contains(t, sub.SubscriptionID, "gcp-sub-")
	assert.Equal(t, "https://example.com/webhook", sub.Callback)
	assert.Equal(t, "consumer-1", sub.ConsumerSubscriptionID)
	require.NotNil(t, sub.Filter)
	assert.Equal(t, "pool-1", sub.Filter.ResourcePoolID)
	assert.Equal(t, "type-1", sub.Filter.ResourceTypeID)

	subID := sub.SubscriptionID

	// Get
	fetched, err := adp.GetSubscription(ctx, subID)
	require.NoError(t, err)
	assert.Equal(t, subID, fetched.SubscriptionID)

	// Update
	updated, err := adp.UpdateSubscription(ctx, subID, &adapter.Subscription{
		Callback:               "https://example.com/updated",
		ConsumerSubscriptionID: "consumer-2",
		Filter: &adapter.SubscriptionFilter{
			ResourceTypeID: "type-2",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, subID, updated.SubscriptionID)
	assert.Equal(t, "https://example.com/updated", updated.Callback)
	assert.Equal(t, "consumer-2", updated.ConsumerSubscriptionID)
	assert.Equal(t, "type-2", updated.Filter.ResourceTypeID)

	// Verify update persisted
	fetched2, err := adp.GetSubscription(ctx, subID)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/updated", fetched2.Callback)

	// List
	subs := adp.ListSubscriptions()
	assert.Len(t, subs, 1)

	// Delete
	err = adp.DeleteSubscription(ctx, subID)
	require.NoError(t, err)

	// Verify deleted
	_, err = adp.GetSubscription(ctx, subID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscription not found")

	// List after delete
	subs = adp.ListSubscriptions()
	assert.Empty(t, subs)
}

// --- Subscription Error Cases ---

func TestGCPSubscription_ErrorCases(t *testing.T) {
	adp := newTestGCPAdapter()
	ctx := context.Background()

	t.Run("create without callback", func(t *testing.T) {
		_, err := adp.CreateSubscription(ctx, &adapter.Subscription{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("get nonexistent", func(t *testing.T) {
		_, err := adp.GetSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("update nonexistent", func(t *testing.T) {
		_, err := adp.UpdateSubscription(ctx, "nonexistent", &adapter.Subscription{
			Callback: "https://example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("update empty callback", func(t *testing.T) {
		// Create first
		_, createErr := adp.CreateSubscription(ctx, &adapter.Subscription{
			SubscriptionID: "test-update-err",
			Callback:       "https://example.com",
		})
		require.NoError(t, createErr)

		_, err := adp.UpdateSubscription(ctx, "test-update-err", &adapter.Subscription{
			Callback: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
}

// --- Subscription Concurrency ---

func TestGCPSubscription_Concurrency(t *testing.T) {
	adp := newTestGCPAdapter()
	ctx := context.Background()

	const numGoroutines = 20
	var wg sync.WaitGroup
	ids := make(chan string, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, err := adp.CreateSubscription(ctx, &adapter.Subscription{
				Callback: "https://example.com/webhook",
			})
			if err == nil {
				ids <- sub.SubscriptionID
			}
		}()
	}

	wg.Wait()
	close(ids)

	uniqueIDs := make(map[string]bool)
	for id := range ids {
		uniqueIDs[id] = true
	}
	assert.Equal(t, numGoroutines, len(uniqueIDs))
}

// --- BuildInstanceExtensions additional coverage ---

func TestBuildInstanceExtensions_NilFields(t *testing.T) {
	adp := newTestGCPAdapter()

	t.Run("instance with nil scheduling", func(t *testing.T) {
		instanceID := uint64(1)
		name := "test-vm"
		status := "RUNNING"
		inst := &computepb.Instance{
			Id:         &instanceID,
			Name:       &name,
			Status:     &status,
			Scheduling: nil,
		}

		exts := adp.TestBuildInstanceExtensions(inst, "test-vm", "us-central1-a", "n1-standard-1")
		require.NotNil(t, exts)
		assert.Equal(t, "test-vm", exts["gcp.name"])
		assert.Equal(t, "us-central1-a", exts["gcp.zone"])
		// Scheduling fields should not be present when scheduling is nil
		_, preemptExists := exts["gcp.preemptible"]
		assert.False(t, preemptExists)
	})

	t.Run("instance with empty network interfaces", func(t *testing.T) {
		instanceID := uint64(2)
		name := "net-vm"
		status := "RUNNING"
		inst := &computepb.Instance{
			Id:                &instanceID,
			Name:              &name,
			Status:            &status,
			NetworkInterfaces: []*computepb.NetworkInterface{},
		}

		exts := adp.TestBuildInstanceExtensions(inst, "net-vm", "us-central1-a", "e2-micro")
		require.NotNil(t, exts)
		// No network interfaces means no gcp.networkInterfaces key
		_, nicsExist := exts["gcp.networkInterfaces"]
		assert.False(t, nicsExist)
	})

	t.Run("instance with empty disks", func(t *testing.T) {
		instanceID := uint64(3)
		name := "disk-vm"
		status := "RUNNING"
		inst := &computepb.Instance{
			Id:     &instanceID,
			Name:   &name,
			Status: &status,
			Disks:  []*computepb.AttachedDisk{},
		}

		exts := adp.TestBuildInstanceExtensions(inst, "disk-vm", "us-central1-a", "e2-micro")
		require.NotNil(t, exts)
		_, disksExist := exts["gcp.disks"]
		assert.False(t, disksExist)
	})

	t.Run("instance with nil labels", func(t *testing.T) {
		instanceID := uint64(4)
		name := "label-vm"
		status := "RUNNING"
		inst := &computepb.Instance{
			Id:     &instanceID,
			Name:   &name,
			Status: &status,
			Labels: nil,
		}

		exts := adp.TestBuildInstanceExtensions(inst, "label-vm", "us-central1-a", "e2-micro")
		require.NotNil(t, exts)
		// Labels should be nil when instance has no labels
		assert.Nil(t, exts["gcp.labels"])
	})

	t.Run("non-Instance type returns nil", func(t *testing.T) {
		exts := adp.TestBuildInstanceExtensions("not-an-instance", "name", "zone", "type")
		assert.Nil(t, exts)
	})

	t.Run("network interface without access configs", func(t *testing.T) {
		instanceID := uint64(5)
		name := "nonat-vm"
		status := "RUNNING"
		nicName := "nic0"
		networkIP := "10.0.0.5"
		inst := &computepb.Instance{
			Id:     &instanceID,
			Name:   &name,
			Status: &status,
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					Name:          &nicName,
					NetworkIP:     &networkIP,
					AccessConfigs: nil,
				},
			},
		}

		exts := adp.TestBuildInstanceExtensions(inst, "nonat-vm", "us-central1-a", "e2-micro")
		require.NotNil(t, exts)
		assert.Equal(t, "10.0.0.5", exts["gcp.internalIP"])
		// No external IP since no access configs
		_, extIPExists := exts["gcp.externalIP"]
		assert.False(t, extIPExists)
	})

	t.Run("instance with multiple disks", func(t *testing.T) {
		instanceID := uint64(6)
		name := "multi-disk-vm"
		status := "RUNNING"
		boot := true
		notBoot := false
		disk1Name := "boot-disk"
		disk2Name := "data-disk"
		disk1Size := int64(100)
		disk2Size := int64(500)
		diskMode := "READ_WRITE"
		diskType := "PERSISTENT"

		inst := &computepb.Instance{
			Id:     &instanceID,
			Name:   &name,
			Status: &status,
			Disks: []*computepb.AttachedDisk{
				{
					DeviceName: &disk1Name,
					Boot:       &boot,
					DiskSizeGb: &disk1Size,
					Mode:       &diskMode,
					Type:       &diskType,
				},
				{
					DeviceName: &disk2Name,
					Boot:       &notBoot,
					DiskSizeGb: &disk2Size,
					Mode:       &diskMode,
					Type:       &diskType,
				},
			},
		}

		exts := adp.TestBuildInstanceExtensions(inst, "multi-disk-vm", "us-central1-a", "n1-standard-1")
		require.NotNil(t, exts)
		disks, ok := exts["gcp.disks"].([]map[string]interface{})
		require.True(t, ok)
		assert.Len(t, disks, 2)
		assert.Equal(t, "boot-disk", disks[0]["deviceName"])
		assert.Equal(t, true, disks[0]["boot"])
		assert.Equal(t, int64(100), disks[0]["sizeGB"])
		assert.Equal(t, "data-disk", disks[1]["deviceName"])
		assert.Equal(t, false, disks[1]["boot"])
		assert.Equal(t, int64(500), disks[1]["sizeGB"])
	})
}

// --- BuildGCPClientOptions coverage ---

func TestBuildGCPClientOptions(t *testing.T) {
	t.Run("credentials JSON", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID:       "test-project",
			Region:          "us-central1",
			OCloudID:        "ocloud-1",
			CredentialsJSON: []byte(`{"type": "service_account"}`),
		}

		// New will fail on actual GCP client creation, but validation passes
		_, err := gcp.New(cfg)
		require.Error(t, err) // Will fail at client creation, not validation
		assert.NotContains(t, err.Error(), "config cannot be nil")
		assert.NotContains(t, err.Error(), "projectID is required")
	})

	t.Run("credentials file", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID:       "test-project",
			Region:          "us-central1",
			OCloudID:        "ocloud-1",
			CredentialsFile: "/nonexistent/path.json",
		}

		_, err := gcp.New(cfg)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "config cannot be nil")
	})

	t.Run("default credentials", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID: "test-project",
			Region:    "us-central1",
			OCloudID:  "ocloud-1",
		}

		_, err := gcp.New(cfg)
		require.Error(t, err) // Will fail at client creation
		assert.NotContains(t, err.Error(), "config cannot be nil")
	})
}

// --- PtrHelper additional tests ---

func TestPtrHelpers_Additional(t *testing.T) {
	t.Run("PtrToString with value", func(t *testing.T) {
		s := "test"
		assert.Equal(t, "test", gcp.PtrToString(&s))
	})

	t.Run("PtrToString with empty", func(t *testing.T) {
		s := ""
		assert.Equal(t, "", gcp.PtrToString(&s))
	})

	t.Run("PtrToInt64 with negative", func(t *testing.T) {
		i := int64(-42)
		assert.Equal(t, int64(-42), gcp.PtrToInt64(&i))
	})

	t.Run("PtrToInt32 with negative", func(t *testing.T) {
		i := int32(-10)
		assert.Equal(t, int32(-10), gcp.PtrToInt32(&i))
	})

	t.Run("PtrToBool false value", func(t *testing.T) {
		b := false
		assert.Equal(t, false, gcp.PtrToBool(&b))
	})
}

// --- DetermineResourcePoolID additional coverage ---

func TestDetermineResourcePoolID_Additional(t *testing.T) {
	tests := []struct {
		name     string
		poolMode string
		zone     string
		want     string
	}{
		{
			name:     "zone mode returns zone pool ID",
			poolMode: "zone",
			zone:     "us-central1-a",
			want:     "gcp-zone-us-central1-a",
		},
		{
			name:     "ig mode falls back to zone pool ID",
			poolMode: "ig",
			zone:     "us-central1-b",
			want:     "gcp-zone-us-central1-b",
		},
		{
			name:     "empty mode returns zone pool ID",
			poolMode: "",
			zone:     "us-central1-c",
			want:     "gcp-zone-us-central1-c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := newTestGCPAdapter()
			adp.TestSetPoolMode(tt.poolMode)
			got := adp.TestDetermineResourcePoolID(tt.zone)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Validation comprehensive tests ---

func TestGCPConfig_Validation_Comprehensive(t *testing.T) {
	tests := []struct {
		name    string
		config  *gcp.Config
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
			name: "empty project ID",
			config: &gcp.Config{
				ProjectID: "",
				Region:    "us-central1",
				OCloudID:  "ocloud-1",
			},
			wantErr: true,
			errMsg:  "projectID is required",
		},
		{
			name: "empty region",
			config: &gcp.Config{
				ProjectID: "proj",
				Region:    "",
				OCloudID:  "ocloud-1",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "empty oCloudID",
			config: &gcp.Config{
				ProjectID: "proj",
				Region:    "us-central1",
				OCloudID:  "",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "invalid poolMode",
			config: &gcp.Config{
				ProjectID: "proj",
				Region:    "us-central1",
				OCloudID:  "ocloud-1",
				PoolMode:  "invalid",
			},
			wantErr: true,
			errMsg:  "poolMode must be 'zone' or 'ig'",
		},
		{
			name: "valid poolMode zone",
			config: &gcp.Config{
				ProjectID: "proj",
				Region:    "us-central1",
				OCloudID:  "ocloud-1",
				PoolMode:  "zone",
			},
			wantErr: true, // Will fail at client creation
			errMsg:  "",   // Not a validation error
		},
		{
			name: "valid poolMode ig",
			config: &gcp.Config{
				ProjectID: "proj",
				Region:    "us-central1",
				OCloudID:  "ocloud-1",
				PoolMode:  "ig",
			},
			wantErr: true,
			errMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gcp.New(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// --- ID Generation edge cases ---

func TestGenerateIDs_Additional(t *testing.T) {
	t.Run("GenerateMachineTypeID empty", func(t *testing.T) {
		assert.Equal(t, "gcp-machine-type-", gcp.GenerateMachineTypeID(""))
	})

	t.Run("GenerateInstanceID with special chars", func(t *testing.T) {
		got := gcp.GenerateInstanceID("my-vm-1", "us-east1-b")
		assert.Equal(t, "gcp-instance-us-east1-b-my-vm-1", got)
	})

	t.Run("GenerateZonePoolID", func(t *testing.T) {
		assert.Equal(t, "gcp-zone-asia-southeast1-a", gcp.GenerateZonePoolID("asia-southeast1-a"))
	})

	t.Run("GenerateIGPoolID", func(t *testing.T) {
		assert.Equal(t, "gcp-ig-us-west2-c-prod-ig", gcp.GenerateIGPoolID("prod-ig", "us-west2-c"))
	})
}

// --- ExtractZoneAndName additional tests ---

func TestExtractZoneAndName_Additional(t *testing.T) {
	adp := newTestGCPAdapter()

	t.Run("empty extensions map", func(t *testing.T) {
		r := &adapter.Resource{
			ResourceID: "gcp-instance-us-central1-a-vm",
			Extensions: map[string]interface{}{},
		}
		_, _, err := adp.TestExtractZoneAndName(r)
		require.Error(t, err)
	})

	t.Run("zone is integer type", func(t *testing.T) {
		r := &adapter.Resource{
			ResourceID: "gcp-instance-us-central1-a-vm",
			Extensions: map[string]interface{}{
				"gcp.zone": 123,
				"gcp.name": "my-vm",
			},
		}
		_, _, err := adp.TestExtractZoneAndName(r)
		require.Error(t, err)
	})

	t.Run("name is integer type", func(t *testing.T) {
		r := &adapter.Resource{
			ResourceID: "gcp-instance-us-central1-a-vm",
			Extensions: map[string]interface{}{
				"gcp.zone": "us-central1-a",
				"gcp.name": 456,
			},
		}
		_, _, err := adp.TestExtractZoneAndName(r)
		require.Error(t, err)
	})
}

// --- BuildInstanceLabels comprehensive tests ---

func TestBuildInstanceLabels_Additional(t *testing.T) {
	adp := newTestGCPAdapter()

	t.Run("all fields set", func(t *testing.T) {
		r := &adapter.Resource{
			TenantID:      "tenant-1",
			Description:   "MY VM",
			GlobalAssetID: "urn:gcp:vm:proj/zone/name",
			Extensions: map[string]interface{}{
				"gcp.labels": map[string]string{
					"custom": "label",
				},
			},
		}
		labels := adp.TestBuildInstanceLabels(r)
		assert.Equal(t, "tenant-1", labels["o2ims_io_tenant-id"])
		assert.Equal(t, "my vm", labels["name"])
		assert.Contains(t, labels["global_asset_id"], "urn_gcp_vm_proj_zone_name")
		assert.Equal(t, "label", labels["custom"])
	})

	t.Run("no fields set", func(t *testing.T) {
		r := &adapter.Resource{}
		labels := adp.TestBuildInstanceLabels(r)
		assert.Empty(t, labels)
	})

	t.Run("labels type assertion fails", func(t *testing.T) {
		r := &adapter.Resource{
			Extensions: map[string]interface{}{
				"gcp.labels": 12345,
			},
		}
		labels := adp.TestBuildInstanceLabels(r)
		assert.Empty(t, labels)
	})
}

// --- ExtractMachineTypeName additional coverage ---

func TestExtractMachineTypeName_Additional(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "full URL",
			url:  "https://compute.googleapis.com/compute/v1/projects/p/zones/z/machineTypes/n2-standard-4",
			want: "n2-standard-4",
		},
		{
			name: "relative URL",
			url:  "zones/z/machineTypes/c2-standard-8",
			want: "c2-standard-8",
		},
		{
			name: "bare name",
			url:  "e2-medium",
			want: "e2-medium",
		},
		{
			name: "empty",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcp.ExtractMachineTypeName(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- ExtractMachineFamily additional coverage ---

func TestExtractMachineFamily_Additional(t *testing.T) {
	tests := []struct {
		name        string
		machineType string
		want        string
	}{
		{name: "n2", machineType: "n2-standard-2", want: "n2"},
		{name: "c3", machineType: "c3-highcpu-4", want: "c3"},
		{name: "g2", machineType: "g2-standard-4", want: "g2"},
		{name: "t2d", machineType: "t2d-standard-1", want: "t2d"},
		{name: "empty", machineType: "", want: ""},
		{name: "no dash", machineType: "custom", want: "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcp.ExtractMachineFamily(tt.machineType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- GetResource invalid format ---

func TestGetResource_InvalidFormat_Additional(t *testing.T) {
	adp := newTestGCPAdapter()
	ctx := context.Background()

	tests := []struct {
		name string
		id   string
	}{
		{name: "no prefix", id: "invalid"},
		{name: "wrong prefix", id: "aws-instance-us-east-1-vm"},
		{name: "prefix only", id: "gcp-instance-"},
		{name: "empty", id: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adp.GetResource(ctx, tt.id)
			require.Error(t, err)
		})
	}
}

// --- CreateResource always returns error ---

func TestCreateResource_AlwaysErrors(t *testing.T) {
	adp := newTestGCPAdapter()
	ctx := context.Background()

	result, err := adp.CreateResource(ctx, &adapter.Resource{
		ResourceTypeID: "gcp-machine-type-n1-standard-1",
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "additional configuration")
}

// --- Resource Pool Operations ---

func TestResourcePoolOperations_ZoneMode_Additional(t *testing.T) {
	adp := newTestGCPAdapter()
	adp.TestSetPoolMode("zone")
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		pool, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{Name: "pool"})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "GCP-managed")
	})

	t.Run("update", func(t *testing.T) {
		pool, err := adp.UpdateResourcePool(ctx, "id", &adapter.ResourcePool{})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "GCP-managed")
	})

	t.Run("delete", func(t *testing.T) {
		err := adp.DeleteResourcePool(ctx, "id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GCP-managed")
	})
}

func TestResourcePoolOperations_IGMode_Additional(t *testing.T) {
	adp := newTestGCPAdapter()
	adp.TestSetPoolMode("ig")
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		pool, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{Name: "ig"})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "not yet implemented")
	})

	t.Run("update", func(t *testing.T) {
		pool, err := adp.UpdateResourcePool(ctx, "id", &adapter.ResourcePool{})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "not yet implemented")
	})

	t.Run("delete", func(t *testing.T) {
		err := adp.DeleteResourcePool(ctx, "id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not yet implemented")
	})
}

// --- Adapter Metadata Tests ---

func TestGCPAdapter_Name(t *testing.T) {
	adp := newTestGCPAdapter()
	assert.Equal(t, "gcp", adp.Name())
}

func TestGCPAdapter_Version(t *testing.T) {
	adp := newTestGCPAdapter()
	assert.Equal(t, "compute-v1", adp.Version())
}

func TestGCPAdapter_Capabilities(t *testing.T) {
	adp := newTestGCPAdapter()
	caps := adp.Capabilities()
	assert.Len(t, caps, 6)
	assert.Contains(t, caps, adapter.CapabilityResourcePools)
	assert.Contains(t, caps, adapter.CapabilityResources)
	assert.Contains(t, caps, adapter.CapabilityResourceTypes)
	assert.Contains(t, caps, adapter.CapabilityDeploymentManagers)
	assert.Contains(t, caps, adapter.CapabilitySubscriptions)
	assert.Contains(t, caps, adapter.CapabilityHealthChecks)
}

// --- CloseClients Tests ---

func TestExportCloseClients(t *testing.T) {
	logger := zap.NewNop()

	t.Run("no clients", func(t *testing.T) {
		gcp.ExportCloseClients(logger)
	})

	t.Run("nil clients", func(t *testing.T) {
		gcp.ExportCloseClients(logger, nil, nil)
	})

	t.Run("successful close", func(t *testing.T) {
		client := &gcp.MockCloser{Err: nil}
		gcp.ExportCloseClients(logger, client)
	})

	t.Run("close with error", func(t *testing.T) {
		client := &gcp.MockCloser{Err: fmt.Errorf("close failed")}
		gcp.ExportCloseClients(logger, client)
	})

	t.Run("mixed nil and real clients", func(t *testing.T) {
		okClient := &gcp.MockCloser{Err: nil}
		errClient := &gcp.MockCloser{Err: fmt.Errorf("error")}
		gcp.ExportCloseClients(logger, nil, okClient, errClient, nil)
	})
}

// --- MachineTypeToResourceType Tests ---

func TestExportMachineTypeToResourceType(t *testing.T) {
	adp := newTestGCPAdapter()

	t.Run("standard machine type", func(t *testing.T) {
		name := "n1-standard-4"
		cpus := int32(4)
		memMB := int32(15360)
		maxDisks := int32(128)
		maxDiskSize := int64(65536)
		isShared := false
		desc := "Standard machine type with 4 vCPUs"

		mt := &computepb.MachineType{
			Name:                         &name,
			GuestCpus:                    &cpus,
			MemoryMb:                     &memMB,
			MaximumPersistentDisks:       &maxDisks,
			MaximumPersistentDisksSizeGb: &maxDiskSize,
			IsSharedCpu:                  &isShared,
			Description:                  &desc,
		}

		rt := adp.ExportMachineTypeToResourceType(mt)
		require.NotNil(t, rt)
		assert.Equal(t, "gcp-machine-type-n1-standard-4", rt.ResourceTypeID)
		assert.Equal(t, "n1-standard-4", rt.Name)
		assert.Equal(t, "Google Cloud Platform", rt.Vendor)
		assert.Equal(t, "n1", rt.Version)
		assert.Equal(t, "compute", rt.ResourceClass)
		assert.Equal(t, "virtual", rt.ResourceKind)
		assert.Contains(t, rt.Description, "n1-standard-4")
		assert.Contains(t, rt.Description, "4 vCPUs")

		// Verify extensions
		assert.Equal(t, "n1-standard-4", rt.Extensions["gcp.machineType"])
		assert.Equal(t, "n1", rt.Extensions["gcp.family"])
		assert.Equal(t, int32(4), rt.Extensions["gcp.guestCpus"])
		assert.Equal(t, int32(15360), rt.Extensions["gcp.memoryMb"])
		assert.Equal(t, false, rt.Extensions["gcp.isSharedCpu"])
	})

	t.Run("shared CPU machine type", func(t *testing.T) {
		name := "e2-micro"
		cpus := int32(2)
		memMB := int32(1024)
		isShared := true
		desc := "Shared-core machine"

		mt := &computepb.MachineType{
			Name:        &name,
			GuestCpus:   &cpus,
			MemoryMb:    &memMB,
			IsSharedCpu: &isShared,
			Description: &desc,
		}

		rt := adp.ExportMachineTypeToResourceType(mt)
		require.NotNil(t, rt)
		assert.Equal(t, "gcp-machine-type-e2-micro", rt.ResourceTypeID)
		assert.Contains(t, rt.Description, "shared CPU")
		assert.Equal(t, true, rt.Extensions["gcp.isSharedCpu"])
	})

	t.Run("machine type with accelerators", func(t *testing.T) {
		name := "a2-highgpu-1g"
		cpus := int32(12)
		memMB := int32(85000)
		isShared := false
		accType := "nvidia-tesla-a100"
		accCount := int32(1)

		mt := &computepb.MachineType{
			Name:        &name,
			GuestCpus:   &cpus,
			MemoryMb:    &memMB,
			IsSharedCpu: &isShared,
			Accelerators: []*computepb.Accelerators{
				{
					GuestAcceleratorType:  &accType,
					GuestAcceleratorCount: &accCount,
				},
			},
		}

		rt := adp.ExportMachineTypeToResourceType(mt)
		require.NotNil(t, rt)
		accs, ok := rt.Extensions["gcp.accelerators"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, accs, 1)
		assert.Equal(t, "nvidia-tesla-a100", accs[0]["type"])
		assert.Equal(t, int32(1), accs[0]["count"])
	})

	t.Run("nil fields default to zero values", func(t *testing.T) {
		mt := &computepb.MachineType{}

		rt := adp.ExportMachineTypeToResourceType(mt)
		require.NotNil(t, rt)
		assert.Equal(t, "gcp-machine-type-", rt.ResourceTypeID)
		assert.Equal(t, int32(0), rt.Extensions["gcp.guestCpus"])
		assert.Equal(t, int32(0), rt.Extensions["gcp.memoryMb"])
	})
}

// --- InstanceToResource Tests ---

func TestExportInstanceToResource(t *testing.T) {
	adp := newTestGCPAdapter()
	adp.TestSetPoolMode("zone")

	t.Run("basic instance", func(t *testing.T) {
		instanceID := uint64(123456)
		name := "web-server-1"
		status := "RUNNING"
		machineTypeURL := "zones/us-central1-a/machineTypes/n1-standard-4"
		desc := "Production web server"

		inst := &computepb.Instance{
			Id:          &instanceID,
			Name:        &name,
			Status:      &status,
			MachineType: &machineTypeURL,
			Description: &desc,
			Labels: map[string]string{
				"o2ims_io_tenant-id": "tenant-abc",
				"env":                "prod",
			},
		}

		resource := adp.ExportInstanceToResource(inst, "us-central1-a")
		require.NotNil(t, resource)
		assert.Equal(t, "gcp-instance-us-central1-a-web-server-1", resource.ResourceID)
		assert.Equal(t, "gcp-machine-type-n1-standard-4", resource.ResourceTypeID)
		assert.Equal(t, "gcp-zone-us-central1-a", resource.ResourcePoolID)
		assert.Equal(t, "tenant-abc", resource.TenantID)
		assert.Equal(t, "Production web server", resource.Description)
		assert.Contains(t, resource.GlobalAssetID, "us-central1-a")
		assert.Contains(t, resource.GlobalAssetID, "web-server-1")
		assert.NotNil(t, resource.Extensions)
		assert.Equal(t, "web-server-1", resource.Extensions["gcp.name"])
		assert.Equal(t, "us-central1-a", resource.Extensions["gcp.zone"])
	})

	t.Run("instance without description uses name", func(t *testing.T) {
		instanceID := uint64(789)
		name := "auto-named"
		status := "RUNNING"
		machineTypeURL := "e2-micro"

		inst := &computepb.Instance{
			Id:          &instanceID,
			Name:        &name,
			Status:      &status,
			MachineType: &machineTypeURL,
		}

		resource := adp.ExportInstanceToResource(inst, "us-east1-b")
		require.NotNil(t, resource)
		assert.Equal(t, "auto-named", resource.Description)
	})

	t.Run("instance without labels", func(t *testing.T) {
		instanceID := uint64(456)
		name := "no-labels"
		status := "STOPPED"
		machineTypeURL := "n1-standard-1"

		inst := &computepb.Instance{
			Id:          &instanceID,
			Name:        &name,
			Status:      &status,
			MachineType: &machineTypeURL,
		}

		resource := adp.ExportInstanceToResource(inst, "us-central1-a")
		require.NotNil(t, resource)
		assert.Empty(t, resource.TenantID)
	})

	t.Run("instance with network interfaces", func(t *testing.T) {
		instanceID := uint64(321)
		name := "net-vm"
		status := "RUNNING"
		machineTypeURL := "e2-small"
		nicName := "nic0"
		networkIP := "10.0.0.5"
		natIP := "35.192.1.1"

		inst := &computepb.Instance{
			Id:          &instanceID,
			Name:        &name,
			Status:      &status,
			MachineType: &machineTypeURL,
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					Name:      &nicName,
					NetworkIP: &networkIP,
					AccessConfigs: []*computepb.AccessConfig{
						{NatIP: &natIP},
					},
				},
			},
		}

		resource := adp.ExportInstanceToResource(inst, "us-central1-a")
		require.NotNil(t, resource)
		assert.Equal(t, "10.0.0.5", resource.Extensions["gcp.internalIP"])
		assert.Equal(t, "35.192.1.1", resource.Extensions["gcp.externalIP"])
	})
}

// --- ValidateGCPConfig Tests ---

func TestExportValidateGCPConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *gcp.Config
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
			name:    "empty projectID",
			cfg:     &gcp.Config{Region: "us-central1", OCloudID: "oc1"},
			wantErr: true,
			errMsg:  "projectID is required",
		},
		{
			name:    "empty region",
			cfg:     &gcp.Config{ProjectID: "proj", OCloudID: "oc1"},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name:    "empty oCloudID",
			cfg:     &gcp.Config{ProjectID: "proj", Region: "us-central1"},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name:    "invalid poolMode",
			cfg:     &gcp.Config{ProjectID: "proj", Region: "us-central1", OCloudID: "oc1", PoolMode: "bad"},
			wantErr: true,
			errMsg:  "poolMode must be 'zone' or 'ig'",
		},
		{
			name: "valid config",
			cfg:  &gcp.Config{ProjectID: "proj", Region: "us-central1", OCloudID: "oc1"},
		},
		{
			name: "valid with zone poolMode",
			cfg:  &gcp.Config{ProjectID: "proj", Region: "us-central1", OCloudID: "oc1", PoolMode: "zone"},
		},
		{
			name: "valid with ig poolMode",
			cfg:  &gcp.Config{ProjectID: "proj", Region: "us-central1", OCloudID: "oc1", PoolMode: "ig"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gcp.ExportValidateGCPConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// --- ApplyGCPDefaults Tests ---

func TestExportApplyGCPDefaults(t *testing.T) {
	t.Run("all defaults applied", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID: "proj",
			Region:    "us-central1",
		}
		dmID, poolMode := gcp.ExportApplyGCPDefaults(cfg)
		assert.Equal(t, "ocloud-gcp-us-central1", dmID)
		assert.Equal(t, "zone", poolMode)
	})

	t.Run("custom values preserved", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID:           "proj",
			Region:              "europe-west1",
			DeploymentManagerID: "custom-dm",
			PoolMode:            "ig",
		}
		dmID, poolMode := gcp.ExportApplyGCPDefaults(cfg)
		assert.Equal(t, "custom-dm", dmID)
		assert.Equal(t, "ig", poolMode)
	})
}

// --- InitializeGCPLogger Tests ---

func TestExportInitializeGCPLogger(t *testing.T) {
	t.Run("nil logger creates new logger", func(t *testing.T) {
		logger, err := gcp.ExportInitializeGCPLogger(nil)
		require.NoError(t, err)
		assert.NotNil(t, logger)
	})

	t.Run("existing logger is returned as-is", func(t *testing.T) {
		existing := zap.NewNop()
		logger, err := gcp.ExportInitializeGCPLogger(existing)
		require.NoError(t, err)
		assert.Equal(t, existing, logger)
	})
}

// --- BuildGCPClientOptions Tests ---

func TestExportBuildGCPClientOptions(t *testing.T) {
	logger := zap.NewNop()

	t.Run("default credentials", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID: "proj",
			Region:    "us-central1",
		}
		count := gcp.ExportBuildGCPClientOptions(cfg, logger)
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("credentials JSON", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID:       "proj",
			Region:          "us-central1",
			CredentialsJSON: []byte(`{"type": "service_account"}`),
		}
		count := gcp.ExportBuildGCPClientOptions(cfg, logger)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("credentials file", func(t *testing.T) {
		cfg := &gcp.Config{
			ProjectID:       "proj",
			Region:          "us-central1",
			CredentialsFile: "/some/path.json",
		}
		count := gcp.ExportBuildGCPClientOptions(cfg, logger)
		assert.GreaterOrEqual(t, count, 1)
	})
}

// --- AddInstanceNetworkInterfaces Tests ---

func TestExportAddInstanceNetworkInterfaces(t *testing.T) {
	t.Run("empty NICs", func(t *testing.T) {
		ext := make(map[string]interface{})
		gcp.ExportAddInstanceNetworkInterfaces(ext, nil)
		_, exists := ext["gcp.networkInterfaces"]
		assert.False(t, exists)
	})

	t.Run("single NIC with external IP", func(t *testing.T) {
		ext := make(map[string]interface{})
		nicName := "nic0"
		network := "projects/p/global/networks/default"
		subnet := "projects/p/regions/r/subnetworks/default"
		networkIP := "10.128.0.2"
		natIP := "35.192.1.1"

		nics := []*computepb.NetworkInterface{
			{
				Name:       &nicName,
				Network:    &network,
				Subnetwork: &subnet,
				NetworkIP:  &networkIP,
				AccessConfigs: []*computepb.AccessConfig{
					{NatIP: &natIP},
				},
			},
		}

		gcp.ExportAddInstanceNetworkInterfaces(ext, nics)

		nicList, ok := ext["gcp.networkInterfaces"].([]map[string]interface{})
		require.True(t, ok)
		assert.Len(t, nicList, 1)
		assert.Equal(t, "nic0", nicList[0]["name"])
		assert.Equal(t, "10.128.0.2", nicList[0]["internalIP"])
		assert.Equal(t, "35.192.1.1", nicList[0]["externalIP"])
		assert.Equal(t, "10.128.0.2", ext["gcp.internalIP"])
		assert.Equal(t, "35.192.1.1", ext["gcp.externalIP"])
	})

	t.Run("NIC without access configs", func(t *testing.T) {
		ext := make(map[string]interface{})
		nicName := "nic0"
		networkIP := "10.0.0.3"

		nics := []*computepb.NetworkInterface{
			{
				Name:      &nicName,
				NetworkIP: &networkIP,
			},
		}

		gcp.ExportAddInstanceNetworkInterfaces(ext, nics)
		assert.Equal(t, "10.0.0.3", ext["gcp.internalIP"])
		_, hasExtIP := ext["gcp.externalIP"]
		assert.False(t, hasExtIP)
	})
}

// --- AddInstanceDisks Tests ---

func TestExportAddInstanceDisks(t *testing.T) {
	t.Run("empty disks", func(t *testing.T) {
		ext := make(map[string]interface{})
		gcp.ExportAddInstanceDisks(ext, nil)
		_, exists := ext["gcp.disks"]
		assert.False(t, exists)
	})

	t.Run("single boot disk", func(t *testing.T) {
		ext := make(map[string]interface{})
		devName := "boot-disk"
		source := "projects/p/zones/z/disks/boot-disk"
		boot := true
		mode := "READ_WRITE"
		sizeGB := int64(100)
		diskType := "PERSISTENT"

		disks := []*computepb.AttachedDisk{
			{
				DeviceName: &devName,
				Source:     &source,
				Boot:       &boot,
				Mode:       &mode,
				DiskSizeGb: &sizeGB,
				Type:       &diskType,
			},
		}

		gcp.ExportAddInstanceDisks(ext, disks)

		diskList, ok := ext["gcp.disks"].([]map[string]interface{})
		require.True(t, ok)
		assert.Len(t, diskList, 1)
		assert.Equal(t, "boot-disk", diskList[0]["deviceName"])
		assert.Equal(t, true, diskList[0]["boot"])
		assert.Equal(t, int64(100), diskList[0]["sizeGB"])
	})
}

// --- AddInstanceSchedulingInfo Tests ---

func TestExportAddInstanceSchedulingInfo(t *testing.T) {
	t.Run("nil scheduling", func(t *testing.T) {
		ext := make(map[string]interface{})
		gcp.ExportAddInstanceSchedulingInfo(ext, nil)
		_, exists := ext["gcp.preemptible"]
		assert.False(t, exists)
	})

	t.Run("preemptible instance", func(t *testing.T) {
		ext := make(map[string]interface{})
		preemptible := true
		autoRestart := false

		scheduling := &computepb.Scheduling{
			Preemptible:      &preemptible,
			AutomaticRestart: &autoRestart,
		}

		gcp.ExportAddInstanceSchedulingInfo(ext, scheduling)
		assert.Equal(t, true, ext["gcp.preemptible"])
		assert.Equal(t, false, ext["gcp.automaticRestart"])
	})

	t.Run("standard instance", func(t *testing.T) {
		ext := make(map[string]interface{})
		preemptible := false
		autoRestart := true

		scheduling := &computepb.Scheduling{
			Preemptible:      &preemptible,
			AutomaticRestart: &autoRestart,
		}

		gcp.ExportAddInstanceSchedulingInfo(ext, scheduling)
		assert.Equal(t, false, ext["gcp.preemptible"])
		assert.Equal(t, true, ext["gcp.automaticRestart"])
	})
}

// --- BuildInstanceExtensions Tests ---

func TestExportBuildInstanceExtensions(t *testing.T) {
	instanceID := uint64(42)
	name := "test-vm"
	status := "RUNNING"
	selfLink := "https://compute.googleapis.com/compute/v1/projects/p/zones/z/instances/test-vm"
	cpuPlatform := "Intel Broadwell"
	creationTS := "2024-01-01T00:00:00Z"

	inst := &computepb.Instance{
		Id:                &instanceID,
		Name:              &name,
		Status:            &status,
		SelfLink:          &selfLink,
		CpuPlatform:       &cpuPlatform,
		CreationTimestamp: &creationTS,
		Labels:            map[string]string{"env": "test"},
	}

	ext := gcp.ExportBuildInstanceExtensions(inst, "test-vm", "us-central1-a", "n1-standard-1")
	require.NotNil(t, ext)

	assert.Equal(t, uint64(42), ext["gcp.id"])
	assert.Equal(t, "test-vm", ext["gcp.name"])
	assert.Equal(t, "us-central1-a", ext["gcp.zone"])
	assert.Equal(t, "n1-standard-1", ext["gcp.machineType"])
	assert.Equal(t, "RUNNING", ext["gcp.status"])
	assert.Equal(t, selfLink, ext["gcp.selfLink"])
	assert.Equal(t, "Intel Broadwell", ext["gcp.cpuPlatform"])
	assert.Equal(t, "2024-01-01T00:00:00Z", ext["gcp.creationTimestamp"])
	assert.Equal(t, map[string]string{"env": "test"}, ext["gcp.labels"])
}

// --- Fake GCP API Tests ---
// These tests use httptest servers that simulate GCP Compute REST API responses,
// enabling testing of functions that require real GCP service clients.

func newFakeGCPServer(t *testing.T) (*gcp.Adapter, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()

	// Zones list
	mux.HandleFunc("/compute/v1/projects/test-project/zones", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{"name": "us-central1-a", "status": "UP", "selfLink": "zones/us-central1-a"},
				{"name": "us-central1-b", "status": "UP", "selfLink": "zones/us-central1-b"},
				{"name": "us-east1-a", "status": "UP", "selfLink": "zones/us-east1-a"},
			},
		})
	})

	// Instances list per zone
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"id":          "123456",
					"name":        "test-vm-1",
					"status":      "RUNNING",
					"zone":        "projects/test-project/zones/us-central1-a",
					"machineType": "projects/test-project/zones/us-central1-a/machineTypes/n1-standard-1",
					"selfLink":    "projects/test-project/zones/us-central1-a/instances/test-vm-1",
				},
			},
		})
	})
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-b/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{},
		})
	})

	// Regions get
	mux.HandleFunc("/compute/v1/projects/test-project/regions/us-central1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":   "us-central1",
			"status": "UP",
			"zones": []string{
				"https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-a",
				"https://www.googleapis.com/compute/v1/projects/test-project/zones/us-central1-b",
			},
		})
	})

	// Machine types per zone
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/machineTypes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"name":        "n1-standard-1",
					"guestCpus":   1,
					"memoryMb":    3840,
					"selfLink":    "projects/test-project/zones/us-central1-a/machineTypes/n1-standard-1",
					"isSharedCpu": false,
				},
				{
					"name":        "n1-standard-2",
					"guestCpus":   2,
					"memoryMb":    7680,
					"selfLink":    "projects/test-project/zones/us-central1-a/machineTypes/n1-standard-2",
					"isSharedCpu": false,
				},
			},
		})
	})

	// Machine type get
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/machineTypes/n1-standard-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":        "n1-standard-1",
			"guestCpus":   1,
			"memoryMb":    3840,
			"selfLink":    "projects/test-project/zones/us-central1-a/machineTypes/n1-standard-1",
			"isSharedCpu": false,
		})
	})

	// Instance get/delete
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/instances/test-vm-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":          "operation-delete-vm1",
				"status":        "DONE",
				"operationType": "delete",
				"selfLink":      "projects/test-project/zones/us-central1-a/operations/operation-delete-vm1",
				"targetLink":    "projects/test-project/zones/us-central1-a/instances/test-vm-1",
				"zone":          "projects/test-project/zones/us-central1-a",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":               "123456",
			"name":             "test-vm-1",
			"status":           "RUNNING",
			"zone":             "projects/test-project/zones/us-central1-a",
			"machineType":      "projects/test-project/zones/us-central1-a/machineTypes/n1-standard-1",
			"selfLink":         "projects/test-project/zones/us-central1-a/instances/test-vm-1",
			"labelFingerprint": "abcdef123456",
		})
	})

	// Instance setLabels
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/instances/test-vm-1/setLabels", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":          "operation-setlabels-vm1",
			"status":        "DONE",
			"operationType": "setLabels",
			"selfLink":      "projects/test-project/zones/us-central1-a/operations/operation-setlabels-vm1",
			"targetLink":    "projects/test-project/zones/us-central1-a/instances/test-vm-1",
			"zone":          "projects/test-project/zones/us-central1-a",
		})
	})

	// Zone operations get (for op.Wait polling)
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/operations/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "DONE",
		})
	})

	// Instance groups list
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/instanceGroups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{},
		})
	})
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-b/instanceGroups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{},
		})
	})

	server := httptest.NewServer(mux)

	adp, err := gcp.ExportNewFakeGCPAdapter(context.Background(), server.URL)
	require.NoError(t, err)

	return adp, server
}

func TestGCPFakeAPI_Health(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	err := adp.Health(context.Background())
	require.NoError(t, err)
}

func TestGCPFakeAPI_ListResources(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("list all resources", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, resources)

		for _, r := range resources {
			assert.NotEmpty(t, r.ResourceID)
			assert.Contains(t, r.ResourceID, "gcp-instance-")
			assert.NotEmpty(t, r.ResourceTypeID)
		}
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 1}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resources), 1)
	})
}

func TestGCPFakeAPI_GetResource(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("get existing resource", func(t *testing.T) {
		resource, err := adp.GetResource(ctx, "gcp-instance-us-central1-a-test-vm-1")
		require.NoError(t, err)
		require.NotNil(t, resource)
		assert.Contains(t, resource.ResourceID, "gcp-instance-")
	})
}

func TestGCPFakeAPI_ListResourcePools_ZoneMode(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("list zone pools", func(t *testing.T) {
		pools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, pools)

		for _, p := range pools {
			assert.NotEmpty(t, p.ResourcePoolID)
			assert.Contains(t, p.ResourcePoolID, "gcp-zone-")
		}
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 1}
		pools, err := adp.ListResourcePools(ctx, filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(pools), 1)
	})
}

func TestGCPFakeAPI_GetDeploymentManager(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("get by configured ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "test-dm-gcp")
		require.NoError(t, err)
		require.NotNil(t, dm)
		assert.Equal(t, "test-dm-gcp", dm.DeploymentManagerID)
		assert.Contains(t, dm.Name, "GCP")
	})

	t.Run("get by default alias", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "default")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("get by empty ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("nonexistent DM", func(t *testing.T) {
		_, err := adp.GetDeploymentManager(ctx, "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, adapter.ErrDeploymentManagerNotFound)
	})
}

func TestGCPFakeAPI_ListDeploymentManagers(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	dms, err := adp.ListDeploymentManagers(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, dms, 1)
}

func TestGCPFakeAPI_ListResourceTypes(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("list resource types", func(t *testing.T) {
		rts, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, rts)

		for _, rt := range rts {
			assert.NotEmpty(t, rt.ResourceTypeID)
			assert.Equal(t, "Google Cloud Platform", rt.Vendor)
			assert.Equal(t, "compute", rt.ResourceClass)
		}
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 1}
		rts, err := adp.ListResourceTypes(ctx, filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(rts), 1)
	})
}

func TestGCPFakeAPI_GetResourceType(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("get existing resource type", func(t *testing.T) {
		rt, err := adp.GetResourceType(ctx, "gcp-machine-type-n1-standard-1")
		require.NoError(t, err)
		require.NotNil(t, rt)
		assert.Equal(t, "gcp-machine-type-n1-standard-1", rt.ResourceTypeID)
		assert.Equal(t, "n1-standard-1", rt.Name)
		assert.Equal(t, "Google Cloud Platform", rt.Vendor)
		assert.Equal(t, "compute", rt.ResourceClass)
	})

	t.Run("get resource type not found returns error", func(t *testing.T) {
		_, err := adp.GetResourceType(ctx, "gcp-machine-type-nonexistent-type")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource type not found")
	})
}

func TestGCPFakeAPI_GetResourcePool(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("get existing zone pool", func(t *testing.T) {
		pool, err := adp.GetResourcePool(ctx, "gcp-zone-us-central1-a")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "gcp-zone-us-central1-a", pool.ResourcePoolID)
		assert.Equal(t, "us-central1-a", pool.Name)
		assert.Equal(t, "test-ocloud", pool.OCloudID)
		assert.NotNil(t, pool.Extensions)
		assert.Equal(t, "us-central1-a", pool.Extensions["gcp.zone"])
	})

	t.Run("get nonexistent zone pool", func(t *testing.T) {
		_, err := adp.GetResourcePool(ctx, "gcp-zone-nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource pool not found")
	})
}

func TestGCPFakeAPI_DeleteResource(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("delete invalid format", func(t *testing.T) {
		err := adp.DeleteResource(ctx, "invalid-format")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource ID")
	})

	t.Run("delete existing resource", func(t *testing.T) {
		err := adp.DeleteResource(ctx, "gcp-instance-us-central1-a-test-vm-1")
		require.NoError(t, err)
	})
}

func TestGCPFakeAPI_UpdateResource(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("update nonexistent resource", func(t *testing.T) {
		_, err := adp.UpdateResource(ctx, "gcp-instance-us-central1-a-nonexistent", &adapter.Resource{
			Description: "updated",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource not found")
	})

	t.Run("update existing resource with labels", func(t *testing.T) {
		updated, err := adp.UpdateResource(ctx, "gcp-instance-us-central1-a-test-vm-1", &adapter.Resource{
			TenantID:    "tenant-new",
			Description: "Updated description",
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Contains(t, updated.ResourceID, "gcp-instance-")
	})

	t.Run("update existing resource without labels", func(t *testing.T) {
		// Empty update - no labels set, should still succeed
		updated, err := adp.UpdateResource(ctx, "gcp-instance-us-central1-a-test-vm-1", &adapter.Resource{})
		require.NoError(t, err)
		require.NotNil(t, updated)
	})
}

func TestGCPFakeAPI_ListResourcePools_IGMode(t *testing.T) {
	// Create a fake server with instance group data
	mux := http.NewServeMux()

	// Zones list
	mux.HandleFunc("/compute/v1/projects/test-project/zones", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{"name": "us-central1-a", "status": "UP"},
				{"name": "us-central1-b", "status": "UP"},
			},
		})
	})

	// Instance groups in zone a
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-a/instanceGroups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"name":        "prod-ig-1",
					"description": "Production instance group",
					"selfLink":    "projects/test-project/zones/us-central1-a/instanceGroups/prod-ig-1",
					"size":        3,
				},
			},
		})
	})

	// Instance groups in zone b (empty)
	mux.HandleFunc("/compute/v1/projects/test-project/zones/us-central1-b/instanceGroups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{},
		})
	})

	// Regions (needed for health/dm)
	mux.HandleFunc("/compute/v1/projects/test-project/regions/us-central1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":   "us-central1",
			"status": "UP",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	adp, err := gcp.ExportNewFakeGCPAdapter(context.Background(), server.URL)
	require.NoError(t, err)
	defer adp.ExportCloseAdapter()

	// Switch to IG mode
	adp.TestSetPoolMode("ig")

	ctx := context.Background()

	t.Run("list IG pools", func(t *testing.T) {
		pools, listErr := adp.ListResourcePools(ctx, nil)
		require.NoError(t, listErr)
		assert.Len(t, pools, 1)
		assert.Equal(t, "gcp-ig-us-central1-a-prod-ig-1", pools[0].ResourcePoolID)
		assert.Equal(t, "prod-ig-1", pools[0].Name)
		assert.Equal(t, "us-central1-a", pools[0].Location)
	})

	t.Run("get existing IG pool", func(t *testing.T) {
		pool, getErr := adp.GetResourcePool(ctx, "gcp-ig-us-central1-a-prod-ig-1")
		require.NoError(t, getErr)
		require.NotNil(t, pool)
		assert.Equal(t, "prod-ig-1", pool.Name)
	})

	t.Run("get nonexistent IG pool", func(t *testing.T) {
		_, getErr := adp.GetResourcePool(ctx, "gcp-ig-nonexistent")
		require.Error(t, getErr)
		assert.Contains(t, getErr.Error(), "resource pool not found")
	})

	t.Run("list IG pools with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 0}
		pools, listErr := adp.ListResourcePools(ctx, filter)
		require.NoError(t, listErr)
		assert.NotNil(t, pools)
	})
}

func TestGCPFakeAPI_ListResources_WithFilter(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()
	defer adp.ExportCloseAdapter()

	ctx := context.Background()

	t.Run("list with resource pool filter", func(t *testing.T) {
		filter := &adapter.Filter{
			ResourcePoolID: "gcp-zone-us-central1-a",
		}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		for _, r := range resources {
			assert.Equal(t, "gcp-zone-us-central1-a", r.ResourcePoolID)
		}
	})

	t.Run("list with resource type filter", func(t *testing.T) {
		filter := &adapter.Filter{
			ResourceTypeID: "gcp-machine-type-n1-standard-1",
		}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		for _, r := range resources {
			assert.Equal(t, "gcp-machine-type-n1-standard-1", r.ResourceTypeID)
		}
	})

	t.Run("list with offset", func(t *testing.T) {
		filter := &adapter.Filter{Offset: 100}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.Empty(t, resources)
	})
}

func TestGCPFakeAPI_Close(t *testing.T) {
	adp, server := newFakeGCPServer(t)
	defer server.Close()

	// Add subscriptions
	_, err := adp.CreateSubscription(context.Background(), &adapter.Subscription{
		Callback: "https://example.com/notify",
	})
	require.NoError(t, err)

	err = adp.Close()
	require.NoError(t, err)

	// Subscriptions should be cleared
	subs := adp.ListSubscriptions()
	assert.Empty(t, subs)
}
