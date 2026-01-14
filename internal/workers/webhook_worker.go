// Package workers provides background workers for processing subscription events.
// It implements webhook delivery with retry logic and dead letter queues.
package workers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/controllers"
)

const (
	// EventStreamKey is the Redis Stream key for webhook events (must match controller).
	EventStreamKey = "o2ims:events"

	// DLQStreamKey is the Redis Stream key for dead letter queue.
	DLQStreamKey = "o2ims:dlq"

	// ConsumerGroup is the consumer group name for webhook workers.
	ConsumerGroup = "webhook-workers"

	// DefaultWorkerCount is the default number of worker goroutines.
	DefaultWorkerCount = 10

	// DefaultTimeout is the default HTTP client timeout.
	DefaultTimeout = 10 * time.Second

	// DefaultMaxRetries is the default maximum number of retry attempts.
	DefaultMaxRetries = 3

	// DefaultRetryBackoff is the default base backoff duration for retries.
	DefaultRetryBackoff = 1 * time.Second

	// DefaultMaxBackoff is the default maximum backoff duration.
	DefaultMaxBackoff = 5 * time.Minute

	// DeliverySuccessStatus is the HTTP status indicating successful delivery.
	DeliverySuccessStatus = 200
)

// WebhookWorker processes webhook notifications from Redis Stream.
type WebhookWorker struct {
	// redisClient is used for stream operations.
	redisClient *redis.Client

	// httpClient is used for webhook delivery.
	HTTPClient *http.Client

	// logger provides structured logging.
	logger *zap.Logger

	// workerCount is the number of worker goroutines.
	WorkerCount int

	// maxRetries is the maximum number of retry attempts.
	MaxRetries int

	// retryBackoff is the base backoff duration for retries.
	retryBackoff time.Duration

	// maxBackoff is the maximum backoff duration.
	maxBackoff time.Duration

	// hmacSecret is the secret key for HMAC signature generation.
	HMACSecret string

	// stopCh is used to signal worker shutdown.
	stopCh chan struct{}

	// wg tracks running goroutines.
	wg sync.WaitGroup
}

// Config holds configuration for creating a WebhookWorker.
type Config struct {
	// RedisClient is used for stream operations.
	RedisClient *redis.Client

	// Logger is the logger to use.
	Logger *zap.Logger

	// WorkerCount is the number of worker goroutines (default: 10).
	WorkerCount int

	// Timeout is the HTTP client timeout (default: 10s).
	Timeout time.Duration

	// MaxRetries is the maximum number of retry attempts (default: 3).
	MaxRetries int

	// RetryBackoff is the base backoff duration for retries (default: 1s).
	RetryBackoff time.Duration

	// MaxBackoff is the maximum backoff duration (default: 5m).
	MaxBackoff time.Duration

	// HMACSecret is the secret key for HMAC signature generation.
	HMACSecret string
}

