// Package main is the entry point for the O2-IMS Gateway application.
// It initializes and starts the production-ready HTTP gateway server that translates
// O2-IMS API requests to Kubernetes API calls.
//
// The application performs the following initialization sequence:
//  1. Load configuration from config file and environment variables
//  2. Initialize structured logging with zap
//  3. Connect to Redis for subscription storage and caching
//  4. Initialize Kubernetes adapter for backend operations
//  5. Configure HTTP server with routes and middleware
//  6. Register health checks for observability
//  7. Start HTTP server with graceful shutdown support
//
// Graceful shutdown is triggered by SIGINT (Ctrl+C) or SIGTERM signals.
//
// Example usage:
//
//	# Start with default config
//	./gateway
//
//	# Start with custom config file
//	./gateway --config=/etc/netweave/config.yaml
//
//	# Start with environment variable overrides
//	export NETWEAVE_SERVER_PORT=9090
//	export NETWEAVE_REDIS_ADDRESSES=redis.example.com:6379
//	./gateway
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
)

const (
	// Version is the application version (set via build flags).
	Version = "1.0.0"

	// ServiceName is the name of this service.
	ServiceName = "netweave-gateway"
)

var (
	// Command-line flags.
	configPath  = flag.String("config", config.DefaultConfigPath, "Path to configuration file")
	showVersion = flag.Bool("version", false, "Show version information and exit")
)

