package certlifecycle_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/certlifecycle"
)

// TestNotifier_Notify verifies basic webhook delivery.
func TestNotifier_Notify(t *testing.T) {
	var receivedBody []byte
	var receivedEventType string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedEventType = r.Header.Get("X-Event-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL: server.URL,
		Logger:     logger,
	})
	require.NoError(t, err)

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertIssued,
		Timestamp:    time.Now().UTC(),
		SerialNumber: "serial-123",
		CommonName:   "test.example.com",
		TenantID:     "tenant-1",
		Status:       "active",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	err = notifier.Notify(context.Background(), event)
	require.NoError(t, err)

	assert.Equal(t, "application/json", receivedContentType)
	assert.Equal(t, "certificate.issued", receivedEventType)

	var decoded certlifecycle.CertEvent
	require.NoError(t, json.Unmarshal(receivedBody, &decoded))
	assert.Equal(t, "serial-123", decoded.SerialNumber)
	assert.Equal(t, "test.example.com", decoded.CommonName)
}

// TestNotifier_HMACSignature verifies HMAC-SHA256 signing.
func TestNotifier_HMACSignature(t *testing.T) {
	secret := "test-hmac-secret"
	var receivedSignature string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Signature-SHA256")
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL: server.URL,
		HMACSecret: secret,
		Logger:     logger,
	})
	require.NoError(t, err)

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertRevoked,
		SerialNumber: "serial-456",
	}

	err = notifier.Notify(context.Background(), event)
	require.NoError(t, err)

	// Verify HMAC signature.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(receivedBody)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	assert.Equal(t, expectedSig, receivedSignature)
	assert.NotEmpty(t, receivedSignature)
}

// TestNotifier_NoHMACWhenSecretEmpty verifies no signature header without secret.
func TestNotifier_NoHMACWhenSecretEmpty(t *testing.T) {
	var receivedSignature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Signature-SHA256")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL: server.URL,
		Logger:     logger,
	})
	require.NoError(t, err)

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertIssued,
		SerialNumber: "serial-789",
	}

	err = notifier.Notify(context.Background(), event)
	require.NoError(t, err)
	assert.Empty(t, receivedSignature)
}

// TestNotifier_NonSuccessStatus verifies error on non-2xx response.
func TestNotifier_NonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL: server.URL,
		Logger:     logger,
	})
	require.NoError(t, err)

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertExpired,
		SerialNumber: "serial-fail",
	}

	err = notifier.Notify(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-2xx status: 500")
}

// TestNotifier_NotifyWithRetry verifies retry behavior on transient failures.
func TestNotifier_NotifyWithRetry(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := attempts.Add(1)
		if count <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL:   server.URL,
		MaxRetries:   3,
		RetryBackoff: 10 * time.Millisecond,
		Logger:       logger,
	})
	require.NoError(t, err)

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertRenewed,
		SerialNumber: "serial-retry",
	}

	err = notifier.NotifyWithRetry(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

// TestNotifier_NotifyWithRetryExhausted verifies error after all retries fail.
func TestNotifier_NotifyWithRetryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL:   server.URL,
		MaxRetries:   2,
		RetryBackoff: 10 * time.Millisecond,
		Logger:       logger,
	})
	require.NoError(t, err)

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertRenewalFailed,
		SerialNumber: "serial-exhausted",
	}

	err = notifier.NotifyWithRetry(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")
}

// TestNotifier_ContextCancellation verifies retry stops on context cancellation.
func TestNotifier_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
		WebhookURL:   server.URL,
		MaxRetries:   10,
		RetryBackoff: 5 * time.Second, // Long backoff so cancellation triggers first.
		Logger:       logger,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	event := &certlifecycle.CertEvent{
		EventType:    certlifecycle.EventCertExpiringSoon,
		SerialNumber: "serial-cancel",
	}

	err = notifier.NotifyWithRetry(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// TestNewNotifier_Validation verifies constructor validation.
func TestNewNotifier_Validation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("missing webhook URL", func(t *testing.T) {
		_, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
			Logger: logger,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webhook URL")
	})

	t.Run("missing logger", func(t *testing.T) {
		_, err := certlifecycle.NewNotifier(&certlifecycle.NotifierConfig{
			WebhookURL: "https://example.com/webhook",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logger")
	})
}

// TestNewCertEvent verifies the event constructor helper.
func TestNewCertEvent(t *testing.T) {
	meta := &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-100",
		CommonName:   "app.example.com",
		TenantID:     "tenant-1",
		UserID:       "user-1",
		Status:       certlifecycle.StatusExpiringSoon,
		ExpiresAt:    time.Now().Add(5 * 24 * time.Hour),
		RenewedFrom:  "serial-99",
		LastError:    "renewal failed",
	}

	event := certlifecycle.NewCertEvent(certlifecycle.EventCertExpiringSoon, meta)

	assert.Equal(t, certlifecycle.EventCertExpiringSoon, event.EventType)
	assert.Equal(t, "serial-100", event.SerialNumber)
	assert.Equal(t, "app.example.com", event.CommonName)
	assert.Equal(t, "tenant-1", event.TenantID)
	assert.Equal(t, "user-1", event.UserID)
	assert.Equal(t, "expiring_soon", event.Status)
	assert.Equal(t, "serial-99", event.RenewedFrom)
	assert.Equal(t, "renewal failed", event.Error)
	assert.False(t, event.Timestamp.IsZero())
}
