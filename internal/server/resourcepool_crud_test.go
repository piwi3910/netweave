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
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
)

// mockResourcePoolAdapter is a mock adapter specifically for resource pool tests.
type mockResourcePoolAdapter struct {
	mockAdapter
	pools map[string]*adapter.ResourcePool
}

func newMockResourcePoolAdapter() *mockResourcePoolAdapter {
	return &mockResourcePoolAdapter{
		pools: map[string]*adapter.ResourcePool{
			"existing-pool": {
				ResourcePoolID: "existing-pool",
				Name:           "Existing Pool",
				Description:    "An existing test pool",
				Location:       "us-west-1",
			},
		},
	}
}

func (m *mockResourcePoolAdapter) CreateResourcePool(
	_ context.Context,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	if _, exists := m.pools[pool.ResourcePoolID]; exists {
		return nil, fmt.Errorf(
			"resource pool with ID %s already exists: %w", pool.ResourcePoolID, adapter.ErrResourcePoolExists,
		)
	}
	m.pools[pool.ResourcePoolID] = pool
	return pool, nil
}

func (m *mockResourcePoolAdapter) UpdateResourcePool(
	_ context.Context,
	id string,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	if _, exists := m.pools[id]; !exists {
		return nil, adapter.ErrResourcePoolNotFound
	}
	pool.ResourcePoolID = id
	m.pools[id] = pool
	return pool, nil
}

func (m *mockResourcePoolAdapter) DeleteResourcePool(_ context.Context, id string) error {
	if _, exists := m.pools[id]; !exists {
		return adapter.ErrResourcePoolNotFound
	}
	delete(m.pools, id)
	return nil
}

// errorReturningResourcePoolAdapter is a mock that returns errors for specific operations.
type errorReturningResourcePoolAdapter struct {
	mockResourcePoolAdapter
	errorOn string // "create", "update", "delete"
}

func (e *errorReturningResourcePoolAdapter) CreateResourcePool(
	ctx context.Context,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	if e.errorOn == "create" {
		return nil, errors.New("simulated adapter create error")
	}
	return e.mockResourcePoolAdapter.CreateResourcePool(ctx, pool)
}

func (e *errorReturningResourcePoolAdapter) UpdateResourcePool(
	ctx context.Context,
	id string,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	if e.errorOn == "update" {
		return nil, errors.New("simulated adapter update error")
	}
	return e.mockResourcePoolAdapter.UpdateResourcePool(ctx, id, pool)
}

func (e *errorReturningResourcePoolAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	if e.errorOn == "delete" {
		return errors.New("simulated adapter delete error")
	}
	return e.mockResourcePoolAdapter.DeleteResourcePool(ctx, id)
}

func TestResourcePoolCreateResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        "test-pool",
		Description: "Test resource pool",
		Location:    "us-west-1",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)

	var created adapter.ResourcePool
	err = json.Unmarshal(resp.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, pool.Name, created.Name)
	assert.Equal(t, pool.Description, created.Description)
	assert.Equal(t, pool.Location, created.Location)
	assert.NotEmpty(t, created.ResourcePoolID)
	// ID should start with "pool-test-pool-" followed by full UUID (36 chars)
	assert.Contains(t, created.ResourcePoolID, "pool-test-pool-")
	assert.Len(t, created.ResourcePoolID, len("pool-test-pool-")+36)
}

func TestResourcePoolCreateWithCustomID(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		ResourcePoolID: "custom-pool-123",
		Name:           "test-pool",
		Description:    "Test resource pool",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)

	var created adapter.ResourcePool
	err = json.Unmarshal(resp.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, "custom-pool-123", created.ResourcePoolID)
}

func TestResourcePoolDuplicateResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		ResourcePoolID: "existing-pool",
		Name:           "test-pool",
		Description:    "Test resource pool",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusConflict, resp.Code)
	assert.Contains(t, resp.Body.String(), "already exists")
}

func TestResourcePoolValidationErrorEmptyName(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Description: "Test pool",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "name is required")
}

func TestResourcePoolValidationErrorNameTooLong(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name: strings.Repeat("a", MaxResourcePoolNameLength+1), // Exceeds max length
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "name must not exceed 255 characters")
}

func TestResourcePoolValidationErrorInvalidIDCharacters(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		ResourcePoolID: "invalid/pool/../id", // Contains invalid characters
		Name:           "test-pool",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "resourcePoolId must contain only alphanumeric characters")
}

func TestResourcePoolValidationErrorDescriptionTooLong(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        "test-pool",
		Description: strings.Repeat("a", MaxResourcePoolDescriptionLength+1), // Exceeds max length
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "description must not exceed 1000 characters")
}

func TestResourcePoolMultipleValidationErrors(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        strings.Repeat("a", 256),  // Name too long
		Description: strings.Repeat("b", 1001), // Description too long
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	// Should contain both error messages
	assert.Contains(t, resp.Body.String(), "name must not exceed 255 characters")
	assert.Contains(t, resp.Body.String(), "description must not exceed 1000 characters")
	assert.Contains(t, resp.Body.String(), ";")
}

func TestResourcePoolSanitizeNameWithSpecialCharacters(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        "Test Pool / With * Special <> Chars",
		Description: "Testing sanitization",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)

	var created adapter.ResourcePool
	err = json.Unmarshal(resp.Body.Bytes(), &created)
	require.NoError(t, err)
	// ID should be sanitized (no special characters)
	assert.NotContains(t, created.ResourcePoolID, "/")
	assert.NotContains(t, created.ResourcePoolID, "*")
	assert.NotContains(t, created.ResourcePoolID, "<")
	assert.NotContains(t, created.ResourcePoolID, ">")
}