func main() {
	// Parse command-line flags
	flag.Parse()

	// Show version and exit if requested
	if *showVersion {
		if _, err := fmt.Fprintf(os.Stdout, "%s version %s\n", ServiceName, Version); err != nil {
			// Error writing to stdout is generally fatal
			panic(err)
		}
		os.Exit(0)
	}

	// Run the application
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main application logic.
// It returns an error if any critical initialization or runtime error occurs.
func run() error {
	// Step 1: Load configuration
	cfg, err := loadConfiguration(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Step 2: Initialize structured logger
	logger, err := setupLogger(cfg)
	if err != nil {
		return err
	}

	logger.Info("O2-IMS Gateway starting",
		zap.String("version", Version),
		zap.String("service", ServiceName),
		zap.String("environment", cfg.Environment),
	)

	// Step 3-6: Initialize components
	components, err := initializeComponents(cfg, logger)
	if err != nil {
		return err
	}
	defer components.Close(logger)

	// Step 7: Setup and run server with graceful shutdown
	return runServerWithShutdown(cfg, logger, components)
}

// applicationComponents holds all initialized application components.
type applicationComponents struct {
	store         *storage.RedisStore
	k8sAdapter    *kubernetes.KubernetesAdapter
	healthChecker *observability.HealthChecker
	server        *server.Server
}

// Close closes all components gracefully.
func (c *applicationComponents) Close(logger *zap.Logger) {
	if c.k8sAdapter != nil {
		if err := c.k8sAdapter.Close(); err != nil {
			logger.Warn("failed to close Kubernetes adapter", zap.Error(err))
		}
	}
	if c.store != nil {
		if err := c.store.Close(); err != nil {
			logger.Warn("failed to close Redis connection", zap.Error(err))
		}
	}
}

// setupLogger initializes and configures the logger with proper cleanup.
func setupLogger(cfg *config.Config) (*zap.Logger, error) {
	logger, err := initializeLogger(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Setup deferred sync with error handling
	go func() {
		if syncErr := logger.Sync(); syncErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to sync logger: %v\n", syncErr)
		}
	}()

	return logger, nil
}

// initializeComponents initializes all application components.
func initializeComponents(cfg *config.Config, logger *zap.Logger) (*applicationComponents, error) {
	// Initialize Redis storage
	store, err := initializeRedisStorage(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize Redis storage", zap.Error(err))
		return nil, fmt.Errorf("failed to initialize Redis storage: %w", err)
	}

	logger.Info("Redis storage initialized successfully",
		zap.String("mode", cfg.Redis.Mode),
		zap.Strings("addresses", cfg.Redis.Addresses),
	)

	// Initialize Kubernetes adapter
	k8sAdapter, err := initializeKubernetesAdapter(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize Kubernetes adapter", zap.Error(err))
		if closeErr := store.Close(); closeErr != nil {
			logger.Warn("failed to close Redis connection during cleanup", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("failed to initialize Kubernetes adapter: %w", err)
	}

	logger.Info("Kubernetes adapter initialized successfully",
		zap.String("adapter", k8sAdapter.Name()),
		zap.String("version", k8sAdapter.Version()),
	)

	// Initialize health checker
	healthChecker := initializeHealthChecker(store, k8sAdapter, logger)
	logger.Info("health checker initialized")

	// Create and configure HTTP server
	srv := server.New(cfg, logger, k8sAdapter, store)
	srv.SetHealthChecker(healthChecker)
	logger.Info("HTTP server created",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
		zap.String("mode", cfg.Server.GinMode),
	)

	// Load OpenAPI specification for documentation endpoints
	// This is fail-fast - server won't start without a valid OpenAPI spec
	spec, err := loadOpenAPISpec(logger)
	if err != nil {
		logger.Error("failed to load OpenAPI specification", zap.Error(err))
		return nil, fmt.Errorf("failed to load OpenAPI specification: %w", err)
	}
	srv.SetOpenAPISpec(spec)
	logger.Info("OpenAPI specification loaded",
		zap.Int("size", len(spec)),
	)

	return &applicationComponents{
		store:         store,
		k8sAdapter:    k8sAdapter,
		healthChecker: healthChecker,
		server:        srv,
	}, nil
}

// runServerWithShutdown starts the server and handles graceful shutdown.
func runServerWithShutdown(cfg *config.Config, logger *zap.Logger, components *applicationComponents) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("starting HTTP server",
			zap.String("address", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)),
			zap.Bool("tls_enabled", cfg.TLS.Enabled),
		)
		if err := components.server.Start(); err != nil {
			serverErrors <- err
		}
	}()

	// Wait for shutdown signal or error
	return handleShutdown(ctx, cancel, components.server, cfg, logger, shutdown, serverErrors)
}

// handleShutdown waits for shutdown signals or errors and performs graceful shutdown.
func handleShutdown(
	ctx context.Context,
	cancel context.CancelFunc,
	srv *server.Server,
	cfg *config.Config,
	logger *zap.Logger,
	shutdown chan os.Signal,
	serverErrors chan error,
) error {
	select {
	case err := <-serverErrors:
		logger.Error("server error", zap.Error(err))
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
		cancel()
		return gracefulShutdown(ctx, srv, cfg, logger)
	}
}

// loadConfiguration loads and validates the application configuration.
func loadConfiguration(configPath string) (*config.Config, error) {
	// Load configuration from file and environment variables
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// initializeLogger creates a structured logger based on configuration.
func initializeLogger(cfg *config.Config) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	// Determine log configuration based on settings
	if cfg.Observability.Logging.Development {
		// Development mode - console output with colors
		loggerCfg := zap.NewDevelopmentConfig()
		loggerCfg.Level = parseLogLevel(cfg.Observability.Logging.Level)
		loggerCfg.OutputPaths = cfg.Observability.Logging.OutputPaths
		loggerCfg.ErrorOutputPaths = cfg.Observability.Logging.ErrorOutputPaths
		logger, err = loggerCfg.Build()
	} else {
		// Production mode - JSON output
		loggerCfg := zap.NewProductionConfig()
		loggerCfg.Level = parseLogLevel(cfg.Observability.Logging.Level)
		loggerCfg.OutputPaths = cfg.Observability.Logging.OutputPaths
		loggerCfg.ErrorOutputPaths = cfg.Observability.Logging.ErrorOutputPaths
		loggerCfg.DisableCaller = !cfg.Observability.Logging.EnableCaller
		loggerCfg.DisableStacktrace = !cfg.Observability.Logging.EnableStacktrace

		// Configure encoding
		if cfg.Observability.Logging.Format == "console" {
			loggerCfg.Encoding = "console"
		} else {
			loggerCfg.Encoding = "json"
		}

		logger, err = loggerCfg.Build()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

// parseLogLevel converts a log level string to zapcore.Level.
func parseLogLevel(level string) zap.AtomicLevel {
	switch level {
	case "debug":
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "fatal":
		return zap.NewAtomicLevelAt(zap.FatalLevel)
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	}
}

// initializeRedisStorage creates and initializes Redis storage.
func initializeRedisStorage(cfg *config.Config, logger *zap.Logger) (*storage.RedisStore, error) {
	password, sentinelPassword, err := getRedisPasswords(cfg, logger)
	if err != nil {
		return nil, err
	}

	redisCfg := buildRedisConfig(cfg, password, sentinelPassword)
	if err := configureRedisMode(redisCfg, cfg, logger); err != nil {
		return nil, err
	}

	logSecurityWarnings(cfg, logger)

	store := storage.NewRedisStore(redisCfg)
	if err := verifyRedisConnectivity(store); err != nil {
		return nil, err
	}

	logger.Info("Redis connectivity verified")
	return store, nil
}

// getRedisPasswords retrieves Redis and Sentinel passwords and logs deprecation warnings.
func getRedisPasswords(cfg *config.Config, logger *zap.Logger) (redisPassword, sentinelPassword string, err error) {
	// Get Redis password
	redisPassword, err = cfg.Redis.GetPassword()
	if err != nil {
		return "", "", fmt.Errorf("failed to get Redis password: %w", err)
	}

	// Log warning if using deprecated direct password configuration
	if cfg.Redis.Password != "" && cfg.Redis.PasswordEnvVar == "" && cfg.Redis.PasswordFile == "" {
		logger.Warn("SECURITY WARNING: Redis password is stored in plaintext configuration. "+
			"This is deprecated and insecure. Use password_env_var or password_file instead.",
			zap.Bool("deprecated_password_field", true))
	}

	// Get Sentinel password (only relevant for Sentinel mode)
	if cfg.Redis.Mode == "sentinel" {
		sentinelPassword, err = cfg.Redis.GetSentinelPassword()
		if err != nil {
			return "", "", fmt.Errorf("failed to get Sentinel password: %w", err)
		}

		// Log warning if using deprecated direct sentinel password configuration
		if cfg.Redis.SentinelPassword != "" && cfg.Redis.SentinelPasswordEnvVar == "" && cfg.Redis.SentinelPasswordFile == "" {
			logger.Warn("SECURITY WARNING: Sentinel password is stored in plaintext configuration. "+
				"This is deprecated and insecure. Use sentinel_password_env_var or sentinel_password_file instead.",
				zap.Bool("deprecated_sentinel_password_field", true))
		}
	}

	return redisPassword, sentinelPassword, nil
}

// buildRedisConfig creates storage.RedisConfig from config.Config.
func buildRedisConfig(cfg *config.Config, password, sentinelPassword string) *storage.RedisConfig {
	return &storage.RedisConfig{
		DB:                     cfg.Redis.DB,
		Password:               password,
		SentinelPassword:       sentinelPassword,
		MaxRetries:             cfg.Redis.MaxRetries,
		DialTimeout:            cfg.Redis.DialTimeout,
		ReadTimeout:            cfg.Redis.ReadTimeout,
		WriteTimeout:           cfg.Redis.WriteTimeout,
		PoolSize:               cfg.Redis.PoolSize,
		AllowInsecureCallbacks: cfg.Security.AllowInsecureCallbacks,
	}
}

// configureRedisMode sets up Redis mode (standalone/sentinel/cluster).
func configureRedisMode(redisCfg *storage.RedisConfig, cfg *config.Config, logger *zap.Logger) error {
	switch cfg.Redis.Mode {
	case "sentinel":
		redisCfg.UseSentinel = true
		redisCfg.SentinelAddrs = cfg.Redis.Addresses
		redisCfg.MasterName = cfg.Redis.MasterName
		logger.Info("configuring Redis in Sentinel mode",
			zap.Strings("sentinel_addresses", cfg.Redis.Addresses),
			zap.String("master_name", cfg.Redis.MasterName),
		)

	case "cluster":
		logger.Warn("Redis cluster mode not yet fully supported, falling back to standalone",
			zap.String("mode", cfg.Redis.Mode),
		)
		fallthrough

	case "standalone":
		redisCfg.UseSentinel = false
		if len(cfg.Redis.Addresses) > 0 {
			redisCfg.Addr = cfg.Redis.Addresses[0]
		} else {
			redisCfg.Addr = "localhost:6379"
		}
		logger.Info("configuring Redis in standalone mode",
			zap.String("address", redisCfg.Addr),
		)

	default:
		return fmt.Errorf("unsupported Redis mode: %s", cfg.Redis.Mode)
	}

	return nil
}

// logSecurityWarnings logs security-related configuration warnings.
func logSecurityWarnings(cfg *config.Config, logger *zap.Logger) {
	if cfg.Security.AllowInsecureCallbacks {
		logger.Warn("SECURITY WARNING: HTTP (non-HTTPS) webhook callbacks are allowed. "+
			"This should ONLY be used in development/testing environments. "+
			"Production deployments MUST enforce HTTPS to prevent man-in-the-middle attacks "+
			"and ensure data confidentiality.",
			zap.Bool("allow_insecure_callbacks", true))
	}
}

// verifyRedisConnectivity tests Redis connection.
func verifyRedisConnectivity(store *storage.RedisStore) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.Ping(ctx); err != nil {
		return fmt.Errorf("redis connectivity check failed: %w", err)
	}

	return nil
}

// initializeKubernetesAdapter creates and initializes the Kubernetes adapter.
func initializeKubernetesAdapter(cfg *config.Config, logger *zap.Logger) (*kubernetes.KubernetesAdapter, error) {
	// Build Kubernetes adapter configuration
	k8sCfg := &kubernetes.Config{
		Kubeconfig:          cfg.Kubernetes.ConfigPath,
		OCloudID:            "default-ocloud",
		DeploymentManagerID: "netweave-k8s-dm",
		Namespace:           cfg.Kubernetes.Namespace,
		Logger:              logger,
	}

	// Set default namespace if not specified
	if k8sCfg.Namespace == "" {
		k8sCfg.Namespace = "o2ims-system"
	}

	// Create Kubernetes adapter
	adapter, err := kubernetes.New(k8sCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes adapter: %w", err)
	}

	// Verify Kubernetes connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := adapter.Health(ctx); err != nil {
		return nil, fmt.Errorf(
			"kubernetes connectivity check failed: %w",
			err,
		)
	}

	logger.Info("Kubernetes connectivity verified")
	return adapter, nil
}

// initializeHealthChecker creates and configures the health checker.
func initializeHealthChecker(
	store *storage.RedisStore,
	adapter *kubernetes.KubernetesAdapter,
	logger *zap.Logger,
) *observability.HealthChecker {
	healthChecker := observability.NewHealthChecker(Version)

	// Set health check timeout
	healthChecker.SetTimeout(5 * time.Second)

	// Register Redis health check
	healthChecker.RegisterHealthCheck("redis", observability.RedisHealthCheck(func(ctx context.Context) error {
		return store.Ping(ctx)
	}))

	// Register Kubernetes health check
	healthChecker.RegisterHealthCheck(
		"kubernetes",
		observability.KubernetesHealthCheck(func(ctx context.Context) error {
			return adapter.Health(ctx)
		}),
	)

	// Register the same checks for readiness
	healthChecker.RegisterReadinessCheck("redis",
		observability.RedisHealthCheck(func(ctx context.Context) error {
			return store.Ping(ctx)
		}))

	healthChecker.RegisterReadinessCheck("kubernetes",
		observability.KubernetesHealthCheck(func(ctx context.Context) error {
			return adapter.Health(ctx)
		}))

	logger.Info("health checks registered",
		zap.Int("health_checks", 2),
		zap.Int("readiness_checks", 2),
	)

	return healthChecker
}

// gracefulShutdown performs graceful shutdown of the application.
func gracefulShutdown(ctx context.Context, srv *server.Server, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("initiating graceful shutdown",
		zap.Duration("timeout", cfg.Server.ShutdownTimeout),
	)

	// Create shutdown context with timeout derived from parent context
	shutdownCtx, cancel := context.WithTimeout(
		ctx,
		cfg.Server.ShutdownTimeout,
	)
	defer cancel()

	// Channel to signal shutdown completion
	shutdownComplete := make(chan error, 1)

	// Perform shutdown in a goroutine with the shutdown context
	go func() {
		// Shutdown HTTP server using the shutdown context
		if err := srv.ShutdownWithContext(shutdownCtx); err != nil {
			shutdownComplete <- fmt.Errorf("server shutdown failed: %w", err)
			return
		}

		shutdownComplete <- nil
	}()

	// Wait for shutdown to complete or timeout
	select {
	case err := <-shutdownComplete:
		if err != nil {
			logger.Error("graceful shutdown failed", zap.Error(err))
			return err
		}
		logger.Info("graceful shutdown completed successfully")
		return nil

	case <-shutdownCtx.Done():
		logger.Warn("graceful shutdown timed out, forcing shutdown")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// loadOpenAPISpec loads the OpenAPI specification from the api/openapi directory.
// The spec is loaded from multiple possible locations to support different deployment scenarios.
// Returns the spec data or an error if not found.
func loadOpenAPISpec(logger *zap.Logger) ([]byte, error) {
	// Possible locations for the OpenAPI spec file (hardcoded paths, not user input)
	// G304: These are predefined trusted paths, not user-controllable input
	specPaths := []string{
		"api/openapi/o2ims.yaml",      // Local development
		"./api/openapi/o2ims.yaml",    // Explicit local path
		"/etc/netweave/openapi.yaml",  // Production deployment
		"/app/api/openapi/o2ims.yaml", // Container deployment
	}

	for _, path := range specPaths {
		// G304: path is from hardcoded list above, not user input - safe from path traversal
		data, err := os.ReadFile(path)
		if err == nil {
			logger.Debug("loaded OpenAPI spec",
				zap.String("path", path),
				zap.Int("size", len(data)),
			)
			return data, nil
		}
	}

	return nil, fmt.Errorf("OpenAPI specification not found in any of the expected locations")
}
