# Deployment Guide

Comprehensive deployment strategies and procedures for the netweave O2-IMS Gateway across development, staging, and production environments.

## Prerequisites

### Infrastructure Requirements

**Kubernetes Cluster:**
- Version: 1.30 or higher
- Minimum nodes: 3 (for HA)
- Node resources: 4 vCPU, 16GB RAM per node
- Storage: 100GB persistent storage for Redis

**Network Requirements:**
- Ingress controller (nginx, Istio, or equivalent)
- Network policies support
- LoadBalancer or NodePort support
- Egress access to backend systems

**Required Components:**
- cert-manager 1.15+ (TLS certificate management)
- Redis 7.4+ (state storage)
- Prometheus 2.x+ (metrics)
- Grafana 9.x+ (dashboards)

### Access Requirements

```bash
# Verify cluster access
kubectl cluster-info
kubectl get nodes

# Verify required namespaces
kubectl create namespace o2ims-system --dry-run=client -o yaml

# Verify RBAC permissions
kubectl auth can-i create deployments --namespace o2ims-system
kubectl auth can-i create services --namespace o2ims-system
kubectl auth can-i create secrets --namespace o2ims-system
```

### Tool Installation

```bash
# Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
helm version

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
kubectl version --client

# cert-manager CLI (optional)
OS=$(go env GOOS); ARCH=$(go env GOARCH); curl -sSL -o cmctl.tar.gz https://github.com/cert-manager/cmctl/releases/latest/download/cmctl-$OS-$ARCH.tar.gz
tar xzf cmctl.tar.gz
sudo mv cmctl /usr/local/bin
```

## Deployment Options

### Option 1: Quick Deploy (Development)

**Purpose**: Local development and testing
**Time**: 5-10 minutes
**Recommended for**: Development, CI/CD testing

```bash
# Clone repository
git clone https://github.com/piwi3910/netweave.git
cd netweave

# Install development tools
make install-tools

# Build and deploy
make deploy-dev

# Verify deployment
kubectl get pods -n o2ims-system
kubectl logs -n o2ims-system -l app=netweave-gateway --tail=20
```

**What this does:**
1. Builds gateway binary
2. Creates Docker image with tag `dev-latest`
3. Deploys to Kubernetes with development configuration
4. Creates self-signed certificates for TLS
5. Deploys single Redis instance (no Sentinel)

**Configuration:**
- 1 gateway replica
- 100m CPU request, 128Mi memory
- Self-signed TLS certificates
- No persistent storage for Redis
- Debug logging enabled

### Option 2: Helm Deployment (Staging/Production)

**Purpose**: Production-ready deployment with full configuration control
**Time**: 15-20 minutes
**Recommended for**: Staging, production, multi-cluster

#### Step 1: Install Prerequisites

**Install cert-manager:**
```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.0/cert-manager.yaml

# Verify installation
kubectl get pods -n cert-manager
cmctl check api
```

**Install Redis Sentinel cluster:**
```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

helm install redis bitnami/redis \
  --namespace o2ims-system \
  --create-namespace \
  --set sentinel.enabled=true \
  --set sentinel.quorum=2 \
  --set replica.replicaCount=2 \
  --set master.persistence.enabled=true \
  --set master.persistence.size=10Gi \
  --set replica.persistence.enabled=true \
  --set replica.persistence.size=10Gi \
  --set auth.password="$(openssl rand -base64 32)"

# Verify Redis deployment
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=redis
kubectl exec -n o2ims-system redis-node-0 -- redis-cli INFO replication
```

**Install Prometheus (if not present):**
```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false

# Verify Prometheus
kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus
```

#### Step 2: Configure TLS Certificates

**Create ClusterIssuer for cert-manager:**
```bash
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ca-issuer
spec:
  ca:
    secretName: ca-key-pair
---
apiVersion: v1
kind: Secret
metadata:
  name: ca-key-pair
  namespace: cert-manager
type: kubernetes.io/tls
data:
  tls.crt: $(cat ca.crt | base64 -w0)
  tls.key: $(cat ca.key | base64 -w0)
EOF

# Verify issuer
kubectl get clusterissuer ca-issuer
cmctl check api
```

#### Step 3: Deploy Gateway with Helm

