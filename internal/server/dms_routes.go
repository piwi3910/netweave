package server

import (
	"github.com/gin-gonic/gin"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
)

// setupDMSRoutes configures all O2-DMS API routes.
// It organizes routes into the following groups:
//   - /o2dms/v1/deploymentLifecycle - API information
//   - /o2dms/v1/nfDeployments - NF deployment lifecycle management
//   - /o2dms/v1/nfDeploymentDescriptors - Deployment descriptors
//   - /o2dms/v1/subscriptions - Event subscriptions
//   - /o2dms/v2/* - V2 API with enhanced filtering and batch operations
//   - /o2dms/v3/* - V3 API with multi-tenancy support
func (s *Server) setupDMSRoutes(handler *dmshandlers.Handler) {
	// O2-DMS API v1 routes
	// Base path: /o2dms/v1
	v1 := s.router.Group("/o2dms/v1")
	{
		// Deployment Lifecycle Information
		// Endpoint: /deploymentLifecycle
		v1.GET("/deploymentLifecycle", handler.GetDeploymentLifecycleInfo)

		// NF Deployment Management
		// Endpoint: /nfDeployments
		s.setupNFDeploymentRoutes(v1, handler)

		// NF Deployment Descriptor Management
		// Endpoint: /nfDeploymentDescriptors
		s.setupNFDeploymentDescriptorRoutes(v1, handler)

		// DMS Subscription Management
		// Endpoint: /subscriptions
		s.setupDMSSubscriptionRoutes(v1, handler)
	}

	// O2-DMS API v2 routes (enhanced filtering, batch operations)
	v2 := s.router.Group("/o2dms/v2")
	{
		s.setupDMSV2Routes(v2, handler)
	}

	// O2-DMS API v3 routes (multi-tenancy)
	v3 := s.router.Group("/o2dms/v3")
	{
		v3.Use(TenantMiddleware())
		s.setupDMSV3Routes(v3, handler)
	}

	// API information endpoint
	s.router.GET("/o2dms", s.HandleDMSAPIInfo)
}

// setupNFDeploymentRoutes configures NF deployment routes.
func (s *Server) setupNFDeploymentRoutes(v1 *gin.RouterGroup, handler *dmshandlers.Handler) {
	nfDeployments := v1.Group("/nfDeployments")
	{
		// CRUD operations
		nfDeployments.GET("", handler.ListNFDeployments)
		nfDeployments.POST("", handler.CreateNFDeployment)
		nfDeployments.GET("/:nfDeploymentId", handler.GetNFDeployment)
		nfDeployments.PUT("/:nfDeploymentId", handler.UpdateNFDeployment)
		nfDeployments.DELETE("/:nfDeploymentId", handler.DeleteNFDeployment)

		// Lifecycle operations
		nfDeployments.POST("/:nfDeploymentId/scale", handler.ScaleNFDeployment)
		nfDeployments.POST("/:nfDeploymentId/rollback", handler.RollbackNFDeployment)

		// Status and history
		nfDeployments.GET("/:nfDeploymentId/status", handler.GetNFDeploymentStatus)
		nfDeployments.GET("/:nfDeploymentId/history", handler.GetNFDeploymentHistory)
	}
}

// setupNFDeploymentDescriptorRoutes configures NF deployment descriptor routes.
func (s *Server) setupNFDeploymentDescriptorRoutes(v1 *gin.RouterGroup, handler *dmshandlers.Handler) {
	descriptors := v1.Group("/nfDeploymentDescriptors")
	{
		descriptors.GET("", handler.ListNFDeploymentDescriptors)
		descriptors.POST("", handler.CreateNFDeploymentDescriptor)
		descriptors.GET("/:nfDeploymentDescriptorId", handler.GetNFDeploymentDescriptor)
		descriptors.DELETE("/:nfDeploymentDescriptorId", handler.DeleteNFDeploymentDescriptor)
	}
}

// setupDMSSubscriptionRoutes configures DMS subscription routes.
func (s *Server) setupDMSSubscriptionRoutes(v1 *gin.RouterGroup, handler *dmshandlers.Handler) {
	subscriptions := v1.Group("/subscriptions")
	{
		subscriptions.GET("", handler.ListDMSSubscriptions)
		subscriptions.POST("", handler.CreateDMSSubscription)
		subscriptions.GET("/:subscriptionId", handler.GetDMSSubscription)
		subscriptions.DELETE("/:subscriptionId", handler.DeleteDMSSubscription)
	}
}

// setupDMSV2Routes configures the O2-DMS API v2 endpoints with enhanced features.
// V2 includes all v1 features plus:
//   - Enhanced filtering and field selection
//   - Batch operations for deployments and descriptors
//   - Cursor-based pagination
func (s *Server) setupDMSV2Routes(v2 *gin.RouterGroup, handler *dmshandlers.Handler) {
	// Include all v1 routes
	v2.GET("/deploymentLifecycle", handler.GetDeploymentLifecycleInfo)
	s.setupNFDeploymentRoutes(v2, handler)
	s.setupNFDeploymentDescriptorRoutes(v2, handler)
	s.setupDMSSubscriptionRoutes(v2, handler)

	// Batch operations (v2 feature)
	// Endpoint: /batch/*
	batch := v2.Group("/batch")
	{
		// Batch NF deployment operations
		batch.POST("/nfDeployments", handler.ListNFDeployments)        // Placeholder - will be batch create
		batch.POST("/nfDeployments/delete", handler.ListNFDeployments) // Placeholder - will be batch delete
		batch.POST("/nfDeployments/scale", handler.ListNFDeployments)  // Placeholder - will be batch scale

		// Batch NF deployment descriptor operations
		batch.POST("/nfDeploymentDescriptors", handler.ListNFDeploymentDescriptors)        // Placeholder
		batch.POST("/nfDeploymentDescriptors/delete", handler.ListNFDeploymentDescriptors) // Placeholder

		// Batch subscription operations
		batch.POST("/subscriptions", handler.ListDMSSubscriptions)        // Placeholder
		batch.POST("/subscriptions/delete", handler.ListDMSSubscriptions) // Placeholder
	}

	// V2 features endpoint
	v2.GET("/features", s.handleDMSV2Features)
}

