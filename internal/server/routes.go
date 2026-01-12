package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
)

// setupRoutes configures all HTTP routes for the O2-IMS Gateway.
// It organizes routes into logical groups:
//   - Health and readiness endpoints
//   - Prometheus metrics endpoint
//   - O2-IMS API v1, v2, v3 endpoints
func (s *Server) setupRoutes() {
	// Health check endpoints (no authentication required)
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/healthz", s.handleHealth)
	s.router.GET("/ready", s.handleReadiness)
	s.router.GET("/readyz", s.handleReadiness)

	// Metrics endpoint (if enabled)
	if s.config.Observability.Metrics.Enabled {
		s.router.GET(s.config.Observability.Metrics.Path, s.handleMetrics)
	}

	// Initialize version configuration
	versionConfig := NewVersionConfig()

	// O2-IMS API v1 routes (original)
	// Base path: /o2ims-infrastructureInventory/v1 (per O-RAN O2 IMS specification)
	v1 := s.router.Group("/o2ims-infrastructureInventory/v1")
	v1.Use(VersioningMiddleware(versionConfig))
	s.setupV1Routes(v1)

	// O2-IMS API v2 routes (enhanced filtering, batch operations)
	v2 := s.router.Group("/o2ims-infrastructureInventory/v2")
	v2.Use(VersioningMiddleware(versionConfig))
	s.setupV2Routes(v2)

	// O2-IMS API v3 routes (multi-tenancy)
	v3 := s.router.Group("/o2ims-infrastructureInventory/v3")
	v3.Use(VersioningMiddleware(versionConfig))
	v3.Use(TenantMiddleware())
	s.setupV3Routes(v3)

	// API information endpoint
	s.router.GET("/o2ims", s.handleAPIInfo)
	s.router.GET("/", s.handleRoot)

	// Documentation endpoints (Swagger UI, OpenAPI spec)
	s.setupDocsRoutes()
}

// setupV1Routes configures the O2-IMS API v1 endpoints.
func (s *Server) setupV1Routes(v1 *gin.RouterGroup) {
	// Infrastructure Inventory Subscription Management
	// Endpoint: /subscriptions
	subscriptions := v1.Group("/subscriptions")
	{
		subscriptions.GET("", s.handleListSubscriptions)
		subscriptions.POST("", s.handleCreateSubscription)
		subscriptions.GET("/:subscriptionId", s.handleGetSubscription)
		subscriptions.PUT("/:subscriptionId", s.handleUpdateSubscription)
		subscriptions.DELETE("/:subscriptionId", s.handleDeleteSubscription)
	}

	// Resource Pool Management
	// Endpoint: /resourcePools
	resourcePools := v1.Group("/resourcePools")
	{
		resourcePools.GET("", s.handleListResourcePools)
		resourcePools.POST("", s.handleCreateResourcePool)
		resourcePools.GET("/:resourcePoolId", s.handleGetResourcePool)
		resourcePools.PUT("/:resourcePoolId", s.handleUpdateResourcePool)
		resourcePools.DELETE("/:resourcePoolId", s.handleDeleteResourcePool)
		resourcePools.GET("/:resourcePoolId/resources", s.handleListResourcesInPool)
	}

	// Resource Management
	// Endpoint: /resources
	resources := v1.Group("/resources")
	{
		resources.GET("", s.handleListResources)
		resources.POST("", s.handleCreateResource)
		resources.GET("/:resourceId", s.handleGetResource)
		resources.PUT("/:resourceId", s.handleUpdateResource)
		resources.DELETE("/:resourceId", s.handleDeleteResource)
	}

	// Resource Type Management
	// Endpoint: /resourceTypes
	resourceTypes := v1.Group("/resourceTypes")
	{
		resourceTypes.GET("", s.handleListResourceTypes)
		resourceTypes.GET("/:resourceTypeId", s.handleGetResourceType)
	}

	// Deployment Manager Management
	// Endpoint: /deploymentManagers
	deploymentManagers := v1.Group("/deploymentManagers")
	{
		deploymentManagers.GET("", s.handleListDeploymentManagers)
		deploymentManagers.GET("/:deploymentManagerId", s.handleGetDeploymentManager)
	}

	// O-Cloud Infrastructure Information
	// Endpoint: /oCloudInfrastructure
	v1.GET("/oCloudInfrastructure", s.handleGetOCloudInfrastructure)

	// API version endpoint
	v1.GET("", s.handleAPIInfo)
}

// setupV2Routes configures the O2-IMS API v2 endpoints with enhanced features.
// V2 includes all v1 endpoints plus:
// - Batch operations for subscriptions and resource pools
// - Enhanced filtering and field selection
// - Cursor-based pagination option
func (s *Server) setupV2Routes(v2 *gin.RouterGroup) {
	// Include all v1 routes
	s.setupV1Routes(v2)

	// Batch operations (v2 feature)
	// Endpoint: /batch/*
	batch := v2.Group("/batch")
	{
		// Batch subscription operations
		batch.POST("/subscriptions", s.batchHandler.BatchCreateSubscriptions)
		batch.POST("/subscriptions/delete", s.batchHandler.BatchDeleteSubscriptions)

		// Batch resource pool operations
		batch.POST("/resourcePools", s.batchHandler.BatchCreateResourcePools)
		batch.POST("/resourcePools/delete", s.batchHandler.BatchDeleteResourcePools)
	}

	// V2 API info with enhanced features
	v2.GET("/features", s.handleV2Features)
}

