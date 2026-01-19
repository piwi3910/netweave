package main

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/keycloak"
)

// ValidationResult contains validation results.
type ValidationResult struct {
	TenantsInRedis    int
	TenantsInKeycloak int
	UsersInRedis      int
	UsersInKeycloak   int
	RolesInRedis      int
	RolesInKeycloak   int
	Discrepancies     []Discrepancy
}

// Discrepancy represents a data mismatch between Redis and Keycloak.
type Discrepancy struct {
	Type        string // "tenant", "user", "role"
	ID          string
	Description string
}

// Validator validates migrated data between Redis and Keycloak.
type Validator struct {
	redisStore     auth.Store
	keycloakClient *keycloak.Client
	logger         *zap.Logger
}

// NewValidator creates a new Validator instance.
func NewValidator(
	redisStore auth.Store,
	keycloakClient *keycloak.Client,
	logger *zap.Logger,
) *Validator {
	return &Validator{
		redisStore:     redisStore,
		keycloakClient: keycloakClient,
		logger:         logger,
	}
}

// Validate performs validation of migrated data.
func (v *Validator) Validate(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{
		Discrepancies: make([]Discrepancy, 0),
	}

	// Validate tenants
	if err := v.validateTenants(ctx, result); err != nil {
		return result, fmt.Errorf("tenant validation failed: %w", err)
	}

	// Validate roles
	if err := v.validateRoles(ctx, result); err != nil {
		return result, fmt.Errorf("role validation failed: %w", err)
	}

	// Validate users
	if err := v.validateUsers(ctx, result); err != nil {
		return result, fmt.Errorf("user validation failed: %w", err)
	}

	return result, nil
}

func (v *Validator) validateTenants(ctx context.Context, result *ValidationResult) error {
	// Get all tenants from Redis
	tenants, err := v.redisStore.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tenants from Redis: %w", err)
	}
	result.TenantsInRedis = len(tenants)

	// Check each tenant exists in Keycloak
	for _, tenant := range tenants {
		exists, err := v.keycloakClient.TenantAttributeExists(ctx, tenant.ID)
		if err != nil {
			v.logger.Warn("Failed to check tenant in Keycloak",
				zap.String("tenantID", tenant.ID),
				zap.Error(err),
			)
			continue
		}

		if exists {
			result.TenantsInKeycloak++
		} else {
			result.Discrepancies = append(result.Discrepancies, Discrepancy{
				Type:        "tenant",
				ID:          tenant.ID,
				Description: fmt.Sprintf("Tenant %s exists in Redis but not in Keycloak", tenant.ID),
			})
		}
	}

	return nil
}

func (v *Validator) validateRoles(ctx context.Context, result *ValidationResult) error {
	// Get all roles from Redis
	roles := auth.GetDefaultRoles()
	customRoles, err := v.redisStore.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles from Redis: %w", err)
	}
	roles = append(roles, customRoles...)
	result.RolesInRedis = len(roles)

	// Check each role exists in Keycloak
	for _, role := range roles {
		exists, err := v.keycloakClient.RoleExists(ctx, string(role.Name))
		if err != nil {
			v.logger.Warn("Failed to check role in Keycloak",
				zap.String("roleID", role.ID),
				zap.Error(err),
			)
			continue
		}

		if exists {
			result.RolesInKeycloak++
		} else {
			result.Discrepancies = append(result.Discrepancies, Discrepancy{
				Type:        "role",
				ID:          role.ID,
				Description: fmt.Sprintf("Role %s exists in Redis but not in Keycloak", string(role.Name)),
			})
		}
	}

	return nil
}

func (v *Validator) validateUsers(ctx context.Context, result *ValidationResult) error {
	// Get all tenants to iterate through users
	tenants, err := v.redisStore.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	// Collect all users
	for _, tenant := range tenants {
		users, err := v.redisStore.ListUsersByTenant(ctx, tenant.ID)
		if err != nil {
			v.logger.Warn("Failed to list users for tenant",
				zap.String("tenantID", tenant.ID),
				zap.Error(err),
			)
			continue
		}
		result.UsersInRedis += len(users)

		// Check each user exists in Keycloak
		for _, user := range users {
			var exists bool
			var checkErr error

			if user.Email != "" {
				exists, checkErr = v.keycloakClient.UserExistsByEmail(ctx, user.Email)
			} else if user.CommonName != "" {
				exists, checkErr = v.keycloakClient.UserExistsByUsername(ctx, user.CommonName)
			} else {
				result.Discrepancies = append(result.Discrepancies, Discrepancy{
					Type:        "user",
					ID:          user.ID,
					Description: fmt.Sprintf("User %s has no email or username", user.ID),
				})
				continue
			}

			if checkErr != nil {
				v.logger.Warn("Failed to check user in Keycloak",
					zap.String("userID", user.ID),
					zap.Error(checkErr),
				)
				continue
			}

			if exists {
				result.UsersInKeycloak++
			} else {
				result.Discrepancies = append(result.Discrepancies, Discrepancy{
					Type:        "user",
					ID:          user.ID,
					Description: fmt.Sprintf("User %s exists in Redis but not in Keycloak", user.Email),
				})
			}
		}
	}

	return nil
}
