package auth

import (
	"context"
)

// Context keys for storing authentication data.
type contextKey string

const (
	// userContextKey is the key for storing the authenticated user in context.
	userContextKey contextKey = "authenticated_user"

	// tenantContextKey is the key for storing the tenant in context.
	tenantContextKey contextKey = "tenant"

	// requestIDContextKey is the key for storing the request ID in context.
	requestIDContextKey contextKey = "request_id"
)

// ContextWithUser adds an authenticated user to the context.
func ContextWithUser(ctx context.Context, user *AuthenticatedUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the authenticated user from the context.
// Returns nil if no user is found in the context.
func UserFromContext(ctx context.Context) *AuthenticatedUser {
	user, ok := ctx.Value(userContextKey).(*AuthenticatedUser)
	if !ok {
		return nil
	}
	return user
}

// ContextWithTenant adds a tenant to the context.
func ContextWithTenant(ctx context.Context, tenant *Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey, tenant)
}

// TenantFromContext retrieves the tenant from the context.
// Returns nil if no tenant is found in the context.
func TenantFromContext(ctx context.Context) *Tenant {
	tenant, ok := ctx.Value(tenantContextKey).(*Tenant)
	if !ok {
		return nil
	}
	return tenant
}

// ContextWithRequestID adds a request ID to the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

// RequestIDFromContext retrieves the request ID from the context.
// Returns an empty string if no request ID is found.
func RequestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDContextKey).(string)
	if !ok {
		return ""
	}
	return requestID
}

// TenantIDFromContext returns the tenant ID from the authenticated user in context.
// Returns an empty string if no user is found or user has no tenant.
func TenantIDFromContext(ctx context.Context) string {
	user := UserFromContext(ctx)
	if user == nil {
		return ""
	}
	return user.TenantID
}

// IsPlatformAdminFromContext checks if the authenticated user is a platform admin.
func IsPlatformAdminFromContext(ctx context.Context) bool {
	user := UserFromContext(ctx)
	if user == nil {
		return false
	}
	return user.IsPlatformAdmin
}

// HasPermissionFromContext checks if the authenticated user has the specified permission.
func HasPermissionFromContext(ctx context.Context, perm Permission) bool {
	user := UserFromContext(ctx)
	if user == nil {
		return false
	}
	return user.HasPermission(perm)
}

// AuthorizeTenantResource implements the fail-closed tenant ownership check
// used by every handler that returns a tenant-owned resource.
//
// It returns true iff the caller is authorized to access a resource whose
// stored TenantID is resourceTenantID. The authorization rules are:
//
//  1. Callers with no authenticated user in context (auth disabled or public
//     route) bypass the check — backwards compatible with non-multi-tenant
//     deployments where tenant filtering is not applicable.
//  2. Platform admins can access any resource regardless of resourceTenantID.
//  3. All other authenticated callers can only access resources whose
//     resourceTenantID exactly matches their own TenantID.
//
// CRITICAL SECURITY PROPERTY: a resource with an empty resourceTenantID is
// NOT accessible to non-platform-admin callers. This closes the bypass
// described in issue #470 (C7) where a short-circuit on
// `resource.TenantID != ""` allowed any caller to access resources that
// adapters or legacy data left without a tenant stamp.
func AuthorizeTenantResource(ctx context.Context, resourceTenantID string) bool {
	user := UserFromContext(ctx)
	if user == nil {
		// No authenticated user — auth middleware is not configured. We
		// cannot enforce tenant isolation without a caller identity, so we
		// fall through. Production deployments that require multi-tenancy
		// must configure auth middleware upstream of these handlers.
		return true
	}
	if user.IsPlatformAdmin {
		return true
	}
	return resourceTenantID != "" && resourceTenantID == user.TenantID
}
