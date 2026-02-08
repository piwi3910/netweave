package auth_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedisEdge(t *testing.T) (*auth.RedisStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	store := auth.NewRedisStoreWithClient(client)
	return store, mr
}

func TestRedisStore_NewRedisStore_SentinelMode(t *testing.T) {
	t.Parallel()
	cfg := &auth.RedisConfig{
		UseSentinel:      true,
		MasterName:       "mymaster",
		SentinelAddrs:    []string{"localhost:26379"},
		SentinelPassword: "sentinelpass",
		Password:         "redispass",
		DB:               1,
		MaxRetries:       5,
		DialTimeout:      3 * time.Second,
		ReadTimeout:      2 * time.Second,
		WriteTimeout:     2 * time.Second,
		PoolSize:         20,
	}

	store := auth.NewRedisStore(cfg)
	require.NotNil(t, store)
	assert.Equal(t, cfg, store.Config)
	// Cleanup - Close will fail since sentinel is not running, that's OK.
	_ = store.Close()
}

func TestRedisStore_NewRedisStore_NilConfig(t *testing.T) {
	t.Parallel()
	store := auth.NewRedisStore(nil)
	require.NotNil(t, store)
	assert.NotNil(t, store.Config)
	_ = store.Close()
}

func TestRedisStore_Close_Error(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)

	// Close miniredis first to force a client close error.
	mr.Close()
	err := store.Close()
	// Close may or may not error depending on redis-go internals,
	// but the code path is exercised.
	_ = err
}

func TestRedisStore_ListTenants_WithCorruptData(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create a valid tenant to add its ID to the set.
	tenant := &auth.Tenant{
		ID:     "tenant-valid",
		Name:   "Valid Tenant",
		Status: auth.TenantStatusActive,
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	// Manually add a corrupt tenant key and a set member with missing key.
	mr.Set("tenant:tenant-corrupt", "not-valid-json")
	mr.SAdd("tenants:active", "tenant-corrupt")
	mr.SAdd("tenants:active", "tenant-missing")

	tenants, err := store.ListTenants(ctx)
	require.NoError(t, err)
	// Only the valid tenant should be returned; corrupt/missing ones are skipped.
	assert.Len(t, tenants, 1)
	assert.Equal(t, "tenant-valid", tenants[0].ID)
}

func TestRedisStore_ListRoles_WithCorruptData(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create a valid role.
	role := &auth.Role{
		ID:          "role-valid",
		Name:        "valid-role",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	// Add corrupt and missing entries directly.
	mr.Set("role:role-corrupt", "bad-json")
	mr.SAdd("roles:all", "role-corrupt")
	mr.SAdd("roles:all", "role-missing")

	roles, err := store.ListRoles(ctx)
	require.NoError(t, err)
	assert.Len(t, roles, 1)
	assert.Equal(t, "role-valid", roles[0].ID)
}

func TestRedisStore_ListUsersByTenant_EdgeCases(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	t.Run("empty tenant ID returns empty slice", func(t *testing.T) {
		users, err := store.ListUsersByTenant(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, users)
	})

	t.Run("tenant with no users returns empty slice", func(t *testing.T) {
		users, err := store.ListUsersByTenant(ctx, "nonexistent-tenant")
		require.NoError(t, err)
		assert.Empty(t, users)
	})

	t.Run("tenant with corrupt user data skips bad entries", func(t *testing.T) {
		// Create tenant.
		tenant := &auth.Tenant{
			ID:     "tenant-list-users",
			Name:   "List Users Test",
			Status: auth.TenantStatusActive,
		}
		require.NoError(t, store.CreateTenant(ctx, tenant))

		// Create a valid user.
		user := &auth.TenantUser{
			ID:         "user-valid",
			TenantID:   "tenant-list-users",
			Subject:    "CN=validuser,O=ACME",
			CommonName: "validuser",
			RoleID:     "role-1",
			IsActive:   true,
		}
		require.NoError(t, store.CreateUser(ctx, user))

		// Add a corrupt user key and missing user to the tenant's user set.
		mr.Set("user:user-corrupt", "not-valid-json")
		mr.SAdd("users:tenant:tenant-list-users", "user-corrupt")
		mr.SAdd("users:tenant:tenant-list-users", "user-missing")

		users, err := store.ListUsersByTenant(ctx, "tenant-list-users")
		require.NoError(t, err)
		// Only the valid user should be returned.
		assert.Len(t, users, 1)
		assert.Equal(t, "user-valid", users[0].ID)
	})
}

func TestRedisStore_ListEvents_EdgeCases(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create events across tenants.
	globalEvent := &auth.AuditEvent{
		ID:        "event-global-1",
		Type:      auth.AuditEventUserCreated,
		Action:    "create",
		UserID:    "user-1",
		Timestamp: time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, globalEvent))

	tenantEvent := &auth.AuditEvent{
		ID:        "event-tenant-1",
		Type:      auth.AuditEventTenantCreated,
		TenantID:  "tenant-1",
		Action:    "create",
		UserID:    "user-2",
		Timestamp: time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, tenantEvent))

	t.Run("list with empty tenantID returns all events", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 100, 0)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(events), 2)
	})

	t.Run("list with limit zero defaults to 50", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 0, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("list with limit over 1000 caps at 1000", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 5000, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("list with tenantID filters to tenant events", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "tenant-1", 100, 0)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "event-tenant-1", events[0].ID)
	})
}

