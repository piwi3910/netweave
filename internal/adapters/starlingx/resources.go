package starlingx

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResources retrieves all resources (StarlingX compute hosts).
func (a *Adapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	personality := a.determinePersonality(ctx, filter)
	hosts, err := a.client.ListHosts(ctx, personality)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	labels, err := a.getLabelsIfNeeded(ctx, filter)
	if err != nil {
		a.logger.Warn("failed to list labels for filtering", zap.Error(err))
	}

	resources := a.convertHostsToResources(ctx, hosts, labels, filter)
	a.logger.Debug("listed resources", zap.Int("count", len(resources)))
	return resources, nil
}

func (a *Adapter) determinePersonality(ctx context.Context, filter *adapter.Filter) string {
	if filter == nil || filter.ResourceTypeID == "" {
		return "compute"
	}

	resourceType, err := a.GetResourceType(ctx, filter.ResourceTypeID)
	if err != nil {
		return "compute"
	}

	if p, ok := resourceType.Extensions["personality"].(string); ok {
		return p
	}
	return "compute"
}

func (a *Adapter) getLabelsIfNeeded(ctx context.Context, filter *adapter.Filter) ([]Label, error) {
	if filter == nil || filter.ResourcePoolID == "" {
		return nil, nil
	}
	return a.client.ListLabels(ctx)
}

func (a *Adapter) convertHostsToResources(ctx context.Context, hosts []IHost, labels []Label, filter *adapter.Filter) []*adapter.Resource {
	resources := make([]*adapter.Resource, 0, len(hosts))
	for i := range hosts {
		resource := a.createResourceFromHost(ctx, &hosts[i], labels, filter)
		if resource != nil {
			resources = append(resources, resource)
		}
	}
	return a.applyResourcePagination(resources, filter)
}

func (a *Adapter) createResourceFromHost(ctx context.Context, host *IHost, labels []Label, filter *adapter.Filter) *adapter.Resource {
	cpus, _ := a.client.GetHostCPUs(ctx, host.UUID)
	memories, _ := a.client.GetHostMemory(ctx, host.UUID)
	disks, _ := a.client.GetHostDisks(ctx, host.UUID)

	resource := MapHostToResource(host, cpus, memories, disks)

	if !a.matchesResourceFilter(resource, host, labels, filter) {
		return nil
	}

	return resource
}

func (a *Adapter) matchesResourceFilter(resource *adapter.Resource, host *IHost, labels []Label, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}

	if filter.TenantID != "" && resource.TenantID != filter.TenantID {
		return false
	}

	if filter.ResourceTypeID != "" && resource.ResourceTypeID != filter.ResourceTypeID {
		return false
	}

	if !a.matchesPoolFilter(resource, host, labels, filter) {
		return false
	}

	return a.matchesLocationFilter(host, filter)
}

func (a *Adapter) matchesPoolFilter(resource *adapter.Resource, host *IHost, labels []Label, filter *adapter.Filter) bool {
	if filter.ResourcePoolID == "" {
		return true
	}

	poolName := ExtractPoolNameForHost(host.UUID, labels)
	expectedPoolID := fmt.Sprintf("starlingx-pool-%s", poolName)
	if expectedPoolID != filter.ResourcePoolID {
		return false
	}
	resource.ResourcePoolID = expectedPoolID
	return true
}

func (a *Adapter) matchesLocationFilter(host *IHost, filter *adapter.Filter) bool {
	if filter.Location == "" {
		return true
	}
	if host.Location == nil {
		return false
	}
	locName, ok := host.Location["name"].(string)
	return ok && locName == filter.Location
}

func (a *Adapter) applyResourcePagination(resources []*adapter.Resource, filter *adapter.Filter) []*adapter.Resource {
	if filter == nil || filter.Limit <= 0 {
		return resources
	}

	start := filter.Offset
	if start >= len(resources) {
		return []*adapter.Resource{}
	}

	end := start + filter.Limit
	if end > len(resources) {
		end = len(resources)
	}

	return resources[start:end]
}

