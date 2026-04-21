package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleListDeploymentManagers lists all deployment managers.
// GET /o2ims/v1/deploymentManagers.
func (s *Server) handleListDeploymentManagers(c *gin.Context) {
	s.logger.Info("listing deployment managers")

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	dms, err := adp.ListDeploymentManagers(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list deployment managers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve deployment managers",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deploymentManagers": dms,
		"total":              len(dms),
	})
}

// handleGetDeploymentManager retrieves a specific deployment manager.
// GET /o2ims/v1/deploymentManagers/:deploymentManagerId.
func (s *Server) handleGetDeploymentManager(c *gin.Context) {
	deploymentManagerID := c.Param("deploymentManagerId")
	s.logger.Info("getting deployment manager", zap.String("deployment_manager_id", deploymentManagerID))

	// Get deployment manager via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	dm, err := adp.GetDeploymentManager(c.Request.Context(), deploymentManagerID)
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

	// List all registered deployment managers instead of looking up a hardcoded ID.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	dms, err := adp.ListDeploymentManagers(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list deployment managers for O-Cloud info", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve O-Cloud information",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if len(dms) == 0 {
		s.logger.Error("no deployment managers registered")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "No deployment managers available",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Use the first registered deployment manager for O-Cloud metadata.
	dm := dms[0]

	c.JSON(http.StatusOK, gin.H{
		"oCloudId":    dm.OCloudID,
		"name":        dm.Name,
		"description": dm.Description,
		"serviceUri":  dm.ServiceURI,
	})
}

// Tenant quota handlers
// These remain as placeholders until quota management is fully implemented
