package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRole_UnmarshalBinary_InvalidJSON(t *testing.T) {
	var role Role
	err := role.UnmarshalBinary([]byte("not valid json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal role")
}

func TestRole_MarshalBinary_Roundtrip(t *testing.T) {
	role := &Role{
		ID:          "role-1",
		Name:        "admin",
		Type:        RoleTypePlatform,
		Description: "Admin role",
		Permissions: []Permission{PermissionSubscriptionRead, PermissionResourceRead},
	}

	data, err := role.MarshalBinary()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var result Role
	err = result.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, role.ID, result.ID)
	assert.Equal(t, role.Name, result.Name)
}

func TestTenant_UnmarshalBinary_InvalidJSON(t *testing.T) {
	var tenant Tenant
	err := tenant.UnmarshalBinary([]byte("not valid json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal tenant")
}

func TestTenant_MarshalBinary_Roundtrip(t *testing.T) {
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: TenantStatusActive,
	}

	data, err := tenant.MarshalBinary()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var result Tenant
	err = result.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, tenant.ID, result.ID)
	assert.Equal(t, tenant.Name, result.Name)
}

func TestTenantUser_UnmarshalBinary_InvalidJSON(t *testing.T) {
	var user TenantUser
	err := user.UnmarshalBinary([]byte("not valid json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal tenant user")
}

func TestTenantUser_MarshalBinary_Roundtrip(t *testing.T) {
	user := &TenantUser{
		ID:         "user-1",
		TenantID:   "tenant-1",
		CommonName: "alice",
		Email:      "alice@example.com",
		IsActive:   true,
	}

	data, err := user.MarshalBinary()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var result TenantUser
	err = result.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, user.ID, result.ID)
	assert.Equal(t, user.Email, result.Email)
}

func TestAuditEvent_UnmarshalBinary_InvalidJSON(t *testing.T) {
	var event AuditEvent
	err := event.UnmarshalBinary([]byte("not valid json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal audit event")
}

func TestAuditEvent_MarshalBinary_Roundtrip(t *testing.T) {
	event := &AuditEvent{
		ID:       "event-1",
		Type:     AuditEventUserCreated,
		TenantID: "tenant-1",
		UserID:   "user-1",
		Action:   "create",
	}

	data, err := event.MarshalBinary()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var result AuditEvent
	err = result.UnmarshalBinary(data)
	require.NoError(t, err)
	assert.Equal(t, event.ID, result.ID)
	assert.Equal(t, event.Type, result.Type)
}
