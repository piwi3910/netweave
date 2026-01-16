package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/controllers"
	"github.com/piwi3910/netweave/internal/storage"
)

func TestNewTMFEventListener(t *testing.T) {
	logger := zap.NewNop()
	mockRedis := miniredis.RunT(t)
	defer mockRedis.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mockRedis.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	hubStore := storage.NewInMemoryHubStore()
	publisher := NewTMFEventPublisher(logger, nil)

	t.Run("valid config", func(t *testing.T) {
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)
		require.NotNil(t, listener)
		assert.Equal(t, "https://gateway.example.com", listener.baseURL)
	})

	t.Run("nil config", func(t *testing.T) {
		listener, err := NewTMFEventListener(nil)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("nil redis client", func(t *testing.T) {
		cfg := &TMFEventListenerConfig{
			RedisClient: nil,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.Contains(t, err.Error(), "redis client cannot be nil")
	})

	t.Run("nil hub store", func(t *testing.T) {
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    nil,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.Contains(t, err.Error(), "hub store cannot be nil")
	})

	t.Run("nil publisher", func(t *testing.T) {
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   nil,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.Contains(t, err.Error(), "publisher cannot be nil")
	})

	t.Run("nil logger", func(t *testing.T) {
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      nil,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.Contains(t, err.Error(), "logger cannot be nil")
	})

	t.Run("empty base URL", func(t *testing.T) {
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "",
		}

		listener, err := NewTMFEventListener(cfg)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.Contains(t, err.Error(), "base URL cannot be empty")
	})
}

func TestTMFEventListener_ProcessMessage(t *testing.T) {
	logger := zap.NewNop()
	mockRedis := miniredis.RunT(t)
	defer mockRedis.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mockRedis.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	hubStore := storage.NewInMemoryHubStore()
	publisher := NewTMFEventPublisher(logger, nil)

	ctx := context.Background()

	t.Run("event delivered to matching hub", func(t *testing.T) {
		// Setup test server
		receivedEvents := 0
		mu := sync.Mutex{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedEvents++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Register hub
		hub := &storage.HubRegistration{
			HubID:          "hub-1",
			Callback:       server.URL,
			Query:          "resourceId=res-456",
			SubscriptionID: "sub-123",
			CreatedAt:      time.Now(),
		}
		err := hubStore.Create(ctx, hub)
		require.NoError(t, err)

		// Create listener
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)

		// Create event
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        string(controllers.EventTypeCreated),
			ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "pool-1",
			GlobalResourceID: "res-456",
			Timestamp:        time.Now(),
			NotificationID:   "notif-abc",
			CallbackURL:      server.URL,
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Create Redis message
		message := redis.XMessage{
			ID: "1234567890-0",
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}

		// Ensure consumer group exists
		err = listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)

		// Process message
		err = listener.processMessage(ctx, message)
		require.NoError(t, err)

		// Verify event was delivered
		mu.Lock()
		assert.Equal(t, 1, receivedEvents)
		mu.Unlock()

		// Cleanup
		err = hubStore.Delete(ctx, hub.HubID)
		require.NoError(t, err)
	})

	t.Run("event not delivered to non-matching hub", func(t *testing.T) {
		receivedEvents := 0
		mu := sync.Mutex{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedEvents++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Register hub with non-matching filter
		hub := &storage.HubRegistration{
			HubID:          "hub-2",
			Callback:       server.URL,
			Query:          "resourceId=res-999", // Different resource ID
			SubscriptionID: "sub-123",
			CreatedAt:      time.Now(),
		}
		err := hubStore.Create(ctx, hub)
		require.NoError(t, err)

		// Create listener
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)

		// Create event
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        string(controllers.EventTypeCreated),
			ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "pool-1",
			GlobalResourceID: "res-456",
			Timestamp:        time.Now(),
			NotificationID:   "notif-abc",
			CallbackURL:      server.URL,
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Create Redis message
		message := redis.XMessage{
			ID: "1234567891-0",
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}

		// Ensure consumer group exists
		err = listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)

		// Process message
		err = listener.processMessage(ctx, message)
		require.NoError(t, err)

		// Verify event was NOT delivered
		mu.Lock()
		assert.Equal(t, 0, receivedEvents)
		mu.Unlock()

		// Cleanup
		err = hubStore.Delete(ctx, hub.HubID)
		require.NoError(t, err)
	})

	t.Run("event delivered to multiple matching hubs", func(t *testing.T) {
		receivedCount := 0
		mu := sync.Mutex{}

		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server1.Close()

		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer server2.Close()

		// Register multiple hubs
		hub1 := &storage.HubRegistration{
			HubID:          "hub-3",
			Callback:       server1.URL,
			Query:          "resourcePoolId=pool-1",
			SubscriptionID: "sub-123",
			CreatedAt:      time.Now(),
		}
		err := hubStore.Create(ctx, hub1)
		require.NoError(t, err)

		hub2 := &storage.HubRegistration{
			HubID:          "hub-4",
			Callback:       server2.URL,
			Query:          "", // Matches all
			SubscriptionID: "sub-124",
			CreatedAt:      time.Now(),
		}
		err = hubStore.Create(ctx, hub2)
		require.NoError(t, err)

		// Create listener
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)

		// Create event
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        string(controllers.EventTypeCreated),
			ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "pool-1",
			GlobalResourceID: "res-456",
			Timestamp:        time.Now(),
			NotificationID:   "notif-def",
			CallbackURL:      server1.URL,
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Create Redis message
		message := redis.XMessage{
			ID: "1234567892-0",
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}

		// Ensure consumer group exists
		err = listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)

		// Process message
		err = listener.processMessage(ctx, message)
		require.NoError(t, err)

		// Verify event was delivered to both hubs
		mu.Lock()
		assert.Equal(t, 2, receivedCount)
		mu.Unlock()

		// Cleanup
		err = hubStore.Delete(ctx, hub1.HubID)
		require.NoError(t, err)
		err = hubStore.Delete(ctx, hub2.HubID)
		require.NoError(t, err)
	})

	t.Run("no hubs registered", func(t *testing.T) {
		// Create listener
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)

		// Create event
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        string(controllers.EventTypeCreated),
			ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "pool-1",
			GlobalResourceID: "res-456",
			Timestamp:        time.Now(),
			NotificationID:   "notif-ghi",
			CallbackURL:      "https://example.com/callback",
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Create Redis message
		message := redis.XMessage{
			ID: "1234567893-0",
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}

		// Ensure consumer group exists
		err = listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)

		// Process message - should succeed even with no hubs
		err = listener.processMessage(ctx, message)
		require.NoError(t, err)
	})

	t.Run("partial delivery failure", func(t *testing.T) {
		receivedCount := 0
		mu := sync.Mutex{}

		// Success server
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		defer successServer.Close()

		// Failure server
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer failServer.Close()

		// Register hubs
		hub1 := &storage.HubRegistration{
			HubID:          "hub-5",
			Callback:       successServer.URL,
			Query:          "",
			SubscriptionID: "sub-125",
			CreatedAt:      time.Now(),
		}
		err := hubStore.Create(ctx, hub1)
		require.NoError(t, err)

		hub2 := &storage.HubRegistration{
			HubID:          "hub-6",
			Callback:       failServer.URL,
			Query:          "",
			SubscriptionID: "sub-126",
			CreatedAt:      time.Now(),
		}
		err = hubStore.Create(ctx, hub2)
		require.NoError(t, err)

		// Create listener
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)

		// Create event
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-125",
			EventType:        string(controllers.EventTypeCreated),
			ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "pool-1",
			GlobalResourceID: "res-456",
			Timestamp:        time.Now(),
			NotificationID:   "notif-jkl",
			CallbackURL:      successServer.URL,
		}

		eventData, err := json.Marshal(event)
		require.NoError(t, err)

		// Create Redis message
		message := redis.XMessage{
			ID: "1234567894-0",
			Values: map[string]interface{}{
				"event": string(eventData),
			},
		}

		// Ensure consumer group exists
		err = listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)

		// Process message - should still succeed even with partial failure
		err = listener.processMessage(ctx, message)
		require.NoError(t, err)

		// Verify successful delivery count
		mu.Lock()
		assert.Equal(t, 1, receivedCount)
		mu.Unlock()

		// Cleanup
		err = hubStore.Delete(ctx, hub1.HubID)
		require.NoError(t, err)
		err = hubStore.Delete(ctx, hub2.HubID)
		require.NoError(t, err)
	})

	t.Run("invalid event data", func(t *testing.T) {
		// Create listener
		cfg := &TMFEventListenerConfig{
			RedisClient: redisClient,
			HubStore:    hubStore,
			Publisher:   publisher,
			Logger:      logger,
			BaseURL:     "https://gateway.example.com",
		}

		listener, err := NewTMFEventListener(cfg)
		require.NoError(t, err)

		// Create message with invalid data
		message := redis.XMessage{
			ID: "1234567895-0",
			Values: map[string]interface{}{
				"event": "invalid json data",
			},
		}

		// Ensure consumer group exists
		err = listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)

		// Process message - should return error
		err = listener.processMessage(ctx, message)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal event")
	})
}

