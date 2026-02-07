package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// testMiddlewareStore is a minimal in-memory store for middleware tests.
type testMiddlewareStore struct {
	logEventErr error
}

func (s *testMiddlewareStore) CreateTenant(_ context.Context, _ *Tenant) error {
	return nil
}

func (s *testMiddlewareStore) GetTenant(_ context.Context, _ string) (*Tenant, error) {
	return nil, ErrTenantNotFound
}

func (s *testMiddlewareStore) UpdateTenant(_ context.Context, _ *Tenant) error { return nil }
func (s *testMiddlewareStore) DeleteTenant(_ context.Context, _ string) error  { return nil }
func (s *testMiddlewareStore) ListTenants(_ context.Context) ([]*Tenant, error) {
	return nil, nil
}

func (s *testMiddlewareStore) IncrementUsage(_ context.Context, _, _ string) error { return nil }
func (s *testMiddlewareStore) DecrementUsage(_ context.Context, _, _ string) error { return nil }

func (s *testMiddlewareStore) CreateUser(_ context.Context, _ *TenantUser) error { return nil }

func (s *testMiddlewareStore) GetUser(_ context.Context, _ string) (*TenantUser, error) {
	return nil, ErrUserNotFound
}

func (s *testMiddlewareStore) GetUserBySubject(_ context.Context, _ string) (*TenantUser, error) {
	return nil, ErrUserNotFound
}

func (s *testMiddlewareStore) GetUserByOAuthSubject(_ context.Context, _ string) (*TenantUser, error) {
	return nil, ErrUserNotFound
}

func (s *testMiddlewareStore) GetUserByEmail(_ context.Context, _ string) (*TenantUser, error) {
	return nil, ErrUserNotFound
}

func (s *testMiddlewareStore) UpdateUser(_ context.Context, _ *TenantUser) error { return nil }
func (s *testMiddlewareStore) DeleteUser(_ context.Context, _ string) error      { return nil }

func (s *testMiddlewareStore) ListUsersByTenant(_ context.Context, _ string) ([]*TenantUser, error) {
	return nil, nil
}

func (s *testMiddlewareStore) UpdateLastLogin(_ context.Context, _ string) error { return nil }

func (s *testMiddlewareStore) CreateRole(_ context.Context, _ *Role) error { return nil }

func (s *testMiddlewareStore) GetRole(_ context.Context, _ string) (*Role, error) {
	return nil, ErrRoleNotFound
}

func (s *testMiddlewareStore) GetRoleByName(_ context.Context, _ RoleName) (*Role, error) {
	return nil, ErrRoleNotFound
}

func (s *testMiddlewareStore) UpdateRole(_ context.Context, _ *Role) error  { return nil }
func (s *testMiddlewareStore) DeleteRole(_ context.Context, _ string) error { return nil }
func (s *testMiddlewareStore) ListRoles(_ context.Context) ([]*Role, error) { return nil, nil }
func (s *testMiddlewareStore) ListRolesByTenant(_ context.Context, _ string) ([]*Role, error) {
	return nil, nil
}

func (s *testMiddlewareStore) InitializeDefaultRoles(_ context.Context) error { return nil }

func (s *testMiddlewareStore) LogEvent(_ context.Context, _ *AuditEvent) error {
	return s.logEventErr
}

func (s *testMiddlewareStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*AuditEvent, error) {
	return nil, nil
}

func (s *testMiddlewareStore) ListEventsByType(_ context.Context, _ AuditEventType, _ int) ([]*AuditEvent, error) {
	return nil, nil
}

func (s *testMiddlewareStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*AuditEvent, error) {
	return nil, nil
}

func (s *testMiddlewareStore) Ping(_ context.Context) error { return nil }
func (s *testMiddlewareStore) Close() error                 { return nil }

func createTestMiddleware(store Store) *Middleware {
	logger := zap.NewNop()
	config := DefaultMiddlewareConfig()
	config.RequireMTLS = true
	return NewMiddleware(store, config, logger, nil, nil)
}

func createGinContext(w *httptest.ResponseRecorder) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	return c
}

