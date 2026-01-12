# Cloud IMS Adapters

**Status:** 📋 Specification
**Version:** 1.0
**Last Updated:** 2026-01-12

## Overview

Cloud IMS adapters provide O2-IMS infrastructure management for public cloud platforms (AWS, Azure, GCP). Each adapter maps cloud-native resources to O2-IMS constructs while preserving cloud-specific capabilities.

## Supported Cloud Providers

| Provider | Status | Node Groups | Compute Units | Instance Types |
|----------|--------|-------------|---------------|----------------|
| **AWS EKS** | 📋 Spec | Node Group | EC2 Instance | Instance Type |
| **Azure AKS** | 📋 Spec | Node Pool | Azure VM | VM SKU |
| **Google GKE** | 📋 Spec | Node Pool | GCE Instance | Machine Type |

---

## AWS EKS Adapter

### Resource Mappings

| O2-IMS Concept | AWS Resource | API |
|----------------|--------------|-----|
| **Deployment Manager** | EKS Cluster | EKS |
| **Resource Pool** | Node Group / Auto Scaling Group | EKS / EC2 Auto Scaling |
| **Resource** | EC2 Instance | EC2 |
| **Resource Type** | Instance Type | EC2 |

### Configuration

```yaml
plugins:
  ims:
    - name: aws-eks
      type: aws
      enabled: true
      config:
        region: us-west-2
        clusterName: my-eks-cluster
        accessKeyId: ${AWS_ACCESS_KEY_ID}
        secretAccessKey: ${AWS_SECRET_ACCESS_KEY}
        ocloudId: ocloud-aws-us-west-2
```

### Implementation

```go
package aws

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/eks"
    "github.com/aws/aws-sdk-go-v2/service/ec2"
)

type AWSAdapter struct {
    name        string
    version     string
    eksClient   *eks.Client
    ec2Client   *ec2.Client
    config      *Config
}

type Config struct {
    Region          string `yaml:"region"`
    ClusterName     string `yaml:"clusterName"`
    AccessKeyID     string `yaml:"accessKeyId"`
    SecretAccessKey string `yaml:"secretAccessKey"`
    OCloudID        string `yaml:"ocloudId"`
}

func (a *AWSAdapter) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    // List EKS node groups
    input := &eks.ListNodegroupsInput{
        ClusterName: aws.String(a.config.ClusterName),
    }

    result, err := a.eksClient.ListNodegroups(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to list node groups: %w", err)
    }

    pools := make([]*ims.ResourcePool, 0, len(result.Nodegroups))
    for _, ngName := range result.Nodegroups {
        ng, err := a.eksClient.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
            ClusterName:   aws.String(a.config.ClusterName),
            NodegroupName: aws.String(ngName),
        })
        if err != nil {
            continue
        }

        pool := a.transformNodeGroupToResourcePool(ng.Nodegroup)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

func (a *AWSAdapter) transformNodeGroupToResourcePool(ng *types.Nodegroup) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: *ng.NodegroupArn,
        Name:          *ng.NodegroupName,
        Description:   fmt.Sprintf("EKS node group: %s", *ng.NodegroupName),
        Location:      a.config.Region,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "aws.nodeGroupName":    *ng.NodegroupName,
            "aws.clusterName":      *ng.ClusterName,
            "aws.instanceTypes":    ng.InstanceTypes,
            "aws.scalingConfig":    ng.ScalingConfig,
            "aws.desiredSize":      *ng.ScalingConfig.DesiredSize,
            "aws.minSize":          *ng.ScalingConfig.MinSize,
            "aws.maxSize":          *ng.ScalingConfig.MaxSize,
            "aws.capacityType":     ng.CapacityType,
            "aws.amiType":          ng.AmiType,
        },
    }
}

func (a *AWSAdapter) ListResources(ctx context.Context, filter *ims.Filter) ([]*ims.Resource, error) {
    // Describe EC2 instances with cluster tag
    input := &ec2.DescribeInstancesInput{
        Filters: []types.Filter{
            {
                Name:   aws.String("tag:kubernetes.io/cluster/" + a.config.ClusterName),
                Values: []string{"owned"},
            },
            {
                Name:   aws.String("instance-state-name"),
                Values: []string{"running", "pending"},
            },
        },
    }

    result, err := a.ec2Client.DescribeInstances(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to describe instances: %w", err)
    }

    resources := make([]*ims.Resource, 0)
    for _, reservation := range result.Reservations {
        for _, instance := range reservation.Instances {
            resource := a.transformEC2InstanceToResource(&instance)
            if filter.Matches(resource) {
                resources = append(resources, resource)
            }
        }
    }

    return resources, nil
}
```

