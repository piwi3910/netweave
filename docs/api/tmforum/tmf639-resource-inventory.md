# TMF639 - Resource Inventory Management API v4

## Overview

The TMF639 Resource Inventory Management API provides standardized access to infrastructure resources and resource pools. This implementation maps TMForum resources to O-RAN O2-IMS resource pools and resources, providing TMForum-compliant access to the same underlying infrastructure.

**Base Path**: `/tmf-api/resourceInventoryManagement/v4`

**TMForum Specification**: [TMF639 v4.0](https://www.tmforum.org/resources/specification/tmf639-resource-inventory-management-api-rest-specification-r19-0-1/)

## Resource Model

### TMF639Resource

The core resource model supporting both resource pools and individual resources:

```json
{
  "id": "string",
  "href": "string",
  "name": "string",
  "description": "string",
  "category": "string",
  "resourceStatus": "string",
  "operationalState": "string",
  "usageState": "string",
  "administrativeState": "string",
  "version": "string",
  "place": [
    {
      "id": "string",
      "href": "string",
      "name": "string",
      "role": "string"
    }
  ],
  "resourceCharacteristic": [
    {
      "name": "string",
      "value": "string",
      "valueType": "string"
    }
  ],
  "resourceSpecification": {
    "id": "string",
    "href": "string",
    "name": "string",
    "version": "string"
  },
  "relatedParty": [
    {
      "id": "string",
      "href": "string",
      "name": "string",
      "role": "string"
    }
  ],
  "note": [
    {
      "date": "2026-01-16T10:00:00Z",
      "author": "string",
      "text": "string"
    }
  ],
  "@type": "string",
  "@baseType": "Resource",
  "@schemaLocation": "string"
}
```

### Key Fields

| Field | Type | Description | Mapping |
|-------|------|-------------|---------|
| `id` | string | Unique resource identifier | O2-IMS resourcePoolId or resourceId |
| `name` | string | Human-readable name | O2-IMS name |
| `category` | string | Resource category | `"resourcePool"` for pools, varies for resources |
| `resourceStatus` | string | Lifecycle status | Maps from O2-IMS administrativeState |
| `operationalState` | string | Operational status | O2-IMS operationalState |
| `place` | array | Geographic/logical location | O2-IMS location |
| `resourceCharacteristic` | array | Flexible attributes | O2-IMS extensions + native fields |

## API Endpoints

### List Resources

Retrieve a list of all resources (both pools and individual resources).

**Request:**
```http
GET /tmf-api/resourceInventoryManagement/v4/resource HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `category` | string | Filter by resource category (e.g., `resourcePool`, `compute`, `storage`) |
| `resourceStatus` | string | Filter by status (e.g., `available`, `reserved`, `unavailable`) |
| `operationalState` | string | Filter by operational state |
| `offset` | integer | Pagination offset (default: 0) |
| `limit` | integer | Items per page (default: 100, max: 500) |
| `fields` | string | Comma-separated list of fields to include |

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

[
  {
    "id": "pool-us-east-1",
    "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-us-east-1",
    "name": "US East Kubernetes Cluster",
    "description": "Production Kubernetes cluster in US East",
    "category": "resourcePool",
    "resourceStatus": "available",
    "operationalState": "enabled",
    "place": [
      {
        "id": "us-east-1",
        "name": "US East Region"
      }
    ],
    "resourceCharacteristic": [
      {"name": "provider", "value": "kubernetes"},
      {"name": "type", "value": "compute-pool"},
      {"name": "nodes", "value": "15"}
    ],
    "@type": "ResourcePool",
    "@baseType": "Resource"
  },
  {
    "id": "node-worker-1",
    "href": "/tmf-api/resourceInventoryManagement/v4/resource/node-worker-1",
    "name": "worker-1.us-east-1.local",
    "description": "Kubernetes worker node",
    "category": "compute",
    "resourceStatus": "available",
    "operationalState": "enabled",
    "resourceCharacteristic": [
      {"name": "cpu", "value": "16"},
      {"name": "memory", "value": "64Gi"},
      {"name": "architecture", "value": "amd64"}
    ],
    "relatedParty": [
      {
        "id": "pool-us-east-1",
        "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-us-east-1",
        "role": "ResourcePool"
      }
    ],
    "@type": "ComputeResource",
    "@baseType": "Resource"
  }
]
```

### Get Resource by ID

Retrieve details of a specific resource.

**Request:**
```http
GET /tmf-api/resourceInventoryManagement/v4/resource/{id} HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
```

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Resource identifier (pool ID or resource ID) |

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "pool-us-east-1",
  "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-us-east-1",
  "name": "US East Kubernetes Cluster",
  "description": "Production Kubernetes cluster in US East",
  "category": "resourcePool",
  "resourceStatus": "available",
  "operationalState": "enabled",
  "place": [
    {
      "id": "us-east-1",
      "name": "US East Region",
      "role": "DataCenter"
    }
  ],
  "resourceCharacteristic": [
    {"name": "provider", "value": "kubernetes", "valueType": "string"},
    {"name": "type", "value": "compute-pool", "valueType": "string"},
    {"name": "nodes", "value": "15", "valueType": "integer"},
    {"name": "capacity.cpu", "value": "240", "valueType": "integer"},
    {"name": "capacity.memory", "value": "960Gi", "valueType": "string"}
  ],
  "resourceSpecification": {
    "id": "kubernetes-pool",
    "name": "Kubernetes Resource Pool"
  },
  "@type": "ResourcePool",
  "@baseType": "Resource"
}
```

**Error Response:**
```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "error": "NotFound",
  "message": "Resource with ID 'invalid-id' not found"
}
```

### Create Resource

Create a new resource (currently supports resource pools only).

**Request:**
```http
POST /tmf-api/resourceInventoryManagement/v4/resource HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Edge Compute Pool",
  "description": "Edge location compute resources",
  "category": "resourcePool",
  "place": [
    {"id": "edge-chicago", "name": "Chicago Edge Site"}
  ],
  "resourceCharacteristic": [
    {"name": "provider", "value": "kubernetes"},
    {"name": "type", "value": "edge-pool"},
    {"name": "location_type", "value": "edge"}
  ]
}
```

**Response:**
```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: /tmf-api/resourceInventoryManagement/v4/resource/pool-edge-chicago