**Review configuration:**
```bash
# Review default values
helm show values ./helm/netweave > values-default.yaml

# Copy and customize for environment
cp helm/netweave/values-prod.yaml values-custom.yaml
# Edit values-custom.yaml as needed
```

**Key configuration options:**

```yaml
# values-custom.yaml
replicaCount: 3

image:
  repository: ghcr.io/piwi3910/netweave
  tag: "v1.0.0"
  pullPolicy: IfNotPresent

resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 1000m
    memory: 1Gi

tls:
  enabled: true
  issuerRef:
    name: ca-issuer
    kind: ClusterIssuer
  certDuration: 2160h  # 90 days
  certRenewBefore: 720h  # 30 days

redis:
  sentinel: true
  masterSet: "mymaster"
  addresses:
    - "redis-node-0.redis-headless.o2ims-system.svc.cluster.local:26379"
    - "redis-node-1.redis-headless.o2ims-system.svc.cluster.local:26379"
    - "redis-node-2.redis-headless.o2ims-system.svc.cluster.local:26379"
  password:
    secretName: redis
    secretKey: redis-password

monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 30s

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: ca-issuer
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
  hosts:
    - host: o2ims.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: o2ims-tls
      hosts:
        - o2ims.example.com
```

**Deploy gateway:**
```bash
helm install netweave ./helm/netweave \
  --namespace o2ims-system \
  --values values-custom.yaml \
  --timeout 10m \
  --wait

# Verify deployment
helm status netweave -n o2ims-system
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave
```

#### Step 4: Verify Deployment

```bash
# Check pod status
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave

# Check pod logs
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --tail=50

# Check TLS certificates
kubectl get certificate -n o2ims-system
kubectl describe certificate netweave-tls -n o2ims-system

# Test health endpoint
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080 &
curl -k https://localhost:8080/healthz

# Test Redis connectivity
kubectl exec -n o2ims-system -it netweave-gateway-0 -- sh -c 'redis-cli -h redis-node-0 PING'

# Check metrics endpoint
curl http://localhost:8080/metrics | grep o2ims_adapter_operations_total
```

### Option 3: Operator Deployment

**Purpose**: Kubernetes-native lifecycle management
**Time**: 20-25 minutes
**Recommended for**: Production with GitOps, multi-cluster management

#### Step 1: Install Operator

```bash
# Install CRD
kubectl apply -f deployments/operator/crd.yaml

# Verify CRD
kubectl get crd o2imsgateways.o2ims.oran.org

# Install operator
kubectl apply -f deployments/operator/operator.yaml

# Verify operator
kubectl get pods -n o2ims-operator-system
kubectl logs -n o2ims-operator-system -l app=o2ims-operator
```

#### Step 2: Create Gateway Custom Resource

```yaml
# o2imsgateway-production.yaml
apiVersion: o2ims.oran.org/v1alpha1
kind: O2IMSGateway
metadata:
  name: netweave-production
  namespace: o2ims-system
spec:
  # Gateway configuration
  replicas: 3
  version: "v1.0.0"

  # TLS configuration
  tls:
    enabled: true
    issuerRef:
      name: ca-issuer
      kind: ClusterIssuer
    certDuration: 2160h
    certRenewBefore: 720h

  # Redis configuration
  redis:
    sentinel: true
    replicas: 3
    persistence:
      enabled: true
      size: 10Gi
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
      limits:
        cpu: 200m
        memory: 512Mi

  # Gateway resources
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 1Gi

  # Monitoring
  monitoring:
    enabled: true
    prometheus:
      enabled: true
      serviceMonitor: true

  # Autoscaling
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 10
    targetCPUUtilizationPercentage: 80

  # Ingress
  ingress:
    enabled: true
    className: nginx
    host: o2ims.example.com
```

**Apply custom resource:**
```bash
kubectl apply -f o2imsgateway-production.yaml

# Watch operator reconcile
kubectl get o2imsgateways -n o2ims-system -w

# Check operator logs
kubectl logs -n o2ims-operator-system -l app=o2ims-operator -f
```

#### Step 3: Verify Operator Deployment

```bash
# Check custom resource status
kubectl get o2imsgateways netweave-production -n o2ims-system -o yaml

# Check created resources
kubectl get all -n o2ims-system -l app.kubernetes.io/managed-by=o2ims-operator

# Verify health
kubectl port-forward -n o2ims-system svc/netweave-production-gateway 8080:8080 &
curl -k https://localhost:8080/healthz
```

