package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// tenantContextAuthStore is a mock auth.Store for testing wrapWithTenantContext.
// It implements the full auth.Store interface with stubs, overriding GetTenant
// to return tenants from a configurable map.
type tenantContextAuthStore struct {
	tenants      map[string]*auth.Tenant
	getTenantErr error
}

func newTenantContextAuthStore() *tenantContextAuthStore {
	return &tenantContextAuthStore{
		tenants: make(map[string]*auth.Tenant),
	}
}

func (m *tenantContextAuthStore) GetTenant(_ context.Context, id string) (*auth.Tenant, error) {
	if m.getTenantErr != nil {
		return nil, m.getTenantErr
	}
	tenant, exists := m.tenants[id]
	if !exists {
		return nil, auth.ErrTenantNotFound
	}
	return tenant, nil
}

// Stub implementations for the remaining auth.Store interface methods.

func (m *tenantContextAuthStore) CreateTenant(_ context.Context, t *auth.Tenant) error {
	m.tenants[t.ID] = t
	return nil
}

func (m *tenantContextAuthStore) UpdateTenant(_ context.Context, _ *auth.Tenant) error { return nil }
func (m *tenantContextAuthStore) DeleteTenant(_ context.Context, _ string) error       { return nil }

func (m *tenantContextAuthStore) ListTenants(_ context.Context) ([]*auth.Tenant, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) IncrementUsage(_ context.Context, _, _ string) error { return nil }
func (m *tenantContextAuthStore) DecrementUsage(_ context.Context, _, _ string) error { return nil }

func (m *tenantContextAuthStore) CreateUser(_ context.Context, _ *auth.TenantUser) error { return nil }

func (m *tenantContextAuthStore) GetUser(_ context.Context, _ string) (*auth.TenantUser, error) {
	return nil, auth.ErrUserNotFound
}

func (m *tenantContextAuthStore) GetUserBySubject(_ context.Context, _ string) (*auth.TenantUser, error) {
	return nil, auth.ErrUserNotFound
}

func (m *tenantContextAuthStore) GetUserByOAuthSubject(_ context.Context, _ string) (*auth.TenantUser, error) {
	return nil, auth.ErrUserNotFound
}

func (m *tenantContextAuthStore) GetUserByEmail(_ context.Context, _ string) (*auth.TenantUser, error) {
	return nil, auth.ErrUserNotFound
}

func (m *tenantContextAuthStore) UpdateUser(_ context.Context, _ *auth.TenantUser) error { return nil }
func (m *tenantContextAuthStore) DeleteUser(_ context.Context, _ string) error           { return nil }

func (m *tenantContextAuthStore) ListUsersByTenant(_ context.Context, _ string) ([]*auth.TenantUser, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) UpdateLastLogin(_ context.Context, _ string) error { return nil }

func (m *tenantContextAuthStore) CreateRole(_ context.Context, _ *auth.Role) error { return nil }

func (m *tenantContextAuthStore) GetRole(_ context.Context, _ string) (*auth.Role, error) {
	return nil, auth.ErrRoleNotFound
}

func (m *tenantContextAuthStore) GetRoleByName(_ context.Context, _ auth.RoleName) (*auth.Role, error) {
	return nil, auth.ErrRoleNotFound
}

func (m *tenantContextAuthStore) UpdateRole(_ context.Context, _ *auth.Role) error { return nil }
func (m *tenantContextAuthStore) DeleteRole(_ context.Context, _ string) error     { return nil }

func (m *tenantContextAuthStore) ListRoles(_ context.Context) ([]*auth.Role, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) ListRolesByTenant(_ context.Context, _ string) ([]*auth.Role, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) InitializeDefaultRoles(_ context.Context) error { return nil }

func (m *tenantContextAuthStore) LogEvent(_ context.Context, _ *auth.AuditEvent) error { return nil }

func (m *tenantContextAuthStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*auth.AuditEvent, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) ListEventsByType(_ context.Context, _ auth.AuditEventType, _ int) ([]*auth.AuditEvent, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*auth.AuditEvent, error) {
	return nil, nil
}

func (m *tenantContextAuthStore) Ping(_ context.Context) error { return nil }
func (m *tenantContextAuthStore) Close() error                 { return nil }

