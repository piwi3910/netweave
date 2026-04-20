package handlers_test

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
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// === Additional mock adapter variants for better coverage ===

// successMockAdapter is a mock adapter that successfully returns data for all operations.
type successMockAdapter struct {
	pools     map[string]*adapter.ResourcePool
	resources map[string]*adapter.Resource
}

func newSuccessMockAdapter() *successMockAdapter {
	return &successMockAdapter{
		pools:     make(map[string]*adapter.ResourcePool),
		resources: make(map[string]*adapter.Resource),
	}
}

func (m *successMockAdapter) Name() string                       { return "success-mock" }
func (m *successMockAdapter) Version() string                    { return "1.0.0" }
func (m *successMockAdapter) Capabilities() []adapter.Capability { return nil }
func (m *successMockAdapter) Health(_ context.Context) error     { return nil }
func (m *successMockAdapter) Close() error                       { return nil }
func (m *successMockAdapter) ListDeploymentManagers(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	return nil, nil
}
func (m *successMockAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, errors.New("not found")
}
func (m *successMockAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	result := make([]*adapter.Resource, 0, len(m.resources))
	for _, r := range m.resources {
		result = append(result, r)
	}
	return result, nil
}
func (m *successMockAdapter) GetResource(_ context.Context, id string) (*adapter.Resource, error) {
	r, ok := m.resources[id]
	if !ok {
		return nil, adapter.ErrResourceNotFound
	}
	return r, nil
}
func (m *successMockAdapter) CreateResource(_ context.Context, r *adapter.Resource) (*adapter.Resource, error) {
	return r, nil
}
func (m *successMockAdapter) UpdateResource(_ context.Context, _ string, r *adapter.Resource) (*adapter.Resource, error) {
	return r, nil
}
func (m *successMockAdapter) DeleteResource(_ context.Context, _ string) error {
	return nil
}
func (m *successMockAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}
func (m *successMockAdapter) GetResourceType(_ context.Context, _ string) (*adapter.ResourceType, error) {
	return nil, errors.New("not found")
}
func (m *successMockAdapter) CreateSubscription(_ context.Context, s *adapter.Subscription) (*adapter.Subscription, error) {
	return s, nil
}
func (m *successMockAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, errors.New("not found")
}
func (m *successMockAdapter) UpdateSubscription(_ context.Context, _ string, s *adapter.Subscription) (*adapter.Subscription, error) {
	return s, nil
}
func (m *successMockAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return nil
}

func (m *successMockAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	result := make([]*adapter.ResourcePool, 0, len(m.pools))
	for _, p := range m.pools {
		result = append(result, p)
	}
	return result, nil
}

func (m *successMockAdapter) GetResourcePool(_ context.Context, id string) (*adapter.ResourcePool, error) {
	p, ok := m.pools[id]
	if !ok {
		return nil, adapter.ErrResourcePoolNotFound
	}
	return p, nil
}

func (m *successMockAdapter) CreateResourcePool(_ context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	if pool.ResourcePoolID == "" {
		pool.ResourcePoolID = "pool-generated-id"
	}
	m.pools[pool.ResourcePoolID] = pool
	return pool, nil
}

func (m *successMockAdapter) UpdateResourcePool(_ context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	if _, ok := m.pools[id]; !ok {
		return nil, adapter.ErrResourcePoolNotFound
	}
	pool.ResourcePoolID = id
	m.pools[id] = pool
	return pool, nil
}

func (m *successMockAdapter) DeleteResourcePool(_ context.Context, id string) error {
	if _, ok := m.pools[id]; !ok {
		return adapter.ErrResourcePoolNotFound
	}
	delete(m.pools, id)
	return nil
}

// errorListMockAdapter returns errors for list operations.
type errorListMockAdapter struct {
	successMockAdapter
}

func (m *errorListMockAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, errors.New("database connection failed")
}

// === Resource Pool handler tests with successful paths ===

