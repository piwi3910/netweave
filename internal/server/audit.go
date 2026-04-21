package server

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

// logAuditEvent logs an audit event with tenant context if auth store is configured.
func (s *Server) logAuditEvent(
	ctx context.Context,
	c *gin.Context,
	eventType auth.AuditEventType,
	resourceType, resourceID, action string,
	details map[string]string,
) {
	if s.AuthStore == nil {
		return // Auth store not configured, skip audit logging
	}

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	// Extract user information from context if available
	var userID, subject string
	if user, exists := c.Get("user"); exists {
		if authUser, ok := user.(*auth.AuthenticatedUser); ok {
			userID = authUser.UserID
			subject = authUser.Subject
		}
	}

	event := &auth.AuditEvent{
		ID:           uuid.New().String(),
		Type:         eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Subject:      subject,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Details:      details,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	}

	if err := s.AuthStore.LogEvent(ctx, event); err != nil {
		s.logger.Warn("failed to log audit event",
			zap.String("event_type", string(eventType)),
			zap.String("resource_type", resourceType),
			zap.String("resource_id", resourceID),
			zap.Error(err))
	}
}
