# O2-IMS Gateway Adapter Monitoring Guide

This guide provides comprehensive documentation for monitoring O2-IMS adapter operations, performance metrics, and health indicators.

## Overview

The O2-IMS Gateway provides extensive observability for all adapter operations through:
- **Prometheus Metrics**: Comprehensive metrics for operations, latency, errors, caching, and backend API calls
- **OpenTelemetry Tracing**: Distributed tracing for request flows through adapters
- **Grafana Dashboards**: Pre-built visualizations for adapter performance
- **Prometheus Alerts**: SLO-based alerting for proactive issue detection

## Quick Start

### Prerequisites
- Prometheus 2.x+ deployed and scraping gateway pods
- Grafana 9.x+ for dashboard visualization
- OpenTelemetry Collector (optional, for distributed tracing)

### Metrics Endpoint
Gateway pods expose Prometheus metrics at:
```
http://<gateway-pod>:8080/metrics
```

### Import Dashboard
1. Open Grafana UI
2. Navigate to Dashboards → Import
3. Upload `deployments/monitoring/grafana-dashboard-adapters.json`
4. Select Prometheus data source
5. Click Import

### Configure Alerts
1. Copy `deployments/monitoring/prometheus-alerts-adapters.yaml` to Prometheus
2. Update `prometheus.yml`:
   ```yaml
   rule_files:
     - /etc/prometheus/prometheus-alerts-adapters.yaml
   ```
3. Reload Prometheus configuration

## Metrics Reference

### Adapter Operation Metrics

#### `o2ims_adapter_operations_total`
**Type**: Counter
**Labels**: `adapter`, `operation`, `status`
**Description**: Total number of adapter operations performed.

**Example Queries**:
```promql
# Operations per second by adapter
sum(rate(o2ims_adapter_operations_total[5m])) by (adapter)

# Success rate percentage
sum(rate(o2ims_adapter_operations_total{status="success"}[5m])) by (adapter)
/ sum(rate(o2ims_adapter_operations_total[5m])) by (adapter) * 100

# Operations breakdown by type
sum(rate(o2ims_adapter_operations_total[5m])) by (adapter, operation)
```

#### `o2ims_adapter_operation_duration_seconds`
**Type**: Histogram
**Labels**: `adapter`, `operation`, `status`
**Buckets**: 1ms to ~16s (exponential)
**Description**: Duration of adapter operations in seconds.

**Example Queries**:
```promql
# p95 latency by adapter and operation
histogram_quantile(0.95,
  sum(rate(o2ims_adapter_operation_duration_seconds_bucket[5m]))
  by (adapter, operation, le)
)

# p99 latency (SLO: < 500ms)
histogram_quantile(0.99,
  sum(rate(o2ims_adapter_operation_duration_seconds_bucket[5m]))
  by (adapter, operation, le)
)

# Average latency
rate(o2ims_adapter_operation_duration_seconds_sum[5m])
/ rate(o2ims_adapter_operation_duration_seconds_count[5m])
```

**SLO Thresholds**:
- p95 < 100ms (warning)
- p99 < 500ms (critical)

#### `o2ims_adapter_operation_errors_total`
**Type**: Counter
**Labels**: `adapter`, `operation`, `error_type`
**Description**: Total number of adapter operation errors.

**Example Queries**:
```promql
# Error rate per second
sum(rate(o2ims_adapter_operation_errors_total[5m])) by (adapter, operation)

# Error types breakdown
sum(rate(o2ims_adapter_operation_errors_total[5m])) by (error_type)

# Error percentage
sum(rate(o2ims_adapter_operation_errors_total[5m])) by (adapter)
/ sum(rate(o2ims_adapter_operations_total[5m])) by (adapter) * 100
```

### Cache Metrics

#### `o2ims_adapter_cache_hits_total`
**Type**: Counter
**Labels**: `adapter`, `operation`
**Description**: Total number of cache hits.

#### `o2ims_adapter_cache_misses_total`
**Type**: Counter
**Labels**: `adapter`, `operation`
**Description**: Total number of cache misses.

**Example Queries**:
```promql
# Cache hit ratio percentage (SLO: > 90%)
sum(rate(o2ims_adapter_cache_hits_total[5m])) by (adapter, operation)
/ (
  sum(rate(o2ims_adapter_cache_hits_total[5m])) by (adapter, operation)
  + sum(rate(o2ims_adapter_cache_misses_total[5m])) by (adapter, operation)
) * 100

# Cache operations per second
sum(rate(o2ims_adapter_cache_hits_total[5m]) + rate(o2ims_adapter_cache_misses_total[5m])) by (adapter)
```

**SLO Threshold**:
- Cache hit ratio > 90%

### Resource Metrics

#### `o2ims_adapter_resources_total`
**Type**: Gauge
**Labels**: `adapter`, `resource_type`
**Description**: Total number of resources managed by adapter.

#### `o2ims_adapter_resource_pools_total`
**Type**: Gauge
**Labels**: `adapter`
**Description**: Total number of resource pools per adapter.

