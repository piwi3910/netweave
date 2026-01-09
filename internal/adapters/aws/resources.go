package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResources retrieves all resources (EC2 instances) matching the provided filter.
func (a *AWSAdapter) ListResources(ctx context.Context, filter *adapter.Filter) (resources []*adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "ListResources", start, err) }()

	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// Build EC2 filters
	var ec2Filters []ec2Types.Filter

	// Filter by availability zone if location is specified
	if filter != nil && filter.Location != "" {
		ec2Filters = append(ec2Filters, ec2Types.Filter{
			Name:   aws.String("availability-zone"),
			Values: []string{filter.Location},
		})
	}

	// Only get running instances by default
	ec2Filters = append(ec2Filters, ec2Types.Filter{
		Name:   aws.String("instance-state-name"),
		Values: []string{"running", "pending", "stopping", "stopped"},
	})

	// Get EC2 instances
	paginator := ec2.NewDescribeInstancesPaginator(a.ec2Client, &ec2.DescribeInstancesInput{
		Filters: ec2Filters,
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe instances: %w", err)
		}

		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				resource := a.instanceToResource(&instance)

				// Apply additional filters using shared helper
				labels := tagsToMap(instance.Tags)
				if !adapter.MatchesFilter(filter, resource.ResourcePoolID, resource.ResourceTypeID, extractTagValue(instance.Tags, "Location"), labels) {
					continue
				}

				resources = append(resources, resource)
			}
		}
	}

	// Apply pagination using shared helper
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific resource (EC2 instance) by ID.
func (a *AWSAdapter) GetResource(ctx context.Context, id string) (resource *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "GetResource", start, err) }()

	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Extract the actual EC2 instance ID from the O2-IMS resource ID
	instanceID := strings.TrimPrefix(id, "aws-instance-")
	if instanceID == id {
		// ID doesn't have the prefix, assume it's the raw instance ID
		instanceID = id
	}

	output, err := a.ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance: %w", err)
	}

	if len(output.Reservations) == 0 || len(output.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("resource not found: %s", id)
	}

	resource = a.instanceToResource(&output.Reservations[0].Instances[0])

	a.logger.Info("retrieved resource",
		zap.String("resourceId", resource.ResourceID))

	return resource, nil
}

// CreateResource creates a new resource (launches an EC2 instance).
func (a *AWSAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (created *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "CreateResource", start, err) }()

	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeId", resource.ResourceTypeID))

	// Extract instance type and validate required parameters
	instanceType := extractInstanceType(resource.ResourceTypeID)
	amiID, err := getRequiredAMI(resource.Extensions)
	if err != nil {
		return nil, err
	}

	// Build run instance input
	input := buildRunInstanceInput(resource, instanceType, amiID)

	// Launch the instance
	output, err := a.ec2Client.RunInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to launch instance: %w", err)
	}

	if len(output.Instances) == 0 {
		return nil, fmt.Errorf("no instance was launched")
	}

	created = a.instanceToResource(&output.Instances[0])

	a.logger.Info("created resource",
		zap.String("resourceId", created.ResourceID),
		zap.String("instanceId", aws.ToString(output.Instances[0].InstanceId)))

	return created, nil
}

// UpdateResource updates an existing EC2 instance's tags and metadata.
// Note: Core instance properties (instance type, AMI) cannot be modified after launch.
func (a *AWSAdapter) UpdateResource(ctx context.Context, id string, resource *adapter.Resource) (updated *adapter.Resource, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "UpdateResource", start, err) }()

	a.logger.Debug("UpdateResource called",
		zap.String("resourceId", id))

	// TODO: Implement instance tag updates via EC2 CreateTags API
	// For now, return not supported
	return nil, fmt.Errorf("updating EC2 instances is not yet implemented")
}

// extractInstanceType extracts the instance type from the resource type ID.
func extractInstanceType(resourceTypeID string) string {
	instanceType := strings.TrimPrefix(resourceTypeID, "aws-instance-type-")
	if instanceType == resourceTypeID {
		return resourceTypeID
	}
	return instanceType
}

// getRequiredAMI extracts and validates the required AMI ID from extensions.
func getRequiredAMI(extensions map[string]interface{}) (string, error) {
	if extensions == nil {
		return "", fmt.Errorf("aws.imageId is required in extensions")
	}

	amiID, ok := extensions["aws.imageId"].(string)
	if !ok || amiID == "" {
		return "", fmt.Errorf("aws.imageId is required in extensions")
	}

	return amiID, nil
}

// buildRunInstanceInput builds the EC2 RunInstances input from resource parameters.
func buildRunInstanceInput(resource *adapter.Resource, instanceType, amiID string) *ec2.RunInstancesInput {
	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: ec2Types.InstanceType(instanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
	}

	if resource.Extensions != nil {
		applyOptionalParameters(input, resource.Extensions)
	}

	if resource.Description != "" {
		applyNameTag(input, resource.Description)
	}

	return input
}

