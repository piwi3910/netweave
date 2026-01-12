package events

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
)

const (
	// Default timeout for HTTP requests.
	defaultHTTPTimeout = 10 * time.Second

	// Default maximum retries.
	defaultMaxRetries = 3

	// Initial retry backoff.
	initialBackoff = 1 * time.Second

	// Maximum retry backoff.
	maxBackoff = 60 * time.Second

	// Backoff multiplier.
	backoffMultiplier = 2
)

// NotifierConfig holds configuration for the webhook notifier.
type NotifierConfig struct {
	// HTTPTimeout is the timeout for HTTP requests
	HTTPTimeout time.Duration

	// MaxRetries is the maximum number of delivery attempts
	MaxRetries int

	// EnableMTLS enables mutual TLS for webhook delivery
	EnableMTLS bool

	// ClientCertFile is the path to the client certificate for mTLS
	ClientCertFile string

	// ClientKeyFile is the path to the client private key for mTLS
	ClientKeyFile string

	// CACertFile is the path to the CA certificate for verifying server certificates
	CACertFile string

	// InsecureSkipVerify disables certificate verification (for testing only)
	InsecureSkipVerify bool
}

// DefaultNotifierConfig returns a NotifierConfig with sensible defaults.
func DefaultNotifierConfig() *NotifierConfig {
	return &NotifierConfig{
		HTTPTimeout:        defaultHTTPTimeout,
		MaxRetries:         defaultMaxRetries,
		EnableMTLS:         false,
		InsecureSkipVerify: false,
	}
}

// WebhookNotifier implements the Notifier interface using HTTP webhooks.
type WebhookNotifier struct {
	config          *NotifierConfig
	httpClient      *http.Client
	logger          *zap.Logger
	deliveryTracker DeliveryTracker
	circuitBreakers map[string]*gobreaker.CircuitBreaker
}

// NewWebhookNotifier creates a new WebhookNotifier instance.
func NewWebhookNotifier(config *NotifierConfig, deliveryTracker DeliveryTracker, logger *zap.Logger) (*WebhookNotifier, error) {
	if config == nil {
		config = DefaultNotifierConfig()
	}
	if logger == nil {
		return nil, errors.New("logger cannot be nil")
	}

	// Log security warning if InsecureSkipVerify is enabled
	if config.InsecureSkipVerify {
		logger.Warn("SECURITY WARNING: TLS certificate verification is disabled for webhook delivery. "+
			"This should ONLY be used in development/testing environments. "+
			"Production deployments MUST use proper certificate validation to prevent man-in-the-middle attacks.",
			zap.Bool("insecure_skip_verify", true))
	}

	// Create HTTP client with optional mTLS
	httpClient, err := createHTTPClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	return &WebhookNotifier{
		config:          config,
		httpClient:      httpClient,
		logger:          logger,
		deliveryTracker: deliveryTracker,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
	}, nil
}

// createHTTPClient creates an HTTP client with optional mTLS configuration.
// WARNING: InsecureSkipVerify disables certificate validation and should only be used in development/testing.
// Production deployments must use proper certificate validation (InsecureSkipVerify=false).
// This security control prevents man-in-the-middle attacks by ensuring webhook endpoints present valid certificates.
func createHTTPClient(config *NotifierConfig) (*http.Client, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Only skip verification if explicitly configured (for development/testing only)
	// Production deployments must validate certificates
	if config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	// Load client certificate for mTLS
	if config.EnableMTLS && config.ClientCertFile != "" && config.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate
	if config.CACertFile != "" {
		caCert, err := os.ReadFile(config.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   config.HTTPTimeout,
	}, nil
}

// Notify sends a notification to a subscriber's callback URL.
func (n *WebhookNotifier) Notify(ctx context.Context, event *Event, subscription *storage.Subscription) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}
	if subscription == nil {
		return errors.New("subscription cannot be nil")
	}

	// Build notification payload
	notification := n.buildNotification(event, subscription)

	// Send HTTP POST request
	return n.sendWebhook(ctx, subscription.Callback, notification)
}

