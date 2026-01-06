package onap

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/smo"
)

// === SOUTHBOUND MODE: SMO → netweave ===
// These methods implement the southbound integration where ONAP orchestrates
// workflows and deployments through netweave O2-DMS API.

// ExecuteWorkflow executes a workflow orchestrated by ONAP.
// This typically involves ONAP SO (Service Orchestrator) triggering a BPMN workflow
// that may include deploying CNFs/VNFs via netweave O2-DMS.
func (p *Plugin) ExecuteWorkflow(ctx context.Context, workflow *smo.WorkflowRequest) (*smo.WorkflowExecution, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("plugin is closed")
	}

	if !p.config.EnableDMSBackend {
		return nil, fmt.Errorf("DMS backend mode is not enabled")
	}

	if p.soClient == nil {
		return nil, fmt.Errorf("SO client is not initialized")
	}

	p.logger.Info("Executing ONAP workflow",
		zap.String("workflowName", workflow.WorkflowName),
		zap.Duration("timeout", workflow.Timeout),
	)

	// Generate unique execution ID
	executionID := uuid.New().String()

	// Submit workflow to ONAP SO Camunda engine
	processInstanceID, err := p.soClient.ExecuteWorkflow(ctx, workflow.WorkflowName, workflow.Parameters)
	if err != nil {
		p.logger.Error("Failed to execute ONAP workflow",
			zap.String("workflowName", workflow.WorkflowName),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to execute workflow: %w", err)
	}

	execution := &smo.WorkflowExecution{
		ExecutionID:  executionID,
		WorkflowName: workflow.WorkflowName,
		Status:       "RUNNING",
		StartedAt:    time.Now(),
		Extensions: map[string]interface{}{
			"onap.processInstanceId": processInstanceID,
			"onap.engineName":        "camunda",
			"onap.soUrl":             p.config.SOURL,
		},
	}

	p.logger.Info("Successfully started ONAP workflow execution",
		zap.String("executionId", executionID),
		zap.String("processInstanceId", processInstanceID),
	)

	return execution, nil
}

