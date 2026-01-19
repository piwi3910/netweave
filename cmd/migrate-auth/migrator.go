package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/keycloak"
)

// MigratorConfig holds configuration for the migrator.
type MigratorConfig struct {
	DryRun     bool
	Concurrent int
}

// MigrationOptions specifies what to migrate.
type MigrationOptions struct {
	OnlyTenants bool
	OnlyUsers   bool
	OnlyRoles   bool
	TenantID    string
}

// MigrationResult contains the results of a migration operation.
type MigrationResult struct {
	TenantsProcessed int
	TenantsCreated   int
	TenantsSkipped   int
	TenantsFailed    int

	UsersProcessed int
	UsersCreated   int
	UsersSkipped   int
	UsersFailed    int

	RolesProcessed int
	RolesCreated   int
	RolesSkipped   int
	RolesFailed    int

	Errors   []error
	Duration time.Duration
}

// Migrator handles migration from Redis to Keycloak.
type Migrator struct {
	redisStore     auth.Store
	keycloakClient *keycloak.Client
	logger         *zap.Logger
	config         MigratorConfig
	mu             sync.Mutex
}

// NewMigrator creates a new Migrator instance.
func NewMigrator(
	redisStore auth.Store,
	keycloakClient *keycloak.Client,
	logger *zap.Logger,
	config MigratorConfig,
) *Migrator {
	return &Migrator{
		redisStore:     redisStore,
		keycloakClient: keycloakClient,
		logger:         logger,
		config:         config,
	}
}

