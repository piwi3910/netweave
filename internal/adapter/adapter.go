// Package adapter provides the abstraction layer for different backend implementations.
// It defines the Adapter interface that all backend systems must implement to provide
// O2-IMS functionality through the netweave gateway.
package adapter

import (
	"context"
	"errors"
)

// Sentinel errors for adapter operations.
var (
	// ErrSubscriptionNotFound is returned when a subscription does not exist.
	ErrSubscriptionNotFound = errors.New("subscription not found")

	// ErrResourcePoolNotFound is returned when a resource pool does not exist.
	ErrResourcePoolNotFound = errors.New("resource pool not found")

	// ErrResourceNotFound is returned when a resource does not exist.
	ErrResourceNotFound = errors.New("resource not found")

	// ErrResourceTypeNotFound is returned when a resource type does not exist.
	ErrResourceTypeNotFound = errors.New("resource type not found")
)

// Capability represents a feature that an adapter supports.
// Capabilities are used during adapter selection to ensure the chosen
// adapter can fulfill the requirements of a specific O2-IMS operation.
type Capability string

const (
	// CapabilityResourcePools indicates support for Resource Pool management (CRUD).
	CapabilityResourcePools Capability = "resource-pools"

	// CapabilityResources indicates support for Resource management (CRUD).
	CapabilityResources Capability = "resources"

	// CapabilityResourceTypes indicates support for Resource Type discovery (read-only).
	CapabilityResourceTypes Capability = "resource-types"

	// CapabilityDeploymentManagers indicates support for Deployment Manager metadata (read-only).
	CapabilityDeploymentManagers Capability = "deployment-managers"

	// CapabilitySubscriptions indicates support for event subscriptions and webhooks.
	CapabilitySubscriptions Capability = "subscriptions"

	// CapabilityMetrics indicates support for metrics and monitoring data.
	CapabilityMetrics Capability = "metrics"

	// CapabilityHealthChecks indicates support for health status reporting.
	CapabilityHealthChecks Capability = "health-checks"
)

