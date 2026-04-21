package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/security/urlredact"
	"github.com/piwi3910/netweave/internal/storage"
)

// handleListSubscriptions lists all subscriptions.
// GET /o2ims/v1/subscriptions.
func (s *Server) handleListSubscriptions(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context for tenant isolation
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("listing subscriptions",
		zap.String("tenant_id", tenantID))

	// Get subscriptions from storage with tenant isolation
	var subs []*storage.Subscription
	var err error

	if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) {
		// Regular tenant user: only see their own subscriptions
		subs, err = s.store.ListByTenant(ctx, tenantID)
	} else {
		// Platform admin or no auth: see all subscriptions
		subs, err = s.store.List(ctx)
	}

	if err != nil {
		s.logger.Error("failed to list subscriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve subscriptions",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Convert to adapter subscriptions for response
	result := make([]*adapter.Subscription, 0, len(subs))
	for _, sub := range subs {
		result = append(result, &adapter.Subscription{
			SubscriptionID:         sub.ID,
			Callback:               sub.Callback,
			ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: sub.Filter.ResourcePoolID,
				ResourceTypeID: sub.Filter.ResourceTypeID,
				ResourceID:     sub.Filter.ResourceID,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"subscriptions": result,
		"total":         len(result),
	})
}

// handleCreateSubscription creates a new subscription.
// POST /o2ims/v1/subscriptions.
func (s *Server) handleCreateSubscription(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("creating subscription",
		zap.String("tenant_id", tenantID))

	var req adapter.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse subscription request body",
			zap.Error(err),
		)
		return
	}

	// Validate callback URL early for fast failure (SSRF protection).
	// Validator errors can include URL details — keep them out of the
	// client response and log them with the request_id for operators.
	if err := s.ValidateCallback(ctx, &req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid callback URL",
			"callback URL validation failed",
			zap.Error(err),
		)
		return
	}

	// Check tenant quota before creating subscription
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.IncrementUsage(ctx, tenantID, "subscriptions"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				s.logger.Warn("subscription quota exceeded",
					zap.String("tenant_id", tenantID))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "QuotaExceeded",
					"message": "Subscription quota exceeded for tenant",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
			s.logger.Error("failed to check subscription quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to check subscription quota",
				"code":    http.StatusInternalServerError,
			})
			return
		}
	}

	// Generate subscription ID
	req.SubscriptionID = "sub-" + uuid.New().String()

	// Create subscription via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	created, err := adp.CreateSubscription(ctx, &req)
	if err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogSubscriptionOperation(
				ctx,
				auth.AuditEventSubscriptionCreated,
				req.SubscriptionID,
				req.Callback,
				user,
				map[string]string{
					"error":     err.Error(),
					"tenant_id": tenantID,
				},
			)
		}

		// Rollback quota increment on failure
		if tenantID != "" && s.AuthStore != nil {
			if decErr := s.AuthStore.DecrementUsage(ctx, tenantID, "subscriptions"); decErr != nil {
				s.logger.Error("failed to rollback subscription quota",
					zap.String("tenant_id", tenantID),
					zap.Error(decErr))
			}
		}

		// Check for conflict error (subscription already exists)
		if errors.Is(err, adapter.ErrSubscriptionExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Subscription already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Update subscription with tenant ID (adapter already stored it in Redis)
	storageSub := &storage.Subscription{
		ID:                     created.SubscriptionID,
		Callback:               created.Callback,
		ConsumerSubscriptionID: created.ConsumerSubscriptionID,
		TenantID:               tenantID,
	}
	if created.Filter != nil {
		storageSub.Filter = storage.SubscriptionFilter{
			ResourcePoolID: created.Filter.ResourcePoolID,
			ResourceTypeID: created.Filter.ResourceTypeID,
			ResourceID:     created.Filter.ResourceID,
		}
	}

	if err := s.store.Update(ctx, storageSub); err != nil {
		s.logger.Error("failed to update subscription with tenant ID", zap.Error(err))
		// Attempt to clean up adapter subscription (best effort)
		_ = adp.DeleteSubscription(ctx, created.SubscriptionID)
		// Rollback quota increment
		if tenantID != "" && s.AuthStore != nil {
			if decErr := s.AuthStore.DecrementUsage(ctx, tenantID, "subscriptions"); decErr != nil {
				s.logger.Error("failed to rollback subscription quota",
					zap.String("tenant_id", tenantID),
					zap.Error(decErr))
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to store subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("subscription created",
		zap.String("subscription_id", created.SubscriptionID),
		zap.String("callback", urlredact.Redact(created.Callback)))

	// Audit log the successful creation
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogSubscriptionOperation(
			ctx,
			auth.AuditEventSubscriptionCreated,
			created.SubscriptionID,
			created.Callback,
			user,
			map[string]string{
				"consumer_subscription_id": created.ConsumerSubscriptionID,
				"tenant_id":                tenantID,
			},
		)
	}

	c.JSON(http.StatusCreated, created)
}

// handleGetSubscription retrieves a specific subscription.
// GET /o2ims/v1/subscriptions/:subscriptionId.
func (s *Server) handleGetSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context for tenant isolation
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("getting subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("tenant_id", tenantID))

	// Get subscription from storage. Both "not found" and "forbidden
	// (different tenant)" must return the same shape so the caller cannot
	// use response variance as a cross-tenant ID oracle.
	sub, err := s.store.Get(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"subscription lookup returned not-found",
				zap.String("subscription_id", subscriptionID),
				zap.String("tenant_id", tenantID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to retrieve subscription",
			"failed to get subscription from storage",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	// Tenant isolation: verify subscription belongs to tenant (unless platform admin).
	if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) && sub.TenantID != tenantID {
		s.writeNotFoundOrForbidden(c, "subscription",
			"tenant attempting to access subscription from different tenant",
			zap.String("tenant_id", tenantID),
			zap.String("subscription_tenant_id", sub.TenantID),
			zap.String("subscription_id", subscriptionID),
		)
		return
	}

	// Convert to adapter subscription for response
	result := &adapter.Subscription{
		SubscriptionID:         sub.ID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: sub.Filter.ResourcePoolID,
			ResourceTypeID: sub.Filter.ResourceTypeID,
			ResourceID:     sub.Filter.ResourceID,
		},
	}

	c.JSON(http.StatusOK, result)
}

// handleUpdateSubscription updates an existing subscription.
// PUT /o2ims/v1/subscriptions/:subscriptionId.
// This endpoint allows updating both the callback URL and/or subscription filters.
// When filter is null, it removes all filters; empty filter object {} also removes filters.
func (s *Server) handleUpdateSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context for tenant isolation
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("updating subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("tenant_id", tenantID))

	// Tenant isolation: verify subscription belongs to tenant before update.
	// Any "wrong tenant" or "missing" case returns the same not-found shape
	// so the caller cannot enumerate sibling tenants' IDs.
	if s.store != nil {
		sub, err := s.store.Get(ctx, subscriptionID)
		if err != nil {
			if errors.Is(err, storage.ErrSubscriptionNotFound) {
				s.writeNotFoundOrForbidden(c, "subscription",
					"subscription lookup returned not-found during update",
					zap.String("subscription_id", subscriptionID),
					zap.String("tenant_id", tenantID),
				)
				return
			}
			s.writeClientError(c, http.StatusInternalServerError, "InternalError",
				"failed to verify subscription ownership",
				"failed to get subscription for tenant check",
				zap.String("subscription_id", subscriptionID),
				zap.Error(err),
			)
			return
		}

		if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) && sub.TenantID != tenantID {
			s.writeNotFoundOrForbidden(c, "subscription",
				"tenant attempting to update subscription from different tenant",
				zap.String("tenant_id", tenantID),
				zap.String("subscription_tenant_id", sub.TenantID),
				zap.String("subscription_id", subscriptionID),
			)
			return
		}
	}

	var req adapter.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse subscription update body",
			zap.Error(err),
		)
		return
	}

	if err := s.ValidateCallback(ctx, &req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid callback URL",
			"callback URL validation failed during update",
			zap.Error(err),
		)
		return
	}

	// Update subscription via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	updated, err := adp.UpdateSubscription(c.Request.Context(), subscriptionID, &req)
	if err != nil {
		if errors.Is(err, adapter.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"adapter reported subscription not found during update",
				zap.String("subscription_id", subscriptionID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to update subscription",
			"adapter failed to update subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("subscription updated",
		zap.String("subscription_id", subscriptionID),
		zap.String("callback", urlredact.Redact(updated.Callback)))

	// Log audit event for subscription update
	s.logAuditEvent(
		c.Request.Context(),
		c,
		auth.AuditEventResourceModified,
		"subscription",
		subscriptionID,
		"subscription_updated",
		map[string]string{
			"callback": updated.Callback,
		},
	)

	c.JSON(http.StatusOK, updated)
}

// handleDeleteSubscription deletes a subscription.
// DELETE /o2ims/v1/subscriptions/:subscriptionId.
func (s *Server) handleDeleteSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("deleting subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("tenant_id", tenantID))

	// Get subscription to extract tenant ID for quota tracking and tenant isolation check
	var storedTenantID string
	if s.store != nil {
		sub, err := s.store.Get(ctx, subscriptionID)
		if err == nil {
			storedTenantID = sub.TenantID

			// Tenant isolation: identical shape for "not found" and "wrong tenant".
			if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) && sub.TenantID != tenantID {
				s.writeNotFoundOrForbidden(c, "subscription",
					"tenant attempting to delete subscription from different tenant",
					zap.String("tenant_id", tenantID),
					zap.String("subscription_tenant_id", sub.TenantID),
					zap.String("subscription_id", subscriptionID),
				)
				return
			}
		} else if errors.Is(err, storage.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"subscription lookup returned not-found during delete",
				zap.String("subscription_id", subscriptionID),
			)
			return
		}
	}

	// Delete from adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	if err := adp.DeleteSubscription(ctx, subscriptionID); err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogSubscriptionOperation(
				ctx,
				auth.AuditEventSubscriptionDeleted,
				subscriptionID,
				"",
				user,
				map[string]string{
					"error":     err.Error(),
					"tenant_id": storedTenantID,
				},
			)
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete subscription",
			"adapter failed to delete subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	// Delete from storage.
	if err := s.store.Delete(ctx, subscriptionID); err != nil {
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogSubscriptionOperation(
				ctx,
				auth.AuditEventSubscriptionDeleted,
				subscriptionID,
				"",
				user,
				map[string]string{
					"error":     err.Error(),
					"tenant_id": storedTenantID,
				},
			)
		}

		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"storage reported subscription not found during delete",
				zap.String("subscription_id", subscriptionID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete subscription",
			"storage failed to delete subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	// Decrement tenant quota after successful deletion
	if storedTenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.DecrementUsage(ctx, storedTenantID, "subscriptions"); err != nil {
			s.logger.Error("failed to decrement subscription quota",
				zap.String("tenant_id", storedTenantID),
				zap.Error(err))
			// Don't fail the delete operation if quota decrement fails
		}
	}

	s.logger.Info("subscription deleted", zap.String("subscription_id", subscriptionID))

	// Audit log the successful deletion
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogSubscriptionOperation(
			ctx,
			auth.AuditEventSubscriptionDeleted,
			subscriptionID,
			"",
			user,
			map[string]string{
				"tenant_id": storedTenantID,
			},
		)
	}

	c.Status(http.StatusNoContent)
}
