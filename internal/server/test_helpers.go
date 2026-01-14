package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
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

// Adapter returns the server adapter for testing.
func (s *Server) Adapter() adapter.Adapter {
	return s.adapter
}

// Store returns the server store for testing.
func (s *Server) Store() storage.Store {
	return s.store
}

// HealthCheck returns the health checker for testing.
func (s *Server) HealthCheck() *observability.HealthChecker {
	return s.healthCheck
}

// HTTPServer returns the HTTP server instance for testing.
func (s *Server) HTTPServer() *http.Server {
	return s.httpServer
}

// AuthMw returns the authentication middleware for testing.
func (s *Server) AuthMw() AuthMiddleware {
	return s.authMw
}

// SetHTTPServer sets the HTTP server for testing (used in test setup).
func (s *Server) SetHTTPServer(srv *http.Server) {
	s.httpServer = srv
}
