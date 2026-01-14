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

// handleDMSAPIInfo returns O2-DMS API information.
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
