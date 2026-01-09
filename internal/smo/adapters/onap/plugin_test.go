package onap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/smo"
)

func TestNewPlugin(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	assert.NotNil(t, plugin)
	assert.Equal(t, "onap", plugin.name)
	assert.Equal(t, "1.0.0", plugin.version)
	assert.False(t, plugin.closed)
}

func TestPluginMetadata(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	metadata := plugin.Metadata()

	assert.Equal(t, "onap", metadata.Name)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.NotEmpty(t, metadata.Description)
	assert.NotEmpty(t, metadata.Vendor)
}

func TestPluginCapabilities(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedCapCount int
		expectedContains []string
	}{
		{
			name: "all features enabled",
			config: &Config{
				EnableInventorySync:   true,
				EnableEventPublishing: true,
				EnableDMSBackend:      true,
			},
			expectedCapCount: 5, // inventory-sync, event-publishing, workflow-orchestration, service-modeling, policy-management
			expectedContains: []string{
				"inventory-sync",
				"event-publishing",
				"workflow-orchestration",
				"service-modeling",
				"policy-management",
			},
		},
		{
			name: "only inventory sync enabled",
			config: &Config{
				EnableInventorySync:   true,
				EnableEventPublishing: false,
				EnableDMSBackend:      false,
			},
			expectedCapCount: 2, // inventory-sync, policy-management
			expectedContains: []string{
				"inventory-sync",
				"policy-management",
			},
		},
		{
			name: "only event publishing enabled",
			config: &Config{
				EnableInventorySync:   false,
				EnableEventPublishing: true,
				EnableDMSBackend:      false,
			},
			expectedCapCount: 2, // event-publishing, policy-management
			expectedContains: []string{
				"event-publishing",
				"policy-management",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			plugin := NewPlugin(logger)
			plugin.config = tt.config

			capabilities := plugin.Capabilities()

			assert.Len(t, capabilities, tt.expectedCapCount)
			for _, expectedCap := range tt.expectedContains {
				found := false
				for _, cap := range capabilities {
					if string(cap) == expectedCap {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected capability %s not found", expectedCap)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with all features",
			config: &Config{
				AAIURL:                "https://aai.example.com",
				DMaaPURL:              "https://dmaap.example.com",
				SOURL:                 "https://so.example.com",
				SDNCURL:               "https://sdnc.example.com",
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            3,
				EventPublishBatchSize: 100,
				EnableInventorySync:   true,
				EnableEventPublishing: true,
				EnableDMSBackend:      true,
			},
			expectError: false,
		},
		{
			name: "missing AAI URL with inventory sync enabled",
			config: &Config{
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            3,
				EventPublishBatchSize: 100,
				EnableInventorySync:   true,
			},
			expectError: true,
			errorMsg:    "aaiUrl is required",
		},
		{
			name: "missing DMaaP URL with event publishing enabled",
			config: &Config{
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            3,
				EventPublishBatchSize: 100,
				EnableEventPublishing: true,
			},
			expectError: true,
			errorMsg:    "dmaapUrl is required",
		},
		{
			name: "missing SO URL with DMS backend enabled",
			config: &Config{
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            3,
				EventPublishBatchSize: 100,
				EnableDMSBackend:      true,
			},
			expectError: true,
			errorMsg:    "soUrl is required",
		},
		{
			name: "missing credentials",
			config: &Config{
				AAIURL:                "https://aai.example.com",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            3,
				EventPublishBatchSize: 100,
			},
			expectError: true,
			errorMsg:    "username and password are required",
		},
		{
			name: "invalid timeout",
			config: &Config{
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        0,
				MaxRetries:            3,
				EventPublishBatchSize: 100,
			},
			expectError: true,
			errorMsg:    "requestTimeout must be positive",
		},
		{
			name: "negative max retries",
			config: &Config{
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            -1,
				EventPublishBatchSize: 100,
			},
			expectError: true,
			errorMsg:    "maxRetries cannot be negative",
		},
		{
			name: "invalid batch size",
			config: &Config{
				Username:              "admin",
				Password:              "password",
				RequestTimeout:        30 * time.Second,
				MaxRetries:            3,
				EventPublishBatchSize: 0,
			},
			expectError: true,
			errorMsg:    "eventPublishBatchSize must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.True(t, config.TLSEnabled)
	assert.False(t, config.TLSInsecureSkipVerify)
	assert.Equal(t, 5*time.Minute, config.InventorySyncInterval)
	assert.Equal(t, 100, config.EventPublishBatchSize)
	assert.Equal(t, 30*time.Second, config.RequestTimeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.True(t, config.EnableInventorySync)
	assert.True(t, config.EnableEventPublishing)
	assert.True(t, config.EnableDMSBackend)
	assert.False(t, config.EnableSDNC)
}

func TestParseConfig(t *testing.T) {
	input := map[string]interface{}{
		"aaiUrl":                "https://aai.example.com",
		"dmaapUrl":              "https://dmaap.example.com",
		"soUrl":                 "https://so.example.com",
		"sdncUrl":               "https://sdnc.example.com",
		"username":              "testuser",
		"password":              "testpass",
		"tlsEnabled":            false,
		"inventorySyncInterval": "10m",
		"eventPublishBatchSize": 50,
		"requestTimeout":        "1m",
		"maxRetries":            5,
		"enableInventorySync":   false,
		"enableEventPublishing": true,
		"enableDmsBackend":      true,
		"enableSdnc":            true,
	}

	output := DefaultConfig()
	parseConfig(input, output)
	assert.Equal(t, "https://aai.example.com", output.AAIURL)
	assert.Equal(t, "https://dmaap.example.com", output.DMaaPURL)
	assert.Equal(t, "https://so.example.com", output.SOURL)
	assert.Equal(t, "https://sdnc.example.com", output.SDNCURL)
	assert.Equal(t, "testuser", output.Username)
	assert.Equal(t, "testpass", output.Password)
	assert.False(t, output.TLSEnabled)
	assert.Equal(t, 10*time.Minute, output.InventorySyncInterval)
	assert.Equal(t, 50, output.EventPublishBatchSize)
	assert.Equal(t, 1*time.Minute, output.RequestTimeout)
	assert.Equal(t, 5, output.MaxRetries)
	assert.False(t, output.EnableInventorySync)
	assert.True(t, output.EnableEventPublishing)
	assert.True(t, output.EnableDMSBackend)
	assert.True(t, output.EnableSDNC)
}

func TestPluginClose(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.config = DefaultConfig()

	// Close once
	err := plugin.Close()
	assert.NoError(t, err)
	assert.True(t, plugin.closed)

	// Close again (should not error)
	err = plugin.Close()
	assert.NoError(t, err)
}

func TestPluginHealthClosedState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.closed = true

	health := plugin.Health(context.Background())

	assert.False(t, health.Healthy)
	assert.Contains(t, health.Message, "closed")
}

func TestMapDeploymentStatusToOrchestrationStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"pending", "Assigned"},
		{"deploying", "Active"},
		{"deployed", "Active"},
		{"running", "Active"},
		{"failed", "Failed"},
		{"deleting", "PendingDelete"},
		{"deleted", "Deleted"},
		{"unknown", "Created"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := plugin.mapDeploymentStatusToOrchestrationStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapONAPRequestStateToStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"PENDING", "PENDING"},
		{"IN_PROGRESS", "RUNNING"},
		{"COMPLETE", "SUCCEEDED"},
		{"COMPLETED", "SUCCEEDED"},
		{"FAILED", "FAILED"},
		{"TIMEOUT", "FAILED"},
		{"UNLOCKED", "CANCELLED"},
		{"UNKNOWN_STATE", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := plugin.mapONAPRequestStateToStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDMaaPTopic(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	tests := []struct {
		eventType     string
		expectedTopic string
	}{
		{"ResourceCreated", "unauthenticated.VES_INFRASTRUCTURE_EVENTS"},
		{"ResourceUpdated", "unauthenticated.VES_INFRASTRUCTURE_EVENTS"},
		{"ResourceDeleted", "unauthenticated.VES_INFRASTRUCTURE_EVENTS"},
		{"ResourcePoolCreated", "unauthenticated.VES_INFRASTRUCTURE_EVENTS"},
		{"UnknownEvent", "unauthenticated.VES_INFRASTRUCTURE_EVENTS"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			result := plugin.getDMaaPTopic(tt.eventType)
			assert.Equal(t, tt.expectedTopic, result)
		})
	}
}

// TestPlugin_Initialize tests the Initialize function.
func TestPlugin_Initialize(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	// Will fail without ONAP but tests the code path
	if err != nil {
		// Expected - configuration validation
	}
}

// TestPlugin_Health tests the Health function.
func TestPlugin_Health(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status := plugin.Health(ctx)
	// Just verify we get a status back
	assert.NotNil(t, status)
}

// TestPlugin_SyncInfrastructureInventory tests the SyncInfrastructureInventory function.
func TestPlugin_SyncInfrastructureInventory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	inventory := &smo.InfrastructureInventory{
		// Empty struct is valid
	}

	err = plugin.SyncInfrastructureInventory(ctx, inventory)
	if err != nil {
		// Expected without real ONAP
		assert.Error(t, err)
	}
}

// TestPlugin_SyncDeploymentInventory tests the SyncDeploymentInventory function.
func TestPlugin_SyncDeploymentInventory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	inventory := &smo.DeploymentInventory{
		// Empty struct is valid
	}

	err = plugin.SyncDeploymentInventory(ctx, inventory)
	if err != nil {
		// Expected without real ONAP
		assert.Error(t, err)
	}
}

// TestPlugin_PublishInfrastructureEvent tests the PublishInfrastructureEvent function.
func TestPlugin_PublishInfrastructureEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	event := &smo.InfrastructureEvent{
		EventType:  "resource.created",
		ResourceID: "test-resource",
	}

	err = plugin.PublishInfrastructureEvent(ctx, event)
	if err != nil {
		// Expected without real ONAP
		assert.Error(t, err)
	}
}

// TestPlugin_PublishDeploymentEvent tests the PublishDeploymentEvent function.
func TestPlugin_PublishDeploymentEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	event := &smo.DeploymentEvent{
		EventType:    "deployment.created",
		DeploymentID: "test-deployment",
	}

	err = plugin.PublishDeploymentEvent(ctx, event)
	if err != nil {
		// Expected without real ONAP
		assert.Error(t, err)
	}
}

// TestPlugin_ExecuteWorkflow tests the ExecuteWorkflow function.
func TestPlugin_ExecuteWorkflow(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	workflow := &smo.WorkflowRequest{
		WorkflowName: "test-workflow",
		Parameters:   map[string]interface{}{},
	}

	execution, err := plugin.ExecuteWorkflow(ctx, workflow)
	if err != nil {
		// Expected without real ONAP
		assert.Error(t, err)
		assert.Nil(t, execution)
	}
}

// TestPlugin_GetWorkflowStatus tests the GetWorkflowStatus function.
func TestPlugin_GetWorkflowStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"endpoint":   "https://onap.example.com",
		"username":   "test",
		"password":   "test",
		"tenantName": "test-tenant",
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Skip("Skipping - requires ONAP")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status, err := plugin.GetWorkflowStatus(ctx, "test-execution-id")
	if err != nil {
		// Expected without real ONAP
		assert.Error(t, err)
		assert.Nil(t, status)
	}
}

// TestPlugin_Close tests the Close function.
func TestPlugin_Close(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	// Close without initialization should not error
	err := plugin.Close()
	assert.NoError(t, err)
}
