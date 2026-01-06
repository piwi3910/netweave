package azure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// CreateSubscription creates a new event subscription.
// Azure adapter uses polling-based subscriptions since Event Grid
// integration would require additional Azure infrastructure setup.
func (a *AzureAdapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	// Validate callback URL
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Generate subscription ID if not provided
	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = fmt.Sprintf("azure-sub-%s", uuid.New().String())
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
	a.subscriptionsMu.Unlock()

	a.logger.Info("created subscription",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", sub.Callback))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *AzureAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
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
func (a *AzureAdapter) DeleteSubscription(ctx context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	a.subscriptionsMu.Lock()
	defer a.subscriptionsMu.Unlock()

	if _, exists := a.subscriptions[id]; !exists {
		return fmt.Errorf("subscription not found: %s", id)
	}

	delete(a.subscriptions, id)

	a.logger.Info("deleted subscription",
		zap.String("subscriptionId", id))

	return nil
}

// ListSubscriptions returns all active subscriptions.
// This is a helper method not part of the Adapter interface.
func (a *AzureAdapter) ListSubscriptions() []*adapter.Subscription {
	a.subscriptionsMu.RLock()
	defer a.subscriptionsMu.RUnlock()

	subs := make([]*adapter.Subscription, 0, len(a.subscriptions))
	for _, sub := range a.subscriptions {
		subs = append(subs, sub)
	}

	return subs
}
