package vmware

import (
	"context"
	"fmt"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi/vim25/mo"
	"go.uber.org/zap"
)

// ListResourceTypes retrieves all resource types (VM profiles) matching the provided filter.
// Resource types are derived from the existing VMs in the datacenter.
func (a *Adapter) ListResourceTypes(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.ResourceType, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "ListResourceTypes", start, err) }()

	a.Logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// Find all VMs to derive resource types
	vms, err := a.finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	var resourceTypes []*adapter.ResourceType

	// Map to track unique resource types
	seen := make(map[string]bool)

	for _, vm := range vms {
		// Get VM properties
		var vmMo mo.VirtualMachine
		err := vm.Properties(ctx, vm.Reference(), []string{"summary.config"}, &vmMo)
		if err != nil {
			continue
		}

		config := vmMo.Summary.Config
		cpuCount := config.NumCpu
		memoryMB := int64(config.MemorySizeMB)

		resourceTypeID := GenerateVMProfileID(cpuCount, memoryMB)

		// Skip if already seen
		if seen[resourceTypeID] {
			continue
		}
		seen[resourceTypeID] = true

		resourceType := a.CreateResourceType(cpuCount, memoryMB)

		// Apply filter
		if !adapter.MatchesFilter(filter, "", resourceType.ResourceTypeID, "", nil) {
			continue
		}

		resourceTypes = append(resourceTypes, resourceType)
	}

	// Add some common VM profiles if no VMs exist
	if len(resourceTypes) == 0 {
		resourceTypes = a.GetDefaultResourceTypes()
	}

	// Apply pagination
	if filter != nil {
		resourceTypes = adapter.ApplyPagination(resourceTypes, filter.Limit, filter.Offset)
	}

	a.Logger.Info("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// GetResourceType retrieves a specific resource type by ID.
func (a *Adapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "GetResourceType", start, err) }()

	a.Logger.Debug("GetResourceType called",
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

// CreateResourceType creates a resource type from CPU and memory specifications.
func (a *Adapter) CreateResourceType(cpuCount int32, memoryMB int64) *adapter.ResourceType {
	resourceTypeID := GenerateVMProfileID(cpuCount, memoryMB)
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

// GetDefaultResourceTypes returns common VM profiles.
func (a *Adapter) GetDefaultResourceTypes() []*adapter.ResourceType {
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
		resourceTypes = append(resourceTypes, a.CreateResourceType(p.cpu, p.memory))
	}

	return resourceTypes
}
