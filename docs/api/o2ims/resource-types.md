# Resource Types API

Resource Type represents a hardware or software profile for compute resources.

## Table of Contents

1. [O2-IMS Specification](#o2-ims-specification)
2. [Kubernetes Mapping](#kubernetes-mapping)
3. [API Operations](#api-operations)
4. [Backend-Specific Mappings](#backend-specific-mappings)

## O2-IMS Specification

### Resource Model

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

### Attributes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resourceTypeId` | string | ✅ | Unique identifier |
| `name` | string | ✅ | Human-readable name |
| `description` | string | ❌ | Description |
| `vendor` | string | ❌ | Vendor name (AWS, Dell, etc.) |
| `model` | string | ❌ | Model/SKU identifier |
| `version` | string | ❌ | Version string |
| `resourceClass` | string | ✅ | Class: `compute`, `storage`, `network` |
| `resourceKind` | string | ✅ | Kind: `physical`, `virtual`, `logical` |
| `extensions` | object | ❌ | Additional specifications |

## Kubernetes Mapping

**No direct equivalent** - aggregate from:
1. **Node capacity** (CPU, memory, storage)
2. **StorageClasses** (storage types)
3. **Machine types** (if using cloud provider)

### Aggregation Logic

Resource types are dynamically discovered by analyzing existing resources:

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

### Example Transformation

**Kubernetes Node**:
```yaml
apiVersion: v1
kind: Node
metadata:
  name: ip-10-0-1-123.ec2.internal
  labels:
    node.kubernetes.io/instance-type: m5.4xlarge
status:
  capacity:
    cpu: "16"
    memory: 64Gi
    ephemeral-storage: 120Gi
  allocatable:
    cpu: "15500m"
    memory: 60Gi
```

**→ O2-IMS ResourceType**:
```json
{
  "resourceTypeId": "compute-m5.4xlarge",
  "name": "Compute m5.4xlarge",
  "description": "AWS EC2 m5.4xlarge instance",
  "vendor": "AWS",
  "model": "m5.4xlarge",
  "resourceClass": "compute",
  "resourceKind": "physical",
  "extensions": {
    "cpu": "16",
    "memory": "64Gi",
    "storage": "120Gi"
  }
}
```

**Kubernetes StorageClass**:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3-encrypted
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  encrypted: "true"
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
```

**→ O2-IMS ResourceType**:
```json
{
  "resourceTypeId": "storage-gp3-encrypted",
  "name": "gp3-encrypted",
  "description": "AWS EBS gp3 encrypted storage",
  "vendor": "AWS",
  "model": "gp3",
  "resourceClass": "storage",
  "resourceKind": "virtual",
  "extensions": {
    "provisioner": "ebs.csi.aws.com",
    "type": "gp3",
    "encrypted": "true",
    "volumeBindingMode": "WaitForFirstConsumer",
    "reclaimPolicy": "Delete"
  }
}
```

## API Operations

### List Resource Types

```http
GET /o2ims-infrastructureInventory/v1/resourceTypes HTTP/1.1
Accept: application/json
```

**Query Parameters**:
- `resourceClass` (string): Filter by class (`compute`, `storage`, `network`)
- `resourceKind` (string): Filter by kind (`physical`, `virtual`, `logical`)
- `vendor` (string): Filter by vendor name
- `limit` (int): Max results (default: 100)
- `offset` (int): Pagination offset (default: 0)

**Response (200 OK)**:
```json
{
  "resourceTypes": [
    {
      "resourceTypeId": "compute-m5.4xlarge",
      "name": "Compute m5.4xlarge",
      "description": "AWS EC2 m5.4xlarge instance",
      "vendor": "AWS",
      "model": "m5.4xlarge",
      "resourceClass": "compute",
      "resourceKind": "physical",
      "extensions": {
        "cpu": "16",
        "memory": "64Gi",
        "storage": "120Gi"
      }
    },
    {
      "resourceTypeId": "storage-gp3-encrypted",
      "name": "gp3-encrypted",
      "description": "AWS EBS gp3 encrypted storage",
      "vendor": "AWS",
      "model": "gp3",
      "resourceClass": "storage",
      "resourceKind": "virtual",
      "extensions": {
        "provisioner": "ebs.csi.aws.com",
        "type": "gp3",
        "encrypted": "true"
      }
    }
  ],
  "total": 2
}
```

**Kubernetes Action**: Aggregate from Nodes + StorageClasses

### Get Resource Type

```http
GET /o2ims-infrastructureInventory/v1/resourceTypes/{id} HTTP/1.1
Accept: application/json
```

**Response (200 OK)**:
```json
{
  "resourceTypeId": "compute-m5.4xlarge",
  "name": "Compute m5.4xlarge",
  "description": "AWS EC2 m5.4xlarge instance type with 16 vCPUs and 64GB RAM",
  "vendor": "AWS",
  "model": "m5.4xlarge",
  "version": "v1",
  "resourceClass": "compute",
  "resourceKind": "physical",
  "extensions": {
    "cpu": "16",
    "memory": "64Gi",
    "storage": "120Gi",
    "network": "Up to 10 Gbps",
    "architecture": "x86_64"
  }
}
```

**Kubernetes Action**: Get specific type info from aggregation

**Error Response (404 Not Found)**:
```json
{
  "error": "NotFound",
  "message": "Resource type not found: compute-nonexistent",
  "code": 404
}
```

### Operations Summary

| Operation | Method | Endpoint | K8s Action | Supported |
|-----------|--------|----------|------------|-----------|
| List | GET | `/resourceTypes` | Aggregate Nodes + StorageClasses | ✅ |
| Get | GET | `/resourceTypes/{id}` | Get specific type info | ✅ |
| Create | POST | `/resourceTypes` | N/A | ❌ Not supported (read-only) |
| Update | PUT | `/resourceTypes/{id}` | N/A | ❌ Not supported (read-only) |
| Delete | DELETE | `/resourceTypes/{id}` | N/A | ❌ Not supported (read-only) |

**Note**: Create/Update/Delete operations are not supported because resource types are dynamically discovered from existing infrastructure, not explicitly managed via the O2-IMS API.

## Backend-Specific Mappings

### Kubernetes Adapter

See [Kubernetes Mapping](#kubernetes-mapping) above.

**Sources**:
1. **Compute Types**: Node labels (`node.kubernetes.io/instance-type`) + capacity
2. **Storage Types**: StorageClass specs

### Dell DTIAS Adapter

**DTIAS Resource**: Server Profile

**API Endpoint**: `GET /v2/resourcetypes`

**Response Format**: Wrapped in `ResourceTypes` array

**Transformation**:
```go
func (a *DTIASAdapter) transformProfileToResourceType(
    profile *dtias.ServerProfile,
) *models.ResourceType {
    return &models.ResourceType{
        ResourceTypeID: fmt.Sprintf("dtias-profile-%s", profile.ID),
        Name:           profile.Name,
        Description:    profile.Description,
        Vendor:         "Dell",
        Model:          profile.HardwareModel,
        Version:        profile.Version,
        ResourceClass:  "compute",
        ResourceKind:   "physical",
        Extensions: map[string]interface{}{
            "infrastructure": "bare-metal",
            "cpu": map[string]interface{}{
                "model":   profile.CPU.Model,
                "cores":   profile.CPU.Cores,
                "threads": profile.CPU.Threads,
                "speed":   profile.CPU.SpeedGHz,
            },
            "memory": map[string]interface{}{
                "total": profile.Memory.TotalGB,
                "type":  profile.Memory.Type,
                "speed": profile.Memory.SpeedMHz,
            },
            "storage": profile.Storage,
            "network": profile.Network,
        },
    }
}
```

### AWS EKS Adapter

**AWS Resource**: EC2 Instance Type

**API Method**: `DescribeInstanceTypes`

**Transformation**:
```go
func (a *AWSAdapter) transformInstanceTypeToResourceType(
    it *ec2.InstanceTypeInfo,
) *models.ResourceType {
    return &models.ResourceType{
        ResourceTypeID: fmt.Sprintf("aws-instance-type-%s", *it.InstanceType),
        Name:           string(*it.InstanceType),
        Description:    fmt.Sprintf("AWS EC2 %s instance type", *it.InstanceType),
        Vendor:         "AWS",
        Model:          string(*it.InstanceType),
        ResourceClass:  "compute",
        ResourceKind:   "physical",
        Extensions: map[string]interface{}{
            "infrastructure": "cloud",
            "vcpu":          *it.VCpuInfo.DefaultVCpus,
            "memory":        fmt.Sprintf("%dMiB", *it.MemoryInfo.SizeInMiB),
            "architecture":  it.ProcessorInfo.SupportedArchitectures,
            "network":       *it.NetworkInfo.NetworkPerformance,
            "storage":       it.InstanceStorageInfo,
        },
    }
}
```

### OpenStack Adapter

**OpenStack Resource**: Flavor

**API Endpoint**: `GET /v2.1/flavors/detail`

**Transformation**:
```go
func (a *OpenStackAdapter) transformFlavorToResourceType(
    flavor *flavors.Flavor,
) *models.ResourceType {
    return &models.ResourceType{
        ResourceTypeID: fmt.Sprintf("openstack-flavor-%s", flavor.ID),
        Name:           flavor.Name,
        Description:    fmt.Sprintf("OpenStack flavor %s", flavor.Name),
        Vendor:         "OpenStack",
        Model:          flavor.Name,
        ResourceClass:  "compute",
        ResourceKind:   "virtual",
        Extensions: map[string]interface{}{
            "infrastructure": "openstack",
            "vcpus":         flavor.VCPUs,
            "ram":           flavor.RAM,
            "disk":          flavor.Disk,
            "ephemeral":     flavor.Ephemeral,
            "swap":          flavor.Swap,
        },
    }
}
```

### VMware Adapter

**VMware Resource**: Host Hardware Profile

**API**: vSphere API (HostSystem)

**Transformation**:
```go
func (a *VMwareAdapter) transformHostProfileToResourceType(
    host *vim25types.HostSystem,
) *models.ResourceType {
    hardware := host.Hardware
    return &models.ResourceType{
        ResourceTypeID: fmt.Sprintf("vmware-host-%s", hardware.SystemInfo.Model),
        Name:           hardware.SystemInfo.Model,
        Description:    fmt.Sprintf("VMware ESXi host %s", hardware.SystemInfo.Model),
        Vendor:         hardware.SystemInfo.Vendor,
        Model:          hardware.SystemInfo.Model,
        Version:        host.Config.Product.Version,
        ResourceClass:  "compute",
        ResourceKind:   "physical",
        Extensions: map[string]interface{}{
            "infrastructure": "virtualization",
            "cpuModel":      hardware.CpuPkg[0].Description,
            "cpuCores":      hardware.CpuInfo.NumCpuCores,
            "cpuThreads":    hardware.CpuInfo.NumCpuThreads,
            "cpuMHz":        hardware.CpuInfo.Hz / 1000000,
            "memoryBytes":   hardware.MemorySize,
        },
    }
}
```

## Discovery and Caching

### Discovery Strategy

Resource types are discovered dynamically:

1. **On Startup**: Perform initial scan of all nodes, storage classes, and cloud provider catalogs
2. **Periodic Refresh**: Re-scan every 5 minutes to detect new types
3. **Event-Driven**: Watch for Node/StorageClass creation events for immediate updates

### Caching

Resource types are cached in Redis with TTL:

```go
// Cache key format
key := fmt.Sprintf("resource-type:%s", resourceTypeID)

// Cache for 5 minutes
redis.Set(ctx, key, json.Marshal(resourceType), 5*time.Minute)

// Invalidate on Node/StorageClass changes
redis.Publish(ctx, "cache:invalidate:resourceTypes", resourceTypeID)
```

## Related Documentation

- [O2-IMS Overview](README.md)
- [Resource Pools](resource-pools.md)
- [Resources](resources.md)
- [Backend Plugins](../../backend-plugins.md)
