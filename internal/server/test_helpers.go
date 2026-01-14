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
// This is useful for testing route handlers.
func NewTestServerWithRouter(router *gin.Engine, logger *zap.Logger) *Server {
	return &Server{
		router: router,
		logger: logger,
	}
}

// NewTestServerWithMetrics creates a Server instance for testing with a custom metrics registry.
// This prevents Prometheus registry conflicts when multiple tests create Server instances.
// Each test gets its own isolated metrics registry to avoid "duplicate metrics collector" panics.
// This is a simplified version of New() that bypasses observability.InitMetrics().
// Usage: Call this at the start of each test that would normally call server.New().
func NewTestServerWithMetrics(cfg *config.Config, logger *zap.Logger, adp adapter.Adapter, store storage.Store) (*Server, *prometheus.Registry) {
	registry := prometheus.NewRegistry()

	// Initialize observability metrics with custom registry (avoids global registry conflicts)
	globalMetrics := observability.InitMetricsWithRegistry("o2ims_test", registry)

	// Set Gin mode
	gin.SetMode(cfg.Server.GinMode)

	// Create router
	router := gin.New()

	// Initialize batch handler (needed for resource CRUD operations)
	batchHandler := handlers.NewBatchHandler(adp, store, logger, globalMetrics)

	// Create minimal server for testing
	srv := &Server{
		config:       cfg,
		logger:       logger,
		router:       router,
		adapter:      adp,
		store:        store,
		metrics:      nil, // Server's own metrics - not needed for these tests
		batchHandler: batchHandler,
	}

	// Setup routes (needed for resource CRUD tests)
	srv.setupRoutes()

	return srv, registry
}

// Getter methods for testing - these expose internal fields for test assertions.
// These should only be used in tests.

// Config returns the server configuration for testing.
func (s *Server) Config() *config.Config {
	return s.config
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
