package vmware

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// CreateSubscription creates a new event subscription.
// VMware adapter uses polling-based subscriptions since vSphere Event
// integration would require additional configuration.
func (a *Adapter) CreateSubscription(
	_ context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "CreateSubscription", start, err) }()

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
		subscriptionID = fmt.Sprintf("vmware-sub-%s", uuid.New().String())
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
	count := len(a.Subscriptions)
	a.SubscriptionsMu.Unlock()

	// Update subscription count metric
	adapter.UpdateSubscriptionCount("vmware", count)

	a.Logger.Info("created subscription",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", sub.Callback))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(_ context.Context, id string) (*adapter.Subscription, error) {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation("vmware", "GetSubscription", start, err) }()

	a.Logger.Debug("GetSubscription called",
		zap.String("id", id))

	a.SubscriptionsMu.RLock()
	subscription, exists := a.Subscriptions[id]
	a.SubscriptionsMu.RUnlock()

	if !exists {
		err = fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		return nil, err
	}

	return subscription, nil
}

// UpdateSubscription updates an existing subscription.
func (a *Adapter) UpdateSubscription(
	_ context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	var err error
	start := time.Now()
	defer func() { adapter.ObserveOperation("vmware", "UpdateSubscription", start, err) }()

	a.Logger.Debug("UpdateSubscription called",
		zap.String("id", id),
		zap.String("callback", sub.Callback))

	// Validate callback URL
	if sub.Callback == "" {
		err = fmt.Errorf("callback URL is required")
		return nil, err
	}

	a.SubscriptionsMu.Lock()
	defer a.SubscriptionsMu.Unlock()

	// Check if subscription exists
	existing, exists := a.Subscriptions[id]
	if !exists {
		err = fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		return nil, err
	}

	// Create updated subscription preserving the ID
	updatedSub := &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Store updated subscription
	a.Subscriptions[id] = updatedSub

	a.Logger.Info("updated subscription",
		zap.String("subscriptionId", id),
		zap.String("oldCallback", existing.Callback),
		zap.String("newCallback", sub.Callback))

	return updatedSub, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(_ context.Context, id string) error {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveOperation("vmware", "DeleteSubscription", start, err) }()

	a.Logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	a.SubscriptionsMu.Lock()
	if _, exists := a.Subscriptions[id]; !exists {
		a.SubscriptionsMu.Unlock()
		err = fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		return err
	}

	delete(a.Subscriptions, id)
	count := len(a.Subscriptions)
	a.SubscriptionsMu.Unlock()

	// Update subscription count metric
	adapter.UpdateSubscriptionCount("vmware", count)

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
