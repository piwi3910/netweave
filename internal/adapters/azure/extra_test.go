package azure

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// helper to create a minimal test adapter without Azure credentials.
func newTestAdapter(poolMode string) *Adapter {
	if poolMode == "" {
		poolMode = "rg"
	}
	return &Adapter{
		logger:              zap.NewNop(),
		oCloudID:            "test-ocloud",
		deploymentManagerID: "ocloud-azure-eastus",
		subscriptionID:      "sub-123",
		location:            "eastus",
		subscriptions:       make(map[string]*adapter.Subscription),
		poolMode:            poolMode,
	}
}

func strPtr(s string) *string { return &s }

func int32Ptr(i int32) *int32 { return &i }

// --- Tests for GetDeploymentManager ---

func TestGetDeploymentManager_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("with configured ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "ocloud-azure-eastus")
		require.NoError(t, err)
		require.NotNil(t, dm)
		assert.Equal(t, "ocloud-azure-eastus", dm.DeploymentManagerID)
		assert.Equal(t, "Azure eastus", dm.Name)
		assert.Contains(t, dm.Description, "eastus")
		assert.Equal(t, "test-ocloud", dm.OCloudID)
		assert.Contains(t, dm.ServiceURI, "sub-123")
		assert.Len(t, dm.SupportedLocations, 4) // eastus + 3 zones
		assert.Contains(t, dm.Extensions, "azure.subscriptionId")
		assert.Equal(t, "sub-123", dm.Extensions["azure.subscriptionId"])
		assert.Equal(t, "eastus", dm.Extensions["azure.location"])
		assert.Equal(t, "rg", dm.Extensions["azure.poolMode"])
		assert.Contains(t, dm.Extensions["azure.portalUrl"], "sub-123")
		assert.Len(t, dm.Capabilities, 4)
	})

	t.Run("with default alias", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "default")
		require.NoError(t, err)
		require.NotNil(t, dm)
		assert.Equal(t, "ocloud-azure-eastus", dm.DeploymentManagerID)
	})

	t.Run("with empty ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("with wrong ID", func(t *testing.T) {
		_, err := adp.GetDeploymentManager(context.Background(), "wrong-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deployment manager wrong-id")
	})
}

// --- Tests for ListDeploymentManagers ---

func TestListDeploymentManagers_Internal(t *testing.T) {
	adp := newTestAdapter("")

	dms, err := adp.ListDeploymentManagers(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, dms, 1)
	assert.Equal(t, "ocloud-azure-eastus", dms[0].DeploymentManagerID)
}

// --- Tests for listAZPools ---

func TestListAZPools_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	t.Run("no filter", func(t *testing.T) {
		pools := adp.listAZPools(context.Background(), nil)
		require.Len(t, pools, 3)
		for i, pool := range pools {
			assert.NotEmpty(t, pool.ResourcePoolID)
			assert.NotEmpty(t, pool.Name)
			assert.NotEmpty(t, pool.Location)
			assert.Equal(t, "test-ocloud", pool.OCloudID)
			assert.Contains(t, pool.Extensions, "azure.zone")
			assert.Contains(t, pool.Extensions, "azure.location")
			assert.Equal(t, "AvailabilityZone", pool.Extensions["azure.type"])
			expectedZone := string(rune('1' + i))
			assert.Contains(t, pool.Name, expectedZone)
		}
	})

	t.Run("with pagination limit", func(t *testing.T) {
		pools := adp.listAZPools(context.Background(), &adapter.Filter{Limit: 2})
		assert.Len(t, pools, 2)
	})

	t.Run("with pagination offset", func(t *testing.T) {
		pools := adp.listAZPools(context.Background(), &adapter.Filter{Limit: 1, Offset: 2})
		require.Len(t, pools, 1)
		assert.Contains(t, pools[0].Name, "3")
	})

	t.Run("with location filter matching", func(t *testing.T) {
		pools := adp.listAZPools(context.Background(), &adapter.Filter{Location: "eastus-1"})
		assert.Len(t, pools, 1)
		assert.Equal(t, "eastus-1", pools[0].Location)
	})

	t.Run("with location filter not matching", func(t *testing.T) {
		pools := adp.listAZPools(context.Background(), &adapter.Filter{Location: "westus-1"})
		assert.Len(t, pools, 0)
	})
}

