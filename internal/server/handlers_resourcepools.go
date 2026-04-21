package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
)

// parseFilterFromRequest parses filter parameters from the request context.
// Returns an adapter.Filter with tenant context applied.
//
// SECURITY: platform admins receive an empty tenant filter so they can see
// every tenant's resources. Non-admin authenticated callers are pinned to
// their own tenant. Unauthenticated callers (auth disabled) get no tenant
// filter — they rely on other controls (e.g., network isolation).
func (s *Server) parseFilterFromRequest(c *gin.Context) (*adapter.Filter, error) {
	ctx := c.Request.Context()
	var tenantID string
	if !auth.IsPlatformAdminFromContext(ctx) {
		tenantID = auth.TenantIDFromContext(ctx)
	}

	// Only v1 routes are registered, so advanced filtering is not available.
	return &adapter.Filter{
		TenantID: tenantID,
		Limit:    100, // Default limit for v1.
	}, nil
}

// Resource Pool handlers

// handleListResourcePools lists all resource pools.
// GET /o2ims/v1/resourcePools.
func (s *Server) handleListResourcePools(c *gin.Context) {
	s.logger.Info("listing resource pools")

	// Parse filter from request (supports v1 basic and v2+ advanced filtering).
	filter, err := s.parseFilterFromRequest(c)
	if err != nil {
		s.logger.Error("failed to parse filter", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "InvalidParameter",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// List resource pools via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	pools, err := adp.ListResourcePools(c.Request.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list resource pools", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource pools",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourcePools": pools,
		"total":         len(pools),
	})
}

// handleGetResourcePool retrieves a specific resource pool.
// GET /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleGetResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("getting resource pool", zap.String("resource_pool_id", resourcePoolID))

	// Get resource pool via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	pool, err := adp.GetResourcePool(c.Request.Context(), resourcePoolID)
	if err != nil {
		s.writeNotFoundOrForbidden(c, "resource pool",
			"adapter failed to fetch resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)
		return
	}

	// Tenant isolation: verify pool belongs to tenant (unless platform admin).
	// SECURITY: fail-closed via auth.AuthorizeTenantResource — a pool with
	// empty TenantID is inaccessible to non-platform-admin callers (issue #470).
	ctx := c.Request.Context()
	if !auth.AuthorizeTenantResource(ctx, pool.TenantID) {
		s.writeNotFoundOrForbidden(c, "resource pool",
			"tenant attempting to access resource pool from different tenant",
			zap.String("tenant_id", auth.TenantIDFromContext(ctx)),
			zap.String("pool_tenant_id", pool.TenantID),
			zap.String("resource_pool_id", resourcePoolID),
		)
		return
	}

	c.JSON(http.StatusOK, pool)
}

// handleListResourcesInPool lists resources in a specific pool.
// GET /o2ims/v1/resourcePools/:resourcePoolId/resources.
func (s *Server) handleListResourcesInPool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("listing resources in pool", zap.String("resource_pool_id", resourcePoolID))

	// Create filter for this resource pool
	filter := &adapter.Filter{
		ResourcePoolID: resourcePoolID,
	}

	// Apply tenant filter for multi-tenancy isolation
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) {
		filter.TenantID = tenantID
	}

	// List resources via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resources, err := adp.ListResources(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list resources in pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resources",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"total":     len(resources),
	})
}

// Validation constants for resource pool fields.
const (
	// MaxResourcePoolNameLength is the maximum allowed length for resource pool names.
	MaxResourcePoolNameLength = 255

	// MaxResourcePoolIDLength is the maximum allowed length for resource pool IDs.
	MaxResourcePoolIDLength = 255

	// MaxResourcePoolDescriptionLength is the maximum allowed length for resource pool descriptions.
	MaxResourcePoolDescriptionLength = 1000
)

// Validation constants for resource extension fields.
const (
	// MaxExtensionKeys is the maximum number of extension keys allowed.
	MaxExtensionKeys = 100

	// MaxExtensionKeyLength is the maximum length for an extension key.
	MaxExtensionKeyLength = 256

	// MaxExtensionValueSize is the maximum size for a single extension value when JSON-encoded.
	MaxExtensionValueSize = 4096

	// MaxExtensionsTotalSize is the maximum total size for all extensions combined (50KB).
	MaxExtensionsTotalSize = 50000
)

