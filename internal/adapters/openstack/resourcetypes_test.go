package openstack_test

import (
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/piwi3910/netweave/internal/adapters/openstack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestTransformFlavorToResourceType tests the transformation from OpenStack flavor to O2-IMS resource type.
func TestTransformFlavorToResourceType(t *testing.T) {
	adapter := &openstack.Adapter{
		Logger: zap.NewNop(),
	}

	osFlavor := &flavors.Flavor{
		ID:         "m1.small",
		Name:       "m1.small",
		VCPUs:      2,
		RAM:        2048,
		Disk:       20,
		Swap:       512,
		Ephemeral:  10,
		IsPublic:   true,
		RxTxFactor: 1.0,
	}

	resourceType := adapter.TransformFlavorToResourceType(osFlavor)

	// Test basic fields
	assert.Equal(t, "openstack-flavor-m1.small", resourceType.ResourceTypeID)
	assert.Equal(t, "m1.small", resourceType.Name)
	assert.Contains(t, resourceType.Description, "m1.small")
	assert.Contains(t, resourceType.Description, "vCPUs: 2")
	assert.Contains(t, resourceType.Description, "RAM: 2048MB")
	assert.Contains(t, resourceType.Description, "Disk: 20GB")
	assert.Equal(t, "OpenStack", resourceType.Vendor)
	assert.Equal(t, "m1.small", resourceType.Model)
	assert.Equal(t, "compute", resourceType.ResourceClass)
	assert.Equal(t, "virtual", resourceType.ResourceKind)

	// Test extensions
	require.NotNil(t, resourceType.Extensions)
	assert.Equal(t, "m1.small", resourceType.Extensions["openstack.flavorId"])
	assert.Equal(t, "m1.small", resourceType.Extensions["openstack.name"])
	assert.Equal(t, 2, resourceType.Extensions["openstack.vcpus"])
	assert.Equal(t, 2048, resourceType.Extensions["openstack.ram"])
	assert.Equal(t, 20, resourceType.Extensions["openstack.disk"])
	assert.Equal(t, 512, resourceType.Extensions["openstack.swap"])
	assert.Equal(t, 10, resourceType.Extensions["openstack.ephemeral"])
	assert.Equal(t, true, resourceType.Extensions["openstack.isPublic"])
	assert.Equal(t, 1.0, resourceType.Extensions["openstack.rxtxFactor"])

	// Extra specs are not currently supported in basic flavor transformation
	_, ok := resourceType.Extensions["openstack.extraSpecs"]
	assert.False(t, ok, "extra specs require separate API calls and are not in basic transformation")
}

// TestTransformFlavorToResourceTypeMinimal tests transformation with minimal data.
func TestTransformFlavorToResourceTypeMinimal(t *testing.T) {
	adapter := &openstack.Adapter{
		Logger: zap.NewNop(),
	}

	osFlavor := &flavors.Flavor{
		ID:   "minimal-flavor",
		Name: "minimal",
	}

	resourceType := adapter.TransformFlavorToResourceType(osFlavor)

	assert.Equal(t, "openstack-flavor-minimal-flavor", resourceType.ResourceTypeID)
	assert.Equal(t, "minimal", resourceType.Name)
	assert.Equal(t, "compute", resourceType.ResourceClass)
	assert.Equal(t, "virtual", resourceType.ResourceKind)
}

// TestTransformFlavorToResourceTypeStorageClass tests resource class determination.
func TestTransformFlavorToResourceTypeStorageClass(t *testing.T) {
	adapter := &openstack.Adapter{
		Logger: zap.NewNop(),
	}

	tests := []struct {
		name      string
		flavor    *flavors.Flavor
		wantClass string
	}{
		{
			name: "compute flavor",
			flavor: &flavors.Flavor{
				ID:    "compute-flavor",
				Name:  "compute",
				VCPUs: 4,
				RAM:   8192,
			},
			wantClass: "compute",
		},
		{
			name: "storage flavor (zero CPU and RAM)",
			flavor: &flavors.Flavor{
				ID:    "storage-flavor",
				Name:  "storage",
				VCPUs: 0,
				RAM:   0,
				Disk:  1000,
			},
			wantClass: "storage",
		},
		{
			name: "default to compute",
			flavor: &flavors.Flavor{
				ID:   "default-flavor",
				Name: "default",
			},
			wantClass: "compute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceType := adapter.TransformFlavorToResourceType(tt.flavor)
			assert.Equal(t, tt.wantClass, resourceType.ResourceClass)
		})
	}
}