### Create Resource Pool

```go
func (a *AWSAdapter) CreateResourcePool(ctx context.Context, pool *ims.ResourcePool) (*ims.ResourcePool, error) {
    // Extract AWS-specific extensions
    instanceTypes := pool.Extensions["aws.instanceTypes"].([]string)
    desiredSize := int32(pool.Extensions["aws.desiredSize"].(float64))
    minSize := int32(pool.Extensions["aws.minSize"].(float64))
    maxSize := int32(pool.Extensions["aws.maxSize"].(float64))

    input := &eks.CreateNodegroupInput{
        ClusterName:   aws.String(a.config.ClusterName),
        NodegroupName: aws.String(pool.Name),
        InstanceTypes: instanceTypes,
        ScalingConfig: &types.NodegroupScalingConfig{
            DesiredSize: aws.Int32(desiredSize),
            MinSize:     aws.Int32(minSize),
            MaxSize:     aws.Int32(maxSize),
        },
        Subnets: []string{/* from extensions */},
        NodeRole: aws.String(/* IAM role ARN */),
    }

    result, err := a.eksClient.CreateNodegroup(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to create node group: %w", err)
    }

    return a.transformNodeGroupToResourcePool(result.Nodegroup), nil
}
```

---

## Azure AKS Adapter

### Resource Mappings

| O2-IMS Concept | Azure Resource | API |
|----------------|----------------|-----|
| **Deployment Manager** | AKS Cluster | AKS |
| **Resource Pool** | Agent Pool (Node Pool) | AKS |
| **Resource** | Virtual Machine | Compute |
| **Resource Type** | VM SKU | Compute |

### Configuration

```yaml
plugins:
  ims:
    - name: azure-aks
      type: azure
      enabled: true
      config:
        subscriptionId: ${AZURE_SUBSCRIPTION_ID}
        resourceGroup: my-resource-group
        clusterName: my-aks-cluster
        tenantId: ${AZURE_TENANT_ID}
        clientId: ${AZURE_CLIENT_ID}
        clientSecret: ${AZURE_CLIENT_SECRET}
        ocloudId: ocloud-azure-eastus
```

### Implementation

```go
package azure

import (
    "context"
    "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
    "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
)

type AzureAdapter struct {
    name           string
    version        string
    aksClient      *armcontainerservice.ManagedClustersClient
    agentPoolClient *armcontainerservice.AgentPoolsClient
    config         *Config
}

func (a *AzureAdapter) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    pager := a.agentPoolClient.NewListPager(
        a.config.ResourceGroup,
        a.config.ClusterName,
        nil,
    )

    pools := make([]*ims.ResourcePool, 0)
    for pager.More() {
        page, err := pager.NextPage(ctx)
        if err != nil {
            return nil, fmt.Errorf("failed to list agent pools: %w", err)
        }

        for _, agentPool := range page.Value {
            pool := a.transformAgentPoolToResourcePool(agentPool)
            if filter.Matches(pool) {
                pools = append(pools, pool)
            }
        }
    }

    return pools, nil
}

func (a *AzureAdapter) transformAgentPoolToResourcePool(ap *armcontainerservice.AgentPool) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: *ap.ID,
        Name:          *ap.Name,
        Description:   fmt.Sprintf("AKS agent pool: %s", *ap.Name),
        Location:      a.config.Region,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "azure.agentPoolName":    *ap.Name,
            "azure.vmSize":           *ap.Properties.VMSize,
            "azure.count":            *ap.Properties.Count,
            "azure.minCount":         ap.Properties.MinCount,
            "azure.maxCount":         ap.Properties.MaxCount,
            "azure.enableAutoScaling": *ap.Properties.EnableAutoScaling,
            "azure.mode":             ap.Properties.Mode,
            "azure.orchestratorVersion": *ap.Properties.OrchestratorVersion,
        },
    }
}
```

