package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

// handleGetTenantQuotas retrieves tenant quotas.
// GET /o2ims/v1/tenants/:tenantId/quotas.
func (s *Server) handleGetTenantQuotas(c *gin.Context) {
	tenantID := c.Param("tenantId")

	// Defense in depth: re-enforce tenant ownership at the handler
	// level so a future routing mistake cannot bypass the middleware
	// (issue #469 / C6). Returns 404 to avoid leaking tenant existence.
	if !s.enforceTenantOwnershipInline(c, tenantID) {
		return
	}

	s.logger.Info("getting tenant quotas", zap.String("tenant_id", tenantID))

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantID,
		"quotas": gin.H{
			"maxSubscriptions":  100,
			"maxResourcePools":  50,
			"maxResources":      1000,
			"usedSubscriptions": 10,
			"usedResourcePools": 5,
			"usedResources":     100,
		},
	})
}

// handleUpdateTenantQuotas updates tenant quotas.
// PUT /o2ims/v1/tenants/:tenantId/quotas.
func (s *Server) handleUpdateTenantQuotas(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenantId")

	// Defense in depth: re-enforce tenant ownership at the handler
	// level so a future routing mistake cannot bypass the middleware
	// (issue #469 / C6). Returns 404 to avoid leaking tenant existence.
	if !s.enforceTenantOwnershipInline(c, tenantID) {
		return
	}

	s.logger.Info("updating tenant quotas", zap.String("tenant_id", tenantID))

	var req struct {
		MaxSubscriptions int `json:"maxSubscriptions,omitempty"`
		MaxResourcePools int `json:"maxResourcePools,omitempty"`
		MaxResources     int `json:"maxResources,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Audit log the quota updates
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		if req.MaxSubscriptions > 0 {
			s.auditLogger.LogQuotaUpdate(ctx, tenantID, user, "maxSubscriptions", 0, req.MaxSubscriptions)
		}
		if req.MaxResourcePools > 0 {
			s.auditLogger.LogQuotaUpdate(ctx, tenantID, user, "maxResourcePools", 0, req.MaxResourcePools)
		}
		if req.MaxResources > 0 {
			s.auditLogger.LogQuotaUpdate(ctx, tenantID, user, "maxResources", 0, req.MaxResources)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantID,
		"quotas": gin.H{
			"maxSubscriptions": req.MaxSubscriptions,
			"maxResourcePools": req.MaxResourcePools,
			"maxResources":     req.MaxResources,
		},
		"updatedAt": "2024-01-01T00:00:00Z",
	})
}