func setupResourcePoolRouter(t *testing.T, adp adapter.Adapter) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()
	handler := handlers.NewResourcePoolHandler(adp, logger)

	router.GET("/o2ims/v1/resourcePools", handler.ListResourcePools)
	router.GET("/o2ims/v1/resourcePools/:resourcePoolId", handler.GetResourcePool)
	router.POST("/o2ims/v1/resourcePools", handler.CreateResourcePool)
	router.PUT("/o2ims/v1/resourcePools/:resourcePoolId", handler.UpdateResourcePool)
	router.DELETE("/o2ims/v1/resourcePools/:resourcePoolId", handler.DeleteResourcePool)

	return router
}

func TestResourcePool_GetSuccess(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID:   "pool-1",
		Name:             "Test Pool",
		Description:      "A test pool",
		Location:         "us-east-1",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "global-1",
		TenantID:         "", // No tenant restriction
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.ResourcePool
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "pool-1", resp.ResourcePoolID)
	assert.Equal(t, "Test Pool", resp.Name)
	assert.Equal(t, "A test pool", resp.Description)
	assert.Equal(t, "us-east-1", resp.Location)
}

func TestResourcePool_GetTenantMismatch(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "Test Pool",
		TenantID:       "tenant-other",
	}
	router := gin.New()
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	handler := handlers.NewResourcePoolHandler(adp, logger)

	// Add middleware to set tenant context
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{
			TenantID: "tenant-mine",
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/o2ims/v1/resourcePools/:resourcePoolId", handler.GetResourcePool)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResourcePool_UpdateSuccess(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "Original Pool",
		TenantID:       "", // No tenant restriction
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{
		Name:        "Updated Pool",
		Description: "Updated description",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/o2ims/v1/resourcePools/pool-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.ResourcePool
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "pool-1", resp.ResourcePoolID)
}

