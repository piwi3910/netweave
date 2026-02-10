package server

import (
	"github.com/gin-gonic/gin"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
)

// setupDMSRoutes configures all O2-DMS API routes under /o2dms/v1.
// All features (batch operations, feature discovery, multi-tenancy) are consolidated in v1.
func (s *Server) setupDMSRoutes(handler *dmshandlers.Handler) {
	// O2-DMS API v1 routes on O2 router (mTLS)
	v1 := s.o2Router.Group("/o2dms/v1")
	v1.Use(PluginGuard(s.pluginRegistry, "o2dms"))

	// Apply dynamic routing middleware to resolve DMS registry per-request.
	v1.Use(s.dmsAdapterMiddleware())

	{
		s.setupDMSV1Routes(v1, handler)
	}

	// API information endpoint on admin router
	s.adminRouter.GET("/o2dms", s.HandleDMSAPIInfo)
}

// setupDMSV1Routes configures the O2-DMS API v1 endpoints with all features.
// This includes batch operations, feature discovery, and multi-tenancy support.
func (s *Server) setupDMSV1Routes(v1 *gin.RouterGroup, handler *dmshandlers.Handler) {
	// Deployment Lifecycle Information
	v1.GET("/deploymentLifecycle", handler.GetDeploymentLifecycleInfo)

	// NF Deployment Management
	s.setupNFDeploymentRoutes(v1, handler)

	// NF Deployment Descriptor Management
	s.setupNFDeploymentDescriptorRoutes(v1, handler)

	// DMS Subscription Management
	s.setupDMSSubscriptionRoutes(v1, handler)

	// Batch operations
	batch := v1.Group("/batch")
	{
		// Batch deployment operations
		batch.POST("/nfDeployments", handler.ListNFDeployments)        // Placeholder - will be batch create
		batch.POST("/nfDeployments/delete", handler.ListNFDeployments) // Placeholder - will be batch delete
		batch.POST("/nfDeployments/scale", handler.ListNFDeployments)  // Placeholder - will be batch scale

		// Batch descriptor operations
		// Placeholder - will be batch create
		batch.POST("/nfDeploymentDescriptors", handler.ListNFDeploymentDescriptors)
		// Placeholder - will be batch delete
		batch.POST("/nfDeploymentDescriptors/delete", handler.ListNFDeploymentDescriptors)
	}

	// Features endpoint
	v1.GET("/features", s.handleDMSFeatures)
}

// handleDMSFeatures returns v1 API feature information.
// GET /o2dms/v1/features.
func (s *Server) handleDMSFeatures(c *gin.Context) {
	c.JSON(200, gin.H{
		"version":     "v1",
		"apiVersion":  "v1",
		"description": "O2-DMS API v1 with batch operations, feature discovery, and multi-tenancy",
		"features": []string{
			"enhanced_filtering",
			"field_selection",
			"cursor_pagination",
			"batch_operations",
			"deployment_history",
			"multi_tenancy",
			"tenant_isolation",
			"tenant_quotas",
			"cross_tenant_visibility",
		},
		"batch_operations": []string{
			"batch_create_deployments",
			"batch_delete_deployments",
			"batch_scale_deployments",
			"batch_create_descriptors",
			"batch_delete_descriptors",
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
