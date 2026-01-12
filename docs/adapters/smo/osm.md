# OSM SMO Adapter

**Status:** 📋 Specification Complete  
**Version:** 1.0  
**Last Updated:** 2026-01-12

## Overview

The OSM (Open Source MANO) adapter integrates netweave with OSM for NFV orchestration. It operates in dual mode: northbound VIM sync and DMS backend NS/VNF lifecycle management.

## Configuration

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

## Implementation

```go
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

// Northbound: Sync VIMs to OSM
func (p *OSMPlugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
    for _, vim := range p.transformToOSMVIMs(inventory) {
        if err := p.client.CreateOrUpdateVIM(ctx, vim); err != nil {
            return fmt.Errorf("failed to sync VIM: %w", err)
        }
    }
    return nil
}

// DMS Backend: Upload NSD/VNFD
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
        PackageType: pkg.PackageType,
        UploadedAt:  time.Now(),
        Extensions: map[string]interface{}{
            "osm.packageId": packageID,
        },
    }, nil
}

// DMS Backend: Instantiate NS
func (p *OSMPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    nsRequest := &NSInstantiateRequest{
        NSName:           req.Name,
        NSDId:            req.PackageID,
        VIMAccountId:     req.VIMAccount,
        AdditionalParams: req.Values,
    }

    nsInstanceID, err := p.client.InstantiateNS(ctx, nsRequest)
    if err != nil {
        return nil, fmt.Errorf("osm ns instantiate failed: %w", err)
    }

    return &dms.Deployment{
        ID:        nsInstanceID,
        Status:    "BUILDING",
        CreatedAt: time.Now(),
        Extensions: map[string]interface{}{
            "osm.nsInstanceId": nsInstanceID,
        },
    }, nil
}

// Get NS status
func (p *OSMPlugin) GetDeploymentStatus(ctx context.Context, id string) (*dms.DeploymentStatus, error) {
    nsInstance, err := p.client.GetNSInstance(ctx, id)
    if err != nil {
        return nil, err
    }

    return &dms.DeploymentStatus{
        DeploymentID: id,
        Status:       mapOSMStatus(nsInstance.OperationalStatus),
        Message:      nsInstance.DetailedStatus,
        UpdatedAt:    nsInstance.ModifyTime,
        Extensions: map[string]interface{}{
            "osm.operationalStatus": nsInstance.OperationalStatus,
            "osm.configStatus":      nsInstance.ConfigStatus,
            "osm.vnfInstances":      len(nsInstance.ConstituentVNFRIds),
        },
    }, nil
}

// Scale NS
func (p *OSMPlugin) ScaleDeployment(ctx context.Context, id string, replicas int) error {
    scaleRequest := &NSScaleRequest{
        ScaleType: "SCALE_VNF",
        ScaleVnfData: ScaleVnfData{
            ScaleVnfType: "SCALE_OUT",
        },
    }
    return p.client.ScaleNS(ctx, id, scaleRequest)
}
```

## Integration Points

### OSM NBI (Northbound Interface)

- Onboard NS/VNF descriptors
- Instantiate and terminate NS
- Scale VNF instances
- Query NS operational status

### OSM VIM Management

- Register VIM accounts (OpenStack, VMware, K8s)
- Update VIM credentials
- Monitor VIM connectivity

## See Also

- [SMO Adapter Interface](README.md)
- [ONAP Integration](onap.md)
