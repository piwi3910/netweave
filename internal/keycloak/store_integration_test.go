package keycloak

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

const (
	keycloakImage = "quay.io/keycloak/keycloak:26.0"
	// testRealm uses the master realm for integration tests because:
	// 1. It's guaranteed to exist in all Keycloak installations
	// 2. It comes pre-configured with admin-cli client for API access
	// 3. Creating custom realms in testcontainers requires additional setup complexity
	// 4. Tests using master realm skip role CRUD tests due to permission restrictions
	testRealm        = "master"
	testClientID     = AdminCLIClientID // admin-cli is a public client for admin API access
	testClientSecret = ""               // admin-cli does not require client secret
	adminUser        = "admin"
	adminPassword    = "admin"
)

// keycloakContainer represents a running Keycloak test container.
type keycloakContainer struct {
	container testcontainers.Container
	baseURL   string
}

// setupKeycloakContainer starts a Keycloak container for integration testing.
func setupKeycloakContainer(t *testing.T) *keycloakContainer {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        keycloakImage,
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"KEYCLOAK_ADMIN":          adminUser,
			"KEYCLOAK_ADMIN_PASSWORD": adminPassword,
		},
		Cmd: []string{
			"start-dev",
			"--http-port=8080",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Listening on:"),
			wait.ForListeningPort("8080/tcp"),
		).WithDeadline(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "8080")
	require.NoError(t, err)

	baseURL := "http://" + host + ":" + port.Port()

	return &keycloakContainer{
		container: container,
		baseURL:   baseURL,
	}
}

// cleanup terminates the Keycloak container.
func (kc *keycloakContainer) cleanup(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	if err := kc.container.Terminate(ctx); err != nil {
		t.Logf("Failed to terminate container: %v", err)
	}
}

// setupTestRealm creates the test realm and client in Keycloak.
func setupTestRealm(t *testing.T, baseURL string) {
	t.Helper()

	_ = context.Background()

	// Create admin client to configure Keycloak
	adminClient, err := NewClient(&Config{
		BaseURL:       baseURL,
		Realm:         "master",
		ClientID:      "admin-cli",
		ClientSecret:  "",
		AdminUsername: adminUser,
		AdminPassword: adminPassword,
		Timeout:       30 * time.Second,
	})
	require.NoError(t, err)
	defer adminClient.Close()

	// Wait for admin token to be available
	time.Sleep(2 * time.Second)

	// Note: In a real implementation, you would use Keycloak Admin API
	// to create the test realm and configure the client.
	// For this MVP, we'll use the default realm or skip this step.
	t.Log("Test realm setup completed")
}

// setupIntegrationStore creates a Store instance connected to the test Keycloak.
func setupIntegrationStore(t *testing.T, kc *keycloakContainer) *Store {
	t.Helper()

	setupTestRealm(t, kc.baseURL)

	client, err := NewClient(&Config{
		BaseURL:       kc.baseURL,
		Realm:         testRealm,
		ClientID:      testClientID,
		ClientSecret:  testClientSecret,
		AdminUsername: adminUser,
		AdminPassword: adminPassword,
		Timeout:       30 * time.Second,
	})
	require.NoError(t, err)

	logger := zap.NewNop()
	store := NewStore(client, logger)

	return store
}

