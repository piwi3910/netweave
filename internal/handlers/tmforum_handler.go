package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	imsadapter "github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/models"
	"go.uber.org/zap"
)

// TMForumHandler handles TMForum API requests and translates them to internal O2-IMS/O2-DMS operations.
// It provides TMF638 (Service Inventory) and TMF639 (Resource Inventory) API implementations.
type TMForumHandler struct {
	adapter     imsadapter.Adapter
	dmsRegistry *registry.Registry
	logger      *zap.Logger
}

// NewTMForumHandler creates a new TMForum API handler.
func NewTMForumHandler(
	adp imsadapter.Adapter,
	dmsReg *registry.Registry,
	logger *zap.Logger,
) *TMForumHandler {
	return &TMForumHandler{
		adapter:     adp,
		dmsRegistry: dmsReg,
		logger:      logger,
	}
}

// ========================================
// TMF639 - Resource Inventory Management
// ========================================

// ListTMF639Resources lists all TMF639 resources (maps to O2-IMS Resource Pools + Resources).
// GET /tmf-api/resourceInventoryManagement/v4/resource
func (h *TMForumHandler) ListTMF639Resources(c *gin.Context) {
	ctx := c.Request.Context()

	// Get query parameters
	category := c.Query("resourceSpecification.category")

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	var resources []interface{}

	// If category is "resourcePool" or empty, include resource pools
	if category == "" || category == "resourcePool" {
		pools, err := h.adapter.ListResourcePools(ctx, nil)
		if err != nil {
			h.logger.Error("failed to list resource pools",
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to retrieve resource pools",
			})
			return
		}

		for _, pool := range pools {
			tmfResource := TransformResourcePoolToTMF639Resource(pool, baseURL)
			resources = append(resources, tmfResource)
		}
	}

	// If category is not "resourcePool", include individual resources
	if category != "resourcePool" {
		resourceList, err := h.adapter.ListResources(ctx, nil)
		if err != nil {
			h.logger.Error("failed to list resources",
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to retrieve resources",
			})
			return
		}

		for _, res := range resourceList {
			tmfResource := TransformResourceToTMF639Resource(res, baseURL)
			resources = append(resources, tmfResource)
		}
	}

	c.JSON(http.StatusOK, resources)
}

// GetTMF639Resource retrieves a single TMF639 resource by ID.
// GET /tmf-api/resourceInventoryManagement/v4/resource/:id
func (h *TMForumHandler) GetTMF639Resource(c *gin.Context) {
	ctx := c.Request.Context()
	resourceID := c.Param("id")

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Try to get as resource pool first
	pool, err := h.adapter.GetResourcePool(ctx, resourceID)
	if err == nil {
		tmfResource := TransformResourcePoolToTMF639Resource(pool, baseURL)
		c.JSON(http.StatusOK, tmfResource)
		return
	}

	// Try to get as individual resource
	resource, err := h.adapter.GetResource(ctx, resourceID)
	if err != nil {
		if err == imsadapter.ErrResourceNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": fmt.Sprintf("Resource with ID '%s' not found", resourceID),
			})
			return
		}

		h.logger.Error("failed to get resource",
			zap.String("resourceId", resourceID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource",
		})
		return
	}

	tmfResource := TransformResourceToTMF639Resource(resource, baseURL)
	c.JSON(http.StatusOK, tmfResource)
}

// CreateTMF639Resource creates a new TMF639 resource.
// POST /tmf-api/resourceInventoryManagement/v4/resource
func (h *TMForumHandler) CreateTMF639Resource(c *gin.Context) {
	ctx := c.Request.Context()

	var createReq models.TMF639ResourceCreate
	if err := c.ShouldBindJSON(&createReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Convert to TMF639Resource for transformation
	tmfResource := &models.TMF639Resource{
		Name:                   createReq.Name,
		Description:            createReq.Description,
		Category:               createReq.Category,
		ResourceCharacteristic: createReq.ResourceCharacteristic,
		ResourceStatus:         createReq.ResourceStatus,
		OperationalState:       createReq.OperationalState,
		Place:                  createReq.Place,
		RelatedParty:           createReq.RelatedParty,
		ResourceSpecification:  createReq.ResourceSpecification,
		ResourceRelationship:   createReq.ResourceRelationship,
		Note:                   createReq.Note,
		ValidFor:               createReq.ValidFor,
		AtBaseType:             createReq.AtBaseType,
		AtSchemaLocation:       createReq.AtSchemaLocation,
		AtType:                 createReq.AtType,
	}

	// Determine if this is a resource pool or individual resource based on category
	if createReq.Category == "resourcePool" || createReq.Category == "" {
		// Create as resource pool
		pool := TransformTMF639ResourceToResourcePool(tmfResource)

		createdPool, err := h.adapter.CreateResourcePool(ctx, pool)
		if err != nil {
			h.logger.Error("failed to create resource pool",
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to create resource pool",
			})
			return
		}

		tmfResponse := TransformResourcePoolToTMF639Resource(createdPool, baseURL)
		c.JSON(http.StatusCreated, tmfResponse)
	} else {
		// Create as individual resource
		resource := TransformTMF639ResourceToResource(tmfResource)

		createdResource, err := h.adapter.CreateResource(ctx, resource)
		if err != nil {
			h.logger.Error("failed to create resource",
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to create resource",
			})
			return
		}

		tmfResponse := TransformResourceToTMF639Resource(createdResource, baseURL)
		c.JSON(http.StatusCreated, tmfResponse)
	}
}