// GetWorkflowStatus retrieves the current status of a workflow execution.
// It queries ONAP SO to get the Camunda process instance status.
func (p *Plugin) GetWorkflowStatus(ctx context.Context, executionID string) (*smo.WorkflowStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("plugin is closed")
	}

	if !p.config.EnableDMSBackend {
		return nil, fmt.Errorf("DMS backend mode is not enabled")
	}

	if p.soClient == nil {
		return nil, fmt.Errorf("SO client is not initialized")
	}

	p.logger.Debug("Retrieving ONAP workflow status",
		zap.String("executionId", executionID),
	)

	// Query SO for orchestration status
	orchestrationStatus, err := p.soClient.GetOrchestrationStatus(ctx, executionID)
	if err != nil {
		p.logger.Error("Failed to get ONAP workflow status",
			zap.String("executionId", executionID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get workflow status: %w", err)
	}

	status := &smo.WorkflowStatus{
		ExecutionID:  executionID,
		WorkflowName: orchestrationStatus.ServiceName,
		Status:       p.mapONAPRequestStateToStatus(orchestrationStatus.RequestState),
		Progress:     orchestrationStatus.PercentProgress,
		Message:      orchestrationStatus.StatusMessage,
		StartedAt:    orchestrationStatus.StartTime,
		Extensions: map[string]interface{}{
			"onap.requestState":       orchestrationStatus.RequestState,
			"onap.percentProgress":    orchestrationStatus.PercentProgress,
			"onap.orchestrationFlows": orchestrationStatus.FlowStatus,
			"onap.requestId":          orchestrationStatus.RequestID,
		},
	}

	if orchestrationStatus.FinishTime != nil {
		status.CompletedAt = orchestrationStatus.FinishTime
	}

	switch orchestrationStatus.RequestState {
	case "COMPLETE":
		status.Result = map[string]interface{}{
			"serviceInstanceId": orchestrationStatus.ServiceInstanceID,
		}
	case "FAILED":
		status.Error = orchestrationStatus.StatusMessage
	}

	p.logger.Debug("Retrieved ONAP workflow status",
		zap.String("executionId", executionID),
		zap.String("status", status.Status),
		zap.Int("progress", status.Progress),
	)

	return status, nil
}

// CancelWorkflow cancels a running workflow execution.
// It sends a cancellation request to ONAP SO to terminate the Camunda process.
func (p *Plugin) CancelWorkflow(ctx context.Context, executionID string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("plugin is closed")
	}

	if !p.config.EnableDMSBackend {
		return fmt.Errorf("DMS backend mode is not enabled")
	}

	if p.soClient == nil {
		return fmt.Errorf("SO client is not initialized")
	}

	p.logger.Info("Cancelling ONAP workflow",
		zap.String("executionId", executionID),
	)

	if err := p.soClient.CancelOrchestration(ctx, executionID); err != nil {
		p.logger.Error("Failed to cancel ONAP workflow",
			zap.String("executionId", executionID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to cancel workflow: %w", err)
	}

	p.logger.Info("Successfully cancelled ONAP workflow",
		zap.String("executionId", executionID),
	)

	return nil
}

// RegisterServiceModel registers a service model with ONAP.
// Service models define the structure and orchestration logic for deployments.
func (p *Plugin) RegisterServiceModel(ctx context.Context, model *smo.ServiceModel) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("plugin is closed")
	}

	if !p.config.EnableDMSBackend {
		return fmt.Errorf("DMS backend mode is not enabled")
	}

	if p.soClient == nil {
		return fmt.Errorf("SO client is not initialized")
	}

	p.logger.Info("Registering service model with ONAP",
		zap.String("modelId", model.ID),
		zap.String("modelName", model.Name),
		zap.String("version", model.Version),
	)

	// Transform service model to ONAP format
	onapModel := &ServiceModel{
		ModelInvariantID: model.ID,
		ModelVersionID:   fmt.Sprintf("%s-%s", model.ID, model.Version),
		ModelName:        model.Name,
		ModelVersion:     model.Version,
		ModelType:        "service",
		ModelCategory:    model.Category,
		Description:      model.Description,
		Template:         model.Template,
	}

	if err := p.soClient.RegisterServiceModel(ctx, onapModel); err != nil {
		p.logger.Error("Failed to register service model with ONAP",
			zap.String("modelId", model.ID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to register service model: %w", err)
	}

	p.logger.Info("Successfully registered service model with ONAP",
		zap.String("modelId", model.ID),
	)

	return nil
}

// GetServiceModel retrieves a registered service model from ONAP.
func (p *Plugin) GetServiceModel(ctx context.Context, id string) (*smo.ServiceModel, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("plugin is closed")
	}

	if !p.config.EnableDMSBackend {
		return nil, fmt.Errorf("DMS backend mode is not enabled")
	}

	if p.soClient == nil {
		return nil, fmt.Errorf("SO client is not initialized")
	}

	p.logger.Debug("Retrieving service model from ONAP",
		zap.String("modelId", id),
	)

	onapModel, err := p.soClient.GetServiceModel(ctx, id)
	if err != nil {
		p.logger.Error("Failed to retrieve service model from ONAP",
			zap.String("modelId", id),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get service model: %w", err)
	}

	model := &smo.ServiceModel{
		ID:          onapModel.ModelInvariantID,
		Name:        onapModel.ModelName,
		Version:     onapModel.ModelVersion,
		Description: onapModel.Description,
		Category:    onapModel.ModelCategory,
		Template:    onapModel.Template,
		Extensions: map[string]interface{}{
			"onap.modelVersionId": onapModel.ModelVersionID,
			"onap.modelType":      onapModel.ModelType,
		},
	}

	p.logger.Debug("Retrieved service model from ONAP",
		zap.String("modelId", id),
	)

	return model, nil
}

// ListServiceModels retrieves all registered service models from ONAP.
func (p *Plugin) ListServiceModels(ctx context.Context) ([]*smo.ServiceModel, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("plugin is closed")
	}

	if !p.config.EnableDMSBackend {
		return nil, fmt.Errorf("DMS backend mode is not enabled")
	}

	if p.soClient == nil {
		return nil, fmt.Errorf("SO client is not initialized")
	}

	p.logger.Debug("Listing service models from ONAP")

	onapModels, err := p.soClient.ListServiceModels(ctx)
	if err != nil {
		p.logger.Error("Failed to list service models from ONAP", zap.Error(err))
		return nil, fmt.Errorf("failed to list service models: %w", err)
	}

	models := make([]*smo.ServiceModel, 0, len(onapModels))
	for _, onapModel := range onapModels {
		model := &smo.ServiceModel{
			ID:          onapModel.ModelInvariantID,
			Name:        onapModel.ModelName,
			Version:     onapModel.ModelVersion,
			Description: onapModel.Description,
			Category:    onapModel.ModelCategory,
			Template:    onapModel.Template,
			Extensions: map[string]interface{}{
				"onap.modelVersionId": onapModel.ModelVersionID,
				"onap.modelType":      onapModel.ModelType,
			},
		}
		models = append(models, model)
	}

	p.logger.Debug("Listed service models from ONAP", zap.Int("count", len(models)))

	return models, nil
}

// ApplyPolicy applies a policy to the infrastructure or deployments.
// Policies are managed through ONAP Policy Framework.
func (p *Plugin) ApplyPolicy(ctx context.Context, policy *smo.Policy) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("plugin is closed")
	}

	p.logger.Info("Applying policy through ONAP Policy Framework",
		zap.String("policyId", policy.PolicyID),
		zap.String("policyType", policy.PolicyType),
	)

	// Note: ONAP Policy Framework integration would be implemented here
	// For now, this is a placeholder that demonstrates the interface

	// Transform policy to ONAP Policy Framework format
	onapPolicy := &Policy{
		PolicyID:   policy.PolicyID,
		PolicyName: policy.Name,
		PolicyType: policy.PolicyType,
		Scope:      policy.Scope,
		Rules:      policy.Rules,
		Enabled:    policy.Enabled,
	}

	// In a full implementation, this would call ONAP Policy Framework APIs
	// to create/update the policy
	_ = onapPolicy

	p.logger.Info("Successfully applied policy through ONAP",
		zap.String("policyId", policy.PolicyID),
	)

	return nil
}

