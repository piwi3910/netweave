package benchmarks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

// BenchmarkMTLSAuthentication benchmarks mTLS certificate authentication.
// Measures the performance of extracting and validating client certificates.
func BenchmarkMTLSAuthentication(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	cfg := &auth.MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: true,
		SkipPaths:   []string{"/health"},
	}

	mw := auth.NewMiddleware(store, cfg, logger, nil, nil)

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{createTestCertificate()},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkOAuth2TokenValidation benchmarks OAuth2 bearer token validation.
// Measures the performance of token extraction and validation.
func BenchmarkOAuth2TokenValidation(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	oauth2Config := &auth.OAuth2Config{
		Enabled:            true,
		Priority:           true,
		AutoProvisionUsers: false,
	}

	mockKeycloak := &mockTokenVerifier{}
	oauth2Auth := auth.NewOAuth2Authenticator(mockKeycloak, store, oauth2Config, logger)

	cfg := &auth.MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
		SkipPaths:   []string{"/health"},
	}

	mw := auth.NewMiddleware(store, cfg, logger, oauth2Auth, oauth2Config)

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkAuthorizationCheck benchmarks RBAC permission checking.
// Measures the performance of permission validation.
func BenchmarkAuthorizationCheck(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	cfg := &auth.MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: true,
		SkipPaths:   []string{},
	}

	mw := auth.NewMiddleware(store, cfg, logger, nil, nil)

	user := &auth.AuthenticatedUser{
		UserID:          "user-123",
		TenantID:        "tenant-123",
		Role:            createTestRole(),
		IsPlatformAdmin: false,
		AuthMethod:      auth.AuthMethodMTLS,
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), user)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.Use(mw.RequirePermission("resources:read"))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkConcurrentAuthentication benchmarks concurrent authentication requests.
// Simulates realistic multi-user load patterns.
func BenchmarkConcurrentAuthentication(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	cfg := &auth.MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: true,
		SkipPaths:   []string{"/health"},
	}

	mw := auth.NewMiddleware(store, cfg, logger, nil, nil)

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{createTestCertificate()},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}
	})
}

// Helper functions

func setupMockAuthStore(b *testing.B) auth.Store {
	b.Helper()

	store := &mockAuthStore{
		users: map[string]*auth.TenantUser{
			"CN=testuser,O=example,OU=test": {
				ID:         "user-123",
				TenantID:   "tenant-123",
				Subject:    "CN=testuser,O=example,OU=test",
				CommonName: "testuser",
				RoleID:     "role-123",
				IsActive:   true,
			},
			"oauth-sub-123": {
				ID:           "user-123",
				TenantID:     "tenant-123",
				OAuthSubject: "oauth-sub-123",
				CommonName:   "testuser",
				RoleID:       "role-123",
				IsActive:     true,
			},
		},
		roles: map[string]*auth.Role{
			"role-123": createTestRole(),
		},
		tenants: map[string]*auth.Tenant{
			"tenant-123": {
				ID:     "tenant-123",
				Name:   "Test Tenant",
				Status: auth.TenantStatusActive,
			},
		},
	}

	return store
}

func createTestCertificate() *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "testuser",
			Organization:       []string{"example"},
			OrganizationalUnit: []string{"test"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
	}
}

func createTestRole() *auth.Role {
	return &auth.Role{
		ID:   "role-123",
		Name: auth.RoleTenantAdmin,
		Type: auth.RoleTypeTenant,
		Permissions: []auth.Permission{
			auth.PermissionResourceRead,
			auth.PermissionResourceCreate,
			auth.PermissionResourcePoolRead,
			auth.PermissionSubscriptionRead,
		},
	}
}

// Mock implementations

type mockAuthStore struct {
	users   map[string]*auth.TenantUser
	roles   map[string]*auth.Role
	tenants map[string]*auth.Tenant
}

// TenantStore methods.

func (m *mockAuthStore) CreateTenant(_ context.Context, _ *auth.Tenant) error {
	return nil
}

