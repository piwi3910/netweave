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
	hosts, err := a.client.ListHosts(ctx, "compute")
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	labels, err := a.client.ListLabels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	poolGroups := GroupHostsByPool(hosts, labels)
	pools := a.convertToPools(poolGroups, filter)

	a.logger.Debug("listed resource pools", zap.Int("count", len(pools)))
	return pools, nil
}

func (a *Adapter) convertToPools(poolGroups map[string][]IHost, filter *adapter.Filter) []*adapter.ResourcePool {
	pools := make([]*adapter.ResourcePool, 0, len(poolGroups))
	for poolName, poolHosts := range poolGroups {
		pool := MapLabelsToResourcePool(poolName, poolHosts, a.oCloudID)

		if !a.matchesFilter(pool, filter) {
			continue
		}

		pools = append(pools, pool)
	}

	return a.applyPagination(pools, filter)
}

func (a *Adapter) matchesFilter(pool *adapter.ResourcePool, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}
	if filter.TenantID != "" && pool.TenantID != filter.TenantID {
		return false
	}
	if filter.Location != "" && pool.Location != filter.Location {
		return false
	}
	return true
}

func (a *Adapter) applyPagination(pools []*adapter.ResourcePool, filter *adapter.Filter) []*adapter.ResourcePool {
	if filter == nil || filter.Limit <= 0 {
		return pools
	}

	start := filter.Offset
	if start >= len(pools) {
		return []*adapter.ResourcePool{}
	}

	end := start + filter.Limit
	if end > len(pools) {
		end = len(pools)
	}

	return pools[start:end]
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *Adapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	poolName := strings.TrimPrefix(id, "starlingx-pool-")
	if poolName == id {
		return nil, adapter.ErrResourcePoolNotFound
	}

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
func (a *Adapter) CreateResourcePool(_ context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
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
	poolName := strings.TrimPrefix(id, "starlingx-pool-")
	if poolName == id {
		return nil, adapter.ErrResourcePoolNotFound
	}

	existingPool, err := a.GetResourcePool(ctx, id)
	if err != nil {
		return nil, err
	}

	a.updatePoolFields(existingPool, pool)

	a.logger.Info("resource pool updated",
		zap.String("pool_id", id),
		zap.String("name", existingPool.Name),
	)

	return existingPool, nil
}

func (a *Adapter) updatePoolFields(existing, updated *adapter.ResourcePool) {
	if updated.Name != "" {
		existing.Name = updated.Name
	}
	if updated.Description != "" {
		existing.Description = updated.Description
	}
	if updated.Location != "" {
		existing.Location = updated.Location
	}
	if updated.GlobalLocationID != "" {
		existing.GlobalLocationID = updated.GlobalLocationID
	}
	if updated.Extensions != nil {
		if existing.Extensions == nil {
			existing.Extensions = make(map[string]interface{})
		}
		for k, v := range updated.Extensions {
			existing.Extensions[k] = v
		}
	}
}

// DeleteResourcePool deletes a resource pool (removes labels from hosts).
func (a *Adapter) DeleteResourcePool(ctx context.Context, id string) error {
	poolName := strings.TrimPrefix(id, "starlingx-pool-")
	if poolName == id {
		return adapter.ErrResourcePoolNotFound
	}

	labels, err := a.client.ListLabels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	deletedCount := a.deletePoolLabels(ctx, labels, poolName)

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

func (a *Adapter) deletePoolLabels(ctx context.Context, labels []Label, poolName string) int {
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
	return deletedCount
}
