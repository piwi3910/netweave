# OpenStack IMS Adapter

**Status:** 📋 Specification Complete
**Version:** 1.0
**Last Updated:** 2026-01-12

## Overview

The OpenStack adapter provides O2-IMS infrastructure management for OpenStack NFVi platforms. It maps Nova compute and Placement API resources to O2-IMS constructs.

## Resource Mappings

| O2-IMS Concept | OpenStack Resource | API |
|----------------|-------------------|-----|
| **Deployment Manager** | OpenStack Region metadata | Keystone |
| **Resource Pool** | Host Aggregate | Placement |
| **Resource** | Nova Instance (VM) | Nova Compute |
| **Resource Type** | Flavor | Nova Compute |

## Capabilities

```go
capabilities := []Capability{
    CapResourcePoolManagement,
    CapResourceManagement,
    CapResourceTypeDiscovery,
    // No native subscriptions - use polling
}
```

## Configuration

```yaml
plugins:
  ims:
    - name: openstack-nfv
      type: openstack
      enabled: true
      config:
        authUrl: https://openstack.example.com:5000/v3
        username: admin
        password: ${OPENSTACK_PASSWORD}
        projectName: o2ims
        domainName: Default
        region: RegionOne
        ocloudId: ocloud-openstack-1
```

## Implementation

```go
// internal/plugins/ims/openstack/plugin.go

package openstack

import (
    "context"
    "github.com/gophercloud/gophercloud"
    "github.com/gophercloud/gophercloud/openstack"
    "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
    "github.com/gophercloud/gophercloud/openstack/placement/v1/aggregates"
)

type OpenStackPlugin struct {
    name      string
    version   string
    provider  *gophercloud.ProviderClient
    compute   *gophercloud.ServiceClient
    placement *gophercloud.ServiceClient
    config    *Config
}

type Config struct {
    AuthURL     string `yaml:"authUrl"`
    Username    string `yaml:"username"`
    Password    string `yaml:"password"`
    ProjectName string `yaml:"projectName"`
    DomainName  string `yaml:"domainName"`
    Region      string `yaml:"region"`
    OCloudID    string `yaml:"ocloudId"`
}

func NewPlugin(config *Config) (*OpenStackPlugin, error) {
    provider, err := openstack.AuthenticatedClient(gophercloud.AuthOptions{
        IdentityEndpoint: config.AuthURL,
        Username:         config.Username,
        Password:         config.Password,
        TenantName:       config.ProjectName,
        DomainName:       config.DomainName,
    })
    if err != nil {
        return nil, err
    }

    compute, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
        Region: config.Region,
    })
    if err != nil {
        return nil, err
    }

    placement, err := openstack.NewPlacementV1(provider, gophercloud.EndpointOpts{
        Region: config.Region,
    })
    if err != nil {
        return nil, err
    }

    return &OpenStackPlugin{
        name:      "openstack",
        version:   "1.0.0",
        provider:  provider,
        compute:   compute,
        placement: placement,
        config:    config,
    }, nil
}
```

### List Resource Pools

```go
func (p *OpenStackPlugin) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    allPages, err := aggregates.List(p.placement).AllPages()
    if err != nil {
        return nil, fmt.Errorf("failed to list host aggregates: %w", err)
    }

    osAggregates, err := aggregates.ExtractAggregates(allPages)
    if err != nil {
        return nil, err
    }

    pools := make([]*ims.ResourcePool, 0, len(osAggregates))
    for _, agg := range osAggregates {
        pool := p.transformHostAggregateToResourcePool(&agg)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

func (p *OpenStackPlugin) transformHostAggregateToResourcePool(agg *aggregates.Aggregate) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: fmt.Sprintf("os-aggregate-%d", agg.ID),
        Name:          agg.Name,
        Description:   fmt.Sprintf("OpenStack host aggregate: %s", agg.Name),
        Location:      agg.AvailabilityZone,
        OCloudID:      p.config.OCloudID,
        Extensions: map[string]interface{}{
            "openstack.aggregateId":      agg.ID,
            "openstack.availabilityZone": agg.AvailabilityZone,
            "openstack.metadata":         agg.Metadata,
            "openstack.hostCount":        len(agg.Hosts),
            "openstack.hosts":            agg.Hosts,
        },
    }
}
```

### List Resources

