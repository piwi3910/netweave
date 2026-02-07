package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/database"
)

const (
	authPgTestImage    = "postgres:16-alpine"
	authPgTestDB       = "netweave_auth_test"
	authPgTestUser     = "testuser"
	authPgTestPassword = "testpass"
)

// setupAuthPgContainer starts a PostgreSQL container with migrations for auth tests.
func setupAuthPgContainer(t *testing.T) *database.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        authPgTestImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     authPgTestUser,
			"POSTGRES_PASSWORD": authPgTestPassword,
			"POSTGRES_DB":       authPgTestDB,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start PostgreSQL container")

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	t.Cleanup(func() {
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", termErr)
		}
	})

	cfg := &database.PostgresConfig{
		Host:           host,
		Port:           mappedPort.Int(),
		Database:       authPgTestDB,
		User:           authPgTestUser,
		PasswordEnvVar: "TEST_PG_PASSWORD",
		SSLMode:        "disable",
		MaxConns:       5,
		MinConns:       1,
	}

	db, err := database.New(ctx, cfg, authPgTestPassword)
	require.NoError(t, err)

	err = database.Migrate(ctx, db.Pool)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func newTestAuthStore(t *testing.T, db *database.DB) *auth.PostgresStore {
	t.Helper()
	return auth.NewPostgresStore(db)
}

// --- Tenant Tests ---

func TestPostgresStore_CreateTenant(t *testing.T) {
	db := setupAuthPgContainer(t)

	tests := []struct {
		name    string
		tenant  *auth.Tenant
		wantErr error
	}{
		{
			name: "valid tenant",
			tenant: &auth.Tenant{
				ID:           "tenant-create-1",
				Name:         "ACME Corp",
				Description:  "Test tenant",
				Status:       auth.TenantStatusActive,
				Quota:        auth.DefaultQuota(),
				ContactEmail: "admin@acme.com",
				Metadata:     map[string]string{"env": "test"},
			},
			wantErr: nil,
		},
		{
			name:    "empty ID",
			tenant:  &auth.Tenant{ID: "", Name: "No ID"},
			wantErr: auth.ErrInvalidTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestAuthStore(t, db)
			ctx := context.Background()

			err := store.CreateTenant(ctx, tt.tenant)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.False(t, tt.tenant.CreatedAt.IsZero())
			}
		})
	}
}

func TestPostgresStore_CreateTenantDuplicate(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	tenant := &auth.Tenant{
		ID:     "tenant-dup-1",
		Name:   "Dup Tenant",
		Status: auth.TenantStatusActive,
		Quota:  auth.DefaultQuota(),
	}

	require.NoError(t, store.CreateTenant(ctx, tenant))

	err := store.CreateTenant(ctx, tenant)
	require.ErrorIs(t, err, auth.ErrTenantExists)
}

func TestPostgresStore_GetTenant(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	original := &auth.Tenant{
		ID:           "tenant-get-1",
		Name:         "Get Tenant",
		Description:  "A test tenant for get",
		Status:       auth.TenantStatusActive,
		Quota:        auth.DefaultQuota(),
		ContactEmail: "test@example.com",
		Metadata:     map[string]string{"key": "value"},
	}
	require.NoError(t, store.CreateTenant(ctx, original))

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{name: "existing tenant", id: "tenant-get-1", wantErr: nil},
		{name: "not found", id: "tenant-nonexistent", wantErr: auth.ErrTenantNotFound},
		{name: "empty ID", id: "", wantErr: auth.ErrInvalidTenantID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.GetTenant(ctx, tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, original.ID, got.ID)
				assert.Equal(t, original.Name, got.Name)
				assert.Equal(t, original.Description, got.Description)
				assert.Equal(t, original.Status, got.Status)
				assert.Equal(t, original.Quota, got.Quota)
				assert.Equal(t, original.ContactEmail, got.ContactEmail)
				assert.Equal(t, original.Metadata, got.Metadata)
			}
		})
	}
}

