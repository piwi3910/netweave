package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// ListBackendAccess handles GET /admin/tenants/:tid/backend-access.
func (h *BackendHandler) ListBackendAccess(c *gin.Context) {
	tenantID := c.Param("tid")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	accessList, err := h.store.ListBackendAccessByTenant(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("failed to list backend access",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to list backend access",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access": accessList,
		"total":  len(accessList),
	})
}

// CreateBackendAccess handles POST /admin/tenants/:tid/backend-access.
func (h *BackendHandler) CreateBackendAccess(c *gin.Context) {
	tenantID := c.Param("tid")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req CreateBackendAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	grantedBy := extractUserID(c)
	access := &backend.Access{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		BackendID:   req.BackendID,
		Permissions: req.Permissions,
		Constraints: req.Constraints,
		GrantedAt:   time.Now().UTC(),
		GrantedBy:   grantedBy,
	}

	if err := h.store.CreateBackendAccess(c.Request.Context(), access); err != nil {
		h.handleCreateAccessError(c, tenantID, err)
		return
	}

	h.logger.Info("backend access created",
		zap.String("access_id", access.ID),
		zap.String("tenant_id", tenantID),
		zap.String("backend_id", req.BackendID),
	)

	c.JSON(http.StatusCreated, access)
}

func (h *BackendHandler) handleCreateAccessError(c *gin.Context, tenantID string, err error) {
	if errors.Is(err, backend.ErrAccessExists) {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "Conflict",
			Message: "Access already exists for this tenant-backend pair",
			Code:    http.StatusConflict,
		})
		return
	}

	h.logger.Error("failed to create backend access",
		zap.String("tenant_id", tenantID),
		zap.Error(err),
	)
	c.JSON(http.StatusInternalServerError, models.ErrorResponse{
		Error:   "InternalError",
		Message: "Failed to create backend access",
		Code:    http.StatusInternalServerError,
	})
}

// UpdateBackendAccess handles PUT /admin/tenants/:tid/backend-access/:aid.
func (h *BackendHandler) UpdateBackendAccess(c *gin.Context) {
	tenantID := c.Param("tid")
	accessID := c.Param("aid")

	if tenantID == "" || accessID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID and access ID are required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdateBackendAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	access := &backend.Access{
		ID:          accessID,
		Permissions: req.Permissions,
		Constraints: req.Constraints,
	}

	if err := h.store.UpdateBackendAccess(c.Request.Context(), access); err != nil {
		h.logger.Error("failed to update backend access",
			zap.String("access_id", accessID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to update backend access",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("backend access updated",
		zap.String("access_id", accessID),
		zap.String("tenant_id", tenantID),
	)

	c.JSON(http.StatusOK, access)
}

// DeleteBackendAccess handles DELETE /admin/tenants/:tid/backend-access/:aid.
func (h *BackendHandler) DeleteBackendAccess(c *gin.Context) {
	tenantID := c.Param("tid")
	accessID := c.Param("aid")

	if tenantID == "" || accessID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant ID and access ID are required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := h.store.DeleteBackendAccess(c.Request.Context(), accessID); err != nil {
		if errors.Is(err, backend.ErrAccessNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Backend access not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to delete backend access",
			zap.String("access_id", accessID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete backend access",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("backend access deleted",
		zap.String("access_id", accessID),
		zap.String("tenant_id", tenantID),
	)
	c.Status(http.StatusNoContent)
}

// extractUserID retrieves the authenticated user ID from the Gin context.
func extractUserID(c *gin.Context) string {
	user := auth.UserFromContext(c.Request.Context())
	if user != nil {
		return user.UserID
	}
	return "unknown"
}
