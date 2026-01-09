package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	store := NewRedisStoreWithClient(client)
	return store, mr
}

func TestRedisStore_TenantOperations(t *testing.T) {
	store, _ := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("create tenant", func(t *testing.T) {
		tenant := &Tenant{
			ID:          "tenant-1",
			Name:        "Test Tenant",
			Description: "A test tenant",
			Status:      TenantStatusActive,
			Quota:       DefaultQuota(),
		}

		err := store.CreateTenant(ctx, tenant)
		require.NoError(t, err)
		assert.NotZero(t, tenant.CreatedAt)
		assert.NotZero(t, tenant.UpdatedAt)
	})

	t.Run("create duplicate tenant fails", func(t *testing.T) {
		tenant := &Tenant{
			ID:     "tenant-1",
			Name:   "Duplicate",
			Status: TenantStatusActive,
		}

		err := store.CreateTenant(ctx, tenant)
		assert.ErrorIs(t, err, ErrTenantExists)
	})

	t.Run("create tenant with empty ID fails", func(t *testing.T) {
		tenant := &Tenant{
			ID:   "",
			Name: "No ID",
		}

		err := store.CreateTenant(ctx, tenant)
		assert.ErrorIs(t, err, ErrInvalidTenantID)
	})

	t.Run("get tenant", func(t *testing.T) {
		tenant, err := store.GetTenant(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, "tenant-1", tenant.ID)
		assert.Equal(t, "Test Tenant", tenant.Name)
	})

	t.Run("get non-existent tenant", func(t *testing.T) {
		_, err := store.GetTenant(ctx, "non-existent")
		assert.ErrorIs(t, err, ErrTenantNotFound)
	})

	t.Run("update tenant", func(t *testing.T) {
		tenant, err := store.GetTenant(ctx, "tenant-1")
		require.NoError(t, err)

		originalCreatedAt := tenant.CreatedAt
		tenant.Name = "Updated Tenant"
		tenant.Status = TenantStatusSuspended

		time.Sleep(10 * time.Millisecond) // Ensure time difference.

		err = store.UpdateTenant(ctx, tenant)
		require.NoError(t, err)

		updated, err := store.GetTenant(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, "Updated Tenant", updated.Name)
		assert.Equal(t, TenantStatusSuspended, updated.Status)
		assert.Equal(t, originalCreatedAt.UTC(), updated.CreatedAt.UTC())
		assert.True(t, updated.UpdatedAt.After(originalCreatedAt))
	})

	t.Run("list tenants", func(t *testing.T) {
		// Create another tenant.
		tenant2 := &Tenant{
			ID:     "tenant-2",
			Name:   "Second Tenant",
			Status: TenantStatusActive,
		}
		err := store.CreateTenant(ctx, tenant2)
		require.NoError(t, err)

		tenants, err := store.ListTenants(ctx)
		require.NoError(t, err)
		assert.Len(t, tenants, 2)
	})

	t.Run("delete tenant", func(t *testing.T) {
		err := store.DeleteTenant(ctx, "tenant-2")
		require.NoError(t, err)

		_, err = store.GetTenant(ctx, "tenant-2")
		assert.ErrorIs(t, err, ErrTenantNotFound)
	})

	t.Run("delete non-existent tenant", func(t *testing.T) {
		err := store.DeleteTenant(ctx, "non-existent")
		assert.ErrorIs(t, err, ErrTenantNotFound)
	})
}

