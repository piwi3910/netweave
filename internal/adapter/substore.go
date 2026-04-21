// Package adapter provides shared abstractions for cloud backend adapters.
//
// This file contains two subscription store implementations that eliminate
// duplication across adapters (AWS, Azure, GCP, OpenStack, VMware, DTIAS):
//
//   - InMemorySubscriptionStore: a thread-safe, in-memory subscription store
//     used by adapters whose underlying cloud provider lacks native event
//     notifications (polling-based subscriptions).
//
//   - StorageBackedSubscriptionStore: a sibling store that delegates to the
//     storage.Store interface so subscriptions survive adapter restarts.
//     Adapters opt-in via configuration; the default remains in-memory.
package adapter

import (
	"context"
	"fmt"
	"sync"
)

// InMemorySubscriptionStore is a thread-safe in-memory subscription store.
// It is the default backing for adapters that lack native event subscriptions
// and implement polling-based notifications.
//
// All methods respect context cancellation: if the caller's context is
// already canceled, the method returns immediately with ctx.Err() wrapped.
//
// The zero value is not usable; construct instances with
// NewInMemorySubscriptionStore.
type InMemorySubscriptionStore struct {
	mu   sync.RWMutex
	subs map[string]*Subscription
}

// NewInMemorySubscriptionStore returns a ready-to-use in-memory store.
func NewInMemorySubscriptionStore() *InMemorySubscriptionStore {
	return &InMemorySubscriptionStore{
		subs: make(map[string]*Subscription),
	}
}

// Create inserts a new subscription.
// Returns ErrSubscriptionExists if the subscription ID is already present.
// Returns an error if sub is nil or sub.SubscriptionID is empty.
func (s *InMemorySubscriptionStore) Create(ctx context.Context, sub *Subscription) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("subscription is nil")
	}
	if sub.SubscriptionID == "" {
		return fmt.Errorf("subscription ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subs[sub.SubscriptionID]; exists {
		return fmt.Errorf("%w: %s", ErrSubscriptionExists, sub.SubscriptionID)
	}

	s.subs[sub.SubscriptionID] = sub
	return nil
}

// Get retrieves a subscription by ID.
// Returns ErrSubscriptionNotFound if no subscription with the given ID exists.
func (s *InMemorySubscriptionStore) Get(ctx context.Context, id string) (*Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, exists := s.subs[id]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrSubscriptionNotFound, id)
	}
	return sub, nil
}

// Update replaces an existing subscription.
// Returns ErrSubscriptionNotFound if no subscription with the given ID exists.
// The caller is responsible for preserving the SubscriptionID in sub.
func (s *InMemorySubscriptionStore) Update(ctx context.Context, id string, sub *Subscription) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("subscription is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subs[id]; !exists {
		return fmt.Errorf("%w: %s", ErrSubscriptionNotFound, id)
	}
	s.subs[id] = sub
	return nil
}

// Delete removes a subscription by ID.
// Returns ErrSubscriptionNotFound if no subscription with the given ID exists.
func (s *InMemorySubscriptionStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subs[id]; !exists {
		return fmt.Errorf("%w: %s", ErrSubscriptionNotFound, id)
	}
	delete(s.subs, id)
	return nil
}

// List returns all subscriptions matching filter. A nil filter returns every
// subscription. A non-nil filter applies the same semantics as
// SubscriptionFilter comparisons (non-empty fields must match the
// corresponding subscription field).
func (s *InMemorySubscriptionStore) List(ctx context.Context, filter *SubscriptionFilter) ([]*Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		if subscriptionMatchesFilter(sub, filter) {
			result = append(result, sub)
		}
	}
	return result, nil
}

// Len returns the current number of subscriptions.
// Safe for concurrent use.
func (s *InMemorySubscriptionStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subs)
}

// Reset removes all subscriptions. Intended for use by Close() on adapters
// that share this store.
func (s *InMemorySubscriptionStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = make(map[string]*Subscription)
}

// Snapshot returns a slice copy of all subscriptions. Unlike List, it does not
// take a context and is intended for callers that expose legacy non-ctx
// signatures (e.g., adapter-local helpers) where threading a context would be
// a breaking change.
func (s *InMemorySubscriptionStore) Snapshot() []*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		result = append(result, sub)
	}
	return result
}

