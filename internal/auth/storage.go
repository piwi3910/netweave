package auth

import (
	"context"
	"errors"
)

// Common sentinel errors for auth storage operations.
var (
	// ErrTenantNotFound is returned when a tenant does not exist.
	ErrTenantNotFound = errors.New("tenant not found")

	// ErrTenantExists is returned when attempting to create a duplicate tenant.
	ErrTenantExists = errors.New("tenant already exists")

	// ErrTenantSuspended is returned when operating on a suspended tenant.
	ErrTenantSuspended = errors.New("tenant is suspended")

	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = errors.New("user not found")

	// ErrUserExists is returned when attempting to create a duplicate user.
	ErrUserExists = errors.New("user already exists")

	// ErrRoleNotFound is returned when a role does not exist.
	ErrRoleNotFound = errors.New("role not found")

	// ErrRoleExists is returned when attempting to create a duplicate role.
	ErrRoleExists = errors.New("role already exists")

	// ErrQuotaExceeded is returned when a tenant exceeds resource quotas.
	ErrQuotaExceeded = errors.New("tenant quota exceeded")

	// ErrInvalidTenantID is returned when a tenant ID is empty or invalid.
	ErrInvalidTenantID = errors.New("invalid tenant ID")

	// ErrInvalidUserID is returned when a user ID is empty or invalid.
	ErrInvalidUserID = errors.New("invalid user ID")

	// ErrInvalidRoleID is returned when a role ID is empty or invalid.
	ErrInvalidRoleID = errors.New("invalid role ID")

	// ErrStorageUnavailable is returned when the storage backend is unavailable.
	ErrStorageUnavailable = errors.New("storage backend unavailable")
)

// TenantStore defines the interface for tenant storage operations.
// Implementations must be safe for concurrent use.
type TenantStore interface {
	// CreateTenant creates a new tenant.
	// Returns ErrTenantExists if a tenant with the same ID already exists.
	CreateTenant(ctx context.Context, tenant *Tenant) error

	// GetTenant retrieves a tenant by ID.
	// Returns ErrTenantNotFound if the tenant does not exist.
	GetTenant(ctx context.Context, id string) (*Tenant, error)

	// UpdateTenant updates an existing tenant.
	// Returns ErrTenantNotFound if the tenant does not exist.
	UpdateTenant(ctx context.Context, tenant *Tenant) error

	// DeleteTenant deletes a tenant by ID.
	// Returns ErrTenantNotFound if the tenant does not exist.
	DeleteTenant(ctx context.Context, id string) error

	// ListTenants retrieves all tenants.
	// Returns an empty slice if no tenants exist.
	ListTenants(ctx context.Context) ([]*Tenant, error)

	// IncrementUsage atomically increments a usage counter for a tenant.
	// usageType can be "subscriptions", "resourcePools", "deployments", or "users".
	IncrementUsage(ctx context.Context, tenantID, usageType string) error

	// DecrementUsage atomically decrements a usage counter for a tenant.
	DecrementUsage(ctx context.Context, tenantID, usageType string) error
}

// UserStore defines the interface for tenant user storage operations.
// Implementations must be safe for concurrent use.
type UserStore interface {
	// CreateUser creates a new user.
	// Returns ErrUserExists if a user with the same ID already exists.
	CreateUser(ctx context.Context, user *TenantUser) error

	// GetUser retrieves a user by ID.
	// Returns ErrUserNotFound if the user does not exist.
	GetUser(ctx context.Context, id string) (*TenantUser, error)

	// GetUserBySubject retrieves a user by certificate subject.
	// Returns ErrUserNotFound if no matching user exists.
	GetUserBySubject(ctx context.Context, subject string) (*TenantUser, error)

	// UpdateUser updates an existing user.
	// Returns ErrUserNotFound if the user does not exist.
	UpdateUser(ctx context.Context, user *TenantUser) error

	// DeleteUser deletes a user by ID.
	// Returns ErrUserNotFound if the user does not exist.
	DeleteUser(ctx context.Context, id string) error

	// ListUsersByTenant retrieves all users for a tenant.
	// Returns an empty slice if no users exist.
	ListUsersByTenant(ctx context.Context, tenantID string) ([]*TenantUser, error)

	// UpdateLastLogin updates the last login timestamp for a user.
	UpdateLastLogin(ctx context.Context, userID string) error
}

// RoleStore defines the interface for role storage operations.
// Implementations must be safe for concurrent use.
type RoleStore interface {
	// CreateRole creates a new role.
	// Returns ErrRoleExists if a role with the same ID already exists.
	CreateRole(ctx context.Context, role *Role) error

	// GetRole retrieves a role by ID.
	// Returns ErrRoleNotFound if the role does not exist.
	GetRole(ctx context.Context, id string) (*Role, error)

	// GetRoleByName retrieves a role by name.
	// Returns ErrRoleNotFound if no matching role exists.
	GetRoleByName(ctx context.Context, name RoleName) (*Role, error)

	// UpdateRole updates an existing role.
	// Returns ErrRoleNotFound if the role does not exist.
	UpdateRole(ctx context.Context, role *Role) error

	// DeleteRole deletes a role by ID.
	// Returns ErrRoleNotFound if the role does not exist.
	DeleteRole(ctx context.Context, id string) error

	// ListRoles retrieves all roles.
	// Returns an empty slice if no roles exist.
	ListRoles(ctx context.Context) ([]*Role, error)

	// ListRolesByTenant retrieves roles for a specific tenant.
	// Includes both global roles and tenant-specific custom roles.
	ListRolesByTenant(ctx context.Context, tenantID string) ([]*Role, error)

	// InitializeDefaultRoles creates the default system roles if they don't exist.
	InitializeDefaultRoles(ctx context.Context) error
}

// AuditStore defines the interface for audit log storage operations.
// Implementations must be safe for concurrent use.
type AuditStore interface {
	// LogEvent creates a new audit event.
	LogEvent(ctx context.Context, event *AuditEvent) error

	// ListEvents retrieves audit events with optional filtering.
	// If tenantID is empty, returns events for all tenants (platform admin only).
	// limit specifies the maximum number of events to return.
	// offset specifies the number of events to skip for pagination.
	ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]*AuditEvent, error)

	// ListEventsByType retrieves audit events of a specific type.
	ListEventsByType(ctx context.Context, eventType AuditEventType, limit int) ([]*AuditEvent, error)

	// ListEventsByUser retrieves audit events for a specific user.
	ListEventsByUser(ctx context.Context, userID string, limit int) ([]*AuditEvent, error)
}

// Store combines all auth storage interfaces.
type Store interface {
	TenantStore
	UserStore
	RoleStore
	AuditStore

	// Close closes the storage connection and releases resources.
	Close() error

	// Ping checks if the storage backend is available.
	Ping(ctx context.Context) error
}
