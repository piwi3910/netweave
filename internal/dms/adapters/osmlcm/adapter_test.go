package osmlcm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
)

// Test credentials constants used only for unit testing.
// These are not real credentials and should never be used in production.
const (
	testUsername   = "admin"
	testSecretData = "test-secret-data"
)

// TestNewAdapter tests adapter creation with various configurations.
func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				NBIEndpoint: "http://localhost:9999",
			},
			wantErr: false,
		},
		{
			name: "valid config with auth",
			config: &Config{
				NBIEndpoint: "http://localhost:9999",
				Username:    testUsername,
				Password:    testSecretData,
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "config with timeout",
			config: &Config{
				NBIEndpoint: "http://localhost:9999",
				Timeout:     5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "config with project",
			config: &Config{
				NBIEndpoint: "http://localhost:9999",
				Project:     "custom-project",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := NewAdapter(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, adp)

				// Verify defaults are applied
				if tt.config.Timeout == 0 {
					assert.Equal(t, DefaultTimeout, adp.config.Timeout)
				}
				if tt.config.Project == "" {
					assert.Equal(t, "admin", adp.config.Project)
				}
			}
		})
	}
}

// TestAdapterMetadata tests adapter metadata methods.
func TestAdapterMetadata(t *testing.T) {
	adp, err := NewAdapter(&Config{})
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, AdapterName, adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.Equal(t, AdapterVersion, adp.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := adp.Capabilities()
		require.NotEmpty(t, caps)
		assert.Contains(t, caps, dmsadapter.CapabilityPackageManagement)
		assert.Contains(t, caps, dmsadapter.CapabilityDeploymentLifecycle)
		assert.Contains(t, caps, dmsadapter.CapabilityScaling)
		assert.Contains(t, caps, dmsadapter.CapabilityHealthChecks)
	})

	t.Run("SupportsRollback", func(t *testing.T) {
		assert.False(t, adp.SupportsRollback())
	})

	t.Run("SupportsScaling", func(t *testing.T) {
		assert.True(t, adp.SupportsScaling())
	})

	t.Run("SupportsGitOps", func(t *testing.T) {
		assert.False(t, adp.SupportsGitOps())
	})
}

// createTestAdapter creates an adapter for testing.
func createTestAdapter(t *testing.T) *Adapter {
	t.Helper()

	adp, err := NewAdapter(&Config{
		NBIEndpoint: "http://localhost:9999",
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	// Initialize the adapter
	_ = adp.initialize()

	return adp
}

// TestListDeploymentPackages tests listing NS/VNF packages.
func TestListDeploymentPackages(t *testing.T) {
	adp := createTestAdapter(t)

	// Add some test packages
	pkg1 := &dmsadapter.DeploymentPackage{
		ID:          "vnfd-1",
		Name:        "vnf-package-1",
		Version:     "1.0.0",
		PackageType: "osm-vnfd",
	}
	pkg2 := &dmsadapter.DeploymentPackage{
		ID:          "nsd-1",
		Name:        "ns-package-1",
		Version:     "1.0.0",
		PackageType: "osm-nsd",
	}
	adp.packages["vnfd-1"] = pkg1
	adp.packages["nsd-1"] = pkg2

	t.Run("list all packages", func(t *testing.T) {
		packages, err := adp.ListDeploymentPackages(context.Background(), nil)
		require.NoError(t, err)
		assert.Len(t, packages, 2)
	})

	t.Run("pagination", func(t *testing.T) {
		packages, err := adp.ListDeploymentPackages(context.Background(), &dmsadapter.Filter{Limit: 1})
		require.NoError(t, err)
		assert.Len(t, packages, 1)
	})

	t.Run("offset pagination", func(t *testing.T) {
		packages, err := adp.ListDeploymentPackages(context.Background(), &dmsadapter.Filter{Offset: 1})
		require.NoError(t, err)
		assert.Len(t, packages, 1)
	})
}

// TestGetDeploymentPackage tests getting a specific package.
func TestGetDeploymentPackage(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test package
	pkg := &dmsadapter.DeploymentPackage{
		ID:          "vnfd-test",
		Name:        "test-vnf",
		Version:     "1.0.0",
		PackageType: "osm-vnfd",
	}
	adp.packages["vnfd-test"] = pkg

	t.Run("package found", func(t *testing.T) {
		result, err := adp.GetDeploymentPackage(context.Background(), "vnfd-test")
		require.NoError(t, err)
		assert.Equal(t, "vnfd-test", result.ID)
	})

	t.Run("package not found", func(t *testing.T) {
		_, err := adp.GetDeploymentPackage(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})
}

// TestUploadDeploymentPackage tests package upload.
func TestUploadDeploymentPackage(t *testing.T) {
	tests := []struct {
		name        string
		pkg         *dmsadapter.DeploymentPackageUpload
		wantErr     bool
		errContains string
		wantType    string
	}{
		{
			name: "valid vnfd package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:        "my-vnf",
				Version:     "1.0.0",
				Description: "Test VNF package",
			},
			wantErr:  false,
			wantType: "osm-vnfd",
		},
		{
			name: "nsd package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:    "my-ns",
				Version: "1.0.0",
				Extensions: map[string]interface{}{
					"osm.packageType": "nsd",
				},
			},
			wantErr:  false,
			wantType: "osm-nsd",
		},
		{
			name:        "nil package",
			pkg:         nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createTestAdapter(t)
			pkg, err := adp.UploadDeploymentPackage(context.Background(), tt.pkg)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, pkg)
				assert.Equal(t, tt.pkg.Name, pkg.Name)
				assert.Equal(t, tt.wantType, pkg.PackageType)
				assert.NotNil(t, pkg.Extensions)
			}
		})
	}
}