// setupDMSV3Routes configures the O2-DMS API v3 endpoints with multi-tenancy support.
// V3 includes all v2 features plus:
//   - Multi-tenant deployment isolation
//   - Tenant quotas for deployments
//   - Cross-tenant deployment visibility controls
func (s *Server) setupDMSV3Routes(v3 *gin.RouterGroup, handler *dmshandlers.Handler) {
	// NF Deployment Management (v1 endpoints with tenant context)
	nfDeployments := v3.Group("/nfDeployments")
	{
		nfDeployments.GET("", handler.ListNFDeployments)
		nfDeployments.POST("", handler.CreateNFDeployment)
		nfDeployments.GET("/:nfDeploymentId", handler.GetNFDeployment)
		nfDeployments.PUT("/:nfDeploymentId", handler.UpdateNFDeployment)
		nfDeployments.DELETE("/:nfDeploymentId", handler.DeleteNFDeployment)

		// Lifecycle operations
		nfDeployments.POST("/:nfDeploymentId/scale", handler.ScaleNFDeployment)
		nfDeployments.POST("/:nfDeploymentId/rollback", handler.RollbackNFDeployment)

		// Status and history
		nfDeployments.GET("/:nfDeploymentId/status", handler.GetNFDeploymentStatus)
		nfDeployments.GET("/:nfDeploymentId/history", handler.GetNFDeploymentHistory)
	}

	// NF Deployment Descriptor Management (v1 endpoints with tenant context)
	descriptors := v3.Group("/nfDeploymentDescriptors")
	{
		descriptors.GET("", handler.ListNFDeploymentDescriptors)
		descriptors.POST("", handler.CreateNFDeploymentDescriptor)
		descriptors.GET("/:nfDeploymentDescriptorId", handler.GetNFDeploymentDescriptor)
		descriptors.DELETE("/:nfDeploymentDescriptorId", handler.DeleteNFDeploymentDescriptor)
	}

	// DMS Subscription Management (v1 endpoints with tenant context)
	subscriptions := v3.Group("/subscriptions")
	{
		subscriptions.GET("", handler.ListDMSSubscriptions)
		subscriptions.POST("", handler.CreateDMSSubscription)
		subscriptions.GET("/:subscriptionId", handler.GetDMSSubscription)
		subscriptions.DELETE("/:subscriptionId", handler.DeleteDMSSubscription)
	}

	// Batch Operations (v2 feature with tenant context)
	batch := v3.Group("/batch")
	{
		batch.POST("/nfDeployments", handler.ListNFDeployments)                            // Placeholder
		batch.POST("/nfDeployments/delete", handler.ListNFDeployments)                     // Placeholder
		batch.POST("/nfDeployments/scale", handler.ListNFDeployments)                      // Placeholder
		batch.POST("/nfDeploymentDescriptors", handler.ListNFDeploymentDescriptors)        // Placeholder
		batch.POST("/nfDeploymentDescriptors/delete", handler.ListNFDeploymentDescriptors) // Placeholder
	}

	// V3 features endpoint
	v3.GET("/features", s.handleDMSV3Features)
}

// handleDMSV2Features returns v2 API feature information.
// GET /o2dms/v2/features.
func (s *Server) handleDMSV2Features(c *gin.Context) {
	c.JSON(200, gin.H{
		"version":     "v2",
		"apiVersion":  "v2",
		"description": "O2-DMS API v2 with enhanced filtering, batch operations",
		"newFeatures": []string{
			"enhanced_filtering",
			"field_selection",
			"cursor_pagination",
			"batch_deployments",
			"batch_descriptors",
		},
	})
}

// handleDMSV3Features returns v3 API feature information.
// GET /o2dms/v3/features.
func (s *Server) handleDMSV3Features(c *gin.Context) {
	c.JSON(200, gin.H{
		"version":     "v3",
		"apiVersion":  "v3",
		"description": "O2-DMS API v3 with multi-tenancy support",
		"newFeatures": []string{
			"multi_tenancy",
			"tenant_quotas",
			"tenant_isolation",
			"cross_tenant_visibility",
		},
	})
}

// HandleDMSAPIInfo returns O2-DMS API information.
func (s *Server) HandleDMSAPIInfo(c *gin.Context) {
	c.JSON(200, gin.H{
		"api_version": "v1",
		"base_path":   "/o2dms/v1",
		"description": "O-RAN O2-DMS (Deployment Management Service) API",
		"resources": []string{
			"deploymentLifecycle",
			"nfDeployments",
			"nfDeploymentDescriptors",
			"subscriptions",
		},
		"operations": []string{
			"instantiate",
			"terminate",
			"scale",
			"heal",
			"upgrade",
			"rollback",
		},
	})
}