// SanitizeResourcePoolID sanitizes a string for use in resource pool IDs.
// Removes special characters that could cause security issues (path traversal, injection).
// Spaces and slashes are replaced with hyphens, all other special characters are dropped.
func SanitizeResourcePoolID(name string) string {
	var result strings.Builder
	for _, ch := range name {
		if isAlphanumericOrAllowed(ch) {
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '/' {
			result.WriteRune('-') // Only replace spaces and slashes with hyphens
		}
		// All other special characters are simply dropped for security
	}

	return strings.ToLower(result.String())
}

// isAlphanumericOrAllowed checks if a rune is alphanumeric, hyphen, or underscore.
func isAlphanumericOrAllowed(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

// SanitizeForLogging removes CRLF characters to prevent log injection attacks.
// This prevents attackers from injecting fake log entries via user-controlled input.
func SanitizeForLogging(s string) string {
	// Remove CR, LF, and other control characters
	sanitized := strings.NewReplacer(
		"\r", "",
		"\n", "",
		"\t", " ",
	).Replace(s)

	// Remove any remaining control characters (ASCII 0-31 except space)
	var result strings.Builder
	for _, ch := range sanitized {
		if ch >= 32 || ch == ' ' {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

// isValidIDCharacter checks if a character is valid for resource pool IDs.
func isValidIDCharacter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

// validateResourcePoolID validates the resource pool ID format.
func validateResourcePoolID(id string) error {
	if len(id) > MaxResourcePoolIDLength {
		return fmt.Errorf("resourcePoolId must not exceed %d characters", MaxResourcePoolIDLength)
	}

	for _, ch := range id {
		if !isValidIDCharacter(ch) {
			return errors.New("resourcePoolId must contain only alphanumeric characters, hyphens, and underscores")
		}
	}

	return nil
}

// ValidateResourcePoolFields validates required fields in a ResourcePool.
func ValidateResourcePoolFields(pool *adapter.ResourcePool) error {
	var validationErrors []string

	// Validate Name is required
	if pool.Name == "" {
		validationErrors = append(validationErrors, "name is required")
	}

	// Validate Name length
	if len(pool.Name) > MaxResourcePoolNameLength {
		validationErrors = append(validationErrors,
			fmt.Sprintf("name must not exceed %d characters", MaxResourcePoolNameLength))
	}

	// Validate ResourcePoolID if provided
	if pool.ResourcePoolID != "" {
		if err := validateResourcePoolID(pool.ResourcePoolID); err != nil {
			validationErrors = append(validationErrors, err.Error())
		}
	}

	// Validate Description length if provided
	if len(pool.Description) > MaxResourcePoolDescriptionLength {
		validationErrors = append(validationErrors,
			fmt.Sprintf("description must not exceed %d characters", MaxResourcePoolDescriptionLength))
	}

	// Return all validation errors together
	if len(validationErrors) > 0 {
		return errors.New(strings.Join(validationErrors, "; "))
	}

	return nil
}

// handleCreateResourcePool creates a new resource pool.
// POST /o2ims/v1/resourcePools.
func (s *Server) handleCreateResourcePool(c *gin.Context) {
	s.logger.Info("creating resource pool")

	var req adapter.ResourcePool
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Validate resource pool fields
	if err := ValidateResourcePoolFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Check tenant quota before creating resource pool
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.IncrementUsage(ctx, tenantID, "resource_pools"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				s.logger.Warn("resource pool quota exceeded",
					zap.String("tenant_id", tenantID))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "QuotaExceeded",
					"message": "Resource pool quota exceeded for tenant",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
			s.logger.Error("failed to check resource pool quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to check resource pool quota",
				"code":    http.StatusInternalServerError,
			})
			return
		}
	}

	// Set tenant ID on the resource pool for isolation
	if tenantID != "" {
		req.TenantID = tenantID
	}

	// Generate resource pool ID if not provided (sanitized with UUID for uniqueness)
	// Format: pool-{sanitized-name}-{uuid}
	// Example: "GPU Pool (Production)" → "pool-gpu-pool-production-a1b2c3d4-e5f6-7890-abcd-1234567890ab"
	if req.ResourcePoolID == "" {
		sanitizedName := SanitizeResourcePoolID(req.Name)
		// Clean up consecutive hyphens that can occur from sanitization
		sanitizedName = strings.ReplaceAll(sanitizedName, "--", "-")
		sanitizedName = strings.Trim(sanitizedName, "-")

		// Ensure total length doesn't exceed 255 chars (per O2-IMS spec)
		// UUID is 36 chars, "pool-" is 5 chars, we need 2 chars for separating hyphens = 43 chars reserved
		// This leaves 212 chars for the sanitized name
		maxNameLength := 212
		if len(sanitizedName) > maxNameLength {
			sanitizedName = sanitizedName[:maxNameLength]
		}

		req.ResourcePoolID = "pool-" + sanitizedName + "-" + uuid.New().String()
	}

	// Create resource pool via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	created, err := adp.CreateResourcePool(c.Request.Context(), &req)
	if err != nil {
		// Rollback quota increment on creation failure
		if tenantID != "" && s.AuthStore != nil {
			if rollbackErr := s.AuthStore.DecrementUsage(ctx, tenantID, "resource_pools"); rollbackErr != nil {
				s.logger.Error("failed to rollback resource pool quota after creation failure",
					zap.String("tenant_id", tenantID),
					zap.Error(rollbackErr))
			}
		}

		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourcePoolCreated,
				"resourcepool",
				req.ResourcePoolID,
				user,
				false,
				map[string]string{
					"name":  req.Name,
					"error": err.Error(),
				},
			)
		}

		// Check for duplicate resource pool using sentinel error
		if errors.Is(err, adapter.ErrResourcePoolExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource pool with ID " + SanitizeForLogging(req.ResourcePoolID) + " already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create resource pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create resource pool",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource pool created",
		zap.String("resource_pool_id", created.ResourcePoolID),
		zap.String("name", SanitizeForLogging(created.Name)))

	// Audit log the successful creation
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourcePoolCreated,
			"resourcepool",
			created.ResourcePoolID,
			user,
			true,
			map[string]string{
				"name": created.Name,
			},
		)
	}

	// Set Location header for REST compliance
	c.Header("Location", "/o2ims/v1/resourcePools/"+created.ResourcePoolID)
	c.JSON(http.StatusCreated, created)
}

