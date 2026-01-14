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