func TestTMFEventListener_EnsureConsumerGroup(t *testing.T) {
	logger := zap.NewNop()
	mockRedis := miniredis.RunT(t)
	defer mockRedis.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mockRedis.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	hubStore := storage.NewInMemoryHubStore()
	publisher := NewTMFEventPublisher(logger, nil)

	cfg := &TMFEventListenerConfig{
		RedisClient: redisClient,
		HubStore:    hubStore,
		Publisher:   publisher,
		Logger:      logger,
		BaseURL:     "https://gateway.example.com",
	}

	listener, err := NewTMFEventListener(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("creates consumer group", func(t *testing.T) {
		err := listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)
	})

	t.Run("handles existing consumer group", func(t *testing.T) {
		// Create again - should not error
		err := listener.ensureConsumerGroup(ctx)
		require.NoError(t, err)
	})
}

func TestTMFEventListener_AcknowledgeMessage(t *testing.T) {
	logger := zap.NewNop()
	mockRedis := miniredis.RunT(t)
	defer mockRedis.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mockRedis.Addr(),
	})
	defer func() { _ = redisClient.Close() }()

	hubStore := storage.NewInMemoryHubStore()
	publisher := NewTMFEventPublisher(logger, nil)

	cfg := &TMFEventListenerConfig{
		RedisClient: redisClient,
		HubStore:    hubStore,
		Publisher:   publisher,
		Logger:      logger,
		BaseURL:     "https://gateway.example.com",
	}

	listener, err := NewTMFEventListener(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Ensure consumer group exists
	err = listener.ensureConsumerGroup(ctx)
	require.NoError(t, err)

	// Add message to stream
	messageID, err := redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: controllers.EventStreamKey,
		Values: map[string]interface{}{
			"event": "test",
		},
	}).Result()
	require.NoError(t, err)

	// Read message as consumer
	streams, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    tmfConsumerGroup,
		Consumer: tmfConsumerName,
		Streams:  []string{controllers.EventStreamKey, ">"},
		Count:    1,
	}).Result()
	require.NoError(t, err)
	require.NotEmpty(t, streams)

	t.Run("acknowledges message", func(t *testing.T) {
		err := listener.acknowledgeMessage(ctx, messageID)
		require.NoError(t, err)
	})

	t.Run("acknowledging non-existent message", func(t *testing.T) {
		// Should not error - Redis XAck returns 0 for non-existent messages but doesn't error
		err := listener.acknowledgeMessage(ctx, fmt.Sprintf("%d-0", time.Now().UnixNano()))
		require.NoError(t, err)
	})
}
