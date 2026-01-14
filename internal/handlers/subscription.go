package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
	internalmodels "github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/piwi3910/netweave/internal/storage"
)

// SubscriptionHandler handles Subscription API endpoints.
type SubscriptionHandler struct {
	Store  storage.Store // Exported for testing
	Logger *zap.Logger   // Exported for testing
}

// NewSubscriptionHandler creates a new SubscriptionHandler.
// It requires a storage backend for subscription persistence and a logger for structured logging.
func NewSubscriptionHandler(store storage.Store, logger *zap.Logger) *SubscriptionHandler {
	if store == nil {
		panic("storage cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &SubscriptionHandler{
		Store:  store,
		Logger: logger,
	}
}

// ListSubscriptions handles GET /o2ims/v1/subscriptions.
// Lists all active subscriptions.
//
// Query Parameters:
//   - filter: Optional filter criteria
//   - offset: Pagination offset
//   - limit: Maximum number of items to return
//
// Response: 200 OK with array of Subscription objects.
func (h *SubscriptionHandler) ListSubscriptions(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.Logger.Info("listing subscriptions",
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Parse query parameters
	filter := internalmodels.ParseQueryParams(c.Request.URL.Query())

	// Get subscriptions filtered by tenant
	var storageSubs []*storage.Subscription
	var err error
	if tenantID != "" {
		storageSubs, err = h.Store.ListByTenant(ctx, tenantID)
	} else {
		// For backward compatibility: if no tenant context, list all
		// This allows non-multi-tenant deployments to work
		storageSubs, err = h.Store.List(ctx)
	}
	if err != nil {
		h.Logger.Error("failed to list subscriptions",
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve subscriptions",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert storage.Subscription to models.Subscription and apply filtering
	subscriptions := make([]models.Subscription, 0, len(storageSubs))
	for _, storageSub := range storageSubs {
		// Apply filtering if resource pool ID is specified
		if len(filter.ResourcePoolID) > 0 && storageSub.Filter.ResourcePoolID != "" {
			found := false
			for _, poolID := range filter.ResourcePoolID {
				if storageSub.Filter.ResourcePoolID == poolID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		subscriptions = append(subscriptions, models.Subscription{
			SubscriptionID:         storageSub.ID,
			Callback:               storageSub.Callback,
			ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
			Filter: models.SubscriptionFilter{
				ResourcePoolID: []string{storageSub.Filter.ResourcePoolID},
				ResourceTypeID: []string{storageSub.Filter.ResourceTypeID},
				ResourceID:     []string{storageSub.Filter.ResourceID},
			},
			CreatedAt: storageSub.CreatedAt,
		})
	}

	// Apply pagination
	totalCount := len(subscriptions)
	start := filter.Offset
	end := start + filter.Limit

	if start > len(subscriptions) {
		start = len(subscriptions)
	}
	if end > len(subscriptions) {
		end = len(subscriptions)
	}

	pagedSubscriptions := subscriptions[start:end]

	response := models.ListResponse{
		Items:      pagedSubscriptions,
		TotalCount: totalCount,
	}

	h.Logger.Info("subscriptions retrieved",
		zap.Int("count", len(pagedSubscriptions)),
		zap.Int("total", totalCount),
	)

	c.JSON(http.StatusOK, response)
}

// CreateSubscription handles POST /o2ims/v1/subscriptions.
// Creates a new subscription for resource change notifications.
//
// Request Body: Subscription object (without subscriptionId)
//
// Response:
//   - 201 Created: Created Subscription object with generated ID
//   - 400 Bad Request: Invalid request body or callback URL
//   - 409 Conflict: Subscription with same consumer ID already exists
func (h *SubscriptionHandler) CreateSubscription(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.Logger.Info("creating subscription",
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Parse and validate request
	sub, err := h.parseAndValidateRequest(c)
	if err != nil {
		return // Error response already sent
	}

	// Create and store subscription
	subscriptionID := uuid.New().String()
	storageSub := h.convertToStorageSubscription(sub, subscriptionID, tenantID)

	if err := h.StoreSubscription(ctx, c, storageSub); err != nil {
		return // Error response already sent
	}

	// Build and send response
	response := h.buildSubscriptionResponse(subscriptionID, storageSub)

	h.Logger.Info("subscription created",
		zap.String("subscription_id", subscriptionID),
		zap.String("callback", sub.Callback),
	)

	c.JSON(http.StatusCreated, response)
}

// parseAndValidateRequest parses and validates the subscription creation reques.
func (h *SubscriptionHandler) parseAndValidateRequest(c *gin.Context) (*models.Subscription, error) {
	var sub models.Subscription

	// Parse request body
	if err := c.ShouldBindJSON(&sub); err != nil {
		h.Logger.Warn("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return nil, fmt.Errorf("failed to bind JSON: %w", err)
	}

	// Validate callback URL
	if err := h.validateCallbackURL(c, sub.Callback); err != nil {
		return nil, err
	}

	return &sub, nil
}

// validateCallbackURL validates the callback URL forma.
func (h *SubscriptionHandler) validateCallbackURL(c *gin.Context, callback string) error {
	if callback == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Callback URL is required",
			Code:    http.StatusBadRequest,
		})
		return fmt.Errorf("callback URL is required")
	}

	callbackURL, err := url.Parse(callback)
	if err != nil || (callbackURL.Scheme != "http" && callbackURL.Scheme != "https") {
		h.Logger.Warn("invalid callback URL",
			zap.String("callback", callback),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid callback URL: must be a valid HTTP or HTTPS URL",
			Code:    http.StatusBadRequest,
		})
		return fmt.Errorf("invalid callback URL")
	}

	return nil
}

// convertToStorageSubscription converts models.Subscription to storage.Subscription.
func (h *SubscriptionHandler) convertToStorageSubscription(
	sub *models.Subscription,
	subscriptionID string,
	tenantID string,
) *storage.Subscription {
	storageFilter := storage.SubscriptionFilter{}
	if len(sub.Filter.ResourcePoolID) > 0 {
		storageFilter.ResourcePoolID = sub.Filter.ResourcePoolID[0]
	}
	if len(sub.Filter.ResourceTypeID) > 0 {
		storageFilter.ResourceTypeID = sub.Filter.ResourceTypeID[0]
	}
	if len(sub.Filter.ResourceID) > 0 {
		storageFilter.ResourceID = sub.Filter.ResourceID[0]
	}

	return &storage.Subscription{
		ID:                     subscriptionID,
		TenantID:               tenantID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter:                 storageFilter,
		CreatedAt:              time.Now(),
	}
}

// StoreSubscription stores the subscription and handles errors.
func (h *SubscriptionHandler) StoreSubscription(
	ctx context.Context,
	c *gin.Context,
	storageSub *storage.Subscription,
) error {
	err := h.Store.Create(ctx, storageSub)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionExists) {
			h.Logger.Warn("subscription already exists",
				zap.String("consumer_subscription_id", storageSub.ConsumerSubscriptionID),
			)
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "Conflict",
				Message: "Subscription already exists",
				Code:    http.StatusConflict,
			})
			return fmt.Errorf("subscription already exists: %w", err)
		}

		h.Logger.Error("failed to create subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to create subscription",
			Code:    http.StatusInternalServerError,
		})
		return fmt.Errorf("failed to create subscription in storage: %w", err)
	}

	return nil
}

