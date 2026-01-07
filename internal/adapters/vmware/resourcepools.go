package vmware

import (
	"context"
	"fmt"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"
)

// ListResourcePools retrieves all resource pools matching the provided filter.
// In "cluster" mode, it lists vSphere Clusters.
// In "pool" mode, it lists vSphere Resource Pools.
func (a *VMwareAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) (pools []*adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "ListResourcePools", start, err) }()

	a.logger.Debug("ListResourcePools called",
		zap.Any("filter", filter),
		zap.String("poolMode", a.poolMode))

	if a.poolMode == "pool" {
		return a.listVSpherePools(ctx, filter)
	}
	return a.listClusterPools(ctx, filter)
}

// listClusterPools lists vSphere Clusters as resource pools.
func (a *VMwareAdapter) listClusterPools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	clusters, err := a.finder.ClusterComputeResourceList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	var pools []*adapter.ResourcePool
	for _, cluster := range clusters {
		clusterName := cluster.Name()
		poolID := generateClusterPoolID(clusterName)

		// Apply filter
		if !adapter.MatchesFilter(filter, poolID, "", clusterName, nil) {
			continue
		}

		// Get cluster properties
		var clusterMo mo.ClusterComputeResource
		err := cluster.Properties(ctx, cluster.Reference(), []string{"summary", "host", "datastore"}, &clusterMo)
		if err != nil {
			a.logger.Warn("failed to get cluster properties",
				zap.String("cluster", clusterName),
				zap.Error(err))
			continue
		}

		pool := &adapter.ResourcePool{
			ResourcePoolID: poolID,
			Name:           clusterName,
			Description:    fmt.Sprintf("vSphere Cluster %s", clusterName),
			Location:       clusterName,
			OCloudID:       a.oCloudID,
			Extensions: map[string]interface{}{
				"vmware.clusterName":    clusterName,
				"vmware.type":           "Cluster",
				"vmware.hostCount":      len(clusterMo.Host),
				"vmware.datastoreCount": len(clusterMo.Datastore),
			},
		}

		// Add summary info if available
		if clusterMo.Summary != nil {
			if summary, ok := clusterMo.Summary.(*types.ComputeResourceSummary); ok {
				pool.Extensions["vmware.totalCpu"] = summary.TotalCpu
				pool.Extensions["vmware.totalMemory"] = summary.TotalMemory
				pool.Extensions["vmware.numCpuCores"] = summary.NumCpuCores
				pool.Extensions["vmware.numCpuThreads"] = summary.NumCpuThreads
				pool.Extensions["vmware.effectiveCpu"] = summary.EffectiveCpu
				pool.Extensions["vmware.effectiveMemory"] = summary.EffectiveMemory
				pool.Extensions["vmware.numHosts"] = summary.NumHosts
				pool.Extensions["vmware.numEffectiveHosts"] = summary.NumEffectiveHosts
			}
		}

		pools = append(pools, pool)
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (cluster mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// listVSpherePools lists vSphere Resource Pools as resource pools.
func (a *VMwareAdapter) listVSpherePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	resourcePools, err := a.finder.ResourcePoolList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to list resource pools: %w", err)
	}

	var pools []*adapter.ResourcePool
	for _, rp := range resourcePools {
		poolName := rp.Name()

		// Get parent cluster name (if available)
		clusterName := "default"
		parent := rp.Reference().Value
		poolID := generateResourcePoolID(poolName, clusterName)

		// Apply filter
		if !adapter.MatchesFilter(filter, poolID, "", poolName, nil) {
			continue
		}

		// Get resource pool properties
		var rpMo mo.ResourcePool
		err := rp.Properties(ctx, rp.Reference(), []string{"summary", "config"}, &rpMo)
		if err != nil {
			a.logger.Warn("failed to get resource pool properties",
				zap.String("pool", poolName),
				zap.Error(err))
			continue
		}

		pool := &adapter.ResourcePool{
			ResourcePoolID: poolID,
			Name:           poolName,
			Description:    fmt.Sprintf("vSphere Resource Pool %s", poolName),
			Location:       poolName,
			OCloudID:       a.oCloudID,
			Extensions: map[string]interface{}{
				"vmware.poolName": poolName,
				"vmware.type":     "ResourcePool",
				"vmware.parent":   parent,
			},
		}

		// Add runtime info
		if rpMo.Summary != nil {
			if summary, ok := rpMo.Summary.(*types.ResourcePoolSummary); ok {
				runtime := &summary.Runtime
				pool.Extensions["vmware.overallStatus"] = string(runtime.OverallStatus)
				// Cpu and Memory are structs, not pointers, so they always exist
				pool.Extensions["vmware.cpuMaxUsage"] = runtime.Cpu.MaxUsage
				pool.Extensions["vmware.cpuOverallUsage"] = runtime.Cpu.OverallUsage
				pool.Extensions["vmware.cpuReservationUsed"] = runtime.Cpu.ReservationUsed
				pool.Extensions["vmware.memoryMaxUsage"] = runtime.Memory.MaxUsage
				pool.Extensions["vmware.memoryOverallUsage"] = runtime.Memory.OverallUsage
				pool.Extensions["vmware.memoryReservationUsed"] = runtime.Memory.ReservationUsed
			}
		}

		pools = append(pools, pool)
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (resource pool mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *VMwareAdapter) GetResourcePool(ctx context.Context, id string) (pool *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "GetResourcePool", start, err) }()

	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

	pools, err := a.ListResourcePools(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, p := range pools {
		if p.ResourcePoolID == id {
			return p, nil
		}
	}

	return nil, fmt.Errorf("resource pool not found: %s", id)
}

// CreateResourcePool creates a new resource pool.
func (a *VMwareAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (result *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "CreateResourcePool", start, err) }()

	a.logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	// Creating vSphere resource pools/clusters requires additional vSphere configuration
	return nil, fmt.Errorf("creating vSphere resource pools is not yet implemented")
}

// UpdateResourcePool updates an existing resource pool.
func (a *VMwareAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (result *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "UpdateResourcePool", start, err) }()

	a.logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	return nil, fmt.Errorf("updating vSphere resource pools is not yet implemented")
}

// DeleteResourcePool deletes a resource pool by ID.
func (a *VMwareAdapter) DeleteResourcePool(ctx context.Context, id string) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "DeleteResourcePool", start, err) }()

	a.logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	return fmt.Errorf("deleting vSphere resource pools is not yet implemented")
}
