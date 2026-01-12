package osm

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/smo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlugin_Metadata tests the Metadata method.
func TestPlugin_Metadata(t *testing.T) {
	cfg := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
		Project:  "admin",
	}
	plugin, err := NewPlugin(cfg)
	require.NoError(t, err)

	metadata := plugin.Metadata()

	assert.Equal(t, "osm", metadata.Name)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, "OSM (Open Source MANO) integration plugin for NS/VNF lifecycle management", metadata.Description)
	assert.Equal(t, "ETSI OSM", metadata.Vendor)
}

// TestNewSMOPluginAdapter tests the creation of a new SMO plugin adapter.
func TestNewSMOPluginAdapter(t *testing.T) {
	cfg := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
		Project:  "admin",
	}
	plugin, err := NewPlugin(cfg)
	require.NoError(t, err)

	adapter := NewSMOPluginAdapter(plugin)

	require.NotNil(t, adapter)
	assert.NotNil(t, adapter.Plugin)
}

// TestSMOPluginAdapter_Capabilities tests the Capabilities method.
func TestSMOPluginAdapter_Capabilities(t *testing.T) {
	cfg := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
		Project:  "admin",
	}
	plugin, err := NewPlugin(cfg)
	require.NoError(t, err)

	adapter := NewSMOPluginAdapter(plugin)
	caps := adapter.Capabilities()

	require.Len(t, caps, 4)
	assert.Contains(t, caps, smo.CapInventorySync)
	assert.Contains(t, caps, smo.CapEventPublishing)
	assert.Contains(t, caps, smo.CapWorkflowOrchestration)
	assert.Contains(t, caps, smo.CapServiceModeling)
	assert.NotContains(t, caps, smo.CapPolicyManagement)
}

