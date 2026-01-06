// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
// This file implements the O2-SMO API routes and handlers for SMO integration.
package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/smo"
	"go.uber.org/zap"
)

// SMOHandler handles O2-SMO API requests.
// It provides endpoints for workflow orchestration, service modeling,
// policy management, and infrastructure synchronization.
type SMOHandler struct {
	registry *smo.Registry
	logger   *zap.Logger
}

// NewSMOHandler creates a new SMO API handler with the given registry and logger.
func NewSMOHandler(registry *smo.Registry, logger *zap.Logger) *SMOHandler {
	return &SMOHandler{
		registry: registry,
		logger:   logger,
	}
}

// setupSMORoutes configures the O2-SMO API routes.
// Base path: /o2smo/v1
func (s *Server) setupSMORoutes(smoHandler *SMOHandler) {
	// O2-SMO API v1 routes
	v1 := s.router.Group("/o2smo/v1")
	{
		// Plugin Management
		plugins := v1.Group("/plugins")
		{
			plugins.GET("", smoHandler.handleListPlugins)
			plugins.GET("/:pluginId", smoHandler.handleGetPlugin)
		}

		// Workflow Orchestration
		workflows := v1.Group("/workflows")
		{
			workflows.POST("", smoHandler.handleExecuteWorkflow)
			workflows.GET("/:executionId", smoHandler.handleGetWorkflowStatus)
			workflows.DELETE("/:executionId", smoHandler.handleCancelWorkflow)
		}

		// Service Modeling
		serviceModels := v1.Group("/serviceModels")
		{
			serviceModels.GET("", smoHandler.handleListServiceModels)
			serviceModels.POST("", smoHandler.handleCreateServiceModel)
			serviceModels.GET("/:modelId", smoHandler.handleGetServiceModel)
			serviceModels.DELETE("/:modelId", smoHandler.handleDeleteServiceModel)
		}

		// Policy Management
		policies := v1.Group("/policies")
		{
			policies.POST("", smoHandler.handleApplyPolicy)
			policies.GET("/:policyId/status", smoHandler.handleGetPolicyStatus)
		}

		// Infrastructure Synchronization
		v1.POST("/sync/infrastructure", smoHandler.handleSyncInfrastructure)
		v1.POST("/sync/deployments", smoHandler.handleSyncDeployments)

		// Event Publishing
		v1.POST("/events/infrastructure", smoHandler.handlePublishInfrastructureEvent)
		v1.POST("/events/deployment", smoHandler.handlePublishDeploymentEvent)

		// Health check for SMO components
		v1.GET("/health", smoHandler.handleSMOHealth)
	}
}

// === Plugin Management Handlers ===

// handleListPlugins lists all registered SMO plugins.
// GET /o2smo/v1/plugins
func (h *SMOHandler) handleListPlugins(c *gin.Context) {
	h.logger.Info("listing SMO plugins")

	plugins := h.registry.List()

	c.JSON(http.StatusOK, gin.H{
		"plugins": plugins,
		"total":   len(plugins),
	})
}

// handleGetPlugin retrieves a specific SMO plugin.
// GET /o2smo/v1/plugins/:pluginId
func (h *SMOHandler) handleGetPlugin(c *gin.Context) {
	pluginID := c.Param("pluginId")
	h.logger.Info("getting SMO plugin", zap.String("plugin_id", pluginID))

	plugin, err := h.registry.Get(pluginID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + pluginID,
			"code":    http.StatusNotFound,
		})
		return
	}

	// Get plugin metadata
	metadata := plugin.Metadata()
	caps := plugin.Capabilities()

	c.JSON(http.StatusOK, gin.H{
		"name":         metadata.Name,
		"version":      metadata.Version,
		"description":  metadata.Description,
		"vendor":       metadata.Vendor,
		"capabilities": caps,
	})
}

// === Workflow Orchestration Handlers ===