// setupV3Routes configures the O2-IMS API v3 endpoints with multi-tenancy support.
// V3 includes all v2 features plus:
// - Multi-tenant isolation
// - Tenant quotas
// - Cross-tenant resource sharing
// - Enhanced audit logging
func (s *Server) setupV3Routes(v3 *gin.RouterGroup) {
	// Infrastructure Inventory Subscription Management (v1 endpoints)
	subscriptions := v3.Group("/subscriptions")
	{
		subscriptions.GET("", s.handleListSubscriptions)
		subscriptions.POST("", s.handleCreateSubscription)
		subscriptions.GET("/:subscriptionId", s.handleGetSubscription)
		subscriptions.PUT("/:subscriptionId", s.handleUpdateSubscription)
		subscriptions.DELETE("/:subscriptionId", s.handleDeleteSubscription)
	}

	// Resource Pool Management (v1 endpoints)
	resourcePools := v3.Group("/resourcePools")
	{
		resourcePools.GET("", s.handleListResourcePools)
		resourcePools.POST("", s.handleCreateResourcePool)
		resourcePools.GET("/:resourcePoolId", s.handleGetResourcePool)
		resourcePools.PUT("/:resourcePoolId", s.handleUpdateResourcePool)
		resourcePools.DELETE("/:resourcePoolId", s.handleDeleteResourcePool)
		resourcePools.GET("/:resourcePoolId/resources", s.handleListResourcesInPool)
	}

	// Resource Management (v1 endpoints)
	resources := v3.Group("/resources")
	{
		resources.GET("", s.handleListResources)
		resources.POST("", s.handleCreateResource)
		resources.GET("/:resourceId", s.handleGetResource)
		resources.PUT("/:resourceId", s.handleUpdateResource)
	}

	// Resource Type Management (v1 endpoints)
	resourceTypes := v3.Group("/resourceTypes")
	{
		resourceTypes.GET("", s.handleListResourceTypes)
		resourceTypes.GET("/:resourceTypeId", s.handleGetResourceType)
	}

	// Deployment Manager Management (v1 endpoints)
	deploymentManagers := v3.Group("/deploymentManagers")
	{
		deploymentManagers.GET("", s.handleListDeploymentManagers)
		deploymentManagers.GET("/:deploymentManagerId", s.handleGetDeploymentManager)
	}

	// Batch Operations (v2 feature)
	batch := v3.Group("/batch")
	{
		// Subscription batch operations
		batch.POST("/subscriptions", s.batchHandler.BatchCreateSubscriptions)
		batch.POST("/subscriptions/delete", s.batchHandler.BatchDeleteSubscriptions)

		// Resource pool batch operations
		batch.POST("/resourcePools", s.batchHandler.BatchCreateResourcePools)
		batch.POST("/resourcePools/delete", s.batchHandler.BatchDeleteResourcePools)
	}

	// Tenant management (v3 feature)
	tenants := v3.Group("/tenants")
	{
		tenants.GET("", s.handleListTenants)
		tenants.POST("", s.handleCreateTenant)
		tenants.GET("/:tenantId", s.handleGetTenant)
		tenants.PUT("/:tenantId", s.handleUpdateTenant)
		tenants.DELETE("/:tenantId", s.handleDeleteTenant)
		tenants.GET("/:tenantId/quotas", s.handleGetTenantQuotas)
		tenants.PUT("/:tenantId/quotas", s.handleUpdateTenantQuotas)
	}

	// V3 API info with multi-tenancy features
	v3.GET("/features", s.handleV3Features)
}

// Health check handlers

// handleHealth returns the health status of the server.
// This endpoint is used by load balancers and monitoring systems.
func (s *Server) handleHealth(c *gin.Context) {
	health := s.healthCheck.CheckHealth(c.Request.Context())

	statusCode := http.StatusOK
	if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, health)
}

// handleReadiness returns the readiness status of the server.
// This endpoint checks if the server is ready to accept traffic.
func (s *Server) handleReadiness(c *gin.Context) {
	readiness := s.healthCheck.CheckReadiness(c.Request.Context())

	statusCode := http.StatusOK
	if !readiness.Ready {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, readiness)
}

// handleMetrics serves Prometheus metrics.
func (s *Server) handleMetrics(c *gin.Context) {
	handler := promhttp.Handler()
	handler.ServeHTTP(c.Writer, c.Request)
}

// API information handlers

// handleRoot returns basic API information.
func (s *Server) handleRoot(c *gin.Context) {
	endpoints := gin.H{
		"health":     "/health",
		"ready":      "/ready",
		"metrics":    s.config.Observability.Metrics.Path,
		"o2ims_base": "/o2ims/v1",
		"o2dms_base": "/o2dms/v1",
		"o2smo_base": "/o2smo/v1",
	}

	c.JSON(http.StatusOK, gin.H{
		"name":        "O2-IMS Gateway",
		"version":     "1.0.0",
		"description": "ORAN O2-IMS, O2-DMS, and O2-SMO compliant API gateway for Kubernetes",
		"api_version": "v1",
		"endpoints":   endpoints,
	})
}

// handleAPIInfo returns O2-IMS API information.
func (s *Server) handleAPIInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"api_version": "v1",
		"base_path":   "/o2ims-infrastructureInventory/v1",
		"resources": []string{
			"subscriptions",
			"resourcePools",
			"resources",
			"resourceTypes",
			"deploymentManagers",
			"oCloudInfrastructure",
		},
	})
}

// Subscription handlers

