package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/aggregates"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResourcePools retrieves all OpenStack host aggregates and transforms them to O2-IMS Resource Pools.
// Host aggregates in OpenStack are logical groupings of compute hosts, which map naturally to O2-IMS Resource Pools.
func (a *Adapter) ListResourcePools(
	_ context.Context,
	filter *adapter.Filter,
) ([]*adapter.ResourcePool, error) {
	a.Logger.Debug("ListResourcePools called",
		zap.Any("filter", filter))

	// Query all host aggregates from Nova
	allPages, err := aggregates.List(a.compute).AllPages()
	if err != nil {
		a.Logger.Error("failed to list host aggregates",
			zap.Error(err))
		return nil, fmt.Errorf("failed to list OpenStack host aggregates: %w", err)
	}

	osAggregates, err := aggregates.ExtractAggregates(allPages)
	if err != nil {
		a.Logger.Error("failed to extract host aggregates",
			zap.Error(err))
		return nil, fmt.Errorf("failed to extract host aggregates: %w", err)
	}

	a.Logger.Debug("retrieved host aggregates from OpenStack",
		zap.Int("count", len(osAggregates)))

	// Transform OpenStack host aggregates to O2-IMS Resource Pools
	pools := make([]*adapter.ResourcePool, 0, len(osAggregates))
	for i := range osAggregates {
		pool := a.TransformHostAggregateToResourcePool(&osAggregates[i])

		// Apply filter
		if adapter.MatchesFilter(filter, pool.ResourcePoolID, "", pool.Location, nil) {
			pools = append(pools, pool)
		}
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.Logger.Info("listed resource pools",
		zap.Int("count", len(pools)))

	return pools, nil
}

// GetResourcePool retrieves a specific OpenStack host aggregate by ID and transforms it to O2-IMS Resource Pool.
func (a *Adapter) GetResourcePool(_ context.Context, id string) (*adapter.ResourcePool, error) {
	var aggregateID int
	if _, err := fmt.Sscanf(id, "openstack-aggregate-%d", &aggregateID); err != nil {
		return nil, fmt.Errorf("invalid resource pool ID format: %s", id)
	}
	osAggregate, err := aggregates.Get(a.compute, aggregateID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenStack host aggregate %d: %w", aggregateID, err)
	}
	return a.TransformHostAggregateToResourcePool(osAggregate), nil
}

// CreateResourcePool creates a new OpenStack host aggregate from an O2-IMS Resource Pool.
func (a *Adapter) CreateResourcePool(
	_ context.Context,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	a.Logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	if pool.Name == "" {
		return nil, fmt.Errorf("resource pool name is required")
	}

	// Extract availability zone from extensions or location
	availabilityZone := pool.Location
	if az, ok := pool.Extensions["openstack.availabilityZone"].(string); ok && az != "" {
		availabilityZone = az
	}

	// Create host aggregate in OpenStack
	createOpts := aggregates.CreateOpts{
		Name:             pool.Name,
		AvailabilityZone: availabilityZone,
	}

	osAggregate, err := aggregates.Create(a.compute, createOpts).Extract()
	if err != nil {
		a.Logger.Error("failed to create host aggregate",
			zap.String("name", pool.Name),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create OpenStack host aggregate: %w", err)
	}

	// If metadata is provided in extensions, set it
	if metadata, ok := pool.Extensions["openstack.metadata"].(map[string]string); ok && len(metadata) > 0 {
		// Convert map[string]string to map[string]interface{}
		metadataIface := make(map[string]interface{}, len(metadata))
		for k, v := range metadata {
			metadataIface[k] = v
		}
		setMetadataOpts := aggregates.SetMetadataOpts{
			Metadata: metadataIface,
		}

		osAggregate, err = aggregates.SetMetadata(a.compute, osAggregate.ID, setMetadataOpts).Extract()
		if err != nil {
			a.Logger.Warn("failed to set host aggregate metadata",
				zap.Int("aggregateID", osAggregate.ID),
				zap.Error(err))
			// Non-fatal: continue with aggregate creation
		}
	}

	// Transform back to O2-IMS Resource Pool
	createdPool := a.TransformHostAggregateToResourcePool(osAggregate)

	a.Logger.Info("created resource pool",
		zap.String("resourcePoolID", createdPool.ResourcePoolID),
		zap.String("name", createdPool.Name),
		zap.Int("aggregateID", osAggregate.ID))

	return createdPool, nil
}

// UpdateResourcePool updates an existing OpenStack host aggregate.
// Note: Only metadata can be updated; name and availability zone are immutable in OpenStack.
func (a *Adapter) UpdateResourcePool(
	_ context.Context,
	id string,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	a.Logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	// Parse resource pool ID to extract OpenStack aggregate ID
	var aggregateID int
	_, err := fmt.Sscanf(id, "openstack-aggregate-%d", &aggregateID)
	if err != nil {
		return nil, fmt.Errorf("invalid resource pool ID format: %s", id)
	}

	// Get existing host aggregate
	osAggregate, err := aggregates.Get(a.compute, aggregateID).Extract()
	if err != nil {
		a.Logger.Error("failed to get host aggregate",
			zap.Int("aggregateID", aggregateID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get OpenStack host aggregate %d: %w", aggregateID, err)
	}

	// Update metadata if provided
	if metadata, ok := pool.Extensions["openstack.metadata"].(map[string]string); ok && len(metadata) > 0 {
		// Convert map[string]string to map[string]interface{}
		metadataIface := make(map[string]interface{}, len(metadata))
		for k, v := range metadata {
			metadataIface[k] = v
		}
		setMetadataOpts := aggregates.SetMetadataOpts{
			Metadata: metadataIface,
		}

		osAggregate, err = aggregates.SetMetadata(a.compute, aggregateID, setMetadataOpts).Extract()
		if err != nil {
			a.Logger.Error("failed to update host aggregate metadata",
				zap.Int("aggregateID", aggregateID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to update OpenStack host aggregate metadata: %w", err)
		}
	}

	// Transform back to O2-IMS Resource Pool
	updatedPool := a.TransformHostAggregateToResourcePool(osAggregate)

	a.Logger.Info("updated resource pool",
		zap.String("resourcePoolID", updatedPool.ResourcePoolID),
		zap.String("name", updatedPool.Name))

	return updatedPool, nil
}

// DeleteResourcePool deletes an OpenStack host aggregate.
func (a *Adapter) DeleteResourcePool(_ context.Context, id string) error {
	a.Logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	// Parse resource pool ID to extract OpenStack aggregate ID
	var aggregateID int
	_, err := fmt.Sscanf(id, "openstack-aggregate-%d", &aggregateID)
	if err != nil {
		return fmt.Errorf("invalid resource pool ID format: %s", id)
	}

	// Delete host aggregate from OpenStack
	err = aggregates.Delete(a.compute, aggregateID).ExtractErr()
	if err != nil {
		a.Logger.Error("failed to delete host aggregate",
			zap.Int("aggregateID", aggregateID),
			zap.Error(err))
		return fmt.Errorf("failed to delete OpenStack host aggregate %d: %w", aggregateID, err)
	}

	a.Logger.Info("deleted resource pool",
		zap.String("resourcePoolID", id),
		zap.Int("aggregateID", aggregateID))

	return nil
}

// TransformHostAggregateToResourcePool converts an OpenStack host aggregate to O2-IMS Resource Pool.
func (a *Adapter) TransformHostAggregateToResourcePool(agg *aggregates.Aggregate) *adapter.ResourcePool {
	resourcePoolID := fmt.Sprintf("openstack-aggregate-%d", agg.ID)

	return &adapter.ResourcePool{
		ResourcePoolID:   resourcePoolID,
		Name:             agg.Name,
		Description:      fmt.Sprintf("OpenStack host aggregate: %s", agg.Name),
		Location:         agg.AvailabilityZone,
		OCloudID:         a.OCloudID,
		GlobalLocationID: "", // Not provided by OpenStack
		Extensions: map[string]interface{}{
			"openstack.aggregateId":      agg.ID,
			"openstack.name":             agg.Name,
			"openstack.availabilityZone": agg.AvailabilityZone,
			"openstack.metadata":         agg.Metadata,
			"openstack.hosts":            agg.Hosts,
			"openstack.hostCount":        len(agg.Hosts),
			"openstack.createdAt":        agg.CreatedAt,
			"openstack.updatedAt":        agg.UpdatedAt,
		},
	}
}
