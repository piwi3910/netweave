package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// pgUniqueViolation is the PostgreSQL error code for unique constraint violations.
const pgUniqueViolation = "23505"

// PostgresStore implements the auth.Store interface using PostgreSQL as the backend.
// It uses sqlc-generated queries for type-safe database access.
//
// Example:
//
//	db, _ := database.New(ctx, cfg, password)
//	store := NewPostgresStore(db)
//	defer store.Close()
type PostgresStore struct {
	db      *database.DB
	queries *dbsqlc.Queries
}

// NewPostgresStore creates a new PostgresStore backed by the given database connection.
func NewPostgresStore(db *database.DB) *PostgresStore {
	return &PostgresStore{
		db:      db,
		queries: dbsqlc.New(db.Pool),
	}
}

// Close closes the PostgreSQL connection pool and releases resources.
func (s *PostgresStore) Close() error {
	s.db.Close()
	return nil
}

// Ping checks if PostgreSQL is available.
func (s *PostgresStore) Ping(ctx context.Context) error {
	if err := s.db.Ping(ctx); err != nil {
		return fmt.Errorf("%w: %s", ErrStorageUnavailable, err.Error())
	}
	return nil
}

// --- TenantStore ---

// CreateTenant creates a new tenant.
// Returns ErrTenantExists if a tenant with the same ID already exists.
func (s *PostgresStore) CreateTenant(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == "" {
		return ErrInvalidTenantID
	}

	now := time.Now().UTC()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	quotaJSON, err := json.Marshal(tenant.Quota)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant quota: %w", err)
	}

	usageJSON, err := json.Marshal(tenant.Usage)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant usage: %w", err)
	}

	metadataJSON, err := marshalStringMap(tenant.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant metadata: %w", err)
	}

	err = s.queries.CreateTenant(ctx, dbsqlc.CreateTenantParams{
		ID:           tenant.ID,
		Name:         tenant.Name,
		Description:  tenant.Description,
		Status:       string(tenant.Status),
		Quota:        quotaJSON,
		Usage:        usageJSON,
		ContactEmail: tenant.ContactEmail,
		Metadata:     metadataJSON,
		CreatedAt:    tenant.CreatedAt,
		UpdatedAt:    tenant.UpdatedAt,
	})
	if err != nil {
		if isPgUniqueViolation(err) {
			return ErrTenantExists
		}
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

// GetTenant retrieves a tenant by ID.
// Returns ErrTenantNotFound if the tenant does not exist.
func (s *PostgresStore) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	if id == "" {
		return nil, ErrInvalidTenantID
	}

	row, err := s.queries.GetTenant(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return dbsqlcTenantToModel(&row)
}

// UpdateTenant updates an existing tenant.
// Returns ErrTenantNotFound if the tenant does not exist.
func (s *PostgresStore) UpdateTenant(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == "" {
		return ErrInvalidTenantID
	}

	exists, err := s.queries.TenantExists(ctx, tenant.ID)
	if err != nil {
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if !exists {
		return ErrTenantNotFound
	}

	tenant.UpdatedAt = time.Now().UTC()

	quotaJSON, err := json.Marshal(tenant.Quota)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant quota: %w", err)
	}

	usageJSON, err := json.Marshal(tenant.Usage)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant usage: %w", err)
	}

	metadataJSON, err := marshalStringMap(tenant.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant metadata: %w", err)
	}

	err = s.queries.UpdateTenant(ctx, dbsqlc.UpdateTenantParams{
		ID:           tenant.ID,
		Name:         tenant.Name,
		Description:  tenant.Description,
		Status:       string(tenant.Status),
		Quota:        quotaJSON,
		Usage:        usageJSON,
		ContactEmail: tenant.ContactEmail,
		Metadata:     metadataJSON,
		UpdatedAt:    tenant.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	return nil
}

// DeleteTenant deletes a tenant by ID.
// Returns ErrTenantNotFound if the tenant does not exist.
func (s *PostgresStore) DeleteTenant(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidTenantID
	}

	rowsAffected, err := s.queries.DeleteTenant(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	if rowsAffected == 0 {
		return ErrTenantNotFound
	}

	return nil
}

// ListTenants retrieves all tenants.
// Returns an empty slice if no tenants exist.
func (s *PostgresStore) ListTenants(ctx context.Context) ([]*Tenant, error) {
	rows, err := s.queries.ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	return dbsqlcTenantsToModels(rows)
}

// IncrementUsage atomically increments a usage counter for a tenant.
func (s *PostgresStore) IncrementUsage(ctx context.Context, tenantID, usageType string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	exists, err := s.queries.TenantExists(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if !exists {
		return ErrTenantNotFound
	}

	err = s.queries.IncrementTenantUsage(ctx, dbsqlc.IncrementTenantUsageParams{
		ID:      tenantID,
		Column2: usageType,
	})
	if err != nil {
		return fmt.Errorf("failed to increment tenant usage: %w", err)
	}

	return nil
}

// DecrementUsage atomically decrements a usage counter for a tenant.
func (s *PostgresStore) DecrementUsage(ctx context.Context, tenantID, usageType string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	exists, err := s.queries.TenantExists(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if !exists {
		return ErrTenantNotFound
	}

	err = s.queries.DecrementTenantUsage(ctx, dbsqlc.DecrementTenantUsageParams{
		ID:      tenantID,
		Column2: usageType,
	})
	if err != nil {
		return fmt.Errorf("failed to decrement tenant usage: %w", err)
	}

	return nil
}

// --- UserStore ---

// CreateUser creates a new user.
// Returns ErrUserExists if a user with the same ID already exists.
func (s *PostgresStore) CreateUser(ctx context.Context, user *TenantUser) error {
	if user.ID == "" {
		return ErrInvalidUserID
	}

	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	err := s.queries.CreateUser(ctx, dbsqlc.CreateUserParams{
		ID:            user.ID,
		TenantID:      user.TenantID,
		Subject:       user.Subject,
		CommonName:    user.CommonName,
		Email:         user.Email,
		OauthSubject:  user.OAuthSubject,
		OauthProvider: user.OAuthProvider,
		RoleID:        user.RoleID,
		IsActive:      user.IsActive,
		LastLoginAt:   timeToTimestamptz(user.LastLoginAt),
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	})
	if err != nil {
		if isPgUniqueViolation(err) {
			return ErrUserExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID.
// Returns ErrUserNotFound if the user does not exist.
func (s *PostgresStore) GetUser(ctx context.Context, id string) (*TenantUser, error) {
	if id == "" {
		return nil, ErrInvalidUserID
	}

	row, err := s.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return dbsqlcUserToModel(&row), nil
}

// GetUserBySubject retrieves a user by certificate subject (mTLS).
// Returns ErrUserNotFound if no matching user exists.
func (s *PostgresStore) GetUserBySubject(ctx context.Context, subject string) (*TenantUser, error) {
	row, err := s.queries.GetUserBySubject(ctx, subject)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by subject: %w", err)
	}

	return dbsqlcUserToModel(&row), nil
}

// GetUserByOAuthSubject retrieves a user by OAuth2 subject identifier.
// Returns ErrUserNotFound if no matching user exists.
func (s *PostgresStore) GetUserByOAuthSubject(ctx context.Context, oauthSubject string) (*TenantUser, error) {
	row, err := s.queries.GetUserByOAuthSubject(ctx, oauthSubject)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by OAuth subject: %w", err)
	}

	return dbsqlcUserToModel(&row), nil
}

// GetUserByEmail retrieves a user by email address.
// Returns ErrUserNotFound if no matching user exists.
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*TenantUser, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return dbsqlcUserToModel(&row), nil
}

// UpdateUser updates an existing user.
// Returns ErrUserNotFound if the user does not exist.
func (s *PostgresStore) UpdateUser(ctx context.Context, user *TenantUser) error {
	if user.ID == "" {
		return ErrInvalidUserID
	}

	exists, err := s.queries.UserExists(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if !exists {
		return ErrUserNotFound
	}

	user.UpdatedAt = time.Now().UTC()

	err = s.queries.UpdateUser(ctx, dbsqlc.UpdateUserParams{
		ID:            user.ID,
		TenantID:      user.TenantID,
		Subject:       user.Subject,
		CommonName:    user.CommonName,
		Email:         user.Email,
		OauthSubject:  user.OAuthSubject,
		OauthProvider: user.OAuthProvider,
		RoleID:        user.RoleID,
		IsActive:      user.IsActive,
		UpdatedAt:     user.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// DeleteUser deletes a user by ID.
// Returns ErrUserNotFound if the user does not exist.
func (s *PostgresStore) DeleteUser(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidUserID
	}

	rowsAffected, err := s.queries.DeleteUser(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if rowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// ListUsersByTenant retrieves all users for a tenant.
// Returns an empty slice if no users exist.
func (s *PostgresStore) ListUsersByTenant(ctx context.Context, tenantID string) ([]*TenantUser, error) {
	rows, err := s.queries.ListUsersByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list users by tenant: %w", err)
	}

	return dbsqlcUsersToModels(rows), nil
}

// UpdateLastLogin updates the last login timestamp for a user.
func (s *PostgresStore) UpdateLastLogin(ctx context.Context, userID string) error {
	if userID == "" {
		return ErrInvalidUserID
	}

	err := s.queries.UpdateLastLogin(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// --- RoleStore ---

// CreateRole creates a new role.
// Returns ErrRoleExists if a role with the same ID already exists.
func (s *PostgresStore) CreateRole(ctx context.Context, role *Role) error {
	if role.ID == "" {
		return ErrInvalidRoleID
	}

	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now

	permsJSON, err := marshalPermissions(role.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal role permissions: %w", err)
	}

	err = s.queries.CreateRole(ctx, dbsqlc.CreateRoleParams{
		ID:          role.ID,
		Name:        string(role.Name),
		Type:        string(role.Type),
		Description: role.Description,
		Permissions: permsJSON,
		TenantID:    role.TenantID,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	})
	if err != nil {
		if isPgUniqueViolation(err) {
			return ErrRoleExists
		}
		return fmt.Errorf("failed to create role: %w", err)
	}

	return nil
}

// GetRole retrieves a role by ID.
// Returns ErrRoleNotFound if the role does not exist.
func (s *PostgresStore) GetRole(ctx context.Context, id string) (*Role, error) {
	if id == "" {
		return nil, ErrInvalidRoleID
	}

	row, err := s.queries.GetRole(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return dbsqlcRoleToModel(&row)
}

// GetRoleByName retrieves a role by name.
// Returns ErrRoleNotFound if no matching role exists.
func (s *PostgresStore) GetRoleByName(ctx context.Context, name RoleName) (*Role, error) {
	row, err := s.queries.GetRoleByName(ctx, string(name))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get role by name: %w", err)
	}

	return dbsqlcRoleToModel(&row)
}

// UpdateRole updates an existing role.
// Returns ErrRoleNotFound if the role does not exist.
func (s *PostgresStore) UpdateRole(ctx context.Context, role *Role) error {
	if role.ID == "" {
		return ErrInvalidRoleID
	}

	exists, err := s.queries.RoleExists(ctx, role.ID)
	if err != nil {
		return fmt.Errorf("failed to check role existence: %w", err)
	}
	if !exists {
		return ErrRoleNotFound
	}

	role.UpdatedAt = time.Now().UTC()

	permsJSON, err := marshalPermissions(role.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal role permissions: %w", err)
	}

	err = s.queries.UpdateRole(ctx, dbsqlc.UpdateRoleParams{
		ID:          role.ID,
		Name:        string(role.Name),
		Type:        string(role.Type),
		Description: role.Description,
		Permissions: permsJSON,
		TenantID:    role.TenantID,
		UpdatedAt:   role.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	return nil
}

// DeleteRole deletes a role by ID.
// Returns ErrRoleNotFound if the role does not exist.
func (s *PostgresStore) DeleteRole(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidRoleID
	}

	rowsAffected, err := s.queries.DeleteRole(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	if rowsAffected == 0 {
		return ErrRoleNotFound
	}

	return nil
}

// ListRoles retrieves all roles.
// Returns an empty slice if no roles exist.
func (s *PostgresStore) ListRoles(ctx context.Context) ([]*Role, error) {
	rows, err := s.queries.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}

	return dbsqlcRolesToModels(rows)
}

// ListRolesByTenant retrieves roles for a specific tenant.
// Includes both global roles and tenant-specific custom roles.
func (s *PostgresStore) ListRolesByTenant(ctx context.Context, tenantID string) ([]*Role, error) {
	rows, err := s.queries.ListRolesByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles by tenant: %w", err)
	}

	return dbsqlcRolesToModels(rows)
}

// InitializeDefaultRoles creates the default system roles if they don't exist.
func (s *PostgresStore) InitializeDefaultRoles(ctx context.Context) error {
	defaults := GetDefaultRoles()

	for _, role := range defaults {
		exists, err := s.queries.RoleExists(ctx, role.ID)
		if err != nil {
			return fmt.Errorf("failed to check default role existence %s: %w", role.ID, err)
		}
		if exists {
			continue
		}

		if err := s.CreateRole(ctx, role); err != nil {
			if errors.Is(err, ErrRoleExists) {
				continue
			}
			return fmt.Errorf("failed to create default role %s: %w", role.ID, err)
		}
	}

	return nil
}

// --- AuditStore ---

// LogEvent creates a new audit event.
func (s *PostgresStore) LogEvent(ctx context.Context, event *AuditEvent) error {
	detailsJSON, err := marshalStringMap(event.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event details: %w", err)
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	err = s.queries.InsertAuditEvent(ctx, dbsqlc.InsertAuditEventParams{
		ID:           event.ID,
		Type:         string(event.Type),
		TenantID:     event.TenantID,
		UserID:       event.UserID,
		Subject:      event.Subject,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Action:       event.Action,
		Details:      detailsJSON,
		ClientIp:     event.ClientIP,
		UserAgent:    event.UserAgent,
		Timestamp:    event.Timestamp,
	})
	if err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	return nil
}

// ListEvents retrieves audit events with optional filtering.
// If tenantID is empty, returns events for all tenants.
func (s *PostgresStore) ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]*AuditEvent, error) {
	rows, err := s.queries.ListAuditEvents(ctx, dbsqlc.ListAuditEventsParams{
		Limit:        safeIntToInt32(limit),
		Offset:       safeIntToInt32(offset),
		TenantFilter: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}

	return dbsqlcAuditEventsToModels(rows)
}

// ListEventsByType retrieves audit events of a specific type.
func (s *PostgresStore) ListEventsByType(ctx context.Context, eventType AuditEventType, limit int) ([]*AuditEvent, error) {
	rows, err := s.queries.ListAuditEventsByType(ctx, dbsqlc.ListAuditEventsByTypeParams{
		Type:  string(eventType),
		Limit: safeIntToInt32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events by type: %w", err)
	}

	return dbsqlcAuditEventsToModels(rows)
}

// ListEventsByUser retrieves audit events for a specific user.
func (s *PostgresStore) ListEventsByUser(ctx context.Context, userID string, limit int) ([]*AuditEvent, error) {
	rows, err := s.queries.ListAuditEventsByUser(ctx, dbsqlc.ListAuditEventsByUserParams{
		UserID: userID,
		Limit:  safeIntToInt32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events by user: %w", err)
	}

	return dbsqlcAuditEventsToModels(rows)
}

// --- Conversion helpers ---

// dbsqlcTenantToModel converts a sqlc Tenant to an auth Tenant.
func dbsqlcTenantToModel(row *dbsqlc.Tenant) (*Tenant, error) {
	var quota TenantQuota
	if err := json.Unmarshal(row.Quota, &quota); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tenant quota: %w", err)
	}

	var usage TenantUsage
	if err := json.Unmarshal(row.Usage, &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tenant usage: %w", err)
	}

	var metadata map[string]string
	if len(row.Metadata) > 0 && string(row.Metadata) != "{}" {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tenant metadata: %w", err)
		}
	}

	return &Tenant{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		Status:       TenantStatus(row.Status),
		Quota:        quota,
		Usage:        usage,
		ContactEmail: row.ContactEmail,
		Metadata:     metadata,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

// dbsqlcTenantsToModels converts a slice of sqlc Tenants to auth Tenants.
func dbsqlcTenantsToModels(rows []dbsqlc.Tenant) ([]*Tenant, error) {
	tenants := make([]*Tenant, 0, len(rows))
	for i := range rows {
		tenant, err := dbsqlcTenantToModel(&rows[i])
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, tenant)
	}
	return tenants, nil
}

// dbsqlcUserToModel converts a sqlc User to an auth TenantUser.
func dbsqlcUserToModel(row *dbsqlc.User) *TenantUser {
	user := &TenantUser{
		ID:            row.ID,
		TenantID:      row.TenantID,
		Subject:       row.Subject,
		CommonName:    row.CommonName,
		Email:         row.Email,
		OAuthSubject:  row.OauthSubject,
		OAuthProvider: row.OauthProvider,
		RoleID:        row.RoleID,
		IsActive:      row.IsActive,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.LastLoginAt.Valid {
		user.LastLoginAt = row.LastLoginAt.Time
	}
	return user
}

// dbsqlcUsersToModels converts a slice of sqlc Users to auth TenantUsers.
func dbsqlcUsersToModels(rows []dbsqlc.User) []*TenantUser {
	users := make([]*TenantUser, 0, len(rows))
	for i := range rows {
		users = append(users, dbsqlcUserToModel(&rows[i]))
	}
	return users
}

// dbsqlcRoleToModel converts a sqlc Role to an auth Role.
func dbsqlcRoleToModel(row *dbsqlc.Role) (*Role, error) {
	perms, err := unmarshalPermissions(row.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal role permissions: %w", err)
	}

	return &Role{
		ID:          row.ID,
		Name:        RoleName(row.Name),
		Type:        RoleType(row.Type),
		Description: row.Description,
		Permissions: perms,
		TenantID:    row.TenantID,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

// dbsqlcRolesToModels converts a slice of sqlc Roles to auth Roles.
func dbsqlcRolesToModels(rows []dbsqlc.Role) ([]*Role, error) {
	roles := make([]*Role, 0, len(rows))
	for i := range rows {
		role, err := dbsqlcRoleToModel(&rows[i])
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// dbsqlcAuditEventToModel converts a sqlc AuditEvent to an auth AuditEvent.
func dbsqlcAuditEventToModel(row *dbsqlc.AuditEvent) (*AuditEvent, error) {
	var details map[string]string
	if len(row.Details) > 0 && string(row.Details) != "{}" && string(row.Details) != "null" {
		if err := json.Unmarshal(row.Details, &details); err != nil {
			return nil, fmt.Errorf("failed to unmarshal audit event details: %w", err)
		}
	}

	return &AuditEvent{
		ID:           row.ID,
		Type:         AuditEventType(row.Type),
		TenantID:     row.TenantID,
		UserID:       row.UserID,
		Subject:      row.Subject,
		ResourceType: row.ResourceType,
		ResourceID:   row.ResourceID,
		Action:       row.Action,
		Details:      details,
		ClientIP:     row.ClientIp,
		UserAgent:    row.UserAgent,
		Timestamp:    row.Timestamp,
	}, nil
}

// dbsqlcAuditEventsToModels converts a slice of sqlc AuditEvents to auth AuditEvents.
func dbsqlcAuditEventsToModels(rows []dbsqlc.AuditEvent) ([]*AuditEvent, error) {
	events := make([]*AuditEvent, 0, len(rows))
	for i := range rows {
		event, err := dbsqlcAuditEventToModel(&rows[i])
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

// --- JSON helpers ---

// marshalPermissions converts a Permission slice to JSON.
func marshalPermissions(perms []Permission) (json.RawMessage, error) {
	strs := make([]string, len(perms))
	for i, p := range perms {
		strs[i] = string(p)
	}
	data, err := json.Marshal(strs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}
	return data, nil
}

// unmarshalPermissions converts JSON to a Permission slice.
func unmarshalPermissions(data json.RawMessage) ([]Permission, error) {
	var strs []string
	if err := json.Unmarshal(data, &strs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
	}
	perms := make([]Permission, len(strs))
	for i, s := range strs {
		perms[i] = Permission(s)
	}
	return perms, nil
}

// marshalStringMap converts a map[string]string to JSON, defaulting to "{}".
func marshalStringMap(m map[string]string) (json.RawMessage, error) {
	if m == nil {
		return json.RawMessage("{}"), nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal string map: %w", err)
	}
	return data, nil
}

// timeToTimestamptz converts a time.Time to pgtype.Timestamptz.
// Zero time results in an invalid (NULL) timestamp.
func timeToTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// safeIntToInt32 safely converts int to int32 with clamping at math.MaxInt32.
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// isPgUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	return false
}
