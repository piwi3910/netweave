package osm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/smo"
)

// TestSyncInfrastructureInventory tests inventory synchronization.
func TestSyncInfrastructureInventory(t *testing.T) {
	tests := []struct {
		name       string
		inventory  *smo.InfrastructureInventory
		serverResp func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
	}{
		{
			name: "successful sync with resource pools",
			inventory: &smo.InfrastructureInventory{
				ResourcePools: []smo.ResourcePool{
					{
						ID:       "pool-1",
						Name:     "openstack-pool",
						Location: "dc-1",
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
			inventory: &smo.InfrastructureInventory{
				ResourcePools: []smo.ResourcePool{},
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

			validateVIMAccountFields(t, vim, tt.pool, tt.wantName, tt.wantType, tt.wantURL, tt.wantUser, tt.wantPass)
			validateVIMAccountConfig(t, vim, tt.pool)
		})
	}
}

// validateVIMAccountFields validates the main fields of a VIM account.
func validateVIMAccountFields(t *testing.T, vim *VIMAccount, pool *ResourcePool, wantName, wantType, wantURL, wantUser, wantPass string) {
	t.Helper()

	if vim.ID != pool.ID {
		t.Errorf("VIM ID = %v, want %v", vim.ID, pool.ID)
	}
	if vim.Name != wantName {
		t.Errorf("VIM Name = %v, want %v", vim.Name, wantName)
	}
	if vim.VIMType != wantType {
		t.Errorf("VIM Type = %v, want %v", vim.VIMType, wantType)
	}
	if vim.VIMURL != wantURL {
		t.Errorf("VIM URL = %v, want %v", vim.VIMURL, wantURL)
	}
	if vim.VIMUser != wantUser {
		t.Errorf("VIM User = %v, want %v", vim.VIMUser, wantUser)
	}
	if vim.VIMPassword != wantPass {
		t.Errorf("VIM Password = %v, want %v", vim.VIMPassword, wantPass)
	}
}

// validateVIMAccountConfig validates the configuration and extensions of a VIM account.
func validateVIMAccountConfig(t *testing.T, vim *VIMAccount, pool *ResourcePool) {
	t.Helper()

	if vim.Config == nil {
		t.Fatal("VIM Config should not be nil")
	}

	// Verify extensions were copied
	if pool.Extensions != nil {
		for k, v := range pool.Extensions {
			if vim.Config[k] != v {
				t.Errorf("VIM Config[%s] = %v, want %v", k, vim.Config[k], v)
			}
		}
	}

	// Verify location was added if present
	if pool.Location != "" {
		if vim.Config["location"] != pool.Location {
			t.Errorf("VIM Config[location] = %v, want %v", vim.Config["location"], pool.Location)
		}
	}
}

// TestPublishInfrastructureEvent tests event publishing.
func TestPublishInfrastructureEvent(t *testing.T) {
	tests := []struct {
		name               string
		event              *smo.InfrastructureEvent
		enableEventPublish bool
		wantErr            bool
	}{
		{
			name: "valid event with publishing enabled",
			event: &smo.InfrastructureEvent{
				EventID:      "event-1",
				EventType:    "ResourceCreated",
				ResourceType: "ResourcePool",
				ResourceID:   "pool-123",
				Timestamp:    time.Now(),
			},
			enableEventPublish: true,
			wantErr:            false,
		},
		{
			name: "valid event with publishing disabled",
			event: &smo.InfrastructureEvent{
				EventID:      "event-2",
				EventType:    "ResourceDeleted",
				ResourceType: "Resource",
				ResourceID:   "resource-456",
				Timestamp:    time.Now(),
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
			handleMockAuth(w)
		case r.URL.Path == "/osm/admin/v1/tokens" && r.Method == http.MethodGet:
			handleMockHealthCheck(w)
		case r.URL.Path == "/osm/admin/v1/vim_accounts" && r.Method == http.MethodPost:
			handleMockCreateVIM(w, r)
		case r.URL.Path == "/osm/admin/v1/vim_accounts" && r.Method == http.MethodGet:
			handleMockListVIM(w)
		default:
			handleMockNotFound(w, r)
		}
	}
}

// handleMockAuth handles mock authentication requests.
func handleMockAuth(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":         "test-token",
		"project_id": "admin",
		"expires":    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
}

// handleMockHealthCheck handles mock health check requests.
func handleMockHealthCheck(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}

// handleMockCreateVIM handles mock VIM account creation.
func handleMockCreateVIM(w http.ResponseWriter, r *http.Request) {
	var vim VIMAccount
	if err := json.NewDecoder(r.Body).Decode(&vim); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	vim.ID = "vim-" + vim.Name
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(vim)
}

// handleMockListVIM handles mock VIM account listing.
func handleMockListVIM(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode([]*VIMAccount{})
}

// handleMockNotFound handles mock 404 responses.
func handleMockNotFound(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && len(r.URL.Path) > len("/osm/admin/v1/vim_accounts/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNotFound)
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
