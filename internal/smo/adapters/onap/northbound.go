package onap

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/smo"
)

// === NORTHBOUND MODE: netweave → ONAP ===
// These methods implement the northbound integration where netweave pushes
// inventory and events to ONAP components (A&AI and DMaaP).

// SyncInfrastructureInventory synchronizes O2-IMS infrastructure inventory to ONAP A&AI.
// It transforms netweave inventory models to ONAP A&AI inventory models and performs
// create-or-update operations for cloud regions, tenants, PNFs, and VNFs.
func (p *Plugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if err := p.validateInventorySync(); err != nil {
		return err
	}

	p.logger.Info("Syncing infrastructure inventory to ONAP A&AI",
		zap.Int("deploymentManagers", len(inventory.DeploymentManagers)),
		zap.Int("resourcePools", len(inventory.ResourcePools)),
		zap.Int("resources", len(inventory.Resources)),
		zap.Int("resourceTypes", len(inventory.ResourceTypes)),
	)

	aaiInventory := p.transformToAAIInventory(inventory)

	if err := p.syncAAIInventory(ctx, aaiInventory); err != nil {
		return err
	}

	p.logger.Info("Successfully synced infrastructure inventory to ONAP A&AI",
		zap.Int("cloudRegions", len(aaiInventory.CloudRegions)),
		zap.Int("tenants", len(aaiInventory.Tenants)),
		zap.Int("pnfs", len(aaiInventory.PNFs)),
		zap.Int("vnfs", len(aaiInventory.VNFs)),
	)

	return nil
}

// validateInventorySync validates prerequisites for inventory synchronization.
func (p *Plugin) validateInventorySync() error {
	if p.Closed {
		return fmt.Errorf("plugin is closed")
	}
	if !p.Config.EnableInventorySync {
		return fmt.Errorf("inventory sync is not enabled")
	}
	if p.aaiClient == nil {
		return fmt.Errorf("A&AI client is not initialized")
	}
	return nil
}

// syncAAIInventory synchronizes all A&AI inventory items.
func (p *Plugin) syncAAIInventory(ctx context.Context, aaiInventory *AAIInventory) error {
	if err := p.syncCloudRegions(ctx, aaiInventory.CloudRegions); err != nil {
		return err
	}
	if err := p.syncTenants(ctx, aaiInventory.Tenants); err != nil {
		return err
	}
	if err := p.syncPNFs(ctx, aaiInventory.PNFs); err != nil {
		return err
	}
	return p.syncVNFs(ctx, aaiInventory.VNFs)
}

// syncCloudRegions synchronizes cloud regions to A&AI.
func (p *Plugin) syncCloudRegions(ctx context.Context, cloudRegions []*CloudRegion) error {
	for _, cloudRegion := range cloudRegions {
		if err := p.aaiClient.CreateOrUpdateCloudRegion(ctx, cloudRegion); err != nil {
			p.logger.Error("Failed to sync cloud region to A&AI",
				zap.String("cloudRegionId", cloudRegion.CloudRegionID),
				zap.Error(err),
			)
			return fmt.Errorf("failed to sync cloud region %s: %w", cloudRegion.CloudRegionID, err)
		}
		p.logger.Debug("Synced cloud region to A&AI", zap.String("cloudRegionId", cloudRegion.CloudRegionID))
	}
	return nil
}

// syncTenants synchronizes tenants to A&AI.
func (p *Plugin) syncTenants(ctx context.Context, tenants []*Tenant) error {
	for _, tenant := range tenants {
		if err := p.aaiClient.CreateOrUpdateTenant(ctx, tenant); err != nil {
			p.logger.Error("Failed to sync tenant to A&AI",
				zap.String("tenantId", tenant.TenantID),
				zap.Error(err),
			)
			return fmt.Errorf("failed to sync tenant %s: %w", tenant.TenantID, err)
		}
		p.logger.Debug("Synced tenant to A&AI", zap.String("tenantId", tenant.TenantID))
	}
	return nil
}

// syncPNFs synchronizes PNFs (physical network functions) to A&AI.
func (p *Plugin) syncPNFs(ctx context.Context, pnfs []*PNF) error {
	for _, pnf := range pnfs {
		if err := p.aaiClient.CreateOrUpdatePNF(ctx, pnf); err != nil {
			p.logger.Error("Failed to sync PNF to A&AI",
				zap.String("pnfName", pnf.PNFName),
				zap.Error(err),
			)
			return fmt.Errorf("failed to sync PNF %s: %w", pnf.PNFName, err)
		}
		p.logger.Debug("Synced PNF to A&AI", zap.String("pnfName", pnf.PNFName))
	}
	return nil
}

