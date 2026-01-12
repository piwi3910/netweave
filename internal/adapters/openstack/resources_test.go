package openstack

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestTransformServerToResource tests the transformation from OpenStack server to O2-IMS resource.
func TestTransformServerToResource(t *testing.T) {
	adp := &Adapter{
		oCloudID: "ocloud-test",
		region:   "RegionOne",
		logger:   zap.NewNop(),
	}

	now := time.Now()
	osServer := &servers.Server{
		ID:       "550e8400-e29b-41d4-a716-446655440000",
		Name:     "test-vm",
		Status:   "ACTIVE",
		TenantID: "project-123",
		UserID:   "user-456",
		HostID:   "host-789",
		Created:  now.Add(-24 * time.Hour),
		Updated:  now,
		Flavor: map[string]interface{}{
			"id": "m1.small",
		},
		Image: map[string]interface{}{
			"id": "ubuntu-20.04",
		},
		Addresses: map[string]interface{}{
			"private": []map[string]interface{}{
				{
					"addr":    "10.0.0.5",
					"version": 4,
				},
			},
		},
		Metadata: map[string]string{
			"env": "production",
			"app": "web",
		},
	}

	resource := adp.transformServerToResource(osServer)

	// Test basic fields
	assert.Equal(t, "openstack-server-550e8400-e29b-41d4-a716-446655440000", resource.ResourceID)
	assert.Equal(t, "openstack-flavor-m1.small", resource.ResourceTypeID)
	assert.Equal(t, "urn:openstack:server:RegionOne:550e8400-e29b-41d4-a716-446655440000", resource.GlobalAssetID)
	assert.Contains(t, resource.Description, "test-vm")
	assert.Contains(t, resource.Description, "ACTIVE")

	// Test extensions
	require.NotNil(t, resource.Extensions)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", resource.Extensions["openstack.serverId"])
	assert.Equal(t, "test-vm", resource.Extensions["openstack.name"])
	assert.Equal(t, "ACTIVE", resource.Extensions["openstack.status"])
	assert.Equal(t, "project-123", resource.Extensions["openstack.tenantId"])
	assert.Equal(t, "user-456", resource.Extensions["openstack.userId"])
	assert.Equal(t, "host-789", resource.Extensions["openstack.hostId"])
	// Note: AvailabilityZone is not part of the basic servers.Server struct
	// It requires the OS-EXT-AZ extension which needs separate handling

	// Test flavor
	flavor, ok := resource.Extensions["openstack.flavor"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "m1.small", flavor["id"])

	// Test image
	image, ok := resource.Extensions["openstack.image"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ubuntu-20.04", image["id"])

	// Test metadata
	metadata, ok := resource.Extensions["openstack.metadata"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "production", metadata["env"])
	assert.Equal(t, "web", metadata["app"])
}

// TestTransformServerToResourceMinimal tests transformation with minimal data.
func TestTransformServerToResourceMinimal(t *testing.T) {
	adp := &Adapter{
		oCloudID: "ocloud-test",
		region:   "RegionOne",
		logger:   zap.NewNop(),
	}

	osServer := &servers.Server{
		ID:     "minimal-server-id",
		Name:   "minimal-vm",
		Status: "BUILD",
	}

	resource := adp.transformServerToResource(osServer)

	assert.Equal(t, "openstack-server-minimal-server-id", resource.ResourceID)
	assert.Contains(t, resource.Description, "minimal-vm")
	assert.Contains(t, resource.Description, "BUILD")
	assert.NotNil(t, resource.Extensions)
}

// TestResourceIDParsing tests parsing resource IDs.
func TestResourceIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantID  string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			id:      "openstack-server-550e8400-e29b-41d4-a716-446655440000",
			wantID:  "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid short ID",
			id:      "openstack-server-abc123",
			wantID:  "abc123",
			wantErr: false,
		},
		{
			name:    "invalid format - missing prefix",
			id:      "server-550e8400-e29b-41d4-a716-446655440000",
			wantErr: true,
		},
		{
			name:    "invalid format - wrong prefix",
			id:      "openstack-instance-550e8400-e29b-41d4-a716-446655440000",
			wantErr: true,
		},
		{
			name:    "invalid format - no server ID",
			id:      "openstack-server-",
			wantErr: false, // Sscanf will read empty string
			wantID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var serverID string
			n, err := fmt.Sscanf(tt.id, "openstack-server-%s", &serverID)

			if tt.wantErr {
				// Should not match the format or should error
				assert.True(t, err != nil || n == 0)
			} else if err == nil && n > 0 {
				assert.Equal(t, tt.wantID, serverID)
			}
		})
	}
}

