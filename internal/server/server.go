// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
// It includes Gin-based routing, middleware setup, and graceful shutdown handling.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/certlifecycle"
	"github.com/piwi3910/netweave/internal/config"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
	dmsregistry "github.com/piwi3910/netweave/internal/dms/registry"
	dmsstorage "github.com/piwi3910/netweave/internal/dms/storage"
	"github.com/piwi3910/netweave/internal/handlers"
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
	config *config.Config
	logger *zap.Logger

	// Multi-port routers: each port gets its own Gin engine with appropriate auth.
	adminRouter   *gin.Engine // Admin, health, docs, metrics (port 8080)
	o2Router      *gin.Engine // O2-IMS, O2-DMS, O2-SMO with mTLS (port 8443)
	tmfRouter     *gin.Engine // TMForum APIs with OAuth2 (port 8444)
	graphqlRouter *gin.Engine // GraphQL API with OAuth2 (port 8445)

	// router is an alias for adminRouter for backward compatibility with tests.
	router *gin.Engine

	// Multi-port HTTP servers
	httpServer       *http.Server // Admin listener (8080)
	o2Server         *http.Server // O2 mTLS listener (8443)
	tmfServer        *http.Server // TMForum listener (8444)
	graphqlServer    *http.Server // GraphQL listener (8445)
	metrics          *Metrics
	adapter          adapter.Adapter
	store            storage.Store
	healthCheck      *observability.HealthChecker
	openAPIValidator *middleware.OpenAPIValidator
	openAPISpec      []byte

	// Handlers
	batchHandler  *handlers.BatchHandler
	tenantHandler *handlers.TenantHandler

	// DMS subsystem.
	dmsRegistry *dmsregistry.Registry
	dmsStore    dmsstorage.Store
	dmsHandler  *dmshandlers.Handler

	smoRegistry *smo.Registry
	smoHandler  *SMOHandler

	// TMForum subsystem
	tmfHandler *handlers.TMForumHandler

	// Dynamic backend adapter routing
	adapterRegistry *backend.AdapterRegistry
	backendStore    backend.Store

	// Frontend plugin toggle system
	pluginRegistry *FrontendPluginRegistry

	// fullAuthStore is the full auth.Store for tenant lookups in wrapWithTenantContext.
	fullAuthStore auth.Store

	// AuthStore is the authentication store interface (public for testing)
	AuthStore    AuthStore
	authMw       AuthMiddleware
	o2AuthMw     AuthMiddleware    // mTLS-only auth middleware for O2 router
	auditLogger  *auth.AuditLogger // Audit logging for security events
	shutdownOnce sync.Once         // Ensures shutdown logic runs only once
}

// AuthStore defines the interface for auth storage operations.
// This allows the server to remain decoupled from the auth package.
type AuthStore interface {
	Ping(ctx context.Context) error
	Close() error
	IncrementUsage(ctx context.Context, tenantID, usageType string) error
	DecrementUsage(ctx context.Context, tenantID, usageType string) error
	LogEvent(ctx context.Context, event *auth.AuditEvent) error
}

