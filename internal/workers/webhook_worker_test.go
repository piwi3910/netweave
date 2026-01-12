package workers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/controllers"
)

func TestNewWebhookWorker(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "nil redis client",
			cfg: &Config{
				Logger: zaptest.NewLogger(t),
			},
			wantErr: true,
			errMsg:  "redis client cannot be nil",
		},
		{
			name: "nil logger",
			cfg: &Config{
				RedisClient: &redis.Client{},
			},
			wantErr: true,
			errMsg:  "logger cannot be nil",
		},
		{
			name: "valid config with defaults",
			cfg: &Config{
				RedisClient: &redis.Client{},
				Logger:      zaptest.NewLogger(t),
			},
			wantErr: false,
		},
		{
			name: "valid config with custom values",
			cfg: &Config{
				RedisClient:  &redis.Client{},
				Logger:       zaptest.NewLogger(t),
				WorkerCount:  5,
				Timeout:      30 * time.Second,
				MaxRetries:   5,
				RetryBackoff: 2 * time.Second,
				MaxBackoff:   10 * time.Minute,
				HMACSecret:   "test-secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, err := NewWebhookWorker(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, worker)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, worker)
			assert.NotNil(t, worker.httpClient)

			if tt.cfg.WorkerCount > 0 {
				assert.Equal(t, tt.cfg.WorkerCount, worker.workerCount)
			} else {
				assert.Equal(t, DefaultWorkerCount, worker.workerCount)
			}

			if tt.cfg.MaxRetries > 0 {
				assert.Equal(t, tt.cfg.MaxRetries, worker.maxRetries)
			} else {
				assert.Equal(t, DefaultMaxRetries, worker.maxRetries)
			}
		})
	}
}

func TestWebhookWorker_DeliverWebhook_Success(t *testing.T) {
	// Setup miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Setup mock webhook server
	receivedEvents := make(chan controllers.ResourceEvent, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-O2IMS-Event-Type"))
		assert.NotEmpty(t, r.Header.Get("X-O2IMS-Notification-ID"))
		assert.NotEmpty(t, r.Header.Get("X-O2IMS-Subscription-ID"))

		// Read and parse event
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var event controllers.ResourceEvent
		err = json.Unmarshal(body, &event)
		require.NoError(t, err)

		receivedEvents <- event

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create worker
	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	// Create test event
	event := &controllers.ResourceEvent{
		SubscriptionID:   "sub-123",
		EventType:        "o2ims.Resource.Created",
		ObjectRef:        "/o2ims/v1/resources/test-node",
		ResourceTypeID:   "k8s-node",
		GlobalResourceID: "test-node",
		Timestamp:        time.Now(),
		NotificationID:   "notif-123",
		CallbackURL:      server.URL,
	}

	ctx := context.Background()

	// Deliver webhook
	err = worker.deliverWebhook(ctx, event)
	require.NoError(t, err)

	// Verify event was received
	select {
	case received := <-receivedEvents:
		assert.Equal(t, event.SubscriptionID, received.SubscriptionID)
		assert.Equal(t, event.EventType, received.EventType)
		assert.Equal(t, event.ObjectRef, received.ObjectRef)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for webhook")
	}
}

func TestWebhookWorker_DeliverWebhook_WithHMAC(t *testing.T) {
	// Setup miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	hmacSecret := "test-secret-key"

	// Setup mock webhook server that verifies HMAC
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		// Verify HMAC signature
		signature := r.Header.Get("X-O2IMS-Signature")
		assert.NotEmpty(t, signature)

		// Calculate expected signature
		mac := hmac.New(sha256.New, []byte(hmacSecret))
		mac.Write(body)
		expectedSignature := hex.EncodeToString(mac.Sum(nil))

		assert.Equal(t, expectedSignature, signature)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create worker with HMAC secret
	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
		HMACSecret:  hmacSecret,
	})
	require.NoError(t, err)

	// Create test event
	event := &controllers.ResourceEvent{
		SubscriptionID: "sub-123",
		EventType:      "o2ims.Resource.Created",
		CallbackURL:    server.URL,
	}

	ctx := context.Background()

	// Deliver webhook
	err = worker.deliverWebhook(ctx, event)
	require.NoError(t, err)
}

