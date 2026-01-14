package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/events"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestTracker(t *testing.T) (*events.RedisDeliveryTracker, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	tracker := events.NewRedisDeliveryTracker(client)

	return tracker, mr
}

func TestNewRedisDeliveryTracker(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		assert.NotNil(t, tracker)
	})

	t.Run("nil client panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewRedisDeliveryTracker(nil)
		})
	})
}

func TestRedisDeliveryTrackerTrack(t *testing.T) {
	tests := []struct {
		name     string
		delivery *events.NotificationDelivery
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid delivery",
			delivery: &events.NotificationDelivery{
				ID:             "delivery-123",
				EventID:        "event-456",
				SubscriptionID: "sub-789",
				CallbackURL:    "https://example.com/callback",
				Status:         events.DeliveryStatusPending,
				Attempts:       0,
				MaxAttempts:    3,
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: false,
		},
		{
			name:     "nil delivery",
			delivery: nil,
			wantErr:  true,
			errMsg:   "delivery cannot be nil",
		},
		{
			name: "empty delivery ID",
			delivery: &events.NotificationDelivery{
				ID:             "",
				EventID:        "event-456",
				SubscriptionID: "sub-789",
				Status:         events.DeliveryStatusPending,
			},
			wantErr: true,
			errMsg:  "delivery ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker, mr := setupTestTracker(t)
			defer mr.Close()

			ctx := context.Background()
			err := tracker.Track(ctx, tt.delivery)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRedisDeliveryTrackerGet(t *testing.T) {
	t.Run("get existing delivery", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		ctx := context.Background()

		// Track a delivery
		delivery := &events.NotificationDelivery{
			ID:             "delivery-123",
			EventID:        "event-456",
			SubscriptionID: "sub-789",
			CallbackURL:    "https://example.com/callback",
			Status:         events.DeliveryStatusDelivered,
			Attempts:       1,
			MaxAttempts:    3,
			CreatedAt:      time.Now().UTC(),
		}

		err := tracker.Track(ctx, delivery)
		require.NoError(t, err)

		// Get the delivery
		retrieved, err := tracker.Get(ctx, delivery.ID)
		require.NoError(t, err)
		assert.Equal(t, delivery.ID, retrieved.ID)
		assert.Equal(t, delivery.EventID, retrieved.EventID)
		assert.Equal(t, delivery.SubscriptionID, retrieved.SubscriptionID)
		assert.Equal(t, delivery.Status, retrieved.Status)
	})

	t.Run("empty delivery ID", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		ctx := context.Background()
		_, err := tracker.Get(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delivery ID cannot be empty")
	})

	t.Run("non-existent delivery", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		ctx := context.Background()
		_, err := tracker.Get(ctx, "non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delivery not found")
	})
}

func TestRedisDeliveryTrackerListByEvent(t *testing.T) {
	t.Run("list deliveries for event", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		ctx := context.Background()
		eventID := "event-123"

		// Track multiple deliveries for the same event
		for i := 0; i < 3; i++ {
			delivery := &events.NotificationDelivery{
				ID:             "delivery-" + string(rune('1'+i)),
				EventID:        eventID,
				SubscriptionID: "sub-" + string(rune('1'+i)),
				Status:         events.DeliveryStatusDelivered,
				CreatedAt:      time.Now().UTC(),
			}
			err := tracker.Track(ctx, delivery)
			require.NoError(t, err)
		}

		// List deliveries
		deliveries, err := tracker.ListByEvent(ctx, eventID)
		require.NoError(t, err)
		assert.Len(t, deliveries, 3)
	})

	t.Run("empty event ID", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		ctx := context.Background()
		_, err := tracker.ListByEvent(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event ID cannot be empty")
	})
}

func TestRedisDeliveryTrackerListFailed(t *testing.T) {
	t.Run("list failed deliveries", func(t *testing.T) {
		tracker, mr := setupTestTracker(t)
		defer mr.Close()

		ctx := context.Background()

		// Track successful delivery
		successDelivery := &events.NotificationDelivery{
			ID:          "delivery-success",
			EventID:     "event-1",
			Status:      events.DeliveryStatusDelivered,
			CreatedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
		}
		err := tracker.Track(ctx, successDelivery)
		require.NoError(t, err)

		// Track failed delivery
		failedDelivery := &events.NotificationDelivery{
			ID:          "delivery-failed",
			EventID:     "event-2",
			Status:      events.DeliveryStatusFailed,
			CreatedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
		}
		err = tracker.Track(ctx, failedDelivery)
		require.NoError(t, err)

		// List failed deliveries
		failed, err := tracker.ListFailed(ctx)
		require.NoError(t, err)
		assert.Len(t, failed, 1)
		assert.Equal(t, "delivery-failed", failed[0].ID)
	})
}
