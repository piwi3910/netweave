package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	internalmodels "github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// ResourcePoolHandler handles Resource Pool API endpoints.
type ResourcePoolHandler struct {
	adapter adapter.Adapter
	logger  *zap.Logger
}

// NewResourcePoolHandler creates a new ResourcePoolHandler.
// It requires an adapter for backend operations and a logger for structured logging.
func NewResourcePoolHandler(adp adapter.Adapter, logger *zap.Logger) *ResourcePoolHandler {
	if adp == nil {
		panic("adapter cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &ResourcePoolHandler{
		adapter: adp,
		logger:  logger,
	}
}

// ListResourcePools handles GET /o2ims/v1/resourcePools.
// Lists all available resource pools with optional filtering.
//
// Query Parameters:
//   - filter: Optional filter criteria (location, labels, etc.)
//   - offset: Pagination offset
//   - limit: Maximum number of items to return
//
// Response: 200 OK with array of ResourcePool objects.
func (h *ResourcePoolHandler) ListResourcePools(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.logger.Info("listing resource pools",
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Parse query parameters
	filter := internalmodels.ParseQueryParams(c.Request.URL.Query())

	// Convert internal filter to adapter filter with tenant context
	adapterFilter := &adapter.Filter{
		TenantID:       tenantID,
		ResourcePoolID: strings.Join(filter.ResourcePoolID, ","),
		Location:       filter.Location,
		Labels:         filter.Labels,
		Extensions:     filter.Extensions,
		Limit:          filter.Limit,
		Offset:         filter.Offset,
	}

	// Get resource pools from adapter
	pools, err := h.adapter.ListResourcePools(ctx, adapterFilter)
	if err != nil {
		h.logger.Error("failed to list resource pools",
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve resource pools",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert adapter.ResourcePool to models.ResourcePool
	resourcePools := make([]models.ResourcePool, 0, len(pools))
	for _, pool := range pools {
		resourcePools = append(resourcePools, models.ResourcePool{
			ResourcePoolID: pool.ResourcePoolID,
			Name:           pool.Name,
			Description:    pool.Description,
			Location:       pool.Location,
			OCloudID:       pool.OCloudID,
			GlobalAssetID:  pool.GlobalLocationID,
			Extensions:     pool.Extensions,
		})
	}

	response := models.ListResponse{
		Items:      resourcePools,
		TotalCount: len(resourcePools),
	}

	h.logger.Info("resource pools retrieved",
		zap.Int("count", len(resourcePools)),
	)

	c.JSON(http.StatusOK, response)
}

// GetResourcePool handles GET /o2ims/v1/resourcePools/:resourcePoolId.
// Retrieves a specific resource pool by ID.
//
// Path Parameters:
//   - resourcePoolId: Unique identifier of the resource pool
//
// Response:
//   - 200 OK: ResourcePool object
//   - 404 Not Found: Resource pool does not exist
func (h *ResourcePoolHandler) GetResourcePool(c *gin.Context) {
	ctx := c.Request.Context()
	resourcePoolID := c.Param("resourcePoolId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.logger.Info("getting resource pool",
		zap.String("resource_pool_id", resourcePoolID),
		zap.String("tenant_id", tenantID),
	)

	if resourcePoolID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Resource pool ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	pool, err := h.adapter.GetResourcePool(ctx, resourcePoolID)
	if err != nil {
		handleGetError(c, err, "Resource pool", resourcePoolID)
		return
	}

	// Verify tenant ownership (return 404 to avoid information disclosure)
	if tenantID != "" && pool.TenantID != tenantID {
		h.logger.Warn("tenant mismatch - resource pool not found for this tenant",
			zap.String("resource_pool_id", resourcePoolID),
			zap.String("tenant_id", tenantID),
			zap.String("pool_tenant_id", pool.TenantID),
		)

		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Resource pool not found: " + resourcePoolID,
			Code:    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, models.ResourcePool{
		ResourcePoolID: pool.ResourcePoolID,
		Name:           pool.Name,
		Description:    pool.Description,
		Location:       pool.Location,
		OCloudID:       pool.OCloudID,
		GlobalAssetID:  pool.GlobalLocationID,
		Extensions:     pool.Extensions,
	})
}

// CreateResourcePool handles POST /o2ims/v1/resourcePools.
// Creates a new resource pool.
//
// Request Body: ResourcePool object (without resourcePoolId)
//
// Response:
//   - 201 Created: Created ResourcePool object with generated ID
//   - 400 Bad Request: Invalid request body
//   - 409 Conflict: Resource pool with same ID already exists
func (h *ResourcePoolHandler) CreateResourcePool(c *gin.Context) {
	ctx := c.Request.Context()
	var pool models.ResourcePool

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.logger.Info("creating resource pool",
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Parse request body
	if err := c.ShouldBindJSON(&pool); err != nil {
		h.logger.Warn("invalid request body",
			zap.Error(err),
		)

		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate required fields
	if pool.Name == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Resource pool name is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Convert models.ResourcePool to adapter.ResourcePool
	adapterPool := &adapter.ResourcePool{
		ResourcePoolID:   pool.ResourcePoolID,
		TenantID:         tenantID,
		Name:             pool.Name,
		Description:      pool.Description,
		Location:         pool.Location,
		OCloudID:         pool.OCloudID,
		GlobalLocationID: pool.GlobalAssetID,
		Extensions:       pool.Extensions,
	}

	// Create resource pool via adapter
	createdPool, err := h.adapter.CreateResourcePool(ctx, adapterPool)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			h.logger.Warn("resource pool already exists",
				zap.String("name", pool.Name),
			)

			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "Conflict",
				Message: "Resource pool already exists",
				Code:    http.StatusConflict,
			})
			return
		}

		h.logger.Error("failed to create resource pool",
			zap.String("name", pool.Name),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to create resource pool",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert back to models.ResourcePool
	response := models.ResourcePool{
		ResourcePoolID: createdPool.ResourcePoolID,
		Name:           createdPool.Name,
		Description:    createdPool.Description,
		Location:       createdPool.Location,
		OCloudID:       createdPool.OCloudID,
		GlobalAssetID:  createdPool.GlobalLocationID,
		Extensions:     createdPool.Extensions,
	}

	h.logger.Info("resource pool created",
		zap.String("resource_pool_id", response.ResourcePoolID),
		zap.String("name", response.Name),
	)

	c.JSON(http.StatusCreated, response)
}

// UpdateResourcePool handles PUT /o2ims/v1/resourcePools/:resourcePoolId.
// Updates an existing resource pool.
//
// Path Parameters:
//   - resourcePoolId: Unique identifier of the resource pool
//
// Request Body: ResourcePool object with updated fields
//
// Response:
//   - 200 OK: Updated ResourcePool object
//   - 400 Bad Request: Invalid request body
//   - 404 Not Found: Resource pool does not exist
func (h *ResourcePoolHandler) UpdateResourcePool(c *gin.Context) {
	ctx := c.Request.Context()
	resourcePoolID := c.Param("resourcePoolId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.logger.Info("updating resource pool",
		zap.String("resource_pool_id", resourcePoolID),
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Validate resource pool ID
	if resourcePoolID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Resource pool ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var pool models.ResourcePool
	if err := c.ShouldBindJSON(&pool); err != nil {
		h.logger.Warn("invalid request body",
			zap.Error(err),
		)

		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// First verify tenant ownership
	existingPool, err := h.adapter.GetResourcePool(ctx, resourcePoolID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("resource pool not found",
				zap.String("resource_pool_id", resourcePoolID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Resource pool not found: " + resourcePoolID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get resource pool for tenant verification",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to update resource pool",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Verify tenant ownership (return 404 to avoid information disclosure)
	if tenantID != "" && existingPool.TenantID != tenantID {
		h.logger.Warn("tenant mismatch - cannot update resource pool from different tenant",
			zap.String("resource_pool_id", resourcePoolID),
			zap.String("tenant_id", tenantID),
			zap.String("pool_tenant_id", existingPool.TenantID),
		)

		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Resource pool not found: " + resourcePoolID,
			Code:    http.StatusNotFound,
		})
		return
	}

	// Convert models.ResourcePool to adapter.ResourcePool
	adapterPool := &adapter.ResourcePool{
		ResourcePoolID:   pool.ResourcePoolID,
		TenantID:         tenantID,
		Name:             pool.Name,
		Description:      pool.Description,
		Location:         pool.Location,
		OCloudID:         pool.OCloudID,
		GlobalLocationID: pool.GlobalAssetID,
		Extensions:       pool.Extensions,
	}

	// Update resource pool via adapter
	updatedPool, err := h.adapter.UpdateResourcePool(ctx, resourcePoolID, adapterPool)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("resource pool not found",
				zap.String("resource_pool_id", resourcePoolID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Resource pool not found: " + resourcePoolID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to update resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to update resource pool",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert back to models.ResourcePool
	response := models.ResourcePool{
		ResourcePoolID: updatedPool.ResourcePoolID,
		Name:           updatedPool.Name,
		Description:    updatedPool.Description,
		Location:       updatedPool.Location,
		OCloudID:       updatedPool.OCloudID,
		GlobalAssetID:  updatedPool.GlobalLocationID,
		Extensions:     updatedPool.Extensions,
	}

	h.logger.Info("resource pool updated",
		zap.String("resource_pool_id", resourcePoolID),
	)

	c.JSON(http.StatusOK, response)
}

// DeleteResourcePool handles DELETE /o2ims/v1/resourcePools/:resourcePoolId.
// Deletes a resource pool.
//
// Path Parameters:
//   - resourcePoolId: Unique identifier of the resource pool
//
// Response:
//   - 204 No Content: Resource pool deleted successfully
//   - 404 Not Found: Resource pool does not exist
//   - 409 Conflict: Resource pool cannot be deleted (has active resources)
func (h *ResourcePoolHandler) DeleteResourcePool(c *gin.Context) {
	ctx := c.Request.Context()
	resourcePoolID := c.Param("resourcePoolId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.logger.Info("deleting resource pool",
		zap.String("resource_pool_id", resourcePoolID),
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Validate resource pool ID
	if resourcePoolID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Resource pool ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// First verify tenant ownership
	existingPool, err := h.adapter.GetResourcePool(ctx, resourcePoolID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("resource pool not found",
				zap.String("resource_pool_id", resourcePoolID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Resource pool not found: " + resourcePoolID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get resource pool for tenant verification",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete resource pool",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Verify tenant ownership (return 404 to avoid information disclosure)
	if tenantID != "" && existingPool.TenantID != tenantID {
		h.logger.Warn("tenant mismatch - cannot delete resource pool from different tenant",
			zap.String("resource_pool_id", resourcePoolID),
			zap.String("tenant_id", tenantID),
			zap.String("pool_tenant_id", existingPool.TenantID),
		)

		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Resource pool not found: " + resourcePoolID,
			Code:    http.StatusNotFound,
		})
		return
	}

	// Delete resource pool via adapter
	err = h.adapter.DeleteResourcePool(ctx, resourcePoolID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Warn("resource pool not found",
				zap.String("resource_pool_id", resourcePoolID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Resource pool not found: " + resourcePoolID,
				Code:    http.StatusNotFound,
			})
			return
		}

		if strings.Contains(err.Error(), "has active resources") || strings.Contains(err.Error(), "conflict") {
			h.logger.Warn("resource pool has active resources",
				zap.String("resource_pool_id", resourcePoolID),
			)

			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "Conflict",
				Message: "Resource pool cannot be deleted: has active resources",
				Code:    http.StatusConflict,
			})
			return
		}

		h.logger.Error("failed to delete resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete resource pool",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("resource pool deleted",
		zap.String("resource_pool_id", resourcePoolID),
	)

	c.Status(http.StatusNoContent)
}
