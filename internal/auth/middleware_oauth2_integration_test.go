package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Common test errors.
var ErrTestInvalidToken = errors.New("invalid token")

// TestMiddleware_DualAuth_OAuth2Priority tests that OAuth2 takes priority when configured.
func TestMiddleware_DualAuth_OAuth2Priority(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup: Create middleware with OAuth2 priority enabled
	store := setupIntegrationStore(t)
	defer store.Close()

	// Create test tenant and roles
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota:  TenantQuota{MaxUsers: 10},
	}
	require.NoError(t, store.CreateTenant(context.Background(), tenant))

	role := &Role{
		ID:          "role-viewer",
		Name:        RoleViewer,
		Type:        RoleTypeTenant,
		Permissions: []Permission{PermissionResourceTypeRead},
	}
	require.NoError(t, store.CreateRole(context.Background(), role))

	// Create OAuth2 user
	oauthUser := &TenantUser{
		ID:            "user-oauth",
		TenantID:      tenant.ID,
		OAuthSubject:  "oauth-sub-123",
		OAuthProvider: "keycloak",
		Email:         "oauth@example.com",
		CommonName:    "OAuth User",
		RoleID:        role.ID,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), oauthUser))

	// Create mTLS user with different subject
	mtlsUser := &TenantUser{
		ID:         "user-mtls",
		TenantID:   tenant.ID,
		Subject:    "CN=mtls-user,O=Test",
		CommonName: "mTLS User",
		RoleID:     role.ID,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), mtlsUser))

	// Setup mock Keycloak client
	mockKeycloak := &mockIntegrationKeycloak{
		verifyTokenFunc: func(ctx context.Context, token string) (map[string]interface{}, error) {
			if token == "valid-oauth-token" {
				return map[string]interface{}{
					"sub":                "oauth-sub-123",
					"email":              "oauth@example.com",
					"preferred_username": "oauth-user",
				}, nil
			}
			return nil, ErrTestInvalidToken
		},
	}

	oauth2Config := &OAuth2Config{
		Enabled:  true,
		Priority: true, // OAuth2 takes priority
	}

	oauth2Auth := NewOAuth2Authenticator(
		mockKeycloak,
		store,
		oauth2Config,
		zap.NewNop(),
	)

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		oauth2Auth,
		oauth2Config,
	)

	// Test: Request with BOTH Bearer token AND certificate
	// Should authenticate via OAuth2 (priority)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user, exists := c.Get("user")
		require.True(t, exists)
		authUser := user.(*AuthenticatedUser)
		c.JSON(http.StatusOK, gin.H{
			"userID":     authUser.UserID,
			"authMethod": authUser.AuthMethod,
		})
	})

	// Create request with both auth methods
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-oauth-token")

	// Add client certificate
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   "mtls-user",
			Organization: []string{"Test"},
		},
	}
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	// Verify OAuth2 was used (priority)
	assert.Equal(t, "user-oauth", response["userID"])
	assert.Equal(t, "oauth2", response["authMethod"])
}

