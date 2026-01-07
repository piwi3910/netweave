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

// ListResources retrieves all resources (Azure VMs) matching the provided filter.
func (a *AzureAdapter) ListResources(ctx context.Context, filter *adapter.Filter) (resources []*adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "ListResources", start, err) }()

	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// List all VMs in the subscription
	pager := a.vmClient.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VMs: %w", err)
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
func (a *AzureAdapter) GetResource(ctx context.Context, id string) (resource *adapter.Resource, err error) {
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
func (a *AzureAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (result *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "CreateResource", start, err) }()

	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Creating Azure VMs requires extensive configuration not available in the O2-IMS model
	return nil, fmt.Errorf("creating Azure VMs requires additional configuration: use Azure portal or CLI")
}

// DeleteResource deletes a resource (Azure VM) by ID.
func (a *AzureAdapter) DeleteResource(ctx context.Context, id string) (err error) {
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
func (a *AzureAdapter) vmToResource(vm *armcompute.VirtualMachine) *adapter.Resource {
	vmName := ptrToString(vm.Name)
	location := ptrToString(vm.Location)

	// Extract resource group from the VM ID
	// Format: /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Compute/virtualMachines/{name}
	resourceGroup := extractResourceGroup(ptrToString(vm.ID))

	resourceID := generateVMID(vmName, resourceGroup)

	// Get VM size as resource type
	var vmSize string
	if vm.Properties != nil && vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
		vmSize = string(*vm.Properties.HardwareProfile.VMSize)
	}
	resourceTypeID := generateVMSizeID(vmSize)

	// Determine resource pool ID based on pool mode
	var resourcePoolID string
	if a.poolMode == "rg" {
		resourcePoolID = generateRGPoolID(resourceGroup)
	} else {
		// In AZ mode, check if VM has an availability zone
		if vm.Zones != nil && len(vm.Zones) > 0 {
			zone := *vm.Zones[0]
			resourcePoolID = generateAZPoolID(location, zone)
		} else {
			// Fallback to location
			resourcePoolID = generateAZPoolID(location, "1")
		}
	}

	// Build extensions with Azure VM details
	extensions := map[string]interface{}{
		"azure.vmId":          ptrToString(vm.ID),
		"azure.vmName":        vmName,
		"azure.resourceGroup": resourceGroup,
		"azure.location":      location,
		"azure.vmSize":        vmSize,
		"azure.tags":          tagsToMap(vm.Tags),
	}

	// Add provisioning and power state
	if vm.Properties != nil {
		if vm.Properties.ProvisioningState != nil {
			extensions["azure.provisioningState"] = *vm.Properties.ProvisioningState
		}
		if vm.Properties.VMID != nil {
			extensions["azure.vmUniqueId"] = *vm.Properties.VMID
		}

		// Add OS profile
		if vm.Properties.OSProfile != nil {
			osProfile := vm.Properties.OSProfile
			extensions["azure.computerName"] = ptrToString(osProfile.ComputerName)
			extensions["azure.adminUsername"] = ptrToString(osProfile.AdminUsername)
		}

		// Add storage profile
		if vm.Properties.StorageProfile != nil {
			storage := vm.Properties.StorageProfile
			if storage.ImageReference != nil {
				extensions["azure.imagePublisher"] = ptrToString(storage.ImageReference.Publisher)
				extensions["azure.imageOffer"] = ptrToString(storage.ImageReference.Offer)
				extensions["azure.imageSku"] = ptrToString(storage.ImageReference.SKU)
				extensions["azure.imageVersion"] = ptrToString(storage.ImageReference.Version)
			}
			if storage.OSDisk != nil {
				extensions["azure.osDiskName"] = ptrToString(storage.OSDisk.Name)
				extensions["azure.osDiskType"] = string(*storage.OSDisk.OSType)
				if storage.OSDisk.DiskSizeGB != nil {
					extensions["azure.osDiskSizeGB"] = *storage.OSDisk.DiskSizeGB
				}
			}
			if len(storage.DataDisks) > 0 {
				dataDisks := make([]map[string]interface{}, 0, len(storage.DataDisks))
				for _, disk := range storage.DataDisks {
					diskInfo := map[string]interface{}{
						"name": ptrToString(disk.Name),
						"lun":  ptrToInt32(disk.Lun),
					}
					if disk.DiskSizeGB != nil {
						diskInfo["sizeGB"] = *disk.DiskSizeGB
					}
					dataDisks = append(dataDisks, diskInfo)
				}
				extensions["azure.dataDisks"] = dataDisks
			}
		}

		// Add network profile
		if vm.Properties.NetworkProfile != nil && len(vm.Properties.NetworkProfile.NetworkInterfaces) > 0 {
			nics := make([]string, 0, len(vm.Properties.NetworkProfile.NetworkInterfaces))
			for _, nic := range vm.Properties.NetworkProfile.NetworkInterfaces {
				if nic.ID != nil {
					nics = append(nics, *nic.ID)
				}
			}
			extensions["azure.networkInterfaces"] = nics
		}
	}

	// Add availability zone
	if vm.Zones != nil && len(vm.Zones) > 0 {
		extensions["azure.availabilityZone"] = *vm.Zones[0]
	}

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:azure:vm:%s:%s:%s", a.subscriptionID, resourceGroup, vmName),
		Description:    vmName,
		Extensions:     extensions,
	}
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