func TestPostgresStore_UpdateTenant(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	tenant := &auth.Tenant{
		ID:     "tenant-update-1",
		Name:   "Original Name",
		Status: auth.TenantStatusActive,
		Quota:  auth.DefaultQuota(),
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	tests := []struct {
		name    string
		update  *auth.Tenant
		wantErr error
	}{
		{
			name: "valid update",
			update: &auth.Tenant{
				ID:     "tenant-update-1",
				Name:   "Updated Name",
				Status: auth.TenantStatusSuspended,
				Quota:  auth.DefaultQuota(),
			},
			wantErr: nil,
		},
		{
			name:    "not found",
			update:  &auth.Tenant{ID: "tenant-nonexistent", Name: "X"},
			wantErr: auth.ErrTenantNotFound,
		},
		{
			name:    "empty ID",
			update:  &auth.Tenant{ID: ""},
			wantErr: auth.ErrInvalidTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateTenant(ctx, tt.update)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				got, getErr := store.GetTenant(ctx, tt.update.ID)
				require.NoError(t, getErr)
				assert.Equal(t, "Updated Name", got.Name)
				assert.Equal(t, auth.TenantStatusSuspended, got.Status)
			}
		})
	}
}

func TestPostgresStore_DeleteTenant(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	tenant := &auth.Tenant{
		ID:     "tenant-delete-1",
		Name:   "Delete Me",
		Status: auth.TenantStatusActive,
		Quota:  auth.DefaultQuota(),
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{name: "existing tenant", id: "tenant-delete-1", wantErr: nil},
		{name: "not found", id: "tenant-nonexistent", wantErr: auth.ErrTenantNotFound},
		{name: "empty ID", id: "", wantErr: auth.ErrInvalidTenantID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.DeleteTenant(ctx, tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				_, getErr := store.GetTenant(ctx, tt.id)
				assert.ErrorIs(t, getErr, auth.ErrTenantNotFound)
			}
		})
	}
}

func TestPostgresStore_ListTenants(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	tenants, err := store.ListTenants(ctx)
	require.NoError(t, err)
	assert.Empty(t, tenants)

	for i := 0; i < 3; i++ {
		require.NoError(t, store.CreateTenant(ctx, &auth.Tenant{
			ID:     "tenant-list-" + string(rune('a'+i)),
			Name:   "List Tenant",
			Status: auth.TenantStatusActive,
			Quota:  auth.DefaultQuota(),
		}))
	}

	tenants, err = store.ListTenants(ctx)
	require.NoError(t, err)
	assert.Len(t, tenants, 3)
}

func TestPostgresStore_IncrementDecrementUsage(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	tenant := &auth.Tenant{
		ID:     "tenant-usage-1",
		Name:   "Usage Tenant",
		Status: auth.TenantStatusActive,
		Quota:  auth.DefaultQuota(),
	}
	require.NoError(t, store.CreateTenant(ctx, tenant))

	// Increment subscriptions usage.
	require.NoError(t, store.IncrementUsage(ctx, "tenant-usage-1", "subscriptions"))
	require.NoError(t, store.IncrementUsage(ctx, "tenant-usage-1", "subscriptions"))

	got, err := store.GetTenant(ctx, "tenant-usage-1")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Usage.Subscriptions)

	// Decrement.
	require.NoError(t, store.DecrementUsage(ctx, "tenant-usage-1", "subscriptions"))

	got, err = store.GetTenant(ctx, "tenant-usage-1")
	require.NoError(t, err)
	assert.Equal(t, 1, got.Usage.Subscriptions)

	// Decrement to zero (not below).
	require.NoError(t, store.DecrementUsage(ctx, "tenant-usage-1", "subscriptions"))
	require.NoError(t, store.DecrementUsage(ctx, "tenant-usage-1", "subscriptions"))

	got, err = store.GetTenant(ctx, "tenant-usage-1")
	require.NoError(t, err)
	assert.Equal(t, 0, got.Usage.Subscriptions)

	// Error cases.
	require.ErrorIs(t, store.IncrementUsage(ctx, "", "subscriptions"), auth.ErrInvalidTenantID)
	require.ErrorIs(t, store.IncrementUsage(ctx, "nonexistent", "subscriptions"), auth.ErrTenantNotFound)
	require.ErrorIs(t, store.DecrementUsage(ctx, "", "subscriptions"), auth.ErrInvalidTenantID)
	require.ErrorIs(t, store.DecrementUsage(ctx, "nonexistent", "subscriptions"), auth.ErrTenantNotFound)
}