// TestMiddleware_DualAuth_MTLSPriority tests that mTLS takes priority when OAuth2 priority is disabled.
func TestMiddleware_DualAuth_MTLSPriority(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup: Create middleware with OAuth2 priority disabled
	store := setupIntegrationStore(t)
	defer store.Close()

	// Create test tenant and roles
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota:  TenantQuota{MaxUsers: 10},
	}
	require.NoError(t, store.CreateTenant(context.Background(), tenant))

	role := &Role{
		ID:          "role-viewer",
		Name:        RoleViewer,
		Type:        RoleTypeTenant,
		Permissions: []Permission{PermissionResourceTypeRead},
	}
	require.NoError(t, store.CreateRole(context.Background(), role))

	// Create OAuth2 user
	oauthUser := &TenantUser{
		ID:            "user-oauth",
		TenantID:      tenant.ID,
		OAuthSubject:  "oauth-sub-123",
		OAuthProvider: "keycloak",
		Email:         "oauth@example.com",
		CommonName:    "OAuth User",
		RoleID:        role.ID,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), oauthUser))

	// Create mTLS user
	mtlsSubject := "CN=mtls-user,O=Test"
	mtlsUser := &TenantUser{
		ID:         "user-mtls",
		TenantID:   tenant.ID,
		Subject:    mtlsSubject,
		CommonName: "mTLS User",
		RoleID:     role.ID,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), mtlsUser))

	// Setup mock Keycloak client
	mockKeycloak := &mockIntegrationKeycloak{
		verifyTokenFunc: func(ctx context.Context, token string) (map[string]interface{}, error) {
			if token == "valid-oauth-token" {
				return map[string]interface{}{
					"sub":   "oauth-sub-123",
					"email": "oauth@example.com",
				}, nil
			}
			return nil, ErrTestInvalidToken
		},
	}

	oauth2Config := &OAuth2Config{
		Enabled:  true,
		Priority: false, // mTLS takes priority
	}

	oauth2Auth := NewOAuth2Authenticator(
		mockKeycloak,
		store,
		oauth2Config,
		zap.NewNop(),
	)

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		oauth2Auth,
		oauth2Config,
	)

	// Test: Request with BOTH Bearer token AND certificate
	// Should authenticate via mTLS (priority)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user, exists := c.Get("user")
		require.True(t, exists)
		authUser := user.(*AuthenticatedUser)
		c.JSON(http.StatusOK, gin.H{
			"userID":     authUser.UserID,
			"authMethod": authUser.AuthMethod,
		})
	})

	// Create request with both auth methods
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-oauth-token")

	// Add client certificate
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   "mtls-user",
			Organization: []string{"Test"},
		},
	}
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	// Verify mTLS was used (priority)
	assert.Equal(t, "user-mtls", response["userID"])
	assert.Equal(t, "mtls", response["authMethod"])
}

// TestMiddleware_DualAuth_OAuth2Only tests OAuth2-only authentication when no certificate present.
func TestMiddleware_DualAuth_OAuth2Only(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	store := setupIntegrationStore(t)
	defer store.Close()

	// Create test tenant and roles
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota:  TenantQuota{MaxUsers: 10},
	}
	require.NoError(t, store.CreateTenant(context.Background(), tenant))

	role := &Role{
		ID:          "role-viewer",
		Name:        RoleViewer,
		Type:        RoleTypeTenant,
		Permissions: []Permission{PermissionResourceTypeRead},
	}
	require.NoError(t, store.CreateRole(context.Background(), role))

	// Create OAuth2 user
	oauthUser := &TenantUser{
		ID:            "user-oauth",
		TenantID:      tenant.ID,
		OAuthSubject:  "oauth-sub-123",
		OAuthProvider: "keycloak",
		Email:         "oauth@example.com",
		CommonName:    "OAuth User",
		RoleID:        role.ID,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), oauthUser))

	// Setup mock Keycloak client
	mockKeycloak := &mockIntegrationKeycloak{
		verifyTokenFunc: func(ctx context.Context, token string) (map[string]interface{}, error) {
			if token == "valid-oauth-token" {
				return map[string]interface{}{
					"sub":   "oauth-sub-123",
					"email": "oauth@example.com",
				}, nil
			}
			return nil, ErrTestInvalidToken
		},
	}

	oauth2Config := &OAuth2Config{
		Enabled: true,
	}

	oauth2Auth := NewOAuth2Authenticator(
		mockKeycloak,
		store,
		oauth2Config,
		zap.NewNop(),
	)

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		oauth2Auth,
		oauth2Config,
	)

	// Test: Request with ONLY Bearer token (no certificate)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user, exists := c.Get("user")
		require.True(t, exists)
		authUser := user.(*AuthenticatedUser)
		c.JSON(http.StatusOK, gin.H{
			"userID":     authUser.UserID,
			"authMethod": authUser.AuthMethod,
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-oauth-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "user-oauth", response["userID"])
	assert.Equal(t, "oauth2", response["authMethod"])
}

