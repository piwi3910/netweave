// Package models contains the O2-DMS data models for the netweave gateway.
// These models represent O-RAN O2 Deployment Management Service (DMS) resources
// as defined in the O-RAN.WG6.O2DMS-INTERFACE specification.
package models

import (
	"time"
)

// NFDeployment represents an O2-DMS NF Deployment.
// An NF Deployment is a running instance of a Network Function that has been
// deployed to the infrastructure using a deployment descriptor.
//
// Example:
//
//	deployment := &NFDeployment{
//	    NFDeploymentID:           "nfd-550e8400-e29b-41d4",
//	    Name:                     "cu-up-deployment-1",
//	    Description:              "CU-UP for cell site A",
//	    NFDeploymentDescriptorID: "nfdd-abc123",
//	    Status:                   NFDeploymentStatusDeployed,
//	}
type NFDeployment struct {
	// NFDeploymentID is the unique identifier for this NF deployment.
	NFDeploymentID string `json:"nfDeploymentId" yaml:"nfDeploymentId"`

	// Name is the human-readable name of the NF deployment.
	Name string `json:"name" yaml:"name"`

	// Description provides additional details about the NF deployment.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// NFDeploymentDescriptorID references the descriptor used to create this deployment.
	NFDeploymentDescriptorID string `json:"nfDeploymentDescriptorId" yaml:"nfDeploymentDescriptorId"`

	// Status is the current status of the NF deployment.
	Status NFDeploymentStatus `json:"status" yaml:"status"`

	// StatusMessage provides additional status information.
	StatusMessage string `json:"statusMessage,omitempty" yaml:"statusMessage,omitempty"`

	// Namespace is the Kubernetes namespace where the deployment runs.
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// Version is the current deployed version/revision number.
	Version int `json:"version" yaml:"version"`

	// ParameterValues contains the deployment parameter values.
	ParameterValues map[string]interface{} `json:"parameterValues,omitempty" yaml:"parameterValues,omitempty"`

	// CreatedAt is the timestamp when the deployment was created.
	CreatedAt time.Time `json:"createdAt" yaml:"createdAt"`

	// UpdatedAt is the timestamp of the last update.
	UpdatedAt time.Time `json:"updatedAt" yaml:"updatedAt"`

	// Extensions contains additional backend-specific or custom fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// NFDeploymentStatus represents the current state of an NF deployment.
type NFDeploymentStatus string

const (
	// NFDeploymentStatusPending indicates the deployment is queued but not started.
	NFDeploymentStatusPending NFDeploymentStatus = "pending"

	// NFDeploymentStatusInstantiating indicates the deployment is being created.
	NFDeploymentStatusInstantiating NFDeploymentStatus = "instantiating"

	// NFDeploymentStatusDeployed indicates the deployment completed successfully.
	NFDeploymentStatusDeployed NFDeploymentStatus = "deployed"

	// NFDeploymentStatusFailed indicates the deployment failed.
	NFDeploymentStatusFailed NFDeploymentStatus = "failed"

	// NFDeploymentStatusUpdating indicates the deployment is being updated.
	NFDeploymentStatusUpdating NFDeploymentStatus = "updating"

	// NFDeploymentStatusScaling indicates the deployment is being scaled.
	NFDeploymentStatusScaling NFDeploymentStatus = "scaling"

	// NFDeploymentStatusTerminating indicates the deployment is being terminated.
	NFDeploymentStatusTerminating NFDeploymentStatus = "terminating"

	// NFDeploymentStatusTerminated indicates the deployment has been terminated.
	NFDeploymentStatusTerminated NFDeploymentStatus = "terminated"
)

// IsValid checks if the NFDeploymentStatus is a valid status value.
func (s NFDeploymentStatus) IsValid() bool {
	switch s {
	case NFDeploymentStatusPending, NFDeploymentStatusInstantiating,
		NFDeploymentStatusDeployed, NFDeploymentStatusFailed,
		NFDeploymentStatusUpdating, NFDeploymentStatusScaling,
		NFDeploymentStatusTerminating, NFDeploymentStatusTerminated:
		return true
	default:
		return false
	}
}

// String returns the string representation of the NFDeploymentStatus.
func (s NFDeploymentStatus) String() string {
	return string(s)
}

// NFDeploymentDescriptor represents an O2-DMS NF Deployment Descriptor.
// A descriptor defines how a Network Function should be deployed, including
// the deployment package, parameters, and constraints.
//
// Example:
//
//	descriptor := &NFDeploymentDescriptor{
//	    NFDeploymentDescriptorID: "nfdd-abc123",
//	    Name:                     "cu-up-descriptor",
//	    Description:              "CU-UP deployment descriptor",
//	    ArtifactName:             "cu-up-chart",
//	    ArtifactVersion:          "1.2.3",
//	}
type NFDeploymentDescriptor struct {
	// NFDeploymentDescriptorID is the unique identifier for this descriptor.
	NFDeploymentDescriptorID string `json:"nfDeploymentDescriptorId" yaml:"nfDeploymentDescriptorId"`

	// Name is the human-readable name of the descriptor.
	Name string `json:"name" yaml:"name"`

	// Description provides additional details about the descriptor.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// ArtifactName is the name of the deployment artifact (e.g., Helm chart name).
	ArtifactName string `json:"artifactName" yaml:"artifactName"`

	// ArtifactVersion is the version of the deployment artifact.
	ArtifactVersion string `json:"artifactVersion,omitempty" yaml:"artifactVersion,omitempty"`

	// ArtifactType is the type of deployment artifact (e.g., "helm-chart", "kustomize").
	ArtifactType string `json:"artifactType,omitempty" yaml:"artifactType,omitempty"`

	// ArtifactRepository is the repository URL for the artifact.
	ArtifactRepository string `json:"artifactRepository,omitempty" yaml:"artifactRepository,omitempty"`

	// InputParameters defines the configurable parameters for this descriptor.
	InputParameters []ParameterDefinition `json:"inputParameters,omitempty" yaml:"inputParameters,omitempty"`

	// OutputParameters defines the outputs available after deployment.
	OutputParameters []ParameterDefinition `json:"outputParameters,omitempty" yaml:"outputParameters,omitempty"`

	// CreatedAt is the timestamp when the descriptor was created.
	CreatedAt time.Time `json:"createdAt" yaml:"createdAt"`

	// UpdatedAt is the timestamp of the last update.
	UpdatedAt time.Time `json:"updatedAt" yaml:"updatedAt"`

	// Extensions contains additional backend-specific or custom fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// ParameterDefinition defines a configurable parameter for NF deployment.
type ParameterDefinition struct {
	// Name is the parameter name.
	Name string `json:"name" yaml:"name"`

	// Description provides details about the parameter.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Type is the parameter type (e.g., "string", "integer", "boolean").
	Type string `json:"type" yaml:"type"`

	// Required indicates if the parameter must be provided.
	Required bool `json:"required" yaml:"required"`

	// DefaultValue is the default value if not specified.
	DefaultValue interface{} `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"`

	// Constraints defines validation constraints for the parameter.
	Constraints *ParameterConstraints `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

// ParameterConstraints defines validation constraints for a parameter.
type ParameterConstraints struct {
	// MinValue is the minimum allowed value (for numeric types).
	MinValue *float64 `json:"minValue,omitempty" yaml:"minValue,omitempty"`

	// MaxValue is the maximum allowed value (for numeric types).
	MaxValue *float64 `json:"maxValue,omitempty" yaml:"maxValue,omitempty"`

	// MinLength is the minimum string length (for string types).
	MinLength *int `json:"minLength,omitempty" yaml:"minLength,omitempty"`

	// MaxLength is the maximum string length (for string types).
	MaxLength *int `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`

	// Pattern is a regex pattern for validation (for string types).
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`

	// AllowedValues is a list of allowed values (enum constraint).
	AllowedValues []interface{} `json:"allowedValues,omitempty" yaml:"allowedValues,omitempty"`
}

// DMSSubscription represents an O2-DMS subscription for deployment events.
// Subscriptions allow consumers to receive webhook notifications when
// NF deployments are created, updated, or deleted.
type DMSSubscription struct {
	// SubscriptionID is the unique identifier for this subscription.
	SubscriptionID string `json:"subscriptionId" yaml:"subscriptionId"`

	// Callback is the webhook URL where notifications will be sent.
	Callback string `json:"callback" yaml:"callback"`

	// ConsumerSubscriptionID is an optional client-provided identifier.
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty" yaml:"consumerSubscriptionId,omitempty"`

	// Filter specifies which events should trigger notifications.
	Filter *DMSSubscriptionFilter `json:"filter,omitempty" yaml:"filter,omitempty"`

	// CreatedAt is the timestamp when the subscription was created.
	CreatedAt time.Time `json:"createdAt" yaml:"createdAt"`

	// UpdatedAt is the timestamp of the last update.
	UpdatedAt time.Time `json:"updatedAt" yaml:"updatedAt"`

	// Extensions contains additional backend-specific or custom fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// DMSSubscriptionFilter defines filtering criteria for DMS subscriptions.
type DMSSubscriptionFilter struct {
	// NFDeploymentIDs filters to specific NF deployments.
	NFDeploymentIDs []string `json:"nfDeploymentIds,omitempty" yaml:"nfDeploymentIds,omitempty"`

	// NFDeploymentDescriptorIDs filters to specific descriptors.
	NFDeploymentDescriptorIDs []string `json:"nfDeploymentDescriptorIds,omitempty" yaml:"nfDeploymentDescriptorIds,omitempty"`

	// EventTypes filters to specific event types.
	EventTypes []DMSEventType `json:"eventTypes,omitempty" yaml:"eventTypes,omitempty"`

	// Namespace filters to a specific Kubernetes namespace.
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// Extensions contains additional filter criteria.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// DMSEventType defines the types of DMS events.
type DMSEventType string

const (
	// DMSEventTypeDeploymentCreated is fired when an NF deployment is created.
	DMSEventTypeDeploymentCreated DMSEventType = "NFDeploymentCreated"

	// DMSEventTypeDeploymentUpdated is fired when an NF deployment is updated.
	DMSEventTypeDeploymentUpdated DMSEventType = "NFDeploymentUpdated"

	// DMSEventTypeDeploymentDeleted is fired when an NF deployment is deleted.
	DMSEventTypeDeploymentDeleted DMSEventType = "NFDeploymentDeleted"

	// DMSEventTypeDeploymentStatusChanged is fired when deployment status changes.
	DMSEventTypeDeploymentStatusChanged DMSEventType = "NFDeploymentStatusChanged"

	// DMSEventTypeDescriptorCreated is fired when a descriptor is created.
	DMSEventTypeDescriptorCreated DMSEventType = "NFDeploymentDescriptorCreated"

	// DMSEventTypeDescriptorDeleted is fired when a descriptor is deleted.
	DMSEventTypeDescriptorDeleted DMSEventType = "NFDeploymentDescriptorDeleted"
)

// IsValid checks if the DMSEventType is a valid event type.
func (e DMSEventType) IsValid() bool {
	switch e {
	case DMSEventTypeDeploymentCreated, DMSEventTypeDeploymentUpdated,
		DMSEventTypeDeploymentDeleted, DMSEventTypeDeploymentStatusChanged,
		DMSEventTypeDescriptorCreated, DMSEventTypeDescriptorDeleted:
		return true
	default:
		return false
	}
}

// String returns the string representation of the DMSEventType.
func (e DMSEventType) String() string {
	return string(e)
}

// DMSNotification represents an O2-DMS event notification.
type DMSNotification struct {
	// SubscriptionID is the ID of the subscription that triggered this notification.
	SubscriptionID string `json:"subscriptionId" yaml:"subscriptionId"`

	// ConsumerSubscriptionID is the client-provided subscription identifier.
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty" yaml:"consumerSubscriptionId,omitempty"`

	// EventType describes the type of event.
	EventType DMSEventType `json:"eventType" yaml:"eventType"`

	// NFDeployment contains the NF deployment that triggered the event (if applicable).
	NFDeployment *NFDeployment `json:"nfDeployment,omitempty" yaml:"nfDeployment,omitempty"`

	// NFDeploymentDescriptor contains the descriptor that triggered the event (if applicable).
	NFDeploymentDescriptor *NFDeploymentDescriptor `json:"nfDeploymentDescriptor,omitempty" yaml:"nfDeploymentDescriptor,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// Extensions contains additional event-specific fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}