// TestWrapWithTenantContext_LoadsTargetTenant verifies that wrapWithTenantContext
// loads the target tenant from the store and sets it in context, so downstream
// handlers get the correct tenant (not the admin's own tenant).
func TestWrapWithTenantContext_LoadsTargetTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	targetTenant := &auth.Tenant{
		ID:     "target-tenant-id",
		Name:   "Target Tenant",
		Status: auth.TenantStatusActive,
		Quota:  auth.TenantQuota{MaxUsers: 10},
		Usage:  auth.TenantUsage{Users: 2},
	}

	adminUser := &auth.AuthenticatedUser{
		UserID:          "admin-user-1",
		TenantID:        "admin-tenant-id",
		Subject:         "CN=admin",
		CommonName:      "Admin User",
		IsPlatformAdmin: true,
	}

	tests := []struct {
		name               string
		tenantID           string
		setupStore         func(*tenantContextAuthStore)
		expectedTenantID   string
		expectedTenantName string
		expectedMaxUsers   int
		expectedUserCount  int
		expectTenantInCtx  bool
		expectedStatus     int
	}{
		{
			name:     "loads target tenant into context",
			tenantID: "target-tenant-id",
			setupStore: func(s *tenantContextAuthStore) {
				s.tenants["target-tenant-id"] = targetTenant
			},
			expectedTenantID:   "target-tenant-id",
			expectedTenantName: "Target Tenant",
			expectedMaxUsers:   10,
			expectedUserCount:  2,
			expectTenantInCtx:  true,
			expectedStatus:     http.StatusOK,
		},
		{
			name:     "overrides user tenant ID",
			tenantID: "target-tenant-id",
			setupStore: func(s *tenantContextAuthStore) {
				s.tenants["target-tenant-id"] = targetTenant
			},
			expectedTenantID:  "target-tenant-id",
			expectTenantInCtx: true,
			expectedStatus:    http.StatusOK,
		},
		{
			name:           "returns 404 when tenant not found",
			tenantID:       "nonexistent-tenant",
			setupStore:     func(_ *tenantContextAuthStore) {},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:     "returns 500 on store error",
			tenantID: "error-tenant",
			setupStore: func(s *tenantContextAuthStore) {
				s.getTenantErr = errors.New("database connection lost")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTenantContextAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				router:        router,
				logger:        logger,
				fullAuthStore: store,
			}

			// The inner handler inspects the context to verify correct tenant.
			var capturedTenant *auth.Tenant
			var capturedTenantID string
			innerHandler := func(c *gin.Context) {
				ctx := c.Request.Context()
				capturedTenant = auth.TenantFromContext(ctx)
				capturedTenantID = auth.TenantIDFromContext(ctx)
				c.JSON(http.StatusOK, gin.H{"ok": true})
			}

			// Register route with wrapWithTenantContext.
			router.GET("/admin/tenants/:tenantId/users", func(c *gin.Context) {
				// Inject admin user context (simulating auth middleware).
				ctx := auth.ContextWithUser(c.Request.Context(), adminUser)
				c.Request = c.Request.WithContext(ctx)
				// Call the wrapped handler.
				srv.wrapWithTenantContext(innerHandler)(c)
			})

			req := httptest.NewRequest(http.MethodGet, "/admin/tenants/"+tt.tenantID+"/users", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus != http.StatusOK {
				return
			}

			// Verify tenant ID was overridden on the user context.
			assert.Equal(t, tt.expectedTenantID, capturedTenantID)

			if tt.expectTenantInCtx {
				require.NotNil(t, capturedTenant, "tenant should be set in context")
				assert.Equal(t, tt.expectedTenantID, capturedTenant.ID)

				if tt.expectedTenantName != "" {
					assert.Equal(t, tt.expectedTenantName, capturedTenant.Name)
				}
				if tt.expectedMaxUsers > 0 {
					assert.Equal(t, tt.expectedMaxUsers, capturedTenant.Quota.MaxUsers)
				}
				if tt.expectedUserCount > 0 {
					assert.Equal(t, tt.expectedUserCount, capturedTenant.Usage.Users)
				}
			}
		})
	}
}

// TestWrapWithTenantContext_NoUserInContext verifies the handler returns 500
// when no authenticated user is in context (defensive check).
func TestWrapWithTenantContext_NoUserInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newTenantContextAuthStore()
	store.tenants["some-tenant"] = &auth.Tenant{
		ID:   "some-tenant",
		Name: "Some Tenant",
	}

	logger := zap.NewNop()
	router := gin.New()

	srv := &Server{
		router:        router,
		logger:        logger,
		fullAuthStore: store,
	}

	handlerCalled := false
	innerHandler := func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}

	router.GET("/admin/tenants/:tenantId/users",
		srv.wrapWithTenantContext(innerHandler),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/some-tenant/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.False(t, handlerCalled, "inner handler should not be called when user is nil")

	var errResp models.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "InternalError", errResp.Error)
	assert.Equal(t, "Authentication context missing", errResp.Message)
}