// Filter provides criteria for filtering O2-IMS resources.
// Filters are used in List operations to narrow down results based on
// resource attributes, labels, location, and custom extensions.
type Filter struct {
	// ResourcePoolID filters resources by their parent resource pool.
	ResourcePoolID string

	// ResourceTypeID filters resources by their type.
	ResourceTypeID string

	// Location filters resources by geographic or logical location.
	Location string

	// Labels provides key-value label matching for resources.
	Labels map[string]string

	// Extensions allows filtering based on vendor-specific extension fields.
	Extensions map[string]interface{}

	// Limit specifies the maximum number of results to return.
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// DeploymentManager represents O2-IMS Deployment Manager metadata.
// A Deployment Manager identifies a specific O-Cloud deployment and provides
// metadata about its capabilities and configuration.
type DeploymentManager struct {
	// DeploymentManagerID is the unique identifier for this deployment manager.
	DeploymentManagerID string `json:"deploymentManagerId"`

	// Name is the human-readable name of the deployment manager.
	Name string `json:"name"`

	// Description provides additional context about the deployment manager.
	Description string `json:"description,omitempty"`

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string `json:"oCloudId"`

	// ServiceURI is the API endpoint for O2-IMS services.
	ServiceURI string `json:"serviceUri"`

	// SupportedLocations lists geographic locations supported by this deployment.
	SupportedLocations []string `json:"supportedLocations,omitempty"`

	// Capabilities lists the features supported by this deployment manager.
	Capabilities []string `json:"capabilities,omitempty"`

	// Extensions provides vendor-specific additional metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ResourcePool represents an O2-IMS Resource Pool.
// A Resource Pool is a logical grouping of infrastructure resources
// (typically nodes/machines) with similar characteristics.
type ResourcePool struct {
	// ResourcePoolID is the unique identifier for this resource pool.
	ResourcePoolID string `json:"resourcePoolId"`

	// Name is the human-readable name of the resource pool.
	Name string `json:"name"`

	// Description provides additional context about the pool's purpose.
	Description string `json:"description,omitempty"`

	// Location identifies the geographic or logical location of the pool.
	Location string `json:"location,omitempty"`

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string `json:"oCloudId"`

	// GlobalLocationID provides geographic coordinates (e.g., "geo:37.7749,-122.4194").
	GlobalLocationID string `json:"globalLocationId,omitempty"`

	// Extensions provides vendor-specific additional metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// Resource represents an O2-IMS Resource (typically a compute node).
// Resources are individual infrastructure units (physical or virtual servers)
// that provide compute, storage, or networking capacity.
type Resource struct {
	// ResourceID is the unique identifier for this resource.
	ResourceID string `json:"resourceId"`

	// ResourceTypeID identifies the type/class of this resource.
	ResourceTypeID string `json:"resourceTypeId"`

	// ResourcePoolID identifies the parent resource pool.
	ResourcePoolID string `json:"resourcePoolId,omitempty"`

	// GlobalAssetID provides a globally unique identifier (URN format).
	GlobalAssetID string `json:"globalAssetId,omitempty"`

	// Description provides additional context about the resource.
	Description string `json:"description,omitempty"`

	// Extensions provides vendor-specific additional metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ResourceType represents an O2-IMS Resource Type.
// A Resource Type describes a category or class of resources with
// specific capabilities and characteristics.
type ResourceType struct {
	// ResourceTypeID is the unique identifier for this resource type.
	ResourceTypeID string `json:"resourceTypeId"`

	// Name is the human-readable name of the resource type.
	Name string `json:"name"`

	// Description provides additional context about the resource type.
	Description string `json:"description,omitempty"`

	// Vendor identifies the vendor providing this resource type.
	Vendor string `json:"vendor,omitempty"`

	// Model identifies the specific model or SKU.
	Model string `json:"model,omitempty"`

	// Version identifies the hardware/software version.
	Version string `json:"version,omitempty"`

	// ResourceClass categorizes the resource (e.g., "compute", "storage", "network").
	ResourceClass string `json:"resourceClass,omitempty"`

	// ResourceKind indicates physical vs. virtual ("physical" or "virtual").
	ResourceKind string `json:"resourceKind,omitempty"`

	// Extensions provides vendor-specific additional metadata.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// Subscription represents an O2-IMS event subscription.
// Subscriptions enable real-time notifications when infrastructure
// resources change state (created, updated, deleted).
type Subscription struct {
	// SubscriptionID is the unique identifier for this subscription.
	SubscriptionID string `json:"subscriptionId"`

	// Callback is the webhook URL where notifications will be sent.
	Callback string `json:"callback"`

	// ConsumerSubscriptionID is an optional client-provided identifier.
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty"`

	// Filter specifies criteria for which events trigger notifications.
	Filter *SubscriptionFilter `json:"filter,omitempty"`
}

// SubscriptionFilter defines criteria for event filtering.
type SubscriptionFilter struct {
	// ResourcePoolID filters events to a specific resource pool.
	ResourcePoolID string `json:"resourcePoolId,omitempty"`

	// ResourceTypeID filters events to a specific resource type.
	ResourceTypeID string `json:"resourceTypeId,omitempty"`

	// ResourceID filters events to a specific resource.
	ResourceID string `json:"resourceId,omitempty"`
}

// Adapter defines the interface that all backend implementations must provide.
// Implementations include Kubernetes, Dell DTIAS, AWS, OpenStack, etc.
// Each adapter translates O2-IMS operations to backend-specific API calls.
type Adapter interface {
	// Metadata methods

	// Name returns the unique name of this adapter (e.g., "kubernetes", "dtias", "aws").
	Name() string

	// Version returns the version of the backend system this adapter supports.
	Version() string

	// Capabilities returns the list of O2-IMS capabilities this adapter supports.
	Capabilities() []Capability

	// Deployment Manager operations

	// GetDeploymentManager retrieves metadata about the deployment manager.
	// Returns the deployment manager or an error if not found.
	GetDeploymentManager(ctx context.Context, id string) (*DeploymentManager, error)

	// Resource Pool operations

	// ListResourcePools retrieves all resource pools matching the provided filter.
	// The filter parameter can be nil to retrieve all pools.
	// Returns a slice of resource pools or an error.
	ListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error)

	// GetResourcePool retrieves a specific resource pool by ID.
	// Returns the resource pool or an error if not found.
	GetResourcePool(ctx context.Context, id string) (*ResourcePool, error)

	// CreateResourcePool creates a new resource pool.
	// Returns the created resource pool with server-assigned fields populated.
	CreateResourcePool(ctx context.Context, pool *ResourcePool) (*ResourcePool, error)

	// UpdateResourcePool updates an existing resource pool.
	// Returns the updated resource pool or an error if the pool doesn't exist.
	UpdateResourcePool(ctx context.Context, id string, pool *ResourcePool) (*ResourcePool, error)

	// DeleteResourcePool deletes a resource pool by ID.
	// Returns an error if the pool doesn't exist or cannot be deleted.
	DeleteResourcePool(ctx context.Context, id string) error

	// Resource operations

	// ListResources retrieves all resources matching the provided filter.
	// The filter parameter can be nil to retrieve all resources.
	// Returns a slice of resources or an error.
	ListResources(ctx context.Context, filter *Filter) ([]*Resource, error)

	// GetResource retrieves a specific resource by ID.
	// Returns the resource or an error if not found.
	GetResource(ctx context.Context, id string) (*Resource, error)

	// CreateResource creates a new resource (e.g., provision a new node).
	// Returns the created resource with server-assigned fields populated.
	CreateResource(ctx context.Context, resource *Resource) (*Resource, error)

	// DeleteResource deletes a resource by ID (e.g., deprovision a node).
	// Returns an error if the resource doesn't exist or cannot be deleted.
	DeleteResource(ctx context.Context, id string) error

	// Resource Type operations

	// ListResourceTypes retrieves all resource types matching the provided filter.
	// The filter parameter can be nil to retrieve all types.
	// Returns a slice of resource types or an error.
	ListResourceTypes(ctx context.Context, filter *Filter) ([]*ResourceType, error)

	// GetResourceType retrieves a specific resource type by ID.
	// Returns the resource type or an error if not found.
	GetResourceType(ctx context.Context, id string) (*ResourceType, error)

	// Subscription operations

	// CreateSubscription creates a new event subscription.
	// Returns the created subscription with server-assigned fields populated.
	CreateSubscription(ctx context.Context, sub *Subscription) (*Subscription, error)

	// GetSubscription retrieves a specific subscription by ID.
	// Returns the subscription or an error if not found.
	GetSubscription(ctx context.Context, id string) (*Subscription, error)

	// UpdateSubscription updates an existing subscription.
	// Returns the updated subscription or an error if not found.
	UpdateSubscription(ctx context.Context, id string, sub *Subscription) (*Subscription, error)

	// DeleteSubscription deletes a subscription by ID.
	// Returns an error if the subscription doesn't exist.
	DeleteSubscription(ctx context.Context, id string) error

	// Lifecycle methods

	// Health performs a health check on the backend system.
	// Returns nil if healthy, or an error describing the health issue.
	Health(ctx context.Context) error

	// Close cleanly shuts down the adapter and releases resources.
	// Returns an error if shutdown fails.
	Close() error
}
