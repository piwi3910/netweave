package server

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestSubscriptionUPDATE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, newMockSubscriptionStore())

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
}
