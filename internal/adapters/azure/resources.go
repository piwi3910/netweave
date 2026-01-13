package azure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResources retrieves all resources (Azure VMs) matching the
// provided filter.
func (a *Adapter) ListResources(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "ListResources", start, err) }()

	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// List all VMs in the subscription
	var resources []*adapter.Resource
	pager := a.vmClient.NewListAllPager(nil)
	for pager.More() {
		page, pageErr := pager.NextPage(ctx)
		if pageErr != nil {
			err = fmt.Errorf("failed to list VMs: %w", pageErr)
			return nil, err
		}

		for _, vm := range page.Value {
			// Only include VMs in the configured location
			location := ptrToString(vm.Location)
			if location != a.location {
				continue
			}

			resource := a.vmToResource(vm)

			// Convert tags to labels
			labels := tagsToMap(vm.Tags)

			// Apply filter
			if !adapter.MatchesFilter(filter, resource.ResourcePoolID, resource.ResourceTypeID, location, labels) {
				continue
			}

			resources = append(resources, resource)
		}
	}

	// Apply pagination
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific resource (Azure VM) by ID.
func (a *Adapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	var (
		resource *adapter.Resource
		err      error
	)
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "GetResource", start, err) }()

	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Parse resource group and VM name from the ID
	// Format: azure-vm-{resourceGroup}-{vmName}
	prefix := "azure-vm-"
	if !strings.HasPrefix(id, prefix) {
		return nil, fmt.Errorf("invalid resource ID format: %s", id)
	}

	remainder := strings.TrimPrefix(id, prefix)
	parts := strings.SplitN(remainder, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid resource ID format: %s", id)
	}

	resourceGroup := parts[0]
	vmName := parts[1]

	vm, err := a.vmClient.Get(ctx, resourceGroup, vmName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM: %w", err)
	}

	resource = a.vmToResource(&vm.VirtualMachine)

	a.logger.Info("retrieved resource",
		zap.String("resourceId", resource.ResourceID))

	return resource, nil
}

// CreateResource creates a new resource (Azure VM).
func (a *Adapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "CreateResource", start, err) }()

	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Creating Azure VMs requires extensive configuration not available in the O2-IMS model
	return nil, fmt.Errorf(
		"creating Azure VMs requires additional configuration: " +
			"use Azure portal or CLI",
	)
}

// UpdateResource updates an existing Azure VM's tags and metadata.
// Note: Core VM properties cannot be modified after creation.
func (a *Adapter) UpdateResource(
	_ context.Context,
	_ string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "UpdateResource", start, err) }()

	a.logger.Debug("UpdateResource called",
		zap.String("resourceID", resource.ResourceID))

	// TODO(#189): Implement VM tag updates via Azure API
	// For now, return not supported
	err = fmt.Errorf("updating Azure VMs is not yet implemented")
	return nil, err
}

// DeleteResource deletes a resource (Azure VM) by ID.
func (a *Adapter) DeleteResource(ctx context.Context, id string) error {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "DeleteResource", start, err) }()

	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Parse resource group and VM name from the ID
	prefix := "azure-vm-"
	if !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("invalid resource ID format: %s", id)
	}

	remainder := strings.TrimPrefix(id, prefix)
	parts := strings.SplitN(remainder, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid resource ID format: %s", id)
	}

	resourceGroup := parts[0]
	vmName := parts[1]

	poller, err := a.vmClient.BeginDelete(ctx, resourceGroup, vmName, nil)
	if err != nil {
		return fmt.Errorf("failed to start VM deletion: %w", err)
	}

	// Wait for deletion to complete
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	a.logger.Info("deleted resource",
		zap.String("resourceId", id))

	return nil
}

// vmToResource converts an Azure VM to an O2-IMS Resource.
func (a *Adapter) vmToResource(vm *armcompute.VirtualMachine) *adapter.Resource {
	vmName := ptrToString(vm.Name)
	location := ptrToString(vm.Location)
	resourceGroup := extractResourceGroup(ptrToString(vm.ID))

	resourceID := generateVMID(vmName, resourceGroup)
	vmSize := a.extractVMSize(vm)
	resourceTypeID := generateVMSizeID(vmSize)
	resourcePoolID := a.determineResourcePoolID(vm, location, resourceGroup)

	extensions := a.buildVMExtensions(vm, vmName, location, resourceGroup, vmSize)

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:azure:vm:%s:%s:%s", a.subscriptionID, resourceGroup, vmName),
		Description:    vmName,
		Extensions:     extensions,
	}
}

func (a *Adapter) extractVMSize(vm *armcompute.VirtualMachine) string {
	if vm.Properties != nil && vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
		return string(*vm.Properties.HardwareProfile.VMSize)
	}
	return ""
}