// --- User Tests ---

func TestPostgresStore_CreateUser(t *testing.T) {
	db := setupAuthPgContainer(t)

	tests := []struct {
		name    string
		user    *auth.TenantUser
		wantErr error
	}{
		{
			name: "valid mTLS user",
			user: &auth.TenantUser{
				ID:         "user-create-1",
				TenantID:   "tenant-1",
				Subject:    "CN=alice,O=ACME",
				CommonName: "alice",
				Email:      "alice@acme.com",
				RoleID:     "role-operator",
				IsActive:   true,
			},
			wantErr: nil,
		},
		{
			name: "valid OAuth user",
			user: &auth.TenantUser{
				ID:            "user-create-2",
				TenantID:      "tenant-1",
				OAuthSubject:  "keycloak-123",
				OAuthProvider: "keycloak",
				CommonName:    "bob",
				Email:         "bob@acme.com",
				RoleID:        "role-viewer",
				IsActive:      true,
			},
			wantErr: nil,
		},
		{
			name:    "empty ID",
			user:    &auth.TenantUser{ID: ""},
			wantErr: auth.ErrInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestAuthStore(t, db)
			ctx := context.Background()

			err := store.CreateUser(ctx, tt.user)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.False(t, tt.user.CreatedAt.IsZero())
			}
		})
	}
}

func TestPostgresStore_CreateUserDuplicate(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:         "user-dup-1",
		TenantID:   "tenant-1",
		CommonName: "alice",
		RoleID:     "role-1",
		IsActive:   true,
	}

	require.NoError(t, store.CreateUser(ctx, user))

	err := store.CreateUser(ctx, user)
	require.ErrorIs(t, err, auth.ErrUserExists)
}