// TestDeleteDeploymentPackage tests package deletion.
func TestDeleteDeploymentPackage(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test package
	adp.packages["vnfd-delete"] = &dmsadapter.DeploymentPackage{
		ID:   "vnfd-delete",
		Name: "test-vnf",
	}

	t.Run("delete existing", func(t *testing.T) {
		err := adp.DeleteDeploymentPackage(context.Background(), "vnfd-delete")
		require.NoError(t, err)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := adp.DeleteDeploymentPackage(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})
}

// TestListDeployments tests listing NS instances.
func TestListDeployments(t *testing.T) {
	adp := createTestAdapter(t)

	// Add test deployments
	dep1 := &dmsadapter.Deployment{
		ID:     "ns-1",
		Name:   "test-ns-1",
		Status: dmsadapter.DeploymentStatusDeployed,
	}
	dep2 := &dmsadapter.Deployment{
		ID:     "ns-2",
		Name:   "test-ns-2",
		Status: dmsadapter.DeploymentStatusFailed,
	}
	adp.deployments["ns-1"] = dep1
	adp.deployments["ns-2"] = dep2

	t.Run("list all", func(t *testing.T) {
		deployments, err := adp.ListDeployments(context.Background(), nil)
		require.NoError(t, err)
		assert.Len(t, deployments, 2)
	})

	t.Run("filter by status", func(t *testing.T) {
		deployments, err := adp.ListDeployments(context.Background(), &dmsadapter.Filter{
			Status: dmsadapter.DeploymentStatusDeployed,
		})
		require.NoError(t, err)
		assert.Len(t, deployments, 1)
	})

	t.Run("pagination", func(t *testing.T) {
		deployments, err := adp.ListDeployments(context.Background(), &dmsadapter.Filter{Limit: 1})
		require.NoError(t, err)
		assert.Len(t, deployments, 1)
	})
}

// TestGetDeployment tests getting a specific deployment.
func TestGetDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	dep := &dmsadapter.Deployment{
		ID:     "ns-test",
		Name:   "test-ns",
		Status: dmsadapter.DeploymentStatusDeployed,
	}
	adp.deployments["ns-test"] = dep

	t.Run("deployment found", func(t *testing.T) {
		result, err := adp.GetDeployment(context.Background(), "ns-test")
		require.NoError(t, err)
		assert.Equal(t, "ns-test", result.ID)
	})

	t.Run("deployment not found", func(t *testing.T) {
		_, err := adp.GetDeployment(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestCreateDeployment tests creating NS instances.
func TestCreateDeployment(t *testing.T) {
	tests := []struct {
		name        string
		request     *dmsadapter.DeploymentRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "valid deployment",
			request: &dmsadapter.DeploymentRequest{
				Name:      "new-ns",
				PackageID: "nsd-1",
			},
			wantErr: false,
		},
		{
			name: "deployment with vim account",
			request: &dmsadapter.DeploymentRequest{
				Name:      "test-ns",
				PackageID: "nsd-1",
				Extensions: map[string]interface{}{
					"osm.vimAccount": "custom-vim",
				},
			},
			wantErr: false,
		},
		{
			name:        "nil request",
			request:     nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name: "missing name",
			request: &dmsadapter.DeploymentRequest{
				PackageID: "nsd-1",
			},
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name: "invalid name",
			request: &dmsadapter.DeploymentRequest{
				Name:      "INVALID",
				PackageID: "nsd-1",
			},
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name: "name too long",
			request: &dmsadapter.DeploymentRequest{
				Name:      "a23456789012345678901234567890123456789012345678901234567890123456789",
				PackageID: "nsd-1",
			},
			wantErr:     true,
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createTestAdapter(t)
			deployment, err := adp.CreateDeployment(context.Background(), tt.request)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, deployment)
				assert.Equal(t, tt.request.Name, deployment.Name)
				assert.NotNil(t, deployment.Extensions)
			}
		})
	}
}

