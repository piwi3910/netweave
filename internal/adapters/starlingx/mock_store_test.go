package starlingx

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

func (m *mockStore) Create(ctx context.Context, sub *storage.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[sub.ID]; exists {
		return storage.ErrSubscriptionExists
	}

	// Deep copy to avoid mutation issues
	copy := *sub
	m.subscriptions[sub.ID] = &copy

	return nil
}

func (m *mockStore) Get(ctx context.Context, id string) (*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, exists := m.subscriptions[id]
	if !exists {
		return nil, storage.ErrSubscriptionNotFound
	}

	// Deep copy to avoid mutation issues
	copy := *sub
	return &copy, nil
}

func (m *mockStore) Update(ctx context.Context, sub *storage.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[sub.ID]; !exists {
		return storage.ErrSubscriptionNotFound
	}

	// Deep copy to avoid mutation issues
	copy := *sub
	m.subscriptions[sub.ID] = &copy

	return nil
}

func (m *mockStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[id]; !exists {
		return storage.ErrSubscriptionNotFound
	}

	delete(m.subscriptions, id)
	return nil
}

func (m *mockStore) List(ctx context.Context) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		// Deep copy
		copy := *sub
		result = append(result, &copy)
	}

	return result, nil
}

func (m *mockStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		if sub.Filter.ResourcePoolID == resourcePoolID {
			copy := *sub
			result = append(result, &copy)
		}
	}

	return result, nil
}

func (m *mockStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		if sub.Filter.ResourceTypeID == resourceTypeID {
			copy := *sub
			result = append(result, &copy)
		}
	}

	return result, nil
}

func (m *mockStore) ListByTenant(ctx context.Context, tenantID string) ([]*storage.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*storage.Subscription
	for _, sub := range m.subscriptions {
		if sub.TenantID == tenantID {
			copy := *sub
			result = append(result, &copy)
		}
	}

	return result, nil
}

func (m *mockStore) Ping(ctx context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}
