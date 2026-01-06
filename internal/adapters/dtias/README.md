# Dell DTIAS Bare-Metal Adapter

## Overview

The Dell DTIAS adapter provides O2-IMS integration with Dell's DTIAS (Dell Technologies Infrastructure as a Service) bare-metal provisioning platform. This adapter translates O2-IMS API operations to DTIAS REST API calls, enabling bare-metal server management for edge deployments.

## Resource Mapping

The adapter maps O2-IMS resources to DTIAS infrastructure components:

| O2-IMS Resource | DTIAS Resource | Description |
|----------------|----------------|-------------|
| **Resource Pool** | Server Pool | Logical grouping of physical servers |
| **Resource** | Physical Server | Bare-metal server with full hardware inventory |
| **Resource Type** | Server Type | Server hardware profile (CPU, RAM, storage) |
| **Deployment Manager** | Datacenter Metadata | DTIAS datacenter information |

## Features

### Core Capabilities

- **Bare-Metal Provisioning**: Provision and deprovision physical servers
- **Hardware Inventory**: Full hardware inventory (CPU, memory, storage, network)
- **Power Management**: Control server power state (on/off/reset/cycle)
- **Health Monitoring**: Real-time hardware health metrics (temperature, fans, voltages)
- **BIOS Configuration**: Server BIOS management
- **Server Pools**: Logical grouping and management of servers

### Production Features

- **mTLS Authentication**: Mutual TLS for secure API communication
- **Automatic Retries**: Configurable retry logic with exponential backoff
- **Error Handling**: Comprehensive error handling with context preservation
- **Context Propagation**: Full support for Go context cancellation and timeouts
- **Structured Logging**: Detailed logging with zap for observability
- **Health Checks**: Readiness and liveness health checks

## Configuration

### Basic Configuration

```yaml
plugins:
  ims:
    - name: dtias-edge
      type: dtias
      enabled: true
      config:
        endpoint: https://dtias.dell.com/api/v1
        apiKey: ${DTIAS_API_KEY}
        ocloudId: ocloud-dtias-edge-1
        datacenter: dc-dallas-1
        timeout: 30s
        retryAttempts: 3
        retryDelay: 2s
```

### mTLS Configuration

```yaml
plugins:
  ims:
    - name: dtias-edge
      type: dtias
      enabled: true
      config:
        endpoint: https://dtias.dell.com/api/v1
        apiKey: ${DTIAS_API_KEY}
        clientCert: /etc/netweave/certs/dtias-client.pem
        clientKey: /etc/netweave/certs/dtias-client-key.pem
        caCert: /etc/netweave/certs/dtias-ca.pem
        ocloudId: ocloud-dtias-edge-1
        datacenter: dc-dallas-1
```

### Configuration Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `endpoint` | string | Yes | - | DTIAS API endpoint URL |
| `apiKey` | string | Yes | - | DTIAS API authentication key |
| `clientCert` | string | No | - | Path to client certificate for mTLS |
| `clientKey` | string | No | - | Path to client key for mTLS |
| `caCert` | string | No | - | Path to CA certificate for server verification |
| `insecureSkipVerify` | bool | No | false | Skip TLS certificate verification (NOT for production) |
| `timeout` | duration | No | 30s | HTTP client timeout |
| `ocloudId` | string | Yes | - | O-Cloud identifier |
| `deploymentManagerId` | string | No | auto | Deployment manager identifier |
| `datacenter` | string | No | - | DTIAS datacenter identifier |
| `retryAttempts` | int | No | 3 | Number of retry attempts for failed API calls |
| `retryDelay` | duration | No | 2s | Delay between retry attempts |

## Usage Examples

### Creating an Adapter

