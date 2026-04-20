package keycloak

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_VerifyToken_Unit(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		response   map[string]interface{}
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:  "valid active token",
			token: "valid-token",
			response: map[string]interface{}{
				"active": true,
				"sub":    "user-123",
				"email":  "user@example.com",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:  "inactive token",
			token: "expired-token",
			response: map[string]interface{}{
				"active": false,
			},
			statusCode: http.StatusOK,
			wantErr:    true,
			errMsg:     "token is not active",
		},
		{
			name:       "server error",
			token:      "any-token",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
			errMsg:     "token introspection failed",
		},
		{
			name:  "missing active field",
			token: "weird-token",
			response: map[string]interface{}{
				"sub": "user-123",
			},
			statusCode: http.StatusOK,
			wantErr:    true,
			errMsg:     "token is not active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/realms/test/protocol/openid-connect/token/introspect" {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

					w.WriteHeader(tt.statusCode)
					if tt.response != nil {
						json.NewEncoder(w).Encode(tt.response)
					} else {
						w.Write([]byte("server error"))
					}
					return
				}
				http.Error(w, "not found", http.StatusNotFound)
			}))
			defer server.Close()

			client, err := NewClient(&Config{
				BaseURL:      server.URL,
				Realm:        "test",
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				Timeout:      5 * time.Second,
			})
			require.NoError(t, err)

			result, err := client.VerifyToken(context.Background(), tt.token)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, true, result["active"])
			}
		})
	}
}

func TestClient_GetUserInfo_Unit(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		response   map[string]interface{}
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:  "valid token returns user info",
			token: "valid-token",
			response: map[string]interface{}{
				"sub":   "user-123",
				"email": "user@example.com",
				"name":  "Test User",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "unauthorized token",
			token:      "invalid-token",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
			errMsg:     "userinfo request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/realms/test/protocol/openid-connect/userinfo" {
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "Bearer "+tt.token, r.Header.Get("Authorization"))

					w.WriteHeader(tt.statusCode)
					if tt.response != nil {
						json.NewEncoder(w).Encode(tt.response)
					} else {
						w.Write([]byte("error"))
					}
					return
				}
				http.Error(w, "not found", http.StatusNotFound)
			}))
			defer server.Close()

			client, err := NewClient(&Config{
				BaseURL:      server.URL,
				Realm:        "test",
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				Timeout:      5 * time.Second,
			})
			require.NoError(t, err)

			result, err := client.GetUserInfo(context.Background(), tt.token)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "user@example.com", result["email"])
			}
		})
	}
}

func TestClient_DevExchangePasswordCredentials_Unit(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		password   string
		response   *TokenResponse
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:     "valid credentials",
			username: "admin",
			password: "secret",
			response: &TokenResponse{
				AccessToken:  "access-token-123",
				ExpiresIn:    300,
				TokenType:    "Bearer",
				RefreshToken: "refresh-token-456",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "invalid credentials",
			username:   "admin",
			password:   "wrong",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
			errMsg:     "token request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/realms/test/protocol/openid-connect/token" {
					assert.Equal(t, http.MethodPost, r.Method)

					w.WriteHeader(tt.statusCode)
					if tt.response != nil {
						json.NewEncoder(w).Encode(tt.response)
					} else {
						w.Write([]byte(`{"error":"invalid_grant"}`))
					}
					return
				}
				http.Error(w, "not found", http.StatusNotFound)
			}))
			defer server.Close()

			client, err := NewClient(&Config{
				BaseURL:      server.URL,
				Realm:        "test",
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				Timeout:      5 * time.Second,
			})
			require.NoError(t, err)

			result, err := client.DevExchangePasswordCredentials(context.Background(), tt.username, tt.password)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.response.AccessToken, result.AccessToken)
				assert.Equal(t, tt.response.TokenType, result.TokenType)
			}
		})
	}
}

