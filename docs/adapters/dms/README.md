# DMS Adapter Interface

**Version:** 1.0
**Last Updated:** 2026-01-12

## Overview

DMS (Deployment Management Services) adapters manage application deployment lifecycle across multiple backend systems. They provide a unified interface for package management, deployment operations, and lifecycle management.

## Core Interface

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

## Data Models

### DeploymentPackage

```go
type DeploymentPackage struct {
    PackageID   string                 `json:"packageId"`
    Name        string                 `json:"name"`
    Version     string                 `json:"version"`
    Description string                 `json:"description,omitempty"`
    PackageType string                 `json:"packageType"`  // "helm", "cnf", "docker", "git-repo"
    PackageURL  string                 `json:"packageUrl"`
    Checksum    string                 `json:"checksum,omitempty"`
    UploadedAt  time.Time              `json:"uploadedAt"`
    Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

type DeploymentPackageUpload struct {
    Name        string
    Version     string
    PackageType string
    Content     []byte  // Package binary content
    URL         string  // Or URL to package repository
}
```

### Deployment

```go
type Deployment struct {
    DeploymentID   string                 `json:"deploymentId"`
    Name           string                 `json:"name"`
    Namespace      string                 `json:"namespace"`
    PackageID      string                 `json:"packageId"`
    PackageVersion string                 `json:"packageVersion"`
    ResourcePoolID string                 `json:"resourcePoolId,omitempty"`  // Link to O2-IMS
    Status         string                 `json:"status"`  // "deployed", "failed", "pending"
    Values         map[string]interface{} `json:"values,omitempty"`
    CreatedAt      time.Time              `json:"createdAt"`
    UpdatedAt      time.Time              `json:"updatedAt"`
    Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

type DeploymentRequest struct {
    Name           string
    Namespace      string
    PackageID      string
    ResourcePoolID string
    Values         map[string]interface{}
}
```

## DMS Capabilities

```go
const (
    CapPackageManagement   Capability = "package-management"
    CapDeploymentLifecycle Capability = "deployment-lifecycle"
    CapRollback            Capability = "rollback"
    CapScaling             Capability = "scaling"
    CapGitOps              Capability = "gitops"
)
```

## Available DMS Adapters

| Adapter | Status | Package Format | Deployment Target | GitOps | Rollback |
|---------|--------|----------------|-------------------|--------|----------|
| **Helm** | 📋 Spec | Helm Chart | Kubernetes | No | Yes |
| **ArgoCD** | 📋 Spec | Git Repo | Kubernetes | Yes | Yes |
| **Flux CD** | 📋 Spec | Git Repo | Kubernetes | Yes | Yes |
| **ONAP-LCM** | 📋 Spec | ONAP Package | Multi-Cloud | No | Yes |
| **OSM-LCM** | 📋 Spec | OSM Package | Multi-Cloud | No | Yes |

## Adapter Documentation

- [Helm Adapter](helm.md) - Helm chart deployment
- [GitOps Adapters](gitops.md) - ArgoCD, Flux CD
- [Orchestrator Adapters](orchestrators.md) - ONAP-LCM, OSM-LCM

## See Also

- [Adapter Pattern Overview](../README.md)
- [IMS Adapters](../ims/README.md)
- [SMO Adapters](../smo/README.md)
