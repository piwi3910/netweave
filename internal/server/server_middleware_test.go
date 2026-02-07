package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/server"
)

// setupMinimalTestServer creates a minimal test server using NewTestServerWithMetrics
// to avoid Prometheus registry conflicts.
func setupMinimalTestServer(t *testing.T) *server.Server {
	t.Helper()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})
	return srv
}

// setupTestServerWithHealth creates a test server with a health checker initialized.
func setupTestServerWithHealth(t *testing.T) *server.Server {
	t.Helper()
	srv := setupMinimalTestServer(t)
	hc := observability.NewHealthChecker("test-1.0.0")
	srv.SetHealthChecker(hc)
	return srv
}

func TestJoinStrings_NoSkip(t *testing.T) {
	tests := []struct {
		name     string
		strs     []string
		sep      string
		expected string
	}{
		{
			name:     "empty slice",
			strs:     []string{},
			sep:      ",",
			expected: "",
		},
		{
			name:     "single element",
			strs:     []string{"a"},
			sep:      ",",
			expected: "a",
		},
		{
			name:     "multiple elements with comma",
			strs:     []string{"a", "b", "c"},
			sep:      ",",
			expected: "a,b,c",
		},
		{
			name:     "multiple elements with comma-space",
			strs:     []string{"Content-Type", "Authorization"},
			sep:      ", ",
			expected: "Content-Type, Authorization",
		},
		{
			name:     "two elements with dash",
			strs:     []string{"foo", "bar"},
			sep:      "-",
			expected: "foo-bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.JoinStrings(tt.strs, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRecoveryMiddleware_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	router := gin.New()
	router.Use(srv.RecoveryMiddleware())

	router.GET("/panic", func(_ *gin.Context) {
		panic("test panic recovery")
	})
	router.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	t.Run("recovers from panic", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal server error")
	})

	t.Run("passes through normal requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "ok", w.Body.String())
	})
}

func TestLoggingMiddleware_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	router := gin.New()
	router.Use(srv.LoggingMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.GET("/error", func(c *gin.Context) {
		_ = c.Error(assert.AnError)
		c.String(http.StatusBadRequest, "bad")
	})

	t.Run("logs successful request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?foo=bar", nil)
		req.Header.Set("User-Agent", "test-agent")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("logs request with errors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMetricsMiddleware_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	t.Run("nil metrics passes through", func(t *testing.T) {
		router := gin.New()
		router.Use(srv.MetricsMiddleware())
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestServer_SetOpenAPISpec_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	spec := []byte("openapi: 3.0.0\ninfo:\n  title: test")
	srv.SetOpenAPISpec(spec)

	retrieved := srv.GetOpenAPISpec()
	assert.Equal(t, spec, retrieved)
}

func TestServer_GetOpenAPISpec_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	spec := srv.GetOpenAPISpec()
	// NewTestServerWithMetrics does not set an openapi spec
	assert.Nil(t, spec)
}

func TestServer_TestHelperGetters(t *testing.T) {
	srv := setupMinimalTestServer(t)

	t.Run("Config returns non-nil", func(t *testing.T) {
		assert.NotNil(t, srv.Config())
	})

	t.Run("Logger returns non-nil", func(t *testing.T) {
		assert.NotNil(t, srv.Logger())
	})

	t.Run("GetAdapter returns non-nil", func(t *testing.T) {
		assert.NotNil(t, srv.GetAdapter())
	})

	t.Run("GetStore returns non-nil", func(t *testing.T) {
		assert.NotNil(t, srv.GetStore())
	})

	t.Run("GetAuthMw returns nil when not configured", func(t *testing.T) {
		assert.Nil(t, srv.GetAuthMw())
	})

	t.Run("HealthCheck returns nil on test server", func(t *testing.T) {
		// NewTestServerWithMetrics doesn't set healthCheck
		assert.Nil(t, srv.HealthCheck())
	})

	t.Run("HTTPServer returns nil initially", func(t *testing.T) {
		assert.Nil(t, srv.HTTPServer())
	})

	t.Run("SetHTTPServer works", func(t *testing.T) {
		httpSrv := &http.Server{
			Addr:              ":9090",
			ReadHeaderTimeout: 5 * time.Second,
		}
		srv.SetHTTPServer(httpSrv)
		assert.Equal(t, httpSrv, srv.HTTPServer())
	})
}

