package server

import (
	"encoding/json"
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

// isAlphanumericOrHyphen checks if a character is alphanumeric or hyphen.
func isAlphanumericOrHyphen(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-'
}

// validateURNNamespaceID validates the URN namespace identifier (nid).
func validateURNNamespaceID(nid string) error {
	if len(nid) < 2 || len(nid) > 32 {
		return errors.New("URN namespace identifier must be 2-32 characters")
	}

	for i, ch := range nid {
		if !isAlphanumericOrHyphen(ch) {
			return errors.New("URN namespace identifier must contain only alphanumeric characters and hyphens")
		}
		if i == 0 && ch == '-' {
			return errors.New("URN namespace identifier must start with alphanumeric character")
		}
	}

	return nil
}

// validateURN validates URN format according to RFC 8141.
// URN format: urn:<nid>:<nss> where:
// - nid (Namespace Identifier): 2-32 alphanumeric characters, case-insensitive.
// - nss (Namespace Specific String): at least 1 character.
func validateURN(urn string) error {
	if !strings.HasPrefix(urn, "urn:") {
		return errors.New("globalAssetId must start with 'urn:'")
	}

	parts := strings.SplitN(urn, ":", 3)
	if len(parts) < 3 {
		return errors.New("globalAssetId must be in URN format: urn:<nid>:<nss> (e.g., urn:o-ran:resource:node-001)")
	}

	if err := validateURNNamespaceID(parts[1]); err != nil {
		return err
	}

	if len(parts[2]) == 0 {
		return errors.New("URN namespace specific string must not be empty")
	}

	return nil
}

// validateExtensions validates resource extensions for size and content.
func validateExtensions(extensions map[string]interface{}) error {
	if len(extensions) > MaxExtensionKeys {
		return fmt.Errorf("extensions map must not exceed %d keys", MaxExtensionKeys)
	}

	totalSize := 0
	for key, value := range extensions {
		if len(key) > MaxExtensionKeyLength {
			return fmt.Errorf("extension keys must not exceed %d characters", MaxExtensionKeyLength)
		}
		// Check JSON-marshaled size to prevent large payloads
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("extension value for key %q must be JSON-serializable", key)
		}
		if len(valueJSON) > MaxExtensionValueSize {
			return fmt.Errorf("extension values must not exceed %d bytes when JSON-encoded", MaxExtensionValueSize)
		}

		// Track total extensions payload size
		totalSize += len(valueJSON)
		if totalSize > MaxExtensionsTotalSize {
			return fmt.Errorf("total extensions payload must not exceed %d bytes (50KB)", MaxExtensionsTotalSize)
		}
	}

	return nil
}

// validateResourceFields validates resource field constraints.
func validateResourceFields(resource *adapter.Resource) error {
	// Validate GlobalAssetID format (URN) if provided
	if resource.GlobalAssetID != "" {
		if err := validateURN(resource.GlobalAssetID); err != nil {
			return err
		}
		if len(resource.GlobalAssetID) > 256 {
			return errors.New("globalAssetId must not exceed 256 characters")
		}
	}

	// Validate Description length
	if len(resource.Description) > 1000 {
		return errors.New("description must not exceed 1000 characters")
	}

	// Validate Extensions
	if resource.Extensions != nil {
		if err := validateExtensions(resource.Extensions); err != nil {
			return err
		}
	}

	return nil
}

