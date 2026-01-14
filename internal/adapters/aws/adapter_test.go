package aws_test

import (
	"context"
	"testing"
	"time"

	awsadapter "github.com/piwi3910/netweave/internal/adapters/aws"

	"github.com/aws/aws-sdk-go-v2/aws"
	autoscalingTypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new AWSAdapter.
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *awsadapter.Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing region",
			config: &awsadapter.Config{
				OCloudID: "ocloud-1",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "missing oCloudID",
			config: &awsadapter.Config{
				Region: "us-east-1",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "invalid pool mode",
			config: &awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "ocloud-1",
				PoolMode: "invalid",
			},
			wantErr: true,
			errMsg:  "poolMode must be 'az' or 'asg'",
		},
		{
			name: "valid config with az pool mode",
			config: &awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "ocloud-1",
				PoolMode: "az",
				Logger:   zap.NewNop(),
			},
			wantErr: false,
		},
		{
			name: "valid config with asg pool mode",
			config: &awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "ocloud-1",
				PoolMode: "asg",
				Logger:   zap.NewNop(),
			},
			wantErr: false,
		},
		{
			name: "valid config with defaults",
			config: &awsadapter.Config{
				Region:   "us-west-2",
				OCloudID: "ocloud-test",
				Logger:   zap.NewNop(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := awsadapter.New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adp)
				if adp != nil {
					defer func() { _ = adp.Close() }()
				}
			}
		})
	}
}

// TestNewWithDefaults tests default value initialization.
func TestNewWithDefaults(t *testing.T) {
	config := &awsadapter.Config{
		Region:   "us-east-1",
		OCloudID: "test-ocloud",
		Logger:   zap.NewNop(),
	}

	adp, err := awsadapter.New(config)
	require.NoError(t, err)
	require.NotNil(t, adp)
	defer func() { _ = adp.Close() }()

	// Check defaults
	assert.Equal(t, "test-ocloud", adp.OCloudID)
	assert.Equal(t, "ocloud-aws-us-east-1", adp.DeploymentManagerID)
	assert.Equal(t, "az", adp.PoolMode)
	assert.Equal(t, "us-east-1", adp.Region)
}

// TestMetadata tests metadata methods.
func TestMetadata(t *testing.T) {
	adp := &awsadapter.Adapter{
		Logger: zap.NewNop(),
	}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "aws", adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.NotEmpty(t, adp.Version())
		assert.Equal(t, "ec2-v2", adp.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := adp.Capabilities()
		assert.NotEmpty(t, caps)
		assert.Len(t, caps, 6)

		// Verify specific capabilities
		assert.Contains(t, caps, adapter.CapabilityResourcePools)
		assert.Contains(t, caps, adapter.CapabilityResources)
		assert.Contains(t, caps, adapter.CapabilityResourceTypes)
		assert.Contains(t, caps, adapter.CapabilityDeploymentManagers)
		assert.Contains(t, caps, adapter.CapabilitySubscriptions)
		assert.Contains(t, caps, adapter.CapabilityHealthChecks)
	})
}

// NOTE: TestMatchesFilter and TestApplyPagination tests moved to internal/adapter/helpers_test.go
// These shared helper functions are now tested in the common adapter package.

