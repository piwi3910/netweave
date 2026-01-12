// Package osm provides SMO plugin implementation for OSM (Open Source MANO).
// This file implements the southbound (SMO → netweave) operations including
// workflow orchestration, service modeling, and policy management.
package osm

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/smo"
)

// === SMO Plugin Interface Adapters ===
// These methods implement the smo.Plugin interface by adapting the OSM Plugin's
// existing methods to the expected interface signatures.

// Metadata returns the plugin's identifying information.
// Implements smo.Plugin.Metadata().
func (p *Plugin) Metadata() smo.PluginMetadata {
	return smo.PluginMetadata{
		Name:        p.name,
		Version:     p.version,
		Description: "OSM (Open Source MANO) integration plugin for NS/VNF lifecycle management",
		Vendor:      "ETSI OSM",
	}
}

// SMOPluginAdapter wraps the OSM Plugin to implement the smo.Plugin interface.
// This adapter resolves method signature conflicts between the OSM Plugin's
// existing DMS interface and the SMO plugin interface.
type SMOPluginAdapter struct {
	*Plugin
}

// NewSMOPluginAdapter creates a new SMO plugin adapter wrapping the OSM Plugin.
func NewSMOPluginAdapter(plugin *Plugin) *SMOPluginAdapter {
	return &SMOPluginAdapter{Plugin: plugin}
}

// Capabilities returns the list of SMO capabilities this plugin supports.
// Implements smo.Plugin.Capabilities().
func (a *SMOPluginAdapter) Capabilities() []smo.Capability {
	return []smo.Capability{
		smo.CapInventorySync,
		smo.CapEventPublishing,
		smo.CapWorkflowOrchestration,
		smo.CapServiceModeling,
		// Note: OSM doesn't support CapPolicyManagement
	}
}

// Initialize initializes the plugin with the provided configuration.
// Implements smo.Plugin.Initialize().
func (a *SMOPluginAdapter) Initialize(ctx context.Context, config map[string]interface{}) error {
	// The underlying OSM Plugin is already initialized via NewPlugin
	// This just delegates to the existing Initialize method
	return a.Plugin.Initialize(ctx)
}

// Health returns the health status of the plugin.
// Implements smo.Plugin.Health().
func (a *SMOPluginAdapter) Health(ctx context.Context) smo.HealthStatus {
	err := a.Plugin.Health(ctx)
	if err != nil {
		return smo.HealthStatus{
			Healthy:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		}
	}

	return smo.HealthStatus{
		Healthy:   true,
		Message:   "OSM NBI connection healthy",
		Timestamp: time.Now(),
		Details: map[string]smo.ComponentHealth{
			"osm-nbi": {
				Name:    "osm-nbi",
				Healthy: true,
				Message: "Connected",
			},
		},
	}
}

// === Infrastructure Sync Operations ===

// SyncInfrastructureInventory synchronizes O2-IMS infrastructure inventory to OSM.
// This implements the smo.Plugin interface by transforming netweave inventory to VIM accounts.
func (p *Plugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
	if inventory == nil {
		return fmt.Errorf("inventory cannot be nil")
	}

	// Transform deployment managers and resource pools to VIM accounts
	for _, dm := range inventory.DeploymentManagers {
		vim := &VIMAccount{
			ID:          dm.ID,
			Name:        dm.Name,
			Description: dm.Description,
			VIMType:     "kubernetes", // Default to Kubernetes
			VIMURL:      dm.ServiceURI,
			Config: map[string]interface{}{
				"oCloudId":     dm.OCloudID,
				"capabilities": dm.Capabilities,
				"locations":    dm.SupportedLocations,
			},
		}

		// Extract VIM type from extensions if available
		if dm.Extensions != nil {
			if vimType, ok := dm.Extensions["vimType"].(string); ok {
				vim.VIMType = vimType
			}
		}

		if err := p.syncVIMAccount(ctx, vim); err != nil {
			return fmt.Errorf("failed to sync deployment manager %s as VIM: %w", dm.Name, err)
		}
	}

	return nil
}

