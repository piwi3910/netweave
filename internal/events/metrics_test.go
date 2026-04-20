package events_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/events"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGaugeMetricFloat64 is a helper for testing gauge metrics that record float64 values.
// It tests: setting values, zero values, and large values.
func testGaugeMetricFloat64(
	t *testing.T,
	recordFunc func(float64),
	gauge prometheus.Gauge,
	val1, val2, largeVal float64,
) {
	t.Helper()

	t.Run("records values", func(t *testing.T) {
		recordFunc(val1)
		result := testutil.ToFloat64(gauge)
		assert.Equal(t, val1, result)

		recordFunc(val2)
		result = testutil.ToFloat64(gauge)
		assert.Equal(t, val2, result)
	})

	t.Run("records zero value", func(t *testing.T) {
		recordFunc(0.0)
		result := testutil.ToFloat64(gauge)
		assert.Equal(t, 0.0, result)
	})

	t.Run("handles large values", func(t *testing.T) {
		recordFunc(largeVal)
		result := testutil.ToFloat64(gauge)
		assert.Equal(t, largeVal, result)
	})
}

// testGaugeMetricInt is a helper for testing gauge metrics that record int values.
// It tests: setting values, zero values, and large values.
func testGaugeMetricInt(t *testing.T, recordFunc func(int), gauge prometheus.Gauge, val1, val2, largeVal int) {
	t.Helper()

	t.Run("records values", func(t *testing.T) {
		recordFunc(val1)
		result := testutil.ToFloat64(gauge)
		assert.Equal(t, float64(val1), result)

		recordFunc(val2)
		result = testutil.ToFloat64(gauge)
		assert.Equal(t, float64(val2), result)
	})

	t.Run("records zero value", func(t *testing.T) {
		recordFunc(0)
		result := testutil.ToFloat64(gauge)
		assert.Equal(t, 0.0, result)
	})

	t.Run("handles large values", func(t *testing.T) {
		recordFunc(largeVal)
		result := testutil.ToFloat64(gauge)
		assert.Equal(t, float64(largeVal), result)
	})
}

// TestRecordEventGenerated tests the events.RecordEventGenerated function.
func TestRecordEventGenerated(t *testing.T) {
	events.EventsGeneratedTotal.Reset()

	t.Run("records event generation", func(t *testing.T) {
		events.RecordEventGenerated("Created", "k8s-node")
		events.RecordEventGenerated("Created", "k8s-node")
		events.RecordEventGenerated("Updated", "k8s-namespace")

		count := testutil.ToFloat64(events.EventsGeneratedTotal.WithLabelValues("Created", "k8s-node"))
		require.Equal(t, 2.0, count)

		count = testutil.ToFloat64(events.EventsGeneratedTotal.WithLabelValues("Updated", "k8s-namespace"))
		require.Equal(t, 1.0, count)
	})
}

// TestRecordQueueDepth tests the events.RecordQueueDepth function.
func TestRecordQueueDepth(t *testing.T) {
	testGaugeMetricFloat64(t, events.RecordQueueDepth, events.EventsQueueDepth, 10.0, 25.0, 1000000.0)
}

// TestRecordNotificationDelivered tests the events.RecordNotificationDelivered function.
// The second label is a bounded 16-bit bucket hash of the subscription ID (see #497);
// tests assert against hash stability rather than raw IDs.
func TestRecordNotificationDelivered(t *testing.T) {
	events.NotificationsDeliveredTotal.Reset()

	sub123Bucket := events.SubscriptionBucketForTest("sub-123")
	sub456Bucket := events.SubscriptionBucketForTest("sub-456")

	t.Run("records successful delivery", func(t *testing.T) {
		events.RecordNotificationDelivered("success", "sub-123", 0.5, 1)
		count := testutil.ToFloat64(events.NotificationsDeliveredTotal.WithLabelValues("success", sub123Bucket))
		require.Equal(t, 1.0, count)
	})

	t.Run("records failed delivery", func(t *testing.T) {
		events.RecordNotificationDelivered("failed", "sub-123", 1.2, 3)
		count := testutil.ToFloat64(events.NotificationsDeliveredTotal.WithLabelValues("failed", sub123Bucket))
		require.Equal(t, 1.0, count)
	})

	t.Run("records multiple deliveries", func(t *testing.T) {
		events.RecordNotificationDelivered("success", "sub-456", 0.3, 1)
		events.RecordNotificationDelivered("success", "sub-456", 0.4, 1)
		count := testutil.ToFloat64(events.NotificationsDeliveredTotal.WithLabelValues("success", sub456Bucket))
		require.Equal(t, 2.0, count)
	})

	t.Run("bucket is deterministic and 16-bit bounded", func(t *testing.T) {
		// Bucket must be the same for the same input and always 4 hex chars.
		b1 := events.SubscriptionBucketForTest("sub-stable-id")
		b2 := events.SubscriptionBucketForTest("sub-stable-id")
		require.Equal(t, b1, b2)
		require.Len(t, b1, 4)
	})
}

