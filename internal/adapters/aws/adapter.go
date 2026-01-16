// Package aws provides an AWS implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to AWS API calls, mapping O2-IMS resources
// to AWS resources like EC2 instances, Auto Scaling Groups, and instance types.
//
// Resource Mapping:
//   - Resource Pools → Availability Zones, Auto Scaling Groups
//   - Resources → EC2 Instances
//   - Resource Types → EC2 Instance Types
//   - Deployment Manager → AWS Region metadata
package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

const (
	poolModeASG = "asg"
)

// Adapter implements the adapter.Adapter interface for AWS backends.
// It provides O2-IMS functionality by mapping O2-IMS resources to AWS resources:
//   - Resource Pools → Availability Zones or Auto Scaling Groups
//   - Resources → EC2 Instances
//   - Resource Types → EC2 Instance Types
//   - Deployment Manager → AWS Region metadata
//   - Subscriptions → EventBridge/CloudWatch Events based (polling as fallback)
type Adapter struct {
	// ec2Client is the AWS EC2 service client.
	ec2Client *ec2.Client

	// asgClient is the AWS Auto Scaling service client.
	asgClient *autoscaling.Client

	// logger provides structured logging.
	Logger *zap.Logger

	// oCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// deploymentManagerID is the identifier for this deployment manager.
	DeploymentManagerID string

	// region is the AWS region this adapter manages.
	Region string

	// subscriptions holds active subscriptions (polling-based fallback).
	// Note: Subscriptions are stored in-memory and will be lost on adapter restart.
	// For production use, consider implementing persistent storage via Redis.
	Subscriptions map[string]*adapter.Subscription

	// subscriptionsMu protects the subscriptions map.
	SubscriptionsMu sync.RWMutex

	// poolMode determines how resource pools are mapped.
	// "az" maps to Availability Zones, "asg" maps to Auto Scaling Groups.
	PoolMode string
}

// Config holds configuration for creating an AWSAdapter.
type Config struct {
	// Region is the AWS region to manage (e.g., "us-east-1").
	Region string

	// AccessKeyID is the AWS access key ID for authentication.
	// If empty, the SDK will use the default credential chain.
	AccessKeyID string

	// SecretAccessKey is the AWS secret access key for authentication.
	// If empty, the SDK will use the default credential chain.
	SecretAccessKey string

	// SessionToken is the AWS session token for temporary credentials (optional).
	SessionToken string

	// Profile is the AWS profile name from the credentials file (optional).
	// If set, AccessKeyID and SecretAccessKey are ignored.
	Profile string

	// OCloudID is the identifier of the parent O-Cloud.
	OCloudID string

	// DeploymentManagerID is the identifier for this deployment manager.
	// If empty, defaults to "ocloud-aws-{region}".
	DeploymentManagerID string

	// PoolMode determines how resource pools are mapped:
	// - "az": Map to Availability Zones (default)
	// - "asg": Map to Auto Scaling Groups
	PoolMode string

	// Timeout is the timeout for AWS API calls.
	// Defaults to 30 seconds if not specified.
	Timeout time.Duration

	// Logger is the logger to use. If nil, a default logger will be created.
	Logger *zap.Logger
}