// UpdateTMF639Resource updates an existing TMF639 resource (PATCH).
// PATCH /tmf-api/resourceInventoryManagement/v4/resource/:id
func (h *TMForumHandler) UpdateTMF639Resource(c *gin.Context) {
	ctx := c.Request.Context()
	resourceID := c.Param("id")

	var updateReq models.TMF639ResourceUpdate
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Try to update as resource pool first
	pool, err := h.adapter.GetResourcePool(ctx, resourceID)
	if err == nil {
		applyTMF639ResourceUpdate(pool, &updateReq)

		updatedPool, err := h.adapter.UpdateResourcePool(ctx, resourceID, pool)
		if err != nil {
			h.logger.Error("failed to update resource pool",
				zap.String("resourcePoolId", resourceID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to update resource pool",
			})
			return
		}

		tmfResponse := TransformResourcePoolToTMF639Resource(updatedPool, baseURL)
		c.JSON(http.StatusOK, tmfResponse)
		return
	}

	// Resource pool not found, return 404
	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Resource with ID '%s' not found", resourceID),
	})
}

// DeleteTMF639Resource deletes a TMF639 resource.
// DELETE /tmf-api/resourceInventoryManagement/v4/resource/:id
func (h *TMForumHandler) DeleteTMF639Resource(c *gin.Context) {
	ctx := c.Request.Context()
	resourceID := c.Param("id")

	// Try to delete as resource pool first
	err := h.adapter.DeleteResourcePool(ctx, resourceID)
	if err == nil {
		c.Status(http.StatusNoContent)
		return
	}

	// Try to delete as individual resource
	err = h.adapter.DeleteResource(ctx, resourceID)
	if err != nil {
		if err == imsadapter.ErrResourceNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": fmt.Sprintf("Resource with ID '%s' not found", resourceID),
			})
			return
		}

		h.logger.Error("failed to delete resource",
			zap.String("resourceId", resourceID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to delete resource",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// ========================================
// TMF638 - Service Inventory Management
// ========================================

// ListTMF638Services lists all TMF638 services (maps to O2-DMS Deployments).
// GET /tmf-api/serviceInventoryManagement/v4/service
func (h *TMForumHandler) ListTMF638Services(c *gin.Context) {
	ctx := c.Request.Context()

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// List all deployments from all adapters
	adapters := h.dmsRegistry.List()
	var services []*models.TMF638Service

	for _, dmsAdapter := range adapters {
		deployments, err := dmsAdapter.ListDeployments(ctx, nil)
		if err != nil {
			h.logger.Warn("failed to list deployments from adapter",
				zap.String("adapter", dmsAdapter.Name()),
				zap.Error(err),
			)
			continue
		}

		for _, dep := range deployments {
			tmfService := TransformDeploymentToTMF638Service(dep, baseURL)
			services = append(services, tmfService)
		}
	}

	c.JSON(http.StatusOK, services)
}

// GetTMF638Service retrieves a single TMF638 service by ID.
// GET /tmf-api/serviceInventoryManagement/v4/service/:id
func (h *TMForumHandler) GetTMF638Service(c *gin.Context) {
	ctx := c.Request.Context()
	serviceID := c.Param("id")

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Try to find deployment across all adapters
	adapters := h.dmsRegistry.List()
	for _, dmsAdapter := range adapters {
		dep, err := dmsAdapter.GetDeployment(ctx, serviceID)
		if err == nil {
			tmfService := TransformDeploymentToTMF638Service(dep, baseURL)
			c.JSON(http.StatusOK, tmfService)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Service with ID '%s' not found", serviceID),
	})
}

// CreateTMF638Service creates a new TMF638 service (deploys via O2-DMS).
// POST /tmf-api/serviceInventoryManagement/v4/service
func (h *TMForumHandler) CreateTMF638Service(c *gin.Context) {
	ctx := c.Request.Context()

	var createReq models.TMF638ServiceCreate
	if err := c.ShouldBindJSON(&createReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Transform to deployment request
	deploymentReq := TransformTMF638ServiceToDeployment(&createReq)

	// Create deployment using default DMS adapter
	dmsAdapter := h.dmsRegistry.GetDefault()
	if dmsAdapter == nil {
		h.logger.Error("no default DMS adapter available")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "No DMS adapter available for deployment",
		})
		return
	}

	deployment, err := dmsAdapter.CreateDeployment(ctx, deploymentReq)
	if err != nil {
		h.logger.Error("failed to create deployment",
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create service",
		})
		return
	}

	tmfService := TransformDeploymentToTMF638Service(deployment, baseURL)
	c.JSON(http.StatusCreated, tmfService)
}