// AuthMiddleware defines the interface for authentication middleware.
type AuthMiddleware interface {
	AuthenticationMiddleware() gin.HandlerFunc
	RequirePermission(permission string) gin.HandlerFunc
	RequirePlatformAdmin() gin.HandlerFunc
	// RequireTenantAccess ensures the authenticated caller is a platform admin
	// or that their TenantID matches the value of the URL parameter named by
	// tenantIDParam. Used to enforce tenant ownership on tenant-admin routes
	// (see issue #469).
	RequireTenantAccess(tenantIDParam string) gin.HandlerFunc
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
// The authStore parameter is optional. If provided, enables multi-tenancy and RBAC features.
//
// The function will panic if essential dependencies are missing or invalid.
//
// Example:
//
//	cfg, _ := config.Load("config/config.yaml")
//	logger, _ := zap.NewProduction()
//	adapter := kubernetes.NewAdapter(cfg, logger)
//	store := storage.NewRedisStore(&storage.RedisConfig{...})
//	authStore := auth.NewRedisStore(&auth.RedisConfig{...})
//	srv := server.New(cfg, logger, adapter, store, authStore)
//	srv := server.New(cfg, logger, adapter, store, authStore, authMw) // with pre-configured auth middleware
func New(
	cfg *config.Config,
	logger *zap.Logger,
	adp adapter.Adapter,
	store storage.Store,
	authStore AuthStore,
	opts ...interface{},
) *Server {
	if cfg == nil {
		panic("config cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	if store == nil {
		panic("store cannot be nil")
	}

	// Set Gin mode based on configuration
	gin.SetMode(cfg.Server.GinMode)

	// Create 4 separate Gin routers for multi-port architecture
	adminRouter := gin.New()
	o2Router := gin.New()
	tmfRouter := gin.New()
	graphqlRouter := gin.New()

	// Initialize metrics
	metrics := initMetrics(cfg)

	// Initialize global observability metrics
	globalMetrics := observability.InitMetrics(cfg.Observability.Metrics.Namespace)

	// Initialize health checker with adapter and storage checks
	healthCheck := initHealthChecker(cfg, adp, store, authStore)

	// Initialize OpenAPI validator
	openAPIValidator, err := initOpenAPIValidator(cfg, logger)
	if err != nil {
		logger.Warn("failed to initialize OpenAPI validator, validation disabled",
			zap.Error(err),
		)
	}

	// Initialize batch handler
	batchHandler := handlers.NewBatchHandler(adp, store, logger, globalMetrics)

	// Initialize auth middleware and tenant handler if auth store is provided
	var authMw AuthMiddleware
	var o2AuthMw AuthMiddleware
	var auditLogger *auth.AuditLogger
	var tenantHandler *handlers.TenantHandler

	// Check if a pre-configured auth middleware was passed via opts
	var preConfiguredMw *auth.Middleware
	for _, opt := range opts {
		if mw, ok := opt.(*auth.Middleware); ok && mw != nil {
			preConfiguredMw = mw
		}
	}

	if authStore != nil {
		// Type assert authStore to auth.Store for middleware initialization
		authStoreTyped, ok := authStore.(auth.Store)
		if !ok {
			logger.Warn("auth store does not implement auth.Store interface, auth middleware disabled")
		} else {
			if preConfiguredMw != nil {
				// Use the pre-configured middleware for admin/TMF/GraphQL (OAuth2-only)
				authMw = preConfiguredMw
			} else {
				// Create OAuth2-only middleware for admin/TMF/GraphQL routers
				adminMwConfig := &auth.MiddlewareConfig{
					Enabled:     true,
					RequireMTLS: false,
					SkipPaths:   []string{"/health", "/healthz", "/ready", "/readyz", "/metrics"},
				}
				authMw = auth.NewMiddleware(authStoreTyped, adminMwConfig, logger, nil, nil)
			}

			// Create mTLS-required middleware for O2 router
			o2MwConfig := &auth.MiddlewareConfig{
				Enabled:     true,
				RequireMTLS: cfg.MultiTenancy.RequireMTLS,
				SkipPaths:   []string{},
			}
			o2AuthMw = auth.NewMiddleware(authStoreTyped, o2MwConfig, logger, nil, nil)

			// Initialize audit logger with the same auth store
			var err error
			auditLogger, err = auth.NewAuditLogger(authStoreTyped, logger)
			if err != nil {
				logger.Warn("failed to initialize audit logger", zap.Error(err))
			}

			// Initialize tenant handler
			tenantHandler = handlers.NewTenantHandler(authStoreTyped, logger)
		}
	}

	// Initialize frontend plugin registry
	pluginReg := NewFrontendPluginRegistry(cfg.FrontendPlugins)

	// Create server instance
	srv := &Server{
		config:           cfg,
		logger:           logger,
		adminRouter:      adminRouter,
		o2Router:         o2Router,
		tmfRouter:        tmfRouter,
		graphqlRouter:    graphqlRouter,
		router:           adminRouter, // backward compat alias
		metrics:          metrics,
		adapter:          adp,
		store:            store,
		healthCheck:      healthCheck,
		openAPIValidator: openAPIValidator,
		openAPISpec:      o2imsOpenAPISpec,
		batchHandler:     batchHandler,
		tenantHandler:    tenantHandler,
		AuthStore:        authStore,
		authMw:           authMw,
		o2AuthMw:         o2AuthMw,
		auditLogger:      auditLogger,
		pluginRegistry:   pluginReg,
	}

	// Setup middleware on all routers
	srv.setupMiddleware()

	// Setup routes on appropriate routers
	srv.setupRoutes()

	// Setup auth routes if multi-tenancy is enabled
	if authStore != nil && authMw != nil {
		authStoreTyped, ok := authStore.(auth.Store)
		if ok {
			if mw, mwOk := authMw.(*auth.Middleware); mwOk {
				srv.SetupAuthRoutes(authStoreTyped, mw)
			} else {
				logger.Warn("auth middleware type mismatch, skipping auth routes")
			}
		} else {
			logger.Warn("auth store does not implement auth.Store interface, admin API disabled")
		}
	}

	return srv
}

// initHealthChecker initializes the health checker with component checks.
func initHealthChecker(
	_ *config.Config,
	adp adapter.Adapter,
	store storage.Store,
	authStore AuthStore,
) *observability.HealthChecker {
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

	if authStore != nil {
		checker.RegisterHealthCheck("auth_storage", func(ctx context.Context) error {
			return authStore.Ping(ctx)
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

	// Exclude TMForum API paths from validation (not in O2-IMS OpenAPI spec)
	validationCfg.ExcludePaths = append(validationCfg.ExcludePaths, "/tmf-api/")

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

// setupMiddleware configures middleware for all Gin routers.
// Shared middleware (recovery, logging, metrics, security headers) is applied to all routers.
// Auth middleware is applied per-router: mTLS for O2, OAuth2 for admin/TMF/GraphQL.
func (s *Server) setupMiddleware() {
	allRouters := []*gin.Engine{s.adminRouter, s.o2Router, s.tmfRouter, s.graphqlRouter}

	// Apply shared middleware to all routers
	bodyLimitMw := s.bodyLimitMiddleware()
	for _, r := range allRouters {
		r.Use(s.RecoveryMiddleware())
		r.Use(s.securityHeadersMiddleware())
		r.Use(s.LoggingMiddleware())

		// Enforce request-body size cap on every router before we read the
		// body anywhere (OpenAPI validator, GraphQL handler, etc.). This
		// ensures GraphQL/TMForum/admin routes that are not covered by the
		// OpenAPI spec still have a hard body limit (issue #494).
		r.Use(bodyLimitMw)

		if s.config.Observability.Metrics.Enabled {
			r.Use(s.MetricsMiddleware())
		}

		if s.config.Security.EnableCORS {
			r.Use(s.corsMiddleware())
		}

		if s.config.Security.RateLimitEnabled {
			r.Use(s.rateLimitMiddleware())
		}

		if s.config.Security.RateLimit.PerResource.Enabled {
			r.Use(s.resourceRateLimitMiddleware())
		}
	}

	// OpenAPI validation only on admin and O2 routers (O2-IMS spec applies there)
	if s.openAPIValidator != nil && s.config.Validation.Enabled {
		s.adminRouter.Use(s.openAPIValidator.Middleware())
		s.o2Router.Use(s.openAPIValidator.Middleware())
		s.logger.Info("OpenAPI request validation enabled")
	}

	// Auth middleware per-router:
	// - O2 router: mTLS-required auth (s.o2AuthMw)
	// - Admin/TMF/GraphQL routers: OAuth2-only auth (s.authMw)
	if s.o2AuthMw != nil {
		s.o2Router.Use(s.o2AuthMw.AuthenticationMiddleware())
		s.logger.Info("O2 router: mTLS authentication middleware enabled")
	}

	if s.authMw != nil {
		s.adminRouter.Use(s.authMw.AuthenticationMiddleware())
		s.tmfRouter.Use(s.authMw.AuthenticationMiddleware())
		s.graphqlRouter.Use(s.authMw.AuthenticationMiddleware())
		s.logger.Info("admin/TMF/GraphQL routers: OAuth2 authentication middleware enabled")
	}
}

// securityHeadersMiddleware returns the security headers middleware.
func (s *Server) securityHeadersMiddleware() gin.HandlerFunc {
	config := &middleware.SecurityHeadersConfig{
		Enabled:               s.config.Security.SecurityHeaders.Enabled,
		HSTSMaxAge:            s.config.Security.SecurityHeaders.HSTSMaxAge,
		HSTSIncludeSubDomains: s.config.Security.SecurityHeaders.HSTSIncludeSubDomains,
		HSTSPreload:           s.config.Security.SecurityHeaders.HSTSPreload,
		ContentSecurityPolicy: s.config.Security.SecurityHeaders.ContentSecurityPolicy,
		FrameOptions:          s.config.Security.SecurityHeaders.FrameOptions,
		ReferrerPolicy:        s.config.Security.SecurityHeaders.ReferrerPolicy,
		TLSEnabled:            s.config.TLS.Enabled,
	}

	// Apply defaults if not configured
	if config.ContentSecurityPolicy == "" {
		config.ContentSecurityPolicy = "default-src 'none'; frame-ancestors 'none'"
	}
	if config.FrameOptions == "" {
		config.FrameOptions = "DENY"
	}
	if config.ReferrerPolicy == "" {
		config.ReferrerPolicy = "strict-origin-when-cross-origin"
	}
	if config.HSTSMaxAge == 0 {
		config.HSTSMaxAge = 31536000 // 1 year
	}

	return middleware.SecurityHeaders(config)
}

// bodyLimitMiddleware returns a global middleware that caps request body
// size on every router (admin, O2, TMF, GraphQL). The default cap is
// controlled by validation.max_body_size in config with a 1 MiB fallback.
// Individual routes may register larger caps via middleware.BodyLimitConfig.PerRoute
// when this function is extended in the future.
func (s *Server) bodyLimitMiddleware() gin.HandlerFunc {
	cfg := middleware.BodyLimitConfig{
		Default: s.config.Validation.MaxBodySize,
		Logger:  s.logger,
	}
	if cfg.Default <= 0 {
		cfg.Default = middleware.DefaultBodyLimit
	}
	return middleware.BodyLimit(cfg)
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
	host := s.config.Server.Host

	// Build TLS configs if TLS is enabled
	var standardTLS, o2TLS *tls.Config
	if s.config.TLS.Enabled {
		var err error
		standardTLS, err = s.buildStandardTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to build standard TLS config: %w", err)
		}
		o2TLS, err = s.buildO2TLSConfig()
		if err != nil {
			return fmt.Errorf("failed to build O2 mTLS config: %w", err)
		}
	}

	// Create admin server (port 8080)
	adminAddr := fmt.Sprintf("%s:%d", host, s.config.Server.Port)
	s.httpServer = s.newHTTPServer(adminAddr, s.adminRouter, standardTLS)

	// Create O2 mTLS server (port 8443)
	o2Addr := fmt.Sprintf("%s:%d", host, s.config.Server.O2Port)
	s.o2Server = s.newHTTPServer(o2Addr, s.o2Router, o2TLS)

	// Create TMForum server (port 8444)
	tmfAddr := fmt.Sprintf("%s:%d", host, s.config.Server.TMFPort)
	s.tmfServer = s.newHTTPServer(tmfAddr, s.tmfRouter, standardTLS)

	// Create GraphQL server (port 8445)
	gqlAddr := fmt.Sprintf("%s:%d", host, s.config.Server.GraphQLPort)
	s.graphqlServer = s.newHTTPServer(gqlAddr, s.graphqlRouter, standardTLS)

	// Channel to listen for errors from any server
	serverErrors := make(chan error, 4)

	// Start all 4 servers
	s.startListener("admin", s.httpServer, serverErrors)
	s.startListener("o2", s.o2Server, serverErrors)
	s.startListener("tmf", s.tmfServer, serverErrors)
	s.startListener("graphql", s.graphqlServer, serverErrors)

	// Brief grace period for port binding failures to surface.
	// ListenAndServe returns immediately on bind errors (e.g. port in use),
	// so 200ms is enough to detect them before entering the main loop.
	time.Sleep(200 * time.Millisecond)

	// Drain any startup errors that arrived during the grace period.
	var startupErrs []error
	for {
		select {
		case err := <-serverErrors:
			startupErrs = append(startupErrs, err)
		default:
			goto doneCheck
		}
	}
doneCheck:
	if len(startupErrs) > 0 {
		s.logger.Error("server startup failed, shutting down all listeners",
			zap.Int("failedCount", len(startupErrs)),
		)
		if shutdownErr := s.Shutdown(); shutdownErr != nil {
			s.logger.Error("shutdown after failed startup reported an error",
				zap.Error(shutdownErr),
			)
		}
		return fmt.Errorf("server startup failed: %w", errors.Join(startupErrs...))
	}

	s.logger.Info("all servers started successfully")

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or a runtime error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		s.logger.Info("shutdown signal received",
			zap.String("signal", sig.String()),
		)
		return s.Shutdown()
	}
}

// newHTTPServer creates an http.Server with shared timeout config.
func (s *Server) newHTTPServer(addr string, handler http.Handler, tlsCfg *tls.Config) *http.Server {
	srv := &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    s.config.Server.ReadTimeout,
		WriteTimeout:   s.config.Server.WriteTimeout,
		IdleTimeout:    s.config.Server.IdleTimeout,
		MaxHeaderBytes: s.config.Server.MaxHeaderBytes,
	}
	if tlsCfg != nil {
		srv.TLSConfig = tlsCfg
	}
	return srv
}

// startListener starts an HTTP/HTTPS server in a goroutine.
func (s *Server) startListener(name string, srv *http.Server, errCh chan<- error) {
	go func() {
		s.logger.Info("starting HTTP server",
			zap.String("name", name),
			zap.String("address", srv.Addr),
		)

		var err error
		if s.config.TLS.Enabled {
			err = srv.ListenAndServeTLS(
				s.config.TLS.CertFile,
				s.config.TLS.KeyFile,
			)
		} else {
			err = srv.ListenAndServe()
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("%s server: %w", name, err)
		}
	}()
}

// buildBaseTLSConfig creates a base TLS config with minimum version setting.
func (s *Server) buildBaseTLSConfig() *tls.Config {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	switch s.config.TLS.MinVersion {
	case "1.2":
		tlsCfg.MinVersion = tls.VersionTLS12
	case "1.3", "":
		tlsCfg.MinVersion = tls.VersionTLS13
	}

	return tlsCfg
}

// loadClientCAs loads the CA certificate pool for client verification.
func (s *Server) loadClientCAs() (*x509.CertPool, error) {
	if s.config.TLS.CAFile == "" {
		return nil, nil
	}

	caCert, err := os.ReadFile(s.config.TLS.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate %s: %w", s.config.TLS.CAFile, err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", s.config.TLS.CAFile)
	}
	s.logger.Info("client CA certificate loaded", zap.String("ca_file", s.config.TLS.CAFile))
	return caCertPool, nil
}

// buildStandardTLSConfig creates TLS config for admin, TMF, and GraphQL servers.
// No client certificate is required (NoClientCert).
func (s *Server) buildStandardTLSConfig() (*tls.Config, error) {
	tlsCfg := s.buildBaseTLSConfig()
	tlsCfg.ClientAuth = tls.NoClientCert

	s.logger.Info("standard TLS configuration built (no client auth)",
		zap.String("min_version", s.config.TLS.MinVersion),
	)

	return tlsCfg, nil
}

// buildO2TLSConfig creates TLS config for the O2 mTLS server.
// Client certificate authentication is configured based on the config's ClientAuth setting.
func (s *Server) buildO2TLSConfig() (*tls.Config, error) {
	tlsCfg := s.buildBaseTLSConfig()

	// Configure client certificate authentication from config
	switch s.config.TLS.ClientAuth {
	case "require-and-verify":
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	case "require":
		tlsCfg.ClientAuth = tls.RequireAnyClientCert
	case "verify":
		tlsCfg.ClientAuth = tls.VerifyClientCertIfGiven
	case "request":
		tlsCfg.ClientAuth = tls.RequestClientCert
	case "none", "":
		tlsCfg.ClientAuth = tls.NoClientCert
	}

	// Validate that CA file is provided when client verification is required
	if tlsCfg.ClientAuth == tls.RequireAndVerifyClientCert ||
		tlsCfg.ClientAuth == tls.VerifyClientCertIfGiven {
		if s.config.TLS.CAFile == "" {
			return nil, fmt.Errorf("CA certificate file required when client_auth is %q", s.config.TLS.ClientAuth)
		}
	}

	// Load CA certificate for client verification
	caCertPool, err := s.loadClientCAs()
	if err != nil {
		return nil, err
	}
	if caCertPool != nil {
		tlsCfg.ClientCAs = caCertPool
	}

	s.logger.Info("O2 mTLS configuration built",
		zap.String("min_version", s.config.TLS.MinVersion),
		zap.String("client_auth", s.config.TLS.ClientAuth),
	)

	return tlsCfg, nil
}

// buildTLSConfig is kept for backward compatibility with tests.
// It delegates to buildO2TLSConfig which preserves the original behavior.
func (s *Server) buildTLSConfig() (*tls.Config, error) {
	return s.buildO2TLSConfig()
}

// Shutdown gracefully shuts down the HTTP server.
// It waits for active requests to complete or until the shutdown timeout expires.
// It also stops SMO health checks and closes the SMO registry.
// This method is safe to call multiple times - only the first call will execute.
//
// Returns an error if the shutdown fails.
func (s *Server) Shutdown() error {
	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(
		context.Background(),
		s.config.Server.ShutdownTimeout,
	)
	defer cancel()

	return s.shutdownWithContext(ctx)
}

// shutdownWithContext performs the actual shutdown logic using the provided context.
// This is the internal implementation that both Shutdown() and ShutdownWithContext() delegate to.
func (s *Server) shutdownWithContext(ctx context.Context) error {
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

		// Shutdown all HTTP servers, collecting all errors
		servers := map[string]*http.Server{
			"admin":   s.httpServer,
			"o2":      s.o2Server,
			"tmf":     s.tmfServer,
			"graphql": s.graphqlServer,
		}

		var shutdownErrs []error
		for name, srv := range servers {
			if srv == nil {
				continue
			}
			if err := srv.Shutdown(ctx); err != nil {
				s.logger.Error("error during shutdown",
					zap.String("server", name),
					zap.Error(err),
				)
				shutdownErrs = append(shutdownErrs, fmt.Errorf("%s: %w", name, err))
			}
		}

		if len(shutdownErrs) > 0 {
			shutdownErr = errors.Join(shutdownErrs...)
		}

		s.logger.Info("all servers shutdown complete")
	})

	return shutdownErr
}

