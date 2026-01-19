package keycloak

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

// mockKeycloakServer creates a test HTTP server that mocks Keycloak API responses.
type mockKeycloakServer struct {
	server          *httptest.Server
	mu              sync.RWMutex
	users           map[string]*User
	roles           map[string]*Role
	realmAttributes map[string]string
	tokens          map[string]string
	adminToken      string
}

func newMockKeycloakServer() *mockKeycloakServer {
	m := &mockKeycloakServer{
		users:           make(map[string]*User),
		roles:           make(map[string]*Role),
		realmAttributes: make(map[string]string),
		tokens:          make(map[string]string),
		adminToken:      "mock-admin-token",
	}

	mux := http.NewServeMux()

	// Well-known endpoint (for Ping)
	mux.HandleFunc("/realms/test/.well-known/openid-configuration", m.handleWellKnown)

	// Token endpoints (both test realm and master realm for admin auth)
	mux.HandleFunc("/realms/test/protocol/openid-connect/token", m.handleToken)
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", m.handleToken)

	// Realm endpoints
	mux.HandleFunc("/admin/realms/test", m.handleRealm)

	// User endpoints
	mux.HandleFunc("/admin/realms/test/users", m.handleUsers)
	mux.HandleFunc("/admin/realms/test/users/", m.handleUserByID)

	// Role endpoints
	mux.HandleFunc("/admin/realms/test/roles", m.handleRoles)
	mux.HandleFunc("/admin/realms/test/roles/", m.handleRoleByName)

	m.server = httptest.NewServer(mux)
	return m
}