```go
func (p *OpenStackPlugin) ListResources(ctx context.Context, filter *ims.Filter) ([]*ims.Resource, error) {
    allPages, err := servers.List(p.compute, servers.ListOpts{}).AllPages()
    if err != nil {
        return nil, fmt.Errorf("failed to list instances: %w", err)
    }

    instances, err := servers.ExtractServers(allPages)
    if err != nil {
        return nil, err
    }

    resources := make([]*ims.Resource, 0, len(instances))
    for _, instance := range instances {
        resource := p.transformNovaInstanceToResource(&instance)
        if filter.Matches(resource) {
            resources = append(resources, resource)
        }
    }

    return resources, nil
}

func (p *OpenStackPlugin) transformNovaInstanceToResource(instance *servers.Server) *ims.Resource {
    return &ims.Resource{
        ResourceID:     instance.ID,
        Name:          instance.Name,
        Description:   fmt.Sprintf("OpenStack instance: %s", instance.Name),
        ResourceTypeID: fmt.Sprintf("os-flavor-%s", instance.Flavor["id"]),
        ResourcePoolID: p.getResourcePoolIDFromInstance(instance),
        OCloudID:      p.config.OCloudID,
        Extensions: map[string]interface{}{
            "openstack.instanceId":       instance.ID,
            "openstack.flavorId":         instance.Flavor["id"],
            "openstack.imageId":          instance.Image["id"],
            "openstack.status":           instance.Status,
            "openstack.hostId":           instance.HostID,
            "openstack.availabilityZone": instance.AvailabilityZone,
            "openstack.addresses":        instance.Addresses,
        },
    }
}
```

### List Resource Types

```go
func (p *OpenStackPlugin) ListResourceTypes(ctx context.Context, filter *ims.Filter) ([]*ims.ResourceType, error) {
    allPages, err := flavors.ListDetail(p.compute, nil).AllPages()
    if err != nil {
        return nil, fmt.Errorf("failed to list flavors: %w", err)
    }

    osFlavors, err := flavors.ExtractFlavors(allPages)
    if err != nil {
        return nil, err
    }

    resourceTypes := make([]*ims.ResourceType, 0, len(osFlavors))
    for _, flavor := range osFlavors {
        rt := &ims.ResourceType{
            ResourceTypeID: fmt.Sprintf("os-flavor-%s", flavor.ID),
            Name:          flavor.Name,
            Description:   fmt.Sprintf("OpenStack flavor: %s", flavor.Name),
            Vendor:        "OpenStack",
            Extensions: map[string]interface{}{
                "openstack.flavorId": flavor.ID,
                "openstack.vcpus":    flavor.VCPUs,
                "openstack.ram":      flavor.RAM,
                "openstack.disk":     flavor.Disk,
                "openstack.swap":     flavor.Swap,
                "openstack.rxtxFactor": flavor.RxTxFactor,
            },
        }

        if filter.Matches(rt) {
            resourceTypes = append(resourceTypes, rt)
        }
    }

    return resourceTypes, nil
}
```

## Testing

```go
func TestOpenStackPlugin_ListResourcePools(t *testing.T) {
    mockProvider := setupMockOpenStack(t)
    plugin := &OpenStackPlugin{
        name:      "openstack",
        provider:  mockProvider,
        config:    &Config{OCloudID: "test-cloud"},
    }

    pools, err := plugin.ListResourcePools(context.Background(), &ims.Filter{})
    require.NoError(t, err)
    assert.NotEmpty(t, pools)

    // Verify pool properties
    for _, pool := range pools {
        assert.NotEmpty(t, pool.ResourcePoolID)
        assert.Contains(t, pool.ResourcePoolID, "os-aggregate-")
        assert.Equal(t, "test-cloud", pool.OCloudID)
    }
}
```

## Authentication

OpenStack supports multiple authentication methods:

```yaml
# Keystone v3 with password
config:
  authUrl: https://openstack.example.com:5000/v3
  username: admin
  password: ${OPENSTACK_PASSWORD}
  projectName: o2ims
  domainName: Default

# Application credentials (recommended)
config:
  authUrl: https://openstack.example.com:5000/v3
  applicationCredentialID: ${APP_CRED_ID}
  applicationCredentialSecret: ${APP_CRED_SECRET}
```

## Performance

- Use pagination for large deployments (1000+ instances)
- Implement caching for flavor list (rarely changes)
- Batch aggregate queries when possible
- Consider Nova API rate limits

## See Also

- [IMS Adapter Interface](README.md)
- [Kubernetes Adapter](kubernetes.md)
- [Cloud Adapters](cloud.md)
