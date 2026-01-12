package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	mu        sync.Mutex
	resources map[string]*adapter.Resource
}

func newMockResourceAdapter() *mockResourceAdapter {
	// Use realistic UUID format for test data
	testResourceID := "550e8400-e29b-41d4-a716-446655440000"
	return &mockResourceAdapter{
		resources: map[string]*adapter.Resource{
			testResourceID: {
				ResourceID:     testResourceID,
				ResourceTypeID: "machine",
				ResourcePoolID: "pool-1",
				Description:    "Test resource",
				GlobalAssetID:  "urn:test:asset:123",
			},
		},
	}
}

func (m *mockResourceAdapter) GetResource(_ context.Context, id string) (*adapter.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if res, ok := m.resources[id]; ok {
		return res, nil
	}
	return nil, adapter.ErrResourceNotFound
}

func (m *mockResourceAdapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate resource ID
	if _, exists := m.resources[resource.ResourceID]; exists {
		return nil, fmt.Errorf("resource with ID %s already exists: %w", resource.ResourceID, adapter.ErrResourceExists)
	}

	m.resources[resource.ResourceID] = resource
	return resource, nil
}

func (m *mockResourceAdapter) UpdateResource(
	_ context.Context,
	id string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.resources[id]; !ok {
		return nil, adapter.ErrResourceNotFound
	}
	// Update the resource in the map
	resource.ResourceID = id
	m.resources[id] = resource
	return resource, nil
}

