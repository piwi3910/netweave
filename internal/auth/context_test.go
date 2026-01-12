package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextWithUser(t *testing.T) {
	user := &AuthenticatedUser{
		UserID:          "user-123",
		TenantID:        "tenant-456",
		Subject:         "CN=test,O=Org",
		CommonName:      "test",
		IsPlatformAdmin: false,
		Role: &Role{
			Name:        RoleOperator,
			Permissions: []Permission{PermissionSubscriptionRead},
		},
	}

	ctx := context.Background()
	ctx = ContextWithUser(ctx, user)

	// Retrieve user from context.
	retrieved := UserFromContext(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, user.UserID, retrieved.UserID)
	assert.Equal(t, user.TenantID, retrieved.TenantID)
	assert.Equal(t, user.Subject, retrieved.Subject)
}

func TestUserFromContext_Nil(t *testing.T) {
	ctx := context.Background()

	// No user in context.
	retrieved := UserFromContext(ctx)
	assert.Nil(t, retrieved)
}

func TestContextWithTenant(t *testing.T) {
	tenant := &Tenant{
		ID:     "tenant-789",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
	}

	ctx := context.Background()
	ctx = ContextWithTenant(ctx, tenant)

	// Retrieve tenant from context.
	retrieved := TenantFromContext(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, tenant.ID, retrieved.ID)
	assert.Equal(t, tenant.Name, retrieved.Name)
}

func TestTenantFromContext_Nil(t *testing.T) {
	ctx := context.Background()

	// No tenant in context.
	retrieved := TenantFromContext(ctx)
	assert.Nil(t, retrieved)
}

func TestContextWithRequestID(t *testing.T) {
	requestID := "req-12345-abcde"

	ctx := context.Background()
	ctx = ContextWithRequestID(ctx, requestID)

	// Retrieve request ID from context.
	retrieved := RequestIDFromContext(ctx)
	assert.Equal(t, requestID, retrieved)
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	// No request ID in context.
	retrieved := RequestIDFromContext(ctx)
	assert.Empty(t, retrieved)
}

func TestTenantIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		user     *AuthenticatedUser
		expected string
	}{
		{
			name: "user with tenant",
			user: &AuthenticatedUser{
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
			user: &AuthenticatedUser{
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
				ctx = ContextWithUser(ctx, tt.user)
			}

			result := TenantIDFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsPlatformAdminFromContext(t *testing.T) {
	tests := []struct {
		name     string
		user     *AuthenticatedUser
		expected bool
	}{
		{
			name: "platform admin",
			user: &AuthenticatedUser{
				UserID:          "admin-1",
				IsPlatformAdmin: true,
			},
			expected: true,
		},
		{
			name: "regular user",
			user: &AuthenticatedUser{
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
				ctx = ContextWithUser(ctx, tt.user)
			}

			result := IsPlatformAdminFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasPermissionFromContext(t *testing.T) {
	tests := []struct {
		name       string
		user       *AuthenticatedUser
		permission Permission
		expected   bool
	}{
		{
			name: "user has permission",
			user: &AuthenticatedUser{
				UserID: "user-1",
				Role: &Role{
					Permissions: []Permission{PermissionSubscriptionRead, PermissionSubscriptionCreate},
				},
			},
			permission: PermissionSubscriptionRead,
			expected:   true,
		},
		{
			name: "user lacks permission",
			user: &AuthenticatedUser{
				UserID: "user-2",
				Role: &Role{
					Permissions: []Permission{PermissionSubscriptionRead},
				},
			},
			permission: PermissionSubscriptionDelete,
			expected:   false,
		},
		{
			name: "platform admin has all permissions",
			user: &AuthenticatedUser{
				UserID:          "admin-1",
				IsPlatformAdmin: true,
				Role:            &Role{Permissions: []Permission{}},
			},
			permission: PermissionTenantDelete,
			expected:   true,
		},
		{
			name:       "no user in context",
			user:       nil,
			permission: PermissionSubscriptionRead,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = ContextWithUser(ctx, tt.user)
			}

			result := HasPermissionFromContext(ctx, tt.permission)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContextChaining(t *testing.T) {
	// Test that multiple values can be stored in context.
	user := &AuthenticatedUser{
		UserID:   "user-1",
		TenantID: "tenant-1",
	}
	tenant := &Tenant{
		ID:   "tenant-1",
		Name: "Test",
	}
	requestID := "req-123"

	ctx := context.Background()
	ctx = ContextWithUser(ctx, user)
	ctx = ContextWithTenant(ctx, tenant)
	ctx = ContextWithRequestID(ctx, requestID)

	// All values should be retrievable.
	assert.Equal(t, user.UserID, UserFromContext(ctx).UserID)
	assert.Equal(t, tenant.ID, TenantFromContext(ctx).ID)
	assert.Equal(t, requestID, RequestIDFromContext(ctx))
}