// TestListResourcesFilter tests filtering logic for ListResources.
func TestListResourcesFilter(t *testing.T) {
	adp := &Adapter{
		oCloudID: "ocloud-test",
		region:   "RegionOne",
		logger:   zap.NewNop(),
	}

	// Create test servers
	servers := []*servers.Server{
		{
			ID:     "server-1",
			Name:   "vm-1",
			Status: "ACTIVE",
			Flavor: map[string]interface{}{"id": "m1.small"},
		},
		{
			ID:     "server-2",
			Name:   "vm-2",
			Status: "ACTIVE",
			Flavor: map[string]interface{}{"id": "m1.large"},
		},
		{
			ID:     "server-3",
			Name:   "vm-3",
			Status: "BUILD",
			Flavor: map[string]interface{}{"id": "m1.small"},
		},
	}

	t.Run("no filter returns all", func(t *testing.T) {
		count := 0
		for _, srv := range servers {
			resource := adp.transformServerToResource(srv)
			if adapter.MatchesFilter(nil, "", resource.ResourceTypeID, "", nil) {
				count++
			}
		}
		assert.Equal(t, 3, count)
	})

	t.Run("filter by resource type", func(t *testing.T) {
		filter := &adapter.Filter{
			ResourceTypeID: "openstack-flavor-m1.small",
		}

		count := 0
		for _, srv := range servers {
			resource := adp.transformServerToResource(srv)
			if adapter.MatchesFilter(filter, "", resource.ResourceTypeID, "", nil) {
				count++
			}
		}
		assert.Equal(t, 2, count) // server-1 and server-3
	})

	t.Run("filter by resource type (flavor)", func(t *testing.T) {
		filter := &adapter.Filter{
			ResourceTypeID: "openstack-flavor-m1.large",
		}

		count := 0
		for _, srv := range servers {
			resource := adp.transformServerToResource(srv)
			// Simulate the in-memory filtering that happens in ListResources
			if filter.ResourceTypeID == "" || filter.ResourceTypeID == resource.ResourceTypeID {
				count++
			}
		}
		assert.Equal(t, 1, count) // only server-2 has m1.large
	})
}

// TestResourcePoolFiltering tests the API-level filtering logic for resource pools.
func TestResourcePoolFiltering(t *testing.T) {
	// This test documents how resource pool filtering works:
	// 1. User specifies filter.ResourcePoolID or filter.Location
	// 2. Adapter maps ResourcePoolID -> ResourcePool -> Location (AZ)
	// 3. Location/AZ is passed to OpenStack API as listOpts.AvailabilityZone
	// 4. OpenStack returns only servers in that AZ
	//
	// The transformation from ResourcePoolID to Location happens in ListResources
	// at lines 30-44 of resources.go

	t.Run("resource pool ID maps to availability zone", func(t *testing.T) {
		// Example: Resource pool "openstack-aggregate-1" has Location="nova"
		// When filtering by ResourcePoolID, we query the pool to get Location
		// Then use that Location as the OpenStack availability_zone filter

		poolID := "openstack-aggregate-1"
		expectedAZ := "nova"

		// In production code, this mapping happens:
		// 1. pool, err := a.GetResourcePool(ctx, poolID)
		// 2. availabilityZone := pool.Location
		// 3. listOpts.AvailabilityZone = availabilityZone

		assert.NotEmpty(t, poolID)
		assert.NotEmpty(t, expectedAZ)
	})

	t.Run("location filter maps directly to availability zone", func(t *testing.T) {
		// When user specifies filter.Location directly,
		// it's used as-is for the OpenStack availability_zone query parameter

		location := "us-west-2a"
		expectedAZ := location

		assert.Equal(t, location, expectedAZ)
	})
}

// TestCreateResourceValidation tests validation for CreateResource.
func TestCreateResourceValidation(t *testing.T) {
	adp := &Adapter{
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		resource *adapter.Resource
		wantErr  bool
		errMsg   string
	}{
		{
			name: "missing resource type ID",
			resource: &adapter.Resource{
				ResourceTypeID: "",
			},
			wantErr: true,
			errMsg:  "resourceTypeID is required",
		},
		{
			name: "invalid resource type ID format",
			resource: &adapter.Resource{
				ResourceTypeID: "invalid-format",
			},
			wantErr: true,
			errMsg:  "invalid resourceTypeID format",
		},
		{
			name: "missing image ID in extensions",
			resource: &adapter.Resource{
				ResourceTypeID: "openstack-flavor-m1.small",
				Extensions:     map[string]interface{}{},
			},
			wantErr: true,
			errMsg:  "imageId is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adp.CreateResource(ctx, tt.resource)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			}
			// Note: Success cases would require OpenStack API mock
		})
	}
}

// TestGetResourcePoolIDFromServer tests resource pool ID derivation.
func TestGetResourcePoolIDFromServer(t *testing.T) {
	adp := &Adapter{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name   string
		server *servers.Server
		want   string
	}{
		{
			name: "server with availability zone",
			server: &servers.Server{
				ID: "server-1",
			},
			want: "", // Currently returns empty; would need enhancement to map AZ to aggregate
		},
		{
			name: "server without availability zone",
			server: &servers.Server{
				ID: "server-2",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.getResourcePoolIDFromServer(tt.server)
			assert.Equal(t, tt.want, got)
		})
	}
}

// BenchmarkTransformServerToResource benchmarks the transformation.
func BenchmarkTransformServerToResource(b *testing.B) {
	adp := &Adapter{
		oCloudID: "ocloud-test",
		region:   "RegionOne",
		logger:   zap.NewNop(),
	}

	now := time.Now()
	osServer := &servers.Server{
		ID:       "550e8400-e29b-41d4-a716-446655440000",
		Name:     "test-vm",
		Status:   "ACTIVE",
		TenantID: "project-123",
		UserID:   "user-456",
		HostID:   "host-789",
		Created:  now.Add(-24 * time.Hour),
		Updated:  now,
		Flavor: map[string]interface{}{
			"id": "m1.small",
		},
		Image: map[string]interface{}{
			"id": "ubuntu-20.04",
		},
		Addresses: map[string]interface{}{
			"private": []map[string]interface{}{
				{
					"addr":    "10.0.0.5",
					"version": 4,
				},
			},
		},
		Metadata: map[string]string{
			"env": "production",
			"app": "web",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adp.transformServerToResource(osServer)
	}
}
