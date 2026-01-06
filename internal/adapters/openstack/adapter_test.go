package openstack

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// TestNew tests the creation of a new OpenStackAdapter
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
			name: "missing authURL",
			config: &Config{
				Username:    "admin",
				Password:    "password",
				ProjectName: "test",
				Region:      "RegionOne",
				OCloudID:    "ocloud-1",
			},
			wantErr: true,
			errMsg:  "authURL is required",
		},
		{
			name: "missing username",
			config: &Config{
				AuthURL:     "https://openstack.example.com:5000/v3",
				Password:    "password",
				ProjectName: "test",
				Region:      "RegionOne",
				OCloudID:    "ocloud-1",
			},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name: "missing password",
			config: &Config{
				AuthURL:     "https://openstack.example.com:5000/v3",
				Username:    "admin",
				ProjectName: "test",
				Region:      "RegionOne",
				OCloudID:    "ocloud-1",
			},
			wantErr: true,
			errMsg:  "password is required",
		},
		{
			name: "missing projectName",
			config: &Config{
				AuthURL:  "https://openstack.example.com:5000/v3",
				Username: "admin",
				Password: "password",
				Region:   "RegionOne",
				OCloudID: "ocloud-1",
			},
			wantErr: true,
			errMsg:  "projectName is required",
		},
		{
			name: "missing region",
			config: &Config{
				AuthURL:     "https://openstack.example.com:5000/v3",
				Username:    "admin",
				Password:    "password",
				ProjectName: "test",
				OCloudID:    "ocloud-1",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "missing oCloudID",
			config: &Config{
				AuthURL:     "https://openstack.example.com:5000/v3",
				Username:    "admin",
				Password:    "password",
				ProjectName: "test",
				Region:      "RegionOne",
			},
			wantErr: true,
			errMsg:  "oCloudID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := New(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adapter)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, adapter)
			}
		})
	}
}

// TestNewWithDefaults tests default value initialization
func TestNewWithDefaults(t *testing.T) {
	// Skip if no OpenStack credentials available
	if os.Getenv("OPENSTACK_AUTH_URL") == "" {
		t.Skip("Skipping test: OPENSTACK_AUTH_URL not set")
	}

	config := &Config{
		AuthURL:     os.Getenv("OPENSTACK_AUTH_URL"),
		Username:    os.Getenv("OPENSTACK_USERNAME"),
		Password:    os.Getenv("OPENSTACK_PASSWORD"),
		ProjectName: os.Getenv("OPENSTACK_PROJECT"),
		Region:      os.Getenv("OPENSTACK_REGION"),
		OCloudID:    "test-ocloud",
		// DomainName not set - should default to "Default"
		// DeploymentManagerID not set - should be auto-generated
		// Timeout not set - should default to 30s
	}

	adapter, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, adapter)
	defer adapter.Close()

	// Check defaults
	assert.Equal(t, "test-ocloud", adapter.oCloudID)
	assert.NotEmpty(t, adapter.deploymentManagerID)
	assert.Contains(t, adapter.deploymentManagerID, "ocloud-openstack-")
}

// TestMetadata tests metadata methods
func TestMetadata(t *testing.T) {
	a := &OpenStackAdapter{
		logger: zap.NewNop(),
	}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "openstack", a.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.NotEmpty(t, a.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := a.Capabilities()
		assert.NotEmpty(t, caps)
		// Import the adapter package to access capability constants
		assert.Len(t, caps, 6) // Should have 6 capabilities
	})
}

// TestMatchesFilter tests the filter matching logic
func TestMatchesFilter(t *testing.T) {
	a := &OpenStackAdapter{
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
				Location: "zone-1",
			},
			location: "zone-1",
			want:     true,
		},
		{
			name: "location filter doesn't match",
			filter: &adapter.Filter{
				Location: "zone-1",
			},
			location: "zone-2",
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
				Location:       "zone-1",
			},
			resourcePoolID: "pool-1",
			location:       "zone-1",
			want:           true,
		},
		{
			name: "multiple filters one doesn't match",
			filter: &adapter.Filter{
				ResourcePoolID: "pool-1",
				Location:       "zone-1",
			},
			resourcePoolID: "pool-1",
			location:       "zone-2",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.matchesFilter(
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

// TestApplyPagination tests the pagination logic
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

// TestGenerateFlavorID tests flavor ID generation
func TestGenerateFlavorID(t *testing.T) {
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
			name:     "empty flavor ID",
			flavorID: "",
			want:     "openstack-flavor-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := &flavors.Flavor{ID: tt.flavorID}
			got := generateFlavorID(flavor)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestClose tests adapter cleanup
func TestClose(t *testing.T) {
	adapter := &OpenStackAdapter{
		logger: zap.NewNop(),
	}

	err := adapter.Close()
	assert.NoError(t, err)
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	t.Run("valid config with all fields", func(t *testing.T) {
		config := &Config{
			AuthURL:             "https://openstack.example.com:5000/v3",
			Username:            "admin",
			Password:            "password",
			ProjectName:         "test-project",
			DomainName:          "TestDomain",
			Region:              "RegionOne",
			OCloudID:            "ocloud-test",
			DeploymentManagerID: "dm-test",
			Timeout:             60 * time.Second,
		}

		// Just test that validation passes (actual connection will fail in test)
		_, err := New(config)
		// We expect an error here because we're not connecting to real OpenStack
		// but the validation should have passed
		if err != nil {
			assert.Contains(t, err.Error(), "OpenStack")
		}
	})

	t.Run("valid config with minimal fields", func(t *testing.T) {
		config := &Config{
			AuthURL:     "https://openstack.example.com:5000/v3",
			Username:    "admin",
			Password:    "password",
			ProjectName: "test-project",
			Region:      "RegionOne",
			OCloudID:    "ocloud-test",
		}

		// Just test that validation passes (actual connection will fail in test)
		_, err := New(config)
		// We expect an error here because we're not connecting to real OpenStack
		// but the validation should have passed
		if err != nil {
			assert.Contains(t, err.Error(), "OpenStack")
		}
	})
}

// BenchmarkMatchesFilter benchmarks the filter matching logic
func BenchmarkMatchesFilter(b *testing.B) {
	a := &OpenStackAdapter{
		logger: zap.NewNop(),
	}

	filter := &adapter.Filter{
		ResourcePoolID: "pool-1",
		ResourceTypeID: "type-1",
		Location:       "zone-1",
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
		a.matchesFilter(filter, "pool-1", "type-1", "zone-1", labels)
	}
}

// BenchmarkApplyPagination benchmarks the pagination logic
func BenchmarkApplyPagination(b *testing.B) {
	items := make([]string, 1000)
	for i := range items {
		items[i] = fmt.Sprintf("item-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		applyPagination(items, 10, 50)
	}
}