func TestPostgresStore_GetUser(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:            "user-get-1",
		TenantID:      "tenant-1",
		Subject:       "CN=charlie,O=ACME",
		CommonName:    "charlie",
		Email:         "charlie@acme.com",
		OAuthSubject:  "oauth-charlie",
		OAuthProvider: "keycloak",
		RoleID:        "role-admin",
		IsActive:      true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	got, err := store.GetUser(ctx, "user-get-1")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)
	assert.Equal(t, user.TenantID, got.TenantID)
	assert.Equal(t, user.Subject, got.Subject)
	assert.Equal(t, user.CommonName, got.CommonName)
	assert.Equal(t, user.Email, got.Email)
	assert.Equal(t, user.OAuthSubject, got.OAuthSubject)
	assert.Equal(t, user.OAuthProvider, got.OAuthProvider)
	assert.Equal(t, user.RoleID, got.RoleID)
	assert.Equal(t, user.IsActive, got.IsActive)

	_, err = store.GetUser(ctx, "user-nonexistent")
	require.ErrorIs(t, err, auth.ErrUserNotFound)

	_, err = store.GetUser(ctx, "")
	require.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestPostgresStore_GetUserBySubject(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:         "user-subject-1",
		TenantID:   "tenant-1",
		Subject:    "CN=subjectuser,O=ACME",
		CommonName: "subjectuser",
		RoleID:     "role-1",
		IsActive:   true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	got, err := store.GetUserBySubject(ctx, "CN=subjectuser,O=ACME")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)

	_, err = store.GetUserBySubject(ctx, "CN=nonexistent")
	require.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestPostgresStore_GetUserByOAuthSubject(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:            "user-oauth-1",
		TenantID:      "tenant-1",
		OAuthSubject:  "kc-unique-id-123",
		OAuthProvider: "keycloak",
		CommonName:    "oauthuser",
		RoleID:        "role-1",
		IsActive:      true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	got, err := store.GetUserByOAuthSubject(ctx, "kc-unique-id-123")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)

	_, err = store.GetUserByOAuthSubject(ctx, "nonexistent-oauth")
	require.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestPostgresStore_GetUserByEmail(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:         "user-email-1",
		TenantID:   "tenant-1",
		Email:      "unique@acme.com",
		CommonName: "emailuser",
		RoleID:     "role-1",
		IsActive:   true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	got, err := store.GetUserByEmail(ctx, "unique@acme.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)

	_, err = store.GetUserByEmail(ctx, "missing@acme.com")
	require.ErrorIs(t, err, auth.ErrUserNotFound)
}

func TestPostgresStore_UpdateUser(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID:         "user-update-1",
		TenantID:   "tenant-1",
		CommonName: "updateme",
		RoleID:     "role-viewer",
		IsActive:   true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	user.CommonName = "updated"
	user.RoleID = "role-admin"
	user.IsActive = false

	require.NoError(t, store.UpdateUser(ctx, user))

	got, err := store.GetUser(ctx, "user-update-1")
	require.NoError(t, err)
	assert.Equal(t, "updated", got.CommonName)
	assert.Equal(t, "role-admin", got.RoleID)
	assert.False(t, got.IsActive)

	err = store.UpdateUser(ctx, &auth.TenantUser{ID: "nonexistent"})
	require.ErrorIs(t, err, auth.ErrUserNotFound)

	err = store.UpdateUser(ctx, &auth.TenantUser{ID: ""})
	require.ErrorIs(t, err, auth.ErrInvalidUserID)
}

func TestPostgresStore_DeleteUser(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID: "user-delete-1", TenantID: "tenant-1",
		CommonName: "deleteme", RoleID: "role-1", IsActive: true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	require.NoError(t, store.DeleteUser(ctx, "user-delete-1"))

	_, err := store.GetUser(ctx, "user-delete-1")
	assert.ErrorIs(t, err, auth.ErrUserNotFound)

	require.ErrorIs(t, store.DeleteUser(ctx, "nonexistent"), auth.ErrUserNotFound)
	require.ErrorIs(t, store.DeleteUser(ctx, ""), auth.ErrInvalidUserID)
}

func TestPostgresStore_ListUsersByTenant(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, store.CreateUser(ctx, &auth.TenantUser{
			ID: "user-list-" + string(rune('a'+i)), TenantID: "tenant-list",
			CommonName: "user", RoleID: "role-1", IsActive: true,
		}))
	}

	users, err := store.ListUsersByTenant(ctx, "tenant-list")
	require.NoError(t, err)
	assert.Len(t, users, 3)

	users, err = store.ListUsersByTenant(ctx, "tenant-empty")
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestPostgresStore_UpdateLastLogin(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	user := &auth.TenantUser{
		ID: "user-login-1", TenantID: "tenant-1",
		CommonName: "loginuser", RoleID: "role-1", IsActive: true,
	}
	require.NoError(t, store.CreateUser(ctx, user))

	require.NoError(t, store.UpdateLastLogin(ctx, "user-login-1"))

	got, err := store.GetUser(ctx, "user-login-1")
	require.NoError(t, err)
	assert.False(t, got.LastLoginAt.IsZero())

	require.ErrorIs(t, store.UpdateLastLogin(ctx, ""), auth.ErrInvalidUserID)
}

// --- Role Tests ---

func TestPostgresStore_CreateRole(t *testing.T) {
	db := setupAuthPgContainer(t)

	tests := []struct {
		name    string
		role    *auth.Role
		wantErr error
	}{
		{
			name: "valid role",
			role: &auth.Role{
				ID:          "role-create-1",
				Name:        "custom-role",
				Type:        auth.RoleTypeTenant,
				Description: "A custom role",
				Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
				TenantID:    "tenant-1",
			},
			wantErr: nil,
		},
		{
			name:    "empty ID",
			role:    &auth.Role{ID: ""},
			wantErr: auth.ErrInvalidRoleID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestAuthStore(t, db)
			ctx := context.Background()

			err := store.CreateRole(ctx, tt.role)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.False(t, tt.role.CreatedAt.IsZero())
			}
		})
	}
}

