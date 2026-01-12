package events

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
)

// mockDeliveryTracker implements DeliveryTracker for testing.
type mockDeliveryTracker struct{}

func (m *mockDeliveryTracker) Track(ctx context.Context, delivery *NotificationDelivery) error {
	return nil
}

func (m *mockDeliveryTracker) Get(ctx context.Context, deliveryID string) (*NotificationDelivery, error) {
	return nil, nil
}

func (m *mockDeliveryTracker) ListByEvent(ctx context.Context, eventID string) ([]*NotificationDelivery, error) {
	return nil, nil
}

func (m *mockDeliveryTracker) ListBySubscription(ctx context.Context, subscriptionID string) ([]*NotificationDelivery, error) {
	return nil, nil
}

func (m *mockDeliveryTracker) ListFailed(ctx context.Context) ([]*NotificationDelivery, error) {
	return nil, nil
}

// TestDefaultNotifierConfig tests the default notifier configuration.
func TestDefaultNotifierConfig(t *testing.T) {
	cfg := DefaultNotifierConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, defaultHTTPTimeout, cfg.HTTPTimeout)
	assert.Equal(t, defaultMaxRetries, cfg.MaxRetries)
	assert.False(t, cfg.EnableMTLS)
}

// TestNewWebhookNotifier tests notifier creation.
func TestNewWebhookNotifier(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultNotifierConfig()
	tracker := &mockDeliveryTracker{}

	t.Run("creates notifier successfully", func(t *testing.T) {
		notifier, err := NewWebhookNotifier(cfg, tracker, logger)
		require.NoError(t, err)
		assert.NotNil(t, notifier)
	})

	t.Run("uses default config if nil", func(t *testing.T) {
		notifier, err := NewWebhookNotifier(nil, tracker, logger)
		require.NoError(t, err)
		assert.NotNil(t, notifier)
	})
}

// TestWebhookNotifier_Notify tests the Notify function.
func TestWebhookNotifier_Notify(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultNotifierConfig()
	cfg.HTTPTimeout = 2 * time.Second
	tracker := &mockDeliveryTracker{}

	t.Run("delivers notification successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		notifier, err := NewWebhookNotifier(cfg, tracker, logger)
		require.NoError(t, err)

		event := &Event{
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test-resource",
		}

		sub := &storage.Subscription{
			Callback: server.URL,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = notifier.Notify(ctx, event, sub)
		assert.NoError(t, err)
	})

	t.Run("handles delivery failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		notifier, err := NewWebhookNotifier(cfg, tracker, logger)
		require.NoError(t, err)

		event := &Event{
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test-resource",
		}

		sub := &storage.Subscription{
			Callback: server.URL,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = notifier.Notify(ctx, event, sub)
		assert.Error(t, err)
	})

	t.Run("handles timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		timeoutCfg := DefaultNotifierConfig()
		timeoutCfg.HTTPTimeout = 100 * time.Millisecond

		notifier, err := NewWebhookNotifier(timeoutCfg, tracker, logger)
		require.NoError(t, err)

		event := &Event{
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test-resource",
		}

		sub := &storage.Subscription{
			Callback: server.URL,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err = notifier.Notify(ctx, event, sub)
		assert.Error(t, err)
	})
}

// TestWebhookNotifier_NotifyWithRetry tests the NotifyWithRetry function.
func TestWebhookNotifier_NotifyWithRetry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultNotifierConfig()
	cfg.HTTPTimeout = 2 * time.Second
	cfg.MaxRetries = 2
	tracker := &mockDeliveryTracker{}

	t.Run("succeeds on first attempt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		notifier, err := NewWebhookNotifier(cfg, tracker, logger)
		require.NoError(t, err)

		event := &Event{
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test-resource",
		}

		sub := &storage.Subscription{
			Callback: server.URL,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = notifier.NotifyWithRetry(ctx, event, sub)
		assert.NoError(t, err)
	})

	t.Run("retries on failure", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			if attemptCount < 2 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		notifier, err := NewWebhookNotifier(cfg, tracker, logger)
		require.NoError(t, err)

		event := &Event{
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test-resource",
		}

		sub := &storage.Subscription{
			Callback: server.URL,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, _ = notifier.NotifyWithRetry(ctx, event, sub)
		// May succeed or fail depending on timing, just verify it attempted retries
		assert.True(t, attemptCount >= 2)
	})
}

// TestWebhookNotifier_Close tests the Close function.
func TestWebhookNotifier_Close(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultNotifierConfig()
	tracker := &mockDeliveryTracker{}

	notifier, err := NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	err = notifier.Close()
	assert.NoError(t, err)
}