// --- Tests for getAZPool ---

func TestGetAZPool_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	t.Run("found", func(t *testing.T) {
		pool, err := adp.getAZPool(context.Background(), "azure-az-eastus-1")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "azure-az-eastus-1", pool.ResourcePoolID)
		assert.Equal(t, "eastus-1", pool.Name)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := adp.getAZPool(context.Background(), "azure-az-westus-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource pool not found")
	})
}

// --- Tests for ListResourcePools with AZ mode ---

func TestListResourcePools_AZMode_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	pools, err := adp.ListResourcePools(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, pools, 3)
}

// --- Tests for GetResourcePool with AZ mode ---

func TestGetResourcePool_AZMode_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	t.Run("found", func(t *testing.T) {
		pool, err := adp.GetResourcePool(context.Background(), "azure-az-eastus-2")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "azure-az-eastus-2", pool.ResourcePoolID)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := adp.GetResourcePool(context.Background(), "azure-az-westus-5")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource pool not found")
	})
}

// --- Tests for CreateResourcePool with AZ mode ---

func TestCreateResourcePool_AZMode_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	_, err := adp.CreateResourcePool(context.Background(), &adapter.ResourcePool{Name: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot create resource pools in 'az' mode")
}

// --- Tests for UpdateResourcePool with AZ mode ---

func TestUpdateResourcePool_AZMode_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	_, err := adp.UpdateResourcePool(context.Background(), "azure-az-eastus-1", &adapter.ResourcePool{Name: "up"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot update resource pools in 'az' mode")
}

// --- Tests for DeleteResourcePool with AZ mode ---

func TestDeleteResourcePool_AZMode_Internal(t *testing.T) {
	adp := newTestAdapter("az")

	err := adp.DeleteResourcePool(context.Background(), "azure-az-eastus-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete resource pools in 'az' mode")
}

// --- Tests for vmToResource ---

func TestVMToResource_Internal(t *testing.T) {
	adp := newTestAdapter("rg")

	vmSize := armcompute.VirtualMachineSizeTypes("Standard_D2s_v3")
	osType := armcompute.OperatingSystemTypesLinux

	t.Run("full VM", func(t *testing.T) {
		vm := &armcompute.VirtualMachine{
			ID:       strPtr("/subscriptions/sub-123/resourceGroups/myRG/providers/Microsoft.Compute/virtualMachines/my-vm"),
			Name:     strPtr("my-vm"),
			Location: strPtr("eastus"),
			Zones:    []*string{strPtr("1")},
			Tags: map[string]*string{
				"o2ims.io/tenant-id": strPtr("tenant-1"),
				"Environment":        strPtr("prod"),
			},
			Properties: &armcompute.VirtualMachineProperties{
				ProvisioningState: strPtr("Succeeded"),
				VMID:              strPtr("unique-vm-id"),
				HardwareProfile: &armcompute.HardwareProfile{
					VMSize: &vmSize,
				},
				OSProfile: &armcompute.OSProfile{
					ComputerName:  strPtr("my-vm"),
					AdminUsername: strPtr("azureuser"),
				},
				StorageProfile: &armcompute.StorageProfile{
					ImageReference: &armcompute.ImageReference{
						Publisher: strPtr("Canonical"),
						Offer:     strPtr("UbuntuServer"),
						SKU:       strPtr("18.04-LTS"),
						Version:   strPtr("latest"),
					},
					OSDisk: &armcompute.OSDisk{
						Name:       strPtr("my-vm-osdisk"),
						OSType:     &osType,
						DiskSizeGB: int32Ptr(128),
					},
					DataDisks: []*armcompute.DataDisk{
						{
							Name:       strPtr("data-disk-1"),
							Lun:        int32Ptr(0),
							DiskSizeGB: int32Ptr(256),
						},
						{
							Name:       strPtr("data-disk-2"),
							Lun:        int32Ptr(1),
							DiskSizeGB: nil,
						},
					},
				},
				NetworkProfile: &armcompute.NetworkProfile{
					NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
						{ID: strPtr("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/networkInterfaces/nic-1")},
						{ID: strPtr("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/networkInterfaces/nic-2")},
					},
				},
			},
		}

		resource := adp.vmToResource(vm)

		assert.Equal(t, "azure-vm-myRG-my-vm", resource.ResourceID)
		assert.Equal(t, "tenant-1", resource.TenantID)
		assert.Equal(t, "azure-vm-size-Standard_D2s_v3", resource.ResourceTypeID)
		assert.Equal(t, "azure-rg-myRG", resource.ResourcePoolID)
		assert.Equal(t, "my-vm", resource.Description)
		assert.Contains(t, resource.GlobalAssetID, "urn:azure:vm:sub-123:myRG:my-vm")

		ext := resource.Extensions
		assert.Equal(t, "my-vm", ext["azure.vmName"])
		assert.Equal(t, "myRG", ext["azure.resourceGroup"])
		assert.Equal(t, "eastus", ext["azure.location"])
		assert.Equal(t, "Standard_D2s_v3", ext["azure.vmSize"])
		assert.Equal(t, "Succeeded", ext["azure.provisioningState"])
		assert.Equal(t, "unique-vm-id", ext["azure.vmUniqueId"])
		assert.Equal(t, "1", ext["azure.availabilityZone"])
		assert.Equal(t, "my-vm", ext["azure.computerName"])
		assert.Equal(t, "azureuser", ext["azure.adminUsername"])
		assert.Equal(t, "Canonical", ext["azure.imagePublisher"])
		assert.Equal(t, "UbuntuServer", ext["azure.imageOffer"])
		assert.Equal(t, "18.04-LTS", ext["azure.imageSku"])
		assert.Equal(t, "latest", ext["azure.imageVersion"])
		assert.Equal(t, "my-vm-osdisk", ext["azure.osDiskName"])
		assert.Equal(t, "Linux", ext["azure.osDiskType"])
		assert.Equal(t, int32(128), ext["azure.osDiskSizeGB"])

		// Data disks
		dataDisks, ok := ext["azure.dataDisks"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, dataDisks, 2)
		assert.Equal(t, "data-disk-1", dataDisks[0]["name"])
		assert.Equal(t, int32(0), dataDisks[0]["lun"])
		assert.Equal(t, int32(256), dataDisks[0]["sizeGB"])
		assert.Equal(t, "data-disk-2", dataDisks[1]["name"])
		assert.NotContains(t, dataDisks[1], "sizeGB")

		// Network interfaces
		nics, ok := ext["azure.networkInterfaces"].([]string)
		require.True(t, ok)
		assert.Len(t, nics, 2)
	})

	t.Run("minimal VM - no properties", func(t *testing.T) {
		vm := &armcompute.VirtualMachine{
			ID:       strPtr("/subscriptions/sub-123/resourceGroups/testRG/providers/Microsoft.Compute/virtualMachines/vm-min"),
			Name:     strPtr("vm-min"),
			Location: strPtr("eastus"),
		}

		resource := adp.vmToResource(vm)
		assert.Equal(t, "azure-vm-testRG-vm-min", resource.ResourceID)
		assert.Equal(t, "", resource.TenantID)
		assert.Equal(t, "vm-min", resource.Description)
	})

	t.Run("AZ pool mode", func(t *testing.T) {
		adpAZ := newTestAdapter("az")
		vm := &armcompute.VirtualMachine{
			ID:       strPtr("/subscriptions/sub-123/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1"),
			Name:     strPtr("vm1"),
			Location: strPtr("eastus"),
			Zones:    []*string{strPtr("2")},
		}

		resource := adpAZ.vmToResource(vm)
		assert.Equal(t, "azure-az-eastus-2", resource.ResourcePoolID)
	})

	t.Run("AZ pool mode without zone", func(t *testing.T) {
		adpAZ := newTestAdapter("az")
		vm := &armcompute.VirtualMachine{
			ID:       strPtr("/subscriptions/sub-123/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm2"),
			Name:     strPtr("vm2"),
			Location: strPtr("eastus"),
		}

		resource := adpAZ.vmToResource(vm)
		assert.Equal(t, "azure-az-eastus-1", resource.ResourcePoolID)
	})
}

// --- Tests for vmSizeToResourceType ---

func TestVMSizeToResourceType_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("standard size", func(t *testing.T) {
		vmSize := &armcompute.VirtualMachineSize{
			Name:                 strPtr("Standard_D2s_v3"),
			NumberOfCores:        int32Ptr(2),
			MemoryInMB:           int32Ptr(8192),
			MaxDataDiskCount:     int32Ptr(4),
			OSDiskSizeInMB:       int32Ptr(1047552),
			ResourceDiskSizeInMB: int32Ptr(16384),
		}

		rt := adp.vmSizeToResourceType(vmSize)
		assert.Equal(t, "azure-vm-size-Standard_D2s_v3", rt.ResourceTypeID)
		assert.Equal(t, "Standard_D2s_v3", rt.Name)
		assert.Equal(t, "Microsoft Azure", rt.Vendor)
		assert.Equal(t, "Standard_D2s_v3", rt.Model)
		assert.Equal(t, "D", rt.Version)
		assert.Equal(t, "compute", rt.ResourceClass)
		assert.Equal(t, "virtual", rt.ResourceKind)
		assert.Contains(t, rt.Description, "2 vCPUs")
		assert.Contains(t, rt.Description, "8 GiB RAM")

		ext := rt.Extensions
		assert.Equal(t, "Standard_D2s_v3", ext["azure.vmSize"])
		assert.Equal(t, "D", ext["azure.vmFamily"])
		assert.Equal(t, int32(2), ext["azure.numberOfCores"])
		assert.Equal(t, int32(8192), ext["azure.memoryInMB"])
		assert.Equal(t, int32(8), ext["azure.memoryInGB"])
		assert.Equal(t, int32(4), ext["azure.maxDataDiskCount"])
		assert.Equal(t, int32(1047552), ext["azure.osDiskSizeInMB"])
		assert.Equal(t, int32(16384), ext["azure.resourceDiskSizeInMB"])
	})

	t.Run("basic size", func(t *testing.T) {
		vmSize := &armcompute.VirtualMachineSize{
			Name:          strPtr("Basic_A0"),
			NumberOfCores: int32Ptr(1),
			MemoryInMB:    int32Ptr(768),
		}

		rt := adp.vmSizeToResourceType(vmSize)
		assert.Equal(t, "azure-vm-size-Basic_A0", rt.ResourceTypeID)
		assert.Equal(t, "A", rt.Version)
		assert.Equal(t, "virtual", rt.ResourceKind)
	})

	t.Run("nil fields", func(t *testing.T) {
		vmSize := &armcompute.VirtualMachineSize{
			Name: strPtr("Unknown"),
		}

		rt := adp.vmSizeToResourceType(vmSize)
		assert.Equal(t, "azure-vm-size-Unknown", rt.ResourceTypeID)
		assert.Equal(t, int32(0), rt.Extensions["azure.numberOfCores"])
		assert.Equal(t, int32(0), rt.Extensions["azure.memoryInMB"])
	})
}

