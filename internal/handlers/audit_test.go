package handlers

import (
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

// setupAuditTestRouter creates a test Gin router with the AuditHandler.
func setupAuditTestRouter(t *testing.T, store *mockAuthStore) (*gin.Engine, *AuditHandler) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()
	handler := NewAuditHandler(store, logger)

	// Middleware to set context from headers for tests.
	router.Use(func(c *gin.Context) {
		tenantID := c.GetHeader("X-Tenant-ID")
		userID := c.GetHeader("X-User-ID")
		isPlatformAdmin := c.GetHeader("X-Is-Platform-Admin") == "true"

		// Create user context with tenant ID.
		user := &auth.AuthenticatedUser{
			UserID:          userID,
			TenantID:        tenantID,
			IsPlatformAdmin: isPlatformAdmin,
		}
		ctx := auth.ContextWithUser(c.Request.Context(), user)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	})

	// Register routes.
	router.GET("/audit/events", handler.ListAuditEvents)
	router.GET("/audit/events/type/:eventType", handler.ListAuditEventsByType)
	router.GET("/audit/events/user/:userId", handler.ListAuditEventsByUser)

	return router, handler
}

// TestAuditHandler_ListAuditEvents tests listing audit events.
func TestAuditHandler_ListAuditEvents(t *testing.T) {
	tests := []struct {
		name            string
		tenantID        string
		queryParams     string
		isPlatformAdmin bool
		setupStore      func(*mockAuthStore)
		wantStatus      int
		validateBody    func(*testing.T, []byte)
	}{
		{
			name:            "list events for tenant",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.events = []*auth.AuditEvent{
					{
						ID:        "event-1",
						TenantID:  "tenant-1",
						Type:      auth.AuditEventAuthSuccess,
						Timestamp: time.Now().UTC(),
					},
					{
						ID:        "event-2",
						TenantID:  "tenant-1",
						Type:      auth.AuditEventUserCreated,
						Timestamp: time.Now().UTC(),
					},
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Events []*auth.AuditEvent `json:"events"`
					Limit  int                `json:"limit"`
					Offset int                `json:"offset"`
					Total  int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 2, response.Total)
				assert.Equal(t, 50, response.Limit)
				assert.Equal(t, 0, response.Offset)
			},
		},
		{
			name:            "list events with custom limit",
			tenantID:        "tenant-1",
			queryParams:     "?limit=10&offset=5",
			isPlatformAdmin: false,
			setupStore:      func(_ *mockAuthStore) {},
			wantStatus:      http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Limit  int `json:"limit"`
					Offset int `json:"offset"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 10, response.Limit)
				assert.Equal(t, 5, response.Offset)
			},
		},
		{
			name:            "list events with limit exceeding max",
			tenantID:        "tenant-1",
			queryParams:     "?limit=5000",
			isPlatformAdmin: false,
			setupStore:      func(_ *mockAuthStore) {},
			wantStatus:      http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Limit int `json:"limit"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 1000, response.Limit)
			},
		},
		{
			name:            "list events with invalid limit uses default",
			tenantID:        "tenant-1",
			queryParams:     "?limit=invalid",
			isPlatformAdmin: false,
			setupStore:      func(_ *mockAuthStore) {},
			wantStatus:      http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Limit int `json:"limit"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 50, response.Limit)
			},
		},
		{
			name:            "platform admin can filter by tenant",
			tenantID:        "tenant-1",
			queryParams:     "?tenantId=tenant-2",
			isPlatformAdmin: true,
			setupStore: func(s *mockAuthStore) {
				s.events = []*auth.AuditEvent{
					{
						ID:       "event-1",
						TenantID: "tenant-2",
						Type:     auth.AuditEventTenantCreated,
					},
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:            "list empty events",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore:      func(_ *mockAuthStore) {},
			wantStatus:      http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Events []*auth.AuditEvent `json:"events"`
					Total  int                `json:"total"`
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
			router, _ := setupAuditTestRouter(t, store)

			url := "/audit/events" + tt.queryParams
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

// TestAuditHandler_ListAuditEventsByType tests listing events by type.
func TestAuditHandler_ListAuditEventsByType(t *testing.T) {
	tests := []struct {
		name            string
		eventType       string
		tenantID        string
		isPlatformAdmin bool
		setupStore      func(*mockAuthStore)
		wantStatus      int
		validateBody    func(*testing.T, []byte)
	}{
		{
			name:            "list events by type",
			eventType:       "user.login",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.events = []*auth.AuditEvent{
					{
						ID:       "event-1",
						TenantID: "tenant-1",
						Type:     auth.AuditEventAuthSuccess,
					},
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Events    []*auth.AuditEvent `json:"events"`
					EventType string             `json:"eventType"`
					Total     int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "user.login", response.EventType)
			},
		},
		{
			name:            "empty event type returns not found",
			eventType:       "",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore:      func(_ *mockAuthStore) {},
			wantStatus:      http.StatusNotFound,
			validateBody:    nil, // 404 from router, no JSON response
		},
		{
			name:            "non-admin sees only own tenant events",
			eventType:       "user.login",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.events = []*auth.AuditEvent{
					{
						ID:       "event-1",
						TenantID: "tenant-1",
						Type:     auth.AuditEventAuthSuccess,
					},
					{
						ID:       "event-2",
						TenantID: "tenant-2",
						Type:     auth.AuditEventAuthSuccess,
					},
					{
						ID:       "event-3",
						TenantID: "", // Global event.
						Type:     auth.AuditEventAuthSuccess,
					},
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Events []*auth.AuditEvent `json:"events"`
					Total  int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				// Should see tenant-1 event and global event, not tenant-2.
				assert.Equal(t, 2, response.Total)
			},
		},
		{
			name:            "platform admin sees all events",
			eventType:       "user.login",
			tenantID:        "tenant-1",
			isPlatformAdmin: true,
			setupStore: func(s *mockAuthStore) {
				s.events = []*auth.AuditEvent{
					{
						ID:       "event-1",
						TenantID: "tenant-1",
						Type:     auth.AuditEventAuthSuccess,
					},
					{
						ID:       "event-2",
						TenantID: "tenant-2",
						Type:     auth.AuditEventAuthSuccess,
					},
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Events []*auth.AuditEvent `json:"events"`
					Total  int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, 2, response.Total)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockAuthStore()
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			router, _ := setupAuditTestRouter(t, store)

			url := "/audit/events/type/" + tt.eventType
			if tt.eventType == "" {
				url = "/audit/events/type/"
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

// TestAuditHandler_ListAuditEventsByUser tests listing events by user.
func TestAuditHandler_ListAuditEventsByUser(t *testing.T) {
	tests := []struct {
		name            string
		targetUserID    string
		currentUserID   string
		tenantID        string
		isPlatformAdmin bool
		setupStore      func(*mockAuthStore)
		wantStatus      int
		validateBody    func(*testing.T, []byte)
	}{
		{
			name:            "list own events",
			targetUserID:    "user-1",
			currentUserID:   "user-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.users["user-1"] = &auth.TenantUser{
					ID:       "user-1",
					TenantID: "tenant-1",
				}
				s.events = []*auth.AuditEvent{
					{
						ID:     "event-1",
						UserID: "user-1",
						Type:   auth.AuditEventAuthSuccess,
					},
				}
			},
			wantStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response struct {
					Events []*auth.AuditEvent `json:"events"`
					UserID string             `json:"userId"`
					Total  int                `json:"total"`
				}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "user-1", response.UserID)
			},
		},
		{
			name:            "list events for user in same tenant",
			targetUserID:    "user-2",
			currentUserID:   "user-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.users["user-1"] = &auth.TenantUser{
					ID:       "user-1",
					TenantID: "tenant-1",
				}
				s.users["user-2"] = &auth.TenantUser{
					ID:       "user-2",
					TenantID: "tenant-1",
				}
				s.events = []*auth.AuditEvent{
					{
						ID:     "event-1",
						UserID: "user-2",
						Type:   auth.AuditEventAuthSuccess,
					},
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:            "list events for user in different tenant denied",
			targetUserID:    "user-2",
			currentUserID:   "user-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.users["user-1"] = &auth.TenantUser{
					ID:       "user-1",
					TenantID: "tenant-1",
				}
				s.users["user-2"] = &auth.TenantUser{
					ID:       "user-2",
					TenantID: "tenant-2",
				}
			},
			wantStatus: http.StatusForbidden,
			validateBody: func(t *testing.T, body []byte) {
				t.Helper()
				var response models.ErrorResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Forbidden", response.Error)
			},
		},
		{
			name:            "platform admin can view any user events",
			targetUserID:    "user-2",
			currentUserID:   "admin-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: true,
			setupStore: func(s *mockAuthStore) {
				s.users["user-2"] = &auth.TenantUser{
					ID:       "user-2",
					TenantID: "tenant-2",
				}
				s.events = []*auth.AuditEvent{
					{
						ID:     "event-1",
						UserID: "user-2",
						Type:   auth.AuditEventAuthSuccess,
					},
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:            "empty user ID returns not found",
			targetUserID:    "",
			currentUserID:   "user-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore:      func(_ *mockAuthStore) {},
			wantStatus:      http.StatusNotFound,
			validateBody:    nil, // 404 from router, no JSON response
		},
		{
			name:            "non-existent target user denied",
			targetUserID:    "user-nonexistent",
			currentUserID:   "user-1",
			tenantID:        "tenant-1",
			isPlatformAdmin: false,
			setupStore: func(s *mockAuthStore) {
				s.users["user-1"] = &auth.TenantUser{
					ID:       "user-1",
					TenantID: "tenant-1",
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
			router, _ := setupAuditTestRouter(t, store)

			url := "/audit/events/user/" + tt.targetUserID
			if tt.targetUserID == "" {
				url = "/audit/events/user/"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-Tenant-ID", tt.tenantID)
			req.Header.Set("X-User-ID", tt.currentUserID)
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

// TestNewAuditHandler tests handler creation.
func TestNewAuditHandler(t *testing.T) {
	store := newMockAuthStore()
	logger := zap.NewNop()

	t.Run("valid creation", func(t *testing.T) {
		handler := NewAuditHandler(store, logger)
		assert.NotNil(t, handler)
	})

	t.Run("nil store panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewAuditHandler(nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewAuditHandler(store, nil)
		})
	})
}