// handleListSubscriptions lists all subscriptions.
// GET /o2ims/v1/subscriptions.
func (s *Server) handleListSubscriptions(c *gin.Context) {
	s.logger.Info("listing subscriptions")

	// Get all subscriptions from storage
	subs, err := s.store.List(c.Request.Context())
	if err != nil {
		s.logger.Error("failed to list subscriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve subscriptions",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Convert to adapter subscriptions for response
	result := make([]*adapter.Subscription, 0, len(subs))
	for _, sub := range subs {
		result = append(result, &adapter.Subscription{
			SubscriptionID:         sub.ID,
			Callback:               sub.Callback,
			ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: sub.Filter.ResourcePoolID,
				ResourceTypeID: sub.Filter.ResourceTypeID,
				ResourceID:     sub.Filter.ResourceID,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"subscriptions": result,
		"total":         len(result),
	})
}

// handleCreateSubscription creates a new subscription.
// POST /o2ims/v1/subscriptions.
func (s *Server) handleCreateSubscription(c *gin.Context) {
	s.logger.Info("creating subscription")

	var req adapter.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Validate callback URL early for fast failure (SSRF protection)
	if err := s.validateCallback(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Generate subscription ID
	req.SubscriptionID = "sub-" + uuid.New().String()

	// Create subscription via adapter
	created, err := s.adapter.CreateSubscription(c.Request.Context(), &req)
	if err != nil {
		// Check for conflict error (subscription already exists)
		if errors.Is(err, adapter.ErrSubscriptionExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Subscription already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Store subscription
	storageSub := &storage.Subscription{
		ID:                     created.SubscriptionID,
		Callback:               created.Callback,
		ConsumerSubscriptionID: created.ConsumerSubscriptionID,
	}
	if created.Filter != nil {
		storageSub.Filter = storage.SubscriptionFilter{
			ResourcePoolID: created.Filter.ResourcePoolID,
			ResourceTypeID: created.Filter.ResourceTypeID,
			ResourceID:     created.Filter.ResourceID,
		}
	}

	if err := s.store.Create(c.Request.Context(), storageSub); err != nil {
		s.logger.Error("failed to store subscription", zap.Error(err))
		// Attempt to clean up adapter subscription (best effort)
		_ = s.adapter.DeleteSubscription(c.Request.Context(), created.SubscriptionID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to store subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("subscription created",
		zap.String("subscription_id", created.SubscriptionID),
		zap.String("callback", created.Callback))

	c.JSON(http.StatusCreated, created)
}

// handleGetSubscription retrieves a specific subscription.
// GET /o2ims/v1/subscriptions/:subscriptionId.
func (s *Server) handleGetSubscription(c *gin.Context) {
	subscriptionID := c.Param("subscriptionId")
	s.logger.Info("getting subscription", zap.String("subscription_id", subscriptionID))

	// Get subscription from storage
	sub, err := s.store.Get(c.Request.Context(), subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Subscription not found: " + subscriptionID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to get subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Convert to adapter subscription for response
	result := &adapter.Subscription{
		SubscriptionID:         sub.ID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: sub.Filter.ResourcePoolID,
			ResourceTypeID: sub.Filter.ResourceTypeID,
			ResourceID:     sub.Filter.ResourceID,
		},
	}

	c.JSON(http.StatusOK, result)
}

// handleUpdateSubscription updates an existing subscription.
// PUT /o2ims/v1/subscriptions/:subscriptionId.
// This endpoint allows updating both the callback URL and/or subscription filters.
// When filter is null, it removes all filters; empty filter object {} also removes filters.
func (s *Server) handleUpdateSubscription(c *gin.Context) {
	subscriptionID := c.Param("subscriptionId")
	s.logger.Info("updating subscription", zap.String("subscription_id", subscriptionID))

	var req adapter.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Validate callback URL early for fast failure
	if err := s.validateCallback(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Update subscription via adapter
	// The adapter handles validation and persistence to its backend storage
	updated, err := s.adapter.UpdateSubscription(c.Request.Context(), subscriptionID, &req)
	if err != nil {
		// Check for not found error using sentinel error
		if errors.Is(err, adapter.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Subscription not found: " + subscriptionID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to update subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to update subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("subscription updated",
		zap.String("subscription_id", subscriptionID),
		zap.String("callback", updated.Callback))

	c.JSON(http.StatusOK, updated)
}

// handleDeleteSubscription deletes a subscription.
// DELETE /o2ims/v1/subscriptions/:subscriptionId.
func (s *Server) handleDeleteSubscription(c *gin.Context) {
	subscriptionID := c.Param("subscriptionId")
	s.logger.Info("deleting subscription", zap.String("subscription_id", subscriptionID))

	// Delete from adapter
	if err := s.adapter.DeleteSubscription(c.Request.Context(), subscriptionID); err != nil {
		s.logger.Error("failed to delete subscription from adapter", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to delete subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Delete from storage
	if err := s.store.Delete(c.Request.Context(), subscriptionID); err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Subscription not found: " + subscriptionID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to delete subscription from storage", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to delete subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("subscription deleted", zap.String("subscription_id", subscriptionID))
	c.Status(http.StatusNoContent)
}

// Resource Pool handlers

// handleListResourcePools lists all resource pools.
// GET /o2ims/v1/resourcePools.
func (s *Server) handleListResourcePools(c *gin.Context) {
	s.logger.Info("listing resource pools")

	// List resource pools via adapter
	pools, err := s.adapter.ListResourcePools(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list resource pools", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource pools",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourcePools": pools,
		"total":         len(pools),
	})
}

// handleGetResourcePool retrieves a specific resource pool.
// GET /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleGetResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("getting resource pool", zap.String("resource_pool_id", resourcePoolID))

	// Get resource pool via adapter
	pool, err := s.adapter.GetResourcePool(c.Request.Context(), resourcePoolID)
	if err != nil {
		s.logger.Error("failed to get resource pool", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Resource pool not found: " + resourcePoolID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, pool)
}

// handleListResourcesInPool lists resources in a specific pool.
// GET /o2ims/v1/resourcePools/:resourcePoolId/resources.
func (s *Server) handleListResourcesInPool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("listing resources in pool", zap.String("resource_pool_id", resourcePoolID))

	// Create filter for this resource pool
	filter := &adapter.Filter{
		ResourcePoolID: resourcePoolID,
	}

	// List resources via adapter
	resources, err := s.adapter.ListResources(c.Request.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list resources in pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resources",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"total":     len(resources),
	})
}

// Validation constants for resource pool fields.
const (
	// MaxResourcePoolNameLength is the maximum allowed length for resource pool names.
	MaxResourcePoolNameLength = 255

	// MaxResourcePoolIDLength is the maximum allowed length for resource pool IDs.
	MaxResourcePoolIDLength = 255

	// MaxResourcePoolDescriptionLength is the maximum allowed length for resource pool descriptions.
	MaxResourcePoolDescriptionLength = 1000
)

// Validation constants for resource extension fields.
const (
	// MaxExtensionKeys is the maximum number of extension keys allowed.
	MaxExtensionKeys = 100

	// MaxExtensionKeyLength is the maximum length for an extension key.
	MaxExtensionKeyLength = 256

	// MaxExtensionValueSize is the maximum size for a single extension value when JSON-encoded.
	MaxExtensionValueSize = 4096

	// MaxExtensionsTotalSize is the maximum total size for all extensions combined (50KB).
	MaxExtensionsTotalSize = 50000
)

// sanitizeResourcePoolID sanitizes a string for use in resource pool IDs.
// Removes special characters that could cause security issues (path traversal, injection).
// Spaces and slashes are replaced with hyphens, all other special characters are dropped.
func sanitizeResourcePoolID(name string) string {
	var result strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '/' {
			result.WriteRune('-') // Only replace spaces and slashes with hyphens
		}
		// All other special characters are simply dropped for security
	}

	return strings.ToLower(result.String())
}

// sanitizeResourceTypeID sanitizes a resource type ID for use in resource IDs.
// Ensures the resulting ID is URL-safe and prevents injection attacks.
// Spaces and slashes are replaced with hyphens, all other special characters are dropped.
func sanitizeResourceTypeID(typeID string) string {
	var result strings.Builder
	for _, ch := range typeID {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '/' {
			result.WriteRune('-') // Only replace spaces and slashes with hyphens
		}
		// All other special characters are simply dropped for security
	}

	return strings.ToLower(result.String())
}

// sanitizeForLogging removes CRLF characters to prevent log injection attacks.
// This prevents attackers from injecting fake log entries via user-controlled input.
func sanitizeForLogging(s string) string {
	// Remove CR, LF, and other control characters
	sanitized := strings.NewReplacer(
		"\r", "",
		"\n", "",
		"\t", " ",
	).Replace(s)

	// Remove any remaining control characters (ASCII 0-31 except space)
	var result strings.Builder
	for _, ch := range sanitized {
		if ch >= 32 || ch == ' ' {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

// isValidIDCharacter checks if a character is valid for resource pool IDs.
func isValidIDCharacter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

// validateResourcePoolID validates the resource pool ID format.
func validateResourcePoolID(id string) error {
	if len(id) > MaxResourcePoolIDLength {
		return fmt.Errorf("resourcePoolId must not exceed %d characters", MaxResourcePoolIDLength)
	}

	for _, ch := range id {
		if !isValidIDCharacter(ch) {
			return errors.New("resourcePoolId must contain only alphanumeric characters, hyphens, and underscores")
		}
	}

	return nil
}

func validateResourcePoolFields(pool *adapter.ResourcePool) error {
	var validationErrors []string

	// Validate Name is required
	if pool.Name == "" {
		validationErrors = append(validationErrors, "name is required")
	}

	// Validate Name length
	if len(pool.Name) > MaxResourcePoolNameLength {
		validationErrors = append(validationErrors,
			fmt.Sprintf("name must not exceed %d characters", MaxResourcePoolNameLength))
	}

	// Validate ResourcePoolID if provided
	if pool.ResourcePoolID != "" {
		if err := validateResourcePoolID(pool.ResourcePoolID); err != nil {
			validationErrors = append(validationErrors, err.Error())
		}
	}

	// Validate Description length if provided
	if len(pool.Description) > MaxResourcePoolDescriptionLength {
		validationErrors = append(validationErrors,
			fmt.Sprintf("description must not exceed %d characters", MaxResourcePoolDescriptionLength))
	}

	// Return all validation errors together
	if len(validationErrors) > 0 {
		return errors.New(strings.Join(validationErrors, "; "))
	}

	return nil
}

// handleCreateResourcePool creates a new resource pool.
// POST /o2ims/v1/resourcePools.
func (s *Server) handleCreateResourcePool(c *gin.Context) {
	s.logger.Info("creating resource pool")

	var req adapter.ResourcePool
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Validate resource pool fields
	if err := validateResourcePoolFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Generate resource pool ID if not provided (sanitized with UUID for uniqueness)
	// Format: pool-{sanitized-name}-{uuid}
	// Example: "GPU Pool (Production)" → "pool-gpu-pool-production-a1b2c3d4-e5f6-7890-abcd-1234567890ab"
	if req.ResourcePoolID == "" {
		sanitizedName := sanitizeResourcePoolID(req.Name)
		// Clean up consecutive hyphens that can occur from sanitization
		sanitizedName = strings.ReplaceAll(sanitizedName, "--", "-")
		sanitizedName = strings.Trim(sanitizedName, "-")

		// Ensure total length doesn't exceed 255 chars (per O2-IMS spec)
		// UUID is 36 chars, "pool-" is 5 chars, we need 2 chars for separating hyphens = 43 chars reserved
		// This leaves 212 chars for the sanitized name
		maxNameLength := 212
		if len(sanitizedName) > maxNameLength {
			sanitizedName = sanitizedName[:maxNameLength]
		}

		req.ResourcePoolID = "pool-" + sanitizedName + "-" + uuid.New().String()
	}

	// Create resource pool via adapter
	created, err := s.adapter.CreateResourcePool(c.Request.Context(), &req)
	if err != nil {
		// Check for duplicate resource pool using sentinel error
		if errors.Is(err, adapter.ErrResourcePoolExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource pool with ID " + sanitizeForLogging(req.ResourcePoolID) + " already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create resource pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create resource pool",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource pool created",
		zap.String("resource_pool_id", created.ResourcePoolID),
		zap.String("name", sanitizeForLogging(created.Name)))

	// Set Location header for REST compliance
	c.Header("Location", "/o2ims/v1/resourcePools/"+created.ResourcePoolID)
	c.JSON(http.StatusCreated, created)
}

// handleUpdateResourcePool updates an existing resource pool.
// PUT /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleUpdateResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("updating resource pool", zap.String("resource_pool_id", resourcePoolID))

	var req adapter.ResourcePool
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Validate field constraints
	if err := validateResourcePoolFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Update resource pool via adapter
	updated, err := s.adapter.UpdateResourcePool(c.Request.Context(), resourcePoolID, &req)
	if err != nil {
		// Check for not found error using sentinel error
		if errors.Is(err, adapter.ErrResourcePoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource pool not found: " + resourcePoolID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to update resource pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to update resource pool",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource pool updated",
		zap.String("resource_pool_id", updated.ResourcePoolID),
		zap.String("name", sanitizeForLogging(updated.Name)))

	c.JSON(http.StatusOK, updated)
}

// handleDeleteResourcePool deletes a resource pool.
// DELETE /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleDeleteResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("deleting resource pool", zap.String("resource_pool_id", resourcePoolID))

	// Delete resource pool via adapter
	if err := s.adapter.DeleteResourcePool(c.Request.Context(), resourcePoolID); err != nil {
		// Check for not found error using sentinel error
		if errors.Is(err, adapter.ErrResourcePoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource pool not found: " + resourcePoolID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to delete resource pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to delete resource pool",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource pool deleted", zap.String("resource_pool_id", resourcePoolID))
	c.Status(http.StatusNoContent)
}

// Resource handlers

// isAlphanumericOrHyphen checks if a character is alphanumeric or hyphen.
func isAlphanumericOrHyphen(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-'
}

// validateURNNamespaceID validates the URN namespace identifier (nid).
func validateURNNamespaceID(nid string) error {
	if len(nid) < 2 || len(nid) > 32 {
		return errors.New("URN namespace identifier must be 2-32 characters")
	}

	for i, ch := range nid {
		if !isAlphanumericOrHyphen(ch) {
			return errors.New("URN namespace identifier must contain only alphanumeric characters and hyphens")
		}
		if i == 0 && ch == '-' {
			return errors.New("URN namespace identifier must start with alphanumeric character")
		}
	}

	return nil
}

// validateURN validates URN format according to RFC 8141.
// URN format: urn:<nid>:<nss> where:
// - nid (Namespace Identifier): 2-32 alphanumeric characters, case-insensitive.
// - nss (Namespace Specific String): at least 1 character.
func validateURN(urn string) error {
	if !strings.HasPrefix(urn, "urn:") {
		return errors.New("globalAssetId must start with 'urn:'")
	}

	parts := strings.SplitN(urn, ":", 3)
	if len(parts) < 3 {
		return errors.New("globalAssetId must be in URN format: urn:<nid>:<nss> (e.g., urn:o-ran:resource:node-001)")
	}

	if err := validateURNNamespaceID(parts[1]); err != nil {
		return err
	}

	if len(parts[2]) == 0 {
		return errors.New("URN namespace specific string must not be empty")
	}

	return nil
}

// validateExtensions validates resource extensions for size and content.
func validateExtensions(extensions map[string]interface{}) error {
	if len(extensions) > MaxExtensionKeys {
		return fmt.Errorf("extensions map must not exceed %d keys", MaxExtensionKeys)
	}

	totalSize := 0
	for key, value := range extensions {
		if len(key) > MaxExtensionKeyLength {
			return fmt.Errorf("extension keys must not exceed %d characters", MaxExtensionKeyLength)
		}
		// Check JSON-marshaled size to prevent large payloads
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("extension value for key %q must be JSON-serializable", key)
		}
		if len(valueJSON) > MaxExtensionValueSize {
			return fmt.Errorf("extension values must not exceed %d bytes when JSON-encoded", MaxExtensionValueSize)
		}

		// Track total extensions payload size
		totalSize += len(valueJSON)
		if totalSize > MaxExtensionsTotalSize {
			return fmt.Errorf("total extensions payload must not exceed %d bytes (50KB)", MaxExtensionsTotalSize)
		}
	}

	return nil
}

// validateResourceFields validates resource field constraints.
func validateResourceFields(resource *adapter.Resource) error {
	// Validate GlobalAssetID format (URN) if provided
	if resource.GlobalAssetID != "" {
		if err := validateURN(resource.GlobalAssetID); err != nil {
			return err
		}
		if len(resource.GlobalAssetID) > 256 {
			return errors.New("globalAssetId must not exceed 256 characters")
		}
	}

	// Validate Description length
	if len(resource.Description) > 1000 {
		return errors.New("description must not exceed 1000 characters")
	}

	// Validate Extensions
	if resource.Extensions != nil {
		if err := validateExtensions(resource.Extensions); err != nil {
			return err
		}
	}

	return nil
}

// handleListResources lists all resources.
// GET /o2ims/v1/resources.
func (s *Server) handleListResources(c *gin.Context) {
	s.logger.Info("listing resources")

	// List resources via adapter
	resources, err := s.adapter.ListResources(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list resources", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resources",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"total":     len(resources),
	})
}

// handleGetResource retrieves a specific resource.
// GET /o2ims/v1/resources/:resourceId.
func (s *Server) handleGetResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("getting resource", zap.String("resource_id", resourceID))

	// Get resource via adapter
	resource, err := s.adapter.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		// Use sentinel error for better error detection
		if errors.Is(err, adapter.ErrResourceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource not found: " + resourceID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to get resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, resource)
}

// validateCreateRequest validates required fields and constraints for resource creation.
func validateCreateRequest(req *adapter.Resource) error {
	if req.ResourceTypeID == "" {
		return adapter.ErrResourceTypeRequired
	}

	if req.ResourcePoolID == "" {
		return adapter.ErrResourcePoolRequired
	}

	return validateResourceFields(req)
}

// handleCreateResource creates a new resource.
// POST /o2ims/v1/resources.
func (s *Server) handleCreateResource(c *gin.Context) {
	s.logger.Info("creating resource")

	var req adapter.Resource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Validate required fields and constraints
	if err := validateCreateRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Generate URL-safe resource ID if not provided
	if req.ResourceID == "" {
		req.ResourceID = "res-" + sanitizeResourceTypeID(req.ResourceTypeID) + "-" + uuid.New().String()
	}

	// Create resource via adapter
	created, err := s.adapter.CreateResource(c.Request.Context(), &req)
	if err != nil {
		// Check if error indicates duplicate resource
		if errors.Is(err, adapter.ErrResourceExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource with ID " + sanitizeForLogging(req.ResourceID) + " already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource created",
		zap.String("resource_id", created.ResourceID),
		zap.String("resource_type_id", sanitizeForLogging(created.ResourceTypeID)))

	// Set Location header for REST compliance
	c.Header("Location", "/o2ims/v1/resources/"+created.ResourceID)
	c.JSON(http.StatusCreated, created)
}

// handleUpdateResource updates an existing resource.
// PUT /o2ims/v1/resources/:resourceId.
func (s *Server) handleUpdateResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("updating resource", zap.String("resource_id", resourceID))

	var req adapter.Resource
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Get existing resource
	existing, err := s.getExistingResource(c, resourceID)
	if err != nil || existing == nil {
		return // Response already sent
	}

	// Validate request
	if err := s.validateUpdateRequest(c, &req, existing); err != nil {
		return // Response already sent
	}

	// Apply update
	s.applyResourceUpdate(c, resourceID, &req, existing)
}

func (s *Server) handleDeleteResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("deleting resource", zap.String("resource_id", resourceID))

	// Delete resource via adapter
	if err := s.adapter.DeleteResource(c.Request.Context(), resourceID); err != nil {
		// Check for not found error using sentinel error
		if errors.Is(err, adapter.ErrResourceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource not found: " + resourceID,
				"code":    http.StatusNotFound,
			})
			return
		}

		s.logger.Error("failed to delete resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to delete resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource deleted", zap.String("resource_id", resourceID))
	c.Status(http.StatusNoContent)
}

// getExistingResource retrieves an existing resource and handles errors.
func (s *Server) getExistingResource(c *gin.Context, resourceID string) (*adapter.Resource, error) {
	existing, err := s.adapter.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		if errors.Is(err, adapter.ErrResourceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource not found: " + resourceID,
				"code":    http.StatusNotFound,
			})
			return nil, err
		}

		s.logger.Error("failed to get resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource",
			"code":    http.StatusInternalServerError,
		})
		return nil, err
	}
	return existing, nil
}

// validateUpdateRequest validates update request and immutable fields.
func (s *Server) validateUpdateRequest(c *gin.Context, req, existing *adapter.Resource) error {
	// Validate field constraints
	if err := validateResourceFields(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return err
	}

	// Check immutable fields
	if err := checkImmutableFields(req, existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return err
	}

	return nil
}

// checkImmutableFields validates that immutable fields haven't changed.
func checkImmutableFields(req, existing *adapter.Resource) error {
	if req.ResourceTypeID != "" && req.ResourceTypeID != existing.ResourceTypeID {
		return errors.New("resourceTypeId is immutable and cannot be changed")
	}
	if req.ResourcePoolID != "" && req.ResourcePoolID != existing.ResourcePoolID {
		return errors.New("resourcePoolId is immutable and cannot be changed")
	}
	return nil
}

// applyResourceUpdate performs the update operation.
func (s *Server) applyResourceUpdate(c *gin.Context, resourceID string, req, existing *adapter.Resource) {
	// Preserve immutable fields
	req.ResourceID = resourceID
	if req.ResourceTypeID == "" {
		req.ResourceTypeID = existing.ResourceTypeID
	}
	if req.ResourcePoolID == "" {
		req.ResourcePoolID = existing.ResourcePoolID
	}

	// Update via adapter
	updated, err := s.adapter.UpdateResource(c.Request.Context(), resourceID, req)
	if err != nil {
		s.logger.Error("failed to update resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to update resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource updated",
		zap.String("resource_id", updated.ResourceID),
		zap.String("resource_type_id", sanitizeForLogging(updated.ResourceTypeID)))

	c.JSON(http.StatusOK, updated)
}

// Resource Type handlers

// handleListResourceTypes lists all resource types.
// GET /o2ims/v1/resourceTypes.
func (s *Server) handleListResourceTypes(c *gin.Context) {
	s.logger.Info("listing resource types")

	// List resource types via adapter
	types, err := s.adapter.ListResourceTypes(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list resource types", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource types",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourceTypes": types,
		"total":         len(types),
	})
}

// handleGetResourceType retrieves a specific resource type.
// GET /o2ims/v1/resourceTypes/:resourceTypeId.
func (s *Server) handleGetResourceType(c *gin.Context) {
	resourceTypeID := c.Param("resourceTypeId")
	s.logger.Info("getting resource type", zap.String("resource_type_id", resourceTypeID))

	// Get resource type via adapter
	resType, err := s.adapter.GetResourceType(c.Request.Context(), resourceTypeID)
	if err != nil {
		s.logger.Error("failed to get resource type", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Resource type not found: " + resourceTypeID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, resType)
}

// Deployment Manager handlers

// handleListDeploymentManagers lists all deployment managers.
// GET /o2ims/v1/deploymentManagers.
func (s *Server) handleListDeploymentManagers(c *gin.Context) {
	s.logger.Info("listing deployment managers")

	// For now, return a single deployment manager representing this gateway
	// In multi-cluster setups, this could list multiple managers
	dm, err := s.adapter.GetDeploymentManager(c.Request.Context(), "default")
	if err != nil {
		s.logger.Error("failed to get deployment manager", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve deployment managers",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deploymentManagers": []*adapter.DeploymentManager{dm},
		"total":              1,
	})
}

// handleGetDeploymentManager retrieves a specific deployment manager.
// GET /o2ims/v1/deploymentManagers/:deploymentManagerId.
func (s *Server) handleGetDeploymentManager(c *gin.Context) {
	deploymentManagerID := c.Param("deploymentManagerId")
	s.logger.Info("getting deployment manager", zap.String("deployment_manager_id", deploymentManagerID))

	// Get deployment manager via adapter
	dm, err := s.adapter.GetDeploymentManager(c.Request.Context(), deploymentManagerID)
	if err != nil {
		s.logger.Error("failed to get deployment manager", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Deployment manager not found: " + deploymentManagerID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, dm)
}

// O-Cloud Infrastructure handlers

// handleGetOCloudInfrastructure retrieves O-Cloud infrastructure information.
// GET /o2ims/v1/oCloudInfrastructure.
func (s *Server) handleGetOCloudInfrastructure(c *gin.Context) {
	s.logger.Info("getting O-Cloud infrastructure information")

	// Get deployment manager to retrieve O-Cloud information
	dm, err := s.adapter.GetDeploymentManager(c.Request.Context(), "default")
	if err != nil {
		s.logger.Error("failed to get O-Cloud information", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve O-Cloud information",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"oCloudId":    dm.OCloudID,
		"name":        dm.Name,
		"description": dm.Description,
		"serviceUri":  dm.ServiceURI,
	})
}

// handleV2Features returns v2 API feature information.
// GET /o2ims/v2/features.
func (s *Server) handleV2Features(c *gin.Context) {
	features := GetV2Features()
	c.JSON(http.StatusOK, gin.H{
		"version":  "v2",
		"features": features,
		"description": "O2-IMS API v2 with enhanced filtering, batch operations, " +
			"field selection, and cursor-based pagination",
	})
}

// handleV3Features returns v3 API feature information.
// GET /o2ims/v3/features.
func (s *Server) handleV3Features(c *gin.Context) {
	features := GetV3Features()
	c.JSON(http.StatusOK, gin.H{
		"version":  "v3",
		"features": features,
		"description": "O2-IMS API v3 with multi-tenancy support, tenant quotas, " +
			"cross-tenant resource sharing, and enhanced audit logging",
	})
}

// Tenant management handlers (v3)

// handleListTenants lists all tenants.
// GET /o2ims/v3/tenants.
func (s *Server) handleListTenants(c *gin.Context) {
	s.logger.Info("listing tenants")

	// Placeholder implementation - in production this would query a tenant store
	c.JSON(http.StatusOK, gin.H{
		"tenants": []gin.H{
			{
				"tenantId":    "default",
				"name":        "Default Tenant",
				"description": "Default system tenant",
				"createdAt":   "2024-01-01T00:00:00Z",
			},
		},
		"total": 1,
	})
}

// handleCreateTenant creates a new tenant.
// POST /o2ims/v3/tenants.
func (s *Server) handleCreateTenant(c *gin.Context) {
	s.logger.Info("creating tenant")

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	tenantID := uuid.New().String()

	c.JSON(http.StatusCreated, gin.H{
		"tenantId":    tenantID,
		"name":        req.Name,
		"description": req.Description,
		"createdAt":   "2024-01-01T00:00:00Z",
	})
}

// handleGetTenant retrieves a specific tenant.
// GET /o2ims/v3/tenants/:tenantId.
func (s *Server) handleGetTenant(c *gin.Context) {
	tenantID := c.Param("tenantId")
	s.logger.Info("getting tenant", zap.String("tenant_id", tenantID))

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Tenant ID cannot be empty",
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Placeholder - return mock tenant
	c.JSON(http.StatusOK, gin.H{
		"tenantId":    tenantID,
		"name":        "Tenant " + tenantID,
		"description": "Tenant description",
		"createdAt":   "2024-01-01T00:00:00Z",
	})
}

// handleUpdateTenant updates a tenant.
// PUT /o2ims/v3/tenants/:tenantId.
func (s *Server) handleUpdateTenant(c *gin.Context) {
	tenantID := c.Param("tenantId")
	s.logger.Info("updating tenant", zap.String("tenant_id", tenantID))

	var req struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tenantId":    tenantID,
		"name":        req.Name,
		"description": req.Description,
		"updatedAt":   "2024-01-01T00:00:00Z",
	})
}

// handleDeleteTenant deletes a tenant.
// DELETE /o2ims/v3/tenants/:tenantId.
func (s *Server) handleDeleteTenant(c *gin.Context) {
	tenantID := c.Param("tenantId")
	s.logger.Info("deleting tenant", zap.String("tenant_id", tenantID))

	if tenantID == "default" {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "Conflict",
			"message": "Cannot delete default tenant",
			"code":    http.StatusConflict,
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// handleGetTenantQuotas retrieves tenant quotas.
// GET /o2ims/v3/tenants/:tenantId/quotas.
func (s *Server) handleGetTenantQuotas(c *gin.Context) {
	tenantID := c.Param("tenantId")
	s.logger.Info("getting tenant quotas", zap.String("tenant_id", tenantID))

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantID,
		"quotas": gin.H{
			"maxSubscriptions":  100,
			"maxResourcePools":  50,
			"maxResources":      1000,
			"usedSubscriptions": 10,
			"usedResourcePools": 5,
			"usedResources":     100,
		},
	})
}

// handleUpdateTenantQuotas updates tenant quotas.
// PUT /o2ims/v3/tenants/:tenantId/quotas.
func (s *Server) handleUpdateTenantQuotas(c *gin.Context) {
	tenantID := c.Param("tenantId")
	s.logger.Info("updating tenant quotas", zap.String("tenant_id", tenantID))

	var req struct {
		MaxSubscriptions int `json:"maxSubscriptions,omitempty"`
		MaxResourcePools int `json:"maxResourcePools,omitempty"`
		MaxResources     int `json:"maxResources,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantID,
		"quotas": gin.H{
			"maxSubscriptions": req.MaxSubscriptions,
			"maxResourcePools": req.MaxResourcePools,
			"maxResources":     req.MaxResources,
		},
		"updatedAt": "2024-01-01T00:00:00Z",
	})
}

// validateCallback validates a subscription callback URL.
// It performs early validation to provide fast failure before calling the adapter.
// Includes SSRF protection to prevent callbacks to localhost and private IP ranges.
//
// SECURITY NOTE: DNS Rebinding Time-of-Check-Time-of-Use (TOCTOU) Vulnerability
// This validation only checks the callback URL at registration time. An attacker could:
// 1. Register a callback URL pointing to a legitimate public server
// 2. Pass this validation
// 3. Change DNS records to point the hostname to localhost/private IPs
// 4. Receive webhooks at the new (malicious) destination
//
// Mitigation strategies for production deployments:
// - Re-validate callback URLs before EACH webhook delivery attempt
// - Cache DNS results with short TTL and re-validate on changes
// - Implement webhook delivery through a dedicated egress proxy that enforces policies
// - Consider additional authentication mechanisms for webhooks (HMAC signatures, mTLS)
func (s *Server) validateCallback(sub *adapter.Subscription) error {
	if sub == nil {
		return fmt.Errorf("subscription cannot be nil")
	}

	if sub.Callback == "" {
		return fmt.Errorf("callback URL is required")
	}

	// Parse URL to validate format
	parsedURL, err := url.Parse(sub.Callback)
	if err != nil {
		return fmt.Errorf("invalid callback URL format: %w", err)
	}

	// Validate scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("callback URL must use http or https scheme")
	}

	// Validate host
	if parsedURL.Host == "" {
		return fmt.Errorf("callback URL must have a valid host")
	}

	// SSRF Protection: Block localhost and private IP ranges
	if err := validateCallbackHost(parsedURL.Hostname()); err != nil {
		return err
	}

	return nil
}

// validateCallbackHost validates that the callback host is not localhost or a private IP address.
// This prevents SSRF (Server-Side Request Forgery) attacks.
func validateCallbackHost(hostname string) error {
	// Block localhost variations
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return fmt.Errorf("callback URL cannot be localhost")
	}

	// Attempt to resolve hostname to IP
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If DNS lookup fails, allow it - the actual webhook delivery will fail naturally
		// This prevents blocking valid hostnames that are temporarily unresolvable
		return nil
	}

	// Check if any resolved IP is in a private range
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("callback URL cannot be a private IP address")
		}
	}

	return nil
}

// Pre-computed private IP ranges for SSRF protection.
// These are computed at package initialization to avoid runtime parsing overhead
// and ensure error handling happens at startup, not during request processing.
var (
	privateIPv4Nets []*net.IPNet
	privateIPv6Nets []*net.IPNet
)

func init() {
	// Parse private IPv4 ranges (RFC 1918 + link-local)
	privateIPv4CIDRs := []string{
		"10.0.0.0/8",     // Private class A
		"172.16.0.0/12",  // Private class B
		"192.168.0.0/16", // Private class C
		"169.254.0.0/16", // Link-local
	}

	for _, cidr := range privateIPv4CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid IPv4 CIDR in privateIPv4CIDRs: %s: %v", cidr, err))
		}
		privateIPv4Nets = append(privateIPv4Nets, network)
	}

	// Parse private IPv6 ranges
	privateIPv6CIDRs := []string{
		"fc00::/7",  // IPv6 unique local addresses (ULA)
		"fe80::/10", // IPv6 link-local
	}

	for _, cidr := range privateIPv6CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid IPv6 CIDR in privateIPv6CIDRs: %s: %v", cidr, err))
		}
		privateIPv6Nets = append(privateIPv6Nets, network)
	}
}

// isPrivateIP checks if an IP address is in a private or reserved range.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if isPrivateIPv4(ip) {
		return true
	}

	return isPrivateIPv6(ip)
}

// isPrivateIPv4 checks if an IPv4 address is in a private range (RFC 1918).
func isPrivateIPv4(ip net.IP) bool {
	for _, network := range privateIPv4Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// isPrivateIPv6 checks if an IPv6 address is in a private range.
func isPrivateIPv6(ip net.IP) bool {
	// Only check IPv6 addresses
	if ip.To4() != nil {
		return false
	}

	for _, network := range privateIPv6Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