// GetResource retrieves a specific resource by ID (host UUID).
func (a *Adapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	host, err := a.client.GetHost(ctx, id)
	if err != nil {
		return nil, adapter.ErrResourceNotFound
	}

	cpus, _ := a.client.GetHostCPUs(ctx, host.UUID)
	memories, _ := a.client.GetHostMemory(ctx, host.UUID)
	disks, _ := a.client.GetHostDisks(ctx, host.UUID)

	resource := MapHostToResource(host, cpus, memories, disks)

	labels, err := a.client.ListLabels(ctx)
	if err == nil {
		poolName := ExtractPoolNameForHost(host.UUID, labels)
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
	createReq, err := a.buildCreateRequest(ctx, resource)
	if err != nil {
		return nil, err
	}

	host, err := a.client.CreateHost(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	createdResource := a.buildCreatedResource(ctx, host, resource.ResourcePoolID)

	a.logger.Info("created resource",
		zap.String("id", createdResource.ResourceID),
		zap.String("hostname", host.Hostname),
		zap.String("personality", host.Personality),
	)

	return createdResource, nil
}

func (a *Adapter) buildCreateRequest(ctx context.Context, resource *adapter.Resource) (*CreateHostRequest, error) {
	if resource.ResourceTypeID == "" {
		return nil, adapter.ErrResourceTypeRequired
	}

	resourceType, err := a.GetResourceType(ctx, resource.ResourceTypeID)
	if err != nil {
		return nil, fmt.Errorf("invalid resource type: %w", err)
	}

	personality, ok := resourceType.Extensions["personality"].(string)
	if !ok || personality == "" {
		return nil, fmt.Errorf("resource type does not specify personality")
	}

	createReq := &CreateHostRequest{
		Personality: personality,
	}

	a.populateCreateRequestExtensions(createReq, resource.Extensions)
	return createReq, nil
}

func (a *Adapter) populateCreateRequestExtensions(req *CreateHostRequest, extensions map[string]interface{}) {
	if extensions == nil {
		return
	}

	if hostname, ok := extensions["hostname"].(string); ok {
		req.Hostname = hostname
	}
	if location, ok := extensions["location"].(map[string]interface{}); ok {
		req.Location = location
	}
	if mgmtMAC, ok := extensions["mgmt_mac"].(string); ok {
		req.MgmtMAC = mgmtMAC
	}
	if mgmtIP, ok := extensions["mgmt_ip"].(string); ok {
		req.MgmtIP = mgmtIP
	}
}

func (a *Adapter) buildCreatedResource(ctx context.Context, host *IHost, poolID string) *adapter.Resource {
	cpus, _ := a.client.GetHostCPUs(ctx, host.UUID)
	memories, _ := a.client.GetHostMemory(ctx, host.UUID)
	disks, _ := a.client.GetHostDisks(ctx, host.UUID)

	createdResource := MapHostToResource(host, cpus, memories, disks)

	if poolID != "" {
		a.assignHostToPool(ctx, host.UUID, poolID, createdResource)
	}

	return createdResource
}

func (a *Adapter) assignHostToPool(ctx context.Context, hostUUID, poolID string, resource *adapter.Resource) {
	poolName := ExtractPoolNameFromPoolID(poolID)
	if poolName == "" {
		return
	}

	labelReq := &CreateLabelRequest{
		HostUUID:   hostUUID,
		LabelKey:   "pool",
		LabelValue: poolName,
	}

	if _, err := a.client.CreateLabel(ctx, labelReq); err != nil {
		a.logger.Warn("failed to assign host to pool",
			zap.String("host_uuid", hostUUID),
			zap.String("pool", poolName),
			zap.Error(err),
		)
	} else {
		resource.ResourcePoolID = poolID
	}
}

// UpdateResource updates a resource's mutable fields.
func (a *Adapter) UpdateResource(ctx context.Context, id string, resource *adapter.Resource) (*adapter.Resource, error) {
	existing, err := a.GetResource(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := a.updateHostFields(ctx, id, resource, existing); err != nil {
		return nil, err
	}

	a.handlePoolReassignment(ctx, id, resource, existing)

	updatedResource, err := a.GetResource(ctx, id)
	if err != nil {
		return nil, err
	}

	a.logger.Info("updated resource", zap.String("id", id))
	return updatedResource, nil
}

func (a *Adapter) updateHostFields(ctx context.Context, id string, resource, existing *adapter.Resource) error {
	updateReq := &UpdateHostRequest{}
	updated := false

	if resource.Description != "" && resource.Description != existing.Description {
		if updateReq.Capabilities == nil {
			updateReq.Capabilities = make(map[string]interface{})
		}
		updateReq.Capabilities["description"] = resource.Description
		updated = true
	}

	if resource.Extensions != nil {
		updated = a.applyExtensionUpdates(updateReq, resource.Extensions) || updated
	}

	if !updated {
		return nil
	}

	if _, err := a.client.UpdateHost(ctx, id, updateReq); err != nil {
		return fmt.Errorf("failed to update host: %w", err)
	}

	return nil
}

func (a *Adapter) applyExtensionUpdates(req *UpdateHostRequest, extensions map[string]interface{}) bool {
	updated := false

	if hostname, ok := extensions["hostname"].(string); ok {
		req.Hostname = &hostname
		updated = true
	}

	if location, ok := extensions["location"].(map[string]interface{}); ok {
		req.Location = location
		updated = true
	}

	if capabilities, ok := extensions["capabilities"].(map[string]interface{}); ok {
		if req.Capabilities == nil {
			req.Capabilities = make(map[string]interface{})
		}
		for k, v := range capabilities {
			req.Capabilities[k] = v
		}
		updated = true
	}

	return updated
}

func (a *Adapter) handlePoolReassignment(ctx context.Context, id string, resource, existing *adapter.Resource) {
	if resource.ResourcePoolID == "" || resource.ResourcePoolID == existing.ResourcePoolID {
		return
	}

	a.removeOldPoolLabels(ctx, id)

	poolName := ExtractPoolNameFromPoolID(resource.ResourcePoolID)
	if poolName != "" {
		labelReq := &CreateLabelRequest{
			HostUUID:   id,
			LabelKey:   "pool",
			LabelValue: poolName,
		}
		if _, err := a.client.CreateLabel(ctx, labelReq); err != nil {
			a.logger.Warn("failed to create pool label", zap.Error(err))
		}
	}
}

func (a *Adapter) removeOldPoolLabels(ctx context.Context, hostUUID string) {
	labels, err := a.client.ListLabels(ctx)
	if err != nil {
		return
	}

	for _, label := range labels {
		if label.HostUUID == hostUUID && (label.LabelKey == "pool" || label.LabelKey == "resource-pool") {
			if err := a.client.DeleteLabel(ctx, label.UUID); err != nil {
				a.logger.Warn("failed to delete old pool label", zap.Error(err))
			}
		}
	}
}

// DeleteResource deletes a resource (deprovisions a host).
func (a *Adapter) DeleteResource(ctx context.Context, id string) error {
	_, err := a.GetResource(ctx, id)
	if err != nil {
		return err
	}

	if err := a.client.DeleteHost(ctx, id); err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	a.logger.Info("deleted resource", zap.String("id", id))
	return nil
}

// Helper functions

// ExtractPoolNameForHost finds the pool name for a given host UUID.
func ExtractPoolNameForHost(hostUUID string, labels []Label) string {
	for _, label := range labels {
		if label.HostUUID == hostUUID && (label.LabelKey == "pool" || label.LabelKey == "resource-pool") {
			return label.LabelValue
		}
	}
	return "default"
}

// ExtractPoolNameFromPoolID extracts pool name from pool ID.
// Format: starlingx-pool-{name}
func ExtractPoolNameFromPoolID(poolID string) string {
	const prefix = "starlingx-pool-"
	if len(poolID) > len(prefix) {
		return poolID[len(prefix):]
	}
	return ""
}
