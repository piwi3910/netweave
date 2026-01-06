package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/smo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockSMOPlugin implements the smo.Plugin interface for testing.
type mockSMOPlugin struct {
	name         string
	version      string
	description  string
	vendor       string
	capabilities []smo.Capability
	healthy      bool
	healthErr    error
	closed       bool

	// Return values for methods
	executeWorkflowResult   *smo.WorkflowExecution
	executeWorkflowErr      error
	getWorkflowStatusResult *smo.WorkflowStatus
	getWorkflowStatusErr    error
	cancelWorkflowErr       error
	listServiceModelsResult []*smo.ServiceModel
	listServiceModelsErr    error
	getServiceModelResult   *smo.ServiceModel
	getServiceModelErr      error
	registerServiceModelErr error
	applyPolicyErr          error
	getPolicyStatusResult   *smo.PolicyStatus
	getPolicyStatusErr      error
	syncInfraErr            error
	syncDeployErr           error
	publishInfraErr         error
	publishDeployErr        error
}

func (m *mockSMOPlugin) Metadata() smo.PluginMetadata {
	return smo.PluginMetadata{
		Name:        m.name,
		Version:     m.version,
		Description: m.description,
		Vendor:      m.vendor,
	}
}

func (m *mockSMOPlugin) Capabilities() []smo.Capability {
	return m.capabilities
}

func (m *mockSMOPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	return nil
}

func (m *mockSMOPlugin) Health(ctx context.Context) smo.HealthStatus {
	return smo.HealthStatus{
		Healthy:   m.healthy,
		Message:   "test",
		Timestamp: time.Now(),
	}
}

func (m *mockSMOPlugin) Close() error {
	m.closed = true
	return nil
}

func (m *mockSMOPlugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
	return m.syncInfraErr
}

func (m *mockSMOPlugin) SyncDeploymentInventory(ctx context.Context, inventory *smo.DeploymentInventory) error {
	return m.syncDeployErr
}

func (m *mockSMOPlugin) PublishInfrastructureEvent(ctx context.Context, event *smo.InfrastructureEvent) error {
	return m.publishInfraErr
}

func (m *mockSMOPlugin) PublishDeploymentEvent(ctx context.Context, event *smo.DeploymentEvent) error {
	return m.publishDeployErr
}

func (m *mockSMOPlugin) ExecuteWorkflow(ctx context.Context, workflow *smo.WorkflowRequest) (*smo.WorkflowExecution, error) {
	if m.executeWorkflowErr != nil {
		return nil, m.executeWorkflowErr
	}
	if m.executeWorkflowResult != nil {
		return m.executeWorkflowResult, nil
	}
	return &smo.WorkflowExecution{
		ExecutionID:  "exec-123",
		WorkflowName: workflow.WorkflowName,
		Status:       "RUNNING",
		StartedAt:    time.Now(),
	}, nil
}

func (m *mockSMOPlugin) GetWorkflowStatus(ctx context.Context, executionID string) (*smo.WorkflowStatus, error) {
	if m.getWorkflowStatusErr != nil {
		return nil, m.getWorkflowStatusErr
	}
	if m.getWorkflowStatusResult != nil {
		return m.getWorkflowStatusResult, nil
	}
	return &smo.WorkflowStatus{
		ExecutionID:  executionID,
		WorkflowName: "test-workflow",
		Status:       "RUNNING",
		Progress:     50,
		StartedAt:    time.Now(),
	}, nil
}

func (m *mockSMOPlugin) CancelWorkflow(ctx context.Context, executionID string) error {
	return m.cancelWorkflowErr
}

func (m *mockSMOPlugin) RegisterServiceModel(ctx context.Context, model *smo.ServiceModel) error {
	return m.registerServiceModelErr
}

func (m *mockSMOPlugin) GetServiceModel(ctx context.Context, id string) (*smo.ServiceModel, error) {
	if m.getServiceModelErr != nil {
		return nil, m.getServiceModelErr
	}
	if m.getServiceModelResult != nil {
		return m.getServiceModelResult, nil
	}
	return &smo.ServiceModel{
		ID:      id,
		Name:    "test-model",
		Version: "1.0.0",
	}, nil
}

func (m *mockSMOPlugin) ListServiceModels(ctx context.Context) ([]*smo.ServiceModel, error) {
	if m.listServiceModelsErr != nil {
		return nil, m.listServiceModelsErr
	}
	if m.listServiceModelsResult != nil {
		return m.listServiceModelsResult, nil
	}
	return []*smo.ServiceModel{
		{ID: "model-1", Name: "model-1", Version: "1.0.0"},
	}, nil
}

