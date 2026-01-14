package gcp_test

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"

	"github.com/piwi3910/netweave/internal/adapters/gcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new GCPAdapter.
func TestNew(t *testing.T) {
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
	adp := &gcp.Adapter{
		Logger: zap.NewNop(),
	}

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
			url:  "https://compute.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/machineTypes/e2-micro",
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
	adp := &gcp.Adapter{
		Logger:        zap.NewNop(),
		Subscriptions: make(map[string]*adapter.Subscription),
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
	adapter, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = adapter.Health(ctx)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
}

// TestGCPAdapter_ListResourcePools tests the ListResourcePools function.
func TestGCPAdapter_ListResourcePools(t *testing.T) {
	adapter, err := gcp.New(&gcp.Config{
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

	pools, err := adapter.ListResourcePools(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, pools)
}

// TestGCPAdapter_ListResources tests the ListResources function.
func TestGCPAdapter_ListResources(t *testing.T) {
	adapter, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resources, err := adapter.ListResources(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, resources)
}

// TestGCPAdapter_ListResourceTypes tests the ListResourceTypes function.
func TestGCPAdapter_ListResourceTypes(t *testing.T) {
	adapter, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	types, err := adapter.ListResourceTypes(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, types)
}

// TestGCPAdapter_GetDeploymentManager tests the GetDeploymentManager function.
func TestGCPAdapter_GetDeploymentManager(t *testing.T) {
	adapter, err := gcp.New(&gcp.Config{
		ProjectID: "test-project",
		Region:    "us-central1",
		OCloudID:  "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm, err := adapter.GetDeploymentManager(ctx, "dm-1")
	if err != nil {
		t.Skip("Skipping - requires GCP credentials")
	}
	assert.NotNil(t, dm)
}