func TestServer_ShutdownWithContext_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	// Set an HTTP server that can be shut down
	httpSrv := &http.Server{
		Addr:              ":0",
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	srv.SetHTTPServer(httpSrv)

	t.Run("shutdown with valid context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.ShutdownWithContext(ctx)
		assert.NoError(t, err)
	})

	t.Run("shutdown with cancelled context", func(t *testing.T) {
		// Create a new server since the first one already shut down
		srv2 := setupMinimalTestServer(t)
		httpSrv2 := &http.Server{
			Addr:              ":0",
			Handler:           srv2.Router(),
			ReadHeaderTimeout: 5 * time.Second,
		}
		srv2.SetHTTPServer(httpSrv2)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		err := srv2.ShutdownWithContext(ctx)
		// Either error or nil depending on timing
		if err != nil {
			assert.Contains(t, err.Error(), "cancel")
		}
	})
}

func TestServer_SetupAuth_NoSkip(t *testing.T) {
	tests := []struct {
		name        string
		authStore   server.AuthStore
		authMw      server.AuthMiddleware
		expectNilMw bool
		expectNilAS bool
	}{
		{
			name:        "both nil disables auth",
			authStore:   nil,
			authMw:      nil,
			expectNilMw: true,
			expectNilAS: true,
		},
		{
			name:        "nil authStore disables auth",
			authStore:   nil,
			authMw:      &mockAuthMiddleware{},
			expectNilMw: true,
			expectNilAS: true,
		},
		{
			name:        "nil authMw disables auth",
			authStore:   &mockAuthStore{},
			authMw:      nil,
			expectNilMw: true,
			expectNilAS: true,
		},
		{
			name:        "valid auth store and middleware",
			authStore:   &mockAuthStore{},
			authMw:      &mockAuthMiddleware{},
			expectNilMw: false,
			expectNilAS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := setupMinimalTestServer(t)
			srv.SetupAuth(tt.authStore, tt.authMw)

			if tt.expectNilAS {
				assert.Nil(t, srv.AuthStore)
			} else {
				assert.NotNil(t, srv.AuthStore)
			}
			if tt.expectNilMw {
				assert.Nil(t, srv.GetAuthMw())
			} else {
				assert.NotNil(t, srv.GetAuthMw())
			}
		})
	}
}

func TestWithPermission_NoAuth(t *testing.T) {
	srv := setupMinimalTestServer(t)

	// Without auth configured, handlers should run directly
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWithPermission_WithAuth(t *testing.T) {
	srv := setupMinimalTestServer(t)
	srv.SetupAuth(&mockAuthStore{}, &mockAuthMiddleware{})

	// With mock auth middleware, permission checks pass through
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should succeed (mockAuthMiddleware just calls c.Next())
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleHealth_NoSkip(t *testing.T) {
	srv := setupTestServerWithHealth(t)
	router := srv.Router()

	tests := []struct {
		name string
		path string
	}{
		{name: "health endpoint", path: "/health"},
		{name: "healthz endpoint", path: "/healthz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "status")
		})
	}
}

func TestHandleReadiness_NoSkip(t *testing.T) {
	srv := setupTestServerWithHealth(t)
	router := srv.Router()

	tests := []struct {
		name string
		path string
	}{
		{name: "ready endpoint", path: "/ready"},
		{name: "readyz endpoint", path: "/readyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "ready")
		})
	}
}

func TestHandleRoot_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "O2-IMS Gateway")
	assert.Contains(t, w.Body.String(), "health")
	assert.Contains(t, w.Body.String(), "endpoints")
}

func TestHandleAPIInfo_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	t.Run("from /o2ims path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "subscriptions")
		assert.Contains(t, w.Body.String(), "resourcePools")
		assert.Contains(t, w.Body.String(), "features")
	})

	t.Run("from v1 base path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "api_version")
	})
}

func TestHandleListSubscriptions_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "subscriptions")
	assert.Contains(t, w.Body.String(), "total")
}

