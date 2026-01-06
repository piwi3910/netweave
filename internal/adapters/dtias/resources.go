package dtias

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResources retrieves all physical servers matching the provided filter.
// Maps DTIAS servers to O2-IMS Resources.
func (a *DTIASAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// Build query parameters
	queryParams := url.Values{}
	if filter != nil {
		if filter.ResourcePoolID != "" {
			queryParams.Set("serverPoolId", filter.ResourcePoolID)
		}
		if filter.ResourceTypeID != "" {
			queryParams.Set("serverType", filter.ResourceTypeID)
		}
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
	path := "/servers"
	if len(queryParams) > 0 {
		path += "?" + queryParams.Encode()
	}

	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	// Parse response
	var servers []Server
	if err := a.client.parseResponse(resp, &servers); err != nil {
		return nil, fmt.Errorf("failed to parse servers response: %w", err)
	}

	// Transform DTIAS servers to O2-IMS resources
	resources := make([]*adapter.Resource, 0, len(servers))
	for _, srv := range servers {
		resource := a.transformServerToResource(&srv)

		// Apply client-side filtering
		if filter != nil && !a.matchesResourceFilter(resource, filter) {
			continue
		}

		resources = append(resources, resource)
	}

	a.logger.Debug("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific physical server by ID.
// Maps a DTIAS server to O2-IMS Resource.
func (a *DTIASAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Query DTIAS API
	path := fmt.Sprintf("/servers/%s", id)
	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	// Parse response
	var server Server
	if err := a.client.parseResponse(resp, &server); err != nil {
		return nil, fmt.Errorf("failed to parse server response: %w", err)
	}

	// Transform to O2-IMS resource
	resource := a.transformServerToResource(&server)

	a.logger.Debug("retrieved resource",
		zap.String("id", resource.ResourceID),
		zap.String("hostname", server.Hostname))

	return resource, nil
}

// CreateResource provisions a new physical server.
// Maps an O2-IMS Resource to a DTIAS server provisioning request.
func (a *DTIASAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Transform O2-IMS resource to DTIAS server provisioning request
	provisionReq := ServerProvisionRequest{
		ServerPoolID: resource.ResourcePoolID,
		ServerTypeID: resource.ResourceTypeID,
		Hostname:     "",
		Metadata:     map[string]string{},
	}

	// Extract hostname and metadata from extensions
	if resource.Extensions != nil {
		if hostname, ok := resource.Extensions["dtias.hostname"].(string); ok {
			provisionReq.Hostname = hostname
		}
		if os, ok := resource.Extensions["dtias.operatingSystem"].(string); ok {
			provisionReq.OperatingSystem = os
		}
		if networkConfig, ok := resource.Extensions["dtias.networkConfig"].(map[string]interface{}); ok {
			provisionReq.NetworkConfig = networkConfig
		}
		if metadata, ok := resource.Extensions["dtias.metadata"].(map[string]string); ok {
			provisionReq.Metadata = metadata
		}
	}

	// Provision server via DTIAS API
	resp, err := a.client.doRequest(ctx, http.MethodPost, "/servers/provision", provisionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to provision server: %w", err)
	}

	// Parse response
	var server Server
	if err := a.client.parseResponse(resp, &server); err != nil {
		return nil, fmt.Errorf("failed to parse provision response: %w", err)
	}

	// Transform to O2-IMS resource
	createdResource := a.transformServerToResource(&server)

	a.logger.Info("provisioned resource",
		zap.String("id", createdResource.ResourceID),
		zap.String("hostname", server.Hostname),
		zap.String("serverType", server.Type))

	return createdResource, nil
}

// DeleteResource deprovisions a physical server.
// Decommissions the DTIAS server and returns it to the available pool.
func (a *DTIASAdapter) DeleteResource(ctx context.Context, id string) error {
	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Decommission server via DTIAS API
	path := fmt.Sprintf("/servers/%s/decommission", id)
	resp, err := a.client.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("failed to decommission server: %w", err)
	}
	resp.Body.Close()

	a.logger.Info("decommissioned resource",
		zap.String("id", id))

	return nil
}

// transformServerToResource transforms a DTIAS Server to an O2-IMS Resource.
func (a *DTIASAdapter) transformServerToResource(srv *Server) *adapter.Resource {
	// Build global asset ID (URN format)
	globalAssetID := fmt.Sprintf("urn:dtias:server:%s", srv.ID)

	// Build description
	description := fmt.Sprintf("Physical server: %s (%s)", srv.Hostname, srv.Type)
	if srv.HealthState != "healthy" {
		description += fmt.Sprintf(" [health: %s]", srv.HealthState)
	}

	return &adapter.Resource{
		ResourceID:     srv.ID,
		ResourceTypeID: fmt.Sprintf("dtias-server-type-%s", srv.Type),
		ResourcePoolID: srv.ServerPoolID,
		GlobalAssetID:  globalAssetID,
		Description:    description,
		Extensions: map[string]interface{}{
			// Server identification
			"dtias.serverId":   srv.ID,
			"dtias.hostname":   srv.Hostname,
			"dtias.serverType": srv.Type,
			"dtias.state":      srv.State,

			// Power and health
			"dtias.powerState":  srv.PowerState,
			"dtias.healthState": srv.HealthState,

			// CPU information
			"dtias.cpu.vendor":         srv.CPU.Vendor,
			"dtias.cpu.model":          srv.CPU.Model,
			"dtias.cpu.architecture":   srv.CPU.Architecture,
			"dtias.cpu.sockets":        srv.CPU.Sockets,
			"dtias.cpu.coresPerSocket": srv.CPU.CoresPerSocket,
			"dtias.cpu.totalCores":     srv.CPU.TotalCores,
			"dtias.cpu.totalThreads":   srv.CPU.TotalThreads,
			"dtias.cpu.frequencyMhz":   srv.CPU.FrequencyMHz,

			// Memory information
			"dtias.memory.totalGb":        srv.Memory.TotalGB,
			"dtias.memory.availableGb":    srv.Memory.AvailableGB,
			"dtias.memory.type":           srv.Memory.Type,
			"dtias.memory.speedMhz":       srv.Memory.SpeedMHz,
			"dtias.memory.dimms":          srv.Memory.DIMMs,
			"dtias.memory.slotsUsed":      srv.Memory.SlotsUsed,
			"dtias.memory.slotsAvailable": srv.Memory.SlotsAvailable,

			// Storage summary
			"dtias.storage.devices": len(srv.Storage),
			"dtias.storage.details": srv.Storage,

			// Network summary
			"dtias.network.interfaces": len(srv.Network),
			"dtias.network.details":    srv.Network,

			// BIOS information
			"dtias.bios.vendor":      srv.BIOS.Vendor,
			"dtias.bios.version":     srv.BIOS.Version,
			"dtias.bios.releaseDate": srv.BIOS.ReleaseDate,

			// Management information (iDRAC/BMC)
			"dtias.management.type":       srv.Management.Type,
			"dtias.management.version":    srv.Management.Version,
			"dtias.management.ipAddress":  srv.Management.IPAddress,
			"dtias.management.macAddress": srv.Management.MACAddress,
			"dtias.management.hostname":   srv.Management.Hostname,

			// Location information
			"dtias.location.datacenter": srv.Location.Datacenter,
			"dtias.location.rack":       srv.Location.Rack,
			"dtias.location.rackUnit":   srv.Location.RackUnit,
			"dtias.location.row":        srv.Location.Row,
			"dtias.location.city":       srv.Location.City,
			"dtias.location.country":    srv.Location.Country,

			// Metadata
			"dtias.metadata": srv.Metadata,

			// Timestamps
			"dtias.createdAt":       srv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"dtias.updatedAt":       srv.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"dtias.lastHealthCheck": srv.LastHealthCheck.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
}

// matchesResourceFilter checks if a resource matches the provided filter.
func (a *DTIASAdapter) matchesResourceFilter(resource *adapter.Resource, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}

	// Filter by resource pool
	if filter.ResourcePoolID != "" && resource.ResourcePoolID != filter.ResourcePoolID {
		return false
	}

	// Filter by resource type
	if filter.ResourceTypeID != "" && resource.ResourceTypeID != filter.ResourceTypeID {
		return false
	}

	// Filter by labels (check metadata)
	if len(filter.Labels) > 0 {
		resourceMetadata, ok := resource.Extensions["dtias.metadata"].(map[string]string)
		if !ok {
			return false
		}
		for key, value := range filter.Labels {
			if resourceMetadata[key] != value {
				return false
			}
		}
	}

	return true
}

// PowerControl performs a power management operation on a server.
// This is a DTIAS-specific operation not directly mapped to O2-IMS.
func (a *DTIASAdapter) PowerControl(ctx context.Context, serverID string, operation ServerPowerOperation) error {
	a.logger.Debug("PowerControl called",
		zap.String("serverId", serverID),
		zap.String("operation", string(operation)))

	// Power control via DTIAS API
	path := fmt.Sprintf("/servers/%s/power", serverID)
	req := map[string]interface{}{
		"operation": operation,
	}

	resp, err := a.client.doRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return fmt.Errorf("failed to perform power operation: %w", err)
	}
	resp.Body.Close()

	a.logger.Info("executed power control",
		zap.String("serverId", serverID),
		zap.String("operation", string(operation)))

	return nil
}

// GetHealthMetrics retrieves hardware health metrics for a server.
// This is a DTIAS-specific operation for monitoring server health.
func (a *DTIASAdapter) GetHealthMetrics(ctx context.Context, serverID string) (*HealthMetrics, error) {
	a.logger.Debug("GetHealthMetrics called",
		zap.String("serverId", serverID))

	// Query health metrics via DTIAS API
	path := fmt.Sprintf("/servers/%s/health/metrics", serverID)
	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get health metrics: %w", err)
	}

	// Parse response
	var metrics HealthMetrics
	if err := a.client.parseResponse(resp, &metrics); err != nil {
		return nil, fmt.Errorf("failed to parse health metrics response: %w", err)
	}

	a.logger.Debug("retrieved health metrics",
		zap.String("serverId", serverID),
		zap.Float64("cpuUtilization", metrics.CPUUtilization),
		zap.Float64("memoryUtilization", metrics.MemoryUtilization))

	return &metrics, nil
}
