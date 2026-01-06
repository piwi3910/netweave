// Package storage provides abstractions for subscription storage.
// It supports Redis-backed storage with automatic failover and caching.
package storage

import (
	"encoding/json"
	"time"
)

// Subscription represents an O2-IMS subscription.
// Subscribers receive webhook notifications when watched resources change.
//
// Example:
//
//	sub := &Subscription{
//	    ID:                     "550e8400-e29b-41d4-a716-446655440000",
//	    Callback:               "https://smo.example.com/notify",
//	    ConsumerSubscriptionID: "smo-sub-123",
//	    Filter: SubscriptionFilter{
//	        ResourcePoolID: "pool-abc",
//	    },
//	}
type Subscription struct {
	// ID is the unique subscription identifier (UUID v4)
	ID string `json:"subscriptionId"`

	// Callback is the webhook URL for notifications
	Callback string `json:"callback"`

	// ConsumerSubscriptionID is the client-provided subscription ID
	ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty"`

	// Filter defines which resource changes trigger notifications
	Filter SubscriptionFilter `json:"filter,omitempty"`

	// CreatedAt is the subscription creation timestamp
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the last update timestamp
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// SubscriptionFilter defines resource filtering criteria for subscriptions.
// Multiple filter fields are combined with AND logic.
type SubscriptionFilter struct {
	// ResourcePoolID filters events by resource pool
	ResourcePoolID string `json:"resourcePoolId,omitempty"`

	// ResourceTypeID filters events by resource type
	ResourceTypeID string `json:"resourceTypeId,omitempty"`

	// ResourceID filters events for a specific resource
	ResourceID string `json:"resourceId,omitempty"`
}

// MarshalBinary implements encoding.BinaryMarshaler for Redis storage.
func (s *Subscription) MarshalBinary() ([]byte, error) {
	return json.Marshal(s)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler for Redis storage.
func (s *Subscription) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, s)
}

// MatchesFilter checks if a resource matches the subscription filter.
// All non-empty filter fields must match (AND logic).
func (f *SubscriptionFilter) MatchesFilter(resourcePoolID, resourceTypeID, resourceID string) bool {
	if f.ResourcePoolID != "" && f.ResourcePoolID != resourcePoolID {
		return false
	}
	if f.ResourceTypeID != "" && f.ResourceTypeID != resourceTypeID {
		return false
	}
	if f.ResourceID != "" && f.ResourceID != resourceID {
		return false
	}
	return true
}
