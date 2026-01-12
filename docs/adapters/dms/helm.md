# Helm DMS Adapter

**Status:** 📋 Specification Complete  
**Version:** 1.0  
**Last Updated:** 2026-01-12

## Overview

The Helm adapter manages CNF deployments using Helm 3. It maps Helm releases to O2-DMS deployments and provides full lifecycle management including scaling and rollback.

## Capabilities

```go
capabilities := []Capability{
    CapPackageManagement,
    CapDeploymentLifecycle,
    CapRollback,
    CapScaling,  // Via values override
}
```

## Configuration

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

## Implementation

```go
package helm

import (
    "context"
    "helm.sh/helm/v3/pkg/action"
    "helm.sh/helm/v3/pkg/chart/loader"
    "helm.sh/helm/v3/pkg/cli"
)

type HelmPlugin struct {
    name      string
    version   string
    config    *Config
    settings  *cli.EnvSettings
    actionCfg *action.Configuration
}

// CreateDeployment → Helm Install
func (p *HelmPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    client := action.NewInstall(p.actionCfg)
    client.Namespace = req.Namespace
    client.ReleaseName = req.Name
    client.Wait = true
    client.Timeout = parseDuration(p.config.Timeout)

    chartPath, err := client.LocateChart(req.PackageID, p.settings)
    if err != nil {
        return nil, err
    }

    chart, err := loader.Load(chartPath)
    if err != nil {
        return nil, err
    }

    release, err := client.Run(chart, req.Values)
    if err != nil {
        return nil, fmt.Errorf("helm install failed: %w", err)
    }

    return &dms.Deployment{
        ID:        release.Name,
        Name:      release.Name,
        PackageID: req.PackageID,
        Namespace: release.Namespace,
        Status:    transformHelmStatus(release.Info.Status),
        Version:   release.Version,
        CreatedAt: release.Info.FirstDeployed.Time,
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
    return client.Run(id)
}

// ScaleDeployment → Update Helm values
func (p *HelmPlugin) ScaleDeployment(ctx context.Context, id string, replicas int) error {
    client := action.NewUpgrade(p.actionCfg)
    client.Wait = true

    getClient := action.NewGet(p.actionCfg)
    release, err := getClient.Run(id)
    if err != nil {
        return err
    }

    values := release.Config
    values["replicaCount"] = replicas

    _, err = client.Run(id, release.Chart, values)
    return err
}
```

## See Also

- [DMS Adapter Interface](README.md)
- [GitOps Adapters](gitops.md)