```go
import (
    "github.com/piwi3910/netweave/internal/adapters/dtias"
    "time"
)

// Create adapter
adapter, err := dtias.New(&dtias.Config{
    Endpoint:            "https://dtias.example.com/api/v1",
    APIKey:              "your-api-key",
    ClientCert:          "/path/to/cert.pem",
    ClientKey:           "/path/to/key.pem",
    CACert:              "/path/to/ca.pem",
    Timeout:             30 * time.Second,
    OCloudID:            "ocloud-dtias-1",
    DeploymentManagerID: "ocloud-dtias-edge-1",
    Datacenter:          "dc-dallas-1",
    RetryAttempts:       3,
    RetryDelay:          2 * time.Second,
})
if err != nil {
    log.Fatalf("failed to create adapter: %v", err)
}
defer adapter.Close()
```

### Listing Server Pools

```go
// List all server pools
pools, err := adapter.ListResourcePools(ctx, nil)
if err != nil {
    log.Fatalf("failed to list pools: %v", err)
}

// List pools with filter
pools, err := adapter.ListResourcePools(ctx, &adapter.Filter{
    Location: "dc-dallas-1",
    Limit:    10,
    Labels: map[string]string{
        "environment": "production",
    },
})
```

### Provisioning a Server

```go
// Provision a new server
resource, err := adapter.CreateResource(ctx, &adapter.Resource{
    ResourceTypeID: "dtias-server-type-r640",
    ResourcePoolID: "pool-123",
    Extensions: map[string]interface{}{
        "dtias.hostname":        "edge-server-01",
        "dtias.operatingSystem": "ubuntu-22.04",
        "dtias.networkConfig": map[string]interface{}{
            "vlan": 100,
        },
    },
})
if err != nil {
    log.Fatalf("failed to provision server: %v", err)
}

fmt.Printf("Provisioned server: %s\n", resource.ResourceID)
```

### Power Management

```go
// Power on a server
err := adapter.PowerControl(ctx, "server-456", dtias.PowerOn)
if err != nil {
    log.Fatalf("failed to power on server: %v", err)
}

// Reset a server
err := adapter.PowerControl(ctx, "server-456", dtias.PowerReset)
```

### Health Monitoring

```go
// Get server health metrics
metrics, err := adapter.GetHealthMetrics(ctx, "server-456")
if err != nil {
    log.Fatalf("failed to get health metrics: %v", err)
}

fmt.Printf("CPU Utilization: %.2f%%\n", metrics.CPUUtilization)
fmt.Printf("Memory Utilization: %.2f%%\n", metrics.MemoryUtilization)
fmt.Printf("CPU Temperature: %.1f°C\n", metrics.CPUTemperature)
fmt.Printf("Power Consumption: %dW\n", metrics.PowerConsumptionWatts)
```

## Subscriptions

**Important**: DTIAS does not have a native event/subscription system. The adapter does not support `CreateSubscription()`, `GetSubscription()`, or `DeleteSubscription()`.

### Implementing Subscriptions via Polling

To implement subscription-like functionality with DTIAS, use a polling-based approach at the gateway layer:

```go
// Get polling recommendations
recommendation := adapter.GetPollingRecommendation()

// Recommended intervals:
// - Resource pools: 60s
// - Resources: 30s
// - Health metrics: 10s

// Example polling implementation:
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        // Poll for server state changes
        resources, err := adapter.ListResources(ctx, nil)
        if err != nil {
            log.Printf("polling error: %v", err)
            continue
        }

        // Compare with previous state to detect changes
        changes := detectChanges(previousState, resources)

        // Send webhook notifications for matching subscriptions
        for _, change := range changes {
            notifySubscribers(change)
        }

        previousState = resources
    case <-ctx.Done():
        return
    }
}
```

## Error Handling

The adapter provides comprehensive error handling with context:

```go
resource, err := adapter.GetResource(ctx, "server-123")
if err != nil {
    // Check for specific error types
    if apiErr, ok := err.(*dtias.APIError); ok {
        log.Printf("DTIAS API error [%s]: %s", apiErr.Code, apiErr.Message)
    } else {
        log.Printf("request failed: %v", err)
    }
}
```

