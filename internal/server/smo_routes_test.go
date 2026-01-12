package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	smoapi "github.com/piwi3910/netweave/internal/smo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockSMOPlugin implements the smoapi.Plugin interface for testing.
type mockSMOPlugin struct {
	name         string
	version      string
	description  string
	vendor       string
	capabilities []smoapi.Capability
	healthy      bool
	closed       bool

	// Return values for methods
	executeWorkflowResult   *smoapi.WorkflowExecution
	executeWorkflowErr      error
	getWorkflowStatusResult *smoapi.WorkflowStatus
	getWorkflowStatusErr    error
	cancelWorkflowErr       error
	listServiceModelsResult []*smoapi.ServiceModel
	listServiceModelsErr    error
	getServiceModelResult   *smoapi.ServiceModel
	getServiceModelErr      error
	registerServiceModelErr error
	applyPolicyErr          error
	getPolicyStatusResult   *smoapi.PolicyStatus
	getPolicyStatusErr      error
	syncInfraErr            error
	syncDeployErr           error
	publishInfraErr         error
	publishDeployErr        error
}

func (m *mockSMOPlugin) Metadata() smoapi.PluginMetadata {
	return smoapi.PluginMetadata{
		Name:        m.name,
		Version:     m.version,
		Description: m.description,
		Vendor:      m.vendor,
	}
}

func (m *mockSMOPlugin) Capabilities() []smoapi.Capability {
	return m.capabilities
}

func (m *mockSMOPlugin) Initialize(_ context.Context, _ map[string]interface{}) error {
	return nil
}

func (m *mockSMOPlugin) Health(_ context.Context) smoapi.HealthStatus {
	return smoapi.HealthStatus{
		Healthy:   m.healthy,
		Message:   "test",
		Timestamp: time.Now(),
	}
}

func (m *mockSMOPlugin) Close() error {
	m.closed = true
	return nil
}

func (m *mockSMOPlugin) SyncInfrastructureInventory(_ context.Context, _ *smoapi.InfrastructureInventory) error {
	return m.syncInfraErr
}

func (m *mockSMOPlugin) SyncDeploymentInventory(_ context.Context, _ *smoapi.DeploymentInventory) error {
	return m.syncDeployErr
}

func (m *mockSMOPlugin) PublishInfrastructureEvent(_ context.Context, _ *smoapi.InfrastructureEvent) error {
	return m.publishInfraErr
}

func (m *mockSMOPlugin) PublishDeploymentEvent(_ context.Context, _ *smoapi.DeploymentEvent) error {
	return m.publishDeployErr
}

func (m *mockSMOPlugin) ExecuteWorkflow(_ context.Context, workflow *smoapi.WorkflowRequest) (*smoapi.WorkflowExecution, error) {
	if m.executeWorkflowErr != nil {
		return nil, m.executeWorkflowErr
	}
	if m.executeWorkflowResult != nil {
		return m.executeWorkflowResult, nil
	}
	return &smoapi.WorkflowExecution{
		ExecutionID:  "exec-123",
		WorkflowName: workflow.WorkflowName,
		Status:       "RUNNING",
		StartedAt:    time.Now(),
	}, nil
}

func (m *mockSMOPlugin) GetWorkflowStatus(_ context.Context, executionID string) (*smoapi.WorkflowStatus, error) {
	if m.getWorkflowStatusErr != nil {
		return nil, m.getWorkflowStatusErr
	}
	if m.getWorkflowStatusResult != nil {
		return m.getWorkflowStatusResult, nil
	}
	return &smoapi.WorkflowStatus{
		ExecutionID:  executionID,
		WorkflowName: "test-workflow",
		Status:       "RUNNING",
		Progress:     50,
		StartedAt:    time.Now(),
	}, nil
}

func (m *mockSMOPlugin) CancelWorkflow(_ context.Context, _ string) error {
	return m.cancelWorkflowErr
}

func (m *mockSMOPlugin) RegisterServiceModel(_ context.Context, _ *smoapi.ServiceModel) error {
	return m.registerServiceModelErr
}

func (m *mockSMOPlugin) GetServiceModel(_ context.Context, id string) (*smoapi.ServiceModel, error) {
	if m.getServiceModelErr != nil {
		return nil, m.getServiceModelErr
	}
	if m.getServiceModelResult != nil {
		return m.getServiceModelResult, nil
	}
	return &smoapi.ServiceModel{
		ID:      id,
		Name:    "test-model",
		Version: "1.0.0",
	}, nil
}