func (m *mockKeycloakServer) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]interface{}{
		"issuer":                 m.server.URL + "/realms/test",
		"authorization_endpoint": m.server.URL + "/realms/test/protocol/openid-connect/auth",
		"token_endpoint":         m.server.URL + "/realms/test/protocol/openid-connect/token",
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *mockKeycloakServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]interface{}{
		"access_token":       m.adminToken,
		"expires_in":         300,
		"refresh_expires_in": 1800,
		"token_type":         "Bearer",
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockKeycloakServer) handleRealm(w http.ResponseWriter, r *http.Request) {
	if !m.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		m.mu.RLock()
		attrs := make(map[string]string, len(m.realmAttributes))
		for k, v := range m.realmAttributes {
			attrs[k] = v
		}
		m.mu.RUnlock()

		resp := map[string]interface{}{
			"id":         "test",
			"realm":      "test",
			"enabled":    true,
			"attributes": attrs,
		}
		json.NewEncoder(w).Encode(resp)

	case http.MethodPut:
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if attrs, ok := req["attributes"].(map[string]interface{}); ok {
			// Replace all attributes (don't merge) to support deletion
			m.mu.Lock()
			m.realmAttributes = make(map[string]string)
			for k, v := range attrs {
				if str, ok := v.(string); ok {
					m.realmAttributes[k] = str
				}
			}
			m.mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockKeycloakServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !m.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		var users []*User
		for _, u := range m.users {
			users = append(users, u)
		}
		json.NewEncoder(w).Encode(users)

	case http.MethodPost:
		var user User
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Check for duplicate username
		for _, u := range m.users {
			if u.Username == user.Username {
				http.Error(w, "user exists", http.StatusConflict)
				return
			}
		}
		// Preserve the ID sent by the client (if provided)
		if user.ID == "" {
			user.ID = "user-" + user.Username
		}
		user.CreatedTimestamp = time.Now().Unix()
		m.users[user.ID] = &user
		w.Header().Set("Location", "/admin/realms/test/users/"+user.ID)
		w.WriteHeader(http.StatusCreated)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockKeycloakServer) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if !m.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract user ID from path
	userID := r.URL.Path[len("/admin/realms/test/users/"):]

	switch r.Method {
	case http.MethodGet:
		user, ok := m.users[userID]
		if !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(user)

	case http.MethodPut:
		if _, ok := m.users[userID]; !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		var user User
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user.ID = userID
		m.users[userID] = &user
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		if _, ok := m.users[userID]; !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		delete(m.users, userID)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockKeycloakServer) handleRoles(w http.ResponseWriter, r *http.Request) {
	if !m.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		var roles []*Role
		for _, role := range m.roles {
			roles = append(roles, role)
		}
		json.NewEncoder(w).Encode(roles)

	case http.MethodPost:
		var role Role
		if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Check for duplicate name
		for _, r := range m.roles {
			if r.Name == role.Name {
				http.Error(w, "role exists", http.StatusConflict)
				return
			}
		}
		// Preserve the ID sent by the client (if provided)
		if role.ID == "" {
			role.ID = "role-" + role.Name
		}
		m.roles[role.Name] = &role
		w.WriteHeader(http.StatusCreated)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockKeycloakServer) handleRoleByName(w http.ResponseWriter, r *http.Request) {
	if !m.checkAuth(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract role name from path
	roleName := r.URL.Path[len("/admin/realms/test/roles/"):]

	switch r.Method {
	case http.MethodGet:
		role, ok := m.roles[roleName]
		if !ok {
			http.Error(w, "role not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(role)

	case http.MethodPut:
		if _, ok := m.roles[roleName]; !ok {
			http.Error(w, "role not found", http.StatusNotFound)
			return
		}
		var role Role
		if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		role.Name = roleName
		m.roles[roleName] = &role
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		if _, ok := m.roles[roleName]; !ok {
			http.Error(w, "role not found", http.StatusNotFound)
			return
		}
		delete(m.roles, roleName)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockKeycloakServer) checkAuth(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	return authHeader == "Bearer "+m.adminToken
}

func (m *mockKeycloakServer) close() {
	m.server.Close()
}

func setupTestStore(t *testing.T) (*Store, *mockKeycloakServer) {
	t.Helper()

	mock := newMockKeycloakServer()

	client, err := NewClient(&Config{
		BaseURL:      mock.server.URL,
		Realm:        "test",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		Timeout:      5 * time.Second,
	})
	require.NoError(t, err)

	logger := zap.NewNop()
	store := NewStore(client, logger)

	return store, mock
}

// TenantStore Tests

func TestStore_CreateTenant(t *testing.T) {
	tests := []struct {
		name    string
		tenant  *auth.Tenant
		wantErr bool
		errType error
	}{
		{
			name: "valid tenant",
			tenant: &auth.Tenant{
				ID:          "tenant-1",
				Name:        "Test Tenant",
				Description: "Test Description",
				Status:      auth.TenantStatusActive,
				Quota:       auth.TenantQuota{MaxUsers: 100},
				CreatedAt:   time.Now(),
			},
			wantErr: false,
		},
		{
			name: "duplicate tenant",
			tenant: &auth.Tenant{
				ID:     "tenant-1",
				Name:   "Duplicate",
				Status: auth.TenantStatusActive,
			},
			wantErr: true,
			errType: auth.ErrTenantExists,
		},
		{
			name: "invalid tenant ID",
			tenant: &auth.Tenant{
				ID:     "",
				Name:   "Invalid",
				Status: auth.TenantStatusActive,
			},
			wantErr: true,
			errType: auth.ErrInvalidTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock := setupTestStore(t)
			defer mock.close()

			ctx := context.Background()

			// For duplicate test, create tenant first
			if errors.Is(tt.errType, auth.ErrTenantExists) {
				firstTenant := &auth.Tenant{
					ID:     tt.tenant.ID,
					Name:   "First Tenant",
					Status: auth.TenantStatusActive,
				}
				err := store.CreateTenant(ctx, firstTenant)
				require.NoError(t, err)
			}

			err := store.CreateTenant(ctx, tt.tenant)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify tenant was created
				retrieved, err := store.GetTenant(ctx, tt.tenant.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.tenant.ID, retrieved.ID)
				assert.Equal(t, tt.tenant.Name, retrieved.Name)
				assert.Equal(t, tt.tenant.Status, retrieved.Status)
			}
		})
	}
}

func TestStore_GetTenant(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:          "tenant-1",
		Name:        "Test Tenant",
		Description: "Description",
		Status:      auth.TenantStatusActive,
		Quota:       auth.TenantQuota{MaxUsers: 50},
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	tests := []struct {
		name     string
		tenantID string
		wantErr  bool
		errType  error
	}{
		{
			name:     "existing tenant",
			tenantID: "tenant-1",
			wantErr:  false,
		},
		{
			name:     "non-existent tenant",
			tenantID: "tenant-999",
			wantErr:  true,
			errType:  auth.ErrTenantNotFound,
		},
		{
			name:     "empty tenant ID",
			tenantID: "",
			wantErr:  true,
			errType:  auth.ErrInvalidTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := store.GetTenant(ctx, tt.tenantID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, retrieved)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, tenant.ID, retrieved.ID)
				assert.Equal(t, tenant.Name, retrieved.Name)
			}
		})
	}
}

