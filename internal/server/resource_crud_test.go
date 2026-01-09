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
)

// mockResourceAdapter implements adapter.Adapter for resource CRUD tests.
type mockResourceAdapter struct {
	mockAdapter
	resources map[string]*adapter.Resource
}

func newMockResourceAdapter() *mockResourceAdapter {
	return &mockResourceAdapter{
		resources: map[string]*adapter.Resource{
			"test-res-123": {
				ResourceID:     "test-res-123",
				ResourceTypeID: "machine",
				ResourcePoolID: "pool-1",
				Description:    "Test resource",
				GlobalAssetID:  "urn:test:asset:123",
			},
		},
	}
}

func (m *mockResourceAdapter) GetResource(_ context.Context, id string) (*adapter.Resource, error) {
	if res, ok := m.resources[id]; ok {
		return res, nil
	}
	return nil, errors.New("resource not found")
}

func (m *mockResourceAdapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	m.resources[resource.ResourceID] = resource
	return resource, nil
}

func TestResourceCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourceAdapter(), &mockStore{})

	// Test POST /resources
	t.Run("POST /resources - create resource", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Test compute resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var created adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		assert.Equal(t, resource.ResourceTypeID, created.ResourceTypeID)
		assert.NotEmpty(t, created.ResourceID)
	})

	t.Run("POST /resources - validation error (empty resourceTypeId)", func(t *testing.T) {
		resource := adapter.Resource{
			ResourcePoolID: "pool-1",
			Description:    "Test resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Resource type ID is required")
	})

	// Test PUT /resources/:id
	t.Run("PUT /resources/:id - update resource description", func(t *testing.T) {
		resource := adapter.Resource{
			Description:   "Updated description",
			GlobalAssetID: "urn:updated:asset:123",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/test-res-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, "test-res-123", updated.ResourceID)
		assert.Equal(t, resource.Description, updated.Description)
		assert.Equal(t, resource.GlobalAssetID, updated.GlobalAssetID)
	})

	t.Run("PUT /resources/:id - update extensions", func(t *testing.T) {
		resource := adapter.Resource{
			Description: "Resource with updated extensions",
			Extensions: map[string]interface{}{
				"cpu":    "32 cores",
				"memory": "128GB",
				"status": "active",
			},
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/test-res-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.NotNil(t, updated.Extensions)
		assert.Equal(t, "32 cores", updated.Extensions["cpu"])
		assert.Equal(t, "128GB", updated.Extensions["memory"])
	})

	t.Run("PUT /resources/:id - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/test-res-123",
			bytes.NewReader([]byte("invalid json")),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "Invalid request body")
	})

	t.Run("PUT /resources/:id - resource not found", func(t *testing.T) {
		resource := adapter.Resource{
			Description: "Test resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/nonexistent-res",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
		assert.Contains(t, resp.Body.String(), "Resource not found")
	})

	t.Run("PUT /resources/:id - preserve immutable fields", func(t *testing.T) {
		resource := adapter.Resource{
			Description: "Updated resource",
			// Not providing ResourceTypeID or ResourcePoolID - should be preserved
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/test-res-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		// ResourceTypeID and ResourcePoolID should be preserved from existing resource
		assert.NotEmpty(t, updated.ResourceTypeID)
		assert.NotEmpty(t, updated.ResourcePoolID)
	})

	t.Run("PUT /resources/:id - reject immutable field modification", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "different-type", // Attempt to change immutable field
			Description:    "Updated resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/test-res-123",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "immutable")
	})

	// Validation tests
	t.Run("POST /resources - invalid GlobalAssetID format", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			GlobalAssetID:  "invalid-not-urn",
			Description:    "Test resource",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "URN format")
	})

	t.Run("POST /resources - description too long", func(t *testing.T) {
		longDesc := string(make([]byte, 1001)) // Exceeds 1000 char limit
		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    longDesc,
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "description")
	})

	t.Run("POST /resources - too many extensions", func(t *testing.T) {
		// Create 101 extensions (exceeds 100 limit)
		extensions := make(map[string]interface{})
		for i := 0; i < 101; i++ {
			extensions[fmt.Sprintf("key%d", i)] = "value"
		}

		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Extensions:     extensions,
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "extensions")
	})

	t.Run("POST /resources - custom resource ID", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceID:     "custom-resource-id",
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Resource with custom ID",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/o2ims-infrastructureInventory/v1/resources",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var created adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		assert.Equal(t, "custom-resource-id", created.ResourceID)
	})
}
