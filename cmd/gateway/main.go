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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/certlifecycle"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/dms/adapters/helm"
	dmsmock "github.com/piwi3910/netweave/internal/dms/adapters/mock"
	dmsregistry "github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/encryption"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/keycloak"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
	vaultpkg "github.com/piwi3910/netweave/internal/vault"
)

const (
	redisModeSentinel = "sentinel"
	adapterTypeMock   = "mock"
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
	// Close errors are logged but not returned since we're shutting down anyway.
	// The Close method still returns aggregated errors for debugging.
	defer func() {
		if closeErr := components.Close(logger); closeErr != nil {
			logger.Error("failed to close components", zap.Error(closeErr))
		}
	}()

	// Step 7: Setup and run server with graceful shutdown
	return runServerWithShutdown(cfg, logger, components)
}

// ApplicationComponents holds all initialized application components.
type ApplicationComponents struct {
	redisStore      *storage.RedisStore          // Always needed for rate limiting, events, pub/sub
	store           storage.Store                // Subscription store (redis, postgres, or dual)
	pgDB            *database.DB                 // PostgreSQL connection (nil if storage_mode="redis")
	adapterRegistry *backend.AdapterRegistry     // Dynamic backend adapter registry
	healthChecker   *observability.HealthChecker // Health and readiness checks
	server          *server.Server
	authStore       server.AuthStore
	certManager     *certlifecycle.Manager // Certificate lifecycle manager (nil if disabled)
}

// NewApplicationComponentsForTest creates an ApplicationComponents instance for testing.
// This is exported to allow test packages to create instances with controlled values.
func NewApplicationComponentsForTest(authStore server.AuthStore) *ApplicationComponents {
	return &ApplicationComponents{
		authStore: authStore,
	}
}

// NewApplicationComponentsForTestFull creates an ApplicationComponents instance for testing
// with all fields configurable.
func NewApplicationComponentsForTestFull(
	authStore server.AuthStore,
	store storage.Store,
	pgDB *database.DB,
) *ApplicationComponents {
	return &ApplicationComponents{
		authStore: authStore,
		store:     store,
		pgDB:      pgDB,
	}
}

// Close closes all components gracefully and returns any errors encountered.
// All components are closed even if earlier close operations fail.
// Errors are aggregated using errors.Join and logged as warnings.
// This ensures cleanup continues for all resources while still surfacing issues
// for debugging connection leaks or other cleanup problems.
func (c *ApplicationComponents) Close(logger *zap.Logger) error {
	var closeErrors []error

	if c.certManager != nil {
		if err := c.certManager.Stop(); err != nil {
			logger.Warn("failed to stop certificate lifecycle manager", zap.Error(err))
			closeErrors = append(closeErrors, fmt.Errorf("cert manager: %w", err))
		}
	}
	if c.adapterRegistry != nil {
		if err := c.adapterRegistry.Close(); err != nil {
			logger.Warn("failed to close adapter registry", zap.Error(err))
			closeErrors = append(closeErrors, fmt.Errorf("adapter registry: %w", err))
		}
	}
	if c.authStore != nil {
		if err := c.authStore.Close(); err != nil {
			logger.Warn("failed to close auth Redis connection", zap.Error(err))
			closeErrors = append(closeErrors, fmt.Errorf("auth store: %w", err))
		}
	}
	if c.store != nil {
		if err := c.store.Close(); err != nil {
			logger.Warn("failed to close subscription store", zap.Error(err))
			closeErrors = append(closeErrors, fmt.Errorf("subscription store: %w", err))
		}
	}
	if c.redisStore != nil && c.redisStore != c.store {
		if err := c.redisStore.Close(); err != nil {
			logger.Warn("failed to close Redis connection", zap.Error(err))
			closeErrors = append(closeErrors, fmt.Errorf("redis store: %w", err))
		}
	}
	if c.pgDB != nil {
		c.pgDB.Close()
		logger.Info("PostgreSQL connection closed")
	}

	return errors.Join(closeErrors...)
}

// setupLogger initializes and configures the logger with proper cleanup.
func setupLogger(cfg *config.Config) (*zap.Logger, error) {
	logger, err := initializeLogger(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Setup deferred sync with error handling
	defer func() {
		if logger != nil {
			if syncErr := logger.Sync(); syncErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to sync logger: %v\n", syncErr)
			}
		}
	}()

	return logger, nil
}

