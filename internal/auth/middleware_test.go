package auth_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockStore is a mock implementation of auth.Store for testing.
type mockStore struct {
	users   map[string]*auth.TenantUser
	roles   map[string]*auth.Role
	tenants map[string]*auth.Tenant
	events  []*auth.AuditEvent
}

func newMockStore() *mockStore {
	return &mockStore{
		users:   make(map[string]*auth.TenantUser),
		roles:   make(map[string]*auth.Role),
		tenants: make(map[string]*auth.Tenant),
		events:  make([]*auth.AuditEvent, 0),
	}
}

func (m *mockStore) CreateTenant(_ context.Context, tenant *auth.Tenant) error {
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockStore) GetTenant(_ context.Context, id string) (*auth.Tenant, error) {
	tenant, ok := m.tenants[id]
	if !ok {
		return nil, auth.ErrTenantNotFound
	}
	return tenant, nil
}

func (m *mockStore) UpdateTenant(_ context.Context, tenant *auth.Tenant) error {
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockStore) DeleteTenant(_ context.Context, id string) error {
	delete(m.tenants, id)
	return nil
}

func (m *mockStore) ListTenants(_ context.Context) ([]*auth.Tenant, error) {
	result := make([]*auth.Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		result = append(result, t)
	}
	return result, nil
}

func (m *mockStore) IncrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) DecrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) CreateUser(_ context.Context, user *auth.TenantUser) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockStore) GetUser(_ context.Context, id string) (*auth.TenantUser, error) {
	user, ok := m.users[id]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (m *mockStore) GetUserBySubject(_ context.Context, subject string) (*auth.TenantUser, error) {
	for _, user := range m.users {
		if user.Subject == subject {
			return user, nil
		}
	}
	return nil, auth.ErrUserNotFound
}

func (m *mockStore) UpdateUser(_ context.Context, user *auth.TenantUser) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockStore) DeleteUser(_ context.Context, id string) error {
	delete(m.users, id)
	return nil
}

func (m *mockStore) ListUsersByTenant(_ context.Context, tenantID string) ([]*auth.TenantUser, error) {
	result := make([]*auth.TenantUser, 0)
	for _, user := range m.users {
		if user.TenantID == tenantID {
			result = append(result, user)
		}
	}
	return result, nil
}

func (m *mockStore) UpdateLastLogin(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) CreateRole(_ context.Context, role *auth.Role) error {
	m.roles[role.ID] = role
	return nil
}

func (m *mockStore) GetRole(_ context.Context, id string) (*auth.Role, error) {
	role, ok := m.roles[id]
	if !ok {
		return nil, auth.ErrRoleNotFound
	}
	return role, nil
}

func (m *mockStore) GetRoleByName(_ context.Context, name auth.RoleName) (*auth.Role, error) {
	for _, role := range m.roles {
		if role.Name == name {
			return role, nil
		}
	}
	return nil, auth.ErrRoleNotFound
}

func (m *mockStore) UpdateRole(_ context.Context, role *auth.Role) error {
	m.roles[role.ID] = role
	return nil
}

func (m *mockStore) DeleteRole(_ context.Context, id string) error {
	delete(m.roles, id)
	return nil
}

func (m *mockStore) ListRoles(_ context.Context) ([]*auth.Role, error) {
	result := make([]*auth.Role, 0, len(m.roles))
	for _, r := range m.roles {
		result = append(result, r)
	}
	return result, nil
}

func (m *mockStore) ListRolesByTenant(ctx context.Context, _ string) ([]*auth.Role, error) {
	return m.ListRoles(ctx)
}

func (m *mockStore) InitializeDefaultRoles(_ context.Context) error {
	return nil
}

func (m *mockStore) LogEvent(_ context.Context, event *auth.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*auth.AuditEvent, error) {
	return m.events, nil
}

func (m *mockStore) ListEventsByType(_ context.Context, _ auth.AuditEventType, _ int) ([]*auth.AuditEvent, error) {
	return m.events, nil
}

