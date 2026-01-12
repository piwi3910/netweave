package events_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordEventGenerated tests the RecordEventGenerated function.
func TestRecordEventGenerated(t *testing.T) {
	eventsGeneratedTotal.Reset()

	t.Run("records event generation", func(t *testing.T) {
		RecordEventGenerated("Created", "k8s-node")
		RecordEventGenerated("Created", "k8s-node")
		RecordEventGenerated("Updated", "k8s-namespace")

		count := testutil.ToFloat64(eventsGeneratedTotal.WithLabelValues("Created", "k8s-node"))
		require.Equal(t, 2.0, count)

		count = testutil.ToFloat64(eventsGeneratedTotal.WithLabelValues("Updated", "k8s-namespace"))
		require.Equal(t, 1.0, count)
	})
}

// TestRecordQueueDepth tests the RecordQueueDepth function.
func TestRecordQueueDepth(t *testing.T) {
	t.Run("records queue depth", func(t *testing.T) {
		RecordQueueDepth(10.0)
		depth := testutil.ToFloat64(eventsQueueDepth)
		assert.Equal(t, 10.0, depth)

		RecordQueueDepth(25.0)
		depth = testutil.ToFloat64(eventsQueueDepth)
		assert.Equal(t, 25.0, depth)
	})

	t.Run("records zero depth", func(t *testing.T) {
		RecordQueueDepth(0.0)
		depth := testutil.ToFloat64(eventsQueueDepth)
		assert.Equal(t, 0.0, depth)
	})

	t.Run("handles large depths", func(t *testing.T) {
		RecordQueueDepth(1000000.0)
		depth := testutil.ToFloat64(eventsQueueDepth)
		assert.Equal(t, 1000000.0, depth)
	})
}

// TestRecordNotificationDelivered tests the RecordNotificationDelivered function.
func TestRecordNotificationDelivered(t *testing.T) {
	notificationsDeliveredTotal.Reset()

	t.Run("records successful delivery", func(t *testing.T) {
		RecordNotificationDelivered("success", "sub-123", 0.5, 1)
		count := testutil.ToFloat64(notificationsDeliveredTotal.WithLabelValues("success", "sub-123"))
		require.Equal(t, 1.0, count)
	})

	t.Run("records failed delivery", func(t *testing.T) {
		RecordNotificationDelivered("failed", "sub-123", 1.2, 3)
		count := testutil.ToFloat64(notificationsDeliveredTotal.WithLabelValues("failed", "sub-123"))
		require.Equal(t, 1.0, count)
	})

	t.Run("records multiple deliveries", func(t *testing.T) {
		RecordNotificationDelivered("success", "sub-456", 0.3, 1)
		RecordNotificationDelivered("success", "sub-456", 0.4, 1)
		count := testutil.ToFloat64(notificationsDeliveredTotal.WithLabelValues("success", "sub-456"))
		require.Equal(t, 2.0, count)
	})
}

// TestRecordNotificationResponseTime tests the RecordNotificationResponseTime function.
func TestRecordNotificationResponseTime(t *testing.T) {
	t.Run("records response time", func(_ *testing.T) {
		RecordNotificationResponseTime("sub-123", "200", 150.5)
		RecordNotificationResponseTime("sub-123", "200", 200.3)

		// Can't easily assert on histogram observations without accessing internal state
		// Just ensure it doesn't panic
	})

	t.Run("records different status codes", func(_ *testing.T) {
		RecordNotificationResponseTime("sub-123", "404", 50.0)
		RecordNotificationResponseTime("sub-123", "500", 100.0)
	})
}

// TestRecordCircuitBreakerState tests the RecordCircuitBreakerState function.
func TestRecordCircuitBreakerState(t *testing.T) {
	t.Run("records closed state", func(t *testing.T) {
		RecordCircuitBreakerState("http://example.com/callback", 0.0)
		state := testutil.ToFloat64(circuitBreakerState.WithLabelValues("http://example.com/callback"))
		assert.Equal(t, 0.0, state)
	})

	t.Run("records half-open state", func(t *testing.T) {
		RecordCircuitBreakerState("http://example.com/callback", 1.0)
		state := testutil.ToFloat64(circuitBreakerState.WithLabelValues("http://example.com/callback"))
		assert.Equal(t, 1.0, state)
	})

	t.Run("records open state", func(t *testing.T) {
		RecordCircuitBreakerState("http://example.com/callback", 2.0)
		state := testutil.ToFloat64(circuitBreakerState.WithLabelValues("http://example.com/callback"))
		assert.Equal(t, 2.0, state)
	})

	t.Run("tracks multiple callbacks", func(t *testing.T) {
		RecordCircuitBreakerState("http://example.com/callback1", 0.0)
		RecordCircuitBreakerState("http://example.com/callback2", 2.0)

		state1 := testutil.ToFloat64(circuitBreakerState.WithLabelValues("http://example.com/callback1"))
		assert.Equal(t, 0.0, state1)

		state2 := testutil.ToFloat64(circuitBreakerState.WithLabelValues("http://example.com/callback2"))
		assert.Equal(t, 2.0, state2)
	})
}

// TestRecordNotificationWorkersActive tests the RecordNotificationWorkersActive function.
func TestRecordNotificationWorkersActive(t *testing.T) {
	t.Run("records worker count", func(t *testing.T) {
		RecordNotificationWorkersActive(5)
		count := testutil.ToFloat64(notificationWorkersActive)
		assert.Equal(t, 5.0, count)

		RecordNotificationWorkersActive(10)
		count = testutil.ToFloat64(notificationWorkersActive)
		assert.Equal(t, 10.0, count)
	})

	t.Run("records zero workers", func(t *testing.T) {
		RecordNotificationWorkersActive(0)
		count := testutil.ToFloat64(notificationWorkersActive)
		assert.Equal(t, 0.0, count)
	})

	t.Run("handles large worker counts", func(t *testing.T) {
		RecordNotificationWorkersActive(100)
		count := testutil.ToFloat64(notificationWorkersActive)
		assert.Equal(t, 100.0, count)
	})
}

// TestRecordFailedDeliveries tests the RecordFailedDeliveries function.
func TestRecordFailedDeliveries(t *testing.T) {
	t.Run("records failed deliveries", func(t *testing.T) {
		RecordFailedDeliveries(3)
		count := testutil.ToFloat64(notificationFailedCurrent)
		assert.Equal(t, 3.0, count)

		RecordFailedDeliveries(7)
		count = testutil.ToFloat64(notificationFailedCurrent)
		assert.Equal(t, 7.0, count)
	})

	t.Run("records zero failures", func(t *testing.T) {
		RecordFailedDeliveries(0)
		count := testutil.ToFloat64(notificationFailedCurrent)
		assert.Equal(t, 0.0, count)
	})

	t.Run("handles large failure counts", func(t *testing.T) {
		RecordFailedDeliveries(1000)
		count := testutil.ToFloat64(notificationFailedCurrent)
		assert.Equal(t, 1000.0, count)
	})
}