// Router returns the admin Gin router (backward compatible).
// This is useful for testing and adding custom routes.
func (s *Server) Router() *gin.Engine {
	return s.adminRouter
}

// O2Router returns the O2 mTLS Gin router.
func (s *Server) O2Router() *gin.Engine {
	return s.o2Router
}

// TMFRouter returns the TMForum Gin router.
func (s *Server) TMFRouter() *gin.Engine {
	return s.tmfRouter
}

// GraphQLRouter returns the GraphQL Gin router.
func (s *Server) GraphQLRouter() *gin.Engine {
	return s.graphqlRouter
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

	// Initialize TMForum handler with the static adapter (may be nil for dynamic routing).
	// When dynamic routing is active, the handler's getActiveAdapter(c) method resolves
	// the adapter per-request from gin context, falling back to the static adapter.
	hubStore := storage.NewInMemoryHubStore()
	s.tmfHandler = handlers.NewTMForumHandler(s.adapter, s.dmsRegistry, hubStore, s.logger)
	if s.adapter != nil {
		s.logger.Info("TMForum API initialized with static adapter")
	} else {
		s.logger.Info("TMForum API initialized with dynamic routing (adapter resolved per-request)")
	}
}

// DMSRegistry returns the DMS adapter registry.
func (s *Server) DMSRegistry() *dmsregistry.Registry {
	return s.dmsRegistry
}

