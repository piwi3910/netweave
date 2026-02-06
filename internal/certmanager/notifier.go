package certmanager

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

// Certificate event type constants define the lifecycle events that trigger notifications.
const (
	// CertEventIssued indicates a new certificate was successfully issued.
	CertEventIssued = "certificate.issued"

	// CertEventRenewed indicates a certificate was successfully renewed.
	CertEventRenewed = "certificate.renewed"

	// CertEventRenewalFailed indicates a certificate renewal attempt failed.
	CertEventRenewalFailed = "certificate.renewal_failed"

	// CertEventExpiringSoon indicates a certificate will expire within the renewal window.
	CertEventExpiringSoon = "certificate.expiring_soon"

	// CertEventExpired indicates a certificate has expired.
	CertEventExpired = "certificate.expired"

	// CertEventRevoked indicates a certificate was revoked.
	CertEventRevoked = "certificate.revoked"
)

const (
	// webhookMaxRetries is the maximum number of delivery attempts for webhook notifications.
	webhookMaxRetries = 3

	// webhookRetryBackoff is the base backoff duration between retry attempts.
	webhookRetryBackoff = 1 * time.Second

	// webhookHTTPTimeout is the timeout for HTTP requests to the webhook endpoint.
	webhookHTTPTimeout = 10 * time.Second

	// hmacSignatureHeader is the HTTP header name for the HMAC signature.
	hmacSignatureHeader = "X-Signature-SHA256"
)

// CertEvent represents a certificate lifecycle event for notification purposes.
type CertEvent struct {
	// EventType identifies the type of certificate event.
	EventType string `json:"event_type"`

	// Certificate is the certificate associated with the event.
	Certificate *Certificate `json:"certificate"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Message is a human-readable description of the event.
	Message string `json:"message"`
}

// CertNotifier defines the interface for sending certificate lifecycle notifications.
type CertNotifier interface {
	// Notify sends a certificate event notification.
	// Returns an error if all delivery attempts fail.
	Notify(ctx context.Context, event CertEvent) error
}

// NotificationConfig holds configuration for certificate notifications.
type NotificationConfig struct {
	// WebhookURL is the HTTP endpoint to send notification events to.
	WebhookURL string

	// EnableAdminNotifications enables sending notifications to administrators.
	EnableAdminNotifications bool

	// EnableUserNotifications enables sending notifications to certificate owners.
	EnableUserNotifications bool

	// HMACSecret is the shared secret used for HMAC-SHA256 webhook signature verification.
	// When set, each webhook request includes an X-Signature-SHA256 header.
	HMACSecret string
}

// WebhookCertNotifier implements CertNotifier by sending HTTP POST webhooks.
type WebhookCertNotifier struct {
	config     *NotificationConfig
	httpClient *http.Client
	logger     *zap.Logger
}

// NewWebhookCertNotifier creates a new WebhookCertNotifier instance.
func NewWebhookCertNotifier(config *NotificationConfig, logger *zap.Logger) (*WebhookCertNotifier, error) {
	if config == nil {
		return nil, fmt.Errorf("notification config cannot be nil")
	}
	if config.WebhookURL == "" {
		return nil, fmt.Errorf("webhook URL is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &WebhookCertNotifier{
		config: config,
		httpClient: &http.Client{
			Timeout: webhookHTTPTimeout,
		},
		logger: logger,
	}, nil
}

// Notify sends a certificate event notification via HTTP POST webhook.
// It retries up to 3 times with 1-second backoff between attempts.
func (n *WebhookCertNotifier) Notify(ctx context.Context, event CertEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal cert event: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= webhookMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context canceled before attempt %d: %w", attempt, err)
		}

		lastErr = n.sendWebhook(ctx, payload)
		if lastErr == nil {
			n.logger.Info("Certificate notification delivered",
				zap.String("event_type", event.EventType),
				zap.Int("attempt", attempt))
			return nil
		}

		n.logger.Warn("Certificate notification delivery attempt failed",
			zap.String("event_type", event.EventType),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", webhookMaxRetries),
			zap.Error(lastErr))

		// Wait before retrying unless this is the last attempt
		if attempt < webhookMaxRetries {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled during backoff: %w", ctx.Err())
			case <-time.After(webhookRetryBackoff):
			}
		}
	}

	return fmt.Errorf("certificate notification delivery failed after %d attempts: %w", webhookMaxRetries, lastErr)
}

// sendWebhook sends a single HTTP POST request to the configured webhook URL.
func (n *WebhookCertNotifier) sendWebhook(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.config.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Netweave-CertManager/1.0")

	// Add HMAC signature if secret is configured
	if n.config.HMACSecret != "" {
		signature := computeHMACSignature(payload, n.config.HMACSecret)
		req.Header.Set(hmacSignatureHeader, signature)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			n.logger.Warn("Failed to close response body", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("webhook returned status %d, failed to read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// computeHMACSignature computes the HMAC-SHA256 signature for a payload.
func computeHMACSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
