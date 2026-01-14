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
	Adapter adapter.Adapter // Exported for testing
	Logger  *zap.Logger     // Exported for testing
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
		Adapter: adp,
		Logger:  logger,
	}
}

// handleGetError handles errors in Get* endpoints with standard error responses.
func handleGetError(c *gin.Context, err error, entityType, entityID string) {
	if strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: entityType + " not found: " + entityID,
			Code:    http.StatusNotFound,
		})
	} else {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve " + entityType,
			Code:    http.StatusInternalServerError,
		})
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
// Response: 200 OK with array of Resource objects.
func (h *ResourceHandler) ListResources(c *gin.Context) {
	ctx := c.Request.Context()

	h.Logger.Info("listing resources",
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
	resources, err := h.Adapter.ListResources(ctx, adapterFilter)
	if err != nil {
		h.Logger.Error("failed to list resources",
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

	h.Logger.Info("resources retrieved",
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
	resourceID := c.Param("resourceId")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Resource ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	resource, err := h.Adapter.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		handleGetError(c, err, "Resource", resourceID)
		return
	}

	c.JSON(http.StatusOK, models.Resource{
		ResourceID:     resource.ResourceID,
		ResourceTypeID: resource.ResourceTypeID,
		ResourcePoolID: resource.ResourcePoolID,
		Name:           resource.ResourceID,
		Description:    resource.Description,
		GlobalAssetID:  resource.GlobalAssetID,
		Extensions:     resource.Extensions,
	})
}