func TestRedisStore_UsageOperations(t *testing.T) {
	store, _ := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant.
	tenant := &Tenant{
		ID:     "tenant-usage",
		Name:   "Usage Test",
		Status: TenantStatusActive,
		Quota: TenantQuota{
			MaxSubscriptions: 2,
			MaxResourcePools: 2,
			MaxDeployments:   2,
			MaxUsers:         2,
		},
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	t.Run("increment subscription usage", func(t *testing.T) {
		err := store.IncrementUsage(ctx, "tenant-usage", "subscriptions")
		require.NoError(t, err)

		updated, err := store.GetTenant(ctx, "tenant-usage")
		require.NoError(t, err)
		assert.Equal(t, 1, updated.Usage.Subscriptions)
	})

	t.Run("increment user usage", func(t *testing.T) {
		err := store.IncrementUsage(ctx, "tenant-usage", "users")
		require.NoError(t, err)

		updated, err := store.GetTenant(ctx, "tenant-usage")
		require.NoError(t, err)
		assert.Equal(t, 1, updated.Usage.Users)
	})

	t.Run("quota exceeded error", func(t *testing.T) {
		// Increment to max.
		err := store.IncrementUsage(ctx, "tenant-usage", "subscriptions")
		require.NoError(t, err)

		// Should fail now.
		err = store.IncrementUsage(ctx, "tenant-usage", "subscriptions")
		assert.ErrorIs(t, err, ErrQuotaExceeded)
	})

	t.Run("decrement usage", func(t *testing.T) {
		err := store.DecrementUsage(ctx, "tenant-usage", "subscriptions")
		require.NoError(t, err)

		updated, err := store.GetTenant(ctx, "tenant-usage")
		require.NoError(t, err)
		assert.Equal(t, 1, updated.Usage.Subscriptions)
	})

	t.Run("invalid usage type", func(t *testing.T) {
		err := store.IncrementUsage(ctx, "tenant-usage", "invalid")
		assert.Error(t, err)
	})
}

func TestRedisStore_UserOperations(t *testing.T) {
	store, _ := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a tenant first.
	tenant := &Tenant{
		ID:     "tenant-users",
		Name:   "User Test",
		Status: TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	t.Run("create user", func(t *testing.T) {
		user := &TenantUser{
			ID:         "user-1",
			TenantID:   "tenant-users",
			Subject:    "CN=alice,O=ACME",
			CommonName: "alice",
			Email:      "alice@example.com",
			RoleID:     "role-admin",
			IsActive:   true,
		}

		err := store.CreateUser(ctx, user)
		require.NoError(t, err)
		assert.NotZero(t, user.CreatedAt)
	})

	t.Run("create duplicate user fails", func(t *testing.T) {
		user := &TenantUser{
			ID:         "user-1",
			TenantID:   "tenant-users",
			Subject:    "CN=alice,O=ACME",
			CommonName: "alice",
		}

		err := store.CreateUser(ctx, user)
		assert.ErrorIs(t, err, ErrUserExists)
	})

	t.Run("create user with duplicate subject fails", func(t *testing.T) {
		user := &TenantUser{
			ID:         "user-2",
			TenantID:   "tenant-users",
			Subject:    "CN=alice,O=ACME", // Same subject as user-1.
			CommonName: "alice2",
		}

		err := store.CreateUser(ctx, user)
		assert.ErrorIs(t, err, ErrUserExists)
	})

	t.Run("get user", func(t *testing.T) {
		user, err := store.GetUser(ctx, "user-1")
		require.NoError(t, err)
		assert.Equal(t, "user-1", user.ID)
		assert.Equal(t, "alice", user.CommonName)
	})

	t.Run("get user by subject", func(t *testing.T) {
		user, err := store.GetUserBySubject(ctx, "CN=alice,O=ACME")
		require.NoError(t, err)
		assert.Equal(t, "user-1", user.ID)
	})

	t.Run("get non-existent user", func(t *testing.T) {
		_, err := store.GetUser(ctx, "non-existent")
		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("update user", func(t *testing.T) {
		user, err := store.GetUser(ctx, "user-1")
		require.NoError(t, err)

		user.Email = "alice.updated@example.com"
		user.IsActive = false

		err = store.UpdateUser(ctx, user)
		require.NoError(t, err)

		updated, err := store.GetUser(ctx, "user-1")
		require.NoError(t, err)
		assert.Equal(t, "alice.updated@example.com", updated.Email)
		assert.False(t, updated.IsActive)
	})

	t.Run("list users by tenant", func(t *testing.T) {
		// Create another user.
		user2 := &TenantUser{
			ID:         "user-3",
			TenantID:   "tenant-users",
			Subject:    "CN=bob,O=ACME",
			CommonName: "bob",
			IsActive:   true,
		}
		err := store.CreateUser(ctx, user2)
		require.NoError(t, err)

		users, err := store.ListUsersByTenant(ctx, "tenant-users")
		require.NoError(t, err)
		assert.Len(t, users, 2)
	})

	t.Run("delete user", func(t *testing.T) {
		err := store.DeleteUser(ctx, "user-3")
		require.NoError(t, err)

		_, err = store.GetUser(ctx, "user-3")
		assert.ErrorIs(t, err, ErrUserNotFound)

		// Verify subject index is cleaned up.
		_, err = store.GetUserBySubject(ctx, "CN=bob,O=ACME")
		assert.ErrorIs(t, err, ErrUserNotFound)
	})
}

func TestRedisStore_RoleOperations(t *testing.T) {
	store, _ := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("create role", func(t *testing.T) {
		role := &Role{
			ID:          "role-1",
			Name:        "custom-role",
			Type:        RoleTypeTenant,
			Description: "Custom role",
			Permissions: []Permission{PermissionSubscriptionRead},
		}

		err := store.CreateRole(ctx, role)
		require.NoError(t, err)
		assert.NotZero(t, role.CreatedAt)
	})

	t.Run("create duplicate role fails", func(t *testing.T) {
		role := &Role{
			ID:   "role-1",
			Name: "duplicate",
			Type: RoleTypeTenant,
		}

		err := store.CreateRole(ctx, role)
		assert.ErrorIs(t, err, ErrRoleExists)
	})

	t.Run("get role", func(t *testing.T) {
		role, err := store.GetRole(ctx, "role-1")
		require.NoError(t, err)
		assert.Equal(t, "role-1", role.ID)
		assert.Equal(t, RoleName("custom-role"), role.Name)
	})

	t.Run("get role by name", func(t *testing.T) {
		role, err := store.GetRoleByName(ctx, "custom-role")
		require.NoError(t, err)
		assert.Equal(t, "role-1", role.ID)
	})

	t.Run("get non-existent role", func(t *testing.T) {
		_, err := store.GetRole(ctx, "non-existent")
		assert.ErrorIs(t, err, ErrRoleNotFound)
	})

	t.Run("update role", func(t *testing.T) {
		role, err := store.GetRole(ctx, "role-1")
		require.NoError(t, err)

		role.Description = "Updated description"
		role.Permissions = append(role.Permissions, PermissionSubscriptionCreate)

		err = store.UpdateRole(ctx, role)
		require.NoError(t, err)

		updated, err := store.GetRole(ctx, "role-1")
		require.NoError(t, err)
		assert.Equal(t, "Updated description", updated.Description)
		assert.Len(t, updated.Permissions, 2)
	})

	t.Run("list roles", func(t *testing.T) {
		roles, err := store.ListRoles(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, roles)
	})

	t.Run("initialize default roles", func(t *testing.T) {
		err := store.InitializeDefaultRoles(ctx)
		require.NoError(t, err)

		roles, err := store.ListRoles(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(roles), 6) // 6 default roles + 1 custom.

		// Verify we can get default roles by name.
		platformAdmin, err := store.GetRoleByName(ctx, RolePlatformAdmin)
		require.NoError(t, err)
		assert.Equal(t, RoleTypePlatform, platformAdmin.Type)
	})

	t.Run("initialize default roles is idempotent", func(t *testing.T) {
		err := store.InitializeDefaultRoles(ctx)
		require.NoError(t, err)

		err = store.InitializeDefaultRoles(ctx)
		require.NoError(t, err)
	})

	t.Run("delete role", func(t *testing.T) {
		err := store.DeleteRole(ctx, "role-1")
		require.NoError(t, err)

		_, err = store.GetRole(ctx, "role-1")
		assert.ErrorIs(t, err, ErrRoleNotFound)
	})
}

func TestRedisStore_AuditOperations(t *testing.T) {
	store, _ := setupTestRedis(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	t.Run("log event", func(t *testing.T) {
		event := &AuditEvent{
			ID:           "event-1",
			Type:         AuditEventUserCreated,
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Subject:      "CN=admin,O=ACME",
			ResourceType: "user",
			ResourceID:   "user-2",
			Action:       "create",
			ClientIP:     "192.168.1.100",
		}

		err := store.LogEvent(ctx, event)
		require.NoError(t, err)
		assert.NotZero(t, event.Timestamp)
	})

	t.Run("log multiple events", func(t *testing.T) {
		for i := 2; i <= 5; i++ {
			event := &AuditEvent{
				ID:       "event-" + string(rune('0'+i)),
				Type:     AuditEventUserCreated,
				TenantID: "tenant-1",
				UserID:   "user-1",
				Action:   "create",
			}
			err := store.LogEvent(ctx, event)
			require.NoError(t, err)
		}
	})

	t.Run("list events", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 10, 0)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(events), 4)
	})

	t.Run("list events by tenant", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "tenant-1", 10, 0)
		require.NoError(t, err)
		assert.NotEmpty(t, events)

		for _, event := range events {
			assert.Equal(t, "tenant-1", event.TenantID)
		}
	})

	t.Run("list events by type", func(t *testing.T) {
		events, err := store.ListEventsByType(ctx, AuditEventUserCreated, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, events)

		for _, event := range events {
			assert.Equal(t, AuditEventUserCreated, event.Type)
		}
	})

	t.Run("list events by user", func(t *testing.T) {
		events, err := store.ListEventsByUser(ctx, "user-1", 10)
		require.NoError(t, err)
		assert.NotEmpty(t, events)

		for _, event := range events {
			assert.Equal(t, "user-1", event.UserID)
		}
	})

	t.Run("list events with pagination", func(t *testing.T) {
		events, err := store.ListEvents(ctx, "", 2, 0)
		require.NoError(t, err)
		assert.Len(t, events, 2)

		events2, err := store.ListEvents(ctx, "", 2, 2)
		require.NoError(t, err)
		assert.NotEmpty(t, events2)

		// Ensure different events.
		if len(events) > 0 && len(events2) > 0 {
			assert.NotEqual(t, events[0].ID, events2[0].ID)
		}
	})
}

