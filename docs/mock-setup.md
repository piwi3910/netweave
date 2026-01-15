# Mock Backend Setup for Local Development and E2E Testing

This document describes how to use the mock backends for O2-IMS, O2-DMS, and SMO to develop and test the netweave gateway without requiring real infrastructure.

## Overview

The netweave gateway includes comprehensive mock implementations for all three backend types:

- **Mock IMS Adapter** (`internal/adapters/mock`): Simulates O2-IMS infrastructure inventory
- **Mock DMS Adapter** (`internal/dms/adapters/mock`): Simulates O2-DMS deployment management
- **Mock SMO Plugin** (`internal/smo/adapters/mock`): Simulates SMO integration and workflows

These mocks provide realistic behavior including:
- Pre-populated sample data
- Async operations simulation (deployments, workflows)
- Full API compliance with O-RAN specifications
- Deterministic behavior for testing

## Quick Start

### 1. Start with Mock Configuration

```bash
# Use the provided mock configuration
./netweave-gateway --config config/mock.yaml
```

The mock configuration (`config/mock.yaml`) automatically:
- Enables all three mock backends
- Pre-populates realistic sample data
- Configures appropriate timeouts and simulation delays
- Disables authentication for easy testing

### 2. Verify Mock Data

Once started, you can verify the mock data is available:

```bash
# Check IMS resource pools
curl http://localhost:8080/o2ims/v1/resourcePools

# Check DMS deployment packages
curl http://localhost:8080/o2dms/v1/deploymentPackages

# Check SMO service models (via GraphQL)
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ serviceModels { id name version } }"}'
```

## Mock IMS Adapter

### Sample Data

The mock IMS adapter includes:

- **4 Resource Types:**
  - CPU: Intel Xeon Gold 6248R (48 cores, 96 threads)
  - GPU: NVIDIA A100 (40GB, 6912 CUDA cores)
  - Memory: Samsung DDR4 ECC (512GB, 3200 MHz)
  - Storage: Samsung PM9A3 NVMe (3.84TB)

- **3 Resource Pools:**
  - `pool-us-east-1`: US East Compute Pool (5 CPU servers)
  - `pool-us-west-2`: US West GPU Pool (8 GPU nodes)
  - `pool-eu-central-1`: EU Central Pool (3 CPU + 3 Storage)

- **19 Resources:** Distributed across the three pools with realistic metadata

### API Examples

```bash
# List all resource pools
curl http://localhost:8080/o2ims/v1/resourcePools

# Get specific resource pool
curl http://localhost:8080/o2ims/v1/resourcePools/pool-us-east-1

# List resources in a pool
curl http://localhost:8080/o2ims/v1/resources?resourcePoolId=pool-us-west-2

# List all resource types
curl http://localhost:8080/o2ims/v1/resourceTypes
```

## Mock DMS Adapter

### Sample Data

The mock DMS adapter includes:

- **5 Deployment Packages:**
  - `oran-cuup` v1.2.0: O-RAN CU-UP network function
  - `oran-cucp` v1.1.5: O-RAN CU-CP network function
  - `oran-du` v2.0.1: O-RAN Distributed Unit
  - `5g-upf` v1.5.2: 5G User Plane Function
  - `5g-smf` v2.1.0: 5G Session Management Function

### Deployment Simulation

The mock DMS simulates realistic deployment lifecycle:

1. **Pending** (0s): Deployment queued
2. **Deploying** (2s): Simulated installation
3. **Deployed** (complete): Deployment successful

Status progression happens automatically in the background.

### API Examples

```bash
# List deployment packages
curl http://localhost:8080/o2dms/v1/deploymentPackages

# Create a new deployment
curl -X POST http://localhost:8080/o2dms/v1/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-cuup-deployment",
    "packageId": "pkg-cuup-001",
    "namespace": "ran-ns",
    "values": {
      "replicas": 3,
      "resources": {
        "cpu": "2",
        "memory": "4Gi"
      }
    }
  }'

# Wait 2 seconds, then check deployment status
sleep 2
curl http://localhost:8080/o2dms/v1/deployments/{deployment-id}

# Rollback a deployment
curl -X POST http://localhost:8080/o2dms/v1/deployments/{deployment-id}/rollback \
  -H "Content-Type: application/json" \
  -d '{"targetVersion": 1}'
```

## Mock SMO Plugin

### Sample Data

The mock SMO plugin includes:

- **3 Service Models:**
  - `sm-5g-ran-001`: 5G RAN Service (CU-UP, CU-CP, DU)
  - `sm-5g-core-001`: 5G Core Service (AMF, SMF, UPF)
  - `sm-mec-001`: MEC Application Service