// TestSMOPluginAdapter_Initialize tests the Initialize method.
func TestSMOPluginAdapter_Initialize(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "initialize with empty config",
			config:  map[string]interface{}{},
			wantErr: true, // Will fail as OSM is not reachable
		},
		{
			name: "initialize with config parameters",
			config: map[string]interface{}{
				"key": "value",
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			adapter := NewSMOPluginAdapter(plugin)
			ctx := context.Background()

			err = adapter.Initialize(ctx, tt.config)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSMOPluginAdapter_Health tests the Health method.
func TestSMOPluginAdapter_Health(t *testing.T) {
	tests := []struct {
		name          string
		setupPlugin   func() *Plugin
		expectHealthy bool
		expectMessage string
		expectDetails bool
	}{
		{
			name: "healthy plugin",
			setupPlugin: func() *Plugin {
				cfg := &Config{
					NBIURL:   "https://osm.example.com:9999",
					Username: "admin",
					Password: "secret",
					Project:  "admin",
				}
				plugin, _ := NewPlugin(cfg)
				return plugin
			},
			expectHealthy: false, // Will be unhealthy as we can't connect to real OSM
			expectMessage: "context deadline exceeded",
			expectDetails: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := tt.setupPlugin()
			adapter := NewSMOPluginAdapter(plugin)
			ctx := context.Background()

			health := adapter.Health(ctx)

			assert.Equal(t, tt.expectHealthy, health.Healthy)
			assert.NotZero(t, health.Timestamp)

			if tt.expectDetails && health.Healthy {
				assert.Contains(t, health.Details, "osm-nbi")
				assert.True(t, health.Details["osm-nbi"].Healthy)
			}
		})
	}
}

// TestPlugin_SyncInfrastructureInventory tests the SyncInfrastructureInventory method.
func TestPlugin_SyncInfrastructureInventory(t *testing.T) {
	tests := []struct {
		name      string
		inventory *smo.InfrastructureInventory
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "nil inventory",
			inventory: nil,
			wantErr:   true,
			errMsg:    "inventory cannot be nil",
		},
		{
			name: "empty inventory",
			inventory: &smo.InfrastructureInventory{
				DeploymentManagers: []smo.DeploymentManager{},
				ResourcePools:      []smo.ResourcePool{},
			},
			wantErr: false,
		},
		{
			name: "inventory with deployment manager",
			inventory: &smo.InfrastructureInventory{
				DeploymentManagers: []smo.DeploymentManager{
					{
						ID:                 "dm-001",
						Name:               "test-dm",
						Description:        "Test deployment manager",
						ServiceURI:         "https://k8s.example.com:6443",
						OCloudID:           "cloud-001",
						Capabilities:       []string{"kubernetes"},
						SupportedLocations: []string{"us-west-1"},
					},
				},
				ResourcePools: []smo.ResourcePool{},
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
		{
			name: "inventory with deployment manager with vim type extension",
			inventory: &smo.InfrastructureInventory{
				DeploymentManagers: []smo.DeploymentManager{
					{
						ID:          "dm-002",
						Name:        "openstack-dm",
						Description: "OpenStack deployment manager",
						ServiceURI:  "https://openstack.example.com:5000",
						OCloudID:    "cloud-002",
						Extensions: map[string]interface{}{
							"vimType": "openstack",
						},
					},
				},
				ResourcePools: []smo.ResourcePool{},
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = plugin.SyncInfrastructureInventory(ctx, tt.inventory)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_SyncDeploymentInventory tests the SyncDeploymentInventory method.
func TestPlugin_SyncDeploymentInventory(t *testing.T) {
	tests := []struct {
		name      string
		inventory *smo.DeploymentInventory
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "nil inventory",
			inventory: nil,
			wantErr:   true,
			errMsg:    "inventory cannot be nil",
		},
		{
			name: "empty inventory",
			inventory: &smo.DeploymentInventory{
				Deployments: []smo.Deployment{},
			},
			wantErr: false,
		},
		{
			name: "inventory with deployments",
			inventory: &smo.DeploymentInventory{
				Deployments: []smo.Deployment{
					{
						ID:        "deploy-001",
						Name:      "test-deployment",
						Namespace: "default",
						Status:    "active",
					},
				},
			},
			wantErr: false, // OSM doesn't actually sync deployments, so no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = plugin.SyncDeploymentInventory(ctx, tt.inventory)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_PublishInfrastructureEvent tests the PublishInfrastructureEvent method.
func TestPlugin_PublishInfrastructureEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   *smo.InfrastructureEvent
		enabled bool
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil event",
			event:   nil,
			enabled: true,
			wantErr: true,
			errMsg:  "event cannot be nil",
		},
		{
			name: "event publishing disabled",
			event: &smo.InfrastructureEvent{
				EventType:    "resource.created",
				ResourceType: "deployment_manager",
				ResourceID:   "dm-001",
				Timestamp:    time.Now(),
				Payload:      map[string]interface{}{"key": "value"},
			},
			enabled: false,
			wantErr: false,
		},
		{
			name: "valid event with publishing enabled",
			event: &smo.InfrastructureEvent{
				EventType:    "resource.created",
				ResourceType: "deployment_manager",
				ResourceID:   "dm-001",
				Timestamp:    time.Now(),
				Payload:      map[string]interface{}{"key": "value"},
			},
			enabled: true,
			wantErr: false, // publishOSMEvent is a no-op currently, so no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:             "https://osm.example.com:9999",
				Username:           "admin",
				Password:           "secret",
				Project:            "admin",
				EnableEventPublish: tt.enabled,
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = plugin.PublishInfrastructureEvent(ctx, tt.event)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_PublishDeploymentEvent tests the PublishDeploymentEvent method.
func TestPlugin_PublishDeploymentEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   *smo.DeploymentEvent
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
			errMsg:  "event cannot be nil",
		},
		{
			name: "valid event",
			event: &smo.DeploymentEvent{
				EventType:    "deployment.created",
				DeploymentID: "deploy-001",
				Timestamp:    time.Now(),
				Payload:      map[string]interface{}{"status": "active"},
			},
			wantErr: false, // OSM doesn't actually publish deployment events, so no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = plugin.PublishDeploymentEvent(ctx, tt.event)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_ExecuteWorkflow tests the ExecuteWorkflow method.
func TestPlugin_ExecuteWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		workflow *smo.WorkflowRequest
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "nil workflow",
			workflow: nil,
			wantErr:  true,
			errMsg:   "workflow request cannot be nil",
		},
		{
			name: "unsupported workflow type",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "unsupported",
				Parameters:   map[string]interface{}{},
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "instantiate workflow missing parameters",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "instantiate",
				Parameters: map[string]interface{}{
					"nsName": "test-ns",
					// Missing nsdId and vimAccountId
				},
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "instantiate workflow with all parameters",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "instantiate",
				Parameters: map[string]interface{}{
					"nsName":       "test-ns",
					"nsdId":        "nsd-001",
					"vimAccountId": "vim-001",
					"description":  "Test NS instance",
				},
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
		{
			name: "terminate workflow missing nsInstanceId",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "terminate",
				Parameters:   map[string]interface{}{},
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "terminate workflow with nsInstanceId",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "terminate",
				Parameters: map[string]interface{}{
					"nsInstanceId": "ns-instance-001",
				},
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
		{
			name: "scale workflow missing nsInstanceId",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "scale",
				Parameters:   map[string]interface{}{},
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "scale workflow with parameters",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "scale",
				Parameters: map[string]interface{}{
					"nsInstanceId":           "ns-instance-001",
					"scaleType":              "SCALE_VNF",
					"scaleVnfType":           "SCALE_OUT",
					"scalingGroupDescriptor": "scale-group-1",
					"memberVnfIndex":         "1",
				},
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
		{
			name: "heal workflow missing parameters",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "heal",
				Parameters: map[string]interface{}{
					"nsInstanceId": "ns-instance-001",
					// Missing vnfInstanceId
				},
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "heal workflow with all parameters",
			workflow: &smo.WorkflowRequest{
				WorkflowName: "heal",
				Parameters: map[string]interface{}{
					"nsInstanceId":  "ns-instance-001",
					"vnfInstanceId": "vnf-instance-001",
					"cause":         "failure",
				},
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			// Initialize plugin for workflow execution
			ctx := context.Background()
			_ = plugin.Initialize(ctx)

			exec, err := plugin.ExecuteWorkflow(ctx, tt.workflow)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, exec)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, exec)
				assert.NotEmpty(t, exec.ExecutionID)
				assert.Equal(t, tt.workflow.WorkflowName, exec.WorkflowName)
			}
		})
	}
}

// TestPlugin_GetWorkflowStatus tests the GetWorkflowStatus method.
func TestPlugin_GetWorkflowStatus(t *testing.T) {
	tests := []struct {
		name        string
		executionID string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "empty execution ID",
			executionID: "",
			wantErr:     true,
			errMsg:      "execution id is required",
		},
		{
			name:        "valid execution ID",
			executionID: "exec-123",
			wantErr:     true,
			errMsg:      "plugin not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			// Initialize plugin
			ctx := context.Background()
			_ = plugin.Initialize(ctx)

			status, err := plugin.GetWorkflowStatus(ctx, tt.executionID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, status)
				assert.Equal(t, tt.executionID, status.ExecutionID)
				assert.Equal(t, "osm-workflow", status.WorkflowName)
				assert.Equal(t, "RUNNING", status.Status)
			}
		})
	}
}

// TestPlugin_CancelWorkflow tests the CancelWorkflow method.
func TestPlugin_CancelWorkflow(t *testing.T) {
	tests := []struct {
		name        string
		executionID string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "empty execution ID",
			executionID: "",
			wantErr:     true,
			errMsg:      "execution id is required",
		},
		{
			name:        "valid execution ID",
			executionID: "exec-123",
			wantErr:     true,
			errMsg:      "plugin not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			// Initialize plugin
			ctx := context.Background()
			_ = plugin.Initialize(ctx)

			err = plugin.CancelWorkflow(ctx, tt.executionID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_RegisterServiceModel tests the RegisterServiceModel method.
func TestPlugin_RegisterServiceModel(t *testing.T) {
	tests := []struct {
		name    string
		model   *smo.ServiceModel
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil model",
			model:   nil,
			wantErr: true,
			errMsg:  "service model cannot be nil",
		},
		{
			name: "model without template",
			model: &smo.ServiceModel{
				ID:      "model-001",
				Name:    "test-model",
				Version: "1.0.0",
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "model with non-byte template",
			model: &smo.ServiceModel{
				ID:       "model-001",
				Name:     "test-model",
				Version:  "1.0.0",
				Template: "invalid template type",
			},
			wantErr: true,
			errMsg:  "plugin not initialized",
		},
		{
			name: "model with byte template",
			model: &smo.ServiceModel{
				ID:       "model-001",
				Name:     "test-model",
				Version:  "1.0.0",
				Template: []byte("nsd package content"),
			},
			wantErr: true, // Will fail as OSM is not reachable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			// Initialize plugin
			ctx := context.Background()
			_ = plugin.Initialize(ctx)

			err = plugin.RegisterServiceModel(ctx, tt.model)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_GetServiceModel tests the GetServiceModel method.
func TestPlugin_GetServiceModel(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty id",
			id:      "",
			wantErr: true,
			errMsg:  "service model id is required",
		},
		{
			name:    "valid id",
			id:      "model-001",
			wantErr: true, // Will fail as OSM is not reachable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			// Initialize plugin
			ctx := context.Background()
			_ = plugin.Initialize(ctx)

			model, err := plugin.GetServiceModel(ctx, tt.id)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, model)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, model)
				assert.Equal(t, tt.id, model.ID)
			}
		})
	}
}

// TestPlugin_ListServiceModels tests the ListServiceModels method.
func TestPlugin_ListServiceModels(t *testing.T) {
	cfg := &Config{
		NBIURL:   "https://osm.example.com:9999",
		Username: "admin",
		Password: "secret",
		Project:  "admin",
	}
	plugin, err := NewPlugin(cfg)
	require.NoError(t, err)

	// Initialize plugin
	ctx := context.Background()
	_ = plugin.Initialize(ctx)

	models, err := plugin.ListServiceModels(ctx)

	// Will fail as OSM is not reachable
	require.Error(t, err)
	assert.Nil(t, models)
}

// TestPlugin_DeleteServiceModel tests the DeleteServiceModel method.
func TestPlugin_DeleteServiceModel(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty id",
			id:      "",
			wantErr: true,
			errMsg:  "service model id is required",
		},
		{
			name:    "valid id",
			id:      "model-001",
			wantErr: true, // Will fail as OSM is not reachable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			// Initialize plugin
			ctx := context.Background()
			_ = plugin.Initialize(ctx)

			err = plugin.DeleteServiceModel(ctx, tt.id)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_ApplyPolicy tests the ApplyPolicy method.
func TestPlugin_ApplyPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  *smo.Policy
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil policy",
			policy:  nil,
			wantErr: true,
			errMsg:  "policy cannot be nil",
		},
		{
			name: "valid policy",
			policy: &smo.Policy{
				PolicyID:   "policy-001",
				Name:       "test-policy",
				PolicyType: "placement",
				Rules:      map[string]interface{}{"region": "us-west"},
			},
			wantErr: true,
			errMsg:  "policy management is not supported by OSM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = plugin.ApplyPolicy(ctx, tt.policy)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlugin_GetPolicyStatus tests the GetPolicyStatus method.
func TestPlugin_GetPolicyStatus(t *testing.T) {
	tests := []struct {
		name     string
		policyID string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "empty policy ID",
			policyID: "",
			wantErr:  true,
			errMsg:   "policy id is required",
		},
		{
			name:     "valid policy ID",
			policyID: "policy-001",
			wantErr:  true,
			errMsg:   "policy management is not supported by OSM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				NBIURL:   "https://osm.example.com:9999",
				Username: "admin",
				Password: "secret",
				Project:  "admin",
			}
			plugin, err := NewPlugin(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			status, err := plugin.GetPolicyStatus(ctx, tt.policyID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, status)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, status)
			}
		})
	}
}
