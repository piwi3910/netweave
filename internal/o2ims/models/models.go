// Package models defines O2-IMS data structures.
package models

import "time"

// DeploymentManager represents an O2-IMS Deployment Manager.
// A Deployment Manager represents a Kubernetes cluster or infrastructure domain.
type DeploymentManager struct {
	DeploymentManagerID string                 `json:"deploymentManagerId"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description,omitempty"`
	OCloudID            string                 `json:"oCloudId"`
	ServiceURI          string                 `json:"serviceUri"`
	SupportedLocations  []string               `json:"supportedLocations,omitempty"`
	Capabilities        []string               `json:"capabilities,omitempty"`
	Extensions          map[string]interface{} `json:"extensions,omitempty"`
}

// ResourcePool represents an O2-IMS Resource Pool.
// A Resource Pool is a collection of infrastructure resources (e.g., NodePool, MachineSet).
type ResourcePool struct {
	ResourcePoolID string                 `json:"resourcePoolId"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	Location       string                 `json:"location,omitempty"`
	OCloudID       string                 `json:"oCloudId"`
	GlobalAssetID  string                 `json:"globalAssetId,omitempty"`
	Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

// Resource represents an O2-IMS Resource.
// A Resource is a single infrastructure unit (e.g., Node, Machine).
type Resource struct {
	ResourceID     string                 `json:"resourceId"`
	ResourceTypeID string                 `json:"resourceTypeId"`
	ResourcePoolID string                 `json:"resourcePoolId,omitempty"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	GlobalAssetID  string                 `json:"globalAssetId,omitempty"`
	ParentID       string                 `json:"parentId,omitempty"`
	Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

// ResourceType represents an O2-IMS Resource Type.
// A Resource Type defines the class of infrastructure resource (e.g., compute-node, storage).
type ResourceType struct {
	ResourceTypeID string                 `json:"resourceTypeId"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	Vendor         string                 `json:"vendor,omitempty"`
	Model          string                 `json:"model,omitempty"`
	Version        string                 `json:"version,omitempty"`
	Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

// Subscription represents an O2-IMS Subscription.
// Subscriptions enable webhook notifications when resources change.
type Subscription struct {
	SubscriptionID         string             `json:"subscriptionId"`
	Callback               string             `json:"callback"`
	ConsumerSubscriptionID string             `json:"consumerSubscriptionId,omitempty"`
	Filter                 SubscriptionFilter `json:"filter,omitempty"`
	CreatedAt              time.Time          `json:"createdAt,omitempty"`
}

// SubscriptionFilter defines filtering criteria for subscription notifications.
type SubscriptionFilter struct {
	ResourcePoolID []string `json:"resourcePoolId,omitempty"`
	ResourceTypeID []string `json:"resourceTypeId,omitempty"`
	ResourceID     []string `json:"resourceId,omitempty"`
}

// ListResponse represents a paginated list response.
type ListResponse struct {
	Items      interface{} `json:"items"`
	TotalCount int         `json:"totalCount,omitempty"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}
