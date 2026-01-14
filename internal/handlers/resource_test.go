package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/piwi3910/netweave/internal/handlers"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// mockResourceAdapter implements adapter.Adapter for testing.
type mockResourceAdapter struct {
	resources         []*adapter.Resource
	listErr           error
	getErr            error
	createErr         error
	deleteErr         error
	resourcePools     []*adapter.ResourcePool
	resourceTypes     []*adapter.ResourceType
	deploymentManager *adapter.DeploymentManager
}

func (m *mockResourceAdapter) Name() string    { return "mock" }
func (m *mockResourceAdapter) Version() string { return "1.0.0" }
func (m *mockResourceAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{adapter.CapabilityResources, adapter.CapabilityResourcePools}
}

func (m *mockResourceAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	if m.deploymentManager == nil {
		return nil, errors.New("deployment manager not configured")
	}
	return m.deploymentManager, nil
}

func (m *mockResourceAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return m.resourcePools, nil
}

func (m *mockResourceAdapter) GetResourcePool(_ context.Context, poolID string) (*adapter.ResourcePool, error) {
	for _, pool := range m.resourcePools {
		if pool.ResourcePoolID == poolID {
			return pool, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockResourceAdapter) CreateResourcePool(
	_ context.Context,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	m.resourcePools = append(m.resourcePools, pool)
	return pool, nil
}

func (m *mockResourceAdapter) UpdateResourcePool(
	_ context.Context,
	_ string,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	return pool, nil
}

func (m *mockResourceAdapter) DeleteResourcePool(_ context.Context, _ string) error {
	return nil
}

func (m *mockResourceAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.resources, nil
}

func (m *mockResourceAdapter) GetResource(_ context.Context, resourceID string) (*adapter.Resource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, resource := range m.resources {
		if resource.ResourceID == resourceID {
			return resource, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockResourceAdapter) CreateResource(_ context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.resources = append(m.resources, resource)
	return resource, nil
}

func (m *mockResourceAdapter) UpdateResource(
	_ context.Context,
	id string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	for i, res := range m.resources {
		if res.ResourceID == id {
			resource.ResourceID = id
			m.resources[i] = resource
			return resource, nil
		}
	}
	return nil, errors.New("resource not found")
}

func (m *mockResourceAdapter) DeleteResource(_ context.Context, _ string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *mockResourceAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return m.resourceTypes, nil
}

func (m *mockResourceAdapter) GetResourceType(_ context.Context, typeID string) (*adapter.ResourceType, error) {
	for _, rt := range m.resourceTypes {
		if rt.ResourceTypeID == typeID {
			return rt, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockResourceAdapter) CreateSubscription(
	_ context.Context,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	return sub, nil
}

func (m *mockResourceAdapter) GetSubscription(_ context.Context, subscriptionID string) (*adapter.Subscription, error) {
	return &adapter.Subscription{SubscriptionID: subscriptionID}, nil
}

func (m *mockResourceAdapter) UpdateSubscription(
	_ context.Context,
	id string,
	sub *adapter.Subscription,
) (*adapter.Subscription, error) {
	sub.SubscriptionID = id
	return sub, nil
}

func (m *mockResourceAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return nil
}

func (m *mockResourceAdapter) Health(_ context.Context) error {
	return nil
}

func (m *mockResourceAdapter) Close() error {
	return nil
}

func TestNewResourceHandler(t *testing.T) {
	adp := &mockResourceAdapter{}
	logger := zap.NewNop()

	handler := handlers.NewResourceHandler(adp, logger)
	assert.NotNil(t, handler)
	assert.Equal(t, adp, handler.Adapter)
	assert.Equal(t, logger, handler.Logger)
}

func TestNewResourceHandler_Panics(t *testing.T) {
	adp := &mockResourceAdapter{}
	logger := zap.NewNop()

	tests := []struct {
		name    string
		adapter adapter.Adapter
		logger  *zap.Logger
	}{
		{"nil adapter", nil, logger},
		{"nil logger", adp, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				handlers.NewResourceHandler(tt.adapter, tt.logger)
			})
		})
	}
}

func TestListResources_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{
		resources: []*adapter.Resource{
			{
				ResourceID:     "res-1",
				ResourceTypeID: "type-1",
				ResourcePoolID: "pool-1",
				Description:    "Test resource 1",
			},
			{
				ResourceID:     "res-2",
				ResourceTypeID: "type-2",
				ResourcePoolID: "pool-2",
				Description:    "Test resource 2",
			},
		},
	}

	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources", handler.ListResources)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.Items, 2)
}

func TestListResources_WithFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{
		resources: []*adapter.Resource{
			{
				ResourceID:     "res-1",
				ResourceTypeID: "type-1",
				ResourcePoolID: "pool-1",
			},
		},
	}

	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources", handler.ListResources)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources?resourcePoolId=pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 1, response.TotalCount)
}

func TestListResources_AdapterError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{
		listErr: errors.New("database error"),
	}

	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources", handler.ListResources)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "InternalError", response.Error)
}

func TestGetResource_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{
		resources: []*adapter.Resource{
			{
				ResourceID:     "res-1",
				ResourceTypeID: "type-1",
				ResourcePoolID: "pool-1",
				Description:    "Test resource",
				GlobalAssetID:  "asset-123",
			},
		},
	}

	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources/:resourceId", handler.GetResource)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/res-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Resource
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "res-1", response.ResourceID)
	assert.Equal(t, "type-1", response.ResourceTypeID)
	assert.Equal(t, "pool-1", response.ResourcePoolID)
	assert.Equal(t, "Test resource", response.Description)
	assert.Equal(t, "asset-123", response.GlobalAssetID)
}

func TestGetResource_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{
		resources: []*adapter.Resource{},
	}

	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources/:resourceId", handler.GetResource)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "NotFound", response.Error)
}

func TestGetResource_EmptyID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{}
	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources/:resourceId", handler.GetResource)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/", nil)
	router.ServeHTTP(w, req)

	// Gin router won't match this route, so it returns 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetResource_AdapterError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adp := &mockResourceAdapter{
		getErr: errors.New("database connection failed"),
	}

	handler := handlers.NewResourceHandler(adp, zap.NewNop())

	router := gin.New()
	router.GET("/o2ims/v1/resources/:resourceId", handler.GetResource)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resources/res-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "InternalError", response.Error)
}