func TestRedisStore_Ping(t *testing.T) {
	store, _ := setupTestRedis(t)

	ctx := context.Background()

	err := store.Ping(ctx)
	assert.NoError(t, err)

	store.Close()

	// After closing, ping should fail.
	err = store.Ping(ctx)
	assert.Error(t, err)
}

func TestNewRedisStore(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		store := NewRedisStore(nil)
		assert.NotNil(t, store)
		assert.NotNil(t, store.config)
		assert.Equal(t, "localhost:6379", store.config.Addr)
		store.Close()
	})

	t.Run("with custom config", func(t *testing.T) {
		mr := miniredis.RunT(t)
		defer mr.Close()

		cfg := &RedisConfig{
			Addr:        mr.Addr(),
			MaxRetries:  5,
			DialTimeout: 10 * time.Second,
		}

		store := NewRedisStore(cfg)
		assert.NotNil(t, store)

		err := store.Ping(context.Background())
		assert.NoError(t, err)
		store.Close()
	})
}

func TestDefaultRedisConfig(t *testing.T) {
	cfg := DefaultRedisConfig()

	assert.Equal(t, "localhost:6379", cfg.Addr)
	assert.Empty(t, cfg.Password)
	assert.Equal(t, 0, cfg.DB)
	assert.False(t, cfg.UseSentinel)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 5*time.Second, cfg.DialTimeout)
	assert.Equal(t, 3*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 3*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 10, cfg.PoolSize)
}

