package starlingx_test

import (
	"context"
	"sync"

	"github.com/piwi3910/netweave/internal/storage"
)

// mockStore is a simple in-memory store for testing
type mockStore struct {
	mu            sync.RWMutex
	subscriptions map[string]*storage.Subscription
}

func newMockStore() *mockStore {
	return &mockStore{
		subscriptions: make(map[string]*storage.Subscription),
	}
}

func (m *mockStore) Create(_ context.Context, sub *storage.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[sub.ID]; exists {
		return storage.ErrSubscriptionExists
	}

	// Deep copy to avoid mutation issues
	subCopy := *sub
	m.subscriptions[sub.ID] = &subCopy

	return nil
}

func (m *mockStore) Get(_ context.Context, id string) (*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, exists := m.subscriptions[id]
	if !exists {
		return nil, storage.ErrSubscriptionNotFound
	}

	// Deep copy to avoid mutation issues
	subCopy := *sub
	return &subCopy, nil
}

func (m *mockStore) Update(_ context.Context, sub *storage.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[sub.ID]; !exists {
		return storage.ErrSubscriptionNotFound
	}

	// Deep copy to avoid mutation issues
	subCopy := *sub
	m.subscriptions[sub.ID] = &subCopy

	return nil
}

func (m *mockStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[id]; !exists {
		return storage.ErrSubscriptionNotFound
	}

	delete(m.subscriptions, id)
	return nil
}

func (m *mockStore) List(_ context.Context) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		// Deep copy
		subCopy := *sub
		result = append(result, &subCopy)
	}

	return result, nil
}

func (m *mockStore) ListByResourcePool(_ context.Context, resourcePoolID string) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		if sub.Filter.ResourcePoolID == resourcePoolID {
			subCopy := *sub
			result = append(result, &subCopy)
		}
	}

	return result, nil
}

func (m *mockStore) ListByResourceType(_ context.Context, resourceTypeID string) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		if sub.Filter.ResourceTypeID == resourceTypeID {
			subCopy := *sub
			result = append(result, &subCopy)
		}
	}

	return result, nil
}

func (m *mockStore) ListByTenant(_ context.Context, tenantID string) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		if sub.TenantID == tenantID {
			subCopy := *sub
			result = append(result, &subCopy)
		}
	}

	return result, nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}
