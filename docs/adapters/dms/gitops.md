# GitOps DMS Adapters

**Status:** 📋 Specification Complete  
**Version:** 1.0  
**Last Updated:** 2026-01-12

## Overview

GitOps adapters manage deployments using declarative Git repositories as the source of truth. Supported platforms include ArgoCD and Flux CD.

## ArgoCD Adapter

### Configuration

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

### Implementation

```go
package argocd

import (
    "github.com/argoproj/argo-cd/v2/pkg/apiclient"
    "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
    argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

type ArgoCDPlugin struct {
    name   string
    client apiclient.Client
    appIf  application.ApplicationServiceClient
    config *Config
}

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

    return transformAppToDeployment(created), nil
}
```

## Flux CD Adapter

### Configuration

```yaml
plugins:
  dms:
    - name: flux-gitops
      type: flux
      enabled: true
      config:
        namespace: flux-system
        gitProvider: github  # github, gitlab, bitbucket
        defaultBranch: main
```

### Implementation

Flux adapter manages GitOps deployments via Flux CD Kustomization and HelmRelease resources.

## See Also

- [DMS Adapter Interface](README.md)
- [Helm Adapter](helm.md)
