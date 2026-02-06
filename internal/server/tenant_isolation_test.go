package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/storage"
)

// setupTestStore creates a test storage with miniredis.
func setupTestStore(t *testing.T) (*storage.RedisStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := &storage.RedisConfig{
		Addr:                   mr.Addr(),
		Password:               "",
		DB:                     0,
		UseSentinel:            false,
		MaxRetries:             1,
		DialTimeout:            1 * time.Second,
		ReadTimeout:            1 * time.Second,
		WriteTimeout:           1 * time.Second,
		PoolSize:               5,
		AllowInsecureCallbacks: true,
	}

	store := storage.NewRedisStore(cfg)
	return store, mr
}

// TestTenantIsolation_ListSubscriptions verifies that tenants can only see their own subscriptions.
func TestTenantIsolation_ListSubscriptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		tenantID         string
		isPlatformAdmin  bool
		setupFunc        func(*storage.RedisStore)
		expectedCount    int
		expectedTenantID string
	}{
		{
			name:            "tenant sees only own subscriptions",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupFunc: func(store *storage.RedisStore) {
				// Create subscriptions for different tenants
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-1",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-2",
					Callback: "https://tenant1.example.com/callback2",
					TenantID: "tenant-1",
				})
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-3",
					Callback: "https://tenant2.example.com/callback",
					TenantID: "tenant-2",
				})
			},
			expectedCount:    2,
			expectedTenantID: "tenant-1",
		},
		{
			name:            "platform admin sees all subscriptions",
			tenantID:        "admin-tenant",
			isPlatformAdmin: true,
			setupFunc: func(store *storage.RedisStore) {
				// Create subscriptions for different tenants
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-1",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-2",
					Callback: "https://tenant2.example.com/callback",
					TenantID: "tenant-2",
				})
			},
			expectedCount:    2,
			expectedTenantID: "",
		},
		{
			name:            "tenant with no subscriptions sees empty list",
			tenantID:        "tenant-empty",
			isPlatformAdmin: false,
			setupFunc: func(store *storage.RedisStore) {
				// Create subscriptions for other tenants
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-1",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
			},
			expectedCount:    0,
			expectedTenantID: "tenant-empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			store, mr := setupTestStore(t)
			defer mr.Close()

			if tt.setupFunc != nil {
				tt.setupFunc(store)
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				store:  store,
				logger: logger,
			}

			// Setup route
			router.GET("/subscriptions", func(c *gin.Context) {
				// Inject authenticated user context
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)

				srv.handleListSubscriptions(c)
			})

			// Execute request
			req := httptest.NewRequest(http.MethodGet, "/subscriptions", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			subscriptions, ok := response["subscriptions"].([]interface{})
			require.True(t, ok, "subscriptions should be an array")
			assert.Equal(t, tt.expectedCount, len(subscriptions))

			// Verify tenant isolation for non-admin users
			if !tt.isPlatformAdmin && tt.expectedTenantID != "" {
				for _, sub := range subscriptions {
					_, ok := sub.(map[string]interface{})
					require.True(t, ok)
					// Note: The response doesn't include tenantID, but we verified
					// it through the expectedCount check
				}
			}
		})
	}
}

// TestTenantIsolation_GetSubscription verifies cross-tenant access prevention.
func TestTenantIsolation_GetSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		subscriptionID   string
		tenantID         string
		isPlatformAdmin  bool
		setupFunc        func(*storage.RedisStore)
		expectedStatus   int
		expectedErrorMsg string
	}{
		{
			name:            "tenant can access own subscription",
			subscriptionID:  "sub-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupFunc: func(store *storage.RedisStore) {
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-1",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:            "tenant cannot access other tenant subscription",
			subscriptionID:  "sub-2",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupFunc: func(store *storage.RedisStore) {
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-2",
					Callback: "https://tenant2.example.com/callback",
					TenantID: "tenant-2",
				})
			},
			expectedStatus:   http.StatusNotFound,
			expectedErrorMsg: "Subscription not found",
		},
		{
			name:            "platform admin can access any subscription",
			subscriptionID:  "sub-3",
			tenantID:        "admin-tenant",
			isPlatformAdmin: true,
			setupFunc: func(store *storage.RedisStore) {
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-3",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			store, mr := setupTestStore(t)
			defer mr.Close()

			if tt.setupFunc != nil {
				tt.setupFunc(store)
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				store:  store,
				logger: logger,
			}

			// Setup route
			router.GET("/subscriptions/:subscriptionId", func(c *gin.Context) {
				// Inject authenticated user context
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)

				srv.handleGetSubscription(c)
			})

			// Execute request
			req := httptest.NewRequest(http.MethodGet, "/subscriptions/"+tt.subscriptionID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedErrorMsg != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], tt.expectedErrorMsg)
			}
		})
	}
}

