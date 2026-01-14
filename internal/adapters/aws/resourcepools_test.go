package aws_test

import (
	"context"
	"testing"

	awsadapter "github.com/piwi3910/netweave/internal/adapters/aws"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetResourcePool tests the GetResourcePool method.
func TestGetResourcePool(t *testing.T) {
	tests := []struct {
		name    string
		poolID  string
		wantErr bool
	}{
		{
			name:    "AZ pool ID",
			poolID:  "aws-az-us-east-1a",
			wantErr: true,
		},
		{
			name:    "ASG pool ID",
			poolID:  "aws-asg-my-autoscaling-group",
			wantErr: true,
		},
		{
			name:    "empty pool ID",
			poolID:  "",
			wantErr: true,
		},
		{
			name:    "invalid pool ID format",
			poolID:  "invalid-pool-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := awsadapter.New(&awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "test-cloud",
			})
			require.NoError(t, err)

			pool, err := adp.GetResourcePool(context.Background(), tt.poolID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, pool)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, pool)
			}
		})
	}
}

// TestCreateResourcePool tests the CreateResourcePool method.
func TestCreateResourcePool(t *testing.T) {
	tests := []struct {
		name    string
		pool    *adapter.ResourcePool
		wantErr bool
	}{
		{
			name: "create AZ pool",
			pool: &adapter.ResourcePool{
				Name:        "us-east-1a",
				Description: "Availability Zone pool",
				Extensions: map[string]interface{}{
					"aws.poolType": "az",
				},
			},
			wantErr: true,
		},
		{
			name: "create ASG pool",
			pool: &adapter.ResourcePool{
				Name:        "my-asg",
				Description: "Auto Scaling Group pool",
				Extensions: map[string]interface{}{
					"aws.poolType": "asg",
					"aws.asgName":  "my-autoscaling-group",
				},
			},
			wantErr: true,
		},
		{
			name: "missing pool name",
			pool: &adapter.ResourcePool{
				Description: "Test pool",
			},
			wantErr: true,
		},
		{
			name: "invalid pool type",
			pool: &adapter.ResourcePool{
				Name: "test-pool",
				Extensions: map[string]interface{}{
					"aws.poolType": "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := awsadapter.New(&awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "test-cloud",
			})
			require.NoError(t, err)

			created, err := adp.CreateResourcePool(context.Background(), tt.pool)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, created)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, created)
				assert.NotEmpty(t, created.ResourcePoolID)
			}
		})
	}
}

// TestUpdateResourcePool tests the UpdateResourcePool method.
func TestUpdateResourcePool(t *testing.T) {
	tests := []struct {
		name    string
		poolID  string
		pool    *adapter.ResourcePool
		wantErr bool
	}{
		{
			name:   "update AZ pool",
			poolID: "aws-az-us-east-1a",
			pool: &adapter.ResourcePool{
				Description: "Updated AZ pool description",
			},
			wantErr: true,
		},
		{
			name:   "update ASG pool",
			poolID: "aws-asg-my-autoscaling-group",
			pool: &adapter.ResourcePool{
				Description: "Updated ASG pool description",
				Extensions: map[string]interface{}{
					"aws.minSize": 1,
					"aws.maxSize": 10,
				},
			},
			wantErr: true, // ASG pool updates require AWS credentials
		},
		{
			name:   "empty pool ID",
			poolID: "",
			pool: &adapter.ResourcePool{
				Description: "Test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := awsadapter.New(&awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "test-cloud",
			})
			require.NoError(t, err)

			updated, err := adp.UpdateResourcePool(context.Background(), tt.poolID, tt.pool)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, updated)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, updated)
			}
		})
	}
}

// TestDeleteResourcePool tests the DeleteResourcePool method.
func TestDeleteResourcePool(t *testing.T) {
	tests := []struct {
		name    string
		poolID  string
		wantErr bool
	}{
		{
			name:    "delete AZ pool - not supported",
			poolID:  "aws-az-us-east-1a",
			wantErr: true,
		},
		{
			name:    "delete ASG pool",
			poolID:  "aws-asg-my-autoscaling-group",
			wantErr: true,
		},
		{
			name:    "empty pool ID",
			poolID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := awsadapter.New(&awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "test-cloud",
			})
			require.NoError(t, err)

			err = adp.DeleteResourcePool(context.Background(), tt.poolID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
