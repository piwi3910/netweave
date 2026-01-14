package azure_test

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/piwi3910/netweave/internal/adapter"
	azadapter "github.com/piwi3910/netweave/internal/adapters/azure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new AzureAdapter.
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *azadapter.Config
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
			config: &azadapter.Config{
				Location: "eastus",
				OCloudID: "ocloud-1",
			},
			wantErr: true,
			errMsg:  "subscriptionID is required",
		},
		{
			name: "missing location",
			config: &azadapter.Config{
				SubscriptionID: "sub-123",
				OCloudID:       "ocloud-1",
			},
			wantErr: true,
			errMsg:  "location is required",
		},
		{
			name: "missing oCloudID",
			config: &azadapter.Config{
				SubscriptionID: "sub-123",
				Location:       "eastus",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "missing tenantID without managed identity",
			config: &azadapter.Config{
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
			config: &azadapter.Config{
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
			config: &azadapter.Config{
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
			config: &azadapter.Config{
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
			adp, err := azadapter.New(tt.config)

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
	adp := &azadapter.Adapter{
		Logger: zap.NewNop(),
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
			got := azadapter.GenerateVMSizeID(tt.vmSize)
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
			got := azadapter.GenerateVMID(tt.vmName, tt.rg)
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
			got := azadapter.GenerateRGPoolID(tt.rg)
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
			got := azadapter.GenerateAZPoolID(tt.location, tt.zone)
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
			got := azadapter.ExtractVMFamily(tt.sizeName)
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
			got := azadapter.ExtractResourceGroup(tt.resourceID)
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
			got := azadapter.TagsToMap(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSubscriptions tests subscription CRUD operations.
func TestSubscriptions(t *testing.T) {
	adp := &azadapter.Adapter{
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

// TestClose tests adapter cleanup.
func TestClose(t *testing.T) {
	adp := &azadapter.Adapter{
		Logger:        zap.NewNop(),
		Subscriptions: make(map[string]*adapter.Subscription),
	}

	// Add some subscriptions
	adp.Subscriptions["sub-1"] = &adapter.Subscription{SubscriptionID: "sub-1"}
	adp.Subscriptions["sub-2"] = &adapter.Subscription{SubscriptionID: "sub-2"}

	err := adp.Close()
	assert.NoError(t, err)

	// Verify subscriptions are cleared
	assert.Empty(t, adp.Subscriptions)
}

// TestPtrHelpers tests pointer helper functions.
func TestPtrHelpers(t *testing.T) {
	t.Run("ptrToString", func(t *testing.T) {
		s := "hello"
		assert.Equal(t, "hello", azadapter.PtrToString(&s))
		assert.Equal(t, "", azadapter.PtrToString(nil))
	})

	t.Run("ptrToInt32", func(t *testing.T) {
		i := int32(42)
		assert.Equal(t, int32(42), azadapter.PtrToInt32(&i))
		assert.Equal(t, int32(0), azadapter.PtrToInt32(nil))
	})
}

// NOTE: BenchmarkMatchesFilter and BenchmarkApplyPagination moved to internal/adapter/helpers_test.go
// TestAzureAdapter_Health tests the Health function.
func TestAzureAdapter_Health(t *testing.T) {
	adapter, err := azadapter.New(&azadapter.Config{
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
	adapter, err := azadapter.New(&azadapter.Config{
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
	adapter, err := azadapter.New(&azadapter.Config{
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
	adapter, err := azadapter.New(&azadapter.Config{
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
	adapter, err := azadapter.New(&azadapter.Config{
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

// TestBuildAzureTags tests Azure tag building from resource fields.
func TestBuildAzureTags(t *testing.T) {
	adp := &azadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name     string
		resource *adapter.Resource
		wantLen  int
		checkTag func(t *testing.T, tags map[string]*string)
	}{
		{
			name: "all fields populated",
			resource: &adapter.Resource{
				TenantID:      "tenant-123",
				Description:   "test VM",
				GlobalAssetID: "urn:azure:vm:sub:rg:vm-123",
				Extensions: map[string]interface{}{
					"azure.tags": map[string]string{
						"Environment": "production",
						"Team":        "platform",
					},
				},
			},
			wantLen: 5,
			checkTag: func(t *testing.T, tags map[string]*string) {
				assert.NotNil(t, tags["o2ims.io/tenant-id"])
				assert.Equal(t, "tenant-123", *tags["o2ims.io/tenant-id"])
				assert.Equal(t, "test VM", *tags["Name"])
				assert.Equal(t, "urn:azure:vm:sub:rg:vm-123", *tags["GlobalAssetID"])
				assert.Equal(t, "production", *tags["Environment"])
				assert.Equal(t, "platform", *tags["Team"])
			},
		},
		{
			name:     "empty resource",
			resource: &adapter.Resource{},
			wantLen:  0,
		},
		{
			name: "only tenant ID",
			resource: &adapter.Resource{
				TenantID: "tenant-456",
			},
			wantLen: 1,
			checkTag: func(t *testing.T, tags map[string]*string) {
				assert.NotNil(t, tags["o2ims.io/tenant-id"])
				assert.Equal(t, "tenant-456", *tags["o2ims.io/tenant-id"])
			},
		},
		{
			name: "custom tags only",
			resource: &adapter.Resource{
				Extensions: map[string]interface{}{
					"azure.tags": map[string]string{
						"CustomKey": "CustomValue",
					},
				},
			},
			wantLen: 1,
			checkTag: func(t *testing.T, tags map[string]*string) {
				assert.NotNil(t, tags["CustomKey"])
				assert.Equal(t, "CustomValue", *tags["CustomKey"])
			},
		},
		{
			name: "wrong custom tags type",
			resource: &adapter.Resource{
				Extensions: map[string]interface{}{
					"azure.tags": "not-a-map",
				},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildAzureTags(tt.resource)
			assert.Len(t, got, tt.wantLen)
			if tt.checkTag != nil {
				tt.checkTag(t, got)
			}
		})
	}
}

// TestStringPtr tests the StringPtr utility function.
func TestStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "non-empty string",
			input: "test",
			want:  "test",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azadapter.StringPtr(tt.input)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

// TestExtractVMSize tests VM size extraction.
func TestExtractVMSize(t *testing.T) {
	adp := &azadapter.Adapter{Logger: zap.NewNop()}

	vmSize := armcompute.VirtualMachineSizeTypes("Standard_D2s_v3")

	tests := []struct {
		name string
		vm   *armcompute.VirtualMachine
		want string
	}{
		{
			name: "with VM size",
			vm: &armcompute.VirtualMachine{
				Properties: &armcompute.VirtualMachineProperties{
					HardwareProfile: &armcompute.HardwareProfile{
						VMSize: &vmSize,
					},
				},
			},
			want: "Standard_D2s_v3",
		},
		{
			name: "nil properties",
			vm:   &armcompute.VirtualMachine{},
			want: "",
		},
		{
			name: "nil hardware profile",
			vm: &armcompute.VirtualMachine{
				Properties: &armcompute.VirtualMachineProperties{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestExtractVMSize(tt.vm)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDetermineResourcePoolID tests resource pool ID determination.
func TestDetermineResourcePoolID(t *testing.T) {
	zone1 := "1"
	zone2 := "2"

	tests := []struct {
		name          string
		poolMode      string
		vm            *armcompute.VirtualMachine
		location      string
		resourceGroup string
		want          string
	}{
		{
			name:          "RG mode",
			poolMode:      "rg",
			vm:            &armcompute.VirtualMachine{},
			location:      "eastus",
			resourceGroup: "myRG",
			want:          "azure-rg-myRG",
		},
		{
			name:     "AZ mode with zone",
			poolMode: "az",
			vm: &armcompute.VirtualMachine{
				Zones: []*string{&zone2},
			},
			location:      "eastus",
			resourceGroup: "myRG",
			want:          "azure-az-eastus-2",
		},
		{
			name:          "AZ mode without zone",
			poolMode:      "az",
			vm:            &armcompute.VirtualMachine{},
			location:      "westus",
			resourceGroup: "myRG",
			want:          "azure-az-westus-1",
		},
		{
			name:     "AZ mode multiple zones",
			poolMode: "az",
			vm: &armcompute.VirtualMachine{
				Zones: []*string{&zone1, &zone2},
			},
			location:      "centralus",
			resourceGroup: "myRG",
			want:          "azure-az-centralus-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := &azadapter.Adapter{
				Logger: zap.NewNop(),
			}
			adp.TestSetPoolMode(tt.poolMode)
			got := adp.TestDetermineResourcePoolID(tt.vm, tt.location, tt.resourceGroup)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildVMExtensions tests VM extensions building.
func TestBuildVMExtensions(t *testing.T) {
	provState := "Succeeded"
	vmID := "unique-vm-id"
	zone1 := "1"

	tests := []struct {
		name          string
		vm            *armcompute.VirtualMachine
		vmName        string
		location      string
		resourceGroup string
		vmSize        string
		checkExts     func(t *testing.T, exts map[string]interface{})
	}{
		{
			name: "complete VM",
			vm: &armcompute.VirtualMachine{
				ID: azadapter.StringPtr("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1"),
				Properties: &armcompute.VirtualMachineProperties{
					ProvisioningState: &provState,
					VMID:              &vmID,
				},
				Zones: []*string{&zone1},
				Tags: map[string]*string{
					"Environment": azadapter.StringPtr("test"),
				},
			},
			vmName:        "vm1",
			location:      "eastus",
			resourceGroup: "myRG",
			vmSize:        "Standard_D2s_v3",
			checkExts: func(t *testing.T, exts map[string]interface{}) {
				assert.Equal(t, "vm1", exts["azure.vmName"])
				assert.Equal(t, "myRG", exts["azure.resourceGroup"])
				assert.Equal(t, "eastus", exts["azure.location"])
				assert.Equal(t, "Standard_D2s_v3", exts["azure.vmSize"])
				assert.Equal(t, "Succeeded", exts["azure.provisioningState"])
				assert.Equal(t, "unique-vm-id", exts["azure.vmUniqueId"])
				assert.Equal(t, "1", exts["azure.availabilityZone"])
				assert.Contains(t, exts, "azure.tags")
			},
		},
		{
			name:          "minimal VM",
			vm:            &armcompute.VirtualMachine{},
			vmName:        "vm2",
			location:      "westus",
			resourceGroup: "testRG",
			vmSize:        "Standard_B1s",
			checkExts: func(t *testing.T, exts map[string]interface{}) {
				assert.Equal(t, "vm2", exts["azure.vmName"])
				assert.Equal(t, "testRG", exts["azure.resourceGroup"])
				assert.Equal(t, "westus", exts["azure.location"])
				assert.Equal(t, "Standard_B1s", exts["azure.vmSize"])
				assert.NotContains(t, exts, "azure.availabilityZone")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := &azadapter.Adapter{Logger: zap.NewNop()}
			got := adp.TestBuildVMExtensions(tt.vm, tt.vmName, tt.location, tt.resourceGroup, tt.vmSize)
			require.NotNil(t, got)
			if tt.checkExts != nil {
				tt.checkExts(t, got)
			}
		})
	}
}