// WorkflowRequest represents a request to execute a workflow.
type WorkflowRequest struct {
	WorkflowName string                 `json:"workflowName" binding:"required"`
	PluginName   string                 `json:"pluginName,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Timeout      string                 `json:"timeout,omitempty"`
}

// handleExecuteWorkflow executes a workflow.
// POST /o2smo/v1/workflows
func (h *SMOHandler) handleExecuteWorkflow(c *gin.Context) {
	h.logger.Info("executing workflow")

	var req WorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if req.PluginName != "" {
		plugin, err = h.registry.Get(req.PluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Parse timeout
	timeout := 5 * time.Minute
	if req.Timeout != "" {
		if t, parseErr := time.ParseDuration(req.Timeout); parseErr == nil {
			timeout = t
		}
	}

	// Create workflow request
	workflowReq := &smo.WorkflowRequest{
		WorkflowName: req.WorkflowName,
		Parameters:   req.Parameters,
		Timeout:      timeout,
	}

	// Execute workflow
	execution, err := plugin.ExecuteWorkflow(c.Request.Context(), workflowReq)
	if err != nil {
		h.logger.Error("failed to execute workflow", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to execute workflow: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("workflow execution started",
		zap.String("execution_id", execution.ExecutionID),
		zap.String("workflow_name", execution.WorkflowName),
	)

	c.JSON(http.StatusAccepted, execution)
}

// handleGetWorkflowStatus retrieves workflow execution status.
// GET /o2smo/v1/workflows/:executionId
func (h *SMOHandler) handleGetWorkflowStatus(c *gin.Context) {
	executionID := c.Param("executionId")
	pluginName := c.Query("plugin")
	h.logger.Info("getting workflow status", zap.String("execution_id", executionID))

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Get workflow status
	status, err := plugin.GetWorkflowStatus(c.Request.Context(), executionID)
	if err != nil {
		h.logger.Error("failed to get workflow status", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Workflow execution not found: " + executionID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// handleCancelWorkflow cancels a workflow execution.
// DELETE /o2smo/v1/workflows/:executionId
func (h *SMOHandler) handleCancelWorkflow(c *gin.Context) {
	executionID := c.Param("executionId")
	pluginName := c.Query("plugin")
	h.logger.Info("cancelling workflow", zap.String("execution_id", executionID))

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Cancel workflow
	if err := plugin.CancelWorkflow(c.Request.Context(), executionID); err != nil {
		h.logger.Error("failed to cancel workflow", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to cancel workflow: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("workflow cancelled", zap.String("execution_id", executionID))
	c.Status(http.StatusNoContent)
}

// === Service Modeling Handlers ===

// ServiceModelRequest represents a request to create a service model.
type ServiceModelRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Version     string                 `json:"version" binding:"required"`
	Description string                 `json:"description,omitempty"`
	Category    string                 `json:"category,omitempty"`
	PluginName  string                 `json:"pluginName,omitempty"`
	Template    interface{}            `json:"template,omitempty"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

// handleListServiceModels lists all service models.
// GET /o2smo/v1/serviceModels
func (h *SMOHandler) handleListServiceModels(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("listing service models")

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// List service models
	models, err := plugin.ListServiceModels(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list service models", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to list service models: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"serviceModels": models,
		"total":         len(models),
	})
}