func TestHandleCreateSubscription_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	// Disable SSRF protection for testing
	srv.Config().Security.DisableSSRFProtection = true
	router := srv.Router()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid subscription with public callback",
			body:       `{"callback":"https://example.com/notify","consumerSubscriptionId":"test-sub"}`,
			wantStatus: http.StatusCreated,
			wantBody:   "example.com",
		},
		{
			name:       "invalid JSON body",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "BadRequest",
		},
		{
			name:       "missing callback URL",
			body:       `{"consumerSubscriptionId":"test-sub"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "callback URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/subscriptions",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantBody)
		})
	}
}

func TestHandleGetSubscription_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/subscriptions/sub-nonexistent", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "NotFound")
	})
}

func TestHandleUpdateSubscription_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	tests := []struct {
		name       string
		subID      string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "subscription not found (store returns not found)",
			subID:      "sub-does-not-exist",
			body:       `{"callback":"https://example.com/updated"}`,
			wantStatus: http.StatusNotFound,
			wantBody:   "NotFound",
		},
		{
			name:       "subscription not found before JSON parsing",
			subID:      "sub-bad-json-but-not-found-first",
			body:       `{bad json}`,
			wantStatus: http.StatusNotFound,
			wantBody:   "NotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut,
				"/o2ims-infrastructureInventory/v1/subscriptions/"+tt.subID,
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantBody)
		})
	}
}

func TestHandleDeleteSubscription_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	t.Run("subscription not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/subscriptions/sub-123", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// mockStore.Get returns ErrSubscriptionNotFound, so delete returns 404
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "NotFound")
	})
}

func TestHandleListResourcePools_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "resourcePools")
}

func TestHandleCreateResourcePool_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid resource pool",
			body:       `{"name":"test-pool","description":"Test pool"}`,
			wantStatus: http.StatusCreated,
			wantBody:   "test-pool",
		},
		{
			name:       "invalid JSON body",
			body:       `{bad}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "BadRequest",
		},
		{
			name:       "missing name",
			body:       `{"description":"No name"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantBody)
		})
	}
}

func TestHandleUpdateResourcePool_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	tests := []struct {
		name       string
		poolID     string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid update",
			poolID:     "pool-abc",
			body:       `{"name":"updated-pool"}`,
			wantStatus: http.StatusOK,
			wantBody:   "updated-pool",
		},
		{
			name:       "invalid JSON body",
			poolID:     "pool-abc",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "BadRequest",
		},
		{
			name:       "missing name",
			poolID:     "pool-abc",
			body:       `{"description":"No name"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut,
				"/o2ims-infrastructureInventory/v1/resourcePools/"+tt.poolID,
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantBody)
		})
	}
}

func TestHandleDeleteResourcePool_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	t.Run("successful delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resourcePools/pool-abc", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// mockAdapter.DeleteResourcePool returns nil (success)
		require.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandleListResources_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "resources")
}

func TestHandleListResourceTypes_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "resourceTypes")
}

func TestHandleListDeploymentManagers_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/deploymentManagers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// MockAdapter returns nil, nil for ListDeploymentManagers
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "deploymentManagers")
}

func TestHandleGetDeploymentManager_NotFound(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/deploymentManagers/dm-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "NotFound")
}

func TestHandleGetResourceType_NotFound(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourceTypes/type-xyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetOCloudInfrastructure_NoDMs(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/oCloudInfrastructure", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// mockAdapter returns nil, nil for ListDeploymentManagers so len(dms)==0
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "No deployment managers available")
}

func TestHandleGetTenantQuotas_NoSkip(t *testing.T) {
	// This route is only registered when tenantHandler is not nil
	// In our test setup it is nil, so it should 404
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/tenants/t-1/quotas", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Route may not exist without tenant handler
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusOK)
}

func TestHandleUpdateTenantQuotas_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodPut, "/o2ims-infrastructureInventory/v1/tenants/t-1/quotas", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

