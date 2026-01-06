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

### Adapter Architecture (Multi-Backend Support)

**Responsibility**: Provide pluggable backend abstraction for O2-IMS operations

The adapter architecture enables the gateway to support multiple infrastructure backends through a unified interface. This allows netweave to manage resources across Kubernetes, bare-metal systems (Dell DTIAS), cloud providers (AWS, Azure), and any future backend implementations.

#### Adapter Interface

All backend implementations must satisfy the `Adapter` interface:

```go
// internal/adapter/adapter.go

package adapter

// Adapter is the pluggable backend interface
type Adapter interface {
    // Metadata
    Name() string
    Version() string
    Capabilities() []Capability

    // Deployment Managers
    ListDeploymentManagers(ctx context.Context, filter *Filter) ([]*DeploymentManager, error)
    GetDeploymentManager(ctx context.Context, id string) (*DeploymentManager, error)

    // Resource Pools
    ListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error)
    GetResourcePool(ctx context.Context, id string) (*ResourcePool, error)
    CreateResourcePool(ctx context.Context, pool *ResourcePool) (*ResourcePool, error)
    UpdateResourcePool(ctx context.Context, id string, pool *ResourcePool) (*ResourcePool, error)
    DeleteResourcePool(ctx context.Context, id string) error

    // Resources
    ListResources(ctx context.Context, filter *Filter) ([]*Resource, error)
    GetResource(ctx context.Context, id string) (*Resource, error)
    CreateResource(ctx context.Context, resource *Resource) (*Resource, error)
    DeleteResource(ctx context.Context, id string) error

    // Resource Types
    ListResourceTypes(ctx context.Context, filter *Filter) ([]*ResourceType, error)
    GetResourceType(ctx context.Context, id string) (*ResourceType, error)

    // Subscriptions (backend may or may not support)
    SupportsSubscriptions() bool
    Subscribe(ctx context.Context, sub *Subscription) error
    Unsubscribe(ctx context.Context, id string) error

    // Health and lifecycle
    Health(ctx context.Context) error
    Close() error
}

// Capability describes what operations a backend supports
type Capability string

const (
    CapResourcePoolCreate   Capability = "resource-pool-create"
    CapResourcePoolUpdate   Capability = "resource-pool-update"
    CapResourcePoolDelete   Capability = "resource-pool-delete"
    CapResourceCreate       Capability = "resource-create"
    CapResourceDelete       Capability = "resource-delete"
    CapSubscriptions        Capability = "subscriptions"
    CapRealTimeEvents       Capability = "real-time-events"
)
```

#### Adapter Registry

The `Registry` manages multiple adapter instances and routes requests to appropriate backends:

```go
// internal/adapter/registry.go

type Registry struct {
    mu       sync.RWMutex
    adapters map[string]Adapter
    routes   map[string]RoutingRule
    default  string  // Default adapter name
}

// RoutingRule determines which backend to use
type RoutingRule struct {
    ResourceType string      // "ResourcePool", "Resource", etc.
    Filter       *Filter     // Optional filter criteria
    AdapterName  string      // Which adapter to route to
    Priority     int         // For fallback scenarios
}

// Route determines which adapter to use for a request
func (r *Registry) Route(resourceType string, filter *Filter) (Adapter, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Check routing rules first (highest priority first)
    for _, rule := range r.sortedRules() {
        if rule.ResourceType == resourceType && rule.MatchesFilter(filter) {
            if adapter, ok := r.adapters[rule.AdapterName]; ok {
                return adapter, nil
            }
        }
    }

    // Fallback to default adapter
    if adapter, ok := r.adapters[r.default]; ok {
        return adapter, nil
    }

    return nil, fmt.Errorf("no adapter found for resource type %s", resourceType)
}
```

#### Backend Implementations

**Directory Structure:**

```
internal/adapters/
├── k8s/               # Kubernetes backend (primary)
│   ├── adapter.go
│   ├── resourcepools.go
│   ├── resources.go
│   ├── resourcetypes.go
│   └── client.go
├── dtias/             # Dell DTIAS backend
│   ├── adapter.go
│   ├── resourcepools.go
│   ├── resources.go
│   └── client.go
├── aws/               # AWS EKS/EC2 backend
│   ├── adapter.go
│   └── ...
├── openstack/         # OpenStack backend
│   ├── adapter.go
│   └── ...
└── mock/              # Mock for testing
    ├── adapter.go
    └── ...
```

