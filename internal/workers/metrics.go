package workers

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WebhookDeliveriesTotal tracks the total number of webhook delivery attempts.
	WebhookDeliveriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "o2ims_webhook_deliveries_total",
			Help: "Total number of webhook delivery attempts",
		},
		[]string{"subscription_id", "status"},
	)

	// WebhookLatency tracks the latency of webhook deliveries.
	WebhookLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "o2ims_webhook_latency_seconds",
			Help:    "Webhook delivery latency in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"subscription_id"},
	)

	// WebhookRetriesTotal tracks the total number of webhook delivery retries.
	WebhookRetriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "o2ims_webhook_retries_total",
			Help: "Total number of webhook delivery retries",
		},
		[]string{"subscription_id", "attempt"},
	)

	// DeadLetterQueueTotal tracks the total number of events moved to DLQ.
	DeadLetterQueueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "o2ims_webhook_dlq_total",
			Help: "Total number of events moved to dead letter queue",
		},
		[]string{"subscription_id"},
	)

	// EventStreamLengthGauge tracks the current length of the event stream.
	EventStreamLengthGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "o2ims_event_stream_length",
			Help: "Current length of the event stream in Redis",
		},
	)

	// ActiveWorkersGauge tracks the current number of active webhook workers.
	ActiveWorkersGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "o2ims_active_webhook_workers",
			Help: "Current number of active webhook worker goroutines",
		},
	)
)