{
  "id": "pool-edge-chicago",
  "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-edge-chicago",
  "name": "Edge Compute Pool",
  "description": "Edge location compute resources",
  "category": "resourcePool",
  "resourceStatus": "available",
  "operationalState": "enabled",
  "place": [
    {"id": "edge-chicago", "name": "Chicago Edge Site"}
  ],
  "resourceCharacteristic": [
    {"name": "provider", "value": "kubernetes"},
    {"name": "type", "value": "edge-pool"},
    {"name": "location_type", "value": "edge"}
  ],
  "@type": "ResourcePool",
  "@baseType": "Resource"
}
```

### Update Resource

Update resource attributes (PATCH semantics).

**Request:**
```http
PATCH /tmf-api/resourceInventoryManagement/v4/resource/{id} HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
Content-Type: application/json

{
  "description": "Updated description",
  "resourceCharacteristic": [
    {"name": "nodes", "value": "20"}
  ]
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "pool-us-east-1",
  "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-us-east-1",
  "name": "US East Kubernetes Cluster",
  "description": "Updated description",
  "category": "resourcePool",
  "resourceCharacteristic": [
    {"name": "nodes", "value": "20"},
    {"name": "provider", "value": "kubernetes"}
  ],
  "@type": "ResourcePool"
}
```

### Delete Resource

Delete a resource.

**Request:**
```http
DELETE /tmf-api/resourceInventoryManagement/v4/resource/{id} HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
```

**Response:**
```http
HTTP/1.1 204 No Content
```

**Error Response:**
```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "error": "NotFound",
  "message": "Resource with ID 'invalid-id' not found"
}
```

## Resource Categories

The `category` field distinguishes resource types:

| Category | Description | O2-IMS Mapping |
|----------|-------------|----------------|
| `resourcePool` | Infrastructure resource pool | ResourcePool |
| `compute` | Compute resource (node, VM, etc.) | Resource with compute type |
| `storage` | Storage resource | Resource with storage type |
| `network` | Network resource | Resource with network type |
| `accelerator` | Hardware accelerator (GPU, FPGA) | Resource with accelerator type |

## Resource Status Values

### resourceStatus (Lifecycle)

| Status | Description | O2-IMS Mapping |
|--------|-------------|----------------|
| `available` | Resource available for use | unlocked |
| `reserved` | Resource reserved/allocated | locked |
| `unavailable` | Resource not available | shutting-down |

### operationalState (Runtime)

| State | Description | O2-IMS Mapping |
|-------|-------------|----------------|
| `enabled` | Operationally active | enabled |
| `disabled` | Operationally inactive | disabled |

## Resource Characteristics

The `resourceCharacteristic` array provides flexible key-value attributes:

### Common Characteristics

| Name | Description | Example Value |
|------|-------------|---------------|
| `provider` | Infrastructure provider | `kubernetes`, `openstack`, `aws` |
| `type` | Resource type | `compute-pool`, `storage-pool` |
| `version` | Resource version | `1.28.0` |
| `capacity.cpu` | CPU capacity | `240` (cores) |
| `capacity.memory` | Memory capacity | `960Gi` |
| `capacity.storage` | Storage capacity | `10Ti` |
| `usage.cpu` | CPU usage | `180` (cores) |
| `usage.memory` | Memory usage | `720Gi` |

### Provider-Specific Characteristics

**Kubernetes:**
- `nodes`: Number of nodes
- `architecture`: CPU architecture (amd64, arm64)
- `os`: Operating system
- `container_runtime`: Container runtime (containerd, cri-o)

**OpenStack:**
- `hypervisor`: Hypervisor type
- `availability_zone`: OpenStack AZ
- `project_id`: Tenant/project ID

**Cloud Providers (AWS/Azure/GCP):**
- `region`: Cloud region
- `instance_type`: Instance type
- `spot`: Whether spot/preemptible instance

## Place References

The `place` array indicates resource location:

```json
{
  "place": [
    {
      "id": "us-east-1",
      "href": "/tmf-api/geographicAddressManagement/v4/geographicAddress/us-east-1",
      "name": "US East Region",
      "role": "DataCenter"
    }
  ]
}
```

## Examples

### List Resource Pools Only

```bash
curl -X GET "https://gateway.example.com/tmf-api/resourceInventoryManagement/v4/resource?category=resourcePool" \
  -H "Authorization: Bearer <token>"
