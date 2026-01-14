package gcp_test

import (
	"context"
	"testing"

	gcpadapter "github.com/piwi3910/netweave/internal/adapters/gcp"

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
			name:    "zone pool ID",
			poolID:  "gcp-zone-us-central1-a",
			wantErr: true,
		},
		{
			name:    "IG pool ID",
			poolID:  "gcp-ig-my-instance-group",
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
			adp, err := gcpadapter.New(&gcpadapter.Config{
				ProjectID: "test-project",
				Region:    "us-central1",
				OCloudID:  "test-cloud",
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
			name: "create zone pool",
			pool: &adapter.ResourcePool{
				Name:        "us-central1-a",
				Description: "Zone pool",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := gcpadapter.New(&gcpadapter.Config{
				ProjectID: "test-project",
				Region:    "us-central1",
				OCloudID:  "test-cloud",
			})
			require.NoError(t, err)

			created, err := adp.CreateResourcePool(context.Background(), tt.pool)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, created)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, created)
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
			name:   "update zone pool",
			poolID: "gcp-zone-us-central1-a",
			pool: &adapter.ResourcePool{
				Description: "Updated zone pool",
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
			adp, err := gcpadapter.New(&gcpadapter.Config{
				ProjectID: "test-project",
				Region:    "us-central1",
				OCloudID:  "test-cloud",
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
			name:    "delete zone pool",
			poolID:  "gcp-zone-us-central1-a",
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
			adp, err := gcpadapter.New(&gcpadapter.Config{
				ProjectID: "test-project",
				Region:    "us-central1",
				OCloudID:  "test-cloud",
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
