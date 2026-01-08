# Subscription Controllers

This package implements the Kubernetes event notification system for O2-IMS subscriptions.

## Overview

The subscription controller watches Kubernetes resources using informers and delivers webhook notifications when resources change. It provides real-time event notifications to subscribers per the O2-IMS specification.

## Architecture

```
┌─────────────────────────────────────────┐
│      Subscription Controller            │
│  (Watches Resources, Queues Events)     │
└─────────────────────────────────────────┘
              ↓ watches
┌─────────────────────────────────────────┐
│    Kubernetes Resources (Nodes, NS)     │
└─────────────────────────────────────────┘
              ↓ change events
┌─────────────────────────────────────────┐
│     Event Queue (Redis Streams)         │
└─────────────────────────────────────────┘
              ↓ workers
┌─────────────────────────────────────────┐
│      Webhook Delivery Workers           │
└─────────────────────────────────────────┘
```

## Features

- **Kubernetes Informers**: Uses native K8s watch mechanisms for efficient event detection
- **Event Filtering**: Supports filtering by resource type, pool, and ID
- **Redis Streams**: Reliable event queueing with consumer groups
- **Metrics**: Prometheus metrics for monitoring event processing
- **High Availability**: Multiple controller instances share subscription load

## Watched Resources

- **Nodes**: Mapped to O2-IMS Resources
- **Namespaces**: Mapped to O2-IMS Resource Pools

## Usage

```go
import (
    "github.com/piwi3910/netweave/internal/controllers"
)

// Create controller
ctrl, err := controllers.NewSubscriptionController(&controllers.Config{
    K8sClient:   k8sClient,
    Store:       subscriptionStore,
    RedisClient: redisClient,
    Logger:      logger,
    OCloudID:    "my-ocloud",
})

// Start controller (blocks until context cancelled)
ctx := context.Background()
if err := ctrl.Start(ctx); err != nil {
    log.Fatal(err)
}
```

## Metrics

- `o2ims_subscription_events_processed_total`: Total events processed
- `o2ims_subscription_events_queued_total`: Total events queued for delivery
- `o2ims_active_subscriptions`: Current number of active subscriptions
- `o2ims_informer_sync_duration_seconds`: Informer cache sync duration

## Configuration

The controller is configured via the `Config` struct:

- `K8sClient`: Kubernetes client for API operations
- `Store`: Subscription storage backend (Redis)
- `RedisClient`: Redis client for event queue
- `Logger`: Structured logger
- `OCloudID`: O-Cloud identifier

## Testing

Run tests:

```bash
go test ./internal/controllers/ -v
```

## Related Packages

- `internal/workers`: Webhook delivery workers
- `internal/storage`: Subscription storage
- `internal/o2ims`: O2-IMS models and handlers
