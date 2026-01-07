// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
// It includes Gin-based routing, middleware setup, and graceful shutdown handling.
package server

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
	dmsregistry "github.com/piwi3910/netweave/internal/dms/registry"
	dmsstorage "github.com/piwi3910/netweave/internal/dms/storage"
	"github.com/piwi3910/netweave/internal/middleware"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/smo"
	"github.com/piwi3910/netweave/internal/storage"
)

// o2imsOpenAPISpec embeds the O2-IMS OpenAPI specification.
//
//go:embed openapi/o2ims.yaml
var o2imsOpenAPISpec []byte

// Server represents the HTTP server for the O2-IMS Gateway.
// It encapsulates the Gin router, configuration, logger, and server state.
//
// The server provides:
//   - O2-IMS API endpoints (/o2ims/v1/*)
//   - Health check endpoints (/health, /ready)
//   - Prometheus metrics endpoint (/metrics)
//   - Request logging and recovery middleware
//   - Graceful shutdown support
//
// Example:
//
//	cfg, err := config.Load("config/config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	logger, _ := zap.NewProduction()
//	srv := server.New(cfg, logger)
//
//	if err := srv.Start(); err != nil {
//	    log.Fatal(err)
//	}
type Server struct {
	config           *config.Config
	logger           *zap.Logger
	router           *gin.Engine
	httpServer       *http.Server
	metrics          *Metrics
	adapter          adapter.Adapter
	store            storage.Store
	healthCheck      *observability.HealthChecker
	openAPIValidator *middleware.OpenAPIValidator
	openAPISpec      []byte

	// DMS subsystem.
	dmsRegistry *dmsregistry.Registry
	dmsStore    dmsstorage.Store
	dmsHandler  *dmshandlers.Handler

	smoRegistry  *smo.Registry
	smoHandler   *SMOHandler
	authStore    AuthStore
	authMw       AuthMiddleware
	shutdownOnce sync.Once // Ensures shutdown logic runs only once
}

// AuthStore defines the interface for auth storage operations.
// This allows the server to remain decoupled from the auth package.
type AuthStore interface {
	Ping(ctx context.Context) error
	Close() error
}

// AuthMiddleware defines the interface for authentication middleware.
type AuthMiddleware interface {
	AuthenticationMiddleware() gin.HandlerFunc
	RequirePermission(permission string) gin.HandlerFunc
	RequirePlatformAdmin() gin.HandlerFunc
}

// Metrics holds Prometheus metrics for the server.
type Metrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	ActiveRequests  prometheus.Gauge
}

// New creates a new Server instance with the given configuration, logger, adapter, and storage.
// It initializes the Gin router, sets up middleware, and configures routes.
//
// The function will panic if essential dependencies are missing or invalid.
//
// Example:
//
//	cfg, _ := config.Load("config/config.yaml")
//	logger, _ := zap.NewProduction()
//	adapter := kubernetes.NewAdapter(cfg, logger)
//	store := storage.NewRedisStore(&storage.RedisConfig{...})
//	srv := server.New(cfg, logger, adapter, store)
func New(cfg *config.Config, logger *zap.Logger, adp adapter.Adapter, store storage.Store) *Server {
	if cfg == nil {
		panic("config cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	if adp == nil {
		panic("adapter cannot be nil")
	}
	if store == nil {
		panic("store cannot be nil")
	}

	// Set Gin mode based on configuration
	gin.SetMode(cfg.Server.GinMode)

	// Create Gin router
	router := gin.New()

	// Initialize metrics
	metrics := initMetrics(cfg)

	// Initialize health checker with adapter and storage checks
	healthCheck := initHealthChecker(cfg, adp, store)

	// Initialize OpenAPI validator
	openAPIValidator, err := initOpenAPIValidator(cfg, logger)
	if err != nil {
		logger.Warn("failed to initialize OpenAPI validator, validation disabled",
			zap.Error(err),
		)
	}

	// Create server instance
	srv := &Server{
		config:           cfg,
		logger:           logger,
		router:           router,
		metrics:          metrics,
		adapter:          adp,
		store:            store,
		healthCheck:      healthCheck,
		openAPIValidator: openAPIValidator,
		openAPISpec:      o2imsOpenAPISpec,
	}

	// Setup middleware
	srv.setupMiddleware()

	// Setup routes
	srv.setupRoutes()

	return srv
}

// initHealthChecker initializes the health checker with component checks.
func initHealthChecker(_ *config.Config, adp adapter.Adapter, store storage.Store) *observability.HealthChecker {
	checker := observability.NewHealthChecker("1.0.0")

	// Register health checks for critical components
	if adp != nil {
		checker.RegisterHealthCheck("adapter", func(ctx context.Context) error {
			return adp.Health(ctx)
		})
	}

	if store != nil {
		checker.RegisterHealthCheck("storage", func(ctx context.Context) error {
			return store.Ping(ctx)
		})
	}

	// Register readiness checks (same components for now)
	if adp != nil {
		checker.RegisterReadinessCheck("adapter", func(ctx context.Context) error {
			return adp.Health(ctx)
		})
	}

	if store != nil {
		checker.RegisterReadinessCheck("storage", func(ctx context.Context) error {
			return store.Ping(ctx)
		})
	}

	return checker
}

// initMetrics initializes Prometheus metrics for the server.
func initMetrics(cfg *config.Config) *Metrics {
	if !cfg.Observability.Metrics.Enabled {
		return nil
	}

	namespace := cfg.Observability.Metrics.Namespace
	subsystem := cfg.Observability.Metrics.Subsystem

	metrics := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		ActiveRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_requests_active",
				Help:      "Number of active HTTP requests",
			},
		),
	}

	// Register metrics
	prometheus.MustRegister(metrics.RequestsTotal)
	prometheus.MustRegister(metrics.RequestDuration)
	prometheus.MustRegister(metrics.ActiveRequests)

	return metrics
}

