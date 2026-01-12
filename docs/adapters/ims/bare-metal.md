# Bare-Metal IMS Adapters

**Status:** 📋 Specification Complete
**Version:** 1.0
**Last Updated:** 2026-01-12

## Overview

Bare-metal IMS adapters provide O2-IMS infrastructure management for physical servers and hypervisors. These adapters are critical for edge deployments and high-performance workloads requiring direct hardware access.

## Supported Platforms

| Platform | Status | Resource Pools | Resources | Resource Types |
|----------|--------|----------------|-----------|----------------|
| **Dell DTIAS** | 📋 Spec | Server Pool | Physical Server | Server Type |
| **VMware vSphere** | 📋 Spec | Resource Pool/Cluster | ESXi Host/VM | VM Template |

---

## Dell DTIAS Adapter

### Overview

Dell Telecom Infrastructure Automation Software (DTIAS) provides automated lifecycle management for bare-metal servers in telecom edge deployments.

### Resource Mappings

| O2-IMS Concept | DTIAS Resource | API Endpoint |
|----------------|----------------|--------------|
| **Deployment Manager** | DTIAS Datacenter | `/v2/metadata` |
| **Resource Pool** | Server Pool | `/v2/inventory/resourcepools` |
| **Resource** | Physical Server | `/v2/inventory/servers` |
| **Resource Type** | Server Type | `/v2/resourcetypes` |

### DTIAS v2.4.0 API Specifics

**Important Implementation Notes:**

- All responses are wrapped in envelope objects (e.g., `ServersInventoryResponse`, `ResourcePoolsInventoryResponse`)
- `GET /v2/inventory/servers/{Id}` returns `JobResponse` (async job status), **not server data**
- Use `GET /v2/inventory/servers?id={id}` with query parameter to retrieve a specific server
- Query parameters: `resourcePool`, `resourceProfileId`, `location`, `pageSize`, `pageNumber`

### Configuration

```yaml
plugins:
  ims:
    - name: dtias-baremetal
      type: dtias
      enabled: true
      config:
        endpoint: https://dtias.dell.com  # Base URL (v2 prefix added by client)
        apiKey: ${DTIAS_API_KEY}
        timeout: 30s
        ocloudId: ocloud-dtias-edge-1
        datacenter: dc-dallas-1
        clientCert: /etc/dtias/client.crt  # Optional mTLS
        clientKey: /etc/dtias/client.key   # Optional mTLS
        caCert: /etc/dtias/ca.crt          # Optional CA verification
```

### Implementation

```go
// internal/plugins/ims/dtias/plugin.go

package dtias

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin/ims"
    "github.com/yourorg/netweave/pkg/dtias-client"
)

type DTIASPlugin struct {
    name    string
    version string
    client  *dtias.Client
    config  *Config
}

type Config struct {
    Endpoint   string        `yaml:"endpoint"`
    APIKey     string        `yaml:"apiKey"`
    Timeout    time.Duration `yaml:"timeout"`
    OCloudID   string        `yaml:"ocloudId"`
    Datacenter string        `yaml:"datacenter"`
    ClientCert string        `yaml:"clientCert"`
    ClientKey  string        `yaml:"clientKey"`
    CACert     string        `yaml:"caCert"`
}

func NewPlugin(config *Config) (*DTIASPlugin, error) {
    clientConfig := &dtias.ClientConfig{
        Endpoint:   config.Endpoint,
        APIKey:     config.APIKey,
        Timeout:    config.Timeout,
        ClientCert: config.ClientCert,
        ClientKey:  config.ClientKey,
        CACert:     config.CACert,
    }

    client, err := dtias.NewClient(clientConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create DTIAS client: %w", err)
    }

    return &DTIASPlugin{
        name:    "dtias",
        version: "1.0.0",
        client:  client,
        config:  config,
    }, nil
}
```

### List Resource Pools

```go
func (p *DTIASPlugin) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    // DTIAS v2.4.0: GET /v2/inventory/resourcepools returns wrapped response
    path := "/v2/inventory/resourcepools"

    serverPools, err := p.client.FetchServerPools(ctx, path)
    if err != nil {
        return nil, fmt.Errorf("dtias api error: %w", err)
    }

    pools := make([]*ims.ResourcePool, 0, len(serverPools))
    for _, pool := range serverPools {
        resourcePool := p.transformServerPoolToPool(&pool)
        if filter.Matches(resourcePool) {
            pools = append(pools, resourcePool)
        }
    }

    return pools, nil
}

func (p *DTIASPlugin) transformServerPoolToPool(pool *dtias.ServerPool) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: pool.ID,
        Name:          pool.Name,
        Description:   pool.Description,
        Location:      pool.Location,
        OCloudID:      p.config.OCloudID,
        Extensions: map[string]interface{}{
            "dtias.poolType":         pool.Type,
            "dtias.provisioningState": pool.ProvisioningState,
            "dtias.serverCount":      pool.ServerCount,
            "dtias.datacenter":       pool.Datacenter,
            "dtias.rackIds":          pool.RackIDs,
        },
    }
}
```

