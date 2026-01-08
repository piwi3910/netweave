package osm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	// testTokenPath is the OSM API endpoint path for token authentication.
	testTokenPath = "/osm/admin/v1/tokens"
)

// TestNewClient tests the creation of a new OSM client.
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				NBIURL:         "https://osm.example.com:9999",
				Username:       "admin",
				Password:       "secret",
				RequestTimeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "invalid URL",
			config: &Config{
				NBIURL:         "://invalid-url",
				Username:       "admin",
				Password:       "secret",
				RequestTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "invalid nbiUrl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewClient() expected error but got none")
					return
				}
				// Check error message contains expected text (not exact match due to wrapping)
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("NewClient() error = %v, want to contain %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("NewClient() unexpected error: %v", err)
				return
			}

			if client == nil {
				t.Error("NewClient() returned nil client")
				return
			}

			// Verify client fields
			if client.config == nil {
				t.Error("Client config is nil")
			}
			if client.httpClient == nil {
				t.Error("Client httpClient is nil")
			}
			if client.baseURL == "" {
				t.Error("Client baseURL is empty")
			}
		})
	}
}

// TestAuthenticate tests the authentication flow.
func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		password   string
		project    string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name:     "successful authentication",
			username: "admin",
			password: "secret",
			project:  "admin",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.URL.Path != testTokenPath {
					t.Errorf("Expected path %s, got %s", testTokenPath, r.URL.Path)
				}

				// Parse request body
				var authReq map[string]string
				if err := json.NewDecoder(r.Body).Decode(&authReq); err != nil {
					t.Errorf("Failed to decode auth request: %v", err)
				}

				// Verify credentials
				if authReq["username"] != "admin" {
					t.Errorf("Expected username 'admin', got %s", authReq["username"])
				}
				if authReq["password"] != "secret" {
					t.Errorf("Expected password 'secret', got %s", authReq["password"])
				}

				// Send successful response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]string{
					"id":         "token-123456",
					"project_id": "admin",
					"expires":    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
				}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: false,
		},
		{
			name:     "authentication failure - invalid credentials",
			username: "admin",
			password: "wrong-password",
			project:  "admin",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"detail": "Invalid credentials"}`))
			},
			wantErr: true,
		},
		{
			name:     "authentication failure - server error",
			username: "admin",
			password: "secret",
			project:  "admin",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail": "Internal server error"}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResp))
			t.Cleanup(func() { server.Close() })

			// Create client
			config := &Config{
				NBIURL:         server.URL,
				Username:       tt.username,
				Password:       tt.password,
				Project:        tt.project,
				RequestTimeout: 5 * time.Second,
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}

			// Test authentication
			ctx := context.Background()
			err = client.Authenticate(ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("Authenticate() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Authenticate() unexpected error: %v", err)
				return
			}

			// Verify token was stored
			client.mu.RLock()
			if client.token == "" {
				t.Error("Token was not stored after successful authentication")
			}
			if client.tokenExpiry.IsZero() {
				t.Error("Token expiry was not set")
			}
			client.mu.RUnlock()
		})
	}
}

// TestAuthenticateWithCachedToken tests authentication with a cached valid token.
func TestAuthenticateWithCachedToken(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]string{
			"id":         "token-123456",
			"project_id": "admin",
			"expires":    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(func() { server.Close() })

	config := &Config{
		NBIURL:         server.URL,
		Username:       "admin",
		Password:       "secret",
		Project:        "admin",
		RequestTimeout: 5 * time.Second,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	ctx := context.Background()

	// First authentication should call the server
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("First Authenticate() failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 server call, got %d", callCount)
	}

	// Second authentication should use cached token (no server call)
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Second Authenticate() failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 server call (cached token), got %d", callCount)
	}
}

// TestAuthenticateTokenExpiry tests token refresh when expired.
func TestAuthenticateTokenExpiry(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// First call: short expiry, second call: normal expiry
		expiryTime := time.Now().Add(1 * time.Hour)
		if callCount == 1 {
			expiryTime = time.Now().Add(-1 * time.Second) // Already expired
		}

		resp := map[string]string{
			"id":         "token-" + string(rune(callCount)),
			"project_id": "admin",
			"expires":    expiryTime.Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(func() { server.Close() })

	config := &Config{
		NBIURL:         server.URL,
		Username:       "admin",
		Password:       "secret",
		Project:        "admin",
		RequestTimeout: 5 * time.Second,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	ctx := context.Background()

	// First authentication (will return expired token)
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("First Authenticate() failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 server call, got %d", callCount)
	}

	// Wait a bit to ensure token is expired
	time.Sleep(100 * time.Millisecond)

	// Second authentication should detect expired token and refresh
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Second Authenticate() failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 server calls (token refresh), got %d", callCount)
	}
}

// TestHealth tests the health check functionality.
func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name: "healthy",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == testTokenPath {
					// Auth request
					if r.Method == http.MethodPost {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						resp := map[string]string{
							"id":         "token-123",
							"project_id": "admin",
							"expires":    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
						}
						_ = json.NewEncoder(w).Encode(resp)
					} else {
						// Health check
						w.WriteHeader(http.StatusOK)
					}
				}
			},
			wantErr: false,
		},
		{
			name: "unhealthy - auth failure",
			serverResp: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResp))
			t.Cleanup(func() { server.Close() })

			config := &Config{
				NBIURL:         server.URL,
				Username:       "admin",
				Password:       "secret",
				Project:        "admin",
				RequestTimeout: 5 * time.Second,
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}

			ctx := context.Background()
			err = client.Health(ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("Health() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Health() unexpected error: %v", err)
			}
		})
	}
}

// TestClose tests client cleanup.
func TestClose(t *testing.T) {
	config := &Config{
		NBIURL:         "https://osm.example.com:9999",
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 5 * time.Second,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Manually set a token to verify it's cleared
	client.mu.Lock()
	client.token = "test-token"
	client.tokenExpiry = time.Now().Add(1 * time.Hour)
	client.mu.Unlock()

	// Close the client
	err = client.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	// Verify token was cleared
	client.mu.RLock()
	if client.token != "" {
		t.Error("Token was not cleared after Close()")
	}
	if !client.tokenExpiry.IsZero() {
		t.Error("Token expiry was not cleared after Close()")
	}
	client.mu.RUnlock()
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
