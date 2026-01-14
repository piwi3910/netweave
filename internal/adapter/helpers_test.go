package adapter_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
)

// TestMatchesFilter tests the shared filter matching logic.
func TestMatchesFilter(t *testing.T) {
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
			got := adapter.MatchesFilter(
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

// TestApplyPagination tests the shared pagination logic.
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
			got := adapter.ApplyPagination(items, tt.limit, tt.offset)
			assert.Equal(t, tt.want, got)
		})
	}
}

// BenchmarkMatchesFilter benchmarks the filter matching logic.
func BenchmarkMatchesFilter(b *testing.B) {
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
		adapter.MatchesFilter(filter, "pool-1", "type-1", "us-east-1a", labels)
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
		adapter.ApplyPagination(items, 10, 50)
	}
}
