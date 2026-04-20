package certlifecycle

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// IssuancesTotal tracks total certificate issuances.
	IssuancesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "issuances_total",
			Help:      "Total number of certificate issuances",
		},
		[]string{"tenant_id", "role_name", "status"},
	)

	// RevocationsTotal tracks total certificate revocations.
	RevocationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "revocations_total",
			Help:      "Total number of certificate revocations",
		},
		[]string{"tenant_id"},
	)

	// RenewalsTotal tracks total certificate renewals by outcome.
	RenewalsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "renewals_total",
			Help:      "Total number of certificate renewals",
		},
		[]string{"tenant_id", "status"},
	)

	// ByStatusGauge tracks the current number of certificates per status.
	ByStatusGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "by_status",
			Help:      "Current number of certificates by lifecycle status",
		},
		[]string{"status"},
	)

	// LifetimeSeconds tracks the observed lifetime of certificates.
	LifetimeSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "lifetime_seconds",
			Help:      "Certificate lifetime from issuance to expiry in seconds",
			Buckets:   []float64{86400, 604800, 2592000, 7776000, 15552000, 31536000},
		},
		[]string{"tenant_id"},
	)

	// RenewalAttempts tracks the number of renewal attempts per certificate.
	RenewalAttempts = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "renewal_attempts",
			Help:      "Number of renewal attempts per certificate",
			Buckets:   []float64{1, 2, 3, 4, 5, 10},
		},
		[]string{"tenant_id", "status"},
	)

	// MonitorLoopDuration tracks the duration of each monitor scan loop.
	MonitorLoopDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "certificates",
			Name:      "monitor_loop_duration_seconds",
			Help:      "Duration of each certificate monitor scan loop in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
	)
)

// RecordIssuance records a certificate issuance metric.
func RecordIssuance(tenantID, roleName, status string) {
	IssuancesTotal.WithLabelValues(tenantID, roleName, status).Inc()
}

// RecordRevocation records a certificate revocation metric.
func RecordRevocation(tenantID string) {
	RevocationsTotal.WithLabelValues(tenantID).Inc()
}

// RecordRenewal records a certificate renewal metric with outcome status.
func RecordRenewal(tenantID, status string) {
	RenewalsTotal.WithLabelValues(tenantID, status).Inc()
}

// RecordCertsByStatus updates the gauge for all certificate statuses from a count map.
func RecordCertsByStatus(counts map[CertificateStatus]int64) {
	// Reset all known statuses to 0 so that statuses with no certs reflect correctly.
	allStatuses := []CertificateStatus{
		StatusActive, StatusExpiringSoon, StatusExpired,
		StatusRevoked, StatusRenewed, StatusRenewalFailed,
	}
	for _, s := range allStatuses {
		ByStatusGauge.WithLabelValues(string(s)).Set(0)
	}

	for status, count := range counts {
		ByStatusGauge.WithLabelValues(string(status)).Set(float64(count))
	}
}

// RecordLifetime records the observed certificate lifetime in seconds.
func RecordLifetime(tenantID string, lifetimeSeconds float64) {
	LifetimeSeconds.WithLabelValues(tenantID).Observe(lifetimeSeconds)
}

// RecordRenewalAttempts records the number of renewal attempts for a certificate.
func RecordRenewalAttempts(tenantID, status string, attempts int) {
	RenewalAttempts.WithLabelValues(tenantID, status).Observe(float64(attempts))
}

// RecordMonitorLoop records the duration of a single monitor scan loop.
func RecordMonitorLoop(durationSeconds float64) {
	MonitorLoopDuration.Observe(durationSeconds)
}
