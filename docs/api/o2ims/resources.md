# Resources API

Resource represents an individual compute resource (node/machine/server) within a resource pool.

## Table of Contents

1. [O2-IMS Specification](#o2-ims-specification)
2. [Kubernetes Mapping](#kubernetes-mapping)
3. [API Operations](#api-operations)
4. [Validation and Error Handling](#validation-and-error-handling)
5. [Backend-Specific Mappings](#backend-specific-mappings)

## O2-IMS Specification

### Resource Model

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

### Attributes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resourceId` | string | ✅ (auto-generated) | Unique ID (UUID format) |
| `resourceTypeId` | string | ✅ | Type identifier |
| `resourcePoolId` | string | ✅ | Parent pool ID |
| `globalAssetId` | string | ❌ | URN format: `urn:o-ran:*` |
| `description` | string | ❌ | Description (max 1000 chars) |
| `extensions` | object | ❌ | Additional metadata (max 50KB) |

## Kubernetes Mapping

**Primary**: `Node` (for running nodes) or `Machine` (for lifecycle)

### Node Example

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

## API Operations

### List Resources

```http
GET /o2ims-infrastructureInventory/v1/resources HTTP/1.1
Accept: application/json
```

**Query Parameters**:
- `resourcePoolId` (string): Filter by pool
- `resourceTypeId` (string): Filter by type
- `limit` (int): Max results (default: 100)
- `offset` (int): Pagination offset (default: 0)

**Response (200 OK)**:
```json
{
  "resources": [
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
        "memory": "64 GB"
      }
    }
  ],
  "total": 1
}
```

**Kubernetes Action**: List Nodes (or Machines) with label filters

### Get Resource

```http
GET /o2ims-infrastructureInventory/v1/resources/{id} HTTP/1.1
Accept: application/json
```

**Response (200 OK)**:
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
    "cpu": "16",
    "memory": "64Gi",
    "zone": "us-east-1a",
    "allocatable": {
      "cpu": "15500m",
      "memory": "60Gi",
      "pods": "110"
    },
    "nodeInfo": {
      "kubeletVersion": "v1.30.0",
      "osImage": "Red Hat Enterprise Linux CoreOS",
      "kernelVersion": "5.14.0-284.el9.x86_64"
    }
  }
}
```

**Kubernetes Action**: Get Node by name

**Error Response (404 Not Found)**:
```json
{
  "error": "NotFound",
  "message": "Resource not found: node-nonexistent",
  "code": 404
}
```

### Create Resource

```http
POST /o2ims-infrastructureInventory/v1/resources HTTP/1.1
Content-Type: application/json

{
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

**Response (201 Created)**:
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
    "team": "ml-platform",
    "status": "Provisioning"
  }
}
```

**Kubernetes Action**: Create Machine (triggers Node provisioning)

**Headers**:
```
Location: /o2ims-infrastructureInventory/v1/resources/a1b2c3d4-e5f6-7890-abcd-1234567890ab
```

**Kubernetes Side Effects**:

After Machine is created, the cluster provisions a Node:

```yaml
# Machine created immediately
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  name: pool-production-us-west-2-xyz123
  namespace: openshift-machine-api
  labels:
    machine.openshift.io/cluster-api-machineset: pool-production-us-west-2
    o2ims.oran.org/resource-id: a1b2c3d4-e5f6-7890-abcd-1234567890ab
  annotations:
    o2ims.oran.org/global-asset-id: "urn:o-ran:resource:node-prod-ai-001"
    o2ims.oran.org/description: "Production workload node for AI training"
spec:
  providerSpec:
    value:
      instanceType: m5.4xlarge
      placement:
        availabilityZone: us-west-2a

# Node appears after ~5 minutes
apiVersion: v1
kind: Node
metadata:
  name: ip-10-0-1-123.ec2.internal
  labels:
    o2ims.oran.org/resource-id: a1b2c3d4-e5f6-7890-abcd-1234567890ab
    o2ims.oran.org/resource-pool-id: pool-production-us-west-2
status:
  conditions:
    - type: Ready
      status: "True"
  capacity:
    cpu: "64"
    memory: 512Gi
```

### Update Resource

```http
PUT /o2ims-infrastructureInventory/v1/resources/{id} HTTP/1.1
Content-Type: application/json

{
  "description": "Updated description",
  "globalAssetId": "urn:o-ran:resource:node-updated-001",
  "extensions": {
    "purpose": "updated-purpose"
  }
}
```

**Response (200 OK)**:
```json
{
  "resourceId": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
  "resourceTypeId": "compute-node-standard",
  "resourcePoolId": "pool-production-us-west-2",
  "description": "Updated description",
  "globalAssetId": "urn:o-ran:resource:node-updated-001",
  "extensions": {
    "purpose": "updated-purpose"
  }
}
```

**Kubernetes Action**: Update Machine/Node annotations and labels (mutable fields only)

**Note**: Only `description`, `globalAssetId`, and `extensions` can be updated. Fields like `resourceTypeId` and `resourcePoolId` are immutable.

### Delete Resource

```http
DELETE /o2ims-infrastructureInventory/v1/resources/{id} HTTP/1.1
```

**Response (204 No Content)**: Empty body

**Kubernetes Action**: Delete Machine or drain+delete Node

**Note**: Deletion may take several minutes as workloads are drained from the node.

## Validation and Error Handling

### Input Validation

**Field Validation**:
- `resourceTypeId` - **Required** (400 if missing)
- `resourcePoolId` - **Required** (400 if missing)
- `resourceId` - Optional on create (auto-generated as plain UUID)
- `description` - Optional, maximum 1000 characters
- `globalAssetId` - Optional, must start with `urn:` if provided
- `extensions` - Optional, limited to 50KB total payload size

### ID Generation

When `resourceId` is not provided on create:
```
Format: Plain UUID (no prefix)
Example: "a1b2c3d4-e5f6-7890-abcd-1234567890ab"
```

### Error Scenarios

**Missing Required Field**:
```http
POST /o2ims-infrastructureInventory/v1/resources
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
POST /o2ims-infrastructureInventory/v1/resources
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
POST /o2ims-infrastructureInventory/v1/resources
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

### HTTP Status Codes

**POST /resources**
- `201 Created` - Resource successfully created
- `400 Bad Request` - Invalid request body or validation errors
- `409 Conflict` - Resource with specified ID already exists
- `500 Internal Server Error` - Backend adapter error

**PUT /resources/{id}**
- `200 OK` - Resource successfully updated
- `400 Bad Request` - Invalid request body or validation errors
- `404 Not Found` - Resource does not exist
- `500 Internal Server Error` - Backend adapter error

**DELETE /resources/{id}**
- `204 No Content` - Resource successfully deleted
- `404 Not Found` - Resource does not exist
- `500 Internal Server Error` - Backend adapter error

**GET /resources/{id}**
- `200 OK` - Resource found and returned
- `404 Not Found` - Resource does not exist
- `500 Internal Server Error` - Backend adapter error

**GET /resources**
- `200 OK` - List of resources returned (may be empty)
- `400 Bad Request` - Invalid query parameters
- `500 Internal Server Error` - Backend adapter error

## Backend-Specific Mappings

### Kubernetes Adapter

See [Kubernetes Mapping](#kubernetes-mapping) above.

### Dell DTIAS Adapter

**DTIAS Resource**: Physical Server

**API Endpoint**: `GET /v2/inventory/servers`

**Note**: `GET /v2/inventory/servers/{Id}` returns `JobResponse` (async operation status), not server data. Use `GET /v2/inventory/servers?id={id}` to retrieve a specific server.

**Transformation**:
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
            "storage": server.Storage,
            "network": server.NICs,
            "status":  server.Status,
            "power":   server.PowerState,
        },
    }
}
```

### AWS EKS Adapter

**AWS Resource**: EC2 Instance

**API Method**: `DescribeInstances`

**Transformation**:
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
            "state":           *instance.State.Name,
            "privateIp":       *instance.PrivateIpAddress,
            "publicIp":        getPublicIP(instance),
            "vpcId":           *instance.VpcId,
            "subnetId":        *instance.SubnetId,
            "architecture":    *instance.Architecture,
            "launchTime":      *instance.LaunchTime,
        },
    }
}
```

### OpenStack Adapter

**OpenStack Resource**: Compute Instance

**API Endpoint**: `GET /v2.1/servers`

**Transformation**:
```go
func (a *OpenStackAdapter) transformServerToResource(
    server *servers.Server,
) *models.Resource {
    return &models.Resource{
        ResourceID:     fmt.Sprintf("openstack-server-%s", server.ID),
        ResourceTypeID: fmt.Sprintf("openstack-flavor-%s", server.Flavor["id"]),
        ResourcePoolID: inferPoolFromServer(server),
        GlobalAssetID:  fmt.Sprintf("urn:openstack:server:%s", server.ID),
        Description:    server.Name,
        Extensions: map[string]interface{}{
            "infrastructure": "openstack",
            "status":        server.Status,
            "addresses":     server.Addresses,
            "flavor":        server.Flavor,
            "image":         server.Image,
            "created":       server.Created,
            "updated":       server.Updated,
        },
    }
}
```

## Related Documentation

- [O2-IMS Overview](README.md)
- [Resource Pools](resource-pools.md)
- [Resource Types](resource-types.md)
- [Backend Plugins](../../backend-plugins.md)
