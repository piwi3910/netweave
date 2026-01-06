# Helm DMS Adapter

Production-grade O2-DMS adapter implementation using Helm 3 for CNF/VNF deployment management.

## Overview

The Helm adapter enables the netweave O2-DMS gateway to manage deployments using Helm charts deployed to Kubernetes clusters. It provides a complete implementation of the DMS adapter interface with support for:

- **Package Management**: Upload and manage Helm charts
- **Deployment Lifecycle**: Install, upgrade, and uninstall Helm releases
- **Rollback**: Revert deployments to previous revisions
- **Scaling**: Horizontal scaling via values updates
- **Status Monitoring**: Real-time deployment status and health checks
- **History**: Track deployment revision history

## Features

### Supported Capabilities

| Capability | Supported | Description |
|------------|-----------|-------------|
| Package Management | ✅ | Upload/manage Helm charts in repositories |
| Deployment Lifecycle | ✅ | Full CRUD operations for deployments |
| Rollback | ✅ | Rollback to any previous revision |
| Scaling | ✅ | Scale deployments via replica count updates |
| Health Checks | ✅ | Monitor deployment and backend health |
| Metrics | ✅ | Deployment metrics and monitoring |
| GitOps | ❌ | Not supported (use ArgoCD adapter for GitOps) |

### Helm Integration

- **Helm Version**: 3.14.0+
- **Storage Backend**: Kubernetes Secrets
- **Repository Support**: ChartMuseum, Harbor, OCI registries
- **Authentication**: Username/password for repository access
- **Timeout Control**: Configurable operation timeouts
- **History Management**: Configurable revision retention

## Configuration

### Basic Configuration

```yaml
plugins:
  dms:
    - name: helm-deployer
      type: helm
      enabled: true
      default: true
      config:
        # Kubernetes Configuration
        kubeconfig: /etc/kubernetes/admin.conf
        namespace: deployments

        # Helm Repository
        repositoryUrl: https://charts.example.com
        repositoryUsername: admin
        repositoryPassword: ${HELM_REPO_PASSWORD}

        # Operation Settings
        timeout: 10m
        maxHistory: 10
        debug: false
```

### Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `kubeconfig` | string | No | `~/.kube/config` | Path to Kubernetes config |
| `namespace` | string | No | `default` | Default namespace for deployments |
| `repositoryUrl` | string | Yes | - | Helm chart repository URL |
| `repositoryUsername` | string | No | - | Repository authentication username |
| `repositoryPassword` | string | No | - | Repository authentication password |
| `timeout` | duration | No | `10m` | Default timeout for operations |
| `maxHistory` | int | No | `10` | Maximum revisions to retain |
| `debug` | bool | No | `false` | Enable verbose Helm output |

## Usage Examples

### Go Code Usage

```go
package main

import (
    "context"
    "time"

    "github.com/yourorg/netweave/internal/dms/adapter"
    "github.com/yourorg/netweave/internal/dms/adapters/helm"
)

func main() {
    // Create Helm adapter
    helmAdapter, err := helm.NewAdapter(&helm.Config{
        Namespace:          "production",
        RepositoryURL:      "https://charts.example.com",
        RepositoryUsername: "admin",
        RepositoryPassword: "secret",
        Timeout:            15 * time.Minute,
        MaxHistory:         20,
    })
    if err != nil {
        panic(err)
    }
    defer helmAdapter.Close()

    ctx := context.Background()

    // Upload a chart package
    pkg, err := helmAdapter.UploadDeploymentPackage(ctx, &adapter.DeploymentPackageUpload{
        Name:        "my-cnf",
        Version:     "1.0.0",
        PackageType: "helm-chart",
        Description: "My CNF application",
        Content:     chartBytes,
    })
    if err != nil {
        panic(err)
    }

    // Create a deployment
    deployment, err := helmAdapter.CreateDeployment(ctx, &adapter.DeploymentRequest{
        Name:      "my-cnf-prod",
        PackageID: pkg.ID,
        Namespace: "production",
        Values: map[string]interface{}{
            "replicaCount": 3,
            "image": map[string]interface{}{
                "tag": "1.0.0",
            },
        },
    })
    if err != nil {
        panic(err)
    }

    // Get deployment status
    status, err := helmAdapter.GetDeploymentStatus(ctx, deployment.ID)
    if err != nil {
        panic(err)
    }

    // Scale deployment
    err = helmAdapter.ScaleDeployment(ctx, deployment.ID, 5)
    if err != nil {
        panic(err)
    }

    // Rollback if needed
    err = helmAdapter.RollbackDeployment(ctx, deployment.ID, 1)
    if err != nil {
        panic(err)
    }

    // Delete deployment
    err = helmAdapter.DeleteDeployment(ctx, deployment.ID)
    if err != nil {
        panic(err)
    }
}
```

