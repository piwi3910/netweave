package auth_test

import (
	"context"
	"testing"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisStore_GetUserByOAuthSubject(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant first.
	tenant := &auth.Tenant{
		ID:     "tenant-oauth",
		Name:   "OAuth Test",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create a user with OAuth subject.
	user := &auth.TenantUser{
		ID:            "user-oauth-1",
		TenantID:      "tenant-oauth",
		Subject:       "CN=oauthuser,O=ACME",
		CommonName:    "oauthuser",
		Email:         "oauth@example.com",
		OAuthSubject:  "keycloak-user-id-123",
		OAuthProvider: "keycloak",
		RoleID:        "role-admin",
		IsActive:      true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Run("find user by OAuth subject", func(t *testing.T) {
		found, err := store.GetUserByOAuthSubject(ctx, "keycloak-user-id-123")
		require.NoError(t, err)
		assert.Equal(t, "user-oauth-1", found.ID)
		assert.Equal(t, "keycloak-user-id-123", found.OAuthSubject)
	})

	t.Run("non-existent OAuth subject returns not found", func(t *testing.T) {
		_, err := store.GetUserByOAuthSubject(ctx, "non-existent-subject")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("empty OAuth subject returns not found", func(t *testing.T) {
		_, err := store.GetUserByOAuthSubject(ctx, "")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})
}

func TestRedisStore_GetUserByEmail(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant first.
	tenant := &auth.Tenant{
		ID:     "tenant-email",
		Name:   "Email Test",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create a user with email.
	user := &auth.TenantUser{
		ID:         "user-email-1",
		TenantID:   "tenant-email",
		Subject:    "CN=emailuser,O=ACME",
		CommonName: "emailuser",
		Email:      "Alice@Example.COM",
		RoleID:     "role-admin",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Run("find user by email (normalized lookup)", func(t *testing.T) {
		found, err := store.GetUserByEmail(ctx, "alice@example.com")
		require.NoError(t, err)
		assert.Equal(t, "user-email-1", found.ID)
	})

	t.Run("find user by email with different case", func(t *testing.T) {
		found, err := store.GetUserByEmail(ctx, "ALICE@EXAMPLE.COM")
		require.NoError(t, err)
		assert.Equal(t, "user-email-1", found.ID)
	})

	t.Run("find user by email with whitespace", func(t *testing.T) {
		found, err := store.GetUserByEmail(ctx, "  alice@example.com  ")
		require.NoError(t, err)
		assert.Equal(t, "user-email-1", found.ID)
	})

	t.Run("non-existent email returns not found", func(t *testing.T) {
		_, err := store.GetUserByEmail(ctx, "nobody@example.com")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("empty email returns not found", func(t *testing.T) {
		_, err := store.GetUserByEmail(ctx, "")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})
}

func TestRedisStore_UpdateLastLogin(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant and user.
	tenant := &auth.Tenant{
		ID:     "tenant-login",
		Name:   "Login Test",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	user := &auth.TenantUser{
		ID:         "user-login-1",
		TenantID:   "tenant-login",
		Subject:    "CN=loginuser,O=ACME",
		CommonName: "loginuser",
		RoleID:     "role-admin",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Run("update last login", func(t *testing.T) {
		err := store.UpdateLastLogin(ctx, "user-login-1")
		require.NoError(t, err)

		updated, err := store.GetUser(ctx, "user-login-1")
		require.NoError(t, err)
		assert.False(t, updated.LastLoginAt.IsZero(), "LastLoginAt should be set")
	})

	t.Run("non-existent user returns error", func(t *testing.T) {
		err := store.UpdateLastLogin(ctx, "non-existent")
		assert.Error(t, err)
	})
}

func TestRedisStore_ListRolesByTenant(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create global roles (no tenant ID).
	globalRole := &auth.Role{
		ID:          "role-global-1",
		Name:        "global-viewer",
		Type:        auth.RoleTypeTenant,
		Description: "Global viewer",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	err := store.CreateRole(ctx, globalRole)
	require.NoError(t, err)

	// Create tenant-specific roles.
	tenantRole := &auth.Role{
		ID:          "role-tenant-a-1",
		Name:        "tenant-a-custom",
		Type:        auth.RoleTypeTenant,
		Description: "Tenant A custom role",
		TenantID:    "tenant-a",
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	err = store.CreateRole(ctx, tenantRole)
	require.NoError(t, err)

	// Create a role for a different tenant.
	otherTenantRole := &auth.Role{
		ID:          "role-tenant-b-1",
		Name:        "tenant-b-custom",
		Type:        auth.RoleTypeTenant,
		Description: "Tenant B custom role",
		TenantID:    "tenant-b",
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	err = store.CreateRole(ctx, otherTenantRole)
	require.NoError(t, err)

	t.Run("list roles for tenant-a includes global and tenant-a roles", func(t *testing.T) {
		roles, err := store.ListRolesByTenant(ctx, "tenant-a")
		require.NoError(t, err)

		// Should include global role and tenant-a role, but not tenant-b role.
		roleIDs := make(map[string]bool)
		for _, r := range roles {
			roleIDs[r.ID] = true
		}
		assert.True(t, roleIDs["role-global-1"], "should include global role")
		assert.True(t, roleIDs["role-tenant-a-1"], "should include tenant-a role")
		assert.False(t, roleIDs["role-tenant-b-1"], "should not include tenant-b role")
	})

	t.Run("list roles for tenant-b includes global and tenant-b roles", func(t *testing.T) {
		roles, err := store.ListRolesByTenant(ctx, "tenant-b")
		require.NoError(t, err)

		roleIDs := make(map[string]bool)
		for _, r := range roles {
			roleIDs[r.ID] = true
		}
		assert.True(t, roleIDs["role-global-1"], "should include global role")
		assert.False(t, roleIDs["role-tenant-a-1"], "should not include tenant-a role")
		assert.True(t, roleIDs["role-tenant-b-1"], "should include tenant-b role")
	})
}

func TestRedisStore_GetUserBySubjectEmpty(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.GetUserBySubject(ctx, "")
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}