func TestResourcePool_UpdateTenantMismatch(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "Test Pool",
		TenantID:       "tenant-other",
	}
	router := gin.New()
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	handler := handlers.NewResourcePoolHandler(adp, logger)

	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{
			TenantID: "tenant-mine",
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/o2ims/v1/resourcePools/:resourcePoolId", handler.UpdateResourcePool)

	body, _ := json.Marshal(models.ResourcePool{Name: "Updated"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/o2ims/v1/resourcePools/pool-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResourcePool_UpdateGetError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return nil, errors.New("internal error")
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Updated"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/o2ims/v1/resourcePools/pool-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResourcePool_UpdateAdapterError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return &adapter.ResourcePool{ResourcePoolID: "pool-1"}, nil
		},
		updateResourcePoolFunc: func(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
			return nil, errors.New("internal update error")
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Updated"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/o2ims/v1/resourcePools/pool-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResourcePool_UpdateAdapterNotFoundError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return &adapter.ResourcePool{ResourcePoolID: "pool-1"}, nil
		},
		updateResourcePoolFunc: func(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
			return nil, adapter.ErrResourcePoolNotFound
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Updated"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/o2ims/v1/resourcePools/pool-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResourcePool_DeleteSuccess(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "Test Pool",
		TenantID:       "", // No tenant restriction
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestResourcePool_DeleteTenantMismatch(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID: "pool-1",
		Name:           "Test Pool",
		TenantID:       "tenant-other",
	}
	router := gin.New()
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	handler := handlers.NewResourcePoolHandler(adp, logger)

	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{
			TenantID: "tenant-mine",
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/o2ims/v1/resourcePools/:resourcePoolId", handler.DeleteResourcePool)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResourcePool_DeleteGetError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return nil, errors.New("internal error")
		},
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResourcePool_DeleteConflictError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return &adapter.ResourcePool{ResourcePoolID: "pool-1"}, nil
		},
		deleteResourcePoolFunc: func(_ context.Context, _ string) error {
			return adapter.ErrResourcePoolHasActiveResources
		},
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestResourcePool_DeleteAdapterNotFoundError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return &adapter.ResourcePool{ResourcePoolID: "pool-1"}, nil
		},
		deleteResourcePoolFunc: func(_ context.Context, _ string) error {
			return adapter.ErrResourcePoolNotFound
		},
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResourcePool_DeleteInternalError(t *testing.T) {
	adp := &mockAdapter{
		getResourcePoolFunc: func(_ context.Context, _ string) (*adapter.ResourcePool, error) {
			return &adapter.ResourcePool{ResourcePoolID: "pool-1"}, nil
		},
		deleteResourcePoolFunc: func(_ context.Context, _ string) error {
			return errors.New("unexpected server error")
		},
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/resourcePools/pool-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResourcePool_CreateAlreadyExists(t *testing.T) {
	adp := &mockAdapter{
		createResourcePoolFunc: func(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
			return nil, adapter.ErrResourcePoolExists
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Test Pool"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

// TestResourcePool_CreateWrappedSentinelReturnsConflict verifies that the
// handler detects adapter.ErrResourcePoolExists via errors.Is even when the
// adapter wraps the sentinel with additional context. Regression guard for
// C9: previously the handler used strings.Contains and would silently break
// if the wrapping changed the message.
func TestResourcePool_CreateWrappedSentinelReturnsConflict(t *testing.T) {
	adp := &mockAdapter{
		createResourcePoolFunc: func(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
			return nil, fmt.Errorf("%w: pool-xyz in tenant T1", adapter.ErrResourcePoolExists)
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Test Pool"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp models.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Conflict", resp.Error)
}

// TestResourcePool_CreateLookalikeStringReturns500 proves the handler does
// NOT fall back to string matching. A plain errors.New("resource pool
// already exists") looks like the old sentinel-message but does not wrap
// it, so the handler must return 500 instead of 409.
func TestResourcePool_CreateLookalikeStringReturns500(t *testing.T) {
	adp := &mockAdapter{
		createResourcePoolFunc: func(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
			return nil, errors.New("resource pool already exists")
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Test Pool"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"handler must NOT match on error strings; only wrapped sentinels count")
}

func TestResourcePool_CreateInternalError(t *testing.T) {
	adp := &mockAdapter{
		createResourcePoolFunc: func(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
			return nil, errors.New("database error")
		},
	}
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Name: "Test Pool"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResourcePool_CreateMissingName(t *testing.T) {
	adp := newSuccessMockAdapter()
	router := setupResourcePoolRouter(t, adp)

	body, _ := json.Marshal(models.ResourcePool{Description: "No name provided"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/resourcePools", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Message, "name is required")
}

func TestResourcePool_ListInternalError(t *testing.T) {
	adp := &errorListMockAdapter{}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resourcePools", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResourcePool_ListWithData(t *testing.T) {
	adp := newSuccessMockAdapter()
	adp.pools["pool-1"] = &adapter.ResourcePool{
		ResourcePoolID:   "pool-1",
		Name:             "Pool One",
		Location:         "us-east-1",
		GlobalLocationID: "g-1",
	}
	adp.pools["pool-2"] = &adapter.ResourcePool{
		ResourcePoolID: "pool-2",
		Name:           "Pool Two",
		Location:       "us-west-2",
	}
	router := setupResourcePoolRouter(t, adp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/resourcePools", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.ListResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.TotalCount)
}

// === Subscription handler tests for additional coverage ===

func TestSubscription_CreateEmptyCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	body, _ := json.Marshal(models.Subscription{
		Callback: "",
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreateSubscription(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Message, "Callback URL is required")
}

func TestSubscription_CreateAlreadyExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{
		createErr: storage.ErrSubscriptionExists,
	}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	body, _ := json.Marshal(models.Subscription{
		Callback: "https://example.com/notify",
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreateSubscription(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestSubscription_CreateWithTenantQuotaExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{}
	authStore := newMockAuthStore()

	quotaStore := &quotaExceededAuthStore{mockAuthStore: authStore}

	handler := handlers.NewSubscriptionHandler(store, quotaStore, zap.NewNop())

	body, _ := json.Marshal(models.Subscription{
		Callback: "https://example.com/notify",
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// Set tenant context after creating the request
	ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "t-1"})
	c.Request = c.Request.WithContext(ctx)

	handler.CreateSubscription(c)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestSubscription_CreateWithTenantQuotaError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{}
	authStore := newMockAuthStore()

	quotaStore := &quotaErrorAuthStore{mockAuthStore: authStore}

	handler := handlers.NewSubscriptionHandler(store, quotaStore, zap.NewNop())

	body, _ := json.Marshal(models.Subscription{
		Callback: "https://example.com/notify",
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "t-1"})
	c.Request = c.Request.WithContext(ctx)

	handler.CreateSubscription(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSubscription_GetEmptyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "subscriptionId", Value: ""}}
	c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions/", nil)

	handler.GetSubscription(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubscription_GetTenantMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				TenantID: "tenant-other",
				Callback: "https://example.com",
			},
		},
	}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "subscriptionId", Value: "sub-1"}}

	c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions/sub-1", nil)
	ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "tenant-mine"})
	c.Request = c.Request.WithContext(ctx)

	handler.GetSubscription(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSubscription_DeleteEmptyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "subscriptionId", Value: ""}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/o2ims/v1/subscriptions/", nil)

	handler.DeleteSubscription(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubscription_DeleteWithTenantVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				TenantID: "tenant-1",
				Callback: "https://example.com",
			},
		},
	}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "tenant-1"})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/o2ims/v1/subscriptions/:subscriptionId", handler.DeleteSubscription)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/subscriptions/sub-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSubscription_DeleteTenantMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				TenantID: "tenant-other",
				Callback: "https://example.com",
			},
		},
	}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "tenant-mine"})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/o2ims/v1/subscriptions/:subscriptionId", handler.DeleteSubscription)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/subscriptions/sub-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSubscription_DeleteTenantGetError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{
		getErr: errors.New("database error"),
	}
	handler := handlers.NewSubscriptionHandler(store, &mockAuthStore{}, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "tenant-1"})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/o2ims/v1/subscriptions/:subscriptionId", handler.DeleteSubscription)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/o2ims/v1/subscriptions/sub-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSubscription_ListByTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &mockSubscriptionStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				TenantID: "tenant-1",
				Callback: "https://example.com/notify",
			},
		},
	}
	// Override ListByTenant to return data
	tenantStore := &tenantListStore{mockSubscriptionStore: store}
	handler := handlers.NewSubscriptionHandler(tenantStore, &mockAuthStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions", nil)
	ctx := auth.ContextWithUser(c.Request.Context(), &auth.AuthenticatedUser{TenantID: "tenant-1"})
	c.Request = c.Request.WithContext(ctx)

	handler.ListSubscriptions(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

// === User handler additional tests ===

func TestUser_UpdateWithRoleChange(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
		RoleID:   "role-viewer",
	}
	store.roles["role-admin"] = &auth.Role{
		ID:   "role-admin",
		Name: auth.RoleTenantAdmin,
		Type: auth.RoleTypeTenant,
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateUserRequest{
		RoleID: "role-admin",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp auth.TenantUser
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "role-admin", resp.RoleID)
}

func TestUser_UpdateWithPlatformRole(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
		RoleID:   "role-viewer",
	}
	store.roles["role-platform"] = &auth.Role{
		ID:   "role-platform",
		Name: auth.RolePlatformAdmin,
		Type: auth.RoleTypePlatform,
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateUserRequest{
		RoleID: "role-platform",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_UpdateInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateUserRequest{
		Email: "not-valid-email",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_UpdateInvalidJSON(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}

	router := setupUserTestRouter(t, store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader([]byte("{invalid}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_CreateWithInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
		Quota:  auth.TenantQuota{MaxUsers: 10},
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.CreateUserRequest{
		Subject:    "CN=test,O=org",
		CommonName: "Test User",
		Email:      "not-an-email",
		RoleID:     "role-1",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenant/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_CreateUserExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
		Quota:  auth.TenantQuota{MaxUsers: 10},
	}
	store.roles["role-1"] = &auth.Role{
		ID:   "role-1",
		Name: auth.RoleViewer,
		Type: auth.RoleTypeTenant,
	}
	// Use a userExistsStore to simulate ErrUserExists on CreateUser
	dupStore := &userExistsStore{mockAuthStore: store}

	handler := handlers.NewUserHandler(dupStore, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant := store.tenants["tenant-1"]
		ctx := auth.ContextWithTenant(c.Request.Context(), tenant)
		user := &auth.AuthenticatedUser{UserID: "caller", TenantID: "tenant-1"}
		ctx = auth.ContextWithUser(ctx, user)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/admin/tenant/users", handler.CreateUser)

	body, _ := json.Marshal(handlers.CreateUserRequest{
		Subject:    "CN=existing,O=org",
		CommonName: "Existing User",
		RoleID:     "role-1",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenant/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUser_DeleteSelf(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
	}

	router := setupUserTestRouter(t, store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/tenant/users/user-1", nil)
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "user-1") // Same user trying to delete themselves
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_GetCurrentUserNoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newMockAuthStore()
	handler := handlers.NewUserHandler(store, zap.NewNop())

	// No user context set
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/tenant/me", nil)

	handler.GetCurrentUser(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// === Tenant handler additional tests ===

func TestTenant_CreateInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.CreateTenantRequest{
		Name:         "Valid Name",
		ContactEmail: "invalid-email",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/platform/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_CreateInvalidName(t *testing.T) {
	store := newMockAuthStore()
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.CreateTenantRequest{
		Name: "Invalid@Name!",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/platform/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_UpdateInvalidName(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		Name: "Invalid@Name!",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_UpdateInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		ContactEmail: "bad-email",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_UpdateWithQuota(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		Quota: &auth.TenantQuota{
			MaxSubscriptions: 200,
			MaxUsers:         50,
		},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTenant_UpdateContactEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		ContactEmail: "new@example.com",
		Metadata:     map[string]string{"key": "value"},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTenant_GetCurrentTenantWithContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newMockAuthStore()
	handler := handlers.NewTenantHandler(store, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant := &auth.Tenant{
			ID:     "tenant-1",
			Name:   "My Tenant",
			Status: auth.TenantStatusActive,
		}
		ctx := auth.ContextWithTenant(c.Request.Context(), tenant)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/admin/tenant", handler.GetCurrentTenant)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenant", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp auth.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "My Tenant", resp.Name)
}

// === Backend handler additional tests ===

func TestBackend_UpdateWithConfig(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{
		ID:          "b1",
		Name:        "k8s",
		Category:    "ims",
		AdapterType: "kubernetes",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	body, _ := json.Marshal(handlers.UpdateBackendRequest{
		Config:      map[string]string{"context": "new-prod"},
		Credentials: map[string]string{"token": "secret"},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "/admin/infrastructure/backends/b1", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBackend_UpdateEncryptionError(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{
		ID:          "b1",
		Name:        "k8s",
		Category:    "ims",
		AdapterType: "kubernetes",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{failEncrypt: true})

	body, _ := json.Marshal(handlers.UpdateBackendRequest{
		Config: map[string]string{"context": "prod"},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "/admin/infrastructure/backends/b1", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBackend_TestNotFound(t *testing.T) {
	store := newMockBackendStore()
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPost, "/admin/infrastructure/backends/missing/test", nil,
	)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBackend_UpdateInvalidJSON(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{
		ID: "b1", Name: "k8s",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "/admin/infrastructure/backends/b1", bytes.NewReader([]byte("{invalid}")),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// === Helper mock types ===

// quotaExceededAuthStore wraps mockAuthStore but returns ErrQuotaExceeded on IncrementUsage.
type quotaExceededAuthStore struct {
	*mockAuthStore
}

func (m *quotaExceededAuthStore) IncrementUsage(_ context.Context, _, _ string) error {
	return auth.ErrQuotaExceeded
}

// quotaErrorAuthStore wraps mockAuthStore but returns generic error on IncrementUsage.
type quotaErrorAuthStore struct {
	*mockAuthStore
}

func (m *quotaErrorAuthStore) IncrementUsage(_ context.Context, _, _ string) error {
	return errors.New("database connection failed")
}

// userExistsStore wraps mockAuthStore but returns ErrUserExists on CreateUser.
type userExistsStore struct {
	*mockAuthStore
}

func (m *userExistsStore) CreateUser(_ context.Context, _ *auth.TenantUser) error {
	return auth.ErrUserExists
}

// tenantListStore wraps mockSubscriptionStore with working ListByTenant.
type tenantListStore struct {
	*mockSubscriptionStore
}

func (m *tenantListStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}

// testAdapterVersion is declared in batch_test.go.
