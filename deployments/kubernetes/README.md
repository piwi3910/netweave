# Kubernetes Deployment Guide

This directory contains Kubernetes manifests for deploying the O2-IMS Gateway.

## Directory Structure

```
kubernetes/
├── base/                       # Base Kustomize configuration
│   ├── namespace.yaml         # o2ims-system namespace
│   ├── serviceaccount.yaml    # ServiceAccount and RBAC
│   ├── configmap.yaml         # Application configuration
│   ├── deployment.yaml        # Gateway deployment (3 replicas)
│   ├── service.yaml           # ClusterIP service
│   └── kustomization.yaml     # Kustomize base
└── README.md                  # This file
```

## Prerequisites

- Kubernetes cluster 1.30+
- kubectl configured with cluster access
- Redis instance (or deploy Redis Sentinel/Cluster)
- Optional: Kustomize for customization

## Quick Start

### Option 1: Direct Deployment with kubectl

```bash
# Deploy all resources
kubectl apply -f deployments/kubernetes/base/

# Check deployment status
kubectl get all -n o2ims-system

# Check pod logs
kubectl logs -f -l app.kubernetes.io/name=netweave -n o2ims-system

# Check health
kubectl exec -it -n o2ims-system deployment/netweave-gateway -- wget -qO- http://localhost:8080/health
```

### Option 2: Deployment with Kustomize

```bash
# Build and view the manifests
kubectl kustomize deployments/kubernetes/base/

# Apply with kustomize
kubectl apply -k deployments/kubernetes/base/

# Or use the Makefile
make k8s-apply
```

## Configuration

### ConfigMap Customization

Edit `deployments/kubernetes/base/configmap.yaml` to customize:

- Redis connection settings
- Server configuration (ports, timeouts)
- Logging levels and formats
- Metrics and observability
- Security settings (CORS, rate limiting)

Example Redis Sentinel configuration:

```yaml
redis:
  mode: sentinel
  addresses:
    - redis-sentinel-0.o2ims-system.svc.cluster.local:26379
    - redis-sentinel-1.o2ims-system.svc.cluster.local:26379
    - redis-sentinel-2.o2ims-system.svc.cluster.local:26379
  master_name: mymaster
  password: "your-redis-password"
```

### Environment Variable Overrides

You can override configuration using environment variables in the deployment:

```yaml
env:
  - name: NETWEAVE_OBSERVABILITY_LOGGING_LEVEL
    value: "debug"
  - name: NETWEAVE_REDIS_ADDRESSES
    value: "redis-cluster.o2ims-system.svc.cluster.local:6379"
```

## Scaling

### Manual Scaling

```bash
# Scale to 5 replicas
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=5
```

### Horizontal Pod Autoscaler (HPA)

Create an HPA for automatic scaling:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: netweave-gateway-hpa
  namespace: o2ims-system
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: netweave-gateway
  minReplicas: 3
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

## Resource Requirements

Default resource configuration per pod:

- **Requests:** 100m CPU, 128Mi memory
- **Limits:** 500m CPU, 512Mi memory

Adjust in `deployment.yaml` based on your workload:

```yaml
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi
```

## Security

### RBAC Permissions

The ServiceAccount has ClusterRole permissions for:

- **Read:** nodes, namespaces, pods, services, endpoints, PVs, PVCs, storage classes
- **Full Access:** deployments, replicasets, statefulsets, configmaps (O2-IMS managed resources)
- **Limited:** secrets (read + create/update for O2-IMS managed secrets)

Review and adjust `serviceaccount.yaml` based on security requirements.

### Pod Security

The deployment enforces:

- Non-root user (UID 1000)
- Read-only root filesystem
- No privilege escalation
- Dropped capabilities
- Seccomp profile

### TLS/mTLS Configuration

To enable TLS, create a Secret with certificates:

```bash
kubectl create secret tls netweave-tls \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key \
  -n o2ims-system
```

