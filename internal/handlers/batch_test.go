package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/piwi3910/netweave/internal/storage"
)

// mockBatchAdapter implements adapter.Adapter for batch testing.
type mockBatchAdapter struct {
	mu                 sync.Mutex
	resourcePools      []*adapter.ResourcePool
	createPoolErr      error
	getPoolErr         error
	deletePoolErr      error
	createPoolCount    int
	failOnCreatePool   int // Fail on nth create (0 = never fail)
	createSubscription func(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error)
}

func (m *mockBatchAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return m.resourcePools, nil
}

func (m *mockBatchAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	if m.getPoolErr != nil {
		return nil, m.getPoolErr
	}
	for _, pool := range m.resourcePools {
		if pool.ResourcePoolID == id {
			return pool, nil
		}
	}
	return nil, errors.New("resource pool not found")
}

func (m *mockBatchAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createPoolCount++
	if m.failOnCreatePool > 0 && m.createPoolCount == m.failOnCreatePool {
		return nil, errors.New("simulated create failure")
	}
	if m.createPoolErr != nil {
		return nil, m.createPoolErr
	}
	m.resourcePools = append(m.resourcePools, pool)
	return pool, nil
}

func (m *mockBatchAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}

func (m *mockBatchAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	if m.deletePoolErr != nil {
		return m.deletePoolErr
	}
	for i, pool := range m.resourcePools {
		if pool.ResourcePoolID == id {
			m.resourcePools = append(m.resourcePools[:i], m.resourcePools[i+1:]...)
			return nil
		}
	}
	return errors.New("resource pool not found")
}

func (m *mockBatchAdapter) Name() string {
	return "mock-adapter"
}

func (m *mockBatchAdapter) Version() string {
	return "1.0.0"
}

func (m *mockBatchAdapter) Capabilities() []adapter.Capability {
	return nil
}

func (m *mockBatchAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, nil
}

func (m *mockBatchAdapter) GetResource(_ context.Context, _ string) (*adapter.Resource, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) CreateResource(_ context.Context, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) UpdateResource(_ context.Context, _ string, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) DeleteResource(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (m *mockBatchAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}

func (m *mockBatchAdapter) GetResourceType(_ context.Context, _ string) (*adapter.ResourceType, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) CreateSubscription(_ context.Context, _ *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (m *mockBatchAdapter) ListDeploymentManagers(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	return nil, nil
}

func (m *mockBatchAdapter) UpdateDeploymentManager(_ context.Context, _ string, _ *adapter.DeploymentManager) (*adapter.DeploymentManager, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) UpdateSubscription(_ context.Context, _ string, _ *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchAdapter) Close() error {
	return nil
}

func (m *mockBatchAdapter) Health(ctx context.Context) error {
	return nil
}

// mockBatchStore implements storage.Store for batch testing.
type mockBatchStore struct {
	mu            sync.Mutex
	subscriptions []*storage.Subscription
	createErr     error
	getErr        error
	deleteErr     error
}

func (m *mockBatchStore) Create(ctx context.Context, sub *storage.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}
	sub.CreatedAt = time.Now()
	m.subscriptions = append(m.subscriptions, sub)
	return nil
}

func (m *mockBatchStore) Get(ctx context.Context, id string) (*storage.Subscription, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, sub := range m.subscriptions {
		if sub.ID == id {
			return sub, nil
		}
	}
	return nil, storage.ErrSubscriptionNotFound
}

func (m *mockBatchStore) Update(ctx context.Context, sub *storage.Subscription) error {
	return nil
}

func (m *mockBatchStore) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for i, sub := range m.subscriptions {
		if sub.ID == id {
			m.subscriptions = append(m.subscriptions[:i], m.subscriptions[i+1:]...)
			return nil
		}
	}
	return storage.ErrSubscriptionNotFound
}

func (m *mockBatchStore) List(ctx context.Context) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}

func (m *mockBatchStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockBatchStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockBatchStore) ListByTenant(ctx context.Context, tenantID string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (m *mockBatchStore) Close() error {
	return nil
}

func (m *mockBatchStore) Ping(ctx context.Context) error {
	return nil
}

func TestNewBatchHandler(t *testing.T) {
	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()

	handler := NewBatchHandler(adapter, store, logger)
	assert.NotNil(t, handler)
	assert.Equal(t, adapter, handler.adapter)
	assert.Equal(t, store, handler.store)
	assert.Equal(t, logger, handler.logger)
}

func TestNewBatchHandler_Panics(t *testing.T) {
	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()

	t.Run("nil adapter panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewBatchHandler(nil, store, logger)
		})
	})

	t.Run("nil store panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewBatchHandler(adapter, nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewBatchHandler(adapter, store, nil)
		})
	})
}

func TestBatchCreateSubscriptions_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchSubscriptionCreate{
		Subscriptions: []models.Subscription{
			{
				Callback: "https://example.com/callback1",
			},
			{
				Callback: "https://example.com/callback2",
			},
		},
		Atomic: false,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchCreateSubscriptions(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, 2, response.SuccessCount)
	assert.Equal(t, 0, response.FailureCount)
	assert.Len(t, response.Results, 2)
}

