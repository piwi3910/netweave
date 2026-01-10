package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/storage"
)

// mockSubscriptionStore implements storage.Store for subscription update tests.
type mockSubscriptionStore struct {
	subscriptions map[string]*storage.Subscription
}

func newMockSubscriptionStore() *mockSubscriptionStore {
	return &mockSubscriptionStore{
		subscriptions: map[string]*storage.Subscription{
			"test-sub-123": {
				ID:       "test-sub-123",
				Callback: "https://smo.example.com/notify",
			},
		},
	}
}

func (m *mockSubscriptionStore) Create(_ context.Context, sub *storage.Subscription) error {
	m.subscriptions[sub.ID] = sub
	return nil
}

func (m *mockSubscriptionStore) Get(_ context.Context, id string) (*storage.Subscription, error) {
	if sub, ok := m.subscriptions[id]; ok {
		return sub, nil
	}
	return nil, storage.ErrSubscriptionNotFound
}

func (m *mockSubscriptionStore) Update(_ context.Context, sub *storage.Subscription) error {
	if _, ok := m.subscriptions[sub.ID]; !ok {
		return storage.ErrSubscriptionNotFound
	}
	m.subscriptions[sub.ID] = sub
	return nil
}

func (m *mockSubscriptionStore) Delete(_ context.Context, id string) error {
	delete(m.subscriptions, id)
	return nil
}

func (m *mockSubscriptionStore) List(_ context.Context) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockSubscriptionStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockSubscriptionStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockSubscriptionStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockSubscriptionStore) Close() error {
	return nil
}

func (m *mockSubscriptionStore) Ping(_ context.Context) error {
	return nil
}

// mockSubscriptionAdapter implements adapter.Adapter for subscription update tests.
// It mimics K8s adapter behavior by using the storage layer.
type mockSubscriptionAdapter struct {
	store storage.Store
}

func (m *mockSubscriptionAdapter) Name() string    { return "mock" }
func (m *mockSubscriptionAdapter) Version() string { return "1.0.0" }
func (m *mockSubscriptionAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{adapter.CapabilitySubscriptions}
}
func (m *mockSubscriptionAdapter) Health(ctx context.Context) error { return nil }
func (m *mockSubscriptionAdapter) Close() error                     { return nil }
func (m *mockSubscriptionAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}
func (m *mockSubscriptionAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}
func (m *mockSubscriptionAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	return nil
}
func (m *mockSubscriptionAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	return resource, nil
}
func (m *mockSubscriptionAdapter) DeleteResource(ctx context.Context, id string) error {
	return nil
}
func (m *mockSubscriptionAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}
func (m *mockSubscriptionAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	return nil, nil
}
func (m *mockSubscriptionAdapter) UpdateSubscription(ctx context.Context, id string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	// Validate callback URL
	if sub.Callback == "" {
		return nil, fmt.Errorf("callback URL is required")
	}

	// Check if subscription exists in storage (like K8s adapter does)
	_, err := m.store.Get(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			return nil, fmt.Errorf("%w: %s", adapter.ErrSubscriptionNotFound, id)
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update in storage
	storageSub := &storage.Subscription{
		ID:       id,
		Callback: sub.Callback,
	}
	if err := m.store.Update(ctx, storageSub); err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	sub.SubscriptionID = id
	return sub, nil
}
func (m *mockSubscriptionAdapter) DeleteSubscription(ctx context.Context, id string) error {
	return nil
}

func TestSubscriptionUPDATE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	store := newMockSubscriptionStore()
	srv := New(cfg, zap.NewNop(), &mockSubscriptionAdapter{store: store}, store)

	t.Run("PUT /subscriptions/:id - update subscription callback", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback:               "https://smo-new.example.com/notify",
			ConsumerSubscriptionID: "consumer-sub-123",
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Subscription
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, "test-sub-123", updated.SubscriptionID)
		assert.Equal(t, subscription.Callback, updated.Callback)
		assert.Equal(t, subscription.ConsumerSubscriptionID, updated.ConsumerSubscriptionID)
	})

	t.Run("PUT /subscriptions/:id - update subscription filter", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback: "https://smo.example.com/notify",
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-new",
				ResourceTypeID: "machine",
			},
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Subscription
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.NotNil(t, updated.Filter)
		assert.Equal(t, "pool-new", updated.Filter.ResourcePoolID)
		assert.Equal(t, "machine", updated.Filter.ResourceTypeID)
	})

	t.Run("PUT /subscriptions/:id - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader([]byte("invalid json")),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Invalid request body")
	})

	t.Run("PUT /subscriptions/:id - subscription not found", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback: "https://smo.example.com/notify",
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/nonexistent-sub",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		assert.Contains(t, resp.Body.String(), "Subscription not found")
	})

	t.Run("PUT /subscriptions/:id - update all fields", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback:               "https://smo-updated.example.com/webhooks",
			ConsumerSubscriptionID: "new-consumer-id",
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-updated",
				ResourceTypeID: "compute",
				ResourceID:     "res-123",
			},
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Subscription
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, subscription.Callback, updated.Callback)
		assert.Equal(t, subscription.ConsumerSubscriptionID, updated.ConsumerSubscriptionID)
		assert.NotNil(t, updated.Filter)
		assert.Equal(t, subscription.Filter.ResourcePoolID, updated.Filter.ResourcePoolID)
		assert.Equal(t, subscription.Filter.ResourceTypeID, updated.Filter.ResourceTypeID)
		assert.Equal(t, subscription.Filter.ResourceID, updated.Filter.ResourceID)
	})

	t.Run("PUT /subscriptions/:id - empty callback URL", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback: "", // Empty callback should fail validation
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-test",
			},
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		// Handler validation should reject empty callback with 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "callback URL is required")
	})

	t.Run("PUT /subscriptions/:id - update only callback (no filter)", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback:               "https://new-callback.example.com/webhook",
			ConsumerSubscriptionID: "consumer-456",
			Filter:                 nil, // No filter update
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Subscription
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, subscription.Callback, updated.Callback)
		assert.Equal(t, subscription.ConsumerSubscriptionID, updated.ConsumerSubscriptionID)
	})

	t.Run("PUT /subscriptions/:id - invalid callback URL format", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback: "not-a-valid-url",
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-test",
			},
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		// Should reject invalid URL format
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "callback URL must use http or https scheme")
	})

	t.Run("PUT /subscriptions/:id - callback with unsupported scheme", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback: "ftp://example.com/webhook",
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: "pool-test",
			},
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		// Should reject unsupported scheme
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "callback URL must use http or https scheme")
	})

	t.Run("PUT /subscriptions/:id - remove filter with null", func(t *testing.T) {
		subscription := adapter.Subscription{
			Callback:               "https://smo.example.com/notify",
			ConsumerSubscriptionID: "consumer-789",
			Filter:                 nil, // Explicitly remove filter
		}

		body, err := json.Marshal(subscription)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/subscriptions/test-sub-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Subscription
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, subscription.Callback, updated.Callback)
		// Filter should be nil since we removed it
		assert.Nil(t, updated.Filter)
	})
}
