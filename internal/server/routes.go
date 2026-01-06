package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// setupRoutes configures all HTTP routes for the O2-IMS Gateway.
// It organizes routes into logical groups:
//   - Health and readiness endpoints
//   - Prometheus metrics endpoint
//   - O2-IMS API v1 endpoints
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

	// O2-IMS API v1 routes
	// Base path: /o2ims/v1
	v1 := s.router.Group("/o2ims/v1")
	{
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
	}

	// API information endpoint
	s.router.GET("/o2ims", s.handleAPIInfo)
	s.router.GET("/", s.handleRoot)
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
	c.JSON(http.StatusOK, gin.H{
		"name":        "O2-IMS Gateway",
		"version":     "1.0.0",
		"description": "ORAN O2-IMS compliant API gateway for Kubernetes",
		"api_version": "v1",
		"endpoints": gin.H{
			"health":   "/health",
			"ready":    "/ready",
			"metrics":  s.config.Observability.Metrics.Path,
			"api_base": "/o2ims/v1",
		},
	})
}

// handleAPIInfo returns O2-IMS API information.
func (s *Server) handleAPIInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"api_version": "v1",
		"base_path":   "/o2ims/v1",
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
// GET /o2ims/v1/subscriptions
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
// POST /o2ims/v1/subscriptions
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
// GET /o2ims/v1/subscriptions/:subscriptionId
func (s *Server) handleGetSubscription(c *gin.Context) {
	subscriptionID := c.Param("subscriptionId")
	s.logger.Info("getting subscription", zap.String("subscription_id", subscriptionID))

	// Get subscription from storage
	sub, err := s.store.Get(c.Request.Context(), subscriptionID)
	if err != nil {
		if err == storage.ErrSubscriptionNotFound {
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
// DELETE /o2ims/v1/subscriptions/:subscriptionId
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
		if err == storage.ErrSubscriptionNotFound {
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
// GET /o2ims/v1/resourcePools
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
// GET /o2ims/v1/resourcePools/:resourcePoolId
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
// GET /o2ims/v1/resourcePools/:resourcePoolId/resources
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
// GET /o2ims/v1/resources
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
// GET /o2ims/v1/resources/:resourceId
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
// GET /o2ims/v1/resourceTypes
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
// GET /o2ims/v1/resourceTypes/:resourceTypeId
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
// GET /o2ims/v1/deploymentManagers
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
// GET /o2ims/v1/deploymentManagers/:deploymentManagerId
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
// GET /o2ims/v1/oCloudInfrastructure
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
