package auth

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for auth operations.
var (
	// AuthenticationAttempts counts authentication attempts.
	AuthenticationAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "authentication_attempts_total",
			Help:      "Total number of authentication attempts",
		},
		[]string{"status", "method"},
	)

	// AuthenticationDuration measures authentication request duration.
	AuthenticationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "authentication_duration_seconds",
			Help:      "Duration of authentication requests in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	// AuthorizationChecks counts authorization checks.
	AuthorizationChecks = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "authorization_checks_total",
			Help:      "Total number of authorization checks",
		},
		[]string{"status", "permission"},
	)

	// TenantOperations counts tenant management operations.
	TenantOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "tenant_operations_total",
			Help:      "Total number of tenant operations",
		},
		[]string{"operation", "status"},
	)

	// UserOperations counts user management operations.
	UserOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "user_operations_total",
			Help:      "Total number of user operations",
		},
		[]string{"operation", "status"},
	)

	// RoleOperations counts role operations.
	RoleOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "role_operations_total",
			Help:      "Total number of role operations",
		},
		[]string{"operation", "status"},
	)

	// AuditEventsLogged counts audit events logged.
	AuditEventsLogged = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "audit_events_logged_total",
			Help:      "Total number of audit events logged",
		},
		[]string{"event_type"},
	)

	// QuotaUsage tracks quota usage per tenant.
	QuotaUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "quota_usage",
			Help:      "Current quota usage per tenant",
		},
		[]string{"tenant_id", "resource_type"},
	)

	// QuotaLimit tracks quota limits per tenant.
	QuotaLimit = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "quota_limit",
			Help:      "Quota limits per tenant",
		},
		[]string{"tenant_id", "resource_type"},
	)

	// QuotaExceeded counts quota exceeded errors.
	QuotaExceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "quota_exceeded_total",
			Help:      "Total number of quota exceeded errors",
		},
		[]string{"tenant_id", "resource_type"},
	)

	// ActiveTenants tracks the number of active tenants.
	ActiveTenants = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "active_tenants",
			Help:      "Number of active tenants",
		},
	)

	// ActiveUsers tracks the number of active users.
	ActiveUsers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "active_users",
			Help:      "Number of active users per tenant",
		},
		[]string{"tenant_id"},
	)

	// StorageOperations counts storage operations.
	StorageOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "storage_operations_total",
			Help:      "Total number of storage operations",
		},
		[]string{"operation", "entity", "status"},
	)

	// StorageOperationDuration measures storage operation duration.
	StorageOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "auth",
			Name:      "storage_operation_duration_seconds",
			Help:      "Duration of storage operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "entity"},
	)
)

// RecordAuthenticationAttempt records an authentication attempt.
func RecordAuthenticationAttempt(status, method string) {
	AuthenticationAttempts.WithLabelValues(status, method).Inc()
}

// RecordAuthorizationCheck records an authorization check.
func RecordAuthorizationCheck(status string, permission Permission) {
	AuthorizationChecks.WithLabelValues(status, string(permission)).Inc()
}

// RecordTenantOperation records a tenant operation.
func RecordTenantOperation(operation, status string) {
	TenantOperations.WithLabelValues(operation, status).Inc()
}

// RecordUserOperation records a user operation.
func RecordUserOperation(operation, status string) {
	UserOperations.WithLabelValues(operation, status).Inc()
}

// RecordRoleOperation records a role operation.
func RecordRoleOperation(operation, status string) {
	RoleOperations.WithLabelValues(operation, status).Inc()
}

// RecordAuditEvent records an audit event.
func RecordAuditEvent(eventType AuditEventType) {
	AuditEventsLogged.WithLabelValues(string(eventType)).Inc()
}

// UpdateQuotaMetrics updates quota metrics for a tenant.
func UpdateQuotaMetrics(tenantID string, usage TenantUsage, quota TenantQuota) {
	QuotaUsage.WithLabelValues(tenantID, "subscriptions").Set(float64(usage.Subscriptions))
	QuotaUsage.WithLabelValues(tenantID, "resource_pools").Set(float64(usage.ResourcePools))
	QuotaUsage.WithLabelValues(tenantID, "deployments").Set(float64(usage.Deployments))
	QuotaUsage.WithLabelValues(tenantID, "users").Set(float64(usage.Users))

	QuotaLimit.WithLabelValues(tenantID, "subscriptions").Set(float64(quota.MaxSubscriptions))
	QuotaLimit.WithLabelValues(tenantID, "resource_pools").Set(float64(quota.MaxResourcePools))
	QuotaLimit.WithLabelValues(tenantID, "deployments").Set(float64(quota.MaxDeployments))
	QuotaLimit.WithLabelValues(tenantID, "users").Set(float64(quota.MaxUsers))
}

// RecordQuotaExceeded records a quota exceeded event.
func RecordQuotaExceeded(tenantID, resourceType string) {
	QuotaExceeded.WithLabelValues(tenantID, resourceType).Inc()
}

// RecordStorageOperation records a storage operation.
func RecordStorageOperation(operation, entity, status string) {
	StorageOperations.WithLabelValues(operation, entity, status).Inc()
}