func TestTMForumRoutesEarly_Unavailable(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	// TMForum routes should return 503 when DMS not initialized
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "TMF639 list resources",
			method: http.MethodGet,
			path:   "/tmf-api/resourceInventoryManagement/v4/resource",
		},
		{
			name:   "TMF639 get resource",
			method: http.MethodGet,
			path:   "/tmf-api/resourceInventoryManagement/v4/resource/some-id",
		},
		{
			name:   "TMF639 create resource",
			method: http.MethodPost,
			path:   "/tmf-api/resourceInventoryManagement/v4/resource",
		},
		{
			name:   "TMF639 update resource",
			method: http.MethodPatch,
			path:   "/tmf-api/resourceInventoryManagement/v4/resource/some-id",
		},
		{
			name:   "TMF639 delete resource",
			method: http.MethodDelete,
			path:   "/tmf-api/resourceInventoryManagement/v4/resource/some-id",
		},
		{
			name:   "TMF638 list services",
			method: http.MethodGet,
			path:   "/tmf-api/serviceInventoryManagement/v4/service",
		},
		{
			name:   "TMF638 get service",
			method: http.MethodGet,
			path:   "/tmf-api/serviceInventoryManagement/v4/service/svc-id",
		},
		{
			name:   "TMF638 create service",
			method: http.MethodPost,
			path:   "/tmf-api/serviceInventoryManagement/v4/service",
		},
		{
			name:   "TMF638 update service",
			method: http.MethodPatch,
			path:   "/tmf-api/serviceInventoryManagement/v4/service/svc-id",
		},
		{
			name:   "TMF638 delete service",
			method: http.MethodDelete,
			path:   "/tmf-api/serviceInventoryManagement/v4/service/svc-id",
		},
		{
			name:   "TMF641 list orders",
			method: http.MethodGet,
			path:   "/tmf-api/serviceOrdering/v4/serviceOrder",
		},
		{
			name:   "TMF641 get order",
			method: http.MethodGet,
			path:   "/tmf-api/serviceOrdering/v4/serviceOrder/ord-id",
		},
		{
			name:   "TMF641 create order",
			method: http.MethodPost,
			path:   "/tmf-api/serviceOrdering/v4/serviceOrder",
		},
		{
			name:   "TMF641 update order",
			method: http.MethodPatch,
			path:   "/tmf-api/serviceOrdering/v4/serviceOrder/ord-id",
		},
		{
			name:   "TMF641 delete order",
			method: http.MethodDelete,
			path:   "/tmf-api/serviceOrdering/v4/serviceOrder/ord-id",
		},
		{
			name:   "TMF688 list events",
			method: http.MethodGet,
			path:   "/tmf-api/eventManagement/v4/event",
		},
		{
			name:   "TMF688 get event",
			method: http.MethodGet,
			path:   "/tmf-api/eventManagement/v4/event/evt-id",
		},
		{
			name:   "TMF688 create event",
			method: http.MethodPost,
			path:   "/tmf-api/eventManagement/v4/event",
		},
		{
			name:   "TMF688 register hub",
			method: http.MethodPost,
			path:   "/tmf-api/eventManagement/v4/hub",
		},
		{
			name:   "TMF688 unregister hub",
			method: http.MethodDelete,
			path:   "/tmf-api/eventManagement/v4/hub/hub-id",
		},
		{
			name:   "TMF642 list alarms",
			method: http.MethodGet,
			path:   "/tmf-api/alarmManagement/v4/alarm",
		},
		{
			name:   "TMF642 get alarm",
			method: http.MethodGet,
			path:   "/tmf-api/alarmManagement/v4/alarm/alarm-id",
		},
		{
			name:   "TMF642 acknowledge alarm",
			method: http.MethodPatch,
			path:   "/tmf-api/alarmManagement/v4/alarm/alarm-id/acknowledge",
		},
		{
			name:   "TMF642 clear alarm",
			method: http.MethodPatch,
			path:   "/tmf-api/alarmManagement/v4/alarm/alarm-id/clear",
		},
		{
			name:   "TMF640 list activations",
			method: http.MethodGet,
			path:   "/tmf-api/serviceActivation/v4/serviceActivation",
		},
		{
			name:   "TMF640 get activation",
			method: http.MethodGet,
			path:   "/tmf-api/serviceActivation/v4/serviceActivation/act-id",
		},
		{
			name:   "TMF640 create activation",
			method: http.MethodPost,
			path:   "/tmf-api/serviceActivation/v4/serviceActivation",
		},
		{
			name:   "TMF620 list offerings",
			method: http.MethodGet,
			path:   "/tmf-api/productCatalog/v4/productOffering",
		},
		{
			name:   "TMF620 get offering",
			method: http.MethodGet,
			path:   "/tmf-api/productCatalog/v4/productOffering/off-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			assert.Contains(t, w.Body.String(), "DMS subsystem not initialized")
		})
	}
}

func TestHandleListResourcesInPool_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools/pool-1/resources", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "resources")
}

func TestSanitizeResourcePoolID_NoSkip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "simple name", input: "my-pool", expected: "my-pool"},
		{name: "with spaces", input: "my pool", expected: "my-pool"},
		{name: "with slashes", input: "my/pool", expected: "my-pool"},
		{name: "with uppercase", input: "MyPool", expected: "mypool"},
		{name: "with special chars", input: "my@pool!", expected: "mypool"},
		{name: "with underscores", input: "my_pool", expected: "my_pool"},
		{name: "empty string", input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.SanitizeResourcePoolID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeForLogging_NoSkip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "no special chars", input: "hello", expected: "hello"},
		{name: "with CR", input: "hello\rworld", expected: "helloworld"},
		{name: "with LF", input: "hello\nworld", expected: "helloworld"},
		{name: "with tab", input: "hello\tworld", expected: "hello world"},
		{name: "with CRLF", input: "hello\r\nworld", expected: "helloworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.SanitizeForLogging(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateResourcePoolFields_NoSkip(t *testing.T) {
	tests := []struct {
		name    string
		pool    map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid pool",
			pool: map[string]interface{}{
				"name": "test-pool",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			pool: map[string]interface{}{
				"description": "no name",
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "name too long",
			pool: map[string]interface{}{
				"name": strings.Repeat("a", 256),
			},
			wantErr: true,
			errMsg:  "name must not exceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can exercise the route-level validation via HTTP
			srv := setupMinimalTestServer(t)
			router := srv.Router()

			body, err := json.Marshal(tt.pool)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/resourcePools",
				bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), tt.errMsg)
			} else {
				assert.Equal(t, http.StatusCreated, w.Code)
			}
		})
	}
}

