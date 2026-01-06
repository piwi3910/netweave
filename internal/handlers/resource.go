package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	internalmodels "github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// ResourceHandler handles Resource API endpoints.
type ResourceHandler struct {
	adapter adapter.Adapter
	logger  *zap.Logger
}

// NewResourceHandler creates a new ResourceHandler.
// It requires an adapter for backend operations and a logger for structured logging.
func NewResourceHandler(adp adapter.Adapter, logger *zap.Logger) *ResourceHandler {
	if adp == nil {
		panic("adapter cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &ResourceHandler{
		adapter: adp,
		logger:  logger,
	}
}

// ListResources handles GET /o2ims/v1/resources.
// Lists all available infrastructure resources with optional filtering.
//
// Query Parameters:
//   - resourcePoolId: Filter by resource pool ID
//   - resourceTypeId: Filter by resource type ID
//   - filter: Additional filter criteria
//   - offset: Pagination offset
//   - limit: Maximum number of items to return
//
// Response: 200 OK with array of Resource objects
func (h *ResourceHandler) ListResources(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.Info("listing resources",
		zap.String("request_id", c.GetString("request_id")),
	)

	// Parse query parameters
	filter := internalmodels.ParseQueryParams(c.Request.URL.Query())

	// Convert internal filter to adapter filter
	adapterFilter := &adapter.Filter{
		ResourcePoolID: strings.Join(filter.ResourcePoolID, ","),
		ResourceTypeID: strings.Join(filter.ResourceTypeID, ","),
		Location:       filter.Location,
		Labels:         filter.Labels,
		Extensions:     filter.Extensions,
		Limit:          filter.Limit,
		Offset:         filter.Offset,
	}

	// Get resources from adapter
	resources, err := h.adapter.ListResources(ctx, adapterFilter)
	if err != nil {
		h.logger.Error("failed to list resources",
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve resources",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert adapter.Resource to models.Resource
	resourceList := make([]models.Resource, 0, len(resources))
	for _, resource := range resources {
		resourceList = append(resourceList, models.Resource{
			ResourceID:     resource.ResourceID,
			ResourceTypeID: resource.ResourceTypeID,
			ResourcePoolID: resource.ResourcePoolID,
			Name:           resource.ResourceID, // Use ResourceID as name if not provided
			Description:    resource.Description,
			GlobalAssetID:  resource.GlobalAssetID,
			Extensions:     resource.Extensions,
		})
	}

	response := models.ListResponse{
		Items:      resourceList,
		TotalCount: len(resourceList),
	}

	h.logger.Info("resources retrieved",
		zap.Int("count", len(resourceList)),
	)

	c.JSON(http.StatusOK, response)
}

// GetResource handles GET /o2ims/v1/resources/:resourceId.
// Retrieves a specific infrastructure resource by ID.
//
// Path Parameters:
//   - resourceId: Unique identifier of the resource
//
// Response:
//   - 200 OK: Resource object
//   - 404 Not Found: Resource does not exist
//   - 500 Internal Server Error: Server error occurred
func (h *ResourceHandler) GetResource(c *gin.Context) {
	ctx := c.Request.Context()
	resourceID := c.Param("resourceId")

	h.logger.Info("getting resource",
		zap.String("resource_id", resourceID),
		zap.String("request_id", c.GetString("request_id")),
	)

	// Validate resource ID
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Resource ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get resource from adapter
	resource, err := h.adapter.GetResource(ctx, resourceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("resource not found",
				zap.String("resource_id", resourceID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Resource not found: " + resourceID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve resource",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert adapter.Resource to models.Resource
	response := models.Resource{
		ResourceID:     resource.ResourceID,
		ResourceTypeID: resource.ResourceTypeID,
		ResourcePoolID: resource.ResourcePoolID,
		Name:           resource.ResourceID, // Use ResourceID as name if not provided
		Description:    resource.Description,
		GlobalAssetID:  resource.GlobalAssetID,
		Extensions:     resource.Extensions,
	}

	h.logger.Info("resource retrieved",
		zap.String("resource_id", resourceID),
	)

	c.JSON(http.StatusOK, response)
}
