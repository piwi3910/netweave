package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// AuditHandler handles Audit log API endpoints.
type AuditHandler struct {
	store  auth.Store
	logger *zap.Logger
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(store auth.Store, logger *zap.Logger) *AuditHandler {
	if store == nil {
		panic("auth store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &AuditHandler{
		store:  store,
		logger: logger,
	}
}

// ListAuditEvents handles GET /audit/events.
// Lists audit events with optional filtering.
func (h *AuditHandler) ListAuditEvents(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	isPlatformAdmin := auth.IsPlatformAdminFromContext(ctx)

	// Parse query parameters.
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	queryTenantID := c.Query("tenantId")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Determine which tenant's events to return.
	var filterTenantID string
	if isPlatformAdmin {
		// Platform admins can filter by any tenant or view all.
		filterTenantID = queryTenantID
	} else {
		// Regular users can only see their own tenant's events.
		filterTenantID = tenantID
	}

	h.logger.Info("listing audit events",
		zap.String("tenant_id", filterTenantID),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
		zap.String("request_id", c.GetString("request_id")),
	)

	events, err := h.store.ListEvents(ctx, filterTenantID, limit, offset)
	if err != nil {
		h.logger.Error("failed to list audit events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve audit events",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"limit":  limit,
		"offset": offset,
		"total":  len(events),
	})
}

// ListAuditEventsByType handles GET /audit/events/type/:eventType.
// Lists audit events of a specific type.
func (h *AuditHandler) ListAuditEventsByType(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	isPlatformAdmin := auth.IsPlatformAdminFromContext(ctx)
	eventType := c.Param("eventType")

	if eventType == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Event type is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Parse limit.
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	h.logger.Info("listing audit events by type",
		zap.String("event_type", eventType),
		zap.Int("limit", limit),
		zap.String("request_id", c.GetString("request_id")),
	)

	events, err := h.store.ListEventsByType(ctx, auth.AuditEventType(eventType), limit)
	if err != nil {
		h.logger.Error("failed to list audit events by type", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve audit events",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Filter by tenant if not platform admin.
	if !isPlatformAdmin {
		filtered := make([]*auth.AuditEvent, 0)
		for _, event := range events {
			if event.TenantID == tenantID || event.TenantID == "" {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"events":    events,
		"eventType": eventType,
		"total":     len(events),
	})
}

// ListAuditEventsByUser handles GET /audit/events/user/:userId.
// Lists audit events for a specific user.
func (h *AuditHandler) ListAuditEventsByUser(c *gin.Context) {
	ctx := c.Request.Context()
	targetUserID := c.Param("userId")

	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "User ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Check access permissions.
	if !h.checkUserAccessPermission(c, ctx, targetUserID) {
		return
	}

	// Parse and validate limit.
	limit := h.parseLimit(c)

	h.logger.Info("listing audit events by user",
		zap.String("target_user_id", targetUserID),
		zap.Int("limit", limit),
		zap.String("request_id", c.GetString("request_id")),
	)

	events, err := h.store.ListEventsByUser(ctx, targetUserID, limit)
	if err != nil {
		h.logger.Error("failed to list audit events by user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve audit events",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"userId": targetUserID,
		"total":  len(events),
	})
}

// checkUserAccessPermission verifies if current user can access target user's audit events.
// Returns false and sends error response if access is denied.
func (h *AuditHandler) checkUserAccessPermission(c *gin.Context, ctx context.Context, targetUserID string) bool {
	tenantID := auth.TenantIDFromContext(ctx)
	isPlatformAdmin := auth.IsPlatformAdminFromContext(ctx)
	currentUser := auth.UserFromContext(ctx)

	// Platform admins can access anyone's events.
	if isPlatformAdmin {
		return true
	}

	// Users can access their own events.
	if currentUser != nil && currentUser.UserID == targetUserID {
		return true
	}

	// Non-platform admins can only view events in their tenant.
	if currentUser != nil {
		targetUser, err := h.store.GetUser(ctx, targetUserID)
		if err == nil && targetUser.TenantID == tenantID {
			return true
		}
	}

	c.JSON(http.StatusForbidden, models.ErrorResponse{
		Error:   "Forbidden",
		Message: "Access denied to audit events for this user",
		Code:    http.StatusForbidden,
	})
	return false
}

// parseLimit parses and validates the limit query parameter.
func (h *AuditHandler) parseLimit(c *gin.Context) int {
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		return 50
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}
