//go:build integration

package server_test

import (
	"bytes"
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

// TestResourceHandler_Integration tests the Resource HTTP endpoints
// with a complete server setup.
func TestResourceHandler_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test server
	logger := zaptest.NewLogger(t)
	mockAdapter := &mockResourceIntegrationAdapter{
		resources: []*adapter.Resource{
			{
				ResourceID:     "resource-1",
				ResourceTypeID: "compute-m5.large",
				ResourcePoolID: "pool-1",
				Description:    "Test compute resource",
				GlobalAssetID:  "urn:resource:1",
				Extensions: map[string]interface{}{
					"vcpu":   "2",
					"memory": "8Gi",
				},
			},
			{
				ResourceID:     "resource-2",
				ResourceTypeID: "storage-ssd",
				ResourcePoolID: "pool-1",
				Description:    "Test storage resource",
				GlobalAssetID:  "urn:resource:2",
				Extensions: map[string]interface{}{
					"capacity": "100Gi",
					"type":     "ssd",
				},
			},
		},
	}

	mockStorage := &mockResourceIntegrationStore{}
	cfg := &config.Config{
		Server: config.ServerConfig{
			GinMode: "test",
		},
	}

	srv, _ := server.NewTestServerWithMetrics(cfg, logger, mockAdapter, mockStorage)
	router := srv.Router()

	t.Run("list_resources", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response structure
		assert.Contains(t, response, "resources")
		resources := response["resources"].([]interface{})
		assert.Len(t, resources, 2, "should return both resources")

		// Verify first resource
		firstResource := resources[0].(map[string]interface{})
		assert.Equal(t, "resource-1", firstResource["resourceId"])
		assert.Equal(t, "compute-m5.large", firstResource["resourceTypeId"])
	})

	t.Run("list_with_pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources?limit=1&offset=0", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		resources := response["resources"].([]interface{})
		assert.LessOrEqual(t, len(resources), 1, "should respect limit parameter")
	})

	t.Run("get_specific_resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources/resource-1", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "resource-1", response["resourceId"])
		assert.Equal(t, "compute-m5.large", response["resourceTypeId"])
		assert.Equal(t, "Test compute resource", response["description"])
	})

	t.Run("get_nonexistent_resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources/nonexistent", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "error")
	})

	t.Run("create_resource", func(t *testing.T) {
		newResource := map[string]interface{}{
			"resourceTypeId": "compute-m5.large",
			"resourcePoolId": "pool-1",
			"description":    "New test resource",
			"extensions": map[string]interface{}{
				"test": "true",
			},
		}

		body, _ := json.Marshal(newResource)
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response["resourceId"], "created resource should have ID")
		assert.Equal(t, "compute-m5.large", response["resourceTypeId"])
	})

	t.Run("update_resource", func(t *testing.T) {
		updatedResource := map[string]interface{}{
			"description": "Updated resource description",
			"extensions": map[string]interface{}{
				"updated": "true",
			},
		}

		body, _ := json.Marshal(updatedResource)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPut, "/o2ims-infrastructureInventory/v1/resources/resource-1", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "Updated resource description", response["description"])
	})

	t.Run("delete_resource", func(t *testing.T) {
		// First verify resource exists
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources/resource-2", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// Delete the resource
		w = httptest.NewRecorder()
		req, _ = http.NewRequestWithContext(context.Background(), http.MethodDelete, "/o2ims-infrastructureInventory/v1/resources/resource-2", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify resource is deleted
		w = httptest.NewRecorder()
		req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources/resource-2", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("list_with_invalid_pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources?limit=invalid", nil)
		router.ServeHTTP(w, req)

		// Should handle gracefully (either ignore or return 400)
		assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, w.Code)
	})
}

// TestResourceHandler_ErrorHandling tests error scenarios in the HTTP handler.
func TestResourceHandler_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zaptest.NewLogger(t)
	mockAdapter := &mockResourceIntegrationAdapter{
		shouldError: true,
	}

	mockStorage := &mockResourceIntegrationStore{}
	cfg := &config.Config{
		Server: config.ServerConfig{
			GinMode: "test",
		},
	}

	srv, _ := server.NewTestServerWithMetrics(cfg, logger, mockAdapter, mockStorage)
	router := srv.Router()

	t.Run("list_returns_error", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "error")
	})

	t.Run("create_with_invalid_body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// mockResourceIntegrationAdapter is a mock adapter for testing HTTP handlers.
type mockResourceIntegrationAdapter struct {
	resources   []*adapter.Resource
	shouldError bool
	deleted     map[string]bool
}

func (m *mockResourceIntegrationAdapter) Name() string                       { return "mock" }
func (m *mockResourceIntegrationAdapter) Version() string                    { return "1.0.0" }
func (m *mockResourceIntegrationAdapter) Capabilities() []adapter.Capability { return nil }
func (m *mockResourceIntegrationAdapter) Health(_ context.Context) error     { return nil }
func (m *mockResourceIntegrationAdapter) Close() error                       { return nil }

func (m *mockResourceIntegrationAdapter) ListResources(_ context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	if m.shouldError {
		return nil, assert.AnError
	}

	resources := make([]*adapter.Resource, 0)
	for _, res := range m.resources {
		if m.deleted != nil && m.deleted[res.ResourceID] {
			continue
		}
		resources = append(resources, res)
	}

	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}
	return resources, nil
}

