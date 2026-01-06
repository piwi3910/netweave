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

// ListResourceTypes retrieves all resource types (EC2 instance types) matching the provided filter.
func (a *AWSAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) (resourceTypes []*adapter.ResourceType, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "ListResourceTypes", start, err) }()

	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// Get EC2 instance types
	paginator := ec2.NewDescribeInstanceTypesPaginator(a.ec2Client, &ec2.DescribeInstanceTypesInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe instance types: %w", err)
		}

		for _, instanceType := range page.InstanceTypes {
			resourceType := a.instanceTypeToResourceType(&instanceType)

			// Apply filter using shared helper
			if !adapter.MatchesFilter(filter, "", resourceType.ResourceTypeID, "", nil) {
				continue
			}

			resourceTypes = append(resourceTypes, resourceType)
		}
	}

	// Apply pagination using shared helper
	if filter != nil {
		resourceTypes = adapter.ApplyPagination(resourceTypes, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// GetResourceType retrieves a specific resource type (EC2 instance type) by ID.
func (a *AWSAdapter) GetResourceType(ctx context.Context, id string) (resourceType *adapter.ResourceType, err error) {
	start := time.Now()
	defer func() { adapter.ObserveOperation("aws", "GetResourceType", start, err) }()

	a.logger.Debug("GetResourceType called",
		zap.String("id", id))

	// Extract the actual instance type from the O2-IMS resource type ID
	instanceType := strings.TrimPrefix(id, "aws-instance-type-")
	if instanceType == id {
		instanceType = id
	}

	output, err := a.ec2Client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2Types.InstanceType{ec2Types.InstanceType(instanceType)},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance type: %w", err)
	}

	if len(output.InstanceTypes) == 0 {
		return nil, fmt.Errorf("resource type not found: %s", id)
	}

	resourceType = a.instanceTypeToResourceType(&output.InstanceTypes[0])

	a.logger.Info("retrieved resource type",
		zap.String("resourceTypeId", resourceType.ResourceTypeID))

	return resourceType, nil
}

// instanceTypeToResourceType converts an EC2 instance type to an O2-IMS ResourceType.
func (a *AWSAdapter) instanceTypeToResourceType(instanceType *ec2Types.InstanceTypeInfo) *adapter.ResourceType {
	typeName := string(instanceType.InstanceType)
	resourceTypeID := generateInstanceTypeID(typeName)

	// Determine resource kind based on virtualization type
	resourceKind := "virtual"
	if instanceType.BareMetal != nil && *instanceType.BareMetal {
		resourceKind = "physical"
	}

	// Parse instance family from type name (e.g., "m5" from "m5.large")
	parts := strings.Split(typeName, ".")
	instanceFamily := ""
	instanceSize := ""
	if len(parts) >= 2 {
		instanceFamily = parts[0]
		instanceSize = parts[1]
	}

	// Build extensions with detailed instance type information
	extensions := map[string]interface{}{
		"aws.instanceType":      typeName,
		"aws.instanceFamily":    instanceFamily,
		"aws.instanceSize":      instanceSize,
		"aws.currentGeneration": instanceType.CurrentGeneration,
		"aws.bareMetal":         aws.ToBool(instanceType.BareMetal),
		"aws.freeTier":          aws.ToBool(instanceType.FreeTierEligible),
		"aws.hypervisor":        string(instanceType.Hypervisor),
	}

	// Add vCPU information
	if instanceType.VCpuInfo != nil {
		extensions["aws.vcpus"] = aws.ToInt32(instanceType.VCpuInfo.DefaultVCpus)
		extensions["aws.vcpuCores"] = aws.ToInt32(instanceType.VCpuInfo.DefaultCores)
		extensions["aws.vcpuThreadsPerCore"] = aws.ToInt32(instanceType.VCpuInfo.DefaultThreadsPerCore)
	}

	// Add memory information
	if instanceType.MemoryInfo != nil {
		extensions["aws.memoryMiB"] = aws.ToInt64(instanceType.MemoryInfo.SizeInMiB)
	}

	// Add storage information
	if instanceType.InstanceStorageInfo != nil {
		extensions["aws.instanceStorageSupported"] = true
		extensions["aws.instanceStorageGiB"] = aws.ToInt64(instanceType.InstanceStorageInfo.TotalSizeInGB)
		if len(instanceType.InstanceStorageInfo.Disks) > 0 {
			extensions["aws.instanceStorageType"] = string(instanceType.InstanceStorageInfo.Disks[0].Type)
		}
	} else {
		extensions["aws.instanceStorageSupported"] = false
	}

	// Add network information
	if instanceType.NetworkInfo != nil {
		extensions["aws.networkPerformance"] = aws.ToString(instanceType.NetworkInfo.NetworkPerformance)
		extensions["aws.maxNetworkInterfaces"] = aws.ToInt32(instanceType.NetworkInfo.MaximumNetworkInterfaces)
		extensions["aws.ipv4AddressesPerInterface"] = aws.ToInt32(instanceType.NetworkInfo.Ipv4AddressesPerInterface)
		extensions["aws.enaSupported"] = instanceType.NetworkInfo.EnaSupport == ec2Types.EnaSupportRequired || instanceType.NetworkInfo.EnaSupport == ec2Types.EnaSupportSupported
	}

	// Add GPU information
	if instanceType.GpuInfo != nil && len(instanceType.GpuInfo.Gpus) > 0 {
		gpuInfo := instanceType.GpuInfo.Gpus[0]
		extensions["aws.gpuCount"] = aws.ToInt32(gpuInfo.Count)
		extensions["aws.gpuManufacturer"] = aws.ToString(gpuInfo.Manufacturer)
		extensions["aws.gpuName"] = aws.ToString(gpuInfo.Name)
		extensions["aws.gpuMemoryMiB"] = aws.ToInt32(gpuInfo.MemoryInfo.SizeInMiB)
	}

	// Add processor information
	if instanceType.ProcessorInfo != nil {
		extensions["aws.processorArchitectures"] = instanceType.ProcessorInfo.SupportedArchitectures
		extensions["aws.processorClockSpeedGhz"] = aws.ToFloat64(instanceType.ProcessorInfo.SustainedClockSpeedInGhz)
	}

	// Add supported usage classes
	if len(instanceType.SupportedUsageClasses) > 0 {
		classes := make([]string, len(instanceType.SupportedUsageClasses))
		for i, c := range instanceType.SupportedUsageClasses {
			classes[i] = string(c)
		}
		extensions["aws.supportedUsageClasses"] = classes
	}

	// Determine description based on instance characteristics
	var description string
	if instanceType.VCpuInfo != nil && instanceType.MemoryInfo != nil {
		vcpus := aws.ToInt32(instanceType.VCpuInfo.DefaultVCpus)
		memGiB := aws.ToInt64(instanceType.MemoryInfo.SizeInMiB) / 1024
		description = fmt.Sprintf("AWS %s: %d vCPUs, %d GiB RAM", typeName, vcpus, memGiB)
	} else {
		description = fmt.Sprintf("AWS EC2 Instance Type %s", typeName)
	}

	return &adapter.ResourceType{
		ResourceTypeID: resourceTypeID,
		Name:           typeName,
		Description:    description,
		Vendor:         "Amazon Web Services",
		Model:          typeName,
		Version:        instanceFamily,
		ResourceClass:  "compute",
		ResourceKind:   resourceKind,
		Extensions:     extensions,
	}
}
