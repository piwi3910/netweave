package dtias

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResourcePools retrieves all server pools matching the provided filter.
// Maps DTIAS server pools to O2-IMS ResourcePools.
func (a *DTIASAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	a.logger.Debug("ListResourcePools called",
		zap.Any("filter", filter))

	// Build query parameters
	queryParams := url.Values{}
	if filter != nil {
		if filter.Location != "" {
			queryParams.Set("datacenter", filter.Location)
		}
		if filter.Limit > 0 {
			queryParams.Set("limit", fmt.Sprintf("%d", filter.Limit))
		}
		if filter.Offset > 0 {
			queryParams.Set("offset", fmt.Sprintf("%d", filter.Offset))
		}
	}

	// Query DTIAS API
	path := "/server-pools"
	if len(queryParams) > 0 {
		path += "?" + queryParams.Encode()
	}

	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list server pools: %w", err)
	}

	// Parse response
	var serverPools []ServerPool
	if err := a.client.parseResponse(resp, &serverPools); err != nil {
		return nil, fmt.Errorf("failed to parse server pools response: %w", err)
	}

	// Transform DTIAS server pools to O2-IMS resource pools
	resourcePools := make([]*adapter.ResourcePool, 0, len(serverPools))
	for _, sp := range serverPools {
		pool := a.transformServerPoolToResourcePool(&sp)

		// Apply client-side filtering
		if filter != nil && !a.matchesFilter(pool, filter) {
			continue
		}

		resourcePools = append(resourcePools, pool)
	}

	a.logger.Debug("listed resource pools",
		zap.Int("count", len(resourcePools)))

	return resourcePools, nil
}

// GetResourcePool retrieves a specific server pool by ID.
// Maps a DTIAS server pool to O2-IMS ResourcePool.
func (a *DTIASAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

	// Query DTIAS API
	path := fmt.Sprintf("/server-pools/%s", id)
	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get server pool: %w", err)
	}

	// Parse response
	var serverPool ServerPool
	if err := a.client.parseResponse(resp, &serverPool); err != nil {
		return nil, fmt.Errorf("failed to parse server pool response: %w", err)
	}

	// Transform to O2-IMS resource pool
	resourcePool := a.transformServerPoolToResourcePool(&serverPool)

	a.logger.Debug("retrieved resource pool",
		zap.String("id", resourcePool.ResourcePoolID),
		zap.String("name", resourcePool.Name))

	return resourcePool, nil
}

// CreateResourcePool creates a new server pool.
// Maps an O2-IMS ResourcePool to a DTIAS server pool and creates it.
func (a *DTIASAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	a.logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	// Transform O2-IMS resource pool to DTIAS server pool request
	createReq := map[string]interface{}{
		"name":        pool.Name,
		"description": pool.Description,
		"datacenter":  a.config.Datacenter,
		"type":        "compute", // Default type
		"metadata":    map[string]string{},
	}

	// Copy location if provided
	if pool.Location != "" {
		createReq["datacenter"] = pool.Location
	}

	// Copy extensions to metadata
	if pool.Extensions != nil {
		if poolType, ok := pool.Extensions["dtias.poolType"].(string); ok {
			createReq["type"] = poolType
		}
		if metadata, ok := pool.Extensions["dtias.metadata"].(map[string]string); ok {
			createReq["metadata"] = metadata
		}
	}

	// Create server pool via DTIAS API
	resp, err := a.client.doRequest(ctx, http.MethodPost, "/server-pools", createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create server pool: %w", err)
	}

	// Parse response
	var serverPool ServerPool
	if err := a.client.parseResponse(resp, &serverPool); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	// Transform to O2-IMS resource pool
	resourcePool := a.transformServerPoolToResourcePool(&serverPool)

	a.logger.Info("created resource pool",
		zap.String("id", resourcePool.ResourcePoolID),
		zap.String("name", resourcePool.Name))

	return resourcePool, nil
}

// UpdateResourcePool updates an existing server pool.
// Maps O2-IMS ResourcePool updates to DTIAS server pool updates.
func (a *DTIASAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	a.logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	// Transform O2-IMS resource pool to DTIAS server pool update request
	updateReq := map[string]interface{}{
		"name":        pool.Name,
		"description": pool.Description,
		"metadata":    map[string]string{},
	}

	// Copy extensions to metadata
	if pool.Extensions != nil {
		if metadata, ok := pool.Extensions["dtias.metadata"].(map[string]string); ok {
			updateReq["metadata"] = metadata
		}
	}

	// Update server pool via DTIAS API
	path := fmt.Sprintf("/server-pools/%s", id)
	resp, err := a.client.doRequest(ctx, http.MethodPut, path, updateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update server pool: %w", err)
	}

	// Parse response
	var serverPool ServerPool
	if err := a.client.parseResponse(resp, &serverPool); err != nil {
		return nil, fmt.Errorf("failed to parse update response: %w", err)
	}

	// Transform to O2-IMS resource pool
	resourcePool := a.transformServerPoolToResourcePool(&serverPool)

	a.logger.Info("updated resource pool",
		zap.String("id", resourcePool.ResourcePoolID),
		zap.String("name", resourcePool.Name))

	return resourcePool, nil
}

// DeleteResourcePool deletes a server pool by ID.
func (a *DTIASAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	a.logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	// Delete server pool via DTIAS API
	path := fmt.Sprintf("/server-pools/%s", id)
	resp, err := a.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete server pool: %w", err)
	}
	resp.Body.Close()

	a.logger.Info("deleted resource pool",
		zap.String("id", id))

	return nil
}

// transformServerPoolToResourcePool transforms a DTIAS ServerPool to an O2-IMS ResourcePool.
func (a *DTIASAdapter) transformServerPoolToResourcePool(sp *ServerPool) *adapter.ResourcePool {
	// Build global location ID (geo URI format)
	globalLocationID := ""
	if sp.Location.Latitude != 0 && sp.Location.Longitude != 0 {
		globalLocationID = fmt.Sprintf("geo:%.6f,%.6f", sp.Location.Latitude, sp.Location.Longitude)
	}

	// Build location string
	location := sp.Datacenter
	if sp.Location.City != "" {
		location = fmt.Sprintf("%s, %s", sp.Location.City, sp.Datacenter)
	}

	return &adapter.ResourcePool{
		ResourcePoolID:   sp.ID,
		Name:             sp.Name,
		Description:      sp.Description,
		Location:         location,
		OCloudID:         a.oCloudID,
		GlobalLocationID: globalLocationID,
		Extensions: map[string]interface{}{
			"dtias.poolId":           sp.ID,
			"dtias.poolType":         sp.Type,
			"dtias.state":            sp.State,
			"dtias.datacenter":       sp.Datacenter,
			"dtias.serverCount":      sp.ServerCount,
			"dtias.availableServers": sp.AvailableServers,
			"dtias.location":         sp.Location,
			"dtias.metadata":         sp.Metadata,
			"dtias.createdAt":        sp.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"dtias.updatedAt":        sp.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
}

// matchesFilter checks if a resource pool matches the provided filter.
func (a *DTIASAdapter) matchesFilter(pool *adapter.ResourcePool, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}

	// Filter by location
	if filter.Location != "" && pool.Location != filter.Location {
		return false
	}

	// Filter by labels
	if len(filter.Labels) > 0 {
		poolMetadata, ok := pool.Extensions["dtias.metadata"].(map[string]string)
		if !ok {
			return false
		}
		for key, value := range filter.Labels {
			if poolMetadata[key] != value {
				return false
			}
		}
	}

	return true
}
