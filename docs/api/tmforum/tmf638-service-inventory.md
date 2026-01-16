# TMF638 - Service Inventory Management API v4

## Overview

The TMF638 Service Inventory Management API provides standardized access to deployed services and their lifecycle. This implementation maps TMForum services to O-RAN O2-DMS deployments, providing TMForum-compliant access to workload deployments across all backend adapters (Kubernetes, Helm, ArgoCD, ONAP, etc.).

**Base Path**: `/tmf-api/serviceInventoryManagement/v4`

**TMForum Specification**: [TMF638 v4.0](https://www.tmforum.org/resources/specification/tmf638-service-inventory-management-api-rest-specification-r19-0-1/)

## Service Model

### TMF638Service

The core service model representing deployed workloads:

```json
{
  "id": "string",
  "href": "string",
  "name": "string",
  "description": "string",
  "serviceType": "string",
  "state": "string",
  "category": "string",
  "startDate": "2026-01-16T10:00:00Z",
  "endDate": "2026-01-16T10:00:00Z",
  "serviceSpecification": {
    "id": "string",
    "href": "string",
    "name": "string",
    "version": "string"
  },
  "serviceCharacteristic": [
    {
      "name": "string",
      "value": "string",
      "valueType": "string"
    }
  ],
  "place": [
    {
      "id": "string",
      "href": "string",
      "name": "string",
      "role": "string"
    }
  ],
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
  "@baseType": "Service",
  "@schemaLocation": "string"
}
```

### Key Fields

| Field | Type | Description | Mapping |
|-------|------|-------------|---------|
| `id` | string | Unique service identifier | O2-DMS deploymentId |
| `name` | string | Service name | O2-DMS name |
| `state` | string | Service lifecycle state | Maps from O2-DMS status |
| `serviceType` | string | Type of service | Derived from deployment type |
| `serviceSpecification.id` | string | Service blueprint reference | O2-DMS packageId |
| `serviceCharacteristic` | array | Service configuration | O2-DMS extensions + parameters |
| `startDate` | datetime | Service start time | O2-DMS createdAt |

## API Endpoints

### List Services

Retrieve a list of all deployed services.

**Request:**
```http
GET /tmf-api/serviceInventoryManagement/v4/service HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `state` | string | Filter by service state (e.g., `pending`, `activated`, `failed`) |
| `serviceType` | string | Filter by service type |
| `category` | string | Filter by service category |
| `offset` | integer | Pagination offset (default: 0) |
| `limit` | integer | Items per page (default: 100, max: 500) |
| `fields` | string | Comma-separated list of fields to include |

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

[
  {
    "id": "dep-5g-upf-001",
    "href": "/tmf-api/serviceInventoryManagement/v4/service/dep-5g-upf-001",
    "name": "5G UPF Deployment",
    "description": "5G User Plane Function for enterprise slice",
    "serviceType": "network-function",
    "state": "activated",
    "category": "5G Core",
    "startDate": "2026-01-15T08:30:00Z",
    "serviceSpecification": {
      "id": "upf-helm-chart",
      "name": "5G UPF Helm Chart",
      "version": "2.1.0"
    },
    "serviceCharacteristic": [
      {"name": "replicas", "value": "3", "valueType": "integer"},
      {"name": "slice_id", "value": "ent-slice-001", "valueType": "string"},
      {"name": "dnn", "value": "internet", "valueType": "string"}
    ],
    "place": [
      {
        "id": "us-east-1",
        "name": "US East Data Center"
      }
    ],
    "@type": "NetworkFunction",
    "@baseType": "Service"
  },
  {
    "id": "dep-edge-app-42",
    "href": "/tmf-api/serviceInventoryManagement/v4/service/dep-edge-app-42",
    "name": "AR Gaming Application",
    "serviceType": "edge-application",
    "state": "activated",
    "startDate": "2026-01-16T09:15:00Z",
    "serviceSpecification": {
      "id": "ar-gaming-app",
      "version": "1.0.3"
    },
    "serviceCharacteristic": [
      {"name": "latency_requirement", "value": "10ms", "valueType": "string"},
      {"name": "gpu_required", "value": "true", "valueType": "boolean"}
    ],
    "@type": "EdgeApplication",
    "@baseType": "Service"
  }
]
```

### Get Service by ID

Retrieve details of a specific service.

**Request:**
```http
GET /tmf-api/serviceInventoryManagement/v4/service/{id} HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
```

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Service identifier (deployment ID) |

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "dep-5g-upf-001",
  "href": "/tmf-api/serviceInventoryManagement/v4/service/dep-5g-upf-001",
  "name": "5G UPF Deployment",
  "description": "5G User Plane Function for enterprise slice",
  "serviceType": "network-function",
  "state": "activated",
  "category": "5G Core",
  "startDate": "2026-01-15T08:30:00Z",
  "serviceSpecification": {
    "id": "upf-helm-chart",
    "href": "/tmf-api/productCatalog/v4/productOffering/upf-helm-chart",
    "name": "5G UPF Helm Chart",
    "version": "2.1.0"
  },
  "serviceCharacteristic": [
    {"name": "replicas", "value": "3", "valueType": "integer"},
    {"name": "slice_id", "value": "ent-slice-001", "valueType": "string"},
    {"name": "dnn", "value": "internet", "valueType": "string"},
    {"name": "cpu_request", "value": "2000m", "valueType": "string"},
    {"name": "memory_request", "value": "4Gi", "valueType": "string"}
  ],
  "place": [
    {
      "id": "us-east-1",
      "name": "US East Data Center",
      "role": "DeploymentSite"
    }
  ],
  "relatedParty": [
    {
      "id": "pool-us-east-1",
      "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-us-east-1",
      "role": "ResourcePool"
    }
  ],
  "@type": "NetworkFunction",
  "@baseType": "Service"
}
```

**Error Response:**
```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "error": "NotFound",
  "message": "Service with ID 'invalid-id' not found"
}
```

### Create Service

Deploy a new service.

**Request:**
```http
POST /tmf-api/serviceInventoryManagement/v4/service HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Enterprise 5G AMF",
  "description": "AMF for enterprise network slice",
  "serviceType": "network-function",
  "category": "5G Core",
  "serviceSpecification": {
    "id": "amf-helm-v2",
    "version": "2.0.1"
  },
  "serviceCharacteristic": [
    {"name": "replicas", "value": "2"},
    {"name": "slice_id", "value": "ent-slice-001"},
    {"name": "plmn", "value": "00101"}
  ],
  "place": [
    {"id": "us-east-1"}
  ]
}
```

**Response:**
```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: /tmf-api/serviceInventoryManagement/v4/service/dep-amf-ent-001