func (m *mockSMOPlugin) ApplyPolicy(ctx context.Context, policy *smo.Policy) error {
	return m.applyPolicyErr
}

func (m *mockSMOPlugin) GetPolicyStatus(ctx context.Context, policyID string) (*smo.PolicyStatus, error) {
	if m.getPolicyStatusErr != nil {
		return nil, m.getPolicyStatusErr
	}
	if m.getPolicyStatusResult != nil {
		return m.getPolicyStatusResult, nil
	}
	now := time.Now()
	return &smo.PolicyStatus{
		PolicyID:     policyID,
		Status:       "active",
		LastEnforced: &now,
	}, nil
}

func newTestMockPlugin(name string) *mockSMOPlugin {
	return &mockSMOPlugin{
		name:         name,
		version:      "1.0.0",
		description:  "Test plugin",
		vendor:       "Test",
		capabilities: []smo.Capability{smo.CapInventorySync, smo.CapWorkflowOrchestration},
		healthy:      true,
	}
}

func setupTestSMOHandler(t *testing.T) (*SMOHandler, *smo.Registry) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	registry := smo.NewRegistry(logger)

	// Register a mock plugin
	plugin := newTestMockPlugin("test-plugin")
	err := registry.Register(context.Background(), "test-plugin", plugin, true)
	require.NoError(t, err)

	handler := NewSMOHandler(registry, logger)
	return handler, registry
}

func setupTestRouter(handler *SMOHandler) *gin.Engine {
	router := gin.New()
	v1 := router.Group("/o2smo/v1")
	{
		v1.GET("/plugins", handler.handleListPlugins)
		v1.GET("/plugins/:pluginId", handler.handleGetPlugin)
		v1.POST("/workflows", handler.handleExecuteWorkflow)
		v1.GET("/workflows/:executionId", handler.handleGetWorkflowStatus)
		v1.DELETE("/workflows/:executionId", handler.handleCancelWorkflow)
		v1.GET("/serviceModels", handler.handleListServiceModels)
		v1.POST("/serviceModels", handler.handleCreateServiceModel)
		v1.GET("/serviceModels/:modelId", handler.handleGetServiceModel)
		v1.DELETE("/serviceModels/:modelId", handler.handleDeleteServiceModel)
		v1.POST("/policies", handler.handleApplyPolicy)
		v1.GET("/policies/:policyId/status", handler.handleGetPolicyStatus)
		v1.POST("/sync/infrastructure", handler.handleSyncInfrastructure)
		v1.POST("/sync/deployments", handler.handleSyncDeployments)
		v1.POST("/events/infrastructure", handler.handlePublishInfrastructureEvent)
		v1.POST("/events/deployment", handler.handlePublishDeploymentEvent)
		v1.GET("/health", handler.handleSMOHealth)
	}
	return router
}

func TestSMOHandler_ListPlugins(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/o2smo/v1/plugins", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	plugins := result["plugins"].([]interface{})
	assert.Len(t, plugins, 1)
	assert.Equal(t, float64(1), result["total"])
}

func TestSMOHandler_GetPlugin(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	t.Run("get existing plugin", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/o2smo/v1/plugins/test-plugin", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var result map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "test-plugin", result["name"])
		assert.Equal(t, "1.0.0", result["version"])
	})

	t.Run("get non-existent plugin", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/o2smo/v1/plugins/non-existent", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
	})
}