// --- Tests for addOSProfileExtensions ---

func TestAddOSProfileExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("nil profile", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addOSProfileExtensions(nil, ext)
		assert.NotContains(t, ext, "azure.computerName")
		assert.NotContains(t, ext, "azure.adminUsername")
	})

	t.Run("with profile", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addOSProfileExtensions(&armcompute.OSProfile{
			ComputerName:  strPtr("myhost"),
			AdminUsername: strPtr("admin"),
		}, ext)
		assert.Equal(t, "myhost", ext["azure.computerName"])
		assert.Equal(t, "admin", ext["azure.adminUsername"])
	})
}

// --- Tests for addStorageProfileExtensions ---

func TestAddStorageProfileExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("nil profile", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addStorageProfileExtensions(nil, ext)
		assert.NotContains(t, ext, "azure.imagePublisher")
	})

	t.Run("with all sub-profiles", func(t *testing.T) {
		osType := armcompute.OperatingSystemTypesWindows
		ext := make(map[string]interface{})
		adp.addStorageProfileExtensions(&armcompute.StorageProfile{
			ImageReference: &armcompute.ImageReference{
				Publisher: strPtr("MicrosoftWindowsServer"),
				Offer:     strPtr("WindowsServer"),
				SKU:       strPtr("2019-Datacenter"),
				Version:   strPtr("latest"),
			},
			OSDisk: &armcompute.OSDisk{
				Name:       strPtr("os-disk"),
				OSType:     &osType,
				DiskSizeGB: int32Ptr(256),
			},
			DataDisks: []*armcompute.DataDisk{
				{
					Name:       strPtr("dd1"),
					Lun:        int32Ptr(0),
					DiskSizeGB: int32Ptr(512),
				},
			},
		}, ext)

		assert.Equal(t, "MicrosoftWindowsServer", ext["azure.imagePublisher"])
		assert.Equal(t, "WindowsServer", ext["azure.imageOffer"])
		assert.Equal(t, "2019-Datacenter", ext["azure.imageSku"])
		assert.Equal(t, "latest", ext["azure.imageVersion"])
		assert.Equal(t, "os-disk", ext["azure.osDiskName"])
		assert.Equal(t, "Windows", ext["azure.osDiskType"])
		assert.Equal(t, int32(256), ext["azure.osDiskSizeGB"])

		dataDisks := ext["azure.dataDisks"].([]map[string]interface{})
		require.Len(t, dataDisks, 1)
		assert.Equal(t, "dd1", dataDisks[0]["name"])
	})

	t.Run("with nil sub-profiles", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addStorageProfileExtensions(&armcompute.StorageProfile{}, ext)
		assert.NotContains(t, ext, "azure.imagePublisher")
		assert.NotContains(t, ext, "azure.osDiskName")
		assert.NotContains(t, ext, "azure.dataDisks")
	})
}

