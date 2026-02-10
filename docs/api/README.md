# O2-IMS API Documentation

**Version:** 2.0
**Date:** 2026-01-19

This directory contains the complete O2-IMS API documentation for the netweave gateway.

## Contents

| Document | Description |
|----------|-------------|
| [O2-IMS Overview](o2ims/README.md) | O2-IMS concepts, architecture, and philosophy |
| [Deployment Managers](o2ims/deployment-managers.md) | Deployment Manager API and Kubernetes mappings |
| [Resource Pools](o2ims/resource-pools.md) | Resource Pool API, CRUD operations, and backend mappings |
| [Resources](o2ims/resources.md) | Resource API, lifecycle management, and transformations |
| [Resource Types](o2ims/resource-types.md) | Resource Type API and aggregation logic |
| [Subscriptions](o2ims/subscriptions.md) | Subscription API, webhook delivery, and update behavior |

## Quick Start

### Base URLs (Multi-Port Architecture)

The gateway exposes different API surfaces on dedicated ports:

| Port | Hostname | Auth | APIs |
|------|----------|------|------|
| 8080 | `api.netweave.local` | OAuth2/TLS | Admin API |
| 8443 | `o2.netweave.local` | mTLS | O2-IMS, O2-DMS |
| 8444 | `tmf.netweave.local` | OAuth2/TLS | TMForum APIs |
| 8445 | `graphql.netweave.local` | OAuth2/TLS | GraphQL API |

```
# O2-IMS API (mTLS on port 8443)
https://o2.netweave.local:8443/o2ims-infrastructureInventory/v1

# TMForum API (OAuth2 on port 8444)
https://tmf.netweave.local:8444/tmf-api/resourceInventoryManagement/v4

# Admin API (OAuth2 on port 8080)
https://api.netweave.local:8080/api/v1
```

### Authentication

The gateway uses **port-based authentication** - each port has its own auth method:

#### O2-IMS/O2-DMS: mTLS Certificate Authentication (Port 8443)

```bash
curl -X GET https://o2.netweave.local:8443/o2ims-infrastructureInventory/v1/resourcePools \
  --cert client.crt \
  --key client.key \
  --cacert ca.crt
```

#### Admin/TMF/GraphQL: OAuth2/OIDC Token Authentication (Ports 8080, 8444, 8445)

```bash
# Get token from Keycloak
TOKEN=$(curl -s -X POST \
  https://auth.netweave.local/realms/netweave/protocol/openid-connect/token \
  -d "client_id=netweave-gateway" \
  -d "client_secret=secret" \
  -d "username=user@example.com" \
  -d "password=password" \
  -d "grant_type=password" \
  | jq -r '.access_token')

# Make authenticated request to TMF API
curl -X GET https://tmf.netweave.local:8444/tmf-api/resourceInventoryManagement/v4/resource \
  -H "Authorization: Bearer $TOKEN"
```

#### Port-Based Authentication

Each port enforces its own authentication:
- **Port 8443** (O2): mTLS with client certificates (clientAuth required)
- **Port 8080** (Admin): OAuth2 Bearer tokens (TLS, no clientAuth)
- **Port 8444** (TMF): OAuth2 Bearer tokens (TLS, no clientAuth)
- **Port 8445** (GraphQL): OAuth2 Bearer tokens (TLS, no clientAuth)

See [Authentication Documentation](../security/authentication.md) for complete details.

## API Versioning Strategy

### Version Support Matrix

| Version | Status | Supported Until | Base Path |
|---------|--------|-----------------|-----------|
| v1 | ✅ Stable | 2027-01-01 | `/o2ims-infrastructureInventory/v1` |
| v2 | 🧪 Beta | N/A | `/o2ims-infrastructureInventory/v1` |
| v3 | 🚧 Alpha | N/A | `/o2ims-infrastructureInventory/v1` |

### Versioning Approach

**URL-Based Versioning**: Each major version has its own URL path segment.

```
GET /o2ims-infrastructureInventory/v1/resourcePools  # Stable
GET /o2ims-infrastructureInventory/v1/resourcePools  # Beta (enhanced features)
```

**Parallel Support**: Multiple versions run simultaneously, allowing gradual migration.

**Deprecation Policy**: 12-month grace period before sunset.

### Version Differences

#### v1 (Current Stable)

**Features**:
- Core O2-IMS resource operations (CRUD)
- Basic filtering (location, labels)
- Simple webhook notifications
- Standard error responses

**Example Response**:
```json
{
  "resourcePoolId": "pool-1",
  "name": "Production Pool",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1"
}
```

#### v2 (Beta - Enhanced Features)

**New in v2**:
- **Health metrics**: `health`, `metrics`, `usage` fields
- **Advanced filtering**: Complex queries, field selection
- **Batch operations**: Create/update multiple resources atomically
- **Cursor-based pagination**: `?cursor=xyz&limit=50`
- **Rich error responses**: Error details, suggestions, retry guidance