// handleListResources lists all resources.
// GET /o2ims/v1/resources.
func (s *Server) handleListResources(c *gin.Context) {
	s.logger.Info("listing resources")

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

	// List resources via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resources, err := adp.ListResources(c.Request.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list resources", zap.Error(err))
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

// handleGetResource retrieves a specific resource.
// GET /o2ims/v1/resources/:resourceId.
func (s *Server) handleGetResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("getting resource", zap.String("resource_id", resourceID))

	// Get resource via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resource, err := adp.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		if errors.Is(err, adapter.ErrResourceNotFound) {
			s.writeNotFoundOrForbidden(c, "resource",
				"adapter reported resource not found",
				zap.String("resource_id", resourceID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to retrieve resource",
			"adapter failed to fetch resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return
	}

	// Tenant isolation: verify resource belongs to tenant (unless platform admin).
	// SECURITY: fail-closed via auth.AuthorizeTenantResource — a resource with
	// empty TenantID is inaccessible to non-platform-admin callers (issue #470).
	ctx := c.Request.Context()
	if !auth.AuthorizeTenantResource(ctx, resource.TenantID) {
		s.writeNotFoundOrForbidden(c, "resource",
			"tenant attempting to access resource from different tenant",
			zap.String("tenant_id", auth.TenantIDFromContext(ctx)),
			zap.String("resource_tenant_id", resource.TenantID),
			zap.String("resource_id", resourceID),
		)
		return
	}

	c.JSON(http.StatusOK, resource)
}

// validateCreateRequest validates required fields and constraints for resource creation.
func validateCreateRequest(req *adapter.Resource) error {
	if req.ResourceTypeID == "" {
		return adapter.ErrResourceTypeRequired
	}

	if req.ResourcePoolID == "" {
		return adapter.ErrResourcePoolRequired
	}

	return validateResourceFields(req)
}

// handleCreateResource creates a new resource.
// POST /o2ims/v1/resources.
func (s *Server) handleCreateResource(c *gin.Context) {
	s.logger.Info("creating resource")

	var req adapter.Resource
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Validate required fields and constraints
	if err := validateCreateRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Check tenant quota before creating resource
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.IncrementUsage(ctx, tenantID, "resources"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				s.logger.Warn("resource quota exceeded",
					zap.String("tenant_id", tenantID))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "QuotaExceeded",
					"message": "Resource quota exceeded for tenant",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
			s.logger.Error("failed to check resource quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to check resource quota",
				"code":    http.StatusInternalServerError,
			})
			return
		}
	}

	// Set tenant ID on the resource for isolation
	if tenantID != "" {
		req.TenantID = tenantID
	}

	// Generate resource ID if not provided (using plain UUID for simplicity)
	if req.ResourceID == "" {
		req.ResourceID = uuid.New().String()
	} else {
		// Validate client-provided resource ID is a valid UUID
		// This prevents path traversal attacks (e.g., "../../../etc/passwd")
		if _, err := uuid.Parse(req.ResourceID); err != nil {
			s.logger.Warn("invalid resource ID format",
				zap.String("resource_id", SanitizeForLogging(req.ResourceID)))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "BadRequest",
				"message": "resourceId must be a valid UUID",
				"code":    http.StatusBadRequest,
			})
			return
		}
	}

	// Create resource via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	created, err := adp.CreateResource(c.Request.Context(), &req)
	if err != nil {
		// Rollback quota increment on creation failure
		if tenantID != "" && s.AuthStore != nil {
			if rollbackErr := s.AuthStore.DecrementUsage(ctx, tenantID, "resources"); rollbackErr != nil {
				s.logger.Error("failed to rollback resource quota after creation failure",
					zap.String("tenant_id", tenantID),
					zap.Error(rollbackErr))
			}
		}

		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourceCreated,
				req.ResourceTypeID,
				req.ResourceID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		// Check if error indicates duplicate resource
		if errors.Is(err, adapter.ErrResourceExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource with ID " + SanitizeForLogging(req.ResourceID) + " already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource created",
		zap.String("resource_id", created.ResourceID),
		zap.String("resource_type_id", SanitizeForLogging(created.ResourceTypeID)))

	// Audit log the resource creation
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourceCreated,
			created.ResourceTypeID,
			created.ResourceID,
			user,
			true,
			map[string]string{
				"resource_pool_id": created.ResourcePoolID,
			},
		)
	}

	// Set Location header for REST compliance
	c.Header("Location", "/o2ims/v1/resources/"+created.ResourceID)
	c.JSON(http.StatusCreated, created)
}

// handleUpdateResource updates an existing resource.
// PUT /o2ims/v1/resources/:resourceId.
func (s *Server) handleUpdateResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("updating resource", zap.String("resource_id", resourceID))

	var req adapter.Resource
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Get existing resource
	existing, err := s.getExistingResource(c, resourceID)
	if err != nil || existing == nil {
		return // Response already sent
	}

	// SECURITY: verify tenant ownership before update. Without this check a
	// non-platform-admin caller with resources:update could modify any
	// tenant's resource. Fail-closed via auth.AuthorizeTenantResource so that
	// resources with empty TenantID cannot be reached by non-admin callers
	// (see issue #470 / issue #482).
	//
	// Gated on the presence of an authenticated user so that non-auth
	// deployments (dev/test) continue to work.
	ctx := c.Request.Context()
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		if !auth.AuthorizeTenantResource(ctx, existing.TenantID) {
			s.logger.Warn("tenant attempting to update resource from different tenant",
				zap.String("tenant_id", user.TenantID),
				zap.String("resource_tenant_id", existing.TenantID),
				zap.String("resource_id", resourceID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource not found: " + resourceID,
				"code":    http.StatusNotFound,
			})
			return
		}
		// Preserve existing tenant stamp; clients cannot move a resource
		// between tenants via update.
		req.TenantID = existing.TenantID
	}

	// Validate request
	if err := s.validateUpdateRequest(c, &req, existing); err != nil {
		return // Response already sent
	}

	// Apply update
	s.applyResourceUpdate(c, resourceID, &req, existing)
}

