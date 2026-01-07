// Package adapter provides metrics instrumentation for adapter implementations.
package adapter

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics provides Prometheus metrics for adapter operations.
// These metrics should be used by all adapter implementations for consistent observability.
var Metrics = struct {
	// OperationDuration tracks the duration of adapter operations in seconds.
	OperationDuration *prometheus.HistogramVec

	// OperationTotal counts the total number of adapter operations.
	OperationTotal *prometheus.CounterVec

	// OperationErrors counts the total number of adapter operation errors.
	OperationErrors *prometheus.CounterVec

	// HealthCheckDuration tracks the duration of health checks in seconds.
	HealthCheckDuration *prometheus.HistogramVec

	// HealthCheckStatus tracks the status of health checks (1 = healthy, 0 = unhealthy).
	HealthCheckStatus *prometheus.GaugeVec

	// SubscriptionCount tracks the current number of active subscriptions.
	SubscriptionCount *prometheus.GaugeVec
}{
	OperationDuration: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "operation_duration_seconds",
			Help:      "Duration of adapter operations in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
		},
		[]string{"adapter", "operation", "status"},
	),

	OperationTotal: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "operations_total",
			Help:      "Total number of adapter operations",
		},
		[]string{"adapter", "operation", "status"},
	),

	OperationErrors: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "operation_errors_total",
			Help:      "Total number of adapter operation errors",
		},
		[]string{"adapter", "operation", "error_type"},
	),

	HealthCheckDuration: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "health_check_duration_seconds",
			Help:      "Duration of adapter health checks in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~5s
		},
		[]string{"adapter"},
	),

	HealthCheckStatus: promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "health_check_status",
			Help:      "Status of adapter health check (1 = healthy, 0 = unhealthy)",
		},
		[]string{"adapter"},
	),

	SubscriptionCount: promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "subscriptions_active",
			Help:      "Number of active subscriptions per adapter",
		},
		[]string{"adapter"},
	),
}

// ObserveOperation records metrics for an adapter operation.
// Call this at the end of each operation to record duration and success/failure.
//
// Example usage:
//
//	start := time.Now()
//	err := a.doOperation()
//	adapter.ObserveOperation("aws", "ListResources", start, err)
func ObserveOperation(adapterName, operation string, start time.Time, err error) {
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		Metrics.OperationErrors.WithLabelValues(adapterName, operation, "unknown").Inc()
	}

	Metrics.OperationDuration.WithLabelValues(adapterName, operation, status).Observe(duration)
	Metrics.OperationTotal.WithLabelValues(adapterName, operation, status).Inc()
}

// ObserveHealthCheck records metrics for an adapter health check.
//
// Example usage:
//
//	start := time.Now()
//	err := a.Health(ctx)
//	adapter.ObserveHealthCheck("aws", start, err)
func ObserveHealthCheck(adapterName string, start time.Time, err error) {
	duration := time.Since(start).Seconds()
	Metrics.HealthCheckDuration.WithLabelValues(adapterName).Observe(duration)

	status := float64(1)
	if err != nil {
		status = 0
	}
	Metrics.HealthCheckStatus.WithLabelValues(adapterName).Set(status)
}

// UpdateSubscriptionCount updates the active subscription count for an adapter.
//
// Example usage:
//
//	adapter.UpdateSubscriptionCount("aws", len(a.subscriptions))
func UpdateSubscriptionCount(adapterName string, count int) {
	Metrics.SubscriptionCount.WithLabelValues(adapterName).Set(float64(count))
}