// New creates a new AWSAdapter with the provided configuration.
// It authenticates with AWS and initializes service clients for EC2 and Auto Scaling.
//
// Example:
//
//	adp, err := aws.New(&aws.Config{
//	    Region:          "us-east-1",
//	    AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
//	    SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
//	    OCloudID:        "ocloud-aws-1",
//	    PoolMode:        "az",
//	})
func New(cfg *Config) (*Adapter, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	deploymentManagerID, poolMode, timeout := applyDefaults(cfg)
	logger, err := initializeLogger(cfg.Logger)
	if err != nil {
		return nil, err
	}

	logger.Info("initializing AWS adapter",
		zap.String("region", cfg.Region),
		zap.String("oCloudID", cfg.OCloudID),
		zap.String("poolMode", poolMode))

	awsCfgOpts := buildAWSConfigOptions(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	awsCfg, err := config.LoadDefaultConfig(ctx, awsCfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	return &Adapter{
		ec2Client:           ec2.NewFromConfig(awsCfg),
		asgClient:           autoscaling.NewFromConfig(awsCfg),
		Logger:              logger,
		OCloudID:            cfg.OCloudID,
		DeploymentManagerID: deploymentManagerID,
		Region:              cfg.Region,
		Subscriptions:       make(map[string]*adapter.Subscription),
		PoolMode:            poolMode,
	}, nil
}

// validateConfig validates required configuration fields.
func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if cfg.Region == "" {
		return fmt.Errorf("region is required")
	}
	if cfg.OCloudID == "" {
		return fmt.Errorf("oCloudID is required")
	}

	// Validate poolMode if provided
	if cfg.PoolMode != "" && cfg.PoolMode != "az" && cfg.PoolMode != poolModeASG {
		return fmt.Errorf("poolMode must be 'az' or 'asg', got %q", cfg.PoolMode)
	}

	return nil
}

// applyDefaults applies default values to configuration.
func applyDefaults(cfg *Config) (string, string, time.Duration) {
	deploymentManagerID := cfg.DeploymentManagerID
	if deploymentManagerID == "" {
		deploymentManagerID = fmt.Sprintf("ocloud-aws-%s", cfg.Region)
	}

	poolMode := cfg.PoolMode
	if poolMode == "" {
		poolMode = "az"
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return deploymentManagerID, poolMode, timeout
}

// initializeLogger creates or returns the configured logger.
func initializeLogger(logger *zap.Logger) (*zap.Logger, error) {
	if logger != nil {
		return logger, nil
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create Logger: %w", err)
	}
	return logger, nil
}

// buildAWSConfigOptions builds AWS SDK configuration options.
func buildAWSConfigOptions(cfg *Config, logger *zap.Logger) []func(*config.LoadOptions) error {
	opts := []func(*config.LoadOptions) error{config.WithRegion(cfg.Region)}

	switch {
	case cfg.Profile != "":
		opts = append(opts, config.WithSharedConfigProfile(cfg.Profile))
		logger.Info("using AWS profile for authentication",
			zap.String("profile", cfg.Profile))
	case cfg.AccessKeyID != "" && cfg.SecretAccessKey != "":
		creds := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)
		opts = append(opts, config.WithCredentialsProvider(creds))
		logger.Info("using static credentials for authentication")
	default:
		logger.Info("using default AWS credential chain")
	}

	return opts
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "aws"
}

// Version returns the AWS API version this adapter supports.
func (a *Adapter) Version() string {
	return "ec2-v2"
}

// Capabilities returns the list of O2-IMS capabilities supported by this adapter.
func (a *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityResourcePools,
		adapter.CapabilityResources,
		adapter.CapabilityResourceTypes,
		adapter.CapabilityDeploymentManagers,
		adapter.CapabilitySubscriptions, // Polling-based
		adapter.CapabilityHealthChecks,
	}
}

// Health performs a health check on the AWS backend.
// It verifies connectivity to EC2 and Auto Scaling services.
// The check uses a 10-second timeout to prevent indefinite blocking.
func (a *Adapter) Health(ctx context.Context) error {
	start := time.Now()
	var err error
	defer func() { adapter.ObserveHealthCheck("aws", start, err) }()

	a.Logger.Debug("health check called")

	// Use a timeout to prevent indefinite blocking
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check EC2 service by describing regions
	_, err = a.ec2Client.DescribeRegions(healthCtx, &ec2.DescribeRegionsInput{
		RegionNames: []string{a.Region},
	})
	if err != nil {
		a.Logger.Error("EC2 health check failed", zap.Error(err))
		err = fmt.Errorf("ec2 API unreachable: %w", err)
		return err
	}

	// Check Auto Scaling service by describing account limits
	_, err = a.asgClient.DescribeAccountLimits(healthCtx, &autoscaling.DescribeAccountLimitsInput{})
	if err != nil {
		a.Logger.Error("Auto Scaling health check failed", zap.Error(err))
		err = fmt.Errorf("auto scaling API unreachable: %w", err)
		return err
	}

	a.Logger.Debug("health check passed")
	return nil
}

