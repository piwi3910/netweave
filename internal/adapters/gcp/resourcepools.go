package gcp

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// ListResourcePools retrieves all resource pools matching the provided filter.
// In "zone" mode, it lists Zones in the region.
// In "ig" mode, it lists Instance Groups.
func (a *GCPAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	a.logger.Debug("ListResourcePools called",
		zap.Any("filter", filter),
		zap.String("poolMode", a.poolMode))

	if a.poolMode == "ig" {
		return a.listIGPools(ctx, filter)
	}
	return a.listZonePools(ctx, filter)
}

// listZonePools lists Zones as resource pools.
func (a *GCPAdapter) listZonePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	var pools []*adapter.ResourcePool

	// List all zones in the project
	it := a.zonesClient.List(ctx, &computepb.ListZonesRequest{
		Project: a.projectID,
	})

	for {
		zone, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list zones: %w", err)
		}

		// Only include zones in the configured region
		zoneName := ptrToString(zone.Name)
		if !strings.HasPrefix(zoneName, a.region) {
			continue
		}

		poolID := generateZonePoolID(zoneName)

		// Apply filter
		if !adapter.MatchesFilter(filter, poolID, "", zoneName, nil) {
			continue
		}

		pool := &adapter.ResourcePool{
			ResourcePoolID: poolID,
			Name:           zoneName,
			Description:    fmt.Sprintf("GCP Zone %s", zoneName),
			Location:       zoneName,
			OCloudID:       a.oCloudID,
			Extensions: map[string]interface{}{
				"gcp.zone":        zoneName,
				"gcp.region":      ptrToString(zone.Region),
				"gcp.status":      ptrToString(zone.Status),
				"gcp.description": ptrToString(zone.Description),
			},
		}

		pools = append(pools, pool)
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (zone mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// listIGPools lists Instance Groups as resource pools.
func (a *GCPAdapter) listIGPools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	var pools []*adapter.ResourcePool

	// List zones in the region first
	zoneIt := a.zonesClient.List(ctx, &computepb.ListZonesRequest{
		Project: a.projectID,
	})

	for {
		zone, err := zoneIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list zones: %w", err)
		}

		zoneName := ptrToString(zone.Name)
		if !strings.HasPrefix(zoneName, a.region) {
			continue
		}

		// List instance groups in this zone
		igIt := a.instanceGroupsClient.List(ctx, &computepb.ListInstanceGroupsRequest{
			Project: a.projectID,
			Zone:    zoneName,
		})

		for {
			ig, err := igIt.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to list instance groups: %w", err)
			}

			igName := ptrToString(ig.Name)
			poolID := generateIGPoolID(igName, zoneName)

			// Apply filter
			if !adapter.MatchesFilter(filter, poolID, "", zoneName, nil) {
				continue
			}

			pool := &adapter.ResourcePool{
				ResourcePoolID: poolID,
				Name:           igName,
				Description:    ptrToString(ig.Description),
				Location:       zoneName,
				OCloudID:       a.oCloudID,
				Extensions: map[string]interface{}{
					"gcp.instanceGroup": igName,
					"gcp.zone":          zoneName,
					"gcp.size":          ptrToInt32(ig.Size),
					"gcp.selfLink":      ptrToString(ig.SelfLink),
					"gcp.fingerprint":   ptrToString(ig.Fingerprint),
				},
			}

			pools = append(pools, pool)
		}
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (instance group mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *GCPAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

	if a.poolMode == "ig" {
		return a.getIGPool(ctx, id)
	}
	return a.getZonePool(ctx, id)
}

// getZonePool retrieves a Zone as a resource pool.
func (a *GCPAdapter) getZonePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	pools, err := a.listZonePools(ctx, nil)
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

// getIGPool retrieves an Instance Group as a resource pool.
func (a *GCPAdapter) getIGPool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	pools, err := a.listIGPools(ctx, nil)
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
// In "zone" mode, this operation is not supported (zones are GCP-managed).
// In "ig" mode, this could create a new Instance Group.
func (a *GCPAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	a.logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	if a.poolMode == "zone" {
		return nil, fmt.Errorf("cannot create resource pools in 'zone' mode: zones are GCP-managed")
	}

	// In IG mode, we could create an Instance Group
	return nil, fmt.Errorf("creating Instance Groups is not yet implemented")
}

// UpdateResourcePool updates an existing resource pool.
func (a *GCPAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	a.logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	if a.poolMode == "zone" {
		return nil, fmt.Errorf("cannot update resource pools in 'zone' mode: zones are GCP-managed")
	}

	return nil, fmt.Errorf("updating Instance Groups is not yet implemented")
}

// DeleteResourcePool deletes a resource pool by ID.
func (a *GCPAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	a.logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	if a.poolMode == "zone" {
		return fmt.Errorf("cannot delete resource pools in 'zone' mode: zones are GCP-managed")
	}

	return fmt.Errorf("deleting Instance Groups is not yet implemented")
}
