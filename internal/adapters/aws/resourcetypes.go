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
func (a *Adapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) (resourceTypes []*adapter.ResourceType, err error) {
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
func (a *Adapter) GetResourceType(ctx context.Context, id string) (resourceType *adapter.ResourceType, err error) {
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
func (a *Adapter) instanceTypeToResourceType(instanceType *ec2Types.InstanceTypeInfo) *adapter.ResourceType {
	typeName := string(instanceType.InstanceType)
	resourceTypeID := generateInstanceTypeID(typeName)

	// Determine resource kind and parse instance type
	resourceKind := determineResourceKind(instanceType)
	instanceFamily, instanceSize := parseInstanceType(typeName)

	// Build extensions and description
	extensions := buildInstanceTypeExtensions(instanceType, instanceFamily, instanceSize)
	description := buildInstanceTypeDescription(instanceType, typeName)

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

// determineResourceKind determines if an instance type is physical or virtual.
func determineResourceKind(instanceType *ec2Types.InstanceTypeInfo) string {
	if instanceType.BareMetal != nil && *instanceType.BareMetal {
		return "physical"
	}
	return "virtual"
}

// parseInstanceType extracts family and size from instance type name.
func parseInstanceType(typeName string) (family, size string) {
	parts := strings.Split(typeName, ".")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

// buildInstanceTypeExtensions builds the extensions map with detailed instance information.
func buildInstanceTypeExtensions(instanceType *ec2Types.InstanceTypeInfo, family, size string) map[string]interface{} {
	typeName := string(instanceType.InstanceType)
	extensions := map[string]interface{}{
		"aws.instanceType":      typeName,
		"aws.instanceFamily":    family,
		"aws.instanceSize":      size,
		"aws.currentGeneration": instanceType.CurrentGeneration,
		"aws.bareMetal":         aws.ToBool(instanceType.BareMetal),
		"aws.freeTier":          aws.ToBool(instanceType.FreeTierEligible),
		"aws.hypervisor":        string(instanceType.Hypervisor),
	}

	addVCPUInfo(extensions, instanceType.VCpuInfo)
	addMemoryInfo(extensions, instanceType.MemoryInfo)
	addStorageInfo(extensions, instanceType.InstanceStorageInfo)
	addNetworkInfo(extensions, instanceType.NetworkInfo)
	addGPUInfo(extensions, instanceType.GpuInfo)
	addProcessorInfo(extensions, instanceType.ProcessorInfo)
	addUsageClasses(extensions, instanceType.SupportedUsageClasses)

	return extensions
}

// addVCPUInfo adds vCPU information to extensions.
func addVCPUInfo(extensions map[string]interface{}, vcpuInfo *ec2Types.VCpuInfo) {
	if vcpuInfo != nil {
		extensions["aws.vcpus"] = aws.ToInt32(vcpuInfo.DefaultVCpus)
		extensions["aws.vcpuCores"] = aws.ToInt32(vcpuInfo.DefaultCores)
		extensions["aws.vcpuThreadsPerCore"] = aws.ToInt32(vcpuInfo.DefaultThreadsPerCore)
	}
}

// addMemoryInfo adds memory information to extensions.
func addMemoryInfo(extensions map[string]interface{}, memoryInfo *ec2Types.MemoryInfo) {
	if memoryInfo != nil {
		extensions["aws.memoryMiB"] = aws.ToInt64(memoryInfo.SizeInMiB)
	}
}

// addStorageInfo adds storage information to extensions.
func addStorageInfo(extensions map[string]interface{}, storageInfo *ec2Types.InstanceStorageInfo) {
	if storageInfo != nil {
		extensions["aws.instanceStorageSupported"] = true
		extensions["aws.instanceStorageGiB"] = aws.ToInt64(storageInfo.TotalSizeInGB)
		if len(storageInfo.Disks) > 0 {
			extensions["aws.instanceStorageType"] = string(storageInfo.Disks[0].Type)
		}
	} else {
		extensions["aws.instanceStorageSupported"] = false
	}
}

// addNetworkInfo adds network information to extensions.
func addNetworkInfo(extensions map[string]interface{}, networkInfo *ec2Types.NetworkInfo) {
	if networkInfo != nil {
		extensions["aws.networkPerformance"] = aws.ToString(networkInfo.NetworkPerformance)
		extensions["aws.maxNetworkInterfaces"] = aws.ToInt32(networkInfo.MaximumNetworkInterfaces)
		extensions["aws.ipv4AddressesPerInterface"] = aws.ToInt32(networkInfo.Ipv4AddressesPerInterface)
		extensions["aws.enaSupported"] = networkInfo.EnaSupport == ec2Types.EnaSupportRequired || networkInfo.EnaSupport == ec2Types.EnaSupportSupported
	}
}

// addGPUInfo adds GPU information to extensions.
func addGPUInfo(extensions map[string]interface{}, gpuInfo *ec2Types.GpuInfo) {
	if gpuInfo != nil && len(gpuInfo.Gpus) > 0 {
		gpu := gpuInfo.Gpus[0]
		extensions["aws.gpuCount"] = aws.ToInt32(gpu.Count)
		extensions["aws.gpuManufacturer"] = aws.ToString(gpu.Manufacturer)
		extensions["aws.gpuName"] = aws.ToString(gpu.Name)
		extensions["aws.gpuMemoryMiB"] = aws.ToInt32(gpu.MemoryInfo.SizeInMiB)
	}
}

// addProcessorInfo adds processor information to extensions.
func addProcessorInfo(extensions map[string]interface{}, processorInfo *ec2Types.ProcessorInfo) {
	if processorInfo != nil {
		extensions["aws.processorArchitectures"] = processorInfo.SupportedArchitectures
		extensions["aws.processorClockSpeedGhz"] = aws.ToFloat64(processorInfo.SustainedClockSpeedInGhz)
	}
}

// addUsageClasses adds supported usage classes to extensions.
func addUsageClasses(extensions map[string]interface{}, usageClasses []ec2Types.UsageClassType) {
	if len(usageClasses) > 0 {
		classes := make([]string, len(usageClasses))
		for i, c := range usageClasses {
			classes[i] = string(c)
		}
		extensions["aws.supportedUsageClasses"] = classes
	}
}

// buildInstanceTypeDescription builds a description based on instance characteristics.
func buildInstanceTypeDescription(instanceType *ec2Types.InstanceTypeInfo, typeName string) string {
	if instanceType.VCpuInfo != nil && instanceType.MemoryInfo != nil {
		vcpus := aws.ToInt32(instanceType.VCpuInfo.DefaultVCpus)
		memGiB := aws.ToInt64(instanceType.MemoryInfo.SizeInMiB) / 1024
		return fmt.Sprintf("AWS %s: %d vCPUs, %d GiB RAM", typeName, vcpus, memGiB)
	}
	return fmt.Sprintf("AWS EC2 Instance Type %s", typeName)
}
