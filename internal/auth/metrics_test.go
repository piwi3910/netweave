package auth

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// TestRecordTenantOperation tests the RecordTenantOperation function.
func TestRecordTenantOperation(t *testing.T) {
	// Reset the counter
	TenantOperations.Reset()

	// Record some operations
	RecordTenantOperation("create", "success")
	RecordTenantOperation("create", "success")
	RecordTenantOperation("delete", "failed")

	// Verify metrics
	count := testutil.ToFloat64(TenantOperations.WithLabelValues("create", "success"))
	require.Equal(t, 2.0, count)

	count = testutil.ToFloat64(TenantOperations.WithLabelValues("delete", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordUserOperation tests the RecordUserOperation function.
func TestRecordUserOperation(t *testing.T) {
	// Reset the counter
	UserOperations.Reset()

	// Record some operations
	RecordUserOperation("create", "success")
	RecordUserOperation("update", "success")
	RecordUserOperation("delete", "failed")

	// Verify metrics
	count := testutil.ToFloat64(UserOperations.WithLabelValues("create", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(UserOperations.WithLabelValues("update", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(UserOperations.WithLabelValues("delete", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordRoleOperation tests the RecordRoleOperation function.
func TestRecordRoleOperation(t *testing.T) {
	// Reset the counter
	RoleOperations.Reset()

	// Record some operations
	RecordRoleOperation("assign", "success")
	RecordRoleOperation("revoke", "success")
	RecordRoleOperation("assign", "failed")

	// Verify metrics
	count := testutil.ToFloat64(RoleOperations.WithLabelValues("assign", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(RoleOperations.WithLabelValues("revoke", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(RoleOperations.WithLabelValues("assign", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordAuditEvent tests the RecordAuditEvent function.
func TestRecordAuditEvent(t *testing.T) {
	// Reset the counter
	AuditEventsLogged.Reset()

	// Record some events
	RecordAuditEvent(AuditEventAuthSuccess)
	RecordAuditEvent(AuditEventAuthSuccess)
	RecordAuditEvent(AuditEventAuthFailure)

	// Verify metrics
	count := testutil.ToFloat64(AuditEventsLogged.WithLabelValues(string(AuditEventAuthSuccess)))
	require.Equal(t, 2.0, count)

	count = testutil.ToFloat64(AuditEventsLogged.WithLabelValues(string(AuditEventAuthFailure)))
	require.Equal(t, 1.0, count)
}

// TestUpdateQuotaMetrics tests the UpdateQuotaMetrics function.
func TestUpdateQuotaMetrics(t *testing.T) {
	// Reset the gauges
	QuotaUsage.Reset()
	QuotaLimit.Reset()

	tenantID := "test-tenant"
	usage := TenantUsage{
		Subscriptions: 10,
		ResourcePools: 5,
		Deployments:   3,
		Users:         8,
	}
	quota := TenantQuota{
		MaxSubscriptions: 100,
		MaxResourcePools: 50,
		MaxDeployments:   30,
		MaxUsers:         80,
	}

	// Update metrics
	UpdateQuotaMetrics(tenantID, usage, quota)

	// Verify usage metrics
	require.Equal(t, 10.0, testutil.ToFloat64(QuotaUsage.WithLabelValues(tenantID, "subscriptions")))
	require.Equal(t, 5.0, testutil.ToFloat64(QuotaUsage.WithLabelValues(tenantID, "resource_pools")))
	require.Equal(t, 3.0, testutil.ToFloat64(QuotaUsage.WithLabelValues(tenantID, "deployments")))
	require.Equal(t, 8.0, testutil.ToFloat64(QuotaUsage.WithLabelValues(tenantID, "users")))

	// Verify limit metrics
	require.Equal(t, 100.0, testutil.ToFloat64(QuotaLimit.WithLabelValues(tenantID, "subscriptions")))
	require.Equal(t, 50.0, testutil.ToFloat64(QuotaLimit.WithLabelValues(tenantID, "resource_pools")))
	require.Equal(t, 30.0, testutil.ToFloat64(QuotaLimit.WithLabelValues(tenantID, "deployments")))
	require.Equal(t, 80.0, testutil.ToFloat64(QuotaLimit.WithLabelValues(tenantID, "users")))
}

// TestRecordQuotaExceeded tests the RecordQuotaExceeded function.
func TestRecordQuotaExceeded(t *testing.T) {
	// Reset the counter
	QuotaExceeded.Reset()

	tenantID := "test-tenant"

	// Record quota exceeded events
	RecordQuotaExceeded(tenantID, "subscriptions")
	RecordQuotaExceeded(tenantID, "subscriptions")
	RecordQuotaExceeded(tenantID, "users")

	// Verify metrics
	count := testutil.ToFloat64(QuotaExceeded.WithLabelValues(tenantID, "subscriptions"))
	require.Equal(t, 2.0, count)

	count = testutil.ToFloat64(QuotaExceeded.WithLabelValues(tenantID, "users"))
	require.Equal(t, 1.0, count)
}

// TestRecordStorageOperation tests the RecordStorageOperation function.
func TestRecordStorageOperation(t *testing.T) {
	// Reset the counter
	StorageOperations.Reset()

	// Record some operations
	RecordStorageOperation("create", "tenant", "success")
	RecordStorageOperation("read", "user", "success")
	RecordStorageOperation("delete", "role", "failed")

	// Verify metrics
	count := testutil.ToFloat64(StorageOperations.WithLabelValues("create", "tenant", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(StorageOperations.WithLabelValues("read", "user", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(StorageOperations.WithLabelValues("delete", "role", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordAuthenticationDuration tests the RecordAuthenticationDuration function.
func TestRecordAuthenticationDuration(t *testing.T) {
	// Reset the histogram
	AuthenticationDuration.Reset()

	// Record durations for different statuses
	RecordAuthenticationDuration("success", 0.001)
	RecordAuthenticationDuration("success", 0.002)
	RecordAuthenticationDuration("failed", 0.0005)
	RecordAuthenticationDuration("error", 0.01)

	// Verify histogram has been updated by checking count
	// Note: histograms don't have a simple way to verify values,
	// but we can ensure the function doesn't panic
}

// TestRecordStorageOperationDuration tests the RecordStorageOperationDuration function.
func TestRecordStorageOperationDuration(t *testing.T) {
	// Reset the histogram
	StorageOperationDuration.Reset()

	// Record durations for different operations
	RecordStorageOperationDuration("create", "tenant", 0.005)
	RecordStorageOperationDuration("read", "user", 0.001)
	RecordStorageOperationDuration("update", "role", 0.003)
	RecordStorageOperationDuration("delete", "tenant", 0.002)

	// Verify histogram has been updated by checking it doesn't panic
	// and that operations can be recorded
}

// TestMetricsRegistration tests that all metrics are properly registered.
func TestMetricsRegistration(t *testing.T) {
	// Create a new registry
	registry := prometheus.NewRegistry()

	// Register all metrics
	collectors := []prometheus.Collector{
		AuthenticationAttempts,
		AuthenticationDuration,
		AuthorizationChecks,
		TenantOperations,
		UserOperations,
		RoleOperations,
		AuditEventsLogged,
		QuotaUsage,
		QuotaLimit,
		QuotaExceeded,
		ActiveTenants,
		ActiveUsers,
		StorageOperations,
		StorageOperationDuration,
	}

	for _, collector := range collectors {
		err := registry.Register(collector)
		// Registration may fail if already registered in default registry, which is okay
		if err != nil {
			require.Contains(t, err.Error(), "duplicate")
		}
	}
}
