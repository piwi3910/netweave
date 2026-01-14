package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// RoleHandler handles Role management API endpoints.
type RoleHandler struct {
	store  auth.Store
	logger *zap.Logger
}

// NewRoleHandler creates a new RoleHandler.
func NewRoleHandler(store auth.Store, logger *zap.Logger) *RoleHandler {
	if store == nil {
		panic("auth store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &RoleHandler{
		store:  store,
		logger: logger,
	}
}

// ListRoles handles GET /roles.
// Lists all roles available to the current tenant.
func (h *RoleHandler) ListRoles(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	isPlatformAdmin := auth.IsPlatformAdminFromContext(ctx)

	h.logger.Info("listing roles",
		zap.String("tenant_id", tenantID),
		zap.Bool("is_platform_admin", isPlatformAdmin),
		zap.String("request_id", c.GetString("request_id")),
	)

	var roles []*auth.Role
	var err error

	if isPlatformAdmin {
		// Platform admins see all roles.
		roles, err = h.store.ListRoles(ctx)
	} else {
		// Regular users see global tenant roles and their tenant's custom roles.
		roles, err = h.store.ListRolesByTenant(ctx, tenantID)
	}

	if err != nil {
		h.logger.Error("failed to list roles", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve roles",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"roles": roles,
		"total": len(roles),
	})
}

// GetRole handles GET /roles/:roleId.
// Retrieves a specific role.
func (h *RoleHandler) GetRole(c *gin.Context) {
	ctx := c.Request.Context()
	roleID := c.Param("roleId")
	tenantID := auth.TenantIDFromContext(ctx)
	isPlatformAdmin := auth.IsPlatformAdminFromContext(ctx)

	if roleID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Role ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	role, err := h.store.GetRole(ctx, roleID)
	if err != nil {
		if errors.Is(err, auth.ErrRoleNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Role not found: " + roleID,
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to get role",
			zap.String("role_id", roleID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to retrieve role",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Check access: platform admins can see all, others only their tenant's roles.
	if !isPlatformAdmin && role.TenantID != "" && role.TenantID != tenantID {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "Forbidden",
			Message: "Access denied to role from different tenant",
			Code:    http.StatusForbidden,
		})
		return
	}

	c.JSON(http.StatusOK, role)
}

// ListPermissions handles GET /permissions.
// Lists all available permissions.
func (h *RoleHandler) ListPermissions(c *gin.Context) {
	// Return all predefined permissions.
	permissions := []auth.Permission{
		auth.PermissionSubscriptionRead,
		auth.PermissionSubscriptionCreate,
		auth.PermissionSubscriptionDelete,
		auth.PermissionResourcePoolRead,
		auth.PermissionResourcePoolCreate,
		auth.PermissionResourcePoolUpdate,
		auth.PermissionResourcePoolDelete,
		auth.PermissionResourceRead,
		auth.PermissionResourceCreate,
		auth.PermissionResourceUpdate,
		auth.PermissionResourceDelete,
		auth.PermissionResourceTypeRead,
		auth.PermissionDeploymentManagerRead,
		auth.PermissionTenantRead,
		auth.PermissionTenantCreate,
		auth.PermissionTenantUpdate,
		auth.PermissionTenantDelete,
		auth.PermissionUserRead,
		auth.PermissionUserCreate,
		auth.PermissionUserUpdate,
		auth.PermissionUserDelete,
		auth.PermissionRoleRead,
		auth.PermissionRoleCreate,
		auth.PermissionRoleUpdate,
		auth.PermissionRoleDelete,
		auth.PermissionAuditRead,
	}

	// Convert to response format.
	type permissionInfo struct {
		Permission auth.Permission `json:"permission"`
		Resource   string          `json:"resource"`
		Action     string          `json:"action"`
	}

	result := make([]permissionInfo, 0, len(permissions))
	for _, p := range permissions {
		result = append(result, permissionInfo{
			Permission: p,
			Resource:   GetResourceFromPermission(p),
			Action:     GetActionFromPermission(p),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"permissions": result,
		"total":       len(result),
	})
}

// getResourceFromPermission extracts the resource part from a permission string.
func GetResourceFromPermission(p auth.Permission) string {
	s := string(p)
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[:i]
		}
	}
	return s
}

// getActionFromPermission extracts the action part from a permission string.
func GetActionFromPermission(p auth.Permission) string {
	s := string(p)
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[i+1:]
		}
	}
	return ""
}
