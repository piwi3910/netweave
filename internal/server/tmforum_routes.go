package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/handlers"
)

// setupTMForumRoutesEarly configures TMForum API route structure during server initialization.
// Routes will return 503 until SetupDMS() is called to initialize the handler.
// This ensures routes are registered with the middleware chain at the correct time.
//
// TMF638 - Service Inventory Management v4
//   - /tmf-api/serviceInventoryManagement/v4/*
//   - Maps to O2-DMS deployments
//
// TMF639 - Resource Inventory Management v4
//   - /tmf-api/resourceInventoryManagement/v4/*
//   - Maps to O2-IMS resources and resource pools
//
// TMF641 - Service Ordering Management v4
//   - /tmf-api/serviceOrdering/v4/*
//   - Maps to O2-DMS deployment operations
func (s *Server) setupTMForumRoutesEarly() {
	s.logger.Info("Registering TMForum API route structure")

	// TMF639 - Resource Inventory Management API v4
	tmf639 := s.router.Group("/tmf-api/resourceInventoryManagement/v4")
	{
		// Resource CRUD operations
		tmf639.GET("/resource", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.ListTMF639Resources
		}))
		tmf639.GET("/resource/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.GetTMF639Resource
		}))
		tmf639.POST("/resource", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.CreateTMF639Resource
		}))
		tmf639.PATCH("/resource/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.UpdateTMF639Resource
		}))
		tmf639.DELETE("/resource/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.DeleteTMF639Resource
		}))
	}

	// TMF638 - Service Inventory Management API v4
	tmf638 := s.router.Group("/tmf-api/serviceInventoryManagement/v4")
	{
		// Service CRUD operations
		tmf638.GET("/service", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.ListTMF638Services
		}))
		tmf638.GET("/service/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.GetTMF638Service
		}))
		tmf638.POST("/service", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.CreateTMF638Service
		}))
		tmf638.PATCH("/service/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.UpdateTMF638Service
		}))
		tmf638.DELETE("/service/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.DeleteTMF638Service
		}))
	}

	// TMF641 - Service Ordering Management API v4
	tmf641 := s.router.Group("/tmf-api/serviceOrdering/v4")
	{
		// Service Order CRUD operations
		tmf641.GET("/serviceOrder", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.ListTMF641ServiceOrders
		}))
		tmf641.GET("/serviceOrder/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.GetTMF641ServiceOrder
		}))
		tmf641.POST("/serviceOrder", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.CreateTMF641ServiceOrder
		}))
		tmf641.PATCH("/serviceOrder/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.UpdateTMF641ServiceOrder
		}))
		tmf641.DELETE("/serviceOrder/:id", s.tmfHandlerOrUnavailable(func(h *handlers.TMForumHandler) gin.HandlerFunc {
			return h.DeleteTMF641ServiceOrder
		}))
	}

	s.logger.Info("TMForum API route structure registered (handlers will be available after DMS initialization)")
}

// tmfHandlerOrUnavailable returns a handler that delegates to the TMForum handler if available,
// or returns 503 Service Unavailable if DMS has not been initialized yet.
func (s *Server) tmfHandlerOrUnavailable(getHandler func(*handlers.TMForumHandler) gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.tmfHandler == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "TMForum API not available",
				"message": "DMS subsystem not initialized",
			})
			return
		}
		handler := getHandler(s.tmfHandler)
		handler(c)
	}
}
