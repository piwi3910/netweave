package gcp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// CreateSubscription creates a new event subscription.
// GCP adapter uses polling-based subscriptions since Pub/Sub
// integration would require additional GCP infrastructure setup.
func (a *GCPAdapter) CreateSubscription(_ context.Context, sub *adapter.Subscription) (result *adapter.Subscription, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "CreateSubscription", start, err) }()

	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	// Validate callback URL
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Generate subscription ID if not provided
	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = fmt.Sprintf("gcp-sub-%s", uuid.New().String())
	}

	// Create the subscription
	newSub := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Store in memory
	a.subscriptionsMu.Lock()
	a.subscriptions[subscriptionID] = newSub
	count := len(a.subscriptions)
	a.subscriptionsMu.Unlock()

	// Update subscription count metric
	adapter.UpdateSubscriptionCount("gcp", count)

	a.logger.Info("created subscription",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", sub.Callback))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *GCPAdapter) GetSubscription(_ context.Context, id string) (sub *adapter.Subscription, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "GetSubscription", start, err) }()

	a.logger.Debug("GetSubscription called",
		zap.String("id", id))

	a.subscriptionsMu.RLock()
	sub, exists := a.subscriptions[id]
	a.subscriptionsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}

	return sub, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *GCPAdapter) DeleteSubscription(_ context.Context, id string) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("gcp", "DeleteSubscription", start, err) }()

	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	a.subscriptionsMu.Lock()
	if _, exists := a.subscriptions[id]; !exists {
		a.subscriptionsMu.Unlock()
		return fmt.Errorf("subscription not found: %s", id)
	}

	delete(a.subscriptions, id)
	count := len(a.subscriptions)
	a.subscriptionsMu.Unlock()

	// Update subscription count metric
	adapter.UpdateSubscriptionCount("gcp", count)

	a.logger.Info("deleted subscription",
		zap.String("subscriptionId", id))

	return nil
}

// ListSubscriptions returns all active subscriptions.
// This is a helper method not part of the Adapter interface.
func (a *GCPAdapter) ListSubscriptions() []*adapter.Subscription {
	a.subscriptionsMu.RLock()
	defer a.subscriptionsMu.RUnlock()

	subs := make([]*adapter.Subscription, 0, len(a.subscriptions))
	for _, sub := range a.subscriptions {
		subs = append(subs, sub)
	}

	return subs
}