// TestListTenants tests tenant listing.
func TestListTenants(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &RedisConfig{Addr: mr.Addr()}
	store := NewRedisStore(cfg)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create multiple tenants
	tenant1 := &Tenant{
		ID:           "tenant-1",
		Name:         "Tenant One",
		Description:  "First tenant",
		Status:       TenantStatusActive,
		Quota:        TenantQuota{MaxSubscriptions: 100},
		ContactEmail: "admin@tenant1.com",
	}
	tenant2 := &Tenant{
		ID:           "tenant-2",
		Name:         "Tenant Two",
		Description:  "Second tenant",
		Status:       TenantStatusActive,
		Quota:        TenantQuota{MaxSubscriptions: 50},
		ContactEmail: "admin@tenant2.com",
	}

	err := store.CreateTenant(ctx, tenant1)
	require.NoError(t, err)
	err = store.CreateTenant(ctx, tenant2)
	require.NoError(t, err)

	// List all tenants
	tenants, err := store.ListTenants(ctx)
	require.NoError(t, err)
	assert.Len(t, tenants, 2)

	// Verify tenant IDs
	ids := make(map[string]bool)
	for _, t := range tenants {
		ids[t.ID] = true
	}
	assert.True(t, ids["tenant-1"])
	assert.True(t, ids["tenant-2"])
}

