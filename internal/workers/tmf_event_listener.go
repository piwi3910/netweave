// Package workers provides background workers for event processing and webhook delivery.
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/controllers"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/storage"
)

const (
	// tmfConsumerGroup is the Redis Stream consumer group for TMF688 event delivery.
	tmfConsumerGroup = "tmf688-event-delivery"

	// tmfConsumerName is the Redis Stream consumer name for this instance.
	tmfConsumerName = "tmf688-consumer"

	// tmfReadBlockTime is how long to wait for new stream messages.
	tmfReadBlockTime = 5 * time.Second

	// tmfReadCount is the number of messages to read at once.
	tmfReadCount = 10

	// tmfMaxRetries is the default maximum number of retries for event delivery.
	tmfMaxRetries = 3

	// tmfRetryDelay is the default initial delay between retries.
	tmfRetryDelay = 1 * time.Second
)

// TMFEventListener listens to O2-IMS resource events and publishes them to TMF688 hubs.
// It reads events from a Redis Stream and delivers them to registered webhook callbacks.
type TMFEventListener struct {
	redisClient *redis.Client
	hubStore    storage.HubStore
	publisher   *TMFEventPublisher
	logger      *zap.Logger
	baseURL     string

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// TMFEventListenerConfig configures the TMF event listener.
type TMFEventListenerConfig struct {
	// RedisClient is the Redis client for stream operations
	RedisClient *redis.Client

	// HubStore provides access to TMF688 hub registrations
	HubStore storage.HubStore

	// Publisher handles webhook delivery
	Publisher *TMFEventPublisher

	// Logger provides structured logging
	Logger *zap.Logger

	// BaseURL is the gateway base URL for constructing event hrefs
	BaseURL string
}

// NewTMFEventListener creates a new TMF688 event listener.
func NewTMFEventListener(cfg *TMFEventListenerConfig) (*TMFEventListener, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if cfg.RedisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if cfg.HubStore == nil {
		return nil, fmt.Errorf("hub store cannot be nil")
	}
	if cfg.Publisher == nil {
		return nil, fmt.Errorf("publisher cannot be nil")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL cannot be empty")
	}

	return &TMFEventListener{
		redisClient: cfg.RedisClient,
		hubStore:    cfg.HubStore,
		publisher:   cfg.Publisher,
		logger:      cfg.Logger,
		baseURL:     cfg.BaseURL,
		stopCh:      make(chan struct{}),
	}, nil
}

// Start starts the event listener and begins processing events.
func (l *TMFEventListener) Start(ctx context.Context) error {
	l.logger.Info("starting TMF688 event listener")

	// Create consumer group if it doesn't exist
	if err := l.ensureConsumerGroup(ctx); err != nil {
		return fmt.Errorf("failed to ensure consumer group: %w", err)
	}

	// Start consumer goroutine
	l.wg.Add(1)
	go l.consumeEvents(ctx)

	l.logger.Info("TMF688 event listener started")

	// Wait for context cancellation
	<-ctx.Done()

	return l.Stop()
}

// Stop stops the event listener and waits for all goroutines to finish.
func (l *TMFEventListener) Stop() error {
	l.logger.Info("stopping TMF688 event listener")

	// Signal shutdown
	close(l.stopCh)

	// Wait for all goroutines to finish
	l.wg.Wait()

	l.logger.Info("TMF688 event listener stopped")
	return nil
}

// ensureConsumerGroup creates the consumer group if it doesn't already exist.
func (l *TMFEventListener) ensureConsumerGroup(ctx context.Context) error {
	// Try to create the consumer group
	err := l.redisClient.XGroupCreateMkStream(
		ctx,
		controllers.EventStreamKey,
		tmfConsumerGroup,
		"0",
	).Err()

	// Ignore error if group already exists
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	return nil
}

// consumeEvents continuously reads and processes events from the Redis Stream.
func (l *TMFEventListener) consumeEvents(ctx context.Context) {
	defer l.wg.Done()

	for {
		select {
		case <-l.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			// Read messages from stream
			streams, err := l.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    tmfConsumerGroup,
				Consumer: tmfConsumerName,
				Streams:  []string{controllers.EventStreamKey, ">"},
				Count:    tmfReadCount,
				Block:    tmfReadBlockTime,
			}).Result()

			if err != nil {
				if errors.Is(err, redis.Nil) {
					// No new messages, continue
					continue
				}
				l.logger.Error("failed to read from stream",
					zap.Error(err))
				time.Sleep(1 * time.Second)
				continue
			}

			// Process each message
			for _, stream := range streams {
				for _, message := range stream.Messages {
					if err := l.processMessage(ctx, message); err != nil {
						l.logger.Error("failed to process message",
							zap.String("messageId", message.ID),
							zap.Error(err))
					}
				}
			}
		}
	}
}

