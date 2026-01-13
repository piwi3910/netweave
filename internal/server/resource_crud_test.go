package server

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

const (
	errorOnCreate = "create"
	errorOnUpdate = "update"
	errorOnGet    = "get"
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

// doResourceRequest is a test helper for making HTTP requests to resource endpoints.
func doResourceRequest(
	t *testing.T,
	srv *Server,
	method,
	path string,
	body interface{},
) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(jsonBytes)
		req := httptest.NewRequest(method, path, reqBody)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		srv.router.ServeHTTP(resp, req)
		return resp, resp.Body.Bytes()
	}
	req := httptest.NewRequest(method, path, nil)
	resp := httptest.NewRecorder()
	srv.router.ServeHTTP(resp, req)
	return resp, resp.Body.Bytes()
}

// createTestResource is a helper that creates a test resource and returns the response.
func createTestResource(
	t *testing.T,
	srv *Server,
	resource adapter.Resource,
) (*adapter.Resource, *httptest.ResponseRecorder) {
	t.Helper()
	resp, respBody := doResourceRequest(t, srv, http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", resource)
	if resp.Code != http.StatusCreated {
		return nil, resp
	}
	var created adapter.Resource
	err := json.Unmarshal(respBody, &created)
	require.NoError(t, err)
	return &created, resp
}

// Test helper functions to reduce code duplication

// setupResourceTestServer creates a test server with mock adapter and config.
func setupResourceTestServer(t *testing.T, adp adapter.Adapter) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	return New(cfg, zap.NewNop(), adp, &mockStore{})
}

// makeResourcePostRequest creates and executes a POST request to /resources.
func makeResourcePostRequest(t *testing.T, srv *Server, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/o2ims-infrastructureInventory/v1/resources",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	srv.router.ServeHTTP(recorder, req)
	return recorder
}

// marshalResourceToJSON marshals a resource to JSON bytes.
func marshalResourceToJSON(t *testing.T, resource adapter.Resource) []byte {
	t.Helper()
	data, err := json.Marshal(resource)
	require.NoError(t, err)
	return data
}

// testInvalidResourceID tests resource creation with invalid ID and expected error.
func testInvalidResourceID(t *testing.T, resourceID, expectedError string) {
	t.Helper()
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	res := adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Test resource",
	}

	body := marshalResourceToJSON(t, res)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), expectedError)
}

func TestResourceCreateResource(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Test compute resource",
	}

	created, resp := createTestResource(t, srv, resource)
	assert.Equal(t, http.StatusCreated, resp.Code)
	assert.Equal(t, resource.ResourceTypeID, created.ResourceTypeID)
	assert.NotEmpty(t, created.ResourceID)
}

func TestResourceVerifyLocationHeader(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
	}

	created, resp := createTestResource(t, srv, resource)
	require.Equal(t, http.StatusCreated, resp.Code)

	location := resp.Header().Get("Location")
	require.NotEmpty(t, location, "Location header should be set")
	require.Contains(t, location, "/o2ims/v1/resources/", "Location header should contain resource path")
	require.Contains(t, location, created.ResourceID, "Location header should contain the created resource ID")
}

func TestResourceValidationErrorEmptyResourceTypeID(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		ResourcePoolID: "pool-1",
		Description:    "Test resource",
	}

	resp, respBody := doResourceRequest(t, srv, http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", resource)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, string(respBody), "resource type ID is required")
}

func TestResourceValidationErrorEmptyResourcePoolID(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		ResourceTypeID: "machine",
		Description:    "Test resource",
	}

	resp, respBody := doResourceRequest(t, srv, http.MethodPost, "/o2ims-infrastructureInventory/v1/resources", resource)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, string(respBody), "resource pool ID is required")
}

func TestResourceSecurityRejectInvalidUUIDPathTraversal(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	testInvalidResourceID(t, "../../../etc/passwd", "resourceId must be a valid UUID")
}

func TestResourceSecurityRejectInvalidUUIDSQLInjection(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	testInvalidResourceID(t, "'; DROP TABLE resources; --", "resourceId must be a valid UUID")
}

func TestResourceAcceptValidClientProvidedUUID(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	// Test valid UUID is accepted
	validUUID := "550e8400-e29b-41d4-a716-446655440001"
	resource := adapter.Resource{
		ResourceID:     validUUID,
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Resource with client-provided UUID",
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusCreated, resp.Code)

	var created adapter.Resource
	err := json.Unmarshal(resp.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, validUUID, created.ResourceID)
}

func TestResourceUpdateUpdateResourceDescription(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		Description:   "Updated description",
		GlobalAssetID: "urn:updated:asset:123",
	}

	body, err := json.Marshal(resource)
	require.NoError(t, err)

	resourceID := "550e8400-e29b-41d4-a716-446655440000"
	url := "/o2ims-infrastructureInventory/v1/resources/" + resourceID

	req := httptest.NewRequest(
		http.MethodPut,
		url,
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	var updated adapter.Resource
	err = json.Unmarshal(resp.Body.Bytes(), &updated)
	require.NoError(t, err)
	assert.Equal(t, resourceID, updated.ResourceID)
	assert.Equal(t, resource.Description, updated.Description)
	assert.Equal(t, resource.GlobalAssetID, updated.GlobalAssetID)
}

func TestResourceUpdateUpdateExtensions(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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
}

func TestResourceUpdateInvalidJSON(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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
}

func TestResourceUpdateResourceNotFound(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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
}

func TestResourceUpdatePreserveImmutableFields(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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
}