// NewWebhookWorker creates a new WebhookWorker.
func NewWebhookWorker(cfg *Config) (*WebhookWorker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if cfg.RedisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Set defaults
	workerCount := cfg.WorkerCount
	if workerCount == 0 {
		workerCount = DefaultWorkerCount
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultMaxRetries
	}

	retryBackoff := cfg.RetryBackoff
	if retryBackoff == 0 {
		retryBackoff = DefaultRetryBackoff
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff == 0 {
		maxBackoff = DefaultMaxBackoff
	}

	return &WebhookWorker{
		redisClient:  cfg.RedisClient,
		HTTPClient:   &http.Client{Timeout: timeout},
		logger:       cfg.Logger,
		WorkerCount:  workerCount,
		MaxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		maxBackoff:   maxBackoff,
		HMACSecret:   cfg.HMACSecret,
		stopCh:       make(chan struct{}),
	}, nil
}

// Start starts the webhook worker and begins processing events.
func (w *WebhookWorker) Start(ctx context.Context) error {
	w.logger.Info("starting webhook worker",
		zap.Int("worker_count", w.WorkerCount))

	// Create consumer group if it doesn't exist
	if err := w.CreateConsumerGroup(ctx); err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Start worker goroutines
	for i := 0; i < w.WorkerCount; i++ {
		w.wg.Add(1)
		consumerName := fmt.Sprintf("worker-%d", i)
		go w.processEvents(ctx, consumerName)
	}

	// Update active workers gauge
	ActiveWorkersGauge.Set(float64(w.WorkerCount))

	w.logger.Info("webhook worker started successfully")

	// Wait for context cancellation
	<-ctx.Done()

	// Stop worker
	return w.Stop()
}

// Stop stops the webhook worker and waits for all goroutines to finish.
func (w *WebhookWorker) Stop() error {
	w.logger.Info("stopping webhook worker")

	// Signal shutdown
	close(w.stopCh)

	// Wait for all goroutines to finish
	w.wg.Wait()

	// Reset active workers gauge
	ActiveWorkersGauge.Set(0)

	w.logger.Info("webhook worker stopped")
	return nil
}

// createConsumerGroup creates the Redis Stream consumer group.
func (w *WebhookWorker) CreateConsumerGroup(ctx context.Context) error {
	// Try to create the consumer group
	err := w.redisClient.XGroupCreateMkStream(ctx, EventStreamKey, ConsumerGroup, "0").Err()
	if err != nil {
		// Ignore error if group already exists
		errMsg := err.Error()
		if errMsg != "BUSYGROUP Consumer Group name already exists" {
			return fmt.Errorf("failed to create consumer group: %w", err)
		}
		w.logger.Debug("consumer group already exists")
	} else {
		w.logger.Info("consumer group created")
	}

	return nil
}

// processEvents processes events from the Redis Stream.
func (w *WebhookWorker) processEvents(ctx context.Context, name string) {
	defer w.wg.Done()

	w.logger.Info("worker started",
		zap.String("consumer", name))

	for {
		select {
		case <-w.stopCh:
			w.logger.Info("worker stopping",
				zap.String("consumer", name))
			return
		case <-ctx.Done():
			w.logger.Info("worker context canceled",
				zap.String("consumer", name))
			return
		default:
			if err := w.ProcessNextEvent(ctx, name); err != nil {
				w.logger.Error("failed to process event",
					zap.String("consumer", name),
					zap.Error(err))
				// Brief sleep to avoid tight loop on persistent errors
				time.Sleep(1 * time.Second)
			}
		}
	}
}

// processNextEvent reads and processes the next event from the stream.
func (w *WebhookWorker) ProcessNextEvent(ctx context.Context, consumerName string) error {
	// Read from stream (blocking with timeout)
	streams, err := w.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{EventStreamKey, ">"},
		Count:    1,
		Block:    5 * time.Second,
	}).Result()

	if err != nil {
		// Timeout is expected when no events are available
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("failed to read from stream: %w", err)
	}

	// Process each message
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := w.HandleMessage(ctx, consumerName, message); err != nil {
				w.logger.Error("failed to handle message",
					zap.String("message_id", message.ID),
					zap.Error(err))
				// Continue processing other messages
			}
		}
	}

	return nil
}

// handleMessage processes a single message from the stream.
func (w *WebhookWorker) HandleMessage(ctx context.Context, _ string, msg redis.XMessage) error {
	// Parse event data
	eventData, ok := msg.Values["event"].(string)
	if !ok {
		w.logger.Error("invalid event data in message")
		// Acknowledge invalid message to remove it from pending
		return w.AcknowledgeMessage(ctx, msg.ID)
	}

	var event controllers.ResourceEvent
	if err := json.Unmarshal([]byte(eventData), &event); err != nil {
		w.logger.Error("failed to unmarshal event",
			zap.Error(err))
		// Acknowledge invalid message to remove it from pending
		return w.AcknowledgeMessage(ctx, msg.ID)
	}

	// Deliver webhook with retries
	startTime := time.Now()
	if err := w.DeliverWithRetries(ctx, &event); err != nil {
		w.logger.Error("failed to deliver webhook after retries",
			zap.String("subscription", event.SubscriptionID),
			zap.Error(err))

		// Track failed delivery
		WebhookDeliveriesTotal.WithLabelValues(event.SubscriptionID, "failed").Inc()

		// Move to dead letter queue
		if err := w.MoveToDLQ(ctx, &event, msg.ID); err != nil {
			w.logger.Error("failed to move to DLQ",
				zap.Error(err))
		}
	} else {
		// Track successful delivery and latency
		duration := time.Since(startTime).Seconds()
		WebhookDeliveriesTotal.WithLabelValues(event.SubscriptionID, "success").Inc()
		WebhookLatency.WithLabelValues(event.SubscriptionID).Observe(duration)
	}

	// Acknowledge message
	return w.AcknowledgeMessage(ctx, msg.ID)
}