// UpdateTMF638Service updates an existing TMF638 service (PATCH).
// PATCH /tmf-api/serviceInventoryManagement/v4/service/:id
func (h *TMForumHandler) UpdateTMF638Service(c *gin.Context) {
	ctx := c.Request.Context()
	serviceID := c.Param("id")

	var updateReq models.TMF638ServiceUpdate
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Find deployment across all adapters
	adapters := h.dmsRegistry.List()
	for _, dmsAdapter := range adapters {
		dep, err := dmsAdapter.GetDeployment(ctx, serviceID)
		if err != nil {
			continue
		}

		// Apply updates
		applyTMF638ServiceUpdate(dep, &updateReq)

		// Note: DMS adapters don't have direct update method
		// Service state changes are typically done via specific operations (scale, rollback, etc.)
		// For now, we return the current state with applied changes
		tmfService := TransformDeploymentToTMF638Service(dep, baseURL)
		c.JSON(http.StatusOK, tmfService)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Service with ID '%s' not found", serviceID),
	})
}

// DeleteTMF638Service deletes a TMF638 service (undeploys via O2-DMS).
// DELETE /tmf-api/serviceInventoryManagement/v4/service/:id
func (h *TMForumHandler) DeleteTMF638Service(c *gin.Context) {
	ctx := c.Request.Context()
	serviceID := c.Param("id")

	// Try to delete deployment from all adapters
	adapters := h.dmsRegistry.List()
	for _, dmsAdapter := range adapters {
		err := dmsAdapter.DeleteDeployment(ctx, serviceID)
		if err == nil {
			c.Status(http.StatusNoContent)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Service with ID '%s' not found", serviceID),
	})
}

// ========================================
// TMF641 - Service Ordering Management
// ========================================

// ListTMF641ServiceOrders lists all TMF641 service orders.
// GET /tmf-api/serviceOrdering/v4/serviceOrder
func (h *TMForumHandler) ListTMF641ServiceOrders(c *gin.Context) {
	ctx := c.Request.Context()

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Query parameters for filtering
	state := c.Query("state")
	externalID := c.Query("externalId")

	// List all deployments and convert to service orders
	adapters := h.dmsRegistry.List()
	var orders []*models.TMF641ServiceOrder

	for _, dmsAdapter := range adapters {
		deployments, err := dmsAdapter.ListDeployments(ctx, nil)
		if err != nil {
			h.logger.Warn("failed to list deployments from adapter",
				zap.String("adapter", dmsAdapter.Name()),
				zap.Error(err),
			)
			continue
		}

		for _, dep := range deployments {
			order := TransformDeploymentToTMF641ServiceOrder(dep, baseURL)

			// Apply filters
			if state != "" && order.State != state {
				continue
			}
			if externalID != "" && order.ExternalId != externalID {
				continue
			}

			orders = append(orders, order)
		}
	}

	c.JSON(http.StatusOK, orders)
}

// GetTMF641ServiceOrder retrieves a single TMF641 service order by ID.
// GET /tmf-api/serviceOrdering/v4/serviceOrder/:id
func (h *TMForumHandler) GetTMF641ServiceOrder(c *gin.Context) {
	ctx := c.Request.Context()
	orderID := c.Param("id")

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Try to find deployment across all adapters
	adapters := h.dmsRegistry.List()
	for _, dmsAdapter := range adapters {
		dep, err := dmsAdapter.GetDeployment(ctx, orderID)
		if err == nil {
			order := TransformDeploymentToTMF641ServiceOrder(dep, baseURL)
			c.JSON(http.StatusOK, order)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Service order with ID '%s' not found", orderID),
	})
}

