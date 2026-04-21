// Package controllers defines event types produced by the subscription event
// pipeline and consumed by internal/workers and internal/handlers.
//
// The Kubernetes-informer based subscription controller that originally lived
// in this package has been removed; event production is handled by backend
// adapters via the tmforum event listener (see internal/workers).
package controllers

import "time"

const (
	// EventStreamKey is the Redis Stream key for webhook events.
	EventStreamKey = "o2ims:events"
)

// EventType represents the type of resource event.
type EventType string

const (
	// EventTypeCreated indicates a resource was created.
	EventTypeCreated EventType = "Created"

	// EventTypeUpdated indicates a resource was updated.
	EventTypeUpdated EventType = "Updated"

	// EventTypeDeleted indicates a resource was deleted.
	EventTypeDeleted EventType = "Deleted"
)

// ResourceEvent represents a resource change event.
type ResourceEvent struct {
	// SubscriptionID is the ID of the subscription receiving this event.
	SubscriptionID string `json:"subscriptionId"`

	// EventType is the type of event (Created, Updated, Deleted).
	EventType string `json:"notificationEventType"`

	// ObjectRef is the O2-IMS API path to the resource.
	ObjectRef string `json:"objectRef"`

	// ResourceTypeID identifies the resource type.
	ResourceTypeID string `json:"resourceTypeId"`

	// ResourcePoolID identifies the resource pool (if applicable).
	ResourcePoolID string `json:"resourcePoolId,omitempty"`

	// GlobalResourceID is the global identifier for the resource.
	GlobalResourceID string `json:"globalResourceId"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// NotificationID is a unique identifier for this notification.
	NotificationID string `json:"notificationId"`

	// CallbackURL is the webhook endpoint to deliver to.
	CallbackURL string `json:"callbackUrl"`

	// TenantID is the tenant that owns the subscription (for multi-tenancy filtering).
	TenantID string `json:"tenantId,omitempty"`

	// BackendID is the backend/adapter instance that generated this event.
	BackendID string `json:"backendId,omitempty"`
}