func (m *mockSMOPlugin) ListServiceModels(_ context.Context) ([]*smoapi.ServiceModel, error) {
	if m.listServiceModelsErr != nil {
		return nil, m.listServiceModelsErr
	}
	if m.listServiceModelsResult != nil {
		return m.listServiceModelsResult, nil
	}
	return []*smoapi.ServiceModel{
		{ID: "model-1", Name: "model-1", Version: "1.0.0"},
	}, nil
}

func (m *mockSMOPlugin) ApplyPolicy(_ context.Context, _ *smoapi.Policy) error {
	return m.applyPolicyErr
}

func (m *mockSMOPlugin) GetPolicyStatus(_ context.Context, policyID string) (*smoapi.PolicyStatus, error) {
	if m.getPolicyStatusErr != nil {
		return nil, m.getPolicyStatusErr
	}
	if m.getPolicyStatusResult != nil {
		return m.getPolicyStatusResult, nil
	}
	now := time.Now()
	return &smoapi.PolicyStatus{
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
		capabilities: []smoapi.Capability{smoapi.CapInventorySync, smoapi.CapWorkflowOrchestration},
		healthy:      true,
	}
}

func setupTestSMOHandler(t *testing.T) (*SMOHandler, *smoapi.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	registry := smoapi.NewRegistry(logger)

	// Register a mock plugin
	plugin := newTestMockPlugin("test-plugin")
	err := registry.Register(context.Background(), "test-plugin", plugin, true)
	require.NoError(t, err)

	handler := NewSMOHandler(registry, logger)
	return handler, registry
}

func setupTestRouter(handler *SMOHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery()) // Add recovery middleware to prevent panics
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

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/plugins", nil)
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
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/plugins/test-plugin", nil)
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
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/plugins/non-existent", nil)
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
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/workflows", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusAccepted, resp.Code)

		var result smoapi.WorkflowExecution
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "test-workflow", result.WorkflowName)
		assert.Equal(t, "RUNNING", result.Status)
		assert.NotEmpty(t, result.ExecutionID)
	})

	t.Run("execute workflow with invalid body", func(t *testing.T) {
		body := `{"invalid": "data"}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/workflows", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestSMOHandler_GetWorkflowStatus(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/workflows/exec-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result smoapi.WorkflowStatus
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "exec-123", result.ExecutionID)
}

func TestSMOHandler_CancelWorkflow(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/o2smo/v1/workflows/exec-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestSMOHandler_ListServiceModels(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/serviceModels", nil)
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
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/serviceModels", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var result smoapi.ServiceModel
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "new-model", result.Name)
		assert.Equal(t, "1.0.0", result.Version)
		assert.NotEmpty(t, result.ID)
	})

	t.Run("create service model with invalid body", func(t *testing.T) {
		body := `{"invalid": "data"}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/serviceModels", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestSMOHandler_GetServiceModel(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/serviceModels/model-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result smoapi.ServiceModel
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "model-123", result.ID)
}

func TestSMOHandler_DeleteServiceModel(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/o2smo/v1/serviceModels/model-123", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	// Delete returns 501 Not Implemented since the interface doesn't support deletion
	assert.Equal(t, http.StatusNotImplemented, resp.Code)
}

func TestSMOHandler_ApplyPolicy(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	t.Run("apply policy successfully", func(t *testing.T) {
		body := `{"name": "test-policy", "policyType": "placement", "enabled": true}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/policies", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusCreated, resp.Code)

		var result smoapi.Policy
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		require.NoError(t, err)

		assert.Equal(t, "test-policy", result.Name)
		assert.NotEmpty(t, result.PolicyID)
	})

	t.Run("apply policy with invalid body", func(t *testing.T) {
		body := `{"invalid": "data"}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/policies", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestSMOHandler_GetPolicyStatus(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/policies/policy-123/status", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result smoapi.PolicyStatus
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/sync/infrastructure", bytes.NewBufferString(body))
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/sync/deployments", bytes.NewBufferString(body))
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/events/infrastructure", bytes.NewBufferString(body))
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/events/deployment", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusAccepted, resp.Code)
}

func TestSMOHandler_Health(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/health", nil)
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
	registry := smoapi.NewRegistry(logger)
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
				req, _ = http.NewRequestWithContext(context.Background(), ep.method, ep.path, bytes.NewBufferString(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequestWithContext(context.Background(), ep.method, ep.path, nil)
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusNotFound, resp.Code)
		})
	}
}