func TestResourcePoolInvalidJSON(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/o2ims-infrastructureInventory/v1/resourcePools",
		bytes.NewReader([]byte("invalid json")),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "Invalid request body")
}

func TestResourcePoolUpdateResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        "Updated Pool",
		Description: "Updated description",
		Location:    "us-east-1",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var updated adapter.ResourcePool
	err = json.Unmarshal(resp.Body.Bytes(), &updated)
	require.NoError(t, err)
	assert.Equal(t, "existing-pool", updated.ResourcePoolID)
	assert.Equal(t, pool.Name, updated.Name)
	assert.Equal(t, pool.Description, updated.Description)
	assert.Equal(t, pool.Location, updated.Location)
}

func TestResourcePoolUpdateResourcePoolNotFound(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        "Updated Pool",
		Description: "Updated description",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/nonexistent-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)
	assert.Contains(t, resp.Body.String(), "not found")
}

func TestResourcePoolUpdateInvalidJSON(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader([]byte("invalid json")),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "Invalid request body")
}

func TestResourcePoolUpdateValidationErrorEmptyName(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name: "", // Empty name - should fail validation
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "name is required")
}

func TestResourcePoolUpdateValidationErrorNameTooLong(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name: strings.Repeat("a", 256), // Name too long
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "name must not exceed 255 characters")
}

func TestResourcePoolUpdateValidationErrorInvalidIDCharacters(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:           "Valid Name",
		ResourcePoolID: "invalid@id!", // Invalid characters
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "resourcePoolId must contain only alphanumeric characters")
}

func TestResourcePoolUpdateValidationErrorDescriptionTooLong(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	pool := adapter.ResourcePool{
		Name:        "Valid Name",
		Description: strings.Repeat("b", 1001), // Description too long
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "description must not exceed 1000 characters")
}

func TestResourcePoolDeleteResourcePool(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/existing-pool", nil)
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
	assert.Empty(t, resp.Body.String())
}

func TestResourcePoolDeleteResourcePoolNotFound(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), newMockResourcePoolAdapter(), &mockStore{})

	req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/nonexistent-pool", nil)
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)
	assert.Contains(t, resp.Body.String(), "not found")
}

func TestResourcePoolCreateAdapterError(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	mockAdp := &errorReturningResourcePoolAdapter{
		mockResourcePoolAdapter: *newMockResourcePoolAdapter(),
		errorOn:                 "create",
	}
	srv := New(cfg, zap.NewNop(), mockAdp, &mockStore{})

	pool := adapter.ResourcePool{
		Name: "test-pool",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Contains(t, resp.Body.String(), "Failed to create resource pool")
}

func TestResourcePoolUpdateAdapterError(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	mockAdp := &errorReturningResourcePoolAdapter{
		mockResourcePoolAdapter: *newMockResourcePoolAdapter(),
		errorOn:                 "update",
	}
	srv := New(cfg, zap.NewNop(), mockAdp, &mockStore{})

	pool := adapter.ResourcePool{
		Name: "test-pool",
	}

	body, err := json.Marshal(pool)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/o2ims-infrastructureInventory/v1/resourcePools/existing-pool",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Contains(t, resp.Body.String(), "Failed to update resource pool")
}

func TestResourcePoolDeleteAdapterError(t *testing.T) {
	t.Skip("Skipping - Prometheus metrics registry conflict - see issue #204")
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	mockAdp := &errorReturningResourcePoolAdapter{
		mockResourcePoolAdapter: *newMockResourcePoolAdapter(),
		errorOn:                 "delete",
	}
	srv := New(cfg, zap.NewNop(), mockAdp, &mockStore{})

	req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/existing-pool", nil)
	resp := httptest.NewRecorder()

	srv.router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Contains(t, resp.Body.String(), "Failed to delete resource pool")
}

// TestSanitizeResourcePoolID tests the ID sanitization function.
func TestSanitizeResourcePoolID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "test-pool",
			expected: "test-pool",
		},
		{
			name:     "spaces to hyphens",
			input:    "Test Pool",
			expected: "test-pool",
		},
		{
			name:     "path traversal attempt - dots dropped, slashes become hyphens",
			input:    "../../../etc/passwd",
			expected: "---etc-passwd",
		},
		{
			name:     "special characters dropped",
			input:    "pool*name?with<special>chars",
			expected: "poolnamewithspecialchars",
		},
		{
			name:     "mixed case to lowercase",
			input:    "MyPoolName",
			expected: "mypoolname",
		},
		{
			name:     "preserve underscores and hyphens",
			input:    "pool_name-123",
			expected: "pool_name-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeResourcePoolID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestValidateResourcePoolFields tests the validation function.
func TestValidateResourcePoolFields(t *testing.T) {
	tests := []struct {
		name        string
		pool        *adapter.ResourcePool
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid pool",
			pool: &adapter.ResourcePool{
				Name:        "test-pool",
				Description: "Test description",
			},
			expectError: false,
		},
		{
			name: "empty name",
			pool: &adapter.ResourcePool{
				Description: "Test description",
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "name too long",
			pool: &adapter.ResourcePool{
				Name: strings.Repeat("a", 256),
			},
			expectError: true,
			errorMsg:    "name must not exceed 255 characters",
		},
		{
			name: "invalid ID characters",
			pool: &adapter.ResourcePool{
				ResourcePoolID: "invalid/id",
				Name:           "test",
			},
			expectError: true,
			errorMsg:    "resourcePoolId must contain only alphanumeric characters",
		},
		{
			name: "description too long",
			pool: &adapter.ResourcePool{
				Name:        "test",
				Description: strings.Repeat("a", 1001),
			},
			expectError: true,
			errorMsg:    "description must not exceed 1000 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourcePoolFields(tt.pool)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
