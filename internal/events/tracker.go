package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes for delivery tracking.
	deliveryKeyPrefix          = "delivery:"
	deliveryEventIndexPrefix   = "deliveries:event:"
	deliverySubscriptionPrefix = "deliveries:subscription:"
	deliveryFailedSet          = "deliveries:failed"
	deliveryTTL                = 7 * 24 * time.Hour // 7 days
)

// RedisDeliveryTracker implements the DeliveryTracker interface using Redis.
type RedisDeliveryTracker struct {
	client redis.UniversalClient
}

// NewRedisDeliveryTracker creates a new RedisDeliveryTracker instance.
func NewRedisDeliveryTracker(client redis.UniversalClient) *RedisDeliveryTracker {
	if client == nil {
		panic("Redis client cannot be nil")
	}

	return &RedisDeliveryTracker{
		client: client,
	}
}

// Track records a delivery attempt.
func (t *RedisDeliveryTracker) Track(ctx context.Context, delivery *NotificationDelivery) error {
	if delivery == nil {
		return errors.New("delivery cannot be nil")
	}
	if delivery.ID == "" {
		return errors.New("delivery ID cannot be empty")
	}

	// Serialize delivery
	data, err := json.Marshal(delivery)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery: %w", err)
	}

	key := deliveryKeyPrefix + delivery.ID

	// Use pipeline for atomic operations
	pipe := t.client.Pipeline()

	// Store delivery data
	pipe.Set(ctx, key, data, deliveryTTL)

	// Add to event index
	if delivery.EventID != "" {
		eventIndexKey := deliveryEventIndexPrefix + delivery.EventID
		pipe.SAdd(ctx, eventIndexKey, delivery.ID)
		pipe.Expire(ctx, eventIndexKey, deliveryTTL)
	}

	// Add to subscription index
	if delivery.SubscriptionID != "" {
		subIndexKey := deliverySubscriptionPrefix + delivery.SubscriptionID
		pipe.SAdd(ctx, subIndexKey, delivery.ID)
		pipe.Expire(ctx, subIndexKey, deliveryTTL)
	}

	// Track failed deliveries
	if delivery.Status == DeliveryStatusFailed {
		pipe.ZAdd(ctx, deliveryFailedSet, redis.Z{
			Score:  float64(delivery.CompletedAt.Unix()),
			Member: delivery.ID,
		})
		pipe.Expire(ctx, deliveryFailedSet, deliveryTTL)
	} else {
		// Remove from failed set if status changed
		pipe.ZRem(ctx, deliveryFailedSet, delivery.ID)
	}

	// Execute pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to track delivery: %w", err)
	}

	return nil
}

// Get retrieves delivery information by ID.
func (t *RedisDeliveryTracker) Get(ctx context.Context, deliveryID string) (*NotificationDelivery, error) {
	if deliveryID == "" {
		return nil, errors.New("delivery ID cannot be empty")
	}

	key := deliveryKeyPrefix + deliveryID

	data, err := t.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errors.New("delivery not found")
		}
		return nil, fmt.Errorf("failed to get delivery: %w", err)
	}

	var delivery NotificationDelivery
	if err := json.Unmarshal(data, &delivery); err != nil {
		return nil, fmt.Errorf("failed to unmarshal delivery: %w", err)
	}

	return &delivery, nil
}

// ListByEvent retrieves all deliveries for a specific event.
func (t *RedisDeliveryTracker) ListByEvent(ctx context.Context, eventID string) ([]*NotificationDelivery, error) {
	if eventID == "" {
		return nil, errors.New("event ID cannot be empty")
	}

	eventIndexKey := deliveryEventIndexPrefix + eventID

	// Get delivery IDs from index
	deliveryIDs, err := t.client.SMembers(ctx, eventIndexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list delivery IDs by event: %w", err)
	}

	if len(deliveryIDs) == 0 {
		return []*NotificationDelivery{}, nil
	}

	// Retrieve deliveries
	return t.getDeliveriesByIDs(ctx, deliveryIDs)
}

// ListBySubscription retrieves all deliveries for a specific subscription.
func (t *RedisDeliveryTracker) ListBySubscription(ctx context.Context, subscriptionID string) ([]*NotificationDelivery, error) {
	if subscriptionID == "" {
		return nil, errors.New("subscription ID cannot be empty")
	}

	subIndexKey := deliverySubscriptionPrefix + subscriptionID

	// Get delivery IDs from index
	deliveryIDs, err := t.client.SMembers(ctx, subIndexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list delivery IDs by subscription: %w", err)
	}

	if len(deliveryIDs) == 0 {
		return []*NotificationDelivery{}, nil
	}

	// Retrieve deliveries
	return t.getDeliveriesByIDs(ctx, deliveryIDs)
}

// ListFailed retrieves all failed deliveries.
func (t *RedisDeliveryTracker) ListFailed(ctx context.Context) ([]*NotificationDelivery, error) {
	// Get failed delivery IDs from sorted set (ordered by completion time)
	deliveryIDs, err := t.client.ZRange(ctx, deliveryFailedSet, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list failed delivery IDs: %w", err)
	}

	if len(deliveryIDs) == 0 {
		return []*NotificationDelivery{}, nil
	}

	// Retrieve deliveries
	return t.getDeliveriesByIDs(ctx, deliveryIDs)
}

// getDeliveriesByIDs retrieves multiple deliveries by their IDs.
func (t *RedisDeliveryTracker) getDeliveriesByIDs(ctx context.Context, deliveryIDs []string) ([]*NotificationDelivery, error) {
	deliveries := make([]*NotificationDelivery, 0, len(deliveryIDs))

	for _, deliveryID := range deliveryIDs {
		delivery, err := t.Get(ctx, deliveryID)
		if err != nil {
			// Skip deliveries that failed to load
			continue
		}
		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}
