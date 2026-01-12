// Package dms provides the abstraction layer for O2-DMS (Deployment Management Services).
// It defines interfaces and models for managing CNF/VNF deployment lifecycle across
// different backend systems (Helm, ArgoCD, Flux, ONAP-LCM, etc.).
package dms

import (
	"context"
	"time"
)

// Capability represents a feature that a DMS adapter supports.
type Capability string

const (
	// CapPackageManagement indicates support for deployment package management.
	CapPackageManagement Capability = "package-management"

	// CapDeploymentLifecycle indicates support for deployment CRUD operations.
	CapDeploymentLifecycle Capability = "deployment-lifecycle"

	// CapRollback indicates support for rollback to previous revisions.
	CapRollback Capability = "rollback"

	// CapScaling indicates support for scaling deployments.
	CapScaling Capability = "scaling"

	// CapGitOps indicates support for GitOps workflows.
	CapGitOps Capability = "gitops"

	// CapHealthChecks indicates support for deployment health monitoring.
	CapHealthChecks Capability = "health-checks"
)

// DeploymentStatus represents the current state of a deployment.
type DeploymentStatus string

const (
	// StatusPending indicates the deployment is being created.
	StatusPending DeploymentStatus = "Pending"

	// StatusProgressing indicates the deployment is in progress.
	StatusProgressing DeploymentStatus = "Progressing"

	// StatusHealthy indicates the deployment is running and healthy.
	StatusHealthy DeploymentStatus = "Healthy"

	// StatusDegraded indicates the deployment is partially unhealthy.
	StatusDegraded DeploymentStatus = "Degraded"

	// StatusFailed indicates the deployment has failed.
	StatusFailed DeploymentStatus = "Failed"

	// StatusSuspended indicates the deployment is suspended.
	StatusSuspended DeploymentStatus = "Suspended"

	// StatusUnknown indicates the deployment status is unknown.
	StatusUnknown DeploymentStatus = "Unknown"
)