// TestGenerateIDs tests ID generation functions.
func TestGenerateIDs(t *testing.T) {
	t.Run("generateInstanceTypeID", func(t *testing.T) {
		tests := []struct {
			instanceType string
			want         string
		}{
			{"m5.large", "aws-instance-type-m5.large"},
			{"t3.micro", "aws-instance-type-t3.micro"},
			{"", "aws-instance-type-"},
		}

		for _, tt := range tests {
			got := awsadapter.GenerateInstanceTypeID(tt.instanceType)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateInstanceID", func(t *testing.T) {
		tests := []struct {
			instanceID string
			want       string
		}{
			{"i-1234567890abcdef0", "aws-instance-i-1234567890abcdef0"},
			{"", "aws-instance-"},
		}

		for _, tt := range tests {
			got := awsadapter.GenerateInstanceID(tt.instanceID)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateAZPoolID", func(t *testing.T) {
		tests := []struct {
			az   string
			want string
		}{
			{"us-east-1a", "aws-az-us-east-1a"},
			{"eu-west-1b", "aws-az-eu-west-1b"},
			{"", "aws-az-"},
		}

		for _, tt := range tests {
			got := awsadapter.GenerateAZPoolID(tt.az)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("generateASGPoolID", func(t *testing.T) {
		tests := []struct {
			asgName string
			want    string
		}{
			{"my-asg", "aws-asg-my-asg"},
			{"prod-web-asg", "aws-asg-prod-web-asg"},
			{"", "aws-asg-"},
		}

		for _, tt := range tests {
			got := awsadapter.GenerateASGPoolID(tt.asgName)
			assert.Equal(t, tt.want, got)
		}
	})
}

// TestSubscriptions tests subscription CRUD operations.
func TestSubscriptions(t *testing.T) {
	adp := &awsadapter.Adapter{
		Logger:        zap.NewNop(),
		Subscriptions: make(map[string]*adapter.Subscription),
	}
	ctx := context.Background()

	t.Run("CreateSubscription", func(t *testing.T) {
		sub := &adapter.Subscription{
			Callback:               "https://example.com/callback",
			ConsumerSubscriptionID: "consumer-sub-1",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.NotEmpty(t, created.SubscriptionID)
		assert.Equal(t, "https://example.com/callback", created.Callback)
		assert.Equal(t, "consumer-sub-1", created.ConsumerSubscriptionID)
	})

	t.Run("CreateSubscription with ID", func(t *testing.T) {
		sub := &adapter.Subscription{
			SubscriptionID: "my-custom-id",
			Callback:       "https://example.com/callback2",
		}

		created, err := adp.CreateSubscription(ctx, sub)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "my-custom-id", created.SubscriptionID)
	})

	t.Run("CreateSubscription without callback", func(t *testing.T) {
		sub := &adapter.Subscription{}

		_, err := adp.CreateSubscription(ctx, sub)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "callback URL is required")
	})

	t.Run("GetSubscription", func(t *testing.T) {
		sub, err := adp.GetSubscription(ctx, "my-custom-id")
		require.NoError(t, err)
		require.NotNil(t, sub)
		assert.Equal(t, "my-custom-id", sub.SubscriptionID)
	})

	t.Run("GetSubscription not found", func(t *testing.T) {
		_, err := adp.GetSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("ListSubscriptions", func(t *testing.T) {
		subs := adp.ListSubscriptions()
		assert.Len(t, subs, 2)
	})

	t.Run("DeleteSubscription", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "my-custom-id")
		require.NoError(t, err)

		_, err = adp.GetSubscription(ctx, "my-custom-id")
		require.Error(t, err)
	})

	t.Run("DeleteSubscription not found", func(t *testing.T) {
		err := adp.DeleteSubscription(ctx, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscription not found")
	})
}

// TestClose tests adapter cleanup.
func TestClose(t *testing.T) {
	adp := &awsadapter.Adapter{
		Logger:        zap.NewNop(),
		Subscriptions: make(map[string]*adapter.Subscription),
	}

	// Add some subscriptions
	adp.Subscriptions["sub-1"] = &adapter.Subscription{SubscriptionID: "sub-1"}
	adp.Subscriptions["sub-2"] = &adapter.Subscription{SubscriptionID: "sub-2"}

	err := adp.Close()
	assert.NoError(t, err)

	// Verify subscriptions are cleared
	assert.Empty(t, adp.Subscriptions)
}

// TestConfigValidation tests configuration validation.
func TestConfigValidation(t *testing.T) {
	t.Run("valid config with all fields", func(t *testing.T) {
		// NOTE: These are AWS documentation example credentials, NOT real credentials.
		// See: https://docs.aws.amazon.com/IAM/latest/UserGuide/security-creds.html
		config := &awsadapter.Config{
			Region:              "us-east-1",
			AccessKeyID:         "AKIAIOSFODNN7EXAMPLE",                     // Example key from AWS docs
			SecretAccessKey:     "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", // Example secret from AWS docs
			SessionToken:        "session-token",
			OCloudID:            "ocloud-test",
			DeploymentManagerID: "dm-test",
			PoolMode:            "asg",
			Timeout:             60 * time.Second,
			Logger:              zap.NewNop(),
		}

		adp, err := awsadapter.New(config)
		require.NoError(t, err)
		require.NotNil(t, adp)
		defer func() { _ = adp.Close() }()

		assert.Equal(t, "dm-test", adp.DeploymentManagerID)
		assert.Equal(t, "asg", adp.PoolMode)
	})

	t.Run("valid config with minimal fields", func(t *testing.T) {
		config := &awsadapter.Config{
			Region:   "us-west-2",
			OCloudID: "ocloud-test",
			Logger:   zap.NewNop(),
		}

		adp, err := awsadapter.New(config)
		require.NoError(t, err)
		require.NotNil(t, adp)
		defer func() { _ = adp.Close() }()

		// Check defaults are applied
		assert.Equal(t, "ocloud-aws-us-west-2", adp.DeploymentManagerID)
		assert.Equal(t, "az", adp.PoolMode)
	})
}

// NOTE: BenchmarkMatchesFilter and BenchmarkApplyPagination moved to internal/adapter/helpers_test.go

// TestAWSAdapter_Health tests the Health function.
func TestAWSAdapter_Health(t *testing.T) {
	adapter, err := awsadapter.New(&awsadapter.Config{
		Region:   "us-east-1",
		OCloudID: "test-cloud",
	})
	require.NoError(t, err)

	err = adapter.Health(context.Background())
	if err != nil {
		t.Skip("Skipping - requires AWS credentials")
	}
}

// TestAWSAdapter_ListResourcePools tests the ListResourcePools function.
func TestAWSAdapter_ListResourcePools(t *testing.T) {
	adapter, err := awsadapter.New(&awsadapter.Config{
		Region:   "us-east-1",
		OCloudID: "test-cloud",
		PoolMode: "az",
	})
	require.NoError(t, err)

	pools, err := adapter.ListResourcePools(context.Background(), nil)
	if err != nil {
		t.Skip("Skipping - requires AWS credentials")
	}
	assert.NotNil(t, pools)
}

// TestAWSAdapter_ListResources tests the ListResources function.
func TestAWSAdapter_ListResources(t *testing.T) {
	adapter, err := awsadapter.New(&awsadapter.Config{
		Region:   "us-east-1",
		OCloudID: "test-cloud",
	})
	require.NoError(t, err)

	resources, err := adapter.ListResources(context.Background(), nil)
	if err != nil {
		t.Skip("Skipping - requires AWS credentials")
	}
	assert.NotNil(t, resources)
}

// TestAWSAdapter_ListResourceTypes tests the ListResourceTypes function.
func TestAWSAdapter_ListResourceTypes(t *testing.T) {
	adapter, err := awsadapter.New(&awsadapter.Config{
		Region:   "us-east-1",
		OCloudID: "test-cloud",
	})
	require.NoError(t, err)

	types, err := adapter.ListResourceTypes(context.Background(), nil)
	if err != nil {
		t.Skip("Skipping - requires AWS credentials")
	}
	assert.NotNil(t, types)
}

// TestAWSAdapter_GetDeploymentManager tests the GetDeploymentManager function.
func TestAWSAdapter_GetDeploymentManager(t *testing.T) {
	adapter, err := awsadapter.New(&awsadapter.Config{
		Region:   "us-east-1",
		OCloudID: "test-cloud",
	})
	require.NoError(t, err)

	dm, err := adapter.GetDeploymentManager(context.Background(), "dm-1")
	if err != nil {
		t.Skip("Skipping - requires AWS credentials")
	}
	assert.NotNil(t, dm)
}

// TestExtractTagValue tests tag value extraction.
func TestExtractTagValue(t *testing.T) {
	tests := []struct {
		name string
		tags []ec2Types.Tag
		key  string
		want string
	}{
		{
			name: "tag exists",
			tags: []ec2Types.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-instance")},
				{Key: aws.String("Environment"), Value: aws.String("prod")},
			},
			key:  "Name",
			want: "test-instance",
		},
		{
			name: "tag not found",
			tags: []ec2Types.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-instance")},
			},
			key:  "NotFound",
			want: "",
		},
		{
			name: "empty tags",
			tags: []ec2Types.Tag{},
			key:  "Name",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := awsadapter.ExtractTagValue(tt.tags, tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTagsToMap tests tag to map conversion.
func TestTagsToMap(t *testing.T) {
	tests := []struct {
		name string
		tags []ec2Types.Tag
		want map[string]string
	}{
		{
			name: "multiple tags",
			tags: []ec2Types.Tag{
				{Key: aws.String("Name"), Value: aws.String("test-instance")},
				{Key: aws.String("Environment"), Value: aws.String("prod")},
			},
			want: map[string]string{
				"Name":        "test-instance",
				"Environment": "prod",
			},
		},
		{
			name: "empty tags",
			tags: []ec2Types.Tag{},
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := awsadapter.TagsToMap(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractASGNameFromPoolID tests ASG name extraction from pool ID.
func TestExtractASGNameFromPoolID(t *testing.T) {
	tests := []struct {
		name   string
		poolID string
		want   string
	}{
		{
			name:   "valid pool ID",
			poolID: "aws-asg-my-asg-name",
			want:   "my-asg-name",
		},
		{
			name:   "invalid pool ID",
			poolID: "invalid-pool-id",
			want:   "",
		},
		{
			name:   "empty pool ID",
			poolID: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := awsadapter.ExtractASGNameFromPoolID(tt.poolID)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractInt32FromExtensions tests int32 extraction from extensions map.
func TestExtractInt32FromExtensions(t *testing.T) {
	tests := []struct {
		name       string
		extensions map[string]interface{}
		key        string
		want       *int32
	}{
		{
			name: "int32 value",
			extensions: map[string]interface{}{
				"count": int32(5),
			},
			key:  "count",
			want: aws.Int32(5),
		},
		{
			name: "float64 value",
			extensions: map[string]interface{}{
				"count": float64(10),
			},
			key:  "count",
			want: aws.Int32(10),
		},
		{
			name: "key not found",
			extensions: map[string]interface{}{
				"other": int32(5),
			},
			key:  "count",
			want: nil,
		},
		{
			name:       "empty extensions",
			extensions: map[string]interface{}{},
			key:        "count",
			want:       nil,
		},
		{
			name: "wrong type",
			extensions: map[string]interface{}{
				"count": "string-value",
			},
			key:  "count",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := awsadapter.ExtractInt32FromExtensions(tt.extensions, tt.key)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}

// TestGetLaunchTemplateName tests launch template name extraction.
func TestGetLaunchTemplateName(t *testing.T) {
	tests := []struct {
		name string
		lt   *autoscalingTypes.LaunchTemplateSpecification
		want string
	}{
		{
			name: "template with name",
			lt: &autoscalingTypes.LaunchTemplateSpecification{
				LaunchTemplateName: aws.String("my-template"),
			},
			want: "my-template",
		},
		{
			name: "template without name",
			lt: &autoscalingTypes.LaunchTemplateSpecification{
				LaunchTemplateId: aws.String("lt-123"),
			},
			want: "",
		},
		{
			name: "nil template",
			lt:   nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := awsadapter.GetLaunchTemplateName(tt.lt)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractInstanceType tests instance type extraction from resource type ID.
func TestExtractInstanceType(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name           string
		resourceTypeID string
		want           string
	}{
		{
			name:           "with prefix",
			resourceTypeID: "aws-instance-type-m5.large",
			want:           "m5.large",
		},
		{
			name:           "without prefix",
			resourceTypeID: "t3.micro",
			want:           "t3.micro",
		},
		{
			name:           "empty string",
			resourceTypeID: "",
			want:           "",
		},
		{
			name:           "prefix only",
			resourceTypeID: "aws-instance-type-",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestExtractInstanceType(tt.resourceTypeID)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGetRequiredAMI tests AMI ID extraction and validation.
func TestGetRequiredAMI(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name       string
		extensions map[string]interface{}
		want       string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "valid AMI ID",
			extensions: map[string]interface{}{
				"aws.imageId": "ami-1234567890abcdef0",
			},
			want:    "ami-1234567890abcdef0",
			wantErr: false,
		},
		{
			name:       "nil extensions",
			extensions: nil,
			wantErr:    true,
			errMsg:     "aws.imageId is required",
		},
		{
			name:       "missing aws.imageId key",
			extensions: map[string]interface{}{},
			wantErr:    true,
			errMsg:     "aws.imageId is required",
		},
		{
			name: "empty AMI ID",
			extensions: map[string]interface{}{
				"aws.imageId": "",
			},
			wantErr: true,
			errMsg:  "aws.imageId is required",
		},
		{
			name: "wrong type",
			extensions: map[string]interface{}{
				"aws.imageId": 12345,
			},
			wantErr: true,
			errMsg:  "aws.imageId is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adp.TestGetRequiredAMI(tt.extensions)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestBuildResourceTags tests EC2 tag building from resource fields.
func TestBuildResourceTags(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name     string
		resource *adapter.Resource
		want     int
		checkTag func(t *testing.T, tags []ec2Types.Tag)
	}{
		{
			name: "all fields populated",
			resource: &adapter.Resource{
				TenantID:      "tenant-123",
				Description:   "test instance",
				GlobalAssetID: "urn:aws:ec2:us-east-1:i-123",
				Extensions: map[string]interface{}{
					"aws.tags": map[string]string{
						"Environment": "production",
						"Team":        "platform",
					},
				},
			},
			want: 5,
			checkTag: func(t *testing.T, tags []ec2Types.Tag) {
				tagMap := make(map[string]string)
				for _, tag := range tags {
					tagMap[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
				}
				assert.Equal(t, "tenant-123", tagMap["o2ims.io/tenant-id"])
				assert.Equal(t, "test instance", tagMap["Name"])
				assert.Equal(t, "urn:aws:ec2:us-east-1:i-123", tagMap["GlobalAssetID"])
				assert.Equal(t, "production", tagMap["Environment"])
				assert.Equal(t, "platform", tagMap["Team"])
			},
		},
		{
			name:     "empty resource",
			resource: &adapter.Resource{},
			want:     0,
		},
		{
			name: "only tenant ID",
			resource: &adapter.Resource{
				TenantID: "tenant-456",
			},
			want: 1,
			checkTag: func(t *testing.T, tags []ec2Types.Tag) {
				assert.Equal(t, "o2ims.io/tenant-id", aws.ToString(tags[0].Key))
				assert.Equal(t, "tenant-456", aws.ToString(tags[0].Value))
			},
		},
		{
			name: "custom tags only",
			resource: &adapter.Resource{
				Extensions: map[string]interface{}{
					"aws.tags": map[string]string{
						"CustomKey": "CustomValue",
					},
				},
			},
			want: 1,
			checkTag: func(t *testing.T, tags []ec2Types.Tag) {
				assert.Equal(t, "CustomKey", aws.ToString(tags[0].Key))
				assert.Equal(t, "CustomValue", aws.ToString(tags[0].Value))
			},
		},
		{
			name: "wrong custom tags type",
			resource: &adapter.Resource{
				Extensions: map[string]interface{}{
					"aws.tags": "not-a-map",
				},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildResourceTags(tt.resource)
			assert.Len(t, got, tt.want)
			if tt.checkTag != nil {
				tt.checkTag(t, got)
			}
		})
	}
}

// TestBuildRunInstanceInput tests RunInstances input building.
func TestBuildRunInstanceInput(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name       string
		resource   *adapter.Resource
		checkInput func(t *testing.T, input *ec2.RunInstancesInput)
	}{
		{
			name: "basic input",
			resource: &adapter.Resource{
				ResourceTypeID: "aws-instance-type-t3.micro",
				Description:    "test-instance",
				Extensions: map[string]interface{}{
					"aws.imageId": "ami-123",
				},
			},
			checkInput: func(t *testing.T, input *ec2.RunInstancesInput) {
				assert.Equal(t, "ami-123", aws.ToString(input.ImageId))
				assert.Equal(t, ec2Types.InstanceType("t3.micro"), input.InstanceType)
				assert.Equal(t, int32(1), aws.ToInt32(input.MinCount))
				assert.Equal(t, int32(1), aws.ToInt32(input.MaxCount))
				require.Len(t, input.TagSpecifications, 1)
				assert.Equal(t, "test-instance", aws.ToString(input.TagSpecifications[0].Tags[0].Value))
			},
		},
		{
			name: "with optional parameters",
			resource: &adapter.Resource{
				ResourceTypeID: "aws-instance-type-m5.large",
				Description:    "test-instance",
				Extensions: map[string]interface{}{
					"aws.imageId":          "ami-456",
					"aws.subnetId":         "subnet-123",
					"aws.securityGroupIds": []string{"sg-123", "sg-456"},
					"aws.keyName":          "my-keypair",
				},
			},
			checkInput: func(t *testing.T, input *ec2.RunInstancesInput) {
				assert.Equal(t, "subnet-123", aws.ToString(input.SubnetId))
				assert.Equal(t, []string{"sg-123", "sg-456"}, input.SecurityGroupIds)
				assert.Equal(t, "my-keypair", aws.ToString(input.KeyName))
			},
		},
		{
			name: "without description",
			resource: &adapter.Resource{
				ResourceTypeID: "aws-instance-type-t2.small",
				Extensions: map[string]interface{}{
					"aws.imageId": "ami-789",
				},
			},
			checkInput: func(t *testing.T, input *ec2.RunInstancesInput) {
				assert.Empty(t, input.TagSpecifications)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := adp.TestBuildRunInstanceInput(tt.resource)
			require.NotNil(t, input)
			if tt.checkInput != nil {
				tt.checkInput(t, input)
			}
		})
	}
}

// TestDetermineResourceKind tests resource kind determination.
func TestDetermineResourceKind(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name         string
		instanceType *ec2Types.InstanceTypeInfo
		want         string
	}{
		{
			name: "bare metal instance",
			instanceType: &ec2Types.InstanceTypeInfo{
				BareMetal: aws.Bool(true),
			},
			want: "physical",
		},
		{
			name: "virtual instance",
			instanceType: &ec2Types.InstanceTypeInfo{
				BareMetal: aws.Bool(false),
			},
			want: "virtual",
		},
		{
			name:         "nil bare metal field",
			instanceType: &ec2Types.InstanceTypeInfo{},
			want:         "virtual",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestDetermineResourceKind(tt.instanceType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestParseInstanceType tests instance type parsing.
func TestParseInstanceType(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name       string
		typeName   string
		wantFamily string
		wantSize   string
	}{
		{
			name:       "standard type",
			typeName:   "m5.large",
			wantFamily: "m5",
			wantSize:   "large",
		},
		{
			name:       "xlarge type",
			typeName:   "t3.2xlarge",
			wantFamily: "t3",
			wantSize:   "2xlarge",
		},
		{
			name:       "bare metal",
			typeName:   "m5.metal",
			wantFamily: "m5",
			wantSize:   "metal",
		},
		{
			name:       "no dot separator",
			typeName:   "invalid",
			wantFamily: "",
			wantSize:   "",
		},
		{
			name:       "empty string",
			typeName:   "",
			wantFamily: "",
			wantSize:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFamily, gotSize := adp.TestParseInstanceType(tt.typeName)
			assert.Equal(t, tt.wantFamily, gotFamily)
			assert.Equal(t, tt.wantSize, gotSize)
		})
	}
}

// TestBuildInstanceTypeExtensions tests extensions building for instance types.
func TestBuildInstanceTypeExtensions(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name         string
		instanceType *ec2Types.InstanceTypeInfo
		family       string
		size         string
		checkExts    func(t *testing.T, exts map[string]interface{})
	}{
		{
			name: "complete instance type",
			instanceType: &ec2Types.InstanceTypeInfo{
				InstanceType:          ec2Types.InstanceType("m5.large"),
				CurrentGeneration:     aws.Bool(true),
				BareMetal:             aws.Bool(false),
				FreeTierEligible:      aws.Bool(false),
				Hypervisor:            ec2Types.InstanceTypeHypervisorNitro,
				VCpuInfo:              &ec2Types.VCpuInfo{DefaultVCpus: aws.Int32(2), DefaultCores: aws.Int32(2), DefaultThreadsPerCore: aws.Int32(1)},
				MemoryInfo:            &ec2Types.MemoryInfo{SizeInMiB: aws.Int64(8192)},
				NetworkInfo:           &ec2Types.NetworkInfo{NetworkPerformance: aws.String("Up to 10 Gigabit"), MaximumNetworkInterfaces: aws.Int32(3), Ipv4AddressesPerInterface: aws.Int32(10), EnaSupport: ec2Types.EnaSupportRequired},
				ProcessorInfo:         &ec2Types.ProcessorInfo{SupportedArchitectures: []ec2Types.ArchitectureType{ec2Types.ArchitectureTypeX8664}, SustainedClockSpeedInGhz: aws.Float64(3.1)},
				SupportedUsageClasses: []ec2Types.UsageClassType{ec2Types.UsageClassTypeOnDemand, ec2Types.UsageClassTypeSpot},
			},
			family: "m5",
			size:   "large",
			checkExts: func(t *testing.T, exts map[string]interface{}) {
				assert.Equal(t, "m5.large", exts["aws.instanceType"])
				assert.Equal(t, "m5", exts["aws.instanceFamily"])
				assert.Equal(t, "large", exts["aws.instanceSize"])
				assert.True(t, aws.ToBool(exts["aws.currentGeneration"].(*bool)))
				assert.False(t, exts["aws.bareMetal"].(bool))
				assert.Equal(t, int32(2), exts["aws.vcpus"])
				assert.Equal(t, int64(8192), exts["aws.memoryMiB"])
				assert.Equal(t, "Up to 10 Gigabit", exts["aws.networkPerformance"])
				assert.Equal(t, float64(3.1), exts["aws.processorClockSpeedGhz"])
				assert.True(t, exts["aws.enaSupported"].(bool))
				assert.Contains(t, exts, "aws.supportedUsageClasses")
			},
		},
		{
			name: "instance with GPU",
			instanceType: &ec2Types.InstanceTypeInfo{
				InstanceType: ec2Types.InstanceType("p3.2xlarge"),
				GpuInfo: &ec2Types.GpuInfo{
					Gpus: []ec2Types.GpuDeviceInfo{
						{
							Count:        aws.Int32(1),
							Manufacturer: aws.String("NVIDIA"),
							Name:         aws.String("Tesla V100"),
							MemoryInfo:   &ec2Types.GpuDeviceMemoryInfo{SizeInMiB: aws.Int32(16384)},
						},
					},
				},
			},
			family: "p3",
			size:   "2xlarge",
			checkExts: func(t *testing.T, exts map[string]interface{}) {
				assert.Equal(t, int32(1), exts["aws.gpuCount"])
				assert.Equal(t, "NVIDIA", exts["aws.gpuManufacturer"])
				assert.Equal(t, "Tesla V100", exts["aws.gpuName"])
				assert.Equal(t, int32(16384), exts["aws.gpuMemoryMiB"])
			},
		},
		{
			name: "instance with storage",
			instanceType: &ec2Types.InstanceTypeInfo{
				InstanceType: ec2Types.InstanceType("m5d.large"),
				InstanceStorageInfo: &ec2Types.InstanceStorageInfo{
					TotalSizeInGB: aws.Int64(75),
					Disks: []ec2Types.DiskInfo{
						{Type: ec2Types.DiskTypeHdd},
					},
				},
			},
			family: "m5d",
			size:   "large",
			checkExts: func(t *testing.T, exts map[string]interface{}) {
				assert.True(t, exts["aws.instanceStorageSupported"].(bool))
				assert.Equal(t, int64(75), exts["aws.instanceStorageGiB"])
				assert.Equal(t, "hdd", exts["aws.instanceStorageType"])
			},
		},
		{
			name: "minimal instance type",
			instanceType: &ec2Types.InstanceTypeInfo{
				InstanceType: ec2Types.InstanceType("t3.nano"),
			},
			family: "t3",
			size:   "nano",
			checkExts: func(t *testing.T, exts map[string]interface{}) {
				assert.Equal(t, "t3.nano", exts["aws.instanceType"])
				assert.False(t, exts["aws.instanceStorageSupported"].(bool))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildInstanceTypeExtensions(tt.instanceType, tt.family, tt.size)
			require.NotNil(t, got)
			if tt.checkExts != nil {
				tt.checkExts(t, got)
			}
		})
	}
}

// TestBuildInstanceTypeDescription tests description building.
func TestBuildInstanceTypeDescription(t *testing.T) {
	adp := &awsadapter.Adapter{Logger: zap.NewNop()}

	tests := []struct {
		name         string
		instanceType *ec2Types.InstanceTypeInfo
		typeName     string
		want         string
	}{
		{
			name: "with vCPU and memory",
			instanceType: &ec2Types.InstanceTypeInfo{
				VCpuInfo:   &ec2Types.VCpuInfo{DefaultVCpus: aws.Int32(4)},
				MemoryInfo: &ec2Types.MemoryInfo{SizeInMiB: aws.Int64(16384)},
			},
			typeName: "m5.xlarge",
			want:     "AWS m5.xlarge: 4 vCPUs, 16 GiB RAM",
		},
		{
			name:         "without info",
			instanceType: &ec2Types.InstanceTypeInfo{},
			typeName:     "t3.micro",
			want:         "AWS EC2 Instance Type t3.micro",
		},
		{
			name: "with only vCPU",
			instanceType: &ec2Types.InstanceTypeInfo{
				VCpuInfo: &ec2Types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
			},
			typeName: "t2.small",
			want:     "AWS EC2 Instance Type t2.small",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.TestBuildInstanceTypeDescription(tt.instanceType, tt.typeName)
			assert.Equal(t, tt.want, got)
		})
	}
}
