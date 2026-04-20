package workers

import (
	"crypto/sha256"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// subscriptionBucketMask bounds subscription_bucket to 16 bits (65536 values).
// The same bucketing function is implemented in internal/events/metrics.go so
// cross-package series share a label space without introducing an import cycle.
const subscriptionBucketMask = 0xffff

// SubscriptionBucket returns a deterministic 16-bit bucket identifier for a
// subscription ID (4-char lowercase hex). This is the label value used for
// webhook metrics to bound cardinality per issue #497.
func SubscriptionBucket(subscriptionID string) string {
	if subscriptionID == "" {
		return "0000"
	}
	sum := sha256.Sum256([]byte(subscriptionID))
	bucket := (uint16(sum[0])<<8 | uint16(sum[1])) & subscriptionBucketMask
	return fmt.Sprintf("%04x", bucket)
}

// All metrics in this file use the canonical Namespace: "netweave", Subsystem:
// "webhook" (or "events") naming scheme. See internal/observability/doc.go for
// the project-wide metric naming rule.
//
// The subscription_bucket label is a 16-bit hash of the subscription ID (see
// internal/events/metrics.go:subscriptionBucket); it bounds cardinality to
// 65536 values regardless of tenant subscription count (#497).
var (
	// WebhookDeliveriesTotal tracks the total number of webhook delivery attempts.
	WebhookDeliveriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "webhook",
			Name:      "deliveries_total",
			Help:      "Total number of webhook delivery attempts",
		},
		[]string{"subscription_bucket", "status"},
	)

	// WebhookLatency tracks the latency of webhook deliveries.
	WebhookLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "webhook",
			Name:      "latency_seconds",
			Help:      "Webhook delivery latency in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"subscription_bucket"},
	)

	// WebhookRetriesTotal tracks the total number of webhook delivery retries.
	WebhookRetriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "webhook",
			Name:      "retries_total",
			Help:      "Total number of webhook delivery retries",
		},
		[]string{"subscription_bucket", "attempt"},
	)

	// DeadLetterQueueTotal tracks the total number of events moved to DLQ.
	DeadLetterQueueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "webhook",
			Name:      "dlq_total",
			Help:      "Total number of events moved to dead letter queue",
		},
		[]string{"subscription_bucket"},
	)

	// EventStreamLengthGauge tracks the current length of the event stream.
	EventStreamLengthGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "events",
			Name:      "stream_length",
			Help:      "Current length of the event stream in Redis",
		},
	)

	// ActiveWorkersGauge tracks the current number of active webhook workers.
	ActiveWorkersGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "webhook",
			Name:      "active_workers",
			Help:      "Current number of active webhook worker goroutines",
		},
	)
)
