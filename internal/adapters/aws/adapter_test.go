package aws

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new AWSAdapter.
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
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
			config: &Config{
				OCloudID: "ocloud-1",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "missing oCloudID",
			config: &Config{
				Region: "us-east-1",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
		{
			name: "invalid pool mode",
			config: &Config{
				Region:   "us-east-1",
				OCloudID: "ocloud-1",
				PoolMode: "invalid",
			},
			wantErr: true,
			errMsg:  "poolMode must be 'az' or 'asg'",
		},
		{
			name: "valid config with az pool mode",
			config: &Config{
				Region:   "us-east-1",
				OCloudID: "ocloud-1",
				PoolMode: "az",
				Logger:   zap.NewNop(),
			},
			wantErr: false,
		},
		{
			name: "valid config with asg pool mode",
			config: &Config{
				Region:   "us-east-1",
				OCloudID: "ocloud-1",
				PoolMode: "asg",
				Logger:   zap.NewNop(),
			},
			wantErr: false,
		},
		{
			name: "valid config with defaults",
			config: &Config{
				Region:   "us-west-2",
				OCloudID: "ocloud-test",
				Logger:   zap.NewNop(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adp)
				if adp != nil {
					defer adp.Close()
				}
			}
		})
	}
}

// TestNewWithDefaults tests default value initialization.
func TestNewWithDefaults(t *testing.T) {
	config := &Config{
		Region:   "us-east-1",
		OCloudID: "test-ocloud",
		Logger:   zap.NewNop(),
	}

	adp, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, adp)
	defer adp.Close()

	// Check defaults
	assert.Equal(t, "test-ocloud", adp.oCloudID)
	assert.Equal(t, "ocloud-aws-us-east-1", adp.deploymentManagerID)
	assert.Equal(t, "az", adp.poolMode)
	assert.Equal(t, "us-east-1", adp.region)
}

// TestMetadata tests metadata methods.
func TestMetadata(t *testing.T) {
	adp := &AWSAdapter{
		logger: zap.NewNop(),
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

// TestMatchesFilter tests the filter matching logic.
func TestMatchesFilter(t *testing.T) {
	adp := &AWSAdapter{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name           string
		filter         *adapter.Filter
		resourcePoolID string
		resourceTypeID string
		location       string
		labels         map[string]string
		want           bool
	}{
		{
			name:           "nil filter matches all",
			filter:         nil,
			resourcePoolID: "pool-1",
			want:           true,
		},
		{
			name: "resource pool filter matches",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-1",
			},
			resourcePoolID: "pool-1",
			want:           true,
		},
		{
			name: "resource pool filter doesn't match",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-1",
			},
			resourcePoolID: "pool-2",
			want:           false,
		},
		{
			name: "resource type filter matches",
			filter: &adapter.Filter{
				ResourceTypeID: "type-1",
			},
			resourceTypeID: "type-1",
			want:           true,
		},
		{
			name: "resource type filter doesn't match",
			filter: &adapter.Filter{
				ResourceTypeID: "type-1",
			},
			resourceTypeID: "type-2",
			want:           false,
		},
		{
			name: "location filter matches",
			filter: &adapter.Filter{
				Location: "us-east-1a",
			},
			location: "us-east-1a",
			want:     true,
		},
		{
			name: "location filter doesn't match",
			filter: &adapter.Filter{
				Location: "us-east-1a",
			},
			location: "us-east-1b",
			want:     false,
		},
		{
			name: "labels filter matches",
			filter: &adapter.Filter{
				Labels: map[string]string{
					"env": "prod",
				},
			},
			labels: map[string]string{
				"env": "prod",
				"app": "web",
			},
			want: true,
		},
		{
			name: "labels filter doesn't match",
			filter: &adapter.Filter{
				Labels: map[string]string{
					"env": "prod",
				},
			},
			labels: map[string]string{
				"env": "dev",
			},
			want: false,
		},
		{
			name: "multiple filters all match",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
			},
			resourcePoolID: "pool-1",
			location:       "us-east-1a",
			want:           true,
		},
		{
			name: "multiple filters one doesn't match",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-1",
				Location:       "us-east-1a",
			},
			resourcePoolID: "pool-1",
			location:       "us-east-1b",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.matchesFilter(
				tt.filter,
				tt.resourcePoolID,
				tt.resourceTypeID,
				tt.location,
				tt.labels,
			)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestApplyPagination tests the pagination logic.
func TestApplyPagination(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

	tests := []struct {
		name   string
		limit  int
		offset int
		want   []string
	}{
		{
			name:   "no pagination",
			limit:  0,
			offset: 0,
			want:   items,
		},
		{
			name:   "limit only",
			limit:  3,
			offset: 0,
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "offset only",
			limit:  0,
			offset: 3,
			want:   []string{"d", "e", "f", "g", "h", "i", "j"},
		},
		{
			name:   "limit and offset",
			limit:  3,
			offset: 2,
			want:   []string{"c", "d", "e"},
		},
		{
			name:   "offset beyond items",
			limit:  3,
			offset: 20,
			want:   []string{},
		},
		{
			name:   "limit larger than remaining items",
			limit:  10,
			offset: 5,
			want:   []string{"f", "g", "h", "i", "j"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyPagination(items, tt.limit, tt.offset)
			assert.Equal(t, tt.want, got)
		})
	}
}

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
			got := generateInstanceTypeID(tt.instanceType)
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
			got := generateInstanceID(tt.instanceID)
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
			got := generateAZPoolID(tt.az)
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
			got := generateASGPoolID(tt.asgName)
			assert.Equal(t, tt.want, got)
		}
	})
}

