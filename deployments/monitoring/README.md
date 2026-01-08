# O2-IMS Adapter Monitoring

This directory contains observability configurations for O2-IMS adapter operations.

## Files

- **grafana-dashboard-adapters.json**: Pre-built Grafana dashboard for adapter metrics
- **prometheus-alerts-adapters.yaml**: Prometheus alert rules for SLO violations

## Quick Start

### Import Grafana Dashboard
```bash
# Import via UI
1. Open Grafana → Dashboards → Import
2. Upload grafana-dashboard-adapters.json
3. Select Prometheus data source

# Or via API
curl -X POST http://grafana:3000/api/dashboards/db \
  -H "Content-Type: application/json" \
  -d @grafana-dashboard-adapters.json
```

### Configure Prometheus Alerts
```bash
# Add to prometheus.yml
rule_files:
  - /etc/prometheus/prometheus-alerts-adapters.yaml

# Reload configuration
curl -X POST http://prometheus:9090/-/reload
```

## Metrics Available

### Adapter Operations
- `o2ims_adapter_operations_total` - Total operations by adapter/operation/status
- `o2ims_adapter_operation_duration_seconds` - Operation latency distribution
- `o2ims_adapter_operation_errors_total` - Error counts by type

### Cache Metrics
- `o2ims_adapter_cache_hits_total` - Cache hits by adapter/operation
- `o2ims_adapter_cache_misses_total` - Cache misses by adapter/operation

### Resource Metrics
- `o2ims_adapter_resources_total` - Resource counts by adapter/type
- `o2ims_adapter_resource_pools_total` - Resource pool counts

### Backend API Metrics
- `o2ims_adapter_backend_requests_total` - Backend API requests
- `o2ims_adapter_backend_latency_seconds` - Backend API latency
- `o2ims_adapter_backend_errors_total` - Backend API errors

### Health Metrics
- `o2ims_adapter_health_check_status` - Health status (1=healthy, 0=unhealthy)
- `o2ims_adapter_health_check_duration_seconds` - Health check duration

## SLO Targets

- API response p95 < 100ms, p99 < 500ms
- Cache hit ratio > 90%
- Error rate < 5%

## Documentation

See `docs/operations/monitoring.md` for comprehensive documentation including:
- Detailed metrics reference
- Example Prometheus queries
- Troubleshooting guide
- Best practices