// initOpenAPIValidator initializes the OpenAPI validator with the embedded spec.
func initOpenAPIValidator(cfg *config.Config, logger *zap.Logger) (*middleware.OpenAPIValidator, error) {
	validationCfg := middleware.DefaultValidationConfig()
	validationCfg.Logger = logger
	validationCfg.ValidateRequest = cfg.Validation.Enabled
	validationCfg.ValidateResponse = cfg.Validation.ValidateResponse

	validator, err := middleware.NewOpenAPIValidator(validationCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI validator: %w", err)
	}

	// Use embedded OpenAPI spec or load from custom path
	var specContent []byte
	if cfg.Validation.SpecPath != "" {
		// Load from custom file path if specified
		if err := validator.LoadSpecFromFile(cfg.Validation.SpecPath); err != nil {
			return nil, fmt.Errorf("failed to load OpenAPI spec from file: %w", err)
		}
		return validator, nil
	}

	// Use embedded spec
	specContent = o2imsOpenAPISpec
	if len(specContent) == 0 {
		return nil, fmt.Errorf("embedded OpenAPI spec is empty")
	}

	if err := validator.LoadSpec(specContent); err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	return validator, nil
}

// setupMiddleware configures middleware for the Gin router.
// Middleware is executed in the order they are added.
func (s *Server) setupMiddleware() {
	// Recovery middleware - must be first to catch panics
	s.router.Use(s.recoveryMiddleware())

	// Request logging middleware
	s.router.Use(s.loggingMiddleware())

	// Metrics middleware (if enabled)
	if s.config.Observability.Metrics.Enabled {
		s.router.Use(s.metricsMiddleware())
	}

	// CORS middleware (if enabled)
	if s.config.Security.EnableCORS {
		s.router.Use(s.corsMiddleware())
	}

	// Rate limiting middleware (if enabled)
	if s.config.Security.RateLimitEnabled {
		s.router.Use(s.rateLimitMiddleware())
	}

	// OpenAPI validation middleware (if enabled and validator is available)
	if s.openAPIValidator != nil && s.config.Validation.Enabled {
		s.router.Use(s.openAPIValidator.Middleware())
		s.logger.Info("OpenAPI request validation enabled")
	}
}

// Start starts the HTTP server and blocks until the server is shut down.
// It supports graceful shutdown on SIGINT and SIGTERM signals.
//
// Returns an error if the server fails to start or encounters an error during shutdown.
//
// Example:
//
//	srv := server.New(cfg, logger)
//	if err := srv.Start(); err != nil {
//	    log.Fatalf("Server failed: %v", err)
//	}
func (s *Server) Start() error {
	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:           addr,
		Handler:        s.router,
		ReadTimeout:    s.config.Server.ReadTimeout,
		WriteTimeout:   s.config.Server.WriteTimeout,
		IdleTimeout:    s.config.Server.IdleTimeout,
		MaxHeaderBytes: s.config.Server.MaxHeaderBytes,
	}

	// Channel to listen for errors from the server
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		s.logger.Info("starting HTTP server",
			zap.String("address", addr),
			zap.String("mode", s.config.Server.GinMode),
		)

		var err error
		if s.config.TLS.Enabled {
			s.logger.Info("TLS enabled",
				zap.String("cert_file", s.config.TLS.CertFile),
				zap.String("min_version", s.config.TLS.MinVersion),
			)
			err = s.httpServer.ListenAndServeTLS(
				s.config.TLS.CertFile,
				s.config.TLS.KeyFile,
			)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or an error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		s.logger.Info("shutdown signal received",
			zap.String("signal", sig.String()),
		)

		// Graceful shutdown
		return s.Shutdown()
	}
}

