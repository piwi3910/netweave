package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EventsProcessedTotal tracks the total number of events processed by the controller.
	EventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "o2ims_subscription_events_processed_total",
			Help: "Total number of subscription events processed",
		},
		[]string{"resource_type", "event_type"},
	)

	// EventsQueuedTotal tracks the total number of events queued for webhook delivery.
	EventsQueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "o2ims_subscription_events_queued_total",
			Help: "Total number of subscription events queued for delivery",
		},
		[]string{"subscription_id", "resource_type"},
	)

	// ActiveSubscriptionsGauge tracks the current number of active subscriptions.
	ActiveSubscriptionsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "o2ims_active_subscriptions",
			Help: "Current number of active subscriptions",
		},
	)

	// InformerSyncDuration tracks the time taken for informer cache sync.
	InformerSyncDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "o2ims_informer_sync_duration_seconds",
			Help:    "Time taken for informer cache sync",
			Buckets: prometheus.DefBuckets,
		},
	)
)