### Workflow Simulation

The mock SMO simulates realistic workflow execution:

1. **Pending** (0s, 0%): Workflow queued
2. **Running** (1s, 25%): Planning workflow steps
3. **Running** (2s, 50%): Executing deployment
4. **Running** (3s, 75%): Validating deployment
5. **Completed** (4s, 100%): Workflow successful

### API Examples

```bash
# List service models
curl http://localhost:8080/smo/v1/serviceModels

# Execute a workflow
curl -X POST http://localhost:8080/smo/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "workflowName": "deploy-5g-ran",
    "parameters": {
      "serviceModelId": "sm-5g-ran-001",
      "location": "us-east-1",
      "cells": 4
    }
  }'

# Check workflow status (immediately - will be pending)
curl http://localhost:8080/smo/v1/workflows/{execution-id}/status

# Wait 4 seconds for completion
sleep 4
curl http://localhost:8080/smo/v1/workflows/{execution-id}/status
```

## Using GraphQL with Mocks

The mock backends are fully compatible with the GraphQL API:

```graphql
query {
  # IMS Resources
  resourcePools(filter: { location: "us-east-1" }) {
    edges {
      node {
        resourcePoolId
        name
        location
      }
    }
  }

  # DMS Deployments
  deployments(filter: { namespace: "ran-ns" }) {
    edges {
      node {
        id
        name
        status
        packageId
      }
    }
  }
}
```

## E2E Testing with Mocks

### Test Structure

The mock backends are designed for E2E testing:

```go
func TestE2E_DeploymentLifecycle(t *testing.T) {
    // Start server with mock configuration
    srv := setupTestServer(t, "config/mock.yaml")
    defer srv.Shutdown()

    // Create deployment using mock DMS
    deployment := createDeployment(t, srv, DeploymentRequest{
        Name:      "test-deployment",
        PackageID: "pkg-cuup-001",
        Namespace: "test-ns",
    })

    // Wait for deployment to complete (mock simulates 2s delay)
    time.Sleep(3 * time.Second)

    // Verify deployment status
    status := getDeploymentStatus(t, srv, deployment.ID)
    assert.Equal(t, "deployed", status.Status)
}
```

### CI Pipeline Integration

The mocks require no external dependencies:

```yaml
# .github/workflows/e2e-tests.yml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Run E2E Tests with Mocks
        run: |
          make test-e2e-mock

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: e2e-test-results
          path: test-results/
```

## Configuration Reference

### Mock IMS Configuration

```yaml
o2ims:
  adapter_type: mock
  mock:
    populate_sample_data: true
    sample_data:
      resource_pools: 3
      resources: 19
      resource_types: 4
```

### Mock DMS Configuration

```yaml
o2dms:
  adapter_type: mock
  mock:
    populate_sample_data: true
    sample_data:
      packages: 5
    simulation:
      deployment_delay: 2s
      rollback_delay: 2s
      delete_delay: 1s
```

### Mock SMO Configuration

```yaml
smo:
  plugin_type: mock
  mock:
    populate_sample_data: true
    sample_data:
      service_models: 3
    simulation:
      workflow_delay: 4s
```

## Limitations

While comprehensive, the mock backends have some limitations:

1. **No Persistence**: All data is in-memory and lost on restart
2. **Simplified Validation**: Some edge cases may not be validated
3. **Fixed Sample Data**: Sample data structure is predetermined
4. **Timing**: Simulation delays are fixed (not configurable per-operation)

For production testing, use real backends (Kubernetes, Helm, ONAP/OSM).

## Troubleshooting

### Mock Not Loading Sample Data

**Problem**: API returns empty lists

**Solution**: Check configuration has `populate_sample_data: true`

### Deployment Stuck in Pending

**Problem**: Deployment never progresses to "deployed"

**Solution**: Wait at least 2 seconds for simulation to complete. Check logs for errors.

### Workflow Never Completes

**Problem**: Workflow status stays in "running"

**Solution**: Wait at least 4 seconds for full workflow simulation. Check if workflow was cancelled.

## Next Steps

- **Add Custom Mock Data**: Modify mock adapters to include your specific test scenarios
- **Extend E2E Tests**: Create comprehensive test suites using the mock backends
- **Performance Testing**: Use mocks for load testing the gateway itself
- **Integration Tests**: Test API compliance without infrastructure dependencies

For real infrastructure testing, see:
- [Kubernetes Adapter Setup](kubernetes-adapter.md)
- [Helm Adapter Setup](helm-adapter.md)
- [ONAP Integration](onap-integration.md)
