package onap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/smo"
)

// testConfig returns a valid Config for testing with TLS disabled.
func testConfig() *Config {
	return &Config{
		AAIURL:                "http://aai.example.com",
		DMaaPURL:              "http://dmaap.example.com",
		SOURL:                 "http://so.example.com",
		SDNCURL:               "http://sdnc.example.com",
		Username:              "admin",
		Password:              "password",
		TLSEnabled:            false,
		RequestTimeout:        5 * time.Second,
		MaxRetries:            0,
		EventPublishBatchSize: 100,
		EnableInventorySync:   true,
		EnableEventPublishing: true,
		EnableDMSBackend:      true,
		EnableSDNC:            true,
	}
}

// setupPluginWithServers creates a Plugin with real httptest servers wired to each client.
// It directly sets the unexported client fields, bypassing Initialize() to avoid the
// deadlock caused by Initialize holding a write lock and Health needing a read lock.
func setupPluginWithServers(t *testing.T, aaiH, dmaapH, soH, sdncH http.HandlerFunc) *Plugin {
	t.Helper()

	aaiServer := httptest.NewServer(aaiH)
	t.Cleanup(aaiServer.Close)
	dmaapServer := httptest.NewServer(dmaapH)
	t.Cleanup(dmaapServer.Close)
	soServer := httptest.NewServer(soH)
	t.Cleanup(soServer.Close)
	sdncServer := httptest.NewServer(sdncH)
	t.Cleanup(sdncServer.Close)

	logger := zaptest.NewLogger(t)
	cfg := &Config{
		AAIURL:                aaiServer.URL,
		DMaaPURL:              dmaapServer.URL,
		SOURL:                 soServer.URL,
		SDNCURL:               sdncServer.URL,
		Username:              "admin",
		Password:              "password",
		TLSEnabled:            false,
		RequestTimeout:        5 * time.Second,
		MaxRetries:            0,
		EventPublishBatchSize: 100,
		EnableInventorySync:   true,
		EnableEventPublishing: true,
		EnableDMSBackend:      true,
		EnableSDNC:            true,
	}

	plugin := NewPlugin(logger)
	plugin.Config = cfg

	// Create clients pointing at test servers, directly setting unexported fields
	aaiClient, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)
	plugin.aaiClient = aaiClient

	dmaapClient, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)
	plugin.dmaapClient = dmaapClient

	soClient, err := NewSOClient(cfg, logger)
	require.NoError(t, err)
	plugin.soClient = soClient

	sdncClient, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)
	plugin.sdncClient = sdncClient

	return plugin
}

func okH() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// ===== Northbound: SyncInfrastructureInventory =====

func TestSyncInfrastructureInventory_Success(t *testing.T) {
	aaiH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	plugin := setupPluginWithServers(t, aaiH, okH(), okH(), okH())
	defer func() { _ = plugin.Close() }()

	inventory := &smo.InfrastructureInventory{
		DeploymentManagers: []smo.DeploymentManager{
			{
				ID:         "dm-1",
				Name:       "DM One",
				OCloudID:   "cloud-1",
				ServiceURI: "https://dm1.example.com",
				Extensions: map[string]interface{}{
					"cloudType": "kubernetes",
				},
			},
			{
				ID:         "dm-2",
				Name:       "DM Two",
				OCloudID:   "cloud-2",
				ServiceURI: "https://dm2.example.com",
			},
		},
		ResourcePools: []smo.ResourcePool{
			{
				ID:          "pool-1",
				Name:        "Pool One",
				Description: "Test pool",
				OCloudID:    "cloud-1",
			},
		},
		Resources: []smo.Resource{
			{
				ID:             "res-1",
				ResourcePoolID: "pool-1",
				GlobalAssetID:  "asset-1",
				Description:    "Physical resource",
				Extensions: map[string]interface{}{
					"resourceKind":    "physical",
					"equipmentType":   "server",
					"equipmentVendor": "Dell",
					"equipmentModel":  "R740",
				},
			},
			{
				ID:             "res-2",
				ResourceTypeID: "rt-1",
				Description:    "Virtual resource",
				Extensions: map[string]interface{}{
					"resourceKind": "virtual",
				},
			},
			{
				ID:          "res-3",
				Description: "Resource without extensions (defaults to physical)",
			},
		},
	}

	err := plugin.SyncInfrastructureInventory(context.Background(), inventory)
	require.NoError(t, err)
}

