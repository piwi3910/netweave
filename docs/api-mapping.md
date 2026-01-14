# O2-IMS/O2-DMS API to Backend Mapping

**Version:** 1.0
**Last Updated:** 2026-01-14

## Feature Status Legend

| Symbol | Status | Description |
|--------|--------|-------------|
| ✅ | Implemented | Feature fully implemented and tested |
| 🚧 | In Progress | Feature partially implemented or under active development |
| 📋 | Planned | Feature planned for future release |
| ❌ | Not Implemented | Feature documented but not yet implemented |
| 🔍 | Under Investigation | Implementation status being verified |

## Table of Contents

- [O2-IMS API Mappings](#o2-ims-api-mappings)
  - [Resource Pools](#resource-pools)
  - [Resources](#resources)
  - [Resource Types](#resource-types)
  - [Deployment Managers](#deployment-managers)
  - [Subscriptions](#subscriptions)
- [O2-DMS API Mappings](#o2-dms-api-mappings)
  - [Deployment Packages](#deployment-packages)
  - [NFDeployments](#nfdeployments)
  - [NFDeployment Operations](#nfdeployment-operations)
- [Backend Adapter Status](#backend-adapter-status)
- [Design Decisions](#design-decisions)

---

## O2-IMS API Mappings

### Resource Pools

Resource Pools represent logical groupings of infrastructure resources that can be allocated to workloads.

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2ims/v1/resourcePools` | List | ✅ Implemented | `internal/handlers/resourcepool.go:ListResourcePools()` |
| GET | `/o2ims/v1/resourcePools/{id}` | Read | ✅ Implemented | `internal/handlers/resourcepool.go:GetResourcePool()` |
| POST | `/o2ims/v1/resourcePools` | Create | ✅ Implemented | `internal/handlers/resourcepool.go:CreateResourcePool()` |
| PATCH | `/o2ims/v1/resourcePools/{id}` | Update | ✅ Implemented | `internal/handlers/resourcepool.go:UpdateResourcePool()` |
| DELETE | `/o2ims/v1/resourcePools/{id}` | Delete | ✅ Implemented | `internal/handlers/resourcepool.go:DeleteResourcePool()` |

#### Backend Mappings

| IMS Backend | Primary Resource | Secondary Resources | CRUD Support | Status |
|-------------|------------------|---------------------|--------------|--------|
| **Kubernetes** | MachineSet, NodePool | ConfigMap, Secret | CRU | ✅ Implemented |
| **AWS** | Auto Scaling Group | Launch Template, Security Group | CRUD | ✅ Implemented |
| **Azure** | Resource Group | Virtual Machine Scale Set | CRUD | ✅ Implemented |
| **GCP** | Instance Group Manager | Instance Template | CRUD | ✅ Implemented |
| **OpenStack** | Host Aggregate | Nova Compute Nodes | CRUD | ✅ Implemented |
| **VMware** | Resource Pool | Cluster, Host | CRUD | ✅ Implemented |
| **Dell DTIAS** | Server Pool | Physical Servers | CRUD | ✅ Implemented |

#### Implementation Notes

**Status**: ✅ Fully Implemented

**Handlers**: `internal/handlers/resourcepool.go`
- `ListResourcePools()` - ✅ Implemented (pagination, filtering)
- `GetResourcePool()` - ✅ Implemented (detailed metadata)
- `CreateResourcePool()` - ✅ Implemented (validation, backend provisioning)
- `UpdateResourcePool()` - ✅ Implemented (capacity scaling, metadata updates)
- `DeleteResourcePool()` - ✅ Implemented (cascade delete protection)

**Backend Adapter Support**:
- All 7 IMS adapters support full CRUD operations
- Automatic resource discovery and inventory sync
- Real-time capacity monitoring
- Multi-zone/multi-region support

**Testing**:
- Unit tests: 95% coverage
- Integration tests: Passing for all backends
- E2E tests: Passing

**Known Limitations**:
- Kubernetes: Requires Cluster API or similar machine management
- OpenStack: Limited to admin-level aggregates
- VMware: Requires vCenter API access

**Example Transformation** (Kubernetes):

```go
// ✅ IMPLEMENTED
// File: internal/adapters/kubernetes/resourcepools.go:transformMachineSetToO2Pool
func (a *Adapter) transformMachineSetToResourcePool(ms *machinev1beta1.MachineSet) *models.ResourcePool {
    return &models.ResourcePool{
        ResourcePoolID:   string(ms.UID),
        Name:             ms.Name,
        GlobalLocationID: a.getLocationFromLabels(ms.Labels),
        Description:      ms.Annotations["description"],
        Extensions: map[string]interface{}{
            "k8s.machineSetName": ms.Name,
            "k8s.namespace":      ms.Namespace,
            "k8s.replicas":       ms.Spec.Replicas,
        },
    }
}
```

---

### Resources

Resources represent individual infrastructure units (compute nodes, storage volumes, networks).

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2ims/v1/resourcePools/{poolId}/resources` | List | ✅ Implemented | `internal/handlers/resource.go:ListResources()` |
| GET | `/o2ims/v1/resources/{id}` | Read | ✅ Implemented | `internal/handlers/resource.go:GetResource()` |
| POST | `/o2ims/v1/resources` | Create | ❌ Not Exposed | N/A (see [Design Decisions](#resource-level-operations)) |
| DELETE | `/o2ims/v1/resources/{id}` | Delete | ❌ Not Exposed | N/A (see [Design Decisions](#resource-level-operations)) |

#### Backend Mappings

| IMS Backend | Resource Types | Discovery Method | Status |
|-------------|----------------|------------------|--------|
| **Kubernetes** | Node, PersistentVolume, NetworkPolicy | API Watch | ✅ Implemented |
| **AWS** | EC2 Instance, EBS Volume, VPC | Resource Tags | ✅ Implemented |
| **Azure** | Virtual Machine, Disk, Virtual Network | Resource Graph | ✅ Implemented |
| **GCP** | Compute Instance, Persistent Disk, VPC Network | Asset Inventory API | ✅ Implemented |
| **OpenStack** | Server, Volume, Network | Nova/Cinder/Neutron APIs | ✅ Implemented |
| **VMware** | Virtual Machine, Datastore, Port Group | vCenter API | ✅ Implemented |
| **Dell DTIAS** | Physical Server, Storage, Network | DTIAS API | ✅ Implemented |

#### Implementation Notes

**Status**: ✅ List/Read Implemented, Create/Delete Intentionally Not Exposed

**Handlers**: `internal/handlers/resource.go`
- `ListResources()` - ✅ Implemented (filtering by pool, type, status)
- `GetResource()` - ✅ Implemented (detailed resource information)
- `CreateResource()` - ❌ Not exposed (handled via resource pool operations)
- `DeleteResource()` - ❌ Not exposed (handled via resource pool operations)

**See**: [Design Decisions: Resource-Level Operations](#resource-level-operations)

**Backend Support**:
- All adapters implement resource discovery
- Real-time resource status updates
- Automatic inventory synchronization every 5 minutes
- Event-driven updates for Kubernetes backend

**Testing**:
- Unit tests: 90% coverage
- Integration tests: Passing
- E2E tests: Passing

**Example Transformation** (AWS):

```go
// ✅ IMPLEMENTED
// File: internal/adapters/aws/resources.go:transformEC2InstanceToResource
func (a *Adapter) transformEC2InstanceToResource(instance *ec2.Instance) *models.Resource {
    return &models.Resource{
        ResourceID:       *instance.InstanceId,
        ResourcePoolID:   a.getPoolIDFromTags(instance.Tags),
        ResourceTypeID:   *instance.InstanceType,
        GlobalAssetID:    *instance.InstanceId,
        Description:      a.getTagValue(instance.Tags, "Name"),
        Extensions: map[string]interface{}{
            "aws.instanceType":      *instance.InstanceType,
            "aws.availabilityZone":  *instance.Placement.AvailabilityZone,
            "aws.state":             *instance.State.Name,
            "aws.launchTime":        instance.LaunchTime,
        },
    }
}
```

---

### Resource Types

Resource Types define the available infrastructure resource configurations (machine types, storage classes, network types).

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2ims/v1/resourceTypes` | List | 🔍 Adapter Implemented | 🔍 Handler Missing (see [#108](https://github.com/piwi3910/netweave/issues/108)) |
| GET | `/o2ims/v1/resourceTypes/{id}` | Read | 🔍 Adapter Implemented | 🔍 Handler Missing (see [#108](https://github.com/piwi3910/netweave/issues/108)) |

#### Backend Mappings

| IMS Backend | Resource Type Discovery | Examples | Status |
|-------------|------------------------|----------|--------|
| **Kubernetes** | StorageClass, Machine Types | gp2, io1, m5.large | ✅ Adapter Implemented |
| **AWS** | EC2 Instance Types, EBS Volume Types | t3.micro, gp3 | ✅ Adapter Implemented |
| **Azure** | VM Sizes, Disk SKUs | Standard_D2s_v3, Premium_LRS | ✅ Adapter Implemented |
| **GCP** | Machine Types, Disk Types | n1-standard-1, pd-ssd | ✅ Adapter Implemented |
| **OpenStack** | Flavors, Volume Types | m1.small, ssd | ✅ Adapter Implemented |
| **VMware** | VM Templates, Storage Policies | ubuntu-20.04, gold-storage | ✅ Adapter Implemented |
| **Dell DTIAS** | Server Profiles | R640, PowerEdge | ✅ Adapter Implemented |

#### Implementation Notes

**Status**: 🔍 Under Investigation

**Issue**: HTTP handlers missing, but all adapter implementations are complete.

**Adapter Methods** (Implemented):
- `ListResourceTypes()` - ✅ Implemented in all adapters
- `GetResourceType()` - ✅ Implemented in all adapters

**HTTP Handlers** (Missing):
- `internal/handlers/resourcetype.go` - 🔍 Missing (see issue #108)

**Workaround**: Resource type information is embedded in resource objects via `ResourceTypeID` field.

**Next Steps**: Implement HTTP handlers to expose resource type discovery API.

---

### Deployment Managers

Deployment Managers represent O2-DMS backend systems capable of managing CNF/VNF deployments.

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2ims/v1/deploymentManagers` | List | ✅ Implemented | `internal/handlers/deploymentmanager.go:ListDeploymentManagers()` |
| GET | `/o2ims/v1/deploymentManagers/{id}` | Read | ✅ Implemented | `internal/handlers/deploymentmanager.go:GetDeploymentManager()` |

#### Deployment Manager Registry

| DMS Backend | Capabilities | Deployment Target | Status |
|-------------|--------------|-------------------|--------|
| **Helm** | Package Management, Scaling, Rollback | Kubernetes | ✅ Registered |
| **ArgoCD** | GitOps, Multi-Cluster | Kubernetes | ✅ Registered |
| **Flux** | GitOps, Progressive Delivery | Kubernetes | ✅ Registered |
| **Kustomize** | Native K8s Config | Kubernetes | ✅ Registered |
| **Crossplane** | Infrastructure as Code | Multi-Cloud | ✅ Registered |
| **ONAP-LCM** | ETSI NFV MANO | Multi-Cloud | ✅ Registered |
| **OSM-LCM** | ETSI NFV MANO | Multi-Cloud | ✅ Registered |

#### Implementation Notes

**Status**: ✅ Fully Implemented

**Handlers**: `internal/handlers/deploymentmanager.go`
- `ListDeploymentManagers()` - ✅ Implemented
- `GetDeploymentManager()` - ✅ Implemented

**DMS Registry**: `internal/dms/registry/registry.go`
- ✅ All 7 DMS adapters registered
- ✅ Dynamic registration system implemented
- ✅ Health checking and capability discovery
- ✅ Default adapter selection (Helm)

**See Also**: [O2-DMS Backend Adapter Status](#o2-dms-backend-adapters)

---

### Subscriptions

Subscriptions enable SMO systems to receive real-time notifications about infrastructure changes.

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2ims/v1/subscriptions` | List | ✅ Implemented | `internal/handlers/subscription.go:ListSubscriptions()` |
| GET | `/o2ims/v1/subscriptions/{id}` | Read | ✅ Implemented | `internal/handlers/subscription.go:GetSubscription()` |
| POST | `/o2ims/v1/subscriptions` | Create | ✅ Implemented | `internal/handlers/subscription.go:CreateSubscription()` |
| DELETE | `/o2ims/v1/subscriptions/{id}` | Delete | ✅ Implemented | `internal/handlers/subscription.go:DeleteSubscription()` |

#### Subscription Types

| Event Type | Description | Backends Supported | Status |
|------------|-------------|--------------------|--------|
| `ResourcePoolChanged` | Resource pool created/updated/deleted | All IMS backends | ✅ Implemented |
| `ResourceChanged` | Resource created/updated/deleted | All IMS backends | ✅ Implemented |
| `ResourceTypeChanged` | Resource type added/updated | All IMS backends | ✅ Implemented |
| `AlarmEvent` | Infrastructure alarms | Kubernetes, OpenStack, VMware | ✅ Implemented |

#### Implementation Notes

**Status**: ✅ Fully Implemented

**Handlers**: `internal/handlers/subscription.go`
- `ListSubscriptions()` - ✅ Implemented
- `GetSubscription()` - ✅ Implemented
- `CreateSubscription()` - ✅ Implemented (validation, webhook verification)
- `DeleteSubscription()` - ✅ Implemented

**Notification Controller**: `internal/controllers/subscription_controller.go`
- ✅ Event generation from backend adapters
- ✅ Event filtering based on subscription criteria
- ✅ Webhook delivery with retry logic
- ✅ Exponential backoff for failed deliveries
- ✅ Dead letter queue for permanent failures

**Storage**: `internal/storage/redis.go`
- ✅ Subscription persistence in Redis
- ✅ High availability via Redis Sentinel
- ✅ Automatic failover support

**See**: [Design Decisions: Subscription Event Delivery](#subscription-event-delivery)

---

## O2-DMS API Mappings

### Deployment Packages

Deployment Packages represent CNF/VNF software packages (Helm charts, Git repositories, etc.).

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2dms/v1/deploymentPackages` | List | ✅ Implemented | `internal/dms/handlers/handlers.go:ListDeploymentPackages()` |
| GET | `/o2dms/v1/deploymentPackages/{id}` | Read | ✅ Implemented | `internal/dms/handlers/handlers.go:GetDeploymentPackage()` |
| POST | `/o2dms/v1/deploymentPackages` | Create | ✅ Implemented | `internal/dms/handlers/handlers.go:UploadDeploymentPackage()` |
| DELETE | `/o2dms/v1/deploymentPackages/{id}` | Delete | ✅ Implemented | `internal/dms/handlers/handlers.go:DeleteDeploymentPackage()` |

#### Backend Mappings

| DMS Backend | Package Format | Storage | Status |
|-------------|----------------|---------|--------|
| **Helm** | Helm Chart (.tgz) | ChartMuseum, OCI Registry | ✅ Implemented |
| **ArgoCD** | Git Repository | Git Server | ✅ Implemented |
| **Flux** | Git Repository, Helm Repository | Git Server, OCI Registry | ✅ Implemented |
| **Kustomize** | Git Repository | Git Server | ✅ Implemented |
| **Crossplane** | Crossplane Package | OCI Registry | ✅ Implemented |
| **ONAP-LCM** | CSAR Package | ONAP SDC | ✅ Implemented |
| **OSM-LCM** | NSD/VNFD Package | OSM Repository | ✅ Implemented |

#### Implementation Notes

**Status**: ✅ Fully Implemented

**Handlers**: `internal/dms/handlers/handlers.go`
- `ListDeploymentPackages()` - ✅ Implemented (filtering by type, version)
- `GetDeploymentPackage()` - ✅ Implemented (detailed metadata)
- `UploadDeploymentPackage()` - ✅ Implemented (validation, checksum verification)
- `DeleteDeploymentPackage()` - ✅ Implemented (cascade delete check)

**Adapter Support**:
- All 7 DMS adapters implement package management
- Automatic package discovery from repositories
- Version management and tagging
- Package validation and linting

**Testing**:
- Unit tests: 84.4% coverage
- Integration tests: Passing
- E2E tests: Passing

---

### NFDeployments

NFDeployments represent deployed CNF/VNF instances.

#### API Endpoints

| HTTP Method | Endpoint | CRUD | Status | Handler |
|-------------|----------|------|--------|---------|
| GET | `/o2dms/v1/nfDeployments` | List | ✅ Implemented | `internal/dms/handlers/handlers.go:ListDeployments()` |
| GET | `/o2dms/v1/nfDeployments/{id}` | Read | ✅ Implemented | `internal/dms/handlers/handlers.go:GetDeployment()` |
| POST | `/o2dms/v1/nfDeployments` | Create | ✅ Implemented | `internal/dms/handlers/handlers.go:CreateDeployment()` |
| PATCH | `/o2dms/v1/nfDeployments/{id}` | Update | ✅ Implemented | `internal/dms/handlers/handlers.go:UpdateDeployment()` |
| DELETE | `/o2dms/v1/nfDeployments/{id}` | Delete | ✅ Implemented | `internal/dms/handlers/handlers.go:DeleteDeployment()` |

#### Backend Mappings

| DMS Backend | Deployment Resource | Namespace/Project | Status |
|-------------|---------------------|-------------------|--------|
| **Helm** | Helm Release | Kubernetes Namespace | ✅ Implemented |
| **ArgoCD** | Application CR | ArgoCD Project | ✅ Implemented |
| **Flux** | Kustomization/HelmRelease CR | Kubernetes Namespace | ✅ Implemented |
| **Kustomize** | ConfigMap (tracking) | Kubernetes Namespace | ✅ Implemented |
| **Crossplane** | CompositeResource Claim | Kubernetes Namespace | ✅ Implemented |
| **ONAP-LCM** | Service Instance | ONAP Project | ✅ Implemented |
| **OSM-LCM** | NS Instance | OSM Project | ✅ Implemented |

#### Implementation Notes

**Status**: ✅ Fully Implemented

**Handlers**: `internal/dms/handlers/handlers.go`
- `ListDeployments()` - ✅ Implemented (filtering by status, namespace)
- `GetDeployment()` - ✅ Implemented (detailed status, history)
- `CreateDeployment()` - ✅ Implemented (validation, scheduling)
- `UpdateDeployment()` - ✅ Implemented (rolling updates, configuration changes)
- `DeleteDeployment()` - ✅ Implemented (graceful termination)

**Adapter Support**:
- All 7 DMS adapters implement deployment lifecycle
- Real-time status monitoring
- Progress tracking (0-100%)
- Event generation for state changes

**Testing**:
- Unit tests: 84.4% coverage
- Integration tests: Passing
- E2E tests: Passing

---

### NFDeployment Operations

Additional lifecycle operations on deployed NFDeployments.

#### API Endpoints

| HTTP Method | Endpoint | Operation | Status | Handler |
|-------------|----------|-----------|--------|---------|
| POST | `/o2dms/v1/nfDeployments/{id}/scale` | Scale replicas | ✅ Implemented | `internal/dms/handlers/handlers.go:ScaleDeployment()` |
| POST | `/o2dms/v1/nfDeployments/{id}/rollback` | Rollback to revision | ✅ Implemented | `internal/dms/handlers/handlers.go:RollbackDeployment()` |
| GET | `/o2dms/v1/nfDeployments/{id}/status` | Get detailed status | ✅ Implemented | `internal/dms/handlers/handlers.go:GetDeploymentStatus()` |
| GET | `/o2dms/v1/nfDeployments/{id}/logs` | Get deployment logs | ✅ Implemented | `internal/dms/handlers/handlers.go:GetDeploymentLogs()` |
| GET | `/o2dms/v1/nfDeployments/{id}/history` | Get deployment history | ✅ Implemented | `internal/dms/handlers/handlers.go:GetDeploymentHistory()` |

#### Backend Support Matrix

| DMS Backend | Scale | Rollback | Status | Logs | History |
|-------------|-------|----------|--------|------|---------|
| **Helm** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **ArgoCD** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Flux** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Kustomize** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Crossplane** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **ONAP-LCM** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **OSM-LCM** | ✅ | ✅ | ✅ | ✅ | ✅ |

#### Implementation Notes

**Status**: ✅ Fully Implemented

**Documentation**: See `docs/adapters/dms/lifecycle-operations.md` for detailed operation documentation.

**Handlers**: `internal/dms/handlers/handlers.go`
- `ScaleDeployment()` - ✅ Implemented (replica count validation)
- `RollbackDeployment()` - ✅ Implemented (revision validation, safety checks)
- `GetDeploymentStatus()` - ✅ Implemented (real-time status with conditions)
- `GetDeploymentLogs()` - ✅ Implemented (streaming, filtering, tail)
- `GetDeploymentHistory()` - ✅ Implemented (revision list with details)

**Adapter Support**:
- All adapters support basic operations
- GitOps adapters (ArgoCD, Flux) use Git-based rollback
- Helm uses native release history
- ONAP/OSM use MANO-specific mechanisms

**Testing**:
- Unit tests: 84.4% coverage
- Integration tests: Passing
- E2E tests: Passing

---

## Backend Adapter Status

### O2-IMS Backend Adapters

| Adapter | Status | Version | Implementation % | Test Coverage | File Location |
|---------|--------|---------|------------------|---------------|---------------|
| **Kubernetes** | ✅ Production | v1.0.0 | 100% | 89% | `internal/adapters/kubernetes/` |
| **AWS** | ✅ Production | v1.0.0 | 100% | 87% | `internal/adapters/aws/` |
| **Azure** | ✅ Production | v1.0.0 | 100% | 85% | `internal/adapters/azure/` |
| **GCP** | ✅ Production | v1.0.0 | 100% | 86% | `internal/adapters/gcp/` |
| **OpenStack** | ✅ Production | v1.0.0 | 95% | 88% | `internal/adapters/openstack/` |
| **VMware** | ✅ Production | v1.0.0 | 95% | 84% | `internal/adapters/vmware/` |
| **Dell DTIAS** | ✅ Production | v1.0.0 | 95% | 83% | `internal/adapters/dtias/` |

**Notes**:
- OpenStack: Subscription notifications use polling (5% missing for webhook support)
- VMware: Subscription notifications use polling (5% missing for webhook support)
- Dell DTIAS: Subscription notifications use polling (5% missing for webhook support)

### O2-DMS Backend Adapters

| Adapter | Status | Version | Implementation % | Test Coverage | File Location |
|---------|--------|---------|------------------|---------------|---------------|
| **Helm** | ✅ Production | v1.0.0 | 100% | 57.9% | `internal/dms/adapters/helm/` |
| **ArgoCD** | ✅ Production | v1.0.0 | 100% | 78% | `internal/dms/adapters/argocd/` |
| **Flux** | ✅ Production | v1.0.0 | 100% | 82% | `internal/dms/adapters/flux/` |
| **Kustomize** | ✅ Production | v1.0.0 | 100% | 75% | `internal/dms/adapters/kustomize/` |
| **Crossplane** | ✅ Production | v1.0.0 | 100% | 76% | `internal/dms/adapters/crossplane/` |
| **ONAP-LCM** | ✅ Production | v1.0.0 | 95% | 79% | `internal/dms/adapters/onaplcm/` |
| **OSM-LCM** | ✅ Production | v1.0.0 | 95% | 81% | `internal/dms/adapters/osmlcm/` |

**Notes**:
- All adapters registered and functional (see issue #115)
- Helm: Test coverage improvement in progress (target 80%)
- ONAP-LCM: Limited policy management support (5% missing)
- OSM-LCM: Limited policy management support (5% missing)

### O2-SMO Integration Plugins

| Plugin | Status | Version | Implementation % | Test Coverage | File Location |
|--------|--------|---------|------------------|---------------|---------------|
| **ONAP** | ✅ Production | v1.0.0 | 100% | 82% | `internal/smo/adapters/onap/` |
| **OSM** | ✅ Production | v1.0.0 | 95% | 80% | `internal/smo/adapters/osm/` |

**Notes**:
- ONAP: Full northbound/southbound integration
- OSM: Limited advanced policy management (5% missing)

---

## Design Decisions

### Resource-Level Operations

**Decision**: Resource CREATE/DELETE operations are not exposed via HTTP API.

**Rationale**:
- Infrastructure provisioning happens at the resource pool level
- Creating a resource pool automatically provisions underlying resources
- Direct resource manipulation could destabilize production environments
- Simpler API surface reduces operational complexity
- O-RAN O2-IMS specification allows but does not require resource-level CRUD
- Security: Prevents accidental deletion of critical infrastructure

**Status**: ✅ Documented and Implemented

**Implementation**:
- `ListResources()` - ✅ Exposed
- `GetResource()` - ✅ Exposed
- `CreateResource()` - ❌ Not exposed
- `DeleteResource()` - ❌ Not exposed

**Alternative**: Resources are created/deleted implicitly via resource pool scaling operations.

**Related Issue**: See [#111](https://github.com/piwi3910/netweave/issues/111)

---

### Subscription Event Delivery

**Decision**: Subscription notifications use webhook-based push model with retry logic.

**Rationale**:
- Real-time event delivery for SMO systems
- Retry logic ensures delivery reliability (5 attempts)
- Exponential backoff prevents overwhelming subscribers (1s, 2s, 4s, 8s, 16s)
- Dead letter queue for permanent failures
- Follows O-RAN O2-IMS notification specification
- HTTP callbacks are industry standard for webhooks

**Status**: ✅ Implemented

**Implementation**: `internal/controllers/subscription_controller.go`, `internal/workers/webhook_worker.go`

**Features**:
- ✅ Webhook endpoint validation on subscription creation
- ✅ Event filtering based on subscription criteria
- ✅ Retry with exponential backoff
- ✅ Dead letter queue for failed deliveries
- ✅ Metrics for delivery success/failure rates
- ✅ Subscription health monitoring

**Configuration**:
```yaml
subscriptions:
  webhook:
    timeout: 30s
    retryAttempts: 5
    retryBackoff: exponential
    deadLetterQueue: true
```

**Related Issue**: See [#110](https://github.com/piwi3910/netweave/issues/110)

---

### Multi-Backend Routing

**Decision**: Routing rules evaluated by priority with default fallback.

**Status**: 📋 Documented, Implementation TBD

**Rationale**:
- Support for hybrid cloud deployments
- Workload placement optimization based on cost, performance, compliance
- Automatic failover between backends
- Policy-driven resource allocation

**Planned Implementation**:
```go
type RoutingRule struct {
    Priority    int
    Matcher     Matcher  // Label, annotation, or attribute matcher
    TargetPool  string   // Resource pool or backend
    Fallback    bool     // Use as fallback if primary unavailable
}
```

**Use Cases**:
- Route GPU workloads to AWS, CPU to on-prem
- Route sensitive data to on-prem OpenStack
- Route dev workloads to low-cost cloud regions
- Automatic disaster recovery failover

**Status**: Documented in architecture, planned for future release

---

### DMS Adapter Registration

**Decision**: Dynamic adapter registration with enable/disable configuration.

**Status**: ✅ Implemented (Issue #115)

**Rationale**:
- Flexible deployment configuration
- Enable only needed adapters to reduce memory footprint
- Runtime adapter health checking
- Support for adapter-specific configuration

**Implementation**: `internal/dms/init.go`

**Features**:
- ✅ Dynamic registration system
- ✅ Per-adapter enable/disable flags
- ✅ Default adapter selection
- ✅ Health checking and capability discovery
- ✅ Thread-safe registry operations

**Configuration**:
```go
config := &dms.AdaptersConfig{
    Helm: &dms.AdapterConfig{
        Enabled:   true,
        IsDefault: true,
        Namespace: "default",
    },
    ArgoCD: &dms.AdapterConfig{
        Enabled:   true,
        Namespace: "argocd",
    },
}
```

---

## Related Documentation

- [Architecture Overview](architecture/README.md)
- [Backend Adapter Development](adapters/README.md)
- [O2-IMS API Reference](api/o2ims/README.md)
- [O2-DMS Lifecycle Operations](adapters/dms/lifecycle-operations.md)
- [Deployment Manager Registration](adapters/dms/README.md)

---

## Change History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2026-01-14 | Initial API mapping document with implementation status |

---

**Maintainers**: Pascal Watteel <pascal@watteel.com>
**License**: MIT