## Deployment Strategies

### Rolling Update (Zero-Downtime)

**Default strategy for production deployments.**

```yaml
# Deployment strategy configuration
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 1  # Keep at least 2 pods running (out of 3)
    maxSurge: 1        # Maximum 4 pods during rollout
```

**Procedure:**
```bash
# Update image version
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set image.tag=v1.1.0 \
  --wait

# Monitor rollout
kubectl rollout status deployment/netweave-gateway -n o2ims-system -w

# Verify new version
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave \
  -o jsonpath='{.items[*].spec.containers[0].image}'
```

**Rollout timeline:**
1. New pod created (0-30s)
2. New pod becomes ready (30s-60s)
3. Old pod terminated (60s-90s)
4. Repeat for remaining pods

### Blue-Green Deployment

**Maximum safety with instant rollback capability.**

```yaml
# blue-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netweave-gateway-blue
  namespace: o2ims-system
  labels:
    app: netweave-gateway
    version: v1.0.0
    color: blue
spec:
  replicas: 3
  selector:
    matchLabels:
      app: netweave-gateway
      color: blue
  template:
    metadata:
      labels:
        app: netweave-gateway
        color: blue
    spec:
      containers:
      - name: gateway
        image: ghcr.io/piwi3910/netweave:v1.0.0
```

```yaml
# green-deployment.yaml (new version)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netweave-gateway-green
  namespace: o2ims-system
  labels:
    app: netweave-gateway
    version: v1.1.0
    color: green
spec:
  replicas: 3
  selector:
    matchLabels:
      app: netweave-gateway
      color: green
  template:
    metadata:
      labels:
        app: netweave-gateway
        color: green
    spec:
      containers:
      - name: gateway
        image: ghcr.io/piwi3910/netweave:v1.1.0
```

**Procedure:**
```bash
# 1. Deploy green version
kubectl apply -f green-deployment.yaml

# 2. Wait for green pods to be ready
kubectl wait --for=condition=ready pod \
  -l color=green -n o2ims-system --timeout=300s

# 3. Test green deployment internally
kubectl port-forward -n o2ims-system deploy/netweave-gateway-green 8080:8080 &
curl -k https://localhost:8080/healthz
curl -k https://localhost:8080/o2ims-infrastructureInventory/v1/api_versions

# 4. Switch Service selector to green
kubectl patch svc netweave-gateway -n o2ims-system \
  -p '{"spec":{"selector":{"color":"green"}}}'

# 5. Monitor for 10 minutes
watch kubectl get pods -n o2ims-system

# 6. If successful, delete blue deployment
kubectl delete deployment netweave-gateway-blue -n o2ims-system

# ROLLBACK if issues:
kubectl patch svc netweave-gateway -n o2ims-system \
  -p '{"spec":{"selector":{"color":"blue"}}}'
```

### Canary Deployment

**Gradual rollout with traffic splitting (requires service mesh).**

**Prerequisites:**
- Istio or similar service mesh installed
- Virtual Service support

```yaml
# canary-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netweave-gateway-canary
  namespace: o2ims-system
spec:
  replicas: 1  # Start with 1 canary pod
  selector:
    matchLabels:
      app: netweave-gateway
      version: v1.1.0
  template:
    metadata:
      labels:
        app: netweave-gateway
        version: v1.1.0
    spec:
      containers:
      - name: gateway
        image: ghcr.io/piwi3910/netweave:v1.1.0
```

```yaml
# virtual-service.yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: netweave-gateway
  namespace: o2ims-system
spec:
  hosts:
    - netweave-gateway.o2ims-system.svc.cluster.local
  http:
    - match:
        - headers:
            x-canary:
              exact: "true"
      route:
        - destination:
            host: netweave-gateway
            subset: v1.1.0
          weight: 100
    - route:
        - destination:
            host: netweave-gateway
            subset: v1.0.0
          weight: 90
        - destination:
            host: netweave-gateway
            subset: v1.1.0
          weight: 10  # 10% canary traffic
```

