package azure_test

import (
	"context"
	"testing"

	azureadapter "github.com/piwi3910/netweave/internal/adapters/azure"

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
			poolID:  "azure-az-eastus-1",
			wantErr: true,
		},
		{
			name:    "RG pool ID",
			poolID:  "azure-rg-my-resource-group",
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
			adp, err := azureadapter.New(&azureadapter.Config{
				Location:       "eastus",
				OCloudID:       "test-cloud",
				SubscriptionID: "test-subscription-id",
				TenantID:       "test-tenant-id",
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
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
			name: "create RG pool",
			pool: &adapter.ResourcePool{
				Name:        "my-resource-group",
				Description: "Resource Group pool",
				Extensions: map[string]interface{}{
					"azure.poolType": "rg",
				},
			},
			wantErr: true,
		},
		{
			name: "create AZ pool",
			pool: &adapter.ResourcePool{
				Name:        "eastus-1",
				Description: "Availability Zone pool",
				Extensions: map[string]interface{}{
					"azure.poolType": "az",
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
					"azure.poolType": "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := azureadapter.New(&azureadapter.Config{
				Location:       "eastus",
				OCloudID:       "test-cloud",
				SubscriptionID: "test-subscription-id",
				TenantID:       "test-tenant-id",
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
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
			name:   "update RG pool",
			poolID: "azure-rg-my-resource-group",
			pool: &adapter.ResourcePool{
				Description: "Updated RG pool description",
			},
			wantErr: true,
		},
		{
			name:   "update AZ pool",
			poolID: "azure-az-eastus-1",
			pool: &adapter.ResourcePool{
				Description: "Updated AZ pool description",
			},
			wantErr: true,
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
			adp, err := azureadapter.New(&azureadapter.Config{
				Location:       "eastus",
				OCloudID:       "test-cloud",
				SubscriptionID: "test-subscription-id",
				TenantID:       "test-tenant-id",
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
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
			name:    "delete RG pool",
			poolID:  "azure-rg-my-resource-group",
			wantErr: true,
		},
		{
			name:    "delete AZ pool - not supported",
			poolID:  "azure-az-eastus-1",
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
			adp, err := azureadapter.New(&azureadapter.Config{
				Location:       "eastus",
				OCloudID:       "test-cloud",
				SubscriptionID: "test-subscription-id",
				TenantID:       "test-tenant-id",
				ClientID:       "test-client-id",
				ClientSecret:   "test-client-secret",
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
