package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
)

var errNotFound = errors.New("subscription not found")

type mockStore struct {
	subscriptions  []*storage.Subscription
	listErr        error
	listByTenantFn func(ctx context.Context, tenantID string) ([]*storage.Subscription, error)
}

func (m *mockStore) Create(_ context.Context, sub *storage.Subscription) error {
	return nil
}

func (m *mockStore) Get(_ context.Context, id string) (*storage.Subscription, error) {
	return nil, errNotFound
}

func (m *mockStore) Update(_ context.Context, sub *storage.Subscription) error {
	return nil
}

func (m *mockStore) Delete(_ context.Context, id string) error {
	return nil
}

func (m *mockStore) List(_ context.Context) ([]*storage.Subscription, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.subscriptions, nil
}

func (m *mockStore) ListByResourcePool(_ context.Context, resourcePoolID string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockStore) ListByResourceType(_ context.Context, resourceTypeID string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockStore) ListByTenant(ctx context.Context, tenantID string) ([]*storage.Subscription, error) {
	if m.listByTenantFn != nil {
		return m.listByTenantFn(ctx, tenantID)
	}
	return m.subscriptions, nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

func TestNewSubscriptionFilter(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		store := &mockStore{}
		logger := zaptest.NewLogger(t)

		filter := NewSubscriptionFilter(store, logger)
		assert.NotNil(t, filter)
	})

	t.Run("nil store panics", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		assert.Panics(t, func() {
			NewSubscriptionFilter(nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		store := &mockStore{}

		assert.Panics(t, func() {
			NewSubscriptionFilter(store, nil)
		})
	})
}

func TestSubscriptionFilterMatchSubscriptions(t *testing.T) {
	tests := []struct {
		name          string
		event         *Event
		subscriptions []*storage.Subscription
		wantCount     int
	}{
		{
			name: "no subscriptions",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{},
			wantCount:     0,
		},
		{
			name: "match all filters",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/callback",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-1",
						ResourceTypeID: "compute-node",
						ResourceID:     "node-1",
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "match resource pool only",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/callback",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-1",
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "match resource type only",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/callback",
					Filter: storage.SubscriptionFilter{
						ResourceTypeID: "compute-node",
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "no match - wrong pool",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/callback",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-2",
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "match multiple subscriptions",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/callback1",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-1",
					},
				},
				{
					ID:       "sub-2",
					Callback: "https://example.com/callback2",
					Filter: storage.SubscriptionFilter{
						ResourceTypeID: "compute-node",
					},
				},
				{
					ID:       "sub-3",
					Callback: "https://example.com/callback3",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-2", // Won't match
					},
				},
			},
			wantCount: 2,
		},
		{
			name: "empty filters match all",
			event: &Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/callback",
					Filter:   storage.SubscriptionFilter{},
				},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{
				subscriptions: tt.subscriptions,
			}
			logger := zaptest.NewLogger(t)
			filter := NewSubscriptionFilter(store, logger)

			ctx := context.Background()
			matched, err := filter.MatchSubscriptions(ctx, tt.event)

			require.NoError(t, err)
			assert.Len(t, matched, tt.wantCount)
		})
	}
}
