package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// === User handler additional tests ===

func TestUser_UpdateWithRoleChange(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
		RoleID:   "role-viewer",
	}
	store.roles["role-admin"] = &auth.Role{
		ID:   "role-admin",
		Name: auth.RoleTenantAdmin,
		Type: auth.RoleTypeTenant,
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateUserRequest{
		RoleID: "role-admin",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp auth.TenantUser
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "role-admin", resp.RoleID)
}

func TestUser_UpdateWithPlatformRole(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
		RoleID:   "role-viewer",
	}
	store.roles["role-platform"] = &auth.Role{
		ID:   "role-platform",
		Name: auth.RolePlatformAdmin,
		Type: auth.RoleTypePlatform,
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateUserRequest{
		RoleID: "role-platform",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_UpdateInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateUserRequest{
		Email: "not-valid-email",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_UpdateInvalidJSON(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}

	router := setupUserTestRouter(t, store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenant/users/user-1", bytes.NewReader([]byte("{invalid}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "other-user")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_CreateWithInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
		Quota:  auth.TenantQuota{MaxUsers: 10},
	}

	router := setupUserTestRouter(t, store)

	body, _ := json.Marshal(handlers.CreateUserRequest{
		Subject:    "CN=test,O=org",
		CommonName: "Test User",
		Email:      "not-an-email",
		RoleID:     "role-1",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenant/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_CreateUserExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
		Quota:  auth.TenantQuota{MaxUsers: 10},
	}
	store.roles["role-1"] = &auth.Role{
		ID:   "role-1",
		Name: auth.RoleViewer,
		Type: auth.RoleTypeTenant,
	}
	// Use a userExistsStore to simulate ErrUserExists on CreateUser
	dupStore := &userExistsStore{mockAuthStore: store}

	handler := handlers.NewUserHandler(dupStore, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant := store.tenants["tenant-1"]
		ctx := auth.ContextWithTenant(c.Request.Context(), tenant)
		user := &auth.AuthenticatedUser{UserID: "caller", TenantID: "tenant-1"}
		ctx = auth.ContextWithUser(ctx, user)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/admin/tenant/users", handler.CreateUser)

	body, _ := json.Marshal(handlers.CreateUserRequest{
		Subject:    "CN=existing,O=org",
		CommonName: "Existing User",
		RoleID:     "role-1",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenant/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUser_DeleteSelf(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["tenant-1"] = &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test",
		Status: auth.TenantStatusActive,
	}
	store.users["user-1"] = &auth.TenantUser{
		ID:       "user-1",
		TenantID: "tenant-1",
		Subject:  "CN=test,O=org",
	}

	router := setupUserTestRouter(t, store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/tenant/users/user-1", nil)
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-User-ID", "user-1") // Same user trying to delete themselves
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_GetCurrentUserNoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newMockAuthStore()
	handler := handlers.NewUserHandler(store, zap.NewNop())

	// No user context set
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/tenant/me", nil)

	handler.GetCurrentUser(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// === Tenant handler additional tests ===

func TestTenant_CreateInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.CreateTenantRequest{
		Name:         "Valid Name",
		ContactEmail: "invalid-email",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/platform/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_CreateInvalidName(t *testing.T) {
	store := newMockAuthStore()
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.CreateTenantRequest{
		Name: "Invalid@Name!",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/platform/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_UpdateInvalidName(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		Name: "Invalid@Name!",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_UpdateInvalidEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		ContactEmail: "bad-email",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenant_UpdateWithQuota(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		Quota: &auth.TenantQuota{
			MaxSubscriptions: 200,
			MaxUsers:         50,
		},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTenant_UpdateContactEmail(t *testing.T) {
	store := newMockAuthStore()
	store.tenants["t-1"] = &auth.Tenant{
		ID:     "t-1",
		Name:   "Original",
		Status: auth.TenantStatusActive,
	}
	router := setupTenantTestRouter(t, store)

	body, _ := json.Marshal(handlers.UpdateTenantRequest{
		ContactEmail: "new@example.com",
		Metadata:     map[string]string{"key": "value"},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/tenants/t-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTenant_GetCurrentTenantWithContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newMockAuthStore()
	handler := handlers.NewTenantHandler(store, zap.NewNop())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenant := &auth.Tenant{
			ID:     "tenant-1",
			Name:   "My Tenant",
			Status: auth.TenantStatusActive,
		}
		ctx := auth.ContextWithTenant(c.Request.Context(), tenant)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/admin/tenant", handler.GetCurrentTenant)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenant", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp auth.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "My Tenant", resp.Name)
}

// === Backend handler additional tests ===

func TestBackend_UpdateWithConfig(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{
		ID:          "b1",
		Name:        "k8s",
		Category:    "ims",
		AdapterType: "kubernetes",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	body, _ := json.Marshal(handlers.UpdateBackendRequest{
		Config:      map[string]string{"context": "new-prod"},
		Credentials: map[string]string{"token": "secret"},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "/admin/infrastructure/backends/b1", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBackend_UpdateEncryptionError(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{
		ID:          "b1",
		Name:        "k8s",
		Category:    "ims",
		AdapterType: "kubernetes",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{failEncrypt: true})

	body, _ := json.Marshal(handlers.UpdateBackendRequest{
		Config: map[string]string{"context": "prod"},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "/admin/infrastructure/backends/b1", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBackend_TestNotFound(t *testing.T) {
	store := newMockBackendStore()
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPost, "/admin/infrastructure/backends/missing/test", nil,
	)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBackend_UpdateInvalidJSON(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{
		ID: "b1", Name: "k8s",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPut, "/admin/infrastructure/backends/b1", bytes.NewReader([]byte("{invalid}")),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// === Helper mock types ===

// quotaExceededAuthStore wraps mockAuthStore but returns ErrQuotaExceeded on IncrementUsage.
type quotaExceededAuthStore struct {
	*mockAuthStore
}

func (m *quotaExceededAuthStore) IncrementUsage(_ context.Context, _, _ string) error {
	return auth.ErrQuotaExceeded
}

// quotaErrorAuthStore wraps mockAuthStore but returns generic error on IncrementUsage.
type quotaErrorAuthStore struct {
	*mockAuthStore
}

func (m *quotaErrorAuthStore) IncrementUsage(_ context.Context, _, _ string) error {
	return errors.New("database connection failed")
}

// userExistsStore wraps mockAuthStore but returns ErrUserExists on CreateUser.
type userExistsStore struct {
	*mockAuthStore
}

func (m *userExistsStore) CreateUser(_ context.Context, _ *auth.TenantUser) error {
	return auth.ErrUserExists
}

// testAdapterVersion is declared in batch_test.go.
