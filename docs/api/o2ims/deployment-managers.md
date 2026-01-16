# Deployment Manager API

Deployment Manager represents the O-Cloud infrastructure manager (typically a Kubernetes cluster).

## Table of Contents

1. [O2-IMS Specification](#o2-ims-specification)
2. [Kubernetes Mapping](#kubernetes-mapping)
3. [API Operations](#api-operations)
4. [Backend-Specific Mappings](#backend-specific-mappings)

## O2-IMS Specification

### Resource Model

```json
{
  "deploymentManagerId": "ocloud-k8s-1",
  "name": "US-East Kubernetes Cloud",
  "description": "Production Kubernetes cluster for RAN workloads",
  "oCloudId": "ocloud-1",
  "serviceUri": "https://api.o2ims.example.com/o2ims-infrastructureInventory/v1"
}
```

### Attributes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `deploymentManagerId` | string | ✅ | Unique identifier |
| `name` | string | ✅ | Human-readable name |
| `description` | string | ❌ | Description |
| `oCloudId` | string | ✅ | Parent O-Cloud ID |
| `serviceUri` | string | ✅ | API endpoint |
| `supportedLocations` | array | ❌ | Geographic locations |
| `capabilities` | object | ❌ | Supported capabilities |
| `extensions` | object | ❌ | Vendor extensions |

## Kubernetes Mapping

**No direct K8s equivalent** - use Custom Resource or ConfigMap

### Option 1: Custom Resource (Recommended)

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
  serviceUri: "https://api.o2ims.example.com/o2ims-infrastructureInventory/v1"
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

### Option 2: ConfigMap (Simpler)

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
  serviceUri: "https://api.o2ims.example.com/o2ims-infrastructureInventory/v1"
```

### Transformation Logic

**Kubernetes → O2-IMS**:

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

## API Operations

### List Deployment Managers

```http
GET /o2ims-infrastructureInventory/v1/deploymentManagers HTTP/1.1
Accept: application/json
```

**Response (200 OK)**:
```json
{
  "deploymentManagers": [
    {
      "deploymentManagerId": "ocloud-k8s-1",
      "name": "US-East Kubernetes Cloud",
      "description": "Production Kubernetes cluster for RAN workloads",
      "oCloudId": "ocloud-1",
      "serviceUri": "https://api.o2ims.example.com/o2ims-infrastructureInventory/v1",
      "supportedLocations": ["us-east-1a", "us-east-1b", "us-east-1c"],
      "capabilities": ["compute", "storage", "networking"],
      "extensions": {
        "totalNodes": 45,
        "k8sVersion": "v1.30.0",
        "containerRuntime": "containerd://1.7.2"
      }
    }
  ],
  "total": 1
}
```

**Kubernetes Action**: List O2DeploymentManager CRs in `o2ims-system` namespace

### Get Deployment Manager

```http
GET /o2ims-infrastructureInventory/v1/deploymentManagers/{id} HTTP/1.1
Accept: application/json
```

**Response (200 OK)**:
```json
{
  "deploymentManagerId": "ocloud-k8s-1",
  "name": "US-East Kubernetes Cloud",
  "description": "Production Kubernetes cluster for RAN workloads",
  "oCloudId": "ocloud-1",
  "serviceUri": "https://api.o2ims.example.com/o2ims-infrastructureInventory/v1",
  "supportedLocations": ["us-east-1a", "us-east-1b", "us-east-1c"],
  "capabilities": ["compute", "storage", "networking"],
  "extensions": {
    "totalNodes": 45,
    "k8sVersion": "v1.30.0",
    "containerRuntime": "containerd://1.7.2",
    "clusterVersion": "1.30.0",
    "provider": "AWS",
    "region": "us-east-1"
  }
}
```

**Kubernetes Action**: Get O2DeploymentManager CR by name

**Error Response (404 Not Found)**:
```json
{
  "error": "NotFound",
  "message": "Deployment Manager not found: ocloud-k8s-nonexistent",
  "code": 404
}
```

### Operations Summary

| Operation | Method | Endpoint | K8s Action | Supported |
|-----------|--------|----------|------------|-----------|
| List | GET | `/deploymentManagers` | List O2DeploymentManager CRs | ✅ |
| Get | GET | `/deploymentManagers/{id}` | Get O2DeploymentManager CR | ✅ |
| Create | POST | `/deploymentManagers` | N/A | ❌ Not supported (cluster-level) |
| Update | PUT | `/deploymentManagers/{id}` | N/A | ❌ Not supported (cluster-level) |
| Delete | DELETE | `/deploymentManagers/{id}` | N/A | ❌ Not supported (cluster-level) |

**Note**: Create/Update/Delete operations are not supported because Deployment Manager represents cluster-level configuration that should be managed via infrastructure-as-code (Terraform, Helm, etc.), not via the O2-IMS API.

## Backend-Specific Mappings

### Kubernetes Adapter

| O2-IMS Field | Kubernetes Source |
|--------------|-------------------|
| `deploymentManagerId` | O2DeploymentManager CR `.spec.deploymentManagerId` |
| `name` | O2DeploymentManager CR `.spec.name` |
| `description` | O2DeploymentManager CR `.spec.description` |
| `oCloudId` | O2DeploymentManager CR `.spec.oCloudId` |
| `serviceUri` | O2DeploymentManager CR `.spec.serviceUri` |
| `supportedLocations` | O2DeploymentManager CR `.spec.supportedLocations` |
| `capabilities` | O2DeploymentManager CR `.spec.capabilities` |
| `extensions.totalNodes` | Count of Nodes (dynamic) |
| `extensions.k8sVersion` | Node `.status.nodeInfo.kubeletVersion` (dynamic) |
| `extensions.containerRuntime` | Node `.status.nodeInfo.containerRuntimeVersion` (dynamic) |

### Dell DTIAS Adapter

```go
func (a *DTIASAdapter) GetDeploymentManager(ctx context.Context, dmID string) (*models.DeploymentManager, error) {
    // DTIAS Site represents the deployment manager
    site, err := a.client.GetSite(ctx, dmID)
    if err != nil {
        return nil, err
    }

    return &models.DeploymentManager{
        DeploymentManagerID: site.ID,
        Name:                site.Name,
        Description:         site.Description,
        OCloudID:            a.oCloudID,
        ServiceURI:          a.serviceURI,
        SupportedLocations:  []string{site.Location},
        Capabilities:        []string{"bare-metal", "compute", "storage"},
        Extensions: map[string]interface{}{
            "infrastructure": "bare-metal",
            "vendor":         "Dell",
            "totalServers":   site.TotalServers,
            "datacenter":     site.DataCenter,
        },
    }, nil
}
```

**DTIAS API**: `GET /v2/inventory/sites/{Id}`

### AWS EKS Adapter

```go
func (a *AWSAdapter) GetDeploymentManager(ctx context.Context, dmID string) (*models.DeploymentManager, error) {
    // EKS Cluster represents the deployment manager
    cluster, err := a.eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
        Name: aws.String(dmID),
    })
    if err != nil {
        return nil, err
    }

    return &models.DeploymentManager{
        DeploymentManagerID: *cluster.Cluster.Name,
        Name:                *cluster.Cluster.Name,
        Description:         fmt.Sprintf("EKS cluster in %s", *cluster.Cluster.Arn),
        OCloudID:            a.oCloudID,
        ServiceURI:          a.serviceURI,
        SupportedLocations:  cluster.Cluster.ResourcesVpcConfig.SubnetIds,
        Capabilities:        []string{"compute", "storage", "networking", "auto-scaling"},
        Extensions: map[string]interface{}{
            "infrastructure": "cloud",
            "provider":       "AWS",
            "version":        *cluster.Cluster.Version,
            "endpoint":       *cluster.Cluster.Endpoint,
            "arn":            *cluster.Cluster.Arn,
            "status":         cluster.Cluster.Status,
        },
    }, nil
}
```

**AWS API**: `DescribeCluster`

### VMware Adapter

```go
func (a *VMwareAdapter) GetDeploymentManager(ctx context.Context, dmID string) (*models.DeploymentManager, error) {
    // vCenter represents the deployment manager
    return &models.DeploymentManager{
        DeploymentManagerID: a.vcenterID,
        Name:                a.vcenterName,
        Description:         "VMware vCenter Server",
        OCloudID:            a.oCloudID,
        ServiceURI:          a.serviceURI,
        SupportedLocations:  a.datacenters,
        Capabilities:        []string{"compute", "storage", "networking", "vmotion"},
        Extensions: map[string]interface{}{
            "infrastructure":   "virtualization",
            "vendor":           "VMware",
            "version":          a.vcenterVersion,
            "totalHosts":       a.getTotalESXiHosts(ctx),
            "totalVMs":         a.getTotalVMs(ctx),
        },
    }, nil
}
```

**VMware API**: vSphere API (AboutInfo)

## Related Documentation

- [O2-IMS Overview](README.md)
- [Resource Pools](resource-pools.md)
- [Backend Plugins](../../backend-plugins.md)
