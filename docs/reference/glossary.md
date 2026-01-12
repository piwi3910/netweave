# Glossary

Comprehensive glossary of O-RAN, Kubernetes, and netweave-specific terms and acronyms.

## Table of Contents

- [O-RAN Terms](#o-ran-terms)
- [Kubernetes Terms](#kubernetes-terms)
- [netweave Terms](#netweave-terms)
- [Infrastructure Terms](#infrastructure-terms)
- [Networking Terms](#networking-terms)
- [Security Terms](#security-terms)
- [Acronyms](#acronyms)

---

## O-RAN Terms

### O2 Interface
The interface between the O-RAN Service Management and Orchestration (SMO) framework and the O-Cloud infrastructure. Consists of O2-IMS (Infrastructure Management), O2-DMS (Deployment Management), and O2-SMO (Orchestration) APIs.

### O2-IMS
**O2 Infrastructure Management Services** - RESTful API for managing cloud infrastructure resources, including resource pools, nodes, and resource types. Part of the O-RAN O2 interface specification.

### O2-DMS
**O2 Deployment Management Services** - RESTful API for managing deployment lifecycle of Cloud Native Functions (CNFs) and Virtual Network Functions (VNFs). Handles package management, deployment orchestration, and lifecycle operations.

### O2-SMO  
**O2 Service Management & Orchestration** - Integration API enabling SMO systems to orchestrate services across O-Cloud infrastructure. Provides workflow execution, service modeling, and policy management.

### O-Cloud
The cloud infrastructure that hosts O-RAN workloads. Consists of physical infrastructure, virtualization layer, and cloud management platform. Managed via O2-IMS API.

### O-RAN
**Open Radio Access Network** - Industry alliance and architecture promoting open, interoperable, intelligent RAN networks. Defines interfaces, specifications, and reference architectures for disaggregated 5G RAN.

### O-DU
**O-RAN Distributed Unit** - Lower-layer RAN protocol stack component handling real-time L1/L2 processing. Deployed as a workload on O-Cloud infrastructure.

### O-CU
**O-RAN Centralized Unit** - Upper-layer RAN protocol stack component handling non-real-time L2/L3 processing. May be further split into O-CU-CP (Control Plane) and O-CU-UP (User Plane).

### O-RU
**O-RAN Radio Unit** - Radio frequency component providing radio transmission/reception capabilities. Connects to O-DU via fronthaul interface.

### SMO
**Service Management and Orchestration** - Framework managing the lifecycle of network services across O-RAN components. Interfaces with O-Cloud via O2 APIs for infrastructure and deployment management.

### Resource Pool
Logical grouping of infrastructure resources with similar characteristics. Corresponds to Kubernetes NodePools, OpenStack host aggregates, or cloud provider instance groups.

### Resource Type
Classification of infrastructure resource capabilities (e.g., compute-node, storage-node, accelerator-node). Defines available resource characteristics and features.

### Deployment Manager
Entity responsible for managing deployment lifecycle operations. In netweave context, represents a Kubernetes cluster, Helm installation, or orchestration system.

---

## Kubernetes Terms

### Adapter
Software component translating O2-IMS API calls to backend-specific operations. Implements standardized interface for different infrastructure providers (Kubernetes, OpenStack, AWS, etc.).

### Custom Resource (CR)
Extension of Kubernetes API representing custom objects. Used extensively in Kubernetes Operator pattern. Example: O2IMSGateway CR for gateway configuration.

### Custom Resource Definition (CRD)
Schema defining structure and validation rules for Custom Resources. Extends Kubernetes API without modifying core codebase.

### DaemonSet
Kubernetes workload ensuring a pod runs on all (or subset of) nodes. Used for node-level services like monitoring agents, log collectors, or network plugins.

### Deployment
Kubernetes workload providing declarative updates for Pods and ReplicaSets. Manages rolling updates, rollbacks, and scaling.

### Event
Kubernetes object recording significant occurrences in the cluster. netweave watches events to detect infrastructure changes for subscription notifications.

### Helm
Package manager for Kubernetes. Packages applications as charts containing all resource definitions, configurations, and dependencies.

### Informer
Kubernetes client-go component providing efficient, cached access to Kubernetes resources with change notifications. Used extensively in netweave controllers.

### Kubeconfig
Configuration file containing cluster connection details, authentication credentials, and context definitions for kubectl and client applications.

### Label
Key-value pair attached to Kubernetes objects for identification and selection. Used extensively for resource filtering and organization.

### Leader Election
Pattern ensuring only one instance of a controller actively reconciles resources. Prevents concurrent modifications and split-brain scenarios in HA deployments.

### MachineSet (OpenShift)
OpenShift abstraction for managing groups of Machines. Analogous to Kubernetes NodePools, corresponds to O2-IMS Resource Pools.

### Namespace
Kubernetes mechanism for isolating groups of resources within a single cluster. Provides scope for names and allows resource quotas and RBAC policies.

### Node
Physical or virtual machine in Kubernetes cluster. Runs kubelet, container runtime, and hosts Pods. Corresponds to O2-IMS Resource.

### NodePool
Logical grouping of Kubernetes nodes with similar configuration. Corresponds to O2-IMS Resource Pool concept.

### Operator
Kubernetes extension using Custom Resources and controllers to manage complex applications. netweave includes O2IMS Gateway Operator for lifecycle management.

### Pod
Smallest deployable unit in Kubernetes. Contains one or more containers sharing network and storage resources.

### ReplicaSet
Maintains stable set of replica Pods running at any given time. Usually managed by Deployment.

### Service
Kubernetes abstraction exposing an application running on a set of Pods as a network service. Provides stable endpoint and load balancing.

### StatefulSet
Kubernetes workload managing stateful applications requiring stable network identities and persistent storage.

---

## netweave Terms

### Gateway
Core netweave component exposing O2-IMS, O2-DMS, and O2-SMO APIs. Stateless, horizontally scalable service routing requests to backend adapters.

### Subscription
O2-IMS concept enabling event notifications. SMO systems subscribe to infrastructure changes; netweave delivers webhook notifications when matching events occur.

### Webhook
HTTP callback delivering event notifications to subscribers. netweave sends POST requests to subscriber-provided callback URLs with event payloads.

### Event Controller
netweave component watching Kubernetes resources for changes and delivering notifications to subscribers. Runs continuously, processing events in real-time.

### Tenant
Isolated entity in multi-tenant deployments. Each tenant has separate resources, quotas, and access controls. Enables multiple SMO systems on single gateway instance.

### Compliance Checker
Tool validating netweave implementation against O-RAN specifications. Generates compliance reports and badges showing API coverage.

### Adapter Interface
Go interface defining required methods for backend adapters. Ensures consistent behavior across different infrastructure providers.

---

## Infrastructure Terms

### Bare Metal
Physical servers without virtualization layer. Managed via IPMI, Redfish, or vendor-specific APIs. Dell DTIAS adapter provides O2-IMS access to bare-metal infrastructure.

### CNF
**Cloud Native Function** - Network function designed for cloud-native environments. Deployed as containers/pods, scales horizontally, follows 12-factor app principles.

### VNF
**Virtual Network Function** - Network function deployed as virtual machines. Traditional approach to network function virtualization.

### IaaS
**Infrastructure as a Service** - Cloud computing model providing virtualized computing resources. Examples: OpenStack, AWS EC2, Azure VMs.

### Fronthaul
Network connection between O-RU (Radio Unit) and O-DU (Distributed Unit). Typically eCPRI or raw Ethernet, requires low latency and high bandwidth.

### Midhaul
Network connection between O-DU and O-CU. Less stringent latency requirements than fronthaul.

### Backhaul
Network connection between RAN and core network. Traditional term, less relevant in O-RAN disaggregated architecture.

---

## Networking Terms

### mTLS
**Mutual TLS** - Authentication mechanism where both client and server verify each other's identity using certificates. Required for production netweave deployments.

### Ingress
Kubernetes resource managing external access to services. Provides HTTP/HTTPS routing, load balancing, and SSL termination.

### Service Mesh
Infrastructure layer handling service-to-service communication. Provides traffic management, security, and observability. Examples: Istio, Linkerd.

### Load Balancer
Distributes network traffic across multiple servers. Kubernetes LoadBalancer service type provisions cloud provider load balancers.

---

## Security Terms

### RBAC
**Role-Based Access Control** - Authorization mechanism restricting system access based on user roles. netweave implements both Kubernetes RBAC and application-level RBAC.

### TLS
**Transport Layer Security** - Cryptographic protocol securing network communications. Successor to SSL, provides encryption, authentication, and integrity.

### Certificate Authority (CA)
Entity issuing digital certificates. cert-manager in Kubernetes can act as CA for internal certificates.

### GPG Signing
Using GPG keys to cryptographically sign git commits. Verifies commit author identity and prevents tampering.

### Secret
Kubernetes object storing sensitive information like passwords, tokens, certificates. Encrypted at rest in etcd.

---

## Acronyms

| Acronym | Expansion | Definition |
|---------|-----------|------------|
| **AAI** | Active and Available Inventory | ONAP component managing network inventory |
| **ADR** | Architecture Decision Record | Document capturing important architectural decisions |
| **API** | Application Programming Interface | Contract for software communication |
| **BBU** | Baseband Unit | Traditional RAN component (replaced by O-DU/O-CU in O-RAN) |
| **CA** | Certificate Authority | Issues digital certificates |
| **CD** | Continuous Delivery/Deployment | Automated software release process |
| **CI** | Continuous Integration | Automated build and test process |
| **CNF** | Cloud Native Function | Network function designed for cloud-native environments |
| **CNCF** | Cloud Native Computing Foundation | Open source foundation for cloud-native projects |
| **CR** | Custom Resource | Kubernetes API extension |
| **CRD** | Custom Resource Definition | Schema for Custom Resources |
| **CRUD** | Create, Read, Update, Delete | Basic data operations |
| **DMS** | Deployment Management Services | O2 API for deployment lifecycle |
| **DMaaP** | Data Movement as a Platform | ONAP message routing component |
| **DU** | Distributed Unit | Lower-layer RAN processing (O-DU in O-RAN) |
| **CU** | Centralized Unit | Upper-layer RAN processing (O-CU in O-RAN) |
| **eCPRI** | Enhanced Common Public Radio Interface | Fronthaul protocol |
| **E2E** | End-to-End | Complete system workflow testing |
| **etcd** | Distributed key-value store | Kubernetes backing store |
| **HA** | High Availability | System design for minimal downtime |
| **HTTP** | HyperText Transfer Protocol | Web communication protocol |
| **HTTPS** | HTTP Secure | HTTP over TLS |
| **IaaS** | Infrastructure as a Service | Cloud computing model |
| **IMS** | Infrastructure Management Services | O2 API for infrastructure management |
| **IPMI** | Intelligent Platform Management Interface | Bare-metal management standard |
| **JSON** | JavaScript Object Notation | Data interchange format |
| **K8s** | Kubernetes | Short form (8 letters between K and s) |
| **LCM** | Lifecycle Management | Managing deployment lifecycle |
| **mTLS** | Mutual TLS | Two-way TLS authentication |
| **NBI** | North Bound Interface | API for external management systems |
| **NF** | Network Function | Software providing network capability |
| **NFV** | Network Function Virtualization | Virtualizing network functions |
| **NFVO** | NFV Orchestrator | Manages network service lifecycle |
| **O-Cloud** | Open Cloud | Cloud infrastructure for O-RAN |
| **ONAP** | Open Network Automation Platform | Linux Foundation network automation project |
| **OSM** | Open Source MANO | ETSI-hosted NFV management and orchestration |
| **PaaS** | Platform as a Service | Cloud computing model |
| **RAN** | Radio Access Network | Wireless network infrastructure |
| **RBAC** | Role-Based Access Control | Authorization mechanism |
| **REST** | Representational State Transfer | API architectural style |
| **RF** | Radio Frequency | Wireless transmission spectrum |
| **RRH** | Remote Radio Head | Traditional RAN component (replaced by O-RU) |
| **RU** | Radio Unit | Radio transmission component (O-RU in O-RAN) |
| **SBI** | South Bound Interface | API for managed resources |
| **SDNC** | Software Defined Network Controller | ONAP network control component |
| **SMO** | Service Management and Orchestration | O-RAN management framework |
| **SO** | Service Orchestrator | ONAP orchestration component |
| **TLS** | Transport Layer Security | Encryption protocol |
| **UUID** | Universally Unique Identifier | Unique ID standard |
| **VIM** | Virtual Infrastructure Manager | Manages virtualized infrastructure |
| **VM** | Virtual Machine | Software emulation of computer |
| **VNF** | Virtual Network Function | Network function as VM |
| **YAML** | YAML Ain't Markup Language | Human-readable data serialization |

---

## Related Documentation

- [O-RAN Compliance](compliance.md) - Spec compliance details
- [Error Codes](error-codes.md) - Error reference
- [Architecture](../architecture.md) - System architecture
- [API Documentation](../api/README.md) - API reference

---

**To suggest additions or corrections to this glossary, please [open an issue](https://github.com/piwi3910/netweave/issues/new).**
