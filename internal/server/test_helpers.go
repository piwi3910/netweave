package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/storage"
	"go.uber.org/zap"
)

// NewTestServer creates a minimal Server instance for testing purposes.
// This function is only used in tests and allows creating a Server with
// specific configuration without all dependencies.
func NewTestServer(cfg *config.Config) *Server {
	return &Server{
		config: cfg,
	}
}

// NewTestServerWithRouter creates a Server instance for testing with router and logger.
// This is useful for testing route handlers. The provided router is used as the admin router,
// and separate routers are created for O2, TMF, and GraphQL to support multi-port architecture.
func NewTestServerWithRouter(router *gin.Engine, logger *zap.Logger) *Server {
	return &Server{
		adminRouter:   router,
		o2Router:      gin.New(),
		tmfRouter:     gin.New(),
		graphqlRouter: gin.New(),
		router:        router,
		logger:        logger,
	}
}

// NewTestServerWithMetrics creates a Server instance for testing with a custom metrics registry.
// This prevents Prometheus registry conflicts when multiple tests create Server instances.
// Each test gets its own isolated metrics registry to avoid "duplicate metrics collector" panics.
// This is a simplified version of New() that bypasses observability.InitMetrics().
// Usage: Call this at the start of each test that would normally call server.New().
func NewTestServerWithMetrics(
	cfg *config.Config,
	logger *zap.Logger,
	adp adapter.Adapter,
	store storage.Store,
) (*Server, *prometheus.Registry) {
	registry := prometheus.NewRegistry()

	// Initialize observability metrics with custom registry (avoids global registry conflicts)
	globalMetrics := observability.InitMetricsWithRegistry("o2ims_test", registry)

	// Set Gin mode
	gin.SetMode(cfg.Server.GinMode)

	// For testing, all routers point to the same engine so that tests can
	// use a single ServeHTTP call to reach any route regardless of which
	// port it would be served on in production.
	router := gin.New()

	// Initialize batch handler (needed for resource CRUD operations)
	batchHandler := handlers.NewBatchHandler(adp, store, logger, globalMetrics)

	// Create minimal server for testing
	srv := &Server{
		config:        cfg,
		logger:        logger,
		adminRouter:   router,
		o2Router:      router,
		tmfRouter:     router,
		graphqlRouter: router,
		router:        router,
		adapter:       adp,
		store:         store,
		metrics:       nil, // Server's own metrics - not needed for these tests
		batchHandler:  batchHandler,
	}

	// Setup routes (needed for resource CRUD tests)
	srv.setupRoutes()

	return srv, registry
}

// NewTestServerWithAuth creates a Server with both the tenant handler and an
// auth middleware wired up, so that route-level security checks
// (RequirePermission, RequireTenantAccess, RequirePlatformAdmin) are actually
// exercised. This is essential for testing cross-tenant access prevention on
// the /admin/tenants/:tenantId endpoints (issue #469).
func NewTestServerWithAuth(
	cfg *config.Config,
	logger *zap.Logger,
	adp adapter.Adapter,
	store storage.Store,
	authStore AuthStore,
	authMw AuthMiddleware,
	tenantHandler *handlers.TenantHandler,
) (*Server, *prometheus.Registry) {
	registry := prometheus.NewRegistry()
	globalMetrics := observability.InitMetricsWithRegistry("o2ims_test", registry)

	gin.SetMode(cfg.Server.GinMode)

	router := gin.New()

	batchHandler := handlers.NewBatchHandler(adp, store, logger, globalMetrics)

	srv := &Server{
		config:        cfg,
		logger:        logger,
		adminRouter:   router,
		o2Router:      router,
		tmfRouter:     router,
		graphqlRouter: router,
		router:        router,
		adapter:       adp,
		store:         store,
		metrics:       nil,
		batchHandler:  batchHandler,
		AuthStore:     authStore,
		authMw:        authMw,
		tenantHandler: tenantHandler,
	}

	srv.setupRoutes()

	return srv, registry
}

// Getter methods for testing - these expose internal fields for test assertions.
// These should only be used in tests.

// Config returns the concrete server configuration for testing.
// Production code reads config through the narrow ServerConfigProvider
// interface (see Server.config), but tests need to mutate fields like
// Security.DisableSSRFProtection directly, so this helper unwraps the
// interface to the concrete *config.Config. Panics if a non-*config.Config
// provider was installed (tests should always use a real Config).
func (s *Server) Config() *config.Config {
	cfg, ok := s.config.(*config.Config)
	if !ok {
		panic("server.Config(): underlying provider is not *config.Config; tests must supply a real config")
	}
	return cfg
}

// Logger returns the server logger for testing.
func (s *Server) Logger() *zap.Logger {
	return s.logger
}

// GetAdapter returns the server adapter for testing.
func (s *Server) GetAdapter() adapter.Adapter {
	return s.adapter
}

// GetStore returns the server store for testing.
func (s *Server) GetStore() storage.Store {
	return s.store
}

// GetAuthMw returns the authentication middleware for testing.
// Note: Returns interface type to match Server's internal storage.
func (s *Server) GetAuthMw() interface{} {
	return s.authMw
}

// HealthCheck returns the health checker for testing.
func (s *Server) HealthCheck() *observability.HealthChecker {
	return s.healthCheck
}

// HTTPServer returns the HTTP server instance for testing.
func (s *Server) HTTPServer() *http.Server {
	return s.httpServer
}

// SetHTTPServer sets the HTTP server for testing (used in test setup).
func (s *Server) SetHTTPServer(srv *http.Server) {
	s.httpServer = srv
}

// PluginRegistry returns the frontend plugin registry for testing.
func (s *Server) PluginRegistry() *FrontendPluginRegistry {
	return s.pluginRegistry
}

// SetPluginRegistry sets the frontend plugin registry for testing.
func (s *Server) SetPluginRegistry(registry *FrontendPluginRegistry) {
	s.pluginRegistry = registry
}

// CORSMiddleware exposes corsMiddleware for testing the CORS policy.
func (s *Server) CORSMiddleware() gin.HandlerFunc {
	return s.corsMiddleware()
}

// BodyLimitMiddleware exposes bodyLimitMiddleware for testing.
func (s *Server) BodyLimitMiddleware() gin.HandlerFunc {
	return s.bodyLimitMiddleware()
}
