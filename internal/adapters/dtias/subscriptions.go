package dtias

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// CreateSubscription creates a new event subscription.
// Since DTIAS does not have a native event/subscription system, subscriptions are
// stored locally and the gateway layer implements polling to detect changes.
// The polling mechanism should periodically:
//  1. Call ListResourcePools() and ListResources() to get current state
//  2. Compare with previous state to detect changes
//  3. Match changes against subscription filters
//  4. Send webhook notifications to matching subscription callbacks
func (a *Adapter) CreateSubscription(
	_ context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	// Validate required fields
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Generate subscription ID if not provided
	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = uuid.New().String()
	}

	// Create the subscription
	newSub := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	// Store the subscription atomically with existence check
	a.subscriptionsMu.Lock()
	if _, exists := a.subscriptions[subscriptionID]; exists {
		a.subscriptionsMu.Unlock()
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionExists, subscriptionID)
	}
	a.subscriptions[subscriptionID] = newSub
	a.subscriptionsMu.Unlock()

	a.logger.Info("subscription created (polling-based)",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", sub.Callback))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(_ context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called",
		zap.String("id", id))

	a.subscriptionsMu.RLock()
	sub, ok := a.subscriptions[id]
	a.subscriptionsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	return sub, nil
}

// UpdateSubscription updates an existing subscription.
// Returns the updated subscription or an error if not found.
func (a *Adapter) UpdateSubscription(
	_ context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("UpdateSubscription called",
		zap.String("id", id))

	// Validate required fields
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Update the subscription atomically with existence check
	a.subscriptionsMu.Lock()
	defer a.subscriptionsMu.Unlock()

	existing, ok := a.subscriptions[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	// Create updated subscription preserving the ID
	updated := &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	a.subscriptions[id] = updated

	a.logger.Info("subscription updated",
		zap.String("subscriptionId", id),
		zap.String("oldCallback", existing.Callback),
		zap.String("newCallback", sub.Callback))

	return updated, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(_ context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	a.subscriptionsMu.Lock()
	defer a.subscriptionsMu.Unlock()

	if _, ok := a.subscriptions[id]; !ok {
		return fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
	}

	delete(a.subscriptions, id)

	a.logger.Info("subscription deleted",
		zap.String("subscriptionId", id))

	return nil
}

// ListSubscriptions returns all active subscriptions.
// This is useful for the polling mechanism to know which subscriptions need notifications.
func (a *Adapter) ListSubscriptions() []*adapter.Subscription {
	a.subscriptionsMu.RLock()
	defer a.subscriptionsMu.RUnlock()

	subs := make([]*adapter.Subscription, 0, len(a.subscriptions))
	for _, sub := range a.subscriptions {
		subs = append(subs, sub)
	}

	return subs
}

// PollingRecommendation provides guidance for implementing subscription-like functionality
// with DTIAS using polling.
//
// Since DTIAS lacks native event subscriptions, the recommended approach is:
//
//  1. Implement a polling controller at the gateway layer that periodically:
//     - Calls ListResourcePools() to detect pool changes
//     - Calls ListResources() to detect server state changes
//     - Compares results with previous state to detect changes
//
//  2. Store subscription filters in Redis and match changes against them
//
//  3. Send webhook notifications for matching changes
//
// Example polling intervals:
//   - Resource pools: 60 seconds (pools change infrequently)
//   - Resources: 30 seconds (server states change more frequently)
//   - Health metrics: 10 seconds (for critical health monitoring)
//
// This approach provides subscription-like functionality without native DTIAS support.
type PollingRecommendation struct {
	// RecommendedIntervals provides suggested polling intervals by resource type.
	RecommendedIntervals map[string]string

	// ChangeDetectionFields lists fields that should be monitored for changes.
	ChangeDetectionFields map[string][]string

	// OptimizationTips provides tips for efficient polling.
	OptimizationTips []string
}

// GetPollingRecommendation returns recommendations for implementing polling-based
// subscriptions with DTIAS.
func (a *Adapter) GetPollingRecommendation() *PollingRecommendation {
	return &PollingRecommendation{
		RecommendedIntervals: map[string]string{
			"resource-pools": "60s",
			"resources":      "30s",
			"health-metrics": "10s",
		},
		ChangeDetectionFields: map[string][]string{
			"resource-pools": {
				"state",
				"serverCount",
				"availableServers",
			},
			"resources": {
				"state",
				"powerState",
				"healthState",
				"serverPoolId",
			},
		},
		OptimizationTips: []string{
			"Use filter parameters to reduce API response sizes",
			"Store ETag or Last-Modified headers to detect changes efficiently",
			"Implement exponential backoff for API rate limiting",
			"Cache resource metadata and only query changed resources",
			"Use Redis to store previous state for efficient change detection",
			"Consider using Redis Pub/Sub for inter-gateway communication in multi-instance deployments",
		},
	}
}
