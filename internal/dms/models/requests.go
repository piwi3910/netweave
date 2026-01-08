// Package models contains the O2-DMS data models for the netweave gateway.
package models

// CreateNFDeploymentRequest contains parameters for creating a new NF deployment.
type CreateNFDeploymentRequest struct {
	// Name is the deployment name.
	Name string `json:"name" binding:"required"`

	// Description provides context about the deployment.
	Description string `json:"description,omitempty"`

	// NFDeploymentDescriptorID references the descriptor to use for deployment.
	NFDeploymentDescriptorID string `json:"nfDeploymentDescriptorId" binding:"required"`

	// Namespace is the target Kubernetes namespace.
	Namespace string `json:"namespace,omitempty"`

	// ParameterValues contains deployment parameter values.
	ParameterValues map[string]interface{} `json:"parameterValues,omitempty"`

	// Extensions provides vendor-specific deployment parameters.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// UpdateNFDeploymentRequest contains parameters for updating an NF deployment.
type UpdateNFDeploymentRequest struct {
	// Description provides updated context about the deployment.
	Description string `json:"description,omitempty"`

	// ParameterValues contains updated parameter values.
	ParameterValues map[string]interface{} `json:"parameterValues,omitempty"`

	// Extensions provides vendor-specific update parameters.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ScaleNFDeploymentRequest contains parameters for scaling an NF deployment.
type ScaleNFDeploymentRequest struct {
	// Replicas is the target number of replicas.
	Replicas int `json:"replicas" binding:"required,min=0"`
}

// HealNFDeploymentRequest contains parameters for healing an NF deployment.
type HealNFDeploymentRequest struct {
	// Cause describes the reason for healing.
	Cause string `json:"cause,omitempty"`

	// AdditionalParams provides vendor-specific healing parameters.
	AdditionalParams map[string]interface{} `json:"additionalParams,omitempty"`
}

// RollbackNFDeploymentRequest contains parameters for rolling back an NF deployment.
type RollbackNFDeploymentRequest struct {
	// TargetRevision is the revision to roll back to.
	// If not specified, rolls back to the previous revision.
	TargetRevision *int `json:"targetRevision,omitempty"`
}

// CreateNFDeploymentDescriptorRequest contains parameters for creating a descriptor.
type CreateNFDeploymentDescriptorRequest struct {
	// Name is the descriptor name.
	Name string `json:"name" binding:"required"`

	// Description provides context about the descriptor.
	Description string `json:"description,omitempty"`

	// ArtifactName is the name of the deployment artifact.
	ArtifactName string `json:"artifactName" binding:"required"`

	// ArtifactVersion is the version of the deployment artifact.
	ArtifactVersion string `json:"artifactVersion,omitempty"`

	// ArtifactType is the type of deployment artifact.
	ArtifactType string `json:"artifactType,omitempty"`

	// ArtifactRepository is the repository URL for the artifact.
	ArtifactRepository string `json:"artifactRepository,omitempty"`

	// InputParameters defines configurable parameters.
	InputParameters []ParameterDefinition `json:"inputParameters,omitempty"`

	// Extensions provides vendor-specific fields.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// CreateDMSSubscriptionRequest contains parameters for creating a DMS subscription.
type CreateDMSSubscriptionRequest struct {
	// Callback is the webhook URL where notifications will be sent.
	Callback string `json:"callback" binding:"required,url"`

	// ConsumerSubscriptionID is an optional client-provided identifier.
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty"`

	// Filter specifies which events should trigger notifications.
	Filter *DMSSubscriptionFilter `json:"filter,omitempty"`

	// Extensions provides vendor-specific fields.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// NFDeploymentListResponse is the response for listing NF deployments.
type NFDeploymentListResponse struct {
	// NFDeployments is the list of NF deployments.
	NFDeployments []*NFDeployment `json:"nfDeployments"`

	// Total is the total number of deployments.
	Total int `json:"total"`
}

// NFDeploymentDescriptorListResponse is the response for listing descriptors.
type NFDeploymentDescriptorListResponse struct {
	// NFDeploymentDescriptors is the list of descriptors.
	NFDeploymentDescriptors []*NFDeploymentDescriptor `json:"nfDeploymentDescriptors"`

	// Total is the total number of descriptors.
	Total int `json:"total"`
}

// DMSSubscriptionListResponse is the response for listing DMS subscriptions.
type DMSSubscriptionListResponse struct {
	// Subscriptions is the list of subscriptions.
	Subscriptions []*DMSSubscription `json:"subscriptions"`

	// Total is the total number of subscriptions.
	Total int `json:"total"`
}

// DeploymentHistoryResponse is the response for deployment history.
type DeploymentHistoryResponse struct {
	// NFDeploymentID is the deployment identifier.
	NFDeploymentID string `json:"nfDeploymentId"`

	// Revisions contains the list of historical revisions.
	Revisions []DeploymentRevision `json:"revisions"`
}

// DeploymentRevision represents a single revision in deployment history.
type DeploymentRevision struct {
	// Revision is the revision number.
	Revision int `json:"revision"`

	// Status indicates the status of this revision.
	Status NFDeploymentStatus `json:"status"`

	// Description provides context about this revision.
	Description string `json:"description,omitempty"`

	// DeployedAt is when this revision was deployed.
	DeployedAt string `json:"deployedAt"`
}

// DeploymentStatusResponse is the response for deployment status.
type DeploymentStatusResponse struct {
	// NFDeploymentID is the deployment identifier.
	NFDeploymentID string `json:"nfDeploymentId"`

	// Status is the current deployment status.
	Status NFDeploymentStatus `json:"status"`

	// StatusMessage provides additional status information.
	StatusMessage string `json:"statusMessage,omitempty"`

	// Progress indicates deployment progress (0-100).
	Progress int `json:"progress"`

	// Conditions contains detailed status conditions.
	Conditions []DeploymentCondition `json:"conditions,omitempty"`

	// UpdatedAt is the timestamp of the last status update.
	UpdatedAt string `json:"updatedAt"`
}

// DeploymentCondition represents a specific condition in deployment status.
type DeploymentCondition struct {
	// Type identifies the condition type.
	Type string `json:"type"`

	// Status indicates if the condition is true, false, or unknown.
	Status string `json:"status"`

	// Reason provides a programmatic identifier for the condition.
	Reason string `json:"reason,omitempty"`

	// Message provides human-readable details.
	Message string `json:"message,omitempty"`

	// LastTransitionTime is when the condition last changed.
	LastTransitionTime string `json:"lastTransitionTime"`
}

// APIError represents an O2-DMS API error response.
type APIError struct {
	// Error is the error type identifier.
	Error string `json:"error"`

	// Message provides human-readable error details.
	Message string `json:"message"`

	// Code is the HTTP status code.
	Code int `json:"code"`

	// Details provides additional error context.
	Details map[string]interface{} `json:"details,omitempty"`
}

// ListFilter provides common filtering parameters for list operations.
type ListFilter struct {
	// Namespace filters by Kubernetes namespace.
	Namespace string `form:"namespace"`

	// Status filters by deployment status.
	Status string `form:"status"`

	// Limit is the maximum number of results to return.
	Limit int `form:"limit"`

	// Offset is the starting position for pagination.
	Offset int `form:"offset"`
}
