package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// CreateSubscription creates a new event subscription.
// AWS adapter uses polling-based subscriptions since CloudWatch Events
// integration would require additional AWS infrastructure setup.
func (a *Adapter) CreateSubscription(
	_ context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "CreateSubscription", start, err) }()

	a.Logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	// Validate callback URL
	if sub.Callback == "" {
		err = fmt.Errorf("callback URL is required")
		return nil, err
	}

	// Generate subscription ID if not provided
	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = fmt.Sprintf("aws-sub-%s", uuid.New().String())
	}

	// Create the subscription
	newSub := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Store in memory
	a.SubscriptionsMu.Lock()
	a.Subscriptions[subscriptionID] = newSub
	subscriptionCount := len(a.Subscriptions)
	a.SubscriptionsMu.Unlock()

	// Update subscription count metric
	adapter.UpdateSubscriptionCount("aws", subscriptionCount)

	a.Logger.Info("created subscription",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", sub.Callback))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(_ context.Context, id string) (*adapter.Subscription, error) {
	var (
		sub *adapter.Subscription
		err error
	)
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "GetSubscription", start, err) }()

	a.Logger.Debug("GetSubscription called",
		zap.String("id", id))

	a.SubscriptionsMu.RLock()
	sub, exists := a.Subscriptions[id]
	a.SubscriptionsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	return sub, nil
}

// UpdateSubscription updates an existing subscription.
func (a *Adapter) UpdateSubscription(
	_ context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	var (
		result *adapter.Subscription
		err    error
	)
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "UpdateSubscription", start, err) }()

	a.Logger.Debug("UpdateSubscription called",
		zap.String("id", id),
		zap.String("callback", sub.Callback))

	// Validate callback URL
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	a.SubscriptionsMu.Lock()
	defer a.SubscriptionsMu.Unlock()

	// Check if subscription exists
	existing, exists := a.Subscriptions[id]
	if !exists {
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	// Create updated subscription preserving the ID
	result = &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Store updated subscription
	a.Subscriptions[id] = result

	a.Logger.Info("updated subscription",
		zap.String("subscriptionId", id),
		zap.String("oldCallback", existing.Callback),
		zap.String("newCallback", sub.Callback))

	return result, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(_ context.Context, id string) error {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "DeleteSubscription", start, err) }()

	a.Logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	a.SubscriptionsMu.Lock()
	if _, exists := a.Subscriptions[id]; !exists {
		a.SubscriptionsMu.Unlock()
		return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	delete(a.Subscriptions, id)
	subscriptionCount := len(a.Subscriptions)
	a.SubscriptionsMu.Unlock()

	// Update subscription count metric
	adapter.UpdateSubscriptionCount("aws", subscriptionCount)

	a.Logger.Info("deleted subscription",
		zap.String("subscriptionId", id))

	return nil
}

// ListSubscriptions returns all active subscriptions.
// This is a helper method not part of the Adapter interface.
func (a *Adapter) ListSubscriptions() []*adapter.Subscription {
	a.SubscriptionsMu.RLock()
	defer a.SubscriptionsMu.RUnlock()

	subs := make([]*adapter.Subscription, 0, len(a.Subscriptions))
	for _, sub := range a.Subscriptions {
		subs = append(subs, sub)
	}

	return subs
}