func TestPostgresStore_CreateRoleDuplicate(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	role := &auth.Role{
		ID:          "role-dup-1",
		Name:        "dup-role",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}

	require.NoError(t, store.CreateRole(ctx, role))

	err := store.CreateRole(ctx, role)
	require.ErrorIs(t, err, auth.ErrRoleExists)
}

func TestPostgresStore_GetRole(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	role := &auth.Role{
		ID:          "role-get-1",
		Name:        "get-role",
		Type:        auth.RoleTypePlatform,
		Description: "For get test",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead, auth.PermissionResourcePoolRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	got, err := store.GetRole(ctx, "role-get-1")
	require.NoError(t, err)
	assert.Equal(t, role.ID, got.ID)
	assert.Equal(t, role.Name, got.Name)
	assert.Equal(t, role.Type, got.Type)
	assert.Equal(t, role.Description, got.Description)
	assert.Equal(t, role.Permissions, got.Permissions)

	_, err = store.GetRole(ctx, "role-nonexistent")
	require.ErrorIs(t, err, auth.ErrRoleNotFound)

	_, err = store.GetRole(ctx, "")
	require.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestPostgresStore_GetRoleByName(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	role := &auth.Role{
		ID:          "role-byname-1",
		Name:        "named-role",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	got, err := store.GetRoleByName(ctx, "named-role")
	require.NoError(t, err)
	assert.Equal(t, role.ID, got.ID)

	_, err = store.GetRoleByName(ctx, "nonexistent-role")
	require.ErrorIs(t, err, auth.ErrRoleNotFound)
}

func TestPostgresStore_UpdateRole(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	role := &auth.Role{
		ID:          "role-update-1",
		Name:        "update-me-role",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	role.Description = "Updated description"
	role.Permissions = append(role.Permissions, auth.PermissionResourcePoolRead)

	require.NoError(t, store.UpdateRole(ctx, role))

	got, err := store.GetRole(ctx, "role-update-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated description", got.Description)
	assert.Len(t, got.Permissions, 2)

	err = store.UpdateRole(ctx, &auth.Role{ID: "nonexistent"})
	require.ErrorIs(t, err, auth.ErrRoleNotFound)

	err = store.UpdateRole(ctx, &auth.Role{ID: ""})
	require.ErrorIs(t, err, auth.ErrInvalidRoleID)
}

func TestPostgresStore_DeleteRole(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	role := &auth.Role{
		ID: "role-delete-1", Name: "delete-role",
		Type: auth.RoleTypeTenant, Permissions: []auth.Permission{},
	}
	require.NoError(t, store.CreateRole(ctx, role))

	require.NoError(t, store.DeleteRole(ctx, "role-delete-1"))

	_, err := store.GetRole(ctx, "role-delete-1")
	assert.ErrorIs(t, err, auth.ErrRoleNotFound)

	require.ErrorIs(t, store.DeleteRole(ctx, "nonexistent"), auth.ErrRoleNotFound)
	require.ErrorIs(t, store.DeleteRole(ctx, ""), auth.ErrInvalidRoleID)
}

func TestPostgresStore_ListRoles(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, store.CreateRole(ctx, &auth.Role{
			ID: "role-list-" + string(rune('a'+i)), Name: auth.RoleName("list-role-" + string(rune('a'+i))),
			Type: auth.RoleTypeTenant, Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
		}))
	}

	roles, err := store.ListRoles(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 3)
}

func TestPostgresStore_ListRolesByTenant(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	// Create a global role (empty tenant_id) and a tenant-specific role.
	require.NoError(t, store.CreateRole(ctx, &auth.Role{
		ID: "role-global-1", Name: "global-role-1",
		Type: auth.RoleTypePlatform, Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}))
	require.NoError(t, store.CreateRole(ctx, &auth.Role{
		ID: "role-tenantx-1", Name: "tenantx-role-1",
		Type: auth.RoleTypeTenant, TenantID: "tenant-x",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}))
	require.NoError(t, store.CreateRole(ctx, &auth.Role{
		ID: "role-tenanty-1", Name: "tenanty-role-1",
		Type: auth.RoleTypeTenant, TenantID: "tenant-y",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}))

	// ListRolesByTenant for "tenant-x" should include global + tenant-x roles.
	roles, err := store.ListRolesByTenant(ctx, "tenant-x")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 2)
}

func TestPostgresStore_InitializeDefaultRoles(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	// Initialize default roles.
	require.NoError(t, store.InitializeDefaultRoles(ctx))

	// Verify all default roles exist.
	defaults := auth.GetDefaultRoles()
	for _, d := range defaults {
		got, err := store.GetRole(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, d.Name, got.Name)
	}

	// Idempotent: calling again should not error.
	require.NoError(t, store.InitializeDefaultRoles(ctx))
}

// --- Audit Tests ---

func TestPostgresStore_LogEvent(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	event := &auth.AuditEvent{
		ID:           "audit-1",
		Type:         auth.AuditEventTenantCreated,
		TenantID:     "tenant-1",
		UserID:       "user-1",
		Subject:      "CN=alice",
		ResourceType: "tenant",
		ResourceID:   "tenant-1",
		Action:       "create",
		Details:      map[string]string{"name": "ACME"},
		ClientIP:     "10.0.0.1",
		UserAgent:    "test-agent",
		Timestamp:    time.Now().UTC(),
	}

	require.NoError(t, store.LogEvent(ctx, event))
}

func TestPostgresStore_ListEvents(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	// Log events for different tenants.
	for i := 0; i < 5; i++ {
		require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
			ID:       "audit-list-" + string(rune('a'+i)),
			Type:     auth.AuditEventTenantCreated,
			TenantID: "tenant-events",
			Action:   "create",
		}))
	}
	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID:       "audit-list-other",
		Type:     auth.AuditEventUserCreated,
		TenantID: "tenant-other",
		Action:   "create",
	}))

	// List all events (empty tenant filter).
	events, err := store.ListEvents(ctx, "", 100, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 6)

	// List by tenant.
	events, err = store.ListEvents(ctx, "tenant-events", 100, 0)
	require.NoError(t, err)
	assert.Len(t, events, 5)

	// Pagination.
	events, err = store.ListEvents(ctx, "tenant-events", 2, 0)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestPostgresStore_ListEventsByType(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID: "audit-type-1", Type: auth.AuditEventAuthSuccess, Action: "login",
	}))
	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID: "audit-type-2", Type: auth.AuditEventAuthFailure, Action: "login",
	}))
	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID: "audit-type-3", Type: auth.AuditEventAuthSuccess, Action: "login",
	}))

	events, err := store.ListEventsByType(ctx, auth.AuditEventAuthSuccess, 100)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestPostgresStore_ListEventsByUser(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID: "audit-user-1", Type: auth.AuditEventAuthSuccess, UserID: "user-audit-1", Action: "login",
	}))
	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID: "audit-user-2", Type: auth.AuditEventAuthSuccess, UserID: "user-audit-1", Action: "login",
	}))
	require.NoError(t, store.LogEvent(ctx, &auth.AuditEvent{
		ID: "audit-user-3", Type: auth.AuditEventAuthSuccess, UserID: "user-audit-2", Action: "login",
	}))

	events, err := store.ListEventsByUser(ctx, "user-audit-1", 100)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestPostgresStore_Ping(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := newTestAuthStore(t, db)
	ctx := context.Background()

	require.NoError(t, store.Ping(ctx))
}

func TestPostgresStore_Close(t *testing.T) {
	db := setupAuthPgContainer(t)
	store := auth.NewPostgresStore(db)

	require.NoError(t, store.Close())
}