Update the deployment to mount the secret:

```yaml
volumeMounts:
  - name: tls
    mountPath: /etc/netweave/tls
    readOnly: true

volumes:
  - name: tls
    secret:
      secretName: netweave-tls
```

Update ConfigMap to enable TLS:

```yaml
tls:
  enabled: true
  cert_file: /etc/netweave/tls/tls.crt
  key_file: /etc/netweave/tls/tls.key
  client_auth: require-and-verify
  min_version: "1.3"
```

## Monitoring

### Prometheus Metrics

The gateway exposes Prometheus metrics at `/metrics`:

```bash
# Access metrics from within the cluster
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080
curl http://localhost:8080/metrics
```

Add a ServiceMonitor for Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: netweave-gateway
  namespace: o2ims-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: netweave
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
```

### Health Checks

The gateway provides health endpoints:

- **Liveness:** `/health` - Application is alive
- **Readiness:** `/health` - Application is ready for traffic

## Troubleshooting

### Common Issues

**Pods not starting:**

```bash
# Check pod events
kubectl describe pod -l app.kubernetes.io/name=netweave -n o2ims-system

# Check logs
kubectl logs -f -l app.kubernetes.io/name=netweave -n o2ims-system

# Check init container (Redis wait)
kubectl logs -f -l app.kubernetes.io/name=netweave -n o2ims-system -c wait-for-redis
```

**Redis connection issues:**

```bash
# Test Redis connectivity
kubectl run -it --rm debug --image=redis:7.4-alpine --restart=Never -- \
  redis-cli -h redis-master.o2ims-system.svc.cluster.local ping
```

**Kubernetes API access issues:**

```bash
# Check ServiceAccount and RBAC
kubectl auth can-i list nodes --as=system:serviceaccount:o2ims-system:netweave-gateway

# Check ClusterRoleBinding
kubectl get clusterrolebinding netweave-gateway -o yaml
```

### Debug Mode

Enable debug logging:

```bash
kubectl set env deployment/netweave-gateway \
  NETWEAVE_OBSERVABILITY_LOGGING_LEVEL=debug \
  -n o2ims-system
```

### Port Forwarding

Access the gateway locally:

```bash
# Port forward to local machine
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080

# Test API
curl http://localhost:8080/o2ims/v1/api_versions
```

## Updating

### Rolling Update

The deployment uses RollingUpdate strategy with zero downtime:

```bash
# Update image
kubectl set image deployment/netweave-gateway \
  gateway=docker.io/netweave:v0.2.0 \
  -n o2ims-system

# Watch rollout status
kubectl rollout status deployment/netweave-gateway -n o2ims-system
```

### Rollback

```bash
# View rollout history
kubectl rollout history deployment/netweave-gateway -n o2ims-system

# Rollback to previous version
kubectl rollout undo deployment/netweave-gateway -n o2ims-system

# Rollback to specific revision
kubectl rollout undo deployment/netweave-gateway -n o2ims-system --to-revision=2
```

## Cleanup

```bash
# Delete all resources
kubectl delete -k deployments/kubernetes/base/

# Or using Makefile
make k8s-delete

# Verify deletion
kubectl get all -n o2ims-system
```

## Advanced Configuration

### Multi-Cluster Setup

For multi-cluster deployments, deploy Redis with cross-cluster replication and configure each gateway instance to connect to the local Redis cluster.

### Istio Service Mesh

If using Istio, add sidecar injection:

```yaml
metadata:
  labels:
    istio-injection: enabled
```

Configure VirtualService and DestinationRule as needed.

### Network Policies

Restrict network access with NetworkPolicy:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netweave-gateway
  namespace: o2ims-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: netweave
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: istio-system
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - protocol: TCP
          port: 6379
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443  # Kubernetes API
```

## Support

For issues or questions:

- GitHub Issues: <https://github.com/piwi3910/netweave/issues>
- Documentation: <https://github.com/piwi3910/netweave/docs>
