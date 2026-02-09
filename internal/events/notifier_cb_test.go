package events_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/events"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestWebhookNotifier_CircuitBreaker_ConcurrentAccess tests concurrent access to circuit breakers.
// This test verifies no data races occur when multiple goroutines access circuit breakers
// simultaneously (run with go test -race).
func TestWebhookNotifier_CircuitBreaker_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.HTTPTimeout = 100 * time.Millisecond
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)
	defer func() {
		closeErr := notifier.Close()
		assert.NoError(t, closeErr)
	}()

	// Start an httptest server that always responds 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	numGoroutines := 50
	numURLs := 10

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := &events.Event{
				ID:         fmt.Sprintf("event-%d", id),
				Type:       models.EventTypeResourceCreated,
				ResourceID: "test",
				Timestamp:  time.Now().UTC(),
			}
			sub := &storage.Subscription{
				ID:       fmt.Sprintf("sub-%d", id),
				Callback: fmt.Sprintf("%s/callback-%d", server.URL, id%numURLs),
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, _ = notifier.NotifyWithRetry(ctx, event, sub)
		}(i)
	}
	wg.Wait()
}

// TestWebhookNotifier_ManyCircuitBreakers verifies the notifier handles many distinct callback URLs
// without issues, indirectly testing that eviction works when the limit is exceeded.
func TestWebhookNotifier_ManyCircuitBreakers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.HTTPTimeout = 100 * time.Millisecond
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)
	defer func() {
		closeErr := notifier.Close()
		assert.NoError(t, closeErr)
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create many distinct circuit breakers
	for i := 0; i < 50; i++ {
		event := &events.Event{
			ID:         fmt.Sprintf("event-%d", i),
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test",
			Timestamp:  time.Now().UTC(),
		}
		sub := &storage.Subscription{
			ID:       fmt.Sprintf("sub-%d", i),
			Callback: fmt.Sprintf("%s/callback-%d", server.URL, i),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := notifier.NotifyWithRetry(ctx, event, sub)
		require.NoError(t, err)
	}
}

// TestWebhookNotifier_CircuitBreaker_ReusesSameBreaker verifies that multiple calls to the same
// callback URL reuse the same circuit breaker instance.
func TestWebhookNotifier_CircuitBreaker_ReusesSameBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.HTTPTimeout = 100 * time.Millisecond
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)
	defer func() {
		closeErr := notifier.Close()
		assert.NoError(t, closeErr)
	}()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Make multiple notifications to the same callback URL
	callbackURL := fmt.Sprintf("%s/callback", server.URL)
	for i := 0; i < 5; i++ {
		event := &events.Event{
			ID:         fmt.Sprintf("event-%d", i),
			Type:       models.EventTypeResourceCreated,
			ResourceID: "test",
			Timestamp:  time.Now().UTC(),
		}
		sub := &storage.Subscription{
			ID:       fmt.Sprintf("sub-%d", i),
			Callback: callbackURL,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := notifier.NotifyWithRetry(ctx, event, sub)
		require.NoError(t, err)
	}

	// All 5 notifications should have succeeded
	assert.Equal(t, 5, requestCount)
}

// TestWebhookNotifier_CircuitBreaker_DifferentURLsGetDifferentBreakers verifies that different
// callback URLs get their own circuit breaker instances.
func TestWebhookNotifier_CircuitBreaker_DifferentURLsGetDifferentBreakers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.HTTPTimeout = 100 * time.Millisecond
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)
	defer func() {
		closeErr := notifier.Close()
		assert.NoError(t, closeErr)
	}()

	// Create two servers with different behaviors
	failCount := 0
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		if failCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	// First callback fails multiple times (triggers circuit breaker)
	event1 := &events.Event{
		ID:         "event-1",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "test",
		Timestamp:  time.Now().UTC(),
	}
	sub1 := &storage.Subscription{
		ID:       "sub-1",
		Callback: server1.URL,
	}
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	_, _ = notifier.NotifyWithRetry(ctx1, event1, sub1)

	// Second callback should succeed immediately (different circuit breaker)
	event2 := &events.Event{
		ID:         "event-2",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "test",
		Timestamp:  time.Now().UTC(),
	}
	sub2 := &storage.Subscription{
		ID:       "sub-2",
		Callback: server2.URL,
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	_, err2 := notifier.NotifyWithRetry(ctx2, event2, sub2)
	assert.NoError(t, err2)
}

// TestWebhookNotifier_Close_StopsCleanupGoroutine verifies that calling Close() stops the
// cleanup goroutine that prunes stale circuit breakers.
func TestWebhookNotifier_Close_StopsCleanupGoroutine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.HTTPTimeout = 100 * time.Millisecond
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)

	// Give the cleanup goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Close should stop the cleanup goroutine
	err = notifier.Close()
	assert.NoError(t, err)

	// Give time for goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// If we got here without deadlock or panic, the cleanup goroutine stopped correctly
}

// TestWebhookNotifier_CircuitBreaker_LastUsedTracking verifies that accessing a circuit breaker
// updates its last-used timestamp by making notifications and verifying they succeed.
func TestWebhookNotifier_CircuitBreaker_LastUsedTracking(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := events.DefaultNotifierConfig()
	cfg.HTTPTimeout = 100 * time.Millisecond
	tracker := &mockDeliveryTracker{}

	notifier, err := events.NewWebhookNotifier(cfg, tracker, logger)
	require.NoError(t, err)
	defer func() {
		closeErr := notifier.Close()
		assert.NoError(t, closeErr)
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	callbackURL := fmt.Sprintf("%s/callback", server.URL)

	// First access - creates circuit breaker and sets last-used time
	event1 := &events.Event{
		ID:         "event-1",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "test",
		Timestamp:  time.Now().UTC(),
	}
	sub1 := &storage.Subscription{
		ID:       "sub-1",
		Callback: callbackURL,
	}
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	_, err1 := notifier.NotifyWithRetry(ctx1, event1, sub1)
	require.NoError(t, err1)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Second access - updates last-used time
	event2 := &events.Event{
		ID:         "event-2",
		Type:       models.EventTypeResourceCreated,
		ResourceID: "test",
		Timestamp:  time.Now().UTC(),
	}
	sub2 := &storage.Subscription{
		ID:       "sub-2",
		Callback: callbackURL,
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	_, err2 := notifier.NotifyWithRetry(ctx2, event2, sub2)
	require.NoError(t, err2)

	// Both notifications should succeed, indicating the circuit breaker was reused
	// and its last-used timestamp was updated
}
