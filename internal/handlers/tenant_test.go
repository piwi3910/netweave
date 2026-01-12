package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

// mockAuthStore is a mock implementation of auth.Store for testing.
type mockAuthStore struct {
	tenants map[string]*auth.Tenant
	users   map[string]*auth.TenantUser
	roles   map[string]*auth.Role
	events  []*auth.AuditEvent
}

func newMockAuthStore() *mockAuthStore {
	return &mockAuthStore{
		tenants: make(map[string]*auth.Tenant),
		users:   make(map[string]*auth.TenantUser),
		roles:   make(map[string]*auth.Role),
		events:  make([]*auth.AuditEvent, 0),
	}
}

func (m *mockAuthStore) CreateTenant(_ context.Context, tenant *auth.Tenant) error {
	if _, exists := m.tenants[tenant.ID]; exists {
		return auth.ErrTenantExists
	}
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockAuthStore) GetTenant(_ context.Context, id string) (*auth.Tenant, error) {
	tenant, exists := m.tenants[id]
	if !exists {
		return nil, auth.ErrTenantNotFound
	}
	return tenant, nil
}

func (m *mockAuthStore) UpdateTenant(_ context.Context, tenant *auth.Tenant) error {
	if _, exists := m.tenants[tenant.ID]; !exists {
		return auth.ErrTenantNotFound
	}
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockAuthStore) DeleteTenant(_ context.Context, id string) error {
	if _, exists := m.tenants[id]; !exists {
		return auth.ErrTenantNotFound
	}
	delete(m.tenants, id)
	return nil
}

func (m *mockAuthStore) ListTenants(_ context.Context) ([]*auth.Tenant, error) {
	result := make([]*auth.Tenant, 0, len(m.tenants))
	for _, tenant := range m.tenants {
		result = append(result, tenant)
	}
	return result, nil
}