### Common Error Codes

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `AUTH_FAILED` | Authentication failure | Verify API key is correct |
| `RESOURCE_NOT_FOUND` | Resource not found | Check resource ID |
| `INSUFFICIENT_CAPACITY` | No available servers | Wait for capacity or choose different pool |
| `PROVISIONING_FAILED` | Server provisioning failed | Check server logs and retry |
| `POWER_OPERATION_FAILED` | Power operation failed | Verify server state and management interface |

## Performance Considerations

### API Rate Limiting

DTIAS API has rate limits. The adapter automatically retries requests with exponential backoff:

- Default retry attempts: 3
- Default retry delay: 2s
- HTTP 429 (Too Many Requests) automatically retried

### Caching Recommendations

- **Server Types**: Cache for 1 hour (rarely change)
- **Server Pools**: Cache for 5 minutes
- **Server State**: Cache for 30 seconds
- **Health Metrics**: No caching (real-time data)

### Query Optimization

```go
// Use filters to reduce response size
filter := &adapter.Filter{
    ResourcePoolID: "pool-123",
    Limit:          100,
}
resources, err := adapter.ListResources(ctx, filter)

// Use pagination for large result sets
for offset := 0; ; offset += 100 {
    filter := &adapter.Filter{
        Limit:  100,
        Offset: offset,
    }
    batch, err := adapter.ListResources(ctx, filter)
    if err != nil || len(batch) == 0 {
        break
    }
    // Process batch
}
```

## Testing

### Unit Tests

```bash
# Run unit tests
go test ./internal/adapters/dtias/...

# Run with coverage
go test -cover ./internal/adapters/dtias/...

# Run with verbose output
go test -v ./internal/adapters/dtias/...
```

### Integration Tests

Integration tests require a DTIAS test environment:

```bash
# Set environment variables
export DTIAS_ENDPOINT="https://dtias-test.example.com/api/v1"
export DTIAS_API_KEY="test-api-key"
export DTIAS_DATACENTER="dc-test-1"

# Run integration tests
go test -tags=integration ./internal/adapters/dtias/...
```

## Security Best Practices

1. **Use mTLS**: Always use client certificates in production
2. **Secure API Keys**: Store API keys in secrets management (Vault, Kubernetes Secrets)
3. **TLS 1.3**: Adapter enforces TLS 1.3 minimum version
4. **Certificate Verification**: Never use `insecureSkipVerify: true` in production
5. **Log Sanitization**: The adapter automatically redacts API keys from logs
6. **Network Segmentation**: Deploy gateway in management network with access to DTIAS API

## Troubleshooting

### Connection Errors

```bash
# Test DTIAS API connectivity
curl -k -H "Authorization: Bearer $DTIAS_API_KEY" \
  https://dtias.example.com/api/v1/health

# Verify mTLS certificates
openssl verify -CAfile ca.pem client.pem
```

### Authentication Errors

- Verify API key is correct and not expired
- Check API key has sufficient permissions
- Ensure client certificate (if using mTLS) is valid

### Performance Issues

- Enable debug logging to identify slow operations
- Check DTIAS API response times
- Implement caching for frequently accessed data
- Use filters to reduce response sizes

### Health Check Failures

```go
// Check adapter health
err := adapter.Health(ctx)
if err != nil {
    log.Printf("health check failed: %v", err)
    // Adapter will automatically retry failed requests
}
```

## References

- [O2-IMS Specification](https://specifications.o-ran.org/)
- [Dell DTIAS Documentation](https://www.dell.com/en-us/dt/apex/cloud-platforms/dtias.htm)
- [netweave Architecture](../../docs/architecture.md)
- [Backend Plugins Documentation](../../docs/backend-plugins.md)

## License

Copyright (c) 2026 netweave contributors. All rights reserved.
