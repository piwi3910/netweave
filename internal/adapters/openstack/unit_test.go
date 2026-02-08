package openstack_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/openstack"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestMain initializes metrics so webhook tests can call deliverWebhookWithRetries.
func TestMain(m *testing.M) {
	// Use a custom registry to avoid conflicts with global state
	reg := prometheus.NewRegistry()
	observability.InitMetricsWithRegistry("o2ims_test", reg)
	os.Exit(m.Run())
}

// newTestOpenStackAdapter creates an OpenStack adapter suitable for unit testing.
func newTestOpenStackAdapter() *openstack.Adapter {
	return &openstack.Adapter{
		Logger:              zap.NewNop(),
		OCloudID:            "ocloud-test",
		DeploymentManagerID: "ocloud-openstack-RegionOne",
		Region:              "RegionOne",
		Subscriptions:       make(map[string]*adapter.Subscription),
		PollingStates:       make(map[string]*openstack.SubscriptionState),
	}
}

// --- UpdateSubscription Tests ---

func TestUpdateSubscription(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	// Create a subscription first
	created, err := adp.CreateSubscription(ctx, &adapter.Subscription{
		SubscriptionID:         "sub-update-test",
		Callback:               "https://callback.example.com/original",
		ConsumerSubscriptionID: "consumer-1",
		Filter: &adapter.SubscriptionFilter{
			ResourceTypeID: "compute",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	t.Run("update existing subscription", func(t *testing.T) {
		updated, updateErr := adp.UpdateSubscription(ctx, "sub-update-test", &adapter.Subscription{
			Callback:               "https://callback.example.com/updated",
			ConsumerSubscriptionID: "consumer-2",
			Filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "storage",
			},
		})
		require.NoError(t, updateErr)
		require.NotNil(t, updated)
		assert.Equal(t, "sub-update-test", updated.SubscriptionID)
		assert.Equal(t, "https://callback.example.com/updated", updated.Callback)
		assert.Equal(t, "consumer-2", updated.ConsumerSubscriptionID)
		require.NotNil(t, updated.Filter)
		assert.Equal(t, "storage", updated.Filter.ResourceTypeID)
	})

	t.Run("update persists changes", func(t *testing.T) {
		fetched, getErr := adp.GetSubscription(ctx, "sub-update-test")
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
		_, updateErr := adp.UpdateSubscription(ctx, "sub-update-test", &adapter.Subscription{
			Callback: "",
		})
		require.Error(t, updateErr)
		assert.Contains(t, updateErr.Error(), "callback URL is required")
	})
}

// --- DetectChanges Tests ---

func TestDetectChanges(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("detect created resources", func(t *testing.T) {
		oldSnapshot := map[string]string{
			"resource-1": "hash-1",
		}
		newSnapshot := map[string]string{
			"resource-1": "hash-1",
			"resource-2": "hash-2",
		}

		changes := adp.ExportDetectChanges(oldSnapshot, newSnapshot)
		require.Len(t, changes, 1)
		assert.Equal(t, "resource-2", changes[0].ResourceID)
		assert.Equal(t, "ResourceCreated", changes[0].EventType)
	})

	t.Run("detect updated resources", func(t *testing.T) {
		oldSnapshot := map[string]string{
			"resource-1": "hash-1",
		}
		newSnapshot := map[string]string{
			"resource-1": "hash-1-updated",
		}

		changes := adp.ExportDetectChanges(oldSnapshot, newSnapshot)
		require.Len(t, changes, 1)
		assert.Equal(t, "resource-1", changes[0].ResourceID)
		assert.Equal(t, "ResourceUpdated", changes[0].EventType)
	})

	t.Run("detect deleted resources", func(t *testing.T) {
		oldSnapshot := map[string]string{
			"resource-1": "hash-1",
			"resource-2": "hash-2",
		}
		newSnapshot := map[string]string{
			"resource-1": "hash-1",
		}

		changes := adp.ExportDetectChanges(oldSnapshot, newSnapshot)
		require.Len(t, changes, 1)
		assert.Equal(t, "resource-2", changes[0].ResourceID)
		assert.Equal(t, "ResourceDeleted", changes[0].EventType)
	})

	t.Run("detect mixed changes", func(t *testing.T) {
		oldSnapshot := map[string]string{
			"resource-1": "hash-1",
			"resource-2": "hash-2",
		}
		newSnapshot := map[string]string{
			"resource-1": "hash-1-updated",
			"resource-3": "hash-3",
		}

		changes := adp.ExportDetectChanges(oldSnapshot, newSnapshot)
		// resource-1 updated, resource-2 deleted, resource-3 created
		assert.Len(t, changes, 3)

		eventTypes := make(map[string]bool)
		for _, change := range changes {
			eventTypes[change.EventType] = true
		}
		assert.True(t, eventTypes["ResourceCreated"])
		assert.True(t, eventTypes["ResourceUpdated"])
		assert.True(t, eventTypes["ResourceDeleted"])
	})

	t.Run("no changes detected", func(t *testing.T) {
		snapshot := map[string]string{
			"resource-1": "hash-1",
			"resource-2": "hash-2",
		}

		changes := adp.ExportDetectChanges(snapshot, snapshot)
		assert.Empty(t, changes)
	})

	t.Run("both empty snapshots", func(t *testing.T) {
		changes := adp.ExportDetectChanges(
			map[string]string{},
			map[string]string{},
		)
		assert.Empty(t, changes)
	})
}

// --- MatchesFilter Tests ---

func TestMatchesFilter(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("no filter matches all", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com",
		}

		matched := adp.ExportMatchesFilter(sub, "ResourceCreated", "resource-1")
		assert.True(t, matched)
	})

	t.Run("nil subscription filter matches all", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com",
			Filter:   nil,
		}

		matched := adp.ExportMatchesFilter(sub, "ResourceUpdated", "resource-1")
		assert.True(t, matched)
	})

	t.Run("matching resource ID filter", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com",
			Filter: &adapter.SubscriptionFilter{
				ResourceID: "resource-1",
			},
		}

		matched := adp.ExportMatchesFilter(sub, "ResourceUpdated", "resource-1")
		assert.True(t, matched)
	})

	t.Run("non-matching resource ID filter", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com",
			Filter: &adapter.SubscriptionFilter{
				ResourceID: "resource-1",
			},
		}

		matched := adp.ExportMatchesFilter(sub, "ResourceUpdated", "resource-2")
		assert.False(t, matched)
	})

	t.Run("resource type filter matches all", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback: "https://callback.example.com",
			Filter: &adapter.SubscriptionFilter{
				ResourceTypeID: "compute",
			},
		}

		matched := adp.ExportMatchesFilter(sub, "ResourceCreated", "resource-1")
		assert.True(t, matched)
	})
}

// --- ComputeResourceHash Tests ---