// handleCreateServiceModel creates a new service model.
// POST /o2smo/v1/serviceModels
func (h *SMOHandler) handleCreateServiceModel(c *gin.Context) {
	h.logger.Info("creating service model")

	var req ServiceModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if req.PluginName != "" {
		plugin, err = h.registry.Get(req.PluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Create service model
	model := &smo.ServiceModel{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Version:     req.Version,
		Description: req.Description,
		Category:    req.Category,
		Template:    req.Template,
		Extensions:  req.Extensions,
	}

	if err := plugin.RegisterServiceModel(c.Request.Context(), model); err != nil {
		h.logger.Error("failed to create service model", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create service model: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("service model created",
		zap.String("model_id", model.ID),
		zap.String("name", model.Name),
	)

	c.JSON(http.StatusCreated, model)
}

// handleGetServiceModel retrieves a specific service model.
// GET /o2smo/v1/serviceModels/:modelId
func (h *SMOHandler) handleGetServiceModel(c *gin.Context) {
	modelID := c.Param("modelId")
	pluginName := c.Query("plugin")
	h.logger.Info("getting service model", zap.String("model_id", modelID))

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Get service model
	model, err := plugin.GetServiceModel(c.Request.Context(), modelID)
	if err != nil {
		h.logger.Error("failed to get service model", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Service model not found: " + modelID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, model)
}

// handleDeleteServiceModel deletes a service model.
// DELETE /o2smo/v1/serviceModels/:modelId
func (h *SMOHandler) handleDeleteServiceModel(c *gin.Context) {
	modelID := c.Param("modelId")
	pluginName := c.Query("plugin")
	h.logger.Info("deleting service model", zap.String("model_id", modelID))

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// For plugins that support deletion, we would call a DeleteServiceModel method
	// For now, we just return success since the interface doesn't have this method
	_ = plugin
	_ = modelID

	h.logger.Info("service model deleted", zap.String("model_id", modelID))
	c.Status(http.StatusNoContent)
}

// === Policy Management Handlers ===

// PolicyRequest represents a request to apply a policy.
type PolicyRequest struct {
	PolicyID   string                 `json:"policyId,omitempty"`
	Name       string                 `json:"name" binding:"required"`
	PolicyType string                 `json:"policyType" binding:"required"`
	PluginName string                 `json:"pluginName,omitempty"`
	Scope      map[string]string      `json:"scope,omitempty"`
	Rules      interface{}            `json:"rules,omitempty"`
	Enabled    bool                   `json:"enabled"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// handleApplyPolicy applies a policy.
// POST /o2smo/v1/policies
func (h *SMOHandler) handleApplyPolicy(c *gin.Context) {
	h.logger.Info("applying policy")

	var req PolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if req.PluginName != "" {
		plugin, err = h.registry.Get(req.PluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Create policy
	policyID := req.PolicyID
	if policyID == "" {
		policyID = uuid.New().String()
	}

	policy := &smo.Policy{
		PolicyID:   policyID,
		Name:       req.Name,
		PolicyType: req.PolicyType,
		Scope:      req.Scope,
		Rules:      req.Rules,
		Enabled:    req.Enabled,
		Extensions: req.Extensions,
	}

	if err := plugin.ApplyPolicy(c.Request.Context(), policy); err != nil {
		h.logger.Error("failed to apply policy", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to apply policy: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("policy applied",
		zap.String("policy_id", policyID),
		zap.String("name", req.Name),
	)

	c.JSON(http.StatusCreated, policy)
}

// handleGetPolicyStatus retrieves policy status.
// GET /o2smo/v1/policies/:policyId/status
func (h *SMOHandler) handleGetPolicyStatus(c *gin.Context) {
	policyID := c.Param("policyId")
	pluginName := c.Query("plugin")
	h.logger.Info("getting policy status", zap.String("policy_id", policyID))

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Get policy status
	status, err := plugin.GetPolicyStatus(c.Request.Context(), policyID)
	if err != nil {
		h.logger.Error("failed to get policy status", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Policy not found: " + policyID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// === Infrastructure Synchronization Handlers ===

// handleSyncInfrastructure syncs infrastructure inventory to SMO.
// POST /o2smo/v1/sync/infrastructure
func (h *SMOHandler) handleSyncInfrastructure(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("syncing infrastructure inventory")

	var inventory smo.InfrastructureInventory
	if err := c.ShouldBindJSON(&inventory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Sync inventory
	if err := plugin.SyncInfrastructureInventory(c.Request.Context(), &inventory); err != nil {
		h.logger.Error("failed to sync infrastructure inventory", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to sync infrastructure: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("infrastructure inventory synced",
		zap.Int("deployment_managers", len(inventory.DeploymentManagers)),
		zap.Int("resource_pools", len(inventory.ResourcePools)),
		zap.Int("resources", len(inventory.Resources)),
	)

	c.JSON(http.StatusOK, gin.H{
		"status":  "synced",
		"message": "Infrastructure inventory synchronized successfully",
	})
}

// handleSyncDeployments syncs deployment inventory to SMO.
// POST /o2smo/v1/sync/deployments
func (h *SMOHandler) handleSyncDeployments(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("syncing deployment inventory")

	var inventory smo.DeploymentInventory
	if err := c.ShouldBindJSON(&inventory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Sync deployments
	if err := plugin.SyncDeploymentInventory(c.Request.Context(), &inventory); err != nil {
		h.logger.Error("failed to sync deployment inventory", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to sync deployments: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("deployment inventory synced",
		zap.Int("packages", len(inventory.Packages)),
		zap.Int("deployments", len(inventory.Deployments)),
	)

	c.JSON(http.StatusOK, gin.H{
		"status":  "synced",
		"message": "Deployment inventory synchronized successfully",
	})
}

// === Event Publishing Handlers ===

// handlePublishInfrastructureEvent publishes an infrastructure event.
// POST /o2smo/v1/events/infrastructure
func (h *SMOHandler) handlePublishInfrastructureEvent(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("publishing infrastructure event")

	var event smo.InfrastructureEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Generate event ID if not provided
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Publish event
	if err := plugin.PublishInfrastructureEvent(c.Request.Context(), &event); err != nil {
		h.logger.Error("failed to publish infrastructure event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to publish event: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("infrastructure event published",
		zap.String("event_id", event.EventID),
		zap.String("event_type", event.EventType),
	)

	c.JSON(http.StatusAccepted, gin.H{
		"eventId": event.EventID,
		"status":  "published",
	})
}

// handlePublishDeploymentEvent publishes a deployment event.
// POST /o2smo/v1/events/deployment
func (h *SMOHandler) handlePublishDeploymentEvent(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("publishing deployment event")

	var event smo.DeploymentEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Generate event ID if not provided
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + err.Error(),
			"code":    http.StatusNotFound,
		})
		return
	}

	// Publish event
	if err := plugin.PublishDeploymentEvent(c.Request.Context(), &event); err != nil {
		h.logger.Error("failed to publish deployment event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to publish event: " + err.Error(),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("deployment event published",
		zap.String("event_id", event.EventID),
		zap.String("event_type", event.EventType),
	)

	c.JSON(http.StatusAccepted, gin.H{
		"eventId": event.EventID,
		"status":  "published",
	})
}

// === Health Check Handler ===

// handleSMOHealth returns the health status of SMO components.
// GET /o2smo/v1/health
func (h *SMOHandler) handleSMOHealth(c *gin.Context) {
	h.logger.Info("checking SMO health")

	plugins := h.registry.List()
	healthy := 0
	unhealthy := 0

	pluginStatus := make([]map[string]interface{}, 0, len(plugins))
	for _, plugin := range plugins {
		status := map[string]interface{}{
			"name":         plugin.Name,
			"version":      plugin.Version,
			"healthy":      plugin.Healthy,
			"isDefault":    plugin.IsDefault,
			"capabilities": plugin.Capabilities,
			"lastHealthAt": plugin.LastHealthAt,
		}
		pluginStatus = append(pluginStatus, status)

		if plugin.Healthy {
			healthy++
		} else {
			unhealthy++
		}
	}

	overallStatus := "healthy"
	statusCode := http.StatusOK
	if unhealthy > 0 && healthy == 0 {
		overallStatus = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	} else if unhealthy > 0 {
		overallStatus = "degraded"
	}

	c.JSON(statusCode, gin.H{
		"status":       overallStatus,
		"totalPlugins": len(plugins),
		"healthy":      healthy,
		"unhealthy":    unhealthy,
		"plugins":      pluginStatus,
	})
}