func TestStore_UpdateTenant(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Original Name",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	tests := []struct {
		name    string
		tenant  *auth.Tenant
		wantErr bool
		errType error
	}{
		{
			name: "update name",
			tenant: &auth.Tenant{
				ID:     "tenant-1",
				Name:   "Updated Name",
				Status: auth.TenantStatusActive,
			},
			wantErr: false,
		},
		{
			name: "update status",
			tenant: &auth.Tenant{
				ID:     "tenant-1",
				Name:   "Test",
				Status: auth.TenantStatusSuspended,
			},
			wantErr: false,
		},
		{
			name: "non-existent tenant",
			tenant: &auth.Tenant{
				ID:     "tenant-999",
				Name:   "Non-existent",
				Status: auth.TenantStatusActive,
			},
			wantErr: true,
			errType: auth.ErrTenantNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateTenant(ctx, tt.tenant)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify update
				retrieved, err := store.GetTenant(ctx, tt.tenant.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.tenant.Name, retrieved.Name)
				assert.Equal(t, tt.tenant.Status, retrieved.Status)
			}
		})
	}
}

func TestStore_DeleteTenant(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	tests := []struct {
		name     string
		tenantID string
		wantErr  bool
		errType  error
	}{
		{
			name:     "delete existing tenant",
			tenantID: "tenant-1",
			wantErr:  false,
		},
		{
			name:     "delete non-existent tenant",
			tenantID: "tenant-999",
			wantErr:  true,
			errType:  auth.ErrTenantNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.DeleteTenant(ctx, tt.tenantID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify deletion
				_, err := store.GetTenant(ctx, tt.tenantID)
				assert.ErrorIs(t, err, auth.ErrTenantNotFound)
			}
		})
	}
}

func TestStore_ListTenants(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create multiple tenants
	tenants := []*auth.Tenant{
		{ID: "tenant-1", Name: "Tenant 1", Status: auth.TenantStatusActive},
		{ID: "tenant-2", Name: "Tenant 2", Status: auth.TenantStatusActive},
		{ID: "tenant-3", Name: "Tenant 3", Status: auth.TenantStatusSuspended},
	}

	for _, tenant := range tenants {
		err := store.CreateTenant(ctx, tenant)
		require.NoError(t, err)
	}

	// List all tenants
	list, err := store.ListTenants(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify all tenants are present
	ids := make(map[string]bool)
	for _, t := range list {
		ids[t.ID] = true
	}
	assert.True(t, ids["tenant-1"])
	assert.True(t, ids["tenant-2"])
	assert.True(t, ids["tenant-3"])
}

func TestStore_IncrementUsage(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
		Usage: auth.TenantUsage{
			Subscriptions: 5,
			Users:         10,
		},
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tenantID  string
		usageType string
		wantErr   bool
	}{
		{
			name:      "increment subscriptions",
			tenantID:  "tenant-1",
			usageType: "subscriptions",
			wantErr:   false,
		},
		{
			name:      "increment users",
			tenantID:  "tenant-1",
			usageType: "users",
			wantErr:   false,
		},
		{
			name:      "invalid tenant",
			tenantID:  "tenant-999",
			usageType: "users",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.IncrementUsage(ctx, tt.tenantID, tt.usageType)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify increment
				retrieved, err := store.GetTenant(ctx, tt.tenantID)
				require.NoError(t, err)

				switch tt.usageType {
				case "subscriptions":
					assert.Equal(t, tenant.Usage.Subscriptions+1, retrieved.Usage.Subscriptions)
				case "users":
					assert.Equal(t, tenant.Usage.Users+1, retrieved.Usage.Users)
				}
			}
		})
	}
}

