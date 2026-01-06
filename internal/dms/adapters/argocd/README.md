# ArgoCD DMS Adapter

This package provides an O2-DMS adapter implementation for ArgoCD, enabling GitOps-based CNF/VNF deployment management.

## Dependency Conflict Resolution

This adapter uses the **Kubernetes dynamic client** to manage ArgoCD Application CRDs directly, rather than importing the ArgoCD library. This approach avoids the dependency conflict documented in [GitHub Issue #7](https://github.com/piwi3910/netweave/issues/7):

- **Problem**: ArgoCD v2 requires `k8s.io/structured-merge-diff/v4`, while newer Kubernetes client versions require `structured-merge-diff/v6`, causing incompatible type conversions.
- **Solution**: By using `k8s.io/client-go/dynamic` to interact with ArgoCD's Application CRD as unstructured objects, we eliminate the need to import the ArgoCD library entirely.

## Features

- Full O2-DMS adapter interface implementation
- GitOps-based deployment management
- No ArgoCD library dependency (uses Kubernetes dynamic client)
- Supports all ArgoCD Application operations:
  - Create, Read, Update, Delete Applications
  - Sync and rollback operations
  - Health and sync status monitoring
  - Deployment history tracking

## Usage

```go
import "github.com/piwi3910/netweave/internal/dms/adapters/argocd"

// Create adapter configuration
config := &argocd.Config{
    Namespace:      "argocd",        // ArgoCD namespace
    DefaultProject: "default",       // Default ArgoCD project
    AutoSync:       true,            // Enable auto-sync for new Applications
    Prune:          true,            // Enable pruning during sync
    SelfHeal:       true,            // Enable self-healing
}

// Create the adapter
adapter, err := argocd.NewAdapter(config)
if err != nil {
    log.Fatal(err)
}
defer adapter.Close()

// Create a deployment
deployment, err := adapter.CreateDeployment(ctx, &adapter.DeploymentRequest{
    Name:      "my-app",
    Namespace: "production",
    Extensions: map[string]interface{}{
        "argocd.repoURL":        "https://github.com/example/repo",
        "argocd.path":           "apps/my-app",
        "argocd.targetRevision": "main",
    },
})
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `Kubeconfig` | Path to kubeconfig file (uses in-cluster config if empty) | "" |
| `Namespace` | Namespace where ArgoCD Applications are created | "argocd" |
| `ArgoServerURL` | ArgoCD server URL (optional) | "" |
| `DefaultProject` | Default ArgoCD project for new Applications | "default" |
| `SyncTimeout` | Timeout for sync operations | 10m |
| `AutoSync` | Enable automatic syncing for new Applications | false |
| `Prune` | Enable pruning of resources not in Git | false |
| `SelfHeal` | Enable automatic self-healing | false |

## Capabilities

This adapter supports the following O2-DMS capabilities:

- `deployment-lifecycle` - Full CRUD operations for deployments
- `gitops` - GitOps-based deployment workflows
- `rollback` - Rollback to previous revisions
- `health-checks` - Health status monitoring
- `metrics` - Deployment metrics

## Extensions

When creating deployments, use the following extensions:

| Extension | Description | Required |
|-----------|-------------|----------|
| `argocd.repoURL` | Git repository URL | Yes |
| `argocd.path` | Path within the repository | No |
| `argocd.targetRevision` | Git revision (branch, tag, commit) | No (defaults to "HEAD") |
| `argocd.chart` | Helm chart name (for Helm-based apps) | No |

## Testing

```bash
# Run unit tests
go test ./internal/dms/adapters/argocd/...

# Run with coverage
go test -cover ./internal/dms/adapters/argocd/...

# Run benchmarks
go test -bench=. ./internal/dms/adapters/argocd/...
```