{
  "id": "dep-amf-ent-001",
  "href": "/tmf-api/serviceInventoryManagement/v4/service/dep-amf-ent-001",
  "name": "Enterprise 5G AMF",
  "description": "AMF for enterprise network slice",
  "serviceType": "network-function",
  "state": "pending",
  "category": "5G Core",
  "startDate": "2026-01-16T10:45:00Z",
  "serviceSpecification": {
    "id": "amf-helm-v2",
    "version": "2.0.1"
  },
  "serviceCharacteristic": [
    {"name": "replicas", "value": "2"},
    {"name": "slice_id", "value": "ent-slice-001"},
    {"name": "plmn", "value": "00101"}
  ],
  "place": [
    {"id": "us-east-1", "name": "US East Data Center"}
  ],
  "@type": "NetworkFunction",
  "@baseType": "Service"
}
```

### Update Service

Update service configuration (PATCH semantics).

**Request:**
```http
PATCH /tmf-api/serviceInventoryManagement/v4/service/{id} HTTP/1.1
Host: gateway.example.com
Authorization: Bearer <token>
Content-Type: application/json

{
  "description": "Updated description",
  "serviceCharacteristic": [
    {"name": "replicas", "value": "5"}
  ]
}
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "dep-5g-upf-001",
  "href": "/tmf-api/serviceInventoryManagement/v4/service/dep-5g-upf-001",
  "name": "5G UPF Deployment",
  "description": "Updated description",
  "state": "activated",
  "serviceCharacteristic": [
    {"name": "replicas", "value": "5"},
    {"name": "slice_id", "value": "ent-slice-001"}
  ],
  "@type": "NetworkFunction"
}
```

### Delete Service

Delete (undeploy) a service.

**Request:**
```http
DELETE /tmf-api/serviceInventoryManagement/v4/service/{id} HTTP/1.1
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
  "message": "Service with ID 'invalid-id' not found"
}
```

## Service States

The `state` field indicates the service lifecycle:

| State | Description | O2-DMS Mapping |
|-------|-------------|----------------|
| `pending` | Deployment queued | Pending |
| `inProgress` | Deployment in progress | Installing, Upgrading |
| `activated` | Service running | Installed |
| `failed` | Deployment failed | Failed |
| `terminated` | Service stopped | - |

State transitions:
```
pending → inProgress → activated
        ↓
      failed