#### Kubernetes Adapter (Primary Implementation)

```go
// internal/adapters/k8s/adapter.go

package k8s

import (
    "context"
    "github.com/yourorg/netweave/internal/adapter"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesAdapter struct {
    client    client.Client
    clientset *kubernetes.Clientset
    config    *Config
}

func NewAdapter(config *Config) (*KubernetesAdapter, error) {
    client, err := createK8sClient(config)
    if err != nil {
        return nil, err
    }

    return &KubernetesAdapter{
        client: client,
        config: config,
    }, nil
}

func (a *KubernetesAdapter) Name() string {
    return "kubernetes"
}

func (a *KubernetesAdapter) Capabilities() []adapter.Capability {
    return []adapter.Capability{
        adapter.CapResourcePoolCreate,
        adapter.CapResourcePoolUpdate,
        adapter.CapResourcePoolDelete,
        adapter.CapResourceCreate,
        adapter.CapResourceDelete,
        adapter.CapSubscriptions,
        adapter.CapRealTimeEvents,
    }
}

// Example: List Resource Pools → List MachineSets
func (a *KubernetesAdapter) ListResourcePools(
    ctx context.Context,
    filter *adapter.Filter,
) ([]*adapter.ResourcePool, error) {
    // 1. List Kubernetes MachineSets
    machineSets := &machinev1beta1.MachineSetList{}
    if err := a.client.List(ctx, machineSets); err != nil {
        return nil, fmt.Errorf("failed to list machinesets: %w", err)
    }

    // 2. Transform to O2-IMS ResourcePool
    pools := make([]*adapter.ResourcePool, 0, len(machineSets.Items))
    for i := range machineSets.Items {
        pool := a.transformMachineSetToResourcePool(&machineSets.Items[i])
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

// Transform MachineSet → O2-IMS ResourcePool
func (a *KubernetesAdapter) transformMachineSetToResourcePool(ms *machinev1beta1.MachineSet) *adapter.ResourcePool {
    return &adapter.ResourcePool{
        ResourcePoolID: string(ms.UID),
        Name:          ms.Name,
        Description:   ms.Annotations["description"],
        Location:      ms.Spec.Template.Spec.ProviderSpec.Value.Zone,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "k8s.machineset.name":       ms.Name,
            "k8s.machineset.namespace":  ms.Namespace,
            "k8s.machineset.replicas":   *ms.Spec.Replicas,
            "k8s.instanceType":          ms.Spec.Template.Spec.ProviderSpec.Value.InstanceType,
        },
    }
}
```

#### Dell DTIAS Adapter Example

```go
// internal/adapters/dtias/adapter.go

package dtias

import (
    "context"
    "github.com/yourorg/netweave/internal/adapter"
    "github.com/yourorg/netweave/pkg/dtias-client"
)

type DTIASAdapter struct {
    client *dtias.Client
    config *Config
}

func NewAdapter(config *Config) (*DTIASAdapter, error) {
    client, err := dtias.NewClient(config.Endpoint, config.APIKey)
    if err != nil {
        return nil, err
    }

    return &DTIASAdapter{
        client: client,
        config: config,
    }, nil
}

func (a *DTIASAdapter) Name() string {
    return "dtias"
}

func (a *DTIASAdapter) Capabilities() []adapter.Capability {
    return []adapter.Capability{
        adapter.CapResourcePoolCreate,
        adapter.CapResourceCreate,
        // DTIAS doesn't support subscriptions
    }
}

func (a *DTIASAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
    // 1. Call DTIAS API
    dtiasGroups, err := a.client.ListResourceGroups(ctx)
    if err != nil {
        return nil, fmt.Errorf("dtias api error: %w", err)
    }

    // 2. Transform DTIAS ResourceGroup → O2-IMS ResourcePool
    pools := make([]*adapter.ResourcePool, 0, len(dtiasGroups))
    for _, group := range dtiasGroups {
        pool := a.transformToDTIASResourcePool(group)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

// Transform DTIAS ResourceGroup → O2-IMS ResourcePool
func (a *DTIASAdapter) transformToDTIASResourcePool(group *dtias.ResourceGroup) *adapter.ResourcePool {
    return &adapter.ResourcePool{
        ResourcePoolID: group.ID,
        Name:          group.Name,
        Description:   group.Description,
        Location:      group.DataCenter,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "dtias.resourceGroupType": group.Type,
            "dtias.provisioningState": group.State,
            "dtias.bareMetalCount":    group.ServerCount,
        },
    }
}
```