func TestWebhookWorker_DeliverWebhook_Failure(t *testing.T) {
	// Setup miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Setup mock webhook server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// Create worker
	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
	})
	require.NoError(t, err)

	// Create test event
	event := &controllers.ResourceEvent{
		SubscriptionID: "sub-123",
		CallbackURL:    server.URL,
	}

	ctx := context.Background()

	// Deliver webhook (should fail)
	err = worker.deliverWebhook(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestWebhookWorker_DeliverWithRetries(t *testing.T) {
	// Setup miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Track number of attempts
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			// Succeed on 3rd attempt
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Create worker with retries
	worker, err := NewWebhookWorker(&Config{
		RedisClient:  rdb,
		Logger:       zaptest.NewLogger(t),
		WorkerCount:  1,
		MaxRetries:   3,
		RetryBackoff: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	// Create test event
	event := &controllers.ResourceEvent{
		SubscriptionID: "sub-123",
		CallbackURL:    server.URL,
	}

	ctx := context.Background()

	// Deliver with retries (should succeed after retries)
	err = worker.deliverWithRetries(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestWebhookWorker_DeliverWithRetries_MaxRetriesExceeded(t *testing.T) {
	// Setup miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Setup server that always fails
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create worker with limited retries
	worker, err := NewWebhookWorker(&Config{
		RedisClient:  rdb,
		Logger:       zaptest.NewLogger(t),
		WorkerCount:  1,
		MaxRetries:   2,
		RetryBackoff: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	// Create test event
	event := &controllers.ResourceEvent{
		SubscriptionID: "sub-123",
		CallbackURL:    server.URL,
	}

	ctx := context.Background()

	// Deliver with retries (should fail after max retries)
	err = worker.deliverWithRetries(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")
	assert.Equal(t, 3, attempts) // Initial attempt + 2 retries
}

func TestWebhookWorker_GenerateHMAC(t *testing.T) {
	worker := &WebhookWorker{
		hmacSecret: "test-secret",
	}

	payload := []byte(`{"subscriptionId":"sub-123"}`)

	// Generate signature
	signature := worker.generateHMAC(payload)

	// Verify signature format
	assert.NotEmpty(t, signature)
	assert.Len(t, signature, 64) // SHA256 hex = 64 chars

	// Verify signature is deterministic
	signature2 := worker.generateHMAC(payload)
	assert.Equal(t, signature, signature2)

	// Verify different payloads produce different signatures
	differentPayload := []byte(`{"subscriptionId":"sub-456"}`)
	differentSignature := worker.generateHMAC(differentPayload)
	assert.NotEqual(t, signature, differentSignature)
}

func TestWebhookWorker_MoveToDLQ(t *testing.T) {
	// Setup miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Create worker
	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
	})
	require.NoError(t, err)

	// Create test event
	event := &controllers.ResourceEvent{
		SubscriptionID: "sub-123",
		EventType:      "o2ims.Resource.Created",
	}

	ctx := context.Background()

	// Move to DLQ
	err = worker.moveToDLQ(ctx, event, "msg-123")
	require.NoError(t, err)

	// Verify event was added to DLQ stream
	streams, err := rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{DLQStreamKey, "0"},
		Count:   1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	// Verify DLQ entry content
	msg := streams[0].Messages[0]
	assert.NotEmpty(t, msg.Values["event"])
	assert.Equal(t, "msg-123", msg.Values["original_id"])
	assert.Equal(t, "sub-123", msg.Values["subscription_id"])
	assert.NotEmpty(t, msg.Values["failed_at"])
}

// TestWebhookWorker_CreateConsumerGroup tests the createConsumerGroup function.
func TestWebhookWorker_CreateConsumerGroup(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("creates consumer group successfully", func(t *testing.T) {
		err := worker.createConsumerGroup(ctx)
		require.NoError(t, err)
	})

	t.Run("handles existing consumer group", func(t *testing.T) {
		// Group already exists from previous test
		err := worker.createConsumerGroup(ctx)
		require.NoError(t, err)
	})
}

// TestWebhookWorker_AcknowledgeMessage tests the acknowledgeMessage function.
func TestWebhookWorker_AcknowledgeMessage(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Create consumer group and add a message
	err = worker.createConsumerGroup(ctx)
	require.NoError(t, err)

	// Add a test message to the stream
	messageID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: EventStreamKey,
		Values: map[string]interface{}{
			"event": `{"subscriptionId":"sub-123"}`,
		},
	}).Result()
	require.NoError(t, err)

	// Read the message to add it to pending
	_, err = rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ConsumerGroup,
		Consumer: "test-consumer",
		Streams:  []string{EventStreamKey, ">"},
		Count:    1,
	}).Result()
	require.NoError(t, err)

	t.Run("acknowledges message successfully", func(t *testing.T) {
		err := worker.acknowledgeMessage(ctx, messageID)
		require.NoError(t, err)
	})
}

// TestWebhookWorker_HandleMessage tests the handleMessage function.
func TestWebhookWorker_HandleMessage(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Setup mock webhook server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
		MaxRetries:  1,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Create consumer group
	err = worker.createConsumerGroup(ctx)
	require.NoError(t, err)

	t.Run("handles valid message successfully", func(t *testing.T) {
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        "o2ims.Resource.Created",
			ObjectRef:        "/o2ims/v1/resources/test-node",
			ResourceTypeID:   "k8s-node",
			GlobalResourceID: "test-node",
			Timestamp:        time.Now(),
			NotificationID:   "notif-123",
			CallbackURL:      server.URL,
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Add message to stream first
		messageID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: EventStreamKey,
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}).Result()
		require.NoError(t, err)

		// Read to add to pending
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: "test-consumer-1",
			Streams:  []string{EventStreamKey, ">"},
			Count:    1,
		}).Result()
		require.NoError(t, err)
		require.Len(t, streams, 1)
		require.Len(t, streams[0].Messages, 1)

		msg := streams[0].Messages[0]
		assert.Equal(t, messageID, msg.ID)

		err = worker.handleMessage(ctx, "test-consumer-1", msg)
		require.NoError(t, err)
	})

	t.Run("handles invalid event data", func(t *testing.T) {
		// Add message with invalid data
		messageID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: EventStreamKey,
			Values: map[string]interface{}{
				"event": 12345, // Invalid type
			},
		}).Result()
		require.NoError(t, err)

		// Read to add to pending
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: "test-consumer-2",
			Streams:  []string{EventStreamKey, ">"},
			Count:    1,
		}).Result()
		require.NoError(t, err)
		require.Len(t, streams, 1)
		require.Len(t, streams[0].Messages, 1)

		msg := streams[0].Messages[0]
		assert.Equal(t, messageID, msg.ID)

		err = worker.handleMessage(ctx, "test-consumer-2", msg)
		require.NoError(t, err) // Should succeed (acknowledged)
	})

	t.Run("handles malformed JSON", func(t *testing.T) {
		// Add message with malformed JSON
		messageID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: EventStreamKey,
			Values: map[string]interface{}{
				"event": `{invalid json}`,
			},
		}).Result()
		require.NoError(t, err)

		// Read to add to pending
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: "test-consumer-3",
			Streams:  []string{EventStreamKey, ">"},
			Count:    1,
		}).Result()
		require.NoError(t, err)
		require.Len(t, streams, 1)
		require.Len(t, streams[0].Messages, 1)

		msg := streams[0].Messages[0]
		assert.Equal(t, messageID, msg.ID)

		err = worker.handleMessage(ctx, "test-consumer-3", msg)
		require.NoError(t, err) // Should succeed (acknowledged)
	})

	t.Run("handles webhook delivery failure", func(t *testing.T) {
		event := &controllers.ResourceEvent{
			SubscriptionID: "sub-123",
			CallbackURL:    "http://invalid-url-that-doesnt-exist.example.com",
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Add message to stream
		messageID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: EventStreamKey,
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}).Result()
		require.NoError(t, err)

		// Read to add to pending
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: "test-consumer-4",
			Streams:  []string{EventStreamKey, ">"},
			Count:    1,
		}).Result()
		require.NoError(t, err)
		require.Len(t, streams, 1)
		require.Len(t, streams[0].Messages, 1)

		msg := streams[0].Messages[0]
		assert.Equal(t, messageID, msg.ID)

		err = worker.handleMessage(ctx, "test-consumer-4", msg)
		require.NoError(t, err) // handleMessage returns nil, moves to DLQ
	})
}