// RawMap returns the underlying map.
//
// This exists solely to preserve legacy test hooks (ExportSubscriptions) that
// want to seed or inspect subscriptions directly. Callers must not access it
// concurrently with adapter operations.
func (s *InMemorySubscriptionStore) RawMap() map[string]*Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subs
}

// subscriptionMatchesFilter reports whether sub's filter matches the supplied
// filter criteria. Non-empty fields in filter must match sub.Filter.
func subscriptionMatchesFilter(sub *Subscription, filter *SubscriptionFilter) bool {
	if filter == nil {
		return true
	}
	sf := sub.Filter
	if filter.ResourcePoolID != "" {
		if sf == nil || sf.ResourcePoolID != filter.ResourcePoolID {
			return false
		}
	}
	if filter.ResourceTypeID != "" {
		if sf == nil || sf.ResourceTypeID != filter.ResourceTypeID {
			return false
		}
	}
	if filter.ResourceID != "" {
		if sf == nil || sf.ResourceID != filter.ResourceID {
			return false
		}
	}
	return true
}

// StorageStore describes the subset of storage.Store operations required by
// StorageBackedSubscriptionStore. It is declared here (instead of importing
// internal/storage directly) to avoid an import cycle: the storage package is
// free to depend on this adapter package via its concrete types if needed, and
// callers wire in any implementation that satisfies this surface.
type StorageStore interface {
	// Create persists a subscription. Implementations return a sentinel
	// "already exists" error when the ID is a duplicate.
	Create(ctx context.Context, id, consumerSubscriptionID, callback string, filter *SubscriptionFilter) error

	// Get retrieves a subscription by ID. Implementations return a sentinel
	// "not found" error when absent.
	Get(ctx context.Context, id string) (*Subscription, error)

	// Update overwrites an existing subscription.
	Update(ctx context.Context, id, consumerSubscriptionID, callback string, filter *SubscriptionFilter) error

	// Delete removes a subscription by ID.
	Delete(ctx context.Context, id string) error

	// List returns all subscriptions, optionally filtered.
	List(ctx context.Context, filter *SubscriptionFilter) ([]*Subscription, error)
}

// StorageBackedSubscriptionStore delegates subscription CRUD to a durable
// storage backend (e.g., Redis, Postgres). It provides the same method set as
// InMemorySubscriptionStore so adapters can select between in-memory and
// persistent storage via configuration.
//
// Adapters must explicitly opt-in; the default remains the in-memory store.
type StorageBackedSubscriptionStore struct {
	backend StorageStore
}

// NewStorageBackedSubscriptionStore returns a store that delegates to
// backend. backend must not be nil.
func NewStorageBackedSubscriptionStore(backend StorageStore) *StorageBackedSubscriptionStore {
	return &StorageBackedSubscriptionStore{backend: backend}
}

// Create persists sub via the backend.
func (s *StorageBackedSubscriptionStore) Create(ctx context.Context, sub *Subscription) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("subscription is nil")
	}
	if sub.SubscriptionID == "" {
		return fmt.Errorf("subscription ID is required")
	}
	return s.backend.Create(ctx, sub.SubscriptionID, sub.ConsumerSubscriptionID, sub.Callback, sub.Filter)
}

// Get retrieves a subscription by ID from the backend.
func (s *StorageBackedSubscriptionStore) Get(ctx context.Context, id string) (*Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}
	return s.backend.Get(ctx, id)
}

// Update replaces the subscription identified by id.
func (s *StorageBackedSubscriptionStore) Update(ctx context.Context, id string, sub *Subscription) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("subscription is nil")
	}
	return s.backend.Update(ctx, id, sub.ConsumerSubscriptionID, sub.Callback, sub.Filter)
}

// Delete removes the subscription identified by id.
func (s *StorageBackedSubscriptionStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
	}
	return s.backend.Delete(ctx, id)
}

// List returns all subscriptions matching filter.
func (s *StorageBackedSubscriptionStore) List(
	ctx context.Context,
	filter *SubscriptionFilter,
) ([]*Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}
	return s.backend.List(ctx, filter)
}
