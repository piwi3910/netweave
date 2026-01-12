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

// ListResourceTypes retrieves all resource types (Azure VM sizes) matching the provided filter.
func (a *Adapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) (resourceTypes []*adapter.ResourceType, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "ListResourceTypes", start, err) }()

	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// List VM sizes for the configured location
	pager := a.vmSizeClient.NewListPager(a.location, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VM sizes: %w", err)
		}

		for _, vmSize := range page.Value {
			resourceType := a.vmSizeToResourceType(vmSize)

			// Apply filter
			if !adapter.MatchesFilter(filter, "", resourceType.ResourceTypeID, "", nil) {
				continue
			}

			resourceTypes = append(resourceTypes, resourceType)
		}
	}

	// Apply pagination
	if filter != nil {
		resourceTypes = adapter.ApplyPagination(resourceTypes, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// GetResourceType retrieves a specific resource type (Azure VM size) by ID.
func (a *Adapter) GetResourceType(ctx context.Context, id string) (resourceType *adapter.ResourceType, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "GetResourceType", start, err) }()

	a.logger.Debug("GetResourceType called",
		zap.String("id", id))

	// Extract VM size name from the ID
	vmSizeName := strings.TrimPrefix(id, "azure-vm-size-")
	if vmSizeName == id {
		vmSizeName = id
	}

	// List all VM sizes and find the matching one
	pager := a.vmSizeClient.NewListPager(a.location, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VM sizes: %w", err)
		}

		for _, vmSize := range page.Value {
			if ptrToString(vmSize.Name) == vmSizeName {
				return a.vmSizeToResourceType(vmSize), nil
			}
		}
	}

	return nil, fmt.Errorf("resource type not found: %s", id)
}

// vmSizeToResourceType converts an Azure VM size to an O2-IMS ResourceType.
func (a *Adapter) vmSizeToResourceType(vmSize *armcompute.VirtualMachineSize) *adapter.ResourceType {
	sizeName := ptrToString(vmSize.Name)
	resourceTypeID := generateVMSizeID(sizeName)

	// Parse VM family from size name (e.g., "Standard_D2s_v3" -> "D")
	family := extractVMFamily(sizeName)

	// Determine resource kind (all Azure VMs are virtual)
	resourceKind := "virtual"

	// Calculate memory in GiB
	memoryGiB := ptrToInt32(vmSize.MemoryInMB) / 1024

	// Build extensions with VM size details
	extensions := map[string]interface{}{
		"azure.vmSize":               sizeName,
		"azure.vmFamily":             family,
		"azure.numberOfCores":        ptrToInt32(vmSize.NumberOfCores),
		"azure.memoryInMB":           ptrToInt32(vmSize.MemoryInMB),
		"azure.memoryInGB":           memoryGiB,
		"azure.maxDataDiskCount":     ptrToInt32(vmSize.MaxDataDiskCount),
		"azure.osDiskSizeInMB":       ptrToInt32(vmSize.OSDiskSizeInMB),
		"azure.resourceDiskSizeInMB": ptrToInt32(vmSize.ResourceDiskSizeInMB),
	}

	// Build description
	cores := ptrToInt32(vmSize.NumberOfCores)
	description := fmt.Sprintf("Azure %s: %d vCPUs, %d GiB RAM", sizeName, cores, memoryGiB)

	return &adapter.ResourceType{
		ResourceTypeID: resourceTypeID,
		Name:           sizeName,
		Description:    description,
		Vendor:         "Microsoft Azure",
		Model:          sizeName,
		Version:        family,
		ResourceClass:  "compute",
		ResourceKind:   resourceKind,
		Extensions:     extensions,
	}
}

// extractVMFamily extracts the VM family from an Azure VM size name.
// e.g., "Standard_D2s_v3" -> "D", "Standard_B2ms" -> "B".
func extractVMFamily(sizeName string) string {
	// Remove "Standard_" prefix
	name := strings.TrimPrefix(sizeName, "Standard_")
	name = strings.TrimPrefix(name, "Basic_")

	// Extract the first letter(s) before any number
	family := ""
	for _, c := range name {
		if c >= '0' && c <= '9' {
			break
		}
		family += string(c)
	}

	return family
}