func (m *mockAuthStore) IncrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockAuthStore) DecrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockAuthStore) CreateUser(_ context.Context, user *auth.TenantUser) error {
	if _, exists := m.users[user.ID]; exists {
		return auth.ErrUserExists
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockAuthStore) GetUser(_ context.Context, id string) (*auth.TenantUser, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (m *mockAuthStore) GetUserBySubject(_ context.Context, subject string) (*auth.TenantUser, error) {
	for _, user := range m.users {
		if user.Subject == subject {
			return user, nil
		}
	}
	return nil, auth.ErrUserNotFound
}

func (m *mockAuthStore) UpdateUser(_ context.Context, user *auth.TenantUser) error {
	if _, exists := m.users[user.ID]; !exists {
		return auth.ErrUserNotFound
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockAuthStore) DeleteUser(_ context.Context, id string) error {
	if _, exists := m.users[id]; !exists {
		return auth.ErrUserNotFound
	}
	delete(m.users, id)
	return nil
}

func (m *mockAuthStore) ListUsersByTenant(_ context.Context, tenantID string) ([]*auth.TenantUser, error) {
	result := make([]*auth.TenantUser, 0)
	for _, user := range m.users {
		if user.TenantID == tenantID {
			result = append(result, user)
		}
	}
	return result, nil
}

func (m *mockAuthStore) UpdateLastLogin(_ context.Context, _ string) error {
	return nil
}

func (m *mockAuthStore) CreateRole(_ context.Context, role *auth.Role) error {
	if _, exists := m.roles[role.ID]; exists {
		return auth.ErrRoleExists
	}
	m.roles[role.ID] = role
	return nil
}

func (m *mockAuthStore) GetRole(_ context.Context, id string) (*auth.Role, error) {
	role, exists := m.roles[id]
	if !exists {
		return nil, auth.ErrRoleNotFound
	}
	return role, nil
}

func (m *mockAuthStore) GetRoleByName(_ context.Context, name auth.RoleName) (*auth.Role, error) {
	for _, role := range m.roles {
		if role.Name == name {
			return role, nil
		}
	}
	return nil, auth.ErrRoleNotFound
}

func (m *mockAuthStore) UpdateRole(_ context.Context, role *auth.Role) error {
	if _, exists := m.roles[role.ID]; !exists {
		return auth.ErrRoleNotFound
	}
	m.roles[role.ID] = role
	return nil
}

func (m *mockAuthStore) DeleteRole(_ context.Context, id string) error {
	if _, exists := m.roles[id]; !exists {
		return auth.ErrRoleNotFound
	}
	delete(m.roles, id)
	return nil
}

func (m *mockAuthStore) ListRoles(_ context.Context) ([]*auth.Role, error) {
	result := make([]*auth.Role, 0, len(m.roles))
	for _, role := range m.roles {
		result = append(result, role)
	}
	return result, nil
}

func (m *mockAuthStore) ListRolesByTenant(ctx context.Context, _ string) ([]*auth.Role, error) {
	return m.ListRoles(ctx)
}

func (m *mockAuthStore) InitializeDefaultRoles(_ context.Context) error {
	return nil
}

func (m *mockAuthStore) LogEvent(_ context.Context, event *auth.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuthStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*auth.AuditEvent, error) {
	return m.events, nil
}

func (m *mockAuthStore) ListEventsByType(_ context.Context, _ auth.AuditEventType, _ int) ([]*auth.AuditEvent, error) {
	return m.events, nil
}

func (m *mockAuthStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*auth.AuditEvent, error) {
	return m.events, nil
}

func (m *mockAuthStore) Ping(_ context.Context) error {
	return nil
}

func (m *mockAuthStore) Close() error {
	return nil
}

// setupTenantTestRouter creates a test Gin router with the TenantHandler.
func setupTenantTestRouter(t *testing.T, store *mockAuthStore) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()
	handler := NewTenantHandler(store, logger)

	// Register routes.
	router.GET("/admin/tenants", handler.ListTenants)
	router.POST("/admin/tenants", handler.CreateTenant)
	router.GET("/admin/tenants/:tenantId", handler.GetTenant)
	router.PUT("/admin/tenants/:tenantId", handler.UpdateTenant)
	router.DELETE("/admin/tenants/:tenantId", handler.DeleteTenant)
	router.GET("/tenant", handler.GetCurrentTenant)

	return router
}

// TestTenantHandler_ListTenants tests listing tenants.
func TestTenantHandler_ListTenants(t *testing.T) {
	tests := []struct {
		name         string
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name: "list empty tenants",
			setupStore: func(_ *mockAuthStore) {
				// No tenants added.
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Tenants []*auth.Tenant `json:"tenants"`
					Total   int            `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 0, response.Total)
				assert.Empty(t, response.Tenants)
			},
		},
		{
			name: "list multiple tenants",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-1"] = &auth.Tenant{
					ID:     "tenant-1",
					Name:   "Tenant 1",
					Status: auth.TenantStatusActive,
				}
				s.tenants["tenant-2"] = &auth.Tenant{
					ID:     "tenant-2",
					Name:   "Tenant 2",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Tenants []*auth.Tenant `json:"tenants"`
					Total   int            `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 2, response.Total)
				assert.Len(t, response.Tenants, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router := setupTenantTestRouter(t, store)

			req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestTenantHandler_CreateTenant tests creating tenants.
func TestTenantHandler_CreateTenant(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  interface{}
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name: "create valid tenant",
			requestBody: CreateTenantRequest{
				Name:         "Test Tenant",
				Description:  "A test tenant",
				ContactEmail: "admin@test.com",
			},
			wantStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response auth.Tenant
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.ID)
				assert.Equal(t, "Test Tenant", response.Name)
				assert.Equal(t, "A test tenant", response.Description)
				assert.Equal(t, auth.TenantStatusActive, response.Status)
			},
		},
		{
			name: "create tenant with custom quota",
			requestBody: CreateTenantRequest{
				Name: "Quota Tenant",
				Quota: &auth.TenantQuota{
					MaxSubscriptions: 100,
					MaxResourcePools: 50,
					MaxDeployments:   200,
					MaxUsers:         20,
				},
			},
			wantStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response auth.Tenant
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 100, response.Quota.MaxSubscriptions)
				assert.Equal(t, 50, response.Quota.MaxResourcePools)
			},
		},
		{
			name:        "create with invalid JSON",
			requestBody: `{invalid json}`,
			wantStatus:  http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
			},
		},
		{
			name:        "create with missing required field",
			requestBody: CreateTenantRequest{},
			wantStatus:  http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			router := setupTenantTestRouter(t, store)

			var body []byte
			var err error
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestTenantHandler_GetTenant tests retrieving a specific tenant.
func TestTenantHandler_GetTenant(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "get existing tenant",
			tenantID: "tenant-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:          "tenant-123",
					Name:        "Test Tenant",
					Description: "A test tenant",
					Status:      auth.TenantStatusActive,
					CreatedAt:   time.Now().UTC(),
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response auth.Tenant
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "tenant-123", response.ID)
				assert.Equal(t, "Test Tenant", response.Name)
			},
		},
		{
			name:     "get non-existent tenant",
			tenantID: "tenant-nonexistent",
			setupStore: func(_ *mockAuthStore) {
				// No tenant added.
			},
			wantStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
			},
		},
		{
			name:       "get with empty tenant ID returns redirect",
			tenantID:   "",
			setupStore: func(_ *mockAuthStore) {},
			wantStatus: http.StatusMovedPermanently, // 301 redirect from router
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router := setupTenantTestRouter(t, store)

			url := "/admin/tenants/" + tt.tenantID
			if tt.tenantID == "" {
				url = "/admin/tenants/"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestTenantHandler_UpdateTenant tests updating tenants.
func TestTenantHandler_UpdateTenant(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		requestBody  interface{}
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "update existing tenant",
			tenantID: "tenant-123",
			requestBody: UpdateTenantRequest{
				Name:        "Updated Tenant",
				Description: "Updated description",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Original Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response auth.Tenant
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Updated Tenant", response.Name)
				assert.Equal(t, "Updated description", response.Description)
			},
		},
		{
			name:     "update non-existent tenant",
			tenantID: "tenant-nonexistent",
			requestBody: UpdateTenantRequest{
				Name: "Updated Tenant",
			},
			setupStore: func(_ *mockAuthStore) {},
			wantStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
			},
		},
		{
			name:        "update with invalid JSON",
			tenantID:    "tenant-123",
			requestBody: `{invalid json}`,
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Original Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "update tenant status",
			tenantID: "tenant-123",
			requestBody: UpdateTenantRequest{
				Status: auth.TenantStatusSuspended,
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Original Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response auth.Tenant
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, auth.TenantStatusSuspended, response.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router := setupTenantTestRouter(t, store)

			var body []byte
			var err error
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			url := "/admin/tenants/" + tt.tenantID
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestTenantHandler_DeleteTenant tests deleting tenants.
func TestTenantHandler_DeleteTenant(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "delete tenant with no resources",
			tenantID: "tenant-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Empty Tenant",
					Status: auth.TenantStatusActive,
					Usage:  auth.TenantUsage{},
				}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:     "delete tenant with active resources",
			tenantID: "tenant-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Active Tenant",
					Status: auth.TenantStatusActive,
					Usage: auth.TenantUsage{
						Subscriptions: 5,
						Users:         3,
					},
				}
			},
			wantStatus: http.StatusAccepted,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, string(auth.TenantStatusPendingDeletion), response["status"])
			},
		},
		{
			name:       "delete non-existent tenant",
			tenantID:   "tenant-nonexistent",
			setupStore: func(_ *mockAuthStore) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router := setupTenantTestRouter(t, store)

			url := "/admin/tenants/" + tt.tenantID
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestTenantHandler_GetCurrentTenant tests getting current tenant.
func TestTenantHandler_GetCurrentTenant(t *testing.T) {
	tests := []struct {
		name         string
		setupContext func(*gin.Context)
		wantStatus   int
	}{
		{
			name: "get current tenant without context",
			setupContext: func(_ *gin.Context) {
				// No tenant in context.
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			router := setupTenantTestRouter(t, store)

			req := httptest.NewRequest(http.MethodGet, "/tenant", nil)
			req.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