// Filter provides criteria for filtering DMS resources.
type Filter struct {
	// Namespace filters deployments by namespace.
	Namespace string

	// Labels provides key-value label matching.
	Labels map[string]string

	// Status filters deployments by status.
	Status DeploymentStatus

	// Limit specifies the maximum number of results to return.
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// DeploymentPackage represents a deployment package (Helm chart, Git repo, etc.).
type DeploymentPackage struct {
	// ID is the unique identifier for this package.
	ID string `json:"id"`

	// Name is the human-readable name of the package.
	Name string `json:"name"`

	// Version is the package version.
	Version string `json:"version"`

	// PackageType indicates the type (helm-chart, git-repo, onap-package, etc.).
	PackageType string `json:"packageType"`

	// Description provides additional context about the package.
	Description string `json:"description,omitempty"`

	// UploadedAt is the timestamp when the package was uploaded.
	UploadedAt time.Time `json:"uploadedAt"`

	// Extensions provides vendor-specific metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// DeploymentPackageUpload represents a package being uploaded.
type DeploymentPackageUpload struct {
	// Name is the package name.
	Name string `json:"name"`

	// Version is the package version.
	Version string `json:"version"`

	// PackageType indicates the type.
	PackageType string `json:"packageType"`

	// Content is the package content (chart archive, descriptor, etc.).
	Content []byte `json:"content"`

	// Metadata provides additional information.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DeploymentRequest represents a request to create a new deployment.
type DeploymentRequest struct {
	// Name is the deployment name.
	Name string `json:"name"`

	// Namespace is the target namespace.
	Namespace string `json:"namespace"`

	// PackageID references the deployment package.
	PackageID string `json:"packageId"`

	// Values provides configuration values for the deployment.
	Values map[string]interface{} `json:"values,omitempty"`

	// GitRepo is the Git repository URL (for GitOps).
	GitRepo string `json:"gitRepo,omitempty"`

	// GitRevision is the Git branch/tag/commit (for GitOps).
	GitRevision string `json:"gitRevision,omitempty"`

	// GitPath is the path within the Git repository (for GitOps).
	GitPath string `json:"gitPath,omitempty"`

	// Labels provides key-value labels for the deployment.
	Labels map[string]string `json:"labels,omitempty"`
}

// Deployment represents a deployed instance.
type Deployment struct {
	// ID is the unique identifier for this deployment.
	ID string `json:"id"`

	// Name is the deployment name.
	Name string `json:"name"`

	// Namespace is the deployment namespace.
	Namespace string `json:"namespace"`

	// PackageID references the source package.
	PackageID string `json:"packageId"`

	// Status is the current deployment status.
	Status DeploymentStatus `json:"status"`

	// Version is the current deployed version/revision.
	Version int `json:"version"`

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time `json:"updatedAt"`

	// Extensions provides vendor-specific metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// DeploymentUpdate represents an update to an existing deployment.
type DeploymentUpdate struct {
	// Values provides new configuration values.
	Values map[string]interface{} `json:"values,omitempty"`

	// PackageID updates the package reference (for upgrades).
	PackageID string `json:"packageId,omitempty"`

	// GitRevision updates the Git revision (for GitOps).
	GitRevision string `json:"gitRevision,omitempty"`
}

// DeploymentStatusDetail provides detailed status information.
type DeploymentStatusDetail struct {
	// DeploymentID references the deployment.
	DeploymentID string `json:"deploymentId"`

	// Status is the current status.
	Status DeploymentStatus `json:"status"`

	// Message provides a human-readable status message.
	Message string `json:"message,omitempty"`

	// Progress indicates completion percentage (0-100).
	Progress int `json:"progress"`

	// UpdatedAt is the last status update timestamp.
	UpdatedAt time.Time `json:"updatedAt"`

	// Conditions provides detailed status conditions.
	Conditions []StatusCondition `json:"conditions,omitempty"`

	// Extensions provides vendor-specific status details.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// StatusCondition represents a specific status condition.
type StatusCondition struct {
	// Type is the condition type (e.g., "Ready", "Available").
	Type string `json:"type"`

	// Status indicates if the condition is true.
	Status bool `json:"status"`

	// Reason provides a programmatic identifier for the condition.
	Reason string `json:"reason,omitempty"`

	// Message provides human-readable details.
	Message string `json:"message,omitempty"`

	// LastTransitionTime is when the condition last changed.
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

// LogOptions provides options for fetching deployment logs.
type LogOptions struct {
	// TailLines limits the number of log lines to return.
	TailLines int `json:"tailLines,omitempty"`

	// Follow indicates whether to stream logs.
	Follow bool `json:"follow,omitempty"`

	// Container specifies a specific container (for multi-container pods).
	Container string `json:"container,omitempty"`
}

// AdapterMetadata provides basic metadata about a DMS adapter.
type AdapterMetadata interface {
	// Name returns the unique name of this adapter (e.g., "helm", "argocd", "flux").
	Name() string

	// Version returns the version of the backend system this adapter supports.
	Version() string

	// Capabilities returns the list of DMS capabilities this adapter supports.
	Capabilities() []Capability
}

// PackageManager provides deployment package management operations.
type PackageManager interface {
	// ListDeploymentPackages retrieves all packages matching the provided filter.
	ListDeploymentPackages(ctx context.Context, filter *Filter) ([]*DeploymentPackage, error)

	// GetDeploymentPackage retrieves a specific package by ID.
	GetDeploymentPackage(ctx context.Context, id string) (*DeploymentPackage, error)

	// UploadDeploymentPackage uploads a new deployment package.
	UploadDeploymentPackage(ctx context.Context, pkg *DeploymentPackageUpload) (*DeploymentPackage, error)

	// DeleteDeploymentPackage deletes a package by ID.
	DeleteDeploymentPackage(ctx context.Context, id string) error
}

// DeploymentManager provides deployment lifecycle operations.
type DeploymentManager interface {
	// ListDeployments retrieves all deployments matching the provided filter.
	ListDeployments(ctx context.Context, filter *Filter) ([]*Deployment, error)

	// GetDeployment retrieves a specific deployment by ID.
	GetDeployment(ctx context.Context, id string) (*Deployment, error)

	// CreateDeployment creates a new deployment.
	CreateDeployment(ctx context.Context, deployment *DeploymentRequest) (*Deployment, error)

	// UpdateDeployment updates an existing deployment.
	UpdateDeployment(ctx context.Context, id string, update *DeploymentUpdate) (*Deployment, error)

	// DeleteDeployment deletes a deployment by ID.
	DeleteDeployment(ctx context.Context, id string) error
}

// DeploymentOperator provides advanced deployment operations.
type DeploymentOperator interface {
	// ScaleDeployment scales a deployment to the specified number of replicas.
	// Returns an error if scaling is not supported.
	ScaleDeployment(ctx context.Context, id string, replicas int) error

	// RollbackDeployment rolls back a deployment to a previous revision.
	// Returns an error if rollback is not supported.
	RollbackDeployment(ctx context.Context, id string, revision int) error

	// GetDeploymentStatus retrieves detailed status for a deployment.
	GetDeploymentStatus(ctx context.Context, id string) (*DeploymentStatusDetail, error)

	// GetDeploymentLogs retrieves logs from a deployment.
	GetDeploymentLogs(ctx context.Context, id string, opts *LogOptions) ([]byte, error)
}

// CapabilityChecker provides capability checks for DMS adapters.
type CapabilityChecker interface {
	// SupportsRollback returns true if the adapter supports rollback.
	SupportsRollback() bool

	// SupportsScaling returns true if the adapter supports scaling.
	SupportsScaling() bool

	// SupportsGitOps returns true if the adapter supports GitOps workflows.
	SupportsGitOps() bool
}

// AdapterLifecycle provides lifecycle management operations.
type AdapterLifecycle interface {
	// Health performs a health check on the backend system.
	Health(ctx context.Context) error

	// Close cleanly shuts down the adapter and releases resources.
	Close() error
}

// Adapter defines the interface that all DMS backend implementations must provide.
// Implementations include Helm, ArgoCD, Flux, ONAP-LCM, OSM-LCM, etc.
// This interface is composed of smaller, focused interfaces to reduce complexity.
type Adapter interface {
	AdapterMetadata
	PackageManager
	DeploymentManager
	DeploymentOperator
	CapabilityChecker
	AdapterLifecycle
}