// TestTenantIsolation_DeleteSubscription verifies tenants cannot delete other tenants' subscriptions.
func TestTenantIsolation_DeleteSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		subscriptionID   string
		tenantID         string
		isPlatformAdmin  bool
		setupFunc        func(*storage.RedisStore)
		expectedStatus   int
		expectedErrorMsg string
	}{
		{
			name:            "tenant cannot delete other tenant subscription",
			subscriptionID:  "sub-1",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			setupFunc: func(store *storage.RedisStore) {
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-1",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
			},
			expectedStatus:   http.StatusNotFound,
			expectedErrorMsg: "Subscription not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			store, mr := setupTestStore(t)
			defer mr.Close()

			if tt.setupFunc != nil {
				tt.setupFunc(store)
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				store:  store,
				logger: logger,
			}

			// Setup route
			router.DELETE("/subscriptions/:subscriptionId", func(c *gin.Context) {
				// Inject authenticated user context
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)

				srv.handleDeleteSubscription(c)
			})

			// Execute request
			req := httptest.NewRequest(http.MethodDelete, "/subscriptions/"+tt.subscriptionID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedErrorMsg != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], tt.expectedErrorMsg)
			}
		})
	}
}

// TestTenantIsolation_UpdateSubscription verifies tenants cannot update other tenants' subscriptions.
func TestTenantIsolation_UpdateSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		subscriptionID   string
		tenantID         string
		isPlatformAdmin  bool
		setupFunc        func(*storage.RedisStore)
		requestBody      string
		expectedStatus   int
		expectedErrorMsg string
	}{
		{
			name:            "tenant cannot update other tenant subscription",
			subscriptionID:  "sub-1",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			setupFunc: func(store *storage.RedisStore) {
				_ = store.Create(context.Background(), &storage.Subscription{
					ID:       "sub-1",
					Callback: "https://tenant1.example.com/callback",
					TenantID: "tenant-1",
				})
			},
			requestBody:      `{"callback": "https://malicious.example.com/callback"}`,
			expectedStatus:   http.StatusNotFound,
			expectedErrorMsg: "Subscription not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			store, mr := setupTestStore(t)
			defer mr.Close()

			if tt.setupFunc != nil {
				tt.setupFunc(store)
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				store:  store,
				logger: logger,
			}

			// Setup route
			router.PUT("/subscriptions/:subscriptionId", func(c *gin.Context) {
				// Inject authenticated user context
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)

				srv.handleUpdateSubscription(c)
			})

			// Execute request
			req := httptest.NewRequest(
				http.MethodPut,
				"/subscriptions/"+tt.subscriptionID,
				strings.NewReader(tt.requestBody),
			)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedErrorMsg != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], tt.expectedErrorMsg)
			}
		})
	}
}

// TestTenantIsolation_NoAuthContext verifies behavior when no auth context exists.
func TestTenantIsolation_NoAuthContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	store, mr := setupTestStore(t)
	defer mr.Close()

	_ = store.Create(context.Background(), &storage.Subscription{
		ID:       "sub-1",
		Callback: "https://example.com/callback",
		TenantID: "",
	})

	logger := zap.NewNop()
	router := gin.New()

	srv := &Server{
		store:  store,
		logger: logger,
	}

	// Setup route - no auth context injected
	router.GET("/subscriptions", srv.handleListSubscriptions)

	// Execute request
	req := httptest.NewRequest(http.MethodGet, "/subscriptions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify response - should still work (returns all when no auth)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	subscriptions, ok := response["subscriptions"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, len(subscriptions))
}

// mockTenantAdapter is a minimal mock adapter for tenant isolation tests.
// It implements the adapter.Adapter interface with in-memory maps for
// resource pools and resources, allowing controlled test scenarios.
type mockTenantAdapter struct {
	resourcePools map[string]*adapter.ResourcePool
	resources     map[string]*adapter.Resource
}

// Metadata interface methods.

func (m *mockTenantAdapter) Name() string                       { return "mock-tenant" }
func (m *mockTenantAdapter) Version() string                    { return "1.0.0" }
func (m *mockTenantAdapter) Capabilities() []adapter.Capability { return nil }

// DeploymentManagerClient interface methods.

func (m *mockTenantAdapter) ListDeploymentManagers(_ context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, adapter.ErrNotImplemented
}

// ResourcePoolClient interface methods.

func (m *mockTenantAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	result := make([]*adapter.ResourcePool, 0, len(m.resourcePools))
	for _, pool := range m.resourcePools {
		result = append(result, pool)
	}
	return result, nil
}

func (m *mockTenantAdapter) GetResourcePool(_ context.Context, id string) (*adapter.ResourcePool, error) {
	pool, ok := m.resourcePools[id]
	if !ok {
		return nil, adapter.ErrResourcePoolNotFound
	}
	return pool, nil
}

