# O2-IMS to Kubernetes API Mapping

**Version:** 1.0
**Date:** 2026-01-06

This document defines how O-RAN O2-IMS resources map to Kubernetes resources in the netweave gateway.

## Table of Contents

1. [Overview](#overview)
2. [Deployment Manager](#deployment-manager)
3. [Resource Pools](#resource-pools)
4. [Resources](#resources)
5. [Resource Types](#resource-types)
6. [Subscriptions](#subscriptions)
7. [Data Transformation Examples](#data-transformation-examples)

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

| Operation | Method | Endpoint | K8s Action |
|-----------|--------|----------|------------|
| List | GET | `/resourcePools` | List MachineSets |
| Get | GET | `/resourcePools/{id}` | Get MachineSet |
| Create | POST | `/resourcePools` | Create MachineSet |
| Update | PUT | `/resourcePools/{id}` | Update MachineSet |
| Delete | DELETE | `/resourcePools/{id}` | Delete MachineSet |

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

| Operation | Method | Endpoint | K8s Action |
|-----------|--------|----------|------------|
| List | GET | `/resources` | List Nodes (or Machines) |
| Get | GET | `/resources/{id}` | Get Node |
| Create | POST | `/resources` | Create Machine (triggers Node) |
| ~~Update~~ | ~~PUT~~ | ~~N/A~~ | Not supported (nodes are immutable) |
| Delete | DELETE | `/resources/{id}` | Delete Machine or drain+delete Node |

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

| Operation | Method | Endpoint | K8s Action |
|-----------|--------|----------|------------|
| List | GET | `/subscriptions` | List from Redis |
| Get | GET | `/subscriptions/{id}` | Get from Redis |
| Create | POST | `/subscriptions` | Store in Redis + start watching |
| Update | PUT | `/subscriptions/{id}` | Update Redis |
| Delete | DELETE | `/subscriptions/{id}` | Delete from Redis + stop watching |

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

### Example 2: Subscribe to Node Events

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

// 2. Generate subscription ID
subID := uuid.New().String()

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

## Summary

**Mapping Completeness**:
- ✅ Deployment Manager → Custom Resource (metadata)
- ✅ Resource Pool → MachineSet (full CRUD)
- ✅ Resource → Node/Machine (full CRUD)
- ✅ Resource Type → Aggregated from Nodes + StorageClasses
- ✅ Subscription → Redis + K8s Informers (full CRUD + webhooks)

**Key Principles**:
1. Use native K8s resources where possible
2. Store O2-IMS-specific data in Redis or CRDs
3. Transform bidirectionally (read and write)
4. Preserve semantic meaning across translation
5. Handle errors gracefully with proper O2-IMS error responses

**Future Extensions**:
- Additional resource types (network, accelerators)
- Advanced filtering (complex queries)
- Batch operations
- Custom resource definitions for all O2-IMS types
