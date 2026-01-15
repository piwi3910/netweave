// Package mock provides a mock SMO plugin with realistic workflow simulation.
// This plugin is designed for:
// - Local development and testing without real SMO infrastructure (ONAP/OSM)
// - E2E testing in CI pipelines
// - API demonstrations and documentation
//
// The mock plugin stores all data in memory and simulates realistic
// workflow execution, inventory sync, and policy management behavior.
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/smo"
)

// Plugin is a mock implementation of the SMO plugin interface.
// It stores all data in memory and provides realistic simulation of SMO operations.
type Plugin struct {
	mu             sync.RWMutex
	workflows      map[string]*workflowExecution
	serviceModels  map[string]*smo.ServiceModel
	policies       map[string]*policyState
	initialized    bool
	config         map[string]interface{}
}

// workflowExecution tracks the execution state of a workflow.
type workflowExecution struct {
	ID          string
	WorkflowName string
	Status      string
	Progress    int
	Message     string
	StartedAt   time.Time
	CompletedAt *time.Time
	Result      map[string]interface{}
}

// policyState tracks the enforcement state of a policy.
type policyState struct {
	Policy    *smo.Policy
	AppliedAt time.Time
	Enforced  bool
	Violations int
}

// NewPlugin creates a new mock SMO plugin.
func NewPlugin() *Plugin {
	return &Plugin{
		workflows:     make(map[string]*workflowExecution),
		serviceModels: make(map[string]*smo.ServiceModel),
		policies:      make(map[string]*policyState),
		initialized:   false,
	}
}

// PluginCore implementation

// Metadata returns the plugin's identifying information.
func (p *Plugin) Metadata() smo.PluginMetadata {
	return smo.PluginMetadata{
		Name:        "mock",
		Version:     "1.0.0",
		Description: "Mock SMO plugin for development and testing",
		Vendor:      "Mock Corp",
	}
}

// Capabilities returns the list of features this plugin supports.
func (p *Plugin) Capabilities() []smo.Capability {
	return []smo.Capability{
		smo.CapInventorySync,
		smo.CapEventPublishing,
		smo.CapWorkflowOrchestration,
		smo.CapServiceModeling,
		smo.CapPolicyManagement,
	}
}

// Initialize initializes the plugin with the provided configuration.
func (p *Plugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = config
	p.initialized = true

	// Populate with sample service models
	p.populateSampleServiceModels()

	return nil
}

// Health checks the health status of all SMO component connections.
func (p *Plugin) Health(ctx context.Context) smo.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return smo.HealthStatus{
			Healthy:   false,
			Message:   "Plugin not initialized",
			Details:   make(map[string]smo.ComponentHealth),
			Timestamp: time.Now(),
		}
	}

	return smo.HealthStatus{
		Healthy: true,
		Message: "All mock components healthy",
		Details: map[string]smo.ComponentHealth{
			"workflow-engine": {
				Name:         "workflow-engine",
				Healthy:      true,
				Message:      "Mock workflow engine operational",
				ResponseTime: 5 * time.Millisecond,
			},
			"inventory-sync": {
				Name:         "inventory-sync",
				Healthy:      true,
				Message:      "Mock inventory sync operational",
				ResponseTime: 3 * time.Millisecond,
			},
			"policy-engine": {
				Name:         "policy-engine",
				Healthy:      true,
				Message:      "Mock policy engine operational",
				ResponseTime: 4 * time.Millisecond,
			},
		},
		Timestamp: time.Now(),
	}
}

// Close cleanly shuts down the plugin and releases all resources.
func (p *Plugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.initialized = false
	p.workflows = make(map[string]*workflowExecution)
	p.serviceModels = make(map[string]*smo.ServiceModel)
	p.policies = make(map[string]*policyState)

	return nil
}

// PluginNorthboundSync implementation

// SyncInfrastructureInventory synchronizes O2-IMS infrastructure inventory to the SMO.
func (p *Plugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	// Mock implementation - just validate the inventory
	if len(inventory.DeploymentManagers) == 0 {
		return fmt.Errorf("no deployment managers in inventory")
	}

	// In a real implementation, this would push to the SMO
	return nil
}

