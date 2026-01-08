// Package adapter provides metrics instrumentation for adapter implementations.
package adapter

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/trace"
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

	// CacheHits tracks the number of cache hits per adapter and operation.
	CacheHits *prometheus.CounterVec

	// CacheMisses tracks the number of cache misses per adapter and operation.
	CacheMisses *prometheus.CounterVec

	// ResourcesTotal tracks the total number of resources managed by each adapter.
	ResourcesTotal *prometheus.GaugeVec

	// ResourcePoolsTotal tracks the total number of resource pools per adapter.
	ResourcePoolsTotal *prometheus.GaugeVec

	// BackendRequestsTotal tracks backend API requests per adapter and endpoint.
	BackendRequestsTotal *prometheus.CounterVec

	// BackendLatency tracks backend API latency per adapter and endpoint.
	BackendLatency *prometheus.HistogramVec

	// BackendErrors tracks backend API errors per adapter and endpoint.
	BackendErrors *prometheus.CounterVec
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

	CacheHits: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"adapter", "operation"},
	),

	CacheMisses: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{"adapter", "operation"},
	),

	ResourcesTotal: promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "resources_total",
			Help:      "Total number of resources managed by adapter",
		},
		[]string{"adapter", "resource_type"},
	),

	ResourcePoolsTotal: promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "resource_pools_total",
			Help:      "Total number of resource pools per adapter",
		},
		[]string{"adapter"},
	),

	BackendRequestsTotal: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "backend_requests_total",
			Help:      "Total number of backend API requests",
		},
		[]string{"adapter", "endpoint", "method", "status"},
	),

	BackendLatency: promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "backend_latency_seconds",
			Help:      "Backend API latency in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
		},
		[]string{"adapter", "endpoint", "method"},
	),

	BackendErrors: promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "adapter",
			Name:      "backend_errors_total",
			Help:      "Total number of backend API errors",
		},
		[]string{"adapter", "endpoint", "method", "error_type"},
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

// RecordCacheHit records a cache hit for the specified adapter and operation.
//
// Example usage:
//
//	adapter.RecordCacheHit("kubernetes", "ListResources")
func RecordCacheHit(adapterName, operation string) {
	Metrics.CacheHits.WithLabelValues(adapterName, operation).Inc()
}

// RecordCacheMiss records a cache miss for the specified adapter and operation.
//
// Example usage:
//
//	adapter.RecordCacheMiss("kubernetes", "ListResources")
func RecordCacheMiss(adapterName, operation string) {
	Metrics.CacheMisses.WithLabelValues(adapterName, operation).Inc()
}

// UpdateResourceCount updates the total number of resources for an adapter and resource type.
//
// Example usage:
//
//	adapter.UpdateResourceCount("kubernetes", "node", len(nodes))
func UpdateResourceCount(adapterName, resourceType string, count int) {
	Metrics.ResourcesTotal.WithLabelValues(adapterName, resourceType).Set(float64(count))
}

// UpdateResourcePoolCount updates the total number of resource pools for an adapter.
//
// Example usage:
//
//	adapter.UpdateResourcePoolCount("kubernetes", len(pools))
func UpdateResourcePoolCount(adapterName string, count int) {
	Metrics.ResourcePoolsTotal.WithLabelValues(adapterName).Set(float64(count))
}

// ObserveBackendRequest records metrics for a backend API request.
// Call this at the end of each backend API call to record latency and success/failure.
//
// Example usage:
//
//	start := time.Now()
//	resp, err := client.Get("/api/v1/nodes")
//	adapter.ObserveBackendRequest("kubernetes", "/api/v1/nodes", "GET", start, resp.StatusCode, err)
func ObserveBackendRequest(adapterName, endpoint, method string, start time.Time, statusCode int, err error) {
	duration := time.Since(start).Seconds()
	status := "success"
	errorType := ""

	if err != nil {
		status = "error"
		errorType = "network_error"
		Metrics.BackendErrors.WithLabelValues(adapterName, endpoint, method, errorType).Inc()
	} else if statusCode >= 400 {
		status = "error"
		if statusCode >= 500 {
			errorType = "server_error"
		} else {
			errorType = "client_error"
		}
		Metrics.BackendErrors.WithLabelValues(adapterName, endpoint, method, errorType).Inc()
	}

	Metrics.BackendLatency.WithLabelValues(adapterName, endpoint, method).Observe(duration)
	Metrics.BackendRequestsTotal.WithLabelValues(adapterName, endpoint, method, status).Inc()
}

// ObserveOperationWithTracing records both metrics and tracing for an adapter operation.
// This is a convenience function that combines ObserveOperation with tracing span management.
// Call this at the end of each operation to record duration, success/failure, and close the span.
//
// Example usage:
//
//	ctx, span := adapter.StartSpan(ctx, "kubernetes", "ListResources")
//	defer adapter.ObserveOperationWithTracing("kubernetes", "ListResources", span, time.Now(), err)
//
//	resources, err := a.doListResources(ctx)
//	if err != nil {
//	    return nil, err
//	}
//	return resources, nil
func ObserveOperationWithTracing(adapterName, operation string, span trace.Span, start time.Time, err error) {
	// Record metrics
	ObserveOperation(adapterName, operation, start, err)

	// Record tracing
	if span != nil {
		if err != nil {
			RecordError(span, err)
		} else {
			RecordSuccess(span, 0)
		}
		span.End()
	}
}

// StartOperation starts a traced operation with automatic metrics recording.
// Returns a context with the span and a cleanup function that should be deferred.
//
// Example usage:
//
//	ctx, end := adapter.StartOperation(ctx, "kubernetes", "ListResources")
//	defer func() { end(err) }()
//
//	resources, err := a.doListResources(ctx)
//	if err != nil {
//	    return nil, err
//	}
//	return resources, nil
func StartOperation(ctx context.Context, adapterName, operation string) (context.Context, func(error)) {
	ctx, span := StartSpan(ctx, adapterName, operation)
	start := time.Now()

	cleanup := func(err error) {
		ObserveOperationWithTracing(adapterName, operation, span, start, err)
	}

	return ctx, cleanup
}
