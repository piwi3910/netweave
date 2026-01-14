package auth_test

import (
	"context"
	"testing"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/stretchr/testify/assert"
)

func TestContextWithUser(t *testing.T) {
	user := &auth.AuthenticatedUser{
		UserID:          "user-123",
		TenantID:        "tenant-456",
		Subject:         "CN=test,O=Org",
		CommonName:      "test",
		IsPlatformAdmin: false,
		Role: &auth.Role{
			Name:        auth.RoleOperator,
			Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
		},
	}

	ctx := context.Background()
	ctx = auth.ContextWithUser(ctx, user)

	// Retrieve user from context.
	retrieved := auth.UserFromContext(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, user.UserID, retrieved.UserID)
	assert.Equal(t, user.TenantID, retrieved.TenantID)
	assert.Equal(t, user.Subject, retrieved.Subject)
}

func TestUserFromContext_Nil(t *testing.T) {
	ctx := context.Background()

	// No user in context.
	retrieved := auth.UserFromContext(ctx)
	assert.Nil(t, retrieved)
}

func TestContextWithTenant(t *testing.T) {
	tenant := &auth.Tenant{
		ID:     "tenant-789",
		Name:   "Test auth.Tenant",
		Status: auth.TenantStatusActive,
	}

	ctx := context.Background()
	ctx = auth.ContextWithTenant(ctx, tenant)

	// Retrieve tenant from context.
	retrieved := auth.TenantFromContext(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, tenant.ID, retrieved.ID)
	assert.Equal(t, tenant.Name, retrieved.Name)
}

func TestTenantFromContext_Nil(t *testing.T) {
	ctx := context.Background()

	// No tenant in context.
	retrieved := auth.TenantFromContext(ctx)
	assert.Nil(t, retrieved)
}

func TestContextWithRequestID(t *testing.T) {
	requestID := "req-12345-abcde"

	ctx := context.Background()
	ctx = auth.ContextWithRequestID(ctx, requestID)

	// Retrieve request ID from context.
	retrieved := auth.RequestIDFromContext(ctx)
	assert.Equal(t, requestID, retrieved)
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	// No request ID in context.
	retrieved := auth.RequestIDFromContext(ctx)
	assert.Empty(t, retrieved)
}

func TestTenantIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		user     *auth.AuthenticatedUser
		expected string
	}{
		{
			name: "user with tenant",
			user: &auth.AuthenticatedUser{
				UserID:   "user-1",
				TenantID: "tenant-abc",
			},
			expected: "tenant-abc",
		},
		{
			name:     "no user in context",
			user:     nil,
			expected: "",
		},
		{
			name: "user with empty tenant",
			user: &auth.AuthenticatedUser{
				UserID:   "user-2",
				TenantID: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = auth.ContextWithUser(ctx, tt.user)
			}

			result := auth.TenantIDFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsPlatformAdminFromContext(t *testing.T) {
	tests := []struct {
		name     string
		user     *auth.AuthenticatedUser
		expected bool
	}{
		{
			name: "platform admin",
			user: &auth.AuthenticatedUser{
				UserID:          "admin-1",
				IsPlatformAdmin: true,
			},
			expected: true,
		},
		{
			name: "regular user",
			user: &auth.AuthenticatedUser{
				UserID:          "user-1",
				IsPlatformAdmin: false,
			},
			expected: false,
		},
		{
			name:     "no user in context",
			user:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = auth.ContextWithUser(ctx, tt.user)
			}

			result := auth.IsPlatformAdminFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasPermissionFromContext(t *testing.T) {
	tests := []struct {
		name       string
		user       *auth.AuthenticatedUser
		permission auth.Permission
		expected   bool
	}{
		{
			name: "user has permission",
			user: &auth.AuthenticatedUser{
				UserID: "user-1",
				Role: &auth.Role{
					Permissions: []auth.Permission{auth.PermissionSubscriptionRead, auth.PermissionSubscriptionCreate},
				},
			},
			permission: auth.PermissionSubscriptionRead,
			expected:   true,
		},
		{
			name: "user lacks permission",
			user: &auth.AuthenticatedUser{
				UserID: "user-2",
				Role: &auth.Role{
					Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
				},
			},
			permission: auth.PermissionSubscriptionDelete,
			expected:   false,
		},
		{
			name: "platform admin has all permissions",
			user: &auth.AuthenticatedUser{
				UserID:          "admin-1",
				IsPlatformAdmin: true,
				Role:            &auth.Role{Permissions: []auth.Permission{}},
			},
			permission: auth.PermissionTenantDelete,
			expected:   true,
		},
		{
			name:       "no user in context",
			user:       nil,
			permission: auth.PermissionSubscriptionRead,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = auth.ContextWithUser(ctx, tt.user)
			}

			result := auth.HasPermissionFromContext(ctx, tt.permission)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContextChaining(t *testing.T) {
	// Test that multiple values can be stored in context.
	user := &auth.AuthenticatedUser{
		UserID:   "user-1",
		TenantID: "tenant-1",
	}
	tenant := &auth.Tenant{
		ID:   "tenant-1",
		Name: "Test",
	}
	requestID := "req-123"

	ctx := context.Background()
	ctx = auth.ContextWithUser(ctx, user)
	ctx = auth.ContextWithTenant(ctx, tenant)
	ctx = auth.ContextWithRequestID(ctx, requestID)

	// All values should be retrievable.
	assert.Equal(t, user.UserID, auth.UserFromContext(ctx).UserID)
	assert.Equal(t, tenant.ID, auth.TenantFromContext(ctx).ID)
	assert.Equal(t, requestID, auth.RequestIDFromContext(ctx))
}
