package osm

import (
	"context"
	"testing"
	"time"
)

const (
	testAdminUser = "admin"
)

// TestNewPlugin tests the creation of a new OSM plugin.
func TestNewPlugin(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			},
			wantErr: false,
		},
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: true,
			errMsg:  "nbiUrl is required",
		},
		{
			name: "missing nbiUrl",
			config: &Config{
				Username: "admin",
				Password: "secret",
			},
			wantErr: true,
			errMsg:  "nbiUrl is required",
		},
		{
			name: "missing username",
			config: &Config{
				NBIURL:   "https://osm.example.com:9999",
				Password: "secret",
			},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name: "missing password",
			config: &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
			},
			wantErr: true,
			errMsg:  "password is required",
		},
		{
			name: "config with defaults applied",
			config: &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				// Project not set - should default to "admin"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, err := NewPlugin(tt.config)

			if tt.wantErr {
				validateExpectedError(t, err, tt.errMsg)
				return
			}

			validatePluginCreation(t, plugin, err)
		})
	}
}

// validateExpectedError validates that an error occurred with the expected message.
func validateExpectedError(t *testing.T, err error, errMsg string) {
	t.Helper()

	if err == nil {
		t.Error("NewPlugin() expected error but got none")
		return
	}
	if errMsg != "" && err.Error() != errMsg {
		t.Errorf("NewPlugin() error = %v, want %v", err.Error(), errMsg)
	}
}

// validatePluginCreation validates successful plugin creation and configuration.
func validatePluginCreation(t *testing.T, plugin *Plugin, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("NewPlugin() unexpected error: %v", err)
		return
	}

	if plugin == nil {
		t.Error("NewPlugin() returned nil plugin")
		return
	}

	validatePluginMetadata(t, plugin)
	validatePluginDefaults(t, plugin)
}

// validatePluginMetadata validates plugin metadata fields.
func validatePluginMetadata(t *testing.T, plugin *Plugin) {
	t.Helper()

	if plugin.Name() != "osm" {
		t.Errorf("Name() = %v, want %v", plugin.Name(), "osm")
	}

	if plugin.Version() == "" {
		t.Error("Version() returned empty string")
	}

	capabilities := plugin.Capabilities()
	if len(capabilities) == 0 {
		t.Error("Capabilities() returned empty list")
	}
}

// validatePluginDefaults validates that default configuration values were applied.
func validatePluginDefaults(t *testing.T, plugin *Plugin) {
	t.Helper()

	if plugin.config.Project == "" {
		t.Error("Project default was not applied")
	}
	if plugin.config.RequestTimeout == 0 {
		t.Error("RequestTimeout default was not applied")
	}
	if plugin.config.InventorySyncInterval == 0 {
		t.Error("InventorySyncInterval default was not applied")
	}
}

// TestPluginMetadata tests plugin metadata methods.
func TestPluginMetadata(t *testing.T) {
	config := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
	}

	plugin, err := NewPlugin(config)
	if err != nil {
		t.Fatalf("NewPlugin() failed: %v", err)
	}

	// Test Name
	if name := plugin.Name(); name != "osm" {
		t.Errorf("Name() = %v, want %v", name, "osm")
	}

	// Test Version
	if version := plugin.Version(); version == "" {
		t.Error("Version() returned empty string")
	}

	// Test Capabilities
	capabilities := plugin.Capabilities()
	expectedCaps := []string{
		"inventory-sync",
		"workflow-orchestration",
		"service-modeling",
		"package-management",
		"deployment-lifecycle",
		"scaling",
	}

	if len(capabilities) != len(expectedCaps) {
		t.Errorf("Capabilities() returned %d capabilities, want %d", len(capabilities), len(expectedCaps))
	}

	// Verify each expected capability is present
	capMap := make(map[string]bool)
	for _, cap := range capabilities {
		capMap[cap] = true
	}

	for _, expected := range expectedCaps {
		if !capMap[expected] {
			t.Errorf("Capabilities() missing expected capability: %s", expected)
		}
	}
}