// syncVNFs synchronizes VNFs (virtual network functions) to A&AI.
func (p *Plugin) syncVNFs(ctx context.Context, vnfs []*VNF) error {
	for _, vnf := range vnfs {
		if err := p.aaiClient.CreateOrUpdateVNF(ctx, vnf); err != nil {
			p.logger.Error("Failed to sync VNF to A&AI",
				zap.String("vnfId", vnf.VNFID),
				zap.Error(err),
			)
			return fmt.Errorf("failed to sync VNF %s: %w", vnf.VNFID, err)
		}
		p.logger.Debug("Synced VNF to A&AI", zap.String("vnfId", vnf.VNFID))
	}
	return nil
}

// SyncDeploymentInventory synchronizes O2-DMS deployment inventory to ONAP A&AI.
// It transforms netweave deployment models to ONAP service instance models.
func (p *Plugin) SyncDeploymentInventory(ctx context.Context, inventory *smo.DeploymentInventory) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.Closed {
		return fmt.Errorf("plugin is closed")
	}

	if !p.Config.EnableInventorySync {
		return fmt.Errorf("inventory sync is not enabled")
	}

	if p.aaiClient == nil {
		return fmt.Errorf("A&AI client is not initialized")
	}

	p.logger.Info("Syncing deployment inventory to ONAP A&AI",
		zap.Int("packages", len(inventory.Packages)),
		zap.Int("deployments", len(inventory.Deployments)),
	)

	// Transform deployments to ONAP service instances
	for _, deployment := range inventory.Deployments {
		serviceInstance := p.transformDeploymentToServiceInstance(&deployment)

		if err := p.aaiClient.CreateOrUpdateServiceInstance(ctx, serviceInstance); err != nil {
			p.logger.Error("Failed to sync service instance to A&AI",
				zap.String("serviceInstanceId", serviceInstance.ServiceInstanceID),
				zap.Error(err),
			)
			return fmt.Errorf("failed to sync service instance %s: %w", serviceInstance.ServiceInstanceID, err)
		}

		p.logger.Debug("Synced service instance to A&AI",
			zap.String("serviceInstanceId", serviceInstance.ServiceInstanceID),
		)
	}

	p.logger.Info("Successfully synced deployment inventory to ONAP A&AI",
		zap.Int("serviceInstances", len(inventory.Deployments)),
	)

	return nil
}

// PublishInfrastructureEvent publishes an infrastructure change event to ONAP DMaaP.
// Events are published in VES (Virtual Event Streaming) format to the appropriate DMaaP topic.
func (p *Plugin) PublishInfrastructureEvent(ctx context.Context, event *smo.InfrastructureEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.Closed {
		return fmt.Errorf("plugin is closed")
	}

	if !p.Config.EnableEventPublishing {
		return fmt.Errorf("event publishing is not enabled")
	}

	if p.dmaapClient == nil {
		return fmt.Errorf("DMaaP client is not initialized")
	}

	p.logger.Debug("Publishing infrastructure event to DMaaP",
		zap.String("eventId", event.EventID),
		zap.String("eventType", event.EventType),
		zap.String("resourceType", event.ResourceType),
		zap.String("resourceId", event.ResourceID),
	)

	// Transform to VES (Virtual Event Streaming) format
	vesEvent := p.transformToVESEvent(event)

	// Determine the DMaaP topic based on event type
	topic := p.GetDMaaPTopic(event.EventType)

	// Publish to DMaaP
	if err := p.dmaapClient.PublishEvent(ctx, topic, vesEvent); err != nil {
		p.logger.Error("Failed to publish infrastructure event to DMaaP",
			zap.String("eventId", event.EventID),
			zap.String("topic", topic),
			zap.Error(err),
		)
		return fmt.Errorf("failed to publish event %s: %w", event.EventID, err)
	}

	p.logger.Info("Successfully published infrastructure event to DMaaP",
		zap.String("eventId", event.EventID),
		zap.String("topic", topic),
	)

	return nil
}

// PublishDeploymentEvent publishes a deployment change event to ONAP DMaaP.
// Events are published in VES format to the deployment events topic.
func (p *Plugin) PublishDeploymentEvent(ctx context.Context, event *smo.DeploymentEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.Closed {
		return fmt.Errorf("plugin is closed")
	}

	if !p.Config.EnableEventPublishing {
		return fmt.Errorf("event publishing is not enabled")
	}

	if p.dmaapClient == nil {
		return fmt.Errorf("DMaaP client is not initialized")
	}

	p.logger.Debug("Publishing deployment event to DMaaP",
		zap.String("eventId", event.EventID),
		zap.String("eventType", event.EventType),
		zap.String("deploymentId", event.DeploymentID),
	)

	// Transform to VES format
	vesEvent := p.transformDeploymentEventToVES(event)

	// Publish to DMaaP deployment events topic
	topic := "unauthenticated.VES_DEPLOYMENT_EVENTS"

	if err := p.dmaapClient.PublishEvent(ctx, topic, vesEvent); err != nil {
		p.logger.Error("Failed to publish deployment event to DMaaP",
			zap.String("eventId", event.EventID),
			zap.String("topic", topic),
			zap.Error(err),
		)
		return fmt.Errorf("failed to publish event %s: %w", event.EventID, err)
	}

	p.logger.Info("Successfully published deployment event to DMaaP",
		zap.String("eventId", event.EventID),
		zap.String("topic", topic),
	)

	return nil
}

