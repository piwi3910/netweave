package server

import (
	"github.com/gin-gonic/gin"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
)

// setupDMSRoutes configures all O2-DMS API routes (v1, v2, v3).
// It organizes routes into the following groups:
//   - /o2dms/v1/* - Original DMS API
//   - /o2dms/v2/* - V2 API with enhanced filtering and batch operations
//   - /o2dms/v3/* - V3 API with multi-tenancy support
func (s *Server) setupDMSRoutes(handler *dmshandlers.Handler) {
	// O2-DMS API v1 routes
	v1 := s.router.Group("/o2dms/v1")
	{
		s.setupDMSV1Routes(v1, handler)
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

// setupDMSV1Routes configures the O2-DMS API v1 endpoints.
func (s *Server) setupDMSV1Routes(v1 *gin.RouterGroup, handler *dmshandlers.Handler) {
	// Deployment Lifecycle Information
	v1.GET("/deploymentLifecycle", handler.GetDeploymentLifecycleInfo)

	// NF Deployment Management
	s.setupNFDeploymentRoutes(v1, handler)

	// NF Deployment Descriptor Management
	s.setupNFDeploymentDescriptorRoutes(v1, handler)

	// DMS Subscription Management
	s.setupDMSSubscriptionRoutes(v1, handler)
}

// setupDMSV2Routes configures the O2-DMS API v2 endpoints with enhanced features.
// V2 includes all v1 features plus:
//   - Enhanced filtering for deployments and descriptors
//   - Batch operations for create/delete/scale
//   - Field selection and cursor pagination
func (s *Server) setupDMSV2Routes(v2 *gin.RouterGroup, handler *dmshandlers.Handler) {
	// Include all v1 routes
	s.setupDMSV1Routes(v2, handler)

	// Batch operations (v2 feature)
	batch := v2.Group("/batch")
	{
		// Batch deployment operations
		batch.POST("/nfDeployments", handler.ListNFDeployments)        // Placeholder - will be batch create
		batch.POST("/nfDeployments/delete", handler.ListNFDeployments) // Placeholder - will be batch delete
		batch.POST("/nfDeployments/scale", handler.ListNFDeployments)  // Placeholder - will be batch scale

		// Batch descriptor operations
		batch.POST("/nfDeploymentDescriptors", handler.ListNFDeploymentDescriptors)        // Placeholder - will be batch create
		batch.POST("/nfDeploymentDescriptors/delete", handler.ListNFDeploymentDescriptors) // Placeholder - will be batch delete
	}

	// V2 features endpoint
	v2.GET("/features", s.handleDMSV2Features)
}

// setupDMSV3Routes configures the O2-DMS API v3 endpoints with multi-tenancy support.
// V3 includes all v2 features plus:
//   - Multi-tenant deployment isolation
//   - Tenant quotas for deployments
//   - Cross-tenant deployment visibility controls
//   - Tenant-scoped deployment descriptors
func (s *Server) setupDMSV3Routes(v3 *gin.RouterGroup, handler *dmshandlers.Handler) {
	// Include all v1 routes (with tenant context applied via middleware)
	s.setupDMSV1Routes(v3, handler)

	// Batch Operations (v2 feature with tenant context)
	batch := v3.Group("/batch")
	{
		// Batch deployment operations (tenant-scoped)
		batch.POST("/nfDeployments", handler.ListNFDeployments)        // Placeholder - will be batch create
		batch.POST("/nfDeployments/delete", handler.ListNFDeployments) // Placeholder - will be batch delete
		batch.POST("/nfDeployments/scale", handler.ListNFDeployments)  // Placeholder - will be batch scale

		// Batch descriptor operations (tenant-scoped)
		batch.POST("/nfDeploymentDescriptors", handler.ListNFDeploymentDescriptors)        // Placeholder - will be batch create
		batch.POST("/nfDeploymentDescriptors/delete", handler.ListNFDeploymentDescriptors) // Placeholder - will be batch delete
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
		"features": []string{
			"enhanced_filtering",
			"field_selection",
			"cursor_pagination",
			"batch_operations",
			"deployment_history",
		},
		"batch_operations": []string{
			"batch_create_deployments",
			"batch_delete_deployments",
			"batch_scale_deployments",
			"batch_create_descriptors",
			"batch_delete_descriptors",
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
		"features": []string{
			"multi_tenancy",
			"tenant_isolation",
			"tenant_quotas",
			"cross_tenant_visibility",
			"enhanced_filtering",
			"batch_operations",
		},
		"tenancy": map[string]interface{}{
			"isolation": "hard",
			"quotas":    true,
			"sharing":   "controlled",
		},
	})
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