// --- Tests for addImageReferenceExtensions ---

func TestAddImageReferenceExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("nil ref", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addImageReferenceExtensions(nil, ext)
		assert.NotContains(t, ext, "azure.imagePublisher")
	})

	t.Run("with ref", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addImageReferenceExtensions(&armcompute.ImageReference{
			Publisher: strPtr("Canonical"),
			Offer:     strPtr("Ubuntu"),
			SKU:       strPtr("20.04"),
			Version:   strPtr("latest"),
		}, ext)
		assert.Equal(t, "Canonical", ext["azure.imagePublisher"])
		assert.Equal(t, "Ubuntu", ext["azure.imageOffer"])
		assert.Equal(t, "20.04", ext["azure.imageSku"])
		assert.Equal(t, "latest", ext["azure.imageVersion"])
	})
}

// --- Tests for addOSDiskExtensions ---

func TestAddOSDiskExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("nil disk", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addOSDiskExtensions(nil, ext)
		assert.NotContains(t, ext, "azure.osDiskName")
	})

	t.Run("with disk and size", func(t *testing.T) {
		osType := armcompute.OperatingSystemTypesLinux
		ext := make(map[string]interface{})
		adp.addOSDiskExtensions(&armcompute.OSDisk{
			Name:       strPtr("my-disk"),
			OSType:     &osType,
			DiskSizeGB: int32Ptr(64),
		}, ext)
		assert.Equal(t, "my-disk", ext["azure.osDiskName"])
		assert.Equal(t, "Linux", ext["azure.osDiskType"])
		assert.Equal(t, int32(64), ext["azure.osDiskSizeGB"])
	})

	t.Run("with disk without size", func(t *testing.T) {
		osType := armcompute.OperatingSystemTypesLinux
		ext := make(map[string]interface{})
		adp.addOSDiskExtensions(&armcompute.OSDisk{
			Name:   strPtr("disk-no-size"),
			OSType: &osType,
		}, ext)
		assert.Equal(t, "disk-no-size", ext["azure.osDiskName"])
		assert.NotContains(t, ext, "azure.osDiskSizeGB")
	})
}

