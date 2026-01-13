package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResourceTypes retrieves all OpenStack flavors and transforms them to O2-IMS Resource Types.
// Flavors in OpenStack define the compute, memory, and storage capacity of instances.
func (a *Adapter) ListResourceTypes(
	_ context.Context,
	filter *adapter.Filter,
) ([]*adapter.ResourceType, error) {
	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// Query all flavors from Nova
	listOpts := flavors.ListOpts{
		// No filters by default; list all available flavors
	}

	allPages, err := flavors.ListDetail(a.compute, listOpts).AllPages()
	if err != nil {
		a.logger.Error("failed to list flavors",
			zap.Error(err))
		return nil, fmt.Errorf("failed to list OpenStack flavors: %w", err)
	}

	osFlavors, err := flavors.ExtractFlavors(allPages)
	if err != nil {
		a.logger.Error("failed to extract flavors",
			zap.Error(err))
		return nil, fmt.Errorf("failed to extract flavors: %w", err)
	}

	a.logger.Debug("retrieved flavors from OpenStack",
		zap.Int("count", len(osFlavors)))

	// Transform OpenStack flavors to O2-IMS Resource Types
	resourceTypes := make([]*adapter.ResourceType, 0, len(osFlavors))
	for i := range osFlavors {
		resourceType := a.transformFlavorToResourceType(&osFlavors[i])

		// Apply filter if needed
		// For resource types, we typically don't filter by pool or location
		resourceTypes = append(resourceTypes, resourceType)
	}

	// Apply pagination
	if filter != nil {
		resourceTypes = adapter.ApplyPagination(resourceTypes, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// GetResourceType retrieves a specific OpenStack flavor by ID and transforms it to O2-IMS Resource Type.
func (a *Adapter) GetResourceType(_ context.Context, id string) (*adapter.ResourceType, error) {
	var flavorID string
	if _, err := fmt.Sscanf(id, "openstack-flavor-%s", &flavorID); err != nil {
		return nil, fmt.Errorf("invalid resource type ID format: %s", id)
	}
	osFlavor, err := flavors.Get(a.compute, flavorID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenStack flavor %s: %w", flavorID, err)
	}
	return a.transformFlavorToResourceType(osFlavor), nil
}

// transformFlavorToResourceType converts an OpenStack flavor to O2-IMS Resource Type.
func (a *Adapter) transformFlavorToResourceType(flavor *flavors.Flavor) *adapter.ResourceType {
	resourceTypeID := generateFlavorID(flavor)

	// Determine resource class based on flavor characteristics
	// Default to compute unless it's clearly a storage-focused flavor
	resourceClass := "compute"
	if flavor.RAM == 0 && flavor.VCPUs == 0 && flavor.Disk > 0 {
		resourceClass = "storage"
	}

	// Build description with flavor specs
	description := fmt.Sprintf("OpenStack flavor: %s (vCPUs: %d, RAM: %dMB, Disk: %dGB)",
		flavor.Name, flavor.VCPUs, flavor.RAM, flavor.Disk)

	// Build extensions with all flavor metadata
	extensions := map[string]interface{}{
		"openstack.flavorId":   flavor.ID,
		"openstack.name":       flavor.Name,
		"openstack.vcpus":      flavor.VCPUs,
		"openstack.ram":        flavor.RAM,
		"openstack.disk":       flavor.Disk,
		"openstack.swap":       flavor.Swap,
		"openstack.ephemeral":  flavor.Ephemeral,
		"openstack.isPublic":   flavor.IsPublic,
		"openstack.rxtxFactor": flavor.RxTxFactor,
	}

	// Note: ExtraSpecs are not in the basic Flavor struct.
	// To retrieve extra specs, you would need to use the flavors/extraspecs package.
	// For now, we omit extra specs from the transformation.

	return &adapter.ResourceType{
		ResourceTypeID: resourceTypeID,
		Name:           flavor.Name,
		Description:    description,
		Vendor:         "OpenStack",
		Model:          flavor.Name,
		Version:        "",            // Flavors don't have versions
		ResourceClass:  resourceClass, // compute, storage, network
		ResourceKind:   "virtual",     // OpenStack instances are virtual
		Extensions:     extensions,
	}
}