func TestResourceCRUD(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
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

	t.Run("POST /resources - verify Location header", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
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

		require.Equal(t, http.StatusCreated, resp.Code)

		// Verify Location header is set
		location := resp.Header().Get("Location")
		require.NotEmpty(t, location, "Location header should be set")
		require.Contains(t, location, "/o2ims/v1/resources/", "Location header should contain resource path")

		// Verify Location header contains the resource ID
		var created adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &created)
		require.NoError(t, err)
		require.Contains(t, location, created.ResourceID, "Location header should contain the created resource ID")
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
		assert.Contains(t, resp.Body.String(), "resource type ID is required")
	})

	t.Run("POST /resources - validation error (empty resourcePoolId)", func(t *testing.T) {
		resource := adapter.Resource{
			ResourceTypeID: "machine",
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
		assert.Contains(t, resp.Body.String(), "resource pool ID is required")
	})

	t.Run("POST /resources - security: reject invalid UUID (path traversal)", func(t *testing.T) {
		// Test path traversal attack attempt
		resource := adapter.Resource{
			ResourceID:     "../../../etc/passwd",
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Malicious resource",
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
		assert.Contains(t, resp.Body.String(), "resourceId must be a valid UUID")
	})

	t.Run("POST /resources - security: reject invalid UUID (SQL injection)", func(t *testing.T) {
		// Test SQL injection attempt
		resource := adapter.Resource{
			ResourceID:     "'; DROP TABLE resources; --",
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Malicious resource",
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
		assert.Contains(t, resp.Body.String(), "resourceId must be a valid UUID")
	})

	t.Run("POST /resources - accept valid client-provided UUID", func(t *testing.T) {
		// Test valid UUID is accepted
		validUUID := "550e8400-e29b-41d4-a716-446655440001"
		resource := adapter.Resource{
			ResourceID:     validUUID,
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Resource with client-provided UUID",
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
		assert.Equal(t, validUUID, created.ResourceID)
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
			"/o2ims-infrastructureInventory/v1/resources/550e8400-e29b-41d4-a716-446655440000",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var updated adapter.Resource
		err = json.Unmarshal(resp.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", updated.ResourceID)
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
			"/o2ims-infrastructureInventory/v1/resources/550e8400-e29b-41d4-a716-446655440000",
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
			"/o2ims-infrastructureInventory/v1/resources/550e8400-e29b-41d4-a716-446655440000",
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
			"/o2ims-infrastructureInventory/v1/resources/550e8400-e29b-41d4-a716-446655440000",
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
			"/o2ims-infrastructureInventory/v1/resources/550e8400-e29b-41d4-a716-446655440000",
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
		assert.Contains(t, resp.Body.String(), "urn:")
	})

	t.Run("POST /resources - description too long", func(t *testing.T) {
		longDesc := strings.Repeat("a", 1001) // Exceeds 1000 char limit
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

	t.Run("POST /resources - extension key exceeds 256 characters", func(t *testing.T) {
		// Create extension with key longer than 256 characters
		longKey := strings.Repeat("a", 257)
		extensions := map[string]interface{}{
			longKey: "value",
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
		assert.Contains(t, resp.Body.String(), "extension keys must not exceed 256 characters")
	})

	t.Run("POST /resources - extension value exceeds 4KB", func(t *testing.T) {
		// Create extension with value larger than 4096 bytes when JSON-encoded
		largeValue := strings.Repeat("x", 4100)
		extensions := map[string]interface{}{
			"key": largeValue,
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
		assert.Contains(t, resp.Body.String(), "extension values must not exceed 4096 bytes")
	})

	t.Run("POST /resources - extensions exceeds 100 keys", func(t *testing.T) {
		srv := New(cfg, zap.NewNop(), newMockResourceAdapter(), &mockStore{})

		// Create map with 101 keys
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
		assert.Contains(t, resp.Body.String(), "extensions map must not exceed 100 keys")
	})

	t.Run("POST /resources - total extensions payload exceeds 50KB", func(t *testing.T) {
		srv := New(cfg, zap.NewNop(), newMockResourceAdapter(), &mockStore{})

		// Create extensions with total size > 50KB
		// Each value ~2KB, 26 keys = ~52KB total
		extensions := make(map[string]interface{})
		largeValue := strings.Repeat("x", 2000)
		for i := 0; i < 26; i++ {
			extensions[fmt.Sprintf("key%d", i)] = largeValue
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
		assert.Contains(t, resp.Body.String(), "total extensions payload must not exceed 50000 bytes")
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

	t.Run("POST /resources - duplicate resource ID", func(t *testing.T) {
		// Use the existing resource ID from the mock
		resource := adapter.Resource{
			ResourceID:     "550e8400-e29b-41d4-a716-446655440000", // Already exists in mock
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Duplicate resource",
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

		assert.Equal(t, http.StatusConflict, resp.Code)
		assert.Contains(t, resp.Body.String(), "already exists")
	})

	// Adapter error scenario tests
	t.Run("POST /resources - adapter error on create", func(t *testing.T) {
		// Create a mock that returns an error
		mockAdp := &mockResourceAdapter{
			resources: map[string]*adapter.Resource{},
		}
		srv := New(cfg, zap.NewNop(), &errorReturningAdapter{mockAdp, "create"}, &mockStore{})

		resource := adapter.Resource{
			ResourceTypeID: "machine",
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

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Failed to create resource")
	})

	t.Run("PUT /resources/:id - adapter error on update", func(t *testing.T) {
		// Create a mock with existing resource that returns error on update
		mockAdp := &mockResourceAdapter{
			resources: map[string]*adapter.Resource{
				"550e8400-e29b-41d4-a716-446655440000": {
					ResourceID:     "550e8400-e29b-41d4-a716-446655440000",
					ResourceTypeID: "machine",
					ResourcePoolID: "pool-1",
					Description:    "Original",
				},
			},
		}
		srv := New(cfg, zap.NewNop(), &errorReturningAdapter{mockAdp, "update"}, &mockStore{})

		resource := adapter.Resource{
			Description: "Updated description",
		}

		body, err := json.Marshal(resource)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/o2ims-infrastructureInventory/v1/resources/550e8400-e29b-41d4-a716-446655440000",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		srv.router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Failed to update resource")
	})
}

// errorReturningAdapter wraps a mock adapter and returns errors for specific operations.
type errorReturningAdapter struct {
	*mockResourceAdapter
	errorOn string // "create", "update", "get"
}

func (e *errorReturningAdapter) CreateResource(
	ctx context.Context,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	if e.errorOn == "create" {
		return nil, errors.New("simulated adapter create error")
	}
	return e.mockResourceAdapter.CreateResource(ctx, resource)
}

func (e *errorReturningAdapter) UpdateResource(
	ctx context.Context,
	id string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	if e.errorOn == "update" {
		return nil, errors.New("simulated adapter update error")
	}
	return e.mockResourceAdapter.UpdateResource(ctx, id, resource)
}

func (e *errorReturningAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	if e.errorOn == "get" {
		return nil, errors.New("simulated adapter get error")
	}
	return e.mockResourceAdapter.GetResource(ctx, id)
}

// TestResourceConcurrency tests concurrent operations on the same resource.
func TestResourceConcurrency(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	t.Run("concurrent creates with same ID", func(t *testing.T) {
		mockAdp := &mockResourceAdapter{
			resources: make(map[string]*adapter.Resource),
		}
		srv := New(cfg, zap.NewNop(), mockAdp, &mockStore{})

		// Use a specific resource ID
		resourceID := "test-concurrent-123"
		resource := adapter.Resource{
			ResourceID:     resourceID,
			ResourceTypeID: "machine",
			ResourcePoolID: "pool-1",
			Description:    "Concurrent test",
		}

		// Run 10 concurrent create requests with the same ID
		const numGoroutines = 10
		results := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
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
				results <- resp.Code
			}()
		}

		// Collect results
		statusCodes := make(map[int]int)
		for i := 0; i < numGoroutines; i++ {
			code := <-results
			statusCodes[code]++
		}

		// Exactly one should succeed (201), others should get 409 Conflict
		assert.Equal(t, 1, statusCodes[http.StatusCreated],
			"Exactly one create should succeed")
		assert.Equal(t, numGoroutines-1, statusCodes[http.StatusConflict],
			"Other creates should get 409 Conflict")

		// Verify only one resource was created
		mockAdp.mu.Lock()
		assert.Equal(t, 1, len(mockAdp.resources),
			"Only one resource should exist")
		mockAdp.mu.Unlock()
	})

	t.Run("concurrent updates to same resource", func(t *testing.T) {
		resourceID := "test-update-concurrent"
		mockAdp := &mockResourceAdapter{
			resources: map[string]*adapter.Resource{
				resourceID: {
					ResourceID:     resourceID,
					ResourceTypeID: "machine",
					ResourcePoolID: "pool-1",
					Description:    "Original",
				},
			},
		}
		srv := New(cfg, zap.NewNop(), mockAdp, &mockStore{})

		// Run 10 concurrent updates
		const numGoroutines = 10
		results := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			i := i
			go func() {
				resource := adapter.Resource{
					Description: fmt.Sprintf("Updated %d", i),
				}

				body, err := json.Marshal(resource)
				require.NoError(t, err)

				req := httptest.NewRequest(
					http.MethodPut,
					"/o2ims-infrastructureInventory/v1/resources/"+resourceID,
					bytes.NewReader(body),
				)
				req.Header.Set("Content-Type", "application/json")
				resp := httptest.NewRecorder()

				srv.router.ServeHTTP(resp, req)
				results <- resp.Code
			}()
		}

		// All updates should succeed (200 OK)
		for i := 0; i < numGoroutines; i++ {
			code := <-results
			assert.Equal(t, http.StatusOK, code,
				"All concurrent updates should succeed")
		}

		// Verify resource still exists with one of the descriptions
		mockAdp.mu.Lock()
		res, exists := mockAdp.resources[resourceID]
		mockAdp.mu.Unlock()

		assert.True(t, exists, "Resource should still exist")
		assert.Contains(t, res.Description, "Updated",
			"Description should be from one of the updates")
	})

	t.Run("concurrent create and get operations", func(t *testing.T) {
		mockAdp := &mockResourceAdapter{
			resources: make(map[string]*adapter.Resource),
		}
		srv := New(cfg, zap.NewNop(), mockAdp, &mockStore{})

		resourceID := "test-create-get-concurrent"
		const numGoroutines = 20

		results := make(chan string, numGoroutines)

		// Half create, half get
		for i := 0; i < numGoroutines; i++ {
			i := i
			go func() {
				if i%2 == 0 {
					// Create operation
					resource := adapter.Resource{
						ResourceID:     resourceID,
						ResourceTypeID: "machine",
						ResourcePoolID: "pool-1",
						Description:    "Test",
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
					results <- fmt.Sprintf("create:%d", resp.Code)
				} else {
					// Get operation
					req := httptest.NewRequest(
						http.MethodGet,
						"/o2ims-infrastructureInventory/v1/resources/"+resourceID,
						nil,
					)
					resp := httptest.NewRecorder()

					srv.router.ServeHTTP(resp, req)
					results <- fmt.Sprintf("get:%d", resp.Code)
				}
			}()
		}

		// Collect results
		createSuccess := 0
		createConflict := 0
		getSuccess := 0
		getNotFound := 0

		for i := 0; i < numGoroutines; i++ {
			result := <-results
			parts := bytes.Split([]byte(result), []byte(":"))
			op := string(parts[0])
			code := string(parts[1])

			switch op {
			case "create":
				switch code {
				case "201":
					createSuccess++
				case "409":
					createConflict++
				}
			case "get":
				switch code {
				case "200":
					getSuccess++
				case "404":
					getNotFound++
				}
			}
		}

		// Exactly one create should succeed
		assert.Equal(t, 1, createSuccess, "Exactly one create should succeed")

		// Gets can be either 200 (if after create) or 404 (if before create)
		assert.Equal(t, numGoroutines/2, getSuccess+getNotFound,
			"All gets should complete with either 200 or 404")
	})
}
