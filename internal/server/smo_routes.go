// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
// This file implements the O2-SMO API routes and handlers for SMO integration.
package server

import (
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/smo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// identifierPattern matches valid alphanumeric identifiers.
var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,255}$`)

// SMO Prometheus metrics.
var (
	smoWorkflowExecutions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "workflow_executions_total",
			Help:      "Total number of workflow execution requests",
		},
		[]string{"workflow_name", "plugin", "status"},
	)

	smoAPIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "api_request_duration_seconds",
			Help:      "Duration of SMO API requests in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"endpoint", "method", "status"},
	)

	smoPluginHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "plugin_health",
			Help:      "Health status of SMO plugins (1=healthy, 0=unhealthy)",
		},
		[]string{"plugin_name"},
	)

	smoServiceModelsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "service_models_total",
			Help:      "Total number of service models",
		},
		[]string{"plugin"},
	)

	smoPoliciesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "policies_applied_total",
			Help:      "Total number of policies applied",
		},
		[]string{"policy_type", "status"},
	)

	smoEventsPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "events_published_total",
			Help:      "Total number of events published",
		},
		[]string{"event_type", "status"},
	)

	smoPluginsRegistered = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "smo",
			Name:      "plugins_registered_total",
			Help:      "Total number of registered SMO plugins",
		},
	)
)

// isValidUUID checks if a string is a valid UUID (any version).
// Uses google/uuid library for proper parsing which handles all UUID versions.
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// isValidIdentifier checks if a string is a valid non-empty identifier.
// Accepts UUIDs and simple alphanumeric identifiers with hyphens/underscores.
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	// Accept UUIDs (any version)
	if isValidUUID(s) {
		return true
	}
	// Accept alphanumeric identifiers with hyphens and underscores (1-256 chars)
	return identifierPattern.MatchString(s)
}

// SMOErrorResponse represents a standardized error response.
type SMOErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// respondWithError sends a standardized error response.
// Use this for errors that should be shown to external clients.
func respondWithError(c *gin.Context, code int, errorType, message string) {
	c.JSON(code, SMOErrorResponse{
		Error:   errorType,
		Message: message,
		Code:    code,
	})
}

// respondWithInternalError logs detailed error internally and returns generic message to client.
// This prevents information disclosure of internal implementation details.
func (h *SMOHandler) respondWithInternalError(c *gin.Context, operation string, err error) {
	// Log detailed error internally for debugging
	h.logger.Error("internal error during SMO operation",
		zap.String("operation", operation),
		zap.String("path", c.Request.URL.Path),
		zap.Error(err),
	)

	// Return generic error to client
	respondWithError(c, http.StatusInternalServerError, "InternalError",
		"An internal error occurred while processing the request")
}

// respondWithBadRequest logs the validation error and returns a safe message.
func (h *SMOHandler) respondWithBadRequest(c *gin.Context, operation string, err error) {
	h.logger.Warn("bad request during SMO operation",
		zap.String("operation", operation),
		zap.String("path", c.Request.URL.Path),
		zap.Error(err),
	)
	respondWithError(c, http.StatusBadRequest, "BadRequest", "Invalid request format")
}

// respondWithNotFound logs the not found error and returns a safe message.
func (h *SMOHandler) respondWithNotFound(c *gin.Context, resourceType string, err error) {
	h.logger.Debug("resource not found",
		zap.String("resource_type", resourceType),
		zap.String("path", c.Request.URL.Path),
		zap.Error(err),
	)
	respondWithError(c, http.StatusNotFound, "NotFound", resourceType+" not found")
}

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
// Base path: /o2smo/v1.
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
// GET /o2smo/v1/plugins.
func (h *SMOHandler) handleListPlugins(c *gin.Context) {
	h.logger.Info("listing SMO plugins")

	plugins := h.registry.List()

	c.JSON(http.StatusOK, gin.H{
		"plugins": plugins,
		"total":   len(plugins),
	})
}

// handleGetPlugin retrieves a specific SMO plugin.
// GET /o2smo/v1/plugins/:pluginId.
func (h *SMOHandler) handleGetPlugin(c *gin.Context) {
	pluginID := c.Param("pluginId")
	h.logger.Info("getting SMO plugin", zap.String("plugin_id", pluginID))

	plugin, err := h.registry.Get(pluginID)
	if err != nil {
		h.respondWithNotFound(c, "Plugin", err)
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
// POST /o2smo/v1/workflows.
func (h *SMOHandler) handleExecuteWorkflow(c *gin.Context) {
	start := time.Now()
	h.logger.Info("executing workflow")

	var req WorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondWithBadRequest(c, "executeWorkflow", err)
		smoAPIRequestDuration.WithLabelValues("workflows", "POST", "400").Observe(time.Since(start).Seconds())
		return
	}

	// Get plugin
	var plugin smo.Plugin
	var err error
	pluginName := req.PluginName
	if pluginName != "" {
		plugin, err = h.registry.Get(pluginName)
	} else {
		plugin, err = h.registry.GetDefault()
		if plugin != nil {
			pluginName = plugin.Metadata().Name
		}
	}

	if err != nil {
		h.respondWithNotFound(c, "Plugin", err)
		smoAPIRequestDuration.WithLabelValues("workflows", "POST", "404").Observe(time.Since(start).Seconds())
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
		h.respondWithInternalError(c, "executeWorkflow", err)
		smoWorkflowExecutions.WithLabelValues(req.WorkflowName, pluginName, "error").Inc()
		smoAPIRequestDuration.WithLabelValues("workflows", "POST", "500").Observe(time.Since(start).Seconds())
		return
	}

	h.logger.Info("workflow execution started",
		zap.String("execution_id", execution.ExecutionID),
		zap.String("workflow_name", execution.WorkflowName),
	)

	// Record metrics
	smoWorkflowExecutions.WithLabelValues(req.WorkflowName, pluginName, "success").Inc()
	smoAPIRequestDuration.WithLabelValues("workflows", "POST", "202").Observe(time.Since(start).Seconds())

	c.JSON(http.StatusAccepted, execution)
}

// handleGetWorkflowStatus retrieves workflow execution status.
// GET /o2smo/v1/workflows/:executionId.
func (h *SMOHandler) handleGetWorkflowStatus(c *gin.Context) {
	executionID := c.Param("executionId")
	pluginName := c.Query("plugin")

	// Validate execution ID
	if !isValidIdentifier(executionID) {
		respondWithError(c, http.StatusBadRequest, "BadRequest", "Invalid execution ID format")
		return
	}

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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Get workflow status
	status, err := plugin.GetWorkflowStatus(c.Request.Context(), executionID)
	if err != nil {
		h.logger.Error("failed to get workflow status", zap.String("execution_id", executionID), zap.Error(err))
		respondWithError(c, http.StatusNotFound, "NotFound", "Workflow execution not found")
		return
	}

	c.JSON(http.StatusOK, status)
}

// handleCancelWorkflow cancels a workflow execution.
// DELETE /o2smo/v1/workflows/:executionId.
func (h *SMOHandler) handleCancelWorkflow(c *gin.Context) {
	executionID := c.Param("executionId")
	pluginName := c.Query("plugin")

	// Validate execution ID
	if !isValidIdentifier(executionID) {
		respondWithError(c, http.StatusBadRequest, "BadRequest", "Invalid execution ID format")
		return
	}

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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Cancel workflow
	if err := plugin.CancelWorkflow(c.Request.Context(), executionID); err != nil {
		h.respondWithInternalError(c, "cancelWorkflow", err)
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
// GET /o2smo/v1/serviceModels.
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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// List service models
	models, err := plugin.ListServiceModels(c.Request.Context())
	if err != nil {
		h.respondWithInternalError(c, "listServiceModels", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"serviceModels": models,
		"total":         len(models),
	})
}

// handleCreateServiceModel creates a new service model.
// POST /o2smo/v1/serviceModels.
func (h *SMOHandler) handleCreateServiceModel(c *gin.Context) {
	h.logger.Info("creating service model")

	var req ServiceModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondWithBadRequest(c, "createServiceModel", err)
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
		h.respondWithNotFound(c, "Plugin", err)
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
		h.respondWithInternalError(c, "createServiceModel", err)
		return
	}

	h.logger.Info("service model created",
		zap.String("model_id", model.ID),
		zap.String("name", model.Name),
	)

	c.JSON(http.StatusCreated, model)
}

// handleGetServiceModel retrieves a specific service model.
// GET /o2smo/v1/serviceModels/:modelId.
func (h *SMOHandler) handleGetServiceModel(c *gin.Context) {
	modelID := c.Param("modelId")
	pluginName := c.Query("plugin")

	// Validate model ID
	if !isValidIdentifier(modelID) {
		respondWithError(c, http.StatusBadRequest, "BadRequest", "Invalid model ID format")
		return
	}

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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Get service model
	model, err := plugin.GetServiceModel(c.Request.Context(), modelID)
	if err != nil {
		h.logger.Error("failed to get service model", zap.String("model_id", modelID), zap.Error(err))
		respondWithError(c, http.StatusNotFound, "NotFound", "Service model not found")
		return
	}

	c.JSON(http.StatusOK, model)
}

// handleDeleteServiceModel deletes a service model.
// DELETE /o2smo/v1/serviceModels/:modelId
// NOTE: Service model deletion is planned for future release - see GitHub issue #33.
func (h *SMOHandler) handleDeleteServiceModel(c *gin.Context) {
	modelID := c.Param("modelId")

	// Validate model ID
	if !isValidIdentifier(modelID) {
		respondWithError(c, http.StatusBadRequest, "BadRequest", "Invalid model ID format")
		return
	}

	h.logger.Info("delete service model requested", zap.String("model_id", modelID))

	// Service model deletion is not implemented in the smo.Plugin interface.
	// This endpoint is documented but returns 501 until interface is extended.
	// Tracked in: https://github.com/piwi3910/netweave/issues/33
	respondWithError(c, http.StatusNotImplemented, "NotImplemented",
		"Service model deletion is not supported by the current plugin interface")
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
// POST /o2smo/v1/policies.
func (h *SMOHandler) handleApplyPolicy(c *gin.Context) {
	h.logger.Info("applying policy")

	var req PolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondWithBadRequest(c, "applyPolicy", err)
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
		h.respondWithNotFound(c, "Plugin", err)
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
		h.respondWithInternalError(c, "applyPolicy", err)
		return
	}

	h.logger.Info("policy applied",
		zap.String("policy_id", policyID),
		zap.String("name", req.Name),
	)

	c.JSON(http.StatusCreated, policy)
}

// handleGetPolicyStatus retrieves policy status.
// GET /o2smo/v1/policies/:policyId/status.
func (h *SMOHandler) handleGetPolicyStatus(c *gin.Context) {
	policyID := c.Param("policyId")
	pluginName := c.Query("plugin")

	// Validate policy ID
	if !isValidIdentifier(policyID) {
		respondWithError(c, http.StatusBadRequest, "BadRequest", "Invalid policy ID format")
		return
	}

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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Get policy status
	status, err := plugin.GetPolicyStatus(c.Request.Context(), policyID)
	if err != nil {
		h.logger.Error("failed to get policy status", zap.String("policy_id", policyID), zap.Error(err))
		respondWithError(c, http.StatusNotFound, "NotFound", "Policy not found")
		return
	}

	c.JSON(http.StatusOK, status)
}

// === Infrastructure Synchronization Handlers ===

// handleSyncInfrastructure syncs infrastructure inventory to SMO.
// POST /o2smo/v1/sync/infrastructure.
func (h *SMOHandler) handleSyncInfrastructure(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("syncing infrastructure inventory")

	var inventory smo.InfrastructureInventory
	if err := c.ShouldBindJSON(&inventory); err != nil {
		h.respondWithBadRequest(c, "syncInfrastructure", err)
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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Sync inventory
	if err := plugin.SyncInfrastructureInventory(c.Request.Context(), &inventory); err != nil {
		h.respondWithInternalError(c, "syncInfrastructure", err)
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
// POST /o2smo/v1/sync/deployments.
func (h *SMOHandler) handleSyncDeployments(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("syncing deployment inventory")

	var inventory smo.DeploymentInventory
	if err := c.ShouldBindJSON(&inventory); err != nil {
		h.respondWithBadRequest(c, "syncDeployments", err)
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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Sync deployments
	if err := plugin.SyncDeploymentInventory(c.Request.Context(), &inventory); err != nil {
		h.respondWithInternalError(c, "syncDeployments", err)
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
// POST /o2smo/v1/events/infrastructure.
func (h *SMOHandler) handlePublishInfrastructureEvent(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("publishing infrastructure event")

	var event smo.InfrastructureEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		h.respondWithBadRequest(c, "publishInfrastructureEvent", err)
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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Publish event
	if err := plugin.PublishInfrastructureEvent(c.Request.Context(), &event); err != nil {
		h.respondWithInternalError(c, "publishInfrastructureEvent", err)
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
// POST /o2smo/v1/events/deployment.
func (h *SMOHandler) handlePublishDeploymentEvent(c *gin.Context) {
	pluginName := c.Query("plugin")
	h.logger.Info("publishing deployment event")

	var event smo.DeploymentEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		h.respondWithBadRequest(c, "publishDeploymentEvent", err)
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
		h.respondWithNotFound(c, "Plugin", err)
		return
	}

	// Publish event
	if err := plugin.PublishDeploymentEvent(c.Request.Context(), &event); err != nil {
		h.respondWithInternalError(c, "publishDeploymentEvent", err)
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
// GET /o2smo/v1/health.
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

		// Update Prometheus health metric
		healthValue := 0.0
		if plugin.Healthy {
			healthy++
			healthValue = 1.0
		} else {
			unhealthy++
		}
		smoPluginHealth.WithLabelValues(plugin.Name).Set(healthValue)
	}

	// Update registry size metric
	smoPluginsRegistered.Set(float64(len(plugins)))

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