---

## Google GKE Adapter

### Resource Mappings

| O2-IMS Concept | GCP Resource | API |
|----------------|--------------|-----|
| **Deployment Manager** | GKE Cluster | GKE |
| **Resource Pool** | Node Pool | GKE |
| **Resource** | Compute Engine Instance | GCE |
| **Resource Type** | Machine Type | GCE |

### Configuration

```yaml
plugins:
  ims:
    - name: gcp-gke
      type: gcp
      enabled: true
      config:
        projectId: my-gcp-project
        region: us-central1
        clusterName: my-gke-cluster
        credentialsFile: /etc/gcp/credentials.json
        ocloudId: ocloud-gcp-us-central1
```

### Implementation

```go
package gcp

import (
    "context"
    container "cloud.google.com/go/container/apiv1"
    "cloud.google.com/go/container/apiv1/containerpb"
)

type GCPAdapter struct {
    name         string
    version      string
    gkeClient    *container.ClusterManagerClient
    config       *Config
}

func (a *GCPAdapter) ListResourcePools(ctx context.Context, filter *ims.Filter) ([]*ims.ResourcePool, error) {
    parent := fmt.Sprintf("projects/%s/locations/%s/clusters/%s",
        a.config.ProjectID, a.config.Region, a.config.ClusterName)

    req := &containerpb.ListNodePoolsRequest{
        Parent: parent,
    }

    resp, err := a.gkeClient.ListNodePools(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to list node pools: %w", err)
    }

    pools := make([]*ims.ResourcePool, 0, len(resp.NodePools))
    for _, np := range resp.NodePools {
        pool := a.transformNodePoolToResourcePool(np)
        if filter.Matches(pool) {
            pools = append(pools, pool)
        }
    }

    return pools, nil
}

func (a *GCPAdapter) transformNodePoolToResourcePool(np *containerpb.NodePool) *ims.ResourcePool {
    return &ims.ResourcePool{
        ResourcePoolID: np.SelfLink,
        Name:          np.Name,
        Description:   fmt.Sprintf("GKE node pool: %s", np.Name),
        Location:      a.config.Region,
        OCloudID:      a.config.OCloudID,
        Extensions: map[string]interface{}{
            "gcp.nodePoolName":      np.Name,
            "gcp.machineType":       np.Config.MachineType,
            "gcp.initialNodeCount":  np.InitialNodeCount,
            "gcp.autoscaling":       np.Autoscaling,
            "gcp.minNodeCount":      np.Autoscaling.MinNodeCount,
            "gcp.maxNodeCount":      np.Autoscaling.MaxNodeCount,
            "gcp.diskSizeGb":        np.Config.DiskSizeGb,
            "gcp.diskType":          np.Config.DiskType,
            "gcp.preemptible":       np.Config.Preemptible,
        },
    }
}
```

## Testing

```go
func TestCloudAdapters_ListResourcePools(t *testing.T) {
    tests := []struct {
        name     string
        adapter  string
        wantPools int
    }{
        {"AWS EKS", "aws", 2},
        {"Azure AKS", "azure", 3},
        {"GCP GKE", "gcp", 2},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            adapter := setupMockCloudAdapter(t, tt.adapter)
            pools, err := adapter.ListResourcePools(context.Background(), &ims.Filter{})

            require.NoError(t, err)
            assert.Len(t, pools, tt.wantPools)
        })
    }
}
```

## Performance

- **AWS**: Use pagination for large node groups, cache EC2 instance types
- **Azure**: Use paging for agent pools, batch resource queries
- **GCP**: Leverage GKE's built-in caching, use regional endpoints

## Security

- **AWS**: Use IAM roles for EKS, never hardcode credentials
- **Azure**: Use Managed Identity, store secrets in Key Vault
- **GCP**: Use Workload Identity, store credentials in Secret Manager

## See Also

- [IMS Adapter Interface](README.md)
- [Kubernetes Adapter](kubernetes.md)
- [Bare-Metal Adapters](bare-metal.md)