// TestWrapWithTenantContext_NilAuthStore verifies that wrapWithTenantContext
// still works when fullAuthStore is nil (graceful degradation).
func TestWrapWithTenantContext_NilAuthStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminUser := &auth.AuthenticatedUser{
		UserID:          "admin-1",
		TenantID:        "admin-tenant",
		IsPlatformAdmin: true,
	}

	logger := zap.NewNop()
	router := gin.New()

	srv := &Server{
		router:        router,
		logger:        logger,
		fullAuthStore: nil, // No auth store configured.
	}

	var capturedTenantID string
	var capturedTenant *auth.Tenant
	innerHandler := func(c *gin.Context) {
		ctx := c.Request.Context()
		capturedTenantID = auth.TenantIDFromContext(ctx)
		capturedTenant = auth.TenantFromContext(ctx)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}

	router.GET("/admin/tenants/:tenantId/users", func(c *gin.Context) {
		ctx := auth.ContextWithUser(c.Request.Context(), adminUser)
		c.Request = c.Request.WithContext(ctx)
		srv.wrapWithTenantContext(innerHandler)(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/target-tenant/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// TenantID should still be overridden on the user context.
	assert.Equal(t, "target-tenant", capturedTenantID)
	// Tenant object should not be set since there's no auth store.
	assert.Nil(t, capturedTenant)
}

// TestWrapWithTenantContext_CanAddUserQuotaCheck verifies the complete flow:
// admin creates user in target tenant, and quota is checked against the target
// tenant (not the admin's tenant).
func TestWrapWithTenantContext_CanAddUserQuotaCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		adminTenant  *auth.Tenant
		targetTenant *auth.Tenant
		expectCanAdd bool
	}{
		{
			name: "target tenant has quota available",
			adminTenant: &auth.Tenant{
				ID:     "admin-tenant",
				Status: auth.TenantStatusActive,
				Quota:  auth.TenantQuota{MaxUsers: 1},
				Usage:  auth.TenantUsage{Users: 1}, // Admin tenant is full.
			},
			targetTenant: &auth.Tenant{
				ID:     "target-tenant",
				Status: auth.TenantStatusActive,
				Quota:  auth.TenantQuota{MaxUsers: 10},
				Usage:  auth.TenantUsage{Users: 2}, // Target has room.
			},
			expectCanAdd: true,
		},
		{
			name: "target tenant quota is exhausted",
			adminTenant: &auth.Tenant{
				ID:     "admin-tenant",
				Status: auth.TenantStatusActive,
				Quota:  auth.TenantQuota{MaxUsers: 100},
				Usage:  auth.TenantUsage{Users: 0}, // Admin tenant has room.
			},
			targetTenant: &auth.Tenant{
				ID:     "target-tenant",
				Status: auth.TenantStatusActive,
				Quota:  auth.TenantQuota{MaxUsers: 5},
				Usage:  auth.TenantUsage{Users: 5}, // Target is full.
			},
			expectCanAdd: false,
		},
		{
			name: "target tenant is suspended",
			adminTenant: &auth.Tenant{
				ID:     "admin-tenant",
				Status: auth.TenantStatusActive,
				Quota:  auth.TenantQuota{MaxUsers: 100},
				Usage:  auth.TenantUsage{Users: 0},
			},
			targetTenant: &auth.Tenant{
				ID:        "target-tenant",
				Status:    auth.TenantStatusSuspended,
				Quota:     auth.TenantQuota{MaxUsers: 10},
				Usage:     auth.TenantUsage{Users: 0},
				CreatedAt: time.Now(),
			},
			expectCanAdd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTenantContextAuthStore()
			store.tenants[tt.adminTenant.ID] = tt.adminTenant
			store.tenants[tt.targetTenant.ID] = tt.targetTenant

			logger := zap.NewNop()
			router := gin.New()

			srv := &Server{
				router:        router,
				logger:        logger,
				fullAuthStore: store,
			}

			adminUser := &auth.AuthenticatedUser{
				UserID:          "admin-user",
				TenantID:        tt.adminTenant.ID,
				IsPlatformAdmin: true,
			}

			// The handler reads the tenant from context and checks CanAddUser.
			var canAdd bool
			innerHandler := func(c *gin.Context) {
				tenant := auth.TenantFromContext(c.Request.Context())
				require.NotNil(t, tenant, "target tenant must be in context")
				canAdd = tenant.CanAddUser()
				c.JSON(http.StatusOK, gin.H{"canAdd": canAdd})
			}

			router.POST("/admin/tenants/:tenantId/users", func(c *gin.Context) {
				ctx := auth.ContextWithUser(c.Request.Context(), adminUser)
				// Set the admin's own tenant in context (as auth middleware would).
				ctx = auth.ContextWithTenant(ctx, tt.adminTenant)
				c.Request = c.Request.WithContext(ctx)
				srv.wrapWithTenantContext(innerHandler)(c)
			})

			req := httptest.NewRequest(http.MethodPost, "/admin/tenants/"+tt.targetTenant.ID+"/users", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tt.expectCanAdd, canAdd,
				"CanAddUser() should reflect target tenant quota, not admin tenant")
		})
	}
}
