package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AuditLogger provides convenient methods for logging audit events.
type AuditLogger struct {
	store  Store
	logger *zap.Logger
}

// NewAuditLogger creates a new AuditLogger.
func NewAuditLogger(store Store, logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		store:  store,
		logger: logger,
	}
}

// LogResourceOperation logs a resource operation (create, modify, delete).
func (a *AuditLogger) LogResourceOperation(
	ctx context.Context,
	eventType AuditEventType,
	resourceType string,
	resourceID string,
	user *AuthenticatedUser,
	success bool,
	details map[string]string,
) {
	event := a.buildBaseEvent(ctx, eventType, user)
	event.ResourceType = resourceType
	event.ResourceID = resourceID

	if details == nil {
		details = make(map[string]string)
	}
	if success {
		details["status"] = "success"
	} else {
		details["status"] = "failure"
	}
	event.Details = details

	a.logEvent(ctx, event)
}

// LogSubscriptionOperation logs a subscription operation.
func (a *AuditLogger) LogSubscriptionOperation(
	ctx context.Context,
	eventType AuditEventType,
	subscriptionID string,
	callback string,
	user *AuthenticatedUser,
	details map[string]string,
) {
	event := a.buildBaseEvent(ctx, eventType, user)
	event.ResourceType = "subscription"
	event.ResourceID = subscriptionID

	if details == nil {
		details = make(map[string]string)
	}
	details["callback"] = callback
	event.Details = details

	a.logEvent(ctx, event)
}

// LogConfigurationChange logs a configuration change event.
func (a *AuditLogger) LogConfigurationChange(
	ctx context.Context,
	eventType AuditEventType,
	configType string,
	user *AuthenticatedUser,
	oldValue string,
	newValue string,
) {
	event := a.buildBaseEvent(ctx, eventType, user)
	event.ResourceType = "configuration"
	event.ResourceID = configType
	event.Details = map[string]string{
		"config_type": configType,
		"old_value":   oldValue,
		"new_value":   newValue,
	}

	a.logEvent(ctx, event)
}

// LogAdminOperation logs an administrative operation.
func (a *AuditLogger) LogAdminOperation(
	ctx context.Context,
	eventType AuditEventType,
	operation string,
	user *AuthenticatedUser,
	details map[string]string,
) {
	event := a.buildBaseEvent(ctx, eventType, user)
	event.Action = operation
	event.Details = details

	a.logEvent(ctx, event)
}

// LogWebhookFailure logs a webhook delivery failure.
func (a *AuditLogger) LogWebhookFailure(
	ctx context.Context,
	subscriptionID string,
	callback string,
	errorMsg string,
	httpStatus int,
) {
	event := &AuditEvent{
		ID:           uuid.New().String(),
		Type:         AuditEventWebhookDeliveryFailed,
		ResourceType: "webhook",
		ResourceID:   subscriptionID,
		Action:       "delivery",
		Details: map[string]string{
			"callback":    callback,
			"error":       errorMsg,
			"http_status": intToString(httpStatus),
		},
		Timestamp: time.Now().UTC(),
	}

	a.logEvent(ctx, event)
}

// LogSignatureVerificationFailure logs a signature verification failure.
func (a *AuditLogger) LogSignatureVerificationFailure(
	ctx context.Context,
	subscriptionID string,
	clientIP string,
	reason string,
) {
	event := &AuditEvent{
		ID:           uuid.New().String(),
		Type:         AuditEventSignatureVerificationFailed,
		ResourceType: "webhook",
		ResourceID:   subscriptionID,
		Action:       "signature_verification",
		ClientIP:     clientIP,
		Details: map[string]string{
			"reason": reason,
		},
		Timestamp: time.Now().UTC(),
	}

	a.logEvent(ctx, event)
}

// LogTenantStatusChange logs a tenant status change (suspend/activate).
func (a *AuditLogger) LogTenantStatusChange(
	ctx context.Context,
	tenantID string,
	oldStatus TenantStatus,
	newStatus TenantStatus,
	user *AuthenticatedUser,
	reason string,
) {
	var eventType AuditEventType
	switch newStatus {
	case TenantStatusSuspended:
		eventType = AuditEventTenantSuspended
	case TenantStatusActive:
		eventType = AuditEventTenantActivated
	default:
		eventType = AuditEventTenantUpdated
	}

	event := a.buildBaseEvent(ctx, eventType, user)
	event.ResourceType = "tenant"
	event.ResourceID = tenantID
	event.Details = map[string]string{
		"old_status": string(oldStatus),
		"new_status": string(newStatus),
		"reason":     reason,
	}

	a.logEvent(ctx, event)
}

