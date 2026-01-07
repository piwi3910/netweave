package handlers

import (
	"bytes"
	"context"
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

// setupUserTestRouter creates a test Gin router with the UserHandler.
func setupUserTestRouter(t *testing.T, store *mockAuthStore) (*gin.Engine, *UserHandler) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()
	handler := NewUserHandler(store, logger)

	// Middleware to set user and tenant context for tests.
	router.Use(func(c *gin.Context) {
		tenantID := c.GetHeader("X-Tenant-ID")
		userID := c.GetHeader("X-User-ID")
		if tenantID != "" {
			// Get tenant from store and add to context
			tenant := store.tenants[tenantID]
			if tenant != nil {
				ctx := auth.ContextWithTenant(c.Request.Context(), tenant)
				c.Request = c.Request.WithContext(ctx)
			}

			// Create user context with tenant ID
			user := &auth.AuthenticatedUser{
				UserID:   userID,
				TenantID: tenantID,
			}
			ctx := auth.ContextWithUser(c.Request.Context(), user)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})

	// Register routes.
	router.GET("/tenant/users", handler.ListUsers)
	router.POST("/tenant/users", handler.CreateUser)
	router.GET("/tenant/users/:userId", handler.GetUser)
	router.PUT("/tenant/users/:userId", handler.UpdateUser)
	router.DELETE("/tenant/users/:userId", handler.DeleteUser)
	router.GET("/user", handler.GetCurrentUser)

	return router, handler
}

// TestUserHandler_ListUsers tests listing users.
func TestUserHandler_ListUsers(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "list empty users",
			tenantID: "tenant-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response struct {
					Users []*auth.TenantUser `json:"users"`
					Total int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 0, response.Total)
			},
		},
		{
			name:     "list users in tenant",
			tenantID: "tenant-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-1"] = &auth.TenantUser{
					ID:       "user-1",
					TenantID: "tenant-123",
					Subject:  "CN=user1,O=test",
				}
				s.users["user-2"] = &auth.TenantUser{
					ID:       "user-2",
					TenantID: "tenant-123",
					Subject:  "CN=user2,O=test",
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response struct {
					Users []*auth.TenantUser `json:"users"`
					Total int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 2, response.Total)
			},
		},
		{
			name:       "list users without tenant context",
			tenantID:   "",
			setupStore: func(s *mockAuthStore) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupUserTestRouter(t, store)

			req := httptest.NewRequest(http.MethodGet, "/tenant/users", nil)
			req.Header.Set("Accept", "application/json")
			if tt.tenantID != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantID)
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

// TestUserHandler_CreateUser tests creating users.
func TestUserHandler_CreateUser(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		requestBody  interface{}
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "create valid user",
			tenantID: "tenant-123",
			requestBody: CreateUserRequest{
				Subject:    "CN=newuser,O=test",
				CommonName: "New User",
				Email:      "newuser@test.com",
				RoleID:     "role-viewer",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
					Quota: auth.TenantQuota{
						MaxUsers: 10,
					},
					Usage: auth.TenantUsage{
						Users: 0,
					},
				}
				s.roles["role-viewer"] = &auth.Role{
					ID:   "role-viewer",
					Name: auth.RoleViewer,
					Type: auth.RoleTypeTenant,
				}
			},
			wantStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.TenantUser
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.ID)
				assert.Equal(t, "CN=newuser,O=test", response.Subject)
				assert.Equal(t, "New User", response.CommonName)
			},
		},
		{
			name:     "create user with invalid role ID",
			tenantID: "tenant-123",
			requestBody: CreateUserRequest{
				Subject:    "CN=newuser,O=test",
				CommonName: "New User",
				RoleID:     "invalid-role",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
					Quota:  auth.TenantQuota{MaxUsers: 10},
				}
			},
			wantStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
			},
		},
		{
			name:     "create user with platform role",
			tenantID: "tenant-123",
			requestBody: CreateUserRequest{
				Subject:    "CN=newuser,O=test",
				CommonName: "New User",
				RoleID:     "role-platform-admin",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
					Quota:  auth.TenantQuota{MaxUsers: 10},
				}
				s.roles["role-platform-admin"] = &auth.Role{
					ID:   "role-platform-admin",
					Name: auth.RolePlatformAdmin,
					Type: auth.RoleTypePlatform,
				}
			},
			wantStatus: http.StatusBadRequest,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "BadRequest", response.Error)
				assert.Contains(t, response.Message, "platform-level roles")
			},
		},
		{
			name:     "create user quota exceeded",
			tenantID: "tenant-123",
			requestBody: CreateUserRequest{
				Subject:    "CN=newuser,O=test",
				CommonName: "New User",
				RoleID:     "role-viewer",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
					Quota:  auth.TenantQuota{MaxUsers: 2},
					Usage:  auth.TenantUsage{Users: 2},
				}
				s.roles["role-viewer"] = &auth.Role{
					ID:   "role-viewer",
					Name: auth.RoleViewer,
					Type: auth.RoleTypeTenant,
				}
			},
			wantStatus: http.StatusForbidden,
			validateBody: func(t *testing.T, body []byte) {
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "QuotaExceeded", response.Error)
			},
		},
		{
			name:        "create with missing required fields",
			tenantID:    "tenant-123",
			requestBody: CreateUserRequest{},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupUserTestRouter(t, store)

			var body []byte
			var err error
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/tenant/users", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			if tt.tenantID != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantID)
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

// TestUserHandler_GetUser tests retrieving a specific user.
func TestUserHandler_GetUser(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		userID       string
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "get existing user",
			tenantID: "tenant-123",
			userID:   "user-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-123"] = &auth.TenantUser{
					ID:         "user-123",
					TenantID:   "tenant-123",
					Subject:    "CN=testuser,O=test",
					CommonName: "Test User",
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.TenantUser
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "user-123", response.ID)
				assert.Equal(t, "Test User", response.CommonName)
			},
		},
		{
			name:     "get non-existent user",
			tenantID: "tenant-123",
			userID:   "user-nonexistent",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
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
			name:     "get user from different tenant",
			tenantID: "tenant-123",
			userID:   "user-456",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-456"] = &auth.TenantUser{
					ID:       "user-456",
					TenantID: "tenant-other",
					Subject:  "CN=otheruser,O=test",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupUserTestRouter(t, store)

			url := "/tenant/users/" + tt.userID
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", "application/json")
			if tt.tenantID != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantID)
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

