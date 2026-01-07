package osm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSyncInfrastructureInventory tests inventory synchronization.
func TestSyncInfrastructureInventory(t *testing.T) {
	tests := []struct {
		name       string
		inventory  *InfrastructureInventory
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name: "successful sync with new VIM accounts",
			inventory: &InfrastructureInventory{
				VIMAccounts: []*VIMAccount{
					{
						Name:          "openstack-vim",
						VIMType:       "openstack",
						VIMURL:        "https://openstack.example.com:5000/v3",
						VIMUser:       "admin",
						VIMPassword:   "secret",
						VIMTenantName: "admin",
					},
				},
			},
			serverResp: mockOSMServer(t),
			wantErr:    false,
		},
		{
			name:       "nil inventory",
			inventory:  nil,
			serverResp: mockOSMServer(t),
			wantErr:    true,
		},
		{
			name: "empty inventory",
			inventory: &InfrastructureInventory{
				VIMAccounts: []*VIMAccount{},
			},
			serverResp: mockOSMServer(t),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResp))
			t.Cleanup(func() { server.Close() })

			plugin := createTestPlugin(t, server.URL)
			ctx := context.Background()

			err := plugin.SyncInfrastructureInventory(ctx, tt.inventory)

			if tt.wantErr {
				if err == nil {
					t.Error("SyncInfrastructureInventory() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("SyncInfrastructureInventory() unexpected error: %v", err)
			}
		})
	}
}

// TestCreateVIMAccount tests VIM account creation.
func TestCreateVIMAccount(t *testing.T) {
	tests := []struct {
		name    string
		vim     *VIMAccount
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid VIM account",
			vim: &VIMAccount{
				Name:        "k8s-vim",
				VIMType:     "kubernetes",
				VIMURL:      "https://k8s.example.com:6443",
				VIMUser:     "admin",
				VIMPassword: "secret",
			},
			wantErr: false,
		},
		{
			name:    "nil VIM account",
			vim:     nil,
			wantErr: true,
			errMsg:  "vim cannot be nil",
		},
		{
			name: "missing VIM name",
			vim: &VIMAccount{
				VIMType:     "kubernetes",
				VIMURL:      "https://k8s.example.com:6443",
				VIMUser:     "admin",
				VIMPassword: "secret",
			},
			wantErr: true,
			errMsg:  "vim name is required",
		},
		{
			name: "missing VIM type",
			vim: &VIMAccount{
				Name:        "k8s-vim",
				VIMURL:      "https://k8s.example.com:6443",
				VIMUser:     "admin",
				VIMPassword: "secret",
			},
			wantErr: true,
			errMsg:  "vim type is required",
		},
		{
			name: "missing VIM URL",
			vim: &VIMAccount{
				Name:        "k8s-vim",
				VIMType:     "kubernetes",
				VIMUser:     "admin",
				VIMPassword: "secret",
			},
			wantErr: true,
			errMsg:  "vim url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(mockOSMServer(t))
			t.Cleanup(func() { server.Close() })

			plugin := createTestPlugin(t, server.URL)
			ctx := context.Background()

			err := plugin.CreateVIMAccount(ctx, tt.vim)

			if tt.wantErr {
				if err == nil {
					t.Error("CreateVIMAccount() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("CreateVIMAccount() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("CreateVIMAccount() unexpected error: %v", err)
			}
		})
	}
}

