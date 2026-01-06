// Package smo provides the plugin interface and types for SMO (Service Management and Orchestration) integration.
// SMO plugins enable bidirectional integration with O-RAN SMO systems like ONAP, OSM, and custom SMO implementations.
package smo

import (
	"context"
	"time"
)

// Plugin defines the interface that all SMO integration plugins must implement.
// SMO plugins operate in dual mode:
// - Northbound: netweave → SMO (inventory sync, event publishing)
// - Southbound: SMO → netweave (workflow orchestration, deployments)
type Plugin interface {
	// Metadata returns the plugin's identifying information.
	Metadata() PluginMetadata

	// Capabilities returns the list of features this plugin supports.
	Capabilities() []Capability

	// Initialize initializes the plugin with the provided configuration.
	// Returns an error if initialization fails (invalid config, connection issues, etc.).
	Initialize(ctx context.Context, config map[string]interface{}) error

	// Health checks the health status of all SMO component connections.
	// Returns HealthStatus indicating overall health and component-specific details.
	Health(ctx context.Context) HealthStatus

	// Close cleanly shuts down the plugin and releases all resources.
	// Returns an error if shutdown fails.
	Close() error

	// === NORTHBOUND MODE: netweave → SMO ===

	// SyncInfrastructureInventory synchronizes O2-IMS infrastructure inventory to the SMO.
	// This includes deployment managers, resource pools, resources, and resource types.
	SyncInfrastructureInventory(ctx context.Context, inventory *InfrastructureInventory) error

	// SyncDeploymentInventory synchronizes O2-DMS deployment inventory to the SMO.
	// This includes deployment packages, active deployments, and deployment status.
	SyncDeploymentInventory(ctx context.Context, inventory *DeploymentInventory) error

	// PublishInfrastructureEvent publishes an infrastructure change event to the SMO.
	// Events are triggered by resource lifecycle changes (created, updated, deleted).
	PublishInfrastructureEvent(ctx context.Context, event *InfrastructureEvent) error

	// PublishDeploymentEvent publishes a deployment change event to the SMO.
	// Events are triggered by deployment lifecycle changes (started, succeeded, failed).
	PublishDeploymentEvent(ctx context.Context, event *DeploymentEvent) error

	// === SOUTHBOUND MODE: SMO → netweave ===

	// ExecuteWorkflow executes a workflow orchestrated by the SMO.
	// Returns WorkflowExecution with execution ID and initial status.
	ExecuteWorkflow(ctx context.Context, workflow *WorkflowRequest) (*WorkflowExecution, error)

	// GetWorkflowStatus retrieves the current status of a workflow execution.
	// Returns WorkflowStatus with detailed progress and state information.
	GetWorkflowStatus(ctx context.Context, executionID string) (*WorkflowStatus, error)

	// CancelWorkflow cancels a running workflow execution.
	// Returns an error if the workflow cannot be cancelled (already completed, not found, etc.).
	CancelWorkflow(ctx context.Context, executionID string) error

	// RegisterServiceModel registers a service model with the SMO.
	// Service models define deployment templates and orchestration logic.
	RegisterServiceModel(ctx context.Context, model *ServiceModel) error

	// GetServiceModel retrieves a registered service model by ID.
	// Returns the service model or an error if not found.
	GetServiceModel(ctx context.Context, id string) (*ServiceModel, error)

	// ListServiceModels retrieves all registered service models.
	// Returns a list of service models or an error.
	ListServiceModels(ctx context.Context) ([]*ServiceModel, error)

	// ApplyPolicy applies a policy to the infrastructure or deployments.
	// Policies define rules for placement, scaling, healing, and resource management.
	ApplyPolicy(ctx context.Context, policy *Policy) error

	// GetPolicyStatus retrieves the current status of an applied policy.
	// Returns PolicyStatus with enforcement details and violations.
	GetPolicyStatus(ctx context.Context, policyID string) (*PolicyStatus, error)
}

// PluginMetadata contains identifying information about a plugin.
type PluginMetadata struct {
	// Name is the unique identifier for the plugin (e.g., "onap", "osm", "custom-smo").
	Name string

	// Version is the semantic version of the plugin implementation.
	Version string

	// Description provides a human-readable description of the plugin.
	Description string

	// Vendor identifies the organization providing the plugin.
	Vendor string
}