// --- Tests for addDataDisksExtensions ---

func TestAddDataDisksExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("nil disks", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addDataDisksExtensions(nil, ext)
		assert.NotContains(t, ext, "azure.dataDisks")
	})

	t.Run("empty disks", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addDataDisksExtensions([]*armcompute.DataDisk{}, ext)
		assert.NotContains(t, ext, "azure.dataDisks")
	})

	t.Run("with disks", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addDataDisksExtensions([]*armcompute.DataDisk{
			{Name: strPtr("d1"), Lun: int32Ptr(0), DiskSizeGB: int32Ptr(100)},
			{Name: strPtr("d2"), Lun: int32Ptr(1)},
		}, ext)

		disks, ok := ext["azure.dataDisks"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, disks, 2)
		assert.Equal(t, "d1", disks[0]["name"])
		assert.Equal(t, int32(100), disks[0]["sizeGB"])
		assert.Equal(t, "d2", disks[1]["name"])
		assert.NotContains(t, disks[1], "sizeGB")
	})
}

// --- Tests for addNetworkProfileExtensions ---

func TestAddNetworkProfileExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("nil profile", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addNetworkProfileExtensions(nil, ext)
		assert.NotContains(t, ext, "azure.networkInterfaces")
	})

	t.Run("empty interfaces", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addNetworkProfileExtensions(&armcompute.NetworkProfile{
			NetworkInterfaces: []*armcompute.NetworkInterfaceReference{},
		}, ext)
		assert.NotContains(t, ext, "azure.networkInterfaces")
	})

	t.Run("with interfaces", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addNetworkProfileExtensions(&armcompute.NetworkProfile{
			NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
				{ID: strPtr("/subs/s/rg/r/providers/Microsoft.Network/networkInterfaces/nic-1")},
				{ID: strPtr("/subs/s/rg/r/providers/Microsoft.Network/networkInterfaces/nic-2")},
				{ID: nil}, // nil ID should be skipped
			},
		}, ext)

		nics, ok := ext["azure.networkInterfaces"].([]string)
		require.True(t, ok)
		assert.Len(t, nics, 2)
	})
}

