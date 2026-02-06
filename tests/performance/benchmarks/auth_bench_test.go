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
	// Setup
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	config := &auth.MiddlewareConfig{
		Enabled:      true,
		RequireMTLS:  true,
		SkipPaths:    []string{"/health"},
	}

	mw := auth.NewMiddleware(store, config, logger, nil, nil)

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Create test request with client certificate
	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{createTestCertificate()},
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkOAuth2TokenValidation benchmarks OAuth2 bearer token validation.
// Measures the performance of token extraction and validation.
func BenchmarkOAuth2TokenValidation(b *testing.B) {
	// Setup
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

	config := &auth.MiddlewareConfig{
		Enabled:      true,
		RequireMTLS:  false,
		SkipPaths:    []string{"/health"},
	}

	mw := auth.NewMiddleware(store, config, logger, oauth2Auth, oauth2Config)

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Create test request with OAuth2 token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkAuthorizationCheck benchmarks RBAC permission checking.
// Measures the performance of permission validation.
func BenchmarkAuthorizationCheck(b *testing.B) {
	// Setup
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	config := &auth.MiddlewareConfig{
		Enabled:      true,
		RequireMTLS:  true,
		SkipPaths:    []string{},
	}

	mw := auth.NewMiddleware(store, config, logger, nil, nil)

	// Create authenticated user context
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

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkConcurrentAuthentication benchmarks concurrent authentication requests.
// Simulates realistic multi-user load patterns.
func BenchmarkConcurrentAuthentication(b *testing.B) {
	// Setup
	gin.SetMode(gin.ReleaseMode)
	logger := zap.NewNop()
	store := setupMockAuthStore(b)

	config := &auth.MiddlewareConfig{
		Enabled:      true,
		RequireMTLS:  true,
		SkipPaths:    []string{"/health"},
	}

	mw := auth.NewMiddleware(store, config, logger, nil, nil)

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{createTestCertificate()},
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Run with parallelism
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
				ID:            "user-123",
				TenantID:      "tenant-123",
				OAuthSubject:  "oauth-sub-123",
				CommonName:    "testuser",
				RoleID:        "role-123",
				IsActive:      true,
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
			CommonName:   "testuser",
			Organization: []string{"example"},
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
			auth.PermissionResourcesRead,
			auth.PermissionResourcesCreate,
			auth.PermissionResourcePoolsRead,
			auth.PermissionSubscriptionsRead,
		},
	}
}

// Mock implementations

type mockAuthStore struct {
	users   map[string]*auth.TenantUser
	roles   map[string]*auth.Role
	tenants map[string]*auth.Tenant
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

func (m *mockAuthStore) GetRole(_ context.Context, roleID string) (*auth.Role, error) {
	role, ok := m.roles[roleID]
	if !ok {
		return nil, auth.ErrRoleNotFound
	}
	return role, nil
}

func (m *mockAuthStore) GetTenant(_ context.Context, tenantID string) (*auth.Tenant, error) {
	tenant, ok := m.tenants[tenantID]
	if !ok {
		return nil, auth.ErrTenantNotFound
	}
	return tenant, nil
}

func (m *mockAuthStore) UpdateLastLogin(_ context.Context, _ string) error {
	return nil
}

func (m *mockAuthStore) CreateUser(_ context.Context, _ *auth.TenantUser) error {
	return nil
}

func (m *mockAuthStore) ListUsersByTenant(_ context.Context, _ string) ([]*auth.TenantUser, error) {
	return nil, nil
}

func (m *mockAuthStore) LogEvent(_ context.Context, _ *auth.AuditEvent) error {
	return nil
}

func (m *mockAuthStore) Ping(_ context.Context) error {
	return nil
}

func (m *mockAuthStore) Close() error {
	return nil
}

func (m *mockAuthStore) InitializeDefaultRoles(_ context.Context) error {
	return nil
}

type mockTokenVerifier struct{}

func (m *mockTokenVerifier) VerifyToken(_ context.Context, _ string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"sub":                "oauth-sub-123",
		"email":              "test@example.com",
		"preferred_username": "testuser",
		"tenant_id":          "tenant-123",
	}, nil
}
