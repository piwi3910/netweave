package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockKeycloakClient mocks the Keycloak client for testing.
type mockKeycloakClient struct {
	verifyTokenFunc func(ctx context.Context, token string) (map[string]interface{}, error)
}

func (m *mockKeycloakClient) VerifyToken(ctx context.Context, token string) (map[string]interface{}, error) {
	if m.verifyTokenFunc != nil {
		return m.verifyTokenFunc(ctx, token)
	}
	return nil, errors.New("not implemented")
}

// mockStore implements Store interface for testing.
type mockStore struct {
	users   map[string]*TenantUser
	roles   map[string]*Role
	tenants map[string]*Tenant
	events  []*AuditEvent
}

func newMockStore() *mockStore {
	return &mockStore{
		users:   make(map[string]*TenantUser),
		roles:   make(map[string]*Role),
		tenants: make(map[string]*Tenant),
		events:  make([]*AuditEvent, 0),
	}
}

func (m *mockStore) CreateTenant(_ context.Context, tenant *Tenant) error {
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockStore) GetTenant(_ context.Context, id string) (*Tenant, error) {
	tenant, ok := m.tenants[id]
	if !ok {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

func (m *mockStore) UpdateTenant(_ context.Context, tenant *Tenant) error {
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *mockStore) DeleteTenant(_ context.Context, id string) error {
	delete(m.tenants, id)
	return nil
}

func (m *mockStore) ListTenants(_ context.Context) ([]*Tenant, error) {
	result := make([]*Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		result = append(result, t)
	}
	return result, nil
}

func (m *mockStore) IncrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) DecrementUsage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) CreateUser(_ context.Context, user *TenantUser) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockStore) GetUser(_ context.Context, id string) (*TenantUser, error) {
	user, ok := m.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (m *mockStore) GetUserBySubject(_ context.Context, subject string) (*TenantUser, error) {
	for _, user := range m.users {
		if user.Subject == subject {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockStore) GetUserByOAuthSubject(_ context.Context, oauthSubject string) (*TenantUser, error) {
	for _, user := range m.users {
		if user.OAuthSubject == oauthSubject {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockStore) GetUserByEmail(_ context.Context, email string) (*TenantUser, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockStore) UpdateUser(_ context.Context, user *TenantUser) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockStore) DeleteUser(_ context.Context, id string) error {
	delete(m.users, id)
	return nil
}

func (m *mockStore) ListUsersByTenant(_ context.Context, tenantID string) ([]*TenantUser, error) {
	result := make([]*TenantUser, 0)
	for _, user := range m.users {
		if user.TenantID == tenantID {
			result = append(result, user)
		}
	}
	return result, nil
}

func (m *mockStore) UpdateLastLogin(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) CreateRole(_ context.Context, role *Role) error {
	m.roles[role.ID] = role
	return nil
}

func (m *mockStore) GetRole(_ context.Context, id string) (*Role, error) {
	role, ok := m.roles[id]
	if !ok {
		return nil, ErrRoleNotFound
	}
	return role, nil
}

func (m *mockStore) GetRoleByName(_ context.Context, name RoleName) (*Role, error) {
	for _, role := range m.roles {
		if role.Name == name {
			return role, nil
		}
	}
	return nil, ErrRoleNotFound
}

func (m *mockStore) UpdateRole(_ context.Context, role *Role) error {
	m.roles[role.ID] = role
	return nil
}

func (m *mockStore) DeleteRole(_ context.Context, id string) error {
	delete(m.roles, id)
	return nil
}

func (m *mockStore) ListRoles(_ context.Context) ([]*Role, error) {
	result := make([]*Role, 0, len(m.roles))
	for _, r := range m.roles {
		result = append(result, r)
	}
	return result, nil
}

func (m *mockStore) ListRolesByTenant(_ context.Context, _ string) ([]*Role, error) {
	result := make([]*Role, 0, len(m.roles))
	for _, r := range m.roles {
		result = append(result, r)
	}
	return result, nil
}

func (m *mockStore) InitializeDefaultRoles(_ context.Context) error {
	return nil
}

func (m *mockStore) RecordAuditEvent(_ context.Context, event *AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockStore) RecordEvent(_ context.Context, event *AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockStore) LogEvent(_ context.Context, event *AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockStore) ListEvents(_ context.Context, _ string, _, _ int) ([]*AuditEvent, error) {
	return m.events, nil
}

func (m *mockStore) ListEventsByType(_ context.Context, _ AuditEventType, _ int) ([]*AuditEvent, error) {
	return m.events, nil
}

func (m *mockStore) ListEventsByUser(_ context.Context, _ string, _ int) ([]*AuditEvent, error) {
	return m.events, nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

// TestOAuth2Authenticator_ExtractBearerToken tests bearer token extraction.
func TestOAuth2Authenticator_ExtractBearerToken(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		wantToken   string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.token",
			wantToken:  "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.token",
			wantErr:    false,
		},
		{
			name:        "missing authorization header",
			authHeader:  "",
			wantErr:     true,
			errContains: "missing Authorization header",
		},
		{
			name:        "invalid format - no bearer prefix",
			authHeader:  "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.token",
			wantErr:     true,
			errContains: "invalid Authorization header format",
		},
		{
			name:        "invalid format - wrong scheme",
			authHeader:  "Basic dXNlcjpwYXNzd29yZA==",
			wantErr:     true,
			errContains: "invalid Authorization header format",
		},
		{
			name:       "bearer with different case",
			authHeader: "bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.token",
			wantToken:  "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.token",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.authHeader != "" {
				c.Request.Header.Set("Authorization", tt.authHeader)
			}

			auth := &OAuth2Authenticator{
				logger: zap.NewNop(),
			}

			token, err := auth.extractBearerToken(c)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
			}
		})
	}
}

// TestOAuth2Authenticator_ExtractClaims tests claims extraction from token.
func TestOAuth2Authenticator_ExtractClaims(t *testing.T) {
	tests := []struct {
		name        string
		tokenClaims map[string]interface{}
		want        *OAuth2Claims
		wantErr     bool
		errContains string
	}{
		{
			name: "complete valid claims",
			tokenClaims: map[string]interface{}{
				"sub":                "user-123",
				"email":              "user@example.com",
				"preferred_username": "john.doe",
				"name":               "John Doe",
				"groups":             []interface{}{"tenant-admin", "tenant-viewer"},
				"tenant_id":          "tenant-1",
			},
			want: &OAuth2Claims{
				Subject:           "user-123",
				Email:             "user@example.com",
				PreferredUsername: "john.doe",
				Name:              "John Doe",
				Groups:            []string{"tenant-admin", "tenant-viewer"},
				TenantID:          "tenant-1",
			},
			wantErr: false,
		},
		{
			name: "minimal valid claims - only sub",
			tokenClaims: map[string]interface{}{
				"sub": "user-123",
			},
			want: &OAuth2Claims{
				Subject: "user-123",
				Groups:  nil,
			},
			wantErr: false,
		},
		{
			name:        "missing sub claim",
			tokenClaims: map[string]interface{}{"email": "user@example.com"},
			wantErr:     true,
			errContains: "missing required 'sub' claim",
		},
		{
			name: "groups with mixed types - only strings extracted",
			tokenClaims: map[string]interface{}{
				"sub":    "user-123",
				"groups": []interface{}{"admin", 123, "viewer"},
			},
			want: &OAuth2Claims{
				Subject: "user-123",
				Groups:  []string{"admin", "viewer"},
			},
			wantErr: false,
		},
		{
			name: "empty groups array",
			tokenClaims: map[string]interface{}{
				"sub":    "user-123",
				"groups": []interface{}{},
			},
			want: &OAuth2Claims{
				Subject: "user-123",
				Groups:  nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &OAuth2Authenticator{
				logger: zap.NewNop(),
			}

			got, err := auth.extractClaims(tt.tokenClaims)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestOAuth2Authenticator_ValidateClaims tests claims validation.
func TestOAuth2Authenticator_ValidateClaims(t *testing.T) {
	tests := []struct {
		name        string
		config      *OAuth2Config
		claims      *OAuth2Claims
		wantErr     bool
		errContains string
	}{
		{
			name: "valid claims - tenant not required",
			config: &OAuth2Config{
				RequireTenantClaim: false,
			},
			claims: &OAuth2Claims{
				Subject: "user-123",
			},
			wantErr: false,
		},
		{
			name: "valid claims - tenant required and present",
			config: &OAuth2Config{
				RequireTenantClaim: true,
			},
			claims: &OAuth2Claims{
				Subject:  "user-123",
				TenantID: "tenant-1",
			},
			wantErr: false,
		},
		{
			name: "missing tenant claim when required",
			config: &OAuth2Config{
				RequireTenantClaim: true,
			},
			claims: &OAuth2Claims{
				Subject: "user-123",
			},
			wantErr:     true,
			errContains: "missing required 'tenant_id' claim",
		},
		{
			name:   "missing subject",
			config: &OAuth2Config{},
			claims: &OAuth2Claims{
				TenantID: "tenant-1",
			},
			wantErr:     true,
			errContains: "missing required 'sub' claim",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &OAuth2Authenticator{
				config: tt.config,
				logger: zap.NewNop(),
			}

			err := auth.validateClaims(tt.claims)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestOAuth2Authenticator_DetermineRole tests role determination from groups.
func TestOAuth2Authenticator_DetermineRole(t *testing.T) {
	// Setup mock store
	store := newMockStore()
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.roles["role-viewer"] = &Role{ID: "role-viewer", Name: "tenant-viewer"}
	store.roles["role-default"] = &Role{ID: "role-default", Name: "tenant-viewer"}

	tests := []struct {
		name        string
		config      *OAuth2Config
		claims      *OAuth2Claims
		wantRoleID  string
		wantErr     bool
		errContains string
	}{
		{
			name: "group mapping match - first group",
			config: &OAuth2Config{
				GroupRoleMapping: map[string]string{
					"platform-admins": "role-admin",
					"tenant-viewers":  "role-viewer",
				},
			},
			claims: &OAuth2Claims{
				Groups: []string{"platform-admins", "other-group"},
			},
			wantRoleID: "role-admin",
			wantErr:    false,
		},
		{
			name: "group mapping match - second group",
			config: &OAuth2Config{
				GroupRoleMapping: map[string]string{
					"platform-admins": "role-admin",
					"tenant-viewers":  "role-viewer",
				},
			},
			claims: &OAuth2Claims{
				Groups: []string{"other-group", "tenant-viewers"},
			},
			wantRoleID: "role-viewer",
			wantErr:    false,
		},
		{
			name: "no group match - use default role",
			config: &OAuth2Config{
				GroupRoleMapping: map[string]string{
					"platform-admins": "role-admin",
				},
				DefaultRole: "role-default",
			},
			claims: &OAuth2Claims{
				Groups: []string{"other-group"},
			},
			wantRoleID: "role-default",
			wantErr:    false,
		},
		{
			name: "no groups - use default role",
			config: &OAuth2Config{
				DefaultRole: "role-default",
			},
			claims: &OAuth2Claims{
				Groups: []string{},
			},
			wantRoleID: "role-default",
			wantErr:    false,
		},
		{
			name: "no group match and no default role",
			config: &OAuth2Config{
				GroupRoleMapping: map[string]string{
					"platform-admins": "role-admin",
				},
			},
			claims: &OAuth2Claims{
				Groups: []string{"other-group"},
			},
			wantErr:     true,
			errContains: "no valid role found",
		},
		{
			name: "group mapping to non-existent role - falls back to default",
			config: &OAuth2Config{
				GroupRoleMapping: map[string]string{
					"bad-group": "non-existent-role",
				},
				DefaultRole: "role-default",
			},
			claims: &OAuth2Claims{
				Groups: []string{"bad-group"},
			},
			wantRoleID: "role-default",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &OAuth2Authenticator{
				store:  store,
				config: tt.config,
				logger: zap.NewNop(),
			}

			roleID, err := auth.determineRole(context.Background(), tt.claims, "tenant-1")

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRoleID, roleID)
			}
		})
	}
}

// TestOAuth2Authenticator_BuildUserFromClaims tests user construction from claims.
func TestOAuth2Authenticator_BuildUserFromClaims(t *testing.T) {
	tests := []struct {
		name             string
		claims           *OAuth2Claims
		tenantID         string
		roleID           string
		wantCommonName   string
		wantEmail        string
		wantOAuthSubject string
	}{
		{
			name: "preferred username as common name",
			claims: &OAuth2Claims{
				Subject:           "keycloak-user-123",
				Email:             "user@example.com",
				PreferredUsername: "john.doe",
				Name:              "John Doe",
			},
			tenantID:         "tenant-1",
			roleID:           "role-1",
			wantCommonName:   "john.doe",
			wantEmail:        "user@example.com",
			wantOAuthSubject: "keycloak-user-123",
		},
		{
			name: "email as fallback common name",
			claims: &OAuth2Claims{
				Subject: "keycloak-user-123",
				Email:   "user@example.com",
				Name:    "John Doe",
			},
			tenantID:         "tenant-1",
			roleID:           "role-1",
			wantCommonName:   "user@example.com",
			wantEmail:        "user@example.com",
			wantOAuthSubject: "keycloak-user-123",
		},
		{
			name: "name as last fallback common name",
			claims: &OAuth2Claims{
				Subject: "keycloak-user-123",
				Name:    "John Doe",
			},
			tenantID:         "tenant-1",
			roleID:           "role-1",
			wantCommonName:   "John Doe",
			wantEmail:        "",
			wantOAuthSubject: "keycloak-user-123",
		},
		{
			name: "no common name available",
			claims: &OAuth2Claims{
				Subject: "keycloak-user-123",
			},
			tenantID:         "tenant-1",
			roleID:           "role-1",
			wantCommonName:   "",
			wantEmail:        "",
			wantOAuthSubject: "keycloak-user-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &OAuth2Authenticator{
				logger: zap.NewNop(),
			}

			user := auth.buildUserFromClaims(tt.claims, tt.tenantID, tt.roleID)

			assert.NotEmpty(t, user.ID)
			assert.Equal(t, tt.tenantID, user.TenantID)
			assert.Equal(t, tt.roleID, user.RoleID)
			assert.Equal(t, tt.wantCommonName, user.CommonName)
			assert.Equal(t, tt.wantEmail, user.Email)
			assert.Equal(t, tt.wantOAuthSubject, user.OAuthSubject)
			assert.Equal(t, "keycloak", user.OAuthProvider)
			assert.Empty(t, user.Subject) // No certificate DN for OAuth users
			assert.True(t, user.IsActive)
			assert.False(t, user.CreatedAt.IsZero())
		})
	}
}

// TestOAuth2Authenticator_ValidateTenantForProvisioning tests tenant validation.
func TestOAuth2Authenticator_ValidateTenantForProvisioning(t *testing.T) {
	tests := []struct {
		name        string
		tenantID    string
		setupStore  func(*mockStore)
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid tenant with quota available",
			tenantID: "tenant-1",
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Name:   "Test Tenant",
					Status: TenantStatusActive,
					Quota: TenantQuota{
						MaxUsers: 10,
					},
				}
				// Add 5 users (below quota)
				for i := 0; i < 5; i++ {
					s.users[string(rune(i))] = &TenantUser{
						ID:       string(rune(i)),
						TenantID: "tenant-1",
					}
				}
			},
			wantErr: false,
		},
		{
			name:     "tenant not found",
			tenantID: "non-existent",
			setupStore: func(s *mockStore) {
				// Empty store
			},
			wantErr:     true,
			errContains: "tenant not found",
		},
		{
			name:     "tenant not active",
			tenantID: "tenant-1",
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Name:   "Test Tenant",
					Status: TenantStatusSuspended,
				}
			},
			wantErr:     true,
			errContains: "tenant is not active",
		},
		{
			name:     "quota exceeded",
			tenantID: "tenant-1",
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Name:   "Test Tenant",
					Status: TenantStatusActive,
					Quota: TenantQuota{
						MaxUsers: 5,
					},
				}
				// Add 5 users (at quota limit)
				for i := 0; i < 5; i++ {
					s.users[string(rune(i))] = &TenantUser{
						ID:       string(rune(i)),
						TenantID: "tenant-1",
					}
				}
			},
			wantErr:     true,
			errContains: "tenant user quota exceeded",
		},
		{
			name:     "unlimited quota (MaxUsers = 0)",
			tenantID: "tenant-1",
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Name:   "Test Tenant",
					Status: TenantStatusActive,
					Quota: TenantQuota{
						MaxUsers: 0, // Unlimited
					},
				}
				// Add many users
				for i := 0; i < 1000; i++ {
					s.users[string(rune(i))] = &TenantUser{
						ID:       string(rune(i)),
						TenantID: "tenant-1",
					}
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{
				tenants: make(map[string]*Tenant),
				users:   make(map[string]*TenantUser),
			}
			tt.setupStore(store)

			auth := &OAuth2Authenticator{
				store:  store,
				logger: zap.NewNop(),
			}

			tenant, err := auth.validateTenantForProvisioning(context.Background(), tt.tenantID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, tenant)
				assert.Equal(t, tt.tenantID, tenant.ID)
			}
		})
	}
}