// TestResourceTypeIDGeneration tests resource type ID generation.
func TestResourceTypeIDGeneration(t *testing.T) {
	tests := []struct {
		name     string
		flavorID string
		want     string
	}{
		{
			name:     "simple flavor ID",
			flavorID: "m1.small",
			want:     "openstack-flavor-m1.small",
		},
		{
			name:     "UUID flavor ID",
			flavorID: "550e8400-e29b-41d4-a716-446655440000",
			want:     "openstack-flavor-550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "flavor with special characters",
			flavorID: "custom.flavor_123",
			want:     "openstack-flavor-custom.flavor_123",
		},
		{
			name:     "numeric flavor ID",
			flavorID: "12345",
			want:     "openstack-flavor-12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := &flavors.Flavor{ID: tt.flavorID}
			got := openstack.GenerateFlavorID(flavor)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResourceTypeIDParsing tests parsing resource type IDs.
func TestResourceTypeIDParsing(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantID  string
		wantErr bool
	}{
		{
			name:    "valid simple ID",
			id:      "openstack-flavor-m1.small",
			wantID:  "m1.small",
			wantErr: false,
		},
		{
			name:    "valid UUID",
			id:      "openstack-flavor-550e8400-e29b-41d4-a716-446655440000",
			wantID:  "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "invalid format - missing prefix",
			id:      "flavor-m1.small",
			wantErr: true,
		},
		{
			name:    "invalid format - wrong prefix",
			id:      "openstack-type-m1.small",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flavorID string
			n, err := fmt.Sscanf(tt.id, "openstack-flavor-%s", &flavorID)

			if tt.wantErr {
				assert.True(t, err != nil || n == 0)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, 1, n)
				assert.Equal(t, tt.wantID, flavorID)
			}
		})
	}
}

// TestFlavorDescriptionGeneration tests description generation.
func TestFlavorDescriptionGeneration(t *testing.T) {
	adapter := &openstack.Adapter{
		Logger: zap.NewNop(),
	}

	tests := []struct {
		name   string
		flavor *flavors.Flavor
		want   []string // Substrings that should be in description
	}{
		{
			name: "full specs",
			flavor: &flavors.Flavor{
				Name:  "m1.large",
				VCPUs: 4,
				RAM:   8192,
				Disk:  80,
			},
			want: []string{"m1.large", "vCPUs: 4", "RAM: 8192MB", "Disk: 80GB"},
		},
		{
			name: "minimal specs",
			flavor: &flavors.Flavor{
				Name: "minimal",
			},
			want: []string{"minimal", "vCPUs: 0", "RAM: 0MB", "Disk: 0GB"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceType := adapter.TransformFlavorToResourceType(tt.flavor)
			for _, substr := range tt.want {
				assert.Contains(t, resourceType.Description, substr)
			}
		})
	}
}

// TestFlavorExtraSpecs tests extra specs handling.
func TestFlavorExtraSpecs(t *testing.T) {
	adapter := &openstack.Adapter{
		Logger: zap.NewNop(),
	}

	t.Run("flavor with extra specs", func(t *testing.T) {
		flavor := &flavors.Flavor{
			ID:    "spec-flavor",
			Name:  "with-specs",
			VCPUs: 4,
			RAM:   8192,
			Disk:  100,
		}

		resourceType := adapter.TransformFlavorToResourceType(flavor)

		// Extra specs are not currently supported in the basic transformation
		// as they require separate API calls using flavors/extraspecs package
		_, ok := resourceType.Extensions["openstack.extraSpecs"]
		assert.False(t, ok, "extra specs should not be present in basic flavor transformation")

		// Verify basic flavor metadata is present
		assert.Equal(t, "spec-flavor", resourceType.Extensions["openstack.flavorId"])
		assert.Equal(t, "with-specs", resourceType.Extensions["openstack.name"])
		assert.Equal(t, 4, resourceType.Extensions["openstack.vcpus"])
	})

	t.Run("flavor without extra specs", func(t *testing.T) {
		flavor := &flavors.Flavor{
			ID:   "no-spec-flavor",
			Name: "without-specs",
		}

		resourceType := adapter.TransformFlavorToResourceType(flavor)

		// Extra specs should not be present in extensions
		_, ok := resourceType.Extensions["openstack.extraSpecs"]
		assert.False(t, ok)
	})

	t.Run("flavor with empty extra specs", func(t *testing.T) {
		flavor := &flavors.Flavor{
			ID:   "empty-spec-flavor",
			Name: "empty-specs",
		}

		resourceType := adapter.TransformFlavorToResourceType(flavor)

		// Empty extra specs should not be present in extensions
		_, ok := resourceType.Extensions["openstack.extraSpecs"]
		assert.False(t, ok)
	})
}

// BenchmarkTransformFlavorToResourceType benchmarks the transformation.
func BenchmarkTransformFlavorToResourceType(b *testing.B) {
	adp := &openstack.Adapter{
		OCloudID: "ocloud-test",
		Logger:   zap.NewNop(),
	}

	osFlavor := &flavors.Flavor{
		ID:         "m1.small",
		Name:       "m1.small",
		VCPUs:      2,
		RAM:        2048,
		Disk:       20,
		Swap:       512,
		Ephemeral:  10,
		IsPublic:   true,
		RxTxFactor: 1.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adp.TransformFlavorToResourceType(osFlavor)
	}
}

// BenchmarkGenerateFlavorID benchmarks flavor ID generation.
func BenchmarkGenerateFlavorID(b *testing.B) {
	flavor := &flavors.Flavor{
		ID: "m1.small",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		openstack.GenerateFlavorID(flavor)
	}
}
