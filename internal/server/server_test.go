package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/storage"
)

// mockAdapter implements adapter.Adapter for testing.
type mockAdapter struct {
	healthErr error
}

func (m *mockAdapter) Name() string    { return "mock" }
func (m *mockAdapter) Version() string { return "1.0.0" }
func (m *mockAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{adapter.CapabilityResourcePools}
}
func (m *mockAdapter) Health(ctx context.Context) error {
	if m.healthErr != nil {
		return m.healthErr
	}
	return nil
}
func (m *mockAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, nil
}
func (m *mockAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	return nil, adapter.ErrResourcePoolNotFound
}
func (m *mockAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}
func (m *mockAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return pool, nil
}
func (m *mockAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, nil
}
func (m *mockAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	return resource, nil
}
func (m *mockAdapter) UpdateResource(ctx context.Context, id string, resource *adapter.Resource) (*adapter.Resource, error) {
	resource.ResourceID = id
	return resource, nil
}
func (m *mockAdapter) DeleteResource(ctx context.Context, id string) error {
	// Return not found for non-existent resources
	if id == "res-nonexistent" || id == "res-123" {
		return adapter.ErrResourceNotFound
	}
	return nil
}
func (m *mockAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}
func (m *mockAdapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockAdapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockAdapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return sub, nil
}
func (m *mockAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	return nil, adapter.ErrResourceNotFound
}
func (m *mockAdapter) UpdateSubscription(ctx context.Context, id string, sub *adapter.Subscription) (*adapter.Subscription, error) {
	// Validate callback URL (consistent with real adapters)
	if sub.Callback == "" {
		return nil, errors.New("callback URL is required")
	}
	sub.SubscriptionID = id
	return sub, nil
}
func (m *mockAdapter) DeleteSubscription(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdapter) Close() error {
	return nil
}

// mockStore implements storage.Store for testing.
type mockStore struct{}

func (m *mockStore) Create(ctx context.Context, sub *storage.Subscription) error {
	return nil
}
func (m *mockStore) Get(ctx context.Context, id string) (*storage.Subscription, error) {
	return nil, storage.ErrSubscriptionNotFound
}
func (m *mockStore) Update(ctx context.Context, sub *storage.Subscription) error {
	return nil
}
func (m *mockStore) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *mockStore) List(ctx context.Context) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockStore) ListByTenant(ctx context.Context, tenantID string) ([]*storage.Subscription, error) {
	return nil, nil
}
func (m *mockStore) Close() error {
	return nil
}
func (m *mockStore) Ping(ctx context.Context) error {
	return nil
}

// mockAuthStore implements AuthStore interface for testing.
type mockAuthStore struct {
	pingErr error
}

func (m *mockAuthStore) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockAuthStore) Close() error {
	return nil
}

// mockAuthMiddleware implements AuthMiddleware interface for testing.
type mockAuthMiddleware struct{}

func (m *mockAuthMiddleware) AuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func (m *mockAuthMiddleware) RequirePermission(_ string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func (m *mockAuthMiddleware) RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func TestNew(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	logger := zap.NewNop()
	adp := &mockAdapter{}
	store := &mockStore{}

	srv := New(cfg, logger, adp, store)

	assert.NotNil(t, srv)
	assert.NotNil(t, srv.router)
	assert.NotNil(t, srv.config)
	assert.NotNil(t, srv.logger)
	assert.NotNil(t, srv.adapter)
	assert.NotNil(t, srv.store)
}

func TestNew_Panics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	logger := zap.NewNop()
	adp := &mockAdapter{}
	store := &mockStore{}

	tests := []struct {
		name   string
		cfg    *config.Config
		logger *zap.Logger
		adp    adapter.Adapter
		store  storage.Store
	}{
		{"nil config", nil, logger, adp, store},
		{"nil logger", cfg, nil, adp, store},
		{"nil adapter", cfg, logger, nil, store},
		{"nil store", cfg, logger, adp, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				New(tt.cfg, tt.logger, tt.adp, tt.store)
			})
		})
	}
}

func TestServer_Router(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := srv.Router()
	assert.NotNil(t, router)
	assert.Equal(t, srv.router, router)
}

func TestServer_SetHealthChecker(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	// Health checker should be set by New
	assert.NotNil(t, srv.healthCheck)
}

func TestServer_SetOpenAPISpec(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	spec := []byte("openapi: 3.0.0")
	srv.SetOpenAPISpec(spec)

	retrieved := srv.GetOpenAPISpec()
	assert.Equal(t, spec, retrieved)
}

func TestServer_GetOpenAPISpec(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	// Should have default spec from New
	spec := srv.GetOpenAPISpec()
	assert.NotNil(t, spec)
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name     string
		strs     []string
		sep      string
		expected string
	}{
		{"empty", []string{}, ",", ""},
		{"single", []string{"a"}, ",", "a"},
		{"multiple", []string{"a", "b", "c"}, ",", "a,b,c"},
		{"with spaces", []string{"a", "b"}, ", ", "a, b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinStrings(tt.strs, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.recoveryMiddleware())

	router.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should recover and return 500
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLoggingMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.loggingMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMetricsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.metricsMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestServer_ShutdownWithContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	// Create a mock HTTP server
	srv.httpServer = &http.Server{
		Addr:    ":8080",
		Handler: srv.router,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.ShutdownWithContext(ctx)
	// Should not error even if server wasn't started
	assert.NoError(t, err)
}

func TestServer_SetupAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		authStore    AuthStore
		authMw       AuthMiddleware
		wantAuthNil  bool
		wantStoreNil bool
	}{
		{
			name:         "successful setup",
			authStore:    &mockAuthStore{},
			authMw:       &mockAuthMiddleware{},
			wantAuthNil:  false,
			wantStoreNil: false,
		},
		{
			name:         "with ping error store",
			authStore:    &mockAuthStore{pingErr: errors.New("connection refused")},
			authMw:       &mockAuthMiddleware{},
			wantAuthNil:  false,
			wantStoreNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: gin.TestMode,
				},
			}
			srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

			// Call SetupAuth
			srv.SetupAuth(tt.authStore, tt.authMw)

			// Verify auth store is set
			if tt.wantStoreNil {
				assert.Nil(t, srv.AuthStore())
			} else {
				assert.NotNil(t, srv.AuthStore())
				assert.Equal(t, tt.authStore, srv.AuthStore())
			}

			// Verify auth middleware is set
			if tt.wantAuthNil {
				assert.Nil(t, srv.authMw)
			} else {
				assert.NotNil(t, srv.authMw)
			}
		})
	}
}

func TestServer_AuthStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv := New(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	// Before setup, should be nil
	assert.Nil(t, srv.AuthStore())

	// After setup
	authStore := &mockAuthStore{}
	srv.SetupAuth(authStore, &mockAuthMiddleware{})

	assert.NotNil(t, srv.AuthStore())
	assert.Equal(t, authStore, srv.AuthStore())
}