func TestStore_DecrementUsage(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant with usage
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
		Usage: auth.TenantUsage{
			Subscriptions: 5,
			Users:         10,
		},
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tenantID  string
		usageType string
		wantErr   bool
	}{
		{
			name:      "decrement subscriptions",
			tenantID:  "tenant-1",
			usageType: "subscriptions",
			wantErr:   false,
		},
		{
			name:      "decrement users",
			tenantID:  "tenant-1",
			usageType: "users",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.DecrementUsage(ctx, tt.tenantID, tt.usageType)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify decrement
				retrieved, err := store.GetTenant(ctx, tt.tenantID)
				require.NoError(t, err)

				switch tt.usageType {
				case "subscriptions":
					assert.Equal(t, tenant.Usage.Subscriptions-1, retrieved.Usage.Subscriptions)
				case "users":
					assert.Equal(t, tenant.Usage.Users-1, retrieved.Usage.Users)
				}
			}
		})
	}
}

// UserStore Tests

func TestStore_CreateUser(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	tests := []struct {
		name    string
		user    *auth.TenantUser
		wantErr bool
		errType error
	}{
		{
			name: "valid user",
			user: &auth.TenantUser{
				ID:         "user-1",
				TenantID:   "tenant-1",
				CommonName: "testuser",
				Email:      "test@example.com",
				RoleID:     "role-1",
				IsActive:   true,
			},
			wantErr: false,
		},
		{
			name: "duplicate user",
			user: &auth.TenantUser{
				ID:         "user-1",
				TenantID:   "tenant-1",
				CommonName: "testuser",
				Email:      "test@example.com",
				RoleID:     "role-1",
				IsActive:   true,
			},
			wantErr: true,
			errType: auth.ErrUserExists,
		},
		{
			name: "invalid user ID",
			user: &auth.TenantUser{
				ID:         "",
				TenantID:   "tenant-1",
				CommonName: "invalid",
				RoleID:     "role-1",
			},
			wantErr: true,
			errType: auth.ErrInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreateUser(ctx, tt.user)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify user was created
				retrieved, err := store.GetUser(ctx, tt.user.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.user.ID, retrieved.ID)
				assert.Equal(t, tt.user.Email, retrieved.Email)
			}
		})
	}
}

func TestStore_GetUser(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create test user
	user := &auth.TenantUser{
		ID:         "user-1",
		TenantID:   "tenant-1",
		CommonName: "testuser",
		Email:      "test@example.com",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name    string
		userID  string
		wantErr bool
		errType error
	}{
		{
			name:    "existing user",
			userID:  "user-1",
			wantErr: false,
		},
		{
			name:    "non-existent user",
			userID:  "user-999",
			wantErr: true,
			errType: auth.ErrUserNotFound,
		},
		{
			name:    "empty user ID",
			userID:  "",
			wantErr: true,
			errType: auth.ErrInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := store.GetUser(ctx, tt.userID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, retrieved)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, user.ID, retrieved.ID)
				assert.Equal(t, user.Email, retrieved.Email)
			}
		})
	}
}

