package events

import (
	"context"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/storage"
)

// SubscriptionFilter implements the Filter interface using subscription criteria.
type SubscriptionFilter struct {
	store  storage.Store
	logger *zap.Logger
}

// NewSubscriptionFilter creates a new SubscriptionFilter instance.
func NewSubscriptionFilter(store storage.Store, logger *zap.Logger) *SubscriptionFilter {
	if store == nil {
		panic("storage cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &SubscriptionFilter{
		store:  store,
		logger: logger,
	}
}

// MatchSubscriptions finds all subscriptions that should receive the event.
// Filtering is based on subscription criteria:
// - Resource pool ID
// - Resource type ID
// - Resource ID
// - Tenant ID (for multi-tenancy)
// All non-empty filter fields must match (AND logic).
func (f *SubscriptionFilter) MatchSubscriptions(ctx context.Context, event *Event) ([]*storage.Subscription, error) {
	// Get all subscriptions
	allSubscriptions, err := f.store.List(ctx)
	if err != nil {
		return nil, err
	}

	// Filter subscriptions by tenant if applicable
	if event.TenantID != "" {
		tenantSubs, err := f.store.ListByTenant(ctx, event.TenantID)
		if err != nil {
			f.logger.Warn("failed to filter subscriptions by tenant",
				zap.Error(err),
				zap.String("tenant_id", event.TenantID),
			)
		} else {
			allSubscriptions = tenantSubs
		}
	}

	// Filter subscriptions based on criteria
	matched := make([]*storage.Subscription, 0)
	for _, sub := range allSubscriptions {
		if f.matchesSubscription(event, sub) {
			matched = append(matched, sub)
		}
	}

	// Record metrics
	RecordSubscriptionsMatched(string(event.Type), len(matched))

	f.logger.Debug("matched subscriptions for event",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.Int("total_subscriptions", len(allSubscriptions)),
		zap.Int("matched_subscriptions", len(matched)),
	)

	return matched, nil
}

// matchesSubscription checks if an event matches a subscription's filter criteria.
func (f *SubscriptionFilter) matchesSubscription(event *Event, sub *storage.Subscription) bool {
	filter := sub.Filter

	// Check resource pool ID filter
	if filter.ResourcePoolID != "" && filter.ResourcePoolID != event.ResourcePoolID {
		return false
	}

	// Check resource type ID filter
	if filter.ResourceTypeID != "" && filter.ResourceTypeID != event.ResourceTypeID {
		return false
	}

	// Check resource ID filter
	if filter.ResourceID != "" && filter.ResourceID != event.ResourceID {
		return false
	}

	// All filters matched
	return true
}
