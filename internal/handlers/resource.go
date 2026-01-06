package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// ResourceHandler handles Resource API endpoints.
type ResourceHandler struct {
	// TODO: Add dependencies (adapter registry, logger, etc.)
}

// NewResourceHandler creates a new ResourceHandler.
func NewResourceHandler() *ResourceHandler {
	return &ResourceHandler{}
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
	// TODO: Implement actual logic
	// 1. Parse query parameters (filters, offset, limit)
	// 2. Route to appropriate adapter(s) based on filter
	// 3. Aggregate results from multiple adapters if needed
	// 4. Apply filtering and pagination
	// 5. Return response

	// Stub: return empty list
	response := models.ListResponse{
		Items:      []models.Resource{},
		TotalCount: 0,
	}

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
func (h *ResourceHandler) GetResource(c *gin.Context) {
	resourceID := c.Param("resourceId")

	// TODO: Implement actual logic
	// 1. Validate resourceId parameter
	// 2. Route to appropriate adapter
	// 3. Get resource from adapter by ID
	// 4. Return resource if found
	// 5. Return 404 if not found

	// Stub: return 404
	c.JSON(http.StatusNotFound, models.ErrorResponse{
		Error:   "NotFound",
		Message: "Resource not found: " + resourceID,
		Code:    http.StatusNotFound,
	})
}
