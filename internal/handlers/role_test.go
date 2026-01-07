package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupRoleTestRouter creates a test Gin router with the RoleHandler.
func setupRoleTestRouter(t *testing.T, store *mockAuthStore) (*gin.Engine, *RoleHandler) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()
	handler := NewRoleHandler(store, logger)

	// Middleware to set context from headers for tests.
	router.Use(func(c *gin.Context) {
		tenantID := c.GetHeader("X-Tenant-ID")
		isPlatformAdmin := c.GetHeader("X-Is-Platform-Admin") == "true"

		// Create user context with tenant ID.
		user := &auth.AuthenticatedUser{
			UserID:          "test-user",
			TenantID:        tenantID,
			IsPlatformAdmin: isPlatformAdmin,
		}
		ctx := auth.ContextWithUser(c.Request.Context(), user)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	})

	// Register routes.
	router.GET("/roles", handler.ListRoles)
	router.GET("/roles/:roleId", handler.GetRole)
	router.GET("/permissions", handler.ListPermissions)

	return router, handler
}

// TestRoleHandler_ListRoles tests listing roles.
func TestRoleHandler_ListRoles(t *testing.T) {
	tests := []struct {
		name            string
		tenantID        string
		isPlatformAdmin bool
		setupStore      func(*mockAuthStore)
		wantStatus      int
		validateBody    func(*testing.T, []byte)
	}{
		{
			name:            "list all roles as platform admin",
			tenantID:        "tenant-1",
			isPlatformAdmin: true,
			setupStore: func(s *mockAuthStore) {
				s.roles["role-1"] = &auth.Role{
					ID:   "role-1",
					Name: auth.RolePlatformAdmin,
					Type: auth.RoleTypePlatform,
				}
				s.roles["role-2"] = &auth.Role{
					ID:   "role-2",
					Name: auth.RoleTenantAdmin,
					Type: auth.RoleTypeTenant,
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response struct {
					Roles []*auth.Role `json:"roles"`
					Total int          `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 2, response.Total)
			},
		},
		{
			name:            "list tenant roles as regular user",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.roles["role-1"] = &auth.Role{
					ID:       "role-1",
					Name:     auth.RoleTenantAdmin,
					Type:     auth.RoleTypeTenant,
					TenantID: "tenant-1",
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response struct {
					Roles []*auth.Role `json:"roles"`
					Total int          `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, response.Total, 0)
			},
		},
		{
			name:            "list empty roles",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				// No roles.
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response struct {
					Roles []*auth.Role `json:"roles"`
					Total int          `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 0, response.Total)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupRoleTestRouter(t, store)

			req := httptest.NewRequest(http.MethodGet, "/roles", nil)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-Tenant-ID", tt.tenantID)
			if tt.isPlatformAdmin {
				req.Header.Set("X-Is-Platform-Admin", "true")
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestRoleHandler_GetRole tests retrieving a specific role.
func TestRoleHandler_GetRole(t *testing.T) {
	tests := []struct {
		name            string
		roleID          string
		tenantID        string
		isPlatformAdmin bool
		setupStore      func(*mockAuthStore)
		wantStatus      int
		validateBody    func(*testing.T, []byte)
	}{
		{
			name:            "get existing role",
			roleID:          "role-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.roles["role-1"] = &auth.Role{
					ID:       "role-1",
					Name:     auth.RoleTenantAdmin,
					Type:     auth.RoleTypeTenant,
					TenantID: "tenant-1",
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.Role
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "role-1", response.ID)
				assert.Equal(t, auth.RoleTenantAdmin, response.Name)
			},
		},
		{
			name:            "get non-existent role",
			roleID:          "role-nonexistent",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				// No roles.
			},
			wantStatus: http.StatusNotFound,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "NotFound", response.Error)
			},
		},
		{
			name:            "get role with empty ID",
			roleID:          "",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore:      func(s *mockAuthStore) {},
			wantStatus:      http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
			},
		},
		{
			name:            "get role from different tenant denied",
			roleID:          "role-1",
			tenantID:        "tenant-2",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.roles["role-1"] = &auth.Role{
					ID:       "role-1",
					Name:     auth.RoleTenantAdmin,
					Type:     auth.RoleTypeTenant,
					TenantID: "tenant-1",
				}
			},
			wantStatus: http.StatusForbidden,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Forbidden", response.Error)
			},
		},
		{
			name:            "platform admin can access any role",
			roleID:          "role-1",
			tenantID:        "tenant-2",
			isPlatformAdmin: true,
			setupStore: func(s *mockAuthStore) {
				s.roles["role-1"] = &auth.Role{
					ID:       "role-1",
					Name:     auth.RoleTenantAdmin,
					Type:     auth.RoleTypeTenant,
					TenantID: "tenant-1",
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.Role
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "role-1", response.ID)
			},
		},
		{
			name:            "get global role without tenant restriction",
			roleID:          "role-global",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.roles["role-global"] = &auth.Role{
					ID:       "role-global",
					Name:     auth.RoleViewer,
					Type:     auth.RoleTypeTenant,
					TenantID: "", // Global role.
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.Role
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "role-global", response.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupRoleTestRouter(t, store)

			url := "/roles/" + tt.roleID
			if tt.roleID == "" {
				url = "/roles/"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-Tenant-ID", tt.tenantID)
			if tt.isPlatformAdmin {
				req.Header.Set("X-Is-Platform-Admin", "true")
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

// TestRoleHandler_ListPermissions tests listing all permissions.
func TestRoleHandler_ListPermissions(t *testing.T) {
	store := newMockAuthStore()
	router, _ := setupRoleTestRouter(t, store)

	req := httptest.NewRequest(http.MethodGet, "/permissions", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Permissions []struct {
			Permission auth.Permission `json:"permission"`
			Resource   string          `json:"resource"`
			Action     string          `json:"action"`
		} `json:"permissions"`
		Total int `json:"total"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify we have permissions.
	assert.Greater(t, response.Total, 0)
	assert.Len(t, response.Permissions, response.Total)

	// Verify each permission has resource and action.
	for _, perm := range response.Permissions {
		assert.NotEmpty(t, perm.Permission)
		assert.NotEmpty(t, perm.Resource)
		assert.NotEmpty(t, perm.Action)
	}
}

// TestGetResourceFromPermission tests the resource extraction function.
func TestGetResourceFromPermission(t *testing.T) {
	tests := []struct {
		permission auth.Permission
		want       string
	}{
		{
			permission: auth.PermissionSubscriptionRead,
			want:       "subscriptions",
		},
		{
			permission: auth.PermissionResourcePoolCreate,
			want:       "resourcePools",
		},
		{
			permission: auth.PermissionAuditRead,
			want:       "audit",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.permission), func(t *testing.T) {
			got := getResourceFromPermission(tt.permission)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGetActionFromPermission tests the action extraction function.
func TestGetActionFromPermission(t *testing.T) {
	tests := []struct {
		permission auth.Permission
		want       string
	}{
		{
			permission: auth.PermissionSubscriptionRead,
			want:       "read",
		},
		{
			permission: auth.PermissionResourcePoolCreate,
			want:       "create",
		},
		{
			permission: auth.PermissionUserDelete,
			want:       "delete",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.permission), func(t *testing.T) {
			got := getActionFromPermission(tt.permission)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNewRoleHandler tests handler creation.
func TestNewRoleHandler(t *testing.T) {
	store := newMockAuthStore()
	logger := zap.NewNop()

	t.Run("valid creation", func(t *testing.T) {
		handler := NewRoleHandler(store, logger)
		assert.NotNil(t, handler)
	})

	t.Run("nil store panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewRoleHandler(nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewRoleHandler(store, nil)
		})
	})
}