// Shutdown gracefully shuts down the HTTP server.
// It waits for active requests to complete or until the shutdown timeout expires.
// It also stops SMO health checks and closes the SMO registry.
// This method is safe to call multiple times - only the first call will execute.
//
// Returns an error if the shutdown fails.
func (s *Server) Shutdown() error {
	var shutdownErr error

	s.shutdownOnce.Do(func() {
		s.logger.Info("initiating graceful shutdown",
			zap.Duration("timeout", s.config.Server.ShutdownTimeout),
		)

		// Stop SMO health checks and close registry
		if s.smoRegistry != nil {
			s.logger.Info("stopping SMO plugin health checks")
			if err := s.smoRegistry.Close(); err != nil {
				s.logger.Warn("error closing SMO registry", zap.Error(err))
			}
		}

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(
			context.Background(),
			s.config.Server.ShutdownTimeout,
		)
		defer cancel()

		// Shutdown HTTP server
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.Error("error during shutdown", zap.Error(err))
			shutdownErr = fmt.Errorf("server shutdown failed: %w", err)
			return
		}

		s.logger.Info("server shutdown complete")
	})

	return shutdownErr
}

// Router returns the underlying Gin router.
// This is useful for testing and adding custom routes.
func (s *Server) Router() *gin.Engine {
	return s.router
}

// SetHealthChecker sets the health checker for the server.
// This allows the main application to configure health checks after server creation.
func (s *Server) SetHealthChecker(hc *observability.HealthChecker) {
	s.healthCheck = hc
}

// SetupDMS initializes the DMS subsystem with the provided registry.
// This must be called after creating the server to enable O2-DMS API endpoints.
func (s *Server) SetupDMS(reg *dmsregistry.Registry) {
	s.dmsRegistry = reg
	s.dmsStore = dmsstorage.NewMemoryStore()
	s.dmsHandler = dmshandlers.NewHandler(reg, s.dmsStore, s.logger)

	// Set up DMS routes.
	s.setupDMSRoutes(s.dmsHandler)

	// Register DMS health check.
	if s.healthCheck != nil {
		s.healthCheck.RegisterHealthCheck("dms", s.dmsHandler.Health)
		s.healthCheck.RegisterReadinessCheck("dms", s.dmsHandler.Health)
	}

	s.logger.Info("DMS subsystem initialized")
}

// DMSRegistry returns the DMS adapter registry.
func (s *Server) DMSRegistry() *dmsregistry.Registry {
	return s.dmsRegistry
}

// SetSMORegistry sets the SMO plugin registry and configures SMO API routes.
// This enables the O2-SMO API endpoints for workflow orchestration, service modeling,
// policy management, and infrastructure synchronization.
// It also starts periodic health checks for registered plugins.
func (s *Server) SetSMORegistry(registry *smo.Registry) {
	s.smoRegistry = registry
	s.smoHandler = NewSMOHandler(registry, s.logger)
	s.setupSMORoutes(s.smoHandler)

	// Start periodic health checks for SMO plugins
	registry.StartHealthChecks(context.Background())

	s.logger.Info("SMO registry configured",
		zap.Int("plugin_count", registry.Count()),
	)
}

// SMORegistry returns the SMO plugin registry.
// This can be used to register additional plugins after server creation.
func (s *Server) SMORegistry() *smo.Registry {
	return s.smoRegistry
}

// recoveryMiddleware recovers from panics and logs the error.
func (s *Server) recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("client_ip", c.ClientIP()),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
				})
			}
		}()
		c.Next()
	}
}

// loggingMiddleware logs HTTP requests and responses.
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Log request details
		s.logger.Info("HTTP request",
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("client_ip", c.ClientIP()),
			zap.Duration("latency", latency),
			zap.Int("body_size", c.Writer.Size()),
			zap.String("user_agent", c.Request.UserAgent()),
		)

		// Log errors if any
		if len(c.Errors) > 0 {
			for _, e := range c.Errors {
				s.logger.Error("request error", zap.Error(e.Err))
			}
		}
	}
}

// metricsMiddleware collects Prometheus metrics for HTTP requests.
func (s *Server) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.metrics == nil {
			c.Next()
			return
		}

		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// Increment active requests
		s.metrics.ActiveRequests.Inc()
		defer s.metrics.ActiveRequests.Dec()

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())

		s.metrics.RequestsTotal.WithLabelValues(
			c.Request.Method,
			path,
			status,
		).Inc()

		s.metrics.RequestDuration.WithLabelValues(
			c.Request.Method,
			path,
			status,
		).Observe(duration)
	}
}

// corsMiddleware adds CORS headers to responses.
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		if len(s.config.Security.AllowedOrigins) == 0 {
			allowed = true // Allow all if not specified
		} else {
			for _, allowedOrigin := range s.config.Security.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}
		}

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers",
				joinStrings(s.config.Security.AllowedHeaders, ", "))
			c.Writer.Header().Set("Access-Control-Allow-Methods",
				joinStrings(s.config.Security.AllowedMethods, ", "))
		}

		// Handle preflight requests
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware implements rate limiting for HTTP requests.
// TODO: Implement Redis-based distributed rate limiting.
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement rate limiting logic
		// For now, just pass through
		c.Next()
	}
}

// joinStrings joins a slice of strings with the given separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
