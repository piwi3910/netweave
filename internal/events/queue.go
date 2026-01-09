package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// Redis stream key for events.
	eventStreamKey = "events:stream"

	// Default batch size for reading from stream.
	defaultBatchSize = 10

	// Block time for reading from stream (milliseconds).
	blockTime = 5000
)

// RedisQueue implements the Queue interface using Redis Streams.
// Redis Streams provide reliable, ordered event delivery with consumer groups.
type RedisQueue struct {
	client redis.UniversalClient
	logger *zap.Logger
}

// NewRedisQueue creates a new RedisQueue instance.
func NewRedisQueue(client redis.UniversalClient, logger *zap.Logger) *RedisQueue {
	if client == nil {
		panic("Redis client cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &RedisQueue{
		client: client,
		logger: logger,
	}
}

// Publish adds an event to the Redis stream.
func (q *RedisQueue) Publish(ctx context.Context, event *Event) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}
	if event.ID == "" {
		return errors.New("event ID cannot be empty")
	}

	// Serialize event to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Add to Redis stream
	args := &redis.XAddArgs{
		Stream: eventStreamKey,
		Values: map[string]interface{}{
			"event": string(eventJSON),
		},
	}

	streamID, err := q.client.XAdd(ctx, args).Result()
	if err != nil {
		RecordEventQueued("error")
		return fmt.Errorf("failed to add event to stream: %w", err)
	}

	RecordEventQueued("success")

	q.logger.Debug("event published to stream",
		zap.String("event_id", event.ID),
		zap.String("stream_id", streamID),
		zap.String("event_type", string(event.Type)),
	)

	return nil
}

// Subscribe subscribes to the event stream using a consumer group.
// Returns a channel that receives events from the stream.
func (q *RedisQueue) Subscribe(ctx context.Context, consumerGroup, consumerName string) (<-chan *Event, error) {
	if consumerGroup == "" {
		return nil, errors.New("consumer group cannot be empty")
	}
	if consumerName == "" {
		return nil, errors.New("consumer name cannot be empty")
	}

	// Create consumer group if it doesn't exist
	err := q.client.XGroupCreateMkStream(ctx, eventStreamKey, consumerGroup, "0").Err()
	if err != nil && !isConsumerGroupExistsError(err) {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Create event channel
	eventCh := make(chan *Event, defaultBatchSize)

	// Start goroutine to read from stream
	go q.readFromStream(ctx, consumerGroup, consumerName, eventCh)

	return eventCh, nil
}

// readFromStream continuously reads events from the Redis stream.
func (q *RedisQueue) readFromStream(ctx context.Context, consumerGroup, consumerName string, eventCh chan<- *Event) {
	defer close(eventCh)

	q.logger.Info("starting stream consumer",
		zap.String("consumer_group", consumerGroup),
		zap.String("consumer_name", consumerName),
	)

	for {
		select {
		case <-ctx.Done():
			q.logger.Info("stopping stream consumer",
				zap.String("consumer_group", consumerGroup),
				zap.String("consumer_name", consumerName),
			)
			return
		default:
			// Read from stream
			streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    consumerGroup,
				Consumer: consumerName,
				Streams:  []string{eventStreamKey, ">"},
				Count:    defaultBatchSize,
				Block:    blockTime * time.Millisecond,
			}).Result()

			if err != nil {
				if errors.Is(err, redis.Nil) {
					// No messages available, continue
					continue
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				q.logger.Error("failed to read from stream",
					zap.Error(err),
					zap.String("consumer_group", consumerGroup),
				)
				time.Sleep(time.Second)
				continue
			}

			// Process messages
			for _, stream := range streams {
				for _, message := range stream.Messages {
					event, err := q.parseEvent(message)
					if err != nil {
						q.logger.Error("failed to parse event",
							zap.Error(err),
							zap.String("stream_id", message.ID),
						)
						// Acknowledge invalid message to prevent blocking
						_ = q.Acknowledge(ctx, consumerGroup, message.ID)
						continue
					}

					// Send event to channel
					select {
					case eventCh <- event:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}
}

// parseEvent parses an event from a Redis stream message.
func (q *RedisQueue) parseEvent(message redis.XMessage) (*Event, error) {
	eventData, ok := message.Values["event"].(string)
	if !ok {
		return nil, errors.New("invalid event data format")
	}

	var event Event
	if err := json.Unmarshal([]byte(eventData), &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

// Acknowledge marks an event as successfully processed.
func (q *RedisQueue) Acknowledge(ctx context.Context, consumerGroup, streamID string) error {
	if consumerGroup == "" {
		return errors.New("consumer group cannot be empty")
	}
	if streamID == "" {
		return errors.New("stream ID cannot be empty")
	}

	err := q.client.XAck(ctx, eventStreamKey, consumerGroup, streamID).Err()
	if err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}

	return nil
}

// Close closes the Redis connection.
func (q *RedisQueue) Close() error {
	// Note: We don't close the Redis client here as it may be shared
	// with other components (storage, rate limiting, etc.)
	return nil
}

// isConsumerGroupExistsError checks if the error is due to consumer group already existing.
func isConsumerGroupExistsError(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