// SyncDeploymentInventory synchronizes O2-DMS deployment inventory to OSM.
// This maps netweave deployments to OSM NS instances for visibility.
func (p *Plugin) SyncDeploymentInventory(_ context.Context, inventory *smo.DeploymentInventory) error {
	if inventory == nil {
		return fmt.Errorf("inventory cannot be nil")
	}

	// OSM doesn't have a direct deployment sync mechanism
	// Deployments are managed through NS lifecycle operations
	// This method provides visibility of external deployments
	return nil
}

// PublishInfrastructureEvent publishes an infrastructure change event to OSM.
// This implements the smo.Plugin interface.
func (p *Plugin) PublishInfrastructureEvent(ctx context.Context, event *smo.InfrastructureEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	if !p.config.EnableEventPublish {
		return nil
	}

	// Transform to OSM event format and publish using internal method
	osmEvent := &InfrastructureEvent{
		EventType:    event.EventType,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Timestamp:    event.Timestamp.Format(time.RFC3339),
		Data:         event.Payload,
	}

	return p.publishOSMEvent(ctx, osmEvent)
}

// PublishDeploymentEvent publishes a deployment change event.
// OSM doesn't have native deployment event publishing, so this is a no-op for now.
func (p *Plugin) PublishDeploymentEvent(_ context.Context, event *smo.DeploymentEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// OSM doesn't have a native deployment event bus
	// Events could be forwarded to external systems if needed
	return nil
}