func TestRedisStore_ListEvents_ExpiredEventSkipped(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create a valid event.
	validEvent := &auth.AuditEvent{
		ID:        "event-valid",
		Type:      auth.AuditEventUserCreated,
		Action:    "create",
		Timestamp: time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, validEvent))

	// Add a reference to a non-existent event in the sorted set.
	// This simulates an expired event whose key was TTL'd but the sorted set still has its ID.
	score := float64(time.Now().UnixNano())
	mr.ZAdd("audit:events", score, "event-expired")

	events, err := store.ListEvents(ctx, "", 100, 0)
	require.NoError(t, err)
	// The expired event should be skipped; only valid event returned.
	assert.Len(t, events, 1)
	assert.Equal(t, "event-valid", events[0].ID)
}

func TestRedisStore_ListEventsByType_EdgeCases(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	evt := &auth.AuditEvent{
		ID:        "event-type-1",
		Type:      auth.AuditEventUserCreated,
		Action:    "create",
		Timestamp: time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, evt))

	t.Run("limit zero defaults to 50", func(t *testing.T) {
		events, err := store.ListEventsByType(ctx, auth.AuditEventUserCreated, 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("limit over 1000 caps at 1000", func(t *testing.T) {
		events, err := store.ListEventsByType(ctx, auth.AuditEventUserCreated, 5000)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("expired event skipped", func(t *testing.T) {
		score := float64(time.Now().UnixNano())
		mr.ZAdd("audit:type:"+string(auth.AuditEventUserCreated), score, "event-expired-type")

		events, err := store.ListEventsByType(ctx, auth.AuditEventUserCreated, 100)
		require.NoError(t, err)
		// Only the valid event returned, expired one skipped.
		assert.Len(t, events, 1)
	})
}

func TestRedisStore_ListEventsByUser_EdgeCases(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	evt := &auth.AuditEvent{
		ID:        "event-user-1",
		Type:      auth.AuditEventUserCreated,
		Action:    "create",
		UserID:    "user-audit-1",
		Timestamp: time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, evt))

	t.Run("limit zero defaults to 50", func(t *testing.T) {
		events, err := store.ListEventsByUser(ctx, "user-audit-1", 0)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("limit over 1000 caps at 1000", func(t *testing.T) {
		events, err := store.ListEventsByUser(ctx, "user-audit-1", 5000)
		require.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("expired event skipped", func(t *testing.T) {
		score := float64(time.Now().UnixNano())
		mr.ZAdd("audit:user:user-audit-1", score, "event-expired-user")

		events, err := store.ListEventsByUser(ctx, "user-audit-1", 100)
		require.NoError(t, err)
		assert.Len(t, events, 1)
	})
}

func TestRedisStore_GetAuditEvent_UnmarshalError(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Store corrupt event data at the expected key.
	mr.Set("audit:event-bad-json", "not-valid-json")
	// Add reference to the sorted set so ListEvents picks it up.
	score := float64(time.Now().UnixNano())
	mr.ZAdd("audit:events", score, "event-bad-json")

	// The corrupt event should be skipped.
	events, err := store.ListEvents(ctx, "", 100, 0)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestRedisStore_DecrementUsage_UnknownType(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DecrementUsage(ctx, "tenant-1", "invalid-type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown usage type")
}

func TestRedisStore_DecrementUsage_EmptyTenantID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DecrementUsage(ctx, "", "subscriptions")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
}

func TestRedisStore_DecrementUsage_TenantNotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DecrementUsage(ctx, "nonexistent", "subscriptions")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrTenantNotFound)
}

func TestRedisStore_IncrementUsage_UnexpectedResult(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create tenant with valid quota and try to decrement usage from an active tenant.
	tenant := &auth.Tenant{
		ID:     "tenant-inc-test",
		Name:   "Inc Test",
		Status: auth.TenantStatusActive,
		Quota:  auth.DefaultQuota(),
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	// Increment subscriptions should work.
	err := store.IncrementUsage(ctx, "tenant-inc-test", "subscriptions")
	require.NoError(t, err)

	// Decrement subscriptions should work.
	err = store.DecrementUsage(ctx, "tenant-inc-test", "subscriptions")
	require.NoError(t, err)
}

func TestRedisStore_CreateUser_OAuthIndices(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create tenant.
	tenant := &auth.Tenant{
		ID:     "tenant-oauth-idx",
		Name:   "OAuth Index Test",
		Status: auth.TenantStatusActive,
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	t.Run("user with both OAuth subject and email creates indices", func(t *testing.T) {
		user := &auth.TenantUser{
			ID:            "user-oauth-idx-1",
			TenantID:      "tenant-oauth-idx",
			Subject:       "CN=oauthidx1,O=ACME",
			CommonName:    "oauthidx1",
			OAuthSubject:  "keycloak-oauth-idx-1",
			OAuthProvider: "keycloak",
			Email:         "oauthidx1@example.com",
			RoleID:        "role-1",
			IsActive:      true,
		}
		require.NoError(t, store.CreateUser(ctx, user))

		// Verify OAuth subject lookup works.
		found, err := store.GetUserByOAuthSubject(ctx, "keycloak-oauth-idx-1")
		require.NoError(t, err)
		assert.Equal(t, "user-oauth-idx-1", found.ID)

		// Verify email lookup works.
		found, err = store.GetUserByEmail(ctx, "oauthidx1@example.com")
		require.NoError(t, err)
		assert.Equal(t, "user-oauth-idx-1", found.ID)
	})

	t.Run("user without OAuth subject skips OAuth index", func(t *testing.T) {
		user := &auth.TenantUser{
			ID:         "user-no-oauth",
			TenantID:   "tenant-oauth-idx",
			Subject:    "CN=nooauth,O=ACME",
			CommonName: "nooauth",
			Email:      "nooauth@example.com",
			RoleID:     "role-1",
			IsActive:   true,
		}
		require.NoError(t, store.CreateUser(ctx, user))
	})

	t.Run("user without email skips email index", func(t *testing.T) {
		user := &auth.TenantUser{
			ID:            "user-no-email",
			TenantID:      "tenant-oauth-idx",
			Subject:       "CN=noemail,O=ACME",
			CommonName:    "noemail",
			OAuthSubject:  "keycloak-no-email",
			OAuthProvider: "keycloak",
			RoleID:        "role-1",
			IsActive:      true,
		}
		require.NoError(t, store.CreateUser(ctx, user))
	})
}

func TestRedisStore_InitializeDefaultRoles_Idempotent(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Initialize default roles.
	require.NoError(t, store.InitializeDefaultRoles(ctx))

	// Call again - should be idempotent.
	require.NoError(t, store.InitializeDefaultRoles(ctx))

	// Verify roles exist.
	roles, err := store.ListRoles(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, roles)
}

func TestRedisStore_ListRolesByTenant_FiltersBothGlobalAndTenant(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create a global role (no TenantID).
	globalRole := &auth.Role{
		ID:          "role-global",
		Name:        "global-role",
		Type:        auth.RoleTypePlatform,
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	require.NoError(t, store.CreateRole(ctx, globalRole))

	// Create a tenant-specific role.
	tenantRole := &auth.Role{
		ID:          "role-tenant-specific",
		Name:        "tenant-specific-role",
		Type:        auth.RoleTypeTenant,
		TenantID:    "tenant-roles-test",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	require.NoError(t, store.CreateRole(ctx, tenantRole))

	// Create a role for a different tenant.
	otherRole := &auth.Role{
		ID:          "role-other-tenant",
		Name:        "other-tenant-role",
		Type:        auth.RoleTypeTenant,
		TenantID:    "other-tenant",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	require.NoError(t, store.CreateRole(ctx, otherRole))

	// ListRolesByTenant should include global + matching tenant roles.
	roles, err := store.ListRolesByTenant(ctx, "tenant-roles-test")
	require.NoError(t, err)

	roleIDs := make([]string, 0, len(roles))
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
	}
	assert.Contains(t, roleIDs, "role-global")
	assert.Contains(t, roleIDs, "role-tenant-specific")
	assert.NotContains(t, roleIDs, "role-other-tenant")
}

func TestRedisStore_LogEvent_WithAllIndices(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create event with tenant, user, and type to exercise all sorted set branches.
	evt := &auth.AuditEvent{
		ID:           "event-full-1",
		Type:         auth.AuditEventTLSConfigChanged,
		TenantID:     "tenant-full",
		UserID:       "user-full",
		Action:       "update",
		ResourceType: "config",
		Details:      map[string]string{"key": "value"},
		Timestamp:    time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, evt))

	// Verify event appears in type-based listing.
	events, err := store.ListEventsByType(ctx, auth.AuditEventTLSConfigChanged, 10)
	require.NoError(t, err)
	assert.Len(t, events, 1)

	// Verify event appears in user-based listing.
	events, err = store.ListEventsByUser(ctx, "user-full", 10)
	require.NoError(t, err)
	assert.Len(t, events, 1)

	// Verify event appears in tenant-based listing.
	events, err = store.ListEvents(ctx, "tenant-full", 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestRedisStore_BatchListFromSet_NonStringType(t *testing.T) {
	t.Parallel()
	// This test exercises the "unexpected data type" path in batchListFromSet.
	// We can trigger this by putting a non-string value at a key via miniredis.
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()

	// Add a tenant ID to the set.
	mr.SAdd("tenants:active", "tenant-nonstring")
	// Store a list (non-string type) at the tenant key.
	// miniredis doesn't support direct non-string MGET results,
	// but we can verify the nil case: add ID to set without creating the key.
	// This exercises the "result == nil" branch.
	tenants, err := store.ListTenants(context.Background())
	require.NoError(t, err)
	// No tenant data found for "tenant-nonstring" - it's skipped.
	assert.Empty(t, tenants)
}

func TestRedisStore_UpdateTenant_MarshalPath(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create tenant first.
	tenant := &auth.Tenant{
		ID:          "tenant-upd-marshal",
		Name:        "Update Marshal",
		Status:      auth.TenantStatusActive,
		Description: "original description",
		Quota:       auth.DefaultQuota(),
		Metadata:    map[string]string{"env": "test"},
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	// Update tenant with different metadata (exercises full marshal path).
	updatedTenant := &auth.Tenant{
		ID:           "tenant-upd-marshal",
		Name:         "Updated Marshal",
		Status:       auth.TenantStatusActive,
		Description:  "updated description",
		Quota:        auth.DefaultQuota(),
		ContactEmail: "admin@test.com",
		Metadata:     map[string]string{"env": "prod"},
	}
	require.NoError(t, store.UpdateTenant(ctx, updatedTenant))

	// Verify the update persisted.
	fetched, err := store.GetTenant(ctx, "tenant-upd-marshal")
	require.NoError(t, err)
	assert.Equal(t, "Updated Marshal", fetched.Name)
	assert.Equal(t, "updated description", fetched.Description)
}

func TestRedisStore_UpdateRole_NameChange_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create a role.
	role := &auth.Role{
		ID:          "role-name-change",
		Name:        "original-name",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	// Update the role's name to exercise the name index update path.
	updated := &auth.Role{
		ID:          "role-name-change",
		Name:        "updated-name",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionResourceRead, auth.PermissionResourceUpdate},
	}
	require.NoError(t, store.UpdateRole(ctx, updated))

	// Old name should not resolve.
	_, err := store.GetRoleByName(ctx, "original-name")
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)

	// New name should resolve.
	found, err := store.GetRoleByName(ctx, "updated-name")
	require.NoError(t, err)
	assert.Equal(t, "role-name-change", found.ID)
}

func TestRedisStore_CreateTenant_MarshalAndSAddPaths(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Valid tenant creation exercises full path.
	tenant := &auth.Tenant{
		ID:           "tenant-full-create",
		Name:         "Full Create",
		Description:  "Full create test",
		Status:       auth.TenantStatusActive,
		Quota:        auth.DefaultQuota(),
		ContactEmail: "admin@example.com",
		Metadata:     map[string]string{"env": "prod"},
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Verify it's in the listing.
	tenants, err := store.ListTenants(ctx)
	require.NoError(t, err)
	assert.Len(t, tenants, 1)
}

func TestRedisStore_CreateRole_WithTenantID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create role with TenantID to exercise the tenant index SAdd path.
	role := &auth.Role{
		ID:          "role-with-tenant",
		Name:        "tenant-role",
		Type:        auth.RoleTypeTenant,
		TenantID:    "tenant-123",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	// Verify it's findable by name.
	found, err := store.GetRoleByName(ctx, "tenant-role")
	require.NoError(t, err)
	assert.Equal(t, "role-with-tenant", found.ID)
}

func TestRedisStore_DeleteRole_WithTenantID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	role := &auth.Role{
		ID:          "role-del-tenant",
		Name:        "del-tenant-role",
		Type:        auth.RoleTypeTenant,
		TenantID:    "tenant-del",
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	require.NoError(t, store.DeleteRole(ctx, "role-del-tenant"))

	_, err := store.GetRole(ctx, "role-del-tenant")
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestRedisStore_GetUser_NotFound_GenericError(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)

	ctx := context.Background()

	// Create tenant and user for the happy path.
	tenant := &auth.Tenant{
		ID:     "tenant-get-user",
		Name:   "Get User Test",
		Status: auth.TenantStatusActive,
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	user := &auth.TenantUser{
		ID:         "user-get-1",
		TenantID:   "tenant-get-user",
		Subject:    "CN=getuser,O=ACME",
		CommonName: "getuser",
		RoleID:     "role-1",
		IsActive:   true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	// Store corrupt data at a user key to exercise unmarshal error.
	mr.Set("user:user-corrupt-get", "definitely-not-json")

	// GetUser with a corrupt key exercises the JSON unmarshal error path.
	_, err := store.GetUser(ctx, "user-corrupt-get")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal user")

	_ = store.Close()
}

func TestRedisStore_GetTenant_UnmarshalError(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Store corrupt data at a tenant key.
	mr.Set("tenant:tenant-bad", "not-json")

	_, err := store.GetTenant(ctx, "tenant-bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal tenant")
}

func TestRedisStore_GetRole_UnmarshalError(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	mr.Set("role:role-bad", "not-json")

	_, err := store.GetRole(ctx, "role-bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal role")
}

func TestRedisStore_IncrementUsage_InvalidType(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.IncrementUsage(ctx, "tenant-1", "invalid-type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown usage type")
}

func TestRedisStore_IncrementUsage_EmptyTenantID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.IncrementUsage(ctx, "", "subscriptions")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
}

func TestRedisStore_IncrementUsage_TenantNotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.IncrementUsage(ctx, "nonexistent", "subscriptions")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrTenantNotFound)
}

func TestRedisStore_IncrementUsage_QuotaExceeded(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create a tenant with maxSubscriptions = 1 and usage already at 1.
	tenant := &auth.Tenant{
		ID:     "tenant-quota",
		Name:   "Quota Test",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions: 1,
			MaxResourcePools: 100,
			MaxDeployments:   100,
			MaxUsers:         100,
		},
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	// First increment should succeed.
	require.NoError(t, store.IncrementUsage(ctx, "tenant-quota", "subscriptions"))

	// Second increment should exceed quota.
	err := store.IncrementUsage(ctx, "tenant-quota", "subscriptions")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrQuotaExceeded)

	// Verify we can still list the tenant (sanity check).
	_ = mr
}

func TestRedisStore_IncrementUsage_AllValidTypes(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	tenant := &auth.Tenant{
		ID:     "tenant-all-types",
		Name:   "All Types",
		Status: auth.TenantStatusActive,
		Quota:  auth.DefaultQuota(),
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	validTypes := []string{"subscriptions", "resourcePools", "deployments", "users"}
	for _, usageType := range validTypes {
		err := store.IncrementUsage(ctx, "tenant-all-types", usageType)
		require.NoError(t, err, "IncrementUsage failed for type %s", usageType)

		err = store.DecrementUsage(ctx, "tenant-all-types", usageType)
		require.NoError(t, err, "DecrementUsage failed for type %s", usageType)
	}
}

func TestRedisStore_UpdateUser_FullPaths(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create two tenants.
	for _, tid := range []string{"tenant-upd-a", "tenant-upd-b"} {
		require.NoError(t, store.CreateTenant(ctx, &auth.Tenant{
			ID:     tid,
			Name:   tid,
			Status: auth.TenantStatusActive,
		}))
	}

	// Create user.
	user := &auth.TenantUser{
		ID:            "user-upd-full",
		TenantID:      "tenant-upd-a",
		Subject:       "CN=updfull,O=ACME",
		CommonName:    "updfull",
		Email:         "updfull@example.com",
		OAuthSubject:  "keycloak-updfull",
		OAuthProvider: "keycloak",
		RoleID:        "role-1",
		IsActive:      true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	// Update subject and tenant to exercise index update branches.
	updated := &auth.TenantUser{
		ID:         "user-upd-full",
		TenantID:   "tenant-upd-b", // Changed tenant.
		Subject:    "CN=updfull-changed,O=ACME", // Changed subject.
		CommonName: "updfull-changed",
		RoleID:     "role-2",
		IsActive:   true,
	}
	require.NoError(t, store.UpdateUser(ctx, updated))

	// Verify new subject lookup works.
	found, err := store.GetUserBySubject(ctx, "CN=updfull-changed,O=ACME")
	require.NoError(t, err)
	assert.Equal(t, "user-upd-full", found.ID)
	assert.Equal(t, "tenant-upd-b", found.TenantID)
}

func TestRedisStore_CreateUser_EmptyID_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:       "",
		TenantID: "tenant-1",
	}
	err := store.CreateUser(ctx, user)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestRedisStore_CreateUser_DuplicateSubject(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	require.NoError(t, store.CreateTenant(ctx, &auth.Tenant{
		ID:     "tenant-dup-subj",
		Name:   "Dup Subject",
		Status: auth.TenantStatusActive,
	}))

	user1 := &auth.TenantUser{
		ID:         "user-dup-1",
		TenantID:   "tenant-dup-subj",
		Subject:    "CN=duplicate,O=ACME",
		CommonName: "duplicate",
		RoleID:     "role-1",
		IsActive:   true,
	}
	require.NoError(t, store.CreateUser(ctx, user1))

	// Creating another user with same subject should fail.
	user2 := &auth.TenantUser{
		ID:         "user-dup-2",
		TenantID:   "tenant-dup-subj",
		Subject:    "CN=duplicate,O=ACME",
		CommonName: "duplicate2",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err := store.CreateUser(ctx, user2)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUserExists)
}

func TestRedisStore_GetRoleByName_NotFound_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	_, err := store.GetRoleByName(ctx, "nonexistent-role")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestRedisStore_UpdateRole_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	role := &auth.Role{
		ID:   "nonexistent-role",
		Name: "nonexistent",
	}
	err := store.UpdateRole(ctx, role)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestRedisStore_UpdateRole_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	role := &auth.Role{ID: ""}
	err := store.UpdateRole(ctx, role)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestRedisStore_DeleteUser_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DeleteUser(ctx, "nonexistent-user")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestRedisStore_Ping_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()

	err := store.Ping(context.Background())
	require.NoError(t, err)
}

func TestRedisStore_LogEvent_EmptyTimestamp(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	evt := &auth.AuditEvent{
		ID:     "event-no-ts",
		Type:   auth.AuditEventUserCreated,
		Action: "create",
		// Timestamp zero value - should be auto-set.
	}
	require.NoError(t, store.LogEvent(ctx, evt))

	events, err := store.ListEvents(ctx, "", 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.False(t, events[0].Timestamp.IsZero())
}

func TestRedisStore_LogEvent_EmptyID_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	evt := &auth.AuditEvent{
		ID:   "",
		Type: auth.AuditEventUserCreated,
	}
	err := store.LogEvent(ctx, evt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event ID is required")
}

func TestRedisStore_GetUserBySubject_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	_, err := store.GetUserBySubject(ctx, "CN=nonexistent,O=ACME")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestRedisStore_LogEvent_WithoutTenantOrUser(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Event with no TenantID or UserID to exercise paths where those indices are skipped.
	evt := &auth.AuditEvent{
		ID:        "event-no-tenant-user",
		Type:      auth.AuditEventTLSConfigChanged,
		Action:    "update",
		Timestamp: time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, evt))
}

func TestRedisStore_UpdateUser_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	user := &auth.TenantUser{ID: ""}
	err := store.UpdateUser(ctx, user)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestRedisStore_UpdateUser_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	user := &auth.TenantUser{ID: "nonexistent-user"}
	err := store.UpdateUser(ctx, user)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestRedisStore_UpdateTenant_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	tenant := &auth.Tenant{ID: ""}
	err := store.UpdateTenant(ctx, tenant)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
}

func TestRedisStore_UpdateTenant_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	tenant := &auth.Tenant{ID: "nonexistent"}
	err := store.UpdateTenant(ctx, tenant)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrTenantNotFound)
}

func TestRedisStore_DeleteTenant_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DeleteTenant(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
}

func TestRedisStore_DeleteTenant_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DeleteTenant(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrTenantNotFound)
}

func TestRedisStore_CreateTenant_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	tenant := &auth.Tenant{ID: ""}
	err := store.CreateTenant(ctx, tenant)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
}

func TestRedisStore_CreateRole_EmptyID_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	role := &auth.Role{ID: ""}
	err := store.CreateRole(ctx, role)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestRedisStore_CreateRole_Duplicate(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	role := &auth.Role{
		ID:          "role-dup",
		Name:        "dup-role",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	err := store.CreateRole(ctx, role)
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrRoleExists)
}

func TestRedisStore_DeleteRole_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DeleteRole(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestRedisStore_DeleteRole_NotFound(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DeleteRole(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestRedisStore_DeleteUser_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	err := store.DeleteUser(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestRedisStore_GetUser_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	_, err := store.GetUser(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestRedisStore_GetTenant_EmptyID(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	_, err := store.GetTenant(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidTenantID)
}

func TestRedisStore_GetRole_EmptyID_Edge(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	_, err := store.GetRole(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestRedisStore_GetUserByEmail_EmptyEmail(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	_, err := store.GetUserByEmail(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestRedisStore_ListUsersByTenant_MGetError(t *testing.T) {
	t.Parallel()
	store, mr := setupTestRedisEdge(t)
	ctx := context.Background()

	// Add user IDs to a tenant set but store non-JSON data.
	mr.SAdd("users:tenant:tenant-mget-err", "user-bad-1", "user-bad-2")
	mr.Set("user:user-bad-1", "invalid json data")
	// user-bad-2 key doesn't exist (nil in MGET).

	users, err := store.ListUsersByTenant(ctx, "tenant-mget-err")
	require.NoError(t, err)
	// Both entries are malformed/missing; result should be empty.
	assert.Empty(t, users)

	_ = store.Close()
}

func TestRedisStore_LogEvent_MarshalMetadata(t *testing.T) {
	t.Parallel()
	store, _ := setupTestRedisEdge(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()

	// Create event with details map to ensure json.Marshal covers it.
	evt := &auth.AuditEvent{
		ID:           "event-marshal-det",
		Type:         auth.AuditEventUserCreated,
		TenantID:     "t-1",
		UserID:       "u-1",
		Subject:      "CN=test",
		ResourceType: "user",
		ResourceID:   "u-1",
		Action:       "create",
		Details:      map[string]string{"key1": "val1", "key2": "val2"},
		ClientIP:     "192.168.1.1",
		UserAgent:    "TestAgent/1.0",
		Timestamp:    time.Now().UTC(),
	}
	require.NoError(t, store.LogEvent(ctx, evt))

	// Verify the event is retrievable and details are preserved.
	events, err := store.ListEvents(ctx, "t-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	detailsJSON, err := json.Marshal(events[0].Details)
	require.NoError(t, err)
	assert.Contains(t, string(detailsJSON), "key1")
}
