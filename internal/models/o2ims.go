// Package models contains the O2-IMS data models for the netweave gateway.
// These models represent O-RAN O2 Interface Management Service (IMS) resources
// as defined in the O-RAN.WG6.O2IMS-INTERFACE specification.
package models

import (
	"time"
)

// DeploymentManager represents an O2-IMS Deployment Manager.
// A Deployment Manager is the top-level entity representing a cloud infrastructure
// deployment (e.g., Kubernetes cluster, OpenStack cloud).
//
// Example:
//
//	dm := &DeploymentManager{
//	    DeploymentManagerID: "ocloud-k8s-1",
//	    Name:                "Production Kubernetes Cluster",
//	    Description:         "Main production cluster in US East",
//	    OCloudID:            "ocloud-1",
//	    ServiceURI:          "https://api.o2ims.example.com/o2ims/v1",
//	}
type DeploymentManager struct {
	// DeploymentManagerID is the unique identifier for this deployment manager.
	DeploymentManagerID string `json:"deploymentManagerId" yaml:"deploymentManagerId"`

	// Name is the human-readable name of the deployment manager.
	Name string `json:"name" yaml:"name"`

	// Description provides additional details about the deployment manager.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// OCloudID is the identifier of the O-Cloud this deployment manager belongs to.
	OCloudID string `json:"oCloudId" yaml:"oCloudId"`

	// ServiceURI is the base URI for accessing this deployment manager's API.
	ServiceURI string `json:"serviceUri" yaml:"serviceUri"`

	// SupportedLocations lists the geographic locations supported by this deployment manager.
	SupportedLocations []string `json:"supportedLocations,omitempty" yaml:"supportedLocations,omitempty"`

	// Capabilities lists the features and operations supported by this deployment manager.
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`

	// Capacity describes the total capacity of this deployment manager.
	Capacity *Capacity `json:"capacity,omitempty" yaml:"capacity,omitempty"`

	// Extensions contains additional backend-specific or custom fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// Capacity represents the total and available capacity of a deployment manager or resource pool.
type Capacity struct {
	// TotalCPU is the total CPU capacity in cores.
	TotalCPU int64 `json:"totalCpu,omitempty" yaml:"totalCpu,omitempty"`

	// TotalMemoryMB is the total memory capacity in megabytes.
	TotalMemoryMB int64 `json:"totalMemoryMb,omitempty" yaml:"totalMemoryMb,omitempty"`

	// TotalStorageGB is the total storage capacity in gigabytes.
	TotalStorageGB int64 `json:"totalStorageGb,omitempty" yaml:"totalStorageGb,omitempty"`

	// AvailableCPU is the available CPU capacity in cores.
	AvailableCPU int64 `json:"availableCpu,omitempty" yaml:"availableCpu,omitempty"`

	// AvailableMemoryMB is the available memory capacity in megabytes.
	AvailableMemoryMB int64 `json:"availableMemoryMb,omitempty" yaml:"availableMemoryMb,omitempty"`

	// AvailableStorageGB is the available storage capacity in gigabytes.
	AvailableStorageGB int64 `json:"availableStorageGb,omitempty" yaml:"availableStorageGb,omitempty"`
}

// ResourcePool represents an O2-IMS Resource Pool.
// A Resource Pool is a logical grouping of compute resources (nodes/machines)
// with similar characteristics. In Kubernetes, this maps to a MachineSet.
//
// Example:
//
//	pool := &ResourcePool{
//	    ResourcePoolID:    "pool-compute-high-mem",
//	    Name:              "High Memory Compute Pool",
//	    Description:       "Nodes with 128GB+ RAM",
//	    Location:          "us-east-1a",
//	    OCloudID:          "ocloud-1",
//	    GlobalLocationID:  "geo:37.7749,-122.4194",
//	}
type ResourcePool struct {
	// ResourcePoolID is the unique identifier for this resource pool.
	ResourcePoolID string `json:"resourcePoolId" yaml:"resourcePoolId"`

	// Name is the human-readable name of the resource pool.
	Name string `json:"name" yaml:"name"`

	// Description provides additional details about the resource pool.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Location is the physical or logical location of the resource pool (e.g., availability zone).
	Location string `json:"location,omitempty" yaml:"location,omitempty"`

	// OCloudID is the identifier of the O-Cloud this resource pool belongs to.
	OCloudID string `json:"oCloudId" yaml:"oCloudId"`

	// GlobalLocationID is an optional geographic identifier (e.g., "geo:37.7749,-122.4194").
	GlobalLocationID string `json:"globalLocationId,omitempty" yaml:"globalLocationId,omitempty"`

	// Extensions contains additional backend-specific or custom fields.
	// Common extensions include:
	//   - machineType: VM or machine instance type
	//   - replicas: number of resources in the pool
	//   - volumeSize: disk size in GB
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// Resource represents an O2-IMS Resource.
// A Resource is an individual compute node or machine within a Resource Pool.
// In Kubernetes, this maps to a Node (for runtime state) or Machine (for lifecycle).
//
// Example:
//
//	resource := &Resource{
//	    ResourceID:     "node-worker-1a-abc123",
//	    ResourceTypeID: "compute-node",
//	    ResourcePoolID: "pool-compute-high-mem",
//	    GlobalAssetID:  "urn:o-ran:node:abc123",
//	    Description:    "Compute node for RAN workloads",
//	}
type Resource struct {
	// ResourceID is the unique identifier for this resource.
	ResourceID string `json:"resourceId" yaml:"resourceId"`

	// ResourceTypeID is the identifier of the resource type.
	ResourceTypeID string `json:"resourceTypeId" yaml:"resourceTypeId"`

	// ResourcePoolID is the identifier of the resource pool this resource belongs to.
	ResourcePoolID string `json:"resourcePoolId,omitempty" yaml:"resourcePoolId,omitempty"`

	// GlobalAssetID is a globally unique identifier for this resource (e.g., URN).
	GlobalAssetID string `json:"globalAssetId,omitempty" yaml:"globalAssetId,omitempty"`

	// Description provides additional details about the resource.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Extensions contains additional backend-specific or custom fields.
	// Common extensions include:
	//   - nodeName: Kubernetes node name
	//   - status: resource status (Ready, NotReady, etc.)
	//   - cpu: CPU capacity
	//   - memory: memory capacity
	//   - labels: Kubernetes labels
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// ResourceType represents an O2-IMS Resource Type.
// A Resource Type defines a category of resources with similar characteristics
// (e.g., compute nodes, storage volumes, network interfaces).
// In Kubernetes, this is aggregated from Node capacities and StorageClasses.
//
// Example:
//
//	rt := &ResourceType{
//	    ResourceTypeID: "compute-node-highmem",
//	    Name:           "High Memory Compute Node",
//	    Description:    "Compute node with 64GB+ RAM",
//	    Vendor:         "AWS",
//	    Model:          "m5.4xlarge",
//	    ResourceClass:  "compute",
//	    ResourceKind:   "physical",
//	}
type ResourceType struct {
	// ResourceTypeID is the unique identifier for this resource type.
	ResourceTypeID string `json:"resourceTypeId" yaml:"resourceTypeId"`

	// Name is the human-readable name of the resource type.
	Name string `json:"name" yaml:"name"`

	// Description provides additional details about the resource type.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Vendor is the vendor/manufacturer of the resource type.
	Vendor string `json:"vendor,omitempty" yaml:"vendor,omitempty"`

	// Model is the specific model identifier for the resource type.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Version is the version of the resource type.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// ResourceClass categorizes the resource (e.g., "compute", "storage", "network").
	ResourceClass string `json:"resourceClass,omitempty" yaml:"resourceClass,omitempty"`

	// ResourceKind specifies the kind of resource (e.g., "physical", "virtual", "logical").
	ResourceKind string `json:"resourceKind,omitempty" yaml:"resourceKind,omitempty"`

	// Extensions contains additional backend-specific or custom fields.
	// Common extensions include:
	//   - cpu: CPU specifications
	//   - memory: memory capacity
	//   - storage: storage capacity
	//   - network: network specifications
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// Subscription represents an O2-IMS Subscription.
// A Subscription allows consumers (e.g., SMO systems) to receive webhook notifications
// when resources matching specific criteria are created, updated, or deleted.
//
// Example:
//
//	sub := &Subscription{
//	    SubscriptionID:         "550e8400-e29b-41d4-a716-446655440000",
//	    Callback:               "https://smo.example.com/notifications",
//	    ConsumerSubscriptionID: "smo-sub-123",
//	    Filter: &SubscriptionFilter{
//	        ResourcePoolID: []string{"pool-compute-high-mem"},
//	        ResourceTypeID: []string{"compute-node"},
//	    },
//	}
type Subscription struct {
	// SubscriptionID is the unique identifier for this subscription.
	SubscriptionID string `json:"subscriptionId" yaml:"subscriptionId"`

	// Callback is the webhook URL where notifications will be sent.
	Callback string `json:"callback" yaml:"callback"`

	// ConsumerSubscriptionID is an optional client-provided identifier for correlation.
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty" yaml:"consumerSubscriptionId,omitempty"`

	// Filter specifies which events should trigger notifications.
	Filter *SubscriptionFilter `json:"filter,omitempty" yaml:"filter,omitempty"`

	// EventTypes lists the types of events this subscription is interested in.
	// Valid values: "ResourceCreated", "ResourceUpdated", "ResourceDeleted",
	// "ResourcePoolCreated", "ResourcePoolUpdated", "ResourcePoolDeleted"
	EventTypes []string `json:"eventTypes,omitempty" yaml:"eventTypes,omitempty"`

	// CreatedAt is the timestamp when the subscription was created.
	CreatedAt time.Time `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`

	// UpdatedAt is the timestamp when the subscription was last updated.
	UpdatedAt time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`

	// Extensions contains additional backend-specific or custom fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// SubscriptionFilter defines filtering criteria for subscription notifications.
// Multiple filters are combined with AND logic (all must match).
type SubscriptionFilter struct {
	// ResourcePoolID filters events to specific resource pools.
	// Empty means all resource pools.
	ResourcePoolID []string `json:"resourcePoolId,omitempty" yaml:"resourcePoolId,omitempty"`

	// ResourceTypeID filters events to specific resource types.
	// Empty means all resource types.
	ResourceTypeID []string `json:"resourceTypeId,omitempty" yaml:"resourceTypeId,omitempty"`

	// ResourceID filters events to specific resources.
	// Empty means all resources.
	ResourceID []string `json:"resourceId,omitempty" yaml:"resourceId,omitempty"`

	// Labels filters events based on resource labels.
	// All specified labels must match (AND logic).
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Extensions contains additional filter criteria.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// Notification represents an O2-IMS event notification sent to subscribers.
// This is sent via HTTP POST to the subscriber's callback URL.
//
// Example:
//
//	notification := &Notification{
//	    SubscriptionID:         "550e8400-e29b-41d4-a716-446655440000",
//	    ConsumerSubscriptionID: "smo-sub-123",
//	    EventType:              "ResourceCreated",
//	    Resource:               resource,
//	    Timestamp:              time.Now(),
//	}
type Notification struct {
	// SubscriptionID is the ID of the subscription that triggered this notification.
	SubscriptionID string `json:"subscriptionId" yaml:"subscriptionId"`

	// ConsumerSubscriptionID is the client-provided subscription identifier.
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty" yaml:"consumerSubscriptionId,omitempty"`

	// EventType describes the type of event (e.g., "ResourceCreated").
	EventType string `json:"eventType" yaml:"eventType"`

	// Resource contains the resource that triggered the event.
	// This can be a Resource, ResourcePool, or other O2-IMS object.
	Resource interface{} `json:"resource" yaml:"resource"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// Extensions contains additional event-specific fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// EventType defines the types of events that can trigger notifications.
type EventType string

const (
	// EventTypeResourceCreated is fired when a new Resource is created.
	EventTypeResourceCreated EventType = "ResourceCreated"

	// EventTypeResourceUpdated is fired when a Resource is updated.
	EventTypeResourceUpdated EventType = "ResourceUpdated"

	// EventTypeResourceDeleted is fired when a Resource is deleted.
	EventTypeResourceDeleted EventType = "ResourceDeleted"

	// EventTypeResourcePoolCreated is fired when a new ResourcePool is created.
	EventTypeResourcePoolCreated EventType = "ResourcePoolCreated"

	// EventTypeResourcePoolUpdated is fired when a ResourcePool is updated.
	EventTypeResourcePoolUpdated EventType = "ResourcePoolUpdated"

	// EventTypeResourcePoolDeleted is fired when a ResourcePool is deleted.
	EventTypeResourcePoolDeleted EventType = "ResourcePoolDeleted"

	// EventTypeResourceTypeCreated is fired when a new ResourceType is detected.
	EventTypeResourceTypeCreated EventType = "ResourceTypeCreated"

	// EventTypeResourceTypeUpdated is fired when a ResourceType changes.
	EventTypeResourceTypeUpdated EventType = "ResourceTypeUpdated"

	// EventTypeResourceTypeDeleted is fired when a ResourceType is removed.
	EventTypeResourceTypeDeleted EventType = "ResourceTypeDeleted"
)

// String returns the string representation of the EventType.
func (e EventType) String() string {
	return string(e)
}

// IsValid checks if the EventType is a valid O2-IMS event type.
func (e EventType) IsValid() bool {
	switch e {
	case EventTypeResourceCreated, EventTypeResourceUpdated, EventTypeResourceDeleted,
		EventTypeResourcePoolCreated, EventTypeResourcePoolUpdated, EventTypeResourcePoolDeleted,
		EventTypeResourceTypeCreated, EventTypeResourceTypeUpdated, EventTypeResourceTypeDeleted:
		return true
	default:
		return false
	}
}