**Procedure:**
```bash
# 1. Deploy canary
kubectl apply -f canary-deployment.yaml
kubectl apply -f virtual-service.yaml

# 2. Monitor canary metrics (10% traffic)
watch 'kubectl top pods -n o2ims-system | grep canary'

# 3. Gradually increase canary traffic
# 10% -> 25% -> 50% -> 75% -> 100%
kubectl patch virtualservice netweave-gateway -n o2ims-system \
  --type merge -p '
  {
    "spec": {
      "http": [{
        "route": [
          {"destination": {"host": "netweave-gateway", "subset": "v1.0.0"}, "weight": 75},
          {"destination": {"host": "netweave-gateway", "subset": "v1.1.0"}, "weight": 25}
        ]
      }]
    }
  }'

# 4. Monitor error rates, latency
kubectl exec -n o2ims-system netweave-gateway-canary-0 -- \
  wget -qO- localhost:8080/metrics | grep o2ims_http_request_duration_seconds

# 5. If successful, scale up canary to replace stable
kubectl scale deployment netweave-gateway-canary -n o2ims-system --replicas=3
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=0

# 6. Update VirtualService to 100% canary
kubectl patch virtualservice netweave-gateway -n o2ims-system \
  --type merge -p '
  {
    "spec": {
      "http": [{
        "route": [
          {"destination": {"host": "netweave-gateway", "subset": "v1.1.0"}, "weight": 100}
        ]
      }]
    }
  }'
```

## Multi-Cluster Deployment

**High availability across multiple Kubernetes clusters.**

```mermaid
graph TB
    subgraph "Cluster 1 (US-East)"
        GW1A[Gateway Pod 1A]
        GW1B[Gateway Pod 1B]
        GW1C[Gateway Pod 1C]
        Redis1[Redis Primary]
        Redis1R[Redis Replica]
    end

    subgraph "Cluster 2 (US-West)"
        GW2A[Gateway Pod 2A]
        GW2B[Gateway Pod 2B]
        GW2C[Gateway Pod 2C]
        Redis2[Redis Replica]
    end

    subgraph "External"
        LB[Global Load Balancer]
        SMO[O2 SMO]
    end

    SMO --> LB
    LB --> GW1A
    LB --> GW2A

    GW1A --> Redis1
    GW1B --> Redis1
    GW1C --> Redis1
    Redis1 --> Redis1R

    Redis1 -.replicate.-> Redis2

    GW2A --> Redis2
    GW2B --> Redis2
    GW2C --> Redis2

    style "Cluster 1 (US-East)" fill:#e1f5ff
    style "Cluster 2 (US-West)" fill:#fff4e6
    style External fill:#e8f5e9
```

### Redis Cross-Cluster Replication

**Configure Redis replication between clusters:**

```yaml
# Cluster 1 (Primary)
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-config
  namespace: o2ims-system
data:
  redis.conf: |
    bind 0.0.0.0
    protected-mode no
    port 6379
    tcp-backlog 511
    timeout 0
    tcp-keepalive 300

    # Replication settings
    repl-diskless-sync yes
    repl-diskless-sync-delay 5
    repl-diskless-load on-empty-db
    repl-ping-replica-period 10
    repl-timeout 60

    # Persistence
    save 900 1
    save 300 10
    save 60 10000
    appendonly yes
    appendfsync everysec
```

```bash
# Configure replication from Cluster 2 Redis to Cluster 1
kubectl exec -n o2ims-system redis-node-0 -c redis -- redis-cli \
  REPLICAOF redis-cluster1.example.com 6379

# Verify replication
kubectl exec -n o2ims-system redis-node-0 -c redis -- redis-cli INFO replication
```

### Gateway Configuration for Multi-Cluster

```yaml
# Gateway ConfigMap (both clusters)
apiVersion: v1
kind: ConfigMap
metadata:
  name: netweave-config
  namespace: o2ims-system
data:
  config.yaml: |
    redis:
      sentinel: true
      master_set: "mymaster"
      sentinel_addresses:
        - "redis-sentinel-0.redis-headless.o2ims-system.svc.cluster.local:26379"
        - "redis-sentinel-1.redis-headless.o2ims-system.svc.cluster.local:26379"
        - "redis-sentinel-2.redis-headless.o2ims-system.svc.cluster.local:26379"
      # Fallback to remote cluster if local unavailable
      fallback_addresses:
        - "redis-sentinel.cluster1.example.com:26379"
      pool_size: 10
      read_timeout: 3s
      write_timeout: 3s
      dial_timeout: 5s
      max_retries: 3

    cache:
      ttl:
        resources: 30s
        resource_pools: 300s
        capabilities: 3600s
```

