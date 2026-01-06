// Package storage provides storage interfaces and implementations for O2-DMS subscriptions.
package storage

import (
	"context"
	"sync"

	"github.com/piwi3910/netweave/internal/dms/models"
)

// MemoryStore is an in-memory implementation of the Store interface.
// It is suitable for testing and single-instance deployments.
type MemoryStore struct {
	mu            sync.RWMutex
	subscriptions map[string]*models.DMSSubscription
}

// NewMemoryStore creates a new in-memory subscription store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		subscriptions: make(map[string]*models.DMSSubscription),
	}
}

// Create creates a new subscription.
func (s *MemoryStore) Create(_ context.Context, sub *models.DMSSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscriptions[sub.SubscriptionID]; exists {
		return ErrSubscriptionExists
	}

	// Store a copy to prevent external modification.
	subCopy := *sub
	s.subscriptions[sub.SubscriptionID] = &subCopy

	return nil
}

// Get retrieves a subscription by ID.
func (s *MemoryStore) Get(_ context.Context, id string) (*models.DMSSubscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, exists := s.subscriptions[id]
	if !exists {
		return nil, ErrSubscriptionNotFound
	}

	// Return a copy to prevent external modification.
	subCopy := *sub
	return &subCopy, nil
}

// List retrieves all subscriptions.
func (s *MemoryStore) List(_ context.Context) ([]*models.DMSSubscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subs := make([]*models.DMSSubscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		// Return copies to prevent external modification.
		subCopy := *sub
		subs = append(subs, &subCopy)
	}

	return subs, nil
}

// Update updates an existing subscription.
func (s *MemoryStore) Update(_ context.Context, sub *models.DMSSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscriptions[sub.SubscriptionID]; !exists {
		return ErrSubscriptionNotFound
	}

	// Store a copy to prevent external modification.
	subCopy := *sub
	s.subscriptions[sub.SubscriptionID] = &subCopy

	return nil
}

// Delete deletes a subscription by ID.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscriptions[id]; !exists {
		return ErrSubscriptionNotFound
	}

	delete(s.subscriptions, id)
	return nil
}

// Ping checks if the storage is healthy.
func (s *MemoryStore) Ping(_ context.Context) error {
	return nil
}

// Close closes the storage connection.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscriptions = make(map[string]*models.DMSSubscription)
	return nil
}