func (m *mockResourceIntegrationAdapter) GetResource(_ context.Context, id string) (*adapter.Resource, error) {
	if m.shouldError {
		return nil, assert.AnError
	}

	if m.deleted != nil && m.deleted[id] {
		return nil, adapter.ErrResourceNotFound
	}

	for _, res := range m.resources {
		if res.ResourceID == id {
			return res, nil
		}
	}
	return nil, adapter.ErrResourceNotFound
}

func (m *mockResourceIntegrationAdapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	if m.shouldError {
		return nil, assert.AnError
	}

	resource.ResourceID = "new-resource-id"
	m.resources = append(m.resources, resource)
	return resource, nil
}

func (m *mockResourceIntegrationAdapter) UpdateResource(_ context.Context, id string, resource *adapter.Resource) (*adapter.Resource, error) {
	if m.shouldError {
		return nil, assert.AnError
	}

	if m.deleted != nil && m.deleted[id] {
		return nil, adapter.ErrResourceNotFound
	}

	for i, res := range m.resources {
		if res.ResourceID == id {
			m.resources[i] = resource
			resource.ResourceID = id
			return resource, nil
		}
	}
	return nil, adapter.ErrResourceNotFound
}

func (m *mockResourceIntegrationAdapter) DeleteResource(_ context.Context, id string) error {
	if m.shouldError {
		return assert.AnError
	}

	if m.deleted == nil {
		m.deleted = make(map[string]bool)
	}

	for _, res := range m.resources {
		if res.ResourceID == id {
			m.deleted[id] = true
			return nil
		}
	}
	return adapter.ErrResourceNotFound
}

// Implement other required methods as stubs.
func (m *mockResourceIntegrationAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}
func (m *mockResourceIntegrationAdapter) GetResourceType(_ context.Context, _ string) (*adapter.ResourceType, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceIntegrationAdapter) ListDeploymentManagers(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	return nil, nil
}
func (m *mockResourceIntegrationAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceIntegrationAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockResourceIntegrationAdapter) GetResourcePool(_ context.Context, _ string) (*adapter.ResourcePool, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceIntegrationAdapter) CreateResourcePool(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockResourceIntegrationAdapter) UpdateResourcePool(_ context.Context, _ string, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockResourceIntegrationAdapter) DeleteResourcePool(_ context.Context, _ string) error {
	return nil
}
func (m *mockResourceIntegrationAdapter) CreateSubscription(_ context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}
func (m *mockResourceIntegrationAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockResourceIntegrationAdapter) UpdateSubscription(_ context.Context, _ string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}
func (m *mockResourceIntegrationAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return nil
}
func (m *mockResourceIntegrationAdapter) ListSubscriptions(_ context.Context, _ *adapter.Filter) ([]*adapter.Subscription, error) {
	return nil, nil
}

// mockResourceIntegrationStore is a minimal mock storage.Store implementation for testing.
type mockResourceIntegrationStore struct{}

func (m *mockResourceIntegrationStore) Create(_ context.Context, _ *storage.Subscription) error {
	return nil
}
func (m *mockResourceIntegrationStore) Get(_ context.Context, _ string) (*storage.Subscription, error) {
	return nil, storage.ErrSubscriptionNotFound
}
func (m *mockResourceIntegrationStore) Update(_ context.Context, _ *storage.Subscription) error {
	return nil
}
func (m *mockResourceIntegrationStore) Delete(_ context.Context, _ string) error { return nil }
func (m *mockResourceIntegrationStore) List(_ context.Context) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceIntegrationStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceIntegrationStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceIntegrationStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockResourceIntegrationStore) Close() error                 { return nil }
func (m *mockResourceIntegrationStore) Ping(_ context.Context) error { return nil }
