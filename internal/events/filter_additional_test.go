package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/events"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
)

func TestSubscriptionFilter_MatchSubscriptions_StoreError(t *testing.T) {
	store := &mockStore{
		listErr: errors.New("store unavailable"),
	}
	logger := zaptest.NewLogger(t)
	filter := events.NewSubscriptionFilter(store, logger)

	ctx := context.Background()
	event := &events.Event{
		ID:           "error-event",
		Type:         models.EventTypeResourceCreated,
		ResourceType: events.ResourceTypeResource,
		ResourceID:   "resource-1",
		Timestamp:    time.Now().UTC(),
	}

	_, err := filter.MatchSubscriptions(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list subscriptions")
}

func TestSubscriptionFilter_MatchSubscriptions_TenantFiltering(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("filters by tenant when event has tenant ID", func(t *testing.T) {
		tenantSubs := []*storage.Subscription{
			{
				ID:       "sub-tenant",
				Callback: "https://example.com/cb",
				TenantID: "tenant-1",
				Filter:   storage.SubscriptionFilter{},
			},
		}

		store := &mockStore{
			subscriptions: []*storage.Subscription{
				{ID: "sub-all-1", Callback: "https://example.com/cb1", Filter: storage.SubscriptionFilter{}},
				{ID: "sub-all-2", Callback: "https://example.com/cb2", Filter: storage.SubscriptionFilter{}},
			},
			listByTenantFn: func(_ context.Context, tenantID string) ([]*storage.Subscription, error) {
				if tenantID == "tenant-1" {
					return tenantSubs, nil
				}
				return nil, nil
			},
		}
		filter := events.NewSubscriptionFilter(store, logger)

		ctx := context.Background()
		event := &events.Event{
			ID:           "tenant-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			TenantID:     "tenant-1",
			Timestamp:    time.Now().UTC(),
		}

		matched, err := filter.MatchSubscriptions(ctx, event)
		require.NoError(t, err)
		// Should only get the tenant-specific subscription
		assert.Len(t, matched, 1)
		assert.Equal(t, "sub-tenant", matched[0].ID)
	})

	t.Run("falls back to all subscriptions when tenant list fails", func(t *testing.T) {
		store := &mockStore{
			subscriptions: []*storage.Subscription{
				{ID: "sub-all", Callback: "https://example.com/cb", Filter: storage.SubscriptionFilter{}},
			},
			listByTenantFn: func(_ context.Context, _ string) ([]*storage.Subscription, error) {
				return nil, errors.New("tenant listing failed")
			},
		}
		filter := events.NewSubscriptionFilter(store, logger)

		ctx := context.Background()
		event := &events.Event{
			ID:           "fallback-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			TenantID:     "tenant-1",
			Timestamp:    time.Now().UTC(),
		}

		matched, err := filter.MatchSubscriptions(ctx, event)
		require.NoError(t, err)
		// Falls back to all subscriptions
		assert.Len(t, matched, 1)
		assert.Equal(t, "sub-all", matched[0].ID)
	})
}

func TestSubscriptionFilter_MatchSubscriptions_ResourceTypeIDMismatch(t *testing.T) {
	logger := zaptest.NewLogger(t)

	store := &mockStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-type-mismatch",
				Callback: "https://example.com/cb",
				Filter: storage.SubscriptionFilter{
					ResourceTypeID: "storage-node",
				},
			},
		},
	}
	filter := events.NewSubscriptionFilter(store, logger)

	ctx := context.Background()
	event := &events.Event{
		ID:             "type-mismatch-event",
		Type:           models.EventTypeResourceCreated,
		ResourceType:   events.ResourceTypeResource,
		ResourceID:     "node-1",
		ResourceTypeID: "compute-node",
		Timestamp:      time.Now().UTC(),
	}

	matched, err := filter.MatchSubscriptions(ctx, event)
	require.NoError(t, err)
	assert.Empty(t, matched)
}

func TestSubscriptionFilter_MatchSubscriptions_ResourcePoolIDMismatch(t *testing.T) {
	logger := zaptest.NewLogger(t)

	store := &mockStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-pool-mismatch",
				Callback: "https://example.com/cb",
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-2",
				},
			},
		},
	}
	filter := events.NewSubscriptionFilter(store, logger)

	ctx := context.Background()
	event := &events.Event{
		ID:             "pool-mismatch-event",
		Type:           models.EventTypeResourceCreated,
		ResourceType:   events.ResourceTypeResource,
		ResourceID:     "node-1",
		ResourcePoolID: "pool-1",
		Timestamp:      time.Now().UTC(),
	}

	matched, err := filter.MatchSubscriptions(ctx, event)
	require.NoError(t, err)
	assert.Empty(t, matched)
}

func TestSubscriptionFilter_MatchSubscriptions_ResourceIDFilter(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name          string
		event         *events.Event
		subscriptions []*storage.Subscription
		wantCount     int
	}{
		{
			name: "resource ID filter matches",
			event: &events.Event{
				ID:             "event-1",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   events.ResourceTypeResource,
				ResourceID:     "specific-resource",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/cb",
					Filter: storage.SubscriptionFilter{
						ResourceID: "specific-resource",
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "resource ID filter does not match",
			event: &events.Event{
				ID:             "event-2",
				Type:           models.EventTypeResourceCreated,
				ResourceType:   events.ResourceTypeResource,
				ResourceID:     "resource-A",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-1",
					Callback: "https://example.com/cb",
					Filter: storage.SubscriptionFilter{
						ResourceID: "resource-B",
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "combined filters with resource ID",
			event: &events.Event{
				ID:             "event-3",
				Type:           models.EventTypeResourceUpdated,
				ResourceType:   events.ResourceTypeResource,
				ResourceID:     "node-1",
				ResourcePoolID: "pool-1",
				ResourceTypeID: "compute-node",
				Timestamp:      time.Now().UTC(),
			},
			subscriptions: []*storage.Subscription{
				{
					ID:       "sub-match",
					Callback: "https://example.com/cb1",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-1",
						ResourceTypeID: "compute-node",
						ResourceID:     "node-1",
					},
				},
				{
					ID:       "sub-nomatch",
					Callback: "https://example.com/cb2",
					Filter: storage.SubscriptionFilter{
						ResourcePoolID: "pool-1",
						ResourceTypeID: "compute-node",
						ResourceID:     "node-2",
					},
				},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{subscriptions: tt.subscriptions}
			filter := events.NewSubscriptionFilter(store, logger)

			ctx := context.Background()
			matched, err := filter.MatchSubscriptions(ctx, tt.event)
			require.NoError(t, err)
			assert.Len(t, matched, tt.wantCount)
		})
	}
}
