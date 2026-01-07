package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	autoscalingTypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResourcePools retrieves all resource pools matching the provided filter.
// In "az" mode, it lists Availability Zones.
// In "asg" mode, it lists Auto Scaling Groups.
func (a *AWSAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) (pools []*adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "ListResourcePools", start, err) }()

	a.logger.Debug("ListResourcePools called",
		zap.Any("filter", filter),
		zap.String("poolMode", a.poolMode))

	if a.poolMode == "asg" {
		return a.listASGPools(ctx, filter)
	}
	return a.listAZPools(ctx, filter)
}

// listAZPools lists Availability Zones as resource pools.
func (a *AWSAdapter) listAZPools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	// Get availability zones
	azsOutput, err := a.ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2Types.Filter{
			{
				Name:   aws.String("region-name"),
				Values: []string{a.region},
			},
			{
				Name:   aws.String("state"),
				Values: []string{"available"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe availability zones: %w", err)
	}

	pools := make([]*adapter.ResourcePool, 0, len(azsOutput.AvailabilityZones))
	for _, az := range azsOutput.AvailabilityZones {
		poolID := generateAZPoolID(aws.ToString(az.ZoneName))
		location := aws.ToString(az.ZoneName)

		// Apply filter using shared helper
		if !adapter.MatchesFilter(filter, poolID, "", location, nil) {
			continue
		}

		pool := &adapter.ResourcePool{
			ResourcePoolID: poolID,
			Name:           aws.ToString(az.ZoneName),
			Description:    fmt.Sprintf("AWS Availability Zone %s", aws.ToString(az.ZoneName)),
			Location:       location,
			OCloudID:       a.oCloudID,
			Extensions: map[string]interface{}{
				"aws.zoneId":   aws.ToString(az.ZoneId),
				"aws.zoneType": aws.ToString(az.ZoneType),
				"aws.region":   aws.ToString(az.RegionName),
				"aws.state":    string(az.State),
			},
		}

		pools = append(pools, pool)
	}

	// Apply pagination using shared helper
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (AZ mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// listASGPools lists Auto Scaling Groups as resource pools.
func (a *AWSAdapter) listASGPools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	// Get Auto Scaling Groups
	var pools []*adapter.ResourcePool
	paginator := autoscaling.NewDescribeAutoScalingGroupsPaginator(a.asgClient, &autoscaling.DescribeAutoScalingGroupsInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe auto scaling groups: %w", err)
		}

		for _, asg := range page.AutoScalingGroups {
			poolID := generateASGPoolID(aws.ToString(asg.AutoScalingGroupName))

			// Get first AZ as location
			var location string
			if len(asg.AvailabilityZones) > 0 {
				location = asg.AvailabilityZones[0]
			}

			// Convert tags to labels
			labels := make(map[string]string)
			for _, tag := range asg.Tags {
				labels[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}

			// Apply filter using shared helper
			if !adapter.MatchesFilter(filter, poolID, "", location, labels) {
				continue
			}

			pool := &adapter.ResourcePool{
				ResourcePoolID: poolID,
				Name:           aws.ToString(asg.AutoScalingGroupName),
				Description:    fmt.Sprintf("AWS Auto Scaling Group %s", aws.ToString(asg.AutoScalingGroupName)),
				Location:       location,
				OCloudID:       a.oCloudID,
				Extensions: map[string]interface{}{
					"aws.asgArn":              aws.ToString(asg.AutoScalingGroupARN),
					"aws.desiredCapacity":     aws.ToInt32(asg.DesiredCapacity),
					"aws.minSize":             aws.ToInt32(asg.MinSize),
					"aws.maxSize":             aws.ToInt32(asg.MaxSize),
					"aws.availabilityZones":   asg.AvailabilityZones,
					"aws.launchTemplate":      getLaunchTemplateName(asg.LaunchTemplate),
					"aws.healthCheckType":     aws.ToString(asg.HealthCheckType),
					"aws.status":              aws.ToString(asg.Status),
					"aws.createdTime":         asg.CreatedTime,
					"aws.defaultCooldown":     aws.ToInt32(asg.DefaultCooldown),
					"aws.terminationPolicies": asg.TerminationPolicies,
				},
			}

			pools = append(pools, pool)
		}
	}

	// Apply pagination using shared helper
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools (ASG mode)",
		zap.Int("count", len(pools)))

	return pools, nil
}

// GetResourcePool retrieves a specific resource pool by ID.
func (a *AWSAdapter) GetResourcePool(ctx context.Context, id string) (pool *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "GetResourcePool", start, err) }()

	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

	if a.poolMode == "asg" {
		return a.getASGPool(ctx, id)
	}
	return a.getAZPool(ctx, id)
}

// getAZPool retrieves an Availability Zone as a resource pool.
func (a *AWSAdapter) getAZPool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	// Extract zone name from pool ID
	pools, err := a.listAZPools(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pool := range pools {
		if pool.ResourcePoolID == id {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("resource pool not found: %s", id)
}

// getASGPool retrieves an Auto Scaling Group as a resource pool.
func (a *AWSAdapter) getASGPool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	pools, err := a.listASGPools(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pool := range pools {
		if pool.ResourcePoolID == id {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("resource pool not found: %s", id)
}

// CreateResourcePool creates a new resource pool.
// In "az" mode, this operation is not supported (AZs are AWS-managed).
// In "asg" mode, this creates a new Auto Scaling Group.
func (a *AWSAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (created *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "CreateResourcePool", start, err) }()

	a.logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	if a.poolMode == "az" {
		return nil, fmt.Errorf("cannot create resource pools in 'az' mode: availability zones are AWS-managed")
	}

	// In ASG mode, we would create an Auto Scaling Group
	// This requires additional configuration not available in the O2-IMS model
	return nil, fmt.Errorf("creating Auto Scaling Groups requires additional configuration: use AWS console or CLI")
}

// UpdateResourcePool updates an existing resource pool.
// In "az" mode, this operation is not supported.
// In "asg" mode, this could update ASG capacity settings.
func (a *AWSAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (updated *adapter.ResourcePool, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "UpdateResourcePool", start, err) }()

	a.logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	if a.poolMode == "az" {
		return nil, fmt.Errorf("cannot update resource pools in 'az' mode: availability zones are AWS-managed")
	}

	// In ASG mode, we could update the ASG capacity
	// For now, return not implemented
	return nil, fmt.Errorf("updating Auto Scaling Groups is not yet implemented")
}

// DeleteResourcePool deletes a resource pool by ID.
// In "az" mode, this operation is not supported.
// In "asg" mode, this could delete an Auto Scaling Group.
func (a *AWSAdapter) DeleteResourcePool(ctx context.Context, id string) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "DeleteResourcePool", start, err) }()

	a.logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	if a.poolMode == "az" {
		return fmt.Errorf("cannot delete resource pools in 'az' mode: availability zones are AWS-managed")
	}

	// In ASG mode, we could delete the ASG
	// This is a destructive operation and requires careful handling
	return fmt.Errorf("deleting Auto Scaling Groups is not yet implemented")
}

// getLaunchTemplateName extracts the launch template name from LaunchTemplateSpecification.
func getLaunchTemplateName(lt *autoscalingTypes.LaunchTemplateSpecification) string {
	if lt == nil {
		return ""
	}
	return aws.ToString(lt.LaunchTemplateName)
}