// NotifyWithRetry sends a notification with automatic retry logic.
func (n *WebhookNotifier) NotifyWithRetry(ctx context.Context, event *Event, subscription *storage.Subscription) (*NotificationDelivery, error) {
	if event == nil {
		return nil, errors.New("event cannot be nil")
	}
	if subscription == nil {
		return nil, errors.New("subscription cannot be nil")
	}

	// Create delivery tracking record
	delivery := &NotificationDelivery{
		ID:             uuid.New().String(),
		EventID:        event.ID,
		SubscriptionID: subscription.ID,
		CallbackURL:    subscription.Callback,
		Status:         DeliveryStatusPending,
		Attempts:       0,
		MaxAttempts:    n.config.MaxRetries,
		CreatedAt:      time.Now().UTC(),
	}

	// Build notification payload
	notification := n.buildNotification(event, subscription)

	// Get or create circuit breaker for this callback URL
	cb := n.getCircuitBreaker(subscription.Callback)

	// Attempt delivery with retries
	backoff := initialBackoff
	for attempt := 1; attempt <= n.config.MaxRetries; attempt++ {
		// Attempt delivery
		err := n.attemptDelivery(ctx, delivery, subscription, cb, notification, attempt)

		// Handle success
		if err == nil {
			return n.handleDeliverySuccess(ctx, delivery, subscription, attempt)
		}

		// Handle failure (including final failure)
		if attempt >= n.config.MaxRetries {
			return n.handleFinalFailure(ctx, delivery, subscription, attempt, err)
		}

		// Prepare for retry
		if retryErr := n.prepareRetry(ctx, delivery, subscription, attempt, err, backoff); retryErr != nil {
			return delivery, retryErr
		}

		// Increase backoff for next attempt
		backoff *= backoffMultiplier
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return delivery, errors.New("unexpected end of retry loop")
}

// attemptDelivery attempts a single notification delivery.
func (n *WebhookNotifier) attemptDelivery(
	ctx context.Context,
	delivery *NotificationDelivery,
	subscription *storage.Subscription,
	cb *gobreaker.CircuitBreaker,
	notification *models.Notification,
	attempt int,
) error {
	delivery.Attempts = attempt
	delivery.LastAttemptAt = time.Now().UTC()
	delivery.Status = DeliveryStatusDelivering

	// Track attempt
	if n.deliveryTracker != nil {
		if err := n.deliveryTracker.Track(ctx, delivery); err != nil {
			n.logger.Warn("failed to track delivery attempt", zap.Error(err))
		}
	}

	// Execute with circuit breaker
	startTime := time.Now()
	err := n.executeWithCircuitBreaker(ctx, cb, subscription.Callback, notification)
	responseTime := time.Since(startTime).Milliseconds()

	delivery.ResponseTime = responseTime

	// Record metrics (use 0 for status code if not set)
	statusCode := "0"
	if delivery.HTTPStatusCode > 0 {
		statusCode = fmt.Sprintf("%d", delivery.HTTPStatusCode)
	}
	RecordNotificationResponseTime(subscription.ID, statusCode, float64(responseTime))

	return err
}

// handleDeliverySuccess handles a successful notification delivery.
func (n *WebhookNotifier) handleDeliverySuccess(
	ctx context.Context,
	delivery *NotificationDelivery,
	subscription *storage.Subscription,
	attempt int,
) (*NotificationDelivery, error) {
	delivery.Status = DeliveryStatusDelivered
	delivery.HTTPStatusCode = http.StatusOK
	delivery.CompletedAt = time.Now().UTC()

	// Record success metrics
	duration := time.Since(delivery.CreatedAt).Seconds()
	RecordNotificationDelivered("success", subscription.ID, duration, attempt)

	n.logger.Info("notification delivered successfully",
		zap.String("delivery_id", delivery.ID),
		zap.String("subscription_id", subscription.ID),
		zap.String("callback", subscription.Callback),
		zap.Int("attempts", attempt),
		zap.Int64("response_time_ms", delivery.ResponseTime),
	)

	if n.deliveryTracker != nil {
		if err := n.deliveryTracker.Track(ctx, delivery); err != nil {
			n.logger.Warn("failed to track successful delivery", zap.Error(err))
		}
	}

	return delivery, nil
}

// handleFinalFailure handles the final delivery failure after all retries exhausted.
func (n *WebhookNotifier) handleFinalFailure(
	ctx context.Context,
	delivery *NotificationDelivery,
	subscription *storage.Subscription,
	attempt int,
	err error,
) (*NotificationDelivery, error) {
	delivery.LastError = err.Error()
	delivery.Status = DeliveryStatusFailed
	delivery.CompletedAt = time.Now().UTC()

	// Record failure metrics
	duration := time.Since(delivery.CreatedAt).Seconds()
	RecordNotificationDelivered("failed", subscription.ID, duration, attempt)

	n.logger.Error("notification delivery failed after all retries",
		zap.String("delivery_id", delivery.ID),
		zap.String("subscription_id", subscription.ID),
		zap.String("callback", subscription.Callback),
		zap.Int("attempts", attempt),
		zap.Error(err),
	)

	if n.deliveryTracker != nil {
		if trackErr := n.deliveryTracker.Track(ctx, delivery); trackErr != nil {
			n.logger.Warn("failed to track failed delivery", zap.Error(trackErr))
		}
	}

	return delivery, fmt.Errorf("delivery failed after %d attempts: %w", attempt, err)
}

// prepareRetry prepares for the next delivery retry.
func (n *WebhookNotifier) prepareRetry(
	ctx context.Context,
	delivery *NotificationDelivery,
	subscription *storage.Subscription,
	attempt int,
	err error,
	backoff time.Duration,
) error {
	delivery.LastError = err.Error()
	delivery.Status = DeliveryStatusRetrying
	delivery.NextAttemptAt = time.Now().Add(backoff)

	n.logger.Warn("notification delivery failed",
		zap.String("delivery_id", delivery.ID),
		zap.String("subscription_id", subscription.ID),
		zap.String("callback", subscription.Callback),
		zap.Int("attempt", attempt),
		zap.Int("max_attempts", n.config.MaxRetries),
		zap.Error(err),
	)

	if n.deliveryTracker != nil {
		if trackErr := n.deliveryTracker.Track(ctx, delivery); trackErr != nil {
			n.logger.Warn("failed to track retry delivery", zap.Error(trackErr))
		}
	}

	// Wait before retry
	select {
	case <-ctx.Done():
		delivery.Status = DeliveryStatusFailed
		delivery.CompletedAt = time.Now().UTC()
		return fmt.Errorf("notification delivery canceled: %w", ctx.Err())
	case <-time.After(backoff):
	}

	return nil
}

// buildNotification builds the O2-IMS notification payload.
func (n *WebhookNotifier) buildNotification(event *Event, subscription *storage.Subscription) *models.Notification {
	return &models.Notification{
		SubscriptionID:         subscription.ID,
		ConsumerSubscriptionID: subscription.ConsumerSubscriptionID,
		EventType:              string(event.Type),
		Resource:               event.Resource,
		Timestamp:              event.Timestamp,
		Extensions:             event.Extensions,
	}
}

// sendWebhook sends an HTTP POST request to the webhook URL.
func (n *WebhookNotifier) sendWebhook(ctx context.Context, callbackURL string, notification *models.Notification) error {
	// Serialize notification
	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "O2-IMS-Gateway/1.0")

	// Send request
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			n.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("webhook returned non-2xx status: %d, failed to read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("webhook returned non-2xx status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// executeWithCircuitBreaker executes a webhook delivery with circuit breaker protection.
func (n *WebhookNotifier) executeWithCircuitBreaker(
	ctx context.Context,
	cb *gobreaker.CircuitBreaker,
	callbackURL string,
	notification *models.Notification,
) error {
	_, err := cb.Execute(func() (interface{}, error) {
		return nil, n.sendWebhook(ctx, callbackURL, notification)
	})
	if err != nil {
		return fmt.Errorf("circuit breaker execution failed: %w", err)
	}
	return nil
}

// getCircuitBreaker gets or creates a circuit breaker for a callback URL.
func (n *WebhookNotifier) getCircuitBreaker(callbackURL string) *gobreaker.CircuitBreaker {
	if cb, ok := n.circuitBreakers[callbackURL]; ok {
		return cb
	}

	// Create new circuit breaker
	settings := gobreaker.Settings{
		Name:        callbackURL,
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Open circuit after 3 consecutive failures
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			n.logger.Info("circuit breaker state changed",
				zap.String("callback", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
			// Record circuit breaker state: 0=closed, 1=half-open, 2=open
			var state float64
			switch to {
			case gobreaker.StateClosed:
				state = 0
			case gobreaker.StateHalfOpen:
				state = 1
			case gobreaker.StateOpen:
				state = 2
			}
			RecordCircuitBreakerState(name, state)
		},
	}

	cb := gobreaker.NewCircuitBreaker(settings)
	n.circuitBreakers[callbackURL] = cb

	return cb
}

// Close closes the notifier and releases resources.
func (n *WebhookNotifier) Close() error {
	n.httpClient.CloseIdleConnections()
	return nil
}
