package certmanager

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// certificateIssuances tracks certificate issuance operations.
	certificateIssuances = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "certmanager_certificate_issuances_total",
			Help: "Total number of certificate issuance attempts.",
		},
		[]string{"status"}, // success, failure
	)

	// certificateRevocations tracks certificate revocation operations.
	certificateRevocations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "certmanager_certificate_revocations_total",
			Help: "Total number of certificate revocation attempts.",
		},
		[]string{"status"}, // success, failure
	)

	// certificateRenewals tracks certificate renewal operations.
	certificateRenewals = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "certmanager_certificate_renewals_total",
			Help: "Total number of certificate renewal attempts.",
		},
		[]string{"status"}, // success, failure, max_retries_exceeded
	)

	// certificatesByStatus tracks certificates by their current status.
	certificatesByStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "certmanager_certificates_by_status",
			Help: "Current number of certificates grouped by status.",
		},
		[]string{"status"}, // active, expiring_soon, expired, revoked, renewal_pending, renewal_failed
	)

	// certificateLifetime tracks certificate lifetime in seconds.
	certificateLifetime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "certmanager_certificate_lifetime_seconds",
			Help:    "Certificate lifetime distribution in seconds.",
			Buckets: []float64{86400, 604800, 2592000, 7776000, 15552000, 31536000}, // 1d, 7d, 30d, 90d, 180d, 365d
		},
	)

	// keycloakUpdateFailures tracks Keycloak attribute update failures.
	keycloakUpdateFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "certmanager_keycloak_update_failures_total",
			Help: "Total number of Keycloak user attribute update failures.",
		},
	)

	// monitorLoopDuration tracks monitor loop execution time.
	monitorLoopDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "certmanager_monitor_loop_duration_seconds",
			Help:    "Monitor loop execution time in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)

	// renewalAttempts tracks renewal attempt counts.
	renewalAttempts = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "certmanager_renewal_attempts",
			Help:    "Distribution of renewal attempt counts before success or failure.",
			Buckets: []float64{1, 2, 3, 4, 5, 10},
		},
	)
)

// updateCertificateMetrics updates gauge metrics based on current certificate state
func (s *Service) updateCertificateMetrics() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count by status.
	statusCounts := make(map[CertificateStatus]float64)
	for _, cert := range s.certificates {
		statusCounts[cert.Status]++
	}

	// Update gauges.
	certificatesByStatus.WithLabelValues(string(CertStatusActive)).Set(statusCounts[CertStatusActive])
	certificatesByStatus.WithLabelValues(string(CertStatusExpiringSoon)).Set(statusCounts[CertStatusExpiringSoon])
	certificatesByStatus.WithLabelValues(string(CertStatusExpired)).Set(statusCounts[CertStatusExpired])
	certificatesByStatus.WithLabelValues(string(CertStatusRevoked)).Set(statusCounts[CertStatusRevoked])
	certificatesByStatus.WithLabelValues(string(CertStatusRenewalPending)).Set(statusCounts[CertStatusRenewalPending])
	certificatesByStatus.WithLabelValues(string(CertStatusRenewalFailed)).Set(statusCounts[CertStatusRenewalFailed])
}