func TestHandleOAuth2AuthenticationFailure(t *testing.T) {
	store := &testMiddlewareStore{}
	mw := createTestMiddleware(store)

	w := httptest.NewRecorder()
	c := createGinContext(w)

	mw.handleOAuth2AuthenticationFailure(c, "req-1", time.Now(), errors.New("token expired"))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired OAuth2 token")
}

func TestHandleMissingCertificate_RequireMTLS(t *testing.T) {
	store := &testMiddlewareStore{}
	mw := createTestMiddleware(store)
	mw.Config.RequireMTLS = true

	w := httptest.NewRecorder()
	c := createGinContext(w)

	mw.handleMissingCertificate(c, "req-1", time.Now())

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Client certificate required")
}

func TestHandleMissingCertificate_NoRequireMTLS(t *testing.T) {
	store := &testMiddlewareStore{}
	mw := createTestMiddleware(store)
	mw.Config.RequireMTLS = false

	w := httptest.NewRecorder()
	c := createGinContext(w)

	mw.handleMissingCertificate(c, "req-1", time.Now())

	// When RequireMTLS is false, it should call c.Next() instead of aborting.
	// Since there are no handlers, status remains 200.
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleAuthenticationError_AllKinds(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "non-authError returns 500",
			err:        errors.New("generic error"),
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Authentication failed",
		},
		{
			name:       "user_lookup with ErrUserNotFound",
			err:        &authError{kind: "user_lookup", err: ErrUserNotFound, subject: "CN=alice"},
			wantStatus: http.StatusForbidden,
			wantMsg:    "Authentication failed",
		},
		{
			name:       "user_lookup with generic error",
			err:        &authError{kind: "user_lookup", err: errors.New("db error"), subject: "CN=alice"},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Authentication failed",
		},
		{
			name:       "user_inactive",
			err:        &authError{kind: "user_inactive", subject: "CN=alice", userID: "user-1"},
			wantStatus: http.StatusForbidden,
			wantMsg:    "Authentication failed",
		},
		{
			name:       "role_lookup",
			err:        &authError{kind: "role_lookup", userID: "user-1", roleID: "role-1", err: errors.New("role error")},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Authentication service temporarily unavailable",
		},
		{
			name:       "tenant_lookup with ErrTenantNotFound",
			err:        &authError{kind: "tenant_lookup", err: ErrTenantNotFound, tenantID: "t-1", userID: "u-1"},
			wantStatus: http.StatusForbidden,
			wantMsg:    "Authentication failed",
		},
		{
			name:       "tenant_lookup with generic error",
			err:        &authError{kind: "tenant_lookup", err: errors.New("db error"), tenantID: "t-1"},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Authentication service temporarily unavailable",
		},
		{
			name:       "tenant_inactive",
			err:        &authError{kind: "tenant_inactive", userID: "user-1", tenantID: "t-1"},
			wantStatus: http.StatusForbidden,
			wantMsg:    "Tenant is suspended",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &testMiddlewareStore{}
			mw := createTestMiddleware(store)

			w := httptest.NewRecorder()
			c := createGinContext(w)

			mw.handleAuthenticationError(c, tt.err, "CN=test", "req-1", time.Now())

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantMsg)
		})
	}
}

func TestLogAuthFailure_StoreError(t *testing.T) {
	store := &testMiddlewareStore{logEventErr: errors.New("store error")}
	mw := createTestMiddleware(store)

	w := httptest.NewRecorder()
	c := createGinContext(w)

	// Should not panic even if store fails.
	mw.logAuthFailure(c.Request.Context(), c, "CN=test", "test reason")
}

func TestLogAccessDenied_StoreError(t *testing.T) {
	store := &testMiddlewareStore{logEventErr: errors.New("store error")}
	mw := createTestMiddleware(store)

	w := httptest.NewRecorder()
	c := createGinContext(w)

	authUser := &AuthenticatedUser{
		UserID:   "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test",
	}

	// Should not panic even if store fails.
	mw.logAccessDenied(c.Request.Context(), c, authUser, PermissionResourceRead)
}
