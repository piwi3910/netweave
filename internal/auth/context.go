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
