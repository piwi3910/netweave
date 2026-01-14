package starlingx

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResources retrieves all resources (StarlingX compute hosts).
func (a *Adapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	// Determine personality filter
	personality := "compute"
	if filter != nil && filter.ResourceTypeID != "" {
		// Extract personality from resource type ID
		// Format: starlingx-{personality}
		if resourceType, err := a.GetResourceType(ctx, filter.ResourceTypeID); err == nil {
			if p, ok := resourceType.Extensions["personality"].(string); ok {
				personality = p
			}
		}
	}

	// Get hosts
	hosts, err := a.client.ListHosts(ctx, personality)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	// Get labels for pool filtering
	var labels []Label
	if filter != nil && filter.ResourcePoolID != "" {
		labels, err = a.client.ListLabels(ctx)
		if err != nil {
			a.logger.Warn("failed to list labels for filtering", zap.Error(err))
		}
	}

	// Convert to resources
	resources := make([]*adapter.Resource, 0, len(hosts))
	for _, host := range hosts {
		// Get hardware inventory
		cpus, _ := a.client.GetHostCPUs(ctx, host.UUID)
		memories, _ := a.client.GetHostMemory(ctx, host.UUID)
		disks, _ := a.client.GetHostDisks(ctx, host.UUID)

		resource := mapHostToResource(&host, cpus, memories, disks)

		// Apply filters
		if filter != nil {
			if filter.TenantID != "" && resource.TenantID != filter.TenantID {
				continue
			}

			if filter.ResourceTypeID != "" && resource.ResourceTypeID != filter.ResourceTypeID {
				continue
			}

			if filter.ResourcePoolID != "" {
				// Check if host belongs to the requested pool
				poolName := extractPoolNameForHost(host.UUID, labels)
				expectedPoolID := fmt.Sprintf("starlingx-pool-%s", poolName)
				if expectedPoolID != filter.ResourcePoolID {
					continue
				}
				resource.ResourcePoolID = expectedPoolID
			}

			if filter.Location != "" {
				if host.Location == nil {
					continue
				}
				if locName, ok := host.Location["name"].(string); !ok || locName != filter.Location {
					continue
				}
			}
		}

		resources = append(resources, resource)
	}

	// Apply pagination
	if filter != nil && filter.Limit > 0 {
		start := filter.Offset
		if start >= len(resources) {
			return []*adapter.Resource{}, nil
		}
		end := start + filter.Limit
		if end > len(resources) {
			end = len(resources)
		}
		resources = resources[start:end]
	}

	a.logger.Debug("listed resources",
		zap.Int("count", len(resources)),
	)

	return resources, nil
}

// GetResource retrieves a specific resource by ID (host UUID).
func (a *Adapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	host, err := a.client.GetHost(ctx, id)
	if err != nil {
		return nil, adapter.ErrResourceNotFound
	}

	// Get hardware inventory
	cpus, _ := a.client.GetHostCPUs(ctx, host.UUID)
	memories, _ := a.client.GetHostMemory(ctx, host.UUID)
	disks, _ := a.client.GetHostDisks(ctx, host.UUID)

	resource := mapHostToResource(host, cpus, memories, disks)

	// Get pool assignment
	labels, err := a.client.ListLabels(ctx)
	if err == nil {
		poolName := extractPoolNameForHost(host.UUID, labels)
		if poolName != "" {
			resource.ResourcePoolID = fmt.Sprintf("starlingx-pool-%s", poolName)
		}
	}

	a.logger.Debug("retrieved resource",
		zap.String("id", id),
		zap.String("hostname", host.Hostname),
	)

	return resource, nil
}

