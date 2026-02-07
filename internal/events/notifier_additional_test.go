package events_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/events"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
)

func TestNewWebhookNotifier_NilLogger(t *testing.T) {
	cfg := events.DefaultNotifierConfig()
	tracker := &mockDeliveryTracker{}

	_, err := events.NewWebhookNotifier(cfg, tracker, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger cannot be nil")
}

func TestNewWebhookNotifier_InsecureSkipVerify(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.InsecureSkipVerify = true
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)
	assert.NotNil(t, notifier)
}

func TestNewWebhookNotifier_InvalidMTLSCerts(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("invalid client certificate path", func(t *testing.T) {
		cfg := &events.NotifierConfig{
			HTTPTimeout:    events.DefaultHTTPTimeout,
			MaxRetries:     events.DefaultMaxRetries,
			EnableMTLS:     true,
			ClientCertFile: "/nonexistent/cert.pem",
			ClientKeyFile:  "/nonexistent/key.pem",
		}
		tracker := &mockDeliveryTracker{}

		_, err := events.NewWebhookNotifier(cfg, tracker, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create HTTP client")
	})

	t.Run("invalid CA certificate path", func(t *testing.T) {
		cfg := &events.NotifierConfig{
			HTTPTimeout: events.DefaultHTTPTimeout,
			MaxRetries:  events.DefaultMaxRetries,
			CACertFile:  "/nonexistent/ca.pem",
		}
		tracker := &mockDeliveryTracker{}

		_, err := events.NewWebhookNotifier(cfg, tracker, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create HTTP client")
	})
}

func TestWebhookNotifier_Notify_NilInputs(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("nil event", func(t *testing.T) {
		sub := &storage.Subscription{Callback: "http://example.com/cb"}
		err := notifier.Notify(ctx, nil, sub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be nil")
	})

	t.Run("nil subscription", func(t *testing.T) {
		event := &events.Event{ID: "test", Type: models.EventTypeResourceCreated}
		err := notifier.Notify(ctx, event, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription cannot be nil")
	})
}

func TestWebhookNotifier_NotifyWithRetry_NilInputs(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("nil event", func(t *testing.T) {
		sub := &storage.Subscription{Callback: "http://example.com/cb"}
		_, err := notifier.NotifyWithRetry(ctx, nil, sub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be nil")
	})

	t.Run("nil subscription", func(t *testing.T) {
		event := &events.Event{ID: "test", Type: models.EventTypeResourceCreated}
		_, err := notifier.NotifyWithRetry(ctx, event, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription cannot be nil")
	})
}

func TestWebhookNotifier_NotifyWithRetry_AllRetriesFail(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &events.NotifierConfig{
		HTTPTimeout:        2 * time.Second,
		MaxRetries:         2,
		InsecureSkipVerify: true,
	}
	tracker := &mockDeliveryTracker{}

	// Server that always returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	event := &events.Event{
		ID:         "retry-fail-event",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "resource-1",
		Timestamp:  time.Now().UTC(),
	}

	sub := &storage.Subscription{
		ID:       "sub-retry",
		Callback: server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	delivery, err := notifier.NotifyWithRetry(ctx, event, sub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delivery failed after")
	require.NotNil(t, delivery)
	assert.Equal(t, events.DeliveryStatusFailed, delivery.Status)
	assert.Equal(t, 2, delivery.Attempts)
}

func TestWebhookNotifier_NotifyWithRetry_ContextCanceled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &events.NotifierConfig{
		HTTPTimeout:        2 * time.Second,
		MaxRetries:         3,
		InsecureSkipVerify: true,
	}
	tracker := &mockDeliveryTracker{}

	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	event := &events.Event{
		ID:         "cancel-event",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "resource-1",
		Timestamp:  time.Now().UTC(),
	}

	sub := &storage.Subscription{
		ID:       "sub-cancel",
		Callback: server.URL,
	}

	// Cancel context after a short delay to hit the retry wait path
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	delivery, err := notifier.NotifyWithRetry(ctx, event, sub)
	require.Error(t, err)
	// Either "canceled" or "delivery failed" depending on timing
	require.NotNil(t, delivery)
}

func TestWebhookNotifier_NotifyWithRetry_NilTracker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &events.NotifierConfig{
		HTTPTimeout:        2 * time.Second,
		MaxRetries:         1,
		InsecureSkipVerify: true,
	}

	// Create notifier without a delivery tracker (nil tracker)
	notifier, err := events.NewWebhookNotifier(cfg, nil, logger)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := &events.Event{
		ID:         "no-tracker-event",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "resource-1",
		Timestamp:  time.Now().UTC(),
	}

	sub := &storage.Subscription{
		ID:       "sub-no-tracker",
		Callback: server.URL,
	}

	ctx := context.Background()
	delivery, err := notifier.NotifyWithRetry(ctx, event, sub)
	require.NoError(t, err)
	assert.Equal(t, events.DeliveryStatusDelivered, delivery.Status)
}

func TestWebhookNotifier_Notify_InvalidURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.InsecureSkipVerify = true
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	event := &events.Event{
		ID:         "invalid-url-event",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "resource-1",
	}

	sub := &storage.Subscription{
		ID:       "sub-invalid-url",
		Callback: "://invalid-url",
	}

	ctx := context.Background()
	err = notifier.Notify(ctx, event, sub)
	require.Error(t, err)
}

func TestWebhookNotifier_GetCircuitBreaker_Reuse(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &events.NotifierConfig{
		HTTPTimeout:        2 * time.Second,
		MaxRetries:         2,
		InsecureSkipVerify: true,
	}
	tracker := &mockDeliveryTracker{}

	// Use two different URLs to test circuit breaker creation and reuse
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	event := &events.Event{
		ID:         "cb-reuse-event",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "resource-1",
		Timestamp:  time.Now().UTC(),
	}

	ctx := context.Background()

	// First call to server1 - creates a new circuit breaker
	sub1 := &storage.Subscription{ID: "sub-1", Callback: server1.URL}
	delivery1, err := notifier.NotifyWithRetry(ctx, event, sub1)
	require.NoError(t, err)
	assert.Equal(t, events.DeliveryStatusDelivered, delivery1.Status)

	// Second call to same server1 - reuses circuit breaker
	delivery2, err := notifier.NotifyWithRetry(ctx, event, sub1)
	require.NoError(t, err)
	assert.Equal(t, events.DeliveryStatusDelivered, delivery2.Status)

	// Call to server2 - creates new circuit breaker
	sub2 := &storage.Subscription{ID: "sub-2", Callback: server2.URL}
	delivery3, err := notifier.NotifyWithRetry(ctx, event, sub2)
	require.NoError(t, err)
	assert.Equal(t, events.DeliveryStatusDelivered, delivery3.Status)
}

// trackingDeliveryTracker records all Track calls for verification.
type trackingDeliveryTracker struct {
	trackCalls []*events.NotificationDelivery
	trackErr   error
}

func (t *trackingDeliveryTracker) Track(_ context.Context, delivery *events.NotificationDelivery) error {
	if t.trackErr != nil {
		return t.trackErr
	}
	t.trackCalls = append(t.trackCalls, delivery)
	return nil
}

func (t *trackingDeliveryTracker) Get(_ context.Context, _ string) (*events.NotificationDelivery, error) {
	return nil, errors.New("not implemented")
}

func (t *trackingDeliveryTracker) ListByEvent(_ context.Context, _ string) ([]*events.NotificationDelivery, error) {
	return nil, nil
}

func (t *trackingDeliveryTracker) ListBySubscription(_ context.Context, _ string) ([]*events.NotificationDelivery, error) {
	return nil, nil
}

func (t *trackingDeliveryTracker) ListFailed(_ context.Context) ([]*events.NotificationDelivery, error) {
	return nil, nil
}

func TestWebhookNotifier_NotifyWithRetry_TrackerError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &events.NotifierConfig{
		HTTPTimeout:        2 * time.Second,
		MaxRetries:         1,
		InsecureSkipVerify: true,
	}

	tracker := &trackingDeliveryTracker{
		trackErr: errors.New("tracker error"),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	event := &events.Event{
		ID:         "tracker-error-event",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "resource-1",
		Timestamp:  time.Now().UTC(),
	}

	sub := &storage.Subscription{
		ID:       "sub-tracker-err",
		Callback: server.URL,
	}

	ctx := context.Background()
	// Should succeed even if tracker fails - tracker errors are just warnings
	delivery, err := notifier.NotifyWithRetry(ctx, event, sub)
	require.NoError(t, err)
	assert.Equal(t, events.DeliveryStatusDelivered, delivery.Status)
}

func TestWebhookNotifier_Notify_NonSuccessStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.InsecureSkipVerify = true

	tests := []struct {
		name       string
		statusCode int
	}{
		{"bad request", http.StatusBadRequest},
		{"not found", http.StatusNotFound},
		{"service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte("error body"))
			}))
			defer server.Close()

			notifier, err := events.NewWebhookNotifier(cfg, nil, logger)
			require.NoError(t, err)

			event := &events.Event{
				ID:         "status-event",
				Type:       models.EventTypeResourceCreated,
				ResourceID: "resource-1",
			}

			sub := &storage.Subscription{
				ID:       "sub-status",
				Callback: server.URL,
			}

			ctx := context.Background()
			err = notifier.Notify(ctx, event, sub)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "non-2xx status")
		})
	}
}