// CreateTMF641ServiceOrder creates a new TMF641 service order.
// POST /tmf-api/serviceOrdering/v4/serviceOrder
func (h *TMForumHandler) CreateTMF641ServiceOrder(c *gin.Context) {
	ctx := c.Request.Context()

	var createReq models.TMF641ServiceOrderCreate
	if err := c.ShouldBindJSON(&createReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Validate service order items
	if len(createReq.ServiceOrderItem) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Service order must contain at least one item",
		})
		return
	}

	// Get default DMS adapter
	dmsAdapter := h.dmsRegistry.GetDefault()
	if dmsAdapter == nil {
		h.logger.Error("no default DMS adapter available")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "No DMS adapter available for service ordering",
		})
		return
	}

	// Process first service order item (simplified implementation)
	// In a full implementation, we would process all items and handle dependencies
	firstItem := createReq.ServiceOrderItem[0]

	deploymentReq := TransformTMF641ServiceOrderToDeployment(&createReq, &firstItem)

	deployment, err := dmsAdapter.CreateDeployment(ctx, deploymentReq)
	if err != nil {
		h.logger.Error("failed to create deployment for service order",
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create service order",
		})
		return
	}

	order := TransformDeploymentToTMF641ServiceOrder(deployment, baseURL)
	c.JSON(http.StatusCreated, order)
}

// UpdateTMF641ServiceOrder updates an existing TMF641 service order (PATCH).
// PATCH /tmf-api/serviceOrdering/v4/serviceOrder/:id
func (h *TMForumHandler) UpdateTMF641ServiceOrder(c *gin.Context) {
	ctx := c.Request.Context()
	orderID := c.Param("id")

	var updateReq models.TMF641ServiceOrderUpdate
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	baseURL := buildBaseURL(c.Request.URL.Scheme, c.Request.Host)

	// Find deployment across all adapters
	adapters := h.dmsRegistry.List()
	for _, dmsAdapter := range adapters {
		dep, err := dmsAdapter.GetDeployment(ctx, orderID)
		if err != nil {
			continue
		}

		// Apply updates
		applyTMF641ServiceOrderUpdate(dep, &updateReq)

		// Return updated order
		order := TransformDeploymentToTMF641ServiceOrder(dep, baseURL)
		c.JSON(http.StatusOK, order)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Service order with ID '%s' not found", orderID),
	})
}

// DeleteTMF641ServiceOrder deletes (cancels) a TMF641 service order.
// DELETE /tmf-api/serviceOrdering/v4/serviceOrder/:id
func (h *TMForumHandler) DeleteTMF641ServiceOrder(c *gin.Context) {
	ctx := c.Request.Context()
	orderID := c.Param("id")

	// Try to delete deployment from all adapters
	adapters := h.dmsRegistry.List()
	for _, dmsAdapter := range adapters {
		err := dmsAdapter.DeleteDeployment(ctx, orderID)
		if err == nil {
			c.Status(http.StatusNoContent)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Service order with ID '%s' not found", orderID),
	})
}

// ========================================
// TMF688 - Event Management
// ========================================

// ListTMF688Events lists all TMF688 events.
// GET /tmf-api/eventManagement/v4/event
func (h *TMForumHandler) ListTMF688Events(c *gin.Context) {
	// Events are typically not stored but generated on-demand
	// This could list recent events from a cache or event store
	// For now, return empty array as events are pushed to subscribers
	c.JSON(http.StatusOK, []models.TMF688Event{})
}

// GetTMF688Event retrieves a single TMF688 event by ID.
// GET /tmf-api/eventManagement/v4/event/:id
func (h *TMForumHandler) GetTMF688Event(c *gin.Context) {
	eventID := c.Param("id")

	// Events are typically not stored
	c.JSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": fmt.Sprintf("Event with ID '%s' not found", eventID),
	})
}

// CreateTMF688Event creates a new TMF688 event (typically for testing).
// POST /tmf-api/eventManagement/v4/event
func (h *TMForumHandler) CreateTMF688Event(c *gin.Context) {
	var createReq models.TMF688EventCreate
	if err := c.ShouldBindJSON(&createReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// In a real implementation, this would publish the event to subscribers
	// For now, return 501 Not Implemented
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "NotImplemented",
		"message": "Event creation not yet implemented",
	})
}

// RegisterTMF688Hub registers a hub for event notifications.
// POST /tmf-api/eventManagement/v4/hub
func (h *TMForumHandler) RegisterTMF688Hub(c *gin.Context) {
	var hubReq models.TMF688HubCreate
	if err := c.ShouldBindJSON(&hubReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// This maps to O2-IMS subscription mechanism
	// Create an O2-IMS subscription with the callback URL
	hub := &models.TMF688Hub{
		ID:       fmt.Sprintf("hub-%d", len(hubReq.Callback)),
		Callback: hubReq.Callback,
		Query:    hubReq.Query,
		AtType:   "EventSubscriptionInput",
	}

	c.JSON(http.StatusCreated, hub)
}

// UnregisterTMF688Hub unregisters a hub.
// DELETE /tmf-api/eventManagement/v4/hub/:id
func (h *TMForumHandler) UnregisterTMF688Hub(c *gin.Context) {
	hubID := c.Param("id")

	// This would map to deleting the corresponding O2-IMS subscription
	h.logger.Info("unregistering event hub",
		zap.String("hubId", hubID),
	)

	c.Status(http.StatusNoContent)
}