// deliverWithRetries attempts webhook delivery with exponential backoff.
func (w *WebhookWorker) DeliverWithRetries(ctx context.Context, event *controllers.ResourceEvent) error {
	var lastErr error

	for attempt := 0; attempt <= w.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration (exponential)
			// #nosec G115 - attempt is bounded by maxRetries (typically ≤ 10)
			backoff := w.retryBackoff * time.Duration(1<<uint(attempt-1))
			if backoff > w.maxBackoff {
				backoff = w.maxBackoff
			}

			w.logger.Info("retrying webhook delivery",
				zap.String("subscription", event.SubscriptionID),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff))

			// Track retry
			WebhookRetriesTotal.WithLabelValues(event.SubscriptionID, fmt.Sprintf("%d", attempt)).Inc()

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context canceled during retry: %w", ctx.Err())
			}
		}

		if err := w.DeliverWebhook(ctx, event); err != nil {
			lastErr = err
			w.logger.Warn("webhook delivery failed",
				zap.String("subscription", event.SubscriptionID),
				zap.Int("attempt", attempt),
				zap.Error(err))
			continue
		}

		// Success
		w.logger.Info("webhook delivered successfully",
			zap.String("subscription", event.SubscriptionID),
			zap.Int("attempts", attempt+1))
		return nil
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// deliverWebhook delivers a webhook notification via HTTP POST.
func (w *WebhookWorker) DeliverWebhook(ctx context.Context, event *controllers.ResourceEvent) error {
	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, event.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-O2IMS-Event-Type", event.EventType)
	req.Header.Set("X-O2IMS-Notification-ID", event.NotificationID)
	req.Header.Set("X-O2IMS-Subscription-ID", event.SubscriptionID)

	// Add HMAC signature if secret is configured
	if w.HMACSecret != "" {
		signature := w.GenerateHMAC(payload)
		req.Header.Set("X-O2IMS-Signature", signature)
	}

	// Send request
	resp, err := w.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			w.logger.Warn("failed to close response body",
				zap.Error(closeErr))
		}
	}()

	// Read response body for logging
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode < DeliverySuccessStatus || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d, body: %s",
			resp.StatusCode, string(respBody))
	}

	return nil
}

// generateHMAC generates an HMAC-SHA256 signature for the payload.
func (w *WebhookWorker) GenerateHMAC(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(w.HMACSecret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// acknowledgeMessage acknowledges a message to remove it from pending.
func (w *WebhookWorker) AcknowledgeMessage(ctx context.Context, messageID string) error {
	if err := w.redisClient.XAck(ctx, EventStreamKey, ConsumerGroup, messageID).Err(); err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}
	return nil
}

// moveToDLQ moves a failed event to the dead letter queue.
func (w *WebhookWorker) MoveToDLQ(ctx context.Context, event *controllers.ResourceEvent, messageID string) error {
	// Marshal event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Add to DLQ stream
	args := &redis.XAddArgs{
		Stream: DLQStreamKey,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]interface{}{
			"event":           string(data),
			"original_id":     messageID,
			"failed_at":       time.Now().Format(time.RFC3339),
			"subscription_id": event.SubscriptionID,
		},
	}

	if _, err := w.redisClient.XAdd(ctx, args).Result(); err != nil {
		return fmt.Errorf("failed to add to DLQ: %w", err)
	}

	w.logger.Info("event moved to DLQ",
		zap.String("subscription", event.SubscriptionID),
		zap.String("message_id", messageID))

	// Track DLQ event
	DeadLetterQueueTotal.WithLabelValues(event.SubscriptionID).Inc()

	return nil
}
