package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// UserHandler handles User management API endpoints.
type UserHandler struct {
	store  auth.Store
	logger *zap.Logger
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(store auth.Store, logger *zap.Logger) *UserHandler {
	if store == nil {
		panic("auth store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &UserHandler{
		store:  store,
		logger: logger,
	}
}

// CreateUserRequest represents the request body for creating a user.
type CreateUserRequest struct {
	Subject    string `json:"subject" binding:"required"`
	CommonName string `json:"commonName" binding:"required"`
	Email      string `json:"email,omitempty"`
	RoleID     string `json:"roleId" binding:"required"`
	IsActive   *bool  `json:"isActive,omitempty"`
}

// UpdateUserRequest represents the request body for updating a user.
type UpdateUserRequest struct {
	Email    string `json:"email,omitempty"`
	RoleID   string `json:"roleId,omitempty"`
	IsActive *bool  `json:"isActive,omitempty"`
}

// ListUsers handles GET /tenant/users.
// Lists all users in the current tenant.
func (h *UserHandler) ListUsers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant context required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	h.logger.Info("listing users",
		zap.String("tenant_id", tenantID),
		zap.String("request_id", c.GetString("request_id")),
	)

	users, err := h.store.ListUsersByTenant(ctx, tenantID)
	if err != nil {
		h.logger.Error("failed to list users",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve users",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": len(users),
	})
}

// CreateUser handles POST /tenant/users.
// Creates a new user in the current tenant.
func (h *UserHandler) CreateUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	tenant := auth.TenantFromContext(ctx)

	if tenantID == "" || tenant == nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Tenant context required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate email if provided
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			h.logger.Warn("invalid email", zap.String("email", req.Email), zap.Error(err))
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Invalid email format",
				Code:    http.StatusBadRequest,
			})
			return
		}
	}

	// Check quota.
	if !tenant.CanAddUser() {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "QuotaExceeded",
			Message: "User quota exceeded for this tenant",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Verify role exists.
	role, err := h.store.GetRole(ctx, req.RoleID)
	if err != nil {
		if errors.Is(err, auth.ErrRoleNotFound) {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Invalid role ID",
				Code:    http.StatusBadRequest,
			})
			return
		}
		h.logger.Error("failed to get role", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to validate role",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Prevent assigning platform roles to tenant users.
	if role.Type == auth.RoleTypePlatform {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Cannot assign platform-level roles to tenant users",
			Code:    http.StatusBadRequest,
		})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	user := &auth.TenantUser{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		Subject:    req.Subject,
		CommonName: req.CommonName,
		Email:      req.Email,
		RoleID:     req.RoleID,
		IsActive:   isActive,
	}

	if err := h.store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "Conflict",
				Message: "User with this subject already exists",
				Code:    http.StatusConflict,
			})
			return
		}

		h.logger.Error("failed to create user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to create user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Increment usage.
	if err := h.store.IncrementUsage(ctx, tenantID, "users"); err != nil {
		h.logger.Warn("failed to increment user usage", zap.Error(err))
	}

	// Log audit event.
	h.logAuditEvent(c, auth.AuditEventUserCreated, user.ID, "user", "create", map[string]string{
		"subject":    user.Subject,
		"commonName": user.CommonName,
	})

	h.logger.Info("user created",
		zap.String("user_id", user.ID),
		zap.String("tenant_id", tenantID),
		zap.String("subject", user.Subject),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.JSON(http.StatusCreated, user)
}

// GetUser handles GET /tenant/users/:userId.
// Retrieves a specific user.
func (h *UserHandler) GetUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	userID := c.Param("userId")

	if userID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "User ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	user, err := h.store.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "User not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Ensure user belongs to the requesting tenant (unless platform admin).
	if !auth.IsPlatformAdminFromContext(ctx) && user.TenantID != tenantID {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "Forbidden",
			Message: "Access denied to user from different tenant",
			Code:    http.StatusForbidden,
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser handles PUT /tenant/users/:userId.
// Updates an existing user.
func (h *UserHandler) UpdateUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	userID := c.Param("userId")

	if userID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "User ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get existing user.
	user, err := h.store.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "User not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get user", zap.String("user_id", userID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Ensure user belongs to the requesting tenant (unless platform admin).
	if !auth.IsPlatformAdminFromContext(ctx) && user.TenantID != tenantID {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "Forbidden",
			Message: "Access denied to user from different tenant",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Validate email if provided
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			h.logger.Warn("invalid email", zap.String("email", req.Email), zap.Error(err))
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Invalid email format",
				Code:    http.StatusBadRequest,
			})
			return
		}
	}

	// Apply updates.
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.RoleID != "" {
		// Verify role exists.
		role, err := h.store.GetRole(ctx, req.RoleID)
		if err != nil {
			if errors.Is(err, auth.ErrRoleNotFound) {
				c.JSON(http.StatusBadRequest, models.ErrorResponse{
					Error:   "BadRequest",
					Message: "Invalid role ID",
					Code:    http.StatusBadRequest,
				})
				return
			}
			h.logger.Error("failed to get role", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "InternalError",
				Message: "Failed to validate role",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		// Prevent assigning platform roles.
		if role.Type == auth.RoleTypePlatform {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Cannot assign platform-level roles to tenant users",
				Code:    http.StatusBadRequest,
			})
			return
		}

		user.RoleID = req.RoleID
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if err := h.store.UpdateUser(ctx, user); err != nil {
		h.logger.Error("failed to update user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to update user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Log audit event.
	h.logAuditEvent(c, auth.AuditEventUserUpdated, user.ID, "user", "update", nil)

	h.logger.Info("user updated",
		zap.String("user_id", user.ID),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.JSON(http.StatusOK, user)
}

// DeleteUser handles DELETE /tenant/users/:userId.
// Deletes a user.
func (h *UserHandler) DeleteUser(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	currentUser := auth.UserFromContext(ctx)
	userID := c.Param("userId")

	if userID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "User ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Prevent self-deletion.
	if currentUser != nil && currentUser.UserID == userID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Cannot delete your own user account",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get user to verify tenant.
	user, err := h.store.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "User not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get user", zap.String("user_id", userID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Ensure user belongs to the requesting tenant (unless platform admin).
	if !auth.IsPlatformAdminFromContext(ctx) && user.TenantID != tenantID {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "Forbidden",
			Message: "Access denied to user from different tenant",
			Code:    http.StatusForbidden,
		})
		return
	}

	if err := h.store.DeleteUser(ctx, userID); err != nil {
		h.logger.Error("failed to delete user",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Decrement usage.
	if err := h.store.DecrementUsage(ctx, user.TenantID, "users"); err != nil {
		h.logger.Warn("failed to decrement user usage", zap.Error(err))
	}

	// Log audit event.
	h.logAuditEvent(c, auth.AuditEventUserDeleted, userID, "user", "delete", map[string]string{
		"subject": user.Subject,
	})

	h.logger.Info("user deleted",
		zap.String("user_id", userID),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.Status(http.StatusNoContent)
}

// GetCurrentUser handles GET /user.
// Returns the current authenticated user's information.
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	ctx := c.Request.Context()
	authUser := auth.UserFromContext(ctx)

	if authUser == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "User not found in context",
			Code:    http.StatusNotFound,
		})
		return
	}

	// Get full user details.
	user, err := h.store.GetUser(ctx, authUser.UserID)
	if err != nil {
		h.logger.Error("failed to get current user",
			zap.String("user_id", authUser.UserID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve user",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

// logAuditEvent logs an audit event for user operations.
func (h *UserHandler) logAuditEvent(c *gin.Context, eventType auth.AuditEventType, resourceID, resourceType, action string, details map[string]string) {
	user := auth.UserFromContext(c.Request.Context())

	event := &auth.AuditEvent{
		ID:           uuid.New().String(),
		Type:         eventType,
		TenantID:     auth.TenantIDFromContext(c.Request.Context()),
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
