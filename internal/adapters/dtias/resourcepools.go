package dtias

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResourcePools retrieves all server pools matching the provided filter.
// Maps DTIAS server pools to O2-IMS ResourcePools.
func (a *DTIASAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	a.logger.Debug("ListResourcePools called", zap.Any("filter", filter))

	path := buildServerPoolsPath(filter)

	serverPools, err := a.fetchServerPools(ctx, path)
	if err != nil {
		return nil, err
	}

	resourcePools := a.transformAndFilterPools(serverPools, filter)

	a.logger.Debug("listed resource pools", zap.Int("count", len(resourcePools)))
	return resourcePools, nil
}

// buildServerPoolsPath builds the API path with query parameters.
func buildServerPoolsPath(filter *adapter.Filter) string {
	path := "/v2/inventory/resourcepools"

	queryParams := url.Values{}
	if filter != nil {
		// DTIAS uses siteId instead of datacenter/location
		if filter.Location != "" {
			queryParams.Set("siteId", filter.Location)
		}
		// Note: DTIAS doesn't use limit/offset for resource pools
		// It returns all pools matching the filter
	}

	if len(queryParams) > 0 {
		path += "?" + queryParams.Encode()
	}
	return path
}

// fetchServerPools retrieves server pools from DTIAS API.
func (a *DTIASAdapter) fetchServerPools(ctx context.Context, path string) ([]ServerPool, error) {
	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list server pools: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Parse DTIAS wrapped response
	var dtiasResp ResourcePoolsInventoryResponse
	if err := a.client.parseResponse(resp, &dtiasResp); err != nil {
		return nil, fmt.Errorf("failed to parse server pools response: %w", err)
	}

	// Check for API errors in response
	if dtiasResp.Error != nil {
		return nil, dtiasResp.Error
	}

	// Return the Rps array (contains resource pools)
	return dtiasResp.Rps, nil
}

// transformAndFilterPools transforms server pools and applies client-side filtering.
func (a *DTIASAdapter) transformAndFilterPools(
	serverPools []ServerPool,
	filter *adapter.Filter,
) []*adapter.ResourcePool {
	resourcePools := make([]*adapter.ResourcePool, 0, len(serverPools))
	for _, sp := range serverPools {
		pool := a.transformServerPoolToResourcePool(&sp)

		// Apply client-side filtering using shared helper
		// Extract dtias metadata for label matching
		var labels map[string]string
		if pool.Extensions != nil {
			if metadata, ok := pool.Extensions["dtias.metadata"].(map[string]string); ok {
				labels = metadata
			}
		}
		if !adapter.MatchesFilter(filter, pool.ResourcePoolID, "", pool.Location, labels) {
			continue
		}
		resourcePools = append(resourcePools, pool)
	}
	return resourcePools
}

// GetResourcePool retrieves a specific server pool by ID.
// Maps a DTIAS server pool to O2-IMS ResourcePool.
func (a *DTIASAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

	// Query and parse DTIAS API wrapped response
	path := fmt.Sprintf("/v2/inventory/resourcepools/%s", id)
	var dtiasResp ResourcePoolInventoryResponse
	if err := a.getAndParseResource(ctx, path, &dtiasResp, "server pool"); err != nil {
		return nil, err
	}

	// Check for API errors in response
	if dtiasResp.Error != nil {
		return nil, dtiasResp.Error
	}

	// Transform to O2-IMS resource pool
	resourcePool := a.transformServerPoolToResourcePool(&dtiasResp.Rp)

	a.logger.Debug("retrieved resource pool",
		zap.String("id", resourcePool.ResourcePoolID),
		zap.String("name", resourcePool.Name))

	return resourcePool, nil
}

// CreateResourcePool creates a new server pool.
// Maps an O2-IMS ResourcePool to a DTIAS server pool and creates it.
func (a *DTIASAdapter) CreateResourcePool(
	ctx context.Context,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
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
	resp, err := a.client.doRequest(ctx, http.MethodPost, "/v2/resourcepools", createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create server pool: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

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
func (a *DTIASAdapter) UpdateResourcePool(
	ctx context.Context,
	id string,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
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
	path := fmt.Sprintf("/v2/resourcepools/%s", id)
	resp, err := a.client.doRequest(ctx, http.MethodPut, path, updateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update server pool: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

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
	path := fmt.Sprintf("/v2/resourcepools/%s", id)
	resp, err := a.client.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete server pool: %w", err)
	}
	_ = resp.Body.Close()

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

// NOTE: Filter matching uses shared helpers from internal/adapter/helpers.go
// Use adapter.MatchesFilter() instead of local implementation.
