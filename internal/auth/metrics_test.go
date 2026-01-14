package auth_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// TestRecordTenantOperation tests the RecordTenantOperation function.
func TestRecordTenantOperation(t *testing.T) {
	// Reset the counter
	auth.TenantOperations.Reset()

	// Record some operations
	auth.RecordTenantOperation("create", "success")
	auth.RecordTenantOperation("create", "success")
	auth.RecordTenantOperation("delete", "failed")

	// Verify metrics
	count := testutil.ToFloat64(auth.TenantOperations.WithLabelValues("create", "success"))
	require.Equal(t, 2.0, count)

	count = testutil.ToFloat64(auth.TenantOperations.WithLabelValues("delete", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordUserOperation tests the RecordUserOperation function.
func TestRecordUserOperation(t *testing.T) {
	// Reset the counter
	auth.UserOperations.Reset()

	// Record some operations
	auth.RecordUserOperation("create", "success")
	auth.RecordUserOperation("update", "success")
	auth.RecordUserOperation("delete", "failed")

	// Verify metrics
	count := testutil.ToFloat64(auth.UserOperations.WithLabelValues("create", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(auth.UserOperations.WithLabelValues("update", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(auth.UserOperations.WithLabelValues("delete", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordRoleOperation tests the RecordRoleOperation function.
func TestRecordRoleOperation(t *testing.T) {
	// Reset the counter
	auth.RoleOperations.Reset()

	// Record some operations
	auth.RecordRoleOperation("assign", "success")
	auth.RecordRoleOperation("revoke", "success")
	auth.RecordRoleOperation("assign", "failed")

	// Verify metrics
	count := testutil.ToFloat64(auth.RoleOperations.WithLabelValues("assign", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(auth.RoleOperations.WithLabelValues("revoke", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(auth.RoleOperations.WithLabelValues("assign", "failed"))
	require.Equal(t, 1.0, count)
}

// TestRecordAuditEvent tests the RecordAuditEvent function.
func TestRecordAuditEvent(t *testing.T) {
	// Reset the counter
	auth.AuditEventsLogged.Reset()

	// Record some events
	auth.RecordAuditEvent(auth.AuditEventAuthSuccess)
	auth.RecordAuditEvent(auth.AuditEventAuthSuccess)
	auth.RecordAuditEvent(auth.AuditEventAuthFailure)

	// Verify metrics
	count := testutil.ToFloat64(auth.AuditEventsLogged.WithLabelValues(string(auth.AuditEventAuthSuccess)))
	require.Equal(t, 2.0, count)

	count = testutil.ToFloat64(auth.AuditEventsLogged.WithLabelValues(string(auth.AuditEventAuthFailure)))
	require.Equal(t, 1.0, count)
}

// TestUpdateQuotaMetrics tests the UpdateQuotaMetrics function.
func TestUpdateQuotaMetrics(t *testing.T) {
	// Reset the gauges
	auth.QuotaUsage.Reset()
	auth.QuotaLimit.Reset()

	tenantID := "test-tenant"
	usage := auth.TenantUsage{
		Subscriptions: 10,
		ResourcePools: 5,
		Deployments:   3,
		Users:         8,
	}
	quota := auth.TenantQuota{
		MaxSubscriptions: 100,
		MaxResourcePools: 50,
		MaxDeployments:   30,
		MaxUsers:         80,
	}

	// Update metrics
	auth.UpdateQuotaMetrics(tenantID, usage, quota)

	// Verify usage metrics
	require.Equal(t, 10.0, testutil.ToFloat64(auth.QuotaUsage.WithLabelValues(tenantID, "subscriptions")))
	require.Equal(t, 5.0, testutil.ToFloat64(auth.QuotaUsage.WithLabelValues(tenantID, "resource_pools")))
	require.Equal(t, 3.0, testutil.ToFloat64(auth.QuotaUsage.WithLabelValues(tenantID, "deployments")))
	require.Equal(t, 8.0, testutil.ToFloat64(auth.QuotaUsage.WithLabelValues(tenantID, "users")))

	// Verify limit metrics
	require.Equal(t, 100.0, testutil.ToFloat64(auth.QuotaLimit.WithLabelValues(tenantID, "subscriptions")))
	require.Equal(t, 50.0, testutil.ToFloat64(auth.QuotaLimit.WithLabelValues(tenantID, "resource_pools")))
	require.Equal(t, 30.0, testutil.ToFloat64(auth.QuotaLimit.WithLabelValues(tenantID, "deployments")))
	require.Equal(t, 80.0, testutil.ToFloat64(auth.QuotaLimit.WithLabelValues(tenantID, "users")))
}

// Testauth.RecordQuotaExceeded tests the auth.RecordQuotaExceeded function.
func TestRecordQuotaExceeded(t *testing.T) {
	// Reset the counter
	auth.QuotaExceeded.Reset()

	tenantID := "test-tenant"

	// Record quota exceeded events
	auth.RecordQuotaExceeded(tenantID, "subscriptions")
	auth.RecordQuotaExceeded(tenantID, "subscriptions")
	auth.RecordQuotaExceeded(tenantID, "users")

	// Verify metrics
	count := testutil.ToFloat64(auth.QuotaExceeded.WithLabelValues(tenantID, "subscriptions"))
	require.Equal(t, 2.0, count)

	count = testutil.ToFloat64(auth.QuotaExceeded.WithLabelValues(tenantID, "users"))
	require.Equal(t, 1.0, count)
}

// TestRecordStorageOperation tests the RecordStorageOperation function.
func TestRecordStorageOperation(t *testing.T) {
	// Reset the counter
	auth.StorageOperations.Reset()

	// Record some operations
	auth.RecordStorageOperation("create", "tenant", "success")
	auth.RecordStorageOperation("read", "user", "success")
	auth.RecordStorageOperation("delete", "role", "failed")

	// Verify metrics
	count := testutil.ToFloat64(auth.StorageOperations.WithLabelValues("create", "tenant", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(auth.StorageOperations.WithLabelValues("read", "user", "success"))
	require.Equal(t, 1.0, count)

	count = testutil.ToFloat64(auth.StorageOperations.WithLabelValues("delete", "role", "failed"))
	require.Equal(t, 1.0, count)
}

// Testauth.RecordAuthenticationDuration tests the auth.RecordAuthenticationDuration function.
func TestRecordAuthenticationDuration(_ *testing.T) {
	// Reset the histogram
	auth.AuthenticationDuration.Reset()

	// Record durations for different statuses
	auth.RecordAuthenticationDuration("success", 0.001)
	auth.RecordAuthenticationDuration("success", 0.002)
	auth.RecordAuthenticationDuration("failed", 0.0005)
	auth.RecordAuthenticationDuration("error", 0.01)

	// Verify histogram has been updated by checking count
	// Note: histograms don't have a simple way to verify values,
	// but we can ensure the function doesn't panic
}

// Testauth.RecordStorageOperationDuration tests the auth.RecordStorageOperationDuration function.
func TestRecordStorageOperationDuration(_ *testing.T) {
	// Reset the histogram
	auth.StorageOperationDuration.Reset()

	// Record durations for different operations
	auth.RecordStorageOperationDuration("create", "tenant", 0.005)
	auth.RecordStorageOperationDuration("read", "user", 0.001)
	auth.RecordStorageOperationDuration("update", "role", 0.003)
	auth.RecordStorageOperationDuration("delete", "tenant", 0.002)

	// Verify histogram has been updated by checking it doesn't panic
	// and that operations can be recorded
}

// TestMetricsRegistration tests that all metrics are properly registered.
func TestMetricsRegistration(t *testing.T) {
	// Create a new registry
	registry := prometheus.NewRegistry()

	// Register all metrics
	collectors := []prometheus.Collector{
		auth.AuthenticationAttempts,
		auth.AuthenticationDuration,
		auth.AuthorizationChecks,
		auth.TenantOperations,
		auth.UserOperations,
		auth.RoleOperations,
		auth.AuditEventsLogged,
		auth.QuotaUsage,
		auth.QuotaLimit,
		auth.QuotaExceeded,
		auth.ActiveTenants,
		auth.ActiveUsers,
		auth.StorageOperations,
		auth.StorageOperationDuration,
	}

	for _, collector := range collectors {
		err := registry.Register(collector)
		// Registration may fail if already registered in default registry, which is okay
		if err != nil {
			require.Contains(t, err.Error(), "duplicate")
		}
	}
}