// Capability represents a feature that a plugin supports.
type Capability string

const (
	// CapInventorySync indicates support for syncing infrastructure/deployment inventory.
	CapInventorySync Capability = "inventory-sync"

	// CapEventPublishing indicates support for publishing infrastructure/deployment events.
	CapEventPublishing Capability = "event-publishing"

	// CapWorkflowOrchestration indicates support for executing SMO-orchestrated workflows.
	CapWorkflowOrchestration Capability = "workflow-orchestration"

	// CapServiceModeling indicates support for service model registration and management.
	CapServiceModeling Capability = "service-modeling"

	// CapPolicyManagement indicates support for policy enforcement and management.
	CapPolicyManagement Capability = "policy-management"
)

// HealthStatus represents the health status of a plugin and its dependencies.
type HealthStatus struct {
	// Healthy indicates whether the overall plugin is healthy.
	Healthy bool

	// Message provides a summary of the health status.
	Message string

	// Details provides component-specific health information (e.g., per-service status).
	Details map[string]ComponentHealth

	// Timestamp indicates when the health check was performed.
	Timestamp time.Time
}

// ComponentHealth represents the health of a specific SMO component.
type ComponentHealth struct {
	// Name is the component identifier (e.g., "aai", "dmaap", "so", "sdnc").
	Name string

	// Healthy indicates whether this component is healthy.
	Healthy bool

	// Message provides details about the component's health.
	Message string

	// ResponseTime is the latency of the health check request (if applicable).
	ResponseTime time.Duration
}

// InfrastructureInventory represents a snapshot of O2-IMS infrastructure inventory.
type InfrastructureInventory struct {
	// DeploymentManagers lists all deployment managers.
	DeploymentManagers []DeploymentManager

	// ResourcePools lists all resource pools.
	ResourcePools []ResourcePool

	// Resources lists all resources.
	Resources []Resource

	// ResourceTypes lists all resource types.
	ResourceTypes []ResourceType

	// Timestamp indicates when the inventory snapshot was taken.
	Timestamp time.Time
}

// DeploymentManager represents O2-IMS deployment manager metadata.
type DeploymentManager struct {
	ID                 string
	Name               string
	Description        string
	OCloudID           string
	ServiceURI         string
	Capabilities       []string
	SupportedLocations []string
	Extensions         map[string]interface{}
}

// ResourcePool represents an O2-IMS resource pool.
type ResourcePool struct {
	ID               string
	Name             string
	Description      string
	Location         string
	GlobalLocationID string
	OCloudID         string
	Extensions       map[string]interface{}
}

// Resource represents an O2-IMS resource (compute node, VM, etc.).
type Resource struct {
	ID             string
	ResourceTypeID string
	ResourcePoolID string
	GlobalAssetID  string
	Description    string
	Extensions     map[string]interface{}
}

// ResourceType represents an O2-IMS resource type.
type ResourceType struct {
	ID            string
	Name          string
	Description   string
	Vendor        string
	Model         string
	Version       string
	ResourceClass string
	ResourceKind  string
	Extensions    map[string]interface{}
}

// DeploymentInventory represents a snapshot of O2-DMS deployment inventory.
type DeploymentInventory struct {
	// Packages lists all deployment packages.
	Packages []DeploymentPackage

	// Deployments lists all active deployments.
	Deployments []Deployment

	// Timestamp indicates when the inventory snapshot was taken.
	Timestamp time.Time
}

// DeploymentPackage represents an O2-DMS deployment package.
type DeploymentPackage struct {
	ID          string
	Name        string
	Version     string
	PackageType string
	UploadedAt  time.Time
	Extensions  map[string]interface{}
}

// Deployment represents an O2-DMS deployment.
type Deployment struct {
	ID         string
	Name       string
	PackageID  string
	Namespace  string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Extensions map[string]interface{}
}

