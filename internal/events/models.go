// Package events provides the event notification system for O2-IMS subscriptions.
// It handles event generation, queuing, filtering, and webhook delivery with retry logic.
package events

import (
	"time"

	"github.com/piwi3910/netweave/internal/models"
)

// Event represents an O2-IMS resource change event.
// Events are generated when resources, resource pools, or resource types change.
type Event struct {
	// ID is the unique event identifier (UUID v4)
	ID string `json:"id"`

	// Type is the event type (ResourceCreated, ResourceUpdated, etc.)
	Type models.EventType `json:"type"`

	// ResourceType identifies the type of resource that changed
	ResourceType ResourceType `json:"resourceType"`

	// ResourceID is the ID of the resource that changed
	ResourceID string `json:"resourceId"`

	// ResourcePoolID is the resource pool ID (if applicable)
	ResourcePoolID string `json:"resourcePoolId,omitempty"`

	// ResourceTypeID is the resource type ID (if applicable)
	ResourceTypeID string `json:"resourceTypeId,omitempty"`

	// Resource contains the full resource data
	Resource interface{} `json:"resource"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// TenantID is the tenant that owns this resource (for multi-tenancy)
	TenantID string `json:"tenantId,omitempty"`

	// Labels contains resource labels for filtering
	Labels map[string]string `json:"labels,omitempty"`

	// Extensions contains additional event-specific fields
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ResourceType identifies the type of resource involved in an event.
type ResourceType string

const (
	// ResourceTypeResource represents an O2-IMS Resource (compute node).
	ResourceTypeResource ResourceType = "resource"

	// ResourceTypeResourcePool represents an O2-IMS Resource Pool.
	ResourceTypeResourcePool ResourceType = "resourcePool"

	// ResourceTypeResourceType represents an O2-IMS Resource Type.
	ResourceTypeResourceType ResourceType = "resourceType"

	// ResourceTypeDeploymentManager represents an O2-IMS Deployment Manager.
	ResourceTypeDeploymentManager ResourceType = "deploymentManager"
)

// String returns the string representation of the ResourceType.
func (r ResourceType) String() string {
	return string(r)
}

// DeliveryStatus represents the status of a notification delivery attempt.
type DeliveryStatus string

const (
	// DeliveryStatusPending indicates the notification is queued for delivery.
	DeliveryStatusPending DeliveryStatus = "pending"

	// DeliveryStatusDelivering indicates delivery is in progress.
	DeliveryStatusDelivering DeliveryStatus = "delivering"

	// DeliveryStatusDelivered indicates successful delivery.
	DeliveryStatusDelivered DeliveryStatus = "delivered"

	// DeliveryStatusFailed indicates delivery failed after all retries.
	DeliveryStatusFailed DeliveryStatus = "failed"

	// DeliveryStatusRetrying indicates delivery is being retried.
	DeliveryStatusRetrying DeliveryStatus = "retrying"
)

// String returns the string representation of the DeliveryStatus.
func (d DeliveryStatus) String() string {
	return string(d)
}

// NotificationDelivery tracks the delivery status of an event notification to a subscriber.
type NotificationDelivery struct {
	// ID is the unique delivery tracking identifier
	ID string `json:"id"`

	// EventID is the event being delivered
	EventID string `json:"eventId"`

	// SubscriptionID is the subscription receiving the notification
	SubscriptionID string `json:"subscriptionId"`

	// CallbackURL is the webhook endpoint
	CallbackURL string `json:"callbackUrl"`

	// Status is the current delivery status
	Status DeliveryStatus `json:"status"`

	// Attempts is the number of delivery attempts made
	Attempts int `json:"attempts"`

	// MaxAttempts is the maximum number of delivery attempts
	MaxAttempts int `json:"maxAttempts"`

	// LastAttemptAt is the timestamp of the last delivery attempt
	LastAttemptAt time.Time `json:"lastAttemptAt,omitempty"`

	// NextAttemptAt is the scheduled time for the next retry
	NextAttemptAt time.Time `json:"nextAttemptAt,omitempty"`

	// LastError contains the error message from the last failed attempt
	LastError string `json:"lastError,omitempty"`

	// HTTPStatusCode is the HTTP status code from the last attempt
	HTTPStatusCode int `json:"httpStatusCode,omitempty"`

	// ResponseTime is the response time of the last attempt in milliseconds
	ResponseTime int64 `json:"responseTime,omitempty"`

	// CreatedAt is when the delivery was created
	CreatedAt time.Time `json:"createdAt"`

	// CompletedAt is when the delivery was completed (success or failure)
	CompletedAt time.Time `json:"completedAt,omitempty"`
}
