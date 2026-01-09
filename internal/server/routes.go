package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	// Base path: /o2ims-infrastructureInventory/v1 (per O-RAN O2 IMS specification)
	v1 := s.router.Group("/o2ims-infrastructureInventory/v1")
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

	// API information endpoint
	s.router.GET("/o2ims", s.handleAPIInfo)
	s.router.GET("/", s.handleRoot)

	// Documentation endpoints (Swagger UI, OpenAPI spec)
	s.setupDocsRoutes()
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

// sanitizeResourcePoolID sanitizes a string for use in resource pool IDs.
// Removes special characters that could cause security issues (path traversal, injection).
func sanitizeResourcePoolID(name string) string {
	// Replace spaces and slashes with hyphens
	sanitized := strings.NewReplacer(
		" ", "-",
		"/", "-",
		"\\", "-",
		"..", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	).Replace(name)

	// Remove any remaining non-alphanumeric characters except hyphens and underscores
	var result strings.Builder
	for _, ch := range sanitized {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			result.WriteRune(ch)
		}
	}

	return strings.ToLower(result.String())
}

// isValidIDCharacter checks if a character is valid for resource pool IDs.
func isValidIDCharacter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

// validateResourcePoolID validates the resource pool ID format.
func validateResourcePoolID(id string) error {
	if len(id) > 255 {
		return errors.New("resourcePoolId must not exceed 255 characters")
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

	// Validate Name length (max 255 characters)
	if len(pool.Name) > 255 {
		validationErrors = append(validationErrors, "name must not exceed 255 characters")
	}

	// Validate ResourcePoolID if provided
	if pool.ResourcePoolID != "" {
		if err := validateResourcePoolID(pool.ResourcePoolID); err != nil {
			validationErrors = append(validationErrors, err.Error())
		}
	}

	// Validate Description length if provided
	if len(pool.Description) > 1000 {
		validationErrors = append(validationErrors, "description must not exceed 1000 characters")
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
	if req.ResourcePoolID == "" {
		// Add UUID suffix to prevent collisions from similar names
		req.ResourcePoolID = "pool-" + sanitizeResourcePoolID(req.Name) + "-" + uuid.New().String()[:8]
	}

	// Create resource pool via adapter
	created, err := s.adapter.CreateResourcePool(c.Request.Context(), &req)
	if err != nil {
		// Check for duplicate resource pool using sentinel error
		if errors.Is(err, adapter.ErrResourcePoolExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource pool with ID " + req.ResourcePoolID + " already exists",
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
		zap.String("name", created.Name))

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
		zap.String("name", updated.Name))

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
	if len(extensions) > 100 {
		return errors.New("extensions map must not exceed 100 keys")
	}

	for key, value := range extensions {
		if len(key) > 256 {
			return errors.New("extension keys must not exceed 256 characters")
		}
		// Check JSON-marshaled size to prevent large payloads
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("extension value for key %q must be JSON-serializable", key)
		}
		if len(valueJSON) > 4096 {
			return errors.New("extension values must not exceed 4096 bytes when JSON-encoded")
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

	// Validate required fields
	if req.ResourceTypeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Resource type ID is required",
			"code":    http.StatusBadRequest,
		})
		return
	}

	if req.ResourcePoolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Resource pool ID is required",
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Validate field constraints
	if err := validateResourceFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Generate resource ID if not provided
	if req.ResourceID == "" {
		req.ResourceID = "res-" + req.ResourceTypeID + "-" + uuid.New().String()
	}

	// Create resource via adapter
	// The adapter is responsible for enforcing uniqueness constraints.
	created, err := s.adapter.CreateResource(c.Request.Context(), &req)
	if err != nil {
		// Check if error indicates duplicate resource using sentinel error
		if errors.Is(err, adapter.ErrResourceExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource with ID " + req.ResourceID + " already exists",
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
		zap.String("resource_type_id", created.ResourceTypeID))

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
		zap.String("resource_type_id", updated.ResourceTypeID))

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