// === Helper methods for inventory transformation ===

// transformToAAIInventory transforms netweave inventory to ONAP A&AI format.
func (p *Plugin) transformToAAIInventory(inventory *smo.InfrastructureInventory) *AAIInventory {
	aaiInventory := &AAIInventory{
		CloudRegions: make([]*CloudRegion, 0),
		Tenants:      make([]*Tenant, 0),
		PNFs:         make([]*PNF, 0),
		VNFs:         make([]*VNF, 0),
	}

	// Transform deployment managers to cloud regions
	for _, dm := range inventory.DeploymentManagers {
		cloudRegion := &CloudRegion{
			CloudOwner:         "netweave",
			CloudRegionID:      dm.OCloudID,
			CloudType:          "openstack", // Default to OpenStack
			OwnerDefinedType:   "o2ims-deployment-manager",
			CloudRegionVersion: "1.0",
			ComplexName:        dm.Name,
			IdentityURL:        dm.ServiceURI,
		}

		// Extract additional metadata from extensions
		if dm.Extensions != nil {
			if cloudType, ok := dm.Extensions["cloudType"].(string); ok {
				cloudRegion.CloudType = cloudType
			}
		}

		aaiInventory.CloudRegions = append(aaiInventory.CloudRegions, cloudRegion)
	}

	// Transform resource pools to tenants
	for _, pool := range inventory.ResourcePools {
		tenant := &Tenant{
			TenantID:      pool.ID,
			TenantName:    pool.Name,
			TenantContext: pool.Description,
			CloudOwner:    "netweave",
			CloudRegionID: pool.OCloudID,
		}

		aaiInventory.Tenants = append(aaiInventory.Tenants, tenant)
	}

	// Transform resources to PNFs or VNFs based on resource kind
	for _, resource := range inventory.Resources {
		// Determine if physical or virtual
		isPhysical := true
		if resource.Extensions != nil {
			if kind, ok := resource.Extensions["resourceKind"].(string); ok {
				isPhysical = (kind == "physical")
			}
		}

		if isPhysical {
			pnf := p.createPNFFromResource(&resource)
			aaiInventory.PNFs = append(aaiInventory.PNFs, pnf)
		} else {
			vnf := p.createVNFFromResource(&resource)
			aaiInventory.VNFs = append(aaiInventory.VNFs, vnf)
		}
	}

	return aaiInventory
}

// transformDeploymentToServiceInstance transforms a netweave deployment to ONAP service instance.
func (p *Plugin) transformDeploymentToServiceInstance(deployment *smo.Deployment) *ServiceInstance {
	return &ServiceInstance{
		ServiceInstanceID:   deployment.ID,
		ServiceInstanceName: deployment.Name,
		ServiceType:         "netweave-deployment",
		ServiceRole:         "o2dms-managed",
		OrchestrationStatus: p.MapDeploymentStatusToOrchestrationStatus(deployment.Status),
		ModelInvariantID:    deployment.PackageID,
		ModelVersionID:      deployment.PackageID, // In practice, this would be version-specific
		SelfLink:            fmt.Sprintf("/o2dms/v1/deployments/%s", deployment.ID),
		CreatedAt:           deployment.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           deployment.UpdatedAt.Format(time.RFC3339),
	}
}

// transformToVESEvent transforms an infrastructure event to VES (Virtual Event Streaming) format.
func (p *Plugin) transformToVESEvent(event *smo.InfrastructureEvent) *VESEvent {
	return &VESEvent{
		Event: VESEventData{
			CommonEventHeader: CommonEventHeader{
				Domain:                  "other",
				EventID:                 event.EventID,
				EventName:               fmt.Sprintf("o2ims_%s", event.EventType),
				EventType:               event.EventType,
				LastEpochMicrosec:       event.Timestamp.UnixMicro(),
				Priority:                "Normal",
				ReportingEntityName:     "netweave-o2ims",
				SourceName:              event.ResourceID,
				StartEpochMicrosec:      event.Timestamp.UnixMicro(),
				Version:                 "4.1",
				VesEventListenerVersion: "7.2",
			},
			OtherFields: OtherFields{
				OtherFieldsVersion: "3.0",
				HashOfNameValuePairArrays: []NameValuePairArray{
					{
						Name: "o2ims-event",
						ArrayOfFields: []NameValuePair{
							{Name: "resourceType", Value: event.ResourceType},
							{Name: "resourceId", Value: event.ResourceID},
							{Name: "eventType", Value: event.EventType},
						},
					},
				},
			},
		},
	}
}

