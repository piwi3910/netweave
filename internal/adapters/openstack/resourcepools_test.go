package openstack

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/aggregates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestTransformHostAggregateToResourcePool tests the transformation from
// OpenStack host aggregate to O2-IMS resource pool.
func TestTransformHostAggregateToResourcePool(t *testing.T) {
	adp := &OpenStackAdapter{
		oCloudID: "ocloud-test",
		logger:   zap.NewNop(),
	}

	now := time.Now()
	osAggregate := &aggregates.Aggregate{
		ID:               42,
		Name:             "test-aggregate",
		AvailabilityZone: "zone-1",
		Hosts:            []string{"host1", "host2", "host3"},
		Metadata: map[string]string{
			"ssd": "true",
			"env": "production",
		},
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now,
	}

	pool := adp.transformHostAggregateToResourcePool(osAggregate)

	// Test basic fields
	assert.Equal(t, "openstack-aggregate-42", pool.ResourcePoolID)
	assert.Equal(t, "test-aggregate", pool.Name)
	assert.Equal(t, "OpenStack host aggregate: test-aggregate", pool.Description)
	assert.Equal(t, "zone-1", pool.Location)
	assert.Equal(t, "ocloud-test", pool.OCloudID)

	// Test extensions
	require.NotNil(t, pool.Extensions)
	assert.Equal(t, 42, pool.Extensions["openstack.aggregateId"])
	assert.Equal(t, "test-aggregate", pool.Extensions["openstack.name"])
	assert.Equal(t, "zone-1", pool.Extensions["openstack.availabilityZone"])
	assert.Equal(t, 3, pool.Extensions["openstack.hostCount"])

	// Test metadata
	metadata, ok := pool.Extensions["openstack.metadata"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "true", metadata["ssd"])
	assert.Equal(t, "production", metadata["env"])

	// Test hosts
	hosts, ok := pool.Extensions["openstack.hosts"].([]string)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"host1", "host2", "host3"}, hosts)
}

// TestTransformHostAggregateToResourcePoolEmpty tests transformation with minimal data.
func TestTransformHostAggregateToResourcePoolEmpty(t *testing.T) {
	adp := &OpenStackAdapter{
		oCloudID: "ocloud-test",
		logger:   zap.NewNop(),
	}

	osAggregate := &aggregates.Aggregate{
		ID:               1,
		Name:             "minimal-aggregate",
		AvailabilityZone: "",
		Hosts:            []string{},
		Metadata:         map[string]string{},
	}

	pool := adp.transformHostAggregateToResourcePool(osAggregate)

	assert.Equal(t, "openstack-aggregate-1", pool.ResourcePoolID)
	assert.Equal(t, "minimal-aggregate", pool.Name)
	assert.Equal(t, "", pool.Location)
	assert.Equal(t, 0, pool.Extensions["openstack.hostCount"])
}

// TestResourcePoolIDParsing tests parsing resource pool IDs.
func TestResourcePoolIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantID  int
		wantErr bool
	}{
		{
			name:    "valid ID",
			id:      "openstack-aggregate-42",
			wantID:  42,
			wantErr: false,
		},
		{
			name:    "valid ID with large number",
			id:      "openstack-aggregate-123456",
			wantID:  123456,
			wantErr: false,
		},
		{
			name:    "invalid format - missing prefix",
			id:      "aggregate-42",
			wantErr: true,
		},
		{
			name:    "invalid format - wrong prefix",
			id:      "openstack-pool-42",
			wantErr: true,
		},
		{
			name:    "invalid format - no number",
			id:      "openstack-aggregate-",
			wantErr: true,
		},
		{
			name:    "invalid format - not a number",
			id:      "openstack-aggregate-abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var aggregateID int
			_, err := fmt.Sscanf(tt.id, "openstack-aggregate-%d", &aggregateID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantID, aggregateID)
			}
		})
	}
}