// TestRecordNotificationResponseTime tests the RecordNotificationResponseTime function.
func TestRecordNotificationResponseTime(t *testing.T) {
	t.Run("records response time", func(_ *testing.T) {
		events.RecordNotificationResponseTime("sub-123", "200", 150.5)
		events.RecordNotificationResponseTime("sub-123", "200", 200.3)

		// Can't easily assert on histogram observations without accessing internal state
		// Just ensure it doesn't panic
	})

	t.Run("records different status codes", func(_ *testing.T) {
		events.RecordNotificationResponseTime("sub-123", "404", 50.0)
		events.RecordNotificationResponseTime("sub-123", "500", 100.0)
	})
}

// TestRecordCircuitBreakerState tests the RecordCircuitBreakerState function.
// The label is now the host portion of the callback URL (see #497), so URLs
// sharing a host collapse to a single series while paths (which may embed
// customer identifiers) are dropped.
func TestRecordCircuitBreakerState(t *testing.T) {
	t.Run("records closed state", func(t *testing.T) {
		events.RecordCircuitBreakerState("http://example.com/callback", 0.0)
		state := testutil.ToFloat64(events.CircuitBreakerState.WithLabelValues("example.com"))
		assert.Equal(t, 0.0, state)
	})

	t.Run("records half-open state", func(t *testing.T) {
		events.RecordCircuitBreakerState("http://example.com/callback", 1.0)
		state := testutil.ToFloat64(events.CircuitBreakerState.WithLabelValues("example.com"))
		assert.Equal(t, 1.0, state)
	})

	t.Run("records open state", func(t *testing.T) {
		events.RecordCircuitBreakerState("http://example.com/callback", 2.0)
		state := testutil.ToFloat64(events.CircuitBreakerState.WithLabelValues("example.com"))
		assert.Equal(t, 2.0, state)
	})

	t.Run("callbacks with same host collapse to one series", func(t *testing.T) {
		events.RecordCircuitBreakerState("http://host-a.example.com/callback1", 0.0)
		events.RecordCircuitBreakerState("http://host-a.example.com/callback2", 2.0)

		// Only one series should exist for host-a.example.com; the second call
		// overwrites the gauge value set by the first.
		state := testutil.ToFloat64(events.CircuitBreakerState.WithLabelValues("host-a.example.com"))
		assert.Equal(t, 2.0, state)
	})

	t.Run("different hosts produce different series", func(t *testing.T) {
		events.RecordCircuitBreakerState("http://host-b.example.com/callback", 1.0)
		events.RecordCircuitBreakerState("http://host-c.example.com/callback", 2.0)

		stateB := testutil.ToFloat64(events.CircuitBreakerState.WithLabelValues("host-b.example.com"))
		assert.Equal(t, 1.0, stateB)

		stateC := testutil.ToFloat64(events.CircuitBreakerState.WithLabelValues("host-c.example.com"))
		assert.Equal(t, 2.0, stateC)
	})
}

// TestRecordNotificationWorkersActive tests the RecordNotificationWorkersActive function.
func TestRecordNotificationWorkersActive(t *testing.T) {
	testGaugeMetricInt(t, events.RecordNotificationWorkersActive, events.NotificationWorkersActive, 5, 10, 100)
}

// TestRecordFailedDeliveries tests the RecordFailedDeliveries function.
func TestRecordFailedDeliveries(t *testing.T) {
	testGaugeMetricInt(t, events.RecordFailedDeliveries, events.NotificationFailedCurrent, 3, 7, 1000)
}