#### Configuration-Driven Routing

Backend selection and routing are configured via YAML:

```yaml
# config/gateway.yaml

adapters:
  # Kubernetes adapter (default)
  - name: kubernetes
    type: k8s
    enabled: true
    default: true
    config:
      kubeconfig: /etc/kubernetes/admin.conf
      namespace: default
      ocloudId: ocloud-kubernetes-1

  # Dell DTIAS adapter
  - name: dtias
    type: dtias
    enabled: true
    config:
      endpoint: https://dtias.dell.com/api
      apiKey: ${DTIAS_API_KEY}
      timeout: 30s
      ocloudId: ocloud-dtias-1

  # AWS adapter (disabled for now)
  - name: aws
    type: aws
    enabled: false
    config:
      region: us-east-1
      credentials: ~/.aws/credentials
      ocloudId: ocloud-aws-1

# Routing rules: which backend for which resource type
routing:
  rules:
    # All bare-metal resource pools go to DTIAS
    - resourceType: ResourcePool
      filter:
        extensions.type: "bare-metal"
      adapter: dtias
      priority: 10

    # Cloud resource pools go to AWS
    - resourceType: ResourcePool
      filter:
        extensions.type: "cloud"
      adapter: aws
      priority: 10

    # GPU resources go to Kubernetes
    - resourceType: Resource
      filter:
        extensions.hasGPU: true
      adapter: kubernetes
      priority: 10

    # Everything else goes to Kubernetes (default)
    - resourceType: "*"
      adapter: kubernetes
      priority: 1
```

#### Multi-Backend Aggregation

For scenarios requiring results from multiple backends:

```go
// internal/adapter/aggregator.go

type AggregatingAdapter struct {
    adapters []Adapter
    strategy AggregationStrategy
}

type AggregationStrategy string

const (
    StrategyMerge    AggregationStrategy = "merge"     // Combine all results
    StrategyFirst    AggregationStrategy = "first"     // First successful response
    StrategyFallback AggregationStrategy = "fallback"  // Try in order until success
)

func (a *AggregatingAdapter) ListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error) {
    switch a.strategy {
    case StrategyMerge:
        return a.mergeListResourcePools(ctx, filter)
    case StrategyFirst:
        return a.firstListResourcePools(ctx, filter)
    case StrategyFallback:
        return a.fallbackListResourcePools(ctx, filter)
    }
}

// Merge results from all backends
func (a *AggregatingAdapter) mergeListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error) {
    var allPools []*ResourcePool

    // Query all adapters in parallel
    results := make(chan []*ResourcePool, len(a.adapters))
    errors := make(chan error, len(a.adapters))

    for _, adapter := range a.adapters {
        go func(adp Adapter) {
            pools, err := adp.ListResourcePools(ctx, filter)
            if err != nil {
                errors <- err
                return
            }
            results <- pools
        }(adapter)
    }

    // Collect results
    for i := 0; i < len(a.adapters); i++ {
        select {
        case pools := <-results:
            allPools = append(allPools, pools...)
        case err := <-errors:
            // Log error but continue (partial results OK)
            log.Warn("adapter failed", "error", err)
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }

    return allPools, nil
}
```

#### Benefits of Adapter Architecture

1. **Easy Extension**: Add new backends by implementing `Adapter` interface
2. **Hot Swappable**: Change backends via configuration without code changes
3. **Intelligent Routing**: Route requests based on resource type, filters, or capabilities
4. **Multi-Backend Aggregation**: Combine results from multiple sources
5. **Testability**: Use mock adapter for testing without real infrastructure
6. **Vendor Independence**: Abstract vendor-specific APIs behind standard interface

