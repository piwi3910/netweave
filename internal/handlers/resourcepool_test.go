package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// mockAdapter is a mock implementation of adapter.Adapter for testing.
type mockAdapter struct {
	listResourcePoolsFunc  func(context.Context, *adapter.Filter) ([]*adapter.ResourcePool, error)
	getResourcePoolFunc    func(context.Context, string) (*adapter.ResourcePool, error)
	createResourcePoolFunc func(context.Context, *adapter.ResourcePool) (*adapter.ResourcePool, error)
	updateResourcePoolFunc func(context.Context, *adapter.ResourcePool) (*adapter.ResourcePool, error)
	deleteResourcePoolFunc func(context.Context, string) error
}

// Implement adapter.Adapter interface methods.
func (m *mockAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	if m.listResourcePoolsFunc != nil {
		return m.listResourcePoolsFunc(ctx, filter)
	}
	return []*adapter.ResourcePool{}, nil
}

func (m *mockAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	if m.getResourcePoolFunc != nil {
		return m.getResourcePoolFunc(ctx, id)
	}
	return nil, errors.New("not found")
}

func (m *mockAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	if m.createResourcePoolFunc != nil {
		return m.createResourcePoolFunc(ctx, pool)
	}
	pool.ResourcePoolID = "pool-placeholder"
	return pool, nil
}

func (m *mockAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	if m.updateResourcePoolFunc != nil {
		return m.updateResourcePoolFunc(ctx, pool)
	}
	return nil, errors.New("not found")
}

func (m *mockAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	if m.deleteResourcePoolFunc != nil {
		return m.deleteResourcePoolFunc(ctx, id)
	}
	return errors.New("not found")
}

// Stub methods for other adapter.Adapter interface requirements.
func (m *mockAdapter) Name() string {
	return "mock-adapter"
}
func (m *mockAdapter) Version() string {
	return "1.0.0"
}
func (m *mockAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{adapter.CapabilityResourcePools}
}
func (m *mockAdapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	return nil, nil
}
func (m *mockAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, nil
}
func (m *mockAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	return nil, nil
}
func (m *mockAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	return nil, nil
}
func (m *mockAdapter) DeleteResource(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}
func (m *mockAdapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	return nil, nil
}
func (m *mockAdapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, nil
}
func (m *mockAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	return nil, nil
}
func (m *mockAdapter) DeleteSubscription(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdapter) Health(ctx context.Context) error {
	return nil
}
func (m *mockAdapter) Close() error {
	return nil
}

// setupTestRouter creates a test Gin router with the ResourcePoolHandler.
func setupTestRouter(t *testing.T) (*gin.Engine, *ResourcePoolHandler) {
	t.Helper()

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create router
	router := gin.New()

	// Create mock adapter
	mockAdp := &mockAdapter{}

	// Create test logger
	logger := zap.NewNop()

	// Create handler
	handler := NewResourcePoolHandler(mockAdp, logger)

	// Register routes
	router.GET("/o2ims/v1/resourcePools", handler.ListResourcePools)
	router.GET("/o2ims/v1/resourcePools/:resourcePoolId", handler.GetResourcePool)
	router.POST("/o2ims/v1/resourcePools", handler.CreateResourcePool)
	router.PUT("/o2ims/v1/resourcePools/:resourcePoolId", handler.UpdateResourcePool)
	router.DELETE("/o2ims/v1/resourcePools/:resourcePoolId", handler.DeleteResourcePool)

	return router, handler
}

// TestResourcePoolHandler_ListResourcePools tests listing resource pools.
func TestResourcePoolHandler_ListResourcePools(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		wantStatus     int
		validateBody   func(*testing.T, []byte)
		validateHeader func(*testing.T, http.Header)
	}{
		{
			name:       "list all resource pools",
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ListResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				// Stub implementation returns empty list
				assert.NotNil(t, response.Items)
				assert.Equal(t, 0, response.TotalCount)
			},
		},
		{
			name:        "list with filter parameter",
			queryParams: "?filter=location:us-east-1",
			wantStatus:  http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ListResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotNil(t, response.Items)
			},
		},
		{
			name:        "list with pagination",
			queryParams: "?offset=10&limit=20",
			wantStatus:  http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ListResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotNil(t, response.Items)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestRouter(t)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resourcePools"+tt.queryParams, nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Assert status code
			assert.Equal(t, tt.wantStatus, w.Code)

			// Validate response body
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}

			// Validate response headers
			if tt.validateHeader != nil {
				tt.validateHeader(t, w.Header())
			}
		})
	}
}

