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
func (a *OpenStackAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// Build list options with server-side filters
	listOpts := a.buildListOptions(ctx, filter)

	// Query OpenStack for servers
	osServers, err := a.queryOpenStackServers(listOpts)
	if err != nil {
		return nil, err
	}

	// Transform and filter servers to resources
	resources := a.transformAndFilterServers(osServers, filter)

	// Apply pagination
	resources = a.applyPaginationIfNeeded(resources, filter)

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// buildListOptions constructs OpenStack list options with server-side filters.
func (a *OpenStackAdapter) buildListOptions(ctx context.Context, filter *adapter.Filter) servers.ListOpts {
	listOpts := servers.ListOpts{
		AllTenants: false, // Only list instances in current project
	}

	if filter == nil {
		return listOpts
	}

	// Determine availability zone from resource pool or location
	availabilityZone := a.getAvailabilityZoneFromFilter(ctx, filter)
	if availabilityZone != "" {
		listOpts.AvailabilityZone = availabilityZone
		a.logger.Debug("filtering servers by availability zone",
			zap.String("availabilityZone", availabilityZone))
	}

	return listOpts
}

// getAvailabilityZoneFromFilter extracts availability zone from filter parameters.
func (a *OpenStackAdapter) getAvailabilityZoneFromFilter(ctx context.Context, filter *adapter.Filter) string {
	// If filtering by location directly, use that
	if filter.Location != "" {
		return filter.Location
	}

	// If filtering by resource pool ID, get the pool's availability zone
	if filter.ResourcePoolID != "" {
		pool, err := a.GetResourcePool(ctx, filter.ResourcePoolID)
		if err != nil {
			a.logger.Warn("failed to get resource pool for filtering, will filter in memory",
				zap.String("resourcePoolID", filter.ResourcePoolID),
				zap.Error(err))
			return ""
		}
		return pool.Location
	}

	return ""
}

// queryOpenStackServers retrieves servers from OpenStack Nova API.
func (a *OpenStackAdapter) queryOpenStackServers(listOpts servers.ListOpts) ([]servers.Server, error) {
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

	return osServers, nil
}

// transformAndFilterServers transforms OpenStack servers to resources and applies filters.
func (a *OpenStackAdapter) transformAndFilterServers(osServers []servers.Server, filter *adapter.Filter) []*adapter.Resource {
	resources := make([]*adapter.Resource, 0, len(osServers))
	for i := range osServers {
		resource := a.transformServerToResource(&osServers[i])

		// Apply additional in-memory filtering
		if filter != nil && !a.resourceMatchesFilter(resource, filter) {
			continue
		}

		resources = append(resources, resource)
	}
	return resources
}

// resourceMatchesFilter checks if a resource matches the given filter criteria.
func (a *OpenStackAdapter) resourceMatchesFilter(resource *adapter.Resource, filter *adapter.Filter) bool {
	// ResourceTypeID filter (by flavor)
	if filter.ResourceTypeID != "" && filter.ResourceTypeID != resource.ResourceTypeID {
		return false
	}

	// Labels filtering - not typically supported by OpenStack servers directly
	if len(filter.Labels) > 0 {
		return false // Skip for now
	}

	return true
}

// applyPaginationIfNeeded applies pagination to resources if filter specifies it.
func (a *OpenStackAdapter) applyPaginationIfNeeded(resources []*adapter.Resource, filter *adapter.Filter) []*adapter.Resource {
	if filter != nil {
		return adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}
	return resources
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

// UpdateResource updates an existing OpenStack instance's metadata.
// Note: Core instance properties cannot be modified after creation.
func (a *OpenStackAdapter) UpdateResource(_ context.Context, _ string, resource *adapter.Resource) (*adapter.Resource, error) {
	a.logger.Debug("UpdateResource called",
		zap.String("resourceID", resource.ResourceID))

	// TODO(#191): Implement instance metadata updates via OpenStack API
	// For now, return not supported
	return nil, fmt.Errorf("updating OpenStack instances is not yet implemented")
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
// This is a best-effort mapping that requires the OS-EXT-AZ extension to be available.
// In production OpenStack deployments, use the availability zone stored in server.Metadata
// or query with the OS-EXT-AZ extension enabled.
func (a *OpenStackAdapter) getResourcePoolIDFromServer(_ *servers.Server) string {
	// Note: The standard gophercloud servers.Server struct doesn't include
	// the availability zone field. To get this information, you need to:
	// 1. Use the OS-EXT-AZ:availability_zone extension when fetching servers
	// 2. Store the AZ in server metadata during creation
	// 3. Query the server details with extensions
	//
	// For now, return empty string. Resource pool filtering works at the
	// API level in ListResources using the availabilityZone query parameter.
	return ""
}
