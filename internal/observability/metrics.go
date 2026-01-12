package observability

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Metric status labels.
	statusSuccess = "success"
	statusError   = "error"
)

// Metrics holds all Prometheus metrics for the O2-IMS Gateway.
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	HTTPRequestsInFlight  prometheus.Gauge
	HTTPResponseSizeBytes *prometheus.HistogramVec

	// Adapter metrics
	AdapterOperationsTotal   *prometheus.CounterVec
	AdapterOperationDuration *prometheus.HistogramVec
	AdapterErrorsTotal       *prometheus.CounterVec

	// Subscription metrics
	SubscriptionsTotal      prometheus.Gauge
	SubscriptionEventsTotal *prometheus.CounterVec
	WebhookDeliveryDuration *prometheus.HistogramVec
	WebhookDeliveryTotal    *prometheus.CounterVec

	// Redis metrics
	RedisOperationsTotal   *prometheus.CounterVec
	RedisOperationDuration *prometheus.HistogramVec
	RedisConnectionsActive prometheus.Gauge
	RedisErrorsTotal       *prometheus.CounterVec

	// Kubernetes metrics
	K8sOperationsTotal   *prometheus.CounterVec
	K8sOperationDuration *prometheus.HistogramVec
	K8sResourceCacheSize *prometheus.GaugeVec
	K8sErrorsTotal       *prometheus.CounterVec

	// Batch operation metrics
	BatchOperationsTotal   *prometheus.CounterVec
	BatchOperationDuration *prometheus.HistogramVec
	BatchItemsProcessed    *prometheus.CounterVec
	BatchRollbacksTotal    *prometheus.CounterVec
	BatchConcurrentWorkers prometheus.Gauge
}

var (
	// globalMetrics is the singleton metrics instance.
	globalMetrics *Metrics
)

// InitMetrics initializes and registers all Prometheus metrics.
// Returns the existing metrics instance if already initialized (idempotent).
func InitMetrics(namespace string) *Metrics {
	// Return existing instance if already initialized
	if globalMetrics != nil {
		return globalMetrics
	}

	if namespace == "" {
		namespace = "o2ims"
	}

	m := &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),

		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path", "status"},
		),

		HTTPRequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being processed",
			},
		),

		HTTPResponseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		// Adapter metrics
		AdapterOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "adapter_operations_total",
				Help:      "Total number of adapter operations",
			},
			[]string{"adapter", "operation", "status"},
		),

		AdapterOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "adapter_operation_duration_seconds",
				Help:      "Adapter operation duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"adapter", "operation"},
		),

		AdapterErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "adapter_errors_total",
				Help:      "Total number of adapter errors",
			},
			[]string{"adapter", "operation", "error_type"},
		),

		// Subscription metrics
		SubscriptionsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "subscriptions_total",
				Help:      "Current number of active subscriptions",
			},
		),

		SubscriptionEventsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "subscription_events_total",
				Help:      "Total number of subscription events generated",
			},
			[]string{"event_type", "resource_type"},
		),

		WebhookDeliveryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "webhook_delivery_duration_seconds",
				Help:      "Webhook delivery latency in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
			},
			[]string{"status"},
		),

		WebhookDeliveryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "webhook_delivery_total",
				Help:      "Total number of webhook delivery attempts",
			},
			[]string{"status", "http_status"},
		),

		// Redis metrics
		RedisOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "redis_operations_total",
				Help:      "Total number of Redis operations",
			},
			[]string{"operation", "status"},
		),

		RedisOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "redis_operation_duration_seconds",
				Help:      "Redis operation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1, .25},
			},
			[]string{"operation"},
		),

		RedisConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "redis_connections_active",
				Help:      "Number of active Redis connections",
			},
		),

		RedisErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "redis_errors_total",
				Help:      "Total number of Redis errors",
			},
			[]string{"operation", "error_type"},
		),

		// Kubernetes metrics
		K8sOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "k8s_operations_total",
				Help:      "Total number of Kubernetes API operations",
			},
			[]string{"operation", "resource", "status"},
		),

		K8sOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "k8s_operation_duration_seconds",
				Help:      "Kubernetes API operation duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
			},
			[]string{"operation", "resource"},
		),

		K8sResourceCacheSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "k8s_resource_cache_size",
				Help:      "Number of Kubernetes resources cached",
			},
			[]string{"resource_type"},
		),

		K8sErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "k8s_errors_total",
				Help:      "Total number of Kubernetes API errors",
			},
			[]string{"operation", "resource", "error_type"},
		),

		// Batch operation metrics
		BatchOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "batch_operations_total",
				Help:      "Total number of batch operations",
			},
			[]string{"operation", "atomic", "status"},
		),

		BatchOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "batch_operation_duration_seconds",
				Help:      "Batch operation duration in seconds",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"operation"},
		),

		BatchItemsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "batch_items_processed_total",
				Help:      "Total number of items processed in batch operations",
			},
			[]string{"operation", "status"},
		),

		BatchRollbacksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "batch_rollbacks_total",
				Help:      "Total number of batch rollbacks",
			},
			[]string{"operation", "reason"},
		),

		BatchConcurrentWorkers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "batch_concurrent_workers",
				Help:      "Number of concurrent workers processing batch items",
			},
		),
	}

	globalMetrics = m
	return m
}

