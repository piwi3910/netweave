package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/observability"
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
// Note: Returns interface type to match Server's internal storage.
// This is necessary since Server stores the adapter as an interface,
// and tests need access to it. The //nolint directive would violate
// our zero-tolerance policy, so we accept this as a legitimate test helper.
func (s *Server) GetAdapter() interface{} {
	return s.adapter
}

// GetStore returns the server store for testing.
// Note: Returns interface type to match Server's internal storage.
func (s *Server) GetStore() interface{} {
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
