package osm

import (
	"context"
	"fmt"
)

// InfrastructureInventory represents infrastructure resources for synchronization.
// This is typically sourced from O2-IMS resource pools and resources.
type InfrastructureInventory struct {
	// VIMAccounts contains VIM (Virtual Infrastructure Manager) account information
	VIMAccounts []*VIMAccount `json:"vimAccounts"`

	// ResourcePools contains logical groupings of infrastructure resources
	ResourcePools []*ResourcePool `json:"resourcePools,omitempty"`
}

// VIMAccount represents a VIM (Virtual Infrastructure Manager) account in OSM.
// VIMs are the infrastructure backends (OpenStack, Kubernetes, VMware, etc.) that
// OSM uses to deploy and manage network services.
type VIMAccount struct {
	// ID is the unique identifier for this VIM account
	ID string `json:"_id,omitempty"`

	// Name is the human-readable name for this VIM account
	Name string `json:"name"`

	// VIMType specifies the type of VIM (openstack, kubernetes, vmware, etc.)
	VIMType string `json:"vim_type"`

	// VIMUrl is the API endpoint URL for the VIM
	VIMURL string `json:"vim_url"`

	// VIMUser is the username for VIM authentication
	VIMUser string `json:"vim_user,omitempty"`

	// VIMPassword is the password for VIM authentication
	VIMPassword string `json:"vim_password,omitempty"`

	// VIMTenantName is the tenant/project name in the VIM
	VIMTenantName string `json:"vim_tenant_name,omitempty"`

	// Config contains additional VIM-specific configuration
	Config map[string]interface{} `json:"config,omitempty"`

	// Description provides additional context about this VIM
	Description string `json:"description,omitempty"`
}