// buildSubscriptionResponse builds the subscription response objec.
func (h *SubscriptionHandler) buildSubscriptionResponse(
	subscriptionID string,
	storageSub *storage.Subscription,
) models.Subscription {
	return models.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               storageSub.Callback,
		ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
		Filter: models.SubscriptionFilter{
			ResourcePoolID: []string{storageSub.Filter.ResourcePoolID},
			ResourceTypeID: []string{storageSub.Filter.ResourceTypeID},
			ResourceID:     []string{storageSub.Filter.ResourceID},
		},
		CreatedAt: storageSub.CreatedAt,
	}
}

// GetSubscription handles GET /o2ims/v1/subscriptions/:subscriptionId.
// Retrieves a specific subscription by ID.
//
// Path Parameters:
//   - subscriptionId: Unique identifier of the subscription
//
// Response:
//   - 200 OK: Subscription object
//   - 404 Not Found: Subscription does not exist
//   - 500 Internal Server Error: Server error occurred
func (h *SubscriptionHandler) GetSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.Logger.Info("getting subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Validate subscription ID
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Subscription ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get subscription from storage
	storageSub, err := h.Store.Get(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			h.Logger.Warn("subscription not found",
				zap.String("subscription_id", subscriptionID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Subscription not found: " + subscriptionID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.Logger.Error("failed to get subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve subscription",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Verify tenant ownership (return 404 to avoid information disclosure)
	if tenantID != "" && storageSub.TenantID != tenantID {
		h.Logger.Warn("tenant mismatch - subscription not found for this tenant",
			zap.String("subscription_id", subscriptionID),
			zap.String("tenant_id", tenantID),
			zap.String("subscription_tenant_id", storageSub.TenantID),
		)

		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Subscription not found: " + subscriptionID,
			Code:    http.StatusNotFound,
		})
		return
	}

	// Convert storage.Subscription to models.Subscription
	response := models.Subscription{
		SubscriptionID:         storageSub.ID,
		Callback:               storageSub.Callback,
		ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
		Filter: models.SubscriptionFilter{
			ResourcePoolID: []string{storageSub.Filter.ResourcePoolID},
			ResourceTypeID: []string{storageSub.Filter.ResourceTypeID},
			ResourceID:     []string{storageSub.Filter.ResourceID},
		},
		CreatedAt: storageSub.CreatedAt,
	}

	h.Logger.Info("subscription retrieved",
		zap.String("subscription_id", subscriptionID),
	)

	c.JSON(http.StatusOK, response)
}

// DeleteSubscription handles DELETE /o2ims/v1/subscriptions/:subscriptionId.
// Deletes a subscription and stops sending notifications.
//
// Path Parameters:
//   - subscriptionId: Unique identifier of the subscription
//
// Response:
//   - 204 No Content: Subscription deleted successfully
//   - 404 Not Found: Subscription does not exist
//   - 500 Internal Server Error: Server error occurred
func (h *SubscriptionHandler) DeleteSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.Logger.Info("deleting subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Validate subscription ID
	if subscriptionID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Subscription ID cannot be empty",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// First, verify tenant ownership by getting the subscription
	if tenantID != "" {
		storageSub, err := h.Store.Get(ctx, subscriptionID)
		if err != nil {
			if errors.Is(err, storage.ErrSubscriptionNotFound) {
				h.Logger.Warn("subscription not found",
					zap.String("subscription_id", subscriptionID),
				)

				c.JSON(http.StatusNotFound, models.ErrorResponse{
					Error:   "NotFound",
					Message: "Subscription not found: " + subscriptionID,
					Code:    http.StatusNotFound,
				})
				return
			}

			h.Logger.Error("failed to get subscription for tenant verification",
				zap.String("subscription_id", subscriptionID),
				zap.Error(err),
			)

			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "InternalError",
				Message: "Failed to delete subscription",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		// Verify tenant ownership (return 404 to avoid information disclosure)
		if storageSub.TenantID != tenantID {
			h.Logger.Warn("tenant mismatch - cannot delete subscription from different tenant",
				zap.String("subscription_id", subscriptionID),
				zap.String("tenant_id", tenantID),
				zap.String("subscription_tenant_id", storageSub.TenantID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Subscription not found: " + subscriptionID,
				Code:    http.StatusNotFound,
			})
			return
		}
	}

	// Delete subscription from storage
	err := h.Store.Delete(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			h.Logger.Warn("subscription not found",
				zap.String("subscription_id", subscriptionID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Subscription not found: " + subscriptionID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.Logger.Error("failed to delete subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete subscription",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.Logger.Info("subscription deleted",
		zap.String("subscription_id", subscriptionID),
	)

	c.Status(http.StatusNoContent)
}