// TestMiddleware_DualAuth_MTLSOnly tests mTLS-only authentication when no token present.
func TestMiddleware_DualAuth_MTLSOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	store := setupIntegrationStore(t)
	defer store.Close()

	// Create test tenant and roles
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota:  TenantQuota{MaxUsers: 10},
	}
	require.NoError(t, store.CreateTenant(context.Background(), tenant))

	role := &Role{
		ID:          "role-viewer",
		Name:        RoleViewer,
		Type:        RoleTypeTenant,
		Permissions: []Permission{PermissionResourceTypeRead},
	}
	require.NoError(t, store.CreateRole(context.Background(), role))

	// Create mTLS user
	mtlsSubject := "CN=mtls-user,O=Test"
	mtlsUser := &TenantUser{
		ID:         "user-mtls",
		TenantID:   tenant.ID,
		Subject:    mtlsSubject,
		CommonName: "mTLS User",
		RoleID:     role.ID,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), mtlsUser))

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		nil, // No OAuth2
		nil,
	)

	// Test: Request with ONLY certificate (no Bearer token)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user, exists := c.Get("user")
		require.True(t, exists)
		authUser := user.(*AuthenticatedUser)
		c.JSON(http.StatusOK, gin.H{
			"userID":     authUser.UserID,
			"authMethod": authUser.AuthMethod,
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)

	// Add client certificate
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   "mtls-user",
			Organization: []string{"Test"},
		},
	}
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "user-mtls", response["userID"])
	assert.Equal(t, "mtls", response["authMethod"])
}

// TestMiddleware_DualAuth_NoAuthMethod tests that request fails when neither auth method present.
func TestMiddleware_DualAuth_NoAuthMethod(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	store := setupIntegrationStore(t)
	defer store.Close()

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: true, // Require some form of authentication
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		nil, // No OAuth2
		nil,
	)

	// Test: Request with NO authentication
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should fail authentication
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestMiddleware_DualAuth_InvalidToken tests that invalid OAuth2 token fails but valid cert succeeds.
func TestMiddleware_DualAuth_InvalidToken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	store := setupIntegrationStore(t)
	defer store.Close()

	// Create test tenant and roles
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota:  TenantQuota{MaxUsers: 10},
	}
	require.NoError(t, store.CreateTenant(context.Background(), tenant))

	role := &Role{
		ID:          "role-viewer",
		Name:        RoleViewer,
		Type:        RoleTypeTenant,
		Permissions: []Permission{PermissionResourceTypeRead},
	}
	require.NoError(t, store.CreateRole(context.Background(), role))

	// Create mTLS user
	mtlsSubject := "CN=mtls-user,O=Test"
	mtlsUser := &TenantUser{
		ID:         "user-mtls",
		TenantID:   tenant.ID,
		Subject:    mtlsSubject,
		CommonName: "mTLS User",
		RoleID:     role.ID,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), mtlsUser))

	// Setup mock Keycloak client that rejects all tokens
	mockKeycloak := &mockIntegrationKeycloak{
		verifyTokenFunc: func(ctx context.Context, token string) (map[string]interface{}, error) {
			return nil, ErrTestInvalidToken
		},
	}

	oauth2Config := &OAuth2Config{
		Enabled:  true,
		Priority: false, // mTLS takes priority
	}

	oauth2Auth := NewOAuth2Authenticator(
		mockKeycloak,
		store,
		oauth2Config,
		zap.NewNop(),
	)

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		oauth2Auth,
		oauth2Config,
	)

	// Test: Request with invalid token but valid certificate
	// Should fall back to mTLS (priority disabled means cert checked first)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user, exists := c.Get("user")
		require.True(t, exists)
		authUser := user.(*AuthenticatedUser)
		c.JSON(http.StatusOK, gin.H{
			"userID":     authUser.UserID,
			"authMethod": authUser.AuthMethod,
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")

	// Add valid client certificate
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   "mtls-user",
			Organization: []string{"Test"},
		},
	}
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	// Should authenticate via mTLS (fallback)
	assert.Equal(t, "user-mtls", response["userID"])
	assert.Equal(t, "mtls", response["authMethod"])
}