// TestResourcePoolHandler_GetResourcePool tests retrieving a specific resource pool.
func TestResourcePoolHandler_GetResourcePool(t *testing.T) {
	tests := []struct {
		name         string
		resourceID   string
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:       "get existing resource pool",
			resourceID: "pool-123",
			wantStatus: http.StatusNotFound, // Stub returns 404
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
				assert.Contains(t, response.Message, "pool-123")
				assert.Equal(t, http.StatusNotFound, response.Code)
			},
		},
		{
			name:       "get with empty ID",
			resourceID: "",
			wantStatus: http.StatusMovedPermanently, // Gin returns 301 for trailing slash redirect
		},
		{
			name:       "get with special characters in ID",
			resourceID: "pool-abc-123-xyz",
			wantStatus: http.StatusNotFound, // Stub returns 404
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Contains(t, response.Message, "pool-abc-123-xyz")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestRouter(t)

			// Create request
			url := "/o2ims/v1/resourcePools/" + tt.resourceID
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Assert status code
			assert.Equal(t, tt.wantStatus, w.Code)

			// Validate response body
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestResourcePoolHandler_CreateResourcePool tests creating resource pools.
func TestResourcePoolHandler_CreateResourcePool(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  interface{}
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name: "create valid resource pool",
			requestBody: models.ResourcePool{
				Name:        "Test Pool",
				Description: "A test resource pool",
				Location:    "us-east-1",
				OCloudID:    "ocloud-123",
			},
			wantStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ResourcePool
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.ResourcePoolID)
				assert.Equal(t, "Test Pool", response.Name)
			},
		},
		{
			name: "create with minimal fields",
			requestBody: models.ResourcePool{
				Name:     "Minimal Pool",
				OCloudID: "ocloud-456",
			},
			wantStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ResourcePool
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.ResourcePoolID)
			},
		},
		{
			name:        "create with invalid JSON",
			requestBody: `{invalid json}`,
			wantStatus:  http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
				assert.Contains(t, response.Message, "Invalid request body")
			},
		},
		{
			name:        "create with empty body",
			requestBody: nil,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestRouter(t)

			// Prepare request body
			var body []byte
			var err error
			if tt.requestBody != nil {
				switch v := tt.requestBody.(type) {
				case string:
					body = []byte(v)
				default:
					body, err = json.Marshal(tt.requestBody)
					require.NoError(t, err)
				}
			}

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Assert status code
			assert.Equal(t, tt.wantStatus, w.Code)

			// Validate response body
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestResourcePoolHandler_UpdateResourcePool tests updating resource pools.
func TestResourcePoolHandler_UpdateResourcePool(t *testing.T) {
	tests := []struct {
		name         string
		resourceID   string
		requestBody  interface{}
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:       "update existing resource pool",
			resourceID: "pool-123",
			requestBody: models.ResourcePool{
				Name:        "Updated Pool",
				Description: "Updated description",
			},
			wantStatus: http.StatusNotFound, // Stub returns 404
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
				assert.Contains(t, response.Message, "pool-123")
			},
		},
		{
			name:       "update non-existent resource pool",
			resourceID: "pool-nonexistent",
			requestBody: models.ResourcePool{
				Name: "Updated Pool",
			},
			wantStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
			},
		},
		{
			name:        "update with invalid JSON",
			resourceID:  "pool-123",
			requestBody: `{invalid json}`,
			wantStatus:  http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
			},
		},
		{
			name:        "update with empty body",
			resourceID:  "pool-123",
			requestBody: nil,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestRouter(t)

			// Prepare request body
			var body []byte
			var err error
			if tt.requestBody != nil {
				switch v := tt.requestBody.(type) {
				case string:
					body = []byte(v)
				default:
					body, err = json.Marshal(tt.requestBody)
					require.NoError(t, err)
				}
			}

			// Create request
			url := "/o2ims/v1/resourcePools/" + tt.resourceID
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Assert status code
			assert.Equal(t, tt.wantStatus, w.Code)

			// Validate response body
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestResourcePoolHandler_DeleteResourcePool tests deleting resource pools.
func TestResourcePoolHandler_DeleteResourcePool(t *testing.T) {
	tests := []struct {
		name         string
		resourceID   string
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:       "delete existing resource pool",
			resourceID: "pool-123",
			wantStatus: http.StatusNotFound, // Stub returns 404
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
				assert.Contains(t, response.Message, "pool-123")
			},
		},
		{
			name:       "delete non-existent resource pool",
			resourceID: "pool-nonexistent",
			wantStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
			},
		},
		{
			name:       "delete with empty ID",
			resourceID: "",
			wantStatus: http.StatusNotFound, // Gin returns 404 for DELETE without trailing slash
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestRouter(t)

			// Create request
			url := "/o2ims/v1/resourcePools/" + tt.resourceID
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Assert status code
			assert.Equal(t, tt.wantStatus, w.Code)

			// Validate response body
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestResourcePoolHandler_ContentTypeHandling tests content type validation.
func TestResourcePoolHandler_ContentTypeHandling(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		acceptType  string
		method      string
		wantStatus  int
	}{
		{
			name:        "POST with application/json",
			contentType: "application/json",
			acceptType:  "application/json",
			method:      http.MethodPost,
			wantStatus:  http.StatusCreated,
		},
		{
			name:        "POST with charset",
			contentType: "application/json; charset=utf-8",
			acceptType:  "application/json",
			method:      http.MethodPost,
			wantStatus:  http.StatusCreated,
		},
		{
			name:       "GET with any accept type",
			acceptType: "*/*",
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := setupTestRouter(t)

			// Prepare request body for POST/PUT
			var body []byte
			if tt.method == http.MethodPost || tt.method == http.MethodPut {
				pool := models.ResourcePool{
					Name:     "Test Pool",
					OCloudID: "ocloud-123",
				}
				var err error
				body, err = json.Marshal(pool)
				require.NoError(t, err)
			}

			// Create request
			url := "/o2ims/v1/resourcePools"
			if tt.method == http.MethodPut || tt.method == http.MethodDelete {
				url += "/pool-123"
			}
			req := httptest.NewRequest(tt.method, url, bytes.NewReader(body))

			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.acceptType != "" {
				req.Header.Set("Accept", tt.acceptType)
			}

			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Assert status code
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestResourcePoolHandler_ErrorResponses tests error response formatting.
func TestResourcePoolHandler_ErrorResponses(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("404 error format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resourcePools/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response models.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response.Error)
		assert.NotEmpty(t, response.Message)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("400 error format", func(t *testing.T) {
		// Send invalid JSON
		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools",
			bytes.NewReader([]byte(`{invalid}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response models.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "BadRequest", response.Error)
		assert.NotEmpty(t, response.Message)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