**Example Queries**:
```promql
# Total resources by type
sum(o2ims_adapter_resources_total) by (resource_type)

# Resources per adapter
sum(o2ims_adapter_resources_total) by (adapter)

# Resource pool count
o2ims_adapter_resource_pools_total
```

### Backend API Metrics

#### `o2ims_adapter_backend_requests_total`
**Type**: Counter
**Labels**: `adapter`, `endpoint`, `method`, `status`
**Description**: Total number of backend API requests.

#### `o2ims_adapter_backend_latency_seconds`
**Type**: Histogram
**Labels**: `adapter`, `endpoint`, `method`
**Buckets**: 1ms to ~16s (exponential)
**Description**: Backend API latency in seconds.

#### `o2ims_adapter_backend_errors_total`
**Type**: Counter
**Labels**: `adapter`, `endpoint`, `method`, `error_type`
**Description**: Total number of backend API errors.

**Example Queries**:
```promql
# Backend API request rate
sum(rate(o2ims_adapter_backend_requests_total[5m])) by (adapter, endpoint)

# Backend API p95 latency
histogram_quantile(0.95,
  sum(rate(o2ims_adapter_backend_latency_seconds_bucket[5m]))
  by (adapter, endpoint, le)
)

# Backend error rate
sum(rate(o2ims_adapter_backend_errors_total[5m])) by (adapter, endpoint, error_type)
```

### Health Check Metrics

#### `o2ims_adapter_health_check_status`
**Type**: Gauge
**Labels**: `adapter`
**Values**: 1 = healthy, 0 = unhealthy
**Description**: Health status of adapter.

#### `o2ims_adapter_health_check_duration_seconds`
**Type**: Histogram
**Labels**: `adapter`
**Buckets**: 10ms to ~5s
**Description**: Duration of health checks in seconds.

**Example Queries**:
```promql
# Unhealthy adapters
o2ims_adapter_health_check_status == 0

# Health check latency
o2ims_adapter_health_check_duration_seconds
```

### Subscription Metrics

#### `o2ims_adapter_subscriptions_active`
**Type**: Gauge
**Labels**: `adapter`
**Description**: Number of active subscriptions per adapter.

**Example Queries**:
```promql
# Total active subscriptions
sum(o2ims_adapter_subscriptions_active)

# Subscriptions per adapter
o2ims_adapter_subscriptions_active
```

## OpenTelemetry Tracing

### Trace Attributes

All adapter operations emit distributed traces with the following standard attributes:

- `adapter.name`: Name of the adapter (e.g., "kubernetes", "aws")
- `adapter.operation`: Operation type (e.g., "ListResources", "GetResource")
- `resource.type`: Type of resource being operated on
- `resource.id`: Resource identifier
- `resource.count`: Number of resources in result
- `backend.endpoint`: Backend API endpoint called
- `backend.method`: HTTP method used
- `backend.status_code`: Backend response status code
- `cache.hit`: Whether cache was hit (true/false)
- `cache.miss`: Whether cache was missed (true/false)

### Trace Structure

Typical trace hierarchy:
```
HTTP Request (API Gateway)
└── Adapter Operation (e.g., ListResources)
    ├── Backend API Call (e.g., K8s API)
    ├── Cache Lookup
    ├── Transform Data
    └── Apply Filter/Pagination
```

### Example Trace Queries

**Jaeger UI**:
```
service=netweave-gateway AND operation=ListResources AND adapter.name=kubernetes
```

**Find slow operations**:
```
service=netweave-gateway AND duration > 500ms
```

## Grafana Dashboard

The pre-built Grafana dashboard (`grafana-dashboard-adapters.json`) includes:

### Panels
1. **Adapter Operation Rate**: Operations per second by adapter and operation
2. **Adapter Operation Latency**: p95 and p99 latency with SLO thresholds
3. **Adapter Error Rate**: Errors per second by adapter and operation
4. **Adapter Success Rate**: Percentage success rate per adapter
5. **Cache Hit Ratio**: Cache effectiveness by adapter and operation
6. **Active Resources by Adapter**: Resource counts with color-coded thresholds
7. **Backend API Latency**: p95 latency for backend calls
8. **Backend API Error Rate**: Backend errors by endpoint and type
9. **Health Check Status**: Green/red health indicators per adapter
10. **Active Subscriptions**: Subscription counts per adapter
11. **Operation Statistics Table**: Top 20 operations with rates

### Alerts in Dashboard
- Adapter Operation Latency p99 > 500ms (5m window)
- Cache Hit Ratio < 90% (10m window)

## Prometheus Alerts

### Alert Configuration

Alerts are defined in `prometheus-alerts-adapters.yaml` with the following categories:

#### SLO Violation Alerts

| Alert | Severity | Threshold | Duration |
|-------|----------|-----------|----------|
| `AdapterOperationP95LatencyHigh` | Warning | > 100ms | 5m |
| `AdapterOperationP99LatencyCritical` | Critical | > 500ms | 5m |
| `AdapterErrorRateHigh` | Warning | > 5% | 5m |
| `AdapterErrorRateCritical` | Critical | > 10% | 5m |
| `AdapterCacheHitRatioLow` | Warning | < 90% | 10m |
| `AdapterBackendLatencyHigh` | Warning | p95 > 500ms | 5m |