func TestBatchCreateSubscriptions_InvalidCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchSubscriptionCreate{
		Subscriptions: []models.Subscription{
			{
				Callback: "",
			},
		},
		Atomic: false,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchCreateSubscriptions(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Equal(t, 0, response.SuccessCount)
	assert.Equal(t, 1, response.FailureCount)
}

func TestBatchCreateSubscriptions_AtomicRollback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchSubscriptionCreate{
		Subscriptions: []models.Subscription{
			{
				Callback: "https://example.com/callback1",
			},
			{
				Callback: "", // Invalid - will fail
			},
		},
		Atomic: true,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchCreateSubscriptions(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Equal(t, 0, response.SuccessCount)
	assert.Equal(t, 2, response.FailureCount)

	// Verify rollback - all should be marked as RolledBack
	for _, result := range response.Results {
		assert.False(t, result.Success)
	}
}

func TestBatchCreateSubscriptions_BatchSizeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	tests := []struct {
		name       string
		count      int
		wantStatus int
	}{
		{
			name:       "empty batch",
			count:      0,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid batch size",
			count:      50,
			wantStatus: http.StatusOK,
		},
		{
			name:       "batch too large",
			count:      101,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs := make([]models.Subscription, tt.count)
			for i := range subs {
				subs[i] = models.Subscription{
					Callback: "https://example.com/callback",
				}
			}

			req := BatchSubscriptionCreate{
				Subscriptions: subs,
				Atomic:        false,
			}

			body, _ := json.Marshal(req)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.BatchCreateSubscriptions(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBatchDeleteSubscriptions_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "https://example.com/callback1"},
			{ID: "sub-2", Callback: "https://example.com/callback2"},
		},
	}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchSubscriptionDelete{
		SubscriptionIDs: []string{"sub-1", "sub-2"},
		Atomic:          false,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions/delete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchDeleteSubscriptions(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, 2, response.SuccessCount)
	assert.Equal(t, 0, response.FailureCount)
	assert.Empty(t, store.subscriptions)
}

func TestBatchDeleteSubscriptions_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchSubscriptionDelete{
		SubscriptionIDs: []string{"non-existent"},
		Atomic:          false,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions/delete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchDeleteSubscriptions(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Equal(t, 0, response.SuccessCount)
	assert.Equal(t, 1, response.FailureCount)
}

func TestBatchDeleteSubscriptions_AtomicFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "https://example.com/callback1"},
		},
	}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchSubscriptionDelete{
		SubscriptionIDs: []string{"sub-1", "non-existent"},
		Atomic:          true,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions/delete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchDeleteSubscriptions(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Equal(t, 0, response.SuccessCount)
	assert.Equal(t, 2, response.FailureCount)

	// Verify no subscriptions were deleted
	assert.Len(t, store.subscriptions, 1)
}

func TestBatchCreateResourcePools_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchResourcePoolCreate{
		ResourcePools: []models.ResourcePool{
			{
				ResourcePoolID: "pool-1",
				Name:           "Pool 1",
			},
			{
				ResourcePoolID: "pool-2",
				Name:           "Pool 2",
			},
		},
		Atomic: false,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/resourcePools", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchCreateResourcePools(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, 2, response.SuccessCount)
	assert.Equal(t, 0, response.FailureCount)
	assert.Len(t, response.Results, 2)
}

func TestBatchCreateResourcePools_AtomicRollback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{
		failOnCreatePool: 2, // Fail on 2nd create
	}

	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchResourcePoolCreate{
		ResourcePools: []models.ResourcePool{
			{
				ResourcePoolID: "pool-1",
				Name:           "Pool 1",
			},
			{
				ResourcePoolID: "pool-2",
				Name:           "Pool 2",
			},
		},
		Atomic: true,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/resourcePools", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchCreateResourcePools(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Equal(t, 0, response.SuccessCount)
	assert.Equal(t, 2, response.FailureCount)
}

func TestBatchDeleteResourcePools_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{
		resourcePools: []*adapter.ResourcePool{
			{ResourcePoolID: "pool-1", Name: "Pool 1"},
			{ResourcePoolID: "pool-2", Name: "Pool 2"},
		},
	}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchResourcePoolDelete{
		ResourcePoolIDs: []string{"pool-1", "pool-2"},
		Atomic:          false,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/resourcePools/delete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchDeleteResourcePools(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, 2, response.SuccessCount)
	assert.Equal(t, 0, response.FailureCount)
	assert.Empty(t, adapter.resourcePools)
}

func TestBatchDeleteResourcePools_AtomicFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{
		resourcePools: []*adapter.ResourcePool{
			{ResourcePoolID: "pool-1", Name: "Pool 1"},
		},
	}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	req := BatchResourcePoolDelete{
		ResourcePoolIDs: []string{"pool-1", "non-existent"},
		Atomic:          true,
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/resourcePools/delete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchDeleteResourcePools(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response BatchResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Equal(t, 0, response.SuccessCount)
	assert.Equal(t, 2, response.FailureCount)

	// Verify no pools were deleted
	assert.Len(t, adapter.resourcePools, 1)
}

func TestBatchHandler_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &mockBatchAdapter{}
	store := &mockBatchStore{}
	logger := zap.NewNop()
	handler := NewBatchHandler(adapter, store, logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/batch/subscriptions", bytes.NewReader([]byte("invalid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchCreateSubscriptions(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "BadRequest", response.Error)
}
