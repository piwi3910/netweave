# O2-DMS and O2-SMO Integration

**Version:** 1.0
**Date:** 2026-01-06

This document describes how the netweave architecture extends to support **O2-DMS** (Deployment Management Services) and integrates with **O2-SMO** (Service Management and Orchestration).

## Table of Contents

1. [Overview](#overview)
2. [O2-DMS Extension](#o2-dms-extension)
3. [O2-SMO Integration](#o2-smo-integration)
4. [Complete Architecture](#complete-architecture)
5. [Use Cases](#use-cases)
6. [Implementation Guide](#implementation-guide)

---

## Overview

### The O-RAN Stack

```
┌─────────────────────────────────────────────────────────────┐
│                    O2-SMO Layer                             │
│  (Service Management & Orchestration)                       │
│  • End-to-end service lifecycle                            │
│  • FCAPS (Fault, Config, Accounting, Perf, Security)       │
│  • Cross-domain orchestration                              │
└─────────────┬───────────────────────┬───────────────────────┘
              │                       │
     O2-IMS API│                       │O2-DMS API
    (Infrastructure)              (Deployments)
              │                       │
              ▼                       ▼
┌─────────────────────────────────────────────────────────────┐
│              netweave Gateway (Extended)                    │
│  ┌──────────────┐              ┌──────────────┐            │
│  │  O2-IMS      │              │  O2-DMS      │            │
│  │  (Infra)     │              │  (Apps)      │            │
│  └──────┬───────┘              └──────┬───────┘            │
│         │                             │                     │
│  ┌──────┴─────────────────────────────┴──────┐             │
│  │   Unified Adapter Registry & Routing      │             │
│  └──────┬─────────────────────────┬───────────┘            │
└─────────┼─────────────────────────┼────────────────────────┘
          │                         │
    ┌─────┴─────┐             ┌─────┴─────┐
    ▼           ▼             ▼           ▼
┌────────┐  ┌────────┐   ┌────────┐  ┌────────┐
│K8s IMS │  │DTIAS   │   │Helm    │  │ArgoCD  │
│Adapter │  │Adapter │   │Adapter │  │Adapter │
└────────┘  └────────┘   └────────┘  └────────┘
```

### Purpose

- **O2-IMS**: Manages infrastructure (compute, storage, network resources)
- **O2-DMS**: Manages application deployments (CNFs, Helm charts, workloads)
- **O2-SMO**: Orchestrates end-to-end services using both IMS and DMS

---

## O2-DMS Extension

### What is O2-DMS?

O2-DMS manages the **lifecycle of applications and workloads** on infrastructure managed by O2-IMS:

- **CNF Deployments**: Cloud-Native Network Functions
- **Package Management**: Helm charts, container images
- **Lifecycle Operations**: Install, upgrade, rollback, scale, delete
- **Configuration Management**: Application-level configuration

### O2-DMS API Resources

| Resource | Purpose | CRUD |
|----------|---------|------|
| **Deployment Packages** | CNF packages, Helm charts | CRUD |
| **Deployments** | Running CNF instances | CRUD |
| **Deployment Templates** | Reusable deployment patterns | CRUD |
| **Lifecycle Operations** | Scale, upgrade, rollback | Execute |

### Architecture Extension

```
┌─────────────────────────────────────────────────────────────┐
│              netweave Gateway (O2-IMS + O2-DMS)             │
│                                                             │
│  ┌──────────────┐              ┌──────────────┐            │
│  │ O2-IMS       │              │ O2-DMS       │            │
│  │ Handler      │              │ Handler      │            │
│  │              │              │              │            │
│  │ /o2ims/v1/*  │              │ /o2dms/v1/*  │            │
│  └──────┬───────┘              └──────┬───────┘            │
│         │                             │                     │
│  ┌──────┴─────────────────────────────┴──────┐             │
│  │        Unified Adapter Registry           │             │
│  │  • IMS Adapters (K8s, DTIAS, AWS)         │             │
│  │  • DMS Adapters (Helm, ArgoCD, Flux)      │             │
│  └──────┬─────────────────────────┬───────────┘            │
└─────────┼─────────────────────────┼────────────────────────┘
          │                         │
    IMS Adapters              DMS Adapters
          │                         │
    ┌─────┴─────┐             ┌─────┴─────┐
    ▼           ▼             ▼           ▼
┌────────┐  ┌────────┐   ┌────────┐  ┌────────┐
│K8s API │  │DTIAS   │   │Helm    │  │ArgoCD  │
│(Nodes) │  │API     │   │Releases│  │Apps    │
└────────┘  └────────┘   └────────┘  └────────┘
```

### DMS Adapter Interface

```go
// internal/adapter/dms_adapter.go

package adapter

// DMSAdapter handles deployment management operations
type DMSAdapter interface {
    // Metadata
    Name() string
    Version() string
    Capabilities() []Capability

    // Deployment Packages (Helm charts, CNF packages)
    ListDeploymentPackages(ctx context.Context, filter *Filter) ([]*DeploymentPackage, error)
    GetDeploymentPackage(ctx context.Context, id string) (*DeploymentPackage, error)
    UploadDeploymentPackage(ctx context.Context, pkg *DeploymentPackage) error
    DeleteDeploymentPackage(ctx context.Context, id string) error

    // Deployments (CNF instances)
    ListDeployments(ctx context.Context, filter *Filter) ([]*Deployment, error)
    GetDeployment(ctx context.Context, id string) (*Deployment, error)
    CreateDeployment(ctx context.Context, dep *Deployment) (*Deployment, error)
    UpdateDeployment(ctx context.Context, id string, dep *Deployment) (*Deployment, error)
    DeleteDeployment(ctx context.Context, id string) error

    // Lifecycle operations
    ScaleDeployment(ctx context.Context, id string, replicas int) error
    RollbackDeployment(ctx context.Context, id string, revision int) error
    UpgradeDeployment(ctx context.Context, id string, packageID string) error

    // Health and lifecycle
    Health(ctx context.Context) error
    Close() error
}

// DeploymentPackage represents a CNF package or Helm chart
type DeploymentPackage struct {
    PackageID       string                 `json:"packageId"`
    Name            string                 `json:"name"`
    Version         string                 `json:"version"`
    Description     string                 `json:"description"`
    PackageType     string                 `json:"packageType"`     // "helm", "cnf", "docker"
    PackageURL      string                 `json:"packageUrl"`
    Checksum        string                 `json:"checksum"`
    UploadedAt      time.Time              `json:"uploadedAt"`
    Extensions      map[string]interface{} `json:"extensions,omitempty"`
}

// Deployment represents a running CNF instance
type Deployment struct {
    DeploymentID    string                 `json:"deploymentId"`
    Name            string                 `json:"name"`
    Namespace       string                 `json:"namespace"`
    PackageID       string                 `json:"packageId"`
    PackageVersion  string                 `json:"packageVersion"`
    ResourcePoolID  string                 `json:"resourcePoolId,omitempty"`  // Link to O2-IMS
    Status          string                 `json:"status"`                     // "deployed", "failed", "pending"
    Values          map[string]interface{} `json:"values,omitempty"`
    CreatedAt       time.Time              `json:"createdAt"`
    UpdatedAt       time.Time              `json:"updatedAt"`
    Extensions      map[string]interface{} `json:"extensions,omitempty"`
}
```

### Helm Adapter Implementation

```go
// internal/adapters/helm/adapter.go

package helm

import (
    "context"
    "github.com/yourorg/netweave/internal/adapter"
    "helm.sh/helm/v3/pkg/action"
    "helm.sh/helm/v3/pkg/chart/loader"
)

type HelmAdapter struct {
    actionConfig *action.Configuration
    kubeClient   kubernetes.Interface
}

func NewHelmAdapter(kubeconfig string) (*HelmAdapter, error) {
    actionConfig := new(action.Configuration)
    if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
        return nil, err
    }

    return &HelmAdapter{
        actionConfig: actionConfig,
    }, nil
}

// List Deployments → List Helm Releases
func (a *HelmAdapter) ListDeployments(ctx context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
    client := action.NewList(a.actionConfig)
    client.All = true

    releases, err := client.Run()
    if err != nil {
        return nil, fmt.Errorf("failed to list helm releases: %w", err)
    }

    deployments := make([]*adapter.Deployment, 0, len(releases))
    for _, release := range releases {
        dep := a.transformReleaseToDeployment(release)
        if filter.Matches(dep) {
            deployments = append(deployments, dep)
        }
    }

    return deployments, nil
}

// Create Deployment → Install Helm Release
func (a *HelmAdapter) CreateDeployment(ctx context.Context, dep *adapter.Deployment) (*adapter.Deployment, error) {
    client := action.NewInstall(a.actionConfig)
    client.ReleaseName = dep.Name
    client.Namespace = dep.Namespace
    client.Wait = true
    client.Timeout = 5 * time.Minute

    // Load chart from URL or repository
    chartPath, err := client.ChartPathOptions.LocateChart(dep.PackageURL, settings)
    if err != nil {
        return nil, fmt.Errorf("failed to locate chart: %w", err)
    }

    chart, err := loader.Load(chartPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load chart: %w", err)
    }

    // Install release
    release, err := client.Run(chart, dep.Values)
    if err != nil {
        return nil, fmt.Errorf("failed to install release: %w", err)
    }

    return a.transformReleaseToDeployment(release), nil
}

// Scale Deployment → Update Helm Release Values
func (a *HelmAdapter) ScaleDeployment(ctx context.Context, id string, replicas int) error {
    client := action.NewUpgrade(a.actionConfig)
    client.ResetValues = false

    // Get current release
    getClient := action.NewGet(a.actionConfig)
    release, err := getClient.Run(id)
    if err != nil {
        return fmt.Errorf("failed to get release: %w", err)
    }

    // Update replicas in values
    values := release.Config
    if values == nil {
        values = make(map[string]interface{})
    }
    values["replicaCount"] = replicas

    // Upgrade release
    _, err = client.Run(id, release.Chart, values)
    return err
}

// Transform Helm Release → O2-DMS Deployment
func (a *HelmAdapter) transformReleaseToDeployment(release *release.Release) *adapter.Deployment {
    return &adapter.Deployment{
        DeploymentID:   release.Name,
        Name:           release.Name,
        Namespace:      release.Namespace,
        PackageID:      release.Chart.Metadata.Name,
        PackageVersion: release.Chart.Metadata.Version,
        Status:         string(release.Info.Status),
        Values:         release.Config,
        CreatedAt:      release.Info.FirstDeployed.Time,
        UpdatedAt:      release.Info.LastDeployed.Time,
        Extensions: map[string]interface{}{
            "helm.chart":      release.Chart.Metadata.Name,
            "helm.version":    release.Chart.Metadata.Version,
            "helm.revision":   release.Version,
            "helm.appVersion": release.Chart.Metadata.AppVersion,
            "helm.notes":      release.Info.Notes,
        },
    }
}
```

### ArgoCD Adapter Implementation

```go
// internal/adapters/argocd/adapter.go

package argocd

import (
    "context"
    "github.com/yourorg/netweave/internal/adapter"
    argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
    "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
)

type ArgoCDAdapter struct {
    clientset versioned.Interface
}

// List Deployments → List ArgoCD Applications
func (a *ArgoCDAdapter) ListDeployments(ctx context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
    apps, err := a.clientset.ArgoprojV1alpha1().Applications("").List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list argocd applications: %w", err)
    }

    deployments := make([]*adapter.Deployment, 0, len(apps.Items))
    for _, app := range apps.Items {
        dep := a.transformAppToDeployment(&app)
        if filter.Matches(dep) {
            deployments = append(deployments, dep)
        }
    }

    return deployments, nil
}

// Create Deployment → Create ArgoCD Application
func (a *ArgoCDAdapter) CreateDeployment(ctx context.Context, dep *adapter.Deployment) (*adapter.Deployment, error) {
    app := &argocdv1alpha1.Application{
        ObjectMeta: metav1.ObjectMeta{
            Name:      dep.Name,
            Namespace: "argocd",
        },
        Spec: argocdv1alpha1.ApplicationSpec{
            Project: "default",
            Source: argocdv1alpha1.ApplicationSource{
                RepoURL:        dep.PackageURL,
                TargetRevision: dep.PackageVersion,
                Path:           ".",
                Helm: &argocdv1alpha1.ApplicationSourceHelm{
                    Values: marshalValues(dep.Values),
                },
            },
            Destination: argocdv1alpha1.ApplicationDestination{
                Server:    "https://kubernetes.default.svc",
                Namespace: dep.Namespace,
            },
            SyncPolicy: &argocdv1alpha1.SyncPolicy{
                Automated: &argocdv1alpha1.SyncPolicyAutomated{
                    Prune:    true,
                    SelfHeal: true,
                },
            },
        },
    }

    created, err := a.clientset.ArgoprojV1alpha1().Applications("argocd").Create(ctx, app, metav1.CreateOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to create argocd application: %w", err)
    }

    return a.transformAppToDeployment(created), nil
}

// Transform ArgoCD Application → O2-DMS Deployment
func (a *ArgoCDAdapter) transformAppToDeployment(app *argocdv1alpha1.Application) *adapter.Deployment {
    return &adapter.Deployment{
        DeploymentID:   app.Name,
        Name:           app.Name,
        Namespace:      app.Spec.Destination.Namespace,
        PackageURL:     app.Spec.Source.RepoURL,
        PackageVersion: app.Spec.Source.TargetRevision,
        Status:         string(app.Status.Health.Status),
        CreatedAt:      app.CreationTimestamp.Time,
        Extensions: map[string]interface{}{
            "argocd.project":    app.Spec.Project,
            "argocd.syncStatus": string(app.Status.Sync.Status),
            "argocd.revision":   app.Status.Sync.Revision,
            "argocd.server":     app.Spec.Destination.Server,
        },
    }
}
```

### O2-DMS API Endpoints

```go
// internal/server/router.go

func (s *Server) setupDMSRoutes() {
    dms := s.router.Group("/o2dms/v1")
    {
        dms.Use(s.authMiddleware())
        dms.Use(s.metricsMiddleware())

        // Deployment Packages
        dms.GET("/deploymentPackages", s.handleListDeploymentPackages)
        dms.GET("/deploymentPackages/:id", s.handleGetDeploymentPackage)
        dms.POST("/deploymentPackages", s.handleUploadDeploymentPackage)
        dms.DELETE("/deploymentPackages/:id", s.handleDeleteDeploymentPackage)

        // Deployments
        dms.GET("/deployments", s.handleListDeployments)
        dms.GET("/deployments/:id", s.handleGetDeployment)
        dms.POST("/deployments", s.handleCreateDeployment)
        dms.PUT("/deployments/:id", s.handleUpdateDeployment)
        dms.DELETE("/deployments/:id", s.handleDeleteDeployment)

        // Lifecycle operations
        dms.POST("/deployments/:id/scale", s.handleScaleDeployment)
        dms.POST("/deployments/:id/rollback", s.handleRollbackDeployment)
        dms.POST("/deployments/:id/upgrade", s.handleUpgradeDeployment)

        // Deployment status and logs
        dms.GET("/deployments/:id/status", s.handleGetDeploymentStatus)
        dms.GET("/deployments/:id/logs", s.handleGetDeploymentLogs)
    }
}

// Example handler
func (s *Server) handleCreateDeployment(c *gin.Context) {
    var req adapter.Deployment
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Route to appropriate DMS adapter
    dmsAdapter, err := s.dmsRegistry.Route("deployment", &adapter.Filter{
        Namespace: req.Namespace,
    })
    if err != nil {
        c.JSON(500, gin.H{"error": "adapter routing failed"})
        return
    }

    // Create deployment
    deployment, err := dmsAdapter.CreateDeployment(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(201, deployment)
}
```

---

## O2-SMO Integration

### What is O2-SMO?

O2-SMO (Service Management and Orchestration) is the **top-level orchestrator** that:

- Manages end-to-end service lifecycle
- Calls O2-IMS to provision infrastructure
- Calls O2-DMS to deploy workloads
- Handles FCAPS (Fault, Configuration, Accounting, Performance, Security)
- Integrates with BSS/OSS systems

### Integration Architecture

```
┌───────────────────────────────────────────────────────────┐
│                    O2 SMO                                 │
│                                                           │
│  ┌─────────────────────────────────────────────┐         │
│  │     Service Orchestration Logic             │         │
│  │  1. Create infrastructure (O2-IMS)          │         │
│  │  2. Deploy CNFs (O2-DMS)                    │         │
│  │  3. Monitor (Subscriptions)                 │         │
│  │  4. Lifecycle management                    │         │
│  └─────────────┬───────────────────────────────┘         │
│                │                                          │
│  ┌─────────────┴───────────────────────────────┐         │
│  │      SMO Northbound Interface               │         │
│  │  • External orchestrators (ONAP, OSM)       │         │
│  │  • BSS/OSS systems                          │         │
│  └─────────────────────────────────────────────┘         │
└───────────────────────────────────────────────────────────┘
                 │           │
        O2-IMS API│           │O2-DMS API
                 │           │
                 ▼           ▼
┌───────────────────────────────────────────────────────────┐
│              netweave Gateway                             │
│                                                           │
│  ┌──────────────┐        ┌──────────────┐               │
│  │ O2-IMS       │        │ O2-DMS       │               │
│  │ (Infra)      │        │ (Apps)       │               │
│  └──────┬───────┘        └──────┬───────┘               │
│         │                       │                        │
│  ┌──────┴───────────────────────┴───────┐               │
│  │   Unified Subscription System        │               │
│  │  • Infrastructure events (O2-IMS)    │               │
│  │  • Deployment events (O2-DMS)        │               │
│  │  • Unified webhook delivery to SMO   │               │
│  └──────────────────────────────────────┘               │
└───────────────────────────────────────────────────────────┘
```

### Unified Subscription System

The key integration point is **unified subscriptions** across IMS and DMS:

```go
// internal/subscription/unified.go

package subscription

type UnifiedSubscriptionStore struct {
    redis      *redis.Client
    imsEvents  chan *IMSEvent
    dmsEvents  chan *DMSEvent
}

// Subscribe to both infrastructure and deployment events
func (s *UnifiedSubscriptionStore) Subscribe(ctx context.Context, sub *Subscription) error {
    // Parse filter to determine which events to watch
    if sub.Filter.ResourcePoolID != "" || sub.Filter.ResourceID != "" {
        // Watch IMS events (infrastructure)
        go s.watchIMSEvents(ctx, sub)
    }

    if sub.Filter.DeploymentID != "" || sub.Filter.PackageID != "" {
        // Watch DMS events (deployments)
        go s.watchDMSEvents(ctx, sub)
    }

    // Store subscription in Redis
    return s.store.Create(ctx, sub)
}

// Unified webhook delivery
func (s *UnifiedSubscriptionStore) deliverWebhook(sub *Subscription, event interface{}) {
    notification := map[string]interface{}{
        "subscriptionId": sub.SubscriptionID,
        "timestamp":      time.Now().UTC(),
    }

    switch e := event.(type) {
    case *IMSEvent:
        notification["eventType"] = "ResourcePoolCreated"
        notification["resourceType"] = "infrastructure"
        notification["resource"] = e.ResourcePool
    case *DMSEvent:
        notification["eventType"] = "DeploymentReady"
        notification["resourceType"] = "deployment"
        notification["deployment"] = e.Deployment
    }

    // POST to SMO callback
    s.httpClient.Post(sub.Callback, notification)
}

// Watch DMS events (Helm releases)
func (s *UnifiedSubscriptionStore) watchDMSEvents(ctx context.Context, sub *Subscription) {
    // Watch Helm release events
    informer := helm.NewReleaseInformer(...)

    informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            release := obj.(*helm.Release)
            if s.matchesFilter(release, sub.Filter) {
                s.deliverWebhook(sub, &DMSEvent{
                    Type:       "DeploymentCreated",
                    Deployment: transformReleaseToDeployment(release),
                })
            }
        },
        UpdateFunc: func(oldObj, newObj interface{}) {
            release := newObj.(*helm.Release)
            if s.matchesFilter(release, sub.Filter) {
                s.deliverWebhook(sub, &DMSEvent{
                    Type:       "DeploymentUpdated",
                    Deployment: transformReleaseToDeployment(release),
                })
            }
        },
    })
}
```

---

## Complete Architecture

### Directory Structure

```
netweave/
├── cmd/
│   ├── gateway/              # Main gateway (IMS + DMS)
│   └── operator/             # Optional operator
├── internal/
│   ├── adapter/
│   │   ├── adapter.go        # IMS adapter interface
│   │   ├── dms_adapter.go    # DMS adapter interface (NEW)
│   │   └── registry.go       # Unified registry
│   ├── adapters/
│   │   ├── k8s/              # Kubernetes IMS adapter
│   │   ├── dtias/            # Dell DTIAS IMS adapter
│   │   ├── aws/              # AWS IMS adapter
│   │   ├── helm/             # Helm DMS adapter (NEW)
│   │   ├── argocd/           # ArgoCD DMS adapter (NEW)
│   │   ├── flux/             # Flux DMS adapter (NEW)
│   │   └── mock/             # Mock adapters
│   ├── o2ims/                # O2-IMS handlers
│   ├── o2dms/                # O2-DMS handlers (NEW)
│   ├── subscription/
│   │   ├── ims.go            # IMS event watching
│   │   ├── dms.go            # DMS event watching (NEW)
│   │   └── unified.go        # Unified subscriptions (NEW)
│   └── server/
│       ├── router.go         # Combined IMS + DMS routes
│       └── tls.go
├── pkg/
│   ├── o2ims/                # O2-IMS models
│   ├── o2dms/                # O2-DMS models (NEW)
│   └── smo/                  # SMO integration helpers (NEW)
└── docs/
    ├── architecture.md       # Updated with DMS
    ├── api-mapping.md        # Updated with DMS mappings
    └── o2dms-o2smo-extension.md  # This file
```

### Configuration Example

```yaml
# config/gateway.yaml

# O2-IMS Adapters
imsAdapters:
  - name: kubernetes
    type: k8s
    enabled: true
    default: true
    config:
      kubeconfig: /etc/kubernetes/admin.conf

  - name: dtias
    type: dtias
    enabled: true
    config:
      endpoint: https://dtias.dell.com/api
      apiKey: ${DTIAS_API_KEY}

# O2-DMS Adapters (NEW)
dmsAdapters:
  - name: helm
    type: helm
    enabled: true
    default: true
    config:
      namespace: default
      driver: secrets

  - name: argocd
    type: argocd
    enabled: true
    config:
      server: argocd-server.argocd.svc.cluster.local
      namespace: argocd

# Unified Routing
routing:
  ims:
    default: kubernetes
    rules:
      - name: bare-metal-to-dtias
        adapter: dtias
        resourceType: resourcePool
        conditions:
          location:
            prefix: dc-

  dms:
    default: helm
    rules:
      - name: gitops-to-argocd
        adapter: argocd
        resourceType: deployment
        conditions:
          extensions:
            deploymentType: gitops
```

---

## Use Cases

### Use Case 1: Deploy a 5G vDU (Virtualized Distributed Unit)

**SMO Workflow:**

```
1. SMO decides to deploy vDU at edge site

2. SMO → O2-IMS: Create Resource Pool
   POST /o2ims/v1/resourcePools
   {
     "name": "vDU-Pool",
     "location": "edge-site-1",
     "extensions": {
       "instanceType": "c5.metal",
       "replicas": 10,
       "cpuPinning": true
     }
   }
   ← Response: resourcePoolId = "pool-vdu-123"

3. SMO → O2-IMS: Subscribe to pool events
   POST /o2ims/v1/subscriptions
   {
     "callback": "https://smo.example.com/notify",
     "filter": {"resourcePoolId": "pool-vdu-123"}
   }

   ← Webhook: Pool ready with 10 nodes

4. SMO → O2-DMS: Upload vDU CNF package
   POST /o2dms/v1/deploymentPackages
   {
     "packageURL": "https://repo.example.com/vdu-v2.1.0.tgz",
     "packageType": "helm",
     "checksum": "sha256:..."
   }
   ← Response: packageId = "pkg-vdu-v2.1.0"

5. SMO → O2-DMS: Deploy vDU
   POST /o2dms/v1/deployments
   {
     "name": "vdu-edge-site-1",
     "packageId": "pkg-vdu-v2.1.0",
     "resourcePoolId": "pool-vdu-123",
     "namespace": "vdu-ns",
     "values": {
       "cellId": "12345",
       "bandwidth": "100MHz",
       "replicaCount": 3
     }
   }
   ← Response: deploymentId = "dep-vdu-001"

6. SMO monitors via unified subscriptions
   ← Webhook: Deployment progressing
   ← Webhook: Deployment ready (all pods healthy)

7. SMO performs configuration management (out of scope for O2)
```

### Use Case 2: Scale Deployment Based on Traffic

```
1. SMO monitors traffic metrics

2. SMO detects high load on vDU

3. SMO → O2-DMS: Scale deployment
   POST /o2dms/v1/deployments/dep-vdu-001/scale
   {
     "replicas": 5
   }

4. netweave → Helm: Upgrade release with new replica count

5. SMO receives webhook notification
   ← Webhook: Deployment scaled to 5 replicas
```

### Use Case 3: Rollback Failed Upgrade

```
1. SMO → O2-DMS: Upgrade deployment
   POST /o2dms/v1/deployments/dep-vdu-001/upgrade
   {
     "packageId": "pkg-vdu-v2.2.0"
   }

2. Deployment fails health checks

3. SMO receives webhook notification
   ← Webhook: Deployment failed

4. SMO → O2-DMS: Rollback deployment
   POST /o2dms/v1/deployments/dep-vdu-001/rollback
   {
     "revision": 1
   }

5. netweave → Helm: Rollback to previous release

6. SMO receives webhook notification
   ← Webhook: Deployment rolled back successfully
```

---

## Implementation Guide

### Phase 1: Extend Adapter Interface (Week 1-2)

**Tasks:**
1. Define `DMSAdapter` interface in `internal/adapter/dms_adapter.go`
2. Create `DeploymentPackage` and `Deployment` models in `pkg/o2dms/models.go`
3. Extend `Registry` to support both IMS and DMS adapters
4. Update configuration structure to support DMS adapters

**Deliverables:**
- Complete DMS adapter interface definition
- Updated registry with dual adapter support
- Configuration schema for DMS adapters

### Phase 2: Implement Helm Adapter (Week 3-4)

**Tasks:**
1. Create Helm adapter in `internal/adapters/helm/`
2. Implement all `DMSAdapter` interface methods
3. Add Helm-specific transformations (Release ↔ Deployment)
4. Write unit tests with mock Helm client
5. Integration tests with real Helm releases

**Deliverables:**
- Working Helm adapter with full CRUD
- Comprehensive test coverage (≥80%)
- Documentation for Helm adapter configuration

### Phase 3: Add O2-DMS API Handlers (Week 5-6)

**Tasks:**
1. Create DMS handlers in `internal/o2dms/handlers/`
2. Define O2-DMS API routes in `internal/server/router.go`
3. Implement request validation middleware
4. Add OpenAPI spec for O2-DMS endpoints
5. E2E tests for all DMS endpoints

**Deliverables:**
- Complete O2-DMS API implementation
- OpenAPI specification
- E2E test suite

### Phase 4: Unified Subscriptions (Week 7-8)

**Tasks:**
1. Extend subscription system to watch DMS events
2. Implement DMS event informers (Helm releases)
3. Update webhook delivery for DMS events
4. Add subscription filtering for deployment events
5. Test unified subscription workflows

**Deliverables:**
- Unified subscription system supporting IMS + DMS
- DMS event watching and delivery
- Integration tests for cross-domain subscriptions

### Phase 5: Additional DMS Adapters (Week 9-12)

**Tasks:**
1. Implement ArgoCD adapter
2. Implement Flux adapter (optional)
3. Add adapter routing configuration
4. Multi-adapter aggregation for deployments
5. Performance testing and optimization

**Deliverables:**
- ArgoCD adapter with full CRUD
- Optional Flux adapter
- Multi-DMS-backend support
- Performance benchmarks

### Testing Strategy

**Unit Tests:**
```go
// internal/adapters/helm/adapter_test.go
func TestHelmAdapter_CreateDeployment(t *testing.T) {
    tests := []struct {
        name    string
        dep     *adapter.Deployment
        wantErr bool
    }{
        {
            name: "successful deployment",
            dep: &adapter.Deployment{
                Name:      "test-app",
                Namespace: "default",
                PackageURL: "https://charts.example.com/test",
                Values: map[string]interface{}{
                    "replicaCount": 3,
                },
            },
            wantErr: false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            adapter := setupMockHelmAdapter(t)
            _, err := adapter.CreateDeployment(context.Background(), tt.dep)
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateDeployment() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

**Integration Tests:**
```go
// tests/integration/o2dms_test.go
func TestO2DMS_DeploymentLifecycle(t *testing.T) {
    // Setup: Start gateway with Helm adapter
    gateway := setupGateway(t)
    defer gateway.Cleanup()

    // Create deployment package
    pkg := createTestPackage(t, gateway)

    // Create deployment
    dep := createDeployment(t, gateway, pkg.PackageID)
    assert.Equal(t, "deployed", dep.Status)

    // Scale deployment
    scaleDeployment(t, gateway, dep.DeploymentID, 5)

    // Delete deployment
    deleteDeployment(t, gateway, dep.DeploymentID)
}
```

**E2E Tests:**
```go
// tests/e2e/smo_workflow_test.go
func TestSMO_DeployVDU(t *testing.T) {
    e2e := setupE2EEnvironment(t)
    defer e2e.Cleanup()

    // SMO creates resource pool
    pool := e2e.CreateResourcePool(...)

    // SMO uploads CNF package
    pkg := e2e.UploadPackage(...)

    // SMO deploys CNF
    deployment := e2e.CreateDeployment(...)

    // Verify deployment is healthy
    assert.Equal(t, "healthy", deployment.Status)
}
```

---

## Summary

### Benefits of O2-DMS + O2-SMO Extension

✅ **Unified Gateway**: Single entry point for both infrastructure (IMS) and applications (DMS)
✅ **Consistent Adapter Pattern**: Same architecture for IMS and DMS backends
✅ **Unified Subscriptions**: Single webhook system for all events
✅ **SMO-Friendly**: Natural orchestration workflow (provision → deploy → monitor)
✅ **Multi-Backend**: Support multiple deployment tools (Helm, ArgoCD, Flux)
✅ **API Versioning**: Independent evolution of O2-IMS and O2-DMS APIs

### Technology Stack Extensions

| Layer | O2-IMS | O2-DMS |
|-------|--------|--------|
| **Adapters** | Kubernetes, DTIAS, AWS | Helm, ArgoCD, Flux |
| **Resources** | Nodes, Pools, Types | Packages, Deployments |
| **API Path** | `/o2ims/v1/*` | `/o2dms/v1/*` |
| **Events** | ResourcePool, Resource | Deployment, Package |

### Next Steps

1. **Review this design** with stakeholders and SMO integration teams
2. **Update ARCHITECTURE_SUMMARY.md** to include O2-DMS overview
3. **Create GitHub issues** for each implementation phase
4. **Begin Phase 1**: Extend adapter interface and registry
5. **Parallel development**: IMS adapters (DTIAS, AWS) and DMS adapters (Helm, ArgoCD)

---

**For questions or clarifications, refer to:**
- Architecture: [docs/architecture.md](architecture.md)
- API Mappings: [docs/api-mapping.md](api-mapping.md)
- Summary: [ARCHITECTURE_SUMMARY.md](../ARCHITECTURE_SUMMARY.md)
