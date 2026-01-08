# netweave Helm Chart

Production-grade Helm chart for deploying the netweave O-RAN O2-IMS Gateway to Kubernetes.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.8+
- cert-manager 1.11+ (for TLS certificates)
- Prometheus Operator (optional, for ServiceMonitor)

## Installing the Chart

### Quick Start (Development)

```bash
helm install netweave ./helm/netweave \
  --namespace o2ims-system \
  --create-namespace \
  --values helm/netweave/values-dev.yaml
```

### Production Installation

```bash
# Create namespace
kubectl create namespace o2ims-system

# Create Redis password secret
kubectl create secret generic redis-password \
  --from-literal=password="${REDIS_PASSWORD}" \
  --namespace o2ims-system

# Create TLS certificates secret
kubectl create secret tls netweave-certs \
  --cert=tls.crt \
  --key=tls.key \
  --namespace=o2ims-system

# Add CA certificate to the secret
kubectl create secret generic netweave-certs \
  --from-file=tls.crt=tls.crt \
  --from-file=tls.key=tls.key \
  --from-file=ca.crt=ca.crt \
  --namespace=o2ims-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Install chart with production values
helm install netweave ./helm/netweave \
  --namespace o2ims-system \
  --values helm/netweave/values-prod.yaml \
  --set image.tag=v1.0.0 \
  --wait \
  --timeout 10m
```

## Upgrading the Chart

```bash
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --values helm/netweave/values-prod.yaml \
  --set image.tag=v1.0.1 \
  --wait
```

## Uninstalling the Chart

```bash
helm uninstall netweave --namespace o2ims-system
```

## Configuration

The following table lists the configurable parameters of the netweave chart and their default values.

### Global Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `docker.io/netweave/gateway` |
| `image.tag` | Image tag | `""` (uses appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full name | `""` |

### Service Account

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |

### Service

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | HTTPS port | `8443` |
| `service.metricsPort` | Metrics port | `9090` |
| `service.healthPort` | Health check port | `8080` |

### Ingress

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.className` | Ingress class name | `nginx` |
| `ingress.hosts` | Ingress hosts | `[{"host":"gateway.example.com"}]` |
| `ingress.tls` | TLS configuration | `[]` |

### Resources

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `1000m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### Autoscaling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable HPA | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `3` |
| `autoscaling.maxReplicas` | Maximum replicas | `10` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU | `70` |
| `autoscaling.targetMemoryUtilizationPercentage` | Target memory | `80` |

### Application Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.server.port` | Server port | `8443` |
| `config.server.tls.enabled` | Enable TLS | `true` |
| `config.server.tls.mtls.enabled` | Enable mTLS | `true` |
| `config.observability.logging.level` | Log level | `info` |
| `config.observability.metrics.enabled` | Enable metrics | `true` |
| `config.rateLimit.enabled` | Enable rate limiting | `false` |

### Redis

| Parameter | Description | Default |
|-----------|-------------|---------|
| `redis.enabled` | Enable Redis dependency | `true` |
| `redis.architecture` | Redis architecture (standalone/replication) | `standalone` |
| `redis.auth.enabled` | Enable Redis authentication | `true` |
| `redis.auth.password` | Redis password | `changeme` |

For a complete list of parameters, see `values.yaml`.

## Examples

### Development Deployment

```bash
helm install netweave ./helm/netweave \
  -f helm/netweave/values-dev.yaml \
  --namespace o2ims-dev \
  --create-namespace
```

### Production with Custom Domain

```bash
helm install netweave ./helm/netweave \
  -f helm/netweave/values-prod.yaml \
  --set ingress.hosts[0].host=o2ims.mycompany.com \
  --set ingress.tls[0].hosts[0]=o2ims.mycompany.com \
  --namespace o2ims-prod
```

### Enable Monitoring

```bash
helm install netweave ./helm/netweave \
  --set monitoring.enabled=true \
  --set monitoring.serviceMonitor.enabled=true \
  --namespace o2ims-system
```

## Testing

```bash
# Lint the chart
helm lint helm/netweave

# Template and verify
helm template netweave helm/netweave --debug

# Run Helm tests
helm test netweave --namespace o2ims-system
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave
```

### View Logs

```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --tail=100 -f
```

### Check Configuration

```bash
kubectl get configmap -n o2ims-system
kubectl describe configmap -n o2ims-system netweave-config
```

### Verify TLS Certificates

```bash
kubectl get secret -n o2ims-system netweave-certs
kubectl describe secret -n o2ims-system netweave-certs
```

## Support

For issues and questions:
- GitHub Issues: https://github.com/piwi3910/netweave/issues
- Documentation: https://github.com/piwi3910/netweave

## License

See the main repository for license information.
