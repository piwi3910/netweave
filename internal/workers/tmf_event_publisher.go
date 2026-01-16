// Package workers provides background workers for event processing and webhook delivery.
package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/models"
)

// TMFEventPublisher publishes TMF688 events to registered webhooks.
// It handles event delivery with retry logic and error handling.
type TMFEventPublisher struct {
	client  *http.Client
	logger  *zap.Logger
	timeout time.Duration
}

// TMFEventPublisherConfig configures the TMF event publisher.
type TMFEventPublisherConfig struct {
	// Timeout is the HTTP client timeout for webhook requests
	Timeout time.Duration

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// RetryDelay is the initial delay between retries
	RetryDelay time.Duration
}

// DefaultTMFEventPublisherConfig returns the default configuration.
func DefaultTMFEventPublisherConfig() *TMFEventPublisherConfig {
	return &TMFEventPublisherConfig{
		Timeout:    10 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
	}
}

// NewTMFEventPublisher creates a new TMF688 event publisher.
func NewTMFEventPublisher(logger *zap.Logger, config *TMFEventPublisherConfig) *TMFEventPublisher {
	if config == nil {
		config = DefaultTMFEventPublisherConfig()
	}

	return &TMFEventPublisher{
		client: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		timeout: config.Timeout,
	}
}

// PublishEvent publishes a TMF688 event to the specified callback URL.
// It returns an error if the event could not be delivered after all retries.
func (p *TMFEventPublisher) PublishEvent(ctx context.Context, callback string, event *models.TMF688Event) error {
	// Marshal event to JSON
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callback, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "O2-IMS-Gateway/1.0")

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Check response status
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned error status: %d", resp.StatusCode)
	}

	p.logger.Debug("published TMF688 event",
		zap.String("callback", callback),
		zap.String("eventId", event.ID),
		zap.String("eventType", event.EventType),
		zap.Int("statusCode", resp.StatusCode))

	return nil
}

// PublishEventWithRetry publishes a TMF688 event with exponential backoff retry logic.
func (p *TMFEventPublisher) PublishEventWithRetry(
	ctx context.Context,
	callback string,
	event *models.TMF688Event,
	maxRetries int,
	retryDelay time.Duration,
) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Try to publish
		err := p.PublishEvent(ctx, callback, event)
		if err == nil {
			// Success
			if attempt > 0 {
				p.logger.Info("published TMF688 event after retries",
					zap.String("callback", callback),
					zap.String("eventId", event.ID),
					zap.String("eventType", event.EventType),
					zap.Int("attempts", attempt+1))
			}
			return nil
		}

		lastErr = err

		// Don't retry on final attempt
		if attempt == maxRetries {
			break
		}

		// Log retry attempt
		p.logger.Warn("failed to publish TMF688 event, retrying",
			zap.String("callback", callback),
			zap.String("eventId", event.ID),
			zap.Int("attempt", attempt+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(err))

		// Wait before retry with exponential backoff
		// Calculate delay with overflow protection
		var delay time.Duration
		if attempt < 0 || attempt > 30 {
			// Cap to prevent overflow: 2^30 is already very large
			delay = retryDelay * time.Duration(1<<30)
		} else {
			// Safe: attempt is in [0, 30] range
			delay = retryDelay * (1 << attempt)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// All retries failed
	p.logger.Error("failed to publish TMF688 event after all retries",
		zap.String("callback", callback),
		zap.String("eventId", event.ID),
		zap.String("eventType", event.EventType),
		zap.Int("attempts", maxRetries+1),
		zap.Error(lastErr))

	return fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}

// PublishToMultipleHubs publishes an event to multiple hub callbacks concurrently.
// It returns a map of callback URLs to errors (only includes failed deliveries).
func (p *TMFEventPublisher) PublishToMultipleHubs(
	ctx context.Context,
	callbacks []string,
	event *models.TMF688Event,
	maxRetries int,
	retryDelay time.Duration,
) map[string]error {
	type result struct {
		callback string
		err      error
	}

	results := make(chan result, len(callbacks))

	// Publish to each callback concurrently
	for _, callback := range callbacks {
		go func(cb string) {
			err := p.PublishEventWithRetry(ctx, cb, event, maxRetries, retryDelay)
			if err != nil {
				results <- result{callback: cb, err: err}
			} else {
				results <- result{callback: cb, err: nil}
			}
		}(callback)
	}

	// Collect results
	errors := make(map[string]error)
	for i := 0; i < len(callbacks); i++ {
		res := <-results
		if res.err != nil {
			errors[res.callback] = res.err
		}
	}

	return errors
}