// TestWebhookWorker_ProcessNextEvent tests the processNextEvent function.
func TestWebhookWorker_ProcessNextEvent(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Setup mock webhook server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Create consumer group
	err = worker.createConsumerGroup(ctx)
	require.NoError(t, err)

	t.Run("handles no events available", func(t *testing.T) {
		err := worker.processNextEvent(ctx, "test-consumer")
		require.NoError(t, err) // Should return nil on timeout
	})

	t.Run("processes event successfully", func(t *testing.T) {
		event := &controllers.ResourceEvent{
			SubscriptionID: "sub-123",
			EventType:      "o2ims.Resource.Created",
			CallbackURL:    server.URL,
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Add event to stream
		_, err = rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: EventStreamKey,
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}).Result()
		require.NoError(t, err)

		// Process the event
		err = worker.processNextEvent(ctx, "test-consumer")
		require.NoError(t, err)
	})
}

// TestWebhookWorker_StartStop tests the Start and Stop lifecycle.
func TestWebhookWorker_StartStop(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 2,
	})
	require.NoError(t, err)

	t.Run("starts and stops successfully", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Start worker in background
		errChan := make(chan error, 1)
		go func() {
			errChan <- worker.Start(ctx)
		}()

		// Give it time to start
		time.Sleep(200 * time.Millisecond)

		// Cancel context to trigger shutdown
		cancel()

		// Wait for Start to return
		err := <-errChan
		require.NoError(t, err)
	})
}

// TestWebhookWorker_Stop tests the Stop function directly.
func TestWebhookWorker_Stop(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	worker, err := NewWebhookWorker(&Config{
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		WorkerCount: 1,
	})
	require.NoError(t, err)

	t.Run("stops without starting", func(t *testing.T) {
		err := worker.Stop()
		require.NoError(t, err)
	})
}