// handleUpdateResourcePool updates an existing resource pool.
// PUT /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleUpdateResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("updating resource pool", zap.String("resource_pool_id", resourcePoolID))

	var req adapter.ResourcePool
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Validate field constraints
	if err := ValidateResourcePoolFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Update resource pool via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	ctx := c.Request.Context()
	// SECURITY: verify tenant ownership before update. Without this check a
	// non-platform-admin caller with resourcePools:update could modify any
	// tenant's pool. Fail-closed via auth.AuthorizeTenantResource so that
	// pools with empty TenantID cannot be reached by non-admin callers
	// (see issue #470 / issue #482).
	//
	// The check is gated on the presence of an authenticated user: when auth
	// middleware is not configured (local dev / tests), tenant enforcement
	// is deferred to deployment-level controls.
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		existing, getErr := adp.GetResourcePool(ctx, resourcePoolID)
		if getErr != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource pool not found: " + resourcePoolID,
				"code":    http.StatusNotFound,
			})
			return
		}
		if !auth.AuthorizeTenantResource(ctx, existing.TenantID) {
			s.logger.Warn("tenant attempting to update resource pool from different tenant",
				zap.String("tenant_id", user.TenantID),
				zap.String("pool_tenant_id", existing.TenantID),
				zap.String("resource_pool_id", resourcePoolID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource pool not found: " + resourcePoolID,
				"code":    http.StatusNotFound,
			})
			return
		}
		// Preserve the existing tenant stamp; clients cannot move a pool
		// between tenants via update.
		req.TenantID = existing.TenantID
	}

	updated, err := adp.UpdateResourcePool(c.Request.Context(), resourcePoolID, &req)
	if err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourcePoolModified,
				"resourcepool",
				resourcePoolID,
				user,
				false,
				map[string]string{
					"name":  req.Name,
					"error": err.Error(),
				},
			)
		}

		if errors.Is(err, adapter.ErrResourcePoolNotFound) {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"adapter reported resource pool not found during update",
				zap.String("resource_pool_id", resourcePoolID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to update resource pool",
			"adapter failed to update resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("resource pool updated",
		zap.String("resource_pool_id", updated.ResourcePoolID),
		zap.String("name", SanitizeForLogging(updated.Name)))

	// Audit log the successful update
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourcePoolModified,
			"resourcepool",
			updated.ResourcePoolID,
			user,
			true,
			map[string]string{
				"name": updated.Name,
			},
		)
	}

	c.JSON(http.StatusOK, updated)
}

// handleDeleteResourcePool deletes a resource pool.
// DELETE /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleDeleteResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)

	// Verify tenant ownership before deletion.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	// SECURITY: fail-closed tenant ownership check. A pool with empty
	// TenantID is inaccessible to non-platform-admin callers. This prevents
	// the bypass described in issue #470 (C7) where adapters or legacy
	// data without a tenant stamp could be deleted from any tenant context.
	//
	// Gated on the presence of an authenticated user so that non-auth
	// deployments (e.g., dev/test) continue to work — production must
	// always configure auth middleware upstream.
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		pool, err := adp.GetResourcePool(ctx, resourcePoolID)
		if err != nil {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"resource pool lookup failed during delete authorization",
				zap.String("resource_pool_id", resourcePoolID),
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			return
		}
		if !auth.AuthorizeTenantResource(ctx, pool.TenantID) {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"tenant attempting to delete resource pool from different tenant",
				zap.String("resource_pool_id", resourcePoolID),
				zap.String("tenant_id", tenantID),
				zap.String("pool_tenant_id", pool.TenantID),
			)
			return
		}
	}

	if err := adp.DeleteResourcePool(ctx, resourcePoolID); err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogResourceOperation(
				ctx,
				auth.AuditEventResourcePoolDeleted,
				"resourcepool",
				resourcePoolID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		if errors.Is(err, adapter.ErrResourcePoolNotFound) {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"adapter reported resource pool not found during delete",
				zap.String("resource_pool_id", resourcePoolID),
			)
			return
		}
		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete resource pool",
			"adapter failed to delete resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)
		return
	}

	// Audit log the successful deletion
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogResourceOperation(
			ctx,
			auth.AuditEventResourcePoolDeleted,
			"resourcepool",
			resourcePoolID,
			user,
			true,
			nil,
		)
	}

	// Decrement tenant usage after successful deletion
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.DecrementUsage(ctx, tenantID, "resource_pools"); err != nil {
			s.logger.Error("failed to decrement resource pool usage",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
		}
	}

	c.Status(http.StatusNoContent)
}

// Resource handlers