func (m *mockTenantAdapter) CreateResourcePool(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) UpdateResourcePool(_ context.Context, _ string, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) DeleteResourcePool(_ context.Context, id string) error {
	if _, ok := m.resourcePools[id]; !ok {
		return adapter.ErrResourcePoolNotFound
	}
	delete(m.resourcePools, id)
	return nil
}

// ResourceClient interface methods.

func (m *mockTenantAdapter) ListResources(_ context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	var result []*adapter.Resource
	for _, r := range m.resources {
		if filter != nil && filter.TenantID != "" && r.TenantID != filter.TenantID {
			continue
		}
		if filter != nil && filter.ResourcePoolID != "" && r.ResourcePoolID != filter.ResourcePoolID {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}

func (m *mockTenantAdapter) GetResource(_ context.Context, id string) (*adapter.Resource, error) {
	r, ok := m.resources[id]
	if !ok {
		return nil, adapter.ErrResourceNotFound
	}
	return r, nil
}

func (m *mockTenantAdapter) CreateResource(_ context.Context, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) UpdateResource(_ context.Context, _ string, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) DeleteResource(_ context.Context, id string) error {
	if _, ok := m.resources[id]; !ok {
		return adapter.ErrResourceNotFound
	}
	delete(m.resources, id)
	return nil
}

// ResourceTypeClient interface methods.

func (m *mockTenantAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) GetResourceType(_ context.Context, _ string) (*adapter.ResourceType, error) {
	return nil, adapter.ErrNotImplemented
}

// SubscriptionClient interface methods.

func (m *mockTenantAdapter) CreateSubscription(_ context.Context, _ *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) UpdateSubscription(_ context.Context, _ string, _ *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, adapter.ErrNotImplemented
}

func (m *mockTenantAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return adapter.ErrNotImplemented
}

// Lifecycle interface methods.

func (m *mockTenantAdapter) Health(_ context.Context) error { return nil }
func (m *mockTenantAdapter) Close() error                   { return nil }

// TestTenantIsolation_GetResourcePool verifies cross-tenant access prevention for resource pools.
func TestTenantIsolation_GetResourcePool(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		tenantID        string
		isPlatformAdmin bool
		poolID          string
		setupPools      map[string]*adapter.ResourcePool
		expectedStatus  int
	}{
		{
			name:            "tenant can access own pool",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			poolID:          "pool-1",
			setupPools: map[string]*adapter.ResourcePool{
				"pool-1": {
					ResourcePoolID: "pool-1",
					TenantID:       "tenant-1",
					Name:           "Tenant 1 Pool",
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:            "tenant cannot access other tenant pool",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			poolID:          "pool-1",
			setupPools: map[string]*adapter.ResourcePool{
				"pool-1": {
					ResourcePoolID: "pool-1",
					TenantID:       "tenant-1",
					Name:           "Tenant 1 Pool",
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:            "platform admin can access any pool",
			tenantID:        "admin-tenant",
			isPlatformAdmin: true,
			poolID:          "pool-1",
			setupPools: map[string]*adapter.ResourcePool{
				"pool-1": {
					ResourcePoolID: "pool-1",
					TenantID:       "tenant-1",
					Name:           "Tenant 1 Pool",
				},
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdp := &mockTenantAdapter{
				resourcePools: tt.setupPools,
				resources:     make(map[string]*adapter.Resource),
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				adapter: mockAdp,
				logger:  logger,
			}

			router.GET("/resourcePools/:resourcePoolId", func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				srv.handleGetResourcePool(c)
			})

			req := httptest.NewRequest(http.MethodGet, "/resourcePools/"+tt.poolID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusNotFound {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], "Resource pool not found")
			}
		})
	}
}

// TestTenantIsolation_GetResource verifies cross-tenant access prevention for resources.
func TestTenantIsolation_GetResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		tenantID        string
		isPlatformAdmin bool
		resourceID      string
		setupResources  map[string]*adapter.Resource
		expectedStatus  int
	}{
		{
			name:            "tenant can access own resource",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			resourceID:      "res-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:            "tenant cannot access other tenant resource",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			resourceID:      "res-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:            "platform admin can access any resource",
			tenantID:        "admin-tenant",
			isPlatformAdmin: true,
			resourceID:      "res-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdp := &mockTenantAdapter{
				resourcePools: make(map[string]*adapter.ResourcePool),
				resources:     tt.setupResources,
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				adapter: mockAdp,
				logger:  logger,
			}

			router.GET("/resources/:resourceId", func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				srv.handleGetResource(c)
			})

			req := httptest.NewRequest(http.MethodGet, "/resources/"+tt.resourceID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusNotFound {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], "Resource not found")
			}
		})
	}
}