```

## Service Types

Common service type values:

| Type | Description | Example Use Case |
|------|-------------|------------------|
| `network-function` | 5G/Telecom NFs | UPF, AMF, SMF, UDM |
| `edge-application` | MEC applications | AR/VR, gaming, video processing |
| `infrastructure-service` | Platform services | Monitoring, logging, service mesh |
| `data-service` | Data processing | Analytics, ML inference |

## Service Characteristics

The `serviceCharacteristic` array provides deployment configuration:

### Common Characteristics

| Name | Description | Example Value |
|------|-------------|---------------|
| `replicas` | Number of instances | `3` |
| `version` | Service version | `2.1.0` |
| `namespace` | Kubernetes namespace | `5g-core` |
| `cpu_request` | CPU request | `2000m` |
| `memory_request` | Memory request | `4Gi` |
| `cpu_limit` | CPU limit | `4000m` |
| `memory_limit` | Memory limit | `8Gi` |

### 5G Network Function Characteristics

| Name | Description | Example Value |
|------|-------------|---------------|
| `slice_id` | Network slice identifier | `ent-slice-001` |
| `plmn` | PLMN identifier | `00101` |
| `dnn` | Data Network Name | `internet`, `ims` |
| `nssai` | Network Slice Selection Assistance Information | `sst:1,sd:000001` |

### Edge Application Characteristics

| Name | Description | Example Value |
|------|-------------|---------------|
| `latency_requirement` | Max latency | `10ms`, `100ms` |
| `gpu_required` | Requires GPU | `true`, `false` |
| `gpu_type` | GPU type | `nvidia-t4` |
| `bandwidth_requirement` | Network bandwidth | `1Gbps` |

## Service Specifications

The `serviceSpecification` references the deployment package/blueprint:

```json
{
  "serviceSpecification": {
    "id": "upf-helm-chart",
    "href": "/tmf-api/productCatalog/v4/productOffering/upf-helm-chart",
    "name": "5G UPF Helm Chart",
    "version": "2.1.0"
  }
}
```

This maps to:
- **O2-DMS**: packageId + version
- **Helm**: Chart name + version
- **ArgoCD**: Application template
- **ONAP**: VNF/NS package

## Examples

### List Services by State

```bash
curl -X GET "https://gateway.example.com/tmf-api/serviceInventoryManagement/v4/service?state=activated" \
  -H "Authorization: Bearer <token>"
```

### Filter by Service Type

```bash
curl -X GET "https://gateway.example.com/tmf-api/serviceInventoryManagement/v4/service?serviceType=network-function" \
  -H "Authorization: Bearer <token>"
```

### Get Specific Fields Only

```bash
curl -X GET "https://gateway.example.com/tmf-api/serviceInventoryManagement/v4/service?fields=id,name,state" \
  -H "Authorization: Bearer <token>"
```

### Deploy 5G NF Service

```bash
curl -X POST "https://gateway.example.com/tmf-api/serviceInventoryManagement/v4/service" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "SMF for Slice 1",
    "serviceType": "network-function",
    "serviceSpecification": {"id": "smf-helm", "version": "1.5.2"},
    "serviceCharacteristic": [
      {"name": "replicas", "value": "2"},
      {"name": "slice_id", "value": "slice-001"},
      {"name": "dnn", "value": "internet"}
    ]
  }'
```

### Scale Service (Update Replicas)

```bash
curl -X PATCH "https://gateway.example.com/tmf-api/serviceInventoryManagement/v4/service/dep-smf-001" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "serviceCharacteristic": [
      {"name": "replicas", "value": "5"}
    ]
  }'
```

### Deploy Edge Application

```bash
curl -X POST "https://gateway.example.com/tmf-api/serviceInventoryManagement/v4/service" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Video Analytics App",
    "serviceType": "edge-application",
    "serviceSpecification": {"id": "video-analytics", "version": "3.2.1"},
    "place": [{"id": "edge-chicago"}],
    "serviceCharacteristic": [
      {"name": "gpu_required", "value": "true"},
      {"name": "latency_requirement", "value": "20ms"}
    ]
  }'
```

## Integration with Other APIs

### TMF641 Service Ordering

Create service via service order:

```bash
curl -X POST "https://gateway.example.com/tmf-api/serviceOrdering/v4/serviceOrder" \
  -H "Content-Type: application/json" \
  -d '{
    "serviceOrderItem": [
      {
        "action": "add",
        "service": {
          "serviceSpecification": {"id": "upf-helm"},
          "serviceCharacteristic": [
            {"name": "replicas", "value": "3"}
          ]
        }
      }
    ]
  }'
```

### TMF639 Resource Inventory

Services reference underlying resources:

```json
{
  "relatedParty": [
    {
      "id": "pool-us-east-1",
      "href": "/tmf-api/resourceInventoryManagement/v4/resource/pool-us-east-1",
      "role": "ResourcePool"
    }
  ]
}
```

### TMF688 Event Management

Subscribe to service state changes:

```bash
curl -X POST "https://gateway.example.com/tmf-api/eventManagement/v4/hub" \
  -H "Content-Type: application/json" \
  -d '{
    "callback": "https://consumer.example.com/webhook",
    "query": "eventType=ServiceStateChangeNotification"
  }'
```

## Best Practices

1. **Use service specifications**: Always reference a valid `serviceSpecification.id`
2. **Monitor service state**: Check `state` field for deployment status
3. **Leverage characteristics**: Use `serviceCharacteristic` for flexible configuration
4. **Subscribe to events**: Use TMF688 to track service lifecycle
5. **Namespace organization**: Use characteristics to specify Kubernetes namespaces
6. **Resource planning**: Check resource availability (TMF639) before deploying

## Limitations

- Service updates (PATCH) have limited support depending on backend adapter capabilities
- Some deployment parameters may be read-only after creation
- Delete operations may fail if service has dependencies
- State transitions depend on backend infrastructure responsiveness

## See Also

- [TMForum API Overview](README.md)
- [TMF639 Resource Inventory API](tmf639-resource-inventory.md)
- [TMF641 Service Ordering Management](README.md)
- [TMF688 Event Management](README.md#event-management-tmf688)
- [API Mapping Guide](../../api-mapping.md)
- [O2-DMS Deployment Management](../o2dms/deployment-management.md)
