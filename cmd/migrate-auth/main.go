// Package main provides the data migration tool for migrating authentication data
// from Redis to Keycloak. This tool supports dry-run mode, incremental migration,
// and rollback capabilities.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/keycloak"
)

var (
	// Global flags.
	cfgFile    string
	dryRun     bool
	verbose    bool
	logLevel   string
	concurrent int

	// Migration-specific flags.
	onlyTenants bool
	onlyUsers   bool
	onlyRoles   bool
	tenantID    string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "migrate-auth",
		Short: "Migrate authentication data from Redis to Keycloak",
		Long: `migrate-auth is a data migration tool for migrating tenants, users, and roles
from Redis storage to Keycloak. It supports dry-run mode for previewing changes,
incremental migration for specific data types or tenants, and rollback capabilities.

Examples:
  # Dry-run migration (preview only)
  migrate-auth migrate --dry-run

  # Migrate all data
  migrate-auth migrate

  # Migrate only tenants
  migrate-auth migrate --only-tenants

  # Migrate only users for specific tenant
  migrate-auth migrate --only-users --tenant-id=tenant-123

  # Validate migration
  migrate-auth validate

  # Generate migration report
  migrate-auth report

  # Export Keycloak state for rollback
  migrate-auth export --output=keycloak-backup.json`,
	}

	// Add global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview changes without applying them")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().IntVar(&concurrent, "concurrent", 5, "number of concurrent operations")

	// Add commands
	rootCmd.AddCommand(migrateCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(reportCmd())
	rootCmd.AddCommand(exportCmd())
	rootCmd.AddCommand(rollbackCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate data from Redis to Keycloak",
		Long: `Migrate tenants, users, and roles from Redis to Keycloak.
Supports incremental migration and dry-run mode.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMigration()
		},
	}

	cmd.Flags().BoolVar(&onlyTenants, "only-tenants", false, "migrate only tenants")
	cmd.Flags().BoolVar(&onlyUsers, "only-users", false, "migrate only users")
	cmd.Flags().BoolVar(&onlyRoles, "only-roles", false, "migrate only roles")
	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "migrate only specific tenant")

	return cmd
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate migrated data",
		Long:  `Compare Redis and Keycloak data to verify migration integrity.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runValidation()
		},
	}
}

func reportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Generate migration report",
		Long:  `Generate a detailed report of migration status and discrepancies.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runReport()
		},
	}
}

func exportCmd() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Keycloak state",
		Long:  `Export current Keycloak state for backup/rollback purposes.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runExport(outputFile)
		},
	}

	cmd.Flags().StringVar(&outputFile, "output", "keycloak-backup.json", "output file path")

	return cmd
}