func TestResourceUpdateRejectImmutableFieldModification(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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
}

func TestResourceInvalidGlobalAssetIDFormat(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		GlobalAssetID:  "invalid-not-urn",
		Description:    "Test resource",
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "urn:")
}

func TestResourceDescriptionTooLong(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	longDesc := strings.Repeat("a", 1001) // Exceeds 1000 char limit
	resource := adapter.Resource{
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    longDesc,
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "description")
}

func TestResourceTooManyExtensions(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "extensions")
}

func TestResourceExtensionKeyExceedsTwoFiftySixCharacters(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "extension keys must not exceed 256 characters")
}

func TestResourceExtensionValueExceedsFourKB(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
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

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "extension values must not exceed 4096 bytes")
}

func TestResourceExtensionsExceedsHundredKeys(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Test POST /resources
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

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "extensions map must not exceed 100 keys")
}

func TestResourceTotalExtensionsPayloadExceedsFiftyKB(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Test POST /resources
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

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "total extensions payload must not exceed 50000 bytes")
}

func TestResourceCustomResourceID(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	resource := adapter.Resource{
		ResourceID:     "custom-resource-id",
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Resource with custom ID",
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusCreated, resp.Code)

	var created adapter.Resource
	err := json.Unmarshal(resp.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, "custom-resource-id", created.ResourceID)
}

func TestResourceDuplicateResourceID(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	srv := setupResourceTestServer(t, newMockResourceAdapter())

	// Test POST /resources
	// Use the existing resource ID from the mock
	resource := adapter.Resource{
		ResourceID:     "550e8400-e29b-41d4-a716-446655440000", // Already exists in mock
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Duplicate resource",
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusConflict, resp.Code)
	assert.Contains(t, resp.Body.String(), "already exists")
}

func TestResourceAdapterErrorOnCreate(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Test POST /resources
	// Create a mock that returns an error
	mockAdp := &mockResourceAdapter{
		resources: map[string]*adapter.Resource{},
	}
	srv := New(cfg, zap.NewNop(), &errorReturningAdapter{mockAdp, errorOnCreate}, &mockStore{})

	resource := adapter.Resource{
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Test resource",
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Contains(t, resp.Body.String(), "Failed to create resource")
}

func TestResourceUpdateAdapterErrorOnUpdate(t *testing.T) {
	t.Skip("Skipping due to issue #204 - refactoring needed")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Test POST /resources
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
	srv := New(cfg, zap.NewNop(), &errorReturningAdapter{mockAdp, errorOnUpdate}, &mockStore{})

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
}

// errorReturningAdapter wraps a mock adapter and returns errors for specific operations.
type errorReturningAdapter struct {
	*mockResourceAdapter
	errorOn string // errorOnCreate, errorOnUpdate, errorOnGet
}

func (e *errorReturningAdapter) CreateResource(
	ctx context.Context,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	if e.errorOn == errorOnCreate {
		return nil, errors.New("simulated adapter create error")
	}
	return e.mockResourceAdapter.CreateResource(ctx, resource)
}

func (e *errorReturningAdapter) UpdateResource(
	ctx context.Context,
	id string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	if e.errorOn == errorOnUpdate {
		return nil, errors.New("simulated adapter update error")
	}
	return e.mockResourceAdapter.UpdateResource(ctx, id, resource)
}

func (e *errorReturningAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	if e.errorOn == errorOnGet {
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
				body := marshalResourceToJSON(t, resource)
				resp := makeResourcePostRequest(t, srv, body)
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
					executeConcurrentCreate(t, srv, resourceID, results)
				} else {
					executeConcurrentGet(srv, resourceID, results)
				}
			}()
		}

		// Collect and verify results
		createSuccess, getSuccess, getNotFound := collectConcurrentResults(results, numGoroutines)

		// Exactly one create should succeed
		assert.Equal(t, 1, createSuccess, "Exactly one create should succeed")

		// Gets can be either 200 (if after create) or 404 (if before create)
		assert.Equal(t, numGoroutines/2, getSuccess+getNotFound,
			"All gets should complete with either 200 or 404")
	})
}

// executeConcurrentCreate performs a concurrent create operation for testing.
func executeConcurrentCreate(t *testing.T, srv *Server, resourceID string, results chan<- string) {
	t.Helper()
	resource := adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: "machine",
		ResourcePoolID: "pool-1",
		Description:    "Test",
	}

	body := marshalResourceToJSON(t, resource)
	resp := makeResourcePostRequest(t, srv, body)
	results <- fmt.Sprintf("create:%d", resp.Code)
}

// executeConcurrentGet performs a concurrent get operation for testing.
func executeConcurrentGet(srv *Server, resourceID string, results chan<- string) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/o2ims-infrastructureInventory/v1/resources/"+resourceID,
		nil,
	)
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)
	results <- fmt.Sprintf("get:%d", resp.Code)
}

// collectConcurrentResults collects and tallies results from concurrent operations.
func collectConcurrentResults(results <-chan string, numGoroutines int) (int, int, int) {
	var createSuccess, getSuccess, getNotFound int
	for i := 0; i < numGoroutines; i++ {
		result := <-results
		parts := bytes.Split([]byte(result), []byte(":"))
		op := string(parts[0])
		code := string(parts[1])

		switch {
		case op == errorOnCreate && code == "201":
			createSuccess++
		case op == errorOnGet && code == "200":
			getSuccess++
		case op == errorOnGet && code == "404":
			getNotFound++
		}
	}
	return createSuccess, getSuccess, getNotFound
}
