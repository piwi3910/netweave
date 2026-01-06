// Package adapter provides the abstraction layer for different O2-DMS backend implementations.
// It defines the DMSAdapter interface that all deployment management systems must implement
// to provide O2-DMS functionality through the netweave gateway.
package adapter

import (
	"context"
	"time"
)

// Capability represents a feature that a DMS adapter supports.
// Capabilities are used during adapter selection to ensure the chosen
// adapter can fulfill the requirements of a specific O2-DMS operation.
type Capability string

const (
	// CapabilityPackageManagement indicates support for deployment package upload and management.
	CapabilityPackageManagement Capability = "package-management"

	// CapabilityDeploymentLifecycle indicates support for deployment CRUD operations.
	CapabilityDeploymentLifecycle Capability = "deployment-lifecycle"

	// CapabilityRollback indicates support for deployment rollback to previous versions.
	CapabilityRollback Capability = "rollback"

	// CapabilityScaling indicates support for horizontal scaling of deployments.
	CapabilityScaling Capability = "scaling"

	// CapabilityGitOps indicates support for GitOps-based deployments.
	CapabilityGitOps Capability = "gitops"

	// CapabilityHealthChecks indicates support for deployment health status reporting.
	CapabilityHealthChecks Capability = "health-checks"

	// CapabilityMetrics indicates support for deployment metrics and monitoring.
	CapabilityMetrics Capability = "metrics"
)

