# Operations Guide

This guide provides information for operators on how to monitor, troubleshoot, and maintain the netweave O2-IMS Gateway.

## Monitoring

The gateway exposes a variety of metrics in the Prometheus format. These metrics can be scraped by a Prometheus server and visualized in Grafana.

### Key Metrics

*   `o2ims_http_requests_total`: Total number of HTTP requests.
*   `o2ims_http_request_duration_seconds`: Latency of HTTP requests.
*   `o2ims_auth_total`: Total number of authentication attempts.
*   `o2ims_authz_total`: Total number of authorization attempts.
*   `o2ims_redis_commands_total`: Total number of Redis commands.
*   `o2ims_webhook_delivery_total`: Total number of webhook deliveries.

### Dashboards

A pre-built Grafana dashboard is available in `deployments/monitoring/grafana-dashboard-adapters.json`. This dashboard provides an overview of the gateway's health and performance.

### Alerts

Prometheus alerts are defined in `deployments/monitoring/prometheus-alerts-adapters.yaml`. These alerts will fire on conditions such as:

*   High request latency
*   High rate of HTTP errors
*   High rate of authentication or authorization failures
*   Redis is down
*   Certificate is about to expire

## Troubleshooting

### Common Issues

*   **502 Bad Gateway**: This usually indicates that the gateway is unable to connect to a backend service, such as the Kubernetes API server or Redis. Check the logs of the gateway pods for more information.
*   **401 Unauthorized**: This indicates an authentication failure. Check that the client is presenting a valid client certificate.
*   **403 Forbidden**: This indicates an authorization failure. Check that the client has the necessary permissions to perform the requested operation.
*   **429 Too Many Requests**: This indicates that the client has exceeded the rate limit. Check the rate limit configuration and the `X-RateLimit-*` headers in the response.

### Logs

The gateway produces structured logs in JSON format. These logs can be collected and analyzed using a centralized logging solution like the ELK stack or Loki.

To view the logs of the gateway pods, you can use `kubectl logs`:

```bash
kubectl logs -l app=netweave-gateway -n o2ims-system -f
```

## Maintenance

### Upgrades

The gateway can be upgraded with zero downtime using a rolling update strategy. See the [Deployment Guide](deployment.md) for more information.

### Backups and Restore

The state of the gateway is stored in Redis. Regular backups of the Redis database should be taken to prevent data loss. See the [Architecture Part 2](architecture-part2.md) document for details on the backup and restore process.