func (m *mockStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*auth.AuditEvent, error) {
	return m.events, nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

// setupTestMiddleware creates a middleware instance for testing.
func setupTestMiddleware(t *testing.T, store *mockStore, config *auth.MiddlewareConfig) *auth.Middleware {
	t.Helper()
	logger := zap.NewNop()
	if config == nil {
		config = auth.DefaultMiddlewareConfig()
	}
	return auth.NewMiddleware(store, config, logger)
}

// TestMiddleware_AuthenticationMiddleware_SkipPaths tests that excluded paths skip auth.
func TestMiddleware_AuthenticationMiddleware_SkipPaths(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		skipPaths  []string
		wantStatus int
	}{
		{
			name:       "health endpoint skipped",
			path:       "/health",
			skipPaths:  []string{"/health", "/ready"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "metrics endpoint skipped",
			path:       "/metrics",
			skipPaths:  []string{"/metrics"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "wildcard prefix match",
			path:       "/api/v1/public/info",
			skipPaths:  []string{"/api/v1/public/*"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "protected path requires auth",
			path:       "/api/v1/subscriptions",
			skipPaths:  []string{"/health"},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			config := &auth.MiddlewareConfig{
				Enabled:     true,
				SkipPaths:   tt.skipPaths,
				RequireMTLS: true,
			}
			mw := setupTestMiddleware(t, store, config)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(mw.AuthenticationMiddleware())
			router.GET(tt.path, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestMiddleware_AuthenticationMiddleware_SkipPaths_EdgeCases tests edge cases for skip paths.
func TestMiddleware_AuthenticationMiddleware_SkipPaths_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		skipPaths  []string
		wantStatus int
	}{
		{
			name:       "empty skip paths list requires auth",
			path:       "/health",
			skipPaths:  []string{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "nil skip paths list requires auth",
			path:       "/health",
			skipPaths:  nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "path with trailing slash - exact match without slash",
			path:       "/health/",
			skipPaths:  []string{"/health"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "path without trailing slash - skip has slash",
			path:       "/health",
			skipPaths:  []string{"/health/"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "path with query parameters - exact match",
			path:       "/health?check=liveness",
			skipPaths:  []string{"/health"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "nested path under wildcard",
			path:       "/api/v1/public/users/123/profile",
			skipPaths:  []string{"/api/v1/public/*"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "partial path match should not skip",
			path:       "/healthcheck",
			skipPaths:  []string{"/health"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "multiple skip paths - first matches",
			path:       "/ready",
			skipPaths:  []string{"/ready", "/health", "/metrics"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "multiple skip paths - last matches",
			path:       "/metrics",
			skipPaths:  []string{"/ready", "/health", "/metrics"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "root path skipped",
			path:       "/",
			skipPaths:  []string{"/"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "double wildcard pattern",
			path:       "/api/v1/public/nested/deep/path",
			skipPaths:  []string{"/api/*/public/*"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			config := &auth.MiddlewareConfig{
				Enabled:     true,
				SkipPaths:   tt.skipPaths,
				RequireMTLS: true,
			}
			mw := setupTestMiddleware(t, store, config)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(mw.AuthenticationMiddleware())

			// Register routes for all test paths
			router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/health/", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/healthcheck", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/ready", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/metrics", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/api/v1/public/users/:id/profile", func(c *gin.Context) { c.Status(http.StatusOK) })
			router.GET("/api/v1/public/nested/deep/path", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code, "path: %s, skipPaths: %v", tt.path, tt.skipPaths)
		})
	}
}

// TestMiddleware_AuthenticationMiddleware_NoCertificate tests behavior without client cert.
func TestMiddleware_AuthenticationMiddleware_NoCertificate(t *testing.T) {
	tests := []struct {
		name        string
		requireMTLS bool
		wantStatus  int
	}{
		{
			name:        "mTLS required - returns 401",
			requireMTLS: true,
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "mTLS not required - allows through",
			requireMTLS: false,
			wantStatus:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			config := &auth.MiddlewareConfig{
				Enabled:     true,
				SkipPaths:   []string{},
				RequireMTLS: tt.requireMTLS,
			}
			mw := setupTestMiddleware(t, store, config)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(mw.AuthenticationMiddleware())
			router.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestMiddleware_AuthenticationMiddleware_WithCertificate tests auth with valid cert.
func TestMiddleware_AuthenticationMiddleware_WithCertificate(t *testing.T) {
	store := newMockStore()

	// Setup test data.
	testTenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test auth.Tenant",
		Status: auth.TenantStatusActive,
	}
	store.tenants[testTenant.ID] = testTenant

	testRole := &auth.Role{
		ID:   "role-1",
		Name: auth.RoleTenantAdmin,
		Type: auth.RoleTypeTenant,
		Permissions: []auth.Permission{
			auth.PermissionSubscriptionRead,
			auth.PermissionSubscriptionCreate,
		},
	}
	store.roles[testRole.ID] = testRole

	testUser := &auth.TenantUser{
		ID:       "user-1",
		TenantID: testTenant.ID,
		Subject:  "CN=testuser,O=TestOrg",
		RoleID:   testRole.ID,
		IsActive: true,
	}
	store.users[testUser.ID] = testUser

	config := &auth.MiddlewareConfig{
		Enabled:     true,
		SkipPaths:   []string{},
		RequireMTLS: true,
	}
	mw := setupTestMiddleware(t, store, config)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user := auth.UserFromContext(c.Request.Context())
		if user != nil {
			c.JSON(http.StatusOK, gin.H{"user_id": user.UserID})
		} else {
			c.Status(http.StatusInternalServerError)
		}
	})

	// Create request with TLS peer certificate.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{
				Subject: pkix.Name{
					CommonName:   "testuser",
					Organization: []string{"TestOrg"},
				},
			},
		},
	}
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestMiddleware_AuthenticationMiddleware_UserNotFound tests behavior when user not in DB.
func TestMiddleware_AuthenticationMiddleware_UserNotFound(t *testing.T) {
	store := newMockStore()

	config := &auth.MiddlewareConfig{
		Enabled:     true,
		SkipPaths:   []string{},
		RequireMTLS: true,
	}
	mw := setupTestMiddleware(t, store, config)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{
				Subject: pkix.Name{
					CommonName:   "unknownuser",
					Organization: []string{"UnknownOrg"},
				},
			},
		},
	}
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestMiddleware_AuthenticationMiddleware_InactiveUser tests behavior for disabled users.
func TestMiddleware_AuthenticationMiddleware_InactiveUser(t *testing.T) {
	store := newMockStore()

	testUser := &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=inactiveuser,O=TestOrg",
		RoleID:   "role-1",
		IsActive: false,
	}
	store.users[testUser.ID] = testUser

	config := &auth.MiddlewareConfig{
		Enabled:     true,
		SkipPaths:   []string{},
		RequireMTLS: true,
	}
	mw := setupTestMiddleware(t, store, config)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{
				Subject: pkix.Name{
					CommonName:   "inactiveuser",
					Organization: []string{"TestOrg"},
				},
			},
		},
	}
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestMiddleware_RequirePermission tests permission-based authorization.
func TestMiddleware_RequirePermission(t *testing.T) {
	tests := []struct {
		name            string
		userPermissions []auth.Permission
		requiredPerm    auth.Permission
		wantStatus      int
	}{
		{
			name:            "user has permission",
			userPermissions: []auth.Permission{auth.PermissionSubscriptionRead, auth.PermissionSubscriptionCreate},
			requiredPerm:    auth.PermissionSubscriptionRead,
			wantStatus:      http.StatusOK,
		},
		{
			name:            "user lacks permission",
			userPermissions: []auth.Permission{auth.PermissionSubscriptionRead},
			requiredPerm:    auth.PermissionSubscriptionCreate,
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "user has no permissions",
			userPermissions: []auth.Permission{},
			requiredPerm:    auth.PermissionSubscriptionRead,
			wantStatus:      http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			mw := setupTestMiddleware(t, store, nil)

			gin.SetMode(gin.TestMode)
			router := gin.New()

			// Inject authenticated user directly.
			router.Use(func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:   "user-1",
					TenantID: "tenant-1",
					Role: &auth.Role{
						ID:          "role-1",
						Name:        auth.RoleTenantAdmin,
						Permissions: tt.userPermissions,
					},
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.Use(mw.RequirePermission(string(tt.requiredPerm)))
			router.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestMiddleware_RequirePermission_NoUser tests when no user is in context.
func TestMiddleware_RequirePermission_NoUser(t *testing.T) {
	store := newMockStore()
	mw := setupTestMiddleware(t, store, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(mw.RequirePermission(string(auth.PermissionSubscriptionRead)))
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestMiddleware_RequirePlatformAdmin tests platform admin requirement.
func TestMiddleware_RequirePlatformAdmin(t *testing.T) {
	tests := []struct {
		name            string
		isPlatformAdmin bool
		wantStatus      int
	}{
		{
			name:            "platform admin allowed",
			isPlatformAdmin: true,
			wantStatus:      http.StatusOK,
		},
		{
			name:            "non-admin denied",
			isPlatformAdmin: false,
			wantStatus:      http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			mw := setupTestMiddleware(t, store, nil)

			gin.SetMode(gin.TestMode)
			router := gin.New()

			router.Use(func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        "tenant-1",
					IsPlatformAdmin: tt.isPlatformAdmin,
					Role:            &auth.Role{},
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.Use(mw.RequirePlatformAdmin())
			router.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestMiddleware_RequireTenantAccess tests tenant access restrictions.
func TestMiddleware_RequireTenantAccess(t *testing.T) {
	tests := []struct {
		name            string
		userTenantID    string
		targetTenantID  string
		isPlatformAdmin bool
		wantStatus      int
	}{
		{
			name:            "same tenant access allowed",
			userTenantID:    "tenant-1",
			targetTenantID:  "tenant-1",
			isPlatformAdmin: false,
			wantStatus:      http.StatusOK,
		},
		{
			name:            "cross-tenant access denied",
			userTenantID:    "tenant-1",
			targetTenantID:  "tenant-2",
			isPlatformAdmin: false,
			wantStatus:      http.StatusForbidden,
		},
		{
			name:            "platform admin cross-tenant allowed",
			userTenantID:    "tenant-1",
			targetTenantID:  "tenant-2",
			isPlatformAdmin: true,
			wantStatus:      http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			mw := setupTestMiddleware(t, store, nil)

			gin.SetMode(gin.TestMode)
			router := gin.New()

			router.Use(func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:          "user-1",
					TenantID:        tt.userTenantID,
					IsPlatformAdmin: tt.isPlatformAdmin,
					Role:            &auth.Role{},
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.Use(mw.RequireTenantAccess("tenantId"))
			router.GET("/tenants/:tenantId/resources", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/tenants/"+tt.targetTenantID+"/resources", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestMiddleware_RequireAnyPermission tests any permission requirement.
func TestMiddleware_RequireAnyPermission(t *testing.T) {
	tests := []struct {
		name            string
		userPermissions []auth.Permission
		requiredPerms   []auth.Permission
		wantStatus      int
	}{
		{
			name:            "has first permission",
			userPermissions: []auth.Permission{auth.PermissionSubscriptionRead},
			requiredPerms:   []auth.Permission{auth.PermissionSubscriptionRead, auth.PermissionSubscriptionCreate},
			wantStatus:      http.StatusOK,
		},
		{
			name:            "has second permission",
			userPermissions: []auth.Permission{auth.PermissionSubscriptionCreate},
			requiredPerms:   []auth.Permission{auth.PermissionSubscriptionRead, auth.PermissionSubscriptionCreate},
			wantStatus:      http.StatusOK,
		},
		{
			name:            "has neither permission",
			userPermissions: []auth.Permission{auth.PermissionSubscriptionDelete},
			requiredPerms:   []auth.Permission{auth.PermissionSubscriptionRead, auth.PermissionSubscriptionCreate},
			wantStatus:      http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			mw := setupTestMiddleware(t, store, nil)

			gin.SetMode(gin.TestMode)
			router := gin.New()

			router.Use(func(c *gin.Context) {
				user := &auth.AuthenticatedUser{
					UserID:   "user-1",
					TenantID: "tenant-1",
					Role: &auth.Role{
						ID:          "role-1",
						Permissions: tt.userPermissions,
					},
				}
				ctx := auth.ContextWithUser(c.Request.Context(), user)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.Use(mw.RequireAnyPermission(tt.requiredPerms...))
			router.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// TestParseDNHeader tests DN header parsing with various inputs.
func TestParseDNHeader(t *testing.T) {
	mw := &auth.Middleware{Logger: zap.NewNop()}

	tests := []struct {
		name    string
		dn      string
		wantCN  string
		wantOrg string
		wantNil bool
	}{
		{
			name:    "valid DN",
			dn:      "CN=testuser,O=TestOrg,OU=Engineering",
			wantCN:  "testuser",
			wantOrg: "TestOrg",
			wantNil: false,
		},
		{
			name:    "valid DN with spaces",
			dn:      "CN=test user, O=Test Org, OU=Engineering",
			wantCN:  "test user",
			wantOrg: "Test Org",
			wantNil: false,
		},
		{
			name:    "empty DN",
			dn:      "",
			wantNil: true,
		},
		{
			name:    "DN too long",
			dn:      string(make([]byte, auth.MaxDNLength+1)),
			wantNil: true,
		},
		{
			name:    "DN with null byte",
			dn:      "CN=test\x00user,O=TestOrg",
			wantNil: true,
		},
		{
			name:    "DN missing CN",
			dn:      "O=TestOrg,OU=Engineering",
			wantNil: true,
		},
		{
			name:    "DN with only CN",
			dn:      "CN=testuser",
			wantCN:  "testuser",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mw.ParseDNHeader(tt.dn)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantCN, result.CommonName)
				if tt.wantOrg != "" {
					require.NotEmpty(t, result.Subject.Organization)
					assert.Equal(t, tt.wantOrg, result.Subject.Organization[0])
				}
			}
		})
	}
}

// TestIsValidDNString tests DN string validation.
func TestIsValidDNString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid string",
			input: "CN=testuser,O=TestOrg",
			want:  true,
		},
		{
			name:  "string with tab",
			input: "CN=test\tuser,O=TestOrg",
			want:  true,
		},
		{
			name:  "string with null byte",
			input: "CN=test\x00user",
			want:  false,
		},
		{
			name:  "string with newline",
			input: "CN=test\nuser",
			want:  false,
		},
		{
			name:  "string with carriage return",
			input: "CN=test\ruser",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.IsValidDNString(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsValidDNKey tests DN key validation.
func TestIsValidDNKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid key CN",
			input: "CN",
			want:  true,
		},
		{
			name:  "valid key O",
			input: "O",
			want:  true,
		},
		{
			name:  "valid key with numbers",
			input: "OU2",
			want:  true,
		},
		{
			name:  "invalid key with hyphen",
			input: "CN-name",
			want:  false,
		},
		{
			name:  "invalid key with equals",
			input: "CN=",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.IsValidDNKey(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSanitizeDNValue tests DN value sanitization.
func TestSanitizeDNValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal value",
			input: "testuser",
			want:  "testuser",
		},
		{
			name:  "value with spaces",
			input: "  test user  ",
			want:  "test user",
		},
		{
			name:  "value with control chars",
			input: "test\x00\x01user",
			want:  "testuser",
		},
		{
			name:  "value with newline",
			input: "test\nuser",
			want:  "testuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.SanitizeDNValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestShouldSkipAuth tests path skip logic.
func TestShouldSkipAuth(t *testing.T) {
	// Use NewMiddleware to ensure patterns are compiled
	mw := auth.NewMiddleware(nil, &auth.MiddlewareConfig{
		SkipPaths: []string{"/health", "/ready", "/api/v1/public/*"},
	}, zap.NewNop())

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "exact match health",
			path: "/health",
			want: true,
		},
		{
			name: "exact match ready",
			path: "/ready",
			want: true,
		},
		{
			name: "wildcard match",
			path: "/api/v1/public/info",
			want: true,
		},
		{
			name: "wildcard match deeper path",
			path: "/api/v1/public/users/123",
			want: true,
		},
		{
			name: "no match",
			path: "/api/v1/subscriptions",
			want: false,
		},
		{
			name: "partial match not allowed",
			path: "/healthz",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mw.ShouldSkipAuth(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMatchesPathPattern tests the glob-style path pattern matching function.
func TestMatchesPathPattern(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pattern string
		want    bool
	}{
		// Exact matches
		{
			name:    "exact match",
			path:    "/api/v1/public",
			pattern: "/api/v1/public",
			want:    true,
		},
		{
			name:    "exact match with trailing slash",
			path:    "/api/v1/public/",
			pattern: "/api/v1/public/",
			want:    true,
		},
		{
			name:    "no match different paths",
			path:    "/api/v1/private",
			pattern: "/api/v1/public",
			want:    false,
		},

		// Single wildcard matching
		{
			name:    "single wildcard matches one segment",
			path:    "/api/v1/public",
			pattern: "/api/*/public",
			want:    true,
		},
		{
			name:    "single wildcard at end",
			path:    "/api/v1/users",
			pattern: "/api/v1/*",
			want:    true,
		},
		{
			name:    "single wildcard at beginning",
			path:    "/api/v1/users",
			pattern: "/*/v1/users",
			want:    true,
		},
		{
			name:    "single wildcard does not match multiple segments",
			path:    "/api/v1/nested/deep/path",
			pattern: "/api/*/path",
			want:    false,
		},

		// Multiple wildcards
		{
			name:    "multiple wildcards",
			path:    "/api/v1/public/resource",
			pattern: "/api/*/public/*",
			want:    true,
		},
		{
			name:    "all wildcards",
			path:    "/a/b/c/d",
			pattern: "/*/*/*/*",
			want:    true,
		},

		// Trailing wildcard matching (matches everything after)
		{
			name:    "trailing wildcard matches nested path",
			path:    "/api/v1/public/nested/deep/path",
			pattern: "/api/*/public/*",
			want:    true,
		},
		{
			name:    "trailing wildcard matches empty",
			path:    "/api/public/",
			pattern: "/api/public/*",
			want:    true,
		},
		{
			name:    "trailing wildcard matches single segment",
			path:    "/api/public/resource",
			pattern: "/api/public/*",
			want:    true,
		},

		// Edge cases
		{
			name:    "empty pattern no match",
			path:    "/api/v1/public",
			pattern: "",
			want:    false,
		},
		{
			name:    "empty path exact match",
			path:    "",
			pattern: "",
			want:    true,
		},
		{
			name:    "root path",
			path:    "/",
			pattern: "/",
			want:    true,
		},
		{
			name:    "wildcard only",
			path:    "/anything",
			pattern: "/*",
			want:    true,
		},
		{
			name:    "double slash in path",
			path:    "/api//v1/public",
			pattern: "/api/*/v1/public",
			want:    false,
		},

		// Security test cases
		{
			name:    "path traversal attempt matches pattern",
			path:    "/api/../admin",
			pattern: "/api/*/admin",
			want:    true, // Pattern matching does not sanitize paths - that's done earlier in request pipeline
		},
		{
			name:    "regex special chars in path",
			path:    "/api/v1/resource.json",
			pattern: "/api/v1/resource.json",
			want:    true,
		},
		{
			name:    "regex special chars with wildcard",
			path:    "/api/v1/resource.json",
			pattern: "/api/*/resource.json",
			want:    true,
		},
		{
			name:    "brackets in path",
			path:    "/api/v1/users[0]",
			pattern: "/api/v1/users[0]",
			want:    true,
		},
		{
			name:    "plus sign in path",
			path:    "/api/v1/search?q=test+query",
			pattern: "/api/v1/search?q=test+query",
			want:    true,
		},

		// Common O2-IMS patterns
		{
			name:    "O2-IMS resource pool wildcard",
			path:    "/o2ims-infrastructureInventory/v1/resourcePools/pool-123",
			pattern: "/o2ims-infrastructureInventory/v1/resourcePools/*",
			want:    true,
		},
		{
			name:    "O2-IMS nested resource",
			path:    "/o2ims-infrastructureInventory/v1/resourcePools/pool-1/resources/res-1",
			pattern: "/o2ims-infrastructureInventory/v1/resourcePools/*/resources/*",
			want:    true,
		},
		{
			name:    "DMS deployment wildcard",
			path:    "/o2ims-deploymentManagement/v1/deployments/deploy-abc",
			pattern: "/o2ims-deploymentManagement/v1/deployments/*",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.MatchesPathPattern(tt.path, tt.pattern)
			assert.Equal(t, tt.want, got, "auth.MatchesPathPattern(%q, %q)", tt.path, tt.pattern)
		})
	}
}

// TestBuildSubject tests subject string building.
func TestBuildSubject(t *testing.T) {
	mw := &auth.Middleware{}

	tests := []struct {
		name string
		cert *auth.CertificateInfo
		want string
	}{
		{
			name: "full subject",
			cert: &auth.CertificateInfo{
				Subject: auth.CertificateSubject{
					CommonName:         "testuser",
					Organization:       []string{"TestOrg"},
					OrganizationalUnit: []string{"Engineering"},
				},
			},
			want: "CN=testuser,O=TestOrg,OU=Engineering",
		},
		{
			name: "CN only",
			cert: &auth.CertificateInfo{
				Subject: auth.CertificateSubject{
					CommonName: "testuser",
				},
			},
			want: "CN=testuser",
		},
		{
			name: "CN and O",
			cert: &auth.CertificateInfo{
				Subject: auth.CertificateSubject{
					CommonName:   "testuser",
					Organization: []string{"TestOrg"},
				},
			},
			want: "CN=testuser,O=TestOrg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mw.BuildSubject(tt.cert)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Testauth.DefaultMiddlewareConfig tests default configuration.
func TestDefaultMiddlewareConfig(t *testing.T) {
	config := auth.DefaultMiddlewareConfig()

	assert.True(t, config.Enabled)
	assert.True(t, config.RequireMTLS)
	assert.Contains(t, config.SkipPaths, "/health")
	assert.Contains(t, config.SkipPaths, "/metrics")
}

// TestNewMiddleware tests middleware creation.
func TestNewMiddleware(t *testing.T) {
	store := newMockStore()
	logger := zap.NewNop()

	t.Run("with config", func(t *testing.T) {
		config := &auth.MiddlewareConfig{Enabled: false}
		mw := auth.NewMiddleware(store, config, logger)
		assert.NotNil(t, mw)
		assert.False(t, mw.Config.Enabled)
	})

	t.Run("without config uses defaults", func(t *testing.T) {
		mw := auth.NewMiddleware(store, nil, logger)
		assert.NotNil(t, mw)
		assert.True(t, mw.Config.Enabled)
	})
}

// TestParseXFCCHeader tests XFCC header parsing.
func TestParseXFCCHeader(t *testing.T) {
	store := newMockStore()
	logger := zap.NewNop()
	mw := auth.NewMiddleware(store, nil, logger)

	tests := []struct {
		name      string
		xfcc      string
		wantNil   bool
		wantCN    string
		wantOrg   string
		wantEmail string
	}{
		{
			name: "valid XFCC header",
			xfcc: `By=spiffe://cluster.local;Hash=abc123;` +
				`Subject="CN=test-user,O=test-org,emailAddress=user@example.com";` +
				`URI=spiffe://cluster.local/ns/default/sa/test`,
			wantNil:   false,
			wantCN:    "test-user",
			wantEmail: "user@example.com",
		},
		{
			name:    "missing Subject field",
			xfcc:    `By=spiffe://cluster.local;Hash=abc123;URI=spiffe://cluster.local/ns/default/sa/test`,
			wantNil: true,
		},
		{
			name:    "malformed Subject field (no closing quote)",
			xfcc:    `By=spiffe://cluster.local;Subject="CN=test-user,O=test-org`,
			wantNil: true,
		},
		{
			name:    "minimal valid XFCC",
			xfcc:    `Subject="CN=minimal"`,
			wantNil: false,
			wantCN:  "minimal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certInfo := mw.ParseXFCCHeader(tt.xfcc)
			if tt.wantNil {
				assert.Nil(t, certInfo)
				return
			}

			require.NotNil(t, certInfo)
			if tt.wantCN != "" {
				assert.Equal(t, tt.wantCN, certInfo.CommonName)
			}
			if tt.wantEmail != "" {
				assert.Equal(t, tt.wantEmail, certInfo.Email)
			}
		})
	}
}

// TestExtractEmail tests email extraction from email list.
func TestExtractEmail(t *testing.T) {
	store := newMockStore()
	logger := zap.NewNop()
	mw := auth.NewMiddleware(store, nil, logger)

	tests := []struct {
		name      string
		emails    []string
		wantEmail string
	}{
		{
			name:      "single email",
			emails:    []string{"user@example.com"},
			wantEmail: "user@example.com",
		},
		{
			name:      "multiple emails (returns first)",
			emails:    []string{"first@example.com", "second@example.com"},
			wantEmail: "first@example.com",
		},
		{
			name:      "empty list",
			emails:    []string{},
			wantEmail: "",
		},
		{
			name:      "nil list",
			emails:    nil,
			wantEmail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email := mw.ExtractEmail(tt.emails)
			assert.Equal(t, tt.wantEmail, email)
		})
	}
}

// TestSanitizeForLogging tests the sanitizeForLogging function.
func TestSanitizeForLogging(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "clean string unchanged",
			input:  "CN=user,O=example",
			maxLen: 100,
			want:   "CN=user,O=example",
		},
		{
			name:   "remove newlines",
			input:  "CN=user\nO=example",
			maxLen: 100,
			want:   "CN=userO=example",
		},
		{
			name:   "remove carriage returns",
			input:  "CN=user\r\nO=example",
			maxLen: 100,
			want:   "CN=userO=example",
		},
		{
			name:   "keep spaces and tabs",
			input:  "CN=user name\tO=example",
			maxLen: 100,
			want:   "CN=user name\tO=example",
		},
		{
			name:   "truncate long strings",
			input:  "CN=very_long_common_name_that_exceeds_maximum_length",
			maxLen: 20,
			want:   "CN=very_long_common_...",
		},
		{
			name:   "remove control characters",
			input:  "CN=user\x00\x01\x02O=example",
			maxLen: 100,
			want:   "CN=userO=example",
		},
		{
			name:   "log injection attack prevented",
			input:  "CN=user\n[ERROR] Fake log entry\nO=evil",
			maxLen: 100,
			want:   "CN=user[ERROR] Fake log entryO=evil",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 100,
			want:   "",
		},
		{
			name:   "only control characters",
			input:  "\n\r\x00\x01",
			maxLen: 100,
			want:   "",
		},
		{
			name:   "unicode characters preserved",
			input:  "CN=用户,O=例",
			maxLen: 100,
			want:   "CN=用户,O=例",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.SanitizeForLogging(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, got)
		})
	}
}