**Adapter Mappings** (see [api-mapping.md](api-mapping.md) for details):
- DeploymentManager → Cluster metadata (Kubernetes ConfigMap or CRD)
- ResourcePool → MachineSet/NodePool (K8s), ResourceGroup (DTIAS), ASG (AWS)
- Resource → Node/Machine (K8s), Server (DTIAS), EC2 Instance (AWS)
- ResourceType → StorageClass/Machine flavors (K8s), Server Types (DTIAS), Instance Types (AWS)

### API Versioning Strategy

**Responsibility**: Provide stable, evolvable O2-IMS API with backwards compatibility

The gateway supports multiple API versions simultaneously, allowing clients to upgrade at their own pace while enabling new features and improvements without breaking existing integrations.

#### Version URL Structure

```
/o2ims/v1/resourcePools       # API v1 (current, stable)
/o2ims/v2/resourcePools       # API v2 (future, with enhancements)
/o2ims/v3/resourcePools       # API v3 (future)
```

#### Router Configuration

```go
// internal/server/router.go

func (s *Server) setupRoutes() {
    // API v1 (current stable version)
    v1 := s.router.Group("/o2ims/v1")
    {
        v1.Use(s.authMiddleware())
        v1.Use(s.metricsMiddleware())

        // Deployment Managers
        v1.GET("/deploymentManagers", s.handleListDeploymentManagersV1)
        v1.GET("/deploymentManagers/:id", s.handleGetDeploymentManagerV1)

        // Resource Pools
        v1.GET("/resourcePools", s.handleListResourcePoolsV1)
        v1.GET("/resourcePools/:id", s.handleGetResourcePoolV1)
        v1.POST("/resourcePools", s.handleCreateResourcePoolV1)
        v1.PUT("/resourcePools/:id", s.handleUpdateResourcePoolV1)
        v1.DELETE("/resourcePools/:id", s.handleDeleteResourcePoolV1)

        // Resources
        v1.GET("/resources", s.handleListResourcesV1)
        v1.GET("/resources/:id", s.handleGetResourceV1)
        v1.POST("/resources", s.handleCreateResourceV1)
        v1.DELETE("/resources/:id", s.handleDeleteResourceV1)

        // Resource Types
        v1.GET("/resourceTypes", s.handleListResourceTypesV1)
        v1.GET("/resourceTypes/:id", s.handleGetResourceTypeV1)

        // Subscriptions
        v1.GET("/subscriptions", s.handleListSubscriptionsV1)
        v1.GET("/subscriptions/:id", s.handleGetSubscriptionV1)
        v1.POST("/subscriptions", s.handleCreateSubscriptionV1)
        v1.PUT("/subscriptions/:id", s.handleUpdateSubscriptionV1)
        v1.DELETE("/subscriptions/:id", s.handleDeleteSubscriptionV1)
    }

    // API v2 (future - enhanced features)
    v2 := s.router.Group("/o2ims/v2")
    {
        v2.Use(s.authMiddleware())
        v2.Use(s.metricsMiddleware())

        // Enhanced Resource Pools with additional fields
        v2.GET("/resourcePools", s.handleListResourcePoolsV2)
        v2.GET("/resourcePools/:id", s.handleGetResourcePoolV2)
        v2.POST("/resourcePools", s.handleCreateResourcePoolV2)
        v2.PUT("/resourcePools/:id", s.handleUpdateResourcePoolV2)
        v2.DELETE("/resourcePools/:id", s.handleDeleteResourcePoolV2)

        // New endpoints in v2
        v2.GET("/resourcePools/:id/metrics", s.handleGetResourcePoolMetrics)
        v2.GET("/resourcePools/:id/events", s.handleGetResourcePoolEvents)

        // Enhanced filtering and pagination
        v2.GET("/resources", s.handleListResourcesV2)
    }
}
```

#### Version-Specific Handlers

```go
// V1 handler - original implementation
func (s *Server) handleListResourcePoolsV1(c *gin.Context) {
    // Parse v1 query parameters
    filter := parseFilterV1(c)

    // Route to appropriate backend
    adapter, err := s.registry.Route("ResourcePool", filter)
    if err != nil {
        c.JSON(500, gin.H{"error": "adapter routing failed"})
        return
    }

    pools, err := adapter.ListResourcePools(c.Request.Context(), filter)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    // Return v1 response format
    c.JSON(200, marshalResourcePoolsV1(pools))
}

// V2 handler - enhanced with new fields and capabilities
func (s *Server) handleListResourcePoolsV2(c *gin.Context) {
    // Parse v2 query parameters (enhanced filtering)
    filter := parseFilterV2(c)

    // Route to appropriate backend
    adapter, err := s.registry.Route("ResourcePool", filter)
    if err != nil {
        c.JSON(500, gin.H{"error": "adapter routing failed"})
        return
    }

    pools, err := adapter.ListResourcePools(c.Request.Context(), filter)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    // Return v2 response format (with additional fields)
    c.JSON(200, marshalResourcePoolsV2(pools))
}
```