// Migrate performs the migration.
func (m *Migrator) Migrate(ctx context.Context, opts MigrationOptions) (*MigrationResult, error) {
	start := time.Now()
	result := &MigrationResult{}

	// Determine what to migrate
	migrateTenants := !opts.OnlyUsers && !opts.OnlyRoles
	migrateRoles := !opts.OnlyTenants && !opts.OnlyUsers
	migrateUsers := !opts.OnlyTenants && !opts.OnlyRoles

	if opts.OnlyTenants {
		migrateTenants = true
	}
	if opts.OnlyUsers {
		migrateUsers = true
	}
	if opts.OnlyRoles {
		migrateRoles = true
	}

	// Migrate in order: tenants -> roles -> users (users depend on roles and tenants)
	if migrateTenants {
		m.logger.Info("Migrating tenants...")
		if err := m.migrateTenants(ctx, opts, result); err != nil {
			return result, fmt.Errorf("tenant migration failed: %w", err)
		}
	}

	if migrateRoles {
		m.logger.Info("Migrating roles...")
		if err := m.migrateRoles(ctx, opts, result); err != nil {
			return result, fmt.Errorf("role migration failed: %w", err)
		}
	}

	if migrateUsers {
		m.logger.Info("Migrating users...")
		if err := m.migrateUsers(ctx, opts, result); err != nil {
			return result, fmt.Errorf("user migration failed: %w", err)
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (m *Migrator) migrateTenants(ctx context.Context, opts MigrationOptions, result *MigrationResult) error {
	// Get all tenants from Redis
	tenants, err := m.redisStore.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tenants from Redis: %w", err)
	}

	// Filter by tenant ID if specified
	if opts.TenantID != "" {
		filteredTenants := make([]*auth.Tenant, 0)
		for _, t := range tenants {
			if t.ID == opts.TenantID {
				filteredTenants = append(filteredTenants, t)
				break
			}
		}
		tenants = filteredTenants
	}

	m.logger.Info("Found tenants in Redis", zap.Int("count", len(tenants)))

	// Migrate tenants with concurrency control
	sem := make(chan struct{}, m.config.Concurrent)
	var wg sync.WaitGroup

	for _, tenant := range tenants {
		wg.Add(1)
		sem <- struct{}{}

		go func(t *auth.Tenant) {
			defer wg.Done()
			defer func() { <-sem }()

			result.TenantsProcessed++

			if err := m.migrateTenant(ctx, t); err != nil {
				m.logger.Error("Failed to migrate tenant",
					zap.String("tenantID", t.ID),
					zap.Error(err),
				)
				result.TenantsFailed++
				m.addError(result, fmt.Errorf("tenant %s: %w", t.ID, err))
			} else {
				if m.config.DryRun {
					result.TenantsSkipped++
					m.logger.Debug("Tenant migration dry-run", zap.String("tenantID", t.ID))
				} else {
					result.TenantsCreated++
					m.logger.Info("Migrated tenant", zap.String("tenantID", t.ID))
				}
			}
		}(tenant)
	}

	wg.Wait()
	return nil
}

func (m *Migrator) migrateTenant(ctx context.Context, tenant *auth.Tenant) error {
	// Check if tenant already exists in Keycloak (as realm or attributes)
	exists, err := m.keycloakClient.RealmExists(ctx, tenant.ID)
	if err != nil {
		return fmt.Errorf("failed to check if realm exists: %w", err)
	}

	if exists {
		m.logger.Debug("Tenant already exists in Keycloak", zap.String("tenantID", tenant.ID))
		return nil
	}

	if m.config.DryRun {
		m.logger.Info("DRY-RUN: Would create tenant",
			zap.String("tenantID", tenant.ID),
			zap.String("name", tenant.Name),
		)
		return nil
	}

	// Create tenant as Keycloak realm or store as attributes
	// For simplicity, we'll create a tenant attribute in the main realm
	// In production, you might create separate realms per tenant
	if err := m.keycloakClient.SetTenantAttribute(ctx, tenant.ID, map[string]interface{}{
		"id":           tenant.ID,
		"name":         tenant.Name,
		"description":  tenant.Description,
		"status":       string(tenant.Status),
		"contactEmail": tenant.ContactEmail,
		"quota":        tenant.Quota,
		"metadata":     tenant.Metadata,
	}); err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

func (m *Migrator) migrateRoles(ctx context.Context, _ MigrationOptions, result *MigrationResult) error {
	// Get default roles from Redis
	roles := auth.GetDefaultRoles()

	// Get custom roles if any
	customRoles, err := m.redisStore.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles from Redis: %w", err)
	}
	roles = append(roles, customRoles...)

	m.logger.Info("Found roles in Redis", zap.Int("count", len(roles)))

	// Migrate roles with concurrency control
	sem := make(chan struct{}, m.config.Concurrent)
	var wg sync.WaitGroup

	for _, role := range roles {
		wg.Add(1)
		sem <- struct{}{}

		go func(r *auth.Role) {
			defer wg.Done()
			defer func() { <-sem }()

			result.RolesProcessed++

			if err := m.migrateRole(ctx, r); err != nil {
				m.logger.Error("Failed to migrate role",
					zap.String("roleID", r.ID),
					zap.String("roleName", string(r.Name)),
					zap.Error(err),
				)
				result.RolesFailed++
				m.addError(result, fmt.Errorf("role %s: %w", r.ID, err))
			} else {
				if m.config.DryRun {
					result.RolesSkipped++
					m.logger.Debug("Role migration dry-run", zap.String("roleID", r.ID))
				} else {
					result.RolesCreated++
					m.logger.Info("Migrated role", zap.String("roleID", r.ID))
				}
			}
		}(role)
	}

	wg.Wait()
	return nil
}

func (m *Migrator) migrateRole(ctx context.Context, role *auth.Role) error {
	// Check if role already exists in Keycloak
	exists, err := m.keycloakClient.RoleExists(ctx, string(role.Name))
	if err != nil {
		return fmt.Errorf("failed to check if role exists: %w", err)
	}

	if exists {
		m.logger.Debug("Role already exists in Keycloak", zap.String("roleID", role.ID))
		return nil
	}

	if m.config.DryRun {
		m.logger.Info("DRY-RUN: Would create role",
			zap.String("roleID", role.ID),
			zap.String("roleName", string(role.Name)),
			zap.String("type", string(role.Type)),
		)
		return nil
	}

	// Convert permissions to Keycloak scopes/claims
	scopes := make([]string, len(role.Permissions))
	for i, perm := range role.Permissions {
		scopes[i] = string(perm)
	}

	// Create role in Keycloak
	if err := m.keycloakClient.CreateRole(ctx, &keycloak.Role{
		Name:        string(role.Name),
		Description: role.Description,
		Composite:   false,
		ClientRole:  role.Type == auth.RoleTypeTenant,
		Attributes: map[string][]string{
			"permissions": scopes,
			"type":        {string(role.Type)},
			"roleId":      {role.ID},
		},
	}); err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}

	return nil
}

func (m *Migrator) migrateUsers(ctx context.Context, opts MigrationOptions, result *MigrationResult) error {
	// Get all tenants to iterate through users
	var tenantIDs []string
	if opts.TenantID != "" {
		tenantIDs = []string{opts.TenantID}
	} else {
		tenants, err := m.redisStore.ListTenants(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tenants: %w", err)
		}
		for _, t := range tenants {
			tenantIDs = append(tenantIDs, t.ID)
		}
	}

	// Collect all users
	var allUsers []*auth.TenantUser
	for _, tenantID := range tenantIDs {
		users, err := m.redisStore.ListUsersByTenant(ctx, tenantID)
		if err != nil {
			m.logger.Warn("Failed to list users for tenant",
				zap.String("tenantID", tenantID),
				zap.Error(err),
			)
			continue
		}
		allUsers = append(allUsers, users...)
	}

	m.logger.Info("Found users in Redis", zap.Int("count", len(allUsers)))

	// Migrate users with concurrency control
	sem := make(chan struct{}, m.config.Concurrent)
	var wg sync.WaitGroup

	for _, user := range allUsers {
		wg.Add(1)
		sem <- struct{}{}

		go func(u *auth.TenantUser) {
			defer wg.Done()
			defer func() { <-sem }()

			result.UsersProcessed++

			if err := m.migrateUser(ctx, u); err != nil {
				m.logger.Error("Failed to migrate user",
					zap.String("userID", u.ID),
					zap.String("tenantID", u.TenantID),
					zap.String("email", u.Email),
					zap.Error(err),
				)
				result.UsersFailed++
				m.addError(result, fmt.Errorf("user %s: %w", u.ID, err))
			} else {
				if m.config.DryRun {
					result.UsersSkipped++
					m.logger.Debug("User migration dry-run", zap.String("userID", u.ID))
				} else {
					result.UsersCreated++
					m.logger.Info("Migrated user", zap.String("userID", u.ID))
				}
			}
		}(user)
	}

	wg.Wait()
	return nil
}

func (m *Migrator) migrateUser(ctx context.Context, user *auth.TenantUser) error {
	// Check if user already exists in Keycloak
	var exists bool
	var err error

	if user.Email != "" {
		exists, err = m.keycloakClient.UserExistsByEmail(ctx, user.Email)
	} else if user.CommonName != "" {
		exists, err = m.keycloakClient.UserExistsByUsername(ctx, user.CommonName)
	} else {
		return fmt.Errorf("user has no email or username")
	}

	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if exists {
		m.logger.Debug("User already exists in Keycloak",
			zap.String("userID", user.ID),
			zap.String("email", user.Email),
		)
		return nil
	}

	if m.config.DryRun {
		m.logger.Info("DRY-RUN: Would create user",
			zap.String("userID", user.ID),
			zap.String("tenantID", user.TenantID),
			zap.String("email", user.Email),
			zap.String("commonName", user.CommonName),
		)
		return nil
	}

	// Get role name for assignment
	role, err := m.redisStore.GetRole(ctx, user.RoleID)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}

	// Create user in Keycloak
	kcUser := &keycloak.User{
		Username:  user.CommonName,
		Email:     user.Email,
		Enabled:   user.IsActive,
		Attributes: map[string][]string{
			"userId":       {user.ID},
			"tenantId":     {user.TenantID},
			"subject":      {user.Subject},
			"oauthSubject": {user.OAuthSubject},
		},
	}

	userID, err := m.keycloakClient.CreateUser(ctx, kcUser)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	m.logger.Debug("Created user in Keycloak",
		zap.String("userID", userID),
		zap.String("email", user.Email),
	)

	// Assign role to user
	if err := m.keycloakClient.AssignRoleToUser(ctx, user.Email, string(role.Name)); err != nil {
		// Non-fatal error - user created but role assignment failed
		m.logger.Warn("Failed to assign role to user",
			zap.String("userID", user.ID),
			zap.String("roleID", user.RoleID),
			zap.Error(err),
		)
	}

	return nil
}

func (m *Migrator) addError(result *MigrationResult, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result.Errors = append(result.Errors, err)
}