// TestOAuth2Authenticator_Authenticate_Integration tests full authentication flow.
func TestOAuth2Authenticator_Authenticate_Integration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		authHeader    string
		setupKeycloak func(*mockKeycloakClient)
		setupStore    func(*mockStore)
		config        *OAuth2Config
		wantErr       bool
		errContains   string
		checkUser     func(*testing.T, *TenantUser)
	}{
		{
			name:       "successful authentication with existing user",
			authHeader: "Bearer valid-token",
			setupKeycloak: func(kc *mockKeycloakClient) {
				kc.verifyTokenFunc = func(ctx context.Context, token string) (map[string]interface{}, error) {
					return map[string]interface{}{
						"sub":                "user-123",
						"email":              "user@example.com",
						"preferred_username": "john.doe",
						"groups":             []interface{}{"tenant-admin"},
						"tenant_id":          "tenant-1",
					}, nil
				}
			},
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Status: TenantStatusActive,
				}
				s.users["existing-user"] = &TenantUser{
					ID:            "existing-user",
					TenantID:      "tenant-1",
					OAuthSubject:  "user-123",
					OAuthProvider: "keycloak",
					Email:         "user@example.com",
					RoleID:        "role-admin",
				}
				s.roles["role-admin"] = &Role{
					ID:   "role-admin",
					Name: "tenant-admin",
				}
			},
			config: &OAuth2Config{
				Enabled:            true,
				RequireTenantClaim: true,
			},
			wantErr: false,
			checkUser: func(t *testing.T, user *TenantUser) {
				assert.Equal(t, "existing-user", user.ID)
				assert.Equal(t, "user-123", user.OAuthSubject)
			},
		},
		{
			name:       "successful authentication with auto-provisioning",
			authHeader: "Bearer valid-token",
			setupKeycloak: func(kc *mockKeycloakClient) {
				kc.verifyTokenFunc = func(ctx context.Context, token string) (map[string]interface{}, error) {
					return map[string]interface{}{
						"sub":                "new-user-123",
						"email":              "newuser@example.com",
						"preferred_username": "jane.doe",
						"groups":             []interface{}{"tenant-viewer"},
						"tenant_id":          "tenant-1",
					}, nil
				}
			},
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Status: TenantStatusActive,
					Quota: TenantQuota{
						MaxUsers: 10,
					},
				}
				s.roles["role-viewer"] = &Role{
					ID:   "role-viewer",
					Name: "tenant-viewer",
				}
			},
			config: &OAuth2Config{
				Enabled:            true,
				AutoProvisionUsers: true,
				RequireTenantClaim: true,
				GroupRoleMapping: map[string]string{
					"tenant-viewer": "role-viewer",
				},
			},
			wantErr: false,
			checkUser: func(t *testing.T, user *TenantUser) {
				assert.NotEmpty(t, user.ID)
				assert.Equal(t, "new-user-123", user.OAuthSubject)
				assert.Equal(t, "keycloak", user.OAuthProvider)
				assert.Equal(t, "newuser@example.com", user.Email)
				assert.Equal(t, "jane.doe", user.CommonName)
				assert.Equal(t, "role-viewer", user.RoleID)
			},
		},
		{
			name:       "token verification failure",
			authHeader: "Bearer invalid-token",
			setupKeycloak: func(kc *mockKeycloakClient) {
				kc.verifyTokenFunc = func(ctx context.Context, token string) (map[string]interface{}, error) {
					return nil, errors.New("invalid token")
				}
			},
			setupStore: func(s *mockStore) {},
			config: &OAuth2Config{
				Enabled: true,
			},
			wantErr:     true,
			errContains: "invalid token",
		},
		{
			name:       "user not found and auto-provisioning disabled",
			authHeader: "Bearer valid-token",
			setupKeycloak: func(kc *mockKeycloakClient) {
				kc.verifyTokenFunc = func(ctx context.Context, token string) (map[string]interface{}, error) {
					return map[string]interface{}{
						"sub":       "unknown-user",
						"tenant_id": "tenant-1",
					}, nil
				}
			},
			setupStore: func(s *mockStore) {
				s.tenants["tenant-1"] = &Tenant{
					ID:     "tenant-1",
					Status: TenantStatusActive,
				}
			},
			config: &OAuth2Config{
				Enabled:            true,
				AutoProvisionUsers: false,
				RequireTenantClaim: true,
			},
			wantErr:     true,
			errContains: "user not found and auto-provisioning disabled",
		},
		{
			name:          "missing bearer token",
			authHeader:    "",
			setupKeycloak: func(kc *mockKeycloakClient) {},
			setupStore:    func(s *mockStore) {},
			config: &OAuth2Config{
				Enabled: true,
			},
			wantErr:     true,
			errContains: "failed to extract bearer token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.authHeader != "" {
				c.Request.Header.Set("Authorization", tt.authHeader)
			}

			keycloakClient := &mockKeycloakClient{}
			tt.setupKeycloak(keycloakClient)

			store := &mockStore{
				tenants: make(map[string]*Tenant),
				users:   make(map[string]*TenantUser),
				roles:   make(map[string]*Role),
			}
			tt.setupStore(store)

			auth := &OAuth2Authenticator{
				keycloakClient: keycloakClient,
				store:          store,
				config:         tt.config,
				logger:         zap.NewNop(),
			}

			user, role, tenant, err := auth.Authenticate(context.Background(), c, "test-request-id")

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.NotNil(t, role)
				assert.NotNil(t, tenant)
				if tt.checkUser != nil {
					tt.checkUser(t, user)
				}
			}
		})
	}
}