#### Response Format Evolution

**V1 Response** (current):
```json
{
  "items": [
    {
      "resourcePoolId": "pool-123",
      "name": "Compute Pool",
      "description": "High-performance compute",
      "location": "us-east-1a",
      "oCloudId": "ocloud-1"
    }
  ]
}
```

**V2 Response** (enhanced):
```json
{
  "items": [
    {
      "resourcePoolId": "pool-123",
      "name": "Compute Pool",
      "description": "High-performance compute",
      "location": "us-east-1a",
      "oCloudId": "ocloud-1",
      "health": {
        "status": "healthy",
        "availableCapacity": 80,
        "utilization": 65.5
      },
      "metrics": {
        "cpuUsagePercent": 45.2,
        "memoryUsagePercent": 72.1,
        "nodeCount": 10,
        "healthyNodeCount": 10
      },
      "tags": ["production", "gpu-enabled"],
      "createdAt": "2026-01-01T00:00:00Z",
      "updatedAt": "2026-01-06T10:30:00Z"
    }
  ],
  "pagination": {
    "totalCount": 42,
    "pageSize": 20,
    "nextPage": "/o2ims/v2/resourcePools?page=2"
  }
}
```

#### Version Negotiation

Clients specify their desired API version via URL path:

```bash
# Client uses v1 (stable)
curl https://netweave.example.com/o2ims/v1/resourcePools \
  --cert client.crt --key client.key --cacert ca.crt

# Client uses v2 (enhanced)
curl https://netweave.example.com/o2ims/v2/resourcePools \
  --cert client.crt --key client.key --cacert ca.crt
```

#### Deprecation Policy

1. **Announce Deprecation**: At least 6 months before removal
2. **Mark as Deprecated**: Add `X-API-Deprecated: true` header to responses
3. **Provide Migration Guide**: Document changes and migration path
4. **Grace Period**: Minimum 12 months from deprecation announcement
5. **Final Removal**: Remove deprecated version after grace period

Example deprecation header:

```http
HTTP/1.1 200 OK
X-API-Deprecated: true
X-API-Deprecation-Date: 2026-07-01
X-API-Sunset-Date: 2027-01-01
X-API-Migration-Guide: https://docs.netweave.io/migration/v1-to-v2
Content-Type: application/json
```

#### Version Support Matrix

| Version | Status | Release Date | Deprecation Date | Sunset Date |
|---------|--------|--------------|------------------|-------------|
| v1 | Stable | 2026-01-01 | - | - |
| v2 | Planned | 2026-07-01 | - | - |
| v3 | Future | TBD | - | - |

#### Benefits of Versioning Strategy

1. **Backwards Compatibility**: Existing clients continue working without changes
2. **Incremental Adoption**: Clients upgrade at their own pace
3. **Innovation**: New features can be added without breaking existing APIs
4. **Clear Migration Path**: Documented upgrade process for each version
5. **Production Stability**: No surprise breaking changes

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

### RBAC and Multi-Tenancy

**Responsibility**: Secure multi-tenant access control and resource isolation

**Implementation**: Built-in from the start

netweave is designed as an **enterprise multi-tenant platform** with comprehensive RBAC from day one. This enables multiple SMO systems (tenants) to securely share the same gateway while maintaining strict resource isolation.

#### Multi-Tenancy Architecture