func TestStore_GetUserByEmail(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create test user
	user := &auth.TenantUser{
		ID:         "user-1",
		TenantID:   "tenant-1",
		CommonName: "testuser",
		Email:      "test@example.com",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name    string
		email   string
		wantErr bool
		errType error
	}{
		{
			name:    "existing user by email",
			email:   "test@example.com",
			wantErr: false,
		},
		{
			name:    "non-existent email",
			email:   "nonexistent@example.com",
			wantErr: true,
			errType: auth.ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := store.GetUserByEmail(ctx, tt.email)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, retrieved)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, user.Email, retrieved.Email)
			}
		})
	}
}

func TestStore_UpdateUser(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create test user
	user := &auth.TenantUser{
		ID:         "user-1",
		TenantID:   "tenant-1",
		CommonName: "original",
		Email:      "original@example.com",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name    string
		user    *auth.TenantUser
		wantErr bool
		errType error
	}{
		{
			name: "update email",
			user: &auth.TenantUser{
				ID:         "user-1",
				TenantID:   "tenant-1",
				CommonName: "original",
				Email:      "updated@example.com",
				RoleID:     "role-1",
				IsActive:   true,
			},
			wantErr: false,
		},
		{
			name: "update active status",
			user: &auth.TenantUser{
				ID:         "user-1",
				TenantID:   "tenant-1",
				CommonName: "original",
				Email:      "original@example.com",
				RoleID:     "role-1",
				IsActive:   false,
			},
			wantErr: false,
		},
		{
			name: "non-existent user",
			user: &auth.TenantUser{
				ID:         "user-999",
				TenantID:   "tenant-1",
				CommonName: "nonexistent",
				RoleID:     "role-1",
			},
			wantErr: true,
			errType: auth.ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateUser(ctx, tt.user)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify update
				retrieved, err := store.GetUser(ctx, tt.user.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.user.Email, retrieved.Email)
				assert.Equal(t, tt.user.IsActive, retrieved.IsActive)
			}
		})
	}
}

func TestStore_DeleteUser(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create test user
	user := &auth.TenantUser{
		ID:         "user-1",
		TenantID:   "tenant-1",
		CommonName: "testuser",
		Email:      "test@example.com",
		RoleID:     "role-1",
		IsActive:   true,
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	tests := []struct {
		name    string
		userID  string
		wantErr bool
		errType error
	}{
		{
			name:    "delete existing user",
			userID:  "user-1",
			wantErr: false,
		},
		{
			name:    "delete non-existent user",
			userID:  "user-999",
			wantErr: true,
			errType: auth.ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.DeleteUser(ctx, tt.userID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify deletion
				_, err := store.GetUser(ctx, tt.userID)
				assert.ErrorIs(t, err, auth.ErrUserNotFound)
			}
		})
	}
}

func TestStore_ListUsersByTenant(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Status: auth.TenantStatusActive,
	}
	err := store.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	// Create multiple users
	users := []*auth.TenantUser{
		{ID: "user-1", TenantID: "tenant-1", CommonName: "user1", RoleID: "role-1", IsActive: true},
		{ID: "user-2", TenantID: "tenant-1", CommonName: "user2", RoleID: "role-1", IsActive: true},
		{ID: "user-3", TenantID: "tenant-1", CommonName: "user3", RoleID: "role-1", IsActive: false},
	}

	for _, user := range users {
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)
	}

	// List users by tenant
	list, err := store.ListUsersByTenant(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify all users are present
	ids := make(map[string]bool)
	for _, u := range list {
		ids[u.ID] = true
	}
	assert.True(t, ids["user-1"])
	assert.True(t, ids["user-2"])
	assert.True(t, ids["user-3"])
}

// RoleStore Tests

func TestStore_InitializeDefaultRoles(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	err := store.InitializeDefaultRoles(ctx)
	require.NoError(t, err)

	// Verify default roles were created
	roles, err := store.ListRoles(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 4)

	// Check specific roles exist
	roleNames := make(map[auth.RoleName]bool)
	for _, role := range roles {
		roleNames[role.Name] = true
	}

	assert.True(t, roleNames[auth.RolePlatformAdmin])
	assert.True(t, roleNames[auth.RoleTenantAdmin])
	assert.True(t, roleNames[auth.RoleOperator])
	assert.True(t, roleNames[auth.RoleViewer])
}

