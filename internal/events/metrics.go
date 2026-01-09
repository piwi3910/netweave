package events

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Event generation metrics.
	eventsGeneratedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "events",
			Name:      "generated_total",
			Help:      "Total number of events generated",
		},
		[]string{"event_type", "resource_type"},
	)

	// Event queue metrics.
	eventsQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "events",
			Name:      "queued_total",
			Help:      "Total number of events queued",
		},
		[]string{"status"},
	)

	eventsQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "events",
			Name:      "queue_depth",
			Help:      "Current depth of the event queue",
		},
	)

	// Notification delivery metrics.
	notificationsDeliveredTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "delivered_total",
			Help:      "Total number of notifications delivered",
		},
		[]string{"status", "subscription_id"},
	)

	notificationDeliveryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "delivery_duration_seconds",
			Help:      "Notification delivery duration in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
		[]string{"status", "subscription_id"},
	)

	notificationAttemptsTotal = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "attempts_total",
			Help:      "Total number of delivery attempts per notification",
			Buckets:   []float64{1, 2, 3, 4, 5, 10},
		},
		[]string{"status", "subscription_id"},
	)

	notificationResponseTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "response_time_milliseconds",
			Help:      "Webhook endpoint response time in milliseconds",
			Buckets:   []float64{10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
		},
		[]string{"subscription_id", "http_status"},
	)

	// Circuit breaker metrics.
	circuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=half-open, 2=open)",
		},
		[]string{"callback_url"},
	)

	// Subscription filtering metrics.
	subscriptionsMatchedTotal = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "o2ims",
			Subsystem: "subscriptions",
			Name:      "matched_total",
			Help:      "Number of subscriptions matched per event",
			Buckets:   []float64{0, 1, 2, 5, 10, 20, 50, 100},
		},
		[]string{"event_type"},
	)

	// Worker metrics.
	notificationWorkersActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "workers_active",
			Help:      "Number of active notification workers",
		},
	)

	// Failed deliveries.
	notificationFailedCurrent = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "o2ims",
			Subsystem: "notifications",
			Name:      "failed_current",
			Help:      "Current number of failed deliveries in dead letter queue",
		},
	)
)

// RecordEventGenerated records an event generation.
func RecordEventGenerated(eventType, resourceType string) {
	eventsGeneratedTotal.WithLabelValues(eventType, resourceType).Inc()
}

// RecordEventQueued records an event being queued.
func RecordEventQueued(status string) {
	eventsQueuedTotal.WithLabelValues(status).Inc()
}

// RecordQueueDepth updates the current queue depth.
func RecordQueueDepth(depth float64) {
	eventsQueueDepth.Set(depth)
}

// RecordNotificationDelivered records a notification delivery.
func RecordNotificationDelivered(status, subscriptionID string, duration float64, attempts int) {
	notificationsDeliveredTotal.WithLabelValues(status, subscriptionID).Inc()
	notificationDeliveryDuration.WithLabelValues(status, subscriptionID).Observe(duration)
	notificationAttemptsTotal.WithLabelValues(status, subscriptionID).Observe(float64(attempts))
}

// RecordNotificationResponseTime records the response time of a webhook endpoint.
func RecordNotificationResponseTime(subscriptionID, httpStatus string, responseTimeMs float64) {
	notificationResponseTime.WithLabelValues(subscriptionID, httpStatus).Observe(responseTimeMs)
}

// RecordCircuitBreakerState records the state of a circuit breaker.
// state: 0=closed, 1=half-open, 2=open
func RecordCircuitBreakerState(callbackURL string, state float64) {
	circuitBreakerState.WithLabelValues(callbackURL).Set(state)
}

// RecordSubscriptionsMatched records the number of subscriptions matched for an event.
func RecordSubscriptionsMatched(eventType string, count int) {
	subscriptionsMatchedTotal.WithLabelValues(eventType).Observe(float64(count))
}

// RecordNotificationWorkersActive records the number of active notification workers.
func RecordNotificationWorkersActive(count int) {
	notificationWorkersActive.Set(float64(count))
}

// RecordFailedDeliveries records the current number of failed deliveries.
func RecordFailedDeliveries(count int) {
	notificationFailedCurrent.Set(float64(count))
}