// TestMiddleware_DualAuth_ConcurrentRequests tests concurrent requests with different auth methods.
func TestMiddleware_DualAuth_ConcurrentRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	store := setupIntegrationStore(t)
	defer store.Close()

	// Create test tenant and roles
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: TenantStatusActive,
		Quota:  TenantQuota{MaxUsers: 10},
	}
	require.NoError(t, store.CreateTenant(context.Background(), tenant))

	role := &Role{
		ID:          "role-viewer",
		Name:        RoleViewer,
		Type:        RoleTypeTenant,
		Permissions: []Permission{PermissionResourceTypeRead},
	}
	require.NoError(t, store.CreateRole(context.Background(), role))

	// Create OAuth2 user
	oauthUser := &TenantUser{
		ID:            "user-oauth",
		TenantID:      tenant.ID,
		OAuthSubject:  "oauth-sub-123",
		OAuthProvider: "keycloak",
		Email:         "oauth@example.com",
		CommonName:    "OAuth User",
		RoleID:        role.ID,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), oauthUser))

	// Create mTLS user
	mtlsSubject := "CN=mtls-user,O=Test"
	mtlsUser := &TenantUser{
		ID:         "user-mtls",
		TenantID:   tenant.ID,
		Subject:    mtlsSubject,
		CommonName: "mTLS User",
		RoleID:     role.ID,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, store.CreateUser(context.Background(), mtlsUser))

	// Setup mock Keycloak client
	mockKeycloak := &mockIntegrationKeycloak{
		verifyTokenFunc: func(ctx context.Context, token string) (map[string]interface{}, error) {
			if token == "valid-oauth-token" {
				return map[string]interface{}{
					"sub":   "oauth-sub-123",
					"email": "oauth@example.com",
				}, nil
			}
			return nil, ErrTestInvalidToken
		},
	}

	oauth2Config := &OAuth2Config{
		Enabled: true,
	}

	oauth2Auth := NewOAuth2Authenticator(
		mockKeycloak,
		store,
		oauth2Config,
		zap.NewNop(),
	)

	middlewareConfig := &MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: false,
	}

	middleware := NewMiddleware(
		store,
		middlewareConfig,
		zap.NewNop(),
		oauth2Auth,
		oauth2Config,
	)

	// Setup router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthenticationMiddleware())
	router.GET("/test", func(c *gin.Context) {
		user, exists := c.Get("user")
		require.True(t, exists)
		authUser := user.(*AuthenticatedUser)
		c.JSON(http.StatusOK, gin.H{
			"userID":     authUser.UserID,
			"authMethod": authUser.AuthMethod,
		})
	})

	// Test: Send 10 OAuth2 requests and 10 mTLS requests concurrently
	results := make(chan map[string]interface{}, 20)

	// Send OAuth2 requests
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer valid-oauth-token")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				_ = json.Unmarshal(w.Body.Bytes(), &response)
				results <- response
			}
		}()
	}

	// Send mTLS requests
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			cert := &x509.Certificate{
				Subject: pkix.Name{
					CommonName:   "mtls-user",
					Organization: []string{"Test"},
				},
			}
			req.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				_ = json.Unmarshal(w.Body.Bytes(), &response)
				results <- response
			}
		}()
	}

	// Collect results
	oauth2Count := 0
	mtlsCount := 0

	for i := 0; i < 20; i++ {
		select {
		case response := <-results:
			if response["authMethod"] == "oauth2" {
				oauth2Count++
				assert.Equal(t, "user-oauth", response["userID"])
			} else if response["authMethod"] == "mtls" {
				mtlsCount++
				assert.Equal(t, "user-mtls", response["userID"])
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent requests")
		}
	}

	// Verify both auth methods worked correctly
	assert.Equal(t, 10, oauth2Count, "Expected 10 OAuth2 authentications")
	assert.Equal(t, 10, mtlsCount, "Expected 10 mTLS authentications")
}

// mockIntegrationKeycloak mocks the Keycloak client for integration tests.
type mockIntegrationKeycloak struct {
	verifyTokenFunc func(ctx context.Context, token string) (map[string]interface{}, error)
}

func (m *mockIntegrationKeycloak) VerifyToken(ctx context.Context, token string) (map[string]interface{}, error) {
	if m.verifyTokenFunc != nil {
		return m.verifyTokenFunc(ctx, token)
	}
	return nil, ErrTestInvalidToken
}

// setupIntegrationStore creates an in-memory store for integration tests.
func setupIntegrationStore(t *testing.T) Store {
	t.Helper()

	store := &integrationStore{
		users:   make(map[string]*TenantUser),
		roles:   make(map[string]*Role),
		tenants: make(map[string]*Tenant),
		events:  make([]*AuditEvent, 0),
	}

	return store
}

// integrationStore implements Store interface for integration tests.
type integrationStore struct {
	users   map[string]*TenantUser
	roles   map[string]*Role
	tenants map[string]*Tenant
	events  []*AuditEvent
}

func (s *integrationStore) CreateTenant(_ context.Context, tenant *Tenant) error {
	s.tenants[tenant.ID] = tenant
	return nil
}