// TestTenantIsolation_ListResourcesInPool verifies tenant filtering when listing resources in a pool.
func TestTenantIsolation_ListResourcesInPool(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		tenantID        string
		isPlatformAdmin bool
		poolID          string
		setupResources  map[string]*adapter.Resource
		expectedCount   int
	}{
		{
			name:            "tenant sees only own resources in pool",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			poolID:          "pool-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
				"res-2": {
					ResourceID:     "res-2",
					TenantID:       "tenant-2",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
				"res-3": {
					ResourceID:     "res-3",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedCount: 2,
		},
		{
			name:            "platform admin sees all resources in pool",
			tenantID:        "admin-tenant",
			isPlatformAdmin: true,
			poolID:          "pool-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
				"res-2": {
					ResourceID:     "res-2",
					TenantID:       "tenant-2",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedCount: 2,
		},
		{
			name:            "tenant with no resources in pool sees empty list",
			tenantID:        "tenant-3",
			isPlatformAdmin: false,
			poolID:          "pool-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdp := &mockTenantAdapter{
				resourcePools: make(map[string]*adapter.ResourcePool),
				resources:     tt.setupResources,
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				adapter: mockAdp,
				logger:  logger,
			}

			router.GET("/resourcePools/:resourcePoolId/resources", func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				srv.handleListResourcesInPool(c)
			})

			req := httptest.NewRequest(http.MethodGet, "/resourcePools/"+tt.poolID+"/resources", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			resources, ok := response["resources"].([]interface{})
			if tt.expectedCount == 0 {
				// When no resources match, the response may contain nil or empty array
				if ok {
					assert.Equal(t, tt.expectedCount, len(resources))
				}
			} else {
				require.True(t, ok, "resources should be an array")
				assert.Equal(t, tt.expectedCount, len(resources))
			}
		})
	}
}

// TestTenantIsolation_DeleteResourcePool verifies tenants cannot delete other tenants' pools.
func TestTenantIsolation_DeleteResourcePool(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		tenantID        string
		isPlatformAdmin bool
		poolID          string
		setupPools      map[string]*adapter.ResourcePool
		expectedStatus  int
	}{
		{
			name:            "tenant cannot delete other tenant pool",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			poolID:          "pool-1",
			setupPools: map[string]*adapter.ResourcePool{
				"pool-1": {
					ResourcePoolID: "pool-1",
					TenantID:       "tenant-1",
					Name:           "Tenant 1 Pool",
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:            "tenant can delete own pool",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			poolID:          "pool-1",
			setupPools: map[string]*adapter.ResourcePool{
				"pool-1": {
					ResourcePoolID: "pool-1",
					TenantID:       "tenant-1",
					Name:           "Tenant 1 Pool",
				},
			},
			expectedStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdp := &mockTenantAdapter{
				resourcePools: tt.setupPools,
				resources:     make(map[string]*adapter.Resource),
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				adapter: mockAdp,
				logger:  logger,
			}

			router.DELETE("/resourcePools/:resourcePoolId", func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				srv.handleDeleteResourcePool(c)
			})

			req := httptest.NewRequest(http.MethodDelete, "/resourcePools/"+tt.poolID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			// Verify the pool still exists if deletion was blocked
			if tt.expectedStatus == http.StatusNotFound {
				_, poolExists := mockAdp.resourcePools[tt.poolID]
				assert.True(t, poolExists, "pool should still exist after blocked deletion")

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], "Resource pool not found")
			}
		})
	}
}

// TestTenantIsolation_DeleteResource verifies tenants cannot delete other tenants' resources.
func TestTenantIsolation_DeleteResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		tenantID        string
		isPlatformAdmin bool
		resourceID      string
		setupResources  map[string]*adapter.Resource
		expectedStatus  int
	}{
		{
			name:            "tenant cannot delete other tenant resource",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			resourceID:      "res-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:            "tenant can delete own resource",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			resourceID:      "res-1",
			setupResources: map[string]*adapter.Resource{
				"res-1": {
					ResourceID:     "res-1",
					TenantID:       "tenant-1",
					ResourceTypeID: "type-1",
					ResourcePoolID: "pool-1",
				},
			},
			expectedStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAdp := &mockTenantAdapter{
				resourcePools: make(map[string]*adapter.ResourcePool),
				resources:     tt.setupResources,
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				adapter: mockAdp,
				logger:  logger,
			}

			router.DELETE("/resources/:resourceId", func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.tenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				srv.handleDeleteResource(c)
			})

			req := httptest.NewRequest(http.MethodDelete, "/resources/"+tt.resourceID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			// Verify the resource still exists if deletion was blocked
			if tt.expectedStatus == http.StatusNotFound {
				_, resExists := mockAdp.resources[tt.resourceID]
				assert.True(t, resExists, "resource should still exist after blocked deletion")

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["message"], "Resource not found")
			}
		})
	}
}
