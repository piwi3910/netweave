package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// ListResourceTypes retrieves all resource types (GCP machine types) matching the provided filter.
func (a *GCPAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) (resourceTypes []*adapter.ResourceType, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "ListResourceTypes", start, err) }()

	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// Get the first zone in the region
	firstZone, err := a.getFirstZoneInRegion(ctx)
	if err != nil {
		return nil, err
	}

	// List and filter machine types
	resourceTypes, err = a.listMachineTypesInZone(ctx, firstZone, filter)
	if err != nil {
		return nil, err
	}

	// Apply pagination
	if filter != nil {
		resourceTypes = adapter.ApplyPagination(resourceTypes, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// getFirstZoneInRegion finds the first zone in the adapter's region.
func (a *GCPAdapter) getFirstZoneInRegion(ctx context.Context) (string, error) {
	zoneIt := a.zonesClient.List(ctx, &computepb.ListZonesRequest{
		Project: a.projectID,
	})

	for {
		zone, err := zoneIt.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to list zones: %w", err)
		}

		zoneName := ptrToString(zone.Name)
		if strings.HasPrefix(zoneName, a.region) {
			return zoneName, nil
		}
	}

	return "", fmt.Errorf("no zones found in region %s", a.region)
}

// listMachineTypesInZone lists machine types in a zone and applies filtering.
func (a *GCPAdapter) listMachineTypesInZone(ctx context.Context, zone string, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	var resourceTypes []*adapter.ResourceType
	seen := make(map[string]bool)

	mtIt := a.machineTypesClient.List(ctx, &computepb.ListMachineTypesRequest{
		Project: a.projectID,
		Zone:    zone,
	})

	for {
		mt, err := mtIt.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list machine types: %w", err)
		}

		machineType := ptrToString(mt.Name)
		if seen[machineType] {
			continue
		}
		seen[machineType] = true

		resourceType := a.machineTypeToResourceType(mt)

		// Apply filter
		if !adapter.MatchesFilter(filter, "", resourceType.ResourceTypeID, "", nil) {
			continue
		}

		resourceTypes = append(resourceTypes, resourceType)
	}

	return resourceTypes, nil
}

// GetResourceType retrieves a specific resource type (GCP machine type) by ID.
func (a *GCPAdapter) GetResourceType(ctx context.Context, id string) (resourceType *adapter.ResourceType, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "GetResourceType", start, err) }()

	a.logger.Debug("GetResourceType called",
		zap.String("id", id))

	// Extract machine type name from the ID
	machineTypeName := strings.TrimPrefix(id, "gcp-machine-type-")
	if machineTypeName == id {
		machineTypeName = id
	}

	// Get the first zone in the region
	zoneIt := a.zonesClient.List(ctx, &computepb.ListZonesRequest{
		Project: a.projectID,
	})

	var firstZone string
	for {
		zone, err := zoneIt.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list zones: %w", err)
		}

		zoneName := ptrToString(zone.Name)
		if strings.HasPrefix(zoneName, a.region) {
			firstZone = zoneName
			break
		}
	}

	if firstZone == "" {
		return nil, fmt.Errorf("no zones found in region %s", a.region)
	}

	// Get the machine type
	mt, err := a.machineTypesClient.Get(ctx, &computepb.GetMachineTypeRequest{
		Project:     a.projectID,
		Zone:        firstZone,
		MachineType: machineTypeName,
	})
	if err != nil {
		return nil, fmt.Errorf("machine type not found: %w", err)
	}

	return a.machineTypeToResourceType(mt), nil
}

// machineTypeToResourceType converts a GCP machine type to an O2-IMS ResourceType.
func (a *GCPAdapter) machineTypeToResourceType(mt *computepb.MachineType) *adapter.ResourceType {
	machineType := ptrToString(mt.Name)
	resourceTypeID := generateMachineTypeID(machineType)

	// Parse machine family from type name (e.g., "n1" from "n1-standard-1")
	family := extractMachineFamily(machineType)

	// All GCP VMs are virtual
	resourceKind := "virtual"

	// Calculate memory in GiB
	memoryMB := ptrToInt32(mt.MemoryMb)
	memoryGiB := memoryMB / 1024

	// Build extensions with machine type details
	extensions := map[string]interface{}{
		"gcp.machineType":                  machineType,
		"gcp.family":                       family,
		"gcp.guestCpus":                    ptrToInt32(mt.GuestCpus),
		"gcp.memoryMb":                     memoryMB,
		"gcp.memoryGb":                     memoryGiB,
		"gcp.maximumPersistentDisks":       ptrToInt32(mt.MaximumPersistentDisks),
		"gcp.maximumPersistentDisksSizeGb": ptrToInt64(mt.MaximumPersistentDisksSizeGb),
		"gcp.isSharedCpu":                  ptrToBool(mt.IsSharedCpu),
		"gcp.description":                  ptrToString(mt.Description),
	}

	// Add accelerator info if available
	if len(mt.Accelerators) > 0 {
		accs := make([]map[string]interface{}, 0, len(mt.Accelerators))
		for _, acc := range mt.Accelerators {
			accInfo := map[string]interface{}{
				"type":  ptrToString(acc.GuestAcceleratorType),
				"count": ptrToInt32(acc.GuestAcceleratorCount),
			}
			accs = append(accs, accInfo)
		}
		extensions["gcp.accelerators"] = accs
	}

	// Build description
	cpus := ptrToInt32(mt.GuestCpus)
	description := fmt.Sprintf("GCP %s: %d vCPUs, %d GiB RAM", machineType, cpus, memoryGiB)
	if ptrToBool(mt.IsSharedCpu) {
		description += " (shared CPU)"
	}

	return &adapter.ResourceType{
		ResourceTypeID: resourceTypeID,
		Name:           machineType,
		Description:    description,
		Vendor:         "Google Cloud Platform",
		Model:          machineType,
		Version:        family,
		ResourceClass:  "compute",
		ResourceKind:   resourceKind,
		Extensions:     extensions,
	}
}

// extractMachineFamily extracts the machine family from a GCP machine type name.
// e.g., "n1-standard-1" -> "n1", "e2-micro" -> "e2".
func extractMachineFamily(machineType string) string {
	parts := strings.Split(machineType, "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return machineType
}