// TestPluginCapabilityChecks tests capability check methods.
func TestPluginCapabilityChecks(t *testing.T) {
	config := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
	}

	plugin, err := NewPlugin(config)
	if err != nil {
		t.Fatalf("NewPlugin() failed: %v", err)
	}

	tests := []struct {
		name     string
		check    func() bool
		expected bool
	}{
		{
			name:     "SupportsWorkflows",
			check:    plugin.SupportsWorkflows,
			expected: true,
		},
		{
			name:     "SupportsServiceModeling",
			check:    plugin.SupportsServiceModeling,
			expected: true,
		},
		{
			name:     "SupportsPolicyManagement",
			check:    plugin.SupportsPolicyManagement,
			expected: false, // OSM doesn't have native policy management
		},
		{
			name:     "SupportsRollback",
			check:    plugin.SupportsRollback,
			expected: false, // OSM doesn't support NS rollback
		},
		{
			name:     "SupportsScaling",
			check:    plugin.SupportsScaling,
			expected: true,
		},
		{
			name:     "SupportsGitOps",
			check:    plugin.SupportsGitOps,
			expected: false, // OSM doesn't support GitOps
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.check()
			if result != tt.expected {
				t.Errorf("%s() = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

// TestPluginLifecycle tests plugin initialization and shutdown.
func TestPluginLifecycle(t *testing.T) {
	config := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
		// Disable inventory sync for testing
		EnableInventorySync: false,
	}

	plugin, err := NewPlugin(config)
	if err != nil {
		t.Fatalf("NewPlugin() failed: %v", err)
	}

	// Test Health before initialization (should fail)
	ctx := context.Background()
	err = plugin.Health(ctx)
	if err == nil {
		t.Error("Health() should fail before initialization")
	}

	// Note: We cannot test Initialize() without a real OSM instance
	// In a real test environment, you would use a mock OSM server

	// Test Close before initialization (should not error)
	err = plugin.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	// Test LastSyncTime
	syncTime := plugin.LastSyncTime()
	if !syncTime.IsZero() {
		t.Error("LastSyncTime() should be zero before any sync")
	}
}

// TestDefaultConfig tests the DefaultConfig function.
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Verify all defaults are set
	tests := []struct {
		name  string
		value interface{}
		want  interface{}
	}{
		{"Project", config.Project, "admin"},
		{"RequestTimeout", config.RequestTimeout, 30 * time.Second},
		{"InventorySyncInterval", config.InventorySyncInterval, 5 * time.Minute},
		{"LCMPollingInterval", config.LCMPollingInterval, 10 * time.Second},
		{"MaxRetries", config.MaxRetries, 3},
		{"RetryDelay", config.RetryDelay, 1 * time.Second},
		{"RetryMaxDelay", config.RetryMaxDelay, 30 * time.Second},
		{"RetryMultiplier", config.RetryMultiplier, 2.0},
		{"EnableInventorySync", config.EnableInventorySync, true},
		{"EnableEventPublish", config.EnableEventPublish, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("DefaultConfig().%s = %v, want %v", tt.name, tt.value, tt.want)
			}
		})
	}
}

// TestMapOSMStatus tests the OSM status mapping function.
func TestMapOSMStatus(t *testing.T) {
	config := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
	}

	plugin, err := NewPlugin(config)
	if err != nil {
		t.Fatalf("NewPlugin() failed: %v", err)
	}

	tests := []struct {
		osmStatus  string
		wantStatus string
	}{
		{"init", "BUILDING"},
		{"building", "BUILDING"},
		{"running", "ACTIVE"},
		{"scaling", "SCALING"},
		{"healing", "HEALING"},
		{"terminating", "DELETING"},
		{"terminated", "DELETED"},
		{"failed", "ERROR"},
		{"error", "ERROR"},
		{"unknown-status", "UNKNOWN"},
		{"", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.osmStatus, func(t *testing.T) {
			result := plugin.mapOSMStatus(tt.osmStatus)
			if result != tt.wantStatus {
				t.Errorf("mapOSMStatus(%s) = %v, want %v", tt.osmStatus, result, tt.wantStatus)
			}
		})
	}
}