// InfrastructureEvent represents an infrastructure change event.
type InfrastructureEvent struct {
	// EventID is the unique identifier for this event.
	EventID string

	// EventType describes the type of change (e.g., "ResourceCreated", "ResourceDeleted").
	EventType string

	// ResourceType indicates the type of resource affected.
	ResourceType string

	// ResourceID is the identifier of the affected resource.
	ResourceID string

	// Timestamp indicates when the event occurred.
	Timestamp time.Time

	// Payload contains event-specific data.
	Payload map[string]interface{}
}

// DeploymentEvent represents a deployment change event.
type DeploymentEvent struct {
	// EventID is the unique identifier for this event.
	EventID string

	// EventType describes the type of change (e.g., "DeploymentStarted", "DeploymentFailed").
	EventType string

	// DeploymentID is the identifier of the affected deployment.
	DeploymentID string

	// Timestamp indicates when the event occurred.
	Timestamp time.Time

	// Payload contains event-specific data.
	Payload map[string]interface{}
}

// WorkflowRequest represents a request to execute a workflow.
type WorkflowRequest struct {
	// WorkflowName identifies the workflow to execute.
	WorkflowName string

	// Parameters provides input parameters for the workflow.
	Parameters map[string]interface{}

	// Timeout specifies the maximum execution time for the workflow.
	Timeout time.Duration
}

// WorkflowExecution represents a workflow execution instance.
type WorkflowExecution struct {
	// ExecutionID is the unique identifier for this execution.
	ExecutionID string

	// WorkflowName is the name of the workflow being executed.
	WorkflowName string

	// Status indicates the current execution status.
	Status string

	// StartedAt indicates when the execution started.
	StartedAt time.Time

	// Extensions provides workflow-specific metadata.
	Extensions map[string]interface{}
}

// WorkflowStatus represents the detailed status of a workflow execution.
type WorkflowStatus struct {
	// ExecutionID is the unique identifier for this execution.
	ExecutionID string

	// WorkflowName is the name of the workflow.
	WorkflowName string

	// Status indicates the current execution status.
	Status string

	// Progress indicates the completion percentage (0-100).
	Progress int

	// Message provides human-readable status information.
	Message string

	// StartedAt indicates when the execution started.
	StartedAt time.Time

	// CompletedAt indicates when the execution completed (if finished).
	CompletedAt *time.Time

	// Result contains the workflow output (if completed successfully).
	Result map[string]interface{}

	// Error contains error details (if failed).
	Error string

	// Extensions provides workflow-specific metadata.
	Extensions map[string]interface{}
}

// ServiceModel represents a service deployment model.
type ServiceModel struct {
	// ID is the unique identifier for this service model.
	ID string

	// Name is the human-readable name of the service model.
	Name string

	// Version is the semantic version of the service model.
	Version string

	// Description provides details about the service model.
	Description string

	// Category categorizes the service (e.g., "5G", "MEC", "network-slice").
	Category string

	// Template contains the service model definition (format depends on SMO).
	Template interface{}

	// Extensions provides SMO-specific metadata.
	Extensions map[string]interface{}
}

// Policy represents an infrastructure or deployment policy.
type Policy struct {
	// PolicyID is the unique identifier for this policy.
	PolicyID string

	// Name is the human-readable name of the policy.
	Name string

	// PolicyType indicates the type of policy (e.g., "placement", "scaling", "healing").
	PolicyType string

	// Scope defines where the policy applies (resource pool, namespace, etc.).
	Scope map[string]string

	// Rules contains the policy rules and conditions.
	Rules interface{}

	// Enabled indicates whether the policy is currently active.
	Enabled bool

	// Extensions provides SMO-specific metadata.
	Extensions map[string]interface{}
}

// PolicyStatus represents the status of an applied policy.
type PolicyStatus struct {
	// PolicyID is the unique identifier for the policy.
	PolicyID string

	// Status indicates the enforcement status (e.g., "active", "violated", "suspended").
	Status string

	// EnforcementCount is the number of times the policy has been enforced.
	EnforcementCount int

	// ViolationCount is the number of times the policy has been violated.
	ViolationCount int

	// LastEnforced indicates when the policy was last enforced.
	LastEnforced *time.Time

	// LastViolation indicates when the policy was last violated.
	LastViolation *time.Time

	// Message provides human-readable status information.
	Message string

	// Extensions provides SMO-specific metadata.
	Extensions map[string]interface{}
}
