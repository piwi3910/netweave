package certlifecycle_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/certlifecycle"
)

// TestRecordIssuance tests the RecordIssuance recording function.
func TestRecordIssuance(t *testing.T) {
	certlifecycle.IssuancesTotal.Reset()

	t.Run("records single issuance", func(t *testing.T) {
		certlifecycle.RecordIssuance("tenant-1", "server", "success")
		count := testutil.ToFloat64(certlifecycle.IssuancesTotal.WithLabelValues("tenant-1", "server", "success"))
		require.Equal(t, 1.0, count)
	})

	t.Run("records multiple issuances same labels", func(t *testing.T) {
		certlifecycle.RecordIssuance("tenant-1", "server", "success")
		count := testutil.ToFloat64(certlifecycle.IssuancesTotal.WithLabelValues("tenant-1", "server", "success"))
		require.Equal(t, 2.0, count)
	})

	t.Run("records different label combinations", func(t *testing.T) {
		certlifecycle.RecordIssuance("tenant-2", "client", "failure")
		count := testutil.ToFloat64(certlifecycle.IssuancesTotal.WithLabelValues("tenant-2", "client", "failure"))
		require.Equal(t, 1.0, count)
	})
}

// TestRecordRevocation tests the RecordRevocation recording function.
func TestRecordRevocation(t *testing.T) {
	certlifecycle.RevocationsTotal.Reset()

	t.Run("records single revocation", func(t *testing.T) {
		certlifecycle.RecordRevocation("tenant-1")
		count := testutil.ToFloat64(certlifecycle.RevocationsTotal.WithLabelValues("tenant-1"))
		require.Equal(t, 1.0, count)
	})

	t.Run("records multiple revocations", func(t *testing.T) {
		certlifecycle.RecordRevocation("tenant-1")
		certlifecycle.RecordRevocation("tenant-1")
		count := testutil.ToFloat64(certlifecycle.RevocationsTotal.WithLabelValues("tenant-1"))
		require.Equal(t, 3.0, count)
	})

	t.Run("tracks different tenants independently", func(t *testing.T) {
		certlifecycle.RecordRevocation("tenant-2")
		count := testutil.ToFloat64(certlifecycle.RevocationsTotal.WithLabelValues("tenant-2"))
		require.Equal(t, 1.0, count)
	})
}

// TestRecordRenewal tests the RecordRenewal recording function.
func TestRecordRenewal(t *testing.T) {
	certlifecycle.RenewalsTotal.Reset()

	t.Run("records successful renewal", func(t *testing.T) {
		certlifecycle.RecordRenewal("tenant-1", "success")
		count := testutil.ToFloat64(certlifecycle.RenewalsTotal.WithLabelValues("tenant-1", "success"))
		require.Equal(t, 1.0, count)
	})

	t.Run("records failed renewal", func(t *testing.T) {
		certlifecycle.RecordRenewal("tenant-1", "failure")
		count := testutil.ToFloat64(certlifecycle.RenewalsTotal.WithLabelValues("tenant-1", "failure"))
		require.Equal(t, 1.0, count)
	})

	t.Run("records max_retries_exceeded", func(t *testing.T) {
		certlifecycle.RecordRenewal("tenant-1", "max_retries_exceeded")
		count := testutil.ToFloat64(certlifecycle.RenewalsTotal.WithLabelValues("tenant-1", "max_retries_exceeded"))
		require.Equal(t, 1.0, count)
	})
}

// TestRecordCertsByStatus tests the RecordCertsByStatus recording function.
func TestRecordCertsByStatus(t *testing.T) {
	certlifecycle.ByStatusGauge.Reset()

	t.Run("records counts for each status", func(t *testing.T) {
		counts := map[certlifecycle.CertificateStatus]int64{
			certlifecycle.StatusActive:        10,
			certlifecycle.StatusExpiringSoon:  3,
			certlifecycle.StatusExpired:       1,
			certlifecycle.StatusRevoked:       2,
			certlifecycle.StatusRenewed:       5,
			certlifecycle.StatusRenewalFailed: 0,
		}

		certlifecycle.RecordCertsByStatus(counts)

		assert.Equal(t, 10.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("active")))
		assert.Equal(t, 3.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("expiring_soon")))
		assert.Equal(t, 1.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("expired")))
		assert.Equal(t, 2.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("revoked")))
		assert.Equal(t, 5.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("renewed")))
		assert.Equal(t, 0.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("renewal_failed")))
	})

	t.Run("resets missing statuses to zero", func(t *testing.T) {
		// First set some statuses high.
		certlifecycle.RecordCertsByStatus(map[certlifecycle.CertificateStatus]int64{
			certlifecycle.StatusActive:  100,
			certlifecycle.StatusRevoked: 50,
		})

		// Now call with only active — revoked should reset to 0.
		certlifecycle.RecordCertsByStatus(map[certlifecycle.CertificateStatus]int64{
			certlifecycle.StatusActive: 20,
		})

		assert.Equal(t, 20.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("active")))
		assert.Equal(t, 0.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("revoked")))
		assert.Equal(t, 0.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("expired")))
	})

	t.Run("handles empty map", func(t *testing.T) {
		certlifecycle.RecordCertsByStatus(map[certlifecycle.CertificateStatus]int64{})

		assert.Equal(t, 0.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("active")))
		assert.Equal(t, 0.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("expiring_soon")))
		assert.Equal(t, 0.0, testutil.ToFloat64(certlifecycle.ByStatusGauge.WithLabelValues("expired")))
	})
}

// TestRecordLifetime tests the RecordLifetime recording function.
func TestRecordLifetime(t *testing.T) {
	t.Run("records lifetime observation", func(_ *testing.T) {
		// Histogram observations can't be easily asserted without internal state.
		// Verify it doesn't panic.
		certlifecycle.RecordLifetime("tenant-1", 86400)
		certlifecycle.RecordLifetime("tenant-1", 2592000)
		certlifecycle.RecordLifetime("tenant-2", 31536000)
	})
}

// TestRecordRenewalAttempts tests the RecordRenewalAttempts recording function.
func TestRecordRenewalAttempts(t *testing.T) {
	t.Run("records renewal attempts", func(_ *testing.T) {
		certlifecycle.RecordRenewalAttempts("tenant-1", "success", 1)
		certlifecycle.RecordRenewalAttempts("tenant-1", "failure", 3)
		certlifecycle.RecordRenewalAttempts("tenant-2", "max_retries_exceeded", 5)
	})
}

// TestRecordMonitorLoop tests the RecordMonitorLoop recording function.
func TestRecordMonitorLoop(t *testing.T) {
	t.Run("records monitor loop duration", func(_ *testing.T) {
		certlifecycle.RecordMonitorLoop(0.05)
		certlifecycle.RecordMonitorLoop(1.2)
		certlifecycle.RecordMonitorLoop(5.5)
	})
}