### List Resources

```go
func (p *DTIASPlugin) ListResources(ctx context.Context, filter *ims.Filter) ([]*ims.Resource, error) {
    // DTIAS v2.4.0: GET /v2/inventory/servers returns ServersInventoryResponse
    path := p.buildServersPath(filter)

    servers, err := p.client.FetchServers(ctx, path)
    if err != nil {
        return nil, fmt.Errorf("dtias api error: %w", err)
    }

    resources := make([]*ims.Resource, 0, len(servers))
    for _, server := range servers {
        resource := p.transformServerToResource(&server)
        if filter.Matches(resource) {
            resources = append(resources, resource)
        }
    }

    return resources, nil
}

func (p *DTIASPlugin) buildServersPath(filter *ims.Filter) string {
    path := "/v2/inventory/servers"
    if filter == nil {
        return path
    }

    queryParams := url.Values{}
    if filter.ResourcePoolID != "" {
        queryParams.Set("resourcePool", filter.ResourcePoolID)
    }
    if filter.ResourceTypeID != "" {
        queryParams.Set("resourceProfileId", filter.ResourceTypeID)
    }
    if filter.Location != "" {
        queryParams.Set("location", filter.Location)
    }
    if filter.Limit > 0 {
        queryParams.Set("pageSize", fmt.Sprintf("%d", filter.Limit))
    }
    if filter.Offset > 0 {
        pageNumber := (filter.Offset / filter.Limit) + 1
        queryParams.Set("pageNumber", fmt.Sprintf("%d", pageNumber))
    }

    if len(queryParams) > 0 {
        path += "?" + queryParams.Encode()
    }
    return path
}
```

### Get Resource (Workaround for v2.4.0)

```go
// GetResource → Retrieve specific server (v2.4.0 workaround)
func (p *DTIASPlugin) GetResource(ctx context.Context, id string) (*ims.Resource, error) {
    // DTIAS v2.4.0: GET /v2/inventory/servers/{Id} returns JobResponse (async operation)
    // Instead, use the list endpoint with id filter to get the server directly
    path := fmt.Sprintf("/v2/inventory/servers?id=%s", id)

    servers, err := p.client.FetchServers(ctx, path)
    if err != nil {
        return nil, fmt.Errorf("dtias api error: %w", err)
    }

    if len(servers) == 0 {
        return nil, fmt.Errorf("server not found: %s", id)
    }

    // Should return exactly one server
    return p.transformServerToResource(&servers[0]), nil
}

func (p *DTIASPlugin) transformServerToResource(server *dtias.Server) *ims.Resource {
    return &ims.Resource{
        ResourceID:     server.ID,
        Name:          server.Hostname,
        Description:   fmt.Sprintf("Bare-metal server: %s", server.Hostname),
        ResourceTypeID: fmt.Sprintf("dtias-server-%s", server.Profile.Type),
        ResourcePoolID: server.ResourcePoolID,
        OCloudID:      p.config.OCloudID,
        Extensions: map[string]interface{}{
            "dtias.serverId":      server.ID,
            "dtias.serverType":    server.Profile.Type,
            "dtias.cpuModel":      server.Hardware.CPU.Model,
            "dtias.cpuCores":      server.Hardware.CPU.TotalCores,
            "dtias.memoryGB":      server.Hardware.Memory.TotalGB,
            "dtias.diskCount":     len(server.Hardware.Disks),
            "dtias.nicCount":      len(server.Hardware.NICs),
            "dtias.provisioningState": server.ProvisioningState,
            "dtias.serviceTag":    server.ServiceTag,
        },
    }
}
```

---

## VMware vSphere Adapter

### Resource Mappings

| O2-IMS Concept | vSphere Resource | vCenter API |
|----------------|------------------|-------------|
| **Deployment Manager** | vCenter Server | `/rest/vcenter` |
| **Resource Pool** | Resource Pool / Cluster | `/rest/vcenter/resource-pool` |
| **Resource** | ESXi Host / VM | `/rest/vcenter/host`, `/rest/vcenter/vm` |
| **Resource Type** | VM Template / Host Profile | `/rest/vcenter/vm-template` |

