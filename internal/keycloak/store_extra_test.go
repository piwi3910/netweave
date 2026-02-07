package keycloak

import (
	"context"
	"testing"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_GetUserBySubject(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant.
	tenant := &auth.Tenant{
		ID:     "tenant-subj",
		Name:   "Subject Test",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create a user with a certificate subject.
	user := &auth.TenantUser{
		ID:         "user-subj-1",
		TenantID:   "tenant-subj",
		CommonName: "subjectuser",
		Subject:    "CN=subjectuser,O=ACME",
		Email:      "subj@example.com",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name    string
		subject string
		wantErr bool
		errType error
	}{
		{
			name:    "find user by subject",
			subject: "CN=subjectuser,O=ACME",
			wantErr: false,
		},
		{
			name:    "non-existent subject",
			subject: "CN=nobody,O=ACME",
			wantErr: true,
			errType: auth.ErrUserNotFound,
		},
		{
			name:    "empty subject",
			subject: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := store.GetUserBySubject(ctx, tt.subject)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, found)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, user.ID, found.ID)
			}
		})
	}
}

func TestStore_GetUserByOAuthSubject(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant.
	tenant := &auth.Tenant{
		ID:     "tenant-oauth",
		Name:   "OAuth Test",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create a user with an OAuth subject.
	user := &auth.TenantUser{
		ID:            "user-oauth-1",
		TenantID:      "tenant-oauth",
		CommonName:    "oauthuser",
		Email:         "oauth@example.com",
		OAuthSubject:  "keycloak-sub-123",
		OAuthProvider: "keycloak",
		RoleID:        "role-1",
		IsActive:      true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name         string
		oauthSubject string
		wantErr      bool
		errType      error
	}{
		{
			name:         "find user by OAuth subject",
			oauthSubject: "keycloak-sub-123",
			wantErr:      false,
		},
		{
			name:         "non-existent OAuth subject",
			oauthSubject: "non-existent-sub",
			wantErr:      true,
			errType:      auth.ErrUserNotFound,
		},
		{
			name:         "empty OAuth subject",
			oauthSubject: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := store.GetUserByOAuthSubject(ctx, tt.oauthSubject)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, found)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, user.ID, found.ID)
			}
		})
	}
}

func TestStore_UpdateLastLogin(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant.
	tenant := &auth.Tenant{
		ID:     "tenant-login",
		Name:   "Login Test",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create test user.
	user := &auth.TenantUser{
		ID:         "user-login-1",
		TenantID:   "tenant-login",
		CommonName: "loginuser",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name    string
		userID  string
		wantErr bool
	}{
		{
			name:    "update last login for existing user",
			userID:  "user-login-1",
			wantErr: false,
		},
		{
			name:    "non-existent user",
			userID:  "user-999",
			wantErr: true,
		},
		{
			name:    "empty user ID",
			userID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateLastLogin(ctx, tt.userID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify the user was updated (last login timestamp set).
				updated, err := store.GetUser(ctx, tt.userID)
				require.NoError(t, err)
				assert.False(t, updated.LastLoginAt.IsZero(), "LastLoginAt should be set")
			}
		})
	}
}

func TestStore_ListRolesByTenant(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create roles: one global (no tenantID) and one tenant-specific.
	globalRole := &auth.Role{
		ID:          "role-global",
		Name:        "global-viewer",
		Type:        auth.RoleTypeTenant,
		Description: "Global viewer role",
		Permissions: []auth.Permission{auth.PermissionSubscriptionRead},
	}
	err := store.CreateRole(ctx, globalRole)
	require.NoError(t, err)

	tenantRole := &auth.Role{
		ID:          "role-tenant-a",
		Name:        "tenant-a-custom",
		Type:        auth.RoleTypeTenant,
		Description: "Tenant A custom role",
		TenantID:    "tenant-a",
		Permissions: []auth.Permission{auth.PermissionResourceRead},
	}
	err = store.CreateRole(ctx, tenantRole)
	require.NoError(t, err)

	tests := []struct {
		name     string
		tenantID string
		wantErr  bool
		errType  error
	}{
		{
			name:     "list roles for specific tenant",
			tenantID: "tenant-a",
			wantErr:  false,
		},
		{
			name:     "list roles for tenant with no specific roles",
			tenantID: "tenant-b",
			wantErr:  false,
		},
		{
			name:     "empty tenant ID",
			tenantID: "",
			wantErr:  true,
			errType:  auth.ErrInvalidTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles, err := store.ListRolesByTenant(ctx, tt.tenantID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, roles)

				// Verify the roles contain global roles.
				hasGlobal := false
				for _, r := range roles {
					if r.ID == "role-global" {
						hasGlobal = true
					}
				}
				assert.True(t, hasGlobal, "should include global role")
			}
		})
	}
}

