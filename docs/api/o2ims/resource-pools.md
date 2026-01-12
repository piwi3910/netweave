# Resource Pools API

Resource Pool represents a logical grouping of compute resources (nodes/machines).

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

### Attributes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resourcePoolId` | string | ✅ (auto-generated) | Unique ID |
| `name` | string | ✅ | Pool name (max 255 chars) |
| `description` | string | ❌ | Description (max 1000 chars) |
| `location` | string | ❌ | Physical location |
| `oCloudId` | string | ❌ | Parent O-Cloud |
| `globalLocationId` | string | ❌ | Geographic coordinates (geo:lat,lon) |
| `extensions` | object | ❌ | Additional metadata (max 50KB) |

## Kubernetes Mapping

**Primary**: `MachineSet` (OpenShift) or `NodePool` (Cluster API)

### MachineSet Example

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

## API Operations

### List Resource Pools

```http
GET /o2ims-infrastructureInventory/v1/resourcePools HTTP/1.1
Accept: application/json
```

**Query Parameters**:
- `location` (string): Filter by location prefix
- `oCloudId` (string): Filter by O-Cloud ID
- `limit` (int): Max results (default: 100)
- `offset` (int): Pagination offset (default: 0)

**Response (200 OK)**:
```json
{
  "resourcePools": [
    {
      "resourcePoolId": "pool-compute-high-mem",
      "name": "High Memory Compute Pool",
      "description": "Nodes with 128GB+ RAM",
      "location": "us-east-1a",
      "oCloudId": "ocloud-1",
      "extensions": {
        "machineType": "m5.4xlarge",
        "replicas": 5
      }
    }
  ],
  "total": 1
}
```

**Kubernetes Action**: List MachineSets with label filters

### Get Resource Pool

```http
GET /o2ims-infrastructureInventory/v1/resourcePools/{id} HTTP/1.1
Accept: application/json
```

**Response (200 OK)**:
```json
{
  "resourcePoolId": "pool-compute-high-mem",
  "name": "High Memory Compute Pool",
  "description": "Nodes with 128GB+ RAM",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1",
  "globalLocationId": "geo:37.7749,-122.4194",
  "extensions": {
    "machineType": "m5.4xlarge",
    "replicas": 5,
    "volumeSize": 120
  }
}
```

**Kubernetes Action**: Get MachineSet by name

**Error Response (404 Not Found)**:
```json
{
  "error": "NotFound",
  "message": "Resource pool not found: pool-nonexistent",
  "code": 404
}
```

### Create Resource Pool

```http
POST /o2ims-infrastructureInventory/v1/resourcePools HTTP/1.1
Content-Type: application/json

{
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

**Response (201 Created)**:
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

**Kubernetes Action**: Create MachineSet with generated name

**Headers**:
```
Location: /o2ims-infrastructureInventory/v1/resourcePools/pool-gpu-pool--production--a1b2c3d4-e5f6-7890-abcd-1234567890ab
```

### Update Resource Pool

```http
PUT /o2ims-infrastructureInventory/v1/resourcePools/{id} HTTP/1.1
Content-Type: application/json

{
  "name": "Updated Pool Name",
  "description": "Updated description",
  "extensions": {
    "replicas": 10
  }
}
```

**Response (200 OK)**:
```json
{
  "resourcePoolId": "pool-compute-high-mem",
  "name": "Updated Pool Name",
  "description": "Updated description",
  "location": "us-east-1a",
  "oCloudId": "ocloud-1",
  "extensions": {
    "replicas": 10
  }
}
```

**Kubernetes Action**: Update MachineSet annotations and spec.replicas

### Delete Resource Pool

```http
DELETE /o2ims-infrastructureInventory/v1/resourcePools/{id} HTTP/1.1
```

**Response (204 No Content)**: Empty body

**Kubernetes Action**: Delete MachineSet (cascading delete of Machines/Nodes)

**Error Response (404 Not Found)**:
```json
{
  "error": "NotFound",
  "message": "Resource pool not found: pool-nonexistent",
  "code": 404
}
```

## Validation and Error Handling

### Input Validation

**Field Validation**:
- `name` - **Required**, maximum 255 characters
- `resourcePoolId` - Optional on create (auto-generated), maximum 255 characters, alphanumeric with hyphens/underscores only
- `description` - Optional, maximum 1000 characters
- `extensions` - Optional, limited to 50KB total payload size

### ID Generation

When `resourcePoolId` is not provided on create, it's auto-generated:

**Sanitization Rules**:
1. Spaces and special characters (`/`, `\`, `..`, `:`, `*`, `?`, `"`, `<`, `>`, `|`) → hyphens
2. Non-alphanumeric characters (except hyphens/underscores) → removed
3. Convert to lowercase
4. Prefix: `pool-`
5. Suffix: Full UUID (36 characters)