func (m *mockAuthStore) GetTenant(_ context.Context, tenantID string) (*auth.Tenant, error) {
	tenant, ok := m.tenants[tenantID]
	if !ok {
		return nil, auth.ErrTenantNotFound
	}
	return tenant, nil
}

func (m *mockAuthStore) UpdateTenant(_ context.Context, _ *auth.Tenant) error {
	return nil
}

func (m *mockAuthStore) DeleteTenant(_ context.Context, _ string) error {
	return nil
}

func (m *mockAuthStore) ListTenants(_ context.Context) ([]*auth.Tenant, error) {
	return nil, nil
}

func (m *mockAuthStore) IncrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockAuthStore) DecrementUsage(_ context.Context, _, _ string) error {
	return nil
}

// UserStore methods.

func (m *mockAuthStore) CreateUser(_ context.Context, _ *auth.TenantUser) error {
	return nil
}

func (m *mockAuthStore) GetUser(_ context.Context, _ string) (*auth.TenantUser, error) {
	return nil, auth.ErrUserNotFound
}

func (m *mockAuthStore) GetUserBySubject(_ context.Context, subject string) (*auth.TenantUser, error) {
	user, ok := m.users[subject]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (m *mockAuthStore) GetUserByOAuthSubject(_ context.Context, subject string) (*auth.TenantUser, error) {
	user, ok := m.users[subject]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (m *mockAuthStore) GetUserByEmail(_ context.Context, _ string) (*auth.TenantUser, error) {
	return nil, auth.ErrUserNotFound
}

func (m *mockAuthStore) UpdateUser(_ context.Context, _ *auth.TenantUser) error {
	return nil
}

func (m *mockAuthStore) DeleteUser(_ context.Context, _ string) error {
	return nil
}

func (m *mockAuthStore) ListUsersByTenant(_ context.Context, _ string) ([]*auth.TenantUser, error) {
	return nil, nil
}

func (m *mockAuthStore) UpdateLastLogin(_ context.Context, _ string) error {
	return nil
}

// RoleStore methods.

func (m *mockAuthStore) CreateRole(_ context.Context, _ *auth.Role) error {
	return nil
}

func (m *mockAuthStore) GetRole(_ context.Context, roleID string) (*auth.Role, error) {
	role, ok := m.roles[roleID]
	if !ok {
		return nil, auth.ErrRoleNotFound
	}
	return role, nil
}

func (m *mockAuthStore) GetRoleByName(_ context.Context, _ auth.RoleName) (*auth.Role, error) {
	return nil, auth.ErrRoleNotFound
}

func (m *mockAuthStore) UpdateRole(_ context.Context, _ *auth.Role) error {
	return nil
}

func (m *mockAuthStore) DeleteRole(_ context.Context, _ string) error {
	return nil
}

func (m *mockAuthStore) ListRoles(_ context.Context) ([]*auth.Role, error) {
	return nil, nil
}

func (m *mockAuthStore) ListRolesByTenant(_ context.Context, _ string) ([]*auth.Role, error) {
	return nil, nil
}

func (m *mockAuthStore) InitializeDefaultRoles(_ context.Context) error {
	return nil
}

// AuditStore methods.

func (m *mockAuthStore) LogEvent(_ context.Context, _ *auth.AuditEvent) error {
	return nil
}

func (m *mockAuthStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*auth.AuditEvent, error) {
	return nil, nil
}

func (m *mockAuthStore) ListEventsByType(_ context.Context, _ auth.AuditEventType, _ int) ([]*auth.AuditEvent, error) {
	return nil, nil
}

func (m *mockAuthStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*auth.AuditEvent, error) {
	return nil, nil
}

// Store methods.

func (m *mockAuthStore) Ping(_ context.Context) error {
	return nil
}

func (m *mockAuthStore) Close() error {
	return nil
}

// Ensure mockAuthStore implements auth.Store.
var _ auth.Store = (*mockAuthStore)(nil)

type mockTokenVerifier struct{}

func (m *mockTokenVerifier) VerifyToken(_ context.Context, _ string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"sub":                "oauth-sub-123",
		"email":              "test@example.com",
		"preferred_username": "testuser",
		"tenant_id":          "tenant-123",
	}, nil
}