```

### Filter by Operational State

```bash
curl -X GET "https://gateway.example.com/tmf-api/resourceInventoryManagement/v4/resource?operationalState=enabled" \
  -H "Authorization: Bearer <token>"
```

### Get Specific Fields Only

```bash
curl -X GET "https://gateway.example.com/tmf-api/resourceInventoryManagement/v4/resource?fields=id,name,resourceStatus" \
  -H "Authorization: Bearer <token>"
```

### Create Edge Resource Pool

```bash
curl -X POST "https://gateway.example.com/tmf-api/resourceInventoryManagement/v4/resource" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "5G Edge Pool",
    "description": "Edge compute for 5G RAN",
    "category": "resourcePool",
    "place": [{"id": "cell-tower-42"}],
    "resourceCharacteristic": [
      {"name": "type", "value": "edge-pool"},
      {"name": "latency_class", "value": "ultra-low"}
    ]
  }'
```

### Update Resource Description

```bash
curl -X PATCH "https://gateway.example.com/tmf-api/resourceInventoryManagement/v4/resource/pool-edge-42" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Updated: 5G MEC infrastructure"
  }'
```

## Integration with Other APIs

### TMF638 Service Inventory

Services reference resources they're deployed on:

```json
{
  "id": "service-123",
  "name": "5G Core UPF",
  "serviceCharacteristic": [
    {"name": "deployed_on_resource", "value": "pool-us-east-1"}
  ]
}
```

### TMF688 Event Management

Subscribe to resource lifecycle events:

```bash
curl -X POST "https://gateway.example.com/tmf-api/eventManagement/v4/hub" \
  -H "Content-Type: application/json" \
  -d '{
    "callback": "https://consumer.example.com/webhook",
    "query": "eventType=ResourceCreationNotification"
  }'
```

Event notification example:
```json
{
  "eventType": "ResourceCreationNotification",
  "eventTime": "2026-01-16T10:30:00Z",
  "event": {
    "resource": {
      "id": "node-new-1",
      "name": "worker-16.us-east-1.local",
      "category": "compute",
      "resourceStatus": "available"
    }
  }
}
```

## Best Practices

1. **Use category filtering**: Filter by `category=resourcePool` when you only need pools
2. **Leverage characteristics**: Use `resourceCharacteristic` for custom metadata
3. **Monitor operational state**: Check `operationalState` for runtime health
4. **Subscribe to events**: Use TMF688 to get notified of resource changes
5. **Pagination for large datasets**: Use `offset` and `limit` for efficient data retrieval

## Limitations

- Resource creation is limited to resource pools (individual resources are managed by infrastructure)
- Delete operations may have backend-specific constraints
- Some O2-IMS fields may not have direct TMForum equivalents (stored in characteristics)

## See Also

- [TMForum API Overview](README.md)
- [TMF638 Service Inventory API](tmf638-service-inventory.md)
- [TMF688 Event Management](README.md#event-management-tmf688)
- [API Mapping Guide](../../api-mapping.md)
- [O2-IMS Resource Management](../o2ims/resource-management.md)
