package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// TenantHandler handles Tenant management API endpoints.
type TenantHandler struct {
	store  auth.Store
	logger *zap.Logger
}

var (
	// tenantNameRegex validates tenant names: alphanumeric, spaces, hyphens, underscores.
	tenantNameRegex = regexp.MustCompile(`^[a-zA-Z0-9\s\-_]+$`)
)

// validateTenantName validates tenant name constraints.
func validateTenantName(name string) error {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 || len(trimmed) > 255 {
		return errors.New("tenant name must be between 1 and 255 characters")
	}
	if !tenantNameRegex.MatchString(trimmed) {
		return errors.New("tenant name can only contain alphanumeric characters, spaces, hyphens, and underscores")
	}
	return nil
}

// validateEmail validates email format using RFC 5322.
func validateEmail(email string) error {
	if email == "" {
		return nil // Email is optional
	}
	_, err := mail.ParseAddress(email)
	if err != nil {
		return errors.New("invalid email format")
	}
	return nil
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(store auth.Store, logger *zap.Logger) *TenantHandler {
	if store == nil {
		panic("auth store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &TenantHandler{
		store:  store,
		logger: logger,
	}
}

// CreateTenantRequest represents the request body for creating a tenant.
type CreateTenantRequest struct {
	Name         string            `json:"name" binding:"required"`
	Description  string            `json:"description,omitempty"`
	ContactEmail string            `json:"contactEmail,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Quota        *auth.TenantQuota `json:"quota,omitempty"`
}

// UpdateTenantRequest represents the request body for updating a tenant.
type UpdateTenantRequest struct {
	Name         string            `json:"name,omitempty"`
	Description  string            `json:"description,omitempty"`
	ContactEmail string            `json:"contactEmail,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Status       auth.TenantStatus `json:"status,omitempty"`
	Quota        *auth.TenantQuota `json:"quota,omitempty"`
}

// ListTenants handles GET /admin/tenants.
// Lists all tenants (platform admin only).
func (h *TenantHandler) ListTenants(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.Info("listing tenants",
		zap.String("request_id", c.GetString("request_id")),
	)

	tenants, err := h.store.ListTenants(ctx)
	if err != nil {
		h.logger.Error("failed to list tenants", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve tenants",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tenants": tenants,
		"total":   len(tenants),
	})
}

// CreateTenant handles POST /admin/tenants.
// Creates a new tenant (platform admin only).
func (h *TenantHandler) CreateTenant(c *gin.Context) {
	ctx := c.Request.Context()
	var req CreateTenantRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate tenant name
	if err := validateTenantName(req.Name); err != nil {
		h.logger.Warn("invalid tenant name", zap.String("name", req.Name), zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate email if provided
	if err := validateEmail(req.ContactEmail); err != nil {
		h.logger.Warn("invalid email", zap.String("email", req.ContactEmail), zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Set default quota if not provided.
	quota := auth.DefaultQuota()
	if req.Quota != nil {
		quota = *req.Quota
	}

	tenant := &auth.Tenant{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Description:  req.Description,
		Status:       auth.TenantStatusActive,
		Quota:        quota,
		Usage:        auth.TenantUsage{},
		ContactEmail: req.ContactEmail,
		Metadata:     req.Metadata,
	}

	if err := h.store.CreateTenant(ctx, tenant); err != nil {
		if errors.Is(err, auth.ErrTenantExists) {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "Conflict",
				Message: "Tenant already exists",
				Code:    http.StatusConflict,
			})
			return
		}

		h.logger.Error("failed to create tenant", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to create tenant",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Log audit event.
	h.logAuditEvent(c, auth.AuditEventTenantCreated, tenant.ID, "tenant", "create", nil)

	h.logger.Info("tenant created",
		zap.String("tenant_id", tenant.ID),
		zap.String("name", tenant.Name),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.JSON(http.StatusCreated, tenant)
}

// GetTenant handles GET /admin/tenants/:tenantId.
// Retrieves a specific tenant by ID.
func (h *TenantHandler) GetTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenantId")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	tenant, err := h.store.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, auth.ErrTenantNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Tenant not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get tenant",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve tenant",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, tenant)
}

func (h *TenantHandler) validateUpdateTenantRequest(c *gin.Context, req *UpdateTenantRequest) error {
	if req.Name != "" {
		if err := validateTenantName(req.Name); err != nil {
			h.logger.Warn("invalid tenant name", zap.String("name", req.Name), zap.Error(err))
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "BadRequest",
				Message: err.Error(),
				Code:    http.StatusBadRequest,
			})
			return err
		}
	}

	if req.ContactEmail != "" {
		if err := validateEmail(req.ContactEmail); err != nil {
			h.logger.Warn("invalid email", zap.String("email", req.ContactEmail), zap.Error(err))
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "BadRequest",
				Message: err.Error(),
				Code:    http.StatusBadRequest,
			})
			return err
		}
	}

	return nil
}

func (h *TenantHandler) getTenantForUpdate(ctx context.Context, c *gin.Context, tenantID string) (*auth.Tenant, error) {
	tenant, err := h.store.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, auth.ErrTenantNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Tenant not found",
				Code:    http.StatusNotFound,
			})
			return nil, fmt.Errorf("tenant not found: %w", err)
		}

		h.logger.Error("failed to get tenant", zap.String("tenant_id", tenantID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve tenant",
			Code:    http.StatusInternalServerError,
		})
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return tenant, nil
}

