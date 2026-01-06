package storage

import (
	"context"
	"errors"
)

// Common sentinel errors for storage operations.
var (
	// ErrSubscriptionNotFound is returned when a subscription does not exist.
	ErrSubscriptionNotFound = errors.New("subscription not found")

	// ErrSubscriptionExists is returned when attempting to create a duplicate subscription.
	ErrSubscriptionExists = errors.New("subscription already exists")

	// ErrInvalidCallback is returned when a callback URL is invalid.
	ErrInvalidCallback = errors.New("invalid callback URL")

	// ErrInvalidID is returned when a subscription ID is invalid.
	ErrInvalidID = errors.New("invalid subscription ID")

	// ErrStorageUnavailable is returned when the storage backend is unavailable.
	ErrStorageUnavailable = errors.New("storage backend unavailable")
)

// Store defines the interface for subscription storage operations.
// Implementations must be safe for concurrent use.
//
// Example usage:
//
//	store := NewRedisStore(cfg)
//	defer store.Close()
//
//	sub := &Subscription{
//	    ID:       uuid.New().String(),
//	    Callback: "https://smo.example.com/notify",
//	}
//
//	err := store.Create(ctx, sub)
//	if err != nil {
//	    log.Error("failed to create subscription", "error", err)
//	}
type Store interface {
	// Create creates a new subscription in the store.
	// Returns ErrSubscriptionExists if a subscription with the same ID already exists.
	// Returns ErrInvalidCallback if the callback URL is invalid.
	// Returns ErrInvalidID if the subscription ID is empty or invalid.
	// The context is used for timeout and cancellation.
	Create(ctx context.Context, sub *Subscription) error

	// Get retrieves a subscription by ID.
	// Returns ErrSubscriptionNotFound if the subscription does not exist.
	// The context is used for timeout and cancellation.
	Get(ctx context.Context, id string) (*Subscription, error)

	// Update updates an existing subscription.
	// Returns ErrSubscriptionNotFound if the subscription does not exist.
	// Returns ErrInvalidCallback if the callback URL is invalid.
	// The context is used for timeout and cancellation.
	Update(ctx context.Context, sub *Subscription) error

	// Delete deletes a subscription by ID.
	// Returns ErrSubscriptionNotFound if the subscription does not exist.
	// The context is used for timeout and cancellation.
	Delete(ctx context.Context, id string) error

	// List retrieves all subscriptions.
	// Returns an empty slice if no subscriptions exist.
	// The context is used for timeout and cancellation.
	List(ctx context.Context) ([]*Subscription, error)

	// ListByResourcePool retrieves subscriptions filtered by resource pool ID.
	// Returns an empty slice if no matching subscriptions exist.
	// The context is used for timeout and cancellation.
	ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*Subscription, error)

	// ListByResourceType retrieves subscriptions filtered by resource type ID.
	// Returns an empty slice if no matching subscriptions exist.
	// The context is used for timeout and cancellation.
	ListByResourceType(ctx context.Context, resourceTypeID string) ([]*Subscription, error)

	// ListByTenant retrieves subscriptions filtered by tenant ID.
	// Returns an empty slice if no matching subscriptions exist.
	// The context is used for timeout and cancellation.
	ListByTenant(ctx context.Context, tenantID string) ([]*Subscription, error)

	// Close closes the storage connection and releases resources.
	// After calling Close, the store should not be used.
	Close() error

	// Ping checks if the storage backend is available.
	// Returns ErrStorageUnavailable if the backend cannot be reached.
	// The context is used for timeout and cancellation.
	Ping(ctx context.Context) error
}