// processMessage processes a single event message.
func (l *TMFEventListener) processMessage(ctx context.Context, message redis.XMessage) error {
	// Parse event from message
	event, err := l.parseEvent(message)
	if err != nil {
		return err
	}

	l.logger.Debug("processing event",
		zap.String("eventType", event.EventType),
		zap.String("resourceId", event.GlobalResourceID))

	// Find matching hubs for this event
	matchingHubs, err := l.findMatchingHubs(ctx, event)
	if err != nil {
		return err
	}

	// If no matching hubs, acknowledge and return
	if len(matchingHubs) == 0 {
		l.logger.Debug("no matching hubs for event",
			zap.String("resourceId", event.GlobalResourceID))
		return l.acknowledgeMessage(ctx, message.ID)
	}

	// Publish event to matching hubs
	if err := l.publishToHubs(ctx, event, matchingHubs); err != nil {
		// Log error but still acknowledge message
		l.logger.Error("failed to publish to some hubs", zap.Error(err))
	}

	// Acknowledge message
	return l.acknowledgeMessage(ctx, message.ID)
}

// parseEvent extracts and parses the event from a Redis message.
func (l *TMFEventListener) parseEvent(message redis.XMessage) (*controllers.ResourceEvent, error) {
	eventData, ok := message.Values["event"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid event data format")
	}

	var event controllers.ResourceEvent
	if err := json.Unmarshal([]byte(eventData), &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

// findMatchingHubs retrieves hubs and filters those matching the event.
func (l *TMFEventListener) findMatchingHubs(
	ctx context.Context,
	event *controllers.ResourceEvent,
) ([]*storage.HubRegistration, error) {
	hubs, err := l.hubStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list hubs: %w", err)
	}

	if len(hubs) == 0 {
		l.logger.Debug("no hubs registered, skipping event")
		return nil, nil
	}

	matchingHubs := make([]*storage.HubRegistration, 0)
	for _, hub := range hubs {
		if handlers.ShouldPublishEventToHub(event, hub) {
			matchingHubs = append(matchingHubs, hub)
		}
	}

	return matchingHubs, nil
}

// publishToHubs transforms and publishes an event to matching hubs.
func (l *TMFEventListener) publishToHubs(
	ctx context.Context,
	event *controllers.ResourceEvent,
	matchingHubs []*storage.HubRegistration,
) error {
	l.logger.Info("publishing event to matching hubs",
		zap.String("eventType", event.EventType),
		zap.String("resourceId", event.GlobalResourceID),
		zap.Int("hubCount", len(matchingHubs)))

	// Transform event to TMF688 format
	tmfEvent := handlers.TransformResourceEventToTMF688(event, l.baseURL)

	// Collect callback URLs
	callbacks := make([]string, len(matchingHubs))
	for i, hub := range matchingHubs {
		callbacks[i] = hub.Callback
	}

	// Publish to all matching hubs concurrently
	errors := l.publisher.PublishToMultipleHubs(ctx, callbacks, tmfEvent, tmfMaxRetries, tmfRetryDelay)

	// Log results
	l.logPublishResults(tmfEvent.ID, matchingHubs, errors)

	if len(errors) > 0 {
		return fmt.Errorf("failed to publish to %d hubs", len(errors))
	}

	return nil
}

// logPublishResults logs the results of publishing to hubs.
func (l *TMFEventListener) logPublishResults(
	eventID string,
	matchingHubs []*storage.HubRegistration,
	errors map[string]error,
) {
	// Log any publishing errors
	for callback, err := range errors {
		l.logger.Error("failed to publish to hub",
			zap.String("callback", callback),
			zap.Error(err))
	}

	// Log successful deliveries
	successCount := len(matchingHubs) - len(errors)
	if successCount > 0 {
		l.logger.Info("event published successfully",
			zap.String("eventId", eventID),
			zap.Int("successCount", successCount),
			zap.Int("failureCount", len(errors)))
	}
}

// acknowledgeMessage acknowledges a message in the Redis Stream.
func (l *TMFEventListener) acknowledgeMessage(ctx context.Context, messageID string) error {
	if err := l.redisClient.XAck(ctx, controllers.EventStreamKey, tmfConsumerGroup, messageID).Err(); err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}
	return nil
}