func (h *TenantHandler) applyTenantUpdates(tenant *auth.Tenant, req *UpdateTenantRequest) {
	if req.Name != "" {
		tenant.Name = req.Name
	}
	if req.Description != "" {
		tenant.Description = req.Description
	}
	if req.ContactEmail != "" {
		tenant.ContactEmail = req.ContactEmail
	}
	if req.Metadata != nil {
		tenant.Metadata = req.Metadata
	}
	if req.Status != "" {
		tenant.Status = req.Status
	}
	if req.Quota != nil {
		tenant.Quota = *req.Quota
	}
}

// UpdateTenant handles PUT /admin/tenants/:tenantId.
// Updates an existing tenant.
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenantId")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := h.validateUpdateTenantRequest(c, &req); err != nil {
		return
	}

	tenant, err := h.getTenantForUpdate(ctx, c, tenantID)
	if err != nil {
		return
	}

	h.applyTenantUpdates(tenant, &req)

	if err := h.store.UpdateTenant(ctx, tenant); err != nil {
		h.logger.Error("failed to update tenant",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to update tenant",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logAuditEvent(c, auth.AuditEventTenantUpdated, tenant.ID, "tenant", "update", nil)

	h.logger.Info("tenant updated",
		zap.String("tenant_id", tenant.ID),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.JSON(http.StatusOK, tenant)
}

// DeleteTenant handles DELETE /admin/tenants/:tenantId.
// Deletes a tenant (marks for deletion or removes).
func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenantId")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Check if tenant exists.
	tenant, err := h.store.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, auth.ErrTenantNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Tenant not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get tenant", zap.String("tenant_id", tenantID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve tenant",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Check for active resources.
	if tenant.Usage.Subscriptions > 0 || tenant.Usage.ResourcePools > 0 || tenant.Usage.Users > 0 {
		// Mark for deletion instead of immediate delete.
		tenant.Status = auth.TenantStatusPendingDeletion
		if err := h.store.UpdateTenant(ctx, tenant); err != nil {
			h.logger.Error("failed to mark tenant for deletion",
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "InternalError",
				Message: "Failed to mark tenant for deletion",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		h.logger.Info("tenant marked for deletion",
			zap.String("tenant_id", tenantID),
			zap.String("request_id", c.GetString("request_id")),
		)

		c.JSON(http.StatusAccepted, gin.H{
			"message":  "Tenant marked for deletion. Active resources must be cleaned up first.",
			"tenantId": tenantID,
			"status":   auth.TenantStatusPendingDeletion,
		})
		return
	}

	// Delete tenant.
	if err := h.store.DeleteTenant(ctx, tenantID); err != nil {
		h.logger.Error("failed to delete tenant",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete tenant",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Log audit event.
	h.logAuditEvent(c, auth.AuditEventTenantDeleted, tenantID, "tenant", "delete", nil)

	h.logger.Info("tenant deleted",
		zap.String("tenant_id", tenantID),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.Status(http.StatusNoContent)
}

// GetCurrentTenant handles GET /tenant.
// Returns the current user's tenant information.
func (h *TenantHandler) GetCurrentTenant(c *gin.Context) {
	tenant := auth.TenantFromContext(c.Request.Context())
	if tenant == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Tenant not found in context",
			Code:    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, tenant)
}

// logAuditEvent logs an audit event for tenant operations.
func (h *TenantHandler) logAuditEvent(c *gin.Context, eventType auth.AuditEventType, resourceID, resourceType, action string, details map[string]string) {
	user := auth.UserFromContext(c.Request.Context())

	event := &auth.AuditEvent{
		ID:           uuid.New().String(),
		Type:         eventType,
		TenantID:     c.GetString("tenant_id"),
		UserID:       "",
		Subject:      "",
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Details:      details,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Timestamp:    time.Now().UTC(),
	}

	if user != nil {
		event.UserID = user.UserID
		event.Subject = user.Subject
	}

	if err := h.store.LogEvent(c.Request.Context(), event); err != nil {
		h.logger.Warn("failed to log audit event",
			zap.String("event_type", string(eventType)),
			zap.Error(err),
		)
	}
}
