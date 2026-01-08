package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResources retrieves all OpenStack Nova instances and transforms them to O2-IMS Resources.
// Nova instances (VMs) are the fundamental compute resources in OpenStack.
func (a *OpenStackAdapter) ListResources(_ context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// Build list options
	listOpts := servers.ListOpts{
		AllTenants: false, // Only list instances in current project
	}

	// Apply resource pool filter if specified
	// For now, we list all and filter in memory
	_ = filter // TODO(#58): implement resource pool filtering

	// Query all servers from Nova
	allPages, err := servers.List(a.compute, listOpts).AllPages()
	if err != nil {
		a.logger.Error("failed to list servers",
			zap.Error(err))
		return nil, fmt.Errorf("failed to list OpenStack servers: %w", err)
	}

	osServers, err := servers.ExtractServers(allPages)
	if err != nil {
		a.logger.Error("failed to extract servers",
			zap.Error(err))
		return nil, fmt.Errorf("failed to extract servers: %w", err)
	}

	a.logger.Debug("retrieved servers from OpenStack",
		zap.Int("count", len(osServers)))

	// Transform OpenStack servers to O2-IMS Resources
	resources := make([]*adapter.Resource, 0, len(osServers))
	for i := range osServers {
		resource := a.transformServerToResource(&osServers[i])

		// Apply filter
		if filter != nil {
			resourcePoolID := a.getResourcePoolIDFromServer(&osServers[i])
			if !adapter.MatchesFilter(filter, resourcePoolID, resource.ResourceTypeID, "", nil) {
				continue
			}
		}

		resources = append(resources, resource)
	}

	// Apply pagination
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific OpenStack Nova instance by ID and transforms it to O2-IMS Resource.
func (a *OpenStackAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Parse resource ID to extract OpenStack server ID
	var serverID string
	_, err := fmt.Sscanf(id, "openstack-server-%s", &serverID)
	if err != nil {
		return nil, fmt.Errorf("invalid resource ID format: %s", id)
	}

	// Get server from OpenStack
	osServer, err := servers.Get(a.compute, serverID).Extract()
	if err != nil {
		a.logger.Error("failed to get server",
			zap.String("serverID", serverID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get OpenStack server %s: %w", serverID, err)
	}

	// Transform to O2-IMS Resource
	resource := a.transformServerToResource(osServer)

	a.logger.Info("retrieved resource",
		zap.String("resourceID", resource.ResourceID),
		zap.String("name", osServer.Name))

	return resource, nil
}

// CreateResource creates a new OpenStack Nova instance from an O2-IMS Resource.
func (a *OpenStackAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeID", resource.ResourceTypeID))

	// Extract and validate required parameters
	flavorID, imageID, err := a.extractRequiredParams(resource)
	if err != nil {
		return nil, err
	}

	// Build server create options
	createOpts := a.buildCreateOptions(ctx, resource, flavorID, imageID)

	// Create server in OpenStack
	osServer, err := a.createOpenStackServer(createOpts)
	if err != nil {
		return nil, err
	}

	// Transform back to O2-IMS Resource
	createdResource := a.transformServerToResource(osServer)

	a.logger.Info("created resource",
		zap.String("resourceID", createdResource.ResourceID),
		zap.String("serverID", osServer.ID),
		zap.String("name", osServer.Name))

	return createdResource, nil
}

// extractRequiredParams extracts and validates required parameters from resource.
func (a *OpenStackAdapter) extractRequiredParams(resource *adapter.Resource) (string, string, error) {
	var flavorID, imageID string
	var err error
	if resource.ResourceTypeID == "" {
		return "", "", fmt.Errorf("resourceTypeID is required")
	}

	// Extract flavor ID from resource type ID
	_, err = fmt.Sscanf(resource.ResourceTypeID, "openstack-flavor-%s", &flavorID)
	if err != nil {
		return "", "", fmt.Errorf("invalid resourceTypeID format: %s", resource.ResourceTypeID)
	}

	// Extract required image ID from extensions
	imageID, ok := resource.Extensions["openstack.imageId"].(string)
	if !ok || imageID == "" {
		return "", "", fmt.Errorf("openstack.imageId is required in extensions")
	}

	return flavorID, imageID, nil
}

// buildCreateOptions builds OpenStack server create options from resource specification.
func (a *OpenStackAdapter) buildCreateOptions(
	ctx context.Context,
	resource *adapter.Resource,
	flavorID, imageID string,
) servers.CreateOpts {
	// Extract optional name parameter
	name := "openstack-instance"
	if n, ok := resource.Extensions["openstack.name"].(string); ok && n != "" {
		name = n
	}

	// Extract availability zone from resource pool
	availabilityZone := a.getAvailabilityZone(ctx, resource.ResourcePoolID)

	createOpts := servers.CreateOpts{
		Name:             name,
		FlavorRef:        flavorID,
		ImageRef:         imageID,
		AvailabilityZone: availabilityZone,
	}

	// Add optional network configuration
	a.addNetworkConfig(&createOpts, resource.Extensions)

	// Add optional security groups
	a.addSecurityGroups(&createOpts, resource.Extensions)

	return createOpts
}

// getAvailabilityZone retrieves availability zone from resource pool.
func (a *OpenStackAdapter) getAvailabilityZone(ctx context.Context, resourcePoolID string) string {
	if resourcePoolID == "" {
		return ""
	}

	pool, err := a.GetResourcePool(ctx, resourcePoolID)
	if err != nil {
		a.logger.Warn("failed to get resource pool for availability zone",
			zap.String("resourcePoolID", resourcePoolID),
			zap.Error(err))
		return ""
	}

	return pool.Location
}

// addNetworkConfig adds network configuration to create options if specified.
func (a *OpenStackAdapter) addNetworkConfig(opts *servers.CreateOpts, extensions map[string]interface{}) {
	networks, ok := extensions["openstack.networks"].([]string)
	if !ok || len(networks) == 0 {
		return
	}

	networksSlice := make([]servers.Network, len(networks))
	for i, netID := range networks {
		networksSlice[i] = servers.Network{UUID: netID}
	}
	opts.Networks = networksSlice
}

// addSecurityGroups adds security groups to create options if specified.
func (a *OpenStackAdapter) addSecurityGroups(opts *servers.CreateOpts, extensions map[string]interface{}) {
	securityGroups, ok := extensions["openstack.securityGroups"].([]string)
	if ok && len(securityGroups) > 0 {
		opts.SecurityGroups = securityGroups
	}
}

// createOpenStackServer creates a server in OpenStack and handles errors.
func (a *OpenStackAdapter) createOpenStackServer(createOpts servers.CreateOpts) (*servers.Server, error) {
	osServer, err := servers.Create(a.compute, createOpts).Extract()
	if err != nil {
		a.logger.Error("failed to create server",
			zap.String("name", createOpts.Name),
			zap.String("flavorID", createOpts.FlavorRef),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create OpenStack server: %w", err)
	}
	return osServer, nil
}

// DeleteResource deletes an OpenStack Nova instance.
func (a *OpenStackAdapter) DeleteResource(_ context.Context, id string) error {
	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Parse resource ID to extract OpenStack server ID
	var serverID string
	_, err := fmt.Sscanf(id, "openstack-server-%s", &serverID)
	if err != nil {
		return fmt.Errorf("invalid resource ID format: %s", id)
	}

	// Delete server from OpenStack
	err = servers.Delete(a.compute, serverID).ExtractErr()
	if err != nil {
		a.logger.Error("failed to delete server",
			zap.String("serverID", serverID),
			zap.Error(err))
		return fmt.Errorf("failed to delete OpenStack server %s: %w", serverID, err)
	}

	a.logger.Info("deleted resource",
		zap.String("resourceID", id),
		zap.String("serverID", serverID))

	return nil
}

// transformServerToResource converts an OpenStack server to O2-IMS Resource.
func (a *OpenStackAdapter) transformServerToResource(server *servers.Server) *adapter.Resource {
	resourceID := fmt.Sprintf("openstack-server-%s", server.ID)

	// Extract flavor ID
	flavorID := ""
	if server.Flavor != nil {
		if id, ok := server.Flavor["id"].(string); ok {
			flavorID = id
		}
	}

	resourceTypeID := fmt.Sprintf("openstack-flavor-%s", flavorID)

	// Get resource pool ID from availability zone
	resourcePoolID := a.getResourcePoolIDFromServer(server)

	// Build extensions with all server metadata
	extensions := map[string]interface{}{
		"openstack.serverId":  server.ID,
		"openstack.name":      server.Name,
		"openstack.status":    server.Status,
		"openstack.tenantId":  server.TenantID,
		"openstack.userId":    server.UserID,
		"openstack.hostId":    server.HostID,
		"openstack.created":   server.Created,
		"openstack.updated":   server.Updated,
		"openstack.addresses": server.Addresses,
		"openstack.metadata":  server.Metadata,
	}

	// Add flavor information
	if server.Flavor != nil {
		extensions["openstack.flavor"] = server.Flavor
	}

	// Add image information
	if server.Image != nil {
		extensions["openstack.image"] = server.Image
	}

	// Build description
	description := fmt.Sprintf("OpenStack instance: %s (status: %s)", server.Name, server.Status)

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:openstack:server:%s:%s", a.region, server.ID),
		Description:    description,
		Extensions:     extensions,
	}
}

// getResourcePoolIDFromServer derives the resource pool ID from a server's availability zone.
// This is a best-effort approach since OpenStack doesn't directly link servers to host aggregates.
func (a *OpenStackAdapter) getResourcePoolIDFromServer(_ *servers.Server) string {
	// In OpenStack, we can't directly determine which host aggregate a server belongs to
	// from the server object alone. We would need to query host aggregates and match
	// the server's host. For now, we return empty string or use availability zone as a proxy.

	// If we have availability zone, we could look up aggregates with that AZ
	// For simplicity, returning empty here; this could be enhanced to query aggregates
	return ""
}