// cleanupOnError closes resources during error handling.
func cleanupOnError(redisStore *storage.RedisStore, logger *zap.Logger) {
	if redisStore != nil {
		if closeErr := redisStore.Close(); closeErr != nil {
			logger.Warn("failed to close Redis connection during cleanup", zap.Error(closeErr))
		}
	}
}

// cleanupPgOnError closes PostgreSQL resources during error handling.
func cleanupPgOnError(pgDB *database.DB) {
	if pgDB != nil {
		pgDB.Close()
	}
}

// initializePostgres creates and initializes a PostgreSQL connection and runs migrations.
func initializePostgres(cfg *config.Config, logger *zap.Logger) (*database.DB, error) {
	pgPassword := os.Getenv(cfg.Postgres.PasswordEnvVar)
	if pgPassword == "" {
		return nil, fmt.Errorf("environment variable %s not set for PostgreSQL password", cfg.Postgres.PasswordEnvVar)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pgDB, err := database.New(ctx, &cfg.Postgres, pgPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	// Run migrations
	if migrateErr := database.Migrate(ctx, pgDB.Pool); migrateErr != nil {
		pgDB.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", migrateErr)
	}

	logger.Info("PostgreSQL initialized and migrations applied",
		zap.String("host", cfg.Postgres.Host),
		zap.Int("port", cfg.Postgres.Port),
		zap.String("database", cfg.Postgres.Database),
		zap.String("storage_mode", cfg.StorageMode),
	)

	return pgDB, nil
}

// selectSubscriptionStore sets dest to the appropriate subscription store based on storage mode.
func selectSubscriptionStore(
	dest *storage.Store,
	cfg *config.Config,
	redisStore *storage.RedisStore,
	pgDB *database.DB,
	logger *zap.Logger,
) {
	switch cfg.StorageMode {
	case "postgres":
		logger.Info("using PostgreSQL as subscription store")
		*dest = storage.NewPostgresStore(pgDB, cfg.Security.AllowInsecureCallbacks)

	case "dual":
		pgStore := storage.NewPostgresStore(pgDB, cfg.Security.AllowInsecureCallbacks)
		logger.Info("using dual store (primary=redis, secondary=postgres)")
		*dest = storage.NewDualStore(redisStore, pgStore, logger)

	default: // "redis"
		logger.Info("using Redis as subscription store")
		*dest = redisStore
	}
}

// initializeBackendAdmin sets up the backend admin API and dynamic adapter registry if PostgreSQL is available.
// Returns the adapter registry (nil if PostgreSQL or encryption is unavailable).
func initializeBackendAdmin(
	cfg *config.Config,
	srv *server.Server,
	pgDB *database.DB,
	logger *zap.Logger,
) *backend.AdapterRegistry {
	if pgDB == nil {
		return nil
	}

	var enc encryption.Encryptor
	createEncryptor(&enc, cfg, logger)
	if enc == nil {
		logger.Warn("no encryptor available, backend admin API disabled")
		return nil
	}

	backendStore := backend.NewPostgresStore(pgDB.Pool)
	schemaRegistry := backend.NewSchemaRegistry()
	backend.RegisterBuiltinSchemas(schemaRegistry)
	backendHandler := handlers.NewBackendHandler(backendStore, enc, schemaRegistry, logger)
	srv.SetupBackendAdmin(backendHandler)

	logger.Info("backend admin API initialized")

	// Initialize dynamic adapter registry
	factory := backend.NewAdapterFactory()
	registry := backend.NewAdapterRegistry(factory, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loaded, loadErr := registry.LoadFromStore(ctx, backendStore)
	if loadErr != nil {
		logger.Warn("failed to load backend adapters from store, dynamic routing disabled",
			zap.Error(loadErr),
		)
		return nil
	}

	srv.SetAdapterRegistry(registry, backendStore)

	logger.Info("dynamic adapter registry initialized",
		zap.Int("adapters_loaded", loaded),
	)

	return registry
}

// createEncryptor sets dest to an encryption.Encryptor from environment configuration.
// If no encryptor can be created, dest is left unchanged (nil).
func createEncryptor(
	dest *encryption.Encryptor,
	_ *config.Config,
	logger *zap.Logger,
) {
	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultToken := os.Getenv("VAULT_TOKEN")

	if vaultAddr != "" && vaultToken != "" {
		vaultCfg := encryption.VaultConfig{
			Address:   vaultAddr,
			Token:     vaultToken,
			MountPath: "transit",
			KeyName:   "netweave",
		}
		logger.Info("using Vault transit encryptor for backend credentials",
			zap.String("vault_addr", vaultAddr),
		)
		*dest = encryption.NewVaultEncryptor(vaultCfg)

		return
	}

	// Fallback to AES for development
	aesKeyStr := os.Getenv("ENCRYPTION_KEY")
	if aesKeyStr == "" {
		logger.Warn("ENCRYPTION_KEY not set, using zero key for development only")
	}
	aesKey := make([]byte, encryption.AESKeySize)
	copy(aesKey, []byte(aesKeyStr))

	enc, aesErr := encryption.NewAESEncryptor(aesKey)
	if aesErr != nil {
		logger.Warn("failed to create AES encryptor", zap.Error(aesErr))

		return
	}

	logger.Info("using AES encryptor for backend credentials (development mode)")
	*dest = enc
}

// initializeComponents initializes all application components.
func initializeComponents(cfg *config.Config, logger *zap.Logger) (*ApplicationComponents, error) {
	// Initialize Redis storage (always needed for rate limiting, events, pub/sub)
	redisStore, storeErr := initializeRedisStorage(cfg, logger)
	if storeErr != nil {
		logger.Error("failed to initialize Redis storage", zap.Error(storeErr))
		return nil, fmt.Errorf("failed to initialize Redis storage: %w", storeErr)
	}

	logger.Info("Redis storage initialized successfully",
		zap.String("mode", cfg.Redis.Mode),
		zap.Strings("addresses", cfg.Redis.Addresses),
	)

	// Initialize PostgreSQL if storage mode requires it
	var pgDB *database.DB
	if cfg.StorageMode == "postgres" || cfg.StorageMode == "dual" {
		var pgErr error
		pgDB, pgErr = initializePostgres(cfg, logger)
		if pgErr != nil {
			cleanupOnError(redisStore, logger)
			return nil, pgErr
		}
	}

	// Select subscription store based on storage mode
	var subscriptionStore storage.Store
	selectSubscriptionStore(&subscriptionStore, cfg, redisStore, pgDB, logger)

	// Initialize health checker (adapter health checks are registered dynamically)
	healthChecker := initializeHealthChecker(redisStore, pgDB, logger)
	logger.Info("health checker initialized")

	// Initialize auth store and middleware if multi-tenancy is enabled
	var authStore server.AuthStore
	var authMw *auth.Middleware
	if cfg.MultiTenancy.Enabled {
		var initErr error
		authStore, authMw, initErr = InitializeAuth(cfg, logger, pgDB)
		if initErr != nil {
			logger.Error("failed to initialize authentication subsystem")
			cleanupOnError(redisStore, logger)
			cleanupPgOnError(pgDB)
			return nil, fmt.Errorf("failed to initialize auth: %w", initErr)
		}

		logger.Info("authentication store initialized",
			zap.Bool("require_mtls", cfg.MultiTenancy.RequireMTLS),
		)
	}

	// Create and configure HTTP server (no static adapter — adapters are loaded dynamically)
	srv, srvErr := createHTTPServer(cfg, logger, subscriptionStore, authStore, authMw)
	if srvErr != nil {
		return nil, srvErr
	}

	// Set health checker
	srv.SetHealthChecker(healthChecker)

	// Initialize backend admin API and dynamic adapter registry if PostgreSQL is available
	adapterReg := initializeBackendAdmin(cfg, srv, pgDB, logger)

	// Create components structure
	components := &ApplicationComponents{
		redisStore:      redisStore,
		store:           subscriptionStore,
		pgDB:            pgDB,
		adapterRegistry: adapterReg,
		healthChecker:   healthChecker,
		server:          srv,
		authStore:       authStore,
	}

	// Log multi-tenancy status
	logMultiTenancyStatus(authStore, logger)

	// Initialize DMS subsystem
	if dmsErr := initializeDMS(cfg, srv, logger); dmsErr != nil {
		logger.Error("failed to initialize DMS subsystem", zap.Error(dmsErr))
		return nil, fmt.Errorf("failed to initialize DMS: %w", dmsErr)
	}

	// Initialize certificate lifecycle manager if enabled and PostgreSQL is available
	if cfg.CertLifecycle.Enabled && pgDB != nil {
		certMgr, certErr := initializeCertLifecycle(cfg, srv, pgDB, logger)
		if certErr != nil {
			logger.Warn("failed to initialize certificate lifecycle manager", zap.Error(certErr))
		} else {
			components.certManager = certMgr
		}
	}

	return components, nil
}

// createHTTPServer creates and configures the HTTP server.
func createHTTPServer(
	cfg *config.Config,
	logger *zap.Logger,
	store storage.Store,
	authStore server.AuthStore,
	authMw *auth.Middleware,
) (*server.Server, error) {
	// Create server (no static adapter — adapters are loaded dynamically via registry)
	srv := server.New(cfg, logger, nil, store, authStore, authMw)

	logger.Info("HTTP server created",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
		zap.String("mode", cfg.Server.GinMode),
		zap.Bool("auth_enabled", authStore != nil),
	)

	// Load OpenAPI specification
	spec, loadErr := loadOpenAPISpec(logger)
	if loadErr != nil {
		logger.Error("failed to load OpenAPI specification", zap.Error(loadErr))
		return nil, fmt.Errorf("failed to load OpenAPI specification: %w", loadErr)
	}

	srv.SetOpenAPISpec(spec)
	logger.Info("OpenAPI specification loaded",
		zap.Int("size", len(spec)),
	)

	return srv, nil
}

// logMultiTenancyStatus logs the multi-tenancy configuration status.
func logMultiTenancyStatus(authStore server.AuthStore, logger *zap.Logger) {
	if authStore != nil {
		logger.Info("multi-tenancy and RBAC enabled")
	} else {
		logger.Info("multi-tenancy is disabled")
	}
}

// runServerWithShutdown starts the server and handles graceful shutdown.
func runServerWithShutdown(cfg *config.Config, logger *zap.Logger, components *ApplicationComponents) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start certificate lifecycle manager if initialized.
	if components.certManager != nil {
		if err := components.certManager.Start(ctx); err != nil {
			logger.Error("failed to start certificate lifecycle manager", zap.Error(err))
		}
	}

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
	cfg, loadErr := config.Load(configPath)
	if loadErr != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, loadErr)
	}

	// Validate configuration
	if validateErr := cfg.Validate(); validateErr != nil {
		return nil, fmt.Errorf("invalid configuration: %w", validateErr)
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
	passwords, passErr := getRedisPasswords(cfg, logger)
	if passErr != nil {
		return nil, passErr
	}

	redisCfg := buildRedisConfig(cfg, passwords.redisPassword, passwords.sentinelPassword)
	if modeErr := configureRedisMode(redisCfg, cfg, logger); modeErr != nil {
		return nil, modeErr
	}

	logSecurityWarnings(cfg, logger)

	store := storage.NewRedisStore(redisCfg)
	if connErr := verifyRedisConnectivity(store); connErr != nil {
		return nil, connErr
	}

	logger.Info("Redis connectivity verified")
	return store, nil
}

// getRedisPasswords retrieves Redis and Sentinel passwords and logs
// deprecation warnings.
// redisPasswordConfig holds Redis and Sentinel passwords.
type redisPasswordConfig struct {
	redisPassword    string
	sentinelPassword string
}

func getRedisPasswords(
	cfg *config.Config,
	logger *zap.Logger,
) (*redisPasswordConfig, error) {
	// Get Redis password
	redisPassword, err := cfg.Redis.GetPassword()
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis password: %w", err)
	}

	// Log warning if using deprecated direct password configuration
	// Use helper method to avoid direct access to sensitive Password field
	if cfg.Redis.IsUsingDeprecatedPassword() {
		logger.Warn("SECURITY WARNING: Redis password is stored in plaintext configuration. "+
			"This is deprecated and insecure. Use password_env_var or password_file instead.",
			zap.Bool("deprecated_password_field", true))
	}

	// Get Sentinel password (only relevant for Sentinel mode)
	var sentinelPassword string
	if cfg.Redis.Mode == redisModeSentinel {
		var sentinelErr error
		sentinelPassword, sentinelErr = cfg.Redis.GetSentinelPassword()
		if sentinelErr != nil {
			return nil, fmt.Errorf("failed to get Sentinel password: %w", sentinelErr)
		}

		// Log warning if using deprecated direct sentinel password configuration
		// Use helper method to avoid direct access to sensitive SentinelPassword field
		if cfg.Redis.IsUsingDeprecatedSentinelPassword() {
			logger.Warn("SECURITY WARNING: Sentinel password is stored in plaintext configuration. "+
				"This is deprecated and insecure. Use sentinel_password_env_var or sentinel_password_file instead.",
				zap.Bool("deprecated_sentinel_password_field", true))
		}
	}

	return &redisPasswordConfig{
		redisPassword:    redisPassword,
		sentinelPassword: sentinelPassword,
	}, nil
}

