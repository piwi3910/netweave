package dtias

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/security/urlredact"
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
	ctx context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", urlredact.Redact(sub.Callback)))

	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	subscriptionID := sub.SubscriptionID
	if subscriptionID == "" {
		subscriptionID = uuid.New().String()
	}

	newSub := &adapter.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	if err := a.Subs.Create(ctx, newSub); err != nil {
		return nil, err
	}

	a.logger.Info("subscription created (polling-based)",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", urlredact.Redact(sub.Callback)))

	return newSub, nil
}

// GetSubscription retrieves a specific subscription by ID.
func (a *Adapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called", zap.String("id", id))
	return a.Subs.Get(ctx, id)
}

// UpdateSubscription updates an existing subscription.
// Returns the updated subscription or an error if not found.
func (a *Adapter) UpdateSubscription(
	ctx context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	a.logger.Debug("UpdateSubscription called", zap.String("id", id))

	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	existing, err := a.Subs.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	updated := &adapter.Subscription{
		SubscriptionID:         id,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
	}

	if err := a.Subs.Update(ctx, id, updated); err != nil {
		return nil, err
	}

	a.logger.Info("subscription updated",
		zap.String("subscription_id", id),
		zap.String("old_callback", existing.Callback),
		zap.String("new_callback", sub.Callback))

	return updated, nil
}

// DeleteSubscription deletes a subscription by ID.
func (a *Adapter) DeleteSubscription(ctx context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called", zap.String("id", id))

	if err := a.Subs.Delete(ctx, id); err != nil {
		return err
	}

	a.logger.Info("subscription deleted", zap.String("subscription_id", id))
	return nil
}

// ListSubscriptions returns all active subscriptions.
// This is useful for the polling mechanism to know which subscriptions need notifications.
func (a *Adapter) ListSubscriptions() []*adapter.Subscription {
	subs, err := a.Subs.List(context.Background(), nil)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			a.logger.Warn("listing subscriptions returned unexpected error", zap.Error(err))
		}
		return nil
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