// setupMockAdminServer creates a mock server that handles both admin token and admin API requests.
func setupMockAdminServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()

	mux := http.NewServeMux()

	// Token endpoint for admin auth.
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"access_token": "admin-token",
			"expires_in":   300,
			"token_type":   "Bearer",
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Admin API handler.
	mux.HandleFunc("/admin/realms/test/", handler)

	server := httptest.NewServer(mux)

	client, err := NewClient(&Config{
		BaseURL:       server.URL,
		Realm:         "test",
		ClientID:      "test-client",
		ClientSecret:  "test-secret",
		AdminUsername: "admin",
		AdminPassword: "admin",
		Timeout:       5 * time.Second,
	})
	require.NoError(t, err)

	return client, server
}

func TestClient_CreateClientRole(t *testing.T) {
	tests := []struct {
		name       string
		clientID   string
		role       *Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			clientID:   "client-uuid-1",
			role:       &Role{Name: "test-role", Description: "Test"},
			statusCode: http.StatusCreated,
			wantErr:    false,
		},
		{
			name:       "conflict",
			clientID:   "client-uuid-1",
			role:       &Role{Name: "duplicate-role"},
			statusCode: http.StatusConflict,
			wantErr:    true,
			errMsg:     "client role already exists",
		},
		{
			name:       "server error",
			clientID:   "client-uuid-1",
			role:       &Role{Name: "error-role"},
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
			errMsg:     "create client role failed",
		},
		{
			name:     "empty clientID",
			clientID: "",
			role:     &Role{Name: "test-role"},
			wantErr:  true,
			errMsg:   "clientID is required",
		},
		{
			name:     "empty role name",
			clientID: "client-uuid-1",
			role:     &Role{Name: ""},
			wantErr:  true,
			errMsg:   "role name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that fail before making HTTP requests.
			if tt.clientID == "" || tt.role.Name == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				err = client.CreateClientRole(context.Background(), tt.clientID, tt.role)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			defer server.Close()

			err := client.CreateClientRole(context.Background(), tt.clientID, tt.role)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_GetClientRole(t *testing.T) {
	tests := []struct {
		name       string
		clientID   string
		roleName   string
		response   *Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			clientID:   "client-uuid-1",
			roleName:   "test-role",
			response:   &Role{ID: "role-id", Name: "test-role", Description: "Test"},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "not found",
			clientID:   "client-uuid-1",
			roleName:   "missing-role",
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "client role not found",
		},
		{
			name:     "empty clientID",
			clientID: "",
			roleName: "test-role",
			wantErr:  true,
			errMsg:   "clientID is required",
		},
		{
			name:     "empty role name",
			clientID: "client-uuid-1",
			roleName: "",
			wantErr:  true,
			errMsg:   "role name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.clientID == "" || tt.roleName == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				_, err = client.GetClientRole(context.Background(), tt.clientID, tt.roleName)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			})
			defer server.Close()

			role, err := client.GetClientRole(context.Background(), tt.clientID, tt.roleName)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.response.Name, role.Name)
			}
		})
	}
}

func TestClient_ListClientRoles(t *testing.T) {
	tests := []struct {
		name       string
		clientID   string
		response   []Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:     "success",
			clientID: "client-uuid-1",
			response: []Role{
				{ID: "role-1", Name: "admin"},
				{ID: "role-2", Name: "viewer"},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:     "empty clientID",
			clientID: "",
			wantErr:  true,
			errMsg:   "clientID is required",
		},
		{
			name:       "server error",
			clientID:   "client-uuid-1",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
			errMsg:     "list client roles failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.clientID == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				_, err = client.ListClientRoles(context.Background(), tt.clientID)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			})
			defer server.Close()

			roles, err := client.ListClientRoles(context.Background(), tt.clientID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Len(t, roles, 2)
			}
		})
	}
}

