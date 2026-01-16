package workers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/models"
)

func TestNewTMFEventPublisher(t *testing.T) {
	logger := zap.NewNop()

	t.Run("with default config", func(t *testing.T) {
		publisher := NewTMFEventPublisher(logger, nil)
		require.NotNil(t, publisher)
		assert.NotNil(t, publisher.client)
		assert.NotNil(t, publisher.logger)
		assert.Equal(t, 10*time.Second, publisher.timeout)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &TMFEventPublisherConfig{
			Timeout:    5 * time.Second,
			MaxRetries: 5,
			RetryDelay: 2 * time.Second,
		}
		publisher := NewTMFEventPublisher(logger, config)
		require.NotNil(t, publisher)
		assert.Equal(t, 5*time.Second, publisher.timeout)
	})
}

func TestTMFEventPublisher_PublishEvent(t *testing.T) {
	logger := zap.NewNop()
	publisher := NewTMFEventPublisher(logger, nil)

	timestamp := time.Now()
	event := &models.TMF688Event{
		ID:          "event-123",
		EventType:   "ResourceCreationNotification",
		EventTime:   &timestamp,
		Description: "Test event",
		Domain:      "O2-IMS",
		AtType:      "Event",
		Event: &models.EventPayload{
			Resource: &models.TMF639Resource{
				ID:   "res-456",
				Name: "test-resource",
			},
		},
	}

	t.Run("successful delivery", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Contains(t, r.Header.Get("User-Agent"), "O2-IMS-Gateway")

			// Verify body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var receivedEvent models.TMF688Event
			err = json.Unmarshal(body, &receivedEvent)
			require.NoError(t, err)
			assert.Equal(t, event.ID, receivedEvent.ID)
			assert.Equal(t, event.EventType, receivedEvent.EventType)

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		err := publisher.PublishEvent(ctx, server.URL, event)
		assert.NoError(t, err)
	})

	t.Run("server returns error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		ctx := context.Background()
		err := publisher.PublishEvent(ctx, server.URL, event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("invalid callback URL", func(t *testing.T) {
		ctx := context.Background()
		err := publisher.PublishEvent(ctx, "http://invalid-host-that-does-not-exist-12345.com", event)
		assert.Error(t, err)
	})

	t.Run("context cancelled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := publisher.PublishEvent(ctx, server.URL, event)
		assert.Error(t, err)
	})
}

func TestTMFEventPublisher_PublishEventWithRetry(t *testing.T) {
	logger := zap.NewNop()
	publisher := NewTMFEventPublisher(logger, nil)

	timestamp := time.Now()
	event := &models.TMF688Event{
		ID:          "event-123",
		EventType:   "ResourceCreationNotification",
		EventTime:   &timestamp,
		Description: "Test event",
		Domain:      "O2-IMS",
		AtType:      "Event",
		Event: &models.EventPayload{
			Resource: &models.TMF639Resource{
				ID:   "res-456",
				Name: "test-resource",
			},
		},
	}

	t.Run("succeeds on first attempt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		err := publisher.PublishEventWithRetry(ctx, server.URL, event, 3, 10*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("succeeds after retries", func(t *testing.T) {
		attemptCount := 0
		mu := sync.Mutex{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			attemptCount++
			currentAttempt := attemptCount
			mu.Unlock()

			if currentAttempt < 3 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		ctx := context.Background()
		err := publisher.PublishEventWithRetry(ctx, server.URL, event, 3, 10*time.Millisecond)
		assert.NoError(t, err)

		mu.Lock()
		assert.Equal(t, 3, attemptCount)
		mu.Unlock()
	})

	t.Run("fails after max retries", func(t *testing.T) {
		attemptCount := 0
		mu := sync.Mutex{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			attemptCount++
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		ctx := context.Background()
		maxRetries := 2
		err := publisher.PublishEventWithRetry(ctx, server.URL, event, maxRetries, 10*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed after")

		mu.Lock()
		assert.Equal(t, maxRetries+1, attemptCount)
		mu.Unlock()
	})

	t.Run("context cancelled during retry", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := publisher.PublishEventWithRetry(ctx, server.URL, event, 10, 100*time.Millisecond)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestTMFEventPublisher_PublishToMultipleHubs(t *testing.T) {
	logger := zap.NewNop()
	publisher := NewTMFEventPublisher(logger, nil)

	timestamp := time.Now()
	event := &models.TMF688Event{
		ID:          "event-123",
		EventType:   "ResourceCreationNotification",
		EventTime:   &timestamp,
		Description: "Test event",
		Domain:      "O2-IMS",
		AtType:      "Event",
		Event: &models.EventPayload{
			Resource: &models.TMF639Resource{
				ID:   "res-456",
				Name: "test-resource",
			},
		},
	}

	t.Run("all hubs succeed", func(t *testing.T) {
		receivedCount := 0
		mu := sync.Mutex{}

		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server1.Close()

		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server2.Close()

		server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server3.Close()

		callbacks := []string{server1.URL, server2.URL, server3.URL}
		ctx := context.Background()

		errors := publisher.PublishToMultipleHubs(ctx, callbacks, event, 2, 10*time.Millisecond)

		assert.Empty(t, errors, "expected no errors")
		mu.Lock()
		assert.Equal(t, 3, receivedCount)
		mu.Unlock()
	})

	t.Run("some hubs fail", func(t *testing.T) {
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer successServer.Close()

		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer failServer.Close()

		callbacks := []string{successServer.URL, failServer.URL}
		ctx := context.Background()

		errors := publisher.PublishToMultipleHubs(ctx, callbacks, event, 1, 10*time.Millisecond)

		assert.Len(t, errors, 1, "expected 1 error")
		assert.Contains(t, errors, failServer.URL)
		assert.NotContains(t, errors, successServer.URL)
	})

	t.Run("empty callback list", func(t *testing.T) {
		callbacks := []string{}
		ctx := context.Background()

		errors := publisher.PublishToMultipleHubs(ctx, callbacks, event, 2, 10*time.Millisecond)

		assert.Empty(t, errors)
	})
}

func TestDefaultTMFEventPublisherConfig(t *testing.T) {
	config := DefaultTMFEventPublisherConfig()
	require.NotNil(t, config)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
}
