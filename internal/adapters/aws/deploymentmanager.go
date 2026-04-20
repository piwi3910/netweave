package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// GetDeploymentManager retrieves metadata about the AWS deployment manager.
// It queries the AWS region information to construct the deployment manager metadata.
func (a *Adapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	// Accept "default" and "" as aliases for the configured DM ID,
	// matching the behavior used by routes.go and handlers.
	if id != a.deploymentManagerID && id != "default" && id != "" {
		return nil, fmt.Errorf("deployment manager %s: %w", id, adapter.ErrDeploymentManagerNotFound)
	}

	// Query AWS region information
	regionsOutput, err := a.ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
		RegionNames: []string{a.region},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe regions: %w", err)
	}

	if len(regionsOutput.Regions) == 0 {
		return nil, fmt.Errorf("region not found: %s", a.region)
	}

	currentRegion := regionsOutput.Regions[0]

	// Get availability zones for this region
	azsOutput, err := a.ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2Types.Filter{
			{
				Name:   aws.String("region-name"),
				Values: []string{a.region},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe availability zones: %w", err)
	}

	// Collect availability zone names as supported locations
	supportedLocations := make([]string, 0, len(azsOutput.AvailabilityZones))
	for _, az := range azsOutput.AvailabilityZones {
		supportedLocations = append(supportedLocations, aws.ToString(az.ZoneName))
	}

	// Construct deployment manager metadata
	dm := &adapter.DeploymentManager{
		DeploymentManagerID: a.deploymentManagerID,
		Name:                fmt.Sprintf("AWS %s", a.region),
		Description:         fmt.Sprintf("AWS cloud deployment in region %s", a.region),
		OCloudID:            a.oCloudID,
		ServiceURI:          fmt.Sprintf("https://ec2.%s.amazonaws.com", a.region),
		SupportedLocations:  supportedLocations,
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
		Extensions: map[string]interface{}{
			"aws.Region":         a.region,
			"aws.RegionEndpoint": aws.ToString(currentRegion.Endpoint),
			"aws.PoolMode":       a.poolMode,
			"aws.optInStatus":    aws.ToString(currentRegion.OptInStatus),
		},
	}

	a.logger.Info("retrieved deployment manager",
		zap.String("deployment_manager_id", dm.DeploymentManagerID),
		zap.String("region", a.region),
		zap.Int("supported_locations", len(supportedLocations)))

	return dm, nil
}

// ListDeploymentManagers retrieves all deployment managers.
// AWS has a single deployment manager per adapter instance.
func (a *Adapter) ListDeploymentManagers(ctx context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	a.logger.Debug("ListDeploymentManagers called")

	dm, err := a.GetDeploymentManager(ctx, a.deploymentManagerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment managers: %w", err)
	}

	return []*adapter.DeploymentManager{dm}, nil
}
