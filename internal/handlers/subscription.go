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
	"github.com/piwi3910/netweave/internal/security/urlredact"
	"github.com/piwi3910/netweave/internal/storage"
)

// SubscriptionHandler handles Subscription API endpoints.
type SubscriptionHandler struct {
	store     storage.Store
	authStore auth.Store
	logger    *zap.Logger
}

// NewSubscriptionHandler creates a new SubscriptionHandler.
// It requires a storage backend for subscription persistence and a logger for structured logging.
func NewSubscriptionHandler(store storage.Store, authStore auth.Store, logger *zap.Logger) *SubscriptionHandler {
	if store == nil {
		panic("storage cannot be nil")
	}
	if authStore == nil {
		panic("auth store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &SubscriptionHandler{
		store:     store,
		authStore: authStore,
		logger:    logger,
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

	h.logger.Info("listing subscriptions",
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Parse query parameters
	filter := internalmodels.ParseQueryParams(c.Request.URL.Query())

	// Get subscriptions filtered by tenant
	var storageSubs []*storage.Subscription
	var err error
	if tenantID != "" {
		storageSubs, err = h.store.ListByTenant(ctx, tenantID)
	} else {
		// For backward compatibility: if no tenant context, list all
		// This allows non-multi-tenant deployments to work
		storageSubs, err = h.store.List(ctx)
	}
	if err != nil {
		h.logger.Error("failed to list subscriptions",
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

	h.logger.Info("subscriptions retrieved",
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
//   - 429 Too Many Requests: Tenant quota exceeded
func (h *SubscriptionHandler) CreateSubscription(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	h.logger.Info("creating subscription",
		zap.String("request_id", c.GetString("request_id")),
		zap.String("tenant_id", tenantID),
	)

	// Parse and validate request
	sub, err := h.parseAndValidateRequest(c)
	if err != nil {
		return // Error response already sent
	}

	// Check tenant quota before creating subscription
	if tenantID != "" && h.authStore != nil {
		if err := h.authStore.IncrementUsage(ctx, tenantID, "subscriptions"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				h.logger.Warn("subscription quota exceeded",
					zap.String("tenant_id", tenantID),
				)
				c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
					Error:   "QuotaExceeded",
					Message: "Subscription quota exceeded for tenant",
					Code:    http.StatusTooManyRequests,
				})
				return
			}
			h.logger.Error("failed to check subscription quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "InternalError",
				Message: "Failed to check subscription quota",
				Code:    http.StatusInternalServerError,
			})
			return
		}
	}

	// Create and store subscription
	subscriptionID := uuid.New().String()
	storageSub := h.convertToStorageSubscription(sub, subscriptionID, tenantID)

	if err := h.StoreSubscription(ctx, c, storageSub); err != nil {
		// Rollback quota increment on failure
		if tenantID != "" && h.authStore != nil {
			if decErr := h.authStore.DecrementUsage(ctx, tenantID, "subscriptions"); decErr != nil {
				h.logger.Error("CRITICAL: quota rollback failed - tenant quota leaked",
					zap.String("tenant_id", tenantID),
					zap.String("subscription_id", subscriptionID),
					zap.Error(decErr),
					zap.NamedError("original_error", err),
				)
			}
		}
		return // Error response already sent
	}

	// Build and send response
	response := h.buildSubscriptionResponse(subscriptionID, storageSub)

	h.logger.Info("subscription created",
		zap.String("subscription_id", subscriptionID),
		zap.String("callback", urlredact.Redact(sub.Callback)),
	)

	c.JSON(http.StatusCreated, response)
}

// parseAndValidateRequest parses and validates the subscription creation reques.
func (h *SubscriptionHandler) parseAndValidateRequest(c *gin.Context) (*models.Subscription, error) {
	var sub models.Subscription

	// Parse request body
	if err := c.ShouldBindJSON(&sub); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
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
		h.logger.Warn("invalid callback URL",
			zap.String("callback", urlredact.Redact(callback)),
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
	err := h.store.Create(ctx, storageSub)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionExists) {
			h.logger.Warn("subscription already exists",
				zap.String("consumer_subscription_id", storageSub.ConsumerSubscriptionID),
			)
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "Conflict",
				Message: "Subscription already exists",
				Code:    http.StatusConflict,
			})
			return fmt.Errorf("subscription already exists: %w", err)
		}

		h.logger.Error("failed to create subscription", zap.Error(err))
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

	h.logger.Info("getting subscription",
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
	storageSub, err := h.store.Get(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			h.logger.Warn("subscription not found",
				zap.String("subscription_id", subscriptionID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Subscription not found: " + subscriptionID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get subscription",
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
		h.logger.Warn("tenant mismatch - subscription not found for this tenant",
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

	h.logger.Info("subscription retrieved",
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

	h.logger.Info("deleting subscription",
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
		storageSub, err := h.store.Get(ctx, subscriptionID)
		if err != nil {
			if errors.Is(err, storage.ErrSubscriptionNotFound) {
				h.logger.Warn("subscription not found",
					zap.String("subscription_id", subscriptionID),
				)

				c.JSON(http.StatusNotFound, models.ErrorResponse{
					Error:   "NotFound",
					Message: "Subscription not found: " + subscriptionID,
					Code:    http.StatusNotFound,
				})
				return
			}

			h.logger.Error("failed to get subscription for tenant verification",
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
			h.logger.Warn("tenant mismatch - cannot delete subscription from different tenant",
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
	err := h.store.Delete(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			h.logger.Warn("subscription not found",
				zap.String("subscription_id", subscriptionID),
			)

			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Subscription not found: " + subscriptionID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to delete subscription",
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

	// Decrement tenant quota after successful deletion
	if tenantID != "" && h.authStore != nil {
		if err := h.authStore.DecrementUsage(ctx, tenantID, "subscriptions"); err != nil {
			h.logger.Error("CRITICAL: quota decrement failed after deletion - tenant quota leaked",
				zap.String("tenant_id", tenantID),
				zap.String("subscription_id", subscriptionID),
				zap.Error(err),
			)
			// Don't fail the delete operation if quota decrement fails
		}
	}

	h.logger.Info("subscription deleted",
		zap.String("subscription_id", subscriptionID),
	)

	c.Status(http.StatusNoContent)
}
