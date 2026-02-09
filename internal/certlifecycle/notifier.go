package certlifecycle

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// CertEventType identifies the type of certificate lifecycle event.
type CertEventType string

const (
	// EventCertIssued indicates a new certificate was issued.
	EventCertIssued CertEventType = "certificate.issued"

	// EventCertRenewed indicates a certificate was successfully renewed.
	EventCertRenewed CertEventType = "certificate.renewed"

	// EventCertRenewalFailed indicates a certificate renewal attempt failed.
	EventCertRenewalFailed CertEventType = "certificate.renewal_failed"

	// EventCertExpiringSoon indicates a certificate is approaching expiry.
	EventCertExpiringSoon CertEventType = "certificate.expiring_soon"

	// EventCertExpired indicates a certificate has expired.
	EventCertExpired CertEventType = "certificate.expired"

	// EventCertRevoked indicates a certificate was revoked.
	EventCertRevoked CertEventType = "certificate.revoked"
)

const (
	// DefaultNotifierMaxRetries is the default max retry attempts for webhook delivery.
	DefaultNotifierMaxRetries = 3

	// DefaultNotifierRetryBackoff is the base backoff between retries.
	DefaultNotifierRetryBackoff = 1 * time.Second

	// DefaultNotifierTimeout is the default HTTP request timeout.
	DefaultNotifierTimeout = 10 * time.Second
)

// CertEvent represents a certificate lifecycle event for webhook notification.
type CertEvent struct {
	EventType    CertEventType `json:"event_type"`
	Timestamp    time.Time     `json:"timestamp"`
	SerialNumber string        `json:"serial_number"`
	CommonName   string        `json:"common_name"`
	TenantID     string        `json:"tenant_id"`
	UserID       string        `json:"user_id"`
	Status       string        `json:"status"`
	ExpiresAt    time.Time     `json:"expires_at"`
	RenewedFrom  string        `json:"renewed_from,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// NotifierConfig holds configuration for the webhook notifier.
type NotifierConfig struct {
	// WebhookURL is the target URL for event notifications.
	WebhookURL string

	// HMACSecret is the secret for HMAC-SHA256 signature generation.
	HMACSecret string

	// MaxRetries is the maximum number of delivery retry attempts.
	MaxRetries int

	// RetryBackoff is the base backoff duration that doubles each attempt.
	RetryBackoff time.Duration

	// Timeout is the HTTP request timeout.
	Timeout time.Duration

	// Logger provides structured logging.
	Logger *zap.Logger
}

// Notifier delivers certificate lifecycle events to a webhook endpoint.
type Notifier struct {
	webhookURL   string
	hmacSecret   string
	maxRetries   int
	retryBackoff time.Duration
	httpClient   *http.Client
	logger       *zap.Logger
}

// NewNotifier creates a new webhook notifier.
func NewNotifier(cfg *NotifierConfig) (*Notifier, error) {
	if cfg.WebhookURL == "" {
		return nil, fmt.Errorf("webhook URL is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultNotifierMaxRetries
	}

	retryBackoff := cfg.RetryBackoff
	if retryBackoff == 0 {
		retryBackoff = DefaultNotifierRetryBackoff
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultNotifierTimeout
	}

	return &Notifier{
		webhookURL:   cfg.WebhookURL,
		hmacSecret:   cfg.HMACSecret,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		httpClient:   &http.Client{Timeout: timeout},
		logger:       cfg.Logger,
	}, nil
}

// Notify sends a certificate event to the webhook endpoint without retries.
func (n *Notifier) Notify(ctx context.Context, event *CertEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Type", string(event.EventType))

	if n.hmacSecret != "" {
		signature := n.generateHMAC(payload)
		req.Header.Set("X-Signature-SHA256", signature)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			n.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Drain response body to allow connection reuse.
	if _, err := io.ReadAll(resp.Body); err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

// NotifyWithRetry sends an event with exponential backoff retries.
func (n *Notifier) NotifyWithRetry(ctx context.Context, event *CertEvent) error {
	var lastErr error

	for attempt := 0; attempt <= n.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := exponentialBackoff(n.retryBackoff, attempt)

			n.logger.Info("retrying webhook notification",
				zap.String("event_type", string(event.EventType)),
				zap.String("serial", event.SerialNumber),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff))

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context canceled during retry: %w", ctx.Err())
			}
		}

		if err := n.Notify(ctx, event); err != nil {
			lastErr = err
			n.logger.Warn("webhook notification failed",
				zap.String("event_type", string(event.EventType)),
				zap.String("serial", event.SerialNumber),
				zap.Int("attempt", attempt),
				zap.Error(err))
			continue
		}

		n.logger.Info("webhook notification delivered",
			zap.String("event_type", string(event.EventType)),
			zap.String("serial", event.SerialNumber),
			zap.Int("attempts", attempt+1))
		return nil
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// exponentialBackoff computes the backoff duration for a given retry attempt.
// Attempt 1 = base, attempt 2 = 2*base, attempt 3 = 4*base, etc.
func exponentialBackoff(base time.Duration, attempt int) time.Duration {
	backoff := base
	for i := 1; i < attempt; i++ {
		backoff *= 2
	}
	return backoff
}

// generateHMAC creates an HMAC-SHA256 signature for the payload.
func (n *Notifier) generateHMAC(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(n.hmacSecret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// NewCertEvent creates a CertEvent from certificate metadata.
func NewCertEvent(eventType CertEventType, meta *CertificateMetadata) *CertEvent {
	return &CertEvent{
		EventType:    eventType,
		Timestamp:    time.Now().UTC(),
		SerialNumber: meta.SerialNumber,
		CommonName:   meta.CommonName,
		TenantID:     meta.TenantID,
		UserID:       meta.UserID,
		Status:       string(meta.Status),
		ExpiresAt:    meta.ExpiresAt,
		RenewedFrom:  meta.RenewedFrom,
		Error:        meta.LastError,
	}
}