// GetMetrics returns the global metrics instance.
func GetMetrics() *Metrics {
	if globalMetrics == nil {
		panic("metrics not initialized - call InitMetrics first")
	}
	return globalMetrics
}

// RecordHTTPRequest records HTTP request metrics.
func (m *Metrics) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration, responseSize int) {
	status := strconv.Itoa(statusCode)
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())
	m.HTTPResponseSizeBytes.WithLabelValues(method, path).Observe(float64(responseSize))
}

// RecordAdapterOperation records adapter operation metrics.
func (m *Metrics) RecordAdapterOperation(adapter, operation string, duration time.Duration, err error) {
	status := statusSuccess
	if err != nil {
		status = statusError
		m.AdapterErrorsTotal.WithLabelValues(adapter, operation, "general").Inc()
	}
	m.AdapterOperationsTotal.WithLabelValues(adapter, operation, status).Inc()
	m.AdapterOperationDuration.WithLabelValues(adapter, operation).Observe(duration.Seconds())
}

// RecordSubscriptionEvent records subscription event metrics.
func (m *Metrics) RecordSubscriptionEvent(eventType, resourceType string) {
	m.SubscriptionEventsTotal.WithLabelValues(eventType, resourceType).Inc()
}

// RecordWebhookDelivery records webhook delivery metrics.
func (m *Metrics) RecordWebhookDelivery(duration time.Duration, httpStatusCode int, err error) {
	status := statusSuccess
	httpStatus := strconv.Itoa(httpStatusCode)

	if err != nil || httpStatusCode >= 400 {
		status = statusError
	}

	m.WebhookDeliveryDuration.WithLabelValues(status).Observe(duration.Seconds())
	m.WebhookDeliveryTotal.WithLabelValues(status, httpStatus).Inc()
}

// RecordRedisOperation records Redis operation metrics.
func (m *Metrics) RecordRedisOperation(operation string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
		m.RedisErrorsTotal.WithLabelValues(operation, "general").Inc()
	}
	m.RedisOperationsTotal.WithLabelValues(operation, status).Inc()
	m.RedisOperationDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordK8sOperation records Kubernetes API operation metrics.
func (m *Metrics) RecordK8sOperation(operation, resource string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
		m.K8sErrorsTotal.WithLabelValues(operation, resource, "general").Inc()
	}
	m.K8sOperationsTotal.WithLabelValues(operation, resource, status).Inc()
	m.K8sOperationDuration.WithLabelValues(operation, resource).Observe(duration.Seconds())
}

// SetSubscriptionCount sets the current subscription count.
func (m *Metrics) SetSubscriptionCount(count int) {
	m.SubscriptionsTotal.Set(float64(count))
}

// SetRedisConnectionsActive sets the number of active Redis connections.
func (m *Metrics) SetRedisConnectionsActive(count int) {
	m.RedisConnectionsActive.Set(float64(count))
}

// SetK8sResourceCacheSize sets the cache size for a specific resource type.
func (m *Metrics) SetK8sResourceCacheSize(resourceType string, size int) {
	m.K8sResourceCacheSize.WithLabelValues(resourceType).Set(float64(size))
}

// HTTPInFlightInc increments the in-flight HTTP request counter.
func (m *Metrics) HTTPInFlightInc() {
	m.HTTPRequestsInFlight.Inc()
}

// HTTPInFlightDec decrements the in-flight HTTP request counter.
func (m *Metrics) HTTPInFlightDec() {
	m.HTTPRequestsInFlight.Dec()
}

// RecordBatchOperation records batch operation metrics.
func (m *Metrics) RecordBatchOperation(
	operation string,
	atomic bool,
	duration time.Duration,
	successCount, failureCount int,
) {
	atomicStr := "false"
	if atomic {
		atomicStr = "true"
	}

	status := "success"
	if failureCount > 0 {
		if successCount > 0 {
			status = "partial"
		} else {
			status = "failure"
		}
	}

	m.BatchOperationsTotal.WithLabelValues(operation, atomicStr, status).Inc()
	m.BatchOperationDuration.WithLabelValues(operation).Observe(duration.Seconds())
	m.BatchItemsProcessed.WithLabelValues(operation, "success").Add(float64(successCount))
	m.BatchItemsProcessed.WithLabelValues(operation, "failure").Add(float64(failureCount))
}

// RecordBatchRollback records batch rollback metrics.
func (m *Metrics) RecordBatchRollback(operation, reason string, count int) {
	m.BatchRollbacksTotal.WithLabelValues(operation, reason).Add(float64(count))
}

// SetBatchConcurrentWorkers sets the current number of concurrent batch workers.
func (m *Metrics) SetBatchConcurrentWorkers(count int) {
	m.BatchConcurrentWorkers.Set(float64(count))
}