func TestStore_CreateRole(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	tests := []struct {
		name    string
		role    *auth.Role
		wantErr bool
		errType error
	}{
		{
			name: "valid role",
			role: &auth.Role{
				ID:          "role-1",
				Name:        "custom-role",
				Description: "Custom Role",
				Type:        auth.RoleTypeTenant,
				Permissions: []auth.Permission{
					auth.PermissionUserRead,
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate role",
			role: &auth.Role{
				ID:          "role-1",
				Name:        "custom-role",
				Description: "Duplicate",
				Type:        auth.RoleTypeTenant,
			},
			wantErr: true,
			errType: auth.ErrRoleExists,
		},
		{
			name: "invalid role ID",
			role: &auth.Role{
				ID:   "",
				Name: "invalid",
				Type: auth.RoleTypeTenant,
			},
			wantErr: true,
			errType: auth.ErrInvalidRoleID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreateRole(ctx, tt.role)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify role was created
				retrieved, err := store.GetRole(ctx, tt.role.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.role.ID, retrieved.ID)
				assert.Equal(t, tt.role.Name, retrieved.Name)
			}
		})
	}
}

func TestStore_GetRole(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test role
	role := &auth.Role{
		ID:          "role-1",
		Name:        "test-role",
		Description: "Test Role",
		Type:        auth.RoleTypeTenant,
	}
	err := store.CreateRole(ctx, role)
	require.NoError(t, err)

	tests := []struct {
		name    string
		roleID  string
		wantErr bool
		errType error
	}{
		{
			name:    "existing role",
			roleID:  "role-1",
			wantErr: false,
		},
		{
			name:    "non-existent role",
			roleID:  "role-999",
			wantErr: true,
			errType: auth.ErrRoleNotFound,
		},
		{
			name:    "empty role ID",
			roleID:  "",
			wantErr: true,
			errType: auth.ErrInvalidRoleID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := store.GetRole(ctx, tt.roleID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, retrieved)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, role.ID, retrieved.ID)
			}
		})
	}
}

func TestStore_GetRoleByName(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test role
	role := &auth.Role{
		ID:          "role-1",
		Name:        "test-role",
		Description: "Test Role",
		Type:        auth.RoleTypeTenant,
	}
	err := store.CreateRole(ctx, role)
	require.NoError(t, err)

	tests := []struct {
		name     string
		roleName auth.RoleName
		wantErr  bool
		errType  error
	}{
		{
			name:     "existing role",
			roleName: "test-role",
			wantErr:  false,
		},
		{
			name:     "non-existent role",
			roleName: "nonexistent",
			wantErr:  true,
			errType:  auth.ErrRoleNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := store.GetRoleByName(ctx, tt.roleName)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, retrieved)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, role.Name, retrieved.Name)
			}
		})
	}
}

func TestStore_UpdateRole(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test role
	role := &auth.Role{
		ID:          "role-1",
		Name:        "test-role",
		Description: "Original Description",
		Type:        auth.RoleTypeTenant,
	}
	err := store.CreateRole(ctx, role)
	require.NoError(t, err)

	tests := []struct {
		name    string
		role    *auth.Role
		wantErr bool
		errType error
	}{
		{
			name: "update description",
			role: &auth.Role{
				ID:          "role-1",
				Name:        "test-role",
				Description: "Updated Description",
				Type:        auth.RoleTypeTenant,
			},
			wantErr: false,
		},
		{
			name: "non-existent role",
			role: &auth.Role{
				ID:          "role-999",
				Name:        "nonexistent",
				Description: "Nonexistent",
				Type:        auth.RoleTypeTenant,
			},
			wantErr: true,
			errType: auth.ErrRoleNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateRole(ctx, tt.role)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify update
				retrieved, err := store.GetRole(ctx, tt.role.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.role.Description, retrieved.Description)
			}
		})
	}
}

