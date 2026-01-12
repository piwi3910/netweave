# IMS Adapter Interface

**Version:** 1.0
**Last Updated:** 2026-01-12

## Overview

IMS (Infrastructure Management Services) adapters manage infrastructure resources across multiple backend systems. They provide a unified interface for resource pool management, resource lifecycle, and resource type discovery.

## Core Interface

```go
// internal/plugin/ims/ims.go

package ims

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin"
)

// IMSPlugin extends the base Plugin interface for infrastructure management
type IMSPlugin interface {
    plugin.Plugin

    // Deployment Managers
    ListDeploymentManagers(ctx context.Context, filter *Filter) ([]*DeploymentManager, error)
    GetDeploymentManager(ctx context.Context, id string) (*DeploymentManager, error)

    // Resource Pools
    ListResourcePools(ctx context.Context, filter *Filter) ([]*ResourcePool, error)
    GetResourcePool(ctx context.Context, id string) (*ResourcePool, error)
    CreateResourcePool(ctx context.Context, pool *ResourcePool) (*ResourcePool, error)
    UpdateResourcePool(ctx context.Context, id string, pool *ResourcePool) (*ResourcePool, error)
    DeleteResourcePool(ctx context.Context, id string) error

    // Resources
    ListResources(ctx context.Context, filter *Filter) ([]*Resource, error)
    GetResource(ctx context.Context, id string) (*Resource, error)
    CreateResource(ctx context.Context, resource *Resource) (*Resource, error)
    DeleteResource(ctx context.Context, id string) error

    // Resource Types
    ListResourceTypes(ctx context.Context, filter *Filter) ([]*ResourceType, error)
    GetResourceType(ctx context.Context, id string) (*ResourceType, error)

    // Subscriptions (if backend supports native subscriptions)
    SupportsNativeSubscriptions() bool
    Subscribe(ctx context.Context, sub *Subscription) error
    Unsubscribe(ctx context.Context, id string) error
}
```

## Data Models

### DeploymentManager

Represents the infrastructure deployment manager (cluster, region, datacenter):

```go
type DeploymentManager struct {
    DeploymentManagerID string                 `json:"deploymentManagerId"`
    Name                string                 `json:"name"`
    Description         string                 `json:"description,omitempty"`
    OCloudID            string                 `json:"oCloudId"`
    ServiceURI          string                 `json:"serviceUri,omitempty"`
    SupportedLocations  []string               `json:"supportedLocations,omitempty"`
    Capabilities        []string               `json:"capabilities,omitempty"`
    Capacity            map[string]interface{} `json:"capacity,omitempty"`
    Extensions          map[string]interface{} `json:"extensions,omitempty"`
}
```

### ResourcePool

Logical grouping of resources with shared characteristics:

```go
type ResourcePool struct {
    ResourcePoolID string                 `json:"resourcePoolId"`
    Name           string                 `json:"name"`
    Description    string                 `json:"description,omitempty"`
    Location       string                 `json:"location,omitempty"`
    OCloudID       string                 `json:"oCloudId"`
    GlobalAssetID  string                 `json:"globalAssetId,omitempty"`
    Extensions     map[string]interface{} `json:"extensions,omitempty"`
}
```

### Resource

Individual infrastructure resource (node, instance, server):

```go
type Resource struct {
    ResourceID     string                 `json:"resourceId"`
    Name           string                 `json:"name"`
    Description    string                 `json:"description,omitempty"`
    ResourceTypeID string                 `json:"resourceTypeId"`
    ResourcePoolID string                 `json:"resourcePoolId,omitempty"`
    OCloudID       string                 `json:"oCloudId"`
    GlobalAssetID  string                 `json:"globalAssetId,omitempty"`
    Extensions     map[string]interface{} `json:"extensions,omitempty"`
}
```

### ResourceType

Classification of resources by capabilities:

```go
type ResourceType struct {
    ResourceTypeID string                 `json:"resourceTypeId"`
    Name           string                 `json:"name"`
    Description    string                 `json:"description,omitempty"`
    Vendor         string                 `json:"vendor,omitempty"`
    Model          string                 `json:"model,omitempty"`
    Version        string                 `json:"version,omitempty"`
    Extensions     map[string]interface{} `json:"extensions,omitempty"`
}
```

## Filter Options

```go
type Filter struct {
    // Resource Pool filters
    ResourcePoolID string
    Location       string

    // Resource filters
    ResourceID     string
    ResourceTypeID string
    Name           string

    // Generic filters
    Labels         map[string]string
    Extensions     map[string]interface{}

    // Pagination
    Limit  int
    Offset int
}

func (f *Filter) Matches(obj interface{}) bool {
    // Implementation based on object type
}
```

## IMS Capabilities

```go
const (
    CapResourcePoolManagement Capability = "resource-pool-management"
    CapResourceManagement     Capability = "resource-management"
    CapResourceTypeDiscovery  Capability = "resource-type-discovery"
    CapRealtimeEvents         Capability = "realtime-events"
)
```

## Subscription Support

Adapters may support native event subscriptions:

```go
type Subscription struct {
    SubscriptionID string                 `json:"subscriptionId"`
    Callback       string                 `json:"callback"`
    Filter         *Filter                `json:"filter,omitempty"`
    Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

// Check if adapter supports native subscriptions
if adapter.SupportsNativeSubscriptions() {
    err := adapter.Subscribe(ctx, subscription)
}
```

## Available IMS Adapters

| Adapter | Status | Resource Pools | Resources | Resource Types |
|---------|--------|----------------|-----------|----------------|
| **Kubernetes** | ✅ Production | MachineSet | Node/Machine | StorageClass |
| **OpenStack** | 📋 Spec Complete | Host Aggregate | Nova Instance | Flavor |
| **Dell DTIAS** | 📋 Spec Complete | Server Pool | Physical Server | Server Type |
| **VMware vSphere** | 📋 Spec Complete | Resource Pool | ESXi Host/VM | VM Template |
| **AWS EKS** | 📋 Spec | Node Group | EC2 Instance | Instance Type |
| **Azure AKS** | 📋 Spec | Node Pool | Azure VM | VM SKU |
| **Google GKE** | 📋 Spec | Node Pool | GCE Instance | Machine Type |

## Implementation Guidelines

### Adapter Structure

```go
package myadapter

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin/ims"
)

type MyAdapter struct {
    name    string
    version string
    client  *MyBackendClient
    config  *Config
}

type Config struct {
    Endpoint string `yaml:"endpoint"`
    APIKey   string `yaml:"apiKey"`
    OCloudID string `yaml:"ocloudId"`
}

func NewAdapter(config *Config) (*MyAdapter, error) {
    client := NewBackendClient(config.Endpoint, config.APIKey)
    return &MyAdapter{
        name:    "myadapter",
        version: "1.0.0",
        client:  client,
        config:  config,
    }, nil
}

// Implement plugin.Plugin interface
func (a *MyAdapter) Name() string { return a.name }
func (a *MyAdapter) Version() string { return a.version }
func (a *MyAdapter) Category() plugin.PluginCategory { return plugin.CategoryIMS }
func (a *MyAdapter) Capabilities() []plugin.Capability {
    return []plugin.Capability{
        plugin.CapResourcePoolManagement,
        plugin.CapResourceManagement,
        plugin.CapResourceTypeDiscovery,
    }
}

// Implement ims.IMSPlugin interface
func (a *MyAdapter) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    // Implementation
}
```

### Transformation Pattern

Each adapter must transform backend-specific data to O2-IMS models:

```go
func (a *MyAdapter) transformBackendPool(backendPool *BackendPool) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: backendPool.ID,
        Name:          backendPool.Name,
        Description:   backendPool.Description,
        Location:      backendPool.Region,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "backend.type":       backendPool.Type,
            "backend.capacity":   backendPool.Capacity,
            "backend.customAttr": backendPool.CustomAttribute,
        },
    }
}
```

### Error Handling

```go
import "fmt"

func (a *MyAdapter) GetResource(ctx context.Context, id string) (*ims.Resource, error) {
    resource, err := a.client.FetchResource(ctx, id)
    if err != nil {
        if IsNotFound(err) {
            return nil, fmt.Errorf("resource not found: %s", id)
        }
        return nil, fmt.Errorf("failed to fetch resource: %w", err)
    }
    return a.transformBackendResource(resource), nil
}
```

## Testing Requirements

### Unit Tests

```go
func TestMyAdapter_ListResourcePools(t *testing.T) {
    tests := []struct {
        name    string
        filter  *ims.Filter
        want    int
        wantErr bool
    }{
        {
            name: "list all pools",
            filter: &ims.Filter{},
            want: 3,
        },
        {
            name: "filter by location",
            filter: &ims.Filter{Location: "us-west"},
            want: 1,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            adapter := setupMockAdapter(t)
            pools, err := adapter.ListResourcePools(context.Background(), tt.filter)

            if tt.wantErr {
                require.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Len(t, pools, tt.want)
        })
    }
}
```

### Integration Tests

```go
func TestMyAdapter_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    adapter := setupRealAdapter(t)
    defer adapter.Close()

    // Test health
    err := adapter.Health(context.Background())
    require.NoError(t, err)

    // Test resource pool operations
    pools, err := adapter.ListResourcePools(context.Background(), &ims.Filter{})
    require.NoError(t, err)
    assert.NotEmpty(t, pools)
}
```

## Configuration Example

```yaml
plugins:
  ims:
    - name: my-adapter
      type: myadapter
      enabled: true
      config:
        endpoint: https://backend.example.com/api
        apiKey: ${BACKEND_API_KEY}
        ocloudId: ocloud-custom-1
        timeout: 30s
```

## See Also

- [Kubernetes Adapter](kubernetes.md) - Reference implementation
- [OpenStack Adapter](openstack.md) - NFVi cloud platform
- [Cloud Adapters](cloud.md) - Public cloud providers
- [Bare-Metal Adapters](bare-metal.md) - Physical infrastructure