## Configuration Management

### Environment-Specific Values

**Development (`values-dev.yaml`):**
```yaml
replicaCount: 1
resources:
  requests:
    cpu: 100m
    memory: 128Mi
tls:
  enabled: false
redis:
  sentinel: false
monitoring:
  enabled: false
autoscaling:
  enabled: false
```

**Staging (`values-staging.yaml`):**
```yaml
replicaCount: 2
resources:
  requests:
    cpu: 250m
    memory: 256Mi
tls:
  enabled: true
redis:
  sentinel: true
monitoring:
  enabled: true
autoscaling:
  enabled: false
```

**Production (`values-prod.yaml`):**
```yaml
replicaCount: 3
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 1000m
    memory: 1Gi
tls:
  enabled: true
redis:
  sentinel: true
monitoring:
  enabled: true
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
```

### Secrets Management

```bash
# Create Redis password secret
kubectl create secret generic redis-password \
  --namespace o2ims-system \
  --from-literal=password="$(openssl rand -base64 32)"

# Create TLS secrets
kubectl create secret tls gateway-tls \
  --namespace o2ims-system \
  --cert=server.crt \
  --key=server.key

# Create CA bundle secret
kubectl create secret generic ca-bundle \
  --namespace o2ims-system \
  --from-file=ca.crt=ca-bundle.crt
```

## Post-Deployment Verification

### Health Checks

```bash
# Gateway health
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/healthz

# Redis health
kubectl exec -n o2ims-system redis-node-0 -- redis-cli PING

# Sentinel health
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL master mymaster
```

### Functional Tests

```bash
# Test API versions endpoint
curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions

# Test resource pool listing
curl -k -H "Accept: application/json" \
  https://o2ims.example.com/o2ims-infrastructureInventory/v1/resourcePools

# Create test subscription
curl -k -X POST https://o2ims.example.com/o2ims-infrastructureInventory/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "callback": "https://smo.example.com/notify",
    "consumerSubscriptionId": "test-sub-001"
  }'
```

### Performance Validation

```bash
# Load test with hey
hey -n 1000 -c 10 -m GET \
  https://o2ims.example.com/o2ims-infrastructureInventory/v1/resourcePools

# Check p95 latency (should be < 100ms)
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_request_duration_seconds | grep quantile
```

## Troubleshooting Deployment Issues

### Pods Not Starting

```bash
# Check pod status
kubectl describe pod -n o2ims-system netweave-gateway-0

# Common issues:
# - ImagePullBackOff: Check image repository and credentials
# - CrashLoopBackOff: Check logs for errors
# - Pending: Check resource constraints
```

### Redis Connection Failures

```bash
# Test Redis connectivity
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  redis-cli -h redis-node-0 PING

# Check Redis logs
kubectl logs -n o2ims-system redis-node-0

# Verify Sentinel
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster
```

### Certificate Issues

```bash
# Check certificate status
kubectl get certificate -n o2ims-system
kubectl describe certificate netweave-tls -n o2ims-system

# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager

# Manually trigger certificate renewal
cmctl renew netweave-tls -n o2ims-system
```

## Rollback Procedures

### Helm Rollback

```bash
# List release history
helm history netweave -n o2ims-system

# Rollback to previous version
helm rollback netweave -n o2ims-system

# Rollback to specific revision
helm rollback netweave 3 -n o2ims-system
```

### Kubectl Rollback

```bash
# Rollback deployment
kubectl rollout undo deployment/netweave-gateway -n o2ims-system

# Rollback to specific revision
kubectl rollout undo deployment/netweave-gateway -n o2ims-system --to-revision=2

# Check rollout history
kubectl rollout history deployment/netweave-gateway -n o2ims-system
```

## Related Documentation

- [Operations Overview](README.md)
- [Monitoring Guide](monitoring.md)
- [Troubleshooting Guide](troubleshooting.md)
- [Upgrade Procedures](upgrades.md)