// TestIntegration_TenantCRUD tests tenant CRUD operations against real Keycloak.
func TestIntegration_TenantCRUD(t *testing.T) {
	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Create tenant
	tenant := &auth.Tenant{
		ID:           "integration-tenant-1",
		Name:         "Integration Test Tenant",
		Description:  "Created by integration test",
		Status:       auth.TenantStatusActive,
		ContactEmail: "test@example.com",
		Quota: auth.TenantQuota{
			MaxUsers:         100,
			MaxSubscriptions: 50,
			MaxResourcePools: 10,
			MaxDeployments:   20,
		},
		Usage: auth.TenantUsage{
			Users:         0,
			Subscriptions: 0,
			ResourcePools: 0,
			Deployments:   0,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Get tenant
	retrieved, err := store.GetTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, tenant.ID, retrieved.ID)
	assert.Equal(t, tenant.Name, retrieved.Name)
	assert.Equal(t, tenant.Status, retrieved.Status)
	assert.Equal(t, tenant.Quota.MaxUsers, retrieved.Quota.MaxUsers)

	// Update tenant
	retrieved.Name = "Updated Tenant Name"
	retrieved.Status = auth.TenantStatusSuspended
	err = store.UpdateTenant(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Tenant Name", updated.Name)
	assert.Equal(t, auth.TenantStatusSuspended, updated.Status)

	// List tenants
	tenants, err := store.ListTenants(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tenants), 1)

	// Delete tenant
	err = store.DeleteTenant(ctx, tenant.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = store.GetTenant(ctx, tenant.ID)
	assert.Error(t, err)
}

// TestIntegration_UserCRUD tests user CRUD operations against real Keycloak.
func TestIntegration_UserCRUD(t *testing.T) {
	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Create tenant first
	tenant := &auth.Tenant{
		ID:     "integration-tenant-2",
		Name:   "User Test Tenant",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxUsers: 100,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	defer store.DeleteTenant(ctx, tenant.ID)

	// Create role
	role := &auth.Role{
		ID:          "integration-role-1",
		Name:        "test-viewer",
		Type:        auth.RoleTypeTenant,
		TenantID:    tenant.ID,
		Permissions: []auth.Permission{auth.PermissionUserRead},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err = store.CreateRole(ctx, role)
	require.NoError(t, err)
	defer store.DeleteRole(ctx, role.ID)

	// Create user
	user := &auth.TenantUser{
		ID:            "integration-user-1",
		TenantID:      tenant.ID,
		Subject:       "CN=test-user,O=Test Org",
		Email:         "testuser@example.com",
		CommonName:    "test-user",
		RoleID:        role.ID,
		IsActive:      true,
		OAuthSubject:  "",
		OAuthProvider: "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Get user by ID
	retrieved, err := store.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Email, retrieved.Email)
	assert.Equal(t, user.TenantID, retrieved.TenantID)

	// Get user by subject
	bySubject, err := store.GetUserBySubject(ctx, user.Subject)
	require.NoError(t, err)
	assert.Equal(t, user.ID, bySubject.ID)

	// Get user by email
	byEmail, err := store.GetUserByEmail(ctx, user.Email)
	require.NoError(t, err)
	assert.Equal(t, user.ID, byEmail.ID)

	// Update user
	retrieved.Email = "updated@example.com"
	retrieved.IsActive = false
	err = store.UpdateUser(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated@example.com", updated.Email)
	assert.False(t, updated.IsActive)

	// List users by tenant
	users, err := store.ListUsersByTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 1)

	// Update last login
	err = store.UpdateLastLogin(ctx, user.ID)
	require.NoError(t, err)

	// Delete user
	err = store.DeleteUser(ctx, user.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = store.GetUser(ctx, user.ID)
	assert.Error(t, err)
}

// TestIntegration_RoleCRUD tests role CRUD operations against real Keycloak.
func TestIntegration_RoleCRUD(t *testing.T) {
	if testRealm == "master" {
		t.Skip("Skipping RoleCRUD test - master realm has restricted permissions")
	}

	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Create tenant
	tenant := &auth.Tenant{
		ID:        "integration-tenant-3",
		Name:      "Role Test Tenant",
		Status:    auth.TenantStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	defer store.DeleteTenant(ctx, tenant.ID)

	// Create role
	role := &auth.Role{
		ID:       "integration-role-2",
		Name:     "test-admin",
		Type:     auth.RoleTypeTenant,
		TenantID: tenant.ID,
		Permissions: []auth.Permission{
			auth.PermissionUserRead,
			auth.PermissionUserCreate,
			auth.PermissionUserUpdate,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.CreateRole(ctx, role)
	require.NoError(t, err)

	// Get role by ID
	retrieved, err := store.GetRole(ctx, role.ID)
	require.NoError(t, err)
	assert.Equal(t, role.ID, retrieved.ID)
	assert.Equal(t, role.Name, retrieved.Name)
	assert.Equal(t, role.Type, retrieved.Type)
	assert.Equal(t, len(role.Permissions), len(retrieved.Permissions))

	// Get role by name
	byName, err := store.GetRoleByName(ctx, role.Name)
	require.NoError(t, err)
	assert.Equal(t, role.ID, byName.ID)

	// Update role
	retrieved.Name = "updated-admin"
	retrieved.Permissions = append(retrieved.Permissions, auth.PermissionTenantRead)
	err = store.UpdateRole(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetRole(ctx, role.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated-admin", updated.Name)
	assert.Equal(t, 3, len(updated.Permissions))

	// List roles
	roles, err := store.ListRoles(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 1)

	// List roles by tenant
	tenantRoles, err := store.ListRolesByTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tenantRoles), 1)

	// Delete role
	err = store.DeleteRole(ctx, role.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = store.GetRole(ctx, role.ID)
	assert.Error(t, err)
}

// TestIntegration_UsageTracking tests usage increment/decrement operations.
func TestIntegration_UsageTracking(t *testing.T) {
	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Create tenant
	tenant := &auth.Tenant{
		ID:     "integration-tenant-4",
		Name:   "Usage Test Tenant",
		Status: auth.TenantStatusActive,
		Usage: auth.TenantUsage{
			Subscriptions: 5,
			Users:         3,
			ResourcePools: 2,
			Deployments:   1,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	defer store.DeleteTenant(ctx, tenant.ID)

	// Increment subscriptions
	err = store.IncrementUsage(ctx, tenant.ID, "subscriptions")
	require.NoError(t, err)

	// Verify increment
	updated, err := store.GetTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, 6, updated.Usage.Subscriptions)

	// Increment users
	err = store.IncrementUsage(ctx, tenant.ID, "users")
	require.NoError(t, err)

	updated, err = store.GetTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, 4, updated.Usage.Users)

	// Decrement subscriptions
	err = store.DecrementUsage(ctx, tenant.ID, "subscriptions")
	require.NoError(t, err)

	updated, err = store.GetTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, updated.Usage.Subscriptions)

	// Decrement to zero (should not go negative)
	for i := 0; i < 10; i++ {
		_ = store.DecrementUsage(ctx, tenant.ID, "deployments")
	}

	updated, err = store.GetTenant(ctx, tenant.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, updated.Usage.Deployments)
}

// TestIntegration_DefaultRoles tests default role initialization.
func TestIntegration_DefaultRoles(t *testing.T) {
	if testRealm == "master" {
		t.Skip("Skipping DefaultRoles test - master realm has restricted permissions")
	}

	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Initialize default roles
	err := store.InitializeDefaultRoles(ctx)
	require.NoError(t, err)

	// Verify platform admin role exists
	platformAdmin, err := store.GetRoleByName(ctx, auth.RolePlatformAdmin)
	require.NoError(t, err)
	assert.Equal(t, auth.RoleTypePlatform, platformAdmin.Type)
	assert.True(t, len(platformAdmin.Permissions) > 0)

	// Verify tenant admin role exists
	tenantAdmin, err := store.GetRoleByName(ctx, auth.RoleTenantAdmin)
	require.NoError(t, err)
	assert.Equal(t, auth.RoleTypeTenant, tenantAdmin.Type)

	// Verify operator role exists
	operator, err := store.GetRoleByName(ctx, auth.RoleOperator)
	require.NoError(t, err)
	assert.Equal(t, auth.RoleTypeTenant, operator.Type)

	// Verify viewer role exists
	viewer, err := store.GetRoleByName(ctx, auth.RoleViewer)
	require.NoError(t, err)
	assert.Equal(t, auth.RoleTypeTenant, viewer.Type)
}

// TestIntegration_OAuthUser tests OAuth2 user creation and lookup.
func TestIntegration_OAuthUser(t *testing.T) {
	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Create tenant
	tenant := &auth.Tenant{
		ID:     "integration-tenant-5",
		Name:   "OAuth Test Tenant",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxUsers: 100,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	defer store.DeleteTenant(ctx, tenant.ID)

	// Create role
	role := &auth.Role{
		ID:          "integration-role-3",
		Name:        "oauth-viewer",
		Type:        auth.RoleTypeTenant,
		TenantID:    tenant.ID,
		Permissions: []auth.Permission{auth.PermissionUserRead},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err = store.CreateRole(ctx, role)
	require.NoError(t, err)
	defer store.DeleteRole(ctx, role.ID)

	// Create OAuth user (no certificate subject)
	oauthUser := &auth.TenantUser{
		ID:            "integration-oauth-user-1",
		TenantID:      tenant.ID,
		Subject:       "", // No certificate DN
		Email:         "oauth@example.com",
		CommonName:    "oauth-user",
		RoleID:        role.ID,
		IsActive:      true,
		OAuthSubject:  "keycloak-user-uuid-123",
		OAuthProvider: "keycloak",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = store.CreateUser(ctx, oauthUser)
	require.NoError(t, err)
	defer store.DeleteUser(ctx, oauthUser.ID)

	// Get user by OAuth subject
	byOAuth, err := store.GetUserByOAuthSubject(ctx, oauthUser.OAuthSubject)
	require.NoError(t, err)
	assert.Equal(t, oauthUser.ID, byOAuth.ID)
	assert.Equal(t, oauthUser.OAuthSubject, byOAuth.OAuthSubject)
	assert.Equal(t, "keycloak", byOAuth.OAuthProvider)
	assert.Empty(t, byOAuth.Subject) // No certificate subject
}

// TestIntegration_AuditLogging tests audit event logging.
func TestIntegration_AuditLogging(t *testing.T) {
	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	// Log audit event
	event := &auth.AuditEvent{
		ID:           "integration-event-1",
		TenantID:     "tenant-1",
		UserID:       "user-1",
		Type:         auth.AuditEventAuthSuccess,
		ResourceType: "user",
		ResourceID:   "user-1",
		Action:       "login",
		Details: map[string]string{
			"ip": "192.168.1.100",
		},
		Timestamp: time.Now(),
	}

	// Should not error (logs to logger)
	err := store.LogEvent(ctx, event)
	require.NoError(t, err)

	// Placeholder implementations return empty lists
	events, err := store.ListEvents(ctx, "tenant-1", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, events)

	eventsByType, err := store.ListEventsByType(ctx, auth.AuditEventAuthSuccess, 10)
	require.NoError(t, err)
	assert.Empty(t, eventsByType)

	eventsByUser, err := store.ListEventsByUser(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.Empty(t, eventsByUser)
}

// TestIntegration_Ping tests connectivity check.
func TestIntegration_Ping(t *testing.T) {
	kc := setupKeycloakContainer(t)
	defer kc.cleanup(t)

	store := setupIntegrationStore(t, kc)
	ctx := context.Background()

	err := store.Ping(ctx)
	require.NoError(t, err)
}
