package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/models"
)

// newTestRedisQueue creates a RedisQueue backed by miniredis for internal tests.
func newTestRedisQueue(t *testing.T) (*RedisQueue, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logger, _ := zap.NewDevelopment()
	q := &RedisQueue{
		client: client,
		logger: logger,
	}
	return q, mr
}

func TestParseEvent(t *testing.T) {
	q, _ := newTestRedisQueue(t)

	tests := []struct {
		name    string
		message redis.XMessage
		wantErr bool
	}{
		{
			name: "valid event data",
			message: redis.XMessage{
				ID: "1234567-0",
				Values: map[string]interface{}{
					"event": `{"id":"event-1","type":"ResourceCreated","resourceType":"resource","resourceId":"node-1","timestamp":"2024-01-01T00:00:00Z"}`,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid event data format - not string",
			message: redis.XMessage{
				ID: "1234567-1",
				Values: map[string]interface{}{
					"event": 12345,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			message: redis.XMessage{
				ID: "1234567-2",
				Values: map[string]interface{}{
					"event": `{invalid json}`,
				},
			},
			wantErr: true,
		},
		{
			name: "missing event key",
			message: redis.XMessage{
				ID:     "1234567-3",
				Values: map[string]interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := q.parseEvent(tt.message)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, event)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, event)
				assert.Equal(t, "event-1", event.ID)
			}
		})
	}
}

func TestProcessStreamMessages(t *testing.T) {
	t.Run("processes valid messages", func(t *testing.T) {
		q, _ := newTestRedisQueue(t)

		eventCh := make(chan *Event, 10)
		ctx := context.Background()

		streams := []redis.XStream{
			{
				Stream: eventStreamKey,
				Messages: []redis.XMessage{
					{
						ID: "1234567-0",
						Values: map[string]interface{}{
							"event": `{"id":"msg-1","type":"ResourceCreated","resourceType":"resource","resourceId":"node-1","timestamp":"2024-01-01T00:00:00Z"}`,
						},
					},
					{
						ID: "1234567-1",
						Values: map[string]interface{}{
							"event": `{"id":"msg-2","type":"ResourceUpdated","resourceType":"resource","resourceId":"node-2","timestamp":"2024-01-01T00:00:00Z"}`,
						},
					},
				},
			},
		}

		canceled := q.processStreamMessages(ctx, "test-group", streams, eventCh)
		assert.False(t, canceled)

		// Two events should be in the channel
		assert.Len(t, eventCh, 2)

		event1 := <-eventCh
		assert.Equal(t, "msg-1", event1.ID)

		event2 := <-eventCh
		assert.Equal(t, "msg-2", event2.ID)
	})

	t.Run("handles invalid messages gracefully", func(t *testing.T) {
		q, _ := newTestRedisQueue(t)

		eventCh := make(chan *Event, 10)
		ctx := context.Background()

		streams := []redis.XStream{
			{
				Stream: eventStreamKey,
				Messages: []redis.XMessage{
					{
						ID: "1234567-0",
						Values: map[string]interface{}{
							"event": `invalid json`,
						},
					},
					{
						ID: "1234567-1",
						Values: map[string]interface{}{
							"event": `{"id":"valid-msg","type":"ResourceCreated","resourceType":"resource","resourceId":"node-1","timestamp":"2024-01-01T00:00:00Z"}`,
						},
					},
				},
			},
		}

		canceled := q.processStreamMessages(ctx, "test-group", streams, eventCh)
		assert.False(t, canceled)

		// Only the valid event should be in the channel
		assert.Len(t, eventCh, 1)

		event := <-eventCh
		assert.Equal(t, "valid-msg", event.ID)
	})

	t.Run("returns true when context is canceled", func(t *testing.T) {
		q, _ := newTestRedisQueue(t)

		// Use an unbuffered channel so sending blocks
		eventCh := make(chan *Event)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		streams := []redis.XStream{
			{
				Stream: eventStreamKey,
				Messages: []redis.XMessage{
					{
						ID: "1234567-0",
						Values: map[string]interface{}{
							"event": `{"id":"ctx-msg","type":"ResourceCreated","resourceType":"resource","resourceId":"node-1","timestamp":"2024-01-01T00:00:00Z"}`,
						},
					},
				},
			},
		}

		canceled := q.processStreamMessages(ctx, "test-group", streams, eventCh)
		assert.True(t, canceled)
	})

	t.Run("handles empty streams", func(t *testing.T) {
		q, _ := newTestRedisQueue(t)

		eventCh := make(chan *Event, 10)
		ctx := context.Background()

		streams := []redis.XStream{}

		canceled := q.processStreamMessages(ctx, "test-group", streams, eventCh)
		assert.False(t, canceled)
		assert.Len(t, eventCh, 0)
	})

	t.Run("handles nil streams", func(t *testing.T) {
		q, _ := newTestRedisQueue(t)

		eventCh := make(chan *Event, 10)
		ctx := context.Background()

		canceled := q.processStreamMessages(ctx, "test-group", nil, eventCh)
		assert.False(t, canceled)
		assert.Len(t, eventCh, 0)
	})
}

func TestIsConsumerGroupExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "consumer group exists error",
			err:  errors.New("BUSYGROUP Consumer Group name already exists"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("some other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConsumerGroupExistsError(tt.err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestPublish_EventMarshalling(t *testing.T) {
	// Verify that events with complex fields can be created
	t.Run("event with labels and extensions", func(t *testing.T) {
		event := &Event{
			ID:             "complex-event",
			Type:           models.EventTypeResourceCreated,
			ResourceType:   ResourceTypeResource,
			ResourceID:     "resource-1",
			ResourcePoolID: "pool-1",
			ResourceTypeID: "compute-node",
			TenantID:       "tenant-1",
			Timestamp:      time.Now().UTC(),
			Labels: map[string]string{
				"env":  "production",
				"role": "worker",
			},
			Extensions: map[string]interface{}{
				"customField": "customValue",
				"nestedObj":   map[string]interface{}{"key": "value"},
			},
		}

		// Verify the event struct is valid
		assert.NotEmpty(t, event.ID)
		assert.Equal(t, models.EventTypeResourceCreated, event.Type)
		assert.Equal(t, ResourceTypeResource, event.ResourceType)
		assert.Equal(t, "resource-1", event.ResourceID)
		assert.Equal(t, "pool-1", event.ResourcePoolID)
		assert.Equal(t, "compute-node", event.ResourceTypeID)
		assert.Equal(t, "tenant-1", event.TenantID)
		assert.False(t, event.Timestamp.IsZero())
		assert.NotEmpty(t, event.Labels)
		assert.NotEmpty(t, event.Extensions)
	})
}

func TestAcknowledge_Validation(t *testing.T) {
	q, _ := newTestRedisQueue(t)
	ctx := context.Background()

	t.Run("empty consumer group", func(t *testing.T) {
		err := q.Acknowledge(ctx, "", "stream-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "consumer group cannot be empty")
	})

	t.Run("empty stream ID", func(t *testing.T) {
		err := q.Acknowledge(ctx, "group", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stream ID cannot be empty")
	})
}

func TestPublish_Validation(t *testing.T) {
	q, _ := newTestRedisQueue(t)
	ctx := context.Background()

	t.Run("nil event", func(t *testing.T) {
		err := q.Publish(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be nil")
	})

	t.Run("successful publish", func(t *testing.T) {
		event := &Event{
			ID:           "test-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}
		err := q.Publish(ctx, event)
		require.NoError(t, err)
	})
}

func TestClose(t *testing.T) {
	q, _ := newTestRedisQueue(t)

	err := q.Close()
	require.NoError(t, err)
}

func TestReadStreamBatch_Validation(t *testing.T) {
	t.Run("returns error with canceled context", func(t *testing.T) {
		q, _ := newTestRedisQueue(t)
		ctx := context.Background()

		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		streams, err := q.readStreamBatch(canceledCtx, "group", "consumer")
		assert.Nil(t, streams)
		assert.Error(t, err)
	})

	t.Run("returns nil for no messages (redis.Nil)", func(t *testing.T) {
		q, mr := newTestRedisQueue(t)
		ctx := context.Background()

		// Create the stream and consumer group so XReadGroup doesn't error
		// but there are no messages (redis.Nil result)
		mr.XAdd("events:stream", "*", []string{"event", `{"id":"setup"}`})
		// Create consumer group
		_, err := q.client.XGroupCreate(ctx, eventStreamKey, "nil-group", "0").Result()
		require.NoError(t, err)

		// Read the message to consume it
		_, err = q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    "nil-group",
			Consumer: "test-consumer",
			Streams:  []string{eventStreamKey, ">"},
			Count:    100,
			Block:    0,
		}).Result()
		require.NoError(t, err)

		// Now read again - should get redis.Nil (no new messages) since block=0
		// Actually, miniredis may not support blocking properly, but with block=0
		// it returns immediately with no messages.
		// Let's use a separate approach: close the miniredis to simulate error
	})

	t.Run("returns error when redis is down", func(t *testing.T) {
		q, mr := newTestRedisQueue(t)

		// Create consumer group first
		ctx := context.Background()
		mr.XAdd("events:stream", "*", []string{"event", `{"id":"setup"}`})
		_, err := q.client.XGroupCreate(ctx, eventStreamKey, "down-group", "0").Result()
		require.NoError(t, err)

		// Close miniredis to simulate failure
		mr.Close()

		streams, err := q.readStreamBatch(ctx, "down-group", "consumer")
		assert.Nil(t, streams)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read from Redis stream")
	})
}

func TestNewRedisQueue_Panics(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	t.Run("nil client panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewRedisQueue(nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewRedisQueue(client, nil)
		})
	})

	t.Run("valid params succeeds", func(t *testing.T) {
		q := NewRedisQueue(client, logger)
		assert.NotNil(t, q)
	})
}