func (s *Server) handleDeleteResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)

	// Get resource info before deletion for audit logging and tenant ownership check.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	var resourceTypeID string
	existing, err := adp.GetResource(ctx, resourceID)
	if err == nil && existing != nil {
		resourceTypeID = existing.ResourceTypeID
	}

	// Verify tenant ownership before deletion. Identical shape is returned
	// for "not found" vs "wrong tenant" to prevent enumeration.
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		if err != nil {
			s.writeNotFoundOrForbidden(c, "resource",
				"resource lookup failed during delete authorization",
				zap.String("resource_id", resourceID),
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			return
		}
		if !auth.AuthorizeTenantResource(ctx, existing.TenantID) {
			s.writeNotFoundOrForbidden(c, "resource",
				"tenant attempting to delete resource from different tenant",
				zap.String("resource_id", resourceID),
				zap.String("tenant_id", tenantID),
				zap.String("resource_tenant_id", existing.TenantID),
			)
			return
		}
	}

	if err := adp.DeleteResource(ctx, resourceID); err != nil {
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogResourceOperation(
				ctx,
				auth.AuditEventResourceDeleted,
				resourceTypeID,
				resourceID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		if errors.Is(err, adapter.ErrResourceNotFound) {
			s.writeNotFoundOrForbidden(c, "resource",
				"adapter reported resource not found during delete",
				zap.String("resource_id", resourceID),
			)
			return
		}
		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete resource",
			"adapter failed to delete resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return
	}

	// Audit log the successful deletion
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogResourceOperation(
			ctx,
			auth.AuditEventResourceDeleted,
			resourceTypeID,
			resourceID,
			user,
			true,
			nil,
		)
	}

	// Decrement tenant usage after successful deletion
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.DecrementUsage(ctx, tenantID, "resources"); err != nil {
			s.logger.Error("failed to decrement resource usage",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
		}
	}

	c.Status(http.StatusNoContent)
}

// getExistingResource retrieves an existing resource and handles errors.
func (s *Server) getExistingResource(c *gin.Context, resourceID string) (*adapter.Resource, error) {
	adp := s.resolveAdapter(c)
	if adp == nil {
		return nil, errors.New("no adapter available")
	}

	existing, err := adp.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		if errors.Is(err, adapter.ErrResourceNotFound) {
			s.writeNotFoundOrForbidden(c, "resource",
				"adapter reported resource not found",
				zap.String("resource_id", resourceID),
			)
			return nil, fmt.Errorf("failed to get resource %s: %w", resourceID, err)
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to retrieve resource",
			"adapter failed to fetch resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get resource %s: %w", resourceID, err)
	}
	return existing, nil
}

// validateUpdateRequest validates update request and immutable fields.
func (s *Server) validateUpdateRequest(c *gin.Context, req, existing *adapter.Resource) error {
	// Validate field constraints
	if err := validateResourceFields(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return err
	}

	// Check immutable fields
	if err := checkImmutableFields(req, existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return err
	}

	return nil
}

// checkImmutableFields validates that immutable fields haven't changed.
func checkImmutableFields(req, existing *adapter.Resource) error {
	if req.ResourceTypeID != "" && req.ResourceTypeID != existing.ResourceTypeID {
		return errors.New("resourceTypeId is immutable and cannot be changed")
	}
	if req.ResourcePoolID != "" && req.ResourcePoolID != existing.ResourcePoolID {
		return errors.New("resourcePoolId is immutable and cannot be changed")
	}
	return nil
}

// applyResourceUpdate performs the update operation.
func (s *Server) applyResourceUpdate(c *gin.Context, resourceID string, req, existing *adapter.Resource) {
	// Preserve immutable fields
	req.ResourceID = resourceID
	if req.ResourceTypeID == "" {
		req.ResourceTypeID = existing.ResourceTypeID
	}
	if req.ResourcePoolID == "" {
		req.ResourcePoolID = existing.ResourcePoolID
	}

	// Update via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	updated, err := adp.UpdateResource(c.Request.Context(), resourceID, req)
	if err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourceModified,
				req.ResourceTypeID,
				resourceID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		s.logger.Error("failed to update resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to update resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource updated",
		zap.String("resource_id", updated.ResourceID),
		zap.String("resource_type_id", SanitizeForLogging(updated.ResourceTypeID)))

	// Audit log the successful update
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourceModified,
			updated.ResourceTypeID,
			updated.ResourceID,
			user,
			true,
			map[string]string{
				"resource_pool_id": updated.ResourcePoolID,
			},
		)
	}

	c.JSON(http.StatusOK, updated)
}

// Resource Type handlers
