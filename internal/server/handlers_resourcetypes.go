package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleListResourceTypes lists all resource types.
// GET /o2ims/v1/resourceTypes.
func (s *Server) handleListResourceTypes(c *gin.Context) {
	s.logger.Info("listing resource types")

	// Parse filter from request (supports v1 basic and v2+ advanced filtering).
	filter, err := s.parseFilterFromRequest(c)
	if err != nil {
		s.logger.Error("failed to parse filter", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "InvalidParameter",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// List resource types via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	types, err := adp.ListResourceTypes(c.Request.Context(), filter)
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

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resType, err := adp.GetResourceType(c.Request.Context(), resourceTypeID)
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