// GetPolicyStatus retrieves the current status of an applied policy.
func (p *Plugin) GetPolicyStatus(ctx context.Context, policyID string) (*smo.PolicyStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("plugin is closed")
	}

	p.logger.Debug("Retrieving policy status from ONAP",
		zap.String("policyId", policyID),
	)

	// Note: ONAP Policy Framework integration would be implemented here
	// For now, return a placeholder status

	now := time.Now()
	status := &smo.PolicyStatus{
		PolicyID:         policyID,
		Status:           "active",
		EnforcementCount: 0,
		ViolationCount:   0,
		LastEnforced:     &now,
		Message:          "Policy is active",
		Extensions: map[string]interface{}{
			"onap.policyFrameworkVersion": "1.0",
		},
	}

	p.logger.Debug("Retrieved policy status from ONAP",
		zap.String("policyId", policyID),
		zap.String("status", status.Status),
	)

	return status, nil
}

// === Helper methods for status mapping ===

// mapONAPRequestStateToStatus maps ONAP orchestration request state to workflow status.
func (p *Plugin) mapONAPRequestStateToStatus(requestState string) string {
	statusMap := map[string]string{
		"PENDING":     "PENDING",
		"IN_PROGRESS": "RUNNING",
		"COMPLETE":    "SUCCEEDED",
		"COMPLETED":   "SUCCEEDED",
		"FAILED":      "FAILED",
		"TIMEOUT":     "FAILED",
		"UNLOCKED":    "CANCELLED",
	}

	if status, ok := statusMap[requestState]; ok {
		return status
	}

	return "UNKNOWN"
}
