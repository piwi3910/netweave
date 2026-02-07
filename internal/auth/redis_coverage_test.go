package auth_test

import (
	"context"
	"testing"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisStore_UpdateUser_SubjectChange(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant.
	tenant := &auth.Tenant{
		ID:     "tenant-upd-subj",
		Name:   "Update Subject Test",
		Status: auth.TenantStatusActive,
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	tenant2 := &auth.Tenant{
		ID:     "tenant-upd-subj-2",
		Name:   "Update Subject Test 2",
		Status: auth.TenantStatusActive,
	}
	require.NoError(t, store.CreateTenant(ctx, tenant2))

	// Create a user.
	user := &auth.TenantUser{
		ID:         "user-upd-subj",
		TenantID:   "tenant-upd-subj",
		Subject:    "CN=alice-update,O=ACME",
		CommonName: "alice-update",
		Email:      "alice-update@example.com",
		RoleID:     "role-admin",
		IsActive:   true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	t.Run("update user subject changes index", func(t *testing.T) {
		updated, err := store.GetUser(ctx, "user-upd-subj")
		require.NoError(t, err)

		updated.Subject = "CN=alice-new,O=ACME"
		err = store.UpdateUser(ctx, updated)
		require.NoError(t, err)

		// New subject should work.
		found, err := store.GetUserBySubject(ctx, "CN=alice-new,O=ACME")
		require.NoError(t, err)
		assert.Equal(t, "user-upd-subj", found.ID)

		// Old subject should not find user.
		_, err = store.GetUserBySubject(ctx, "CN=alice-update,O=ACME")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("update user tenant changes index", func(t *testing.T) {
		updated, err := store.GetUser(ctx, "user-upd-subj")
		require.NoError(t, err)

		updated.TenantID = "tenant-upd-subj-2"
		err = store.UpdateUser(ctx, updated)
		require.NoError(t, err)

		// New tenant should list the user.
		users, err := store.ListUsersByTenant(ctx, "tenant-upd-subj-2")
		require.NoError(t, err)
		assert.Len(t, users, 1)
		assert.Equal(t, "user-upd-subj", users[0].ID)

		// Old tenant should not list the user.
		users, err = store.ListUsersByTenant(ctx, "tenant-upd-subj")
		require.NoError(t, err)
		assert.Empty(t, users)
	})
}

func TestRedisStore_UpdateUser_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty user ID", func(t *testing.T) {
		user := &auth.TenantUser{ID: ""}
		err := store.UpdateUser(ctx, user)
		assert.ErrorIs(t, err, auth.ErrInvalidUserID)
	})

	t.Run("non-existent user", func(t *testing.T) {
		user := &auth.TenantUser{ID: "non-existent-user"}
		err := store.UpdateUser(ctx, user)
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})
}

func TestRedisStore_CreateUser_EmptyID(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	user := &auth.TenantUser{
		ID:       "",
		TenantID: "tenant-1",
		Subject:  "CN=nobody,O=ACME",
	}
	err := store.CreateUser(ctx, user)
	assert.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestRedisStore_DeleteUser_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty ID", func(t *testing.T) {
		err := store.DeleteUser(ctx, "")
		assert.ErrorIs(t, err, auth.ErrInvalidUserID)
	})

	t.Run("non-existent user", func(t *testing.T) {
		err := store.DeleteUser(ctx, "non-existent-del")
		assert.ErrorIs(t, err, auth.ErrUserNotFound)
	})
}

func TestRedisStore_ListUsersByTenant_Empty(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID returns empty", func(t *testing.T) {
		users, err := store.ListUsersByTenant(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, users)
	})

	t.Run("non-existent tenant returns empty", func(t *testing.T) {
		users, err := store.ListUsersByTenant(ctx, "non-existent-tenant")
		require.NoError(t, err)
		assert.Empty(t, users)
	})
}

func TestRedisStore_DecrementUsage_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID", func(t *testing.T) {
		err := store.DecrementUsage(ctx, "", "subscriptions")
		assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
	})

	t.Run("invalid usage type", func(t *testing.T) {
		err := store.DecrementUsage(ctx, "some-tenant", "invalid-type")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown usage type")
	})

	t.Run("non-existent tenant", func(t *testing.T) {
		err := store.DecrementUsage(ctx, "non-existent-tenant-dec", "subscriptions")
		assert.Error(t, err)
	})
}

func TestRedisStore_IncrementUsage_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID", func(t *testing.T) {
		err := store.IncrementUsage(ctx, "", "subscriptions")
		assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
	})

	t.Run("non-existent tenant", func(t *testing.T) {
		err := store.IncrementUsage(ctx, "non-existent-tenant-inc", "subscriptions")
		assert.Error(t, err)
	})
}

func TestRedisStore_IncrementUsage_AllTypes(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	tenant := &auth.Tenant{
		ID:     "tenant-usage-all",
		Name:   "Usage All Types",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions: 10,
			MaxResourcePools: 10,
			MaxDeployments:   10,
			MaxUsers:         10,
		},
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	types := []string{"subscriptions", "resourcePools", "deployments", "users"}
	for _, usageType := range types {
		t.Run("increment "+usageType, func(t *testing.T) {
			err := store.IncrementUsage(ctx, "tenant-usage-all", usageType)
			require.NoError(t, err)
		})
		t.Run("decrement "+usageType, func(t *testing.T) {
			err := store.DecrementUsage(ctx, "tenant-usage-all", usageType)
			require.NoError(t, err)
		})
	}
}

