// Package auth provides multi-tenancy and RBAC (Role-Based Access Control) for the O2-IMS Gateway.
// It includes tenant management, user roles, permissions, and middleware for authentication/authorization.
package auth

import (
	"encoding/json"
	"fmt"
	"time"
)

// Permission represents a specific action that can be performed on a resource.
// Permissions follow the format "resource:action" (e.g., "subscriptions:create", "resourcePools:read").
type Permission string

// Predefined permissions for O2-IMS resources.
const (
	// Subscription permissions.
	PermissionSubscriptionRead   Permission = "subscriptions:read"
	PermissionSubscriptionCreate Permission = "subscriptions:create"
	PermissionSubscriptionDelete Permission = "subscriptions:delete"

	// Resource pool permissions.
	PermissionResourcePoolRead   Permission = "resourcePools:read"
	PermissionResourcePoolCreate Permission = "resourcePools:create"
	PermissionResourcePoolUpdate Permission = "resourcePools:update"
	PermissionResourcePoolDelete Permission = "resourcePools:delete"

	// Resource permissions.
	PermissionResourceRead   Permission = "resources:read"
	PermissionResourceCreate Permission = "resources:create"
	PermissionResourceUpdate Permission = "resources:update"
	PermissionResourceDelete Permission = "resources:delete"

	// Resource type permissions.
	PermissionResourceTypeRead Permission = "resourceTypes:read"

	// Deployment manager permissions.
	PermissionDeploymentManagerRead Permission = "deploymentManagers:read"

	// Tenant management permissions (platform-level).
	PermissionTenantRead   Permission = "tenants:read"
	PermissionTenantCreate Permission = "tenants:create"
	PermissionTenantUpdate Permission = "tenants:update"
	PermissionTenantDelete Permission = "tenants:delete"

	// User management permissions (tenant-level).
	PermissionUserRead   Permission = "users:read"
	PermissionUserCreate Permission = "users:create"
	PermissionUserUpdate Permission = "users:update"
	PermissionUserDelete Permission = "users:delete"

	// Role management permissions.
	PermissionRoleRead   Permission = "roles:read"
	PermissionRoleCreate Permission = "roles:create"
	PermissionRoleUpdate Permission = "roles:update"
	PermissionRoleDelete Permission = "roles:delete"

	// Audit log permissions.
	PermissionAuditRead Permission = "audit:read"
)

// RoleType defines the scope of a role.
type RoleType string

const (
	// RoleTypePlatform indicates a platform-level role (cross-tenant).
	RoleTypePlatform RoleType = "platform"

	// RoleTypeTenant indicates a tenant-scoped role.
	RoleTypeTenant RoleType = "tenant"
)

// RoleName represents a predefined role.
type RoleName string

// Predefined roles.
const (
	// Platform-level roles.
	RolePlatformAdmin RoleName = "platform-admin"
	RoleTenantAdmin   RoleName = "tenant-admin"

	// Tenant-scoped roles.
	RoleOwner    RoleName = "owner"
	RoleAdmin    RoleName = "admin"
	RoleOperator RoleName = "operator"
	RoleViewer   RoleName = "viewer"
)