// Test helper exports for testing private functions

// TestExtractInstanceType exports extractInstanceType for testing.
func (a *Adapter) TestExtractInstanceType(resourceTypeID string) string {
	return extractInstanceType(resourceTypeID)
}

// TestGetRequiredAMI exports getRequiredAMI for testing.
func (a *Adapter) TestGetRequiredAMI(extensions map[string]interface{}) (string, error) {
	return getRequiredAMI(extensions)
}

// TestBuildResourceTags exports buildResourceTags for testing.
func (a *Adapter) TestBuildResourceTags(resource *adapter.Resource) []ec2Types.Tag {
	return buildResourceTags(resource)
}

// TestExtractInstanceID exports extractInstanceID for testing.
func (a *Adapter) TestExtractInstanceID(id string) string {
	return extractInstanceID(id)
}

// TestBuildRunInstanceInput exports buildRunInstanceInput for testing.
func (a *Adapter) TestBuildRunInstanceInput(resource interface{}) *ec2.RunInstancesInput {
	r, ok := resource.(*adapter.Resource)
	if !ok {
		return nil
	}
	instanceType := extractInstanceType(r.ResourceTypeID)
	amiID, _ := getRequiredAMI(r.Extensions)
	return buildRunInstanceInput(r, instanceType, amiID)
}

// TestDetermineResourceKind exports determineResourceKind for testing.
func (a *Adapter) TestDetermineResourceKind(instanceType *ec2Types.InstanceTypeInfo) string {
	return determineResourceKind(instanceType)
}

// TestParseInstanceType exports parseInstanceType for testing.
func (a *Adapter) TestParseInstanceType(typeName string) (string, string) {
	return parseInstanceType(typeName)
}

// TestBuildInstanceTypeExtensions exports buildInstanceTypeExtensions for testing.
func (a *Adapter) TestBuildInstanceTypeExtensions(
	instanceType *ec2Types.InstanceTypeInfo,
	family, size string,
) map[string]interface{} {
	return buildInstanceTypeExtensions(instanceType, family, size)
}

// TestBuildInstanceTypeDescription exports buildInstanceTypeDescription for testing.
func (a *Adapter) TestBuildInstanceTypeDescription(instanceType *ec2Types.InstanceTypeInfo, typeName string) string {
	return buildInstanceTypeDescription(instanceType, typeName)
}

// Close cleanly shuts down the adapter and releases resources.
func (a *Adapter) Close() error {
	a.Logger.Info("closing AWS adapter")

	// Clear subscriptions
	a.SubscriptionsMu.Lock()
	a.Subscriptions = make(map[string]*adapter.Subscription)
	a.SubscriptionsMu.Unlock()

	// Sync logger before shutdown
	// Ignore sync errors on stderr/stdout
	_ = a.Logger.Sync()

	return nil
}

// GenerateInstanceTypeID generates a consistent resource type ID for an instance type.
func GenerateInstanceTypeID(instanceType string) string {
	return fmt.Sprintf("aws-instance-type-%s", instanceType)
}

// GenerateInstanceID generates a consistent resource ID for an EC2 instance.
func GenerateInstanceID(instanceID string) string {
	return fmt.Sprintf("aws-instance-%s", instanceID)
}

// GenerateAZPoolID generates a consistent resource pool ID for an Availability Zone.
func GenerateAZPoolID(az string) string {
	return fmt.Sprintf("aws-az-%s", az)
}

// GenerateASGPoolID generates a consistent resource pool ID for an Auto Scaling Group.
func GenerateASGPoolID(asgName string) string {
	return fmt.Sprintf("aws-asg-%s", asgName)
}

// ExtractTagValue extracts a value from AWS tags.
func ExtractTagValue(tags []ec2Types.Tag, key string) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

// TagsToMap converts AWS tags to a map.
func TagsToMap(tags []ec2Types.Tag) map[string]string {
	result := make(map[string]string)
	for _, tag := range tags {
		result[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return result
}