// TestListResourcePoolsFilter tests filtering logic for ListResourcePools.
func TestListResourcePoolsFilter(t *testing.T) {
	adp := &OpenStackAdapter{
		oCloudID: "ocloud-test",
		logger:   zap.NewNop(),
	}

	// Create test aggregates
	aggregates := []*aggregates.Aggregate{
		{
			ID:               1,
			Name:             "pool-zone1",
			AvailabilityZone: "zone-1",
		},
		{
			ID:               2,
			Name:             "pool-zone2",
			AvailabilityZone: "zone-2",
		},
		{
			ID:               3,
			Name:             "pool-zone1-2",
			AvailabilityZone: "zone-1",
		},
	}

	t.Run("no filter returns all", func(t *testing.T) {
		count := 0
		for _, agg := range aggregates {
			pool := adp.transformHostAggregateToResourcePool(agg)
			if adapter.MatchesFilter(nil, pool.ResourcePoolID, "", pool.Location, nil) {
				count++
			}
		}
		assert.Equal(t, 3, count)
	})

	t.Run("filter by location", func(t *testing.T) {
		filter := &adapter.Filter{
			Location: "zone-1",
		}

		count := 0
		for _, agg := range aggregates {
			pool := adp.transformHostAggregateToResourcePool(agg)
			if adapter.MatchesFilter(filter, pool.ResourcePoolID, "", pool.Location, nil) {
				count++
			}
		}
		assert.Equal(t, 2, count) // pool-zone1 and pool-zone1-2
	})

	t.Run("filter by non-existent location", func(t *testing.T) {
		filter := &adapter.Filter{
			Location: "zone-3",
		}

		count := 0
		for _, agg := range aggregates {
			pool := adp.transformHostAggregateToResourcePool(agg)
			if adapter.MatchesFilter(filter, pool.ResourcePoolID, "", pool.Location, nil) {
				count++
			}
		}
		assert.Equal(t, 0, count)
	})
}

// TestListResourcePoolsPagination tests pagination for ListResourcePools.
func TestListResourcePoolsPagination(t *testing.T) {
	// Create test pools
	pools := make([]*adapter.ResourcePool, 10)
	for i := range pools {
		pools[i] = &adapter.ResourcePool{
			ResourcePoolID: fmt.Sprintf("pool-%d", i),
			Name:           fmt.Sprintf("Pool %d", i),
		}
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
	}{
		{
			name:      "no pagination",
			limit:     0,
			offset:    0,
			wantCount: 10,
		},
		{
			name:      "limit 5",
			limit:     5,
			offset:    0,
			wantCount: 5,
		},
		{
			name:      "offset 3, no limit",
			limit:     0,
			offset:    3,
			wantCount: 7,
		},
		{
			name:      "limit 3, offset 2",
			limit:     3,
			offset:    2,
			wantCount: 3,
		},
		{
			name:      "offset beyond items",
			limit:     5,
			offset:    15,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.ApplyPagination(pools, tt.limit, tt.offset)
			assert.Len(t, result, tt.wantCount)
		})
	}
}

// TestCreateResourcePoolValidation tests validation for CreateResourcePool.
func TestCreateResourcePoolValidation(t *testing.T) {
	adp := &OpenStackAdapter{
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	t.Run("empty name fails", func(t *testing.T) {
		pool := &adapter.ResourcePool{
			Name: "",
		}

		_, err := adp.CreateResourcePool(ctx, pool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

// BenchmarkTransformHostAggregateToResourcePool benchmarks the transformation.
func BenchmarkTransformHostAggregateToResourcePool(b *testing.B) {
	adp := &OpenStackAdapter{
		oCloudID: "ocloud-test",
		logger:   zap.NewNop(),
	}

	now := time.Now()
	osAggregate := &aggregates.Aggregate{
		ID:               42,
		Name:             "test-aggregate",
		AvailabilityZone: "zone-1",
		Hosts:            []string{"host1", "host2", "host3"},
		Metadata: map[string]string{
			"ssd": "true",
			"env": "production",
		},
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adp.transformHostAggregateToResourcePool(osAggregate)
	}
}
