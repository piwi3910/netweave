// Package storage provides storage interfaces and implementations for O2-DMS subscriptions.
package storage

import (
	"context"
	"errors"

	"github.com/piwi3910/netweave/internal/dms/models"
)

// ErrSubscriptionNotFound is returned when a subscription is not found.
var ErrSubscriptionNotFound = errors.New("subscription not found")

// ErrSubscriptionExists is returned when a subscription already exists.
var ErrSubscriptionExists = errors.New("subscription already exists")

// Store defines the interface for DMS subscription storage.
type Store interface {
	// Create creates a new subscription.
	// Returns ErrSubscriptionExists if a subscription with the same ID exists.
	Create(ctx context.Context, sub *models.DMSSubscription) error

	// Get retrieves a subscription by ID.
	// Returns ErrSubscriptionNotFound if the subscription doesn't exist.
	Get(ctx context.Context, id string) (*models.DMSSubscription, error)

	// List retrieves all subscriptions.
	List(ctx context.Context) ([]*models.DMSSubscription, error)

	// Update updates an existing subscription.
	// Returns ErrSubscriptionNotFound if the subscription doesn't exist.
	Update(ctx context.Context, sub *models.DMSSubscription) error

	// Delete deletes a subscription by ID.
	// Returns ErrSubscriptionNotFound if the subscription doesn't exist.
	Delete(ctx context.Context, id string) error

	// Ping checks if the storage is healthy.
	Ping(ctx context.Context) error

	// Close closes the storage connection.
	Close() error
}