func TestClient_GetUserRoleMappings(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		response   *RoleMapping
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:   "success",
			userID: "user-123",
			response: &RoleMapping{
				RealmMappings: []Role{{Name: "admin"}},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "user not found",
			userID:     "user-999",
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:    "empty userID",
			userID:  "",
			wantErr: true,
			errMsg:  "userID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.userID == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				_, err = client.GetUserRoleMappings(context.Background(), tt.userID)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			})
			defer server.Close()

			mapping, err := client.GetUserRoleMappings(context.Background(), tt.userID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Len(t, mapping.RealmMappings, 1)
			}
		})
	}
}

func TestClient_AddRealmRolesToUser(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		roles      []Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			userID:     "user-123",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "user not found",
			userID:     "user-999",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:    "empty userID",
			userID:  "",
			roles:   []Role{{Name: "admin"}},
			wantErr: true,
			errMsg:  "userID is required",
		},
		{
			name:    "empty roles",
			userID:  "user-123",
			roles:   []Role{},
			wantErr: true,
			errMsg:  "at least one role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.userID == "" || len(tt.roles) == 0 {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				err = client.AddRealmRolesToUser(context.Background(), tt.userID, tt.roles)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			defer server.Close()

			err := client.AddRealmRolesToUser(context.Background(), tt.userID, tt.roles)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_RemoveRealmRolesFromUser(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		roles      []Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			userID:     "user-123",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "user not found",
			userID:     "user-999",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:    "empty userID",
			userID:  "",
			roles:   []Role{{Name: "admin"}},
			wantErr: true,
			errMsg:  "userID is required",
		},
		{
			name:    "empty roles",
			userID:  "user-123",
			roles:   []Role{},
			wantErr: true,
			errMsg:  "at least one role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.userID == "" || len(tt.roles) == 0 {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				err = client.RemoveRealmRolesFromUser(context.Background(), tt.userID, tt.roles)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			defer server.Close()

			err := client.RemoveRealmRolesFromUser(context.Background(), tt.userID, tt.roles)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_AddClientRolesToUser(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		clientID   string
		roles      []Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			userID:     "user-123",
			clientID:   "client-uuid-1",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "not found",
			userID:     "user-999",
			clientID:   "client-uuid-1",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:     "empty userID",
			userID:   "",
			clientID: "client-uuid-1",
			roles:    []Role{{Name: "admin"}},
			wantErr:  true,
			errMsg:   "userID is required",
		},
		{
			name:     "empty clientID",
			userID:   "user-123",
			clientID: "",
			roles:    []Role{{Name: "admin"}},
			wantErr:  true,
			errMsg:   "clientID is required",
		},
		{
			name:     "empty roles",
			userID:   "user-123",
			clientID: "client-uuid-1",
			roles:    []Role{},
			wantErr:  true,
			errMsg:   "at least one role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.userID == "" || tt.clientID == "" || len(tt.roles) == 0 {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				err = client.AddClientRolesToUser(context.Background(), tt.userID, tt.clientID, tt.roles)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			defer server.Close()

			err := client.AddClientRolesToUser(context.Background(), tt.userID, tt.clientID, tt.roles)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_RemoveClientRolesFromUser(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		clientID   string
		roles      []Role
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			userID:     "user-123",
			clientID:   "client-uuid-1",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "not found",
			userID:     "user-999",
			clientID:   "client-uuid-1",
			roles:      []Role{{Name: "admin"}},
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:     "empty userID",
			userID:   "",
			clientID: "client-uuid-1",
			roles:    []Role{{Name: "admin"}},
			wantErr:  true,
			errMsg:   "userID is required",
		},
		{
			name:     "empty clientID",
			userID:   "user-123",
			clientID: "",
			roles:    []Role{{Name: "admin"}},
			wantErr:  true,
			errMsg:   "clientID is required",
		},
		{
			name:     "empty roles",
			userID:   "user-123",
			clientID: "client-uuid-1",
			roles:    []Role{},
			wantErr:  true,
			errMsg:   "at least one role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.userID == "" || tt.clientID == "" || len(tt.roles) == 0 {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				err = client.RemoveClientRolesFromUser(context.Background(), tt.userID, tt.clientID, tt.roles)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			defer server.Close()

			err := client.RemoveClientRolesFromUser(context.Background(), tt.userID, tt.clientID, tt.roles)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_GetClientID(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
		response []struct {
			ID       string `json:"id"`
			ClientID string `json:"clientId"`
		}
		statusCode int
		wantErr    bool
		errMsg     string
		wantID     string
	}{
		{
			name:     "found",
			clientID: "netweave-gateway",
			response: []struct {
				ID       string `json:"id"`
				ClientID string `json:"clientId"`
			}{
				{ID: "uuid-1", ClientID: "account"},
				{ID: "uuid-2", ClientID: "netweave-gateway"},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantID:     "uuid-2",
		},
		{
			name:     "not found",
			clientID: "missing-client",
			response: []struct {
				ID       string `json:"id"`
				ClientID string `json:"clientId"`
			}{
				{ID: "uuid-1", ClientID: "account"},
			},
			statusCode: http.StatusOK,
			wantErr:    true,
			errMsg:     "client not found",
		},
		{
			name:     "empty clientID",
			clientID: "",
			wantErr:  true,
			errMsg:   "clientID is required",
		},
		{
			name:       "server error",
			clientID:   "netweave-gateway",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
			errMsg:     "list clients failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.clientID == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				_, err = client.GetClientID(context.Background(), tt.clientID)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			// The GetClientID calls "/clients" which is under admin API.
			mux := http.NewServeMux()
			tokenPath := "/realms/master/protocol/openid-connect/token"
			mux.HandleFunc(tokenPath, func(w http.ResponseWriter, _ *http.Request) {
				resp := map[string]interface{}{"access_token": "admin-token", "expires_in": 300, "token_type": "Bearer"}
				json.NewEncoder(w).Encode(resp)
			})
			mux.HandleFunc("/admin/realms/test/clients", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			client, err := NewClient(&Config{
				BaseURL:       server.URL,
				Realm:         "test",
				ClientID:      "test",
				ClientSecret:  "secret",
				AdminUsername: "admin",
				AdminPassword: "admin",
				Timeout:       5 * time.Second,
			})
			require.NoError(t, err)

			id, err := client.GetClientID(context.Background(), tt.clientID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

func TestClient_GetUserByUsername_Unit(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		response   []User
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:     "found",
			username: "alice",
			response: []User{
				{ID: "user-1", Username: "alice", Email: "alice@example.com"},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "not found (empty result)",
			username:   "nobody",
			response:   []User{},
			statusCode: http.StatusOK,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:     "empty username",
			username: "",
			wantErr:  true,
			errMsg:   "username is required",
		},
		{
			name:       "server error",
			username:   "alice",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
			errMsg:     "get user by username failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.username == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				_, err = client.GetUserByUsername(context.Background(), tt.username)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				}
			})
			defer server.Close()

			user, err := client.GetUserByUsername(context.Background(), tt.username)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "alice", user.Username)
			}
		})
	}
}

func TestClient_SetUserPassword_Unit(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		password   string
		temporary  bool
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "success",
			userID:     "user-123",
			password:   "newP@ss!",
			temporary:  false,
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "user not found",
			userID:     "user-999",
			password:   "newP@ss!",
			statusCode: http.StatusNotFound,
			wantErr:    true,
			errMsg:     "user not found",
		},
		{
			name:     "empty userID",
			userID:   "",
			password: "newP@ss!",
			wantErr:  true,
			errMsg:   "userID is required",
		},
		{
			name:     "empty password",
			userID:   "user-123",
			password: "",
			wantErr:  true,
			errMsg:   "password is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.userID == "" || tt.password == "" {
				client, err := NewClient(&Config{
					BaseURL:      "http://localhost",
					Realm:        "test",
					ClientID:     "test",
					ClientSecret: "secret",
					Timeout:      time.Second,
				})
				require.NoError(t, err)
				err = client.SetUserPassword(context.Background(), tt.userID, tt.password, tt.temporary)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			defer server.Close()

			err := client.SetUserPassword(context.Background(), tt.userID, tt.password, tt.temporary)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_GetUserAttribute(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		attrName  string
		user      *User
		wantErr   bool
		errMsg    string
		wantValue string
	}{
		{
			name:     "attribute found",
			userID:   "user-1",
			attrName: "tenant_id",
			user: &User{
				ID:         "user-1",
				Username:   "alice",
				Attributes: map[string][]string{"tenant_id": {"tenant-1"}},
			},
			wantErr:   false,
			wantValue: "tenant-1",
		},
		{
			name:     "attribute not found",
			userID:   "user-1",
			attrName: "missing",
			user: &User{
				ID:         "user-1",
				Username:   "alice",
				Attributes: map[string][]string{"tenant_id": {"tenant-1"}},
			},
			wantErr: true,
			errMsg:  "attribute not found",
		},
		{
			name:     "nil attributes",
			userID:   "user-1",
			attrName: "tenant_id",
			user: &User{
				ID:       "user-1",
				Username: "alice",
			},
			wantErr: true,
			errMsg:  "attribute not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := setupMockAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(tt.user)
			})
			defer server.Close()

			val, err := client.GetUserAttribute(context.Background(), tt.userID, tt.attrName)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, val)
			}
		})
	}
}

