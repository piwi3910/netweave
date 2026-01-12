# O2-IMS to Kubernetes API Mapping

**Version:** 1.1
**Date:** 2026-01-09

This document defines how O-RAN O2-IMS resources map to Kubernetes resources in the netweave gateway.

## Table of Contents

1. [Overview](#overview)
2. [Multi-Tenancy and RBAC](#multi-tenancy-and-rbac)
3. [Multi-Backend Adapter Routing](#multi-backend-adapter-routing)
4. [API Versioning and Evolution](#api-versioning-and-evolution)
5. [Deployment Manager](#deployment-manager)
6. [Resource Pools](#resource-pools)
7. [Resources](#resources)
8. [Resource Types](#resource-types)
9. [Subscriptions](#subscriptions)
10. [Data Transformation Examples](#data-transformation-examples)
11. [Backend-Specific Mappings](#backend-specific-mappings)

---

## Overview

### Mapping Philosophy

**Goals**:
1. **Semantic Correctness**: O2-IMS concepts accurately represent Kubernetes reality
2. **Bidirectional**: Both read (GET/LIST) and write (POST/PUT/DELETE) operations supported
3. **Idempotent**: Operations can be safely repeated
4. **Kubernetes-Native**: Leverage existing K8s resources where possible

**Approach**:
- **Direct Mapping**: Where O2-IMS and K8s concepts align (e.g., Node → Resource)
- **Aggregation**: Where O2-IMS requires combining multiple K8s resources
- **Custom Resources**: Where no direct K8s equivalent exists (e.g., DeploymentManager)

### Supported O2-IMS Resources

| O2-IMS Resource | K8s Primary Resource | Status | CRUD Support |
|-----------------|----------------------|--------|--------------|
| Deployment Manager | Custom (cluster metadata) | ✅ Full | R |
| Resource Pool | MachineSet / NodePool | ✅ Full | CRUD |
| Resource | Node / Machine | ✅ Full | CRUD |
| Resource Type | StorageClass, Machine Types | ✅ Full | R |
| Subscription | Redis (O2-IMS specific) | ✅ Full | CRUD |

---

## Multi-Tenancy and RBAC

### Overview

The netweave gateway provides multi-tenancy and Role-Based Access Control (RBAC) to enable secure, isolated access to O2-IMS resources across different organizations or teams.

### Key Concepts

| Concept | Description |
|---------|-------------|
| **Tenant** | Isolated organization with its own users, quotas, and resources |
| **User** | Entity authenticated via mTLS certificate, belonging to a tenant |
| **Role** | Collection of permissions (platform-level or tenant-scoped) |
| **Permission** | Granular access right (e.g., `subscription:create`, `resource:read`) |

### Authentication

Authentication is performed via mTLS client certificates. The certificate's Subject DN is used to identify users:

```
CN=john.doe,O=Tenant A,OU=Engineering
```

The middleware extracts user identity from the `X-Forwarded-Client-Cert` (XFCC) header when behind a proxy, or directly from the TLS connection.

### Tenant Management API

#### List Tenants (Platform Admin Only)

```http
GET /admin/tenants HTTP/1.1
Accept: application/json
```

**Response:**
```json
{
  "tenants": [
    {
      "id": "tenant-123",
      "name": "Acme Corporation",
      "description": "Primary production tenant",
      "status": "active",
      "contactEmail": "admin@acme.com",
      "quota": {
        "maxSubscriptions": 100,
        "maxResourcePools": 50,
        "maxDeployments": 200,
        "maxUsers": 25
      },
      "usage": {
        "subscriptions": 45,
        "resourcePools": 12,
        "deployments": 78,
        "users": 15
      },
      "createdAt": "2026-01-01T00:00:00Z",
      "updatedAt": "2026-01-05T12:30:00Z"
    }
  ],
  "total": 1
}
```

#### Create Tenant (Platform Admin Only)

```http
POST /admin/tenants HTTP/1.1
Content-Type: application/json

{
  "name": "New Tenant",
  "description": "Description of the tenant",
  "contactEmail": "admin@newtenant.com",
  "quota": {
    "maxSubscriptions": 50,
    "maxResourcePools": 25,
    "maxDeployments": 100,
    "maxUsers": 10
  }
}
```

**Response:** `201 Created`
```json
{
  "id": "tenant-456",
  "name": "New Tenant",
  "status": "active",
  ...
}
```

#### Get Tenant

```http
GET /admin/tenants/{tenantId} HTTP/1.1
```

#### Update Tenant (Platform Admin Only)

```http
PUT /admin/tenants/{tenantId} HTTP/1.1
Content-Type: application/json

{
  "name": "Updated Name",
  "description": "Updated description",
  "status": "suspended",
  "quota": {
    "maxSubscriptions": 200
  }
}
```

#### Delete Tenant (Platform Admin Only)

```http
DELETE /admin/tenants/{tenantId} HTTP/1.1
```

**Note:** Tenants with active resources are marked as `pending_deletion` and resources must be cleaned up first.

### User Management API

#### List Users (Tenant Admin)

```http
GET /tenant/users HTTP/1.1
Accept: application/json
```

**Response:**
```json
{
  "users": [
    {
      "id": "user-123",
      "tenantId": "tenant-456",
      "subject": "CN=john.doe,O=Acme,OU=Engineering",
      "commonName": "john.doe",
      "email": "john.doe@acme.com",
      "roleId": "role-tenant-admin",
      "isActive": true,
      "createdAt": "2026-01-01T00:00:00Z",
      "lastLoginAt": "2026-01-06T09:15:00Z"
    }
  ],
  "total": 1
}
```

#### Create User (Tenant Admin)

```http
POST /tenant/users HTTP/1.1
Content-Type: application/json

{
  "subject": "CN=jane.smith,O=Acme,OU=Operations",
  "commonName": "jane.smith",
  "email": "jane.smith@acme.com",
  "roleId": "role-tenant-viewer"
}
```

#### Update User (Tenant Admin)

```http
PUT /tenant/users/{userId} HTTP/1.1
Content-Type: application/json

{
  "email": "new.email@acme.com",
  "roleId": "role-tenant-operator",
  "isActive": false
}
```

#### Delete User (Tenant Admin)

```http
DELETE /tenant/users/{userId} HTTP/1.1
```

### Role System

#### Platform-Level Roles

| Role | Permissions | Description |
|------|-------------|-------------|
| `platform_admin` | All permissions | Full system administration |
| `platform_operator` | Tenant read, some write | Platform operations |
| `platform_viewer` | Read-only platform access | Monitoring and auditing |

#### Tenant-Scoped Roles

| Role | Permissions | Description |
|------|-------------|-------------|
| `tenant_admin` | Full tenant management | Manage users, quotas, resources within tenant |
| `tenant_operator` | Create/manage resources | Day-to-day operations |
| `tenant_viewer` | Read-only tenant access | View resources and status |

#### List Available Roles

```http
GET /roles HTTP/1.1
Accept: application/json
```

**Response:**
```json
{
  "roles": [
    {
      "id": "role-tenant-admin",
      "name": "tenant_admin",
      "description": "Full tenant management",
      "type": "tenant",
      "permissions": [
        "tenant:read",
        "tenant:update",
        "user:create",
        "user:read",
        "user:update",
        "user:delete",
        "subscription:create",
        "subscription:read",
        "subscription:delete",
        "resource:read"
      ]
    }
  ]
}
```

### Permissions

| Permission | Description |
|------------|-------------|
| `tenant:create` | Create new tenants (platform admin) |
| `tenant:read` | View tenant information |
| `tenant:update` | Modify tenant settings |
| `tenant:delete` | Delete tenants |
| `user:create` | Create users within tenant |
| `user:read` | View user information |
| `user:update` | Modify user settings |
| `user:delete` | Remove users |
| `subscription:create` | Create subscriptions |
| `subscription:read` | View subscriptions |
| `subscription:delete` | Remove subscriptions |
| `resource:read` | View resources |
| `resource:create` | Create resources |
| `resource:delete` | Delete resources |
| `audit:read` | View audit logs |

### Audit Logging

All authentication and authorization events are logged for compliance:

```http
GET /admin/audit?limit=100&offset=0 HTTP/1.1
```

**Response:**
```json
{
  "events": [
    {
      "id": "event-123",
      "type": "authentication_success",
      "tenantId": "tenant-456",
      "userId": "user-789",
      "subject": "CN=john.doe,O=Acme",
      "action": "user_authenticated",
      "details": {
        "method": "mtls",
        "path": "/o2ims/v1/subscriptions"
      },
      "clientIp": "10.0.1.100",
      "userAgent": "curl/7.68.0",
      "timestamp": "2026-01-06T10:30:00Z"
    }
  ],
  "total": 1
}
```

**Event Types:**
- `authentication_success` - Successful authentication
- `authentication_failure` - Failed authentication attempt
- `authorization_failure` - Permission denied
- `tenant_created` - New tenant created
- `tenant_updated` - Tenant modified
- `tenant_deleted` - Tenant removed
- `user_created` - New user created
- `user_updated` - User modified
- `user_deleted` - User removed

### Quota Enforcement

Tenants have configurable quotas that are enforced atomically:

| Resource | Default Limit | Description |
|----------|---------------|-------------|
| `maxSubscriptions` | 100 | Maximum O2-IMS subscriptions |
| `maxResourcePools` | 50 | Maximum resource pools |
| `maxDeployments` | 200 | Maximum deployments |
| `maxUsers` | 25 | Maximum users per tenant |

When a quota is exceeded, the API returns:
```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{
  "error": "QuotaExceeded",
  "message": "User quota exceeded for tenant",
  "code": 403
}
```

### Tenant Isolation

All O2-IMS resources are isolated by tenant:

1. **Subscriptions**: Each subscription belongs to a tenant
2. **Resources**: Resources are scoped to tenant's resource pools
3. **Audit Logs**: Tenants can only view their own audit events
4. **Users**: Users can only access their own tenant's data

**Example: Tenant-scoped subscription query**
```http
GET /o2ims/v1/subscriptions HTTP/1.1
X-Tenant-ID: tenant-456
```

Returns only subscriptions belonging to `tenant-456`.

---

## Multi-Backend Adapter Routing

### Architecture Overview

The netweave gateway uses a **pluggable adapter pattern** that allows routing O2-IMS requests to different backend systems based on configuration rules. This enables:

1. **Multi-Technology Support**: Kubernetes, Dell DTIAS, AWS, OpenStack, etc.
2. **Hybrid Deployments**: Different resource pools can be managed by different backends
3. **Migration Paths**: Gradually move from legacy systems to Kubernetes
4. **Vendor Flexibility**: Avoid lock-in by supporting multiple infrastructure providers

### Adapter Interface

All backends implement the same `Adapter` interface:

```go
// internal/adapter/adapter.go
package adapter

type Adapter interface {
    // Metadata
    Name() string                    // "kubernetes", "dtias", "aws"
    Version() string                 // Backend version
    Capabilities() []Capability      // Supported operations

    // Resource Pools
    ListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error)
    GetResourcePool(ctx context.Context, id string) (*ResourcePool, error)
    CreateResourcePool(ctx context.Context, pool *ResourcePool) (*ResourcePool, error)
    UpdateResourcePool(ctx context.Context, id string, pool *ResourcePool) (*ResourcePool, error)
    DeleteResourcePool(ctx context.Context, id string) error

    // Resources (Nodes/Machines)
    ListResources(ctx context.Context, filter *Filter) ([]*Resource, error)
    GetResource(ctx context.Context, id string) (*Resource, error)
    CreateResource(ctx context.Context, resource *Resource) (*Resource, error)
    DeleteResource(ctx context.Context, id string) error

    // Resource Types
    ListResourceTypes(ctx context.Context, filter *Filter) ([]*ResourceType, error)
    GetResourceType(ctx context.Context, id string) (*ResourceType, error)

    // Lifecycle
    Health(ctx context.Context) error
    Close() error
}
```

### Routing Configuration

Routing rules determine which adapter handles which requests:

```yaml
# config/routing.yaml
routing:
  # Default adapter for requests not matching any rule
  default: kubernetes

  # Routing rules (evaluated in priority order)
  rules:
    # Rule 1: Bare-metal pools → Dell DTIAS
    - name: bare-metal-to-dtias
      priority: 100
      adapter: dtias
      resourceType: resourcePool
      conditions:
        labels:
          infrastructure.type: bare-metal
        location:
          prefix: dc-

    # Rule 2: Cloud pools → AWS adapter
    - name: cloud-to-aws
      priority: 90
      adapter: aws
      resourceType: resourcePool
      conditions:
        labels:
          infrastructure.type: cloud
        location:
          prefix: aws-

    # Rule 3: GPU resources → Kubernetes (with GPU support)
    - name: gpu-to-kubernetes
      priority: 80
      adapter: kubernetes
      resourceType: resource
      conditions:
        extensions:
          gpuType: "*"  # Any GPU type

    # Rule 4: Everything else → Kubernetes (default)
    - name: default-to-kubernetes
      priority: 1
      adapter: kubernetes
      resourceType: "*"
```

### Request Routing Flow

```
┌─────────────────────────────────────────────────────────────┐
│ O2-IMS API Request                                          │
│ GET /o2ims/v1/resourcePools?location=dc-dallas             │
└───────────────────┬─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────────────────────┐
│ Gateway Router                                              │
│ 1. Parse request (resource type, filters, params)          │
│ 2. Query Adapter Registry                                  │
└───────────────────┬─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────────────────────┐
│ Adapter Registry                                            │
│ 1. Evaluate routing rules by priority                      │
│ 2. Match conditions (location=dc-* → rule "bare-metal")    │
│ 3. Return adapter: "dtias"                                 │
└───────────────────┬─────────────────────────────────────────┘
                    │
        ┌───────────┴───────────┬─────────────────┐
        ▼                       ▼                 ▼
┌──────────────┐    ┌──────────────────┐   ┌─────────────┐
│ Kubernetes   │    │ Dell DTIAS       │   │ AWS EKS     │
│ Adapter      │    │ Adapter ◄────────┼───┤ Adapter     │
└──────────────┘    └──────────────────┘   └─────────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ DTIAS REST API   │
                    │ - List pools     │
                    │ - Transform to   │
                    │   O2-IMS format  │
                    └──────────────────┘
```

### Adapter Registry Implementation

```go
// internal/adapter/registry.go
package adapter

type Registry struct {
    mu       sync.RWMutex
    adapters map[string]Adapter
    routes   []RoutingRule
    default  string
}

// Route determines which adapter to use
func (r *Registry) Route(
    resourceType string,
    filter *Filter,
) (Adapter, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Evaluate rules by priority (highest first)
    for _, rule := range r.sortedRules() {
        if rule.Matches(resourceType, filter) {
            if adapter, ok := r.adapters[rule.AdapterName]; ok {
                return adapter, nil
            }
        }
    }

    // Fallback to default
    if adapter, ok := r.adapters[r.default]; ok {
        return adapter, nil
    }

    return nil, fmt.Errorf("no adapter available for %s", resourceType)
}

// RoutingRule defines when to use a specific adapter
type RoutingRule struct {
    Name         string            `yaml:"name"`
    Priority     int               `yaml:"priority"`
    AdapterName  string            `yaml:"adapter"`
    ResourceType string            `yaml:"resourceType"`  // "resourcePool", "resource", "*"
    Conditions   RuleConditions    `yaml:"conditions"`
}

type RuleConditions struct {
    Labels     map[string]string  `yaml:"labels"`      // Key-value matches
    Location   LocationMatch      `yaml:"location"`    // Location prefix/regex
    Extensions map[string]string  `yaml:"extensions"`  // Extension field matches
}

func (r *RoutingRule) Matches(resourceType string, filter *Filter) bool {
    // Match resource type
    if r.ResourceType != "*" && r.ResourceType != resourceType {
        return false
    }

    // Match location
    if r.Conditions.Location.Prefix != "" {
        if !strings.HasPrefix(filter.Location, r.Conditions.Location.Prefix) {
            return false
        }
    }

    // Match labels
    for key, value := range r.Conditions.Labels {
        if filter.Labels[key] != value {
            return false
        }
    }

    // Match extensions
    for key, pattern := range r.Conditions.Extensions {
        extValue, ok := filter.Extensions[key]
        if !ok {
            return false
        }
        if pattern != "*" && extValue != pattern {
            return false
        }
    }

    return true
}
```

### Multi-Backend Aggregation

For `List` operations, results can be **aggregated from multiple backends**:

```go
// internal/handlers/resource_pools.go
func (h *Handler) ListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error) {
    var allPools []*ResourcePool
    var mu sync.Mutex
    var wg sync.WaitGroup

    // Get all relevant adapters
    adapters := h.registry.GetAdaptersForOperation("resourcePool", "list", filter)

    // Query each adapter in parallel
    for _, adapter := range adapters {
        wg.Add(1)
        go func(a Adapter) {
            defer wg.Done()

            pools, err := a.ListResourcePools(ctx, filter)
            if err != nil {
                log.Error("adapter failed", "adapter", a.Name(), "error", err)
                return
            }

            mu.Lock()
            allPools = append(allPools, pools...)
            mu.Unlock()
        }(adapter)
    }

    wg.Wait()

    // Deduplicate and sort results
    return deduplicateAndSort(allPools), nil
}
```

**Aggregation Strategies**:

| Strategy | Description | Use Case |
|----------|-------------|----------|
| **Merge** | Combine results from all adapters | List all pools across all backends |
| **First** | Return first successful response | Fallback chain (try primary, then backup) |
| **Fanout** | Send to all, wait for all | Ensure consistency across backends |
| **Priority** | Query highest-priority adapter only | Single source of truth per resource type |

### Routing Examples

**Example 1: Bare-Metal Pool → Dell DTIAS**

```bash
# Request
curl -X GET 'https://netweave.example.com/o2ims/v1/resourcePools?location=dc-dallas'

# Routing Decision
location=dc-dallas → prefix match "dc-" → route to "dtias" adapter

# Backend Call
DTIAS Adapter → GET https://dtias.example.com/api/v1/infrastructure/pools
                → Transform DTIAS response to O2-IMS format
```

**Example 2: Cloud Pool → AWS**

```bash
# Request
curl -X POST https://netweave.example.com/o2ims/v1/resourcePools \
  -d '{
    "name": "Production EKS Pool",
    "location": "aws-us-west-2",
    "extensions": {"instanceType": "m5.2xlarge"}
  }'

# Routing Decision
location=aws-us-west-2 → prefix match "aws-" → route to "aws" adapter

# Backend Call
AWS Adapter → Create EKS NodeGroup via AWS SDK
            → Transform AWS response to O2-IMS format
```

**Example 3: GPU Resource → Kubernetes**

```bash
# Request
curl -X GET 'https://netweave.example.com/o2ims/v1/resources?extensions.gpuType=A100'

# Routing Decision
extensions.gpuType=A100 → match GPU rule → route to "kubernetes" adapter

# Backend Call
Kubernetes Adapter → List Nodes with label nvidia.com/gpu=A100
                   → Transform Node to O2-IMS Resource
```

---

## API Versioning and Evolution

### Versioning Strategy

The netweave gateway supports **multiple simultaneous API versions** to enable evolution without breaking existing clients:

- **URL-Based Versioning**: `/o2ims/v1/...`, `/o2ims/v2/...`, `/o2ims/v3/...`
- **Parallel Support**: Multiple versions active simultaneously
- **Independent Evolution**: Each version has its own handlers and response formats
- **Deprecation Policy**: 12-month grace period before sunset

### Version Differences and Mappings

#### v1 (Current Stable)

**Base O2-IMS Specification**:
- Simple resource representations
- Basic filtering (location, labels)
- Standard CRUD operations
- Basic error responses

**Example**: Resource Pool in v1
```json
{
  "resourcePoolId": "pool-1",
  "name": "Production Pool",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1"
}
```

**Adapter Mapping (v1)**:
```go
// v1 handler
func (h *HandlerV1) GetResourcePool(ctx context.Context, id string) (*v1.ResourcePool, error) {
    adapter, err := h.registry.Route("resourcePool", &Filter{ResourcePoolID: id})
    if err != nil {
        return nil, err
    }

    // Get from adapter
    pool, err := adapter.GetResourcePool(ctx, id)
    if err != nil {
        return nil, err
    }

    // Transform to v1 format (simple)
    return &v1.ResourcePool{
        ResourcePoolID: pool.ResourcePoolID,
        Name:           pool.Name,
        Location:       pool.Location,
        OCloudID:       pool.OCloudID,
    }, nil
}
```

#### v2 (Enhanced Features)

**Enhanced Specification**:
- **Additional fields**: `health`, `metrics`, `usage`
- **Enhanced filtering**: Complex queries, field selection
- **Batch operations**: Create/update multiple resources
- **Pagination**: Cursor-based pagination
- **Rich error responses**: Error details, suggestions

**Example**: Resource Pool in v2
```json
{
  "resourcePoolId": "pool-1",
  "name": "Production Pool",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1",
  "health": {
    "status": "healthy",
    "lastChecked": "2026-01-06T10:30:00Z"
  },
  "metrics": {
    "totalCapacity": 100,
    "usedCapacity": 75,
    "utilizationPercent": 75.0
  },
  "usage": {
    "activeResources": 15,
    "totalResources": 20
  }
}
```

**Adapter Mapping (v2)**:
```go
// v2 handler with enhanced data
func (h *HandlerV2) GetResourcePool(ctx context.Context, id string) (*v2.ResourcePool, error) {
    adapter, err := h.registry.Route("resourcePool", &Filter{ResourcePoolID: id})
    if err != nil {
        return nil, err
    }

    // Get base data from adapter
    pool, err := adapter.GetResourcePool(ctx, id)
    if err != nil {
        return nil, err
    }

    // v2 enhancement: Add health metrics
    health, err := h.computePoolHealth(ctx, pool)
    if err != nil {
        log.Warn("failed to compute health", "error", err)
        health = &v2.Health{Status: "unknown"}
    }

    // v2 enhancement: Add usage metrics
    metrics, err := h.computePoolMetrics(ctx, pool)
    if err != nil {
        log.Warn("failed to compute metrics", "error", err)
    }

    // Transform to v2 format (enhanced)
    return &v2.ResourcePool{
        ResourcePoolID: pool.ResourcePoolID,
        Name:           pool.Name,
        Location:       pool.Location,
        OCloudID:       pool.OCloudID,
        Health:         health,      // NEW in v2
        Metrics:        metrics,     // NEW in v2
        Usage:          computeUsage(pool),  // NEW in v2
    }, nil
}
```

### Version-Specific Routing

Different API versions can route to different adapters or use different routing rules:

```yaml
# config/routing-v2.yaml
versioning:
  v1:
    # v1 uses simple routing
    default: kubernetes
    rules:
      - name: bare-metal
        adapter: dtias
        conditions:
          location:
            prefix: dc-

  v2:
    # v2 adds enhanced routing with capability checks
    default: kubernetes
    rules:
      - name: bare-metal-with-metrics
        adapter: dtias-v2  # Enhanced DTIAS adapter with metrics support
        conditions:
          location:
            prefix: dc-
          capabilities:
            - metrics
            - health-check

      - name: cloud-with-auto-scaling
        adapter: aws-v2
        conditions:
          location:
            prefix: aws-
          features:
            - auto-scaling
```

### Deprecation Headers

When a version is deprecated, include deprecation headers:

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-API-Deprecated: true
X-API-Deprecation-Date: 2026-07-01
X-API-Sunset-Date: 2027-01-01
X-API-Migration-Guide: https://docs.netweave.io/migration/v1-to-v2

{
  "resourcePoolId": "pool-1",
  ...
}
```

### Version Support Matrix

| Version | Status | Supported Until | Recommended |
|---------|--------|-----------------|-------------|
| v1 | Stable | 2027-01-01 | Yes (current) |
| v2 | Beta | N/A (not released) | No (testing only) |
| v3 | Alpha | N/A (not released) | No (development) |

---

## Deployment Manager

### O2-IMS Specification

```json
{
  "deploymentManagerId": "ocloud-k8s-1",
  "name": "US-East Kubernetes Cloud",
  "description": "Production Kubernetes cluster for RAN workloads",
  "oCloudId": "ocloud-1",
  "serviceUri": "https://api.o2ims.example.com/o2ims/v1"
}
```

**Attributes**:
- `deploymentManagerId` (string, required): Unique identifier
- `name` (string, required): Human-readable name
- `description` (string, optional): Description
- `oCloudId` (string, required): Parent O-Cloud ID
- `serviceUri` (string, required): API endpoint
- `supportedLocations` (array, optional): Geographic locations
- `capabilities` (object, optional): Supported capabilities
- `extensions` (object, optional): Vendor extensions

### Kubernetes Mapping

**No direct K8s equivalent** - use Custom Resource or ConfigMap

**Option 1: Custom Resource (Recommended)**:
```yaml
apiVersion: o2ims.oran.org/v1alpha1
kind: O2DeploymentManager
metadata:
  name: ocloud-k8s-1
  namespace: o2ims-system
spec:
  deploymentManagerId: "ocloud-k8s-1"
  name: "US-East Kubernetes Cloud"
  description: "Production Kubernetes cluster for RAN workloads"
  oCloudId: "ocloud-1"
  serviceUri: "https://api.o2ims.example.com/o2ims/v1"
  supportedLocations:
    - "us-east-1a"
    - "us-east-1b"
    - "us-east-1c"
  capabilities:
    - "compute"
    - "storage"
    - "networking"
  extensions:
    clusterVersion: "1.30.0"
    provider: "AWS"
    region: "us-east-1"
```

**Option 2: ConfigMap** (simpler, no CRD needed):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: o2ims-deployment-manager
  namespace: o2ims-system
data:
  deploymentManagerId: "ocloud-k8s-1"
  name: "US-East Kubernetes Cloud"
  description: "Production Kubernetes cluster for RAN workloads"
  oCloudId: "ocloud-1"
  serviceUri: "https://api.o2ims.example.com/o2ims/v1"
```

**Transformation Logic**:
```go
func (a *KubernetesAdapter) GetDeploymentManager(
    ctx context.Context,
    dmID string,
) (*models.DeploymentManager, error) {
    // Read from CRD or ConfigMap
    var dm o2imsv1alpha1.O2DeploymentManager
    err := a.client.Get(ctx, types.NamespacedName{
        Name:      dmID,
        Namespace: "o2ims-system",
    }, &dm)
    if err != nil {
        return nil, err
    }

    // Add dynamic cluster information
    nodes := &corev1.NodeList{}
    a.client.List(ctx, nodes)

    return &models.DeploymentManager{
        DeploymentManagerID: dm.Spec.DeploymentManagerID,
        Name:                dm.Spec.Name,
        Description:         dm.Spec.Description,
        OCloudID:            dm.Spec.OCloudID,
        ServiceURI:          dm.Spec.ServiceURI,
        SupportedLocations:  dm.Spec.SupportedLocations,
        Capabilities:        dm.Spec.Capabilities,
        Extensions: map[string]interface{}{
            "totalNodes":     len(nodes.Items),
            "k8sVersion":     nodes.Items[0].Status.NodeInfo.KubeletVersion,
            "containerRuntime": nodes.Items[0].Status.NodeInfo.ContainerRuntimeVersion,
        },
    }, nil
}
```

### API Operations

| Operation | Method | Endpoint | K8s Action |
|-----------|--------|----------|------------|
| List | GET | `/deploymentManagers` | List O2DeploymentManager CRs |
| Get | GET | `/deploymentManagers/{id}` | Get O2DeploymentManager CR |
| ~~Create~~ | ~~POST~~ | ~~N/A~~ | Not supported (cluster-level) |
| ~~Update~~ | ~~PUT~~ | ~~N/A~~ | Not supported (cluster-level) |
| ~~Delete~~ | ~~DELETE~~ | ~~N/A~~ | Not supported (cluster-level) |

---

## Resource Pools

### O2-IMS Specification

```json
{
  "resourcePoolId": "pool-compute-high-mem",
  "name": "High Memory Compute Pool",
  "description": "Nodes with 128GB+ RAM for memory-intensive workloads",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1",
  "globalLocationId": "geo:37.7749,-122.4194",
  "extensions": {
    "machineType": "n1-highmem-16",
    "replicas": 5
  }
}
```

**Attributes**:
- `resourcePoolId` (string): Unique ID
- `name` (string): Pool name
- `description` (string): Description
- `location` (string): Physical location
- `oCloudId` (string): Parent O-Cloud
- `globalLocationId` (string, optional): Geographic coordinates
- `extensions` (object): Additional metadata

### Kubernetes Mapping

**Primary**: `MachineSet` (OpenShift) or `NodePool` (Cluster API)

**MachineSet Example**:
```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: pool-compute-high-mem
  namespace: openshift-machine-api
  labels:
    o2ims.oran.org/resource-pool-id: pool-compute-high-mem
    o2ims.oran.org/o-cloud-id: ocloud-1
spec:
  replicas: 5
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machineset: pool-compute-high-mem

  template:
    metadata:
      labels:
        machine.openshift.io/cluster-api-machineset: pool-compute-high-mem
        o2ims.oran.org/resource-pool-id: pool-compute-high-mem

    spec:
      providerSpec:
        value:
          apiVersion: machine.openshift.io/v1beta1
          kind: AWSMachineProviderConfig
          instanceType: m5.4xlarge  # 16 vCPU, 64 GB RAM
          blockDevices:
            - ebs:
                volumeSize: 120
                volumeType: gp3
          placement:
            availabilityZone: us-east-1a
```

### Transformation Logic

**O2-IMS → Kubernetes**:
```go
func transformO2PoolToMachineSet(pool *models.ResourcePool) *machinev1beta1.MachineSet {
    return &machinev1beta1.MachineSet{
        ObjectMeta: metav1.ObjectMeta{
            Name:      pool.ResourcePoolID,
            Namespace: "openshift-machine-api",
            Labels: map[string]string{
                "o2ims.oran.org/resource-pool-id": pool.ResourcePoolID,
                "o2ims.oran.org/o-cloud-id":       pool.OCloudID,
            },
        },
        Spec: machinev1beta1.MachineSetSpec{
            Replicas: ptr.To(int32(pool.Extensions["replicas"].(int))),
            Selector: metav1.LabelSelector{
                MatchLabels: map[string]string{
                    "machine.openshift.io/cluster-api-machineset": pool.ResourcePoolID,
                },
            },
            Template: machinev1beta1.MachineTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{
                        "machine.openshift.io/cluster-api-machineset": pool.ResourcePoolID,
                        "o2ims.oran.org/resource-pool-id":             pool.ResourcePoolID,
                    },
                },
                Spec: machinev1beta1.MachineSpec{
                    ProviderSpec: machinev1beta1.ProviderSpec{
                        Value: runtime.RawExtension{
                            Raw: marshalAWSProviderConfig(pool),
                        },
                    },
                },
            },
        },
    }
}
```

**Kubernetes → O2-IMS**:
```go
func transformMachineSetToO2Pool(ms *machinev1beta1.MachineSet) *models.ResourcePool {
    // Extract provider-specific config
    providerConfig := parseAWSProviderConfig(ms.Spec.Template.Spec.ProviderSpec.Value)

    return &models.ResourcePool{
        ResourcePoolID: ms.Name,
        Name:           ms.Annotations["o2ims.oran.org/name"],
        Description:    ms.Annotations["o2ims.oran.org/description"],
        Location:       providerConfig.Placement.AvailabilityZone,
        OCloudID:       ms.Labels["o2ims.oran.org/o-cloud-id"],
        Extensions: map[string]interface{}{
            "machineType": providerConfig.InstanceType,
            "replicas":    *ms.Spec.Replicas,
            "volumeSize":  providerConfig.BlockDevices[0].EBS.VolumeSize,
        },
    }
}
```

### API Operations

| Operation | Method | Endpoint | K8s Action | Notes |
|-----------|--------|----------|------------|-------|
| List | GET | `/resourcePools` | List MachineSets | Supports filtering by location, labels |
| Get | GET | `/resourcePools/{id}` | Get MachineSet | Returns 404 if not found |
| Create | POST | `/resourcePools` | Create MachineSet | Requires name; generates ID |
| Update | PUT | `/resourcePools/{id}` | Update MachineSet | Updates name, description, extensions |
| Delete | DELETE | `/resourcePools/{id}` | Delete MachineSet | Returns 204 No Content |

#### CREATE Operation Example

**Request:**
```bash
curl -X POST https://gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GPU Pool (Production)",
    "description": "High-performance GPU nodes for ML workloads",
    "location": "us-west-2a",
    "oCloudId": "ocloud-prod-us-west-2",
    "globalLocationId": "geo:47.6062,-122.3321",
    "extensions": {
      "instanceType": "p4d.24xlarge",
      "replicas": 5,
      "datacenter": "us-west-2a"
    }
  }'
```

**Response (201 Created):**
```json
{
  "resourcePoolId": "pool-gpu-pool--production--a1b2c3d4-e5f6-7890-abcd-1234567890ab",
  "name": "GPU Pool (Production)",
  "description": "High-performance GPU nodes for ML workloads",
  "location": "us-west-2a",
  "oCloudId": "ocloud-prod-us-west-2",
  "globalLocationId": "geo:47.6062,-122.3321",
  "extensions": {
    "instanceType": "p4d.24xlarge",
    "replicas": 5,
    "datacenter": "us-west-2a"
  }
}
```

**Validation Rules:**
- `name` is **required** (400 Bad Request if missing)
- `name` max length: 255 characters
- `resourcePoolId` is auto-generated if not provided (format: `pool-{sanitized-name}-{uuid}`)
- `description` max length: 1000 characters
- `extensions` limited to 50KB total payload size

#### HTTP Status Codes

**POST /resourcePools**
- `201 Created` - Resource pool successfully created
- `400 Bad Request` - Invalid request body or validation errors
- `409 Conflict` - Resource pool with the specified ID already exists
- `500 Internal Server Error` - Backend adapter error

**PUT /resourcePools/{id}**
- `200 OK` - Resource pool successfully updated
- `400 Bad Request` - Invalid request body or validation errors
- `404 Not Found` - Resource pool with specified ID does not exist
- `500 Internal Server Error` - Backend adapter error

**DELETE /resourcePools/{id}**
- `204 No Content` - Resource pool successfully deleted
- `404 Not Found` - Resource pool with specified ID does not exist
- `500 Internal Server Error` - Backend adapter error

**GET /resourcePools/{id}**
- `200 OK` - Resource pool found and returned
- `404 Not Found` - Resource pool with specified ID does not exist
- `500 Internal Server Error` - Backend adapter error

**GET /resourcePools**
- `200 OK` - List of resource pools returned (may be empty)
- `400 Bad Request` - Invalid query parameters
- `500 Internal Server Error` - Backend adapter error

#### Input Validation

The gateway performs the following validation on resource pool operations:

**Field Validation:**
- `name` - Required, maximum 255 characters
- `resourcePoolId` - Optional on create (auto-generated if not provided), maximum 255 characters, alphanumeric with hyphens and underscores only
- `description` - Optional, maximum 1000 characters

**Input Sanitization:**
When `resourcePoolId` is not provided on create, it's auto-generated from the pool name with the following sanitization:
- Spaces and special characters (`/`, `\`, `..`, `:`, `*`, `?`, `"`, `<`, `>`, `|`) replaced with hyphens
- Non-alphanumeric characters (except hyphens and underscores) removed
- Converted to lowercase
- Prefix `pool-` added
- Full UUID (36 characters) appended for uniqueness

Example: `"GPU Pool (Production)"` → `"pool-gpu-pool--production--a1b2c3d4-e5f6-7890-abcd-1234567890ab"`

**Note:** The UUID suffix ensures that similar or identical pool names generate unique IDs, maintaining idempotency for create operations.

#### UUID Design Rationale

All auto-generated IDs (resource pools, resources, subscriptions) use full RFC 4122 compliant UUIDs (36 characters):

**Format Examples:**
- Resource Pool: `pool-{sanitized-name}-{uuid}` → `pool-gpu-cluster-a1b2c3d4-e5f6-7890-abcd-1234567890ab`
- Resource: `{uuid}` → `a1b2c3d4-e5f6-7890-abcd-1234567890ab`
- Subscription: `sub-{uuid}` → `sub-a1b2c3d4-e5f6-7890-abcd-1234567890ab`

**Why Full UUIDs?**
- **Standard Compliance**: RFC 4122 UUID v4 format (universally recognized)
- **Collision Resistance**: 2^122 possible combinations (effectively zero collision probability)
- **Simplicity**: No custom truncation logic needed
- **Compatibility**: Works with all UUID-aware systems and tools
- **Maintainability**: Standard format is predictable and well-documented

---

## Resources

### O2-IMS Specification

```json
{
  "resourceId": "node-worker-1a-abc123",
  "resourceTypeId": "compute-node",
  "resourcePoolId": "pool-compute-high-mem",
  "globalAssetId": "urn:o-ran:node:abc123",
  "description": "Compute node for RAN workloads",
  "extensions": {
    "nodeName": "ip-10-0-1-123.ec2.internal",
    "status": "Ready",
    "cpu": "16 cores",
    "memory": "64 GB",
    "labels": {
      "topology.kubernetes.io/zone": "us-east-1a"
    }
  }
}
```

### Kubernetes Mapping

**Primary**: `Node` (for running nodes) or `Machine` (for lifecycle)

**Node Example**:
```yaml
apiVersion: v1
kind: Node
metadata:
  name: ip-10-0-1-123.ec2.internal
  labels:
    node-role.kubernetes.io/worker: ""
    topology.kubernetes.io/zone: us-east-1a
    o2ims.oran.org/resource-pool-id: pool-compute-high-mem
    o2ims.oran.org/resource-id: node-worker-1a-abc123
status:
  conditions:
    - type: Ready
      status: "True"
  capacity:
    cpu: "16"
    memory: 64Gi
    pods: "110"
  allocatable:
    cpu: "15500m"
    memory: 60Gi
    pods: "110"
  nodeInfo:
    kubeletVersion: v1.30.0
    osImage: "Red Hat Enterprise Linux CoreOS"
    kernelVersion: 5.14.0-284.el9.x86_64
```

### Transformation Logic

**Kubernetes → O2-IMS**:
```go
func transformNodeToO2Resource(node *corev1.Node) *models.Resource {
    // Determine resource pool from labels or MachineSet owner
    poolID := node.Labels["o2ims.oran.org/resource-pool-id"]
    if poolID == "" {
        poolID = inferResourcePoolFromNode(node)
    }

    return &models.Resource{
        ResourceID:     node.Name,
        ResourceTypeID: "compute-node",
        ResourcePoolID: poolID,
        GlobalAssetID:  fmt.Sprintf("urn:o-ran:node:%s", node.UID),
        Description:    fmt.Sprintf("Kubernetes worker node in %s", node.Labels["topology.kubernetes.io/zone"]),
        Extensions: map[string]interface{}{
            "nodeName": node.Name,
            "status":   getNodeStatus(node),
            "cpu":      node.Status.Capacity.Cpu().String(),
            "memory":   node.Status.Capacity.Memory().String(),
            "zone":     node.Labels["topology.kubernetes.io/zone"],
            "labels":   node.Labels,
            "allocatable": map[string]string{
                "cpu":    node.Status.Allocatable.Cpu().String(),
                "memory": node.Status.Allocatable.Memory().String(),
                "pods":   node.Status.Allocatable.Pods().String(),
            },
            "nodeInfo": map[string]string{
                "kubeletVersion":        node.Status.NodeInfo.KubeletVersion,
                "osImage":              node.Status.NodeInfo.OSImage,
                "kernelVersion":         node.Status.NodeInfo.KernelVersion,
                "containerRuntimeVersion": node.Status.NodeInfo.ContainerRuntimeVersion,
            },
        },
    }
}

func getNodeStatus(node *corev1.Node) string {
    for _, cond := range node.Status.Conditions {
        if cond.Type == corev1.NodeReady {
            if cond.Status == corev1.ConditionTrue {
                return "Ready"
            }
            return "NotReady"
        }
    }
    return "Unknown"
}
```

**O2-IMS → Kubernetes** (for Machine creation):
```go
func transformO2ResourceToMachine(resource *models.Resource) *machinev1beta1.Machine {
    poolID := resource.ResourcePoolID
    machineSet := getMachineSetForPool(poolID)

    return &machinev1beta1.Machine{
        ObjectMeta: metav1.ObjectMeta{
            GenerateName: poolID + "-",
            Namespace:    "openshift-machine-api",
            Labels: map[string]string{
                "machine.openshift.io/cluster-api-machineset": poolID,
                "o2ims.oran.org/resource-id":                  resource.ResourceID,
                "o2ims.oran.org/resource-pool-id":             poolID,
            },
        },
        Spec: machinev1beta1.MachineSpec{
            ProviderSpec: machineSet.Spec.Template.Spec.ProviderSpec,
        },
    }
}
```

### API Operations

| Operation | Method | Endpoint | K8s Action | Notes |
|-----------|--------|----------|------------|-------|
| List | GET | `/resources` | List Nodes (or Machines) | Supports filtering by pool, type |
| Get | GET | `/resources/{id}` | Get Node | Returns 404 if not found |
| Create | POST | `/resources` | Create Machine (triggers Node) | Requires resourceTypeId; generates ID |
| Update | PUT | `/resources/{id}` | Update mutable fields (description, globalAssetId, extensions) | Implemented in main |
| Delete | DELETE | `/resources/{id}` | Delete Machine or drain+delete Node | Returns 204 No Content |

#### CREATE Operation Example

**Request:**
```bash
curl -X POST https://gateway.example.com/o2ims-infrastructureInventory/v1/resources \
  -H "Content-Type: application/json" \
  -d '{
    "resourceTypeId": "compute-node-standard",
    "resourcePoolId": "pool-production-us-west-2",
    "description": "Production workload node for AI training",
    "globalAssetId": "urn:o-ran:resource:node-prod-ai-001",
    "extensions": {
      "datacenter": "us-west-2a",
      "purpose": "ml-training",
      "team": "ml-platform"
    }
  }'
```

**Response (201 Created):**
```json
{
  "resourceId": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
  "resourceTypeId": "compute-node-standard",
  "resourcePoolId": "pool-production-us-west-2",
  "description": "Production workload node for AI training",
  "globalAssetId": "urn:o-ran:resource:node-prod-ai-001",
  "extensions": {
    "datacenter": "us-west-2a",
    "purpose": "ml-training",
    "team": "ml-platform"
  }
}
```

**Validation Rules:**
- `resourceTypeId` is **required** (400 Bad Request if missing)
- `resourcePoolId` is **required** (400 Bad Request if missing)
- `resourceId` is auto-generated if not provided (format: plain UUID)
- `description` max length: 1000 characters
- `globalAssetId` must be valid URN format if provided
- `extensions` limited to 50KB total payload size

---

## Resource Types

### O2-IMS Specification

```json
{
  "resourceTypeId": "compute-node-highmem",
  "name": "High Memory Compute Node",
  "description": "Compute node with 64GB+ RAM",
  "vendor": "AWS",
  "model": "m5.4xlarge",
  "version": "v1",
  "resourceClass": "compute",
  "resourceKind": "physical",
  "extensions": {
    "cpu": "16 cores",
    "memory": "64 GB",
    "storage": "120 GB SSD",
    "network": "10 Gbps"
  }
}
```

### Kubernetes Mapping

**No direct equivalent** - aggregate from:
1. **Node capacity** (CPU, memory, storage)
2. **StorageClasses** (storage types)
3. **Machine types** (if using cloud provider)

**Transformation Logic**:
```go
func (a *KubernetesAdapter) ListResourceTypes(
    ctx context.Context,
) ([]models.ResourceType, error) {
    var types []models.ResourceType

    // 1. Get unique machine types from Nodes
    nodes := &corev1.NodeList{}
    a.client.List(ctx, nodes)

    machineTypes := make(map[string]*models.ResourceType)
    for _, node := range nodes.Items {
        instanceType := node.Labels["node.kubernetes.io/instance-type"]
        if _, exists := machineTypes[instanceType]; !exists {
            machineTypes[instanceType] = &models.ResourceType{
                ResourceTypeID: fmt.Sprintf("compute-%s", instanceType),
                Name:           fmt.Sprintf("Compute %s", instanceType),
                ResourceClass:  "compute",
                ResourceKind:   "physical",
                Extensions: map[string]interface{}{
                    "cpu":    node.Status.Capacity.Cpu().String(),
                    "memory": node.Status.Capacity.Memory().String(),
                },
            }
        }
    }

    for _, rt := range machineTypes {
        types = append(types, *rt)
    }

    // 2. Get storage types from StorageClasses
    storageClasses := &storagev1.StorageClassList{}
    a.client.List(ctx, storageClasses)

    for _, sc := range storageClasses.Items {
        types = append(types, models.ResourceType{
            ResourceTypeID: fmt.Sprintf("storage-%s", sc.Name),
            Name:           sc.Name,
            ResourceClass:  "storage",
            ResourceKind:   "virtual",
            Extensions: map[string]interface{}{
                "provisioner":      sc.Provisioner,
                "volumeBindingMode": string(*sc.VolumeBindingMode),
                "reclaimPolicy":    string(*sc.ReclaimPolicy),
            },
        })
    }

    return types, nil
}
```

### API Operations

| Operation | Method | Endpoint | K8s Action |
|-----------|--------|----------|------------|
| List | GET | `/resourceTypes` | Aggregate Nodes + StorageClasses |
| Get | GET | `/resourceTypes/{id}` | Get specific type info |
| ~~Create~~ | ~~POST~~ | ~~N/A~~ | Not supported (read-only) |

---

## Subscriptions

### O2-IMS Specification

```json
{
  "subscriptionId": "550e8400-e29b-41d4-a716-446655440000",
  "callback": "https://smo.example.com/notifications",
  "consumerSubscriptionId": "smo-sub-123",
  "filter": {
    "resourcePoolId": "pool-compute-high-mem",
    "resourceTypeId": "compute-node"
  }
}
```

### Kubernetes Mapping

**Storage**: Redis (subscriptions are O2-IMS concept, not in K8s)

**Event Sources**: Kubernetes Informers (watch API)
- Node events (add, update, delete)
- Machine events
- MachineSet events
- Pod events (optional)

**Webhook Delivery**:
```go
type SubscriptionController struct {
    redis      *redis.Client
    k8sClient  client.Client
    httpClient *http.Client
}

func (c *SubscriptionController) watchNodeEvents(ctx context.Context) {
    nodeInformer := c.k8sClient.Informer(&corev1.Node{})

    nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            node := obj.(*corev1.Node)
            c.notifySubscribers(ctx, "ResourceCreated", transformNodeToO2Resource(node))
        },
        UpdateFunc: func(oldObj, newObj interface{}) {
            node := newObj.(*corev1.Node)
            c.notifySubscribers(ctx, "ResourceUpdated", transformNodeToO2Resource(node))
        },
        DeleteFunc: func(obj interface{}) {
            node := obj.(*corev1.Node)
            c.notifySubscribers(ctx, "ResourceDeleted", transformNodeToO2Resource(node))
        },
    })
}

func (c *SubscriptionController) notifySubscribers(
    ctx context.Context,
    eventType string,
    resource *models.Resource,
) {
    // 1. Get matching subscriptions from Redis
    subs, err := c.getMatchingSubscriptions(ctx, resource)
    if err != nil {
        log.Error("failed to get subscriptions", "error", err)
        return
    }

    // 2. For each subscription, send webhook
    for _, sub := range subs {
        go c.sendWebhook(ctx, sub, eventType, resource)
    }
}

func (c *SubscriptionController) sendWebhook(
    ctx context.Context,
    sub *models.Subscription,
    eventType string,
    resource *models.Resource,
) {
    notification := map[string]interface{}{
        "subscriptionId":         sub.SubscriptionID,
        "consumerSubscriptionId": sub.ConsumerSubscriptionID,
        "eventType":              eventType,
        "resource":               resource,
        "timestamp":              time.Now().UTC().Format(time.RFC3339),
    }

    body, _ := json.Marshal(notification)

    // Retry with exponential backoff
    backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
    for attempt, delay := range backoff {
        req, _ := http.NewRequestWithContext(ctx, "POST", sub.Callback, bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")

        resp, err := c.httpClient.Do(req)
        if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
            log.Info("webhook delivered", "subscription", sub.SubscriptionID, "attempt", attempt+1)
            return
        }

        log.Warn("webhook failed, retrying", "subscription", sub.SubscriptionID, "attempt", attempt+1)
        time.Sleep(delay)
    }

    log.Error("webhook failed after retries", "subscription", sub.SubscriptionID)
}
```

### API Operations

| Operation | Method | Endpoint | Action | Notes |
|-----------|--------|----------|--------|-------|
| List | GET | `/subscriptions` | List from Redis | Returns all subscriptions for tenant |
| Get | GET | `/subscriptions/{id}` | Get from Redis | Returns single subscription |
| Create | POST | `/subscriptions` | Store in Redis + start watching | Validates callback URL with SSRF protection |
| Update | PUT | `/subscriptions/{id}` | Update Redis | Updates callback, filter; preserves ID, tenantId, createdAt |
| Delete | DELETE | `/subscriptions/{id}` | Delete from Redis + stop watching | Returns 204 No Content |

#### Callback URL Security

**SSRF Protection:**

The gateway implements Server-Side Request Forgery (SSRF) protection to prevent malicious callback URLs:

- **Blocked:** `localhost`, `127.0.0.1`, `::1` (loopback addresses)
- **Blocked:** Private IPv4 ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`)
- **Blocked:** Link-local addresses (`169.254.0.0/16`)
- **Blocked:** IPv6 private ranges (ULA `fc00::/7`, link-local `fe80::/10`)
- **Allowed:** Public internet-accessible URLs only

**Example - Rejected Callback:**
```json
POST /o2ims/v1/subscriptions
{
  "callback": "http://localhost/admin",
  "filter": {}
}

Response: 400 Bad Request
{
  "error": "BadRequest",
  "message": "callback URL cannot be localhost",
  "code": 400
}
```

### Batch Operations (v2+)

**Version:** Available in API v2 and later

Batch operations enable efficient bulk creation and deletion of multiple subscriptions or resource pools in a single API call, with support for atomic transactions.

| Operation | Method | Endpoint | Description | Atomic Support |
|-----------|--------|----------|-------------|----------------|
| Batch Create Subscriptions | POST | `/o2ims/v2/batch/subscriptions` | Create multiple subscriptions | ✅ Yes |
| Batch Delete Subscriptions | POST | `/o2ims/v2/batch/subscriptions/delete` | Delete multiple subscriptions | ✅ Yes |
| Batch Create Resource Pools | POST | `/o2ims/v2/batch/resourcePools` | Create multiple resource pools | ✅ Yes |
| Batch Delete Resource Pools | POST | `/o2ims/v2/batch/resourcePools/delete` | Delete multiple resource pools | ✅ Yes |

#### Batch Create Subscriptions

**Request:**
```http
POST /o2ims/v2/batch/subscriptions HTTP/1.1
Content-Type: application/json

{
  "subscriptions": [
    {
      "callback": "https://smo.example.com/notify1",
      "consumerSubscriptionId": "smo-sub-1",
      "filter": {
        "resourcePoolId": ["pool-compute"]
      }
    },
    {
      "callback": "https://smo.example.com/notify2",
      "consumerSubscriptionId": "smo-sub-2",
      "filter": {
        "resourceTypeId": ["compute-node"]
      }
    }
  ],
  "atomic": false
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "results": [
    {
      "index": 0,
      "status": 201,
      "success": true,
      "data": {
        "subscriptionId": "550e8400-e29b-41d4-a716-446655440000",
        "callback": "https://smo.example.com/notify1",
        "consumerSubscriptionId": "smo-sub-1",
        "createdAt": "2026-01-11T10:00:00Z"
      }
    },
    {
      "index": 1,
      "status": 201,
      "success": true,
      "data": {
        "subscriptionId": "550e8400-e29b-41d4-a716-446655440001",
        "callback": "https://smo.example.com/notify2",
        "consumerSubscriptionId": "smo-sub-2",
        "createdAt": "2026-01-11T10:00:01Z"
      }
    }
  ],
  "success": true,
  "successCount": 2,
  "failureCount": 0
}
```

#### Atomic Batch Operations

When `atomic: true` is set, all operations in the batch must succeed or all will be rolled back:

**Request:**
```http
POST /o2ims/v2/batch/subscriptions HTTP/1.1
Content-Type: application/json

{
  "subscriptions": [
    {
      "callback": "https://smo.example.com/notify1",
      "consumerSubscriptionId": "smo-sub-1"
    },
    {
      "callback": "invalid-url",  // This will fail validation
      "consumerSubscriptionID": "smo-sub-2"
    }
  ],
  "atomic": true
}
```

**Response:**
```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "results": [
    {
      "index": 0,
      "status": 409,
      "success": false,
      "error": {
        "error": "RolledBack",
        "message": "Operation rolled back due to atomic batch failure",
        "code": 409
      }
    },
    {
      "index": 1,
      "status": 400,
      "success": false,
      "error": {
        "error": "BadRequest",
        "message": "Invalid callback URL",
        "code": 400
      }
    }
  ],
  "success": false,
  "successCount": 0,
  "failureCount": 2
}
```

#### Batch Operation Constraints

- **Min batch size:** 1 operation
- **Max batch size:** 100 operations per request
- **Concurrency limit:** 10 concurrent operations per batch request
- **Timeout:** Standard API timeout applies to entire batch
- **HTTP Status Codes:**
  - `200 OK` - All operations succeeded
  - `207 Multi-Status` - Partial success (some operations failed)
  - `400 Bad Request` - All operations failed or atomic batch failed
  - `401 Unauthorized` - Authentication required
  - `403 Forbidden` - Insufficient permissions

---

## Data Transformation Examples

### Example 1: Create Resource Pool

**Request** (O2-IMS):
```http
POST /o2ims/v1/resourcePools HTTP/1.1
Content-Type: application/json

{
  "name": "GPU Pool",
  "description": "Nodes with NVIDIA A100 GPUs",
  "location": "us-west-2a",
  "oCloudId": "ocloud-1",
  "extensions": {
    "instanceType": "p4d.24xlarge",
    "replicas": 3,
    "gpuType": "A100",
    "gpuCount": 8
  }
}
```

**Transformation** (Gateway):
```go
// 1. Validate request
if pool.Name == "" {
    return errors.New("name is required")
}

// 2. Generate ID
poolID := generatePoolID(pool.Name)  // "pool-gpu-a100"

// 3. Create MachineSet
ms := &machinev1beta1.MachineSet{
    ObjectMeta: metav1.ObjectMeta{
        Name:      poolID,
        Namespace: "openshift-machine-api",
        Labels: map[string]string{
            "o2ims.oran.org/resource-pool-id": poolID,
            "o2ims.oran.org/o-cloud-id":       pool.OCloudID,
        },
        Annotations: map[string]string{
            "o2ims.oran.org/name":        pool.Name,
            "o2ims.oran.org/description": pool.Description,
        },
    },
    Spec: machinev1beta1.MachineSetSpec{
        Replicas: ptr.To(int32(3)),
        Template: machinev1beta1.MachineTemplateSpec{
            Spec: machinev1beta1.MachineSpec{
                ProviderSpec: machinev1beta1.ProviderSpec{
                    Value: &runtime.RawExtension{
                        Raw: []byte(`{
                            "apiVersion": "machine.openshift.io/v1beta1",
                            "kind": "AWSMachineProviderConfig",
                            "instanceType": "p4d.24xlarge",
                            "placement": {
                                "availabilityZone": "us-west-2a"
                            },
                            "blockDevices": [{
                                "ebs": {
                                    "volumeSize": 500,
                                    "volumeType": "gp3"
                                }
                            }]
                        }`),
                    },
                },
            },
        },
    },
}

// 4. Apply to K8s
err := k8sClient.Create(ctx, ms)

// 5. Invalidate cache
redis.Publish(ctx, "cache:invalidate:resourcePools", poolID)

// 6. Return O2-IMS response
return &models.ResourcePool{
    ResourcePoolID: poolID,
    Name:           pool.Name,
    Description:    pool.Description,
    Location:       pool.Location,
    OCloudID:       pool.OCloudID,
    Extensions:     pool.Extensions,
}
```

**Response** (O2-IMS):
```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: /o2ims/v1/resourcePools/pool-gpu-a100

{
  "resourcePoolId": "pool-gpu-a100",
  "name": "GPU Pool",
  "description": "Nodes with NVIDIA A100 GPUs",
  "location": "us-west-2a",
  "oCloudId": "ocloud-1",
  "extensions": {
    "instanceType": "p4d.24xlarge",
    "replicas": 3,
    "gpuType": "A100",
    "gpuCount": 8
  }
}
```

---

### Example 2: Create Resource (Provision New Node)

**Request** (O2-IMS):
```http
POST /o2ims/v1/resources HTTP/1.1
Content-Type: application/json

{
  "resourceTypeId": "compute-node",
  "resourcePoolId": "pool-compute-high-mem",
  "globalAssetId": "urn:o-ran:resource:node-prod-042",
  "description": "High-memory compute node for RAN workloads",
  "extensions": {
    "cpu": "64 cores",
    "memory": "512GB",
    "disk": "2TB NVMe",
    "zone": "us-east-1a"
  }
}
```

**Transformation** (Gateway):
```go
// 1. Validate required fields
if resource.ResourceTypeID == "" {
    return errors.New("resource type ID is required")
}
if resource.ResourcePoolID == "" {
    return errors.New("resource pool ID is required")
}

// 2. Validate field constraints
validationErrors := validateResourceFields(&resource)
if len(validationErrors) > 0 {
    return fmt.Errorf("validation failed: %v", validationErrors)
}

// 3. Generate resource ID if not provided
if resource.ResourceID == "" {
    resource.ResourceID = fmt.Sprintf("res-%s-%s",
        resource.ResourceTypeID,
        uuid.New().String())
}

// 4. Get resource pool to determine machine template
pool, err := adapter.GetResourcePool(ctx, resource.ResourcePoolID)
if err != nil {
    return err
}

// 5. Create Machine (triggers Node provisioning)
machine := &machinev1beta1.Machine{
    ObjectMeta: metav1.ObjectMeta{
        GenerateName: resource.ResourcePoolID + "-",
        Namespace:    "openshift-machine-api",
        Labels: map[string]string{
            "machine.openshift.io/cluster-api-machineset": resource.ResourcePoolID,
            "o2ims.oran.org/resource-id":                  resource.ResourceID,
            "o2ims.oran.org/resource-pool-id":             resource.ResourcePoolID,
        },
        Annotations: map[string]string{
            "o2ims.oran.org/global-asset-id": resource.GlobalAssetID,
            "o2ims.oran.org/description":     resource.Description,
        },
    },
    Spec: pool.MachineTemplate.Spec,
}

// 6. Apply to K8s
err = k8sClient.Create(ctx, machine)

// 7. Wait for Machine to be provisioned (creates Node)
// This happens asynchronously via machine controller

// 8. Return O2-IMS response
return &models.Resource{
    ResourceID:     resource.ResourceID,
    ResourceTypeID: resource.ResourceTypeID,
    ResourcePoolID: resource.ResourcePoolID,
    GlobalAssetID:  resource.GlobalAssetID,
    Description:    resource.Description,
    Extensions:     resource.Extensions,
}
```

**Response** (O2-IMS):
```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: /o2ims/v1/resources/a1b2c3d4-e5f6-7890-abcd-1234567890ab

{
  "resourceId": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
  "resourceTypeId": "compute-node",
  "resourcePoolId": "pool-compute-high-mem",
  "globalAssetId": "urn:o-ran:resource:node-prod-042",
  "description": "High-memory compute node for RAN workloads",
  "extensions": {
    "cpu": "64 cores",
    "memory": "512GB",
    "disk": "2TB NVMe",
    "zone": "us-east-1a",
    "nodeName": "ip-10-0-1-123.ec2.internal",
    "status": "Provisioning"
  }
}
```

**Kubernetes Side Effects**:
```yaml
# Machine created (triggers Node provisioning)
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  name: pool-compute-high-mem-xyz123
  namespace: openshift-machine-api
  labels:
    machine.openshift.io/cluster-api-machineset: pool-compute-high-mem
    o2ims.oran.org/resource-id: a1b2c3d4-e5f6-7890-abcd-1234567890ab
  annotations:
    o2ims.oran.org/global-asset-id: "urn:o-ran:resource:node-prod-042"
    o2ims.oran.org/description: "High-memory compute node for RAN workloads"
spec:
  # Machine spec from resource pool template
  providerSpec:
    value:
      instanceType: m5.4xlarge
      placement:
        availabilityZone: us-east-1a

# After ~5 minutes, Node appears:
apiVersion: v1
kind: Node
metadata:
  name: ip-10-0-1-123.ec2.internal
  labels:
    o2ims.oran.org/resource-id: a1b2c3d4-e5f6-7890-abcd-1234567890ab
    o2ims.oran.org/resource-pool-id: pool-compute-high-mem
status:
  conditions:
    - type: Ready
      status: "True"
  capacity:
    cpu: "64"
    memory: 512Gi
```

**Error Scenarios**:

**Missing Required Field**:
```http
POST /o2ims/v1/resources
{"resourceTypeId": "compute-node"}

→ HTTP 400 Bad Request
{
  "error": "BadRequest",
  "message": "Resource pool ID is required",
  "code": 400
}
```

**Invalid GlobalAssetID**:
```http
POST /o2ims/v1/resources
{
  "resourceTypeId": "compute-node",
  "resourcePoolId": "pool-compute-high-mem",
  "globalAssetId": "invalid-not-urn"
}

→ HTTP 400 Bad Request
{
  "error": "BadRequest",
  "message": "globalAssetId must start with 'urn:'",
  "code": 400
}
```

**Duplicate Resource ID**:
```http
POST /o2ims/v1/resources
{
  "resourceId": "existing-resource-id",
  "resourceTypeId": "compute-node",
  "resourcePoolId": "pool-compute-high-mem"
}

→ HTTP 409 Conflict
{
  "error": "Conflict",
  "message": "Resource with ID 'existing-resource-id' already exists",
  "code": 409
}
```

---

### Example 3: Subscribe to Node Events

**Request** (O2-IMS):
```http
POST /o2ims/v1/subscriptions HTTP/1.1
Content-Type: application/json

{
  "callback": "https://smo.example.com/o2ims/notifications",
  "consumerSubscriptionId": "smo-subscription-456",
  "filter": {
    "resourcePoolId": "pool-gpu-a100"
  }
}
```

**Processing** (Gateway):
```go
// 1. Validate callback URL
if !isValidURL(sub.Callback) {
    return errors.New("invalid callback URL")
}

// 2. Generate subscription ID (full UUID for uniqueness)
subID := "sub-" + uuid.New().String()

// 3. Store in Redis
err := redis.HSet(ctx, "subscription:"+subID,
    "id", subID,
    "callback", sub.Callback,
    "consumerSubscriptionId", sub.ConsumerSubscriptionID,
    "filter", json.Marshal(sub.Filter),
    "createdAt", time.Now().UTC(),
)

// 4. Add to index
redis.SAdd(ctx, "subscriptions:active", subID)
redis.SAdd(ctx, "subscriptions:resourcePool:pool-gpu-a100", subID)

// 5. Publish event (subscription controller picks up)
redis.Publish(ctx, "subscriptions:created", subID)
```

**Event Flow**:
```
1. New Node joins pool-gpu-a100 (K8s event)
   ↓
2. Node Informer detects (Subscription Controller)
   ↓
3. Query subscriptions matching pool-gpu-a100
   ↓
4. For subscription-456:
   - Transform Node → O2 Resource
   - POST webhook to https://smo.example.com/o2ims/notifications
   ↓
5. SMO receives notification:
   {
     "subscriptionId": "550e8400...",
     "consumerSubscriptionId": "smo-subscription-456",
     "eventType": "ResourceCreated",
     "resource": {
       "resourceId": "node-gpu-1",
       "resourcePoolId": "pool-gpu-a100",
       ...
     },
     "timestamp": "2026-01-06T10:30:00Z"
   }
```

---

---

## Backend-Specific Mappings

This section shows how different backend adapters map their native resources to O2-IMS concepts.

### Complete Plugin Ecosystem

netweave supports **25+ backend plugins** across three categories:

| Category | Backends | Documentation |
|----------|----------|---------------|
| **O2-IMS Plugins** | 10+ (Kubernetes, OpenStack, DTIAS, VMware, AWS, Azure, GKE, Equinix, etc.) | Infrastructure management |
| **O2-DMS Plugins** | 7+ (Helm, ArgoCD, Flux, ONAP-LCM, OSM-LCM, Kustomize, Crossplane) | Deployment management |
| **O2-SMO Plugins** | 5+ (ONAP, OSM, Custom SMO, Cloudify, Camunda) | SMO integration & orchestration |

**For complete plugin specifications, implementation code, interfaces, and configuration examples, see [docs/backend-plugins.md](backend-plugins.md).**

This section provides detailed transformation examples for key backends.

### Kubernetes Adapter Mappings

| O2-IMS Resource | Kubernetes Resource | Transformation Notes |
|-----------------|---------------------|----------------------|
| Deployment Manager | O2DeploymentManager CRD or ConfigMap | Static cluster metadata |
| Resource Pool | MachineSet | 1:1 mapping with replicas |
| Resource | Node (runtime) + Machine (lifecycle) | Node for current state, Machine for CRUD |
| Resource Type | Aggregated from Nodes + StorageClasses | Dynamic aggregation based on instance types |
| Subscription | Redis + K8s Informers | Subscriptions stored in Redis, events from K8s |

**Example Transformation: MachineSet → ResourcePool**

```go
func transformMachineSetToResourcePool(ms *machinev1beta1.MachineSet) *models.ResourcePool {
    // Extract AWS provider config
    providerConfig := parseAWSProviderConfig(ms.Spec.Template.Spec.ProviderSpec.Value)

    return &models.ResourcePool{
        ResourcePoolID: ms.Name,
        Name:           ms.Annotations["o2ims.oran.org/name"],
        Description:    ms.Annotations["o2ims.oran.org/description"],
        Location:       providerConfig.Placement.AvailabilityZone,
        OCloudID:       ms.Labels["o2ims.oran.org/o-cloud-id"],
        Extensions: map[string]interface{}{
            "machineType": providerConfig.InstanceType,
            "replicas":    *ms.Spec.Replicas,
            "volumeSize":  providerConfig.BlockDevices[0].EBS.VolumeSize,
            "zone":        providerConfig.Placement.AvailabilityZone,
        },
    }
}
```

### Dell DTIAS Adapter Mappings

**DTIAS** (Dell Telecom Infrastructure Automation Service) uses a different data model:

| O2-IMS Resource | DTIAS Resource | API Endpoint | Transformation Notes |
|-----------------|----------------|--------------|----------------------|
| Deployment Manager | Infrastructure Site | `GET /v2/inventory/sites/{Id}` | DTIAS v2.4.0 site metadata (wrapped response) |
| Resource Pool | Server Pool | `GET /v2/inventory/resourcepools` | DTIAS v2.4.0 pool with bare-metal servers (wrapped in `Rps` array) |
| Resource | Physical Server | `GET /v2/inventory/servers` | Individual bare-metal server (wrapped in `Full`/`Brief` arrays). Note: `GET /v2/inventory/servers/{Id}` returns `JobResponse`, use list with `?id=` filter instead |
| Resource Type | Server Type | `GET /v2/resourcetypes` | Hardware type/profile (wrapped in `ResourceTypes` array) |

**Example Transformation: DTIAS Server Pool → O2-IMS ResourcePool**

```go
func (a *DTIASAdapter) transformServerPoolToResourcePool(
    pool *dtias.ServerPool,
) *models.ResourcePool {
    return &models.ResourcePool{
        ResourcePoolID: fmt.Sprintf("dtias-pool-%s", pool.ID),
        Name:           pool.Name,
        Description:    pool.Description,
        Location:       pool.DataCenter.Location,
        OCloudID:       a.oCloudID,
        GlobalLocationID: fmt.Sprintf("geo:%s,%s",
            pool.DataCenter.Latitude,
            pool.DataCenter.Longitude,
        ),
        Extensions: map[string]interface{}{
            "infrastructure": "bare-metal",
            "vendor":         "Dell",
            "serverCount":    pool.ServerCount,
            "availableServers": pool.AvailableCount,
            "profiles":       pool.SupportedProfiles,
            "datacenter":     pool.DataCenter.Name,
            "rackRange":      pool.RackRange,
        },
    }
}
```

**Example Transformation: DTIAS Server → O2-IMS Resource**

```go
func (a *DTIASAdapter) transformServerToResource(
    server *dtias.Server,
) *models.Resource {
    return &models.Resource{
        ResourceID:     fmt.Sprintf("dtias-server-%s", server.SerialNumber),
        ResourceTypeID: fmt.Sprintf("dtias-profile-%s", server.ProfileID),
        ResourcePoolID: fmt.Sprintf("dtias-pool-%s", server.PoolID),
        GlobalAssetID:  fmt.Sprintf("urn:dtias:server:%s", server.SerialNumber),
        Description:    fmt.Sprintf("Dell bare-metal server %s", server.Model),
        Extensions: map[string]interface{}{
            "infrastructure": "bare-metal",
            "vendor":         "Dell",
            "model":          server.Model,
            "serialNumber":   server.SerialNumber,
            "serviceTag":     server.ServiceTag,
            "cpu": map[string]interface{}{
                "model":  server.CPU.Model,
                "cores":  server.CPU.Cores,
                "threads": server.CPU.Threads,
                "speed":  server.CPU.SpeedGHz,
            },
            "memory": map[string]interface{}{
                "total":  server.Memory.TotalGB,
                "type":   server.Memory.Type,
                "speed":  server.Memory.SpeedMHz,
            },
            "storage": server.Storage,  // Array of disks
            "network": server.NICs,     // Array of network interfaces
            "status":  server.Status,   // "available", "in-use", "maintenance"
            "power":   server.PowerState, // "on", "off"
        },
    }
}
```

**DTIAS API v2.4.0 Flow Example**:

```
1. O2-IMS Request:
   POST /o2ims/v1/resources
   { "resourcePoolId": "dtias-pool-123", ... }

2. Routing:
   location=dc-dallas → route to "dtias" adapter

3. DTIAS Adapter:
   → Transform O2-IMS request to DTIAS v2.4.0 format
   → POST https://dtias.example.com/v2/resources/allocate
     {
       "serverPoolId": "123",
       "serverTypeId": "high-memory-type",
       "hostname": "edge-node-01",
       "networkConfig": {...},
       "metadata": {...}
     }
   → Wait for allocation (async)
   → Parse wrapped response (Server object)
   → Transform DTIAS response to O2-IMS Resource
   → Return to client

4. O2-IMS Response:
   HTTP 201 Created
   { "resourceId": "dtias-server-ABC123", ... }

Note: DTIAS v2.4.0 wraps all responses:
- GET /v2/inventory/servers returns ServersInventoryResponse{Full: []Server, Brief: []ServerBrief}
- GET /v2/inventory/resourcepools returns ResourcePoolsInventoryResponse{Rps: []ServerPool}
- GET /v2/inventory/servers/{Id} returns JobResponse (async operation status, not server data)
  Use GET /v2/inventory/servers?id={id} to retrieve a specific server instead
```

### AWS EKS Adapter Mappings

For AWS cloud deployments, the adapter uses AWS SDK to manage infrastructure:

| O2-IMS Resource | AWS Resource | AWS API | Transformation Notes |
|-----------------|--------------|---------|----------------------|
| Deployment Manager | EKS Cluster | `DescribeCluster` | Cluster metadata |
| Resource Pool | EKS NodeGroup | `DescribeNodegroup` | Auto Scaling Group wrapper |
| Resource | EC2 Instance | `DescribeInstances` | Individual EC2 instance in NodeGroup |
| Resource Type | EC2 Instance Type | `DescribeInstanceTypes` | m5.2xlarge, c5.4xlarge, etc. |

**Example Transformation: AWS NodeGroup → O2-IMS ResourcePool**

```go
func (a *AWSAdapter) transformNodeGroupToResourcePool(
    ng *eks.Nodegroup,
) *models.ResourcePool {
    return &models.ResourcePool{
        ResourcePoolID: fmt.Sprintf("aws-nodegroup-%s", *ng.NodegroupName),
        Name:           *ng.NodegroupName,
        Description:    fmt.Sprintf("EKS NodeGroup in %s", *ng.ClusterName),
        Location:       *ng.Subnets[0],  // Primary subnet AZ
        OCloudID:       a.oCloudID,
        Extensions: map[string]interface{}{
            "infrastructure":  "cloud",
            "provider":        "AWS",
            "clusterName":     *ng.ClusterName,
            "instanceTypes":   ng.InstanceTypes,  // Can be multiple types
            "amiType":         *ng.AmiType,
            "diskSize":        *ng.DiskSize,
            "capacityType":    *ng.CapacityType,  // ON_DEMAND or SPOT
            "scalingConfig": map[string]interface{}{
                "minSize":     *ng.ScalingConfig.MinSize,
                "maxSize":     *ng.ScalingConfig.MaxSize,
                "desiredSize": *ng.ScalingConfig.DesiredSize,
            },
            "subnets":     ng.Subnets,
            "labels":      ng.Labels,
            "taints":      ng.Taints,
            "launchTemplate": ng.LaunchTemplate,
        },
    }
}
```

**Example Transformation: EC2 Instance → O2-IMS Resource**

```go
func (a *AWSAdapter) transformEC2InstanceToResource(
    instance *ec2.Instance,
    nodegroupName string,
) *models.Resource {
    return &models.Resource{
        ResourceID:     fmt.Sprintf("aws-instance-%s", *instance.InstanceId),
        ResourceTypeID: fmt.Sprintf("aws-instance-type-%s", *instance.InstanceType),
        ResourcePoolID: fmt.Sprintf("aws-nodegroup-%s", nodegroupName),
        GlobalAssetID:  fmt.Sprintf("urn:aws:ec2:%s:%s", *instance.Placement.AvailabilityZone, *instance.InstanceId),
        Description:    fmt.Sprintf("EC2 instance %s", *instance.InstanceType),
        Extensions: map[string]interface{}{
            "infrastructure":  "cloud",
            "provider":        "AWS",
            "instanceId":      *instance.InstanceId,
            "instanceType":    *instance.InstanceType,
            "availabilityZone": *instance.Placement.AvailabilityZone,
            "state":           *instance.State.Name,  // running, stopped, etc.
            "privateIp":       *instance.PrivateIpAddress,
            "publicIp":        getPublicIP(instance),
            "vpcId":           *instance.VpcId,
            "subnetId":        *instance.SubnetId,
            "architecture":    *instance.Architecture,  // x86_64, arm64
            "virtualizationType": *instance.VirtualizationType,
            "platform":        instance.Platform,  // windows or nil for linux
            "launchTime":      *instance.LaunchTime,
        },
    }
}
```

### Comparison: Multi-Backend Resource Pool Creation

**Same O2-IMS Request, Different Backend Actions**:

```json
POST /o2ims/v1/resourcePools
{
  "name": "Production Pool",
  "description": "High-performance compute nodes",
  "location": "us-east-1a",  // or "dc-dallas" for bare-metal
  "oCloudId": "ocloud-1",
  "extensions": {
    "replicas": 5
  }
}
```

#### Kubernetes Adapter
```go
// Creates a MachineSet
k8sClient.Create(ctx, &machinev1beta1.MachineSet{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "production-pool",
        Namespace: "openshift-machine-api",
    },
    Spec: machinev1beta1.MachineSetSpec{
        Replicas: ptr.To(int32(5)),
        Template: machinev1beta1.MachineTemplateSpec{
            Spec: machinev1beta1.MachineSpec{
                ProviderSpec: awsProviderSpec,  // AWS, GCP, Azure, etc.
            },
        },
    },
})
```

#### Dell DTIAS Adapter
```go
// Calls DTIAS API to create server pool
dtiasClient.CreateServerPool(ctx, &dtias.CreatePoolRequest{
    Name:        "Production Pool",
    Description: "High-performance compute nodes",
    DataCenter:  "dc-dallas",
    ProfileID:   "high-performance-profile",
    ServerCount: 5,
    RackRange:   "A01-A05",
    NetworkVLANs: []string{"vlan-100", "vlan-200"},
})
```

#### AWS Adapter
```go
// Creates an EKS NodeGroup
eksClient.CreateNodegroup(ctx, &eks.CreateNodegroupInput{
    ClusterName:   aws.String("production-cluster"),
    NodegroupName: aws.String("production-pool"),
    Subnets:       []string{"subnet-123", "subnet-456"},
    InstanceTypes: []string{"m5.2xlarge"},
    ScalingConfig: &eks.NodegroupScalingConfig{
        MinSize:     aws.Int64(3),
        MaxSize:     aws.Int64(10),
        DesiredSize: aws.Int64(5),
    },
    DiskSize:     aws.Int64(100),
    AmiType:      aws.String("AL2_x86_64"),
    CapacityType: aws.String("ON_DEMAND"),
})
```

### Summary

**Adapter Pattern Benefits**:
1. ✅ **Unified O2-IMS API**: SMO sees consistent interface regardless of backend
2. ✅ **Backend Flexibility**: Add/remove backends without API changes
3. ✅ **Technology Migration**: Gradually migrate between backends
4. ✅ **Vendor Independence**: Avoid lock-in to single infrastructure provider
5. ✅ **Hybrid Deployments**: Mix cloud and bare-metal in single O-Cloud

**Mapping Completeness**:
- ✅ Deployment Manager → Custom Resource (metadata)
- ✅ Resource Pool → MachineSet / DTIAS Pool / AWS NodeGroup (full CRUD)
- ✅ Resource → Node/Machine / DTIAS Server / EC2 Instance (full CRUD)
- ✅ Resource Type → Aggregated from Nodes + StorageClasses + Provider Catalogs
- ✅ Subscription → Redis + K8s Informers / DTIAS Webhooks / AWS EventBridge (full CRUD + webhooks)

**Key Principles**:
1. Use native K8s resources where possible
2. Store O2-IMS-specific data in Redis or CRDs
3. Transform bidirectionally (read and write)
4. Preserve semantic meaning across translation
5. Handle errors gracefully with proper O2-IMS error responses
6. **Route intelligently based on resource characteristics**
7. **Support multiple backends simultaneously**
8. **Abstract backend complexity from SMO clients**

**Future Extensions**:
- Additional adapters (OpenStack, VMware, Azure)
- Advanced filtering (complex queries)
- Batch operations
- Custom resource definitions for all O2-IMS types
- Cross-backend resource migration