// TestIncrementUsage tests usage increment.
func TestIncrementUsage(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &RedisConfig{Addr: mr.Addr()}
	store := NewRedisStore(cfg)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	tenant := &Tenant{
		ID:          "tenant-incr",
		Name:        "Test Tenant",
		Description: "Test tenant for increment",
		Status:      TenantStatusActive,
		Quota:       TenantQuota{MaxSubscriptions: 100, MaxUsers: 50},
	}

	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Increment subscriptions
	err = store.IncrementUsage(ctx, "tenant-incr", "subscriptions")
	require.NoError(t, err)

	// Increment users
	err = store.IncrementUsage(ctx, "tenant-incr", "users")
	require.NoError(t, err)
	err = store.IncrementUsage(ctx, "tenant-incr", "users")
	require.NoError(t, err)

	// Verify usage
	retrieved, err := store.GetTenant(ctx, "tenant-incr")
	require.NoError(t, err)
	assert.Equal(t, 1, retrieved.Usage.Subscriptions)
	assert.Equal(t, 2, retrieved.Usage.Users)
}

// TestDecrementUsage tests usage decrement.
func TestDecrementUsage(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &RedisConfig{Addr: mr.Addr()}
	store := NewRedisStore(cfg)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	tenant := &Tenant{
		ID:          "tenant-decr",
		Name:        "Test Tenant",
		Description: "Test tenant for decrement",
		Status:      TenantStatusActive,
		Usage:       TenantUsage{Subscriptions: 5, Users: 10},
		Quota:       TenantQuota{MaxSubscriptions: 100, MaxUsers: 50},
	}

	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Decrement subscriptions
	err = store.DecrementUsage(ctx, "tenant-decr", "subscriptions")
	require.NoError(t, err)

	// Decrement users twice
	err = store.DecrementUsage(ctx, "tenant-decr", "users")
	require.NoError(t, err)
	err = store.DecrementUsage(ctx, "tenant-decr", "users")
	require.NoError(t, err)

	// Verify usage
	retrieved, err := store.GetTenant(ctx, "tenant-decr")
	require.NoError(t, err)
	assert.Equal(t, 4, retrieved.Usage.Subscriptions)
	assert.Equal(t, 8, retrieved.Usage.Users)
}

// TestUpdateTenant tests tenant update.
func TestUpdateTenant(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &RedisConfig{Addr: mr.Addr()}
	store := NewRedisStore(cfg)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	tenant := &Tenant{
		ID:           "tenant-update",
		Name:         "Original Name",
		Description:  "Original description",
		Status:       TenantStatusActive,
		ContactEmail: "original@example.com",
		Quota:        TenantQuota{MaxSubscriptions: 100},
	}

	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Update tenant
	tenant.Name = "Updated Name"
	tenant.Description = "Updated description"
	tenant.ContactEmail = "updated@example.com"
	tenant.Quota.MaxSubscriptions = 200

	err = store.UpdateTenant(ctx, tenant)
	require.NoError(t, err)

	// Verify updates
	retrieved, err := store.GetTenant(ctx, "tenant-update")
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)
	assert.Equal(t, "Updated description", retrieved.Description)
	assert.Equal(t, "updated@example.com", retrieved.ContactEmail)
	assert.Equal(t, 200, retrieved.Quota.MaxSubscriptions)
}

// TestDeleteTenant tests tenant deletion.
func TestDeleteTenant(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &RedisConfig{Addr: mr.Addr()}
	store := NewRedisStore(cfg)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	tenant := &Tenant{
		ID:          "tenant-delete",
		Name:        "To Delete",
		Description: "Tenant for deletion test",
		Status:      TenantStatusActive,
	}

	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Delete tenant
	err = store.DeleteTenant(ctx, "tenant-delete")
	require.NoError(t, err)

	// Verify deletion
	_, err = store.GetTenant(ctx, "tenant-delete")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTenantNotFound)
}