// CreateResource creates a new resource (provisions a new host).
func (a *Adapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	// Validate required fields
	if resource.ResourceTypeID == "" {
		return nil, adapter.ErrResourceTypeRequired
	}

	// Extract personality from resource type
	resourceType, err := a.GetResourceType(ctx, resource.ResourceTypeID)
	if err != nil {
		return nil, fmt.Errorf("invalid resource type: %w", err)
	}

	personality, ok := resourceType.Extensions["personality"].(string)
	if !ok || personality == "" {
		return nil, fmt.Errorf("resource type does not specify personality")
	}

	// Build create request
	createReq := &CreateHostRequest{
		Personality: personality,
	}

	// Extract optional fields from extensions
	if resource.Extensions != nil {
		if hostname, ok := resource.Extensions["hostname"].(string); ok {
			createReq.Hostname = hostname
		}
		if location, ok := resource.Extensions["location"].(map[string]interface{}); ok {
			createReq.Location = location
		}
		if mgmtMAC, ok := resource.Extensions["mgmt_mac"].(string); ok {
			createReq.MgmtMAC = mgmtMAC
		}
		if mgmtIP, ok := resource.Extensions["mgmt_ip"].(string); ok {
			createReq.MgmtIP = mgmtIP
		}
	}

	// Create host in StarlingX
	host, err := a.client.CreateHost(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	// Get hardware inventory
	cpus, _ := a.client.GetHostCPUs(ctx, host.UUID)
	memories, _ := a.client.GetHostMemory(ctx, host.UUID)
	disks, _ := a.client.GetHostDisks(ctx, host.UUID)

	createdResource := mapHostToResource(host, cpus, memories, disks)

	// Assign to pool if specified
	if resource.ResourcePoolID != "" {
		poolName := extractPoolNameFromPoolID(resource.ResourcePoolID)
		if poolName != "" {
			labelReq := &CreateLabelRequest{
				HostUUID:   host.UUID,
				LabelKey:   "pool",
				LabelValue: poolName,
			}
			if _, err := a.client.CreateLabel(ctx, labelReq); err != nil {
				a.logger.Warn("failed to assign host to pool",
					zap.String("host_uuid", host.UUID),
					zap.String("pool", poolName),
					zap.Error(err),
				)
			} else {
				createdResource.ResourcePoolID = resource.ResourcePoolID
			}
		}
	}

	a.logger.Info("created resource",
		zap.String("id", createdResource.ResourceID),
		zap.String("hostname", host.Hostname),
		zap.String("personality", host.Personality),
	)

	return createdResource, nil
}

// UpdateResource updates a resource's mutable fields.
func (a *Adapter) UpdateResource(ctx context.Context, id string, resource *adapter.Resource) (*adapter.Resource, error) {
	// Get existing resource
	existing, err := a.GetResource(ctx, id)
	if err != nil {
		return nil, err
	}

	// Build update request
	updateReq := &UpdateHostRequest{}
	updated := false

	if resource.Description != "" && resource.Description != existing.Description {
		// Description is stored in extensions
		if updateReq.Capabilities == nil {
			updateReq.Capabilities = make(map[string]interface{})
		}
		updateReq.Capabilities["description"] = resource.Description
		updated = true
	}

	if resource.Extensions != nil {
		if hostname, ok := resource.Extensions["hostname"].(string); ok {
			updateReq.Hostname = &hostname
			updated = true
		}
		if location, ok := resource.Extensions["location"].(map[string]interface{}); ok {
			updateReq.Location = location
			updated = true
		}
		if capabilities, ok := resource.Extensions["capabilities"].(map[string]interface{}); ok {
			if updateReq.Capabilities == nil {
				updateReq.Capabilities = make(map[string]interface{})
			}
			for k, v := range capabilities {
				updateReq.Capabilities[k] = v
			}
			updated = true
		}
	}

	if updated {
		_, err := a.client.UpdateHost(ctx, id, updateReq)
		if err != nil {
			return nil, fmt.Errorf("failed to update host: %w", err)
		}
	}

	// Handle pool reassignment
	if resource.ResourcePoolID != "" && resource.ResourcePoolID != existing.ResourcePoolID {
		// Remove old pool labels
		labels, err := a.client.ListLabels(ctx)
		if err == nil {
			for _, label := range labels {
				if label.HostUUID == id && (label.LabelKey == "pool" || label.LabelKey == "resource-pool") {
					a.client.DeleteLabel(ctx, label.UUID)
				}
			}
		}

		// Add new pool label
		poolName := extractPoolNameFromPoolID(resource.ResourcePoolID)
		if poolName != "" {
			labelReq := &CreateLabelRequest{
				HostUUID:   id,
				LabelKey:   "pool",
				LabelValue: poolName,
			}
			a.client.CreateLabel(ctx, labelReq)
		}
	}

	// Get updated resource
	updatedResource, err := a.GetResource(ctx, id)
	if err != nil {
		return nil, err
	}

	a.logger.Info("updated resource",
		zap.String("id", id),
	)

	return updatedResource, nil
}

// DeleteResource deletes a resource (deprovisions a host).
func (a *Adapter) DeleteResource(ctx context.Context, id string) error {
	// Verify resource exists
	_, err := a.GetResource(ctx, id)
	if err != nil {
		return err
	}

	// Delete host
	if err := a.client.DeleteHost(ctx, id); err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	a.logger.Info("deleted resource",
		zap.String("id", id),
	)

	return nil
}

// Helper functions

// extractPoolNameForHost finds the pool name for a given host UUID.
func extractPoolNameForHost(hostUUID string, labels []Label) string {
	for _, label := range labels {
		if label.HostUUID == hostUUID && (label.LabelKey == "pool" || label.LabelKey == "resource-pool") {
			return label.LabelValue
		}
	}
	return "default"
}

// extractPoolNameFromPoolID extracts pool name from pool ID.
// Format: starlingx-pool-{name}
func extractPoolNameFromPoolID(poolID string) string {
	const prefix = "starlingx-pool-"
	if len(poolID) > len(prefix) {
		return poolID[len(prefix):]
	}
	return ""
}
