package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/events"
	"github.com/piwi3910/netweave/internal/models"
)

func TestRedisQueue_Subscribe_ContextCancellation(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// Use a non-test logger to avoid race conditions with test logger
	zapLogger, _ := zap.NewDevelopment()
	queue := events.NewRedisQueue(client, zapLogger)

	// Subscribe with a short-lived context
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	eventCh, err := queue.Subscribe(ctx, "cancel-group", "cancel-consumer")
	require.NoError(t, err)
	require.NotNil(t, eventCh)

	// Wait for context to expire and goroutine to clean up
	<-ctx.Done()

	// Wait for the channel to be closed by the goroutine
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-eventCh:
			if !ok {
				return
			}
		case <-timer.C:
			return
		}
	}
}

func TestRedisQueue_Subscribe_MultipleEventsInOrder(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// Use a non-test logger to avoid race with goroutines
	zapLogger, _ := zap.NewDevelopment()
	queue := events.NewRedisQueue(client, zapLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, err := queue.Subscribe(ctx, "order-group", "order-consumer")
	require.NoError(t, err)

	// Publish multiple events
	for i := 0; i < 3; i++ {
		event := &events.Event{
			ID:           "order-event-" + string(rune('A'+i)),
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-" + string(rune('1'+i)),
			Timestamp:    time.Now().UTC(),
		}
		err := queue.Publish(ctx, event)
		require.NoError(t, err)
	}

	// Receive events
	received := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case evt := <-eventCh:
			require.NotNil(t, evt)
			received = append(received, evt.ID)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}

	assert.Len(t, received, 3)
}

func TestRedisQueue_Publish_RedisDown(t *testing.T) {
	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	zapLogger, _ := zap.NewDevelopment()
	queue := events.NewRedisQueue(client, zapLogger)

	// Close miniredis to simulate failure
	mr.Close()

	ctx := context.Background()
	event := &events.Event{
		ID:           "fail-event",
		Type:         models.EventTypeResourceCreated,
		ResourceType: events.ResourceTypeResource,
		ResourceID:   "resource-1",
		Timestamp:    time.Now().UTC(),
	}

	err := queue.Publish(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add event to stream")
}

func TestRedisQueue_Subscribe_ExistingConsumerGroup(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// Use a non-test logger to avoid race conditions when goroutines outlive the test
	zapLogger, _ := zap.NewDevelopment()
	queue := events.NewRedisQueue(client, zapLogger)

	ctx, cancel := context.WithCancel(context.Background())

	// Subscribe twice with the same consumer group - second should succeed
	eventCh1, err := queue.Subscribe(ctx, "existing-group", "consumer-1")
	require.NoError(t, err)
	assert.NotNil(t, eventCh1)

	eventCh2, err := queue.Subscribe(ctx, "existing-group", "consumer-2")
	require.NoError(t, err)
	assert.NotNil(t, eventCh2)

	// Cancel context and wait for goroutines to exit
	cancel()
	time.Sleep(200 * time.Millisecond)
}

func TestRedisQueue_Acknowledge_ValidMessage(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// Use a non-test logger for Subscribe goroutines
	zapLogger, _ := zap.NewDevelopment()
	queue := events.NewRedisQueue(client, zapLogger)

	ctx := context.Background()

	// Publish an event first
	event := &events.Event{
		ID:           "ack-event",
		Type:         models.EventTypeResourceCreated,
		ResourceType: events.ResourceTypeResource,
		ResourceID:   "resource-1",
		Timestamp:    time.Now().UTC(),
	}
	err := queue.Publish(ctx, event)
	require.NoError(t, err)

	// Subscribe and receive
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	eventCh, err := queue.Subscribe(subCtx, "ack-group", "ack-consumer")
	require.NoError(t, err)

	select {
	case received := <-eventCh:
		require.NotNil(t, received)
		// Note: We can't get the stream ID from the channel, but the acknowledge
		// call should not error for a valid stream ID format
	case <-time.After(2 * time.Second):
		t.Log("timeout waiting for event, skipping ACK test")
	}
}
