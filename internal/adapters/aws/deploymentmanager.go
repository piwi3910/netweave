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
	a.Logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	if id != a.DeploymentManagerID {
		return nil, fmt.Errorf("deployment manager not found: %s", id)
	}

	// Query AWS region information
	regionsOutput, err := a.ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
		RegionNames: []string{a.Region},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe regions: %w", err)
	}

	if len(regionsOutput.Regions) == 0 {
		return nil, fmt.Errorf("region not found: %s", a.Region)
	}

	currentRegion := regionsOutput.Regions[0]

	// Get availability zones for this region
	azsOutput, err := a.ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2Types.Filter{
			{
				Name:   aws.String("region-name"),
				Values: []string{a.Region},
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
		DeploymentManagerID: a.DeploymentManagerID,
		Name:                fmt.Sprintf("AWS %s", a.Region),
		Description:         fmt.Sprintf("AWS cloud deployment in region %s", a.Region),
		OCloudID:            a.OCloudID,
		ServiceURI:          fmt.Sprintf("https://ec2.%s.amazonaws.com", a.Region),
		SupportedLocations:  supportedLocations,
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
		Extensions: map[string]interface{}{
			"aws.Region":         a.Region,
			"aws.RegionEndpoint": aws.ToString(currentRegion.Endpoint),
			"aws.PoolMode":       a.PoolMode,
			"aws.optInStatus":    aws.ToString(currentRegion.OptInStatus),
		},
	}

	a.Logger.Info("retrieved deployment manager",
		zap.String("deploymentManagerID", dm.DeploymentManagerID),
		zap.String("region", a.Region),
		zap.Int("supportedLocations", len(supportedLocations)))

	return dm, nil
}