// ResourcePool represents a logical grouping of infrastructure resources.
type ResourcePool struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Location    string                 `json:"location,omitempty"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

// InfrastructureEvent represents an infrastructure change event.
type InfrastructureEvent struct {
	// EventType specifies the type of event (created, updated, deleted)
	EventType string `json:"eventType"`

	// ResourceType specifies what type of resource changed (resource_pool, resource, etc.)
	ResourceType string `json:"resourceType"`

	// ResourceID is the unique identifier of the affected resource
	ResourceID string `json:"resourceId"`

	// Timestamp is when the event occurred
	Timestamp string `json:"timestamp"`

	// Data contains the resource data (for created/updated events)
	Data map[string]interface{} `json:"data,omitempty"`
}

// syncOSMInfrastructure synchronizes OSM-specific infrastructure inventory to OSM.
// This is an internal method for VIM account synchronization.
//
// Northbound flow:
//  1. netweave O2-IMS discovers infrastructure (resource pools, resources)
//  2. netweave transforms infrastructure to VIM account representation
//  3. This method syncs VIM accounts to OSM
//  4. OSM can now deploy NS/VNF to the registered infrastructure
func (p *Plugin) syncOSMInfrastructure(ctx context.Context, inventory *InfrastructureInventory) error {
	if inventory == nil {
		return fmt.Errorf("inventory cannot be nil")
	}

	// Sync each VIM account
	for _, vim := range inventory.VIMAccounts {
		if err := p.syncVIMAccount(ctx, vim); err != nil {
			return fmt.Errorf("failed to sync VIM account %s: %w", vim.Name, err)
		}
	}

	return nil
}

// syncVIMAccount creates or updates a single VIM account in OSM.
func (p *Plugin) syncVIMAccount(ctx context.Context, vim *VIMAccount) error {
	// Check if VIM account already exists
	existingVIM, err := p.GetVIMAccount(ctx, vim.ID)
	if err == nil && existingVIM != nil {
		// VIM exists - update it
		return p.UpdateVIMAccount(ctx, vim.ID, vim)
	}

	// VIM doesn't exist - create it
	return p.CreateVIMAccount(ctx, vim)
}

// CreateVIMAccount creates a new VIM account in OSM.
// This registers a new infrastructure backend that OSM can use for deployments.
func (p *Plugin) CreateVIMAccount(ctx context.Context, vim *VIMAccount) error {
	if vim == nil {
		return fmt.Errorf("vim cannot be nil")
	}

	// Validate required fields
	if vim.Name == "" {
		return fmt.Errorf("vim name is required")
	}
	if vim.VIMType == "" {
		return fmt.Errorf("vim type is required")
	}
	if vim.VIMURL == "" {
		return fmt.Errorf("vim url is required")
	}

	// Create VIM account via OSM NBI
	var result VIMAccount
	if err := p.client.post(ctx, "/osm/admin/v1/vim_accounts", vim, &result); err != nil {
		return fmt.Errorf("failed to create VIM account: %w", err)
	}

	// Update ID if it was assigned by OSM
	if result.ID != "" {
		vim.ID = result.ID
	}

	return nil
}

// GetVIMAccount retrieves a VIM account from OSM by ID.
func (p *Plugin) GetVIMAccount(ctx context.Context, id string) (*VIMAccount, error) {
	if id == "" {
		return nil, fmt.Errorf("vim id is required")
	}

	var vim VIMAccount
	if err := p.client.get(ctx, fmt.Sprintf("/osm/admin/v1/vim_accounts/%s", id), &vim); err != nil {
		return nil, fmt.Errorf("failed to get VIM account: %w", err)
	}

	return &vim, nil
}

// ListVIMAccounts retrieves all VIM accounts from OSM.
func (p *Plugin) ListVIMAccounts(ctx context.Context) ([]*VIMAccount, error) {
	var vims []*VIMAccount
	if err := p.client.get(ctx, "/osm/admin/v1/vim_accounts", &vims); err != nil {
		return nil, fmt.Errorf("failed to list VIM accounts: %w", err)
	}

	return vims, nil
}

// UpdateVIMAccount updates an existing VIM account in OSM.
func (p *Plugin) UpdateVIMAccount(ctx context.Context, id string, vim *VIMAccount) error {
	if id == "" {
		return fmt.Errorf("vim id is required")
	}
	if vim == nil {
		return fmt.Errorf("vim cannot be nil")
	}

	// Update via OSM NBI (using PATCH for partial update)
	if err := p.client.patch(ctx, fmt.Sprintf("/osm/admin/v1/vim_accounts/%s", id), vim, nil); err != nil {
		return fmt.Errorf("failed to update VIM account: %w", err)
	}

	return nil
}

// DeleteVIMAccount deletes a VIM account from OSM.
func (p *Plugin) DeleteVIMAccount(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("vim id is required")
	}

	if err := p.client.delete(ctx, fmt.Sprintf("/osm/admin/v1/vim_accounts/%s", id)); err != nil {
		return fmt.Errorf("failed to delete VIM account: %w", err)
	}

	return nil
}

// publishOSMEvent publishes an infrastructure change event to OSM.
// This is an internal method for OSM-specific event publishing.
// OSM doesn't have a native event bus, so this method can be extended to:
//  1. Store events in OSM's operational state
//  2. Trigger OSM workflows based on events
//  3. Forward events to external systems via OSM's notification service
func (p *Plugin) publishOSMEvent(ctx context.Context, event *InfrastructureEvent) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}

	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	if !p.config.EnableEventPublish {
		// Event publishing is disabled
		return nil
	}

	// OSM doesn't have a native event bus API, so we have a few options:
	// 1. Store in OSM operational state (nslcmop collection)
	// 2. Use OSM's notification service
	// 3. Forward to an external event bus
	//
	// For now, we'll use OSM's notification service if available,
	// or log the event for future processing.

	// Event publishing via OSM notification service is planned for future release.
	// Implementation will involve:
	// - Formatting the event for OSM's notification format
	// - Posting to /osm/nslcm/v1/notifications or similar endpoint
	// - Handling delivery confirmation
	// Tracked in: https://github.com/piwi3910/netweave/issues/33

	// Event will be published via OSM notification service in future release
	return nil
}

// TransformVIMAccount transforms netweave infrastructure inventory to OSM VIM account format.
// This helper method converts O2-IMS resource pools to VIM accounts suitable for OSM.
func TransformVIMAccount(pool *ResourcePool, vimType, vimURL, username, password string) *VIMAccount {
	vim := &VIMAccount{
		ID:          pool.ID,
		Name:        pool.Name,
		Description: pool.Description,
		VIMType:     vimType,
		VIMURL:      vimURL,
		VIMUser:     username,
		VIMPassword: password,
		Config:      make(map[string]interface{}),
	}

	// Copy extensions to config
	if pool.Extensions != nil {
		for k, v := range pool.Extensions {
			vim.Config[k] = v
		}
	}

	// Add location information if available
	if pool.Location != "" {
		vim.Config["location"] = pool.Location
	}

	return vim
}