**Tenant Model**:
```
┌─────────────────────────────────────────────────────────────┐
│                    netweave O2-IMS Gateway                  │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  Tenant A    │  │  Tenant B    │  │  Tenant C    │     │
│  │  (SMO-Alpha) │  │  (SMO-Beta)  │  │  (SMO-Gamma) │     │
│  │              │  │              │  │              │     │
│  │ • Users      │  │ • Users      │  │ • Users      │     │
│  │ • Roles      │  │ • Roles      │  │ • Roles      │     │
│  │ • Resources  │  │ • Resources  │  │ • Resources  │     │
│  │ • Quotas     │  │ • Quotas     │  │ • Quotas     │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Tenant Identification** (via client certificate):
```go
// Extract tenant from client certificate CN
// CN format: "user-id.tenant-id.o2ims.example.com"
func extractTenantFromCert(cert *x509.Certificate) (string, string, error) {
    cn := cert.Subject.CommonName
    parts := strings.Split(cn, ".")

    if len(parts) < 2 {
        return "", "", fmt.Errorf("invalid CN format")
    }

    return parts[0], parts[1], nil // userID, tenantID
}
```

**Tenant Middleware**:
```go
// Extracts and validates tenant for every request
func (m *TenantMiddleware) ExtractTenant() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Extract tenant ID from certificate CN
        tenantID := extractTenantFromCert(c.Request.TLS.PeerCertificates[0])

        // 2. Load and validate tenant
        tenant, err := m.store.Get(c.Request.Context(), tenantID)
        if err != nil || tenant.Status != "active" {
            c.AbortWithStatusJSON(403, gin.H{"error": "invalid tenant"})
            return
        }

        // 3. Store in context
        c.Set("tenant", tenant)
        c.Set("tenantId", tenantID)
        c.Next()
    }
}
```

#### RBAC Model

**Role Hierarchy**:
```
System Roles (cross-tenant):
├─ PlatformAdmin   - Full system access
├─ TenantAdmin     - Create/manage tenants
└─ Auditor         - Read-only audit access

Tenant Roles (scoped to specific tenant):
├─ Owner           - Full tenant access
├─ Admin           - Manage users, resources, policies
├─ Operator        - CRUD on resources
├─ Viewer          - Read-only access
└─ Custom Roles    - User-defined permissions
```

**Permission Model**:
```go
type Permission struct {
    Resource string   // "ResourcePool", "Resource", "Subscription"
    Action   Action   // "create", "read", "update", "delete", "list", "manage"
    Scope    Scope    // "tenant", "shared", "all"
}

// Example: Operator role permissions
operatorRole := &Role{
    Name: "Operator",
    Permissions: []Permission{
        {Resource: "ResourcePool", Action: "manage", Scope: "tenant"},
        {Resource: "Resource", Action: "manage", Scope: "tenant"},
        {Resource: "Subscription", Action: "manage", Scope: "tenant"},
    },
}
```

**Authorization Enforcement**:
```go
// Every API endpoint is protected
v1.POST("/resourcePools",
    tenantMiddleware.ExtractTenant(),
    authzMiddleware.RequirePermission("ResourcePool", "create"),
    handleCreateResourcePool)

// Authorization check
func (a *Authorizer) Authorize(
    userID, tenantID, resource string,
    action Action,
) (bool, error) {
    // 1. Get user's role bindings
    bindings := a.getUserBindings(userID, tenantID)

    // 2. Collect permissions from all roles
    permissions := a.collectPermissions(bindings)

    // 3. Check if any permission allows the action
    return a.matchPermission(permissions, resource, action, tenantID)
}
```

#### Tenant Isolation

**Resource Filtering**:
All list operations automatically filter by tenant:

```go
func (h *Handler) ListResourcePools(c *gin.Context) {
    tenantID := c.GetString("tenantId")

    // Tenant filter is ALWAYS applied
    filter := &adapter.Filter{
        TenantID: tenantID,  // CRITICAL: prevent cross-tenant access
    }

    pools, _ := adapter.ListResourcePools(ctx, filter)
    c.JSON(200, pools)
}
```

**Kubernetes Label Strategy**:
All Kubernetes resources MUST be labeled with tenant ID:

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: production-pool
  labels:
    # Tenant isolation label (REQUIRED)
    o2ims.oran.org/tenant: smo-alpha

    # O2-IMS resource labels
    o2ims.oran.org/resource-pool-id: pool-123
spec:
  replicas: 5
  template:
    metadata:
      labels:
        o2ims.oran.org/tenant: smo-alpha
```