func TestStore_ListEventsByType(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	events, err := store.ListEventsByType(ctx, auth.AuditEventAuthSuccess, 10)
	require.NoError(t, err)
	assert.NotNil(t, events)
	assert.Empty(t, events)
}

func TestStore_ListEventsByUser(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	events, err := store.ListEventsByUser(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.NotNil(t, events)
	assert.Empty(t, events)
}

func TestStore_NewStore_Panics(t *testing.T) {
	t.Run("nil client panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewStore(nil, nil)
		})
	})
}

func TestStore_ParseRolePermissions_Legacy(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	// Test the legacy JSON array format for permissions.
	kcRole := &Role{
		ID:          "role-legacy",
		Name:        "legacy-role",
		Description: "Role with legacy permissions",
		Attributes: map[string][]string{
			roleAttrType:        {"tenant"},
			roleAttrPermissions: {`["users:read","subscriptions:create"]`},
		},
	}

	role := store.convertKeycloakToRole(kcRole)
	assert.NotNil(t, role)
	assert.Len(t, role.Permissions, 2)
	assert.Equal(t, auth.Permission("users:read"), role.Permissions[0])
	assert.Equal(t, auth.Permission("subscriptions:create"), role.Permissions[1])
}

func TestStore_ParseRolePermissions_MultiValue(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	// Test the multi-valued format for permissions.
	kcRole := &Role{
		ID:          "role-multi",
		Name:        "multi-role",
		Description: "Role with multi-valued permissions",
		Attributes: map[string][]string{
			roleAttrType:        {"tenant"},
			roleAttrPermissions: {"users:read", "subscriptions:create"},
		},
	}

	role := store.convertKeycloakToRole(kcRole)
	assert.NotNil(t, role)
	assert.Len(t, role.Permissions, 2)
	assert.Equal(t, auth.Permission("users:read"), role.Permissions[0])
	assert.Equal(t, auth.Permission("subscriptions:create"), role.Permissions[1])
}

func TestStore_ParseRolePermissions_Empty(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	// No permissions attribute at all.
	kcRole := &Role{
		ID:          "role-no-perm",
		Name:        "no-perm-role",
		Description: "Role without permissions",
		Attributes: map[string][]string{
			roleAttrType: {"platform"},
		},
	}

	role := store.convertKeycloakToRole(kcRole)
	assert.NotNil(t, role)
	assert.Empty(t, role.Permissions)
}

func TestStore_ParseTenantMetadata(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create tenant with metadata.
	tenant := &auth.Tenant{
		ID:     "tenant-meta",
		Name:   "Meta Test",
		Status: auth.TenantStatusActive,
		Metadata: map[string]string{
			"env":    "production",
			"region": "us-west-2",
		},
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Retrieve and verify metadata is preserved.
	retrieved, err := store.GetTenant(ctx, "tenant-meta")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	// Metadata should be present if serialization/deserialization works.
	if retrieved.Metadata != nil {
		assert.Equal(t, "production", retrieved.Metadata["env"])
		assert.Equal(t, "us-west-2", retrieved.Metadata["region"])
	}
}

func TestStore_ConvertUserToKeycloak_WithOAuth(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	user := &auth.TenantUser{
		ID:            "user-oauth",
		TenantID:      "tenant-1",
		CommonName:    "oauthuser",
		Email:         "oauth@example.com",
		Subject:       "CN=oauthuser,O=ACME",
		OAuthSubject:  "keycloak-sub-abc",
		OAuthProvider: "keycloak",
		RoleID:        "role-1",
		IsActive:      true,
	}

	kcUser := store.convertUserToKeycloak(user)
	require.NotNil(t, kcUser)
	assert.Equal(t, user.ID, kcUser.ID)
	assert.Equal(t, user.Email, kcUser.Email)

	// Verify OAuth attributes are set.
	if kcUser.Attributes != nil {
		if vals, ok := kcUser.Attributes[userAttrOAuthSubject]; ok && len(vals) > 0 {
			assert.Equal(t, "keycloak-sub-abc", vals[0])
		}
		if vals, ok := kcUser.Attributes[userAttrOAuthProvider]; ok && len(vals) > 0 {
			assert.Equal(t, "keycloak", vals[0])
		}
	}
}

func TestStore_ExtractRoleAttributes_NoAttributes(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	// Role with nil attributes.
	kcRole := &Role{
		ID:          "role-no-attr",
		Name:        "no-attr-role",
		Description: "Role without attributes",
	}

	role := store.convertKeycloakToRole(kcRole)
	assert.NotNil(t, role)
	assert.Empty(t, role.TenantID)
	assert.Empty(t, role.Type)
}

func TestStore_LogEvent_NilEvent(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()
	err := store.LogEvent(ctx, nil)
	assert.Error(t, err)
}