func rollbackCmd() *cobra.Command {
	var inputFile string

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback migration",
		Long:  `Restore Keycloak state from a backup file.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runRollback(inputFile)
		},
	}

	cmd.Flags().StringVar(&inputFile, "input", "keycloak-backup.json", "backup file path")

	return cmd
}

func runMigration() error {
	logger := createLogger()
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting migration",
		zap.Bool("dryRun", dryRun),
		zap.Bool("onlyTenants", onlyTenants),
		zap.Bool("onlyUsers", onlyUsers),
		zap.Bool("onlyRoles", onlyRoles),
		zap.String("tenantID", tenantID),
	)

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create Redis store
	redisStore := createRedisStore(cfg, logger)

	// Create Keycloak client
	keycloakClient, err := createKeycloakClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	// Create migrator
	migrator := NewMigrator(redisStore, keycloakClient, logger, MigratorConfig{
		DryRun:     dryRun,
		Concurrent: concurrent,
	})

	// Run migration
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	result, err := migrator.Migrate(ctx, MigrationOptions{
		OnlyTenants: onlyTenants,
		OnlyUsers:   onlyUsers,
		OnlyRoles:   onlyRoles,
		TenantID:    tenantID,
	})
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Print results
	printMigrationResult(result, logger)

	if dryRun {
		logger.Info("Dry-run complete - no changes were made")
	} else {
		logger.Info("Migration complete")
	}

	return nil
}

func runValidation() error {
	logger := createLogger()
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting validation")

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	redisStore := createRedisStore(cfg, logger)

	keycloakClient, err := createKeycloakClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	validator := NewValidator(redisStore, keycloakClient, logger)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	result, err := validator.Validate(ctx)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	printValidationResult(result, logger)

	if len(result.Discrepancies) > 0 {
		return fmt.Errorf("validation found %d discrepancies", len(result.Discrepancies))
	}

	logger.Info("Validation successful - no discrepancies found")
	return nil
}

func runReport() error {
	logger := createLogger()
	defer func() { _ = logger.Sync() }()

	logger.Info("Generating migration report")

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	redisStore := createRedisStore(cfg, logger)

	keycloakClient, err := createKeycloakClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	reporter := NewReporter(redisStore, keycloakClient, logger)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	report, err := reporter.GenerateReport(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	printReport(report, logger)
	return nil
}

func runExport(outputFile string) error {
	logger := createLogger()
	defer func() { _ = logger.Sync() }()

	logger.Info("Exporting Keycloak state", zap.String("output", outputFile))

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	keycloakClient, err := createKeycloakClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	exporter := NewExporter(keycloakClient, logger)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := exporter.Export(ctx, outputFile); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	logger.Info("Export complete", zap.String("file", outputFile))
	return nil
}

func runRollback(inputFile string) error {
	logger := createLogger()
	defer func() { _ = logger.Sync() }()

	logger.Info("Rolling back from backup", zap.String("input", inputFile))

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	keycloakClient, err := createKeycloakClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	rollbacker := NewRollbacker(keycloakClient, logger)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := rollbacker.Rollback(ctx, inputFile); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	logger.Info("Rollback complete")
	return nil
}

func loadConfig() (*config.Config, error) {
	if cfgFile == "" {
		cfgFile = "config.yaml"
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}
	return cfg, nil
}

func createLogger() *zap.Logger {
	level := zapcore.InfoLevel
	if verbose {
		level = zapcore.DebugLevel
	}

	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Encoding:         "console",
		EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, _ := cfg.Build()
	return logger
}

// createRedisStore creates a Redis store instance.
// Returns auth.Store interface to allow for dependency injection and testing.
func createRedisStore(cfg *config.Config, _ *zap.Logger) auth.Store {
	// Determine address
	addr := "localhost:6379"
	if len(cfg.Redis.Addresses) > 0 {
		addr = cfg.Redis.Addresses[0]
	}

	// Determine if using sentinel
	useSentinel := cfg.Redis.Mode == "sentinel"
	sentinelAddrs := []string{}
	if useSentinel {
		sentinelAddrs = cfg.Redis.Addresses
	}

	return auth.NewRedisStore(&auth.RedisConfig{
		Addr:             addr,
		Password:         cfg.Redis.Password,
		SentinelPassword: cfg.Redis.SentinelPassword,
		DB:               cfg.Redis.DB,
		UseSentinel:      useSentinel,
		SentinelAddrs:    sentinelAddrs,
		MasterName:       cfg.Redis.MasterName,
	})
}

func createKeycloakClient(cfg *config.Config) (*keycloak.Client, error) {
	client, err := keycloak.NewClient(&keycloak.Config{
		BaseURL:       cfg.OAuth2.KeycloakBaseURL,
		Realm:         cfg.OAuth2.KeycloakRealm,
		ClientID:      cfg.OAuth2.KeycloakClientID,
		ClientSecret:  cfg.OAuth2.KeycloakSecret,
		AdminUsername: cfg.OAuth2.KeycloakAdminUsername,
		AdminPassword: cfg.OAuth2.KeycloakAdminPassword,
		Timeout:       30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Keycloak client: %w", err)
	}
	return client, nil
}

func printMigrationResult(result *MigrationResult, logger *zap.Logger) {
	logger.Info("Migration Summary",
		zap.Int("tenantsProcessed", result.TenantsProcessed),
		zap.Int("tenantsCreated", result.TenantsCreated),
		zap.Int("tenantsSkipped", result.TenantsSkipped),
		zap.Int("tenantsFailed", result.TenantsFailed),
		zap.Int("usersProcessed", result.UsersProcessed),
		zap.Int("usersCreated", result.UsersCreated),
		zap.Int("usersSkipped", result.UsersSkipped),
		zap.Int("usersFailed", result.UsersFailed),
		zap.Int("rolesProcessed", result.RolesProcessed),
		zap.Int("rolesCreated", result.RolesCreated),
		zap.Int("rolesSkipped", result.RolesSkipped),
		zap.Int("rolesFailed", result.RolesFailed),
		zap.Duration("duration", result.Duration),
	)

	if len(result.Errors) > 0 {
		logger.Warn("Migration errors", zap.Int("count", len(result.Errors)))
		for i, err := range result.Errors {
			if i >= 10 {
				logger.Warn("... and more errors", zap.Int("total", len(result.Errors)))
				break
			}
			logger.Error("Migration error", zap.Error(err))
		}
	}
}

func printValidationResult(result *ValidationResult, logger *zap.Logger) {
	logger.Info("Validation Summary",
		zap.Int("tenantsInRedis", result.TenantsInRedis),
		zap.Int("tenantsInKeycloak", result.TenantsInKeycloak),
		zap.Int("usersInRedis", result.UsersInRedis),
		zap.Int("usersInKeycloak", result.UsersInKeycloak),
		zap.Int("rolesInRedis", result.RolesInRedis),
		zap.Int("rolesInKeycloak", result.RolesInKeycloak),
		zap.Int("discrepancies", len(result.Discrepancies)),
	)

	if len(result.Discrepancies) > 0 {
		logger.Warn("Found discrepancies")
		for i, disc := range result.Discrepancies {
			if i >= 10 {
				logger.Warn("... and more discrepancies", zap.Int("total", len(result.Discrepancies)))
				break
			}
			logger.Warn("Discrepancy",
				zap.String("type", disc.Type),
				zap.String("id", disc.ID),
				zap.String("description", disc.Description),
			)
		}
	}
}

func printReport(report *MigrationReport, logger *zap.Logger) {
	logger.Info("=== Migration Report ===")
	logger.Info("Report details",
		zap.Time("generatedAt", report.GeneratedAt),
	)
	logger.Info("",
		zap.String("section", "Redis Stats"),
		zap.Int("tenants", report.RedisStats.Tenants),
		zap.Int("users", report.RedisStats.Users),
		zap.Int("roles", report.RedisStats.Roles),
	)
	logger.Info("",
		zap.String("section", "Keycloak Stats"),
		zap.Int("realms", report.KeycloakStats.Realms),
		zap.Int("users", report.KeycloakStats.Users),
		zap.Int("roles", report.KeycloakStats.Roles),
	)
	logger.Info("",
		zap.String("section", "Migration Status"),
		zap.Int("migratedTenants", report.Status.MigratedTenants),
		zap.Int("migratedUsers", report.Status.MigratedUsers),
		zap.Int("migratedRoles", report.Status.MigratedRoles),
		zap.Int("pending", report.Status.Pending),
	)

	if len(report.Recommendations) > 0 {
		logger.Info("Recommendations", zap.Strings("items", report.Recommendations))
	}
}