// Role represents a collection of permissions that can be assigned to users.
// Roles can be either platform-level (cross-tenant) or tenant-scoped.
//
// Example:
//
//	role := &Role{
//	    ID:          "role-123",
//	    Name:        "operator",
//	    Type:        RoleTypeTenant,
//	    Permissions: []Permission{PermissionSubscriptionRead, PermissionResourcePoolRead},
//	}
type Role struct {
	// ID is the unique role identifier.
	ID string `json:"roleId"`

	// Name is the human-readable role name.
	Name RoleName `json:"name"`

	// Type indicates whether this is a platform or tenant role.
	Type RoleType `json:"type"`

	// Description provides details about the role.
	Description string `json:"description,omitempty"`

	// Permissions is the list of permissions granted by this role.
	Permissions []Permission `json:"permissions"`

	// TenantID is set for custom tenant-specific roles (empty for global roles).
	TenantID string `json:"tenantId,omitempty"`

	// CreatedAt is the role creation timestamp.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// HasPermission checks if the role includes the specified permission.
func (r *Role) HasPermission(perm Permission) bool {
	for _, p := range r.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// MarshalBinary implements encoding.BinaryMarshaler for Redis storage.
func (r *Role) MarshalBinary() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal role: %w", err)
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler for Redis storage.
func (r *Role) UnmarshalBinary(data []byte) error {
	if err := json.Unmarshal(data, r); err != nil {
		return fmt.Errorf("failed to unmarshal role: %w", err)
	}
	return nil
}

// TenantStatus represents the operational status of a tenant.
type TenantStatus string

const (
	// TenantStatusActive indicates the tenant is operational.
	TenantStatusActive TenantStatus = "active"

	// TenantStatusSuspended indicates the tenant is temporarily disabled.
	TenantStatusSuspended TenantStatus = "suspended"

	// TenantStatusPendingDeletion indicates the tenant is scheduled for deletion.
	TenantStatusPendingDeletion TenantStatus = "pending_deletion"
)

// TenantQuota defines resource limits for a tenant.
type TenantQuota struct {
	// MaxSubscriptions is the maximum number of subscriptions allowed.
	MaxSubscriptions int `json:"maxSubscriptions"`

	// MaxResourcePools is the maximum number of resource pools allowed.
	MaxResourcePools int `json:"maxResourcePools"`

	// MaxDeployments is the maximum number of deployments allowed.
	MaxDeployments int `json:"maxDeployments"`

	// MaxUsers is the maximum number of users allowed.
	MaxUsers int `json:"maxUsers"`

	// MaxRequestsPerMinute is the rate limit for API requests.
	MaxRequestsPerMinute int `json:"maxRequestsPerMinute"`
}

// DefaultQuota returns the default quota for new tenants.
func DefaultQuota() TenantQuota {
	return TenantQuota{
		MaxSubscriptions:     100,
		MaxResourcePools:     50,
		MaxDeployments:       200,
		MaxUsers:             20,
		MaxRequestsPerMinute: 1000,
	}
}

// TenantUsage tracks current resource usage for a tenant.
type TenantUsage struct {
	// Subscriptions is the current number of subscriptions.
	Subscriptions int `json:"subscriptions"`

	// ResourcePools is the current number of resource pools.
	ResourcePools int `json:"resourcePools"`

	// Deployments is the current number of deployments.
	Deployments int `json:"deployments"`

	// Users is the current number of users.
	Users int `json:"users"`
}

// Tenant represents an isolated organizational unit in the gateway.
// Each tenant has its own resources, users, and quotas.
//
// Example:
//
//	tenant := &Tenant{
//	    ID:     "tenant-abc",
//	    Name:   "ACME Corporation",
//	    Status: TenantStatusActive,
//	    Quota:  DefaultQuota(),
//	}
type Tenant struct {
	// ID is the unique tenant identifier.
	ID string `json:"tenantId"`

	// Name is the human-readable tenant name.
	Name string `json:"name"`

	// Description provides details about the tenant.
	Description string `json:"description,omitempty"`

	// Status indicates the operational status of the tenant.
	Status TenantStatus `json:"status"`

	// Quota defines resource limits for the tenant.
	Quota TenantQuota `json:"quota"`

	// Usage tracks current resource usage.
	Usage TenantUsage `json:"usage"`

	// ContactEmail is the primary contact email for the tenant.
	ContactEmail string `json:"contactEmail,omitempty"`

	// Metadata contains additional tenant-specific key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is the tenant creation timestamp.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// IsActive returns true if the tenant is in active status.
func (t *Tenant) IsActive() bool {
	return t.Status == TenantStatusActive
}

// CanCreateSubscription checks if the tenant can create more subscriptions.
func (t *Tenant) CanCreateSubscription() bool {
	return t.IsActive() && t.Usage.Subscriptions < t.Quota.MaxSubscriptions
}

// CanCreateResourcePool checks if the tenant can create more resource pools.
func (t *Tenant) CanCreateResourcePool() bool {
	return t.IsActive() && t.Usage.ResourcePools < t.Quota.MaxResourcePools
}

// CanCreateDeployment checks if the tenant can create more deployments.
func (t *Tenant) CanCreateDeployment() bool {
	return t.IsActive() && t.Usage.Deployments < t.Quota.MaxDeployments
}

// CanAddUser checks if the tenant can add more users.
func (t *Tenant) CanAddUser() bool {
	return t.IsActive() && t.Usage.Users < t.Quota.MaxUsers
}

// MarshalBinary implements encoding.BinaryMarshaler for Redis storage.
func (t *Tenant) MarshalBinary() ([]byte, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tenant: %w", err)
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler for Redis storage.
func (t *Tenant) UnmarshalBinary(data []byte) error {
	if err := json.Unmarshal(data, t); err != nil {
		return fmt.Errorf("failed to unmarshal tenant: %w", err)
	}
	return nil
}

// TenantUser represents a user's association with a tenant and their role.
// A user is identified by their certificate subject (from mTLS).
//
// Example:
//
//	user := &TenantUser{
//	    ID:       "user-123",
//	    TenantID: "tenant-abc",
//	    Subject:  "CN=alice,O=ACME,OU=Engineering",
//	    RoleID:   "role-operator",
//	}
type TenantUser struct {
	// ID is the unique user identifier.
	ID string `json:"userId"`

	// TenantID is the tenant this user belongs to.
	TenantID string `json:"tenantId"`

	// Subject is the certificate subject (CN, O, OU, etc.).
	Subject string `json:"subject"`

	// CommonName is extracted from the certificate CN field.
	CommonName string `json:"commonName"`

	// Email is the user's email (optional, may be in certificate).
	Email string `json:"email,omitempty"`

	// RoleID is the role assigned to this user.
	RoleID string `json:"roleId"`

	// IsActive indicates whether the user is enabled.
	IsActive bool `json:"isActive"`

	// LastLoginAt is the timestamp of the last successful authentication.
	LastLoginAt time.Time `json:"lastLoginAt,omitempty"`

	// CreatedAt is the user creation timestamp.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// MarshalBinary implements encoding.BinaryMarshaler for Redis storage.
func (u *TenantUser) MarshalBinary() ([]byte, error) {
	data, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tenant user: %w", err)
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler for Redis storage.
func (u *TenantUser) UnmarshalBinary(data []byte) error {
	if err := json.Unmarshal(data, u); err != nil {
		return fmt.Errorf("failed to unmarshal tenant user: %w", err)
	}
	return nil
}

// AuthenticatedUser represents the current authenticated user context.
// This is stored in the request context after authentication.
type AuthenticatedUser struct {
	// UserID is the unique user identifier.
	UserID string

	// TenantID is the tenant the user belongs to.
	TenantID string

	// Subject is the certificate subject.
	Subject string

	// CommonName is the certificate CN.
	CommonName string

	// Role is the user's role.
	Role *Role

	// IsPlatformAdmin indicates if this is a platform administrator.
	IsPlatformAdmin bool
}

// HasPermission checks if the authenticated user has the specified permission.
func (u *AuthenticatedUser) HasPermission(perm Permission) bool {
	if u.IsPlatformAdmin {
		return true
	}
	if u.Role == nil {
		return false
	}
	return u.Role.HasPermission(perm)
}

// AuditEventType represents the type of audit event.
type AuditEventType string

const (
	// AuditEventAuthSuccess indicates successful authentication.
	AuditEventAuthSuccess AuditEventType = "auth.success"
	// AuditEventAuthFailure indicates failed authentication.
	AuditEventAuthFailure AuditEventType = "auth.failure"
	// AuditEventAccessDenied indicates access was denied.
	AuditEventAccessDenied AuditEventType = "access.denied"

	// AuditEventTenantCreated indicates a tenant was created.
	AuditEventTenantCreated AuditEventType = "tenant.created"
	// AuditEventTenantUpdated indicates a tenant was updated.
	AuditEventTenantUpdated AuditEventType = "tenant.updated"
	// AuditEventTenantDeleted indicates a tenant was deleted.
	AuditEventTenantDeleted AuditEventType = "tenant.deleted"
	// AuditEventTenantSuspended indicates a tenant was suspended.
	AuditEventTenantSuspended AuditEventType = "tenant.suspended"
	// AuditEventTenantActivated indicates a tenant was activated.
	AuditEventTenantActivated AuditEventType = "tenant.activated"

	// AuditEventUserCreated indicates a user was created.
	AuditEventUserCreated AuditEventType = "user.created"
	// AuditEventUserUpdated indicates a user was updated.
	AuditEventUserUpdated AuditEventType = "user.updated"
	// AuditEventUserDeleted indicates a user was deleted.
	AuditEventUserDeleted AuditEventType = "user.deleted"
	// AuditEventUserEnabled indicates a user was enabled.
	AuditEventUserEnabled AuditEventType = "user.enabled"
	// AuditEventUserDisabled indicates a user was disabled.
	AuditEventUserDisabled AuditEventType = "user.disabled"

	// AuditEventRoleAssigned indicates a role was assigned.
	AuditEventRoleAssigned AuditEventType = "role.assigned"
	// AuditEventRoleRevoked indicates a role was revoked.
	AuditEventRoleRevoked AuditEventType = "role.revoked"
	// AuditEventRolePermissionModified indicates role permissions were modified.
	AuditEventRolePermissionModified AuditEventType = "role.permission.modified"

	// AuditEventResourceCreated indicates a resource was created.
	AuditEventResourceCreated AuditEventType = "resource.created"
	// AuditEventResourceModified indicates a resource was modified.
	AuditEventResourceModified AuditEventType = "resource.modified"
	// AuditEventResourceDeleted indicates a resource was deleted.
	AuditEventResourceDeleted AuditEventType = "resource.deleted"

	// AuditEventResourcePoolCreated indicates a resource pool was created.
	AuditEventResourcePoolCreated AuditEventType = "resourcepool.created"
	// AuditEventResourcePoolModified indicates a resource pool was modified.
	AuditEventResourcePoolModified AuditEventType = "resourcepool.modified"
	// AuditEventResourcePoolDeleted indicates a resource pool was deleted.
	AuditEventResourcePoolDeleted AuditEventType = "resourcepool.deleted"

	// AuditEventDeploymentManagerAccessed indicates a deployment manager was accessed.
	AuditEventDeploymentManagerAccessed AuditEventType = "deploymentmanager.accessed"
	// AuditEventDeploymentManagerModified indicates a deployment manager was modified.
	AuditEventDeploymentManagerModified AuditEventType = "deploymentmanager.modified"

	// AuditEventSubscriptionCreated indicates a subscription was created.
	AuditEventSubscriptionCreated AuditEventType = "subscription.created"
	// AuditEventSubscriptionDeleted indicates a subscription was deleted.
	AuditEventSubscriptionDeleted AuditEventType = "subscription.deleted"
	// AuditEventSubscriptionFilterModified indicates a subscription filter was modified.
	AuditEventSubscriptionFilterModified AuditEventType = "subscription.filter.modified"
	// AuditEventWebhookDeliveryFailed indicates a webhook delivery failed.
	AuditEventWebhookDeliveryFailed AuditEventType = "webhook.delivery.failed"
	// AuditEventSignatureVerificationFailed indicates signature verification failed.
	AuditEventSignatureVerificationFailed AuditEventType = "signature.verification.failed"

	// AuditEventQuotaUpdated indicates a quota was updated.
	AuditEventQuotaUpdated AuditEventType = "quota.updated"
	// AuditEventRateLimitUpdated indicates a rate limit was updated.
	AuditEventRateLimitUpdated AuditEventType = "ratelimit.updated"
	// AuditEventTLSConfigChanged indicates TLS configuration was changed.
	AuditEventTLSConfigChanged AuditEventType = "tls.config.changed"
	// AuditEventSecuritySetting indicates a security setting was changed.
	AuditEventSecuritySetting AuditEventType = "security.setting.changed"

	// AuditEventBulkOperation indicates a bulk administrative operation.
	AuditEventBulkOperation AuditEventType = "admin.bulk.operation"
	// AuditEventTokenRotated indicates an administrative token was rotated.
	AuditEventTokenRotated AuditEventType = "admin.token.rotated"
	// AuditEventConfigExport indicates configuration was exported.
	AuditEventConfigExport AuditEventType = "admin.config.export"
	// AuditEventAuditExport indicates audit logs were exported.
	AuditEventAuditExport AuditEventType = "admin.audit.export"
)

// AuditEvent represents a logged security or administrative event.
type AuditEvent struct {
	// ID is the unique event identifier.
	ID string `json:"eventId"`

	// Type is the type of audit event.
	Type AuditEventType `json:"type"`

	// TenantID is the tenant associated with the event (if applicable).
	TenantID string `json:"tenantId,omitempty"`

	// UserID is the user who triggered the event.
	UserID string `json:"userId,omitempty"`

	// Subject is the certificate subject of the actor.
	Subject string `json:"subject,omitempty"`

	// ResourceType is the type of resource affected.
	ResourceType string `json:"resourceType,omitempty"`

	// ResourceID is the ID of the resource affected.
	ResourceID string `json:"resourceId,omitempty"`

	// Action describes the action performed.
	Action string `json:"action"`

	// Details contains additional event-specific information.
	Details map[string]string `json:"details,omitempty"`

	// ClientIP is the IP address of the client.
	ClientIP string `json:"clientIp,omitempty"`

	// UserAgent is the client's user agent string.
	UserAgent string `json:"userAgent,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
}

// MarshalBinary implements encoding.BinaryMarshaler for Redis storage.
func (e *AuditEvent) MarshalBinary() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit event: %w", err)
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler for Redis storage.
func (e *AuditEvent) UnmarshalBinary(data []byte) error {
	if err := json.Unmarshal(data, e); err != nil {
		return fmt.Errorf("failed to unmarshal audit event: %w", err)
	}
	return nil
}

// GetDefaultRoles returns the predefined system roles.
func GetDefaultRoles() []*Role {
	return []*Role{
		{
			ID:          "role-platform-admin",
			Name:        RolePlatformAdmin,
			Type:        RoleTypePlatform,
			Description: "Full platform access across all tenants",
			Permissions: []Permission{
				PermissionTenantRead, PermissionTenantCreate, PermissionTenantUpdate, PermissionTenantDelete,
				PermissionUserRead, PermissionUserCreate, PermissionUserUpdate, PermissionUserDelete,
				PermissionRoleRead, PermissionRoleCreate, PermissionRoleUpdate, PermissionRoleDelete,
				PermissionSubscriptionRead, PermissionSubscriptionCreate, PermissionSubscriptionDelete,
				PermissionResourcePoolRead, PermissionResourcePoolCreate, PermissionResourcePoolUpdate, PermissionResourcePoolDelete,
				PermissionResourceRead, PermissionResourceCreate, PermissionResourceUpdate, PermissionResourceDelete,
				PermissionResourceTypeRead,
				PermissionDeploymentManagerRead,
				PermissionAuditRead,
			},
		},
		{
			ID:          "role-tenant-admin",
			Name:        RoleTenantAdmin,
			Type:        RoleTypePlatform,
			Description: "Administrative access for tenant management",
			Permissions: []Permission{
				PermissionTenantRead, PermissionTenantCreate, PermissionTenantUpdate,
				PermissionUserRead, PermissionUserCreate, PermissionUserUpdate, PermissionUserDelete,
				PermissionAuditRead,
			},
		},
		{
			ID:          "role-owner",
			Name:        RoleOwner,
			Type:        RoleTypeTenant,
			Description: "Full access within a tenant",
			Permissions: []Permission{
				PermissionUserRead, PermissionUserCreate, PermissionUserUpdate, PermissionUserDelete,
				PermissionRoleRead,
				PermissionSubscriptionRead, PermissionSubscriptionCreate, PermissionSubscriptionDelete,
				PermissionResourcePoolRead, PermissionResourcePoolCreate, PermissionResourcePoolUpdate, PermissionResourcePoolDelete,
				PermissionResourceRead, PermissionResourceCreate, PermissionResourceUpdate, PermissionResourceDelete,
				PermissionResourceTypeRead,
				PermissionDeploymentManagerRead,
				PermissionAuditRead,
			},
		},
		{
			ID:          "role-admin",
			Name:        RoleAdmin,
			Type:        RoleTypeTenant,
			Description: "Administrative access within a tenant",
			Permissions: []Permission{
				PermissionUserRead, PermissionUserCreate, PermissionUserUpdate,
				PermissionRoleRead,
				PermissionSubscriptionRead, PermissionSubscriptionCreate, PermissionSubscriptionDelete,
				PermissionResourcePoolRead, PermissionResourcePoolCreate, PermissionResourcePoolUpdate, PermissionResourcePoolDelete,
				PermissionResourceRead, PermissionResourceCreate, PermissionResourceUpdate, PermissionResourceDelete,
				PermissionResourceTypeRead,
				PermissionDeploymentManagerRead,
			},
		},
		{
			ID:          "role-operator",
			Name:        RoleOperator,
			Type:        RoleTypeTenant,
			Description: "Operational access for resource management",
			Permissions: []Permission{
				PermissionSubscriptionRead, PermissionSubscriptionCreate, PermissionSubscriptionDelete,
				PermissionResourcePoolRead,
				PermissionResourceRead, PermissionResourceCreate, PermissionResourceUpdate,
				PermissionResourceTypeRead,
				PermissionDeploymentManagerRead,
			},
		},
		{
			ID:          "role-viewer",
			Name:        RoleViewer,
			Type:        RoleTypeTenant,
			Description: "Read-only access to resources",
			Permissions: []Permission{
				PermissionSubscriptionRead,
				PermissionResourcePoolRead,
				PermissionResourceRead,
				PermissionResourceTypeRead,
				PermissionDeploymentManagerRead,
			},
		},
	}
}
