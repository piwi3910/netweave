package vmware

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi/vim25/mo"
	"go.uber.org/zap"
)

// ListResourceTypes retrieves all resource types (VM profiles) matching the provided filter.
// Resource types are derived from the existing VMs in the datacenter.
func (a *VMwareAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// Find all VMs to derive resource types
	vms, err := a.finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	// Map to track unique resource types
	seen := make(map[string]bool)
	var resourceTypes []*adapter.ResourceType

	for _, vm := range vms {
		// Get VM properties
		var vmMo mo.VirtualMachine
		err := vm.Properties(ctx, vm.Reference(), []string{"summary.config"}, &vmMo)
		if err != nil {
			continue
		}

		if vmMo.Summary.Config == nil {
			continue
		}

		config := vmMo.Summary.Config
		cpuCount := config.NumCpu
		memoryMB := int64(config.MemorySizeMB)

		resourceTypeID := generateVMProfileID(cpuCount, memoryMB)

		// Skip if already seen
		if seen[resourceTypeID] {
			continue
		}
		seen[resourceTypeID] = true

		resourceType := a.createResourceType(cpuCount, memoryMB)

		// Apply filter
		if !adapter.MatchesFilter(filter, "", resourceType.ResourceTypeID, "", nil) {
			continue
		}

		resourceTypes = append(resourceTypes, resourceType)
	}

	// Add some common VM profiles if no VMs exist
	if len(resourceTypes) == 0 {
		resourceTypes = a.getDefaultResourceTypes()
	}

	// Apply pagination
	if filter != nil {
		resourceTypes = adapter.ApplyPagination(resourceTypes, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// GetResourceType retrieves a specific resource type by ID.
func (a *VMwareAdapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	a.logger.Debug("GetResourceType called",
		zap.String("id", id))

	resourceTypes, err := a.ListResourceTypes(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, rt := range resourceTypes {
		if rt.ResourceTypeID == id {
			return rt, nil
		}
	}

	return nil, fmt.Errorf("resource type not found: %s", id)
}

// createResourceType creates a resource type from CPU and memory specifications.
func (a *VMwareAdapter) createResourceType(cpuCount int32, memoryMB int64) *adapter.ResourceType {
	resourceTypeID := generateVMProfileID(cpuCount, memoryMB)
	memoryGB := memoryMB / 1024

	// All vSphere VMs are virtual
	resourceKind := "virtual"

	// Build extensions
	extensions := map[string]interface{}{
		"vmware.numCpu":       cpuCount,
		"vmware.memorySizeMB": memoryMB,
		"vmware.memorySizeGB": memoryGB,
	}

	// Build description
	description := fmt.Sprintf("VMware VM Profile: %d vCPUs, %d GiB RAM", cpuCount, memoryGB)

	return &adapter.ResourceType{
		ResourceTypeID: resourceTypeID,
		Name:           fmt.Sprintf("VM-%dcpu-%dGB", cpuCount, memoryGB),
		Description:    description,
		Vendor:         "VMware",
		Model:          fmt.Sprintf("%dcpu-%dGB", cpuCount, memoryGB),
		Version:        "vSphere",
		ResourceClass:  "compute",
		ResourceKind:   resourceKind,
		Extensions:     extensions,
	}
}

// getDefaultResourceTypes returns common VM profiles.
func (a *VMwareAdapter) getDefaultResourceTypes() []*adapter.ResourceType {
	profiles := []struct {
		cpu    int32
		memory int64
	}{
		{1, 1024},   // 1 CPU, 1 GB
		{1, 2048},   // 1 CPU, 2 GB
		{2, 2048},   // 2 CPU, 2 GB
		{2, 4096},   // 2 CPU, 4 GB
		{4, 4096},   // 4 CPU, 4 GB
		{4, 8192},   // 4 CPU, 8 GB
		{8, 8192},   // 8 CPU, 8 GB
		{8, 16384},  // 8 CPU, 16 GB
		{16, 32768}, // 16 CPU, 32 GB
		{32, 65536}, // 32 CPU, 64 GB
	}

	resourceTypes := make([]*adapter.ResourceType, 0, len(profiles))
	for _, p := range profiles {
		resourceTypes = append(resourceTypes, a.createResourceType(p.cpu, p.memory))
	}

	return resourceTypes
}