**Example**:
```
"GPU Pool (Production)" → "pool-gpu-pool--production--a1b2c3d4-e5f6-7890-abcd-1234567890ab"
```

### UUID Design Rationale

**Format**: RFC 4122 compliant UUIDs (36 characters)

**Why Full UUIDs?**
- **Standard Compliance**: RFC 4122 UUID v4 format (universally recognized)
- **Collision Resistance**: 2^122 possible combinations (effectively zero collision probability)
- **Simplicity**: No custom truncation logic needed
- **Compatibility**: Works with all UUID-aware systems and tools
- **Maintainability**: Standard format is predictable and well-documented

### HTTP Status Codes

**POST /resourcePools**
- `201 Created` - Resource pool successfully created
- `400 Bad Request` - Invalid request body or validation errors
- `409 Conflict` - Resource pool with specified ID already exists
- `500 Internal Server Error` - Backend adapter error

**PUT /resourcePools/{id}**
- `200 OK` - Resource pool successfully updated
- `400 Bad Request` - Invalid request body or validation errors
- `404 Not Found` - Resource pool does not exist
- `500 Internal Server Error` - Backend adapter error

**DELETE /resourcePools/{id}**
- `204 No Content` - Resource pool successfully deleted
- `404 Not Found` - Resource pool does not exist
- `500 Internal Server Error` - Backend adapter error

**GET /resourcePools/{id}**
- `200 OK` - Resource pool found and returned
- `404 Not Found` - Resource pool does not exist
- `500 Internal Server Error` - Backend adapter error

**GET /resourcePools**
- `200 OK` - List of resource pools returned (may be empty)
- `400 Bad Request` - Invalid query parameters
- `500 Internal Server Error` - Backend adapter error

## Backend-Specific Mappings

### Kubernetes Adapter

See [Kubernetes Mapping](#kubernetes-mapping) above.

### Dell DTIAS Adapter

**DTIAS Resource**: Server Pool

**API Endpoint**: `GET /v2/inventory/resourcepools`

**Transformation**:
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

**DTIAS API Flow**:
```
1. O2-IMS Request: POST /resourcePools
2. Route to DTIAS adapter (location=dc-*)
3. Transform to DTIAS format
4. POST https://dtias.example.com/v2/resources/allocate
5. Parse wrapped response (ServerPool object)
6. Transform to O2-IMS ResourcePool
7. Return to client
```

### AWS EKS Adapter

**AWS Resource**: EKS NodeGroup

**API Method**: `DescribeNodegroup`

**Transformation**:
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
            "instanceTypes":   ng.InstanceTypes,
            "amiType":         *ng.AmiType,
            "diskSize":        *ng.DiskSize,
            "capacityType":    *ng.CapacityType,  // ON_DEMAND or SPOT
            "scalingConfig": map[string]interface{}{
                "minSize":     *ng.ScalingConfig.MinSize,
                "maxSize":     *ng.ScalingConfig.MaxSize,
                "desiredSize": *ng.ScalingConfig.DesiredSize,
            },
        },
    }
}
```

**AWS API Flow**:
```
1. O2-IMS Request: POST /resourcePools
2. Route to AWS adapter (location=aws-*)
3. Create EKS NodeGroup via AWS SDK
4. Transform AWS response to O2-IMS ResourcePool
5. Return to client
```

### OpenStack Adapter

**OpenStack Resource**: Host Aggregate

**API Endpoint**: `GET /v2.1/os-aggregates`

**Transformation**:
```go
func (a *OpenStackAdapter) transformAggregateToResourcePool(
    agg *aggregates.Aggregate,
) *models.ResourcePool {
    return &models.ResourcePool{
        ResourcePoolID: fmt.Sprintf("openstack-aggregate-%d", agg.ID),
        Name:           agg.Name,
        Description:    "OpenStack host aggregate",
        Location:       agg.AvailabilityZone,
        OCloudID:       a.oCloudID,
        Extensions: map[string]interface{}{
            "infrastructure": "openstack",
            "availabilityZone": agg.AvailabilityZone,
            "hosts":           agg.Hosts,
            "metadata":        agg.Metadata,
        },
    }
}
```

## Related Documentation

- [O2-IMS Overview](README.md)
- [Resources](resources.md)
- [Resource Types](resource-types.md)
- [Backend Plugins](../../backend-plugins.md)