// SetAdapterRegistry configures the dynamic backend adapter registry and backend store.
// When set, O2-IMS API handlers will resolve adapters per-tenant using backend access records
// instead of always using the static adapter.
func (s *Server) SetAdapterRegistry(registry *backend.AdapterRegistry, store backend.Store) {
	s.adapterRegistry = registry
	s.backendStore = store
	s.logger.Info("adapter registry configured",
		zap.Int("adapter_count", registry.Count()),
	)
}

// SetupBackendAdmin registers the backend admin API routes on the server.
// The handler manages backend instances, DMS links, and tenant access configuration.
// Routes are registered under /admin/infrastructure to consolidate all admin endpoints.
func (s *Server) SetupBackendAdmin(handler *handlers.BackendHandler) {
	infra := s.adminRouter.Group("/admin/infrastructure")
	handler.RegisterRoutes(infra)
	s.logger.Info("backend admin API routes registered")
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

// SetupAuth configures multi-tenancy and RBAC for the server.
// It sets up authentication middleware, authorization checks, and tenant/user/role management routes.
// This must be called after creating the server and before starting it.
// If authStore is nil, this method is a no-op (multi-tenancy disabled).
func (s *Server) SetupAuth(authStore AuthStore, authMw AuthMiddleware) {
	if authStore == nil || authMw == nil {
		s.logger.Info("multi-tenancy is disabled, skipping auth setup")
		return
	}

	s.AuthStore = authStore
	s.authMw = authMw

	// Register auth store health check.
	if s.healthCheck != nil {
		s.healthCheck.RegisterHealthCheck("auth_store", func(ctx context.Context) error {
			return s.AuthStore.Ping(ctx)
		})
		s.healthCheck.RegisterReadinessCheck("auth_store", func(ctx context.Context) error {
			return s.AuthStore.Ping(ctx)
		})
	}

	s.logger.Info("multi-tenancy and RBAC enabled")
}

// SetupCertLifecycle registers certificate lifecycle admin API routes on the server.
// The handler provides endpoints for querying certificate metadata and monitoring stats.
// Routes are registered under /admin/certificates.
func (s *Server) SetupCertLifecycle(handler *certlifecycle.Handler) {
	admin := s.adminRouter.Group("/admin")
	handler.RegisterRoutes(admin)
	s.logger.Info("certificate lifecycle admin routes registered")
}

// setupTenantRateLimiter configures per-tenant rate limiting based on tenant quota settings.
// This must be called after SetupAuthRoutes since it requires fullAuthStore.
func (s *Server) setupTenantRateLimiter() {
	if s.fullAuthStore == nil {
		return
	}

	redisStore, ok := s.store.(*storage.RedisStore)
	if !ok {
		s.logger.Warn("per-tenant rate limiting requires RedisStore, disabled")
		return
	}

	rlStore := NewRedisRateLimitStore(redisStore.Client)

	getLimit := func(ctx context.Context, tenantID string) int {
		tenant, err := s.fullAuthStore.GetTenant(ctx, tenantID)
		if err != nil {
			return defaultRateLimit
		}
		if tenant.Quota.MaxRequestsPerMinute > 0 {
			return tenant.Quota.MaxRequestsPerMinute
		}
		return defaultRateLimit
	}

	rl := NewRateLimiter(rlStore, s.logger, getLimit)
	// Apply per-tenant rate limiting to all routers
	s.adminRouter.Use(rl.Middleware())
	s.o2Router.Use(rl.Middleware())
	s.tmfRouter.Use(rl.Middleware())
	s.graphqlRouter.Use(rl.Middleware())
	s.logger.Info("per-tenant rate limiting enabled")
}

// AuthStore returns the authentication store interface.
// Returns nil if auth is not configured.
// This method returns an interface by design (registry pattern).

// RecoveryMiddleware recovers from panics and logs the error.
func (s *Server) RecoveryMiddleware() gin.HandlerFunc {
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

// LoggingMiddleware logs HTTP requests and responses.
func (s *Server) LoggingMiddleware() gin.HandlerFunc {
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

// MetricsMiddleware collects Prometheus metrics for HTTP requests.
func (s *Server) MetricsMiddleware() gin.HandlerFunc {
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
//
// Security model (issue #495):
//
//   - An empty AllowedOrigins list means "deny all cross-origin requests",
//     NOT "allow all". A common prior bug was to reflect any Origin alongside
//     Access-Control-Allow-Credentials: true, which completely defeats CORS.
//   - Wildcard "*" is honored but MUST NOT be combined with credentials:
//     when "*" is listed, the middleware emits Access-Control-Allow-Origin: *
//     and omits the credentials header.
//   - An exact origin match mirrors the request Origin and allows credentials.
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowedOrigins := s.config.Security.AllowedOrigins

		// Empty list: deny-all. Do not emit any CORS headers, but still
		// shortcut preflight so downstream handlers don't see the OPTIONS.
		if len(allowedOrigins) == 0 {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.Next()
			return
		}

		wildcard := false
		exactMatch := false
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" {
				wildcard = true
				continue
			}
			if allowedOrigin == origin && origin != "" {
				exactMatch = true
				break
			}
		}

		switch {
		case exactMatch:
			// Exact origin allow-list match. Safe to combine with credentials.
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Headers",
				JoinStrings(s.config.Security.AllowedHeaders, ", "))
			c.Writer.Header().Set("Access-Control-Allow-Methods",
				JoinStrings(s.config.Security.AllowedMethods, ", "))
		case wildcard:
			// Wildcard: never reflect the request origin with credentials=true.
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			c.Writer.Header().Set("Access-Control-Allow-Headers",
				JoinStrings(s.config.Security.AllowedHeaders, ", "))
			c.Writer.Header().Set("Access-Control-Allow-Methods",
				JoinStrings(s.config.Security.AllowedMethods, ", "))
		default:
			// Origin not allow-listed: emit no CORS headers at all.
		}

		// Handle preflight requests
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware implements Redis-based distributed rate limiting for HTTP requests.
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	// Get Redis client from store
	redisStore, ok := s.store.(*storage.RedisStore)
	if !ok {
		s.logger.Warn("rate limiting requires RedisStore, disabled")
		return func(c *gin.Context) {
			c.Next()
		}
	}

	// Convert config types to middleware types
	rateLimitConfig := &middleware.RateLimitConfig{
		Enabled:     s.config.Security.RateLimitEnabled,
		RedisClient: redisStore.Client,
		PerTenant: middleware.TenantLimitConfig{
			RequestsPerSecond: s.config.Security.RateLimit.PerTenant.RequestsPerSecond,
			BurstSize:         s.config.Security.RateLimit.PerTenant.BurstSize,
		},
		Global: middleware.GlobalLimitConfig{
			RequestsPerSecond:     s.config.Security.RateLimit.Global.RequestsPerSecond,
			MaxConcurrentRequests: s.config.Security.RateLimit.Global.MaxConcurrentRequests,
		},
		DefaultFailMode: middleware.FailMode(s.config.Security.RateLimit.FailMode),
	}

	// Convert endpoint configs
	for _, ep := range s.config.Security.RateLimit.PerEndpoint {
		rateLimitConfig.PerEndpoint = append(rateLimitConfig.PerEndpoint, middleware.EndpointLimitConfig{
			Path:              ep.Path,
			Method:            ep.Method,
			RequestsPerSecond: ep.RequestsPerSecond,
			BurstSize:         ep.BurstSize,
			FailMode:          middleware.FailMode(ep.FailMode),
		})
	}

	// Create rate limiter
	rateLimiter, err := middleware.NewRateLimiter(rateLimitConfig, s.logger)
	if err != nil {
		s.logger.Error("failed to create rate limiter, rate limiting disabled",
			zap.Error(err),
		)
		// Return pass-through middleware if rate limiter creation fails
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return rateLimiter.Middleware()
}

// resourceRateLimitMiddleware implements resource-type-specific rate limiting for O2-IMS operations.
func (s *Server) resourceRateLimitMiddleware() gin.HandlerFunc {
	// Get Redis client from store
	redisStore, ok := s.store.(*storage.RedisStore)
	if !ok {
		s.logger.Warn("resource rate limiting requires RedisStore, disabled")
		return func(c *gin.Context) {
			c.Next()
		}
	}

	// Convert config types to middleware types
	resourceConfig := &middleware.ResourceRateLimitConfig{
		Enabled:     s.config.Security.RateLimit.PerResource.Enabled,
		RedisClient: redisStore.Client,
		DeploymentManagers: middleware.ResourceTypeLimits{
			ReadsPerMinute:  s.config.Security.RateLimit.PerResource.DeploymentManagers.ReadsPerMinute,
			WritesPerMinute: s.config.Security.RateLimit.PerResource.DeploymentManagers.WritesPerMinute,
			ListPageSizeMax: s.config.Security.RateLimit.PerResource.DeploymentManagers.ListPageSizeMax,
		},
		ResourcePools: middleware.ResourceTypeLimits{
			ReadsPerMinute:  s.config.Security.RateLimit.PerResource.ResourcePools.ReadsPerMinute,
			WritesPerMinute: s.config.Security.RateLimit.PerResource.ResourcePools.WritesPerMinute,
			ListPageSizeMax: s.config.Security.RateLimit.PerResource.ResourcePools.ListPageSizeMax,
		},
		Resources: middleware.ResourceTypeLimits{
			ReadsPerMinute:  s.config.Security.RateLimit.PerResource.Resources.ReadsPerMinute,
			WritesPerMinute: s.config.Security.RateLimit.PerResource.Resources.WritesPerMinute,
			ListPageSizeMax: s.config.Security.RateLimit.PerResource.Resources.ListPageSizeMax,
		},
		ResourceTypes: middleware.ResourceTypeLimits{
			ReadsPerMinute:  s.config.Security.RateLimit.PerResource.ResourceTypes.ReadsPerMinute,
			WritesPerMinute: s.config.Security.RateLimit.PerResource.ResourceTypes.WritesPerMinute,
			ListPageSizeMax: s.config.Security.RateLimit.PerResource.ResourceTypes.ListPageSizeMax,
		},
		Subscriptions: middleware.SubscriptionLimits{
			CreatesPerHour: s.config.Security.RateLimit.PerResource.Subscriptions.CreatesPerHour,
			MaxActive:      s.config.Security.RateLimit.PerResource.Subscriptions.MaxActive,
			ReadsPerMinute: s.config.Security.RateLimit.PerResource.Subscriptions.ReadsPerMinute,
		},
	}

	// Create resource rate limiter
	resourceRateLimiter, err := middleware.NewResourceRateLimiter(resourceConfig, s.logger)
	if err != nil {
		s.logger.Error("failed to create resource rate limiter, resource rate limiting disabled",
			zap.Error(err),
		)
		// Return pass-through middleware if rate limiter creation fails
		return func(c *gin.Context) {
			c.Next()
		}
	}

	s.logger.Info("resource rate limiting enabled",
		zap.Bool("deployment_managers", resourceConfig.DeploymentManagers.ReadsPerMinute > 0),
		zap.Bool("resource_pools", resourceConfig.ResourcePools.ReadsPerMinute > 0),
		zap.Bool("resources", resourceConfig.Resources.ReadsPerMinute > 0),
		zap.Bool("subscriptions", resourceConfig.Subscriptions.CreatesPerHour > 0),
	)

	return resourceRateLimiter.Middleware()
}

// JoinStrings joins a slice of strings with the given separator.
func JoinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// SetOpenAPISpec sets the OpenAPI specification content.
// This is primarily used for testing.
func (s *Server) SetOpenAPISpec(spec []byte) {
	s.openAPISpec = spec
}

// GetOpenAPISpec returns the OpenAPI specification content.
// This is primarily used for testing.
func (s *Server) GetOpenAPISpec() []byte {
	return s.openAPISpec
}

// ShutdownWithContext gracefully shuts down the HTTP server using the provided context.
// It waits for active requests to complete or until the context is canceled.
// This is a wrapper around Shutdown() that respects the provided context.
func (s *Server) ShutdownWithContext(ctx context.Context) error {
	// Create a channel to signal when shutdown completes
	done := make(chan error, 1)

	// Run shutdown in a goroutine with the provided context
	go func(shutdownCtx context.Context) {
		done <- s.shutdownWithContext(shutdownCtx)
	}(ctx)

	// Wait for either shutdown to complete or context to be canceled
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("server shutdown context cancelled: %w", ctx.Err())
	}
}
