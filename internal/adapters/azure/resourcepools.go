package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResourcePools retrieves all resource pools matching the provided filter.
// In "rg" mode, it lists Resource Groups.
// In "az" mode, it lists Availability Zones.
func (a *AzureAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) (pools []*adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "ListResourcePools", start, err) }()

	a.logger.Debug("ListResourcePools called",
		zap.Any("filter", filter),
		zap.String("poolMode", a.poolMode))

	if a.poolMode == "az" {
		return a.listAZPools(ctx, filter)
	}
	return a.listRGPools(ctx, filter)
}

// listRGPools lists Resource Groups as resource pools.
func (a *AzureAdapter) listRGPools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	var pools []*adapter.ResourcePool

	pager := a.resourceGroupClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list resource groups: %w", err)
		}

		for _, rg := range page.Value {
			// Only include resource groups in the configured location
			location := ptrToString(rg.Location)
			if location != a.location {
				continue
			}

			rgName := ptrToString(rg.Name)
			poolID := generateRGPoolID(rgName)

			// Convert tags to labels
			labels := tagsToMap(rg.Tags)

			// Apply filter
			if !adapter.MatchesFilter(filter, poolID, "", location, labels) {
				continue
			}

			pool := &adapter.ResourcePool{
				ResourcePoolID: poolID,
				Name:           rgName,
				Description:    fmt.Sprintf("Azure Resource Group %s", rgName),
				Location:       location,
				OCloudID:       a.oCloudID,
				Extensions: map[string]interface{}{
					"azure.resourceGroupId":   ptrToString(rg.ID),
					"azure.resourceGroupName": rgName,
					"azure.location":          location,
					"azure.provisioningState": string(*rg.Properties.ProvisioningState),
					"azure.managedBy":         ptrToString(rg.ManagedBy),
					"azure.tags":              labels,
				},
			}

			pools = append(pools, pool)
		}
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (RG mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// listAZPools lists Availability Zones as resource pools.
func (a *AzureAdapter) listAZPools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	// Azure has 3 availability zones (1, 2, 3) in supported regions
	// Not all regions support availability zones, but we'll list them anyway
	zones := []string{"1", "2", "3"}

	pools := make([]*adapter.ResourcePool, 0, len(zones))
	for _, zone := range zones {
		poolID := generateAZPoolID(a.location, zone)
		zoneName := fmt.Sprintf("%s-%s", a.location, zone)

		// Apply filter
		if !adapter.MatchesFilter(filter, poolID, "", zoneName, nil) {
			continue
		}

		pool := &adapter.ResourcePool{
			ResourcePoolID: poolID,
			Name:           zoneName,
			Description:    fmt.Sprintf("Azure Availability Zone %s in %s", zone, a.location),
			Location:       zoneName,
			OCloudID:       a.oCloudID,
			Extensions: map[string]interface{}{
				"azure.zone":     zone,
				"azure.location": a.location,
				"azure.type":     "AvailabilityZone",
			},
		}

		pools = append(pools, pool)
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (AZ mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *AzureAdapter) GetResourcePool(ctx context.Context, id string) (pool *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "GetResourcePool", start, err) }()

	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

	if a.poolMode == "az" {
		return a.getAZPool(ctx, id)
	}
	return a.getRGPool(ctx, id)
}

// getRGPool retrieves a Resource Group as a resource pool.
func (a *AzureAdapter) getRGPool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	pools, err := a.listRGPools(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pool := range pools {
		if pool.ResourcePoolID == id {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("resource pool not found: %s", id)
}

// getAZPool retrieves an Availability Zone as a resource pool.
func (a *AzureAdapter) getAZPool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	pools, err := a.listAZPools(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pool := range pools {
		if pool.ResourcePoolID == id {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("resource pool not found: %s", id)
}

// CreateResourcePool creates a new resource pool.
// In "rg" mode, this creates a new Resource Group.
// In "az" mode, this operation is not supported (AZs are Azure-managed).
func (a *AzureAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (result *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "CreateResourcePool", start, err) }()

	a.logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	if a.poolMode == "az" {
		return nil, fmt.Errorf("cannot create resource pools in 'az' mode: availability zones are Azure-managed")
	}

	// In RG mode, we could create a Resource Group
	// This requires the Resource Group name and location
	return nil, fmt.Errorf("creating Resource Groups is not yet implemented")
}

// UpdateResourcePool updates an existing resource pool.
func (a *AzureAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (result *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "UpdateResourcePool", start, err) }()

	a.logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	if a.poolMode == "az" {
		return nil, fmt.Errorf("cannot update resource pools in 'az' mode: availability zones are Azure-managed")
	}

	return nil, fmt.Errorf("updating Resource Groups is not yet implemented")
}

// DeleteResourcePool deletes a resource pool by ID.
func (a *AzureAdapter) DeleteResourcePool(ctx context.Context, id string) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("azure", "DeleteResourcePool", start, err) }()

	a.logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	if a.poolMode == "az" {
		return fmt.Errorf("cannot delete resource pools in 'az' mode: availability zones are Azure-managed")
	}

	return fmt.Errorf("deleting Resource Groups is not yet implemented")
}