#### Availability Alerts

| Alert | Severity | Condition | Duration |
|-------|----------|-----------|----------|
| `AdapterUnhealthy` | Critical | Health check == 0 | 2m |
| `AdapterNoOperations` | Warning | No ops in 5m | 5m |
| `AdapterBackendErrorsHigh` | Warning | > 0.1 errors/s | 5m |

#### Resource Alerts

| Alert | Severity | Condition | Duration |
|-------|----------|-----------|----------|
| `AdapterResourcePoolCountDrop` | Warning | -20% drop in 10m | 5m |
| `AdapterResourceCountDrop` | Warning | -20% drop in 10m | 5m |
| `AdapterNoActiveSubscriptions` | Info | 0 subscriptions | 10m |

### Alert Routing

Configure AlertManager to route alerts appropriately:

```yaml
route:
  group_by: ['alertname', 'adapter']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h

  routes:
    - match:
        severity: critical
      receiver: pagerduty

    - match:
        severity: warning
      receiver: slack

    - match:
        severity: info
      receiver: email
```

## Troubleshooting Guide

### High Latency (p99 > 500ms)

**Diagnosis**:
1. Check backend API latency:
   ```promql
   histogram_quantile(0.95, sum(rate(o2ims_adapter_backend_latency_seconds_bucket[5m])) by (adapter, endpoint, le))
   ```
2. Check for backend errors:
   ```promql
   sum(rate(o2ims_adapter_backend_errors_total[5m])) by (adapter, endpoint)
   ```
3. Review traces in Jaeger for slow operations

**Common Causes**:
- Backend API slowness (Kubernetes API, cloud provider API)
- Network latency to backend
- Large result sets without pagination
- Missing cache hits

**Resolution**:
- Optimize backend queries
- Implement pagination
- Tune cache TTLs
- Scale backend infrastructure

### High Error Rate (> 5%)

**Diagnosis**:
1. Check error types:
   ```promql
   sum(rate(o2ims_adapter_operation_errors_total[5m])) by (error_type)
   ```
2. Check backend errors:
   ```promql
   sum(rate(o2ims_adapter_backend_errors_total[5m])) by (error_type)
   ```
3. Review logs for error details

**Common Causes**:
- Backend API authentication failures
- Backend API rate limiting
- Network connectivity issues
- Invalid resource configurations

**Resolution**:
- Verify credentials/authentication
- Implement retry with exponential backoff
- Check backend API quotas
- Validate resource configurations

### Low Cache Hit Ratio (< 90%)

**Diagnosis**:
1. Check cache hit ratio by operation:
   ```promql
   sum(rate(o2ims_adapter_cache_hits_total[5m])) by (operation) / (sum(rate(o2ims_adapter_cache_hits_total[5m])) + sum(rate(o2ims_adapter_cache_misses_total[5m]))) by (operation) * 100
   ```
2. Check for cache invalidations
3. Review cache configuration

**Common Causes**:
- Cache TTL too short
- High resource churn rate
- Cache not enabled for operation
- Insufficient cache size

**Resolution**:
- Increase cache TTL (if data allows)
- Increase cache size
- Enable caching for frequently-accessed operations
- Implement cache warming

### Adapter Unhealthy

**Diagnosis**:
1. Check health check status:
   ```promql
   o2ims_adapter_health_check_status
   ```
2. Review health check logs
3. Test backend connectivity manually

**Common Causes**:
- Backend API unreachable
- Authentication expired/invalid
- Network connectivity issues
- Backend service degraded

**Resolution**:
- Verify backend endpoint reachability
- Refresh credentials
- Check network policies/firewalls
- Contact backend provider

## Best Practices

### Monitoring Setup
1. **Set up alerts** for all SLO violations
2. **Create on-call runbooks** for each alert type
3. **Configure alert routing** to appropriate teams
4. **Test alerts** regularly with synthetic failures
5. **Review dashboards** weekly for trends

### Performance Optimization
1. **Monitor cache hit ratios** and optimize cache configuration
2. **Track p95/p99 latencies** and investigate outliers
3. **Profile slow operations** using distributed tracing
4. **Optimize backend queries** to reduce latency
5. **Implement pagination** for large result sets

### Capacity Planning
1. **Monitor resource counts** for growth trends
2. **Track operation rates** to predict scaling needs
3. **Review backend API usage** against quotas
4. **Plan for peak loads** based on historical data

## Metric Labels Best Practices

### Required Labels
- `adapter`: Always identify which adapter (kubernetes, aws, etc.)
- `operation`: Specify the operation type
- `status`: Use "success" or "error" consistently

### Optional Labels
- `resource_type`: For resource-specific metrics
- `endpoint`: For backend API metrics
- `error_type`: Categorize errors for better diagnosis

### Label Cardinality
⚠️ **Warning**: Avoid high-cardinality labels (e.g., resource IDs, timestamps) in metrics as they can cause Prometheus performance issues.

## References

- [Prometheus Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/)
- [OpenTelemetry Tracing Spec](https://opentelemetry.io/docs/specs/otel/trace/api/)
- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)