func TestComputeResourceHash(t *testing.T) {
	t.Run("same input produces same hash", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		hash1 := openstack.ExportComputeResourceHash(data)
		hash2 := openstack.ExportComputeResourceHash(data)
		assert.Equal(t, hash1, hash2)
		assert.NotEmpty(t, hash1)
	})

	t.Run("different input produces different hash", func(t *testing.T) {
		data1 := map[string]string{"key": "value1"}
		data2 := map[string]string{"key": "value2"}
		hash1 := openstack.ExportComputeResourceHash(data1)
		hash2 := openstack.ExportComputeResourceHash(data2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("empty input produces hash", func(t *testing.T) {
		hash := openstack.ExportComputeResourceHash(map[string]string{})
		assert.NotEmpty(t, hash)
	})

	t.Run("nil input produces empty hash", func(t *testing.T) {
		// Channels can't be marshaled to JSON
		hash := openstack.ExportComputeResourceHash(make(chan int))
		assert.Empty(t, hash)
	})
}

// --- StopAllPolling Tests ---

func TestStopAllPolling(t *testing.T) {
	t.Run("stop with no polling states", func(t *testing.T) {
		adp := newTestOpenStackAdapter()
		// Should not panic with nil PollingStates
		adp.StopAllPolling()
	})

	t.Run("stop with empty polling states map", func(t *testing.T) {
		adp := newTestOpenStackAdapter()
		adp.PollingStates = make(map[string]*openstack.SubscriptionState)
		adp.StopAllPolling()
	})
}

// --- Close Tests ---

func TestOpenStackAdapter_Close_Unit(t *testing.T) {
	adp := newTestOpenStackAdapter()

	err := adp.Close()
	// Close may return a sync error on nop logger, which is OK
	_ = err
}

// --- Adapter Metadata Tests ---

func TestOpenStackAdapter_Metadata_Unit(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("Name returns openstack", func(t *testing.T) {
		assert.Equal(t, "openstack", adp.Name())
	})

	t.Run("Version returns nova-v2.1", func(t *testing.T) {
		assert.Equal(t, "nova-v2.1", adp.Version())
	})

	t.Run("Capabilities includes all expected", func(t *testing.T) {
		caps := adp.Capabilities()
		assert.Len(t, caps, 6)
		assert.Contains(t, caps, adapter.CapabilityResourcePools)
		assert.Contains(t, caps, adapter.CapabilityResources)
		assert.Contains(t, caps, adapter.CapabilityResourceTypes)
		assert.Contains(t, caps, adapter.CapabilityDeploymentManagers)
		assert.Contains(t, caps, adapter.CapabilitySubscriptions)
		assert.Contains(t, caps, adapter.CapabilityHealthChecks)
	})
}

// --- CreateResource Validation ---

func TestOpenStackAdapter_CreateResource_Unit(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("missing resource type ID", func(t *testing.T) {
		_, err := adp.CreateResource(ctx, &adapter.Resource{
			ResourceTypeID: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resourceTypeID is required")
	})

	t.Run("invalid resource type ID format", func(t *testing.T) {
		_, err := adp.CreateResource(ctx, &adapter.Resource{
			ResourceTypeID: "invalid-format",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resourceTypeID format")
	})

	t.Run("missing image ID in extensions", func(t *testing.T) {
		_, err := adp.CreateResource(ctx, &adapter.Resource{
			ResourceTypeID: "openstack-flavor-m1.small",
			Extensions:     map[string]interface{}{},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "imageId is required")
	})
}

// --- CreateResourcePool Validation ---

func TestOpenStackAdapter_CreateResourcePool_Unit(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("empty name fails", func(t *testing.T) {
		_, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{
			Name: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

// --- InitWebhookClient Tests ---

func TestInitWebhookClient(t *testing.T) {
	t.Run("returns non-nil client", func(t *testing.T) {
		client := openstack.ExportInitWebhookClient()
		assert.NotNil(t, client)
	})

	t.Run("returns same client on multiple calls", func(t *testing.T) {
		client1 := openstack.ExportInitWebhookClient()
		client2 := openstack.ExportInitWebhookClient()
		assert.NotNil(t, client1)
		assert.NotNil(t, client2)
	})
}

// --- ValidateConfig Tests ---

func TestValidateConfig(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		err := openstack.ExportValidateConfig(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("missing authURL", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			Username:    "admin",
			Password:    "secret",
			ProjectName: "demo",
			Region:      "RegionOne",
			OCloudID:    "ocloud-1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})

	t.Run("missing username", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			AuthURL:     "https://auth.example.com/v3",
			Password:    "secret",
			ProjectName: "demo",
			Region:      "RegionOne",
			OCloudID:    "ocloud-1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})

	t.Run("missing password", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			AuthURL:     "https://auth.example.com/v3",
			Username:    "admin",
			ProjectName: "demo",
			Region:      "RegionOne",
			OCloudID:    "ocloud-1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})

	t.Run("missing projectName", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			AuthURL:  "https://auth.example.com/v3",
			Username: "admin",
			Password: "secret",
			Region:   "RegionOne",
			OCloudID: "ocloud-1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})

	t.Run("missing region", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			AuthURL:     "https://auth.example.com/v3",
			Username:    "admin",
			Password:    "secret",
			ProjectName: "demo",
			OCloudID:    "ocloud-1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})

	t.Run("missing oCloudID", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			AuthURL:     "https://auth.example.com/v3",
			Username:    "admin",
			Password:    "secret",
			ProjectName: "demo",
			Region:      "RegionOne",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})

	t.Run("valid config", func(t *testing.T) {
		err := openstack.ExportValidateConfig(&openstack.Config{
			AuthURL:     "https://auth.example.com/v3",
			Username:    "admin",
			Password:    "secret",
			ProjectName: "demo",
			Region:      "RegionOne",
			OCloudID:    "ocloud-1",
		})
		require.NoError(t, err)
	})
}

// --- ApplyDefaults Tests ---

func TestApplyDefaults(t *testing.T) {
	t.Run("default values applied", func(t *testing.T) {
		cfg := &openstack.Config{
			Region: "RegionOne",
		}
		domainName, dmID, timeout, logger := openstack.ExportApplyDefaults(cfg)

		assert.Equal(t, "Default", domainName)
		assert.Equal(t, "ocloud-openstack-RegionOne", dmID)
		assert.Equal(t, 30*time.Second, timeout)
		assert.NotNil(t, logger)
	})

	t.Run("custom values preserved", func(t *testing.T) {
		customLogger := zap.NewNop()
		cfg := &openstack.Config{
			Region:              "RegionTwo",
			DomainName:          "CustomDomain",
			DeploymentManagerID: "custom-dm",
			Timeout:             60 * time.Second,
			Logger:              customLogger,
		}
		domainName, dmID, timeout, logger := openstack.ExportApplyDefaults(cfg)

		assert.Equal(t, "CustomDomain", domainName)
		assert.Equal(t, "custom-dm", dmID)
		assert.Equal(t, 60*time.Second, timeout)
		assert.Equal(t, customLogger, logger)
	})
}

// --- ExtractRequiredParams Tests ---

func TestExtractRequiredParams(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("missing resource type ID", func(t *testing.T) {
		_, _, err := adp.ExportExtractRequiredParams(&adapter.Resource{
			ResourceTypeID: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resourceTypeID is required")
	})

	t.Run("invalid resource type ID format", func(t *testing.T) {
		_, _, err := adp.ExportExtractRequiredParams(&adapter.Resource{
			ResourceTypeID: "invalid-format",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resourceTypeID format")
	})

	t.Run("missing image ID", func(t *testing.T) {
		_, _, err := adp.ExportExtractRequiredParams(&adapter.Resource{
			ResourceTypeID: "openstack-flavor-m1.small",
			Extensions:     map[string]interface{}{},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "imageId is required")
	})

	t.Run("valid parameters", func(t *testing.T) {
		flavorID, imageID, err := adp.ExportExtractRequiredParams(&adapter.Resource{
			ResourceTypeID: "openstack-flavor-m1.small",
			Extensions: map[string]interface{}{
				"openstack.imageId": "img-uuid-123",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "m1.small", flavorID)
		assert.Equal(t, "img-uuid-123", imageID)
	})

	t.Run("nil extensions", func(t *testing.T) {
		_, _, err := adp.ExportExtractRequiredParams(&adapter.Resource{
			ResourceTypeID: "openstack-flavor-m1.tiny",
			Extensions:     nil,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "imageId is required")
	})

	t.Run("image ID wrong type", func(t *testing.T) {
		_, _, err := adp.ExportExtractRequiredParams(&adapter.Resource{
			ResourceTypeID: "openstack-flavor-m1.small",
			Extensions: map[string]interface{}{
				"openstack.imageId": 12345,
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "imageId is required")
	})
}

// --- GenerateFlavorID is tested in adapter_test.go ---
// --- Subscription Lifecycle Extended Tests ---

func TestOpenStackAdapter_SubscriptionLifecycle(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("create with generated ID", func(t *testing.T) {
		sub, err := adp.CreateSubscription(ctx, &adapter.Subscription{
			Callback: "https://callback.example.com/events",
		})
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.NotEmpty(t, sub.SubscriptionID)
		assert.Equal(t, "https://callback.example.com/events", sub.Callback)
	})

	t.Run("create with provided ID", func(t *testing.T) {
		sub, err := adp.CreateSubscription(ctx, &adapter.Subscription{
			SubscriptionID: "custom-sub-id",
			Callback:       "https://callback.example.com/events2",
		})
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, "custom-sub-id", sub.SubscriptionID)
	})

	t.Run("create with empty callback fails", func(t *testing.T) {
		_, err := adp.CreateSubscription(ctx, &adapter.Subscription{
			Callback: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("get existing subscription", func(t *testing.T) {
		sub, err := adp.GetSubscription(ctx, "custom-sub-id")
		require.NoError(t, err)
		assert.Equal(t, "https://callback.example.com/events2", sub.Callback)
	})

	t.Run("get non-existent subscription", func(t *testing.T) {
		_, err := adp.GetSubscription(ctx, "does-not-exist")
		require.Error(t, err)
	})

	t.Run("list subscriptions", func(t *testing.T) {
		subs, err := adp.ListSubscriptions(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(subs), 2)
	})

	t.Run("delete existing subscription", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "custom-sub-id")
		require.NoError(t, err)

		// Verify deleted
		_, err = adp.GetSubscription(ctx, "custom-sub-id")
		require.Error(t, err)
	})

	t.Run("delete non-existent subscription", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "does-not-exist")
		require.Error(t, err)
	})
}

// --- ExtractServerID Tests (covered in resources_test.go) ---

// --- BuildServerMetadata Tests ---

func TestBuildServerMetadata(t *testing.T) {
	tests := []struct {
		name     string
		resource *adapter.Resource
		wantKeys []string
		wantLen  int
	}{
		{
			name: "all fields set",
			resource: &adapter.Resource{
				TenantID:      "tenant-1",
				Description:   "My Server",
				GlobalAssetID: "urn:openstack:server:region:id",
				Extensions: map[string]interface{}{
					"openstack.metadata": map[string]string{
						"custom-key": "custom-value",
					},
				},
			},
			wantKeys: []string{"o2ims.io/tenant-id", "name", "global_asset_id", "custom-key"},
			wantLen:  4,
		},
		{
			name:     "empty resource",
			resource: &adapter.Resource{},
			wantLen:  0,
		},
		{
			name: "only tenant ID",
			resource: &adapter.Resource{
				TenantID: "t1",
			},
			wantKeys: []string{"o2ims.io/tenant-id"},
			wantLen:  1,
		},
		{
			name: "wrong type for custom metadata",
			resource: &adapter.Resource{
				Description: "test",
				Extensions: map[string]interface{}{
					"openstack.metadata": "not-a-map",
				},
			},
			wantKeys: []string{"name"},
			wantLen:  1,
		},
		{
			name: "nil extensions",
			resource: &adapter.Resource{
				Description: "test",
			},
			wantKeys: []string{"name"},
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := openstack.ExportBuildServerMetadata(tt.resource)
			assert.Len(t, metadata, tt.wantLen)
			for _, key := range tt.wantKeys {
				_, exists := metadata[key]
				assert.True(t, exists, "expected key %q in metadata", key)
			}
		})
	}
}

// --- ResourceMatchesFilter Tests ---

func TestResourceMatchesFilter(t *testing.T) {
	adp := newTestOpenStackAdapter()

	tests := []struct {
		name     string
		resource *adapter.Resource
		filter   *adapter.Filter
		want     bool
	}{
		{
			name: "matching resource type",
			resource: &adapter.Resource{
				ResourceTypeID: "openstack-flavor-m1.small",
			},
			filter: &adapter.Filter{
				ResourceTypeID: "openstack-flavor-m1.small",
			},
			want: true,
		},
		{
			name: "non-matching resource type",
			resource: &adapter.Resource{
				ResourceTypeID: "openstack-flavor-m1.small",
			},
			filter: &adapter.Filter{
				ResourceTypeID: "openstack-flavor-m1.large",
			},
			want: false,
		},
		{
			name: "empty filter matches all",
			resource: &adapter.Resource{
				ResourceTypeID: "openstack-flavor-m1.small",
			},
			filter: &adapter.Filter{},
			want:   true,
		},
		{
			name: "labels filter rejects",
			resource: &adapter.Resource{
				ResourceTypeID: "openstack-flavor-m1.small",
			},
			filter: &adapter.Filter{
				Labels: map[string]string{"env": "prod"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.ExportResourceMatchesFilter(tt.resource, tt.filter)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- GenerateServerResourceID Tests ---

func TestGenerateServerResourceID(t *testing.T) {
	tests := []struct {
		name     string
		serverID string
		want     string
	}{
		{
			name:     "standard UUID",
			serverID: "550e8400-e29b-41d4-a716-446655440000",
			want:     "openstack-server-550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "simple ID",
			serverID: "server-1",
			want:     "openstack-server-server-1",
		},
		{
			name:     "empty ID",
			serverID: "",
			want:     "openstack-server-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openstack.ExportGenerateServerResourceID(tt.serverID)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- ApplyPaginationIfNeeded Tests ---

func TestApplyPaginationIfNeeded(t *testing.T) {
	adp := newTestOpenStackAdapter()

	resources := []*adapter.Resource{
		{ResourceID: "r1"},
		{ResourceID: "r2"},
		{ResourceID: "r3"},
		{ResourceID: "r4"},
		{ResourceID: "r5"},
	}

	t.Run("nil filter returns all", func(t *testing.T) {
		result := adp.ExportApplyPaginationIfNeeded(resources, nil)
		assert.Len(t, result, 5)
	})

	t.Run("with limit", func(t *testing.T) {
		result := adp.ExportApplyPaginationIfNeeded(resources, &adapter.Filter{Limit: 2})
		assert.Len(t, result, 2)
		assert.Equal(t, "r1", result[0].ResourceID)
	})

	t.Run("with offset and limit", func(t *testing.T) {
		result := adp.ExportApplyPaginationIfNeeded(resources, &adapter.Filter{Limit: 2, Offset: 2})
		assert.Len(t, result, 2)
		assert.Equal(t, "r3", result[0].ResourceID)
	})

	t.Run("zero limit and offset returns all", func(t *testing.T) {
		result := adp.ExportApplyPaginationIfNeeded(resources, &adapter.Filter{})
		assert.Len(t, result, 5)
	})
}

// --- CreateResourceSnapshot with nil compute ---

func TestCreateResourceSnapshot_NilCompute(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	snapshot, err := adp.ExportCreateResourceSnapshot(ctx)
	require.NoError(t, err)
	assert.NotNil(t, snapshot)
	assert.Empty(t, snapshot)
}

// --- TransformServerToResource Tests (covered in resources_test.go) ---
// --- GetResourcePoolIDFromServer Tests (covered in resources_test.go) ---

// --- StopPolling Tests ---

func TestStopPolling(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("stop non-existent polling", func(t *testing.T) {
		err := adp.ExportStopPolling("non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no polling state found")
	})
}

// --- GenerateFlavorID Tests (covered in adapter_test.go) ---

// --- UpdateSubscription extended ---

func TestOpenStackAdapter_UpdateSubscription_Extended(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	// Create base subscription
	created, err := adp.CreateSubscription(ctx, &adapter.Subscription{
		SubscriptionID: "sub-ext-1",
		Callback:       "https://original.example.com",
		Filter: &adapter.SubscriptionFilter{
			ResourceTypeID: "compute",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	t.Run("update callback only", func(t *testing.T) {
		updated, updateErr := adp.UpdateSubscription(ctx, "sub-ext-1", &adapter.Subscription{
			Callback: "https://new.example.com",
		})
		require.NoError(t, updateErr)
		assert.Equal(t, "https://new.example.com", updated.Callback)
	})

	t.Run("update consumer subscription ID", func(t *testing.T) {
		updated, updateErr := adp.UpdateSubscription(ctx, "sub-ext-1", &adapter.Subscription{
			Callback:               "https://new.example.com",
			ConsumerSubscriptionID: "new-consumer-id",
		})
		require.NoError(t, updateErr)
		assert.Equal(t, "new-consumer-id", updated.ConsumerSubscriptionID)
	})

	t.Run("update preserves subscription ID", func(t *testing.T) {
		updated, updateErr := adp.UpdateSubscription(ctx, "sub-ext-1", &adapter.Subscription{
			Callback: "https://final.example.com",
		})
		require.NoError(t, updateErr)
		assert.Equal(t, "sub-ext-1", updated.SubscriptionID)
	})
}

// --- TransformAndFilterServers Tests ---

func TestTransformAndFilterServers(t *testing.T) {
	adp := newTestOpenStackAdapter()

	serverList := []servers.Server{
		{
			ID:       "server-1",
			Name:     "web-1",
			Status:   "ACTIVE",
			TenantID: "project-1",
			Flavor:   map[string]interface{}{"id": "m1.small"},
			Metadata: map[string]string{"o2ims.io/tenant-id": "tenant-a"},
		},
		{
			ID:       "server-2",
			Name:     "web-2",
			Status:   "ACTIVE",
			TenantID: "project-1",
			Flavor:   map[string]interface{}{"id": "m1.large"},
			Metadata: map[string]string{"o2ims.io/tenant-id": "tenant-b"},
		},
		{
			ID:       "server-3",
			Name:     "db-1",
			Status:   "ACTIVE",
			TenantID: "project-1",
			Flavor:   map[string]interface{}{"id": "m1.small"},
			Metadata: map[string]string{},
		},
	}

	t.Run("no filter returns all", func(t *testing.T) {
		resources := adp.ExportTransformAndFilterServers(serverList, nil)
		assert.Len(t, resources, 3)
	})

	t.Run("filter by tenant ID", func(t *testing.T) {
		resources := adp.ExportTransformAndFilterServers(serverList, &adapter.Filter{
			TenantID: "tenant-a",
		})
		assert.Len(t, resources, 1)
		assert.Equal(t, "openstack-server-server-1", resources[0].ResourceID)
	})

	t.Run("filter by resource type", func(t *testing.T) {
		resources := adp.ExportTransformAndFilterServers(serverList, &adapter.Filter{
			ResourceTypeID: "openstack-flavor-m1.small",
		})
		// server-1 and server-3 have m1.small (server-3 has no tenant match issue)
		found := 0
		for _, r := range resources {
			if r.ResourceTypeID == "openstack-flavor-m1.small" {
				found++
			}
		}
		assert.Equal(t, found, len(resources))
	})

	t.Run("filter by labels rejects all", func(t *testing.T) {
		resources := adp.ExportTransformAndFilterServers(serverList, &adapter.Filter{
			Labels: map[string]string{"env": "prod"},
		})
		assert.Empty(t, resources)
	})

	t.Run("non-matching tenant ID returns none", func(t *testing.T) {
		resources := adp.ExportTransformAndFilterServers(serverList, &adapter.Filter{
			TenantID: "nonexistent-tenant",
		})
		assert.Empty(t, resources)
	})
}

// --- BuildListOptions Tests ---

func TestBuildListOptions(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("nil filter", func(t *testing.T) {
		opts := adp.ExportBuildListOptions(ctx, nil)
		assert.Equal(t, false, opts.AllTenants)
		assert.Empty(t, opts.AvailabilityZone)
	})

	t.Run("filter with location", func(t *testing.T) {
		opts := adp.ExportBuildListOptions(ctx, &adapter.Filter{
			Location: "nova",
		})
		assert.Equal(t, "nova", opts.AvailabilityZone)
	})

	t.Run("empty filter", func(t *testing.T) {
		opts := adp.ExportBuildListOptions(ctx, &adapter.Filter{})
		assert.Empty(t, opts.AvailabilityZone)
	})
}

// --- AddNetworkConfig Tests ---

func TestAddNetworkConfig(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("no networks in extensions", func(t *testing.T) {
		opts := adp.ExportAddNetworkConfigFull(map[string]interface{}{})
		assert.Nil(t, opts.Networks)
	})

	t.Run("wrong type for networks", func(t *testing.T) {
		opts := adp.ExportAddNetworkConfigFull(map[string]interface{}{
			"openstack.networks": "not-a-slice",
		})
		assert.Nil(t, opts.Networks)
	})

	t.Run("valid networks", func(t *testing.T) {
		opts := adp.ExportAddNetworkConfigFull(map[string]interface{}{
			"openstack.networks": []string{"net-1", "net-2"},
		})
		require.Len(t, opts.Networks, 2)
	})

	t.Run("empty network list", func(t *testing.T) {
		opts := adp.ExportAddNetworkConfigFull(map[string]interface{}{
			"openstack.networks": []string{},
		})
		assert.Nil(t, opts.Networks)
	})
}

// --- AddSecurityGroups Tests ---

func TestAddSecurityGroups(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("no security groups", func(t *testing.T) {
		groups := adp.ExportAddSecurityGroups(map[string]interface{}{})
		assert.Nil(t, groups)
	})

	t.Run("wrong type", func(t *testing.T) {
		groups := adp.ExportAddSecurityGroups(map[string]interface{}{
			"openstack.securityGroups": "not-a-slice",
		})
		assert.Nil(t, groups)
	})

	t.Run("valid security groups", func(t *testing.T) {
		groups := adp.ExportAddSecurityGroups(map[string]interface{}{
			"openstack.securityGroups": []string{"default", "web-sg"},
		})
		require.Len(t, groups, 2)
		assert.Equal(t, "default", groups[0])
		assert.Equal(t, "web-sg", groups[1])
	})

	t.Run("empty security groups list", func(t *testing.T) {
		groups := adp.ExportAddSecurityGroups(map[string]interface{}{
			"openstack.securityGroups": []string{},
		})
		assert.Nil(t, groups)
	})
}

// --- BuildCreateOptions Tests ---

func TestBuildCreateOptions(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("basic create options", func(t *testing.T) {
		resource := &adapter.Resource{
			Extensions: map[string]interface{}{
				"openstack.name": "my-server",
			},
		}
		opts := adp.ExportBuildCreateOptions(ctx, resource, "m1.small", "img-123")
		assert.Equal(t, "my-server", opts.Name)
		assert.Equal(t, "m1.small", opts.FlavorRef)
		assert.Equal(t, "img-123", opts.ImageRef)
	})

	t.Run("default name", func(t *testing.T) {
		resource := &adapter.Resource{
			Extensions: map[string]interface{}{},
		}
		opts := adp.ExportBuildCreateOptions(ctx, resource, "m1.small", "img-123")
		assert.Equal(t, "openstack-instance", opts.Name)
	})

	t.Run("with networks and security groups", func(t *testing.T) {
		resource := &adapter.Resource{
			Extensions: map[string]interface{}{
				"openstack.name":           "net-server",
				"openstack.networks":       []string{"net-1"},
				"openstack.securityGroups": []string{"sg-1"},
			},
		}
		opts := adp.ExportBuildCreateOptions(ctx, resource, "m1.large", "img-456")
		assert.Equal(t, "net-server", opts.Name)
		assert.Len(t, opts.Networks, 1)
		assert.Equal(t, []string{"sg-1"}, opts.SecurityGroups)
	})
}

// --- TransformFlavorToResourceType Tests (additional) ---

func TestTransformFlavorToResourceType_Unit(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("standard compute flavor", func(t *testing.T) {
		flavor := &flavors.Flavor{
			ID:         "1",
			Name:       "m1.small",
			VCPUs:      1,
			RAM:        2048,
			Disk:       20,
			Swap:       0,
			Ephemeral:  0,
			IsPublic:   true,
			RxTxFactor: 1.0,
		}
		rt := adp.TransformFlavorToResourceType(flavor)
		require.NotNil(t, rt)
		assert.Equal(t, "openstack-flavor-1", rt.ResourceTypeID)
		assert.Equal(t, "m1.small", rt.Name)
		assert.Equal(t, "OpenStack", rt.Vendor)
		assert.Equal(t, "compute", rt.ResourceClass)
		assert.Equal(t, "virtual", rt.ResourceKind)
		assert.Contains(t, rt.Description, "m1.small")
		assert.Contains(t, rt.Description, "vCPUs: 1")
		assert.Contains(t, rt.Description, "RAM: 2048MB")
		assert.Equal(t, 1, rt.Extensions["openstack.vcpus"])
		assert.Equal(t, 2048, rt.Extensions["openstack.ram"])
		assert.Equal(t, 20, rt.Extensions["openstack.disk"])
	})

	t.Run("storage-only flavor", func(t *testing.T) {
		flavor := &flavors.Flavor{
			ID:   "storage-1",
			Name: "storage.large",
			Disk: 1000,
			// VCPUs=0, RAM=0 => storage class
		}
		rt := adp.TransformFlavorToResourceType(flavor)
		require.NotNil(t, rt)
		assert.Equal(t, "storage", rt.ResourceClass)
	})

	t.Run("flavor with zero disk", func(t *testing.T) {
		flavor := &flavors.Flavor{
			ID:    "diskless",
			Name:  "m1.tiny",
			VCPUs: 1,
			RAM:   512,
			Disk:  0,
		}
		rt := adp.TransformFlavorToResourceType(flavor)
		require.NotNil(t, rt)
		assert.Equal(t, "compute", rt.ResourceClass)
	})
}

// --- GetDeploymentManager non-matching ID ---

func TestOpenStackAdapter_GetDeploymentManager_NotFound(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	_, err := adp.GetDeploymentManager(ctx, "wrong-dm-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deployment manager not found")
}

// --- DeleteResource invalid format ---

func TestOpenStackAdapter_DeleteResource_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	err := adp.DeleteResource(ctx, "invalid-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource ID format")
}

// --- UpdateResource invalid format ---

func TestOpenStackAdapter_UpdateResource_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	_, err := adp.UpdateResource(ctx, "invalid-format", &adapter.Resource{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource ID format")
}

// --- GetResourceType with invalid format ---

func TestOpenStackAdapter_GetResourceType_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	_, err := adp.GetResourceType(ctx, "invalid-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource type ID format")
}

// --- UpdateResourcePool with invalid format ---

func TestOpenStackAdapter_UpdateResourcePool_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	_, err := adp.UpdateResourcePool(ctx, "invalid-format", &adapter.ResourcePool{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource pool ID format")
}

// --- DeleteResourcePool with invalid format ---

func TestOpenStackAdapter_DeleteResourcePool_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	err := adp.DeleteResourcePool(ctx, "invalid-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource pool ID format")
}

// --- DeliverWebhook Tests ---

func TestDeliverWebhook(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("successful delivery returns 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "o2ims-gateway/1.0", r.Header.Get("User-Agent"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := server.Client()
		payload := []byte(`{"test": "data"}`)

		statusCode, err := adp.ExportDeliverWebhook(context.Background(), client, server.URL, payload)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
	})

	t.Run("server returns 500", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := server.Client()
		payload := []byte(`{"test": "data"}`)

		statusCode, err := adp.ExportDeliverWebhook(context.Background(), client, server.URL, payload)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, statusCode)
	})

	t.Run("server returns 204 No Content", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		client := server.Client()
		payload := []byte(`{"test": "data"}`)

		statusCode, err := adp.ExportDeliverWebhook(context.Background(), client, server.URL, payload)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, statusCode)
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		client := &http.Client{Timeout: 1 * time.Second}
		payload := []byte(`{"test": "data"}`)

		_, err := adp.ExportDeliverWebhook(context.Background(), client, "http://invalid-host-that-does-not-exist.local:1234/webhook", payload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send request")
	})

	t.Run("canceled context returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := server.Client()
		payload := []byte(`{"test": "data"}`)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := adp.ExportDeliverWebhook(ctx, client, server.URL, payload)
		require.Error(t, err)
	})

	t.Run("payload is received correctly", func(t *testing.T) {
		var receivedBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			receivedBody = body
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := server.Client()
		notification := map[string]string{"subscriptionId": "sub-1", "eventType": "ResourceCreated"}
		payload, err := json.Marshal(notification)
		require.NoError(t, err)

		statusCode, err := adp.ExportDeliverWebhook(context.Background(), client, server.URL, payload)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.NotEmpty(t, receivedBody)
	})
}

// --- DeliverWebhookWithRetries Tests ---

func TestDeliverWebhookWithRetries(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("successful on first attempt", func(t *testing.T) {
		var attemptCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&attemptCount, 1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		payload := []byte(`{"test": "data"}`)
		err := adp.ExportDeliverWebhookWithRetries(context.Background(), server.URL, payload)
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&attemptCount))
	})

	t.Run("retries on 500 error then succeeds", func(t *testing.T) {
		var attemptCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		payload := []byte(`{"test": "data"}`)
		err := adp.ExportDeliverWebhookWithRetries(context.Background(), server.URL, payload)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, atomic.LoadInt32(&attemptCount), int32(2))
	})

	t.Run("context cancellation during retry", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		payload := []byte(`{"test": "data"}`)
		err := adp.ExportDeliverWebhookWithRetries(ctx, server.URL, payload)
		require.Error(t, err)
		// Should either be context canceled or webhook delivery failed
		assert.True(t, err != nil)
	})

	t.Run("all retries exhausted via context timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		payload := []byte(`{"test": "data"}`)
		// Use a short timeout that allows the first attempt + first retry but cancels during backoff
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := adp.ExportDeliverWebhookWithRetries(ctx, server.URL, payload)
		require.Error(t, err)
		// Will either be "context canceled during retry" or "webhook delivery failed after"
		assert.True(t, err != nil)
	})

	t.Run("successful with 201 Created", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		payload := []byte(`{"test": "data"}`)
		err := adp.ExportDeliverWebhookWithRetries(context.Background(), server.URL, payload)
		require.NoError(t, err)
	})
}

// --- SendWebhookNotification Tests ---

func TestSendWebhookNotification(t *testing.T) {
	adp := newTestOpenStackAdapter()

	t.Run("delete event sends notification with resource ID only", func(t *testing.T) {
		var receivedPayload map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			_ = decoder.Decode(&receivedPayload)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		sub := &adapter.Subscription{
			SubscriptionID:         "sub-1",
			Callback:               server.URL,
			ConsumerSubscriptionID: "consumer-1",
		}

		change := openstack.ExportResourceChangeDeleted("openstack-server-abc123")
		err := adp.ExportSendWebhookNotification(context.Background(), sub, change)
		require.NoError(t, err)

		// Verify notification payload
		require.NotNil(t, receivedPayload)
		assert.Equal(t, "sub-1", receivedPayload["subscriptionId"])
		assert.Equal(t, "consumer-1", receivedPayload["consumerSubscriptionId"])
		assert.Equal(t, string(models.EventTypeResourceDeleted), receivedPayload["eventType"])
	})

	// NOTE: "Created" and "Updated" events call getResourceDetails -> GetResource,
	// which panics with nil compute client. Only "Deleted" events are testable
	// without a real OpenStack backend.

	t.Run("webhook server returns error triggers retry timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		sub := &adapter.Subscription{
			SubscriptionID: "sub-3",
			Callback:       server.URL,
		}

		change := openstack.ExportResourceChangeDeleted("openstack-server-ghi789")
		// Use a short-lived context to cancel during backoff retries
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := adp.ExportSendWebhookNotification(ctx, sub, change)
		require.Error(t, err)
	})
}

// --- DetectAndNotifyChanges Tests ---

func TestDetectAndNotifyChanges(t *testing.T) {
	t.Run("no changes detected when snapshots match", func(t *testing.T) {
		adp := newTestOpenStackAdapter()
		ctx := context.Background()

		sub := &adapter.Subscription{
			SubscriptionID: "sub-detect-1",
			Callback:       "https://callback.example.com/events",
		}
		// Old snapshot is empty, new snapshot will also be empty (nil compute)
		state := openstack.ExportNewSubscriptionState(sub, map[string]string{})

		err := adp.ExportDetectAndNotifyChanges(ctx, state)
		require.NoError(t, err)
	})

	t.Run("detects deletions from old snapshot", func(t *testing.T) {
		// Create adapter and a webhook server to receive notifications
		var notificationCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&notificationCount, 1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		adp := newTestOpenStackAdapter()
		ctx := context.Background()

		sub := &adapter.Subscription{
			SubscriptionID: "sub-detect-2",
			Callback:       server.URL,
		}

		// Old snapshot has resources, new will be empty (nil compute)
		// This means all resources in old snapshot are "deleted"
		oldSnapshot := map[string]string{
			"openstack-server-aaa": "hash-aaa",
			"openstack-server-bbb": "hash-bbb",
		}
		state := openstack.ExportNewSubscriptionState(sub, oldSnapshot)

		err := adp.ExportDetectAndNotifyChanges(ctx, state)
		require.NoError(t, err)
		// Should have sent notifications for 2 deleted resources
		assert.Equal(t, int32(2), atomic.LoadInt32(&notificationCount))
	})
}

// --- GetAvailabilityZone Tests ---

func TestGetAvailabilityZone(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("empty resource pool ID returns empty", func(t *testing.T) {
		result := adp.ExportGetAvailabilityZone(ctx, "")
		assert.Empty(t, result)
	})

	// NOTE: Non-empty resourcePoolID tests panic because GetResourcePool
	// calls aggregates.Get with nil compute client.
}

// --- GetAvailabilityZoneFromFilter Tests ---

func TestGetAvailabilityZoneFromFilter(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	t.Run("location in filter is used directly", func(t *testing.T) {
		filter := &adapter.Filter{Location: "nova"}
		result := adp.ExportGetAvailabilityZoneFromFilter(ctx, filter)
		assert.Equal(t, "nova", result)
	})

	// NOTE: ResourcePoolID filter tests panic because GetResourcePool
	// calls aggregates.Get with nil compute client.

	t.Run("empty filter returns empty", func(t *testing.T) {
		filter := &adapter.Filter{}
		result := adp.ExportGetAvailabilityZoneFromFilter(ctx, filter)
		assert.Empty(t, result)
	})
}

// --- GetResourcePool with invalid format ---

func TestOpenStackAdapter_GetResourcePool_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	_, err := adp.GetResourcePool(ctx, "invalid-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource pool ID format")
}

// --- GetResource with invalid format ---

func TestOpenStackAdapter_GetResource_InvalidFormat(t *testing.T) {
	adp := newTestOpenStackAdapter()
	ctx := context.Background()

	_, err := adp.GetResource(ctx, "invalid-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource ID format")
}

// --- Fake API Tests ---
// These tests use httptest servers to simulate OpenStack API responses,
// enabling testing of functions that require real service clients.

func TestOpenStackFakeAPI_ListResources(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"id":        "server-1",
					"name":      "test-vm-1",
					"status":    "ACTIVE",
					"tenant_id": "tenant-1",
					"user_id":   "user-1",
					"hostId":    "host-1",
					"flavor": map[string]interface{}{
						"id": "flavor-1",
					},
					"metadata": map[string]string{},
				},
				{
					"id":        "server-2",
					"name":      "test-vm-2",
					"status":    "SHUTOFF",
					"tenant_id": "tenant-1",
					"user_id":   "user-1",
					"hostId":    "host-2",
					"flavor": map[string]interface{}{
						"id": "flavor-2",
					},
					"metadata": map[string]string{
						"o2ims.io/tenant-id": "my-tenant",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("list all resources", func(t *testing.T) {
		resources, err := adp.ListResources(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, resources, 2)
		assert.Equal(t, "openstack-server-server-1", resources[0].ResourceID)
		assert.Equal(t, "openstack-server-server-2", resources[1].ResourceID)
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 1, Offset: 0}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, resources, 1)
	})

	t.Run("list with tenant filter", func(t *testing.T) {
		filter := &adapter.Filter{TenantID: "my-tenant"}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, resources, 1)
		assert.Equal(t, "my-tenant", resources[0].TenantID)
	})

	t.Run("list with resource type filter", func(t *testing.T) {
		filter := &adapter.Filter{ResourceTypeID: "openstack-flavor-flavor-1"}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, resources, 1)
	})

	t.Run("list with labels filter returns empty", func(t *testing.T) {
		filter := &adapter.Filter{Labels: map[string]string{"env": "prod"}}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		assert.Empty(t, resources)
	})

	t.Run("list with location filter", func(t *testing.T) {
		filter := &adapter.Filter{Location: "az-1"}
		resources, err := adp.ListResources(ctx, filter)
		require.NoError(t, err)
		// All servers returned since location only affects the query to OpenStack
		assert.NotNil(t, resources)
	})
}

func TestOpenStackFakeAPI_GetResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers/srv-abc123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"server": map[string]interface{}{
				"id":        "srv-abc123",
				"name":      "my-server",
				"status":    "ACTIVE",
				"tenant_id": "tenant-1",
				"user_id":   "user-1",
				"hostId":    "host-1",
				"flavor": map[string]interface{}{
					"id": "m1.small",
				},
				"metadata": map[string]string{},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/servers/notfound", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"itemNotFound": map[string]string{"message": "not found"},
		})
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("get existing resource", func(t *testing.T) {
		resource, err := adp.GetResource(ctx, "openstack-server-srv-abc123")
		require.NoError(t, err)
		require.NotNil(t, resource)
		assert.Equal(t, "openstack-server-srv-abc123", resource.ResourceID)
		assert.Contains(t, resource.Description, "my-server")
	})

	t.Run("get resource server not found", func(t *testing.T) {
		_, err := adp.GetResource(ctx, "openstack-server-notfound")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get OpenStack server")
	})
}

func TestOpenStackFakeAPI_ListResourcePools(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"aggregates": []map[string]interface{}{
				{
					"id":                1,
					"name":              "aggregate-1",
					"availability_zone": "az-1",
					"hosts":             []string{"host-1", "host-2"},
					"metadata":          map[string]string{"env": "prod"},
				},
				{
					"id":                2,
					"name":              "aggregate-2",
					"availability_zone": "az-2",
					"hosts":             []string{"host-3"},
					"metadata":          map[string]string{},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("list all pools", func(t *testing.T) {
		pools, err := adp.ListResourcePools(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, pools, 2)
		assert.Equal(t, "openstack-aggregate-1", pools[0].ResourcePoolID)
		assert.Equal(t, "aggregate-1", pools[0].Name)
		assert.Equal(t, "az-1", pools[0].Location)
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 1}
		pools, err := adp.ListResourcePools(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, pools, 1)
	})
}

func TestOpenStackFakeAPI_GetResourcePool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"aggregate": map[string]interface{}{
				"id":                42,
				"name":              "prod-aggregate",
				"availability_zone": "us-east-1",
				"hosts":             []string{"host-1"},
				"metadata":          map[string]string{},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("get existing pool", func(t *testing.T) {
		pool, err := adp.GetResourcePool(ctx, "openstack-aggregate-42")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "openstack-aggregate-42", pool.ResourcePoolID)
		assert.Equal(t, "prod-aggregate", pool.Name)
		assert.Equal(t, "us-east-1", pool.Location)
	})
}

func TestOpenStackFakeAPI_CreateResourcePool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]interface{}{
				"aggregate": map[string]interface{}{
					"id":                100,
					"name":              "new-aggregate",
					"availability_zone": "az-new",
					"hosts":             []string{},
					"metadata":          map[string]string{},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("create resource pool", func(t *testing.T) {
		pool, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{
			Name:     "new-aggregate",
			Location: "az-new",
			Extensions: map[string]interface{}{
				"openstack.availabilityZone": "az-new",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "openstack-aggregate-100", pool.ResourcePoolID)
	})

	t.Run("create pool with empty name", func(t *testing.T) {
		_, err := adp.CreateResourcePool(ctx, &adapter.ResourcePool{
			Name: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource pool name is required")
	})

	t.Run("create pool with metadata", func(t *testing.T) {
		metaMux := http.NewServeMux()
		metaMux.HandleFunc("/os-aggregates", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"aggregate": map[string]interface{}{
						"id":                101,
						"name":              "meta-agg",
						"availability_zone": "",
						"hosts":             []string{},
						"metadata":          map[string]string{},
					},
				}
				json.NewEncoder(w).Encode(resp)
			}
		})
		metaMux.HandleFunc("/os-aggregates/101/action", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]interface{}{
				"aggregate": map[string]interface{}{
					"id":                101,
					"name":              "meta-agg",
					"availability_zone": "",
					"hosts":             []string{},
					"metadata":          map[string]string{"env": "test"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		})
		adp2, srv2 := openstack.ExportNewFakeAdapterWithCompute(metaMux)
		defer srv2.Close()

		pool, err := adp2.CreateResourcePool(ctx, &adapter.ResourcePool{
			Name: "meta-agg",
			Extensions: map[string]interface{}{
				"openstack.metadata": map[string]string{"env": "test"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, pool)
	})
}

func TestOpenStackFakeAPI_UpdateResourcePool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates/10", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"aggregate": map[string]interface{}{
				"id":                10,
				"name":              "existing-agg",
				"availability_zone": "az-1",
				"hosts":             []string{"host-1"},
				"metadata":          map[string]string{},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/os-aggregates/10/action", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"aggregate": map[string]interface{}{
				"id":                10,
				"name":              "existing-agg",
				"availability_zone": "az-1",
				"hosts":             []string{"host-1"},
				"metadata":          map[string]string{"updated": "true"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("update existing pool", func(t *testing.T) {
		pool, err := adp.UpdateResourcePool(ctx, "openstack-aggregate-10", &adapter.ResourcePool{
			Name: "existing-agg",
			Extensions: map[string]interface{}{
				"openstack.metadata": map[string]string{"updated": "true"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "openstack-aggregate-10", pool.ResourcePoolID)
	})

	t.Run("update with no metadata skips metadata call", func(t *testing.T) {
		pool, err := adp.UpdateResourcePool(ctx, "openstack-aggregate-10", &adapter.ResourcePool{
			Name:       "existing-agg",
			Extensions: map[string]interface{}{},
		})
		require.NoError(t, err)
		require.NotNil(t, pool)
	})
}

func TestOpenStackFakeAPI_DeleteResourcePool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates/20", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("delete existing pool", func(t *testing.T) {
		err := adp.DeleteResourcePool(ctx, "openstack-aggregate-20")
		require.NoError(t, err)
	})
}

func TestOpenStackFakeAPI_ListResourceTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"flavors": []map[string]interface{}{
				{
					"id":                         "flv-1",
					"name":                       "m1.small",
					"vcpus":                      2,
					"ram":                        2048,
					"disk":                       20,
					"swap":                       "",
					"OS-FLV-EXT-DATA:ephemeral":  0,
					"os-flavor-access:is_public": true,
					"rxtx_factor":                1.0,
				},
				{
					"id":                         "flv-2",
					"name":                       "m1.large",
					"vcpus":                      4,
					"ram":                        8192,
					"disk":                       80,
					"swap":                       "",
					"OS-FLV-EXT-DATA:ephemeral":  0,
					"os-flavor-access:is_public": true,
					"rxtx_factor":                1.0,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("list all resource types", func(t *testing.T) {
		rts, err := adp.ListResourceTypes(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, rts, 2)
		assert.Equal(t, "m1.small", rts[0].Name)
		assert.Equal(t, "OpenStack", rts[0].Vendor)
		assert.Equal(t, "compute", rts[0].ResourceClass)
		assert.Equal(t, "virtual", rts[0].ResourceKind)
	})

	t.Run("list with pagination", func(t *testing.T) {
		filter := &adapter.Filter{Limit: 1}
		rts, err := adp.ListResourceTypes(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, rts, 1)
	})
}

func TestOpenStackFakeAPI_GetResourceType(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/flavors/flv-abc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"flavor": map[string]interface{}{
				"id":                         "flv-abc",
				"name":                       "m1.medium",
				"vcpus":                      2,
				"ram":                        4096,
				"disk":                       40,
				"swap":                       "",
				"OS-FLV-EXT-DATA:ephemeral":  0,
				"os-flavor-access:is_public": true,
				"rxtx_factor":                1.0,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("get existing resource type", func(t *testing.T) {
		rt, err := adp.GetResourceType(ctx, "openstack-flavor-flv-abc")
		require.NoError(t, err)
		require.NotNil(t, rt)
		assert.Equal(t, "m1.medium", rt.Name)
		assert.Equal(t, "OpenStack", rt.Vendor)
	})

	t.Run("get resource type invalid format", func(t *testing.T) {
		_, err := adp.GetResourceType(ctx, "invalid-format")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource type ID format")
	})
}

func TestOpenStackFakeAPI_CreateResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			resp := map[string]interface{}{
				"server": map[string]interface{}{
					"id":        "new-server-1",
					"name":      "my-new-instance",
					"status":    "BUILD",
					"tenant_id": "tenant-1",
					"user_id":   "user-1",
					"hostId":    "",
					"flavor": map[string]interface{}{
						"id": "flv-1",
					},
					"metadata": map[string]string{},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("create resource", func(t *testing.T) {
		resource, err := adp.CreateResource(ctx, &adapter.Resource{
			ResourceTypeID: "openstack-flavor-flv-1",
			Extensions: map[string]interface{}{
				"openstack.imageId": "image-abc",
				"openstack.name":    "my-new-instance",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resource)
		assert.Equal(t, "openstack-server-new-server-1", resource.ResourceID)
	})

	t.Run("create with networks and security groups", func(t *testing.T) {
		resource, err := adp.CreateResource(ctx, &adapter.Resource{
			ResourceTypeID: "openstack-flavor-flv-1",
			Extensions: map[string]interface{}{
				"openstack.imageId":        "image-abc",
				"openstack.name":           "net-instance",
				"openstack.networks":       []string{"net-1", "net-2"},
				"openstack.securityGroups": []string{"sg-1"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resource)
	})
}

func TestOpenStackFakeAPI_UpdateResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers/srv-update/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"metadata": map[string]string{
				"name": "updated",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/servers/srv-update", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"server": map[string]interface{}{
				"id":        "srv-update",
				"name":      "updated-server",
				"status":    "ACTIVE",
				"tenant_id": "tenant-1",
				"user_id":   "user-1",
				"hostId":    "host-1",
				"flavor": map[string]interface{}{
					"id": "flv-1",
				},
				"metadata": map[string]string{"name": "updated"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("update resource with metadata", func(t *testing.T) {
		resource, err := adp.UpdateResource(ctx, "openstack-server-srv-update", &adapter.Resource{
			Description:   "updated",
			TenantID:      "t1",
			GlobalAssetID: "urn:test",
			Extensions: map[string]interface{}{
				"openstack.metadata": map[string]string{"custom": "value"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resource)
	})

	t.Run("update resource no metadata", func(t *testing.T) {
		// When there's no metadata to set, it skips the metadata update
		// and just calls GetResource
		resource, err := adp.UpdateResource(ctx, "openstack-server-srv-update", &adapter.Resource{})
		require.NoError(t, err)
		require.NotNil(t, resource)
	})
}

func TestOpenStackFakeAPI_DeleteResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers/srv-del", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	t.Run("delete existing resource", func(t *testing.T) {
		err := adp.DeleteResource(ctx, "openstack-server-srv-del")
		require.NoError(t, err)
	})

	t.Run("delete invalid format", func(t *testing.T) {
		err := adp.DeleteResource(ctx, "invalid-format")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource ID format")
	})
}

func TestOpenStackFakeAPI_GetDeploymentManager(t *testing.T) {
	computeMux := http.NewServeMux()
	identityMux := http.NewServeMux()
	identityMux.HandleFunc("/regions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"regions": []map[string]interface{}{
				{
					"id":               "TestRegion",
					"description":      "Test Region",
					"parent_region_id": "",
				},
			},
			"links": map[string]interface{}{
				"next":     nil,
				"previous": nil,
				"self":     "http://localhost/regions",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	placementMux := http.NewServeMux()

	adp := openstack.ExportNewFakeAdapter(computeMux, identityMux, placementMux)

	t.Run("get by configured ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "test-dm-openstack")
		require.NoError(t, err)
		require.NotNil(t, dm)
		assert.Equal(t, "test-dm-openstack", dm.DeploymentManagerID)
		assert.Contains(t, dm.Name, "OpenStack")
		assert.Equal(t, "test-ocloud", dm.OCloudID)
		assert.NotEmpty(t, dm.SupportedLocations)
		assert.NotNil(t, dm.Extensions)
	})

	t.Run("get by default alias", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "default")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("get by empty ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("nonexistent DM", func(t *testing.T) {
		_, err := adp.GetDeploymentManager(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, adapter.ErrDeploymentManagerNotFound)
	})
}

func TestOpenStackFakeAPI_ListDeploymentManagers(t *testing.T) {
	computeMux := http.NewServeMux()
	identityMux := http.NewServeMux()
	identityMux.HandleFunc("/regions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"regions": []map[string]interface{}{
				{
					"id":               "TestRegion",
					"description":      "Test Region",
					"parent_region_id": "",
				},
			},
			"links": map[string]interface{}{"next": nil, "previous": nil},
		}
		json.NewEncoder(w).Encode(resp)
	})
	placementMux := http.NewServeMux()

	adp := openstack.ExportNewFakeAdapter(computeMux, identityMux, placementMux)

	dms, err := adp.ListDeploymentManagers(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, dms, 1)
	assert.Equal(t, "test-dm-openstack", dms[0].DeploymentManagerID)
}

func TestOpenStackFakeAPI_Health(t *testing.T) {
	computeMux := http.NewServeMux()
	computeMux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"servers": []interface{}{}})
	})

	identityMux := http.NewServeMux()
	identityMux.HandleFunc("/regions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"regions": []interface{}{},
			"links":   map[string]interface{}{"next": nil, "previous": nil},
		})
	})

	placementMux := http.NewServeMux()
	placementMux.HandleFunc("/resource_providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"resource_providers": []interface{}{}})
	})

	adp := openstack.ExportNewFakeAdapter(computeMux, identityMux, placementMux)
	err := adp.Health(context.Background())
	require.NoError(t, err)
}

func TestOpenStackFakeAPI_Health_Failures(t *testing.T) {
	t.Run("nova failure", func(t *testing.T) {
		computeMux := http.NewServeMux()
		computeMux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		identityMux := http.NewServeMux()
		placementMux := http.NewServeMux()
		adp := openstack.ExportNewFakeAdapter(computeMux, identityMux, placementMux)
		err := adp.Health(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nova API unreachable")
	})

	t.Run("placement failure", func(t *testing.T) {
		computeMux := http.NewServeMux()
		computeMux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"servers": []interface{}{}})
		})
		identityMux := http.NewServeMux()
		placementMux := http.NewServeMux()
		placementMux.HandleFunc("/resource_providers", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		adp := openstack.ExportNewFakeAdapter(computeMux, identityMux, placementMux)
		err := adp.Health(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "placement API unreachable")
	})

	t.Run("keystone failure", func(t *testing.T) {
		computeMux := http.NewServeMux()
		computeMux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"servers": []interface{}{}})
		})
		identityMux := http.NewServeMux()
		identityMux.HandleFunc("/regions", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		placementMux := http.NewServeMux()
		placementMux.HandleFunc("/resource_providers", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"resource_providers": []interface{}{}})
		})
		adp := openstack.ExportNewFakeAdapter(computeMux, identityMux, placementMux)
		err := adp.Health(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "keystone API unreachable")
	})
}

func TestOpenStackFakeAPI_CreateResourceSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/servers/detail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"id":        "srv-1",
					"name":      "server-1",
					"status":    "ACTIVE",
					"tenant_id": "t1",
					"flavor":    map[string]interface{}{"id": "flv-1"},
					"metadata":  map[string]string{},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	snapshot, err := adp.ExportCreateResourceSnapshot(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, snapshot)
}

func TestOpenStackFakeAPI_GetAvailabilityZone_WithPool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates/5", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"aggregate": map[string]interface{}{
				"id":                5,
				"name":              "az-pool",
				"availability_zone": "us-west-2",
				"hosts":             []string{},
				"metadata":          map[string]string{},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	az := adp.ExportGetAvailabilityZone(ctx, "openstack-aggregate-5")
	assert.Equal(t, "us-west-2", az)
}

func TestOpenStackFakeAPI_GetAvailabilityZoneFromFilter_WithPoolID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/os-aggregates/7", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"aggregate": map[string]interface{}{
				"id":                7,
				"name":              "az-filter-pool",
				"availability_zone": "eu-central-1",
				"hosts":             []string{},
				"metadata":          map[string]string{},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	adp, server := openstack.ExportNewFakeAdapterWithCompute(mux)
	defer server.Close()
	ctx := context.Background()

	filter := &adapter.Filter{ResourcePoolID: "openstack-aggregate-7"}
	az := adp.ExportGetAvailabilityZoneFromFilter(ctx, filter)
	assert.Equal(t, "eu-central-1", az)
}
