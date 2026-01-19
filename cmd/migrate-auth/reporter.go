package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/keycloak"
)

// MigrationReport contains a comprehensive migration status report.
type MigrationReport struct {
	GeneratedAt     time.Time
	RedisStats      RedisStats
	KeycloakStats   KeycloakStats
	Status          MigrationStatus
	Recommendations []string
}

// RedisStats contains Redis data statistics.
type RedisStats struct {
	Tenants int
	Users   int
	Roles   int
}

// KeycloakStats contains Keycloak data statistics.
type KeycloakStats struct {
	Realms int
	Users  int
	Roles  int
}

// MigrationStatus contains migration progress information.
type MigrationStatus struct {
	MigratedTenants int
	MigratedUsers   int
	MigratedRoles   int
	Pending         int
}

// Reporter generates migration status reports.
type Reporter struct {
	redisStore     auth.Store
	keycloakClient *keycloak.Client
	logger         *zap.Logger
}

// NewReporter creates a new Reporter instance.
func NewReporter(
	redisStore auth.Store,
	keycloakClient *keycloak.Client,
	logger *zap.Logger,
) *Reporter {
	return &Reporter{
		redisStore:     redisStore,
		keycloakClient: keycloakClient,
		logger:         logger,
	}
}

// GenerateReport generates a comprehensive migration report.
func (r *Reporter) GenerateReport(ctx context.Context) (*MigrationReport, error) {
	report := &MigrationReport{
		GeneratedAt:     time.Now(),
		Recommendations: make([]string, 0),
	}

	// Gather Redis statistics
	if err := r.gatherRedisStats(ctx, report); err != nil {
		return report, fmt.Errorf("failed to gather Redis stats: %w", err)
	}

	// Gather Keycloak statistics
	if err := r.gatherKeycloakStats(ctx, report); err != nil {
		return report, fmt.Errorf("failed to gather Keycloak stats: %w", err)
	}

	// Calculate migration status
	r.calculateMigrationStatus(report)

	// Generate recommendations
	r.generateRecommendations(report)

	return report, nil
}

func (r *Reporter) gatherRedisStats(ctx context.Context, report *MigrationReport) error {
	// Count tenants
	tenants, err := r.redisStore.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}
	report.RedisStats.Tenants = len(tenants)

	// Count roles
	roles := auth.GetDefaultRoles()
	customRoles, err := r.redisStore.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles: %w", err)
	}
	roles = append(roles, customRoles...)
	report.RedisStats.Roles = len(roles)

	// Count users across all tenants
	for _, tenant := range tenants {
		users, err := r.redisStore.ListUsersByTenant(ctx, tenant.ID)
		if err != nil {
			r.logger.Warn("Failed to list users for tenant",
				zap.String("tenantID", tenant.ID),
				zap.Error(err),
			)
			continue
		}
		report.RedisStats.Users += len(users)
	}

	return nil
}

func (r *Reporter) gatherKeycloakStats(ctx context.Context, report *MigrationReport) error {
	// For now, we store data in the main realm, so realms = 1
	report.KeycloakStats.Realms = 1

	// Count users in Keycloak
	users, err := r.keycloakClient.ListUsers(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to list users from Keycloak: %w", err)
	}
	report.KeycloakStats.Users = len(users)

	// Count roles in Keycloak
	roles, err := r.keycloakClient.ListRealmRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles from Keycloak: %w", err)
	}
	report.KeycloakStats.Roles = len(roles)

	return nil
}

func (r *Reporter) calculateMigrationStatus(report *MigrationReport) {
	// Simple calculation based on Keycloak vs Redis counts
	// In reality, you'd want more sophisticated tracking

	// Tenants (stored as attributes, harder to count directly)
	report.Status.MigratedTenants = report.RedisStats.Tenants // Assume all migrated if script ran

	// Roles
	report.Status.MigratedRoles = report.KeycloakStats.Roles

	// Users
	report.Status.MigratedUsers = report.KeycloakStats.Users

	// Calculate pending
	pendingRoles := report.RedisStats.Roles - report.Status.MigratedRoles
	pendingUsers := report.RedisStats.Users - report.Status.MigratedUsers
	if pendingRoles < 0 {
		pendingRoles = 0
	}
	if pendingUsers < 0 {
		pendingUsers = 0
	}
	report.Status.Pending = pendingRoles + pendingUsers
}

func (r *Reporter) generateRecommendations(report *MigrationReport) {
	// Check if migration is complete
	if report.Status.Pending == 0 {
		report.Recommendations = append(report.Recommendations,
			"Migration appears complete. Run validation to verify data integrity.")
	} else {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Migration incomplete: %d items pending", report.Status.Pending))
	}

	// Check for mismatches
	if report.RedisStats.Users != report.KeycloakStats.Users {
		diff := report.RedisStats.Users - report.KeycloakStats.Users
		if diff > 0 {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Found %d users in Redis but not in Keycloak. Run migration again.", diff))
		}
	}

	if report.RedisStats.Roles != report.KeycloakStats.Roles {
		diff := report.RedisStats.Roles - report.KeycloakStats.Roles
		if diff > 0 {
			report.Recommendations = append(report.Recommendations,
				fmt.Sprintf("Found %d roles in Redis but not in Keycloak. Run migration again.", diff))
		}
	}

	// Recommend validation
	if len(report.Recommendations) == 0 {
		report.Recommendations = append(report.Recommendations,
			"All data migrated successfully. Run 'migrate-auth validate' to verify integrity.")
	}
}