// TestUpdateDeployment tests updating NS instances.
func TestUpdateDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["ns-update"] = &dmsadapter.Deployment{
		ID:      "ns-update",
		Name:    "test-ns",
		Version: 1,
	}

	tests := []struct {
		name        string
		deployID    string
		update      *dmsadapter.DeploymentUpdate
		wantErr     bool
		errContains string
	}{
		{
			name:     "update description",
			deployID: "ns-update",
			update: &dmsadapter.DeploymentUpdate{
				Description: "Updated description",
			},
			wantErr: false,
		},
		{
			name:        "nil update",
			deployID:    "ns-update",
			update:      nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name:     "deployment not found",
			deployID: "nonexistent",
			update: &dmsadapter.DeploymentUpdate{
				Description: "Test",
			},
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment, err := adp.UpdateDeployment(context.Background(), tt.deployID, tt.update)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, deployment)
			}
		})
	}
}

// TestDeleteDeployment tests deleting NS instances.
func TestDeleteDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["ns-delete"] = &dmsadapter.Deployment{
		ID:   "ns-delete",
		Name: "test-ns",
	}

	t.Run("delete existing", func(t *testing.T) {
		err := adp.DeleteDeployment(context.Background(), "ns-delete")
		require.NoError(t, err)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := adp.DeleteDeployment(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestScaleDeployment tests scaling NS instances.
func TestScaleDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["ns-scale"] = &dmsadapter.Deployment{
		ID:   "ns-scale",
		Name: "test-ns",
	}

	t.Run("scale up", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "ns-scale", 5)
		require.NoError(t, err)

		// Verify scale count is recorded
		dep := adp.deployments["ns-scale"]
		assert.Equal(t, 5, dep.Extensions["osm.scaleCount"])
	})

	t.Run("scale to zero", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "ns-scale", 0)
		require.NoError(t, err)
	})

	t.Run("negative replicas", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "ns-scale", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})

	t.Run("deployment not found", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "nonexistent", 3)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestRollbackDeployment tests that rollback is not supported.
func TestRollbackDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	t.Run("rollback not supported", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-ns", 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOperationNotSupported)
	})

	t.Run("negative revision", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-ns", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})
}