**Backend Enforcement**:
```go
// Kubernetes adapter filters by tenant label
func (a *KubernetesAdapter) ListResourcePools(
    ctx context.Context,
    filter *adapter.Filter,
) ([]*ResourcePool, error) {
    // Label selector for tenant isolation
    listOpts := &client.ListOptions{
        LabelSelector: labels.SelectorFromSet(labels.Set{
            "o2ims.oran.org/tenant": filter.TenantID,
        }),
    }

    machineSets := &machinev1beta1.MachineSetList{}
    a.client.List(ctx, machineSets, listOpts)

    // Double-check tenant isolation
    for _, pool := range pools {
        if pool.TenantID != filter.TenantID {
            continue // Skip other tenant's resources
        }
    }

    return pools, nil
}
```

#### Tenant Quotas

**Resource Limits**:
```go
type ResourceQuotas struct {
    MaxResourcePools     int `json:"maxResourcePools"`
    MaxResources         int `json:"maxResources"`
    MaxSubscriptions     int `json:"maxSubscriptions"`
    MaxCPUCores          int `json:"maxCpuCores"`
    MaxMemoryGB          int `json:"maxMemoryGb"`
}

// Enforce quota before creation
func (h *Handler) CreateResourcePool(c *gin.Context) {
    tenant := c.MustGet("tenant").(*tenant.Tenant)

    // Check current usage
    current := h.quotaManager.GetUsage(ctx, tenant.TenantID)
    if current.ResourcePools >= tenant.Quotas.MaxResourcePools {
        c.JSON(429, gin.H{"error": "resource pool quota exceeded"})
        return
    }

    // Proceed with creation
    pool, _ := adapter.CreateResourcePool(ctx, pool)
    c.JSON(201, pool)
}
```

#### Audit Logging

**Every operation is logged with tenant context**:
```go
type AuditEntry struct {
    Timestamp   time.Time `json:"timestamp"`
    TenantID    string    `json:"tenantId"`
    UserID      string    `json:"userId"`
    Action      string    `json:"action"`
    Resource    string    `json:"resource"`
    ResourceID  string    `json:"resourceId,omitempty"`
    Result      string    `json:"result"` // "success", "denied", "error"
    IPAddress   string    `json:"ipAddress"`
}

// Audit middleware logs all operations
func (m *AuditMiddleware) LogOperation() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next() // Process request

        m.logger.LogEntry(&AuditEntry{
            Timestamp:  start,
            TenantID:   c.GetString("tenantId"),
            UserID:     c.GetString("userId"),
            Action:     c.Request.Method,
            Resource:   c.Request.URL.Path,
            Result:     getResult(c.Writer.Status()),
            IPAddress:  c.ClientIP(),
        })
    }
}
```

#### API Design for Multi-Tenancy

**Admin API** (platform-level, requires system role):
```
POST   /admin/v1/tenants                    # Create tenant
GET    /admin/v1/tenants                    # List all tenants
GET    /admin/v1/tenants/:id                # Get tenant
PUT    /admin/v1/tenants/:id                # Update tenant
DELETE /admin/v1/tenants/:id                # Delete tenant

GET    /admin/v1/tenants/:id/users          # List tenant users
POST   /admin/v1/tenants/:id/users          # Create user
GET    /admin/v1/audit                      # Query audit logs
```

**O2-IMS API** (automatically tenant-scoped):
```
# All requests automatically scoped to authenticated tenant
GET    /o2ims/v1/resourcePools              # List (tenant's only)
POST   /o2ims/v1/resourcePools              # Create (in tenant)
GET    /o2ims/v1/resourcePools/:id          # Get (tenant check)
DELETE /o2ims/v1/resourcePools/:id          # Delete (tenant check)
```

#### Security Considerations

**Defense in Depth**:
1. ✅ Tenant filtering at API layer (middleware)
2. ✅ Tenant filtering at adapter layer (backend queries)
3. ✅ Kubernetes RBAC for backend isolation
4. ✅ Network policies for pod-level isolation

**Threat Mitigation**:
- **Cross-Tenant Data Access**: Label-based filtering + tenant verification on all ops
- **Privilege Escalation**: Immutable system roles, role binding validation
- **Resource Exhaustion**: Per-tenant quotas, rate limiting
- **Audit Trail**: All operations logged with tenant context

**For complete RBAC and multi-tenancy documentation, see [docs/rbac-multitenancy.md](rbac-multitenancy.md).**

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