// SyncDeploymentInventory synchronizes O2-DMS deployment inventory to the SMO.
func (p *Plugin) SyncDeploymentInventory(ctx context.Context, inventory *smo.DeploymentInventory) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	// Mock implementation - just validate the inventory
	// In a real implementation, this would push to the SMO
	return nil
}

// PluginNorthboundEvents implementation

// PublishInfrastructureEvent publishes an infrastructure change event to the SMO.
func (p *Plugin) PublishInfrastructureEvent(ctx context.Context, event *smo.InfrastructureEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	// Mock implementation - events are logged but not actually sent
	// In a real implementation, this would publish to SMO event bus (DMaaP, Kafka, etc.)
	return nil
}

// PublishDeploymentEvent publishes a deployment change event to the SMO.
func (p *Plugin) PublishDeploymentEvent(ctx context.Context, event *smo.DeploymentEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	// Mock implementation - events are logged but not actually sent
	// In a real implementation, this would publish to SMO event bus
	return nil
}

// PluginSouthboundWorkflow implementation

// ExecuteWorkflow executes a workflow orchestrated by the SMO.
func (p *Plugin) ExecuteWorkflow(ctx context.Context, workflow *smo.WorkflowRequest) (*smo.WorkflowExecution, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return nil, fmt.Errorf("plugin not initialized")
	}

	executionID := uuid.New().String()
	now := time.Now()

	exec := &workflowExecution{
		ID:           executionID,
		WorkflowName: workflow.WorkflowName,
		Status:       "pending",
		Progress:     0,
		Message:      "Workflow execution started",
		StartedAt:    now,
		CompletedAt:  nil,
		Result:       make(map[string]interface{}),
	}

	p.workflows[executionID] = exec

	// Simulate async workflow execution
	go p.simulateWorkflow(executionID)

	return &smo.WorkflowExecution{
		ExecutionID:  executionID,
		WorkflowName: workflow.WorkflowName,
		Status:       "pending",
		StartedAt:    now,
		Extensions:   workflow.Parameters,
	}, nil
}

// GetWorkflowStatus retrieves the current status of a workflow execution.
func (p *Plugin) GetWorkflowStatus(ctx context.Context, executionID string) (*smo.WorkflowStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	exec, ok := p.workflows[executionID]
	if !ok {
		return nil, fmt.Errorf("workflow execution not found: %s", executionID)
	}

	status := &smo.WorkflowStatus{
		ExecutionID:  exec.ID,
		WorkflowName: exec.WorkflowName,
		Status:       exec.Status,
		Progress:     exec.Progress,
		Message:      exec.Message,
		StartedAt:    exec.StartedAt,
		CompletedAt:  exec.CompletedAt,
		Result:       exec.Result,
	}

	return status, nil
}

// CancelWorkflow cancels a running workflow execution.
func (p *Plugin) CancelWorkflow(ctx context.Context, executionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	exec, ok := p.workflows[executionID]
	if !ok {
		return fmt.Errorf("workflow execution not found: %s", executionID)
	}

	if exec.Status == "completed" || exec.Status == "failed" || exec.Status == "cancelled" {
		return fmt.Errorf("workflow already finished: %s", exec.Status)
	}

	now := time.Now()
	exec.Status = "cancelled"
	exec.CompletedAt = &now
	exec.Message = "Workflow execution cancelled"

	return nil
}

// PluginSouthboundServiceModel implementation

// RegisterServiceModel registers a service model with the SMO.
func (p *Plugin) RegisterServiceModel(ctx context.Context, model *smo.ServiceModel) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	p.serviceModels[model.ID] = model
	return nil
}

// GetServiceModel retrieves a registered service model by ID.
func (p *Plugin) GetServiceModel(ctx context.Context, id string) (*smo.ServiceModel, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	model, ok := p.serviceModels[id]
	if !ok {
		return nil, fmt.Errorf("service model not found: %s", id)
	}

	return model, nil
}