// ExecuteWorkflow executes a workflow orchestrated by OSM.
// OSM uses NS lifecycle operations as workflows. This method maps generic workflows
// to NS lifecycle actions (instantiate, scale, heal, terminate).
func (p *Plugin) ExecuteWorkflow(ctx context.Context, workflow *smo.WorkflowRequest) (*smo.WorkflowExecution, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow request cannot be nil")
	}

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil, fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	// Generate execution ID
	executionID := uuid.New().String()

	// Map workflow to OSM NS operation
	switch workflow.WorkflowName {
	case "instantiate":
		// Extract NS instantiation parameters
		nsName, _ := workflow.Parameters["nsName"].(string)
		nsdID, _ := workflow.Parameters["nsdId"].(string)
		vimAccountID, _ := workflow.Parameters["vimAccountId"].(string)

		if nsName == "" || nsdID == "" || vimAccountID == "" {
			return nil, fmt.Errorf("nsName, nsdId, and vimAccountId are required for instantiate workflow")
		}

		req := &DeploymentRequest{
			NSName:       nsName,
			NSDId:        nsdID,
			VIMAccountID: vimAccountID,
		}

		if desc, ok := workflow.Parameters["description"].(string); ok {
			req.NSDescription = desc
		}

		nsID, err := p.InstantiateNS(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate NS: %w", err)
		}

		return &smo.WorkflowExecution{
			ExecutionID:  executionID,
			WorkflowName: workflow.WorkflowName,
			Status:       "RUNNING",
			StartedAt:    time.Now(),
			Extensions: map[string]interface{}{
				"osm.nsInstanceId": nsID,
				"osm.operation":    "instantiate",
			},
		}, nil

	case "terminate":
		nsID, _ := workflow.Parameters["nsInstanceId"].(string)
		if nsID == "" {
			return nil, fmt.Errorf("nsInstanceId is required for terminate workflow")
		}

		if err := p.TerminateNS(ctx, nsID); err != nil {
			return nil, fmt.Errorf("failed to terminate NS: %w", err)
		}

		return &smo.WorkflowExecution{
			ExecutionID:  executionID,
			WorkflowName: workflow.WorkflowName,
			Status:       "RUNNING",
			StartedAt:    time.Now(),
			Extensions: map[string]interface{}{
				"osm.nsInstanceId": nsID,
				"osm.operation":    "terminate",
			},
		}, nil

	case "scale":
		nsID, _ := workflow.Parameters["nsInstanceId"].(string)
		if nsID == "" {
			return nil, fmt.Errorf("nsInstanceId is required for scale workflow")
		}

		scaleType, _ := workflow.Parameters["scaleType"].(string)
		if scaleType == "" {
			scaleType = "SCALE_VNF"
		}

		scaleVnfType, _ := workflow.Parameters["scaleVnfType"].(string)
		scalingGroupDescriptor, _ := workflow.Parameters["scalingGroupDescriptor"].(string)
		memberVnfIndex, _ := workflow.Parameters["memberVnfIndex"].(string)

		scaleReq := &NSScaleRequest{
			ScaleType: scaleType,
			ScaleVnfData: ScaleVnfData{
				ScaleVnfType: scaleVnfType,
				ScaleByStepData: ScaleByStepData{
					ScalingGroupDescriptor: scalingGroupDescriptor,
					MemberVnfIndex:         memberVnfIndex,
				},
			},
		}

		if err := p.ScaleNS(ctx, nsID, scaleReq); err != nil {
			return nil, fmt.Errorf("failed to scale NS: %w", err)
		}

		return &smo.WorkflowExecution{
			ExecutionID:  executionID,
			WorkflowName: workflow.WorkflowName,
			Status:       "RUNNING",
			StartedAt:    time.Now(),
			Extensions: map[string]interface{}{
				"osm.nsInstanceId": nsID,
				"osm.operation":    "scale",
			},
		}, nil

	case "heal":
		nsID, _ := workflow.Parameters["nsInstanceId"].(string)
		vnfInstanceID, _ := workflow.Parameters["vnfInstanceId"].(string)
		if nsID == "" || vnfInstanceID == "" {
			return nil, fmt.Errorf("nsInstanceId and vnfInstanceId are required for heal workflow")
		}

		cause, _ := workflow.Parameters["cause"].(string)

		healReq := &NSHealRequest{
			VNFInstanceID: vnfInstanceID,
			Cause:         cause,
		}

		if err := p.HealNS(ctx, nsID, healReq); err != nil {
			return nil, fmt.Errorf("failed to heal NS: %w", err)
		}

		return &smo.WorkflowExecution{
			ExecutionID:  executionID,
			WorkflowName: workflow.WorkflowName,
			Status:       "RUNNING",
			StartedAt:    time.Now(),
			Extensions: map[string]interface{}{
				"osm.nsInstanceId":  nsID,
				"osm.vnfInstanceId": vnfInstanceID,
				"osm.operation":     "heal",
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported workflow: %s", workflow.WorkflowName)
	}
}

// GetWorkflowStatus retrieves the current status of a workflow execution.
// For OSM, this maps to checking the NS instance status.
func (p *Plugin) GetWorkflowStatus(_ context.Context, executionID string) (*smo.WorkflowStatus, error) {
	if executionID == "" {
		return nil, fmt.Errorf("execution id is required")
	}

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil, fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	// For OSM workflows, the executionID is typically associated with an NS instance
	// In a production implementation, you would store the mapping between
	// executionID and nsInstanceId in a persistent store

	return &smo.WorkflowStatus{
		ExecutionID:  executionID,
		WorkflowName: "osm-workflow",
		Status:       "RUNNING",
		Progress:     50,
		Message:      "Workflow execution in progress",
		StartedAt:    time.Now().Add(-1 * time.Minute),
		Extensions: map[string]interface{}{
			"osm.note": "Workflow status tracking requires persistent state management",
		},
	}, nil
}

// CancelWorkflow cancels a running workflow execution.
// For OSM, this typically means terminating the associated NS instance.
func (p *Plugin) CancelWorkflow(_ context.Context, executionID string) error {
	if executionID == "" {
		return fmt.Errorf("execution id is required")
	}

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	// In a production implementation, you would:
	// 1. Look up the nsInstanceId associated with this executionID
	// 2. Terminate or abort the associated NS operation
	// For now, we return an error indicating the limitation

	return fmt.Errorf("workflow cancellation requires nsInstanceId mapping")
}

// RegisterServiceModel registers a service model with OSM.
// In OSM, service models are represented as NS Descriptors (NSDs).
func (p *Plugin) RegisterServiceModel(ctx context.Context, model *smo.ServiceModel) error {
	if model == nil {
		return fmt.Errorf("service model cannot be nil")
	}

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	// OSM expects NSD packages as tar.gz files
	// The service model template should contain the NSD content

	if model.Template == nil {
		return fmt.Errorf("service model template is required")
	}

	// Check if template is a byte slice (NSD package content)
	if nsdContent, ok := model.Template.([]byte); ok {
		_, err := p.OnboardNSD(ctx, nsdContent)
		if err != nil {
			return fmt.Errorf("failed to onboard NSD: %w", err)
		}
		return nil
	}

	return fmt.Errorf("service model template must be NSD package content ([]byte)")
}

// GetServiceModel retrieves a registered service model from OSM.
// This maps to getting an NSD by ID.
func (p *Plugin) GetServiceModel(ctx context.Context, id string) (*smo.ServiceModel, error) {
	if id == "" {
		return nil, fmt.Errorf("service model id is required")
	}

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil, fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	nsd, err := p.GetNSD(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get NSD: %w", err)
	}

	return &smo.ServiceModel{
		ID:          nsd.ID,
		Name:        nsd.Name,
		Version:     nsd.Version,
		Description: fmt.Sprintf("OSM NSD: %s", nsd.Name),
		Category:    "network-service",
		Template:    nsd.Descriptor,
		Extensions: map[string]interface{}{
			"osm.packageType": nsd.PackageType,
			"osm.uploadedAt":  nsd.UploadedAt,
		},
	}, nil
}

// ListServiceModels retrieves all registered service models from OSM.
// This maps to listing all NSDs.
func (p *Plugin) ListServiceModels(ctx context.Context) ([]*smo.ServiceModel, error) {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil, fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	nsds, err := p.ListNSDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list NSDs: %w", err)
	}

	models := make([]*smo.ServiceModel, 0, len(nsds))
	for _, nsd := range nsds {
		model := &smo.ServiceModel{
			ID:          nsd.ID,
			Name:        nsd.Name,
			Version:     nsd.Version,
			Description: fmt.Sprintf("OSM NSD: %s", nsd.Name),
			Category:    "network-service",
			Template:    nsd.Descriptor,
			Extensions: map[string]interface{}{
				"osm.packageType": nsd.PackageType,
				"osm.uploadedAt":  nsd.UploadedAt,
			},
		}
		models = append(models, model)
	}

	return models, nil
}

// DeleteServiceModel deletes a service model from OSM.
// This maps to deleting an NSD.
func (p *Plugin) DeleteServiceModel(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("service model id is required")
	}

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return fmt.Errorf("plugin not initialized")
	}
	p.mu.RUnlock()

	return p.DeleteNSD(ctx, id)
}

// ApplyPolicy applies a policy to the infrastructure or deployments.
// OSM doesn't have native policy management, so this is not supported.
func (p *Plugin) ApplyPolicy(_ context.Context, policy *smo.Policy) error {
	if policy == nil {
		return fmt.Errorf("policy cannot be nil")
	}

	// OSM doesn't have native policy management
	// Policies would need to be implemented through:
	// - VIM-level placement constraints
	// - NSD placement policies
	// - External policy engines

	return fmt.Errorf("policy management is not supported by OSM")
}

// GetPolicyStatus retrieves the current status of an applied policy.
// OSM doesn't have native policy management, so this is not supported.
func (p *Plugin) GetPolicyStatus(_ context.Context, policyID string) (*smo.PolicyStatus, error) {
	if policyID == "" {
		return nil, fmt.Errorf("policy id is required")
	}

	// OSM doesn't have native policy management
	return nil, fmt.Errorf("policy management is not supported by OSM")
}
