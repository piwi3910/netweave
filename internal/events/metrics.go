package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metric cardinality bounds for high-risk labels.
//
// Subscriptions and callback URLs are attacker-controlled in O2-IMS: a tenant
// with `subscriptions:create` permission can register arbitrary numbers of
// subscriptions with unique callback URLs. Without bounds, each new
// subscription creates a permanent, publicly-scrapable Prometheus series.
//
// To bound the label space we replace:
//   - subscriptionID -> 16-bit bucket (0000..ffff = 65536 series max per metric)
//   - callbackURL    -> host only (small enumerable set)
//
// These transformations are one-way and deterministic: a given subscription
// always hashes to the same bucket, so rates/counters remain useful for
// aggregated analysis, but no PII (tenant identifiers embedded in callback
// path tokens) is exposed via /metrics.
const (
	// subscriptionBucketMask bounds subscription_bucket to 16 bits (0..65535).
	// Expressed as the modulus applied to the first 4 hex chars of sha256(id).
	subscriptionBucketMask = 0xffff
)

var (
	// EventsGeneratedTotal tracks total number of events generated.
	EventsGeneratedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "events",
			Name:      "generated_total",
			Help:      "Total number of events generated",
		},
		[]string{"event_type", "resource_type"},
	)

	// Event queue metrics.
	eventsQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "events",
			Name:      "queued_total",
			Help:      "Total number of events queued",
		},
		[]string{"status"},
	)

	// EventsQueueDepth tracks the current depth of the event queue.
	EventsQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "events",
			Name:      "queue_depth",
			Help:      "Current depth of the event queue",
		},
	)

	// NotificationsDeliveredTotal tracks total number of notifications delivered.
	// The subscription_bucket label is a 16-bit hash of the subscription ID,
	// bounding cardinality to 65536 values regardless of tenant subscription count.
	NotificationsDeliveredTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "delivered_total",
			Help:      "Total number of notifications delivered",
		},
		[]string{"status", "subscription_bucket"},
	)

	notificationDeliveryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "delivery_duration_seconds",
			Help:      "Notification delivery duration in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
		[]string{"status"},
	)

	notificationAttempts = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "attempts",
			Help:      "Number of delivery attempts per notification",
			Buckets:   []float64{1, 2, 3, 4, 5, 10},
		},
		[]string{"status"},
	)

	notificationResponseTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "response_time_seconds",
			Help:      "Webhook endpoint response time in seconds",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
		},
		[]string{"http_status"},
	)

	// CircuitBreakerState tracks the state of circuit breakers for notification delivery.
	// The callback_host label is the host (and port) portion of the callback URL,
	// stripping query strings and path segments that may contain customer identifiers.
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=half-open, 2=open)",
		},
		[]string{"callback_host"},
	)

	// Subscription filtering metrics.
	subscriptionsMatched = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "subscriptions",
			Name:      "matched",
			Help:      "Number of subscriptions matched per event",
			Buckets:   []float64{0, 1, 2, 5, 10, 20, 50, 100},
		},
		[]string{"event_type"},
	)

	// NotificationWorkersActive tracks the number of active notification workers.
	NotificationWorkersActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "workers_active",
			Help:      "Number of active notification workers",
		},
	)

	// NotificationFailedCurrent tracks the current number of failed notification deliveries.
	NotificationFailedCurrent = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "notifications",
			Name:      "failed_current",
			Help:      "Current number of failed deliveries in dead letter queue",
		},
	)
)

// subscriptionBucket returns a deterministic 16-bit bucket identifier for a
// subscription ID. The result is always a 4-character lowercase hex string,
// bounding cardinality to 65536 values regardless of subscription count.
func subscriptionBucket(subscriptionID string) string {
	if subscriptionID == "" {
		return "0000"
	}
	sum := sha256.Sum256([]byte(subscriptionID))
	bucket := (uint16(sum[0])<<8 | uint16(sum[1])) & subscriptionBucketMask
	return fmt.Sprintf("%04x", bucket)
}

// callbackHost extracts a stable host identifier from a callback URL.
// Unparseable or empty URLs collapse to the fixed "unknown" label.
// Query strings and paths (which may embed customer identifiers) are discarded.
func callbackHost(callbackURL string) string {
	if callbackURL == "" {
		return "unknown"
	}
	parsed, err := url.Parse(callbackURL)
	if err != nil || parsed.Host == "" {
		// Fall back to a stable 8-char hash of the raw input when we cannot
		// extract a host. This keeps cardinality finite without leaking path.
		sum := sha256.Sum256([]byte(callbackURL))
		return "hash:" + hex.EncodeToString(sum[:4])
	}
	return parsed.Host
}

// RecordEventGenerated records an event generation.
func RecordEventGenerated(eventType, resourceType string) {
	EventsGeneratedTotal.WithLabelValues(eventType, resourceType).Inc()
}

// RecordEventQueued records an event being queued.
func RecordEventQueued(status string) {
	eventsQueuedTotal.WithLabelValues(status).Inc()
}

// RecordQueueDepth updates the current queue depth.
func RecordQueueDepth(depth float64) {
	EventsQueueDepth.Set(depth)
}

// RecordNotificationDelivered records a notification delivery.
// The subscription ID is hashed to a bounded 16-bit bucket before being used
// as a label value; see subscriptionBucket for rationale.
func RecordNotificationDelivered(status, subscriptionID string, duration float64, attempts int) {
	bucket := subscriptionBucket(subscriptionID)
	NotificationsDeliveredTotal.WithLabelValues(status, bucket).Inc()
	notificationDeliveryDuration.WithLabelValues(status).Observe(duration)
	notificationAttempts.WithLabelValues(status).Observe(float64(attempts))
}

// RecordNotificationResponseTime records the response time of a webhook endpoint.
// responseTimeMs is in milliseconds and will be converted to seconds for the metric.
// The subscriptionID is accepted for API compatibility but no longer used as a
// label value (see issue #497 for cardinality rationale).
func RecordNotificationResponseTime(_ /* subscriptionID */, httpStatus string, responseTimeMs float64) {
	notificationResponseTime.WithLabelValues(httpStatus).Observe(responseTimeMs / 1000.0)
}

// RecordCircuitBreakerState records the state of a circuit breaker.
// state: 0=closed, 1=half-open, 2=open. The callback URL is reduced to its
// host portion before being used as a label value to bound cardinality and
// prevent leaking customer identifiers embedded in path tokens.
func RecordCircuitBreakerState(callbackURL string, state float64) {
	CircuitBreakerState.WithLabelValues(callbackHost(callbackURL)).Set(state)
}

// RecordSubscriptionsMatched records the number of subscriptions matched for an event.
func RecordSubscriptionsMatched(eventType string, count int) {
	subscriptionsMatched.WithLabelValues(eventType).Observe(float64(count))
}

// RecordNotificationWorkersActive records the number of active notification workers.
func RecordNotificationWorkersActive(count int) {
	NotificationWorkersActive.Set(float64(count))
}

// RecordFailedDeliveries records the current number of failed deliveries.
func RecordFailedDeliveries(count int) {
	NotificationFailedCurrent.Set(float64(count))
}