// TestTransformVIMAccount tests VIM account transformation.
func TestTransformVIMAccountVariations(t *testing.T) {
	tests := []struct {
		name     string
		pool     *ResourcePool
		vimType  string
		vimURL   string
		username string
		password string
		wantName string
		wantType string
		wantURL  string
		wantUser string
		wantPass string
	}{
		{
			name: "full resource pool",
			pool: &ResourcePool{
				ID:          "pool-1",
				Name:        "production-pool",
				Description: "Production infrastructure",
				Location:    "us-east-1",
				Extensions: map[string]interface{}{
					"region": "east",
					"az":     "us-east-1a",
				},
			},
			vimType:  "openstack",
			vimURL:   "https://os.example.com:5000",
			username: "admin",
			password: "secret",
			wantName: "production-pool",
			wantType: "openstack",
			wantURL:  "https://os.example.com:5000",
			wantUser: "admin",
			wantPass: "secret",
		},
		{
			name: "minimal resource pool",
			pool: &ResourcePool{
				ID:   "pool-2",
				Name: "dev-pool",
			},
			vimType:  "kubernetes",
			vimURL:   "https://k8s.dev.example.com",
			username: "",
			password: "",
			wantName: "dev-pool",
			wantType: "kubernetes",
			wantURL:  "https://k8s.dev.example.com",
			wantUser: "",
			wantPass: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vim := TransformVIMAccount(tt.pool, tt.vimType, tt.vimURL, tt.username, tt.password)

			if vim.ID != tt.pool.ID {
				t.Errorf("VIM ID = %v, want %v", vim.ID, tt.pool.ID)
			}
			if vim.Name != tt.wantName {
				t.Errorf("VIM Name = %v, want %v", vim.Name, tt.wantName)
			}
			if vim.VIMType != tt.wantType {
				t.Errorf("VIM Type = %v, want %v", vim.VIMType, tt.wantType)
			}
			if vim.VIMURL != tt.wantURL {
				t.Errorf("VIM URL = %v, want %v", vim.VIMURL, tt.wantURL)
			}
			if vim.VIMUser != tt.wantUser {
				t.Errorf("VIM User = %v, want %v", vim.VIMUser, tt.wantUser)
			}
			if vim.VIMPassword != tt.wantPass {
				t.Errorf("VIM Password = %v, want %v", vim.VIMPassword, tt.wantPass)
			}

			// Verify config was initialized
			if vim.Config == nil {
				t.Fatal("VIM Config should not be nil")
			}

			// Verify extensions were copied
			if tt.pool.Extensions != nil {
				for k, v := range tt.pool.Extensions {
					if vim.Config[k] != v {
						t.Errorf("VIM Config[%s] = %v, want %v", k, vim.Config[k], v)
					}
				}
			}

			// Verify location was added if present
			if tt.pool.Location != "" {
				if vim.Config["location"] != tt.pool.Location {
					t.Errorf("VIM Config[location] = %v, want %v", vim.Config["location"], tt.pool.Location)
				}
			}
		})
	}
}

// TestPublishInfrastructureEvent tests event publishing.
func TestPublishInfrastructureEvent(t *testing.T) {
	tests := []struct {
		name               string
		event              *InfrastructureEvent
		enableEventPublish bool
		wantErr            bool
	}{
		{
			name: "valid event with publishing enabled",
			event: &InfrastructureEvent{
				EventType:    "created",
				ResourceType: "resource_pool",
				ResourceID:   "pool-123",
				Timestamp:    time.Now().Format(time.RFC3339),
			},
			enableEventPublish: true,
			wantErr:            false,
		},
		{
			name: "valid event with publishing disabled",
			event: &InfrastructureEvent{
				EventType:    "deleted",
				ResourceType: "resource",
				ResourceID:   "resource-456",
				Timestamp:    time.Now().Format(time.RFC3339),
			},
			enableEventPublish: false,
			wantErr:            false, // Should succeed but do nothing
		},
		{
			name:               "nil event",
			event:              nil,
			enableEventPublish: true,
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(mockOSMServer(t))
			t.Cleanup(func() { server.Close() })

			plugin := createTestPlugin(t, server.URL)
			plugin.config.EnableEventPublish = tt.enableEventPublish

			ctx := context.Background()
			err := plugin.PublishInfrastructureEvent(ctx, tt.event)

			if tt.wantErr {
				if err == nil {
					t.Error("PublishInfrastructureEvent() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("PublishInfrastructureEvent() unexpected error: %v", err)
			}
		})
	}
}

// mockOSMServer creates a mock OSM server handler for testing.
func mockOSMServer(_ *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/osm/admin/v1/tokens" && r.Method == http.MethodPost:
			// Authentication
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":         "test-token",
				"project_id": "admin",
				"expires":    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})

		case r.URL.Path == "/osm/admin/v1/tokens" && r.Method == http.MethodGet:
			// Health check
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/osm/admin/v1/vim_accounts" && r.Method == http.MethodPost:
			// Create VIM account
			var vim VIMAccount
			if err := json.NewDecoder(r.Body).Decode(&vim); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			vim.ID = "vim-" + vim.Name
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(vim)

		case r.URL.Path == "/osm/admin/v1/vim_accounts" && r.Method == http.MethodGet:
			// List VIM accounts
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]*VIMAccount{})

		default:
			if r.Method == http.MethodGet && len(r.URL.Path) > len("/osm/admin/v1/vim_accounts/") {
				// Get specific VIM account
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// createTestPlugin creates a plugin configured for testing.
func createTestPlugin(t *testing.T, serverURL string) *Plugin {
	t.Helper()
	config := &Config{
		NBIURL:              serverURL,
		Username:            "admin",
		Password:            "secret",
		Project:             "admin",
		RequestTimeout:      5 * time.Second,
		EnableInventorySync: false, // Disable for testing
		EnableEventPublish:  true,
	}

	plugin, err := NewPlugin(config)
	if err != nil {
		t.Fatalf("Failed to create test plugin: %v", err)
	}

	// Authenticate for tests
	ctx := context.Background()
	if err := plugin.client.Authenticate(ctx); err != nil {
		t.Logf("Warning: Authentication failed in test setup: %v", err)
		// Don't fail - some tests may not need authentication
	}

	return plugin
}