### Configuration

```yaml
plugins:
  ims:
    - name: vsphere-enterprise
      type: vsphere
      enabled: true
      config:
        vcenterUrl: https://vcenter.example.com/sdk
        username: administrator@vsphere.local
        password: ${VSPHERE_PASSWORD}
        datacenter: Datacenter1
        insecureSkipVerify: false
        ocloudId: ocloud-vsphere-1
```

### Implementation

```go
package vsphere

import (
    "context"
    "github.com/vmware/govmomi"
    "github.com/vmware/govmomi/vim25/soap"
)

type VSphereAdapter struct {
    name    string
    version string
    client  *govmomi.Client
    config  *Config
}

type Config struct {
    VCenterURL         string `yaml:"vcenterUrl"`
    Username           string `yaml:"username"`
    Password           string `yaml:"password"`
    Datacenter         string `yaml:"datacenter"`
    InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
    OCloudID           string `yaml:"ocloudId"`
}

func NewAdapter(config *Config) (*VSphereAdapter, error) {
    ctx := context.Background()

    u, err := soap.ParseURL(config.VCenterURL)
    if err != nil {
        return nil, fmt.Errorf("failed to parse vCenter URL: %w", err)
    }

    u.User = url.UserPassword(config.Username, config.Password)

    client, err := govmomi.NewClient(ctx, u, config.InsecureSkipVerify)
    if err != nil {
        return nil, fmt.Errorf("failed to create vSphere client: %w", err)
    }

    return &VSphereAdapter{
        name:    "vsphere",
        version: "1.0.0",
        client:  client,
        config:  config,
    }, nil
}

func (a *VSphereAdapter) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    finder := find.NewFinder(a.client.Client, true)
    dc, err := finder.Datacenter(ctx, a.config.Datacenter)
    if err != nil {
        return nil, fmt.Errorf("failed to find datacenter: %w", err)
    }

    finder.SetDatacenter(dc)

    resourcePools, err := finder.ResourcePoolList(ctx, "*")
    if err != nil {
        return nil, fmt.Errorf("failed to list resource pools: %w", err)
    }

    pools := make([]*ims.ResourcePool, 0, len(resourcePools))
    for _, rp := range resourcePools {
        pool := a.transformVSpherePoolToResourcePool(rp)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

func (a *VSphereAdapter) transformVSpherePoolToResourcePool(rp *object.ResourcePool) *ims.ResourcePool {
    ctx := context.Background()
    config, _ := rp.Config(ctx)

    return &ims.ResourcePool{
        ResourcePoolID: rp.Reference().Value,
        Name:          rp.Name(),
        Description:   fmt.Sprintf("vSphere resource pool: %s", rp.Name()),
        Location:      a.config.Datacenter,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "vsphere.resourcePoolId": rp.Reference().Value,
            "vsphere.cpuLimit":       config.CpuAllocation.Limit,
            "vsphere.memoryLimit":    config.MemoryAllocation.Limit,
            "vsphere.cpuReservation": config.CpuAllocation.Reservation,
            "vsphere.memoryReservation": config.MemoryAllocation.Reservation,
        },
    }
}
```

## Testing

```go
func TestDTIASAdapter_ListResources(t *testing.T) {
    mockClient := setupMockDTIASClient(t)
    adapter := &DTIASPlugin{
        name:   "dtias",
        client: mockClient,
        config: &Config{OCloudID: "test-cloud"},
    }

    resources, err := adapter.ListResources(context.Background(), &ims.Filter{})
    require.NoError(t, err)
    assert.NotEmpty(t, resources)

    // Verify server properties
    for _, resource := range resources {
        assert.NotEmpty(t, resource.ResourceID)
        assert.NotEmpty(t, resource.Extensions["dtias.cpuCores"])
        assert.NotEmpty(t, resource.Extensions["dtias.memoryGB"])
    }
}
```

## Performance

- **DTIAS**: Use pagination for large deployments (1000+ servers), cache resource types
- **vSphere**: Leverage PropertyCollector for bulk queries, use views for filtering

## Security

- **DTIAS**: Use mTLS for API authentication, rotate API keys regularly
- **vSphere**: Use service accounts with least privilege, enable SSO

## See Also

- [IMS Adapter Interface](README.md)
- [Kubernetes Adapter](kubernetes.md)
- [Cloud Adapters](cloud.md)