// ListServiceModels retrieves all registered service models.
func (p *Plugin) ListServiceModels(ctx context.Context) ([]*smo.ServiceModel, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	models := make([]*smo.ServiceModel, 0, len(p.serviceModels))
	for _, model := range p.serviceModels {
		models = append(models, model)
	}

	return models, nil
}

// PluginSouthboundPolicy implementation

// ApplyPolicy applies a policy to the infrastructure or deployments.
func (p *Plugin) ApplyPolicy(ctx context.Context, policy *smo.Policy) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return fmt.Errorf("plugin not initialized")
	}

	p.policies[policy.PolicyID] = &policyState{
		Policy:     policy,
		AppliedAt:  time.Now(),
		Enforced:   true,
		Violations: 0,
	}

	return nil
}

// GetPolicyStatus retrieves the current status of an applied policy.
func (p *Plugin) GetPolicyStatus(ctx context.Context, policyID string) (*smo.PolicyStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	state, ok := p.policies[policyID]
	if !ok {
		return nil, fmt.Errorf("policy not found: %s", policyID)
	}

	status := &smo.PolicyStatus{
		PolicyID:         policyID,
		Status:           "active",
		EnforcementCount: 10,
		ViolationCount:   state.Violations,
		LastEnforced:     &state.AppliedAt,
		LastViolation:    nil,
		Message:          "Policy is being enforced",
	}

	return status, nil
}

// Helper methods

func (p *Plugin) populateSampleServiceModels() {
	// Sample service models for common O-RAN services
	models := []*smo.ServiceModel{
		{
			ID:          "sm-5g-ran-001",
			Name:        "5G RAN Service",
			Version:     "1.0.0",
			Description: "5G RAN network service with CU-UP, CU-CP, and DU components",
			Category:    "ran",
			Template:    map[string]interface{}{"type": "5g-ran", "components": []string{"cu-up", "cu-cp", "du"}},
			Extensions: map[string]interface{}{
				"cells":       4,
				"bandwidth":   "100MHz",
				"plmn":        "00101",
				"cu_cp_count": 1,
				"cu_up_count": 1,
				"du_count":    3,
			},
		},
		{
			ID:          "sm-5g-core-001",
			Name:        "5G Core Service",
			Version:     "1.0.0",
			Description: "5G Core network service with AMF, SMF, UPF components",
			Category:    "core",
			Template:    map[string]interface{}{"type": "5g-core", "components": []string{"amf", "smf", "upf"}},
			Extensions: map[string]interface{}{
				"amf_count":   2,
				"smf_count":   2,
				"upf_count":   4,
				"nrf_enabled": true,
			},
		},
		{
			ID:          "sm-mec-001",
			Name:        "MEC Application Service",
			Version:     "1.0.0",
			Description: "Multi-access Edge Computing application service",
			Category:    "edge",
			Template:    map[string]interface{}{"type": "mec", "components": []string{"apps", "platform"}},
			Extensions: map[string]interface{}{
				"edge_sites": 3,
				"app_count":  5,
			},
		},
	}

	for _, model := range models {
		p.serviceModels[model.ID] = model
	}
}

func (p *Plugin) simulateWorkflow(executionID string) {
	// Simulate workflow progression
	stages := []struct {
		delay    time.Duration
		status   string
		progress int
		message  string
	}{
		{1 * time.Second, "running", 25, "Planning workflow steps"},
		{1 * time.Second, "running", 50, "Executing deployment"},
		{1 * time.Second, "running", 75, "Validating deployment"},
		{1 * time.Second, "completed", 100, "Workflow completed successfully"},
	}

	for _, stage := range stages {
		time.Sleep(stage.delay)

		p.mu.Lock()
		if exec, ok := p.workflows[executionID]; ok {
			if exec.Status == "cancelled" {
				p.mu.Unlock()
				return
			}

			exec.Status = stage.status
			exec.Progress = stage.progress
			exec.Message = stage.message

			if stage.status == "completed" {
				now := time.Now()
				exec.CompletedAt = &now
				exec.Result = map[string]interface{}{
					"success":       true,
					"deploymentId":  fmt.Sprintf("dep-%s", uuid.New().String()[:8]),
					"resourceCount": 5,
				}
			}
		}
		p.mu.Unlock()
	}
}