// TestUserHandler_UpdateUser tests updating users.
func TestUserHandler_UpdateUser(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		userID       string
		requestBody  interface{}
		setupStore   func(*mockAuthStore)
		wantStatus   int
		validateBody func(*testing.T, []byte)
	}{
		{
			name:     "update user email",
			tenantID: "tenant-123",
			userID:   "user-123",
			requestBody: UpdateUserRequest{
				Email: "updated@test.com",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-123"] = &auth.TenantUser{
					ID:         "user-123",
					TenantID:   "tenant-123",
					Subject:    "CN=testuser,O=test",
					CommonName: "Test User",
					Email:      "old@test.com",
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.TenantUser
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "updated@test.com", response.Email)
			},
		},
		{
			name:     "deactivate user",
			tenantID: "tenant-123",
			userID:   "user-123",
			requestBody: func() UpdateUserRequest {
				isActive := false
				return UpdateUserRequest{IsActive: &isActive}
			}(),
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-123"] = &auth.TenantUser{
					ID:       "user-123",
					TenantID: "tenant-123",
					IsActive: true,
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response auth.TenantUser
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.False(t, response.IsActive)
			},
		},
		{
			name:     "update non-existent user",
			tenantID: "tenant-123",
			userID:   "user-nonexistent",
			requestBody: UpdateUserRequest{
				Email: "test@test.com",
			},
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupUserTestRouter(t, store)

			var body []byte
			var err error
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			url := "/tenant/users/" + tt.userID
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			if tt.tenantID != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantID)
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

// TestUserHandler_DeleteUser tests deleting users.
func TestUserHandler_DeleteUser(t *testing.T) {
	tests := []struct {
		name         string
		tenantID     string
		userID       string
		setupStore   func(*mockAuthStore)
		setupContext func(context.Context) context.Context
		wantStatus   int
	}{
		{
			name:     "delete existing user",
			tenantID: "tenant-123",
			userID:   "user-123",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-123"] = &auth.TenantUser{
					ID:       "user-123",
					TenantID: "tenant-123",
					Subject:  "CN=testuser,O=test",
				}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:     "delete non-existent user",
			tenantID: "tenant-123",
			userID:   "user-nonexistent",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:     "delete user from different tenant",
			tenantID: "tenant-123",
			userID:   "user-456",
			setupStore: func(s *mockAuthStore) {
				s.tenants["tenant-123"] = &auth.Tenant{
					ID:     "tenant-123",
					Name:   "Test Tenant",
					Status: auth.TenantStatusActive,
				}
				s.users["user-456"] = &auth.TenantUser{
					ID:       "user-456",
					TenantID: "tenant-other",
					Subject:  "CN=otheruser,O=test",
				}
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupUserTestRouter(t, store)

			url := "/tenant/users/" + tt.userID
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req.Header.Set("Accept", "application/json")
			if tt.tenantID != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantID)
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
