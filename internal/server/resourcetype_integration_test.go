//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
)

// TestResourceTypeHandler_Integration tests the ResourceType HTTP endpoints
// with a complete server setup.
func TestResourceTypeHandler_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test server
	logger := zaptest.NewLogger(t)
	mockAdapter := &mockResourceTypeAdapter{
		types: []*adapter.ResourceType{
			{
				ResourceTypeID: "compute-m5.large",
				Name:           "M5 Large Compute",
				Description:    "General purpose compute instance",
				ResourceClass:  "compute",
				ResourceKind:   "virtual",
				Extensions: map[string]interface{}{
					"vcpu":   "2",
					"memory": "8Gi",
				},
			},
			{
				ResourceTypeID: "storage-ssd",
				Name:           "SSD Storage",
				Description:    "Solid state storage",
				ResourceClass:  "storage",
				ResourceKind:   "virtual",
				Extensions: map[string]interface{}{
					"type": "ssd",
					"iops": "3000",
				},
			},
		},
	}

	mockStorage := &mockResourceTypeStore{}
	cfg := &config.Config{
		Server: config.ServerConfig{
			GinMode: "test",
		},
	}

	srv, _ := server.NewTestServerWithMetrics(cfg, logger, mockAdapter, mockStorage)
	router := srv.Router()

	t.Run("list_resource_types", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response structure
		assert.Contains(t, response, "resourceTypes")
		types := response["resourceTypes"].([]interface{})
		assert.Len(t, types, 2, "should return both resource types")

		// Verify first type
		firstType := types[0].(map[string]interface{})
		assert.Equal(t, "compute-m5.large", firstType["resourceTypeId"])
		assert.Equal(t, "M5 Large Compute", firstType["name"])
	})

	t.Run("list_with_pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes?limit=1&offset=0", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		types := response["resourceTypes"].([]interface{})
		assert.LessOrEqual(t, len(types), 1, "should respect limit parameter")
	})

	t.Run("get_specific_resource_type", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes/compute-m5.large", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "compute-m5.large", response["resourceTypeId"])
		assert.Equal(t, "M5 Large Compute", response["name"])
		assert.Equal(t, "compute", response["resourceClass"])
	})

	t.Run("get_nonexistent_resource_type", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes/nonexistent", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "error")
		assert.Contains(t, response["error"], "not found")
	})

	t.Run("list_with_invalid_pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes?limit=invalid", nil)
		router.ServeHTTP(w, req)

		// Should handle gracefully (either ignore or return 400)
		assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, w.Code)
	})
}

// TestResourceTypeHandler_ErrorHandling tests error scenarios in the HTTP handler.
func TestResourceTypeHandler_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zaptest.NewLogger(t)
	mockAdapter := &mockResourceTypeAdapter{
		shouldError: true,
	}

	mockStorage := &mockResourceTypeStore{}
	cfg := &config.Config{
		Server: config.ServerConfig{
			GinMode: "test",
		},
	}

	srv, _ := server.NewTestServerWithMetrics(cfg, logger, mockAdapter, mockStorage)
	router := srv.Router()

	t.Run("list_returns_error", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "error")
	})
}

// mockResourceTypeAdapter is a mock adapter for testing HTTP handlers.
type mockResourceTypeAdapter struct {
	types       []*adapter.ResourceType
	shouldError bool
}

func (m *mockResourceTypeAdapter) Name() string                       { return "mock" }
func (m *mockResourceTypeAdapter) Version() string                    { return "1.0.0" }
func (m *mockResourceTypeAdapter) Capabilities() []adapter.Capability { return nil }
func (m *mockResourceTypeAdapter) Health(_ context.Context) error     { return nil }
func (m *mockResourceTypeAdapter) Close() error                       { return nil }

func (m *mockResourceTypeAdapter) ListResourceTypes(_ context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	if m.shouldError {
		return nil, assert.AnError
	}

	types := m.types
	if filter != nil {
		types = adapter.ApplyPagination(types, filter.Limit, filter.Offset)
	}
	return types, nil
}

func (m *mockResourceTypeAdapter) GetResourceType(_ context.Context, id string) (*adapter.ResourceType, error) {
	if m.shouldError {
		return nil, assert.AnError
	}

	for _, rt := range m.types {
		if rt.ResourceTypeID == id {
			return rt, nil
		}
	}
	return nil, adapter.ErrResourceNotFound
}

// Implement other required methods as stubs.
func (m *mockResourceTypeAdapter) ListDeploymentManagers(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceTypeAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) GetResourcePool(_ context.Context, _ string) (*adapter.ResourcePool, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceTypeAdapter) CreateResourcePool(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) UpdateResourcePool(_ context.Context, _ string, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) DeleteResourcePool(_ context.Context, _ string) error {
	return nil
}
func (m *mockResourceTypeAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) GetResource(_ context.Context, _ string) (*adapter.Resource, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceTypeAdapter) CreateResource(_ context.Context, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) UpdateResource(_ context.Context, _ string, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, nil
}
func (m *mockResourceTypeAdapter) DeleteResource(_ context.Context, _ string) error {
	return nil
}
func (m *mockResourceTypeAdapter) CreateSubscription(_ context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}
func (m *mockResourceTypeAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceTypeAdapter) UpdateSubscription(_ context.Context, _ string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}
func (m *mockResourceTypeAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return nil
}
func (m *mockResourceTypeAdapter) ListSubscriptions(_ context.Context, _ *adapter.Filter) ([]*adapter.Subscription, error) {
	return nil, nil
}

// mockResourceTypeStore is a minimal mock storage.Store implementation for testing.
type mockResourceTypeStore struct{}

func (m *mockResourceTypeStore) Create(_ context.Context, _ *storage.Subscription) error { return nil }
func (m *mockResourceTypeStore) Get(_ context.Context, _ string) (*storage.Subscription, error) {
	return nil, storage.ErrSubscriptionNotFound
}
func (m *mockResourceTypeStore) Update(_ context.Context, _ *storage.Subscription) error { return nil }
func (m *mockResourceTypeStore) Delete(_ context.Context, _ string) error                { return nil }
func (m *mockResourceTypeStore) List(_ context.Context) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceTypeStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceTypeStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceTypeStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceTypeStore) Close() error                 { return nil }
func (m *mockResourceTypeStore) Ping(_ context.Context) error { return nil }