// Filter provides criteria for filtering O2-DMS resources.
// Filters are used in List operations to narrow down results based on
// deployment attributes, labels, namespace, and custom extensions.
type Filter struct {
	// Namespace filters deployments by Kubernetes namespace.
	Namespace string

	// Status filters deployments by their current status.
	Status DeploymentStatus

	// Labels provides key-value label matching for deployments.
	Labels map[string]string

	// Extensions allows filtering based on vendor-specific extension fields.
	Extensions map[string]interface{}

	// Limit specifies the maximum number of results to return.
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// DeploymentStatus represents the current state of a deployment.
type DeploymentStatus string

const (
	// DeploymentStatusPending indicates the deployment is queued but not started.
	DeploymentStatusPending DeploymentStatus = "pending"

	// DeploymentStatusDeploying indicates the deployment is in progress.
	DeploymentStatusDeploying DeploymentStatus = "deploying"

	// DeploymentStatusDeployed indicates the deployment completed successfully.
	DeploymentStatusDeployed DeploymentStatus = "deployed"

	// DeploymentStatusFailed indicates the deployment failed.
	DeploymentStatusFailed DeploymentStatus = "failed"

	// DeploymentStatusRollingBack indicates a rollback is in progress.
	DeploymentStatusRollingBack DeploymentStatus = "rolling-back"

	// DeploymentStatusDeleting indicates the deployment is being removed.
	DeploymentStatusDeleting DeploymentStatus = "deleting"
)

// DeploymentPackage represents an O2-DMS deployment package (e.g., Helm chart, ArgoCD app).
type DeploymentPackage struct {
	// ID is the unique identifier for this package.
	ID string `json:"id"`

	// Name is the human-readable name of the package.
	Name string `json:"name"`

	// Version is the package version (semver format).
	Version string `json:"version"`

	// PackageType identifies the package format (e.g., "helm-chart", "git-repo").
	PackageType string `json:"packageType"`

	// Description provides additional context about the package.
	Description string `json:"description,omitempty"`

	// UploadedAt is the timestamp when the package was uploaded.
	UploadedAt time.Time `json:"uploadedAt"`

	// Extensions provides vendor-specific additional metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// DeploymentPackageUpload contains data for uploading a new deployment package.
type DeploymentPackageUpload struct {
	// Name is the package name.
	Name string

	// Version is the package version.
	Version string

	// PackageType identifies the package format.
	PackageType string

	// Description provides context about the package.
	Description string

	// Content contains the package data (e.g., tarball, chart archive).
	Content []byte

	// Repository is the Helm repository URL (for Helm packages).
	Repository string

	// Extensions provides vendor-specific upload parameters.
	Extensions map[string]interface{}
}

// Deployment represents an O2-DMS deployment instance.
type Deployment struct {
	// ID is the unique identifier for this deployment (e.g., Helm release name).
	ID string `json:"id"`

	// Name is the human-readable name of the deployment.
	Name string `json:"name"`

	// PackageID references the deployment package used.
	PackageID string `json:"packageId"`

	// Namespace is the Kubernetes namespace where the deployment runs.
	Namespace string `json:"namespace"`

	// Status is the current deployment status.
	Status DeploymentStatus `json:"status"`

	// Version is the current deployed version/revision.
	Version int `json:"version"`

	// Description provides additional context.
	Description string `json:"description,omitempty"`

	// CreatedAt is the timestamp when the deployment was created.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp of the last update.
	UpdatedAt time.Time `json:"updatedAt"`

	// Extensions provides vendor-specific additional metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// DeploymentRequest contains parameters for creating a new deployment.
type DeploymentRequest struct {
	// Name is the deployment name.
	Name string

	// PackageID references the deployment package to deploy.
	PackageID string

	// Namespace is the target Kubernetes namespace.
	Namespace string

	// Values contains deployment configuration values (e.g., Helm values).
	Values map[string]interface{}

	// Description provides context about the deployment.
	Description string

	// Extensions provides vendor-specific deployment parameters.
	Extensions map[string]interface{}
}

// DeploymentUpdate contains parameters for updating an existing deployment.
type DeploymentUpdate struct {
	// Values contains updated configuration values.
	Values map[string]interface{}

	// Description provides context about the update.
	Description string

	// Extensions provides vendor-specific update parameters.
	Extensions map[string]interface{}
}

// DeploymentStatusDetail provides detailed status information for a deployment.
type DeploymentStatusDetail struct {
	// DeploymentID is the deployment identifier.
	DeploymentID string `json:"deploymentId"`

	// Status is the current deployment status.
	Status DeploymentStatus `json:"status"`

	// Message provides human-readable status information.
	Message string `json:"message,omitempty"`

	// Progress indicates deployment progress (0-100).
	Progress int `json:"progress"`

	// Conditions contains detailed status conditions.
	Conditions []DeploymentCondition `json:"conditions,omitempty"`

	// UpdatedAt is the timestamp of the last status update.
	UpdatedAt time.Time `json:"updatedAt"`

	// Extensions provides vendor-specific status information.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// DeploymentCondition represents a specific condition in the deployment status.
type DeploymentCondition struct {
	// Type identifies the condition type (e.g., "Ready", "Available").
	Type string `json:"type"`

	// Status indicates if the condition is true, false, or unknown.
	Status string `json:"status"`

	// Reason provides a programmatic identifier for the condition.
	Reason string `json:"reason,omitempty"`

	// Message provides human-readable details.
	Message string `json:"message,omitempty"`

	// LastTransitionTime is when the condition last changed.
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

// LogOptions specifies parameters for retrieving deployment logs.
type LogOptions struct {
	// Container specifies which container's logs to retrieve.
	Container string

	// TailLines limits the number of recent log lines to return.
	TailLines int

	// Since returns logs after this timestamp.
	Since time.Time

	// Follow indicates if logs should be streamed.
	Follow bool
}

// DeploymentHistory represents the revision history of a deployment.
type DeploymentHistory struct {
	// DeploymentID is the deployment identifier.
	DeploymentID string `json:"deploymentId"`

	// Revisions contains the list of historical revisions.
	Revisions []DeploymentRevision `json:"revisions"`
}

// DeploymentRevision represents a single revision in deployment history.
type DeploymentRevision struct {
	// Revision is the revision number.
	Revision int `json:"revision"`

	// Version is the package version deployed in this revision.
	Version string `json:"version"`

	// DeployedAt is when this revision was deployed.
	DeployedAt time.Time `json:"deployedAt"`

	// Status indicates if this revision deployed successfully.
	Status DeploymentStatus `json:"status"`

	// Description provides context about this revision.
	Description string `json:"description,omitempty"`
}

// DMSAdapter defines the interface that all DMS backend implementations must provide.
// Implementations include Helm, ArgoCD, Flux, ONAP-LCM, OSM-LCM, etc.
// Each adapter translates O2-DMS operations to backend-specific API calls.
type DMSAdapter interface {
	// Metadata methods

	// Name returns the unique name of this adapter (e.g., "helm", "argocd", "flux").
	Name() string

	// Version returns the version of the backend system this adapter supports.
	Version() string

	// Capabilities returns the list of O2-DMS capabilities this adapter supports.
	Capabilities() []Capability

	// Package Management operations

	// ListDeploymentPackages retrieves all deployment packages matching the filter.
	// The filter parameter can be nil to retrieve all packages.
	// Returns a slice of packages or an error.
	ListDeploymentPackages(ctx context.Context, filter *Filter) ([]*DeploymentPackage, error)

	// GetDeploymentPackage retrieves a specific deployment package by ID.
	// Returns the package or an error if not found.
	GetDeploymentPackage(ctx context.Context, id string) (*DeploymentPackage, error)

	// UploadDeploymentPackage uploads a new deployment package.
	// Returns the created package with server-assigned fields populated.
	UploadDeploymentPackage(ctx context.Context, pkg *DeploymentPackageUpload) (*DeploymentPackage, error)

	// DeleteDeploymentPackage deletes a deployment package by ID.
	// Returns an error if the package doesn't exist or is in use.
	DeleteDeploymentPackage(ctx context.Context, id string) error

	// Deployment Lifecycle operations

	// ListDeployments retrieves all deployments matching the provided filter.
	// The filter parameter can be nil to retrieve all deployments.
	// Returns a slice of deployments or an error.
	ListDeployments(ctx context.Context, filter *Filter) ([]*Deployment, error)

	// GetDeployment retrieves a specific deployment by ID.
	// Returns the deployment or an error if not found.
	GetDeployment(ctx context.Context, id string) (*Deployment, error)

	// CreateDeployment creates a new deployment from a package.
	// Returns the created deployment with server-assigned fields populated.
	CreateDeployment(ctx context.Context, req *DeploymentRequest) (*Deployment, error)

	// UpdateDeployment updates an existing deployment (e.g., upgrade to new version).
	// Returns the updated deployment or an error if the deployment doesn't exist.
	UpdateDeployment(ctx context.Context, id string, update *DeploymentUpdate) (*Deployment, error)

	// DeleteDeployment deletes a deployment by ID (uninstall).
	// Returns an error if the deployment doesn't exist or cannot be deleted.
	DeleteDeployment(ctx context.Context, id string) error

	// Deployment Operations

	// ScaleDeployment scales a deployment to the specified number of replicas.
	// Returns an error if scaling is not supported or fails.
	ScaleDeployment(ctx context.Context, id string, replicas int) error

	// RollbackDeployment rolls back a deployment to a previous revision.
	// Returns an error if rollback is not supported or fails.
	RollbackDeployment(ctx context.Context, id string, revision int) error

	// GetDeploymentStatus retrieves detailed status for a deployment.
	// Returns the status or an error if the deployment doesn't exist.
	GetDeploymentStatus(ctx context.Context, id string) (*DeploymentStatusDetail, error)

	// GetDeploymentHistory retrieves the revision history for a deployment.
	// Returns the history or an error if the deployment doesn't exist.
	GetDeploymentHistory(ctx context.Context, id string) (*DeploymentHistory, error)

	// GetDeploymentLogs retrieves logs for a deployment.
	// Returns the logs as bytes or an error if the deployment doesn't exist.
	GetDeploymentLogs(ctx context.Context, id string, opts *LogOptions) ([]byte, error)

	// Capability checks

	// SupportsRollback indicates if the adapter supports deployment rollback.
	SupportsRollback() bool

	// SupportsScaling indicates if the adapter supports horizontal scaling.
	SupportsScaling() bool

	// SupportsGitOps indicates if the adapter supports GitOps workflows.
	SupportsGitOps() bool

	// Lifecycle methods

	// Health performs a health check on the backend system.
	// Returns nil if healthy, or an error describing the health issue.
	Health(ctx context.Context) error

	// Close cleanly shuts down the adapter and releases resources.
	// Returns an error if shutdown fails.
	Close() error
}
