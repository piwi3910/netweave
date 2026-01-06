// Package handlers provides HTTP handlers for O2-IMS API endpoints.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	internalmodels "github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// DeploymentManagerHandler handles Deployment Manager API endpoints.
type DeploymentManagerHandler struct {
	adapter adapter.Adapter
	logger  *zap.Logger
}

// NewDeploymentManagerHandler creates a new DeploymentManagerHandler.
// It requires an adapter for backend operations and a logger for structured logging.
func NewDeploymentManagerHandler(adp adapter.Adapter, logger *zap.Logger) *DeploymentManagerHandler {
	if adp == nil {
		panic("adapter cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &DeploymentManagerHandler{
		adapter: adp,
		logger:  logger,
	}
}

// ListDeploymentManagers handles GET /o2ims/v1/deploymentManagers.
// Lists all available deployment managers (Kubernetes clusters).
//
// Query Parameters:
//   - filter: Optional filter criteria
//   - offset: Pagination offset
//   - limit: Maximum number of items to return
//
// Response: 200 OK with array of DeploymentManager objects
func (h *DeploymentManagerHandler) ListDeploymentManagers(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.Info("listing deployment managers",
		zap.String("request_id", c.GetString("request_id")),
	)

	// Parse query parameters
	filter := internalmodels.ParseQueryParams(c.Request.URL.Query())

	// For deployment managers, we typically return a single manager
	// representing the current cluster. In multi-cluster setups,
	// this would iterate through all registered adapters.
	deploymentManagers := []models.DeploymentManager{}

	// Get deployment manager from adapter
	// Note: The adapter interface doesn't have ListDeploymentManagers,
	// so we'll get the single deployment manager
	dm, err := h.adapter.GetDeploymentManager(ctx, "")
	if err != nil {
		h.logger.Error("failed to get deployment manager",
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve deployment managers",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert adapter.DeploymentManager to models.DeploymentManager
	deploymentManagers = append(deploymentManagers, models.DeploymentManager{
		DeploymentManagerID: dm.DeploymentManagerID,
		Name:                dm.Name,
		Description:         dm.Description,
		OCloudID:            dm.OCloudID,
		ServiceURI:          dm.ServiceURI,
		SupportedLocations:  dm.SupportedLocations,
		Capabilities:        dm.Capabilities,
		Extensions:          dm.Extensions,
	})

	// Apply pagination
	totalCount := len(deploymentManagers)
	start := filter.Offset
	end := start + filter.Limit

	if start > len(deploymentManagers) {
		start = len(deploymentManagers)
	}
	if end > len(deploymentManagers) {
		end = len(deploymentManagers)
	}

	pagedManagers := deploymentManagers[start:end]

	response := models.ListResponse{
		Items:      pagedManagers,
		TotalCount: totalCount,
	}

	h.logger.Info("deployment managers retrieved",
		zap.Int("count", len(pagedManagers)),
		zap.Int("total", totalCount),
	)

	c.JSON(http.StatusOK, response)
}

// GetDeploymentManager handles GET /o2ims/v1/deploymentManagers/:deploymentManagerId.
// Retrieves a specific deployment manager by ID.
//
// Path Parameters:
//   - deploymentManagerId: Unique identifier of the deployment manager
//
// Response:
//   - 200 OK: DeploymentManager object
//   - 404 Not Found: Deployment manager does not exist
//   - 500 Internal Server Error: Server error occurred
func (h *DeploymentManagerHandler) GetDeploymentManager(c *gin.Context) {
	ctx := c.Request.Context()
	deploymentManagerID := c.Param("deploymentManagerId")

	h.logger.Info("getting deployment manager",
		zap.String("deployment_manager_id", deploymentManagerID),
		zap.String("request_id", c.GetString("request_id")),
	)

	// Validate deployment manager ID
	if deploymentManagerID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Deployment manager ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get deployment manager from adapter
	dm, err := h.adapter.GetDeploymentManager(ctx, deploymentManagerID)
	if err != nil {
		// Check if it's a "not found" error
		if err.Error() == "deployment manager not found" {
			h.logger.Warn("deployment manager not found",
				zap.String("deployment_manager_id", deploymentManagerID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Deployment manager not found: " + deploymentManagerID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get deployment manager",
			zap.String("deployment_manager_id", deploymentManagerID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve deployment manager",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert adapter.DeploymentManager to models.DeploymentManager
	response := models.DeploymentManager{
		DeploymentManagerID: dm.DeploymentManagerID,
		Name:                dm.Name,
		Description:         dm.Description,
		OCloudID:            dm.OCloudID,
		ServiceURI:          dm.ServiceURI,
		SupportedLocations:  dm.SupportedLocations,
		Capabilities:        dm.Capabilities,
		Extensions:          dm.Extensions,
	}

	h.logger.Info("deployment manager retrieved",
		zap.String("deployment_manager_id", deploymentManagerID),
	)

	c.JSON(http.StatusOK, response)
}
