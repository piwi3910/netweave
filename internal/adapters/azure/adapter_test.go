package azure

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new AzureAdapter.
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
			name: "missing subscriptionID",
			config: &Config{
				Location: "eastus",
				OCloudID: "ocloud-1",
			},
			wantErr: true,
			errMsg:  "subscriptionID is required",
		},
		{
			name: "missing location",
			config: &Config{
				SubscriptionID: "sub-123",
				OCloudID:       "ocloud-1",
			},
			wantErr: true,
			errMsg:  "location is required",
		},
		{
			name: "missing oCloudID",
			config: &Config{
				SubscriptionID: "sub-123",
				Location:       "eastus",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "missing tenantID without managed identity",
			config: &Config{
				SubscriptionID:     "sub-123",
				Location:           "eastus",
				OCloudID:           "ocloud-1",
				UseManagedIdentity: false,
			},
			wantErr: true,
			errMsg:  "tenantID is required",
		},
		{
			name: "missing clientID without managed identity",
			config: &Config{
				SubscriptionID:     "sub-123",
				Location:           "eastus",
				OCloudID:           "ocloud-1",
				TenantID:           "tenant-123",
				UseManagedIdentity: false,
			},
			wantErr: true,
			errMsg:  "clientID is required",
		},
		{
			name: "missing clientSecret without managed identity",
			config: &Config{
				SubscriptionID:     "sub-123",
				Location:           "eastus",
				OCloudID:           "ocloud-1",
				TenantID:           "tenant-123",
				ClientID:           "client-123",
				UseManagedIdentity: false,
			},
			wantErr: true,
			errMsg:  "clientSecret is required",
		},
		{
			name: "invalid pool mode",
			config: &Config{
				SubscriptionID:     "sub-123",
				Location:           "eastus",
				OCloudID:           "ocloud-1",
				TenantID:           "tenant-123",
				ClientID:           "client-123",
				ClientSecret:       "secret-123",
				PoolMode:           "invalid",
				UseManagedIdentity: false,
			},
			wantErr: true,
			errMsg:  "poolMode must be 'rg' or 'az'",
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
	adp := &AzureAdapter{
		logger: zap.NewNop(),
	}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "azure", adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.NotEmpty(t, adp.Version())
		assert.Equal(t, "compute-2023-09-01", adp.Version())
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
	t.Run("generateVMSizeID", func(t *testing.T) {
		tests := []struct {
			vmSize string
			want   string
		}{
			{"Standard_D2s_v3", "azure-vm-size-Standard_D2s_v3"},
			{"Standard_B2ms", "azure-vm-size-Standard_B2ms"},
			{"", "azure-vm-size-"},
		}

		for _, tt := range tests {
			got := generateVMSizeID(tt.vmSize)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateVMID", func(t *testing.T) {
		tests := []struct {
			vmName string
			rg     string
			want   string
		}{
			{"my-vm", "my-rg", "azure-vm-my-rg-my-vm"},
			{"vm1", "rg1", "azure-vm-rg1-vm1"},
		}

		for _, tt := range tests {
			got := generateVMID(tt.vmName, tt.rg)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateRGPoolID", func(t *testing.T) {
		tests := []struct {
			rg   string
			want string
		}{
			{"my-resource-group", "azure-rg-my-resource-group"},
			{"prod-rg", "azure-rg-prod-rg"},
		}

		for _, tt := range tests {
			got := generateRGPoolID(tt.rg)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateAZPoolID", func(t *testing.T) {
		tests := []struct {
			location string
			zone     string
			want     string
		}{
			{"eastus", "1", "azure-az-eastus-1"},
			{"westeurope", "2", "azure-az-westeurope-2"},
		}

		for _, tt := range tests {
			got := generateAZPoolID(tt.location, tt.zone)
			assert.Equal(t, tt.want, got)
		}
	})
}

// TestExtractVMFamily tests VM family extraction.
func TestExtractVMFamily(t *testing.T) {
	tests := []struct {
		sizeName string
		want     string
	}{
		{"Standard_D2s_v3", "D"},
		{"Standard_B2ms", "B"},
		{"Standard_E4s_v3", "E"},
		{"Standard_NC6", "NC"},
		{"Basic_A0", "A"},
		{"Standard_M128ms", "M"},
	}

	for _, tt := range tests {
		t.Run(tt.sizeName, func(t *testing.T) {
			got := extractVMFamily(tt.sizeName)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractResourceGroup tests resource group extraction from Azure resource IDs.
func TestExtractResourceGroup(t *testing.T) {
	tests := []struct {
		resourceID string
		want       string
	}{
		{
			resourceID: "/subscriptions/12345/resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachines/my-vm",
			want:       "my-rg",
		},
		{
			resourceID: "/subscriptions/abc-123/resourcegroups/prod-rg/providers/Microsoft.Compute/virtualMachines/vm1",
			want:       "prod-rg",
		},
		{
			resourceID: "",
			want:       "",
		},
		{
			resourceID: "/subscriptions/12345",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.resourceID, func(t *testing.T) {
			got := extractResourceGroup(tt.resourceID)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTagsToMap tests Azure tags conversion.
func TestTagsToMap(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name string
		tags map[string]*string
		want map[string]string
	}{
		{
			name: "nil tags",
			tags: nil,
			want: map[string]string{},
		},
		{
			name: "empty tags",
			tags: map[string]*string{},
			want: map[string]string{},
		},
		{
			name: "valid tags",
			tags: map[string]*string{
				"env":  strPtr("prod"),
				"team": strPtr("devops"),
			},
			want: map[string]string{
				"env":  "prod",
				"team": "devops",
			},
		},
		{
			name: "tags with nil value",
			tags: map[string]*string{
				"env": strPtr("prod"),
				"nil": nil,
			},
			want: map[string]string{
				"env": "prod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsToMap(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSubscriptions tests subscription CRUD operations.
func TestSubscriptions(t *testing.T) {
	adp := &AzureAdapter{
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

// TestClose tests adapter cleanup.
func TestClose(t *testing.T) {
	adp := &AzureAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	// Add some subscriptions
	adp.subscriptions["sub-1"] = &adapter.Subscription{SubscriptionID: "sub-1"}
	adp.subscriptions["sub-2"] = &adapter.Subscription{SubscriptionID: "sub-2"}

	err := adp.Close()
	assert.NoError(t, err)

	// Verify subscriptions are cleared
	assert.Empty(t, adp.subscriptions)
}

// TestPtrHelpers tests pointer helper functions.
func TestPtrHelpers(t *testing.T) {
	t.Run("ptrToString", func(t *testing.T) {
		s := "hello"
		assert.Equal(t, "hello", ptrToString(&s))
		assert.Equal(t, "", ptrToString(nil))
	})

	t.Run("ptrToInt32", func(t *testing.T) {
		i := int32(42)
		assert.Equal(t, int32(42), ptrToInt32(&i))
		assert.Equal(t, int32(0), ptrToInt32(nil))
	})
}

// NOTE: BenchmarkMatchesFilter and BenchmarkApplyPagination moved to internal/adapter/helpers_test.go
// TestAzureAdapter_Health tests the Health function.
func TestAzureAdapter_Health(t *testing.T) {
	adapter, err := New(&Config{
		SubscriptionID:     "test-sub",
		Location:           "eastus",
		OCloudID:           "test-cloud",
		UseManagedIdentity: true,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err = adapter.Health(ctx)
	if err != nil {
		t.Skip("Skipping - requires Azure credentials")
	}
}

// TestAzureAdapter_ListResourcePools tests the ListResourcePools function.
func TestAzureAdapter_ListResourcePools(t *testing.T) {
	adapter, err := New(&Config{
		SubscriptionID:     "test-sub",
		Location:           "eastus",
		OCloudID:           "test-cloud",
		PoolMode:           "rg",
		UseManagedIdentity: true,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pools, err := adapter.ListResourcePools(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires Azure credentials")
	}
	assert.NotNil(t, pools)
}

// TestAzureAdapter_ListResources tests the ListResources function.
func TestAzureAdapter_ListResources(t *testing.T) {
	adapter, err := New(&Config{
		SubscriptionID:     "test-sub",
		Location:           "eastus",
		OCloudID:           "test-cloud",
		UseManagedIdentity: true,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resources, err := adapter.ListResources(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires Azure credentials")
	}
	assert.NotNil(t, resources)
}

// TestAzureAdapter_ListResourceTypes tests the ListResourceTypes function.
func TestAzureAdapter_ListResourceTypes(t *testing.T) {
	adapter, err := New(&Config{
		SubscriptionID:     "test-sub",
		Location:           "eastus",
		OCloudID:           "test-cloud",
		UseManagedIdentity: true,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	types, err := adapter.ListResourceTypes(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires Azure credentials")
	}
	assert.NotNil(t, types)
}

// TestAzureAdapter_GetDeploymentManager tests the GetDeploymentManager function.
func TestAzureAdapter_GetDeploymentManager(t *testing.T) {
	adapter, err := New(&Config{
		SubscriptionID:     "test-sub",
		Location:           "eastus",
		OCloudID:           "test-cloud",
		UseManagedIdentity: true,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm, err := adapter.GetDeploymentManager(ctx, "dm-1")
	if err != nil {
		t.Skip("Skipping - requires Azure credentials")
	}
	assert.NotNil(t, dm)
}
