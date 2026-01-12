package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		name       string
		role       *Role
		permission Permission
		want       bool
	}{
		{
			name: "role has permission",
			role: &Role{
				ID:          "role-1",
				Name:        RoleViewer,
				Type:        RoleTypeTenant,
				Permissions: []Permission{PermissionSubscriptionRead, PermissionResourcePoolRead},
			},
			permission: PermissionSubscriptionRead,
			want:       true,
		},
		{
			name: "role does not have permission",
			role: &Role{
				ID:          "role-2",
				Name:        RoleViewer,
				Type:        RoleTypeTenant,
				Permissions: []Permission{PermissionSubscriptionRead},
			},
			permission: PermissionSubscriptionCreate,
			want:       false,
		},
		{
			name: "empty permissions",
			role: &Role{
				ID:          "role-3",
				Name:        RoleViewer,
				Type:        RoleTypeTenant,
				Permissions: []Permission{},
			},
			permission: PermissionSubscriptionRead,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.role.HasPermission(tt.permission)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRole_MarshalBinary(t *testing.T) {
	role := &Role{
		ID:          "role-test",
		Name:        RoleAdmin,
		Type:        RoleTypeTenant,
		Description: "Test role",
		Permissions: []Permission{PermissionUserRead, PermissionUserCreate},
	}

	data, err := role.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal and verify.
	var unmarshaled Role
	err = unmarshaled.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, role.ID, unmarshaled.ID)
	assert.Equal(t, role.Name, unmarshaled.Name)
	assert.Equal(t, role.Type, unmarshaled.Type)
	assert.Equal(t, len(role.Permissions), len(unmarshaled.Permissions))
}

func TestTenant_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status TenantStatus
		want   bool
	}{
		{"active tenant", TenantStatusActive, true},
		{"suspended tenant", TenantStatusSuspended, false},
		{"pending deletion", TenantStatusPendingDeletion, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{
				ID:     "tenant-1",
				Status: tt.status,
			}
			assert.Equal(t, tt.want, tenant.IsActive())
		})
	}
}

func TestTenant_QuotaChecks(t *testing.T) {
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota: TenantQuota{
			MaxSubscriptions: 10,
			MaxResourcePools: 5,
			MaxDeployments:   20,
			MaxUsers:         3,
		},
		Usage: TenantUsage{
			Subscriptions: 5,
			ResourcePools: 5,
			Deployments:   10,
			Users:         2,
		},
	}

	t.Run("can create subscription within quota", func(t *testing.T) {
		assert.True(t, tenant.CanCreateSubscription())
	})

	t.Run("cannot create resource pool at quota", func(t *testing.T) {
		assert.False(t, tenant.CanCreateResourcePool())
	})

	t.Run("can create deployment within quota", func(t *testing.T) {
		assert.True(t, tenant.CanCreateDeployment())
	})

	t.Run("can add user within quota", func(t *testing.T) {
		assert.True(t, tenant.CanAddUser())
	})

	t.Run("suspended tenant cannot create anything", func(t *testing.T) {
		suspendedTenant := &Tenant{
			ID:     "tenant-2",
			Status: TenantStatusSuspended,
			Quota:  DefaultQuota(),
			Usage:  TenantUsage{},
		}
		assert.False(t, suspendedTenant.CanCreateSubscription())
		assert.False(t, suspendedTenant.CanCreateResourcePool())
		assert.False(t, suspendedTenant.CanCreateDeployment())
		assert.False(t, suspendedTenant.CanAddUser())
	})
}

func TestTenant_MarshalBinary(t *testing.T) {
	tenant := &Tenant{
		ID:          "tenant-test",
		Name:        "Test Tenant",
		Description: "A test tenant",
		Status:      TenantStatusActive,
		Quota:       DefaultQuota(),
		Usage: TenantUsage{
			Subscriptions: 5,
		},
		Metadata: map[string]string{
			"env": "test",
		},
	}

	data, err := tenant.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal and verify.
	var unmarshaled Tenant
	err = unmarshaled.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, tenant.ID, unmarshaled.ID)
	assert.Equal(t, tenant.Name, unmarshaled.Name)
	assert.Equal(t, tenant.Status, unmarshaled.Status)
	assert.Equal(t, tenant.Usage.Subscriptions, unmarshaled.Usage.Subscriptions)
}

