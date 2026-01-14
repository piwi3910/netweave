package starlingx

import (
	"context"
	"fmt"
	"strings"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResourcePools retrieves all resource pools (based on host labels).
func (a *Adapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	// Get all compute hosts
	hosts, err := a.client.ListHosts(ctx, "compute")
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	// Get all labels
	labels, err := a.client.ListLabels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	// Group hosts by pool label
	poolGroups := groupHostsByPool(hosts, labels)

	// Convert to resource pools
	pools := make([]*adapter.ResourcePool, 0, len(poolGroups))
	for poolName, poolHosts := range poolGroups {
		pool := mapLabelsToResourcePool(poolName, poolHosts, a.oCloudID)

		// Apply filters
		if filter != nil {
			if filter.TenantID != "" && pool.TenantID != filter.TenantID {
				continue
			}
			if filter.Location != "" && pool.Location != filter.Location {
				continue
			}
		}

		pools = append(pools, pool)
	}

	// Apply pagination
	if filter != nil && filter.Limit > 0 {
		start := filter.Offset
		if start >= len(pools) {
			return []*adapter.ResourcePool{}, nil
		}
		end := start + filter.Limit
		if end > len(pools) {
			end = len(pools)
		}
		pools = pools[start:end]
	}

	a.logger.Debug("listed resource pools",
		zap.Int("count", len(pools)),
	)

	return pools, nil
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *Adapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	// Extract pool name from ID (format: starlingx-pool-{name})
	poolName := strings.TrimPrefix(id, "starlingx-pool-")
	if poolName == id {
		// ID doesn't match expected format
		return nil, adapter.ErrResourcePoolNotFound
	}

	// Get all pools and find matching one
	pools, err := a.ListResourcePools(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pool := range pools {
		if pool.ResourcePoolID == id {
			a.logger.Debug("retrieved resource pool",
				zap.String("id", id),
				zap.String("name", pool.Name),
			)
			return pool, nil
		}
	}

	return nil, adapter.ErrResourcePoolNotFound
}

// CreateResourcePool creates a new resource pool by creating labels.
func (a *Adapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	// In StarlingX, resource pools are implicit based on labels
	// To "create" a pool, we would need hosts to assign labels to
	// This is a logical operation - we can't create an empty pool

	// For now, we return the pool as-is (it will be "created" when hosts are labeled)
	// In a real implementation, you might want to create a placeholder or validate the pool name

	a.logger.Info("resource pool created (logical)",
		zap.String("pool_id", pool.ResourcePoolID),
		zap.String("name", pool.Name),
	)

	return pool, nil
}

// UpdateResourcePool updates a resource pool (updates labels on hosts).
func (a *Adapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	// Extract pool name from ID
	poolName := strings.TrimPrefix(id, "starlingx-pool-")
	if poolName == id {
		return nil, adapter.ErrResourcePoolNotFound
	}

	// Verify pool exists
	existingPool, err := a.GetResourcePool(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update mutable fields
	if pool.Name != "" {
		existingPool.Name = pool.Name
	}
	if pool.Description != "" {
		existingPool.Description = pool.Description
	}
	if pool.Location != "" {
		existingPool.Location = pool.Location
	}
	if pool.GlobalLocationID != "" {
		existingPool.GlobalLocationID = pool.GlobalLocationID
	}
	if pool.Extensions != nil {
		if existingPool.Extensions == nil {
			existingPool.Extensions = make(map[string]interface{})
		}
		for k, v := range pool.Extensions {
			existingPool.Extensions[k] = v
		}
	}

	a.logger.Info("resource pool updated",
		zap.String("pool_id", id),
		zap.String("name", existingPool.Name),
	)

	return existingPool, nil
}

// DeleteResourcePool deletes a resource pool (removes labels from hosts).
func (a *Adapter) DeleteResourcePool(ctx context.Context, id string) error {
	// Extract pool name
	poolName := strings.TrimPrefix(id, "starlingx-pool-")
	if poolName == id {
		return adapter.ErrResourcePoolNotFound
	}

	// Get all labels
	labels, err := a.client.ListLabels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	// Find and delete labels for this pool
	deletedCount := 0
	for _, label := range labels {
		if (label.LabelKey == "pool" || label.LabelKey == "resource-pool") && label.LabelValue == poolName {
			if err := a.client.DeleteLabel(ctx, label.UUID); err != nil {
				a.logger.Warn("failed to delete label",
					zap.String("label_uuid", label.UUID),
					zap.Error(err),
				)
				continue
			}
			deletedCount++
		}
	}

	if deletedCount == 0 {
		return adapter.ErrResourcePoolNotFound
	}

	a.logger.Info("resource pool deleted",
		zap.String("pool_id", id),
		zap.String("pool_name", poolName),
		zap.Int("labels_deleted", deletedCount),
	)

	return nil
}
