package gcp_test

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/gcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew_Validation tests config validation without requiring GCP credentials.
func TestNew_Validation(t *testing.T) {
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
			name: "missing projectID",
			config: &gcp.Config{
				Region:   "us-central1",
				OCloudID: "ocloud-1",
			},
			wantErr: true,
			errMsg:  "projectID is required",
		},
		{
			name: "missing region",
			config: &gcp.Config{
				ProjectID: "my-project",
				OCloudID:  "ocloud-1",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "missing oCloudID",
			config: &gcp.Config{
				ProjectID: "my-project",
				Region:    "us-central1",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "invalid pool mode",
			config: &gcp.Config{
				ProjectID: "my-project",
				Region:    "us-central1",
				OCloudID:  "ocloud-1",
				PoolMode:  "invalid",
			},
			wantErr: true,
			errMsg:  "poolMode must be 'zone' or 'ig'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := gcp.New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adp)
				if adp != nil {
					defer func() { _ = adp.Close() }()
				}
			}
		})
	}
}

// TestMetadata tests metadata methods.
func TestMetadata(t *testing.T) {
	adp := gcp.NewTestAdapter()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "gcp", adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.NotEmpty(t, adp.Version())
		assert.Equal(t, "compute-v1", adp.Version())
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
	t.Run("gcp.GenerateMachineTypeID", func(t *testing.T) {
		tests := []struct {
			machineType string
			want        string
		}{
			{"n1-standard-1", "gcp-machine-type-n1-standard-1"},
			{"e2-micro", "gcp-machine-type-e2-micro"},
			{"", "gcp-machine-type-"},
		}

		for _, tt := range tests {
			got := gcp.GenerateMachineTypeID(tt.machineType)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("gcp.GenerateInstanceID", func(t *testing.T) {
		tests := []struct {
			instanceName string
			zone         string
			want         string
		}{
			{"my-instance", "us-central1-a", "gcp-instance-us-central1-a-my-instance"},
			{"vm1", "europe-west1-b", "gcp-instance-europe-west1-b-vm1"},
		}

		for _, tt := range tests {
			got := gcp.GenerateInstanceID(tt.instanceName, tt.zone)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("gcp.GenerateZonePoolID", func(t *testing.T) {
		tests := []struct {
			zone string
			want string
		}{
			{"us-central1-a", "gcp-zone-us-central1-a"},
			{"europe-west1-b", "gcp-zone-europe-west1-b"},
		}

		for _, tt := range tests {
			got := gcp.GenerateZonePoolID(tt.zone)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateIGPoolID", func(t *testing.T) {
		tests := []struct {
			igName string
			zone   string
			want   string
		}{
			{"my-ig", "us-central1-a", "gcp-ig-us-central1-a-my-ig"},
			{"prod-ig", "europe-west1-b", "gcp-ig-europe-west1-b-prod-ig"},
		}

		for _, tt := range tests {
			got := gcp.GenerateIGPoolID(tt.igName, tt.zone)
			assert.Equal(t, tt.want, got)
		}
	})
}

// TestExtractMachineFamily tests machine family extraction.
func TestExtractMachineFamily(t *testing.T) {
	tests := []struct {
		machineType string
		want        string
	}{
		{"n1-standard-1", "n1"},
		{"e2-micro", "e2"},
		{"c2-standard-4", "c2"},
		{"m1-megamem-96", "m1"},
		{"a2-highgpu-1g", "a2"},
	}

	for _, tt := range tests {
		t.Run(tt.machineType, func(t *testing.T) {
			got := gcp.ExtractMachineFamily(tt.machineType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractMachineTypeName tests machine type name extraction from URL.
func TestExtractMachineTypeName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{
			url:  "zones/us-central1-a/machineTypes/n1-standard-1",
			want: "n1-standard-1",
		},
		{
			url: "https://compute.googleapis.com/compute/v1/projects/my-project/zones/" +
				"us-central1-a/machineTypes/e2-micro",
			want: "e2-micro",
		},
		{
			url:  "n1-standard-1",
			want: "n1-standard-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := gcp.ExtractMachineTypeName(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractZoneName tests zone name extraction from URL.
func TestExtractZoneName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{
			url:  "https://compute.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a",
			want: "us-central1-a",
		},
		{
			url:  "zones/europe-west1-b",
			want: "europe-west1-b",
		},
		{
			url:  "us-central1-a",
			want: "us-central1-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := gcp.ExtractZoneName(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSubscriptions tests subscription CRUD operations.
func TestSubscriptions(t *testing.T) {
	adp := gcp.NewTestAdapter()
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

// TestPtrHelpers tests pointer helper functions.
func TestPtrHelpers(t *testing.T) {
	t.Run("ptrToString", func(t *testing.T) {
		s := "hello"
		assert.Equal(t, "hello", gcp.PtrToString(&s))
		assert.Equal(t, "", gcp.PtrToString(nil))
	})

	t.Run("ptrToInt64", func(t *testing.T) {
		i := int64(42)
		assert.Equal(t, int64(42), gcp.PtrToInt64(&i))
		assert.Equal(t, int64(0), gcp.PtrToInt64(nil))
	})

	t.Run("ptrToInt32", func(t *testing.T) {
		i := int32(42)
		assert.Equal(t, int32(42), gcp.PtrToInt32(&i))
		assert.Equal(t, int32(0), gcp.PtrToInt32(nil))
	})

	t.Run("ptrToBool", func(t *testing.T) {
		b := true
		assert.Equal(t, true, gcp.PtrToBool(&b))
		assert.Equal(t, false, gcp.PtrToBool(nil))
	})
}

// NOTE: BenchmarkMatchesFilter moved to internal/adapter/helpers_test.go

// TestGCPAdapter_Health tests the Health function.
func TestGCPAdapter_Health(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adp, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = adp.Health(ctx)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
}

// TestGCPAdapter_ListResourcePools tests the ListResourcePools function.
func TestGCPAdapter_ListResourcePools(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adp, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
		PoolMode:  "zone",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pools, err := adp.ListResourcePools(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, pools)
}

// TestGCPAdapter_ListResources tests the ListResources function.
func TestGCPAdapter_ListResources(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adp, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resources, err := adp.ListResources(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, resources)
}

// TestGCPAdapter_ListResourceTypes tests the ListResourceTypes function.
func TestGCPAdapter_ListResourceTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adp, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	types, err := adp.ListResourceTypes(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, types)
}

// TestGCPAdapter_GetDeploymentManager tests the GetDeploymentManager function.
func TestGCPAdapter_GetDeploymentManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adp, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm, err := adp.GetDeploymentManager(ctx, "dm-1")
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, dm)

	t.Run("default alias returns configured DM", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "default")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("empty string alias returns configured DM", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})
}

// TestBuildInstanceLabels tests label building logic for GCP instances.
func TestBuildInstanceLabels(t *testing.T) {
	adp := gcp.NewTestAdapter()

	tests := []struct {
		name     string
		resource *adapter.Resource
		want     map[string]string
	}{
		{
			name: "all fields populated",
			resource: &adapter.Resource{
				TenantID:      "tenant-123",
				Description:   "Test Instance",
				GlobalAssetID: "urn:gcp:compute:project:us-central1-a:myvm",
				Extensions: map[string]interface{}{
					"gcp.labels": map[string]string{
						"environment": "production",
						"team":        "platform",
					},
				},
			},
			want: map[string]string{
				"o2ims_io_tenant-id": "tenant-123",
				"name":               "test instance",
				"global_asset_id":    "urn_gcp_compute_project_us-central1-a_myvm",
				"environment":        "production",
				"team":               "platform",
			},
		},
		{
			name: "minimal fields",
			resource: &adapter.Resource{
				TenantID: "tenant-456",
			},
			want: map[string]string{
				"o2ims_io_tenant-id": "tenant-456",
			},
		},
		{
			name:     "empty resource",
			resource: &adapter.Resource{},
			want:     map[string]string{},
		},
		{
			name: "custom labels only",
			resource: &adapter.Resource{
				Extensions: map[string]interface{}{
					"gcp.labels": map[string]string{
						"custom": "value",
					},
				},
			},
			want: map[string]string{
				"custom": "value",
			},
		},
		{
			name: "description with mixed case",
			resource: &adapter.Resource{
				Description: "My-Test-VM",
			},
			want: map[string]string{
				"name": "my-test-vm",
			},
		},
		{
			name: "global asset ID with special chars",
			resource: &adapter.Resource{
				GlobalAssetID: "urn:gcp:compute:my-project/region/zone:instance",
			},
			want: map[string]string{
				"global_asset_id": "urn_gcp_compute_my-project_region_zone_instance",
			},
		},
		{
			name: "wrong type for custom labels",
			resource: &adapter.Resource{
				Extensions: map[string]interface{}{
					"gcp.labels": "not a map",
				},
			},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildInstanceLabels(tt.resource)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDetermineResourcePoolID tests resource pool ID determination.
func TestDetermineResourcePoolID(t *testing.T) {
	tests := []struct {
		name     string
		poolMode string
		zone     string
		want     string
	}{
		{
			name:     "zone mode",
			poolMode: "zone",
			zone:     "us-central1-a",
			want:     "gcp-zone-us-central1-a",
		},
		{
			name:     "ig mode fallback",
			poolMode: "ig",
			zone:     "europe-west1-b",
			want:     "gcp-zone-europe-west1-b",
		},
		{
			name:     "empty mode defaults to zone",
			poolMode: "",
			zone:     "asia-east1-c",
			want:     "gcp-zone-asia-east1-c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := gcp.NewTestAdapter()
			adp.TestSetPoolMode(tt.poolMode)
			got := adp.TestDetermineResourcePoolID(tt.zone)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildInstanceExtensions tests GCP instance extensions building.
func TestBuildInstanceExtensions(t *testing.T) {
	adp := gcp.NewTestAdapter()

	instName := "test-vm"
	instName2 := "minimal-vm"
	status := "RUNNING"
	selfLink := "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instances/test-vm"
	cpuPlatform := "Intel Haswell"
	creationTS := "2024-01-15T10:00:00Z"
	instanceID := uint64(1234567890)
	preemptible := true
	autoRestart := false

	networkIP := "10.0.0.5"
	natIP := "34.123.45.67"
	nicName := "nic0"
	network := "default"
	subnetwork := "default-us-central1"

	diskSource := "projects/my-project/zones/us-central1-a/disks/boot-disk"
	diskDevice := "boot-disk"
	diskMode := "READ_WRITE"
	diskType := "PERSISTENT"
	diskSize := int64(100)
	boot := true

	tests := []struct {
		name         string
		instance     *computepb.Instance
		instanceName string
		zone         string
		machineType  string
		checkFunc    func(t *testing.T, exts map[string]interface{})
	}{
		{
			name: "complete instance with network and disks",
			instance: &computepb.Instance{
				Id:                &instanceID,
				Name:              &instName,
				Status:            &status,
				SelfLink:          &selfLink,
				CpuPlatform:       &cpuPlatform,
				CreationTimestamp: &creationTS,
				Labels: map[string]string{
					"env": "test",
				},
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						Name:       &nicName,
						Network:    &network,
						Subnetwork: &subnetwork,
						NetworkIP:  &networkIP,
						AccessConfigs: []*computepb.AccessConfig{
							{NatIP: &natIP},
						},
					},
				},
				Disks: []*computepb.AttachedDisk{
					{
						Source:     &diskSource,
						DeviceName: &diskDevice,
						Mode:       &diskMode,
						Type:       &diskType,
						DiskSizeGb: &diskSize,
						Boot:       &boot,
					},
				},
				Scheduling: &computepb.Scheduling{
					Preemptible:      &preemptible,
					AutomaticRestart: &autoRestart,
				},
			},
			instanceName: "test-vm",
			zone:         "us-central1-a",
			machineType:  "n1-standard-1",
			checkFunc: func(t *testing.T, exts map[string]interface{}) {
				t.Helper()
				assert.Equal(t, uint64(1234567890), exts["gcp.id"])
				assert.Equal(t, "test-vm", exts["gcp.name"])
				assert.Equal(t, "us-central1-a", exts["gcp.zone"])
				assert.Equal(t, "n1-standard-1", exts["gcp.machineType"])
				assert.Equal(t, "RUNNING", exts["gcp.status"])
				assert.Equal(t, selfLink, exts["gcp.selfLink"])
				assert.Equal(t, "Intel Haswell", exts["gcp.cpuPlatform"])
				assert.Equal(t, creationTS, exts["gcp.creationTimestamp"])
				assert.Equal(t, true, exts["gcp.preemptible"])
				assert.Equal(t, false, exts["gcp.automaticRestart"])
				assert.Equal(t, "10.0.0.5", exts["gcp.internalIP"])
				assert.Equal(t, "34.123.45.67", exts["gcp.externalIP"])

				// Check labels
				labels, ok := exts["gcp.labels"].(map[string]string)
				require.True(t, ok)
				assert.Equal(t, "test", labels["env"])

				// Check network interfaces
				nics, ok := exts["gcp.networkInterfaces"].([]map[string]interface{})
				require.True(t, ok)
				require.Len(t, nics, 1)
				assert.Equal(t, "nic0", nics[0]["name"])
				assert.Equal(t, "10.0.0.5", nics[0]["internalIP"])
				assert.Equal(t, "34.123.45.67", nics[0]["externalIP"])

				// Check disks
				disks, ok := exts["gcp.disks"].([]map[string]interface{})
				require.True(t, ok)
				require.Len(t, disks, 1)
				assert.Equal(t, "boot-disk", disks[0]["deviceName"])
				assert.Equal(t, true, disks[0]["boot"])
				assert.Equal(t, int64(100), disks[0]["sizeGB"])
			},
		},
		{
			name: "minimal instance",
			instance: &computepb.Instance{
				Id:     &instanceID,
				Name:   &instName2,
				Status: &status,
			},
			instanceName: "minimal-vm",
			zone:         "europe-west1-b",
			machineType:  "e2-micro",
			checkFunc: func(t *testing.T, exts map[string]interface{}) {
				t.Helper()
				assert.Equal(t, uint64(1234567890), exts["gcp.id"])
				assert.Equal(t, "minimal-vm", exts["gcp.name"])
				assert.Equal(t, "europe-west1-b", exts["gcp.zone"])
				assert.Equal(t, "e2-micro", exts["gcp.machineType"])
				assert.Equal(t, "RUNNING", exts["gcp.status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildInstanceExtensions(tt.instance, tt.instanceName, tt.zone, tt.machineType)
			require.NotNil(t, got)
			tt.checkFunc(t, got)
		})
	}
}

// TestSubscriptions_UpdateAndEdgeCases tests subscription update and additional edge cases.
func TestSubscriptions_UpdateAndEdgeCases(t *testing.T) {
	adp := gcp.NewTestAdapter()
	ctx := context.Background()

	// Create a subscription first
	created, err := adp.CreateSubscription(ctx, &adapter.Subscription{
		SubscriptionID:         "sub-update-test",
		Callback:               "https://example.com/original",
		ConsumerSubscriptionID: "consumer-1",
		Filter: &adapter.SubscriptionFilter{
			ResourceTypeID: "compute",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	t.Run("UpdateSubscription success", func(t *testing.T) {
		updated, updateErr := adp.UpdateSubscription(ctx, "sub-update-test", &adapter.Subscription{
			Callback:               "https://example.com/updated",
			ConsumerSubscriptionID: "consumer-2",
			Filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "storage",
			},
		})
		require.NoError(t, updateErr)
		require.NotNil(t, updated)
		assert.Equal(t, "sub-update-test", updated.SubscriptionID)
		assert.Equal(t, "https://example.com/updated", updated.Callback)
		assert.Equal(t, "consumer-2", updated.ConsumerSubscriptionID)
		require.NotNil(t, updated.Filter)
		assert.Equal(t, "storage", updated.Filter.ResourceTypeID)
	})

	t.Run("UpdateSubscription persists changes", func(t *testing.T) {
		fetched, fetchErr := adp.GetSubscription(ctx, "sub-update-test")
		require.NoError(t, fetchErr)
		assert.Equal(t, "https://example.com/updated", fetched.Callback)
		assert.Equal(t, "consumer-2", fetched.ConsumerSubscriptionID)
	})

	t.Run("UpdateSubscription not found", func(t *testing.T) {
		_, updateErr := adp.UpdateSubscription(ctx, "nonexistent", &adapter.Subscription{
			Callback: "https://example.com/updated",
		})
		require.Error(t, updateErr)
		assert.Contains(t, updateErr.Error(), "subscription not found")
	})

	t.Run("UpdateSubscription empty callback", func(t *testing.T) {
		_, updateErr := adp.UpdateSubscription(ctx, "sub-update-test", &adapter.Subscription{
			Callback: "",
		})
		require.Error(t, updateErr)
		assert.Contains(t, updateErr.Error(), "callback URL is required")
	})

	t.Run("CreateSubscription with filter preserved", func(t *testing.T) {
		sub, createErr := adp.CreateSubscription(ctx, &adapter.Subscription{
			SubscriptionID: "sub-with-filter",
			Callback:       "https://example.com/filtered",
			Filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "compute",
			},
		})
		require.NoError(t, createErr)
		require.NotNil(t, sub)
		require.NotNil(t, sub.Filter)
		assert.Equal(t, "compute", sub.Filter.ResourceTypeID)
	})
}

// TestResourcePoolOperations_ZoneMode tests resource pool operations in zone mode (all return errors).
func TestResourcePoolOperations_ZoneMode(t *testing.T) {
	adp := gcp.NewTestAdapter()
	adp.TestSetPoolMode("zone")

	ctx := context.Background()

	t.Run("CreateResourcePool in zone mode returns error", func(t *testing.T) {
		pool, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{
			Name:        "test-pool",
			Description: "test description",
		})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "zone")
		assert.Contains(t, err.Error(), "GCP-managed")
	})

	t.Run("UpdateResourcePool in zone mode returns error", func(t *testing.T) {
		pool, err := adp.UpdateResourcePool(ctx, "some-id", &adapter.ResourcePool{
			Description: "updated description",
		})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "zone")
		assert.Contains(t, err.Error(), "GCP-managed")
	})

	t.Run("DeleteResourcePool in zone mode returns error", func(t *testing.T) {
		err := adp.DeleteResourcePool(ctx, "some-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "zone")
		assert.Contains(t, err.Error(), "GCP-managed")
	})
}

// TestResourcePoolOperations_IGMode tests resource pool operations in IG mode.
func TestResourcePoolOperations_IGMode(t *testing.T) {
	adp := gcp.NewTestAdapter()
	adp.TestSetPoolMode("ig")

	ctx := context.Background()

	t.Run("CreateResourcePool in IG mode returns not implemented", func(t *testing.T) {
		pool, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{
			Name:        "test-ig",
			Description: "test instance group",
		})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "not yet implemented")
	})

	t.Run("UpdateResourcePool in IG mode returns not implemented", func(t *testing.T) {
		pool, err := adp.UpdateResourcePool(ctx, "some-id", &adapter.ResourcePool{
			Description: "updated",
		})
		require.Error(t, err)
		assert.Nil(t, pool)
		assert.Contains(t, err.Error(), "not yet implemented")
	})

	t.Run("DeleteResourcePool in IG mode returns not implemented", func(t *testing.T) {
		err := adp.DeleteResourcePool(ctx, "some-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not yet implemented")
	})
}

// TestCreateResource_ReturnsError tests that CreateResource always returns error.
func TestCreateResource_ReturnsError(t *testing.T) {
	adp := gcp.NewTestAdapter()

	ctx := context.Background()
	result, err := adp.CreateResource(ctx, &adapter.Resource{
		ResourceTypeID: "gcp-machine-type-n1-standard-1",
		Description:    "Test instance",
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "additional configuration")
}

// TestGetResource_InvalidFormat tests GetResource with invalid ID formats.
func TestGetResource_InvalidFormat(t *testing.T) {
	adp := gcp.NewTestAdapter()

	ctx := context.Background()

	tests := []struct {
		name        string
		resourceID  string
		errContains string
	}{
		{
			name:        "missing prefix",
			resourceID:  "invalid-format",
			errContains: "invalid resource ID format",
		},
		{
			name:        "prefix only no remainder",
			resourceID:  "gcp-instance-",
			errContains: "invalid resource ID format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, err := adp.GetResource(ctx, tt.resourceID)
			require.Error(t, err)
			assert.Nil(t, resource)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// TestExtractZoneAndName_Exported tests the extractZoneAndName function via test helper.
func TestExtractZoneAndName_Exported(t *testing.T) {
	adp := gcp.NewTestAdapter()

	tests := []struct {
		name         string
		resource     *adapter.Resource
		wantZone     string
		wantInstance string
		expectErr    bool
	}{
		{
			name: "valid resource with zone and name",
			resource: &adapter.Resource{
				ResourceID: "gcp-instance-us-central1-a-my-vm",
				Extensions: map[string]interface{}{
					"gcp.zone": "us-central1-a",
					"gcp.name": "my-vm",
				},
			},
			wantZone:     "us-central1-a",
			wantInstance: "my-vm",
			expectErr:    false,
		},
		{
			name: "missing gcp.zone",
			resource: &adapter.Resource{
				ResourceID: "gcp-instance-us-central1-a-my-vm",
				Extensions: map[string]interface{}{
					"gcp.name": "my-vm",
				},
			},
			expectErr: true,
		},
		{
			name: "missing gcp.name",
			resource: &adapter.Resource{
				ResourceID: "gcp-instance-us-central1-a-my-vm",
				Extensions: map[string]interface{}{
					"gcp.zone": "us-central1-a",
				},
			},
			expectErr: true,
		},
		{
			name: "nil extensions",
			resource: &adapter.Resource{
				ResourceID: "gcp-instance-us-central1-a-my-vm",
			},
			expectErr: true,
		},
		{
			name: "wrong type for zone",
			resource: &adapter.Resource{
				ResourceID: "gcp-instance-us-central1-a-my-vm",
				Extensions: map[string]interface{}{
					"gcp.zone": 123,
					"gcp.name": "my-vm",
				},
			},
			expectErr: true,
		},
		{
			name: "wrong type for name",
			resource: &adapter.Resource{
				ResourceID: "gcp-instance-us-central1-a-my-vm",
				Extensions: map[string]interface{}{
					"gcp.zone": "us-central1-a",
					"gcp.name": 456,
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone, instanceName, err := adp.TestExtractZoneAndName(tt.resource)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantZone, zone)
				assert.Equal(t, tt.wantInstance, instanceName)
			}
		})
	}
}

// TestExtractMachineFamily_EdgeCases tests edge cases for machine family extraction.
func TestExtractMachineFamily_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		machineType string
		want        string
	}{
		{
			name:        "empty string",
			machineType: "",
			want:        "",
		},
		{
			name:        "no dash",
			machineType: "custom",
			want:        "custom",
		},
		{
			name:        "single dash",
			machineType: "n1-micro",
			want:        "n1",
		},
		{
			name:        "multiple dashes",
			machineType: "a2-highgpu-1g",
			want:        "a2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcp.ExtractMachineFamily(tt.machineType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractZoneName_EdgeCases tests edge cases for zone name extraction.
func TestExtractZoneName_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "no slash at all",
			url:  "us-central1-a",
			want: "us-central1-a",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
		{
			name: "trailing slash",
			url:  "https://example.com/zones/",
			want: "",
		},
		{
			name: "single slash prefix",
			url:  "/us-central1-a",
			want: "us-central1-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcp.ExtractZoneName(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractMachineTypeName_EdgeCases tests edge cases for machine type name extraction.
func TestExtractMachineTypeName_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "plain name no slashes",
			url:  "n1-standard-1",
			want: "n1-standard-1",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
		{
			name: "full URL",
			url:  "https://compute.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/machineTypes/e2-medium",
			want: "e2-medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcp.ExtractMachineTypeName(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}