func TestTenantUser_MarshalBinary(t *testing.T) {
	user := &TenantUser{
		ID:         "user-test",
		TenantID:   "tenant-1",
		Subject:    "CN=alice,O=ACME",
		CommonName: "alice",
		Email:      "alice@example.com",
		RoleID:     "role-admin",
		IsActive:   true,
	}

	data, err := user.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal and verify.
	var unmarshaled TenantUser
	err = unmarshaled.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, user.ID, unmarshaled.ID)
	assert.Equal(t, user.TenantID, unmarshaled.TenantID)
	assert.Equal(t, user.Subject, unmarshaled.Subject)
	assert.Equal(t, user.IsActive, unmarshaled.IsActive)
}

func TestAuthenticatedUser_HasPermission(t *testing.T) {
	tests := []struct {
		name       string
		user       *AuthenticatedUser
		permission Permission
		want       bool
	}{
		{
			name: "platform admin has all permissions",
			user: &AuthenticatedUser{
				UserID:          "admin-1",
				TenantID:        "tenant-1",
				IsPlatformAdmin: true,
				Role:            &Role{Permissions: []Permission{}},
			},
			permission: PermissionTenantDelete,
			want:       true,
		},
		{
			name: "user with permission",
			user: &AuthenticatedUser{
				UserID:          "user-1",
				TenantID:        "tenant-1",
				IsPlatformAdmin: false,
				Role: &Role{
					Permissions: []Permission{PermissionSubscriptionRead, PermissionSubscriptionCreate},
				},
			},
			permission: PermissionSubscriptionCreate,
			want:       true,
		},
		{
			name: "user without permission",
			user: &AuthenticatedUser{
				UserID:          "user-2",
				TenantID:        "tenant-1",
				IsPlatformAdmin: false,
				Role: &Role{
					Permissions: []Permission{PermissionSubscriptionRead},
				},
			},
			permission: PermissionSubscriptionDelete,
			want:       false,
		},
		{
			name: "user with nil role",
			user: &AuthenticatedUser{
				UserID:          "user-3",
				TenantID:        "tenant-1",
				IsPlatformAdmin: false,
				Role:            nil,
			},
			permission: PermissionSubscriptionRead,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.user.HasPermission(tt.permission)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuditEvent_MarshalBinary(t *testing.T) {
	event := &AuditEvent{
		ID:           "event-test",
		Type:         AuditEventUserCreated,
		TenantID:     "tenant-1",
		UserID:       "user-1",
		Subject:      "CN=alice,O=ACME",
		ResourceType: "user",
		ResourceID:   "user-2",
		Action:       "create",
		Details: map[string]string{
			"commonName": "bob",
		},
		ClientIP:  "192.168.1.100",
		UserAgent: "Mozilla/5.0",
	}

	data, err := event.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal and verify.
	var unmarshaled AuditEvent
	err = unmarshaled.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, event.ID, unmarshaled.ID)
	assert.Equal(t, event.Type, unmarshaled.Type)
	assert.Equal(t, event.TenantID, unmarshaled.TenantID)
	assert.Equal(t, event.Action, unmarshaled.Action)
}

func TestGetDefaultRoles(t *testing.T) {
	roles := GetDefaultRoles()

	assert.Len(t, roles, 6, "expected 6 default roles")

	roleNames := make(map[RoleName]bool)
	for _, role := range roles {
		roleNames[role.Name] = true

		// Verify each role has required fields.
		assert.NotEmpty(t, role.ID, "role ID should not be empty")
		assert.NotEmpty(t, role.Name, "role name should not be empty")
		assert.NotEmpty(t, role.Type, "role type should not be empty")
		assert.NotEmpty(t, role.Permissions, "role should have permissions")
	}

	// Verify expected roles exist.
	assert.True(t, roleNames[RolePlatformAdmin], "platform-admin role should exist")
	assert.True(t, roleNames[RoleTenantAdmin], "tenant-admin role should exist")
	assert.True(t, roleNames[RoleOwner], "owner role should exist")
	assert.True(t, roleNames[RoleAdmin], "admin role should exist")
	assert.True(t, roleNames[RoleOperator], "operator role should exist")
	assert.True(t, roleNames[RoleViewer], "viewer role should exist")
}

func TestDefaultQuota(t *testing.T) {
	quota := DefaultQuota()

	assert.Equal(t, 100, quota.MaxSubscriptions)
	assert.Equal(t, 50, quota.MaxResourcePools)
	assert.Equal(t, 200, quota.MaxDeployments)
	assert.Equal(t, 20, quota.MaxUsers)
	assert.Equal(t, 1000, quota.MaxRequestsPerMinute)
}
