package controllers

import (
	"crypto/sha256"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All metrics in this file use the canonical Namespace: "netweave",
// Subsystem: "controller" naming scheme. See internal/observability/doc.go for
// the project-wide metric naming rule.
//
// subscriptionBucketMask bounds the subscription_bucket label to 16 bits
// (0..65535 = 65536 values max per metric), matching the bucketing used in
// internal/events and internal/workers. See issue #497.
const subscriptionBucketMask = 0xffff

// SubscriptionBucket returns a deterministic 16-bit bucket identifier for a
// subscription ID (4-char lowercase hex), used as a bounded label value.
func SubscriptionBucket(subscriptionID string) string {
	if subscriptionID == "" {
		return "0000"
	}
	sum := sha256.Sum256([]byte(subscriptionID))
	bucket := (uint16(sum[0])<<8 | uint16(sum[1])) & subscriptionBucketMask
	return fmt.Sprintf("%04x", bucket)
}

var (
	// EventsProcessedTotal tracks the total number of events processed by the controller.
	EventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "controller",
			Name:      "events_processed_total",
			Help:      "Total number of subscription events processed",
		},
		[]string{"resource_type", "event_type"},
	)

	// EventsQueuedTotal tracks the total number of events queued for webhook delivery.
	// The subscription_bucket label is a 16-bit hash of the subscription ID (see #497).
	EventsQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "netweave",
			Subsystem: "controller",
			Name:      "events_queued_total",
			Help:      "Total number of subscription events queued for delivery",
		},
		[]string{"subscription_bucket", "resource_type"},
	)

	// ActiveSubscriptionsGauge tracks the current number of active subscriptions.
	ActiveSubscriptionsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "netweave",
			Subsystem: "controller",
			Name:      "active_subscriptions",
			Help:      "Current number of active subscriptions",
		},
	)

	// InformerSyncDuration tracks the time taken for informer cache sync.
	InformerSyncDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "netweave",
			Subsystem: "controller",
			Name:      "informer_sync_duration_seconds",
			Help:      "Time taken for informer cache sync",
			Buckets:   prometheus.DefBuckets,
		},
	)
)