func TestSyncInfrastructureInventory_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.SyncInfrastructureInventory(context.Background(), &smo.InfrastructureInventory{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestSyncInfrastructureInventory_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableInventorySync = false
	plugin.Config = cfg

	err := plugin.SyncInfrastructureInventory(context.Background(), &smo.InfrastructureInventory{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory sync is not enabled")
}

func TestSyncInfrastructureInventory_NoAAIClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	err := plugin.SyncInfrastructureInventory(context.Background(), &smo.InfrastructureInventory{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "A&AI client is not initialized")
}

func TestSyncInfrastructureInventory_AAIError(t *testing.T) {
	aaiH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	plugin := setupPluginWithServers(t, aaiH, okH(), okH(), okH())
	defer func() { _ = plugin.Close() }()

	inventory := &smo.InfrastructureInventory{
		DeploymentManagers: []smo.DeploymentManager{
			{ID: "dm-1", Name: "DM One", OCloudID: "cloud-1"},
		},
	}

	err := plugin.SyncInfrastructureInventory(context.Background(), inventory)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sync cloud region")
}

// ===== Northbound: SyncDeploymentInventory =====

func TestSyncDeploymentInventory_Success(t *testing.T) {
	aaiH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	plugin := setupPluginWithServers(t, aaiH, okH(), okH(), okH())
	defer func() { _ = plugin.Close() }()

	now := time.Now()
	inventory := &smo.DeploymentInventory{
		Deployments: []smo.Deployment{
			{
				ID:        "dep-1",
				Name:      "Deployment One",
				PackageID: "pkg-1",
				Status:    "deployed",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	err := plugin.SyncDeploymentInventory(context.Background(), inventory)
	require.NoError(t, err)
}

func TestSyncDeploymentInventory_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.SyncDeploymentInventory(context.Background(), &smo.DeploymentInventory{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestSyncDeploymentInventory_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableInventorySync = false
	plugin.Config = cfg

	err := plugin.SyncDeploymentInventory(context.Background(), &smo.DeploymentInventory{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory sync is not enabled")
}

func TestSyncDeploymentInventory_NoAAIClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	err := plugin.SyncDeploymentInventory(context.Background(), &smo.DeploymentInventory{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "A&AI client is not initialized")
}

func TestSyncDeploymentInventory_AAIError(t *testing.T) {
	aaiH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	plugin := setupPluginWithServers(t, aaiH, okH(), okH(), okH())
	defer func() { _ = plugin.Close() }()

	inventory := &smo.DeploymentInventory{
		Deployments: []smo.Deployment{
			{
				ID:        "dep-1",
				Name:      "Deployment",
				PackageID: "pkg-1",
				Status:    "deployed",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	err := plugin.SyncDeploymentInventory(context.Background(), inventory)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sync service instance")
}

// ===== Northbound: PublishInfrastructureEvent =====

func TestPublishInfrastructureEvent_Success(t *testing.T) {
	dmaapH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	plugin := setupPluginWithServers(t, okH(), dmaapH, okH(), okH())
	defer func() { _ = plugin.Close() }()

	event := &smo.InfrastructureEvent{
		EventID:      "evt-1",
		EventType:    "ResourceCreated",
		ResourceType: "compute",
		ResourceID:   "res-1",
		Timestamp:    time.Now(),
	}

	err := plugin.PublishInfrastructureEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestPublishInfrastructureEvent_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.PublishInfrastructureEvent(context.Background(), &smo.InfrastructureEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestPublishInfrastructureEvent_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableEventPublishing = false
	plugin.Config = cfg

	err := plugin.PublishInfrastructureEvent(context.Background(), &smo.InfrastructureEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event publishing is not enabled")
}

func TestPublishInfrastructureEvent_NoDMaaPClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	err := plugin.PublishInfrastructureEvent(context.Background(), &smo.InfrastructureEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMaaP client is not initialized")
}

func TestPublishInfrastructureEvent_DMaaPError(t *testing.T) {
	dmaapH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("dmaap error"))
	})

	plugin := setupPluginWithServers(t, okH(), dmaapH, okH(), okH())
	defer func() { _ = plugin.Close() }()

	event := &smo.InfrastructureEvent{
		EventID:      "evt-1",
		EventType:    "ResourceCreated",
		ResourceType: "compute",
		ResourceID:   "res-1",
		Timestamp:    time.Now(),
	}

	err := plugin.PublishInfrastructureEvent(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to publish event")
}

// ===== Northbound: PublishDeploymentEvent =====

func TestPublishDeploymentEvent_Success(t *testing.T) {
	dmaapH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	plugin := setupPluginWithServers(t, okH(), dmaapH, okH(), okH())
	defer func() { _ = plugin.Close() }()

	event := &smo.DeploymentEvent{
		EventID:      "evt-1",
		EventType:    "DeploymentStarted",
		DeploymentID: "dep-1",
		Timestamp:    time.Now(),
	}

	err := plugin.PublishDeploymentEvent(context.Background(), event)
	require.NoError(t, err)
}

func TestPublishDeploymentEvent_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.PublishDeploymentEvent(context.Background(), &smo.DeploymentEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestPublishDeploymentEvent_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableEventPublishing = false
	plugin.Config = cfg

	err := plugin.PublishDeploymentEvent(context.Background(), &smo.DeploymentEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event publishing is not enabled")
}

func TestPublishDeploymentEvent_NoDMaaPClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	err := plugin.PublishDeploymentEvent(context.Background(), &smo.DeploymentEvent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMaaP client is not initialized")
}

func TestPublishDeploymentEvent_DMaaPError(t *testing.T) {
	dmaapH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("dmaap error"))
	})

	plugin := setupPluginWithServers(t, okH(), dmaapH, okH(), okH())
	defer func() { _ = plugin.Close() }()

	event := &smo.DeploymentEvent{
		EventID:      "evt-1",
		EventType:    "DeploymentStarted",
		DeploymentID: "dep-1",
		Timestamp:    time.Now(),
	}

	err := plugin.PublishDeploymentEvent(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to publish event")
}

// ===== Plugin Health =====

func TestHealth_AllHealthy(t *testing.T) {
	plugin := setupPluginWithServers(t, okH(), okH(), okH(), okH())
	defer func() { _ = plugin.Close() }()

	health := plugin.Health(context.Background())
	assert.True(t, health.Healthy)
	assert.Contains(t, health.Message, "all ONAP components are healthy")
	assert.NotEmpty(t, health.Details)
}

func TestHealth_SomeUnhealthy(t *testing.T) {
	aaiH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	plugin := setupPluginWithServers(t, aaiH, okH(), okH(), okH())
	defer func() { _ = plugin.Close() }()

	health := plugin.Health(context.Background())
	assert.False(t, health.Healthy)
	assert.Contains(t, health.Message, "one or more ONAP components are unhealthy")
}

func TestHealth_NoClients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	health := plugin.Health(context.Background())
	assert.True(t, health.Healthy)
}

func TestHealth_ClosedPlugin(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true

	health := plugin.Health(context.Background())
	assert.False(t, health.Healthy)
	assert.Contains(t, health.Message, "plugin is closed")
}

// ===== Initialize =====

func TestInitialize_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true

	err := plugin.Initialize(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestInitialize_InvalidConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	config := map[string]interface{}{
		"enableInventorySync": true,
		// missing aaiUrl and credentials
	}

	err := plugin.Initialize(context.Background(), config)
	require.Error(t, err)
}

// ===== Southbound: ExecuteWorkflow =====

func TestExecuteWorkflow_Success(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"id":           "process-123",
			"definitionId": "def-1",
			"ended":        false,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	workflow := &smo.WorkflowRequest{
		WorkflowName: "test-workflow",
		Parameters: map[string]interface{}{
			"param1": "value1",
		},
	}

	execution, err := plugin.ExecuteWorkflow(context.Background(), workflow)
	require.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, "RUNNING", execution.Status)
	assert.Equal(t, "test-workflow", execution.WorkflowName)
	assert.NotEmpty(t, execution.ExecutionID)
}

func TestExecuteWorkflow_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	_, err := plugin.ExecuteWorkflow(context.Background(), &smo.WorkflowRequest{WorkflowName: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestExecuteWorkflow_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableDMSBackend = false
	plugin.Config = cfg

	_, err := plugin.ExecuteWorkflow(context.Background(), &smo.WorkflowRequest{WorkflowName: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMS backend mode is not enabled")
}

func TestExecuteWorkflow_NoSOClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	_, err := plugin.ExecuteWorkflow(context.Background(), &smo.WorkflowRequest{WorkflowName: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SO client is not initialized")
}

func TestExecuteWorkflow_SOError(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("SO error"))
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	_, err := plugin.ExecuteWorkflow(context.Background(), &smo.WorkflowRequest{
		WorkflowName: "test-workflow",
		Parameters:   map[string]interface{}{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute workflow")
}

// ===== Southbound: GetWorkflowStatus =====

func TestGetWorkflowStatus_Success(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		now := time.Now()
		resp := OrchestrationStatus{
			RequestID:         "req-1",
			ServiceInstanceID: "svc-1",
			ServiceName:       "test-service",
			RequestState:      "COMPLETE",
			StatusMessage:     "done",
			PercentProgress:   100,
			StartTime:         now,
			FinishTime:        &now,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	status, err := plugin.GetWorkflowStatus(context.Background(), "exec-123")
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "SUCCEEDED", status.Status)
	assert.Equal(t, 100, status.Progress)
}

func TestGetWorkflowStatus_Failed(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := OrchestrationStatus{
			RequestID:       "req-1",
			ServiceName:     "test-service",
			RequestState:    "FAILED",
			StatusMessage:   "deployment failed",
			PercentProgress: 50,
			StartTime:       time.Now(),
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	status, err := plugin.GetWorkflowStatus(context.Background(), "exec-123")
	require.NoError(t, err)
	assert.Equal(t, "FAILED", status.Status)
	assert.Equal(t, "deployment failed", status.Error)
}

func TestGetWorkflowStatus_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	_, err := plugin.GetWorkflowStatus(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestGetWorkflowStatus_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableDMSBackend = false
	plugin.Config = cfg

	_, err := plugin.GetWorkflowStatus(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMS backend mode is not enabled")
}

func TestGetWorkflowStatus_NoSOClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	_, err := plugin.GetWorkflowStatus(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SO client is not initialized")
}

// ===== Southbound: CancelWorkflow =====

func TestCancelWorkflow_Success(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	err := plugin.CancelWorkflow(context.Background(), "exec-123")
	require.NoError(t, err)
}

func TestCancelWorkflow_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.CancelWorkflow(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestCancelWorkflow_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableDMSBackend = false
	plugin.Config = cfg

	err := plugin.CancelWorkflow(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMS backend mode is not enabled")
}

func TestCancelWorkflow_NoSOClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	err := plugin.CancelWorkflow(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SO client is not initialized")
}

// ===== Southbound: RegisterServiceModel =====

func TestRegisterServiceModel_Success(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	model := &smo.ServiceModel{
		ID:       "model-1",
		Name:     "Test Model",
		Version:  "1.0",
		Category: "5G",
	}

	err := plugin.RegisterServiceModel(context.Background(), model)
	require.NoError(t, err)
}

func TestRegisterServiceModel_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.RegisterServiceModel(context.Background(), &smo.ServiceModel{ID: "m1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestRegisterServiceModel_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableDMSBackend = false
	plugin.Config = cfg

	err := plugin.RegisterServiceModel(context.Background(), &smo.ServiceModel{ID: "m1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMS backend mode is not enabled")
}

func TestRegisterServiceModel_NoSOClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	err := plugin.RegisterServiceModel(context.Background(), &smo.ServiceModel{ID: "m1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SO client is not initialized")
}

// ===== Southbound: GetServiceModel =====

func TestGetServiceModel_Success(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := ServiceModel{
			ModelInvariantID: "model-1",
			ModelVersionID:   "model-1-v1",
			ModelName:        "Test Model",
			ModelVersion:     "1.0",
			ModelType:        "service",
			ModelCategory:    "5G",
			Description:      "Test model",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	model, err := plugin.GetServiceModel(context.Background(), "model-1")
	require.NoError(t, err)
	assert.Equal(t, "model-1", model.ID)
	assert.Equal(t, "Test Model", model.Name)
}

func TestGetServiceModel_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	_, err := plugin.GetServiceModel(context.Background(), "model-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

func TestGetServiceModel_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	cfg := testConfig()
	cfg.EnableDMSBackend = false
	plugin.Config = cfg

	_, err := plugin.GetServiceModel(context.Background(), "model-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DMS backend mode is not enabled")
}

func TestGetServiceModel_NoSOClient(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	_, err := plugin.GetServiceModel(context.Background(), "model-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SO client is not initialized")
}

// ===== Southbound: ListServiceModels =====

func TestListServiceModels_Success(t *testing.T) {
	soH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := []*ServiceModel{
			{
				ModelInvariantID: "model-1",
				ModelVersionID:   "model-1-v1",
				ModelName:        "Model A",
				ModelVersion:     "1.0",
				ModelType:        "service",
			},
			{
				ModelInvariantID: "model-2",
				ModelVersionID:   "model-2-v1",
				ModelName:        "Model B",
				ModelVersion:     "2.0",
				ModelType:        "service",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	plugin := setupPluginWithServers(t, okH(), okH(), soH, okH())
	defer func() { _ = plugin.Close() }()

	models, err := plugin.ListServiceModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 2)
}

func TestListServiceModels_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	_, err := plugin.ListServiceModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

// ===== Southbound: ApplyPolicy =====

func TestApplyPolicy_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	policy := &smo.Policy{
		PolicyID:   "policy-1",
		PolicyType: "placement",
	}

	err := plugin.ApplyPolicy(context.Background(), policy)
	require.NoError(t, err)
}

func TestApplyPolicy_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	err := plugin.ApplyPolicy(context.Background(), &smo.Policy{PolicyID: "p1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

// ===== Southbound: GetPolicyStatus =====

func TestGetPolicyStatus_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	status, err := plugin.GetPolicyStatus(context.Background(), "policy-1")
	require.NoError(t, err)
	assert.Equal(t, "active", status.Status)
	assert.Equal(t, "policy-1", status.PolicyID)
	assert.NotNil(t, status.LastEnforced)
}

func TestGetPolicyStatus_Closed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Closed = true
	plugin.Config = testConfig()

	_, err := plugin.GetPolicyStatus(context.Background(), "policy-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is closed")
}

// ===== Plugin Close =====

func TestClose_WithClients(t *testing.T) {
	plugin := setupPluginWithServers(t, okH(), okH(), okH(), okH())

	err := plugin.Close()
	require.NoError(t, err)
	assert.True(t, plugin.Closed)

	// Close again should be no-op
	err = plugin.Close()
	require.NoError(t, err)
}

// ===== Capabilities =====

func TestCapabilities_NilConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)

	caps := plugin.Capabilities()
	assert.Empty(t, caps)
}

// ===== Config Parsing Edge Cases =====

func TestGetIntValue_Types(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{"int value", 42, 42},
		{"int64 value", int64(42), 42},
		{"float64 value", float64(42), 42},
		{"wrong type", "not-int", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]interface{}{"key": tt.input}
			result := getIntValue(m, "key", 100)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetIntValue_MissingKey(t *testing.T) {
	m := map[string]interface{}{}
	result := getIntValue(m, "missing", 99)
	assert.Equal(t, 99, result)
}

func TestGetDurationValue_Types(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected time.Duration
	}{
		{"string duration", "10s", 10 * time.Second},
		{"duration type", 10 * time.Second, 10 * time.Second},
		{"invalid string", "invalid", 30 * time.Second},
		{"wrong type", 12345, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]interface{}{"key": tt.input}
			result := getDurationValue(m, "key", 30*time.Second)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBoolValue_WrongType(t *testing.T) {
	m := map[string]interface{}{"key": "not-a-bool"}
	result := getBoolValue(m, "key", true)
	assert.True(t, result)
}

func TestGetStringValue_WrongType(t *testing.T) {
	m := map[string]interface{}{"key": 12345}
	result := getStringValue(m, "key", "default")
	assert.Equal(t, "default", result)
}

func TestParseTLSFields(t *testing.T) {
	cfg := DefaultConfig()
	input := map[string]interface{}{
		"tlsCertFile":           "/etc/cert.pem",
		"tlsKeyFile":            "/etc/key.pem",
		"tlsCAFile":             "/etc/ca.pem",
		"tlsInsecureSkipVerify": true,
	}
	ParseConfig(input, cfg)
	assert.Equal(t, "/etc/cert.pem", cfg.TLSCertFile)
	assert.Equal(t, "/etc/key.pem", cfg.TLSKeyFile)
	assert.Equal(t, "/etc/ca.pem", cfg.TLSCAFile)
	assert.True(t, cfg.TLSInsecureSkipVerify)
}

// ===== Helper Functions =====

func TestIsNilInterface(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{"nil interface", nil, true},
		{"nil pointer", (*Plugin)(nil), true},
		{"non-nil pointer", &Plugin{}, false},
		{"nil map", (map[string]string)(nil), true},
		{"nil slice", ([]string)(nil), true},
		{"non-nil string", "hello", false},
		{"int value", 42, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNilInterface(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformToAAIInventory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	inventory := &smo.InfrastructureInventory{
		DeploymentManagers: []smo.DeploymentManager{
			{
				ID:         "dm-1",
				Name:       "DM One",
				OCloudID:   "cloud-1",
				ServiceURI: "https://dm1.example.com",
				Extensions: map[string]interface{}{
					"cloudType": "kubernetes",
				},
			},
		},
		ResourcePools: []smo.ResourcePool{
			{
				ID:          "pool-1",
				Name:        "Pool One",
				Description: "Test pool",
				OCloudID:    "cloud-1",
			},
		},
		Resources: []smo.Resource{
			{
				ID:             "res-phys",
				GlobalAssetID:  "asset-1",
				ResourcePoolID: "pool-1",
				Description:    "Physical",
				Extensions: map[string]interface{}{
					"resourceKind":    "physical",
					"equipmentType":   "server",
					"equipmentVendor": "Dell",
					"equipmentModel":  "R740",
				},
			},
			{
				ID:             "res-virt",
				ResourceTypeID: "rt-1",
				Description:    "Virtual",
				Extensions: map[string]interface{}{
					"resourceKind": "virtual",
				},
			},
		},
	}

	aaiInventory := plugin.transformToAAIInventory(inventory)
	require.Len(t, aaiInventory.CloudRegions, 1)
	assert.Equal(t, "kubernetes", aaiInventory.CloudRegions[0].CloudType)
	assert.Equal(t, "cloud-1", aaiInventory.CloudRegions[0].CloudRegionID)

	require.Len(t, aaiInventory.Tenants, 1)
	assert.Equal(t, "pool-1", aaiInventory.Tenants[0].TenantID)

	require.Len(t, aaiInventory.PNFs, 1)
	assert.Equal(t, "res-phys", aaiInventory.PNFs[0].PNFName)
	assert.Equal(t, "server", aaiInventory.PNFs[0].EquipType)
	assert.Equal(t, "Dell", aaiInventory.PNFs[0].EquipVendor)
	assert.Equal(t, "R740", aaiInventory.PNFs[0].EquipModel)

	require.Len(t, aaiInventory.VNFs, 1)
	assert.Equal(t, "res-virt", aaiInventory.VNFs[0].VNFID)
	assert.Equal(t, "rt-1", aaiInventory.VNFs[0].VNFType)
}

func TestTransformDeploymentToServiceInstance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	now := time.Now()
	deployment := &smo.Deployment{
		ID:        "dep-1",
		Name:      "Test Deployment",
		PackageID: "pkg-1",
		Status:    "deployed",
		CreatedAt: now,
		UpdatedAt: now,
	}

	si := plugin.transformDeploymentToServiceInstance(deployment)
	assert.Equal(t, "dep-1", si.ServiceInstanceID)
	assert.Equal(t, "Test Deployment", si.ServiceInstanceName)
	assert.Equal(t, "netweave-deployment", si.ServiceType)
	assert.Equal(t, "Active", si.OrchestrationStatus)
	assert.Equal(t, "pkg-1", si.ModelInvariantID)
}

func TestTransformToVESEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	now := time.Now()
	event := &smo.InfrastructureEvent{
		EventID:      "evt-1",
		EventType:    "ResourceCreated",
		ResourceType: "compute",
		ResourceID:   "res-1",
		Timestamp:    now,
	}

	ves := plugin.transformToVESEvent(event)
	assert.Equal(t, "evt-1", ves.Event.CommonEventHeader.EventID)
	assert.Equal(t, "o2ims_ResourceCreated", ves.Event.CommonEventHeader.EventName)
	assert.Equal(t, "res-1", ves.Event.CommonEventHeader.SourceName)
	assert.Equal(t, "netweave-o2ims", ves.Event.CommonEventHeader.ReportingEntityName)
}

func TestTransformDeploymentEventToVES(t *testing.T) {
	logger := zaptest.NewLogger(t)
	plugin := NewPlugin(logger)
	plugin.Config = testConfig()

	now := time.Now()
	event := &smo.DeploymentEvent{
		EventID:      "evt-2",
		EventType:    "DeploymentCreated",
		DeploymentID: "dep-1",
		Timestamp:    now,
	}

	ves := plugin.transformDeploymentEventToVES(event)
	assert.Equal(t, "evt-2", ves.Event.CommonEventHeader.EventID)
	assert.Equal(t, "o2dms_DeploymentCreated", ves.Event.CommonEventHeader.EventName)
	assert.Equal(t, "dep-1", ves.Event.CommonEventHeader.SourceName)
	assert.Equal(t, "netweave-o2dms", ves.Event.CommonEventHeader.ReportingEntityName)
}

// ===== AAI Client Tests =====

func TestAAIClient_Health_Success(t *testing.T) {
	server := httptest.NewServer(okH())
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.NoError(t, err)
}

func TestAAIClient_Health_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestAAIClient_CreateOrUpdateCloudRegion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	cr := &CloudRegion{
		CloudOwner:    "netweave",
		CloudRegionID: "region-1",
		CloudType:     "openstack",
	}

	err = client.CreateOrUpdateCloudRegion(context.Background(), cr)
	require.NoError(t, err)
}

func TestAAIClient_CreateOrUpdateTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	tenant := &Tenant{
		TenantID:      "tenant-1",
		TenantName:    "Test Tenant",
		CloudOwner:    "netweave",
		CloudRegionID: "region-1",
	}

	err = client.CreateOrUpdateTenant(context.Background(), tenant)
	require.NoError(t, err)
}

func TestAAIClient_CreateOrUpdatePNF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	pnf := &PNF{
		PNFName: "pnf-1",
		PNFID:   "id-1",
	}

	err = client.CreateOrUpdatePNF(context.Background(), pnf)
	require.NoError(t, err)
}

func TestAAIClient_CreateOrUpdateVNF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	vnf := &VNF{
		VNFID:   "vnf-1",
		VNFName: "Test VNF",
		VNFType: "firewall",
	}

	err = client.CreateOrUpdateVNF(context.Background(), vnf)
	require.NoError(t, err)
}

func TestAAIClient_CreateOrUpdateServiceInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	si := &ServiceInstance{
		ServiceInstanceID:   "svc-1",
		ServiceInstanceName: "Test Service",
		ServiceType:         "netweave-deployment",
	}

	err = client.CreateOrUpdateServiceInstance(context.Background(), si)
	require.NoError(t, err)
}

func TestAAIClient_GetServiceInstance_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := ServiceInstance{
			ServiceInstanceID:   "svc-1",
			ServiceInstanceName: "Test Service",
			ServiceType:         "netweave-deployment",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	si, err := client.GetServiceInstance(context.Background(), "svc-1")
	require.NoError(t, err)
	assert.Equal(t, "svc-1", si.ServiceInstanceID)
}

func TestAAIClient_GetServiceInstance_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	_, err = client.GetServiceInstance(context.Background(), "svc-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestAAIClient_PutResource_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.AAIURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	cr := &CloudRegion{
		CloudOwner:    "netweave",
		CloudRegionID: "region-1",
	}

	err = client.CreateOrUpdateCloudRegion(context.Background(), cr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad request")
}

func TestAAIClient_Close(t *testing.T) {
	cfg := testConfig()
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	err = client.Close()
	require.NoError(t, err)
}

func TestAAIClient_ShouldStopRetrying(t *testing.T) {
	cfg := testConfig()
	logger := zaptest.NewLogger(t)

	client, err := NewAAIClient(cfg, logger)
	require.NoError(t, err)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"400 error", fmt.Errorf("status 400"), true},
		{"429 error", fmt.Errorf("status 429"), false},
		{"500 error", fmt.Errorf("status 500"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.shouldStopRetrying(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===== DMaaP Client Tests =====

func TestDMaaPClient_Health_Success(t *testing.T) {
	server := httptest.NewServer(okH())
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.NoError(t, err)
}

func TestDMaaPClient_Health_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestDMaaPClient_PublishEvent_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	event := &VESEvent{
		Event: VESEventData{
			CommonEventHeader: CommonEventHeader{
				EventID:   "evt-1",
				EventName: "test",
			},
		},
	}

	err = client.PublishEvent(context.Background(), "test-topic", event)
	require.NoError(t, err)
}

func TestDMaaPClient_PublishEvents_Empty(t *testing.T) {
	cfg := testConfig()
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	err = client.PublishEvents(context.Background(), "test-topic", []*VESEvent{})
	require.NoError(t, err)
}

func TestDMaaPClient_PublishEvents_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	events := []*VESEvent{
		{Event: VESEventData{CommonEventHeader: CommonEventHeader{EventID: "evt-1"}}},
	}

	err = client.PublishEvents(context.Background(), "test-topic", events)
	require.Error(t, err)
}

func TestDMaaPClient_SubscribeTopic_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		msgs := []json.RawMessage{
			json.RawMessage(`{"key":"value1"}`),
			json.RawMessage(`{"key":"value2"}`),
		}
		_ = json.NewEncoder(w).Encode(msgs)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	msgs, err := client.SubscribeTopic(context.Background(), "test-topic", "group-1", "consumer-1")
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

func TestDMaaPClient_SubscribeTopic_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("topic not found"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	_, err = client.SubscribeTopic(context.Background(), "test-topic", "group-1", "consumer-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestDMaaPClient_CreateTopic_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	err = client.CreateTopic(context.Background(), "new-topic", 3, 2)
	require.NoError(t, err)
}

func TestDMaaPClient_CreateTopic_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("topic already exists"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.DMaaPURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	err = client.CreateTopic(context.Background(), "existing-topic", 3, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "topic already exists")
}

func TestDMaaPClient_Close(t *testing.T) {
	cfg := testConfig()
	logger := zaptest.NewLogger(t)

	client, err := NewDMaaPClient(cfg, logger)
	require.NoError(t, err)

	err = client.Close()
	require.NoError(t, err)
}

// ===== SO Client Tests =====

func TestSOClient_Health_Success(t *testing.T) {
	server := httptest.NewServer(okH())
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.NoError(t, err)
}

func TestSOClient_Health_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestSOClient_CreateServiceInstance_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		resp := ServiceInstanceResponse{
			RequestID:         "req-1",
			ServiceInstanceID: "svc-1",
			RequestState:      "IN_PROGRESS",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	req := &ServiceInstanceRequest{
		RequestDetails: RequestDetails{
			ModelInfo: ModelInfo{
				ModelType: "service",
			},
		},
	}

	resp, err := client.CreateServiceInstance(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "svc-1", resp.ServiceInstanceID)
}

func TestSOClient_DeleteServiceInstance_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		resp := ServiceInstanceResponse{
			RequestID:         "req-2",
			ServiceInstanceID: "svc-1",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	req := &ServiceInstanceRequest{}
	resp, err := client.DeleteServiceInstance(context.Background(), "svc-1", req)
	require.NoError(t, err)
	assert.Equal(t, "svc-1", resp.ServiceInstanceID)
}

func TestSOClient_GetOrchestrationStatus_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := OrchestrationStatus{
			RequestID:       "req-1",
			RequestState:    "COMPLETE",
			PercentProgress: 100,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	status, err := client.GetOrchestrationStatus(context.Background(), "req-1")
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", status.RequestState)
}

func TestSOClient_GetOrchestrationStatus_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	_, err = client.GetOrchestrationStatus(context.Background(), "req-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestSOClient_CancelOrchestration_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	err = client.CancelOrchestration(context.Background(), "req-1")
	require.NoError(t, err)
}

func TestSOClient_CancelOrchestration_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	err = client.CancelOrchestration(context.Background(), "req-1")
	require.Error(t, err)
}

func TestSOClient_ExecuteWorkflow_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := WorkflowExecutionResponse{
			ProcessInstanceID: "proc-123",
			DefinitionID:      "def-1",
			Ended:             false,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	processID, err := client.ExecuteWorkflow(context.Background(), "test-wf", map[string]interface{}{"key": "val"})
	require.NoError(t, err)
	assert.Equal(t, "proc-123", processID)
}

func TestSOClient_ExecuteWorkflow_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("workflow error"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	_, err = client.ExecuteWorkflow(context.Background(), "test-wf", nil)
	require.Error(t, err)
}

func TestSOClient_RegisterServiceModel_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	model := &ServiceModel{
		ModelInvariantID: "model-1",
		ModelName:        "Test",
	}

	err = client.RegisterServiceModel(context.Background(), model)
	require.NoError(t, err)
}

func TestSOClient_RegisterServiceModel_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("model exists"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	model := &ServiceModel{ModelInvariantID: "model-1"}

	err = client.RegisterServiceModel(context.Background(), model)
	require.Error(t, err)
}

func TestSOClient_GetServiceModel_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := ServiceModel{
			ModelInvariantID: "model-1",
			ModelName:        "Test",
			ModelVersion:     "1.0",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	model, err := client.GetServiceModel(context.Background(), "model-1")
	require.NoError(t, err)
	assert.Equal(t, "model-1", model.ModelInvariantID)
}

func TestSOClient_ListServiceModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := []*ServiceModel{
			{ModelInvariantID: "m1", ModelName: "Model A"},
			{ModelInvariantID: "m2", ModelName: "Model B"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SOURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	models, err := client.ListServiceModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 2)
}

func TestSOClient_Close(t *testing.T) {
	cfg := testConfig()
	logger := zaptest.NewLogger(t)

	client, err := NewSOClient(cfg, logger)
	require.NoError(t, err)

	err = client.Close()
	require.NoError(t, err)
}

// ===== SDNC Client Tests =====

func TestSDNCClient_Health_Success(t *testing.T) {
	server := httptest.NewServer(okH())
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.NoError(t, err)
}

func TestSDNCClient_Health_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	err = client.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func sdncOKResponse() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := SDNCResponse{
			Output: SDNCOutput{
				ResponseCode:    "200",
				ResponseMessage: "OK",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func TestSDNCClient_CreateNetwork_Success(t *testing.T) {
	server := httptest.NewServer(sdncOKResponse())
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	netInfo := &NetworkInformation{
		NetworkID:   "net-1",
		NetworkType: "overlay",
	}

	resp, err := client.CreateNetwork(context.Background(), netInfo)
	require.NoError(t, err)
	assert.Equal(t, "200", resp.Output.ResponseCode)
}

func TestSDNCClient_DeleteNetwork_Success(t *testing.T) {
	server := httptest.NewServer(sdncOKResponse())
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	resp, err := client.DeleteNetwork(context.Background(), "net-1", "overlay")
	require.NoError(t, err)
	assert.Equal(t, "200", resp.Output.ResponseCode)
}

func TestSDNCClient_ConfigureVNF_Success(t *testing.T) {
	server := httptest.NewServer(sdncOKResponse())
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	params := map[string]interface{}{"key": "value"}
	resp, err := client.ConfigureVNF(context.Background(), "vnf-1", "svc-1", params)
	require.NoError(t, err)
	assert.Equal(t, "200", resp.Output.ResponseCode)
}

func TestSDNCClient_ActivateService_Success(t *testing.T) {
	server := httptest.NewServer(sdncOKResponse())
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	resp, err := client.ActivateService(context.Background(), "svc-1", "type-1")
	require.NoError(t, err)
	assert.Equal(t, "200", resp.Output.ResponseCode)
}

func TestSDNCClient_DeactivateService_Success(t *testing.T) {
	server := httptest.NewServer(sdncOKResponse())
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	resp, err := client.DeactivateService(context.Background(), "svc-1", "type-1")
	require.NoError(t, err)
	assert.Equal(t, "200", resp.Output.ResponseCode)
}

func TestSDNCClient_Operation_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("SDNC error"))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	_, err = client.CreateNetwork(context.Background(), &NetworkInformation{
		NetworkID:   "net-1",
		NetworkType: "overlay",
	})
	require.Error(t, err)
}

func TestSDNCClient_Operation_BadResponseCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := SDNCResponse{
			Output: SDNCOutput{
				ResponseCode:    "500",
				ResponseMessage: "internal error",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.SDNCURL = server.URL
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	_, err = client.ActivateService(context.Background(), "svc-1", "type-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error")
}

func TestSDNCClient_Close(t *testing.T) {
	cfg := testConfig()
	logger := zaptest.NewLogger(t)

	client, err := NewSDNCClient(cfg, logger)
	require.NoError(t, err)

	err = client.Close()
	require.NoError(t, err)
}

// ===== TLS Config Tests =====

func TestCreateTLSConfig_Disabled(t *testing.T) {
	cfg := &Config{TLSEnabled: false}

	tlsCfg, err := createTLSConfig(cfg)
	require.NoError(t, err)
	assert.NotNil(t, tlsCfg)
}

func TestCreateTLSConfig_Enabled(t *testing.T) {
	cfg := &Config{
		TLSEnabled:            true,
		TLSInsecureSkipVerify: true,
	}

	tlsCfg, err := createTLSConfig(cfg)
	require.NoError(t, err)
	assert.True(t, tlsCfg.InsecureSkipVerify)
}

func TestCreateTLSConfig_InvalidCertFile(t *testing.T) {
	cfg := &Config{
		TLSEnabled:  true,
		TLSCertFile: "/nonexistent/cert.pem",
		TLSKeyFile:  "/nonexistent/key.pem",
	}

	_, err := createTLSConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load client certificate")
}

func TestCreateTLSConfig_InvalidCAFile(t *testing.T) {
	cfg := &Config{
		TLSEnabled: true,
		TLSCAFile:  "/nonexistent/ca.pem",
	}

	_, err := createTLSConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read CA certificate")
}

// ===== Generate Transaction ID =====

func TestGenerateTransactionID(t *testing.T) {
	id1 := generateTransactionID()
	id2 := generateTransactionID()

	assert.Contains(t, id1, "netweave-")
	assert.NotEqual(t, id1, id2)
}