// === Input Validation Tests ===

func TestSMOHandler_InvalidIdentifiers(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
	}{
		{
			name:     "empty execution ID",
			method:   "GET",
			path:     "/o2smo/v1/workflows/",
			wantCode: http.StatusNotFound, // Gin returns 404 for missing path param
		},
		{
			name:     "invalid model ID with special chars",
			method:   "GET",
			path:     "/o2smo/v1/serviceModels/../etc/passwd",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "too long model ID",
			method:   "GET",
			path:     "/o2smo/v1/serviceModels/" + strings.Repeat("a", 300),
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), tt.method, tt.path, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			// Note: Some of these may return 404 due to Gin routing
			assert.True(t, resp.Code == tt.wantCode || resp.Code == http.StatusNotFound)
		})
	}
}

func TestSMOHandler_MalformedJSON(t *testing.T) {
	handler, _ := setupTestSMOHandler(t)
	router := setupTestRouter(handler)

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "malformed workflow JSON",
			path: "/o2smo/v1/workflows",
			body: `{"workflowName": "test", invalid}`,
		},
		{
			name: "malformed service model JSON",
			path: "/o2smo/v1/serviceModels",
			body: `{"name": "test"`,
		},
		{
			name: "malformed policy JSON",
			path: "/o2smo/v1/policies",
			body: `not json at all`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			assert.Equal(t, http.StatusBadRequest, resp.Code)
		})
	}
}

func TestSMOHandler_PluginErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	registry := smoapi.NewRegistry(logger)

	// Register a mock plugin that returns errors
	plugin := &mockSMOPlugin{
		name:                 "error-plugin",
		version:              "1.0.0",
		capabilities:         []smoapi.Capability{smoapi.CapWorkflowOrchestration},
		healthy:              true,
		executeWorkflowErr:   assert.AnError,
		getWorkflowStatusErr: assert.AnError,
		cancelWorkflowErr:    assert.AnError,
		listServiceModelsErr: assert.AnError,
		getServiceModelErr:   assert.AnError,
		applyPolicyErr:       assert.AnError,
		getPolicyStatusErr:   assert.AnError,
		syncInfraErr:         assert.AnError,
		publishInfraErr:      assert.AnError,
	}
	err := registry.Register(context.Background(), "error-plugin", plugin, true)
	require.NoError(t, err)

	handler := NewSMOHandler(registry, logger)
	router := setupTestRouter(handler)

	t.Run("workflow execution error", func(t *testing.T) {
		body := `{"workflowName": "test-workflow"}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/workflows", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("get workflow status error", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/workflows/exec-123", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("list service models error", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/serviceModels", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("apply policy error", func(t *testing.T) {
		body := `{"name": "test-policy", "policyType": "placement"}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/o2smo/v1/policies", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})
}

func TestSMOHandler_HealthDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	registry := smoapi.NewRegistry(logger)

	// Register one healthy and one unhealthy plugin
	healthyPlugin := &mockSMOPlugin{
		name:    "healthy-plugin",
		version: "1.0.0",
		healthy: true,
	}
	unhealthyPlugin := &mockSMOPlugin{
		name:    "unhealthy-plugin",
		version: "1.0.0",
		healthy: false,
	}

	err := registry.Register(context.Background(), "healthy-plugin", healthyPlugin, true)
	require.NoError(t, err)
	err = registry.Register(context.Background(), "unhealthy-plugin", unhealthyPlugin, false)
	require.NoError(t, err)

	handler := NewSMOHandler(registry, logger)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/health", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "degraded", result["status"])
	assert.Equal(t, float64(2), result["totalPlugins"])
	assert.Equal(t, float64(1), result["healthy"])
	assert.Equal(t, float64(1), result["unhealthy"])
}

func TestSMOHandler_HealthUnhealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	registry := smoapi.NewRegistry(logger)

	// Register only unhealthy plugins
	unhealthyPlugin := &mockSMOPlugin{
		name:    "unhealthy-plugin",
		version: "1.0.0",
		healthy: false,
	}

	err := registry.Register(context.Background(), "unhealthy-plugin", unhealthyPlugin, true)
	require.NoError(t, err)

	handler := NewSMOHandler(registry, logger)
	router := setupTestRouter(handler)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/o2smo/v1/health", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", result["status"])
}
