# Orchestrator DMS Adapters

**Status:** 📋 Specification Complete  
**Version:** 1.0  
**Last Updated:** 2026-01-12

## Overview

Orchestrator adapters integrate with telecom-specific lifecycle management systems including ONAP and OSM.

## ONAP-LCM Adapter

### Configuration

```yaml
plugins:
  dms:
    - name: onap-lcm
      type: onap-dms
      enabled: true
      config:
        soUrl: https://onap-so.example.com:8080
        sdncUrl: https://onap-sdnc.example.com:8282
        username: ${ONAP_USERNAME}
        password: ${ONAP_PASSWORD}
```

### Implementation

```go
package onapdms

type ONAPDMSPlugin struct {
    name       string
    soClient   *ServiceOrchestratorClient
    sdncClient *SDNCClient
}

func (p *ONAPDMSPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
    soRequest := &ServiceInstanceRequest{
        RequestDetails: RequestDetails{
            ModelInfo: ModelInfo{
                ModelInvariantId: req.ServiceModelID,
                ModelVersionId:   req.ServiceModelVersion,
            },
            CloudConfiguration: CloudConfiguration{
                TenantID:      req.TenantID,
                CloudRegionID: req.CloudRegion,
            },
        },
    }

    response, err := p.soClient.CreateServiceInstance(ctx, soRequest)
    if err != nil {
        return nil, fmt.Errorf("onap so create failed: %w", err)
    }

    return &dms.Deployment{
        ID:        response.ServiceInstanceID,
        Status:    mapONAPStatus(response.RequestState),
        CreatedAt: time.Now(),
        Extensions: map[string]interface{}{
            "onap.serviceInstanceId": response.ServiceInstanceID,
            "onap.requestId":         response.RequestID,
        },
    }, nil
}
```

## OSM-LCM Adapter

### Configuration

```yaml
plugins:
  dms:
    - name: osm-lcm
      type: osm-dms
      enabled: true
      config:
        nbiUrl: https://osm.example.com:9999
        username: admin
        password: ${OSM_PASSWORD}
        project: admin
```

### Implementation

```go
package osmdms

type OSMDMSPlugin struct {
    name   string
    client *OSMClient
}

func (p *OSMDMSPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
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
```

## See Also

- [DMS Adapter Interface](README.md)
- [SMO Adapters](../smo/README.md)