// LogUserStatusChange logs a user status change (enable/disable).
func (a *AuditLogger) LogUserStatusChange(
	ctx context.Context,
	userID string,
	enabled bool,
	actor *AuthenticatedUser,
	reason string,
) {
	eventType := AuditEventUserDisabled
	if enabled {
		eventType = AuditEventUserEnabled
	}

	event := a.buildBaseEvent(ctx, eventType, actor)
	event.ResourceType = "user"
	event.ResourceID = userID
	event.Details = map[string]string{
		"enabled": boolToString(enabled),
		"reason":  reason,
	}

	a.logEvent(ctx, event)
}

// LogQuotaUpdate logs a quota update.
func (a *AuditLogger) LogQuotaUpdate(
	ctx context.Context,
	tenantID string,
	user *AuthenticatedUser,
	quotaType string,
	oldValue int,
	newValue int,
) {
	event := a.buildBaseEvent(ctx, AuditEventQuotaUpdated, user)
	event.ResourceType = "quota"
	event.ResourceID = tenantID
	event.Details = map[string]string{
		"quota_type": quotaType,
		"old_value":  intToString(oldValue),
		"new_value":  intToString(newValue),
	}

	a.logEvent(ctx, event)
}

// LogBulkOperation logs a bulk administrative operation.
func (a *AuditLogger) LogBulkOperation(
	ctx context.Context,
	operationType string,
	resourceType string,
	affectedCount int,
	user *AuthenticatedUser,
	details map[string]string,
) {
	event := a.buildBaseEvent(ctx, AuditEventBulkOperation, user)
	event.ResourceType = resourceType
	event.Action = operationType

	if details == nil {
		details = make(map[string]string)
	}
	details["affected_count"] = intToString(affectedCount)
	event.Details = details

	a.logEvent(ctx, event)
}

// buildBaseEvent creates a base AuditEvent with common fields populated.
func (a *AuditLogger) buildBaseEvent(
	ctx context.Context,
	eventType AuditEventType,
	user *AuthenticatedUser,
) *AuditEvent {
	event := &AuditEvent{
		ID:        uuid.New().String(),
		Type:      eventType,
		Action:    string(eventType),
		Timestamp: time.Now().UTC(),
	}

	if user != nil {
		event.UserID = user.UserID
		event.TenantID = user.TenantID
		event.Subject = user.Subject
	}

	// Try to get client info from context
	if clientIP := ClientIPFromContext(ctx); clientIP != "" {
		event.ClientIP = clientIP
	}
	if userAgent := UserAgentFromContext(ctx); userAgent != "" {
		event.UserAgent = userAgent
	}

	return event
}

// logEvent logs the event to storage and structured logger.
func (a *AuditLogger) logEvent(ctx context.Context, event *AuditEvent) {
	// Log to structured logger
	a.logger.Info("audit event",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.String("tenant_id", event.TenantID),
		zap.String("user_id", event.UserID),
		zap.String("resource_type", event.ResourceType),
		zap.String("resource_id", event.ResourceID),
		zap.String("action", event.Action),
		zap.Any("details", event.Details),
		zap.String("client_ip", event.ClientIP),
	)

	// Store in Redis for persistence and querying
	if a.store != nil {
		if err := a.store.LogEvent(ctx, event); err != nil {
			a.logger.Error("failed to store audit event",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
		}
	}
}

// ClientIPFromContext extracts the client IP from context.
func ClientIPFromContext(ctx context.Context) string {
	if ip, ok := ctx.Value(contextKeyClientIP).(string); ok {
		return ip
	}
	return ""
}

// UserAgentFromContext extracts the user agent from context.
func UserAgentFromContext(ctx context.Context) string {
	if ua, ok := ctx.Value(contextKeyUserAgent).(string); ok {
		return ua
	}
	return ""
}

// Context keys for audit logging.
type contextKey string

const (
	contextKeyClientIP  contextKey = "client_ip"
	contextKeyUserAgent contextKey = "user_agent"
)

// WithClientIP adds client IP to context.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, contextKeyClientIP, ip)
}

// WithUserAgent adds user agent to context.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, contextKeyUserAgent, ua)
}

// Helper functions for string conversion.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intToString(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