func TestRedisStore_UpdateRole_NameChange(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a role.
	role := &auth.Role{
		ID:          "role-name-change",
		Name:        "original-name",
		Type:        auth.RoleTypeTenant,
		Description: "Test role for name change",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	t.Run("update role name updates index", func(t *testing.T) {
		updated, err := store.GetRole(ctx, "role-name-change")
		require.NoError(t, err)

		updated.Name = "new-name"
		err = store.UpdateRole(ctx, updated)
		require.NoError(t, err)

		// New name should find the role.
		found, err := store.GetRoleByName(ctx, "new-name")
		require.NoError(t, err)
		assert.Equal(t, "role-name-change", found.ID)

		// Old name should not find the role.
		_, err = store.GetRoleByName(ctx, "original-name")
		assert.ErrorIs(t, err, auth.ErrRoleNotFound)
	})
}

func TestRedisStore_UpdateRole_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty role ID", func(t *testing.T) {
		role := &auth.Role{ID: ""}
		err := store.UpdateRole(ctx, role)
		assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
	})

	t.Run("non-existent role", func(t *testing.T) {
		role := &auth.Role{ID: "non-existent-role-upd"}
		err := store.UpdateRole(ctx, role)
		assert.ErrorIs(t, err, auth.ErrRoleNotFound)
	})
}

func TestRedisStore_DeleteRole_TenantSpecific(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant-specific role.
	role := &auth.Role{
		ID:          "role-tenant-del",
		Name:        "tenant-del-role",
		Type:        auth.RoleTypeTenant,
		TenantID:    "tenant-del",
		Description: "Tenant-specific role for deletion test",
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	// Delete the role.
	err := store.DeleteRole(ctx, "role-tenant-del")
	require.NoError(t, err)

	// Verify it's gone.
	_, err = store.GetRole(ctx, "role-tenant-del")
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestRedisStore_DeleteRole_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty role ID", func(t *testing.T) {
		err := store.DeleteRole(ctx, "")
		assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
	})

	t.Run("non-existent role", func(t *testing.T) {
		err := store.DeleteRole(ctx, "non-existent-role-del")
		assert.ErrorIs(t, err, auth.ErrRoleNotFound)
	})
}

func TestRedisStore_CreateRole_EmptyID(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	role := &auth.Role{ID: ""}
	err := store.CreateRole(ctx, role)
	assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestRedisStore_GetRoleByName_NotFound(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.GetRoleByName(ctx, "non-existent-name")
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestRedisStore_GetRole_EmptyID(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.GetRole(ctx, "")
	assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestRedisStore_LogEvent_EmptyID(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	event := &auth.AuditEvent{
		ID: "",
	}
	err := store.LogEvent(ctx, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event ID is required")
}

func TestRedisStore_ListEvents_LimitEdgeCases(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create some events.
	for i := 0; i < 3; i++ {
		event := &auth.AuditEvent{
			ID:       "event-lim-" + string(rune('a'+i)),
			Type:     auth.AuditEventAuthSuccess,
			TenantID: "tenant-lim",
			UserID:   "user-lim",
			Action:   "test",
		}
		require.NoError(t, store.LogEvent(ctx, event))
	}

	t.Run("zero limit defaults to 50", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 0, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("negative limit defaults to 50", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", -1, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("large limit capped at 1000", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 5000, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})
}

func TestRedisStore_ListEventsByType_LimitEdgeCases(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("zero limit defaults to 50", func(t *testing.T) {
		events, err := store.ListEventsByType(ctx, auth.AuditEventAuthSuccess, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("large limit capped at 1000", func(t *testing.T) {
		events, err := store.ListEventsByType(ctx, auth.AuditEventAuthSuccess, 5000)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})
}

func TestRedisStore_ListEventsByUser_LimitEdgeCases(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("zero limit defaults to 50", func(t *testing.T) {
		events, err := store.ListEventsByUser(ctx, "user-1", 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("large limit capped at 1000", func(t *testing.T) {
		events, err := store.ListEventsByUser(ctx, "user-1", 5000)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})
}

func TestRedisStore_UpdateTenant_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID", func(t *testing.T) {
		tenant := &auth.Tenant{ID: ""}
		err := store.UpdateTenant(ctx, tenant)
		assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
	})

	t.Run("non-existent tenant", func(t *testing.T) {
		tenant := &auth.Tenant{ID: "non-existent-upd"}
		err := store.UpdateTenant(ctx, tenant)
		assert.ErrorIs(t, err, auth.ErrTenantNotFound)
	})
}

func TestRedisStore_DeleteTenant_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID", func(t *testing.T) {
		err := store.DeleteTenant(ctx, "")
		assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
	})

	t.Run("non-existent tenant", func(t *testing.T) {
		err := store.DeleteTenant(ctx, "non-existent-del-t")
		assert.ErrorIs(t, err, auth.ErrTenantNotFound)
	})
}

func TestRedisStore_CreateTenant_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID", func(t *testing.T) {
		tenant := &auth.Tenant{ID: ""}
		err := store.CreateTenant(ctx, tenant)
		assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
	})

	t.Run("duplicate tenant", func(t *testing.T) {
		tenant := &auth.Tenant{
			ID:     "tenant-dup-create",
			Name:   "Dup Test",
			Status: auth.TenantStatusActive,
		}
		require.NoError(t, store.CreateTenant(ctx, tenant))

		err := store.CreateTenant(ctx, tenant)
		assert.ErrorIs(t, err, auth.ErrTenantExists)
	})
}

func TestRedisStore_GetTenant_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty tenant ID", func(t *testing.T) {
		_, err := store.GetTenant(ctx, "")
		assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
	})
}

func TestRedisStore_GetUser_Errors(t *testing.T) {
	store := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("empty user ID", func(t *testing.T) {
		_, err := store.GetUser(ctx, "")
		assert.ErrorIs(t, err, auth.ErrInvalidUserID)
	})
}
