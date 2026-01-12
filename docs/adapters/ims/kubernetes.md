# Kubernetes IMS Adapter

**Status:** ✅ Production Ready (Core Implementation)
**Version:** 1.0
**Last Updated:** 2026-01-12

## Overview

The Kubernetes adapter provides O2-IMS infrastructure management for Kubernetes clusters. It maps Kubernetes native resources to O2-IMS constructs, enabling SMO systems to manage containerized infrastructure.

## Resource Mappings

| O2-IMS Concept | Kubernetes Resource | Description |
|----------------|---------------------|-------------|
| **Deployment Manager** | Cluster metadata (CRD) | Kubernetes cluster information |
| **Resource Pool** | MachineSet / NodePool | Logical grouping of nodes |
| **Resource** | Node (running) / Machine (lifecycle) | Compute nodes in the cluster |
| **Resource Type** | StorageClass + Machine flavors | Node types and storage capabilities |

## Capabilities

```go
capabilities := []Capability{
    CapResourcePoolManagement,
    CapResourceManagement,
    CapResourceTypeDiscovery,
    CapRealtimeEvents,  // Via Kubernetes informers
}
```

## Configuration

```yaml
plugins:
  ims:
    - name: kubernetes
      type: kubernetes
      enabled: true
      default: true
      config:
        kubeconfig: /etc/kubernetes/admin.conf
        namespace: default
        ocloudId: ocloud-k8s-1
        enableInformers: true
        resyncPeriod: 30s
```

## Implementation

### Adapter Structure

```go
// internal/plugins/ims/kubernetes/plugin.go

package kubernetes

import (
    "context"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "github.com/yourorg/netweave/internal/plugin/ims"
)

type KubernetesAdapter struct {
    name         string
    version      string
    client       kubernetes.Interface
    config       *Config
    informers    map[string]cache.SharedIndexInformer
}

type Config struct {
    Kubeconfig      string        `yaml:"kubeconfig"`
    Namespace       string        `yaml:"namespace"`
    OCloudID        string        `yaml:"ocloudId"`
    EnableInformers bool          `yaml:"enableInformers"`
    ResyncPeriod    time.Duration `yaml:"resyncPeriod"`
}

func NewAdapter(config *Config) (*KubernetesAdapter, error) {
    kubeConfig, err := clientcmd.BuildConfigFromFlags("", config.Kubeconfig)
    if err != nil {
        return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
    }

    client, err := kubernetes.NewForConfig(kubeConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
    }

    adapter := &KubernetesAdapter{
        name:      "kubernetes",
        version:   "1.0.0",
        client:    client,
        config:    config,
        informers: make(map[string]cache.SharedIndexInformer),
    }

    if config.EnableInformers {
        if err := adapter.setupInformers(); err != nil {
            return nil, fmt.Errorf("failed to setup informers: %w", err)
        }
    }

    return adapter, nil
}
```

### List Resource Pools

Maps MachineSets to Resource Pools:

```go
func (a *KubernetesAdapter) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    // List MachineSets from machine.openshift.io/v1beta1 API
    machineSets, err := a.getMachineSets(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to list machinesets: %w", err)
    }

    pools := make([]*ims.ResourcePool, 0, len(machineSets.Items))
    for _, ms := range machineSets.Items {
        pool := a.transformMachineSetToResourcePool(&ms)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

func (a *KubernetesAdapter) transformMachineSetToResourcePool(ms *machinev1beta1.MachineSet) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: string(ms.UID),
        Name:          ms.Name,
        Description:   fmt.Sprintf("MachineSet: %s", ms.Name),
        Location:      ms.Spec.Template.Spec.ProviderSpec.Value["zone"],
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "k8s.namespace":       ms.Namespace,
            "k8s.machineSetName":  ms.Name,
            "k8s.replicas":        ms.Spec.Replicas,
            "k8s.readyReplicas":   ms.Status.ReadyReplicas,
            "k8s.availableReplicas": ms.Status.AvailableReplicas,
            "k8s.labels":          ms.Labels,
        },
    }
}
```

### List Resources

Maps Nodes/Machines to Resources:

```go
func (a *KubernetesAdapter) ListResources(ctx context.Context, filter *ims.Filter) ([]*ims.Resource, error) {
    nodes, err := a.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list nodes: %w", err)
    }

    resources := make([]*ims.Resource, 0, len(nodes.Items))
    for _, node := range nodes.Items {
        resource := a.transformNodeToResource(&node)
        if filter.Matches(resource) {
            resources = append(resources, resource)
        }
    }

    return resources, nil
}

func (a *KubernetesAdapter) transformNodeToResource(node *corev1.Node) *ims.Resource {
    // Determine resource type from node labels
    resourceTypeID := a.getResourceTypeFromNode(node)

    // Determine resource pool from MachineSet owner reference
    resourcePoolID := a.getResourcePoolFromNode(node)

    return &ims.Resource{
        ResourceID:     string(node.UID),
        Name:          node.Name,
        Description:   fmt.Sprintf("Kubernetes node: %s", node.Name),
        ResourceTypeID: resourceTypeID,
        ResourcePoolID: resourcePoolID,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "k8s.nodeName":       node.Name,
            "k8s.nodeGroup":      node.Labels["node-group"],
            "k8s.instanceType":   node.Labels["node.kubernetes.io/instance-type"],
            "k8s.zone":           node.Labels["topology.kubernetes.io/zone"],
            "k8s.region":         node.Labels["topology.kubernetes.io/region"],
            "k8s.status":         node.Status.Phase,
            "k8s.conditions":     a.summarizeConditions(node.Status.Conditions),
            "k8s.capacity":       node.Status.Capacity,
            "k8s.allocatable":    node.Status.Allocatable,
        },
    }
}
```

### List Resource Types

Aggregates from StorageClasses and node types:

```go
func (a *KubernetesAdapter) ListResourceTypes(ctx context.Context, filter *ims.Filter) ([]*ims.ResourceType, error) {
    resourceTypes := make([]*ims.ResourceType, 0)

    // 1. Get unique node instance types
    nodes, err := a.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list nodes: %w", err)
    }

    instanceTypes := make(map[string]*ims.ResourceType)
    for _, node := range nodes.Items {
        instanceType := node.Labels["node.kubernetes.io/instance-type"]
        if instanceType == "" {
            continue
        }

        if _, exists := instanceTypes[instanceType]; !exists {
            instanceTypes[instanceType] = &ims.ResourceType{
                ResourceTypeID: fmt.Sprintf("k8s-node-%s", instanceType),
                Name:          instanceType,
                Description:   fmt.Sprintf("Kubernetes node type: %s", instanceType),
                Vendor:        a.detectVendor(&node),
                Extensions: map[string]interface{}{
                    "k8s.instanceType": instanceType,
                    "k8s.capacity":     node.Status.Capacity,
                },
            }
        }
    }

    for _, rt := range instanceTypes {
        if filter.Matches(rt) {
            resourceTypes = append(resourceTypes, rt)
        }
    }

    // 2. Get StorageClasses
    storageClasses, err := a.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list storage classes: %w", err)
    }

    for _, sc := range storageClasses.Items {
        rt := &ims.ResourceType{
            ResourceTypeID: fmt.Sprintf("k8s-storage-%s", sc.Name),
            Name:          sc.Name,
            Description:   fmt.Sprintf("Storage class: %s", sc.Name),
            Vendor:        sc.Provisioner,
            Extensions: map[string]interface{}{
                "k8s.storageClass":  sc.Name,
                "k8s.provisioner":   sc.Provisioner,
                "k8s.reclaimPolicy": sc.ReclaimPolicy,
                "k8s.volumeBindingMode": sc.VolumeBindingMode,
            },
        }

        if filter.Matches(rt) {
            resourceTypes = append(resourceTypes, rt)
        }
    }

    return resourceTypes, nil
}
```

### Create Resource Pool

Creates a new MachineSet:

```go
func (a *KubernetesAdapter) CreateResourcePool(ctx context.Context, pool *ims.ResourcePool) (*ims.ResourcePool, error) {
    // Extract Kubernetes-specific extensions
    replicas := int32(pool.Extensions["k8s.replicas"].(float64))
    instanceType := pool.Extensions["k8s.instanceType"].(string)
    zone := pool.Extensions["k8s.zone"].(string)

    // Create MachineSet
    machineSet := &machinev1beta1.MachineSet{
        ObjectMeta: metav1.ObjectMeta{
            Name:      pool.Name,
            Namespace: a.config.Namespace,
            Labels: map[string]string{
                "o2ims.resourcePoolId": pool.ResourcePoolID,
            },
        },
        Spec: machinev1beta1.MachineSetSpec{
            Replicas: &replicas,
            Template: machinev1beta1.MachineTemplateSpec{
                Spec: machinev1beta1.MachineSpec{
                    ProviderSpec: machinev1beta1.ProviderSpec{
                        Value: &runtime.RawExtension{
                            Raw: a.buildProviderSpec(instanceType, zone),
                        },
                    },
                },
            },
        },
    }

    created, err := a.machineClient.MachineSets(a.config.Namespace).Create(ctx, machineSet, metav1.CreateOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to create machineset: %w", err)
    }

    return a.transformMachineSetToResourcePool(created), nil
}
```

### Real-Time Events (Informers)

Supports native event subscriptions via Kubernetes informers:

```go
func (a *KubernetesAdapter) SupportsNativeSubscriptions() bool {
    return a.config.EnableInformers
}

func (a *KubernetesAdapter) setupInformers() error {
    // Node informer
    nodeInformer := cache.NewSharedIndexInformer(
        &cache.ListWatch{
            ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
                return a.client.CoreV1().Nodes().List(context.Background(), options)
            },
            WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
                return a.client.CoreV1().Nodes().Watch(context.Background(), options)
            },
        },
        &corev1.Node{},
        a.config.ResyncPeriod,
        cache.Indexers{},
    )

    nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc:    a.handleNodeAdd,
        UpdateFunc: a.handleNodeUpdate,
        DeleteFunc: a.handleNodeDelete,
    })

    a.informers["nodes"] = nodeInformer

    // MachineSet informer
    // ... similar setup

    return nil
}

func (a *KubernetesAdapter) handleNodeAdd(obj interface{}) {
    node := obj.(*corev1.Node)
    resource := a.transformNodeToResource(node)

    // Trigger subscription webhooks
    a.notifySubscribers(&ims.Event{
        Type:     "ResourceCreated",
        Resource: resource,
    })
}
```

## Testing

### Unit Tests

```go
func TestKubernetesAdapter_ListResourcePools(t *testing.T) {
    tests := []struct {
        name       string
        machinesets []machinev1beta1.MachineSet
        filter     *ims.Filter
        want       int
    }{
        {
            name: "list all machinesets",
            machinesets: []machinev1beta1.MachineSet{
                {ObjectMeta: metav1.ObjectMeta{Name: "ms-1"}},
                {ObjectMeta: metav1.ObjectMeta{Name: "ms-2"}},
            },
            filter: &ims.Filter{},
            want:   2,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            adapter := setupMockKubernetesAdapter(t, tt.machinesets)
            pools, err := adapter.ListResourcePools(context.Background(), tt.filter)

            require.NoError(t, err)
            assert.Len(t, pools, tt.want)
        })
    }
}
```

### Integration Tests

```go
func TestKubernetesAdapter_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    adapter := setupRealKubernetesAdapter(t)
    defer adapter.Close()

    // Test list nodes
    resources, err := adapter.ListResources(context.Background(), &ims.Filter{})
    require.NoError(t, err)
    assert.NotEmpty(t, resources)

    // Verify node properties
    for _, resource := range resources {
        assert.NotEmpty(t, resource.ResourceID)
        assert.NotEmpty(t, resource.Name)
        assert.Equal(t, adapter.config.OCloudID, resource.OCloudID)
    }
}
```

## Performance Considerations

- **Informers**: Use informers for real-time events to avoid polling
- **Caching**: Leverage Kubernetes client-go caching mechanisms
- **Pagination**: Implement pagination for large clusters (1000+ nodes)
- **Rate Limiting**: Respect Kubernetes API rate limits

## Security

- Use RBAC with minimum required permissions
- Store kubeconfig securely (Kubernetes Secrets)
- Use service accounts for in-cluster deployments
- Enable TLS for external cluster access

## See Also

- [IMS Adapter Interface](README.md)
- [OpenStack Adapter](openstack.md)
- [Cloud Adapters](cloud.md)
