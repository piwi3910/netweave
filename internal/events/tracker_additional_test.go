package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/events"
)

func TestRedisDeliveryTracker_ListBySubscription(t *testing.T) {
	t.Run("list deliveries for subscription", func(t *testing.T) {
		mr := miniredis.RunT(t)
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		tracker := events.NewRedisDeliveryTracker(client)

		ctx := context.Background()
		subscriptionID := "sub-list-test"

		// Track multiple deliveries for the same subscription
		for i := 0; i < 3; i++ {
			delivery := &events.NotificationDelivery{
				ID:             "delivery-sub-" + string(rune('A'+i)),
				EventID:        "event-" + string(rune('1'+i)),
				SubscriptionID: subscriptionID,
				Status:         events.DeliveryStatusDelivered,
				CreatedAt:      time.Now().UTC(),
			}
			err := tracker.Track(ctx, delivery)
			require.NoError(t, err)
		}

		// List by subscription
		deliveries, err := tracker.ListBySubscription(ctx, subscriptionID)
		require.NoError(t, err)
		assert.Len(t, deliveries, 3)
	})

	t.Run("empty subscription ID", func(t *testing.T) {
		mr := miniredis.RunT(t)
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		tracker := events.NewRedisDeliveryTracker(client)

		ctx := context.Background()
		_, err := tracker.ListBySubscription(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription ID cannot be empty")
	})

	t.Run("non-existent subscription", func(t *testing.T) {
		mr := miniredis.RunT(t)
		defer mr.Close()

		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		tracker := events.NewRedisDeliveryTracker(client)

		ctx := context.Background()
		deliveries, err := tracker.ListBySubscription(ctx, "nonexistent-sub")
		require.NoError(t, err)
		assert.Empty(t, deliveries)
	})
}

func TestRedisDeliveryTracker_Track_FailedStatus(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	tracker := events.NewRedisDeliveryTracker(client)

	ctx := context.Background()

	// Track a delivery as failed
	delivery := &events.NotificationDelivery{
		ID:             "delivery-failed-test",
		EventID:        "event-fail",
		SubscriptionID: "sub-fail",
		CallbackURL:    "https://example.com/webhook",
		Status:         events.DeliveryStatusFailed,
		Attempts:       3,
		MaxAttempts:    3,
		LastError:      "connection refused",
		CreatedAt:      time.Now().UTC(),
		CompletedAt:    time.Now().UTC(),
	}
	err := tracker.Track(ctx, delivery)
	require.NoError(t, err)

	// List failed should contain it
	failed, err := tracker.ListFailed(ctx)
	require.NoError(t, err)
	assert.Len(t, failed, 1)
	assert.Equal(t, "delivery-failed-test", failed[0].ID)

	// Now update to delivered status
	delivery.Status = events.DeliveryStatusDelivered
	err = tracker.Track(ctx, delivery)
	require.NoError(t, err)

	// List failed should no longer contain it
	failed, err = tracker.ListFailed(ctx)
	require.NoError(t, err)
	assert.Empty(t, failed)
}

func TestRedisDeliveryTracker_Track_EmptyEventAndSubscriptionIDs(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	tracker := events.NewRedisDeliveryTracker(client)

	ctx := context.Background()

	// Track a delivery with empty event and subscription IDs
	delivery := &events.NotificationDelivery{
		ID:        "delivery-minimal",
		Status:    events.DeliveryStatusPending,
		CreatedAt: time.Now().UTC(),
	}
	err := tracker.Track(ctx, delivery)
	require.NoError(t, err)

	// Get it back
	retrieved, err := tracker.Get(ctx, "delivery-minimal")
	require.NoError(t, err)
	assert.Equal(t, "delivery-minimal", retrieved.ID)
	assert.Equal(t, events.DeliveryStatusPending, retrieved.Status)
}

func TestRedisDeliveryTracker_ListByEvent_NoDeliveries(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	tracker := events.NewRedisDeliveryTracker(client)

	ctx := context.Background()

	deliveries, err := tracker.ListByEvent(ctx, "nonexistent-event")
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}

func TestRedisDeliveryTracker_ListFailed_Empty(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	tracker := events.NewRedisDeliveryTracker(client)

	ctx := context.Background()

	failed, err := tracker.ListFailed(ctx)
	require.NoError(t, err)
	assert.Empty(t, failed)
}