// TestGetDeploymentStatus tests getting deployment status.
func TestGetDeploymentStatus(t *testing.T) {
	adp := createTestAdapter(t)

	// Add test deployments with different statuses
	adp.deployments["ns-deployed"] = &dmsadapter.Deployment{
		ID:     "ns-deployed",
		Name:   "test-ns",
		Status: dmsadapter.DeploymentStatusDeployed,
	}
	adp.deployments["ns-deploying"] = &dmsadapter.Deployment{
		ID:     "ns-deploying",
		Name:   "test-ns",
		Status: dmsadapter.DeploymentStatusDeploying,
	}
	adp.deployments["ns-failed"] = &dmsadapter.Deployment{
		ID:     "ns-failed",
		Name:   "test-ns",
		Status: dmsadapter.DeploymentStatusFailed,
	}

	t.Run("get deployed status", func(t *testing.T) {
		status, err := adp.GetDeploymentStatus(context.Background(), "ns-deployed")
		require.NoError(t, err)
		assert.Equal(t, "ns-deployed", status.DeploymentID)
		assert.Equal(t, dmsadapter.DeploymentStatusDeployed, status.Status)
		assert.Equal(t, 100, status.Progress)
		assert.NotEmpty(t, status.Conditions)
		assert.Equal(t, "True", status.Conditions[0].Status)
		assert.Equal(t, "InstantiationSucceeded", status.Conditions[0].Reason)
	})

	t.Run("get deploying status", func(t *testing.T) {
		status, err := adp.GetDeploymentStatus(context.Background(), "ns-deploying")
		require.NoError(t, err)
		assert.Equal(t, 50, status.Progress)
		assert.Equal(t, "False", status.Conditions[0].Status)
		assert.Equal(t, "Instantiating", status.Conditions[0].Reason)
	})

	t.Run("get failed status", func(t *testing.T) {
		status, err := adp.GetDeploymentStatus(context.Background(), "ns-failed")
		require.NoError(t, err)
		assert.Equal(t, 0, status.Progress)
		assert.Equal(t, "InstantiationFailed", status.Conditions[0].Reason)
	})

	t.Run("deployment not found", func(t *testing.T) {
		_, err := adp.GetDeploymentStatus(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestGetDeploymentHistory tests getting deployment history.
func TestGetDeploymentHistory(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["ns-history"] = &dmsadapter.Deployment{
		ID:      "ns-history",
		Name:    "test-ns",
		Version: 3,
	}

	t.Run("get history", func(t *testing.T) {
		history, err := adp.GetDeploymentHistory(context.Background(), "ns-history")
		require.NoError(t, err)
		assert.Equal(t, "ns-history", history.DeploymentID)
		assert.NotEmpty(t, history.Revisions)
		assert.Equal(t, 3, history.Revisions[0].Revision)
	})

	t.Run("deployment not found", func(t *testing.T) {
		_, err := adp.GetDeploymentHistory(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestGetDeploymentLogs tests getting deployment logs.
func TestGetDeploymentLogs(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["ns-logs"] = &dmsadapter.Deployment{
		ID:   "ns-logs",
		Name: "test-ns",
	}

	t.Run("get logs", func(t *testing.T) {
		logs, err := adp.GetDeploymentLogs(context.Background(), "ns-logs", nil)
		require.NoError(t, err)
		assert.NotEmpty(t, logs)
		// Verify it's valid JSON
		assert.Contains(t, string(logs), "deploymentId")
	})

	t.Run("deployment not found", func(t *testing.T) {
		_, err := adp.GetDeploymentLogs(context.Background(), "nonexistent", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestHealth tests health check functionality.
func TestHealth(t *testing.T) {
	t.Run("healthy without endpoint", func(t *testing.T) {
		adp, err := NewAdapter(&Config{})
		require.NoError(t, err)

		err = adp.Health(context.Background())
		require.NoError(t, err)
	})

	t.Run("healthy with mock server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		adp, err := NewAdapter(&Config{
			NBIEndpoint: server.URL,
		})
		require.NoError(t, err)

		err = adp.Health(context.Background())
		require.NoError(t, err)
	})

	t.Run("unhealthy with server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		adp, err := NewAdapter(&Config{
			NBIEndpoint: server.URL,
		})
		require.NoError(t, err)

		err = adp.Health(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "health check failed")
	})
}

// TestClose tests adapter close functionality.
func TestClose(t *testing.T) {
	adp := createTestAdapter(t)

	err := adp.Close()
	require.NoError(t, err)
	assert.Nil(t, adp.httpClient)
}

// TestValidateName tests name validation.
func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my-ns", false},
		{"valid with numbers", "ns-123", false},
		{"valid single char", "a", false},
		{"empty", "", true},
		{"too long", "a23456789012345678901234567890123456789012345678901234567890123456789", true},
		{"uppercase", "MyNS", true},
		{"starts with hyphen", "-ns", true},
		{"ends with hyphen", "ns-", true},
		{"contains underscore", "my_ns", true},
		{"contains space", "my ns", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidName)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCalculateProgress tests progress calculation.
func TestCalculateProgress(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	tests := []struct {
		name   string
		status dmsadapter.DeploymentStatus
		want   int
	}{
		{"deployed", dmsadapter.DeploymentStatusDeployed, 100},
		{"deploying", dmsadapter.DeploymentStatusDeploying, 50},
		{"pending", dmsadapter.DeploymentStatusPending, 25},
		{"failed", dmsadapter.DeploymentStatusFailed, 0},
		{"unknown", dmsadapter.DeploymentStatus("unknown"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.calculateProgress(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConditionStatus tests condition status helper.
func TestConditionStatus(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	t.Run("deployed returns True", func(t *testing.T) {
		assert.Equal(t, "True", adp.conditionStatus(dmsadapter.DeploymentStatusDeployed))
	})

	t.Run("other status returns False", func(t *testing.T) {
		assert.Equal(t, "False", adp.conditionStatus(dmsadapter.DeploymentStatusDeploying))
		assert.Equal(t, "False", adp.conditionStatus(dmsadapter.DeploymentStatusFailed))
	})
}

// TestConditionReason tests condition reason helper.
func TestConditionReason(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	tests := []struct {
		status dmsadapter.DeploymentStatus
		want   string
	}{
		{dmsadapter.DeploymentStatusDeployed, "InstantiationSucceeded"},
		{dmsadapter.DeploymentStatusDeploying, "Instantiating"},
		{dmsadapter.DeploymentStatusFailed, "InstantiationFailed"},
		{dmsadapter.DeploymentStatus("unknown"), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.want, adp.conditionReason(tt.status))
		})
	}
}

// TestContextCancellation tests that adapter methods respect context cancellation.
func TestContextCancellation(t *testing.T) {
	adp := createTestAdapter(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "ListDeployments",
			fn: func() error {
				_, err := adp.ListDeployments(ctx, nil)
				return err
			},
		},
		{
			name: "GetDeployment",
			fn: func() error {
				_, err := adp.GetDeployment(ctx, "test")
				return err
			},
		},
		{
			name: "CreateDeployment",
			fn: func() error {
				_, err := adp.CreateDeployment(ctx, &dmsadapter.DeploymentRequest{
					Name:      "test",
					PackageID: "nsd-1",
				})
				return err
			},
		},
		{
			name: "UpdateDeployment",
			fn: func() error {
				_, err := adp.UpdateDeployment(ctx, "test", &dmsadapter.DeploymentUpdate{})
				return err
			},
		},
		{
			name: "DeleteDeployment",
			fn: func() error {
				return adp.DeleteDeployment(ctx, "test")
			},
		},
		{
			name: "ScaleDeployment",
			fn: func() error {
				return adp.ScaleDeployment(ctx, "test", 3)
			},
		},
		{
			name: "RollbackDeployment",
			fn: func() error {
				return adp.RollbackDeployment(ctx, "test", 1)
			},
		},
		{
			name: "ListDeploymentPackages",
			fn: func() error {
				_, err := adp.ListDeploymentPackages(ctx, nil)
				return err
			},
		},
		{
			name: "GetDeploymentPackage",
			fn: func() error {
				_, err := adp.GetDeploymentPackage(ctx, "test")
				return err
			},
		},
		{
			name: "UploadDeploymentPackage",
			fn: func() error {
				_, err := adp.UploadDeploymentPackage(ctx, &dmsadapter.DeploymentPackageUpload{
					Name:    "test",
					Version: "1.0.0",
				})
				return err
			},
		},
		{
			name: "DeleteDeploymentPackage",
			fn: func() error {
				return adp.DeleteDeploymentPackage(ctx, "test")
			},
		},
		{
			name: "GetDeploymentStatus",
			fn: func() error {
				_, err := adp.GetDeploymentStatus(ctx, "test")
				return err
			},
		},
		{
			name: "GetDeploymentHistory",
			fn: func() error {
				_, err := adp.GetDeploymentHistory(ctx, "test")
				return err
			},
		},
		{
			name: "GetDeploymentLogs",
			fn: func() error {
				_, err := adp.GetDeploymentLogs(ctx, "test", nil)
				return err
			},
		},
		{
			name: "Health",
			fn: func() error {
				return adp.Health(ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}

// TestApplyPagination tests pagination logic.
func TestApplyPagination(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	deployments := []*dmsadapter.Deployment{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
		wantFirst string
	}{
		{"no pagination", 0, 0, 5, "1"},
		{"limit only", 2, 0, 2, "1"},
		{"offset only", 10, 2, 3, "3"},
		{"limit and offset", 2, 1, 2, "2"},
		{"offset beyond length", 10, 10, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adp.applyPagination(deployments, tt.limit, tt.offset)
			assert.Len(t, result, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
			}
		})
	}
}

// TestApplyPackagePagination tests package pagination logic.
func TestApplyPackagePagination(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	packages := []*dmsadapter.DeploymentPackage{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
		wantFirst string
	}{
		{"no pagination", 0, 0, 5, "1"},
		{"limit only", 2, 0, 2, "1"},
		{"offset only", 10, 2, 3, "3"},
		{"limit and offset", 2, 1, 2, "2"},
		{"offset beyond length", 10, 10, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adp.applyPackagePagination(packages, tt.limit, tt.offset)
			assert.Len(t, result, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
			}
		})
	}
}

// TestDoRequest tests the HTTP request helper.
func TestDoRequest(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		adp, err := NewAdapter(&Config{
			NBIEndpoint: server.URL,
		})
		require.NoError(t, err)
		_ = adp.initialize()

		resp, err := adp.doRequest(context.Background(), http.MethodGet, "/test", nil)
		require.NoError(t, err)
		assert.Contains(t, string(resp), "ok")
	})

	t.Run("request with basic auth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, testUsername, user)
			assert.Equal(t, testSecretData, pass)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		adp, err := NewAdapter(&Config{
			NBIEndpoint: server.URL,
			Username:    testUsername,
			Password:    testSecretData,
		})
		require.NoError(t, err)
		_ = adp.initialize()

		_, err = adp.doRequest(context.Background(), http.MethodGet, "/test", nil)
		require.NoError(t, err)
	})

	t.Run("request with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		adp, err := NewAdapter(&Config{
			NBIEndpoint: server.URL,
		})
		require.NoError(t, err)
		_ = adp.initialize()

		body := map[string]string{"key": "value"}
		_, err = adp.doRequest(context.Background(), http.MethodPost, "/test", body)
		require.NoError(t, err)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "bad request"}`))
		}))
		defer server.Close()

		adp, err := NewAdapter(&Config{
			NBIEndpoint: server.URL,
		})
		require.NoError(t, err)
		_ = adp.initialize()

		_, err = adp.doRequest(context.Background(), http.MethodGet, "/test", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})
}

// Benchmark tests.

// BenchmarkCreateDeployment benchmarks deployment creation.
func BenchmarkCreateDeployment(b *testing.B) {
	adp, _ := NewAdapter(&Config{
		NBIEndpoint: "http://localhost:9999",
	})
	_ = adp.initialize()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		name := "bench-ns-" + string(rune('a'+i%26))
		_, _ = adp.CreateDeployment(ctx, &dmsadapter.DeploymentRequest{
			Name:      name,
			PackageID: "nsd-1",
		})
	}
}

// BenchmarkListDeployments benchmarks deployment listing.
func BenchmarkListDeployments(b *testing.B) {
	adp, _ := NewAdapter(&Config{
		NBIEndpoint: "http://localhost:9999",
	})
	_ = adp.initialize()

	// Add some deployments
	for i := 0; i < 100; i++ {
		adp.deployments[string(rune('a'+i))] = &dmsadapter.Deployment{
			ID:   string(rune('a' + i)),
			Name: "test-ns",
		}
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = adp.ListDeployments(ctx, nil)
	}
}