// TestTransformVIMAccount tests the VIM account transformation function.
func TestTransformVIMAccount(t *testing.T) {
	pool := &ResourcePool{
		ID:          "pool-123",
		Name:        "edge-pool-1",
		Description: "Edge computing pool",
		Location:    "Dallas, TX",
		Extensions: map[string]interface{}{
			"region": "us-south",
			"zone":   "dallas-1",
		},
	}

	vim := TransformVIMAccount(
		pool,
		"openstack",
		"https://openstack.example.com:5000/v3",
		"admin",
		"secret",
	)

	verifyVIMBasicFields(t, vim, pool)
	verifyVIMConfig(t, vim, pool)
}

// verifyVIMBasicFields checks basic VIM account fields.
func verifyVIMBasicFields(t *testing.T, vim *VIMAccount, pool *ResourcePool) {
	t.Helper()
	if vim.ID != pool.ID {
		t.Errorf("VIM ID = %v, want %v", vim.ID, pool.ID)
	}
	if vim.Name != pool.Name {
		t.Errorf("VIM Name = %v, want %v", vim.Name, pool.Name)
	}
	if vim.Description != pool.Description {
		t.Errorf("VIM Description = %v, want %v", vim.Description, pool.Description)
	}
	if vim.VIMType != "openstack" {
		t.Errorf("VIM Type = %v, want %v", vim.VIMType, "openstack")
	}
	if vim.VIMURL != "https://openstack.example.com:5000/v3" {
		t.Errorf("VIM URL = %v, want expected URL", vim.VIMURL)
	}
	if vim.VIMUser != testAdminUser {
		t.Errorf("VIM User = %v, want %v", vim.VIMUser, testAdminUser)
	}
	if vim.VIMPassword != "secret" {
		t.Errorf("VIM Password = %v, want %v", vim.VIMPassword, "secret")
	}
}

// verifyVIMConfig checks VIM config fields.
func verifyVIMConfig(t *testing.T, vim *VIMAccount, pool *ResourcePool) {
	t.Helper()
	if vim.Config == nil {
		t.Fatal("VIM Config is nil")
	}
	if vim.Config["region"] != "us-south" {
		t.Errorf("VIM Config[region] = %v, want %v", vim.Config["region"], "us-south")
	}
	if vim.Config["zone"] != "dallas-1" {
		t.Errorf("VIM Config[zone] = %v, want %v", vim.Config["zone"], "dallas-1")
	}
	if vim.Config["location"] != pool.Location {
		t.Errorf("VIM Config[location] = %v, want %v", vim.Config["location"], pool.Location)
	}
}

// TestTransformVIMAccountWithMinimalData tests transformation with minimal data.
func TestTransformVIMAccountWithMinimalData(t *testing.T) {
	pool := &ResourcePool{
		ID:   "pool-456",
		Name: "minimal-pool",
	}

	vim := TransformVIMAccount(
		pool,
		"kubernetes",
		"https://k8s.example.com:6443",
		"",
		"",
	)

	if vim.ID != pool.ID {
		t.Errorf("VIM ID = %v, want %v", vim.ID, pool.ID)
	}
	if vim.Name != pool.Name {
		t.Errorf("VIM Name = %v, want %v", vim.Name, pool.Name)
	}
	if vim.VIMType != "kubernetes" {
		t.Errorf("VIM Type = %v, want %v", vim.VIMType, "kubernetes")
	}

	// Config should still be initialized (even if empty)
	if vim.Config == nil {
		t.Error("VIM Config should be initialized even with minimal data")
	}
}
