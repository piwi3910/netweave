# ONAP SMO Adapter

**Status:** 📋 Specification Complete  
**Version:** 1.0  
**Last Updated:** 2026-01-12

## Overview

The ONAP adapter integrates netweave with ONAP (Open Network Automation Platform) for end-to-end service orchestration. It operates in dual mode: northbound inventory sync and DMS backend orchestration.

## Configuration

```yaml
plugins:
  smo:
    - name: onap-integration
      type: onap
      enabled: true
      config:
        # Northbound Configuration
        aaiUrl: https://onap-aai.example.com:8443
        dmaapUrl: https://onap-dmaap.example.com:3904

        # DMS Backend Configuration
        soUrl: https://onap-so.example.com:8080
        sdncUrl: https://onap-sdnc.example.com:8282

        # Authentication
        username: aai@aai.onap.org
        password: ${ONAP_PASSWORD}

        # Settings
        inventorySyncInterval: 5m
        eventPublishBatchSize: 100
```

## Implementation

```go
package onap

import (
    "context"
    "github.com/yourorg/netweave/internal/plugin/smo"
    "github.com/yourorg/netweave/internal/plugin/dms"
)

type ONAPPlugin struct {
    name    string
    version string
    config  *Config

    // Northbound clients
    aaiClient   *AAIClient
    dmaapClient *DMaaPClient

    // DMS backend clients
    soClient   *ServiceOrchestratorClient
    sdncClient *SDNCClient
}

// Northbound: Sync to A&AI
func (p *ONAPPlugin) SyncInfrastructureInventory(ctx context.Context, inventory *smo.InfrastructureInventory) error {
    aaiInventory := p.transformToAAIInventory(inventory)

    for _, cloudRegion := range aaiInventory.CloudRegions {
        if err := p.aaiClient.CreateOrUpdateCloudRegion(ctx, cloudRegion); err != nil {
            return fmt.Errorf("failed to sync cloud region: %w", err)
        }
    }

    for _, pnf := range aaiInventory.PNFs {
        if err := p.aaiClient.CreateOrUpdatePNF(ctx, pnf); err != nil {
            return fmt.Errorf("failed to sync PNF: %w", err)
        }
    }

    return nil
}

// Northbound: Publish to DMaaP
func (p *ONAPPlugin) PublishInfrastructureEvent(ctx context.Context, event *smo.InfrastructureEvent) error {
    vesEvent := p.transformToVESEvent(event)
    topic := "unauthenticated.VES_INFRASTRUCTURE_EVENTS"
    return p.dmaapClient.PublishEvent(ctx, topic, vesEvent)
}

// DMS Backend: Create service via SO
func (p *ONAPPlugin) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
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

// Workflow: Execute Camunda BPMN
func (p *ONAPPlugin) ExecuteWorkflow(ctx context.Context, workflow *smo.WorkflowRequest) (*smo.WorkflowExecution, error) {
    execution, err := p.soClient.ExecuteWorkflow(ctx, workflow.WorkflowName, workflow.Parameters)
    if err != nil {
        return nil, err
    }

    return &smo.WorkflowExecution{
        ExecutionID:  execution.ProcessInstanceID,
        WorkflowName: workflow.WorkflowName,
        Status:       "RUNNING",
        StartedAt:    time.Now(),
        Extensions: map[string]interface{}{
            "onap.processInstanceId": execution.ProcessInstanceID,
            "onap.engineName":        "camunda",
        },
    }, nil
}
```

## Integration Points

### A&AI (Active and Available Inventory)

- Sync infrastructure resources as Cloud Regions, Tenants
- Register physical servers as PNFs (Physical Network Functions)
- Update resource relationships

### DMaaP (Data Movement as a Platform)

- Publish infrastructure events in VES (Virtual Event Streaming) format
- Subscribe to service orchestration events
- Message routing and filtering

### SO (Service Orchestrator)

- Trigger service instance creation
- Monitor orchestration workflows
- Execute BPMN workflows via Camunda

### SDNC (Software Defined Network Controller)

- Configure network elements
- Pre/post deployment configuration
- Network service activation

## See Also

- [SMO Adapter Interface](README.md)
- [OSM Integration](osm.md)
