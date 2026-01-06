# netweave Backend Plugins - Comprehensive Architecture

**Version:** 1.0
**Date:** 2026-01-06
**Status:** Complete Architecture Specification

## Table of Contents

1. [Overview](#overview)
2. [Plugin Architecture](#plugin-architecture)
3. [O2-IMS Backend Plugins](#o2-ims-backend-plugins)
4. [O2-DMS Backend Plugins](#o2-dms-backend-plugins)
5. [O2-SMO Integration Plugins](#o2-smo-integration-plugins)
6. [Observability Plugins](#observability-plugins)
7. [Plugin Registry & Routing](#plugin-registry--routing)
8. [Implementation Specifications](#implementation-specifications)

---

## Overview

### Plugin Ecosystem Philosophy

netweave implements a **comprehensive plugin architecture** supporting three distinct plugin categories:

```
┌─────────────────────────────────────────────────────────────────┐
│                    netweave Plugin Ecosystem                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐      │
│  │   O2-IMS      │  │   O2-DMS      │  │   O2-SMO      │      │
│  │   Plugins     │  │   Plugins     │  │   Plugins     │      │
│  │               │  │               │  │               │      │
│  │ Infrastructure│  │  Deployment   │  │ Orchestration │      │
│  │  Management   │  │  Management   │  │  Integration  │      │
│  └───────────────┘  └───────────────┘  └───────────────┘      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Plugin Categories

| Category | Purpose | Plugin Count | Examples |
|----------|---------|--------------|----------|
| **O2-IMS** | Infrastructure resource management | 10+ | Kubernetes, OpenStack, AWS, Azure |
| **O2-DMS** | CNF/VNF deployment lifecycle | 7+ | Helm, ArgoCD, Flux, ONAP-LCM |
| **O2-SMO** | SMO integration & orchestration | 5+ | ONAP, OSM, Custom SMO |
| **Observability** | Metrics, tracing, monitoring | 3+ | Prometheus, Jaeger, Grafana |

### Multi-Layer Integration

```
┌─────────────────────────────────────────────────────────────────┐
│                      O-RAN SMO Systems                          │
│                  (ONAP, OSM, Custom SMO)                        │
└────────────┬────────────────────────────────────────────────────┘
             │
             │ O2-IMS API + O2-DMS API + SMO Northbound API
             ▼
┌─────────────────────────────────────────────────────────────────┐
│                  netweave Unified Gateway                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │ IMS Registry │  │ DMS Registry │  │ SMO Registry │         │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘         │
│         │                 │                  │                  │
│    ┌────┴─────┐      ┌───┴────┐       ┌────┴─────┐           │
│    │ Routing  │      │Routing │       │ Routing  │           │
│    │  Engine  │      │ Engine │       │  Engine  │           │
│    └────┬─────┘      └───┬────┘       └────┬─────┘           │
└─────────┼─────────────────┼─────────────────┼─────────────────┘
          │                 │                 │
          ▼                 ▼                 ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ IMS Backends │  │ DMS Backends │  │ SMO Backends │
│              │  │              │  │              │
│ • K8s        │  │ • Helm       │  │ • ONAP       │
│ • OpenStack  │  │ • ArgoCD     │  │ • OSM        │
│ • AWS        │  │ • Flux       │  │ • Custom     │
│ • VMware     │  │ • ONAP-LCM   │  └──────────────┘
│ • DTIAS      │  │ • OSM-LCM    │
└──────────────┘  └──────────────┘
```

---

## Plugin Architecture

### Unified Plugin Interface

All plugins implement a base interface with category-specific extensions:

```go
// internal/plugin/plugin.go

package plugin

import "context"

// Plugin is the base interface all plugins must implement
type Plugin interface {
    // Metadata
    Name() string                    // Unique plugin identifier
    Version() string                 // Plugin version (semver)
    Category() PluginCategory        // IMS, DMS, SMO, Observability
    Capabilities() []Capability      // Supported operations

    // Lifecycle
    Initialize(ctx context.Context, config map[string]interface{}) error
    Health(ctx context.Context) HealthStatus
    Close() error
}

type PluginCategory string

const (
    CategoryIMS           PluginCategory = "ims"
    CategoryDMS           PluginCategory = "dms"
    CategorySMO           PluginCategory = "smo"
    CategoryObservability PluginCategory = "observability"
)

type Capability string

const (
    // IMS Capabilities
    CapResourcePoolManagement Capability = "resource-pool-management"
    CapResourceManagement     Capability = "resource-management"
    CapResourceTypeDiscovery  Capability = "resource-type-discovery"
    CapRealtimeEvents         Capability = "realtime-events"

    // DMS Capabilities
    CapPackageManagement      Capability = "package-management"
    CapDeploymentLifecycle    Capability = "deployment-lifecycle"
    CapRollback               Capability = "rollback"
    CapScaling                Capability = "scaling"
    CapGitOps                 Capability = "gitops"

    // SMO Capabilities
    CapWorkflowOrchestration  Capability = "workflow-orchestration"
    CapServiceModeling        Capability = "service-modeling"
    CapPolicyManagement       Capability = "policy-management"
    CapInventorySync          Capability = "inventory-sync"

    // Observability Capabilities
    CapMetricsCollection      Capability = "metrics-collection"
    CapDistributedTracing     Capability = "distributed-tracing"
    CapAlertManagement        Capability = "alert-management"
)

type HealthStatus struct {
    Healthy   bool              `json:"healthy"`
    Message   string            `json:"message,omitempty"`
    Details   map[string]string `json:"details,omitempty"`
    Timestamp time.Time         `json:"timestamp"`
}
```

### Category-Specific Interfaces

#### IMS Plugin Interface

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

#### DMS Plugin Interface

```go
// internal/plugin/dms/dms.go

package dms

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin"
)

// DMSPlugin extends the base Plugin interface for deployment management
type DMSPlugin interface {
    plugin.Plugin

    // Package Management
    ListDeploymentPackages(ctx context.Context, filter *Filter) ([]*DeploymentPackage, error)
    GetDeploymentPackage(ctx context.Context, id string) (*DeploymentPackage, error)
    UploadDeploymentPackage(ctx context.Context, pkg *DeploymentPackageUpload) (*DeploymentPackage, error)
    DeleteDeploymentPackage(ctx context.Context, id string) error

    // Deployment Lifecycle
    ListDeployments(ctx context.Context, filter *Filter) ([]*Deployment, error)
    GetDeployment(ctx context.Context, id string) (*Deployment, error)
    CreateDeployment(ctx context.Context, deployment *DeploymentRequest) (*Deployment, error)
    UpdateDeployment(ctx context.Context, id string, update *DeploymentUpdate) (*Deployment, error)
    DeleteDeployment(ctx context.Context, id string) error

    // Operations
    ScaleDeployment(ctx context.Context, id string, replicas int) error
    RollbackDeployment(ctx context.Context, id string, revision int) error
    GetDeploymentStatus(ctx context.Context, id string) (*DeploymentStatus, error)
    GetDeploymentLogs(ctx context.Context, id string, opts *LogOptions) ([]byte, error)

    // Capabilities
    SupportsRollback() bool
    SupportsScaling() bool
    SupportsGitOps() bool
}
```

#### SMO Plugin Interface

```go
// internal/plugin/smo/smo.go

package smo

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin"
)

// SMOPlugin extends the base Plugin interface for SMO integration
type SMOPlugin interface {
    plugin.Plugin

    // Northbound Integration (netweave → SMO)

    // Inventory Synchronization
    SyncInfrastructureInventory(ctx context.Context, inventory *InfrastructureInventory) error
    SyncDeploymentInventory(ctx context.Context, inventory *DeploymentInventory) error

    // Event Publishing
    PublishInfrastructureEvent(ctx context.Context, event *InfrastructureEvent) error
    PublishDeploymentEvent(ctx context.Context, event *DeploymentEvent) error

    // Southbound Integration (SMO → netweave)

    // Workflow Orchestration
    ExecuteWorkflow(ctx context.Context, workflow *WorkflowRequest) (*WorkflowExecution, error)
    GetWorkflowStatus(ctx context.Context, executionID string) (*WorkflowStatus, error)
    CancelWorkflow(ctx context.Context, executionID string) error

    // Service Modeling
    RegisterServiceModel(ctx context.Context, model *ServiceModel) error
    GetServiceModel(ctx context.Context, id string) (*ServiceModel, error)
    ListServiceModels(ctx context.Context) ([]*ServiceModel, error)

    // Policy Management
    ApplyPolicy(ctx context.Context, policy *Policy) error
    GetPolicyStatus(ctx context.Context, policyID string) (*PolicyStatus, error)

    // Capabilities
    SupportsWorkflows() bool
    SupportsServiceModeling() bool
    SupportsPolicyManagement() bool
}
```

---

## O2-IMS Backend Plugins

### Plugin Inventory

| Plugin | Status | Priority | Resource Pools | Resources | Resource Types |
|--------|--------|----------|---------------|-----------|----------------|
| **Kubernetes** | ✅ Core | Production | MachineSet | Node/Machine | StorageClass |
| **Mock** | ✅ Core | Testing | In-Memory | In-Memory | In-Memory |
| **OpenStack** | 📋 Spec | Highest | Host Aggregate | Nova Instance | Flavor |
| **Dell DTIAS** | 📋 Spec | High | Server Pool | Physical Server | Server Type |
| **VMware vSphere** | 📋 Spec | Medium-High | Resource Pool | ESXi Host/VM | VM Template |
| **AWS EKS** | 📋 Spec | Medium | Node Group | EC2 Instance | Instance Type |
| **Azure AKS** | 📋 Spec | Medium | Node Pool | Azure VM | VM SKU |
| **Google GKE** | 📋 Spec | Low-Medium | Node Pool | GCE Instance | Machine Type |
| **Equinix Metal** | 📋 Spec | Low | Metal Pool | Bare-Metal | Server Plan |
| **Red Hat OpenShift** | 📋 Spec | Medium | MachineSet | Node/Machine | MachineConfig |

### 1. Kubernetes Plugin (Core)

**Status:** ✅ Production Ready (Core Implementation)

**Directory:** `internal/plugins/ims/kubernetes/`

**Mapping:**
- **Resource Pool** → MachineSet (OpenShift) / NodePool (GKE/AKS style)
- **Resource** → Node (running) / Machine (lifecycle)
- **Resource Type** → Aggregated from StorageClasses + Machine flavors
- **Deployment Manager** → Cluster metadata (CRD)

**Capabilities:**
```go
capabilities := []Capability{
    CapResourcePoolManagement,
    CapResourceManagement,
    CapResourceTypeDiscovery,
    CapRealtimeEvents,
}
```

**Configuration:**
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

### 2. OpenStack NFVi Plugin

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/ims/openstack/`

**Mapping:**
- **Resource Pool** → Host Aggregate (Nova placement)
- **Resource** → Nova Instance (VM)
- **Resource Type** → Flavor
- **Deployment Manager** → OpenStack Region metadata

**Capabilities:**
```go
capabilities := []Capability{
    CapResourcePoolManagement,
    CapResourceManagement,
    CapResourceTypeDiscovery,
    // No native subscriptions - use polling
}
```

**Configuration:**
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

**Implementation Spec:**
```go
// internal/plugins/ims/openstack/plugin.go

package openstack

import (
    "context"
    "github.com/gophercloud/gophercloud"
    "github.com/gophercloud/gophercloud/openstack"
    "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
    "github.com/gophercloud/gophercloud/openstack/placement/v1/aggregates"
    "github.com/yourorg/netweave/internal/plugin/ims"
)

type OpenStackPlugin struct {
    name         string
    version      string
    provider     *gophercloud.ProviderClient
    compute      *gophercloud.ServiceClient
    placement    *gophercloud.ServiceClient
    config       *Config
}

type Config struct {
    AuthURL      string `yaml:"authUrl"`
    Username     string `yaml:"username"`
    Password     string `yaml:"password"`
    ProjectName  string `yaml:"projectName"`
    DomainName   string `yaml:"domainName"`
    Region       string `yaml:"region"`
    OCloudID     string `yaml:"ocloudId"`
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

// ListResourcePools → List OpenStack Host Aggregates
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

// ListResources → List Nova Instances
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

### 3. Dell DTIAS Bare-Metal Plugin

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/ims/dtias/`

**Mapping:**
- **Resource Pool** → DTIAS Resource Group / Server Pool
- **Resource** → Physical Server
- **Resource Type** → Server Type (compute/storage/network)
- **Deployment Manager** → DTIAS Datacenter metadata

**Capabilities:**
```go
capabilities := []Capability{
    CapResourcePoolManagement,
    CapResourceManagement,
    CapResourceTypeDiscovery,
    // No native subscriptions
}
```

**Configuration:**
```yaml
plugins:
  ims:
    - name: dtias-baremetal
      type: dtias
      enabled: true
      config:
        endpoint: https://dtias.dell.com/api/v1
        apiKey: ${DTIAS_API_KEY}
        timeout: 30s
        ocloudId: ocloud-dtias-edge-1
        datacenter: dc-dallas-1
```

**Implementation Spec:**
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
}

func NewPlugin(config *Config) (*DTIASPlugin, error) {
    client, err := dtias.NewClient(config.Endpoint, config.APIKey)
    if err != nil {
        return nil, err
    }

    return &DTIASPlugin{
        name:    "dtias",
        version: "1.0.0",
        client:  client,
        config:  config,
    }, nil
}

// ListResourcePools → List DTIAS Resource Groups
func (p *DTIASPlugin) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    dtiasGroups, err := p.client.ListResourceGroups(ctx)
    if err != nil {
        return nil, fmt.Errorf("dtias api error: %w", err)
    }

    pools := make([]*ims.ResourcePool, 0, len(dtiasGroups))
    for _, group := range dtiasGroups {
        pool := p.transformResourceGroupToPool(group)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

func (p *DTIASPlugin) transformResourceGroupToPool(group *dtias.ResourceGroup) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: group.ID,
        Name:          group.Name,
        Description:   group.Description,
        Location:      group.DataCenter,
        OCloudID:      p.config.OCloudID,
        Extensions: map[string]interface{}{
            "dtias.resourceGroupType": group.Type,
            "dtias.provisioningState": group.State,
            "dtias.bareMetalCount":    group.ServerCount,
            "dtias.datacenter":        group.DataCenter,
        },
    }
}

// ListResources → List DTIAS Physical Servers
func (p *DTIASPlugin) ListResources(ctx context.Context, filter *ims.Filter) ([]*ims.Resource, error) {
    servers, err := p.client.ListServers(ctx)
    if err != nil {
        return nil, fmt.Errorf("dtias api error: %w", err)
    }

    resources := make([]*ims.Resource, 0, len(servers))
    for _, server := range servers {
        resource := p.transformServerToResource(server)
        if filter.Matches(resource) {
            resources = append(resources, resource)
        }
    }

    return resources, nil
}

func (p *DTIASPlugin) transformServerToResource(server *dtias.Server) *ims.Resource {
    return &ims.Resource{
        ResourceID:     server.ID,
        Name:          server.Hostname,
        Description:   fmt.Sprintf("Bare-metal server: %s", server.Hostname),
        ResourceTypeID: fmt.Sprintf("dtias-server-%s", server.Type),
        ResourcePoolID: server.ResourceGroupID,
        OCloudID:      p.config.OCloudID,
        Extensions: map[string]interface{}{
            "dtias.serverId":      server.ID,
            "dtias.serverType":    server.Type,
            "dtias.cpuModel":      server.CPU.Model,
            "dtias.cpuCores":      server.CPU.Cores,
            "dtias.memoryGB":      server.Memory.TotalGB,
            "dtias.diskCount":     len(server.Disks),
            "dtias.nicCount":      len(server.NICs),
            "dtias.provisioningState": server.State,
        },
    }
}
```

### 4. VMware vSphere Plugin

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/ims/vsphere/`

**Mapping:**
- **Resource Pool** → vSphere Resource Pool / Cluster
- **Resource** → ESXi Host / Virtual Machine
- **Resource Type** → VM Template / Host Profile
- **Deployment Manager** → vCenter metadata

**Configuration:**
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

### 5-9. Additional IMS Plugins

**AWS EKS, Azure AKS, Google GKE, Equinix Metal, Red Hat OpenShift**

Similar structure to above plugins, each with specific API client implementations and resource transformations.

---

## O2-DMS Backend Plugins

### Plugin Inventory

| Plugin | Status | Priority | Package Format | Deployment Target | GitOps |
|--------|--------|----------|---------------|-------------------|--------|
| **Helm** | 📋 Spec | Highest | Helm Chart | Kubernetes | No |
| **ArgoCD** | 📋 Spec | Highest | Git Repo | Kubernetes | Yes |
| **Flux CD** | 📋 Spec | Medium | Git Repo | Kubernetes | Yes |
| **Kustomize** | 📋 Spec | Low-Medium | Git Repo | Kubernetes | Partial |
| **ONAP-LCM** | 📋 Spec | High | ONAP Package | Multi-Cloud | No |
| **OSM-LCM** | 📋 Spec | Medium | OSM Package | Multi-Cloud | No |
| **Crossplane** | 📋 Spec | Low | Crossplane XR | Multi-Cloud | Partial |

### 1. Helm Plugin (Core DMS)

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/dms/helm/`

**Capabilities:**
```go
capabilities := []Capability{
    CapPackageManagement,
    CapDeploymentLifecycle,
    CapRollback,
    CapScaling,  // Via values override
}
```

**Configuration:**
```yaml
plugins:
  dms:
    - name: helm-deployer
      type: helm
      enabled: true
      default: true
      config:
        kubeconfig: /etc/kubernetes/admin.conf
        chartRepository: https://charts.example.com
        namespace: deployments
        timeout: 10m
        maxHistory: 10
```

**Implementation Spec:**
```go
// internal/plugins/dms/helm/plugin.go

package helm

import (
    "context"
    "helm.sh/helm/v3/pkg/action"
    "helm.sh/helm/v3/pkg/chart/loader"
    "helm.sh/helm/v3/pkg/cli"
    "github.com/yourorg/netweave/internal/plugin/dms"
)

type HelmPlugin struct {
    name      string
    version   string
    config    *Config
    settings  *cli.EnvSettings
    actionCfg *action.Configuration
}

type Config struct {
    Kubeconfig      string `yaml:"kubeconfig"`
    ChartRepository string `yaml:"chartRepository"`
    Namespace       string `yaml:"namespace"`
    Timeout         string `yaml:"timeout"`
    MaxHistory      int    `yaml:"maxHistory"`
}

func NewPlugin(config *Config) (*HelmPlugin, error) {
    settings := cli.New()
    settings.KubeConfig = config.Kubeconfig

    actionCfg := new(action.Configuration)
    if err := actionCfg.Init(settings.RESTClientGetter(), config.Namespace, "secret", log.Printf); err != nil {
        return nil, err
    }

    return &HelmPlugin{
        name:      "helm",
        version:   "3.14.0",
        config:    config,
        settings:  settings,
        actionCfg: actionCfg,
    }, nil
}

// UploadDeploymentPackage → Store Helm Chart
func (p *HelmPlugin) UploadDeploymentPackage(ctx context.Context, pkg *dms.DeploymentPackageUpload) (*dms.DeploymentPackage, error) {
    // Upload chart to repository (ChartMuseum, Harbor, etc.)
    // For now, assume charts are in a repository

    return &dms.DeploymentPackage{
        ID:          generateID(),
        Name:        pkg.Name,
        Version:     pkg.Version,
        PackageType: "helm-chart",
        UploadedAt:  time.Now(),
        Extensions: map[string]interface{}{
            "helm.chartName":    pkg.Name,
            "helm.chartVersion": pkg.Version,
            "helm.repository":   p.config.ChartRepository,
        },
    }, nil
}

// CreateDeployment → Helm Install
func (p *HelmPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    client := action.NewInstall(p.actionCfg)
    client.Namespace = req.Namespace
    client.ReleaseName = req.Name
    client.Wait = true
    client.Timeout = parseDuration(p.config.Timeout)

    // Load chart
    chartPath, err := client.LocateChart(req.PackageID, p.settings)
    if err != nil {
        return nil, err
    }

    chart, err := loader.Load(chartPath)
    if err != nil {
        return nil, err
    }

    // Install release
    release, err := client.Run(chart, req.Values)
    if err != nil {
        return nil, fmt.Errorf("helm install failed: %w", err)
    }

    return &dms.Deployment{
        ID:          release.Name,
        Name:        release.Name,
        PackageID:   req.PackageID,
        Namespace:   release.Namespace,
        Status:      transformHelmStatus(release.Info.Status),
        Version:     release.Version,
        CreatedAt:   release.Info.FirstDeployed.Time,
        UpdatedAt:   release.Info.LastDeployed.Time,
        Extensions: map[string]interface{}{
            "helm.releaseName": release.Name,
            "helm.revision":    release.Version,
            "helm.chart":       release.Chart.Name(),
        },
    }, nil
}

// RollbackDeployment → Helm Rollback
func (p *HelmPlugin) RollbackDeployment(ctx context.Context, id string, revision int) error {
    client := action.NewRollback(p.actionCfg)
    client.Version = revision
    client.Wait = true
    client.Timeout = parseDuration(p.config.Timeout)

    return client.Run(id)
}

// ScaleDeployment → Update Helm values (replicas)
func (p *HelmPlugin) ScaleDeployment(ctx context.Context, id string, replicas int) error {
    client := action.NewUpgrade(p.actionCfg)
    client.Wait = true
    client.Timeout = parseDuration(p.config.Timeout)

    // Get current release
    getClient := action.NewGet(p.actionCfg)
    release, err := getClient.Run(id)
    if err != nil {
        return err
    }

    // Override replicas in values
    values := release.Config
    values["replicaCount"] = replicas

    // Upgrade release with new values
    _, err = client.Run(id, release.Chart, values)
    return err
}
```

### 2. ArgoCD GitOps Plugin

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/dms/argocd/`

**Capabilities:**
```go
capabilities := []Capability{
    CapPackageManagement,  // Git repo management
    CapDeploymentLifecycle,
    CapRollback,
    CapGitOps,
}
```

**Configuration:**
```yaml
plugins:
  dms:
    - name: argocd-gitops
      type: argocd
      enabled: true
      config:
        serverUrl: https://argocd.example.com
        authToken: ${ARGOCD_AUTH_TOKEN}
        namespace: argocd
        defaultProject: default
        syncPolicy:
          automated: true
          prune: true
          selfHeal: true
```

**Implementation Spec:**
```go
// internal/plugins/dms/argocd/plugin.go

package argocd

import (
    "context"
    "github.com/argoproj/argo-cd/v2/pkg/apiclient"
    "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
    "github.com/yourorg/netweave/internal/plugin/dms"
)

type ArgoCDPlugin struct {
    name    string
    version string
    client  apiclient.Client
    appIf   application.ApplicationServiceClient
    config  *Config
}

type Config struct {
    ServerURL      string `yaml:"serverUrl"`
    AuthToken      string `yaml:"authToken"`
    Namespace      string `yaml:"namespace"`
    DefaultProject string `yaml:"defaultProject"`
    SyncPolicy     struct {
        Automated bool `yaml:"automated"`
        Prune     bool `yaml:"prune"`
        SelfHeal  bool `yaml:"selfHeal"`
    } `yaml:"syncPolicy"`
}

func NewPlugin(config *Config) (*ArgoCDPlugin, error) {
    client, err := apiclient.NewClient(&apiclient.ClientOptions{
        ServerAddr: config.ServerURL,
        AuthToken:  config.AuthToken,
    })
    if err != nil {
        return nil, err
    }

    _, appIf, err := client.NewApplicationClient()
    if err != nil {
        return nil, err
    }

    return &ArgoCDPlugin{
        name:    "argocd",
        version: "2.10.0",
        client:  client,
        appIf:   appIf,
        config:  config,
    }, nil
}

// CreateDeployment → Create ArgoCD Application
func (p *ArgoCDPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    app := &argocdv1alpha1.Application{
        ObjectMeta: metav1.ObjectMeta{
            Name:      req.Name,
            Namespace: p.config.Namespace,
        },
        Spec: argocdv1alpha1.ApplicationSpec{
            Project: p.config.DefaultProject,
            Source: argocdv1alpha1.ApplicationSource{
                RepoURL:        req.GitRepo,
                TargetRevision: req.GitRevision,
                Path:           req.GitPath,
                Helm: &argocdv1alpha1.ApplicationSourceHelm{
                    Values: marshalValues(req.Values),
                },
            },
            Destination: argocdv1alpha1.ApplicationDestination{
                Server:    "https://kubernetes.default.svc",
                Namespace: req.Namespace,
            },
            SyncPolicy: &argocdv1alpha1.SyncPolicy{
                Automated: &argocdv1alpha1.SyncPolicyAutomated{
                    Prune:    p.config.SyncPolicy.Prune,
                    SelfHeal: p.config.SyncPolicy.SelfHeal,
                },
            },
        },
    }

    created, err := p.appIf.Create(ctx, &application.ApplicationCreateRequest{
        Application: app,
    })
    if err != nil {
        return nil, fmt.Errorf("argocd create failed: %w", err)
    }

    return &dms.Deployment{
        ID:        created.Name,
        Name:      created.Name,
        PackageID: req.GitRepo,
        Namespace: req.Namespace,
        Status:    transformArgoCDStatus(created.Status.Health.Status),
        CreatedAt: created.CreationTimestamp.Time,
        Extensions: map[string]interface{}{
            "argocd.appName":      created.Name,
            "argocd.project":      created.Spec.Project,
            "argocd.repoURL":      created.Spec.Source.RepoURL,
            "argocd.revision":     created.Spec.Source.TargetRevision,
            "argocd.syncStatus":   created.Status.Sync.Status,
            "argocd.healthStatus": created.Status.Health.Status,
        },
    }, nil
}

// RollbackDeployment → Rollback to previous Git revision
func (p *ArgoCDPlugin) RollbackDeployment(ctx context.Context, id string, revision int) error {
    // Get application history
    app, err := p.appIf.Get(ctx, &application.ApplicationQuery{Name: &id})
    if err != nil {
        return err
    }

    // Find target revision in history
    if revision >= len(app.Status.History) {
        return fmt.Errorf("revision %d not found", revision)
    }

    targetRevision := app.Status.History[revision].Revision

    // Update application to target revision
    _, err = p.appIf.UpdateSpec(ctx, &application.ApplicationUpdateSpecRequest{
        Name: &id,
        Spec: &argocdv1alpha1.ApplicationSpec{
            Source: argocdv1alpha1.ApplicationSource{
                TargetRevision: targetRevision,
            },
        },
    })

    return err
}
```

### 3-7. Additional DMS Plugins

**Flux CD, Kustomize, ONAP-LCM, OSM-LCM, Crossplane**

Similar structure with specific implementations for each deployment technology.

---

## O2-SMO Integration Plugins

### SMO Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                       O-RAN SMO Layer                           │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐      │
│  │ ONAP          │  │ OSM           │  │ Custom SMO    │      │
│  │ (AT&T, Amdocs)│  │ (ETSI-hosted) │  │ (Vendor)      │      │
│  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘      │
│          │                  │                  │               │
└──────────┼──────────────────┼──────────────────┼───────────────┘
           │                  │                  │
           │ Northbound APIs  │                  │
           ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              netweave Unified Gateway                           │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ SMO Plugin Registry                                      │  │
│  │ • ONAP Plugin (northbound + DMS backend)                 │  │
│  │ • OSM Plugin (northbound + DMS backend)                  │  │
│  │ • Custom SMO Plugin                                      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │ O2-IMS API   │  │ O2-DMS API   │  │ SMO North API│         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

### Dual-Mode SMO Integration

SMO plugins operate in **two modes**:

1. **Northbound Mode**: netweave → SMO (inventory sync, event publishing)
2. **DMS Backend Mode**: SMO orchestrates deployments through netweave DMS API

### Plugin Inventory

| Plugin | Status | Priority | Northbound | DMS Backend | Workflow Engine |
|--------|--------|----------|------------|-------------|-----------------|
| **ONAP** | 📋 Spec | Highest | Yes | Yes (SO + SDNC) | Yes |
| **OSM** | 📋 Spec | High | Yes | Yes (LCM) | Yes |
| **Custom SMO** | 📋 Spec | Medium | Yes | Configurable | Optional |
| **Cloudify** | 📋 Spec | Low | No | Yes (TOSCA) | Yes |
| **Camunda** | 📋 Spec | Low | No | No (Workflow only) | Yes |

### 1. ONAP Integration Plugin

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/smo/onap/`

**Dual Role:**
- **Northbound**: Sync inventory to ONAP A&AI, publish events to ONAP DMaaP
- **DMS Backend**: Execute deployments via ONAP SO (Service Orchestrator)

**Capabilities:**
```go
capabilities := []Capability{
    CapInventorySync,
    CapWorkflowOrchestration,
    CapServiceModeling,
    CapPolicyManagement,
}
```

**Configuration:**
```yaml
plugins:
  smo:
    - name: onap-integration
      type: onap
      enabled: true
      config:
        # Northbound Configuration
        aaiUrl: https://onap-aai.example.com:8443
        dmaapUrl: https://onap-dmaap.example.com:3904

        # DMS Backend Configuration
        soUrl: https://onap-so.example.com:8080
        sdncUrl: https://onap-sdnc.example.com:8282

        # Authentication
        username: aai@aai.onap.org
        password: ${ONAP_PASSWORD}

        # Settings
        inventorySyncInterval: 5m
        eventPublishBatchSize: 100
```

**Implementation Spec:**
```go
// internal/plugins/smo/onap/plugin.go

package onap

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin/smo"
    "github.com/yourorg/netweave/internal/plugin/dms"
)

type ONAPPlugin struct {
    name    string
    version string
    config  *Config

    // Northbound clients
    aaiClient   *AAIClient
    dmaapClient *DMaaPClient

    // DMS backend clients
    soClient   *ServiceOrchestratorClient
    sdncClient *SDNCClient
}

type Config struct {
    // Northbound
    AAIURL   string `yaml:"aaiUrl"`
    DMaaPURL string `yaml:"dmaapUrl"`

    // DMS Backend
    SOURL   string `yaml:"soUrl"`
    SDNCURL string `yaml:"sdncUrl"`

    // Auth
    Username string `yaml:"username"`
    Password string `yaml:"password"`

    // Settings
    InventorySyncInterval time.Duration `yaml:"inventorySyncInterval"`
    EventPublishBatchSize int           `yaml:"eventPublishBatchSize"`
}

func NewPlugin(config *Config) (*ONAPPlugin, error) {
    aaiClient := NewAAIClient(config.AAIURL, config.Username, config.Password)
    dmaapClient := NewDMaaPClient(config.DMaaPURL, config.Username, config.Password)
    soClient := NewSOClient(config.SOURL, config.Username, config.Password)
    sdncClient := NewSDNCClient(config.SDNCURL, config.Username, config.Password)

    return &ONAPPlugin{
        name:        "onap",
        version:     "1.0.0",
        config:      config,
        aaiClient:   aaiClient,
        dmaapClient: dmaapClient,
        soClient:    soClient,
        sdncClient:  sdncClient,
    }, nil
}

// === NORTHBOUND MODE: Inventory Sync ===

// SyncInfrastructureInventory → Sync to ONAP A&AI
func (p *ONAPPlugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
    // Transform netweave inventory to ONAP A&AI format
    aaiInventory := p.transformToAAIInventory(inventory)

    // Sync to A&AI (Cloud Regions, Tenants, VNFs, PNFs)
    for _, cloudRegion := range aaiInventory.CloudRegions {
        if err := p.aaiClient.CreateOrUpdateCloudRegion(ctx, cloudRegion); err != nil {
            return fmt.Errorf("failed to sync cloud region: %w", err)
        }
    }

    for _, pnf := range aaiInventory.PNFs {
        if err := p.aaiClient.CreateOrUpdatePNF(ctx, pnf); err != nil {
            return fmt.Errorf("failed to sync PNF: %w", err)
        }
    }

    return nil
}

// PublishInfrastructureEvent → Publish to ONAP DMaaP
func (p *ONAPPlugin) PublishInfrastructureEvent(ctx context.Context, event *smo.InfrastructureEvent) error {
    // Transform to VES (Virtual Event Streaming) format
    vesEvent := p.transformToVESEvent(event)

    // Publish to DMaaP topic
    topic := "unauthenticated.VES_INFRASTRUCTURE_EVENTS"
    return p.dmaapClient.PublishEvent(ctx, topic, vesEvent)
}

// === DMS BACKEND MODE: Deployment Lifecycle ===

// Implements dms.DMSPlugin interface as well

// CreateDeployment → Trigger ONAP SO orchestration
func (p *ONAPPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    // Create ONAP service instance request
    soRequest := &ServiceInstanceRequest{
        RequestDetails: RequestDetails{
            ModelInfo: ModelInfo{
                ModelType:         "service",
                ModelInvariantId:  req.ServiceModelID,
                ModelVersionId:    req.ServiceModelVersion,
                ModelName:         req.Name,
            },
            CloudConfiguration: CloudConfiguration{
                TenantID:       req.TenantID,
                CloudRegionID:  req.CloudRegion,
            },
            RequestInfo: RequestInfo{
                InstanceName:   req.Name,
                Source:         "netweave-o2ims",
                RequestorID:    "o2ims-gateway",
            },
            RequestParameters: RequestParameters{
                UserParams: req.Values,
            },
        },
    }

    // Submit to ONAP SO
    response, err := p.soClient.CreateServiceInstance(ctx, soRequest)
    if err != nil {
        return nil, fmt.Errorf("onap so create failed: %w", err)
    }

    return &dms.Deployment{
        ID:        response.ServiceInstanceID,
        Name:      req.Name,
        PackageID: req.ServiceModelID,
        Status:    p.mapONAPStatus(response.RequestState),
        CreatedAt: time.Now(),
        Extensions: map[string]interface{}{
            "onap.serviceInstanceId": response.ServiceInstanceID,
            "onap.requestId":         response.RequestID,
            "onap.orchestrationStatus": response.RequestState,
        },
    }, nil
}

// GetDeploymentStatus → Query ONAP SO orchestration status
func (p *ONAPPlugin) GetDeploymentStatus(ctx context.Context, id string) (*dms.DeploymentStatus, error) {
    // Query A&AI for service instance
    serviceInstance, err := p.aaiClient.GetServiceInstance(ctx, id)
    if err != nil {
        return nil, err
    }

    // Query SO for orchestration status
    orchestrationStatus, err := p.soClient.GetOrchestrationStatus(ctx, serviceInstance.OrchestrationRequestID)
    if err != nil {
        return nil, err
    }

    return &dms.DeploymentStatus{
        DeploymentID: id,
        Status:       p.mapONAPStatus(orchestrationStatus.RequestState),
        Progress:     orchestrationStatus.PercentProgress,
        Message:      orchestrationStatus.StatusMessage,
        UpdatedAt:    orchestrationStatus.FinishTime,
        Extensions: map[string]interface{}{
            "onap.requestState":     orchestrationStatus.RequestState,
            "onap.percentProgress":  orchestrationStatus.PercentProgress,
            "onap.orchestrationFlows": orchestrationStatus.FlowStatus,
        },
    }, nil
}

// DeleteDeployment → Trigger ONAP service instance deletion
func (p *ONAPPlugin) DeleteDeployment(ctx context.Context, id string) error {
    deleteRequest := &ServiceInstanceDeleteRequest{
        RequestDetails: RequestDetails{
            RequestInfo: RequestInfo{
                Source:      "netweave-o2ims",
                RequestorID: "o2ims-gateway",
            },
        },
    }

    _, err := p.soClient.DeleteServiceInstance(ctx, id, deleteRequest)
    return err
}

// === WORKFLOW ORCHESTRATION ===

// ExecuteWorkflow → Execute ONAP workflow (Camunda BPMN)
func (p *ONAPPlugin) ExecuteWorkflow(ctx context.Context, workflow *smo.WorkflowRequest) (*smo.WorkflowExecution, error) {
    // Submit workflow to ONAP SO Camunda engine
    execution, err := p.soClient.ExecuteWorkflow(ctx, workflow.WorkflowName, workflow.Parameters)
    if err != nil {
        return nil, err
    }

    return &smo.WorkflowExecution{
        ExecutionID:  execution.ProcessInstanceID,
        WorkflowName: workflow.WorkflowName,
        Status:       "RUNNING",
        StartedAt:    time.Now(),
        Extensions: map[string]interface{}{
            "onap.processInstanceId": execution.ProcessInstanceID,
            "onap.engineName":        "camunda",
        },
    }, nil
}
```

### 2. OSM Integration Plugin

**Status:** 📋 Specification Complete

**Directory:** `internal/plugins/smo/osm/`

**Dual Role:**
- **Northbound**: Sync inventory to OSM, publish events
- **DMS Backend**: Execute NS/VNF deployments via OSM LCM

**Capabilities:**
```go
capabilities := []Capability{
    CapInventorySync,
    CapWorkflowOrchestration,
    CapServiceModeling,  // NS/VNF descriptors
}
```

**Configuration:**
```yaml
plugins:
  smo:
    - name: osm-integration
      type: osm
      enabled: true
      config:
        # OSM NBI (Northbound Interface)
        nbiUrl: https://osm.example.com:9999
        username: admin
        password: ${OSM_PASSWORD}
        project: admin

        # Settings
        inventorySyncInterval: 5m
        lcmPollingInterval: 10s
```

**Implementation Spec:**
```go
// internal/plugins/smo/osm/plugin.go

package osm

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin/smo"
    "github.com/yourorg/netweave/internal/plugin/dms"
)

type OSMPlugin struct {
    name    string
    version string
    config  *Config
    client  *OSMClient
}

type Config struct {
    NBIURL                string        `yaml:"nbiUrl"`
    Username              string        `yaml:"username"`
    Password              string        `yaml:"password"`
    Project               string        `yaml:"project"`
    InventorySyncInterval time.Duration `yaml:"inventorySyncInterval"`
    LCMPollingInterval    time.Duration `yaml:"lcmPollingInterval"`
}

func NewPlugin(config *Config) (*OSMPlugin, error) {
    client := NewOSMClient(config.NBIURL, config.Username, config.Password, config.Project)

    return &OSMPlugin{
        name:    "osm",
        version: "1.0.0",
        config:  config,
        client:  client,
    }, nil
}

// === NORTHBOUND MODE ===

// SyncInfrastructureInventory → Sync VIMs to OSM
func (p *OSMPlugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
    // Transform to OSM VIM accounts
    for _, vim := range p.transformToOSMVIMs(inventory) {
        if err := p.client.CreateOrUpdateVIM(ctx, vim); err != nil {
            return fmt.Errorf("failed to sync VIM: %w", err)
        }
    }
    return nil
}

// === DMS BACKEND MODE ===

// UploadDeploymentPackage → Upload NSD/VNFD to OSM
func (p *OSMPlugin) UploadDeploymentPackage(ctx context.Context, pkg *dms.DeploymentPackageUpload) (*dms.DeploymentPackage, error) {
    var packageID string
    var err error

    switch pkg.PackageType {
    case "nsd":
        packageID, err = p.client.OnboardNSD(ctx, pkg.Content)
    case "vnfd":
        packageID, err = p.client.OnboardVNFD(ctx, pkg.Content)
    default:
        return nil, fmt.Errorf("unsupported package type: %s", pkg.PackageType)
    }

    if err != nil {
        return nil, err
    }

    return &dms.DeploymentPackage{
        ID:          packageID,
        Name:        pkg.Name,
        Version:     pkg.Version,
        PackageType: pkg.PackageType,
        UploadedAt:  time.Now(),
        Extensions: map[string]interface{}{
            "osm.packageType": pkg.PackageType,
            "osm.packageId":   packageID,
        },
    }, nil
}

// CreateDeployment → Instantiate NS in OSM
func (p *OSMPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    nsRequest := &NSInstantiateRequest{
        NSName:        req.Name,
        NSDId:         req.PackageID,
        VIMAccountId:  req.VIMAccount,
        AdditionalParams: req.Values,
    }

    nsInstanceID, err := p.client.InstantiateNS(ctx, nsRequest)
    if err != nil {
        return nil, fmt.Errorf("osm ns instantiate failed: %w", err)
    }

    return &dms.Deployment{
        ID:        nsInstanceID,
        Name:      req.Name,
        PackageID: req.PackageID,
        Status:    "BUILDING",
        CreatedAt: time.Now(),
        Extensions: map[string]interface{}{
            "osm.nsInstanceId": nsInstanceID,
            "osm.nsdId":        req.PackageID,
        },
    }, nil
}

// GetDeploymentStatus → Query OSM NS instance status
func (p *OSMPlugin) GetDeploymentStatus(ctx context.Context, id string) (*dms.DeploymentStatus, error) {
    nsInstance, err := p.client.GetNSInstance(ctx, id)
    if err != nil {
        return nil, err
    }

    return &dms.DeploymentStatus{
        DeploymentID: id,
        Status:       p.mapOSMStatus(nsInstance.OperationalStatus),
        Message:      nsInstance.DetailedStatus,
        UpdatedAt:    nsInstance.ModifyTime,
        Extensions: map[string]interface{}{
            "osm.operationalStatus":    nsInstance.OperationalStatus,
            "osm.configStatus":         nsInstance.ConfigStatus,
            "osm.vnfInstances":         len(nsInstance.ConstituentVNFRIds),
            "osm.detailedStatus":       nsInstance.DetailedStatus,
        },
    }, nil
}

// ScaleDeployment → Scale NS (add/remove VNF instances)
func (p *OSMPlugin) ScaleDeployment(ctx context.Context, id string, replicas int) error {
    scaleRequest := &NSScaleRequest{
        ScaleType:      "SCALE_VNF",
        ScaleVnfData: ScaleVnfData{
            ScaleVnfType:      "SCALE_OUT",
            ScaleByStepData: ScaleByStepData{
                ScalingGroupDescriptor: "default",
                MemberVnfIndex:         "1",
            },
        },
    }

    return p.client.ScaleNS(ctx, id, scaleRequest)
}
```

### 3-5. Additional SMO Plugins

**Custom SMO, Cloudify, Camunda**

Similar implementations for custom SMO frameworks and workflow engines.

---

## Observability Plugins

### Plugin Inventory

| Plugin | Status | Priority | Metrics | Traces | Alerts |
|--------|--------|----------|---------|--------|--------|
| **Prometheus/Thanos** | 📋 Spec | High | Yes | No | Yes |
| **Jaeger** | 📋 Spec | Medium | No | Yes | No |
| **Grafana Loki** | 📋 Spec | Medium | No | Logs | No |

### Implementation Specifications

All observability plugins integrate with netweave's existing observability framework and extend it with plugin-specific backends.

---

## Plugin Registry & Routing

### Unified Plugin Registry

```go
// internal/plugin/registry.go

package plugin

import (
    "context"
    "sync"
)

// Registry manages all plugin categories
type Registry struct {
    mu sync.RWMutex

    // Plugin stores by category
    imsPlugins    map[string]ims.IMSPlugin
    dmsPlugins    map[string]dms.DMSPlugin
    smoPlugins    map[string]smo.SMOPlugin
    observability map[string]ObservabilityPlugin

    // Routing rules
    imsRoutes []IMSRoutingRule
    dmsRoutes []DMSRoutingRule
    smoRoutes []SMORoutingRule

    // Default plugins
    defaultIMS string
    defaultDMS string
    defaultSMO string
}

type IMSRoutingRule struct {
    Name         string
    Priority     int
    PluginName   string
    ResourceType string
    Conditions   map[string]interface{}
}

type DMSRoutingRule struct {
    Name         string
    Priority     int
    PluginName   string
    PackageType  string
    Conditions   map[string]interface{}
}

type SMORoutingRule struct {
    Name         string
    Priority     int
    PluginName   string
    Mode         string  // "northbound" or "dms-backend"
    Conditions   map[string]interface{}
}

// RouteIMS determines which IMS plugin to use
func (r *Registry) RouteIMS(resourceType string, filter *ims.Filter) (ims.IMSPlugin, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Check routing rules (highest priority first)
    for _, rule := range r.sortedIMSRules() {
        if rule.ResourceType == resourceType && rule.MatchesConditions(filter) {
            if plugin, ok := r.imsPlugins[rule.PluginName]; ok {
                return plugin, nil
            }
        }
    }

    // Fallback to default
    if plugin, ok := r.imsPlugins[r.defaultIMS]; ok {
        return plugin, nil
    }

    return nil, fmt.Errorf("no IMS plugin found for resource type %s", resourceType)
}

// RouteDMS determines which DMS plugin to use
func (r *Registry) RouteDMS(packageType string, req *dms.DeploymentRequest) (dms.DMSPlugin, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Check routing rules
    for _, rule := range r.sortedDMSRules() {
        if rule.PackageType == packageType && rule.MatchesConditions(req) {
            if plugin, ok := r.dmsPlugins[rule.PluginName]; ok {
                return plugin, nil
            }
        }
    }

    // Fallback to default
    if plugin, ok := r.dmsPlugins[r.defaultDMS]; ok {
        return plugin, nil
    }

    return nil, fmt.Errorf("no DMS plugin found for package type %s", packageType)
}

// GetSMOPlugin returns SMO plugin by name or default
func (r *Registry) GetSMOPlugin(name string) (smo.SMOPlugin, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    if name == "" {
        name = r.defaultSMO
    }

    if plugin, ok := r.smoPlugins[name]; ok {
        return plugin, nil
    }

    return nil, fmt.Errorf("SMO plugin %s not found", name)
}
```

### Configuration-Driven Routing

```yaml
# config/plugins.yaml

plugins:
  # IMS Plugins
  ims:
    - name: kubernetes
      type: kubernetes
      enabled: true
      default: true
      config:
        kubeconfig: /etc/kubernetes/admin.conf
        ocloudId: ocloud-k8s-1

    - name: openstack-nfv
      type: openstack
      enabled: true
      config:
        authUrl: https://openstack.example.com:5000/v3
        username: admin
        password: ${OPENSTACK_PASSWORD}
        ocloudId: ocloud-openstack-1

    - name: dtias-edge
      type: dtias
      enabled: true
      config:
        endpoint: https://dtias.dell.com/api/v1
        apiKey: ${DTIAS_API_KEY}
        ocloudId: ocloud-dtias-edge-1

  # DMS Plugins
  dms:
    - name: helm-deployer
      type: helm
      enabled: true
      default: true
      config:
        kubeconfig: /etc/kubernetes/admin.conf
        namespace: deployments

    - name: argocd-gitops
      type: argocd
      enabled: true
      config:
        serverUrl: https://argocd.example.com
        authToken: ${ARGOCD_AUTH_TOKEN}

    - name: onap-lcm
      type: onap-dms
      enabled: true
      config:
        soUrl: https://onap-so.example.com:8080
        sdncUrl: https://onap-sdnc.example.com:8282

  # SMO Plugins
  smo:
    - name: onap-integration
      type: onap
      enabled: true
      default: true
      config:
        aaiUrl: https://onap-aai.example.com:8443
        dmaapUrl: https://onap-dmaap.example.com:3904
        soUrl: https://onap-so.example.com:8080

    - name: osm-integration
      type: osm
      enabled: true
      config:
        nbiUrl: https://osm.example.com:9999
        username: admin
        password: ${OSM_PASSWORD}

# Routing Rules
routing:
  ims:
    rules:
      # OpenStack for legacy NFV
      - name: openstack-nfv-resources
        priority: 100
        plugin: openstack-nfv
        resourceType: "*"
        conditions:
          labels:
            infrastructure.type: openstack

      # Bare-metal to DTIAS
      - name: bare-metal-edge
        priority: 95
        plugin: dtias-edge
        resourceType: "*"
        conditions:
          labels:
            infrastructure.type: bare-metal

      # Default to Kubernetes
      - name: default-kubernetes
        priority: 1
        plugin: kubernetes
        resourceType: "*"

  dms:
    rules:
      # GitOps deployments to ArgoCD
      - name: gitops-argocd
        priority: 100
        plugin: argocd-gitops
        packageType: git-repo
        conditions:
          gitOps: true

      # ONAP service models to ONAP LCM
      - name: onap-services
        priority: 95
        plugin: onap-lcm
        packageType: onap-service

      # Default to Helm
      - name: default-helm
        priority: 1
        plugin: helm-deployer
        packageType: helm-chart

  smo:
    # SMO plugins don't use automatic routing
    # They're invoked explicitly via API or configuration
    default: onap-integration
```

---

## Implementation Specifications

### Directory Structure

```
netweave/
├── internal/
│   ├── plugin/
│   │   ├── plugin.go                 # Base plugin interface
│   │   ├── registry.go               # Unified plugin registry
│   │   ├── loader.go                 # Dynamic plugin loading
│   │   │
│   │   ├── ims/
│   │   │   ├── ims.go                # IMS plugin interface
│   │   │   ├── filter.go             # Filter/query structures
│   │   │   └── models.go             # IMS data models
│   │   │
│   │   ├── dms/
│   │   │   ├── dms.go                # DMS plugin interface
│   │   │   ├── lifecycle.go          # Deployment lifecycle
│   │   │   └── models.go             # DMS data models
│   │   │
│   │   └── smo/
│   │       ├── smo.go                # SMO plugin interface
│   │       ├── inventory.go          # Inventory sync
│   │       ├── workflow.go           # Workflow orchestration
│   │       └── models.go             # SMO data models
│   │
│   └── plugins/
│       ├── ims/
│       │   ├── kubernetes/
│       │   │   ├── plugin.go
│       │   │   ├── resourcepools.go
│       │   │   ├── resources.go
│       │   │   ├── resourcetypes.go
│       │   │   └── client.go
│       │   │
│       │   ├── mock/
│       │   │   ├── plugin.go
│       │   │   └── storage.go
│       │   │
│       │   ├── openstack/
│       │   │   ├── plugin.go
│       │   │   ├── resourcepools.go
│       │   │   ├── resources.go
│       │   │   └── client.go
│       │   │
│       │   ├── dtias/
│       │   │   ├── plugin.go
│       │   │   ├── resourcepools.go
│       │   │   └── client.go
│       │   │
│       │   ├── vsphere/
│       │   │   ├── plugin.go
│       │   │   └── client.go
│       │   │
│       │   ├── aws/
│       │   ├── azure/
│       │   ├── gke/
│       │   └── equinix/
│       │
│       ├── dms/
│       │   ├── helm/
│       │   │   ├── plugin.go
│       │   │   ├── lifecycle.go
│       │   │   └── client.go
│       │   │
│       │   ├── argocd/
│       │   │   ├── plugin.go
│       │   │   └── client.go
│       │   │
│       │   ├── flux/
│       │   ├── kustomize/
│       │   ├── onap-lcm/
│       │   ├── osm-lcm/
│       │   └── crossplane/
│       │
│       ├── smo/
│       │   ├── onap/
│       │   │   ├── plugin.go
│       │   │   ├── northbound.go      # A&AI, DMaaP integration
│       │   │   ├── dms-backend.go     # SO, SDNC integration
│       │   │   ├── workflow.go        # Camunda workflows
│       │   │   └── clients/
│       │   │       ├── aai.go
│       │   │       ├── dmaap.go
│       │   │       ├── so.go
│       │   │       └── sdnc.go
│       │   │
│       │   ├── osm/
│       │   │   ├── plugin.go
│       │   │   ├── northbound.go      # VIM sync
│       │   │   ├── dms-backend.go     # NS/VNF LCM
│       │   │   └── client.go
│       │   │
│       │   ├── custom/
│       │   ├── cloudify/
│       │   └── camunda/
│       │
│       └── observability/
│           ├── prometheus/
│           ├── jaeger/
│           └── loki/
│
├── config/
│   └── plugins.yaml                   # Plugin configuration
│
└── docs/
    └── backend-plugins.md             # This document
```

### Plugin Lifecycle

```go
// Plugin initialization sequence

func InitializePlugins(ctx context.Context, config *Config) (*plugin.Registry, error) {
    registry := plugin.NewRegistry()

    // 1. Load IMS plugins
    for _, pluginCfg := range config.Plugins.IMS {
        if !pluginCfg.Enabled {
            continue
        }

        plugin, err := loadIMSPlugin(pluginCfg)
        if err != nil {
            return nil, fmt.Errorf("failed to load IMS plugin %s: %w", pluginCfg.Name, err)
        }

        if err := plugin.Initialize(ctx, pluginCfg.Config); err != nil {
            return nil, fmt.Errorf("failed to initialize IMS plugin %s: %w", pluginCfg.Name, err)
        }

        registry.RegisterIMS(pluginCfg.Name, plugin, pluginCfg.Default)
    }

    // 2. Load DMS plugins
    for _, pluginCfg := range config.Plugins.DMS {
        if !pluginCfg.Enabled {
            continue
        }

        plugin, err := loadDMSPlugin(pluginCfg)
        if err != nil {
            return nil, fmt.Errorf("failed to load DMS plugin %s: %w", pluginCfg.Name, err)
        }

        if err := plugin.Initialize(ctx, pluginCfg.Config); err != nil {
            return nil, fmt.Errorf("failed to initialize DMS plugin %s: %w", pluginCfg.Name, err)
        }

        registry.RegisterDMS(pluginCfg.Name, plugin, pluginCfg.Default)
    }

    // 3. Load SMO plugins
    for _, pluginCfg := range config.Plugins.SMO {
        if !pluginCfg.Enabled {
            continue
        }

        plugin, err := loadSMOPlugin(pluginCfg)
        if err != nil {
            return nil, fmt.Errorf("failed to load SMO plugin %s: %w", pluginCfg.Name, err)
        }

        if err := plugin.Initialize(ctx, pluginCfg.Config); err != nil {
            return nil, fmt.Errorf("failed to initialize SMO plugin %s: %w", pluginCfg.Name, err)
        }

        registry.RegisterSMO(pluginCfg.Name, plugin, pluginCfg.Default)
    }

    // 4. Load routing rules
    if err := registry.LoadRoutingRules(config.Routing); err != nil {
        return nil, fmt.Errorf("failed to load routing rules: %w", err)
    }

    return registry, nil
}
```

---

## Summary

This comprehensive backend plugin architecture enables netweave to:

✅ **O2-IMS Multi-Backend Support**: 10+ infrastructure backends (Kubernetes, OpenStack, VMware, AWS, Azure, etc.)
✅ **O2-DMS Deployment Management**: 7+ deployment engines (Helm, ArgoCD, Flux, ONAP-LCM, OSM-LCM)
✅ **O2-SMO Integration**: Dual-mode integration with ONAP, OSM, and custom SMO frameworks
✅ **Unified Plugin Architecture**: Consistent interface across all plugin categories
✅ **Configuration-Driven Routing**: Intelligent request routing based on rules
✅ **Extensible Design**: Easy to add new backends without modifying core code
✅ **Production-Grade**: Enterprise-ready plugin lifecycle management

**All specifications are complete and ready for implementation.**

---

**Next Steps**: Implementation phase can begin with any plugin category based on priorities.