func TestSMOHandler_ExecuteWorkflow(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	t.Run("execute workflow successfully", func(t *testing.T) {
		body := `{"workflowName": "test-workflow", "parameters": {"key": "value"}}`
		req, _ := http.NewRequest("POST", "/o2smo/v1/workflows", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusAccepted, resp.Code)

		var result smo.WorkflowExecution
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "test-workflow", result.WorkflowName)
		assert.Equal(t, "RUNNING", result.Status)
		assert.NotEmpty(t, result.ExecutionID)
	})

	t.Run("execute workflow with invalid body", func(t *testing.T) {
		body := `{"invalid": "data"}`
		req, _ := http.NewRequest("POST", "/o2smo/v1/workflows", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestSMOHandler_GetWorkflowStatus(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/o2smo/v1/workflows/exec-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result smo.WorkflowStatus
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "exec-123", result.ExecutionID)
}

func TestSMOHandler_CancelWorkflow(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("DELETE", "/o2smo/v1/workflows/exec-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestSMOHandler_ListServiceModels(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/o2smo/v1/serviceModels", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	models := result["serviceModels"].([]interface{})
	assert.Len(t, models, 1)
}

func TestSMOHandler_CreateServiceModel(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	t.Run("create service model successfully", func(t *testing.T) {
		body := `{"name": "new-model", "version": "1.0.0", "description": "Test model"}`
		req, _ := http.NewRequest("POST", "/o2smo/v1/serviceModels", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var result smo.ServiceModel
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "new-model", result.Name)
		assert.Equal(t, "1.0.0", result.Version)
		assert.NotEmpty(t, result.ID)
	})

	t.Run("create service model with invalid body", func(t *testing.T) {
		body := `{"invalid": "data"}`
		req, _ := http.NewRequest("POST", "/o2smo/v1/serviceModels", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestSMOHandler_GetServiceModel(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/o2smo/v1/serviceModels/model-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result smo.ServiceModel
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "model-123", result.ID)
}

func TestSMOHandler_DeleteServiceModel(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("DELETE", "/o2smo/v1/serviceModels/model-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestSMOHandler_ApplyPolicy(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	t.Run("apply policy successfully", func(t *testing.T) {
		body := `{"name": "test-policy", "policyType": "placement", "enabled": true}`
		req, _ := http.NewRequest("POST", "/o2smo/v1/policies", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var result smo.Policy
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "test-policy", result.Name)
		assert.NotEmpty(t, result.PolicyID)
	})

	t.Run("apply policy with invalid body", func(t *testing.T) {
		body := `{"invalid": "data"}`
		req, _ := http.NewRequest("POST", "/o2smo/v1/policies", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestSMOHandler_GetPolicyStatus(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/o2smo/v1/policies/policy-123/status", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result smo.PolicyStatus
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "policy-123", result.PolicyID)
	assert.Equal(t, "active", result.Status)
}

func TestSMOHandler_SyncInfrastructure(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	body := `{
		"deploymentManagers": [{"id": "dm-1", "name": "test-dm"}],
		"resourcePools": [{"id": "pool-1", "name": "test-pool"}],
		"resources": [],
		"resourceTypes": []
	}`
	req, _ := http.NewRequest("POST", "/o2smo/v1/sync/infrastructure", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "synced", result["status"])
}

func TestSMOHandler_SyncDeployments(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	body := `{
		"packages": [{"id": "pkg-1", "name": "test-pkg"}],
		"deployments": [{"id": "deploy-1", "name": "test-deploy"}]
	}`
	req, _ := http.NewRequest("POST", "/o2smo/v1/sync/deployments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestSMOHandler_PublishInfrastructureEvent(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	body := `{
		"eventType": "ResourceCreated",
		"resourceType": "compute-node",
		"resourceId": "node-1"
	}`
	req, _ := http.NewRequest("POST", "/o2smo/v1/events/infrastructure", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusAccepted, resp.Code)

	var result map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "published", result["status"])
	assert.NotEmpty(t, result["eventId"])
}

func TestSMOHandler_PublishDeploymentEvent(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	body := `{
		"eventType": "DeploymentCreated",
		"deploymentId": "deploy-1"
	}`
	req, _ := http.NewRequest("POST", "/o2smo/v1/events/deployment", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusAccepted, resp.Code)
}

func TestSMOHandler_Health(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/o2smo/v1/health", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "healthy", result["status"])
	assert.Equal(t, float64(1), result["totalPlugins"])
	assert.Equal(t, float64(1), result["healthy"])
	assert.Equal(t, float64(0), result["unhealthy"])
}

func TestSMOHandler_PluginNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	registry := smo.NewRegistry(logger)
	// Don't register any plugins
	handler := NewSMOHandler(registry, logger)
	router := setupTestRouter(handler)

	// All endpoints should return not found for plugin operations
	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/o2smo/v1/workflows", `{"workflowName": "test"}`},
		{"GET", "/o2smo/v1/workflows/exec-1", ""},
		{"DELETE", "/o2smo/v1/workflows/exec-1", ""},
		{"GET", "/o2smo/v1/serviceModels", ""},
		{"POST", "/o2smo/v1/serviceModels", `{"name": "test", "version": "1.0.0"}`},
		{"GET", "/o2smo/v1/serviceModels/model-1", ""},
		{"POST", "/o2smo/v1/policies", `{"name": "test", "policyType": "placement"}`},
		{"GET", "/o2smo/v1/policies/policy-1/status", ""},
		{"POST", "/o2smo/v1/sync/infrastructure", `{}`},
		{"POST", "/o2smo/v1/sync/deployments", `{}`},
		{"POST", "/o2smo/v1/events/infrastructure", `{"eventType": "test"}`},
		{"POST", "/o2smo/v1/events/deployment", `{"eventType": "test"}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.body != "" {
				req, _ = http.NewRequest(ep.method, ep.path, bytes.NewBufferString(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequest(ep.method, ep.path, nil)
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusNotFound, resp.Code)
		})
	}
}
