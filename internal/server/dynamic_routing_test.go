package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
	dmsregistry "github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/smo"
)

// ========================================
// Context Helper Tests
// ========================================

func TestIMSAdapterFromContext(t *testing.T) {
	tests := []struct {
		name      string
		setupCtx  func(c *gin.Context)
		expectNil bool
	}{
		{
			name:      "no adapter in context returns nil",
			setupCtx:  func(_ *gin.Context) {},
			expectNil: true,
		},
		{
			name: "valid adapter in context",
			setupCtx: func(c *gin.Context) {
				c.Set("resolved_ims_adapter", &mockAdapter{})
			},
			expectNil: false,
		},
		{
			name: "wrong type in context returns nil",
			setupCtx: func(c *gin.Context) {
				c.Set("resolved_ims_adapter", "not-an-adapter")
			},
			expectNil: true,
		},
		{
			name: "nil value in context returns nil",
			setupCtx: func(c *gin.Context) {
				c.Set("resolved_ims_adapter", nil)
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

			tt.setupCtx(c)

			adp := server.IMSAdapterFromContext(c)
			if tt.expectNil {
				assert.Nil(t, adp)
			} else {
				assert.NotNil(t, adp)
			}
		})
	}
}

func TestDMSRegistryFromContext(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name      string
		setupCtx  func(c *gin.Context)
		expectNil bool
	}{
		{
			name:      "no registry in context returns nil",
			setupCtx:  func(_ *gin.Context) {},
			expectNil: true,
		},
		{
			name: "valid registry in context",
			setupCtx: func(c *gin.Context) {
				reg := dmsregistry.NewRegistry(logger, nil)
				c.Set("resolved_dms_registry", reg)
			},
			expectNil: false,
		},
		{
			name: "wrong type in context returns nil",
			setupCtx: func(c *gin.Context) {
				c.Set("resolved_dms_registry", "not-a-registry")
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

			tt.setupCtx(c)

			reg := server.DMSRegistryFromContext(c)
			if tt.expectNil {
				assert.Nil(t, reg)
			} else {
				assert.NotNil(t, reg)
			}
		})
	}
}

func TestSMORegistryFromContext(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name      string
		setupCtx  func(c *gin.Context)
		expectNil bool
	}{
		{
			name:      "no registry in context returns nil",
			setupCtx:  func(_ *gin.Context) {},
			expectNil: true,
		},
		{
			name: "valid registry in context",
			setupCtx: func(c *gin.Context) {
				reg := smo.NewRegistry(logger)
				c.Set("resolved_smo_registry", reg)
			},
			expectNil: false,
		},
		{
			name: "wrong type in context returns nil",
			setupCtx: func(c *gin.Context) {
				c.Set("resolved_smo_registry", 42)
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

			tt.setupCtx(c)

			reg := server.SMORegistryFromContext(c)
			if tt.expectNil {
				assert.Nil(t, reg)
			} else {
				assert.NotNil(t, reg)
			}
		})
	}
}

// ========================================
// DMS Middleware Integration Tests
// ========================================

func TestDMSMiddleware_InjectsDMSRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	router := gin.New()
	srv := server.NewTestServerWithRouter(router, logger)

	// Create a DMS registry with a mock adapter.
	dmsReg := dmsregistry.NewRegistry(logger, nil)
	mockAdp := newMockDMSAdapter()
	err := dmsReg.Register(context.Background(), "test-dms-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	srv.SetupDMS(dmsReg)

	// The deployment lifecycle endpoint is on the O2 router (mTLS port).
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/deploymentLifecycle", nil)
	w := httptest.NewRecorder()
	srv.O2Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify the adapter was resolved from the registry.
	adapters, ok := response["adapters"].([]interface{})
	require.True(t, ok)
	assert.Len(t, adapters, 1)

	firstAdapter, ok := adapters[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test-dms-adapter", firstAdapter["name"])
}

func TestDMSMiddleware_ListNFDeployments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	router := gin.New()
	srv := server.NewTestServerWithRouter(router, logger)

	// Create a DMS registry with a mock adapter.
	dmsReg := dmsregistry.NewRegistry(logger, nil)
	mockAdp := newMockDMSAdapter()
	err := dmsReg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)

	srv.SetupDMS(dmsReg)

	// List NF deployments endpoint is on the O2 router (mTLS port).
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/nfDeployments", nil)
	w := httptest.NewRecorder()
	srv.O2Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ========================================
// SMO Middleware Integration Tests
// ========================================

func TestSMOMiddleware_ListPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	router := gin.New()
	srv := server.NewTestServerWithRouter(router, logger)

	// Create SMO registry.
	smoReg := smo.NewRegistry(logger)
	srv.SetSMORegistry(smoReg)

	// List plugins endpoint is on the O2 router (mTLS port).
	req := httptest.NewRequest(http.MethodGet, "/o2smo/v1/plugins", nil)
	w := httptest.NewRecorder()
	srv.O2Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response, "plugins")
	assert.Contains(t, response, "total")
}

func TestSMOMiddleware_HealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	router := gin.New()
	srv := server.NewTestServerWithRouter(router, logger)

	// Create SMO registry.
	smoReg := smo.NewRegistry(logger)
	srv.SetSMORegistry(smoReg)

	// Health endpoint is on the O2 router (mTLS port).
	req := httptest.NewRequest(http.MethodGet, "/o2smo/v1/health", nil)
	w := httptest.NewRecorder()
	srv.O2Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ========================================
// TMForum Middleware Integration Tests
// ========================================

func TestTMForumMiddleware_Returns503WithoutDMS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Create a server with a mock adapter but no DMS.
	srv, _ := server.NewTestServerWithMetrics(cfg, logger, &mockAdapter{}, &mockStore{})

	// Without DMS initialization, TMForum routes should return 503.
	req := httptest.NewRequest(http.MethodGet, "/tmf-api/resourceInventoryManagement/v4/resource", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestTMForumMiddleware_WorksWithDMS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}

	// Create a server with a mock adapter.
	srv, _ := server.NewTestServerWithMetrics(cfg, logger, &mockAdapter{}, &mockStore{})

	// Initialize DMS subsystem.
	dmsReg := dmsregistry.NewRegistry(logger, nil)
	mockAdp := newMockDMSAdapter()
	err := dmsReg.Register(context.Background(), "test-adapter", "mock", mockAdp, nil, true)
	require.NoError(t, err)
	srv.SetupDMS(dmsReg)

	// With DMS initialized, TMForum routes should work (return 200).
	req := httptest.NewRequest(http.MethodGet, "/tmf-api/resourceInventoryManagement/v4/resource", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ========================================
// GraphQL Integration Tests
// ========================================

func TestGraphQLRoute_WithStaticAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: "debug",
		},
	}

	srv, _ := server.NewTestServerWithMetrics(cfg, logger, &mockAdapter{}, &mockStore{})

	// In debug mode, GET /graphql returns the playground HTML.
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGraphQLRoute_WithoutAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: "debug",
		},
	}

	srv, _ := server.NewTestServerWithMetrics(cfg, logger, nil, &mockStore{})

	// Without any adapter source, GraphQL routes should not be registered.
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Should return 404 since GraphQL routes were not registered.
	assert.Equal(t, http.StatusNotFound, w.Code)
}