// buildRedisConfig creates storage.RedisConfig from config.Config.
func buildRedisConfig(cfg *config.Config, password, redisModeSentinelPassword string) *storage.RedisConfig {
	return &storage.RedisConfig{
		DB:                     cfg.Redis.DB,
		Password:               password,
		SentinelPassword:       redisModeSentinelPassword,
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
	case redisModeSentinel:
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

// initializeDMS initializes the DMS (Deployment Management Service) subsystem.
// It creates a DMS registry and registers available deployment management adapters.
//
// Currently registered adapters:
//   - Helm: Package manager for Kubernetes applications
//
// Future adapters to be added:
//   - ArgoCD: GitOps continuous delivery tool
//   - Flux: GitOps toolkit for Kubernetes
//   - Crossplane: Infrastructure-as-Code tool
//   - Kustomize: Template-free Kubernetes configuration
//   - ONAP LCM: Open Network Automation Platform lifecycle manager
//   - OSM LCM: Open Source MANO lifecycle manager
//
// Parameters:
//   - cfg: Application configuration
//   - srv: Server instance to configure with DMS routes
//   - k8sAdapter: Kubernetes adapter for cluster access
//   - logger: Structured logger
//
// Returns an error if DMS initialization fails.
func initializeDMS(
	cfg *config.Config,
	srv *server.Server,
	logger *zap.Logger,
) error {
	// Create DMS registry with default configuration
	dmsReg := dmsregistry.NewRegistry(logger, nil)

	// Determine DMS adapter type from environment variable
	dmsAdapterType := os.Getenv("DMS_ADAPTER_TYPE")
	if dmsAdapterType == "" {
		// If ADAPTER_TYPE is set to mock, also default DMS to mock
		if os.Getenv("ADAPTER_TYPE") == adapterTypeMock {
			dmsAdapterType = adapterTypeMock
		} else {
			dmsAdapterType = "helm"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch dmsAdapterType {
	case adapterTypeMock:
		if regErr := registerMockDMSAdapter(ctx, dmsReg, logger); regErr != nil {
			return regErr
		}
	default:
		if regErr := registerHelmDMSAdapter(ctx, cfg, dmsReg, logger); regErr != nil {
			return regErr
		}
	}

	// Setup DMS routes and handlers
	srv.SetupDMS(dmsReg)

	logger.Info("DMS subsystem initialized successfully",
		zap.String("base_path", "/o2dms/v1"),
		zap.Int("endpoints", 4), // deploymentLifecycle, nfDeployments, nfDeploymentDescriptors, subscriptions
	)

	return nil
}

// registerMockDMSAdapter registers the mock DMS adapter.
func registerMockDMSAdapter(ctx context.Context, dmsReg *dmsregistry.Registry, logger *zap.Logger) error {
	logger.Info("initializing mock DMS adapter")
	mockDMSAdapter := dmsmock.NewAdapter(true) // Pre-populate with sample data
	if initErr := mockDMSAdapter.Initialize(ctx); initErr != nil {
		return fmt.Errorf("failed to initialize mock DMS adapter: %w", initErr)
	}

	mockConfig := map[string]interface{}{
		"populated": true,
		"packages":  5,
	}

	regErr := dmsReg.Register(
		ctx, adapterTypeMock, adapterTypeMock, mockDMSAdapter, mockConfig, true,
	)
	if regErr != nil {
		return fmt.Errorf("failed to register mock DMS adapter: %w", regErr)
	}

	logger.Info("mock DMS adapter registered successfully",
		zap.String("adapter", adapterTypeMock),
		zap.Int("packages", 5),
	)
	return nil
}

// registerHelmDMSAdapter registers the Helm DMS adapter.
func registerHelmDMSAdapter(
	ctx context.Context,
	cfg *config.Config,
	dmsReg *dmsregistry.Registry,
	logger *zap.Logger,
) error {
	helmConfig := &helm.Config{
		Kubeconfig: cfg.Kubernetes.ConfigPath,
		Namespace:  cfg.Kubernetes.Namespace,
		Timeout:    30 * time.Second,
	}

	helmAdapter, createErr := helm.NewAdapter(helmConfig)
	if createErr != nil {
		return fmt.Errorf("failed to create Helm adapter: %w", createErr)
	}

	helmAdapterConfig := map[string]interface{}{
		"namespace": helmConfig.Namespace,
		"timeout":   helmConfig.Timeout,
	}

	if regErr := dmsReg.Register(ctx, "helm", "helm", helmAdapter, helmAdapterConfig, true); regErr != nil {
		return fmt.Errorf("failed to register Helm adapter: %w", regErr)
	}

	logger.Info("Helm DMS adapter registered successfully",
		zap.String("adapter", "helm"),
	)
	return nil
}

// initializeCertLifecycle creates and configures the certificate lifecycle manager.
// It initializes the certificate metadata store, manager, and admin API handler.
func initializeCertLifecycle(
	cfg *config.Config,
	srv *server.Server,
	pgDB *database.DB,
	logger *zap.Logger,
) (*certlifecycle.Manager, error) {
	// Create certificate metadata store.
	certStore := certlifecycle.NewPostgresStore(pgDB)

	// Create vault PKI client if vault is configured.
	var vaultPKI certlifecycle.VaultPKI
	vaultAddr := os.Getenv("VAULT_ADDR")
	vaultToken := os.Getenv("VAULT_TOKEN")
	if vaultAddr != "" && vaultToken != "" {
		vaultCfg := &vaultpkg.Config{
			Address: vaultAddr,
			Token:   vaultToken,
			PKIPath: cfg.CertLifecycle.VaultPKIPath,
		}
		vaultClient, vaultErr := vaultpkg.NewClient(vaultCfg)
		if vaultErr != nil {
			return nil, fmt.Errorf("failed to create vault client: %w", vaultErr)
		}
		vaultPKI = vaultClient
	}

	// Resolve HMAC secret from environment.
	hmacSecret := ""
	if cfg.CertLifecycle.WebhookHMACSecretEnvVar != "" {
		hmacSecret = os.Getenv(cfg.CertLifecycle.WebhookHMACSecretEnvVar)
	}

	// Create lifecycle manager.
	mgr, err := certlifecycle.NewManager(&certlifecycle.ManagerConfig{
		Store:         certStore,
		VaultClient:   vaultPKI,
		Logger:        logger,
		ScanInterval:  cfg.CertLifecycle.ScanInterval,
		RenewalWindow: cfg.CertLifecycle.RenewalWindow,
		MaxRetries:    cfg.CertLifecycle.MaxRenewalRetries,
		RetryInterval: cfg.CertLifecycle.RenewalRetryInterval,
		WebhookURL:    cfg.CertLifecycle.WebhookURL,
		HMACSecret:    hmacSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cert lifecycle manager: %w", err)
	}

	// Register admin API handler.
	handler := certlifecycle.NewHandler(certStore, logger)
	srv.SetupCertLifecycle(handler)

	logger.Info("certificate lifecycle manager initialized",
		zap.Duration("scan_interval", cfg.CertLifecycle.ScanInterval),
		zap.Duration("renewal_window", cfg.CertLifecycle.RenewalWindow),
		zap.Bool("vault_configured", vaultPKI != nil),
		zap.Bool("webhook_configured", cfg.CertLifecycle.WebhookURL != ""),
	)

	return mgr, nil
}

// initializeHealthChecker creates and configures the health checker.
func initializeHealthChecker(
	redisStore *storage.RedisStore,
	pgDB *database.DB,
	logger *zap.Logger,
) *observability.HealthChecker {
	healthChecker := observability.NewHealthChecker(Version)

	// Set health check timeout
	healthChecker.SetTimeout(5 * time.Second)

	checkCount := 1

	// Register Redis health check
	healthChecker.RegisterHealthCheck("redis", observability.RedisHealthCheck(func(ctx context.Context) error {
		return redisStore.Ping(ctx)
	}))

	// Register the same checks for readiness
	healthChecker.RegisterReadinessCheck("redis",
		observability.RedisHealthCheck(func(ctx context.Context) error {
			return redisStore.Ping(ctx)
		}))

	// Register PostgreSQL health check if available
	if pgDB != nil {
		checkCount++
		healthChecker.RegisterHealthCheck("postgres", func(ctx context.Context) error {
			return pgDB.Ping(ctx)
		})
		healthChecker.RegisterReadinessCheck("postgres", func(ctx context.Context) error {
			return pgDB.Ping(ctx)
		})
	}

	logger.Info("health checks registered",
		zap.Int("health_checks", checkCount),
		zap.Int("readiness_checks", checkCount),
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
		data, err := os.ReadFile(filepath.Clean(path))
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

// buildAuthRedisConfig creates the Redis configuration for the auth store.
func buildAuthRedisConfig(
	cfg *config.Config,
	passwords *redisPasswordConfig,
) *auth.RedisConfig {
	authRedisCfg := &auth.RedisConfig{
		DB:               cfg.Redis.DB,
		Password:         passwords.redisPassword,
		SentinelPassword: passwords.sentinelPassword,
		MaxRetries:       cfg.Redis.MaxRetries,
		DialTimeout:      cfg.Redis.DialTimeout,
		ReadTimeout:      cfg.Redis.ReadTimeout,
		WriteTimeout:     cfg.Redis.WriteTimeout,
		PoolSize:         cfg.Redis.PoolSize,
	}

	switch cfg.Redis.Mode {
	case redisModeSentinel:
		authRedisCfg.UseSentinel = true
		authRedisCfg.SentinelAddrs = cfg.Redis.Addresses
		authRedisCfg.MasterName = cfg.Redis.MasterName
	default:
		authRedisCfg.UseSentinel = false
		if len(cfg.Redis.Addresses) > 0 {
			authRedisCfg.Addr = cfg.Redis.Addresses[0]
		} else {
			authRedisCfg.Addr = "localhost:6379"
		}
	}

	return authRedisCfg
}

// createAuthStore sets dest to the auth store based on the configured backend.
func createAuthStore(
	dest *auth.Store,
	cfg *config.Config,
	authRedisCfg *auth.RedisConfig,
	pgDB *database.DB,
	logger *zap.Logger,
) error {
	switch cfg.Auth.Backend {
	case "keycloak":
		return createKeycloakAuthStore(dest, cfg, logger)

	case "postgres":
		if pgDB == nil {
			return fmt.Errorf(
				"auth backend is postgres but PostgreSQL is not initialized",
			)
		}
		logger.Info("PostgreSQL auth store initialized",
			zap.String("backend", "postgres"),
		)
		*dest = auth.NewPostgresStore(pgDB)

		return nil

	default: // "redis"
		logger.Info("Redis auth store initialized",
			zap.String("backend", "redis"),
		)
		*dest = auth.NewRedisStore(authRedisCfg)

		return nil
	}
}

// createKeycloakAuthStore initializes a Keycloak-backed auth store and sets dest.
func createKeycloakAuthStore(
	dest *auth.Store,
	cfg *config.Config,
	logger *zap.Logger,
) error {
	keycloakCfg := &keycloak.Config{
		BaseURL:       cfg.Auth.Keycloak.BaseURL,
		Realm:         cfg.Auth.Keycloak.Realm,
		ClientID:      cfg.Auth.Keycloak.ClientID,
		ClientSecret:  cfg.Auth.Keycloak.ClientSecret,
		AdminUsername: cfg.Auth.Keycloak.AdminUsername,
		AdminPassword: cfg.Auth.Keycloak.AdminPassword,
		Timeout:       cfg.Auth.Keycloak.Timeout,
	}

	// Get secrets from environment if configured.
	if secret, secretErr := cfg.Auth.Keycloak.GetClientSecret(); secretErr == nil {
		keycloakCfg.ClientSecret = secret
	}
	if adminPass, adminErr := cfg.Auth.Keycloak.GetAdminPassword(); adminErr == nil {
		keycloakCfg.AdminPassword = adminPass
	}

	keycloakClient, clientErr := keycloak.NewClient(keycloakCfg)
	if clientErr != nil {
		return fmt.Errorf("failed to initialize Keycloak client: %w", clientErr)
	}

	logger.Info("Keycloak auth store initialized",
		zap.String("backend", "keycloak"),
		zap.String("base_url", cfg.Auth.Keycloak.BaseURL),
		zap.String("realm", cfg.Auth.Keycloak.Realm),
	)

	*dest = keycloak.NewStore(keycloakClient, logger)

	return nil
}

// initializeOAuth2 creates the OAuth2 authenticator and config if OAuth2 is enabled.
func initializeOAuth2(
	cfg *config.Config,
	authStore auth.Store,
	logger *zap.Logger,
) (*auth.OAuth2Authenticator, *auth.OAuth2Config, error) {
	if !cfg.OAuth2.Enabled {
		return nil, nil, nil
	}

	// Resolve OAuth2 client secret from env var if direct value is empty.
	oauth2Secret := cfg.OAuth2.KeycloakSecret
	if oauth2Secret == "" && cfg.OAuth2.KeycloakSecretEnvVar != "" {
		oauth2Secret = os.Getenv(cfg.OAuth2.KeycloakSecretEnvVar)
	}

	// Create Keycloak client.
	keycloakCfg := &keycloak.Config{
		BaseURL:      cfg.OAuth2.KeycloakBaseURL,
		Realm:        cfg.OAuth2.KeycloakRealm,
		ClientID:     cfg.OAuth2.KeycloakClientID,
		ClientSecret: oauth2Secret,
		Timeout:      10 * time.Second,
	}

	keycloakClient, oauth2ClientErr := keycloak.NewClient(keycloakCfg)
	if oauth2ClientErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to initialize OAuth2 Keycloak client: %w",
			oauth2ClientErr,
		)
	}

	// Create OAuth2 config.
	oauth2Cfg := &auth.OAuth2Config{
		Enabled:            cfg.OAuth2.Enabled,
		Priority:           cfg.OAuth2.Priority,
		AutoProvisionUsers: cfg.OAuth2.AutoProvisionUsers,
		DefaultRole:        cfg.OAuth2.DefaultRole,
		GroupRoleMapping:   cfg.OAuth2.GroupRoleMapping,
		RequireTenantClaim: cfg.OAuth2.RequireTenantClaim,
	}

	// Create OAuth2 authenticator.
	oauth2Auth := auth.NewOAuth2Authenticator(
		keycloakClient,
		authStore,
		oauth2Cfg,
		logger,
	)

	logger.Info("OAuth2 authentication enabled",
		zap.String("keycloak_url", cfg.OAuth2.KeycloakBaseURL),
		zap.String("realm", cfg.OAuth2.KeycloakRealm),
		zap.Bool("priority", cfg.OAuth2.Priority),
		zap.Bool("auto_provision", cfg.OAuth2.AutoProvisionUsers),
	)

	return oauth2Auth, oauth2Cfg, nil
}

// InitializeAuth initializes the authentication store and middleware.
// Errors are returned without exposing sensitive credential details in messages.
// This function is only called when cfg.MultiTenancy.Enabled is true.
//
// The initialization sequence:
//  1. Builds Redis configuration for the auth store
//  2. Creates the auth store based on the configured backend (redis, keycloak, postgres)
//  3. Verifies store connectivity and initializes default roles if configured
//  4. Initializes OAuth2 authenticator if enabled
//  5. Creates authentication middleware with configured skip paths and mTLS requirements
//
// Parameters:
//   - cfg: Application configuration containing multi-tenancy and Redis settings
//   - logger: Structured logger for logging initialization progress and errors
//   - pgDB: Optional PostgreSQL database (required if auth backend is "postgres")
//
// Returns:
//   - authStore: Initialized auth store for auth data (tenants, users, roles, audit)
//   - authMw: Configured authentication middleware for request validation
//   - err: Any error encountered during initialization
func InitializeAuth(
	cfg *config.Config,
	logger *zap.Logger,
	pgDB *database.DB,
) (auth.Store, *auth.Middleware, error) {
	// Get Redis password (reuse the same logic as main storage).
	passwords, err := getRedisPasswords(cfg, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Redis passwords for auth: %w", err)
	}

	// Build auth Redis config.
	authRedisCfg := buildAuthRedisConfig(cfg, passwords)

	// Create auth store based on configured backend.
	var authStore auth.Store
	if storeErr := createAuthStore(&authStore, cfg, authRedisCfg, pgDB, logger); storeErr != nil {
		return nil, nil, fmt.Errorf("failed to create auth store: %w", storeErr)
	}

	// Verify connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if pingErr := authStore.Ping(ctx); pingErr != nil {
		return nil, nil, fmt.Errorf("auth store connectivity check failed: %w", pingErr)
	}

	// Initialize default roles if configured.
	if cfg.MultiTenancy.InitializeDefaultRoles {
		if rolesErr := authStore.InitializeDefaultRoles(ctx); rolesErr != nil {
			return nil, nil, fmt.Errorf("failed to initialize default roles: %w", rolesErr)
		}
		logger.Info("default roles initialized")
	}

	// Initialize OAuth2 authenticator if enabled.
	oauth2Auth, oauth2Cfg, oauth2Err := initializeOAuth2(cfg, authStore, logger)
	if oauth2Err != nil {
		return nil, nil, oauth2Err
	}

	// Create middleware config.
	mwConfig := &auth.MiddlewareConfig{
		Enabled:     true,
		SkipPaths:   cfg.MultiTenancy.SkipAuthPaths,
		RequireMTLS: cfg.MultiTenancy.RequireMTLS,
	}

	// Create auth middleware.
	authMw := auth.NewMiddleware(authStore, mwConfig, logger, oauth2Auth, oauth2Cfg)

	logger.Info("auth middleware created",
		zap.Int("skip_paths", len(mwConfig.SkipPaths)),
		zap.Bool("require_mtls", mwConfig.RequireMTLS),
	)

	return authStore, authMw, nil
}