func (s *integrationStore) GetTenant(_ context.Context, id string) (*Tenant, error) {
	tenant, ok := s.tenants[id]
	if !ok {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

func (s *integrationStore) UpdateTenant(_ context.Context, tenant *Tenant) error {
	s.tenants[tenant.ID] = tenant
	return nil
}

func (s *integrationStore) DeleteTenant(_ context.Context, id string) error {
	delete(s.tenants, id)
	return nil
}

func (s *integrationStore) ListTenants(_ context.Context) ([]*Tenant, error) {
	tenants := make([]*Tenant, 0, len(s.tenants))
	for _, tenant := range s.tenants {
		tenants = append(tenants, tenant)
	}
	return tenants, nil
}

func (s *integrationStore) CreateUser(_ context.Context, user *TenantUser) error {
	s.users[user.ID] = user
	return nil
}

func (s *integrationStore) GetUser(_ context.Context, id string) (*TenantUser, error) {
	user, ok := s.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *integrationStore) GetUserBySubject(_ context.Context, subject string) (*TenantUser, error) {
	for _, user := range s.users {
		if user.Subject == subject {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (s *integrationStore) GetUserByOAuthSubject(_ context.Context, oauthSubject string) (*TenantUser, error) {
	for _, user := range s.users {
		if user.OAuthSubject == oauthSubject {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (s *integrationStore) GetUserByEmail(_ context.Context, email string) (*TenantUser, error) {
	for _, user := range s.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (s *integrationStore) UpdateUser(_ context.Context, user *TenantUser) error {
	s.users[user.ID] = user
	return nil
}

func (s *integrationStore) DeleteUser(_ context.Context, id string) error {
	delete(s.users, id)
	return nil
}

func (s *integrationStore) ListUsersByTenant(_ context.Context, tenantID string) ([]*TenantUser, error) {
	users := make([]*TenantUser, 0)
	for _, user := range s.users {
		if user.TenantID == tenantID {
			users = append(users, user)
		}
	}
	return users, nil
}

func (s *integrationStore) UpdateLastLogin(_ context.Context, userID string) error {
	if user, ok := s.users[userID]; ok {
		user.LastLoginAt = time.Now().UTC()
	}
	return nil
}

func (s *integrationStore) CreateRole(_ context.Context, role *Role) error {
	s.roles[role.ID] = role
	return nil
}

func (s *integrationStore) GetRole(_ context.Context, id string) (*Role, error) {
	role, ok := s.roles[id]
	if !ok {
		return nil, ErrRoleNotFound
	}
	return role, nil
}

func (s *integrationStore) UpdateRole(_ context.Context, role *Role) error {
	s.roles[role.ID] = role
	return nil
}

func (s *integrationStore) DeleteRole(_ context.Context, id string) error {
	delete(s.roles, id)
	return nil
}

func (s *integrationStore) ListRoles(_ context.Context) ([]*Role, error) {
	roles := make([]*Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *integrationStore) ListRolesByTenant(_ context.Context, _ string) ([]*Role, error) {
	return s.ListRoles(context.Background())
}

func (s *integrationStore) LogEvent(_ context.Context, event *AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *integrationStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*AuditEvent, error) {
	return s.events, nil
}

func (s *integrationStore) ListEventsByType(_ context.Context, _ AuditEventType, _ int) ([]*AuditEvent, error) {
	return s.events, nil
}

func (s *integrationStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*AuditEvent, error) {
	return s.events, nil
}

func (s *integrationStore) GetRoleByName(_ context.Context, name RoleName) (*Role, error) {
	for _, role := range s.roles {
		if role.Name == name {
			return role, nil
		}
	}
	return nil, ErrRoleNotFound
}

func (s *integrationStore) IncrementUsage(_ context.Context, tenantID, usageType string) error {
	// Not needed for integration tests
	return nil
}

func (s *integrationStore) DecrementUsage(_ context.Context, tenantID, usageType string) error {
	// Not needed for integration tests
	return nil
}

func (s *integrationStore) InitializeDefaultRoles(_ context.Context) error {
	// Not needed for integration tests
	return nil
}

func (s *integrationStore) Ping(_ context.Context) error {
	return nil
}

func (s *integrationStore) Close() error {
	return nil
}
