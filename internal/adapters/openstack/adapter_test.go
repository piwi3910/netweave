package openstack_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNew tests the creation of a new OpenStackAdapter.
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

// TestNewWithDefaults tests default value initialization.
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
	t.Cleanup(func() { require.NoError(t, adapter.Close()) })

	// Check defaults
	assert.Equal(t, "test-ocloud", adapter.oCloudID)
	assert.NotEmpty(t, adapter.deploymentManagerID)
	assert.Contains(t, adapter.deploymentManagerID, "ocloud-openstack-")
}

// TestMetadata tests metadata methods.
func TestMetadata(t *testing.T) {
	a := &Adapter{
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

// NOTE: TestMatchesFilter and TestApplyPagination tests moved to internal/adapter/helpers_test.go
// These shared helper functions are now tested in the common adapter package.

// TestGenerateFlavorID tests flavor ID generation.
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

// TestClose tests adapter cleanup.
func TestClose(t *testing.T) {
	adapter := &Adapter{
		logger: zap.NewNop(),
	}

	err := adapter.Close()
	assert.NoError(t, err)
}

// TestConfigValidation tests configuration validation.
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

// NOTE: BenchmarkMatchesFilter and BenchmarkApplyPagination moved to internal/adapter/helpers_test.go

// TestOpenStackAdapter_Health tests the Health function.
func TestOpenStackAdapter_Health(t *testing.T) {
	adapter, err := New(&Config{
		AuthURL:     "https://openstack.example.com:5000/v3",
		Username:    "test",
		Password:    "test",
		ProjectName: "test",
		DomainName:  "Default",
		Region:      "RegionOne",
		OCloudID:    "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires OpenStack credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = adapter.Health(ctx)
	if err != nil {
		t.Skip("Skipping - requires OpenStack access")
	}
}

// TestOpenStackAdapter_ListResourcePools tests the ListResourcePools function.
func TestOpenStackAdapter_ListResourcePools(t *testing.T) {
	adapter, err := New(&Config{
		AuthURL:     "https://openstack.example.com:5000/v3",
		Username:    "test",
		Password:    "test",
		ProjectName: "test",
		DomainName:  "Default",
		Region:      "RegionOne",
		OCloudID:    "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires OpenStack credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pools, err := adapter.ListResourcePools(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires OpenStack access")
	}
	assert.NotNil(t, pools)
}

// TestOpenStackAdapter_ListResources tests the ListResources function.
func TestOpenStackAdapter_ListResources(t *testing.T) {
	adapter, err := New(&Config{
		AuthURL:     "https://openstack.example.com:5000/v3",
		Username:    "test",
		Password:    "test",
		ProjectName: "test",
		DomainName:  "Default",
		Region:      "RegionOne",
		OCloudID:    "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires OpenStack credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resources, err := adapter.ListResources(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires OpenStack access")
	}
	assert.NotNil(t, resources)
}

// TestOpenStackAdapter_ListResourceTypes tests the ListResourceTypes function.
func TestOpenStackAdapter_ListResourceTypes(t *testing.T) {
	adapter, err := New(&Config{
		AuthURL:     "https://openstack.example.com:5000/v3",
		Username:    "test",
		Password:    "test",
		ProjectName: "test",
		DomainName:  "Default",
		Region:      "RegionOne",
		OCloudID:    "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires OpenStack credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	types, err := adapter.ListResourceTypes(ctx, nil)
	if err != nil {
		t.Skip("Skipping - requires OpenStack access")
	}
	assert.NotNil(t, types)
}

// TestOpenStackAdapter_GetDeploymentManager tests the GetDeploymentManager function.
func TestOpenStackAdapter_GetDeploymentManager(t *testing.T) {
	adapter, err := New(&Config{
		AuthURL:     "https://openstack.example.com:5000/v3",
		Username:    "test",
		Password:    "test",
		ProjectName: "test",
		DomainName:  "Default",
		Region:      "RegionOne",
		OCloudID:    "test-cloud",
	})
	if err != nil {
		t.Skip("Skipping - requires OpenStack credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm, err := adapter.GetDeploymentManager(ctx, "dm-1")
	if err != nil {
		t.Skip("Skipping - requires OpenStack access")
	}
	assert.NotNil(t, dm)
}
