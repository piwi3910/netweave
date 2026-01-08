# Webhook Workers

This package implements webhook delivery workers for O2-IMS subscription notifications.

## Overview

Webhook workers consume events from Redis Streams and deliver HTTP POST notifications to subscriber callback URLs. They provide reliable delivery with retries, exponential backoff, and dead letter queues.

## Architecture

```
┌─────────────────────────────────────────┐
│     Event Queue (Redis Streams)         │
└─────────────────────────────────────────┘
              ↓ consumer group
┌─────────────────────────────────────────┐
│      Webhook Worker Pool (10+)          │
└─────────────────────────────────────────┘
              ↓ HTTP POST
┌─────────────────────────────────────────┐
│      SMO Callback URLs                  │
└─────────────────────────────────────────┘
```

## Features

- **Consumer Groups**: Multiple workers share event processing load
- **Retry Logic**: Exponential backoff with configurable max retries
- **HMAC Signatures**: Optional request signing for security
- **Dead Letter Queue**: Failed deliveries moved to DLQ for analysis
- **Metrics**: Comprehensive Prometheus metrics
- **Graceful Shutdown**: Clean worker termination

## Webhook Delivery

### Request Format

```http
POST {callback_url}
Content-Type: application/json
X-O2IMS-Event-Type: o2ims.Resource.Created
X-O2IMS-Notification-ID: notif-12345
X-O2IMS-Subscription-ID: sub-67890
X-O2IMS-Signature: abc123... (optional HMAC)

{
  "subscriptionId": "sub-67890",
  "notificationEventType": "o2ims.Resource.Created",
  "objectRef": "/o2ims/v1/resources/node-1",
  "resourceTypeId": "k8s-node",
  "resourcePoolId": "pool-1",
  "globalResourceId": "node-1",
  "timestamp": "2026-01-08T10:30:00Z",
  "notificationId": "notif-12345"
}
```

### Response

- **2xx**: Success - event acknowledged
- **4xx/5xx**: Failure - will retry with exponential backoff

## Usage

```go
import (
    "github.com/piwi3910/netweave/internal/workers"
)

// Create worker
worker, err := workers.NewWebhookWorker(&workers.Config{
    RedisClient:  redisClient,
    Logger:       logger,
    WorkerCount:  10,
    Timeout:      10 * time.Second,
    MaxRetries:   3,
    RetryBackoff: 1 * time.Second,
    MaxBackoff:   5 * time.Minute,
    HMACSecret:   "your-secret-key",
})

// Start workers (blocks until context cancelled)
ctx := context.Background()
if err := worker.Start(ctx); err != nil {
    log.Fatal(err)
}
```

## Configuration

- `WorkerCount`: Number of concurrent worker goroutines (default: 10)
- `Timeout`: HTTP client timeout (default: 10s)
- `MaxRetries`: Maximum retry attempts (default: 3)
- `RetryBackoff`: Base backoff duration (default: 1s)
- `MaxBackoff`: Maximum backoff duration (default: 5m)
- `HMACSecret`: Optional HMAC signing key

## Retry Strategy

The worker uses exponential backoff for retries:

- Attempt 1: Immediate
- Attempt 2: 1s delay
- Attempt 3: 2s delay
- Attempt 4: 4s delay
- etc., up to `MaxBackoff`

After `MaxRetries`, the event is moved to the dead letter queue.

## Dead Letter Queue

Failed events are stored in Redis Stream `o2ims:dlq` with:
- Original event data
- Failure timestamp
- Original message ID
- Subscription ID

## Metrics

- `o2ims_webhook_deliveries_total{subscription_id,status}`: Delivery attempts
- `o2ims_webhook_latency_seconds{subscription_id}`: Delivery latency
- `o2ims_webhook_retries_total{subscription_id,attempt}`: Retry count
- `o2ims_webhook_dlq_total{subscription_id}`: DLQ entries
- `o2ims_event_stream_length`: Event queue length
- `o2ims_active_webhook_workers`: Active worker count

## HMAC Signature Verification

When `HMACSecret` is configured, workers sign requests with:

```
X-O2IMS-Signature: hex(HMAC-SHA256(secret, body))
```

Subscribers should verify signatures to authenticate webhook sources.

## Testing

Run tests:

```bash
go test ./internal/workers/ -v
```

## Related Packages

- `internal/controllers`: Subscription controller (event producer)
- `internal/storage`: Subscription storage
- `internal/o2ims`: O2-IMS models