**Example Response** (v2):
```json
{
  "resourcePoolId": "pool-1",
  "name": "Production Pool",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1",
  "health": {
    "status": "healthy",
    "lastChecked": "2026-01-12T10:30:00Z"
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

**Batch Operations** (v2+):
```http
POST /o2ims-infrastructureInventory/v1/batch/subscriptions
Content-Type: application/json

{
  "subscriptions": [
    {"callback": "https://smo.example.com/notify1", "filter": {}},
    {"callback": "https://smo.example.com/notify2", "filter": {}}
  ],
  "atomic": true
}
```

See [Subscriptions - Batch Operations](o2ims/subscriptions.md#batch-operations) for details.

### Deprecation Headers

When a version is deprecated, responses include:

```http
HTTP/1.1 200 OK
X-API-Deprecated: true
X-API-Deprecation-Date: 2026-07-01
X-API-Sunset-Date: 2027-01-01
X-API-Migration-Guide: https://docs.netweave.io/migration/v1-to-v2

{...}
```

### Migration Best Practices

1. **Test in Beta**: Use v2 endpoints in staging before production
2. **Parallel Adoption**: Run v1 and v2 clients side-by-side during migration
3. **Monitor Deprecation Headers**: Track when v1 will sunset
4. **Upgrade Gradually**: Migrate one service at a time
5. **Validate Responses**: v2 may include additional fields - ensure clients handle unknown fields gracefully

## Multi-Backend Adapter Architecture

The netweave gateway uses a **pluggable adapter pattern** to route O2-IMS requests to different infrastructure backends.

### Supported Backends

| Backend | Type | Resources |
|---------|------|-----------|
| **Kubernetes** | Container orchestration | MachineSets, Nodes |
| **Dell DTIAS** | Bare-metal | Server pools, physical servers |
| **AWS EKS** | Cloud (managed K8s) | NodeGroups, EC2 instances |
| **Azure AKS** | Cloud (managed K8s) | NodePools, VMs |
| **OpenStack** | Private cloud | Host aggregates, compute nodes |
| **VMware** | Virtualization | Resource pools, ESXi hosts |

See [Backend Plugins](../backend-plugins.md) for complete list of 25+ adapters.

### Routing Logic

Requests are routed based on resource characteristics:

```yaml
# Example routing rule
rules:
  - name: bare-metal-to-dtias
    priority: 100
    adapter: dtias
    conditions:
      location:
        prefix: dc-  # dc-dallas, dc-chicago
```

**Request Routing Example**:
```bash
# Bare-metal pool → Dell DTIAS
GET /o2ims-infrastructureInventory/v1/resourcePools?location=dc-dallas

# Cloud pool → AWS EKS
GET /o2ims-infrastructureInventory/v1/resourcePools?location=aws-us-west-2
```

See [Multi-Backend Adapter Routing](o2ims/README.md#multi-backend-adapter-routing) for details.

## Common Response Codes

| Code | Status | Description |
|------|--------|-------------|
| 200 | OK | Request successful |
| 201 | Created | Resource created successfully |
| 204 | No Content | Resource deleted successfully |
| 400 | Bad Request | Invalid request body or parameters |
| 401 | Unauthorized | Authentication required |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource does not exist |
| 409 | Conflict | Resource already exists |
| 500 | Internal Server Error | Backend adapter error |

**Standard Error Response**:
```json
{
  "error": "BadRequest",
  "message": "Resource pool name is required",
  "code": 400
}
```

## Pagination

**v1**: Offset-based pagination
```bash
GET /o2ims-infrastructureInventory/v1/resourcePools?limit=50&offset=100
```

**v2**: Cursor-based pagination (recommended)
```bash
GET /o2ims/v2/resourcePools?limit=50&cursor=eyJpZCI6InBvb2wtMTIzIn0
```

## Filtering

**Basic Filtering** (v1):
```bash
GET /o2ims-infrastructureInventory/v1/resources?resourcePoolId=pool-compute&location=us-east-1a
```

**Advanced Filtering** (v2):
```bash
GET /o2ims/v2/resources?filter=resourcePoolId:eq:pool-compute,status:in:[Ready,NotReady]
```

## Rate Limiting

| Resource Type | Limit | Window |
|---------------|-------|--------|
| Read (GET) | 1000 requests | 1 minute |
| Write (POST/PUT/DELETE) | 100 requests | 1 minute |
| Subscriptions | 10 create/update | 1 minute |
| Batch operations | 5 requests | 1 minute |

**Rate Limit Headers**:
```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 987
X-RateLimit-Reset: 1673020800
```

## OpenAPI Specification

Full OpenAPI 3.0 specification: [openapi/o2ims.yaml](../openapi/o2ims.yaml)

## Resources

- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)
- [Architecture Documentation](../architecture.md)
- [Backend Plugins Guide](../backend-plugins.md)
- [Webhook Security Guide](../webhook-security.md)
