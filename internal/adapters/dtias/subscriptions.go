package dtias

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// CreateSubscription creates a new event subscription.
// NOTE: DTIAS does not have a native event/subscription system.
// This implementation returns an error indicating that subscriptions must be
// implemented at a higher layer using polling or an external event system.
func (a *DTIASAdapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	a.logger.Debug("CreateSubscription called",
		zap.String("callback", sub.Callback))

	// DTIAS does not support native subscriptions
	// Subscriptions should be implemented at the gateway layer using:
	// 1. Polling-based change detection
	// 2. External event system (e.g., Redis Pub/Sub, Kafka)
	// 3. Kubernetes informers if DTIAS servers are represented as CRDs

	return nil, fmt.Errorf("DTIAS adapter does not support native subscriptions - subscriptions must be implemented at the gateway layer using polling or external event system")
}

// GetSubscription retrieves a specific subscription by ID.
// NOTE: DTIAS does not have a native subscription system.
func (a *DTIASAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	a.logger.Debug("GetSubscription called",
		zap.String("id", id))

	return nil, fmt.Errorf("DTIAS adapter does not support native subscriptions")
}

// DeleteSubscription deletes a subscription by ID.
// NOTE: DTIAS does not have a native subscription system.
func (a *DTIASAdapter) DeleteSubscription(ctx context.Context, id string) error {
	a.logger.Debug("DeleteSubscription called",
		zap.String("id", id))

	return fmt.Errorf("DTIAS adapter does not support native subscriptions")
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
	// RecommendedIntervals provides suggested polling intervals by resource type
	RecommendedIntervals map[string]string

	// ChangeDetectionFields lists fields that should be monitored for changes
	ChangeDetectionFields map[string][]string

	// OptimizationTips provides tips for efficient polling
	OptimizationTips []string
}

// GetPollingRecommendation returns recommendations for implementing polling-based
// subscriptions with DTIAS.
func (a *DTIASAdapter) GetPollingRecommendation() *PollingRecommendation {
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