// --- Tests for addVMPropertiesExtensions ---

func TestAddVMPropertiesExtensions_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("full properties", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addVMPropertiesExtensions(&armcompute.VirtualMachineProperties{
			ProvisioningState: strPtr("Succeeded"),
			VMID:              strPtr("vm-unique-id"),
			OSProfile: &armcompute.OSProfile{
				ComputerName: strPtr("host"),
			},
		}, ext)
		assert.Equal(t, "Succeeded", ext["azure.provisioningState"])
		assert.Equal(t, "vm-unique-id", ext["azure.vmUniqueId"])
		assert.Equal(t, "host", ext["azure.computerName"])
	})

	t.Run("nil optional fields", func(t *testing.T) {
		ext := make(map[string]interface{})
		adp.addVMPropertiesExtensions(&armcompute.VirtualMachineProperties{}, ext)
		assert.NotContains(t, ext, "azure.provisioningState")
		assert.NotContains(t, ext, "azure.vmUniqueId")
	})
}

// --- Tests for UpdateSubscription ---

func TestUpdateSubscription_Internal(t *testing.T) {
	adp := newTestAdapter("")

	// Create initial subscription
	initial := &adapter.Subscription{
		SubscriptionID: "sub-1",
		Callback:       "https://example.com/old",
	}
	adp.subscriptions["sub-1"] = initial

	t.Run("success", func(t *testing.T) {
		updated, err := adp.UpdateSubscription(context.Background(), "sub-1", &adapter.Subscription{
			Callback:               "https://example.com/new",
			ConsumerSubscriptionID: "consumer-new",
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "sub-1", updated.SubscriptionID)
		assert.Equal(t, "https://example.com/new", updated.Callback)
		assert.Equal(t, "consumer-new", updated.ConsumerSubscriptionID)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := adp.UpdateSubscription(context.Background(), "nonexistent", &adapter.Subscription{
			Callback: "https://example.com/cb",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("empty callback", func(t *testing.T) {
		_, err := adp.UpdateSubscription(context.Background(), "sub-1", &adapter.Subscription{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})
}

// --- Tests for validateAzureConfig ---

func TestValidateAzureConfig_Internal(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{"nil config", nil, true, "config cannot be nil"},
		{"missing subscriptionID", &Config{Location: "eastus", OCloudID: "oc"}, true, "subscriptionID is required"},
		{"missing location", &Config{SubscriptionID: "s", OCloudID: "oc"}, true, "location is required"},
		{"missing oCloudID", &Config{SubscriptionID: "s", Location: "eastus"}, true, "oCloudID is required"},
		{"missing tenantID", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc"}, true, "tenantID is required"},
		{"missing clientID", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", TenantID: "t"}, true, "clientID is required"},
		{"missing clientSecret", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", TenantID: "t", ClientID: "c"}, true, "clientSecret is required"},
		{"invalid poolMode", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", TenantID: "t", ClientID: "c", ClientSecret: "s", PoolMode: "bad"}, true, "poolMode must be"},
		{"managed identity ok", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", UseManagedIdentity: true}, false, ""},
		{"valid rg mode", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", TenantID: "t", ClientID: "c", ClientSecret: "s", PoolMode: "rg"}, false, ""},
		{"valid az mode", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", TenantID: "t", ClientID: "c", ClientSecret: "s", PoolMode: "az"}, false, ""},
		{"valid default mode", &Config{SubscriptionID: "s", Location: "eastus", OCloudID: "oc", TenantID: "t", ClientID: "c", ClientSecret: "s"}, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAzureConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// --- Tests for applyAzureDefaults ---

func TestApplyAzureDefaults_Internal(t *testing.T) {
	t.Run("all defaults", func(t *testing.T) {
		dmID, pm := applyAzureDefaults(&Config{Location: "eastus"})
		assert.Equal(t, "ocloud-azure-eastus", dmID)
		assert.Equal(t, "rg", pm)
	})

	t.Run("custom values", func(t *testing.T) {
		dmID, pm := applyAzureDefaults(&Config{
			Location:            "westus",
			DeploymentManagerID: "custom-dm",
			PoolMode:            "az",
		})
		assert.Equal(t, "custom-dm", dmID)
		assert.Equal(t, "az", pm)
	})
}

// --- Tests for initializeAzureLogger ---

func TestInitializeAzureLogger_Internal(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		l := zap.NewNop()
		r, err := initializeAzureLogger(l)
		require.NoError(t, err)
		assert.Equal(t, l, r)
	})

	t.Run("nil logger", func(t *testing.T) {
		r, err := initializeAzureLogger(nil)
		require.NoError(t, err)
		assert.NotNil(t, r)
	})
}

// --- Tests for parseAzureResourceID ---

func TestParseAzureResourceID_Internal(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantRG  string
		wantVM  string
		wantErr bool
	}{
		{"valid", "azure-vm-myRG-my-vm", "myRG", "my-vm", false},
		{"valid hyphens", "azure-vm-rg1-vm-with-hyphens", "rg1", "vm-with-hyphens", false},
		{"invalid prefix", "aws-vm-rg-vm", "", "", true},
		{"no vm name", "azure-vm-rg", "", "", true},
		{"empty", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rg, vm, err := parseAzureResourceID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid resource ID format")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRG, rg)
				assert.Equal(t, tt.wantVM, vm)
			}
		})
	}
}

// --- Tests for CreateResource ---

func TestCreateResource_Internal(t *testing.T) {
	adp := newTestAdapter("")

	_, err := adp.CreateResource(context.Background(), &adapter.Resource{
		ResourceTypeID: "azure-vm-size-Standard_D2s_v3",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating Azure VMs requires additional configuration")
}

// --- Tests for GetResource with invalid format ---

func TestGetResource_InvalidFormat_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("no prefix", func(t *testing.T) {
		_, err := adp.GetResource(context.Background(), "bad-format")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource ID format")
	})

	t.Run("no vm name", func(t *testing.T) {
		_, err := adp.GetResource(context.Background(), "azure-vm-rg")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource ID format")
	})
}

// --- Tests for UpdateResource with invalid format ---

func TestUpdateResource_InvalidFormat_Internal(t *testing.T) {
	adp := newTestAdapter("")

	_, err := adp.UpdateResource(context.Background(), "bad-id", &adapter.Resource{
		Description: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resource ID format")
}

// --- Tests for DeleteResource with invalid format ---

func TestDeleteResource_InvalidFormat_Internal(t *testing.T) {
	adp := newTestAdapter("")

	t.Run("no prefix", func(t *testing.T) {
		err := adp.DeleteResource(context.Background(), "bad-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource ID format")
	})

	t.Run("no vm name", func(t *testing.T) {
		err := adp.DeleteResource(context.Background(), "azure-vm-rg")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource ID format")
	})
}