// applyOptionalParameters applies optional parameters from extensions to the run input.
func applyOptionalParameters(input *ec2.RunInstancesInput, extensions map[string]interface{}) {
	if subnet, ok := extensions["aws.subnetId"].(string); ok && subnet != "" {
		input.SubnetId = aws.String(subnet)
	}

	if sgs, ok := extensions["aws.securityGroupIds"].([]string); ok && len(sgs) > 0 {
		input.SecurityGroupIds = sgs
	}

	if key, ok := extensions["aws.keyName"].(string); ok && key != "" {
		input.KeyName = aws.String(key)
	}
}

// applyNameTag adds a Name tag to the instance.
func applyNameTag(input *ec2.RunInstancesInput, description string) {
	input.TagSpecifications = []ec2Types.TagSpecification{
		{
			ResourceType: ec2Types.ResourceTypeInstance,
			Tags: []ec2Types.Tag{
				{
					Key:   aws.String("Name"),
					Value: aws.String(description),
				},
			},
		},
	}
}

// DeleteResource deletes a resource (terminates an EC2 instance) by ID.
func (a *AWSAdapter) DeleteResource(ctx context.Context, id string) (err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "DeleteResource", start, err) }()

	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Extract the actual EC2 instance ID from the O2-IMS resource ID
	instanceID := strings.TrimPrefix(id, "aws-instance-")
	if instanceID == id {
		instanceID = id
	}

	_, err = a.ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("failed to terminate instance: %w", err)
	}

	a.logger.Info("deleted resource",
		zap.String("resourceId", id),
		zap.String("instanceId", instanceID))

	return nil
}

// instanceToResource converts an EC2 instance to an O2-IMS Resource.
func (a *AWSAdapter) instanceToResource(instance *ec2Types.Instance) *adapter.Resource {
	instanceID := aws.ToString(instance.InstanceId)
	resourceID := generateInstanceID(instanceID)
	resourceTypeID := generateInstanceTypeID(string(instance.InstanceType))

	// Determine resource pool ID based on pool mode
	var resourcePoolID string
	if a.poolMode == "az" {
		resourcePoolID = generateAZPoolID(aws.ToString(instance.Placement.AvailabilityZone))
	} else {
		// In ASG mode, we would need to look up which ASG this instance belongs to
		// For now, use the AZ as fallback
		resourcePoolID = generateAZPoolID(aws.ToString(instance.Placement.AvailabilityZone))
	}

	// Get instance name from tags
	name := extractTagValue(instance.Tags, "Name")
	if name == "" {
		name = instanceID
	}

	// Build extensions with EC2 instance details
	extensions := map[string]interface{}{
		"aws.instanceId":       instanceID,
		"aws.instanceType":     string(instance.InstanceType),
		"aws.availabilityZone": aws.ToString(instance.Placement.AvailabilityZone),
		"aws.state":            string(instance.State.Name),
		"aws.stateCode":        aws.ToInt32(instance.State.Code),
		"aws.imageId":          aws.ToString(instance.ImageId),
		"aws.privateIp":        aws.ToString(instance.PrivateIpAddress),
		"aws.publicIp":         aws.ToString(instance.PublicIpAddress),
		"aws.privateDns":       aws.ToString(instance.PrivateDnsName),
		"aws.publicDns":        aws.ToString(instance.PublicDnsName),
		"aws.vpcId":            aws.ToString(instance.VpcId),
		"aws.subnetId":         aws.ToString(instance.SubnetId),
		"aws.architecture":     string(instance.Architecture),
		"aws.platform":         aws.ToString(instance.PlatformDetails),
		"aws.launchTime":       instance.LaunchTime,
		"aws.tags":             tagsToMap(instance.Tags),
	}

	// Add EBS volume information
	if len(instance.BlockDeviceMappings) > 0 {
		volumes := make([]map[string]interface{}, 0, len(instance.BlockDeviceMappings))
		for _, bdm := range instance.BlockDeviceMappings {
			if bdm.Ebs != nil {
				volume := map[string]interface{}{
					"deviceName": aws.ToString(bdm.DeviceName),
					"volumeId":   aws.ToString(bdm.Ebs.VolumeId),
					"status":     string(bdm.Ebs.Status),
				}
				volumes = append(volumes, volume)
			}
		}
		extensions["aws.volumes"] = volumes
	}

	// Add network interface information
	if len(instance.NetworkInterfaces) > 0 {
		interfaces := make([]map[string]interface{}, 0, len(instance.NetworkInterfaces))
		for _, eni := range instance.NetworkInterfaces {
			iface := map[string]interface{}{
				"interfaceId": aws.ToString(eni.NetworkInterfaceId),
				"subnetId":    aws.ToString(eni.SubnetId),
				"privateIp":   aws.ToString(eni.PrivateIpAddress),
				"macAddress":  aws.ToString(eni.MacAddress),
				"status":      string(eni.Status),
			}
			interfaces = append(interfaces, iface)
		}
		extensions["aws.networkInterfaces"] = interfaces
	}

	// Add CPU and memory information if available
	if instance.CpuOptions != nil {
		extensions["aws.cpuCoreCount"] = aws.ToInt32(instance.CpuOptions.CoreCount)
		extensions["aws.cpuThreadsPerCore"] = aws.ToInt32(instance.CpuOptions.ThreadsPerCore)
	}

	return &adapter.Resource{
		ResourceID:     resourceID,
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:aws:ec2:%s:%s", a.region, instanceID),
		Description:    name,
		Extensions:     extensions,
	}
}
