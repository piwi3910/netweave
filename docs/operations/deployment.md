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
- NGINX Ingress controller (with ssl-passthrough enabled for mTLS)
- Network policies support
- LoadBalancer or NodePort support
- Egress access to backend systems

**Network Ports:**

| Port | Protocol | Direction | Purpose |
|------|----------|-----------|---------|
| 443 | TCP | Inbound | NGINX Ingress HTTPS (all hostnames) |
| 8080 | TCP | Internal | Admin port (OAuth2, health, docs, metrics) |
| 8443 | TCP | Internal | O2 port (mTLS, O2-IMS/DMS/SMO) |
| 8444 | TCP | Internal | TMF port (OAuth2, TMForum APIs) |
| 8445 | TCP | Internal | GraphQL port (OAuth2, GraphQL API) |
| 6379 | TCP | Internal | Redis communication |
| 9090 | TCP | Internal | Metrics (Prometheus) |

**Required Components:**
- HashiCorp Vault 1.15+ (PKI certificate management)
- Redis 7.4+ (state storage)
- Prometheus 2.x+ (metrics)
- Grafana 9.x+ (dashboards)

### Access Requirements

```bash
# Verify cluster access
kubectl cluster-info
kubectl get nodes

# Verify required namespaces
kubectl create namespace netweave --dry-run=client -o yaml

# Verify RBAC permissions
kubectl auth can-i create deployments --namespace netweave
kubectl auth can-i create services --namespace netweave
kubectl auth can-i create secrets --namespace netweave
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

# Vault CLI (optional, for certificate management)
curl -sSL https://releases.hashicorp.com/vault/1.17.0/vault_1.17.0_linux_amd64.zip -o vault.zip
unzip vault.zip
sudo mv vault /usr/local/bin
vault version
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
kubectl get pods -n netweave
kubectl logs -n netweave -l app=netweave-gateway --tail=20
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

**Install HashiCorp Vault (for PKI certificate management):**
```bash
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update

helm install vault hashicorp/vault \
  --namespace vault-system \
  --create-namespace \
  --set server.dev.enabled=false

# Verify installation
kubectl get pods -n vault-system
vault status
```

**Install Redis Sentinel cluster:**
```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

helm install redis bitnami/redis \
  --namespace netweave \
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
kubectl get pods -n netweave -l app.kubernetes.io/name=redis
kubectl exec -n netweave redis-node-0 -- redis-cli INFO replication
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

**Install Keycloak (Optional - for Keycloak auth backend):**

If you want to use Keycloak as the authentication backend instead of Redis:

```bash
# Add Bitnami repo (if not already added)
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Install Keycloak
helm install keycloak bitnami/keycloak \
  --namespace netweave \
  --set auth.adminUser=admin \
  --set auth.adminPassword="$(openssl rand -base64 32)" \
  --set postgresql.enabled=true \
  --set postgresql.auth.password="$(openssl rand -base64 32)" \
  --set service.type=ClusterIP \
  --set ingress.enabled=false

# Verify Keycloak deployment
kubectl get pods -n netweave -l app.kubernetes.io/name=keycloak
kubectl logs -n netweave -l app.kubernetes.io/name=keycloak --tail=20

# Port-forward to access Keycloak admin console
kubectl port-forward -n netweave svc/keycloak 8080:80
# Access at: http://localhost:8080
# Username: admin
# Password: (retrieve with: kubectl get secret -n netweave keycloak -o jsonpath='{.data.admin-password}' | base64 -d)
```

**Configure Keycloak realm and client:**