### REST API Usage

The Helm adapter is accessed through the O2-DMS REST API:

#### Create Deployment

```bash
curl -X POST https://netweave.example.com/o2dms/v1/deployments \
  --cert client.crt \
  --key client.key \
  --cacert ca.crt \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-cnf-prod",
    "packageId": "my-cnf-1.0.0",
    "namespace": "production",
    "values": {
      "replicaCount": 3,
      "image": {
        "tag": "1.0.0"
      }
    }
  }'
```

#### Get Deployment Status

```bash
curl -X GET https://netweave.example.com/o2dms/v1/deployments/my-cnf-prod/status \
  --cert client.crt \
  --key client.key \
  --cacert ca.crt
```

#### Scale Deployment

```bash
curl -X POST https://netweave.example.com/o2dms/v1/deployments/my-cnf-prod/scale \
  --cert client.crt \
  --key client.key \
  --cacert ca.crt \
  -H "Content-Type: application/json" \
  -d '{
    "replicas": 5
  }'
```

#### Rollback Deployment

```bash
curl -X POST https://netweave.example.com/o2dms/v1/deployments/my-cnf-prod/rollback \
  --cert client.crt \
  --key client.key \
  --cacert ca.crt \
  -H "Content-Type: application/json" \
  -d '{
    "revision": 1
  }'
```

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│              O2-DMS REST API Gateway                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         DMS Adapter Registry                         │  │
│  │  • Route requests to appropriate adapter             │  │
│  │  • Support multiple DMS backends                     │  │
│  └────────────────────┬─────────────────────────────────┘  │
└─────────────────────────┼─────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              Helm DMS Adapter                               │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  DMSAdapter Interface Implementation                 │  │
│  │  • Package Management (Chart upload/delete)          │  │
│  │  • Deployment Lifecycle (Install/Upgrade/Delete)     │  │
│  │  • Operations (Scale/Rollback/Status)                │  │
│  │  • Health Checks                                     │  │
│  └────────────────────┬─────────────────────────────────┘  │
└─────────────────────────┼─────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              Helm 3 SDK (helm.sh/helm/v3)                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Action Configuration                                │  │
│  │  • Install    • Upgrade    • Uninstall               │  │
│  │  • List       • Get        • Status                  │  │
│  │  • History    • Rollback                             │  │
│  └────────────────────┬─────────────────────────────────┘  │
└─────────────────────────┼─────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              Kubernetes API                                 │
│  • Secrets (Helm release storage)                           │
│  • ConfigMaps                                               │
│  • Deployments, StatefulSets, DaemonSets                    │
│  • Services, Ingresses                                      │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Request Reception**: O2-DMS API receives deployment request
2. **Adapter Selection**: Registry routes to Helm adapter
3. **Chart Resolution**: Adapter locates Helm chart in repository
4. **Helm Execution**: Helm SDK performs operation (install/upgrade/etc.)
5. **Kubernetes Apply**: Helm applies manifests to Kubernetes
6. **Status Monitoring**: Adapter tracks release status
7. **Response**: Return deployment status to API caller

## Error Handling

The Helm adapter provides comprehensive error handling:

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `chart not found` | Chart doesn't exist in repository | Verify package ID and repository URL |
| `release already exists` | Deployment name conflict | Use unique deployment name or delete existing |
| `failed to locate chart` | Repository unreachable | Check repository URL and credentials |
| `helm install failed` | Kubernetes resource creation failed | Check namespace permissions and resource quotas |
| `deployment not found` | Release doesn't exist | Verify deployment ID |
| `rollback failed` | Target revision doesn't exist | Check available revisions with GetDeploymentHistory |

### Error Examples