func TestStore_DeleteRole(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create test role
	role := &auth.Role{
		ID:          "role-1",
		Name:        "test-role",
		Description: "Test Role",
		Type:        auth.RoleTypeTenant,
	}
	err := store.CreateRole(ctx, role)
	require.NoError(t, err)

	tests := []struct {
		name    string
		roleID  string
		wantErr bool
		errType error
	}{
		{
			name:    "delete existing role",
			roleID:  "role-1",
			wantErr: false,
		},
		{
			name:    "delete non-existent role",
			roleID:  "role-999",
			wantErr: true,
			errType: auth.ErrRoleNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.DeleteRole(ctx, tt.roleID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)

				// Verify deletion
				_, err := store.GetRole(ctx, tt.roleID)
				assert.ErrorIs(t, err, auth.ErrRoleNotFound)
			}
		})
	}
}

func TestStore_ListRoles(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Create multiple roles
	roles := []*auth.Role{
		{ID: "role-1", Name: "role-1", Description: "Role 1", Type: auth.RoleTypeTenant},
		{ID: "role-2", Name: "role-2", Description: "Role 2", Type: auth.RoleTypeTenant},
		{ID: "role-3", Name: "role-3", Description: "Role 3", Type: auth.RoleTypePlatform},
	}

	for _, role := range roles {
		err := store.CreateRole(ctx, role)
		require.NoError(t, err)
	}

	// List all roles
	list, err := store.ListRoles(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify all roles are present
	ids := make(map[string]bool)
	for _, r := range list {
		ids[r.ID] = true
	}
	assert.True(t, ids["role-1"])
	assert.True(t, ids["role-2"])
	assert.True(t, ids["role-3"])
}

// AuditStore Tests

func TestStore_LogEvent(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	event := &auth.AuditEvent{
		ID:       "event-1",
		TenantID: "tenant-1",
		UserID:   "user-1",
		Type:     auth.AuditEventAuthSuccess,
		Action:   "login",
		Details: map[string]string{
			"ip": "192.168.1.1",
		},
		Timestamp: time.Now(),
	}

	err := store.LogEvent(ctx, event)
	require.NoError(t, err)
}

func TestStore_ListEvents(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	// Log multiple events
	events := []*auth.AuditEvent{
		{
			ID:        "event-1",
			TenantID:  "tenant-1",
			UserID:    "user-1",
			Type:      auth.AuditEventAuthSuccess,
			Action:    "login",
			Timestamp: time.Now(),
		},
		{
			ID:        "event-2",
			TenantID:  "tenant-1",
			UserID:    "user-2",
			Type:      auth.AuditEventAuthFailure,
			Action:    "login",
			Timestamp: time.Now(),
		},
	}

	for _, event := range events {
		err := store.LogEvent(ctx, event)
		require.NoError(t, err)
	}

	// List events (placeholder implementation returns empty)
	list, err := store.ListEvents(ctx, "tenant-1", 10, 0)
	require.NoError(t, err)
	assert.NotNil(t, list)
}

// Store interface compliance tests

func TestStore_Close(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	err := store.Close()
	assert.NoError(t, err)
}

func TestStore_Ping(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()
	err := store.Ping(ctx)
	assert.NoError(t, err)
}

// Error handling tests

func TestStore_ErrorHandling(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	ctx := context.Background()

	t.Run("network error simulation", func(t *testing.T) {
		// Close mock server to simulate network error
		mock.close()

		err := store.Ping(ctx)
		assert.Error(t, err)
	})
}

// Conversion function tests

func TestStore_SerializeTenantToAttributes(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	tenant := &auth.Tenant{
		ID:          "tenant-1",
		Name:        "Test Tenant",
		Description: "Description",
		Status:      auth.TenantStatusActive,
		Quota:       auth.TenantQuota{MaxUsers: 100},
		Usage: auth.TenantUsage{
			Subscriptions: 5,
			Users:         10,
		},
	}

	attrs, err := store.serializeTenantToAttributes(tenant)
	require.NoError(t, err)
	assert.NotEmpty(t, attrs)

	// Verify key attributes are present
	assert.Contains(t, attrs, "tenant_tenant-1_name")
	assert.Contains(t, attrs, "tenant_tenant-1_status")
	assert.Equal(t, "Test Tenant", attrs["tenant_tenant-1_name"])
	assert.Equal(t, "active", attrs["tenant_tenant-1_status"])
}

func TestStore_DeserializeTenantFromAttributes(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	attrs := map[string]string{
		"tenant_tenant-1_name":           "Test Tenant",
		"tenant_tenant-1_description":    "Description",
		"tenant_tenant-1_status":         "active",
		"tenant_tenant-1_quota_maxUsers": "100",
		"tenant_tenant-1_usage_users":    "10",
		"tenant_tenant-1_createdAt":      "2024-01-01T00:00:00Z",
	}

	tenant, err := store.deserializeTenantFromAttributes("tenant-1", attrs)
	require.NoError(t, err)
	assert.NotNil(t, tenant)
	assert.Equal(t, "tenant-1", tenant.ID)
	assert.Equal(t, "Test Tenant", tenant.Name)
	assert.Equal(t, auth.TenantStatusActive, tenant.Status)
}

func TestStore_ConvertUserToKeycloak(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	user := &auth.TenantUser{
		ID:         "user-1",
		TenantID:   "tenant-1",
		CommonName: "testuser",
		Email:      "test@example.com",
		RoleID:     "role-1",
		IsActive:   true,
	}

	kcUser := store.convertUserToKeycloak(user)
	require.NotNil(t, kcUser)
	assert.Equal(t, user.ID, kcUser.ID)
	assert.Equal(t, user.CommonName, kcUser.Username)
	assert.Equal(t, user.Email, kcUser.Email)
	assert.Equal(t, user.IsActive, kcUser.Enabled)
}

func TestStore_ConvertKeycloakToUser(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	now := time.Now().Unix()
	kcUser := &User{
		ID:               "user-1",
		Username:         "testuser",
		Email:            "test@example.com",
		Enabled:          true,
		CreatedTimestamp: now,
		Attributes: map[string][]string{
			userAttrTenantID: {"tenant-1"},
			userAttrRoleID:   {"role-1"},
		},
	}

	user := store.convertKeycloakToUser(kcUser)
	assert.NotNil(t, user)
	assert.Equal(t, kcUser.ID, user.ID)
	assert.Equal(t, kcUser.Username, user.CommonName)
	assert.Equal(t, kcUser.Email, user.Email)
	assert.Equal(t, "tenant-1", user.TenantID)
	assert.Equal(t, "role-1", user.RoleID)
}

func TestStore_ConvertRoleToKeycloak(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	role := &auth.Role{
		ID:          "role-1",
		Name:        "test-role",
		Description: "Test Role",
		Type:        auth.RoleTypeTenant,
		Permissions: []auth.Permission{
			auth.PermissionUserRead,
			auth.PermissionUserCreate,
		},
	}

	kcRole := store.convertRoleToKeycloak(role)
	require.NotNil(t, kcRole)
	assert.Equal(t, string(role.Name), kcRole.Name)
	assert.Equal(t, role.Description, kcRole.Description)
}

func TestStore_ConvertKeycloakToRole(t *testing.T) {
	store, mock := setupTestStore(t)
	defer mock.close()

	kcRole := &Role{
		ID:          "role-1",
		Name:        "test-role",
		Description: "Test Role",
		Attributes: map[string][]string{
			roleAttrType: {"tenant"},
		},
	}

	role := store.convertKeycloakToRole(kcRole)
	assert.NotNil(t, role)
	assert.Equal(t, "role-1", role.ID)
	assert.Equal(t, auth.RoleName("test-role"), role.Name)
	assert.Equal(t, "Test Role", role.Description)
	assert.Equal(t, auth.RoleTypeTenant, role.Type)
}