1. Access Keycloak admin console (http://localhost:8080 via port-forward)
2. Create a new realm: `netweave`
3. Create a client for the gateway:
   - Client ID: `netweave-gateway`
   - Client Protocol: `openid-connect`
   - Access Type: `confidential`
   - Service Accounts Enabled: `true`
   - Valid Redirect URIs: `*` (restrict in production)
4. Save and note the client secret from the Credentials tab
5. Create admin user or use master realm admin
6. Grant realm management permissions to the admin user

**Create Keycloak secrets for gateway:**

```bash
# Get Keycloak admin password
KEYCLOAK_ADMIN_PASSWORD=$(kubectl get secret -n netweave keycloak -o jsonpath='{.data.admin-password}' | base64 -d)

# Get client secret from Keycloak admin console (Clients -> netweave-gateway -> Credentials tab)
KEYCLOAK_CLIENT_SECRET="your-client-secret-from-keycloak"

# Create secrets for gateway
kubectl create secret generic keycloak-credentials \
  --namespace netweave \
  --from-literal=client-secret="$KEYCLOAK_CLIENT_SECRET" \
  --from-literal=admin-password="$KEYCLOAK_ADMIN_PASSWORD"

# Verify secret
kubectl get secret -n netweave keycloak-credentials
```

#### Step 2: Configure TLS Certificates

**Configure Vault PKI for certificate management:**
```bash
# Enable PKI secrets engine
vault secrets enable pki
vault secrets tune -max-lease-ttl=87600h pki

# Generate root CA
vault write -field=certificate pki/root/generate/internal \
  common_name="Netweave Root CA" \
  issuer_name="netweave-root" \
  ttl=87600h > ca.crt

# Enable intermediate PKI
vault secrets enable -path=pki_int pki
vault secrets tune -max-lease-ttl=43800h pki_int

# Create intermediate CA
vault write -format=json pki_int/intermediate/generate/internal \
  common_name="Netweave Intermediate CA" | jq -r '.data.csr' > pki_int.csr

# Sign intermediate with root
vault write -format=json pki/root/sign-intermediate \
  csr=@pki_int.csr \
  format=pem_bundle \
  ttl=43800h | jq -r '.data.certificate' > signed_certificate.pem

vault write pki_int/intermediate/set-signed certificate=@signed_certificate.pem

# Create PKI role for gateway certificates
vault write pki_int/roles/netweave-mtls \
  allowed_domains="netweave.local" \
  allow_subdomains=true \
  max_ttl=720h

# Verify PKI setup
vault read pki_int/roles/netweave-mtls
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
    - "redis-node-0.redis-headless.netweave.svc.cluster.local:26379"
    - "redis-node-1.redis-headless.netweave.svc.cluster.local:26379"
    - "redis-node-2.redis-headless.netweave.svc.cluster.local:26379"
  password:
    secretName: redis
    secretKey: redis-password

# Authentication backend configuration
auth:
  backend: redis  # "redis" (default) or "keycloak"

  # Keycloak configuration (only used if backend: keycloak)
  keycloak:
    baseURL: http://keycloak.netweave.svc.cluster.local
    realm: netweave
    clientID: netweave-gateway
    clientSecretEnvVar: KEYCLOAK_CLIENT_SECRET
    adminUsername: admin
    adminPasswordEnvVar: KEYCLOAK_ADMIN_PASSWORD
    timeout: 30s

  # Environment variables for secrets (set via extraEnv)
  extraEnv:
    - name: KEYCLOAK_CLIENT_SECRET
      valueFrom:
        secretKeyRef:
          name: keycloak-credentials
          key: client-secret
    - name: KEYCLOAK_ADMIN_PASSWORD
      valueFrom:
        secretKeyRef:
          name: keycloak-credentials
          key: admin-password

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

# Multi-port gateway configuration
# The gateway runs 4 separate listeners with independent TLS/auth
# See docs/architecture/system-overview.md for details
gateway:
  server:
    port: 8080       # Admin (OAuth2)
    o2_port: 8443    # O2 (mTLS)
    tmf_port: 8444   # TMForum (OAuth2)
    graphql_port: 8445  # GraphQL (OAuth2)

# 4 separate ingress resources (one per hostname/port)
ingress:
  admin:
    enabled: true
    className: nginx
    host: api.netweave.local
    tls:
      secretName: netweave-admin-tls
  o2:
    enabled: true
    className: nginx
    host: o2.netweave.local
    annotations:
      nginx.ingress.kubernetes.io/ssl-passthrough: "true"
  tmf:
    enabled: true
    className: nginx
    host: tmf.netweave.local
    tls:
      secretName: netweave-tmf-tls
  graphql:
    enabled: true
    className: nginx
    host: graphql.netweave.local
    tls:
      secretName: netweave-graphql-tls
```

**Deploy gateway:**
```bash
helm install netweave ./helm/netweave \
  --namespace netweave \
  --values values-custom.yaml \
  --timeout 10m \
  --wait

# Verify deployment
helm status netweave -n netweave
kubectl get pods -n netweave -l app.kubernetes.io/name=netweave
```

#### Step 4: Verify Deployment

```bash
# Check pod status
kubectl get pods -n netweave -l app.kubernetes.io/name=netweave

# Check pod logs
kubectl logs -n netweave -l app.kubernetes.io/name=netweave --tail=50

# Check TLS certificates
kubectl get certificate -n netweave
kubectl describe certificate netweave-tls -n netweave

# Test health endpoint
kubectl port-forward -n netweave svc/netweave-gateway 8080:8080 &
curl -k https://localhost:8080/healthz

# Test Redis connectivity
kubectl exec -n netweave -it netweave-gateway-0 -- sh -c 'redis-cli -h redis-node-0 PING'

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
  namespace: netweave
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

  # Ingress (4 separate ingresses for multi-port gateway)
  ingress:
    admin:
      enabled: true
      host: api.netweave.local
    o2:
      enabled: true
      host: o2.netweave.local
    tmf:
      enabled: true
      host: tmf.netweave.local
    graphql:
      enabled: true
      host: graphql.netweave.local
```

**Apply custom resource:**
```bash
kubectl apply -f o2imsgateway-production.yaml

# Watch operator reconcile
kubectl get o2imsgateways -n netweave -w

# Check operator logs
kubectl logs -n o2ims-operator-system -l app=o2ims-operator -f
```

#### Step 3: Verify Operator Deployment

```bash
# Check custom resource status
kubectl get o2imsgateways netweave-production -n netweave -o yaml

# Check created resources
kubectl get all -n netweave -l app.kubernetes.io/managed-by=o2ims-operator

# Verify health
kubectl port-forward -n netweave svc/netweave-production-gateway 8080:8080 &
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
  --namespace netweave \
  --set image.tag=v1.1.0 \
  --wait

# Monitor rollout
kubectl rollout status deployment/netweave-gateway -n netweave -w

# Verify new version
kubectl get pods -n netweave -l app.kubernetes.io/name=netweave \
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
  namespace: netweave
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
  namespace: netweave
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
  -l color=green -n netweave --timeout=300s

# 3. Test green deployment internally
kubectl port-forward -n netweave deploy/netweave-gateway-green 8080:8080 &
curl -k https://localhost:8080/healthz
curl -k https://localhost:8080/o2ims-infrastructureInventory/v1/api_versions

# 4. Switch Service selector to green
kubectl patch svc netweave-gateway -n netweave \
  -p '{"spec":{"selector":{"color":"green"}}}'

# 5. Monitor for 10 minutes
watch kubectl get pods -n netweave

# 6. If successful, delete blue deployment
kubectl delete deployment netweave-gateway-blue -n netweave

# ROLLBACK if issues:
kubectl patch svc netweave-gateway -n netweave \
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
  namespace: netweave
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
  namespace: netweave
spec:
  hosts:
    - netweave-gateway.netweave.svc.cluster.local
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
watch 'kubectl top pods -n netweave | grep canary'

# 3. Gradually increase canary traffic
# 10% -> 25% -> 50% -> 75% -> 100%
kubectl patch virtualservice netweave-gateway -n netweave \
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
kubectl exec -n netweave netweave-gateway-canary-0 -- \
  wget -qO- localhost:8080/metrics | grep o2ims_http_request_duration_seconds

# 5. If successful, scale up canary to replace stable
kubectl scale deployment netweave-gateway-canary -n netweave --replicas=3
kubectl scale deployment netweave-gateway -n netweave --replicas=0

# 6. Update VirtualService to 100% canary
kubectl patch virtualservice netweave-gateway -n netweave \
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
  namespace: netweave
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
kubectl exec -n netweave redis-node-0 -c redis -- redis-cli \
  REPLICAOF redis-cluster1.example.com 6379

# Verify replication
kubectl exec -n netweave redis-node-0 -c redis -- redis-cli INFO replication
```

### Gateway Configuration for Multi-Cluster

```yaml
# Gateway ConfigMap (both clusters)
apiVersion: v1
kind: ConfigMap
metadata:
  name: netweave-config
  namespace: netweave
data:
  config.yaml: |
    redis:
      sentinel: true
      master_set: "mymaster"
      sentinel_addresses:
        - "redis-sentinel-0.redis-headless.netweave.svc.cluster.local:26379"
        - "redis-sentinel-1.redis-headless.netweave.svc.cluster.local:26379"
        - "redis-sentinel-2.redis-headless.netweave.svc.cluster.local:26379"
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
  --namespace netweave \
  --from-literal=password="$(openssl rand -base64 32)"

# Create TLS secrets
kubectl create secret tls gateway-tls \
  --namespace netweave \
  --cert=server.crt \
  --key=server.key

# Create CA bundle secret
kubectl create secret generic ca-bundle \
  --namespace netweave \
  --from-file=ca.crt=ca-bundle.crt
```

## Post-Deployment Verification

### Health Checks

```bash
# Gateway health
kubectl exec -n netweave netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/healthz

# Redis health
kubectl exec -n netweave redis-node-0 -- redis-cli PING

# Sentinel health
kubectl exec -n netweave redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL master mymaster
```

### Functional Tests

```bash
# Test health endpoint (admin port, no mTLS)
curl -k https://api.netweave.local/health

# Test O2-IMS API (O2 port, requires mTLS client cert)
curl --cert client.crt --key client.key --cacert ca.crt \
  https://o2.netweave.local/o2ims-infrastructureInventory/v1/api_versions

# Test resource pool listing (O2 port)
curl --cert client.crt --key client.key --cacert ca.crt \
  -H "Accept: application/json" \
  https://o2.netweave.local/o2ims-infrastructureInventory/v1/resourcePools

# Create test subscription (O2 port)
curl --cert client.crt --key client.key --cacert ca.crt \
  -X POST https://o2.netweave.local/o2ims-infrastructureInventory/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "callback": "https://smo.example.com/notify",
    "consumerSubscriptionId": "test-sub-001"
  }'
```

### Performance Validation

```bash
# Load test with hey (O2 port, requires mTLS)
hey -n 1000 -c 10 -m GET \
  -cert client.crt -key client.key \
  https://o2.netweave.local/o2ims-infrastructureInventory/v1/resourcePools

# Check p95 latency (should be < 100ms)
kubectl exec -n netweave netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_request_duration_seconds | grep quantile
```

## Troubleshooting Deployment Issues

### Pods Not Starting

```bash
# Check pod status
kubectl describe pod -n netweave netweave-gateway-0

# Common issues:
# - ImagePullBackOff: Check image repository and credentials
# - CrashLoopBackOff: Check logs for errors
# - Pending: Check resource constraints
```

### Redis Connection Failures

```bash
# Test Redis connectivity
kubectl exec -n netweave netweave-gateway-0 -- \
  redis-cli -h redis-node-0 PING

# Check Redis logs
kubectl logs -n netweave redis-node-0

# Verify Sentinel
kubectl exec -n netweave redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster
```

### Certificate Issues

```bash
# Check certificate status
kubectl get certificate -n netweave
kubectl describe certificate netweave-tls -n netweave

# Check Vault PKI status
vault read pki_int/cert/ca
kubectl get secret netweave-tls -n netweave

# Issue new certificate via Vault PKI
vault write pki_int/issue/netweave-mtls \
  common_name="o2.netweave.local" \
  alt_names="api.netweave.local,tmf.netweave.local,graphql.netweave.local" \
  ttl=720h
```

## Rollback Procedures

### Helm Rollback

```bash
# List release history
helm history netweave -n netweave

# Rollback to previous version
helm rollback netweave -n netweave

# Rollback to specific revision
helm rollback netweave 3 -n netweave
```

### Kubectl Rollback

```bash
# Rollback deployment
kubectl rollout undo deployment/netweave-gateway -n netweave

# Rollback to specific revision
kubectl rollout undo deployment/netweave-gateway -n netweave --to-revision=2

# Check rollout history
kubectl rollout history deployment/netweave-gateway -n netweave
```

## Related Documentation

- [Operations Overview](README.md)
- [Monitoring Guide](monitoring.md)
- [Troubleshooting Guide](troubleshooting.md)
- [Upgrade Procedures](upgrades.md)