```go
// Handle deployment creation errors
deployment, err := adapter.CreateDeployment(ctx, req)
if err != nil {
    if strings.Contains(err.Error(), "already exists") {
        // Handle name conflict
        return fmt.Errorf("deployment name already in use: %w", err)
    }
    if strings.Contains(err.Error(), "chart not found") {
        // Handle missing chart
        return fmt.Errorf("chart package not found: %w", err)
    }
    return fmt.Errorf("deployment failed: %w", err)
}
```

## Performance Considerations

### Optimization Tips

1. **Timeout Configuration**: Adjust timeouts based on deployment complexity
2. **History Limits**: Reduce `maxHistory` for faster list operations
3. **Concurrent Operations**: Helm adapter is safe for concurrent use
4. **Resource Limits**: Set appropriate Kubernetes resource limits
5. **Namespace Isolation**: Use separate namespaces for different environments

### Performance Metrics

| Operation | Typical Duration | Notes |
|-----------|-----------------|-------|
| Install | 30s - 5m | Depends on chart complexity |
| Upgrade | 30s - 5m | Similar to install |
| Rollback | 10s - 2m | Faster than install |
| Scale | 5s - 30s | Just updates values |
| Status | < 1s | Quick metadata lookup |
| List | < 5s | Scales with release count |

## Testing

### Unit Tests

Run unit tests with:

```bash
go test ./internal/dms/adapters/helm/... -v
```

### Integration Tests

Integration tests require a Kubernetes cluster:

```bash
# Start kind cluster
kind create cluster --name helm-test

# Run integration tests
go test ./internal/dms/adapters/helm/... -tags=integration -v

# Cleanup
kind delete cluster --name helm-test
```

### Test Coverage

Current test coverage: **80%+** (target: ≥80%)

```bash
go test ./internal/dms/adapters/helm/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Troubleshooting

### Debug Mode

Enable debug logging in configuration:

```yaml
config:
  debug: true
```

### Common Issues

#### Issue: Releases not showing up

**Solution**: Check namespace configuration and Kubernetes permissions

```bash
kubectl get secrets -n <namespace> -l owner=helm
```

#### Issue: Chart repository authentication fails

**Solution**: Verify credentials and repository URL

```bash
helm repo add test-repo https://charts.example.com \
  --username admin \
  --password secret
helm repo update
```

#### Issue: Timeout during deployment

**Solution**: Increase timeout in configuration

```yaml
config:
  timeout: 20m  # Increase for complex deployments
```

## Production Deployment

### Prerequisites

- Kubernetes cluster 1.23+
- Helm 3.10+ installed on gateway pods
- Helm chart repository (ChartMuseum, Harbor, or OCI registry)
- Valid kubeconfig with appropriate RBAC permissions

### RBAC Requirements

The service account needs these Kubernetes permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: helm-adapter-role
rules:
  # Helm release storage
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "update", "delete"]
  
  # Kubernetes resources deployed by Helm
  - apiGroups: ["", "apps", "batch", "extensions"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### High Availability

For HA deployments:

1. **Stateless Design**: Helm adapter is stateless (state in Kubernetes)
2. **Multiple Replicas**: Run 3+ gateway pods with Helm adapter
3. **Shared Storage**: Helm release storage in Kubernetes (automatic)
4. **Concurrency Safety**: Helm SDK handles concurrent operations

## Security

### Best Practices

1. **Repository Credentials**: Store in Kubernetes Secrets, not config files
2. **Kubeconfig Security**: Use service account tokens, not admin kubeconfigs
3. **Namespace Isolation**: Deploy to dedicated namespaces
4. **RBAC Restrictions**: Grant minimum required permissions
5. **TLS Verification**: Always verify repository TLS certificates

### Secrets Management

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: helm-repo-credentials
type: Opaque
stringData:
  username: admin
  password: ${HELM_REPO_PASSWORD}
```

Reference in adapter config:

```yaml
config:
  repositoryUsername: ${HELM_REPO_USERNAME}
  repositoryPassword: ${HELM_REPO_PASSWORD}
```

## Contributing

See [CONTRIBUTING.md](../../../../CONTRIBUTING.md) for contribution guidelines.

## License

See [LICENSE](../../../../LICENSE) for license information.
