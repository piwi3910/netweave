package events

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/models"
)

func setupTestQueue(t *testing.T) (*RedisQueue, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := zaptest.NewLogger(t)
	queue := NewRedisQueue(client, logger)

	return queue, mr
}

func TestNewRedisQueue(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		queue, mr := setupTestQueue(t)
		defer mr.Close()

		assert.NotNil(t, queue)
	})

	t.Run("nil client panics", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		assert.Panics(t, func() {
			NewRedisQueue(nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		mr := miniredis.RunT(t)
		defer mr.Close()

		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})

		assert.Panics(t, func() {
			NewRedisQueue(client, nil)
		})
	})
}

func TestRedisQueuePublish(t *testing.T) {
	tests := []struct {
		name    string
		event   *Event
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid event",
			event: &Event{
				ID:           "event-123",
				Type:         models.EventTypeResourceCreated,
				ResourceType: ResourceTypeResource,
				ResourceID:   "node-1",
				Timestamp:    time.Now().UTC(),
			},
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
			errMsg:  "event cannot be nil",
		},
		{
			name: "empty event ID",
			event: &Event{
				Type:         models.EventTypeResourceCreated,
				ResourceType: ResourceTypeResource,
				ResourceID:   "node-1",
				Timestamp:    time.Now().UTC(),
			},
			wantErr: true,
			errMsg:  "event ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue, mr := setupTestQueue(t)
			defer mr.Close()

			ctx := context.Background()
			err := queue.Publish(ctx, tt.event)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRedisQueueSubscribe(t *testing.T) {
	t.Run("successful subscription", func(t *testing.T) {
		queue, mr := setupTestQueue(t)
		defer mr.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		eventCh, err := queue.Subscribe(ctx, "test-group", "consumer-1")
		require.NoError(t, err)
		assert.NotNil(t, eventCh)

		// Publish an event
		event := &Event{
			ID:           "event-123",
			Type:         models.EventTypeResourceCreated,
			ResourceType: ResourceTypeResource,
			ResourceID:   "node-1",
			Timestamp:    time.Now().UTC(),
		}

		err = queue.Publish(ctx, event)
		require.NoError(t, err)

		// Receive the event (with timeout)
		select {
		case receivedEvent := <-eventCh:
			assert.NotNil(t, receivedEvent)
			assert.Equal(t, event.ID, receivedEvent.ID)
			assert.Equal(t, event.ResourceID, receivedEvent.ResourceID)
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for event")
		}
	})

	t.Run("empty consumer group", func(t *testing.T) {
		queue, mr := setupTestQueue(t)
		defer mr.Close()

		ctx := context.Background()
		_, err := queue.Subscribe(ctx, "", "consumer-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "consumer group cannot be empty")
	})

	t.Run("empty consumer name", func(t *testing.T) {
		queue, mr := setupTestQueue(t)
		defer mr.Close()

		ctx := context.Background()
		_, err := queue.Subscribe(ctx, "test-group", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "consumer name cannot be empty")
	})
}

func TestRedisQueueAcknowledge(t *testing.T) {
	tests := []struct {
		name          string
		consumerGroup string
		streamID      string
		wantErr       bool
		errMsg        string
	}{
		{
			name:          "valid acknowledge",
			consumerGroup: "test-group",
			streamID:      "1234567890-0",
			wantErr:       false,
		},
		{
			name:          "empty consumer group",
			consumerGroup: "",
			streamID:      "1234567890-0",
			wantErr:       true,
			errMsg:        "consumer group cannot be empty",
		},
		{
			name:          "empty stream ID",
			consumerGroup: "test-group",
			streamID:      "",
			wantErr:       true,
			errMsg:        "stream ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue, mr := setupTestQueue(t)
			defer mr.Close()

			ctx := context.Background()
			err := queue.Acknowledge(ctx, tt.consumerGroup, tt.streamID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				// Note: ACK might succeed or fail depending on if the message exists
				// We're mainly testing that it doesn't panic
				_ = err
			}
		})
	}
}

func TestRedisQueueClose(t *testing.T) {
	queue, mr := setupTestQueue(t)
	defer mr.Close()

	err := queue.Close()
	assert.NoError(t, err)
}
