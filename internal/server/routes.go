package server

import (
	"errors"
	"net/http"

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
		subscriptions.DELETE("/:subscriptionId", s.handleDeleteSubscription)
	}

	// Resource Pool Management
	// Endpoint: /resourcePools
	resourcePools := v1.Group("/resourcePools")
	{
		resourcePools.GET("", s.handleListResourcePools)
		resourcePools.GET("/:resourcePoolId", s.handleGetResourcePool)
		resourcePools.GET("/:resourcePoolId/resources", s.handleListResourcesInPool)
	}

	// Resource Management
	// Endpoint: /resources
	resources := v1.Group("/resources")
	{
		resources.GET("", s.handleListResources)
		resources.GET("/:resourceId", s.handleGetResource)
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
		batch.POST("/subscriptions", s.handleBatchCreateSubscriptions)
		batch.POST("/subscriptions/delete", s.handleBatchDeleteSubscriptions)

		// Batch resource pool operations
		batch.POST("/resourcePools", s.handleBatchCreateResourcePools)
		batch.POST("/resourcePools/delete", s.handleBatchDeleteResourcePools)
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
	// Include all v2 routes
	s.setupV2Routes(v3)

	// Tenant management (v3 feature)
	// Endpoint: /tenants/*
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

	// Generate subscription ID
	req.SubscriptionID = uuid.New().String()

	// Create subscription via adapter
	created, err := s.adapter.CreateSubscription(c.Request.Context(), &req)
	if err != nil {
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

// Resource handlers

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
		s.logger.Error("failed to get resource", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Resource not found: " + resourceID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, resource)
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

// Batch operation handlers (v2+)

// handleBatchCreateSubscriptions handles POST /o2ims/v2/batch/subscriptions.
// Creates multiple subscriptions in a single request.
func (s *Server) handleBatchCreateSubscriptions(c *gin.Context) {
	s.logger.Info("batch creating subscriptions")

	var req struct {
		Subscriptions []struct {
			Callback               string `json:"callback"`
			ConsumerSubscriptionID string `json:"consumerSubscriptionId,omitempty"`
			Filter                 struct {
				ResourcePoolID []string `json:"resourcePoolId,omitempty"`
				ResourceTypeID []string `json:"resourceTypeId,omitempty"`
				ResourceID     []string `json:"resourceId,omitempty"`
			} `json:"filter,omitempty"`
		} `json:"subscriptions"`
		Atomic bool `json:"atomic,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	if len(req.Subscriptions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "At least one subscription is required",
			"code":    http.StatusBadRequest,
		})
		return
	}

	results := make([]gin.H, 0, len(req.Subscriptions))
	successCount := 0
	var createdIDs []string

	for i, sub := range req.Subscriptions {
		subscriptionID := uuid.New().String()

		storageSub := &storage.Subscription{
			ID:                     subscriptionID,
			Callback:               sub.Callback,
			ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		}

		if len(sub.Filter.ResourcePoolID) > 0 {
			storageSub.Filter.ResourcePoolID = sub.Filter.ResourcePoolID[0]
		}
		if len(sub.Filter.ResourceTypeID) > 0 {
			storageSub.Filter.ResourceTypeID = sub.Filter.ResourceTypeID[0]
		}
		if len(sub.Filter.ResourceID) > 0 {
			storageSub.Filter.ResourceID = sub.Filter.ResourceID[0]
		}

		if err := s.store.Create(c.Request.Context(), storageSub); err != nil {
			results = append(results, gin.H{
				"index":   i,
				"success": false,
				"error":   err.Error(),
			})
			continue
		}

		createdIDs = append(createdIDs, subscriptionID)
		successCount++
		results = append(results, gin.H{
			"index":          i,
			"success":        true,
			"subscriptionId": subscriptionID,
		})
	}

	// Handle atomic rollback if needed
	if req.Atomic && successCount < len(req.Subscriptions) {
		for _, id := range createdIDs {
			_ = s.store.Delete(c.Request.Context(), id)
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":   "AtomicFailure",
			"message": "Batch operation rolled back due to partial failure",
			"code":    http.StatusConflict,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results":      results,
		"successCount": successCount,
		"failureCount": len(req.Subscriptions) - successCount,
	})
}

// handleBatchDeleteSubscriptions handles POST /o2ims/v2/batch/subscriptions/delete.
// Deletes multiple subscriptions in a single request.
func (s *Server) handleBatchDeleteSubscriptions(c *gin.Context) {
	s.logger.Info("batch deleting subscriptions")

	var req struct {
		SubscriptionIDs []string `json:"subscriptionIds"`
		Atomic          bool     `json:"atomic,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	if len(req.SubscriptionIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "At least one subscription ID is required",
			"code":    http.StatusBadRequest,
		})
		return
	}

	results := make([]gin.H, 0, len(req.SubscriptionIDs))
	successCount := 0

	for i, id := range req.SubscriptionIDs {
		if err := s.store.Delete(c.Request.Context(), id); err != nil {
			results = append(results, gin.H{
				"index":   i,
				"success": false,
				"error":   "Subscription not found: " + id,
			})
		} else {
			successCount++
			results = append(results, gin.H{
				"index":   i,
				"success": true,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results":      results,
		"successCount": successCount,
		"failureCount": len(req.SubscriptionIDs) - successCount,
	})
}

// handleBatchCreateResourcePools handles POST /o2ims/v2/batch/resourcePools.
// Creates multiple resource pools in a single request.
func (s *Server) handleBatchCreateResourcePools(c *gin.Context) {
	s.logger.Info("batch creating resource pools")

	var req struct {
		ResourcePools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description,omitempty"`
			Location    string                 `json:"location,omitempty"`
			OCloudID    string                 `json:"oCloudId,omitempty"`
			Extensions  map[string]interface{} `json:"extensions,omitempty"`
		} `json:"resourcePools"`
		Atomic bool `json:"atomic,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	if len(req.ResourcePools) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "At least one resource pool is required",
			"code":    http.StatusBadRequest,
		})
		return
	}

	results := make([]gin.H, 0, len(req.ResourcePools))
	successCount := 0
	var createdIDs []string

	for i, pool := range req.ResourcePools {
		adapterPool := &adapter.ResourcePool{
			Name:        pool.Name,
			Description: pool.Description,
			Location:    pool.Location,
			OCloudID:    pool.OCloudID,
			Extensions:  pool.Extensions,
		}

		created, err := s.adapter.CreateResourcePool(c.Request.Context(), adapterPool)
		if err != nil {
			results = append(results, gin.H{
				"index":   i,
				"success": false,
				"error":   err.Error(),
			})
			continue
		}

		createdIDs = append(createdIDs, created.ResourcePoolID)
		successCount++
		results = append(results, gin.H{
			"index":          i,
			"success":        true,
			"resourcePoolId": created.ResourcePoolID,
		})
	}

	// Handle atomic rollback if needed
	if req.Atomic && successCount < len(req.ResourcePools) {
		for _, id := range createdIDs {
			_ = s.adapter.DeleteResourcePool(c.Request.Context(), id)
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":   "AtomicFailure",
			"message": "Batch operation rolled back due to partial failure",
			"code":    http.StatusConflict,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results":      results,
		"successCount": successCount,
		"failureCount": len(req.ResourcePools) - successCount,
	})
}

// handleBatchDeleteResourcePools handles POST /o2ims/v2/batch/resourcePools/delete.
// Deletes multiple resource pools in a single request.
func (s *Server) handleBatchDeleteResourcePools(c *gin.Context) {
	s.logger.Info("batch deleting resource pools")

	var req struct {
		ResourcePoolIDs []string `json:"resourcePoolIds"`
		Atomic          bool     `json:"atomic,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	if len(req.ResourcePoolIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "At least one resource pool ID is required",
			"code":    http.StatusBadRequest,
		})
		return
	}

	results := make([]gin.H, 0, len(req.ResourcePoolIDs))
	successCount := 0

	for i, id := range req.ResourcePoolIDs {
		if err := s.adapter.DeleteResourcePool(c.Request.Context(), id); err != nil {
			results = append(results, gin.H{
				"index":   i,
				"success": false,
				"error":   "Resource pool not found: " + id,
			})
		} else {
			successCount++
			results = append(results, gin.H{
				"index":   i,
				"success": true,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results":      results,
		"successCount": successCount,
		"failureCount": len(req.ResourcePoolIDs) - successCount,
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