// transformDeploymentEventToVES transforms a deployment event to VES format.
func (p *Plugin) transformDeploymentEventToVES(event *smo.DeploymentEvent) *VESEvent {
	return &VESEvent{
		Event: VESEventData{
			CommonEventHeader: CommonEventHeader{
				Domain:                  "other",
				EventID:                 event.EventID,
				EventName:               fmt.Sprintf("o2dms_%s", event.EventType),
				EventType:               event.EventType,
				LastEpochMicrosec:       event.Timestamp.UnixMicro(),
				Priority:                "Normal",
				ReportingEntityName:     "netweave-o2dms",
				SourceName:              event.DeploymentID,
				StartEpochMicrosec:      event.Timestamp.UnixMicro(),
				Version:                 "4.1",
				VesEventListenerVersion: "7.2",
			},
			OtherFields: OtherFields{
				OtherFieldsVersion: "3.0",
				HashOfNameValuePairArrays: []NameValuePairArray{
					{
						Name: "o2dms-event",
						ArrayOfFields: []NameValuePair{
							{Name: "deploymentId", Value: event.DeploymentID},
							{Name: "eventType", Value: event.EventType},
						},
					},
				},
			},
		},
	}
}

// getDMaaPTopic determines the appropriate DMaaP topic for an event type.
func (p *Plugin) GetDMaaPTopic(eventType string) string {
	// Map event types to DMaaP topics
	topicMap := map[string]string{
		"ResourceCreated":     "unauthenticated.VES_INFRASTRUCTURE_EVENTS",
		"ResourceUpdated":     "unauthenticated.VES_INFRASTRUCTURE_EVENTS",
		"ResourceDeleted":     "unauthenticated.VES_INFRASTRUCTURE_EVENTS",
		"ResourcePoolCreated": "unauthenticated.VES_INFRASTRUCTURE_EVENTS",
		"ResourcePoolUpdated": "unauthenticated.VES_INFRASTRUCTURE_EVENTS",
		"ResourcePoolDeleted": "unauthenticated.VES_INFRASTRUCTURE_EVENTS",
	}

	if topic, ok := topicMap[eventType]; ok {
		return topic
	}

	// Default topic for unknown event types
	return "unauthenticated.VES_INFRASTRUCTURE_EVENTS"
}

// mapDeploymentStatusToOrchestrationStatus maps netweave deployment status to ONAP orchestration status.
func (p *Plugin) MapDeploymentStatusToOrchestrationStatus(status string) string {
	statusMap := map[string]string{
		"pending":   "Assigned",
		"deploying": "Active",
		"deployed":  "Active",
		"running":   "Active",
		"failed":    "Failed",
		"deleting":  "PendingDelete",
		"deleted":   "Deleted",
	}

	if onapStatus, ok := statusMap[status]; ok {
		return onapStatus
	}

	return "Created"
}

// createPNFFromResource creates a PNF (Physical Network Function) from a resource.
func (p *Plugin) createPNFFromResource(resource *smo.Resource) *PNF {
	pnf := &PNF{
		PNFName:  resource.ID,
		PNFName2: resource.Description,
		PNFID:    resource.GlobalAssetID,
		InMaint:  false,
		FrameID:  resource.ResourcePoolID,
	}

	// Extract additional metadata from extensions
	if resource.Extensions == nil {
		return pnf
	}

	if equipType, ok := resource.Extensions["equipmentType"].(string); ok {
		pnf.EquipType = equipType
	}
	if equipVendor, ok := resource.Extensions["equipmentVendor"].(string); ok {
		pnf.EquipVendor = equipVendor
	}
	if equipModel, ok := resource.Extensions["equipmentModel"].(string); ok {
		pnf.EquipModel = equipModel
	}

	return pnf
}

// createVNFFromResource creates a VNF (Virtual Network Function) from a resource.
func (p *Plugin) createVNFFromResource(resource *smo.Resource) *VNF {
	return &VNF{
		VNFID:                resource.ID,
		VNFName:              resource.Description,
		VNFType:              resource.ResourceTypeID,
		InMaint:              false,
		IsClosedLoopDisabled: false,
	}
}