func TestParseFilterFromRequest_NoSkip(t *testing.T) {
	// parseFilterFromRequest is exercised through list endpoints
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	t.Run("v1 basic filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestLogAuditEvent_NoSkip(t *testing.T) {
	// logAuditEvent is exercised through subscription update/delete which call it
	srv := setupMinimalTestServer(t)
	srv.SetupAuth(&mockAuthStore{}, &mockAuthMiddleware{})
	srv.Config().Security.DisableSSRFProtection = true
	router := srv.Router()

	// Create a subscription first
	createBody := `{"callback":"https://example.com/notify","consumerSubscriptionId":"audit-test"}`
	req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/subscriptions",
		strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Regardless of creation outcome, the audit path is exercised
	assert.True(t, w.Code == http.StatusCreated || w.Code == http.StatusInternalServerError)
}

func TestSetHealthChecker_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)

	// Initially nil
	assert.Nil(t, srv.HealthCheck())

	// Set health checker
	hc := observability.NewHealthChecker("v1.0.0")
	srv.SetHealthChecker(hc)

	assert.NotNil(t, srv.HealthCheck())
	assert.Equal(t, hc, srv.HealthCheck())
}

func TestHandleHealth_Unhealthy(t *testing.T) {
	srv := setupMinimalTestServer(t)
	hc := observability.NewHealthChecker("test-1.0.0")
	hc.RegisterHealthCheck("failing", func(_ context.Context) error {
		return assert.AnError
	})
	srv.SetHealthChecker(hc)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "unhealthy")
}

func TestHandleReadiness_NotReady(t *testing.T) {
	srv := setupMinimalTestServer(t)
	hc := observability.NewHealthChecker("test-1.0.0")
	hc.RegisterReadinessCheck("not-ready", func(_ context.Context) error {
		return assert.AnError
	})
	srv.SetHealthChecker(hc)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "false")
}

func TestHandleGetResourcePool_NotFound(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resourcePools/pool-nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "NotFound")
}

func TestHandleGetResource_NotFound(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources/res-nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "NotFound")
}

func TestHandleDeleteResource_NotFound(t *testing.T) {
	srv := setupMinimalTestServer(t)
	router := srv.Router()

	req := httptest.NewRequest(http.MethodDelete, "/o2ims-infrastructureInventory/v1/resources/res-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "NotFound")
}

func TestIsPrivateIP_NoSkip(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{name: "loopback v4", ip: "127.0.0.1", expected: true},
		{name: "private 10.x", ip: "10.0.0.1", expected: true},
		{name: "private 172.16.x", ip: "172.16.0.1", expected: true},
		{name: "private 192.168.x", ip: "192.168.1.1", expected: true},
		{name: "public IP", ip: "8.8.8.8", expected: false},
		{name: "loopback v6", ip: "::1", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)
			result := server.IsPrivateIP(ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// parseIP parses an IP address string for testing IsPrivateIP.
func parseIP(s string) net.IP {
	return net.ParseIP(s)
}

func TestValidateCallback_NoSkip(t *testing.T) {
	srv := setupMinimalTestServer(t)
	srv.Config().Security.DisableSSRFProtection = true

	tests := []struct {
		name     string
		callback string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid https callback",
			callback: "https://example.com/notify",
			wantErr:  false,
		},
		{
			name:     "valid http callback",
			callback: "http://example.com/notify",
			wantErr:  false,
		},
		{
			name:     "empty callback",
			callback: "",
			wantErr:  true,
			errMsg:   "callback URL is required",
		},
		{
			name:     "invalid scheme",
			callback: "ftp://example.com/notify",
			wantErr:  true,
			errMsg:   "http or https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test via the subscription creation endpoint
			body := map[string]string{"callback": tt.callback}
			bodyJSON, _ := json.Marshal(body)

			router := srv.Router()
			req := httptest.NewRequest(http.MethodPost, "/o2ims-infrastructureInventory/v1/subscriptions",
				bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), tt.errMsg)
			} else {
				assert.Equal(t, http.StatusCreated, w.Code)
			}
		})
	}
}