func (a *Adapter) determineResourcePoolID(vm *armcompute.VirtualMachine, location, resourceGroup string) string {
	if a.poolMode == "rg" {
		return generateRGPoolID(resourceGroup)
	}

	if len(vm.Zones) > 0 {
		return generateAZPoolID(location, *vm.Zones[0])
	}
	return generateAZPoolID(location, "1")
}

func (a *Adapter) buildVMExtensions(
	vm *armcompute.VirtualMachine,
	vmName, location, resourceGroup, vmSize string,
) map[string]interface{} {
	extensions := map[string]interface{}{
		"azure.vmId":          ptrToString(vm.ID),
		"azure.vmName":        vmName,
		"azure.resourceGroup": resourceGroup,
		"azure.location":      location,
		"azure.vmSize":        vmSize,
		"azure.tags":          tagsToMap(vm.Tags),
	}

	if vm.Properties != nil {
		a.addVMPropertiesExtensions(vm.Properties, extensions)
	}

	if len(vm.Zones) > 0 {
		extensions["azure.availabilityZone"] = *vm.Zones[0]
	}

	return extensions
}

func (a *Adapter) addVMPropertiesExtensions(
	props *armcompute.VirtualMachineProperties,
	extensions map[string]interface{},
) {
	if props.ProvisioningState != nil {
		extensions["azure.provisioningState"] = *props.ProvisioningState
	}
	if props.VMID != nil {
		extensions["azure.vmUniqueId"] = *props.VMID
	}

	a.addOSProfileExtensions(props.OSProfile, extensions)
	a.addStorageProfileExtensions(props.StorageProfile, extensions)
	a.addNetworkProfileExtensions(props.NetworkProfile, extensions)
}

func (a *Adapter) addOSProfileExtensions(
	osProfile *armcompute.OSProfile,
	extensions map[string]interface{},
) {
	if osProfile == nil {
		return
	}
	extensions["azure.computerName"] = ptrToString(osProfile.ComputerName)
	extensions["azure.adminUsername"] = ptrToString(osProfile.AdminUsername)
}

func (a *Adapter) addStorageProfileExtensions(
	storage *armcompute.StorageProfile,
	extensions map[string]interface{},
) {
	if storage == nil {
		return
	}

	a.addImageReferenceExtensions(storage.ImageReference, extensions)
	a.addOSDiskExtensions(storage.OSDisk, extensions)
	a.addDataDisksExtensions(storage.DataDisks, extensions)
}

func (a *Adapter) addImageReferenceExtensions(
	imgRef *armcompute.ImageReference,
	extensions map[string]interface{},
) {
	if imgRef == nil {
		return
	}
	extensions["azure.imagePublisher"] = ptrToString(imgRef.Publisher)
	extensions["azure.imageOffer"] = ptrToString(imgRef.Offer)
	extensions["azure.imageSku"] = ptrToString(imgRef.SKU)
	extensions["azure.imageVersion"] = ptrToString(imgRef.Version)
}

func (a *Adapter) addOSDiskExtensions(
	osDisk *armcompute.OSDisk,
	extensions map[string]interface{},
) {
	if osDisk == nil {
		return
	}
	extensions["azure.osDiskName"] = ptrToString(osDisk.Name)
	extensions["azure.osDiskType"] = string(*osDisk.OSType)
	if osDisk.DiskSizeGB != nil {
		extensions["azure.osDiskSizeGB"] = *osDisk.DiskSizeGB
	}
}

func (a *Adapter) addDataDisksExtensions(
	dataDisks []*armcompute.DataDisk,
	extensions map[string]interface{},
) {
	if len(dataDisks) == 0 {
		return
	}

	diskInfos := make([]map[string]interface{}, 0, len(dataDisks))
	for _, disk := range dataDisks {
		diskInfo := map[string]interface{}{
			"name": ptrToString(disk.Name),
			"lun":  ptrToInt32(disk.Lun),
		}
		if disk.DiskSizeGB != nil {
			diskInfo["sizeGB"] = *disk.DiskSizeGB
		}
		diskInfos = append(diskInfos, diskInfo)
	}
	extensions["azure.dataDisks"] = diskInfos
}

func (a *Adapter) addNetworkProfileExtensions(
	network *armcompute.NetworkProfile,
	extensions map[string]interface{},
) {
	if network == nil || len(network.NetworkInterfaces) == 0 {
		return
	}

	nics := make([]string, 0, len(network.NetworkInterfaces))
	for _, nic := range network.NetworkInterfaces {
		if nic.ID != nil {
			nics = append(nics, *nic.ID)
		}
	}
	extensions["azure.networkInterfaces"] = nics
}

// extractResourceGroup extracts the resource group name from an Azure resource ID.
func extractResourceGroup(resourceID string) string {
	// Format: /subscriptions/{sub}/resourceGroups/{rg}/providers/...
	parts := strings.Split(resourceID, "/")
	for i, part := range parts {
		if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