func TestClient_SetUserAttribute(t *testing.T) {
	t.Run("sets attribute successfully", func(t *testing.T) {
		var updatedUser *User

		mux := http.NewServeMux()
		mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{"access_token": "admin-token", "expires_in": 300, "token_type": "Bearer"}
			json.NewEncoder(w).Encode(resp)
		})
		mux.HandleFunc("/admin/realms/test/users/user-1", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				user := &User{
					ID:         "user-1",
					Username:   "alice",
					Attributes: map[string][]string{"existing": {"val"}},
				}
				json.NewEncoder(w).Encode(user)
			case http.MethodPut:
				json.NewDecoder(r.Body).Decode(&updatedUser)
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "not allowed", http.StatusMethodNotAllowed)
			}
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client, err := NewClient(&Config{
			BaseURL:       server.URL,
			Realm:         "test",
			ClientID:      "test",
			ClientSecret:  "secret",
			AdminUsername: "admin",
			AdminPassword: "admin",
			Timeout:       5 * time.Second,
		})
		require.NoError(t, err)

		err = client.SetUserAttribute(context.Background(), "user-1", "custom_attr", "custom_value")
		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		assert.Equal(t, []string{"custom_value"}, updatedUser.Attributes["custom_attr"])
	})

	t.Run("sets attribute on user with nil attributes", func(t *testing.T) {
		var updatedUser *User

		mux := http.NewServeMux()
		mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{"access_token": "admin-token", "expires_in": 300, "token_type": "Bearer"}
			json.NewEncoder(w).Encode(resp)
		})
		mux.HandleFunc("/admin/realms/test/users/user-2", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				user := &User{ID: "user-2", Username: "bob"}
				json.NewEncoder(w).Encode(user)
			case http.MethodPut:
				json.NewDecoder(r.Body).Decode(&updatedUser)
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "not allowed", http.StatusMethodNotAllowed)
			}
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client, err := NewClient(&Config{
			BaseURL:       server.URL,
			Realm:         "test",
			ClientID:      "test",
			ClientSecret:  "secret",
			AdminUsername: "admin",
			AdminPassword: "admin",
			Timeout:       5 * time.Second,
		})
		require.NoError(t, err)

		err = client.SetUserAttribute(context.Background(), "user-2", "new_attr", "new_value")
		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		assert.Equal(t, []string{"new_value"}, updatedUser.Attributes["new_attr"])
	})
}