// TestSubscriptions tests subscription CRUD operations.
func TestSubscriptions(t *testing.T) {
	adp := &AWSAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
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
	adp := &AWSAdapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}

	// Add some subscriptions
	adp.subscriptions["sub-1"] = &adapter.Subscription{SubscriptionID: "sub-1"}
	adp.subscriptions["sub-2"] = &adapter.Subscription{SubscriptionID: "sub-2"}

	err := adp.Close()
	assert.NoError(t, err)

	// Verify subscriptions are cleared
	assert.Empty(t, adp.subscriptions)
}

// TestConfigValidation tests configuration validation.
func TestConfigValidation(t *testing.T) {
	t.Run("valid config with all fields", func(t *testing.T) {
		config := &Config{
			Region:              "us-east-1",
			AccessKeyID:         "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey:     "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			SessionToken:        "session-token",
			OCloudID:            "ocloud-test",
			DeploymentManagerID: "dm-test",
			PoolMode:            "asg",
			Timeout:             60 * time.Second,
			Logger:              zap.NewNop(),
		}

		adp, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, adp)
		defer adp.Close()

		assert.Equal(t, "dm-test", adp.deploymentManagerID)
		assert.Equal(t, "asg", adp.poolMode)
	})

	t.Run("valid config with minimal fields", func(t *testing.T) {
		config := &Config{
			Region:   "us-west-2",
			OCloudID: "ocloud-test",
			Logger:   zap.NewNop(),
		}

		adp, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, adp)
		defer adp.Close()

		// Check defaults are applied
		assert.Equal(t, "ocloud-aws-us-west-2", adp.deploymentManagerID)
		assert.Equal(t, "az", adp.poolMode)
	})
}

// BenchmarkMatchesFilter benchmarks the filter matching logic.
func BenchmarkMatchesFilter(b *testing.B) {
	adp := &AWSAdapter{
		logger: zap.NewNop(),
	}

	filter := &adapter.Filter{
		ResourcePoolID: "pool-1",
		ResourceTypeID: "type-1",
		Location:       "us-east-1a",
		Labels: map[string]string{
			"env": "prod",
			"app": "web",
		},
	}

	labels := map[string]string{
		"env":     "prod",
		"app":     "web",
		"version": "1.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adp.matchesFilter(filter, "pool-1", "type-1", "us-east-1a", labels)
	}
}

// BenchmarkApplyPagination benchmarks the pagination logic.
func BenchmarkApplyPagination(b *testing.B) {
	items := make([]string, 1000)
	for i := range items {
		items[i] = "item"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		applyPagination(items, 10, 50)
	}
}
