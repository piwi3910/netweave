# netweave O2-IMS Gateway - Architecture Documentation

**Version:** 1.0
**Date:** 2026-01-06
**Status:** Draft

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [System Overview](#system-overview)
3. [Architecture Goals](#architecture-goals)
4. [Component Architecture](#component-architecture)
5. [Data Flow](#data-flow)
6. [Storage Architecture](#storage-architecture)
7. [Security Architecture](#security-architecture)
8. [High Availability & Disaster Recovery](#high-availability--disaster-recovery)
9. [Scalability](#scalability)
10. [Deployment Architecture](#deployment-architecture)
11. [Technology Stack](#technology-stack)
12. [Design Decisions](#design-decisions)

---

## Executive Summary

**netweave** is an ORAN O2-IMS compliant API gateway that enables disaggregation of telecom infrastructure by translating standardized O2-IMS API requests into native Kubernetes API calls. This allows Service Management and Orchestration (SMO) systems to manage infrastructure resources across multiple vendor backends through a single, standardized interface.

### Key Capabilities

- **O2-IMS Compliance**: Full implementation of O-RAN O2 Infrastructure Management Services specification
- **Kubernetes Native**: Translates O2-IMS requests to native Kubernetes API operations
- **Multi-Cluster Ready**: Single or multi-cluster deployment with Redis-based state synchronization
- **High Availability**: Stateless gateway pods with automatic failover
- **Production Grade**: Enterprise security, observability, and operational excellence
- **Extensible**: Plugin-based adapter architecture for future backend integrations

### Target Use Cases

1. **Telecom Infrastructure Management**: Enable SMO to manage O-Cloud infrastructure via standard O2-IMS APIs
2. **Multi-Vendor Disaggregation**: Abstract vendor-specific APIs behind O2-IMS standard
3. **Cloud-Native RAN**: Manage Kubernetes-based RAN workload infrastructure
4. **Infrastructure Lifecycle**: Provision, monitor, and manage infrastructure resources
5. **Event Subscriptions**: Real-time notifications of infrastructure changes

---

## System Overview

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       O2 SMO Systems                            │
│              (Service Management & Orchestration)               │
└─────────────────────────┬───────────────────────────────────────┘
                          │ O2-IMS API (REST/HTTPS)
                          │ mTLS Authentication
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│              Kubernetes Ingress / LoadBalancer                  │
│  • TLS Termination (native)  • Client Cert Validation (Go)     │
│  • Load Balancing (K8s)      • Rate Limiting (middleware)      │
│  • Health-based routing      • cert-manager certificates       │
└─────────────────────────┬───────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌─────────┐     ┌─────────┐     ┌─────────┐
    │Gateway 1│     │Gateway 2│     │Gateway 3│  (3+ pods)
    │         │     │         │     │         │
    │STATELESS│     │STATELESS│     │STATELESS│
    │All Equal│     │All Equal│     │All Equal│
    │Native TLS│     │Native TLS│     │Native TLS│
    └────┬────┘     └────┬────┘     └────┬────┘
         │               │               │
         └───────────────┼───────────────┘
                         ▼
         ┌───────────────────────────────┐
         │      Redis (Always Present)   │
         │  • Subscriptions              │
         │  • Cache                      │
         │  • Pub/Sub                    │
         │  • Session Sync               │
         └───────┬───────────────────────┘
                 │
    ┌────────────┼────────────┐
    ▼            ▼            ▼
┌─────────┐  ┌─────────┐  ┌─────────┐
│Sentinel │  │Sentinel │  │Sentinel │
│   1     │  │   2     │  │   3     │
└─────────┘  └─────────┘  └─────────┘
                 │
                 ▼
         ┌───────────────────────────────┐
         │    Kubernetes API Server      │
         │  • Nodes (Resources)          │
         │  • Pods, Deployments          │
         │  • PersistentVolumes          │
         │  • MachineSets (Pools)        │
         │  • StorageClasses (Types)     │
         └───────────────────────────────┘
                 ▲
                 │
         ┌───────────────────────────────┐
         │  Subscription Controller      │
         │  • Watches Redis              │
         │  • Watches K8s Resources      │
         │  • Sends Webhooks             │
         └───────────────────────────────┘
```

### System Context

```
┌──────────────────────────────────────────────────────────────────┐
│                     External Systems                             │
│                                                                  │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐  │
│  │   O2 SMO     │      │ Monitoring   │      │  Logging     │  │
│  │   (Client)   │      │ (Prometheus) │      │  (ELK/Loki)  │  │
│  └──────────────┘      └──────────────┘      └──────────────┘  │
│         │                      │                      │         │
└─────────┼──────────────────────┼──────────────────────┼─────────┘
          │ O2-IMS               │ Metrics              │ Logs
          │ (HTTPS/mTLS)         │ (Prometheus)         │ (JSON)
          ▼                      ▼                      ▼
┌──────────────────────────────────────────────────────────────────┐
│                    netweave O2-IMS Gateway                       │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Gateway Layer                                             │ │
│  │  • O2-IMS API Implementation                              │ │
│  │  • Request Validation (OpenAPI)                           │ │
│  │  • Authentication/Authorization                           │ │
│  │  • Rate Limiting                                          │ │
│  └────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Translation Layer                                         │ │
│  │  • O2-IMS ↔ Kubernetes Mapping                           │ │
│  │  • Data Transformation                                     │ │
│  │  • Error Translation                                       │ │
│  └────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Adapter Layer                                             │ │
│  │  • Kubernetes Adapter (active)                            │ │
│  │  • Future Adapters (Dell DTIAS, etc.)                     │ │
│  └────────────────────────────────────────────────────────────┘ │
└──────────────┬───────────────────────────────┬──────────────────┘
               │                               │
               │ K8s API                       │ Redis Protocol
               │ (gRPC/HTTPS)                  │ (RESP)
               ▼                               ▼
┌──────────────────────────┐    ┌──────────────────────────┐
│  Kubernetes Cluster      │    │  Redis Sentinel Cluster  │
│  • Nodes                 │    │  • Master + Replicas     │
│  • Workloads             │    │  • Automatic Failover    │
│  • Storage               │    │  • Persistence (AOF+RDB) │
└──────────────────────────┘    └──────────────────────────┘
```

---

## Architecture Goals

### Functional Goals

1. **O2-IMS Compliance**
   - Full implementation of O-RAN O2-IMS specification
   - OpenAPI-driven development
   - Strict schema validation

2. **Backend Abstraction**
   - Translate O2-IMS to Kubernetes API
   - Support for future backend adapters (Dell DTIAS, etc.)
   - Consistent error handling and responses

3. **Real-Time Notifications**
   - Subscription-based event delivery
   - Webhook notifications to SMO systems
   - Filtering and transformation

### Non-Functional Goals

1. **Performance**
   - API response: p95 < 100ms, p99 < 500ms
   - Webhook delivery: < 1s from event to notification
   - Cache hit ratio: > 90%
   - Support 1000+ req/sec per cluster

2. **Reliability**
   - 99.9% uptime (< 8.76 hours downtime/year)
   - Zero-downtime deployments
   - Automatic failover < 30s
   - Graceful degradation

3. **Scalability**
   - Horizontal scaling (add more gateway pods)
   - Multi-cluster support
   - Handle 10,000+ nodes per cluster
   - 100+ concurrent subscriptions

4. **Security**
   - mTLS everywhere
   - Zero-trust networking
   - No hardcoded secrets
   - Minimal attack surface
   - Audit logging

5. **Observability**
   - Comprehensive metrics (Prometheus)
   - Distributed tracing (Jaeger)
   - Structured logging
   - Health checks and dashboards

6. **Operability**
   - GitOps-friendly
   - Configuration as code
   - Simple rollback procedures
   - Clear operational runbooks

---

## Component Architecture

### Gateway Pods

**Responsibility**: Handle O2-IMS API requests, translate to backend operations

```go
// High-level structure
type Gateway struct {
    // HTTP server (Gin framework)
    router *gin.Engine

    // O2-IMS handlers
    dmHandler   *DeploymentManagerHandler
    poolHandler *ResourcePoolHandler
    resHandler  *ResourceHandler
    subHandler  *SubscriptionHandler

    // Backend adapter
    adapter adapter.BackendAdapter

    // Cache layer
    cache cache.Cache

    // Observability
    metrics *prometheus.Registry
    tracer  trace.Tracer
    logger  *zap.Logger
}
```

**Characteristics**:
- **Stateless**: No local state (all in Redis or K8s)
- **Identical**: All pods are equal, no leader
- **Scalable**: Add/remove pods without coordination
- **Fast startup**: < 5s to ready state

**Lifecycle**:
1. Load configuration (env vars, ConfigMap)
2. Connect to Redis (Sentinel)
3. Connect to Kubernetes API
4. Register routes and middleware
5. Start HTTP server on port 8080
6. Signal readiness (liveness/readiness probes)

### Redis Cluster

**Responsibility**: Shared state, caching, pub/sub

**Deployment**: Redis Sentinel for HA
- 1 master + 2+ replicas per cluster
- Automatic failover via Sentinel (quorum=2)
- Persistence: AOF + RDB snapshots

**Data Stored**:
1. **Subscriptions** (primary data)
   ```
   subscription:{uuid} → Hash {id, callback, filter, ...}
   subscriptions:active → Set of UUIDs
   subscriptions:resourcePool:{id} → Set of UUIDs
   ```

2. **Cache** (performance optimization)
   ```
   cache:nodes:list → JSON (TTL: 30s)
   cache:resourcePools:list → JSON (TTL: 30s)
   cache:node:{id} → JSON (TTL: 60s)
   ```

3. **Pub/Sub** (inter-pod communication)
   ```
   cache:invalidate:nodes → Event stream
   subscriptions:created → Event stream
   subscriptions:deleted → Event stream
   ```

4. **Distributed Locks** (coordination)
   ```
   lock:webhook:{sub_id}:{event_id} → {pod_id} (TTL: 10s)
   lock:cache:refresh → {pod_id} (TTL: 5s)
   ```

### Subscription Controller

**Responsibility**: Watch for K8s changes, send webhook notifications

```go
type SubscriptionController struct {
    redis      *redis.Client
    k8sClient  client.Client
    subStore   storage.SubscriptionStore
    webhookSvc *WebhookService
}

// Main loop
func (c *SubscriptionController) Run(ctx context.Context) {
    // Watch K8s resources
    nodeInformer.AddEventHandler(...)
    podInformer.AddEventHandler(...)

    // Watch Redis subscription events
    go c.syncRedisSubscriptions(ctx)

    // Process notification queue
    go c.processWebhooks(ctx)
}
```

**Deployment**: 3+ pods with leader election
- Only leader actively sends webhooks (prevents duplicates)
- Standby pods ready for immediate takeover
- Leader election via Kubernetes Lease object

**Event Processing**:
1. K8s informer detects change (Node added, Pod failed, etc.)
2. Query subscriptions from Redis matching the change
3. For each matching subscription:
   - Transform K8s event to O2-IMS format
   - Enqueue webhook delivery
4. Webhook worker:
   - Acquire distributed lock (prevent duplicates)
   - POST to callback URL
   - Retry with exponential backoff (3 attempts)
   - Update subscription status

### Kubernetes Adapter

**Responsibility**: Translate O2-IMS operations to Kubernetes API calls

```go
type KubernetesAdapter struct {
    client    client.Client
    clientset *kubernetes.Clientset
}

// Example: List Resource Pools → List MachineSets
func (a *KubernetesAdapter) ListResourcePools(
    ctx context.Context,
    dmID string,
    filter *Filter,
) ([]models.ResourcePool, error) {
    // 1. List Kubernetes MachineSets
    machineSets := &machinev1beta1.MachineSetList{}
    err := a.client.List(ctx, machineSets)

    // 2. Transform to O2-IMS ResourcePool
    pools := make([]models.ResourcePool, len(machineSets.Items))
    for i, ms := range machineSets.Items {
        pools[i] = transformMachineSetToResourcePool(&ms)
    }

    // 3. Apply filters
    return applyFilters(pools, filter), nil
}
```

**Mappings** (see [api-mapping.md](api-mapping.md) for details):
- DeploymentManager → Cluster metadata (custom)
- ResourcePool → MachineSet / NodePool
- Resource → Node / Machine
- ResourceType → StorageClass, Machine flavors

### TLS and Certificate Management

**Responsibility**: Secure communication, certificate lifecycle

**Implementation**: Native Go TLS + cert-manager

**Features**:
1. **Native Go TLS 1.3**
   ```go
   // internal/server/tls.go
   func configureTLS(cfg *config.Config) *tls.Config {
       // Load server certificate
       cert, _ := tls.LoadX509KeyPair(cfg.TLS.CertPath, cfg.TLS.KeyPath)

       // Load CA for client verification
       caCert, _ := os.ReadFile(cfg.TLS.CACertPath)
       caCertPool := x509.NewCertPool()
       caCertPool.AppendCertsFromPEM(caCert)

       return &tls.Config{
           Certificates: []tls.Certificate{cert},
           ClientAuth:   tls.RequireAndVerifyClientCert,
           ClientCAs:    caCertPool,
           MinVersion:   tls.VersionTLS13,
           CipherSuites: []uint16{
               tls.TLS_AES_256_GCM_SHA384,
               tls.TLS_AES_128_GCM_SHA256,
               tls.TLS_CHACHA20_POLY1305_SHA256,
           },
       }
   }
   ```

2. **cert-manager Integration**
   - Automatic certificate issuance
   - Auto-renewal (90-day rotation)
   - Kubernetes Secret storage
   - No manual certificate management

3. **Kubernetes Ingress**
   - Native LoadBalancer or Ingress controller
   - TLS passthrough to application
   - Health-based routing
   - Session affinity (optional)

4. **Client Certificate Validation**
   ```go
   // Middleware to extract and validate client cert
   func ClientCertAuth() gin.HandlerFunc {
       return func(c *gin.Context) {
           if c.Request.TLS == nil || len(c.Request.TLS.PeerCertificates) == 0 {
               c.AbortWithStatusJSON(401, gin.H{"error": "client certificate required"})
               return
           }

           clientCert := c.Request.TLS.PeerCertificates[0]
           clientCN := clientCert.Subject.CommonName

           // Store identity for authorization
           c.Set("clientIdentity", clientCN)
           c.Next()
       }
   }
   ```

---

## Data Flow

### Request Flow: List Resource Pools

```
┌─────────┐
│   SMO   │
└────┬────┘
     │ 1. GET /o2ims/v1/resourcePools
     │    Authorization: mTLS client cert
     ▼
┌─────────────────┐
│ K8s Ingress     │
│ • TLS handshake │
│ • Validate cert │
│ • Route to pod  │
└────┬────────────┘
     │ 2. Forward to healthy gateway pod
     ▼
┌─────────────────┐
│  Gateway Pod    │
│                 │
│ 3. Middleware:  │
│    • Auth       │
│    • Logging    │
│    • Metrics    │
└────┬────────────┘
     │ 4. Check cache (Redis)
     ▼
┌─────────────────┐
│  Redis          │
│ Key: cache:     │
│   resourcePools │
└────┬────────────┘
     │ 5a. Cache HIT → Return
     │ 5b. Cache MISS ↓
     ▼
┌─────────────────┐
│ K8s Adapter     │
│ 6. List         │
│    MachineSets  │
└────┬────────────┘
     │ 7. API call
     ▼
┌─────────────────┐
│ Kubernetes API  │
│ 8. Return       │
│    MachineSets  │
└────┬────────────┘
     │ 9. Transform to O2-IMS
     ▼
┌─────────────────┐
│ Gateway Pod     │
│ 10. Cache result│
│     (Redis)     │
│ 11. Return JSON │
└────┬────────────┘
     │ 12. O2-IMS response
     ▼
┌─────────┐
│   SMO   │
└─────────┘

Timeline:
- Redis cache hit: 5-10ms
- Cache miss + K8s: 50-100ms
```

### Write Flow: Create Resource Pool

```
┌─────────┐
│   SMO   │
└────┬────┘
     │ 1. POST /o2ims/v1/resourcePools
     │    Body: {name, resources, ...}
     ▼
┌─────────────────┐
│ K8s Ingress     │
└────┬────────────┘
     │
     ▼
┌─────────────────┐
│  Gateway Pod    │
│ 2. Validate     │
│    request body │
│    (OpenAPI)    │
└────┬────────────┘
     │ 3. Transform to MachineSet
     ▼
┌─────────────────┐
│ K8s Adapter     │
│ 4. Create       │
│    MachineSet   │
└────┬────────────┘
     │ 5. API call (client-go)
     ▼
┌─────────────────┐
│ Kubernetes API  │
│ 6. Create       │
│    resource     │
│ (atomic)        │
└────┬────────────┘
     │ 7. Success
     ▼
┌─────────────────┐
│ Gateway Pod     │
│ 8. Invalidate   │
│    cache (Redis)│
└────┬────────────┘
     │ 9. Publish event
     ▼
┌─────────────────┐
│ Redis Pub/Sub   │
│ cache:invalidate│
│ :resourcePools  │
└────┬────────────┘
     │ 10. All pods receive
     ▼
┌─────────────────┐
│ All Gateway Pods│
│ Clear local     │
│ cache           │
└─────────────────┘
     │ 11. Return response
     ▼
┌─────────┐
│   SMO   │
└─────────┘

Timeline: 100-200ms
```

### Subscription Notification Flow

```
┌─────────────────┐
│ Kubernetes API  │
│ Node added      │
└────┬────────────┘
     │ 1. Event emitted
     ▼
┌─────────────────┐
│ K8s Informer    │
│ (in controller) │
│ 2. Detects      │
│    change       │
└────┬────────────┘
     │ 3. Query matching subscriptions
     ▼
┌─────────────────┐
│ Redis           │
│ subscriptions:  │
│ node:*          │
└────┬────────────┘
     │ 4. Return matching subs
     ▼
┌──────────────────┐
│ Subscription     │
│ Controller       │
│ 5. For each sub: │
│    • Transform   │
│    • Enqueue     │
└────┬─────────────┘
     │ 6. Acquire lock
     ▼
┌─────────────────┐
│ Redis Lock      │
│ Prevents        │
│ duplicates      │
└────┬────────────┘
     │ 7. Send webhook
     ▼
┌─────────────────┐
│ Webhook Worker  │
│ 8. POST to      │
│    callback URL │
│ (retry 3x)      │
└────┬────────────┘
     │ 9. HTTP POST
     ▼
┌─────────┐
│   SMO   │
│ Webhook │
│ endpoint│
└─────────┘

Timeline: < 1s from K8s event to webhook
```

---

## Storage Architecture

### Redis Data Model

#### Subscriptions

```redis
# Subscription object
HSET subscription:550e8400-e29b-41d4-a716-446655440000
  id "550e8400-e29b-41d4-a716-446655440000"
  callback "https://smo.example.com/notifications"
  filter '{"resourcePoolId":"pool-123"}'
  consumerSubscriptionId "smo-sub-456"
  createdAt "2026-01-06T10:30:00Z"
  data '{"id":"550e...","callback":"https://..."}'

# Index: All active subscriptions
SADD subscriptions:active "550e8400-e29b-41d4-a716-446655440000"

# Index: By resource pool
SADD subscriptions:resourcePool:pool-123 "550e8400-e29b-41d4-a716-446655440000"

# Index: By resource type
SADD subscriptions:resourceType:compute "550e8400-e29b-41d4-a716-446655440000"
```

#### Cache

```redis
# Cache resource lists (short TTL)
SETEX cache:nodes '{"items":[{...}]}' 30
SETEX cache:resourcePools '{"items":[{...}]}' 30
SETEX cache:resources:pool-123 '{"items":[{...}]}' 30

# Cache individual resources (longer TTL)
SETEX cache:node:node-abc '{"id":"node-abc",...}' 60
SETEX cache:resourcePool:pool-123 '{"id":"pool-123",...}' 60

# Cache statistics
SETEX cache:stats:summary '{"nodes":100,"pools":10}' 120
```

#### Pub/Sub Channels

```redis
# Cache invalidation
PUBLISH cache:invalidate:nodes "node-abc-deleted"
PUBLISH cache:invalidate:resourcePools "pool-123-updated"

# Subscription events
PUBLISH subscriptions:created "550e8400-e29b-41d4-a716-446655440000"
PUBLISH subscriptions:deleted "550e8400-e29b-41d4-a716-446655440000"
```

#### Distributed Locks

```redis
# Webhook delivery lock (prevent duplicate sends)
SET lock:webhook:550e8400:event-123 "gateway-pod-2" NX EX 10

# Cache refresh lock (only one pod refreshes)
SET lock:cache:nodes "gateway-pod-1" NX EX 5

# Background job lock
SET lock:job:cleanup "controller-pod-1" NX EX 30
```

### Redis Sentinel Configuration

```yaml
# 3-node Sentinel setup (quorum=2)
Master:
  - Host: redis-master-0
  - Port: 6379
  - Persistence: AOF (appendonly yes) + RDB
  - Max Memory: 2GB
  - Eviction: allkeys-lru

Replicas:
  - redis-replica-1 (async replication from master)
  - redis-replica-2 (async replication from master)

Sentinels:
  - redis-sentinel-0:26379
  - redis-sentinel-1:26379
  - redis-sentinel-2:26379
  - Quorum: 2
  - Down-after-milliseconds: 5000
  - Failover-timeout: 10000
```

### Kubernetes State (Source of Truth)

All infrastructure state lives in Kubernetes:

```
Resources (K8s) → O2-IMS Mapping
─────────────────────────────────
Nodes          → Resources
MachineSets    → ResourcePools
Machines       → Resources (with lifecycle)
StorageClasses → ResourceTypes
PVs            → Storage Resources
```

Gateway never modifies K8s state directly in Redis - Redis is only for:
1. Subscriptions (O2-IMS specific, not in K8s)
2. Performance caching
3. Inter-pod communication

---

## Security Architecture

### Zero-Trust Principles

1. **Assume Breach**: Design for scenarios where perimeter is compromised
2. **Verify Explicitly**: Authenticate and authorize every request
3. **Least Privilege**: Minimal permissions for all components
4. **Encrypt Everything**: mTLS for all communication

### Authentication & Authorization

#### North-Bound (SMO → Gateway)

```
┌──────────┐
│   SMO    │
└────┬─────┘
     │ 1. mTLS Handshake
     │    Client Certificate
     ▼
┌────────────────┐
│ K8s Ingress    │
│ • TLS accept   │
│ • Forward      │
└────┬───────────┘
     │ 2. TLS connection to pod
     ▼
┌────────────────┐
│ Gateway Pod    │
│ (Go TLS 1.3)   │
│ • Verify cert  │
│ • Check CN/SAN │
│ • Validate CA  │
│ • Extract CN   │
│ • Map to roles │
│ • Authorize    │
└────────────────┘
```

**Certificate Requirements**:
- Client certificates issued by trusted CA
- CN contains SMO identifier
- SAN includes callback domain
- Certificates rotated every 90 days (cert-manager automation)

**Authorization Model**:
```go
type Permission string

const (
    ReadResources  Permission = "resources:read"
    WriteResources Permission = "resources:write"
    ManageSubscriptions Permission = "subscriptions:manage"
)

// Map SMO identity to permissions
func authorize(clientCN string, requiredPerm Permission) bool {
    // Example: CN=smo-system-1,OU=orchestration
    roles := getRolesFromCertificate(clientCN)
    return roles.Has(requiredPerm)
}
```

#### South-Bound (Gateway → Kubernetes)

```
┌────────────────┐
│ Gateway Pod    │
└────┬───────────┘
     │ 1. Use ServiceAccount token
     │    Mounted at /var/run/secrets/kubernetes.io/serviceaccount
     ▼
┌────────────────┐
│ Kubernetes API │
│ • Verify token │
│ • Check RBAC   │
└────────────────┘
```

**ServiceAccount Permissions** (RBAC):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: o2ims-gateway
rules:
  # Read infrastructure resources
  - apiGroups: [""]
    resources: ["nodes", "persistentvolumes", "storageclasses"]
    verbs: ["get", "list", "watch"]

  # Manage machine resources
  - apiGroups: ["machine.openshift.io"]
    resources: ["machinesets", "machines"]
    verbs: ["get", "list", "watch", "create", "update", "delete"]

  # NO access to secrets, pods, deployments (least privilege)
```

### mTLS Architecture

#### Certificate Hierarchy

```
Root CA (cert-manager ClusterIssuer)
  │
  ├─ Server CA (for gateway pods)
  │   ├─ Gateway pod server certs (TLS)
  │   ├─ Redis TLS certs
  │   └─ Internal service certs
  │
  ├─ Client CA (for external clients)
  │   ├─ SMO client certs (issued externally or via cert-manager)
  │   └─ Trusted client certificates
  │
  └─ Webhook CA
      └─ Outbound webhook client certs (for calling SMO)
```

#### mTLS Flows

**External (SMO → Gateway)**:
```
[SMO] ──TLS 1.3 (mTLS)──> [K8s Ingress] ──TLS passthrough──> [Gateway Pod]
  │                             │                                  │
  Client Cert            Passthrough/SNI               Go TLS 1.3 Server
  (external)                                           • Validate client cert
                                                       • Check CN against CA
                                                       • Authorize based on CN
```

**Internal (Gateway → Redis)**:
```
[Gateway Pod] ──TLS 1.3──> [Redis]
      │                       │
   Optional mTLS         Server Cert
   (cert-manager)        (cert-manager)

Note: Redis TLS optional but recommended
```

**Outbound (Controller → SMO Webhook)**:
```
[Subscription Controller] ──TLS 1.3──> [SMO Webhook Endpoint]
            │                                   │
       Client Cert                         Validate cert
    (webhook CA, optional)                 (SMO's CA)
```

### Secrets Management

**No Hardcoded Secrets - EVER**

All secrets via:
1. **Kubernetes Secrets** (for small secrets)
   ```yaml
   env:
     - name: REDIS_PASSWORD
       valueFrom:
         secretKeyRef:
           name: redis-credentials
           key: password
   ```

2. **cert-manager** (for certificates)
   - Automatic issuance
   - Auto-renewal before expiry
   - Rotation without downtime

3. **External Secrets Operator** (optional, for enterprise)
   - Sync from HashiCorp Vault
   - Sync from AWS Secrets Manager
   - Automatic rotation

**Secrets Lifecycle**:
```
1. Creation    → cert-manager or External Secrets Operator
2. Storage     → Kubernetes Secrets (etcd encrypted at rest)
3. Delivery    → Mounted as volume or environment variable
4. Rotation    → Automatic (cert-manager watches expiry)
5. Deletion    → Kubernetes Secret deletion
```

### Network Security

#### Network Policies

```yaml
# Restrict gateway pod ingress
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress
spec:
  podSelector:
    matchLabels:
      app: netweave-gateway
  ingress:
    # Allow from ingress controller
    - from:
      - namespaceSelector:
          matchLabels:
            name: ingress-nginx  # or your ingress controller namespace
      ports:
      - protocol: TCP
        port: 8443  # HTTPS port
```

```yaml
# Restrict gateway pod egress
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
spec:
  podSelector:
    matchLabels:
      app: netweave-gateway
  egress:
    # Only to K8s API server
    - to:
      - namespaceSelector:
          matchLabels:
            name: kube-system
      ports:
      - protocol: TCP
        port: 6443

    # Only to Redis
    - to:
      - podSelector:
          matchLabels:
            app: redis
      ports:
      - protocol: TCP
        port: 6379

    # Only to SMO webhooks (external)
    - to:
      - namespaceSelector: {}
      ports:
      - protocol: TCP
        port: 443
```

### Security Monitoring

**Audit Logging**:
- All API requests logged (structured)
- Authentication failures logged
- Authorization denials logged
- Sensitive data redacted

**Metrics**:
```
# Authentication metrics
o2ims_auth_total{status="success|failure"}
o2ims_auth_failures_by_client{client_cn="..."}

# Authorization metrics
o2ims_authz_total{resource="...",action="...",result="allow|deny"}

# TLS metrics
o2ims_tls_handshake_duration_seconds
o2ims_tls_cert_expiry_seconds
```

**Alerts**:
- Certificate expiring in < 7 days
- Repeated authentication failures
- Authorization denial spike
- TLS handshake failures

---

*Continued in next section...*
