package onaplcm

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
				SOEndpoint: "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "valid config with auth",
			config: &Config{
				SOEndpoint: "http://localhost:8080",
				Username:   "admin",
				Password:   "secret",
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
				SOEndpoint: "http://localhost:8080",
				Timeout:    5 * time.Minute,
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
func createTestAdapter(t *testing.T) *ONAPLCMAdapter {
	t.Helper()

	adp, err := NewAdapter(&Config{
		SOEndpoint: "http://localhost:8080",
		Timeout:    5 * time.Second,
	})
	require.NoError(t, err)

	// Initialize the adapter
	_ = adp.initialize()

	return adp
}

// TestListDeploymentPackages tests listing VNF packages.
func TestListDeploymentPackages(t *testing.T) {
	adp := createTestAdapter(t)

	// Add some test packages
	pkg1 := &dmsadapter.DeploymentPackage{
		ID:          "vnfd-1",
		Name:        "vnf-package-1",
		Version:     "1.0.0",
		PackageType: "onap-vnf",
	}
	pkg2 := &dmsadapter.DeploymentPackage{
		ID:          "vnfd-2",
		Name:        "vnf-package-2",
		Version:     "1.0.0",
		PackageType: "onap-vnf",
	}
	adp.packages["vnfd-1"] = pkg1
	adp.packages["vnfd-2"] = pkg2

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
}

// TestGetDeploymentPackage tests getting a specific package.
func TestGetDeploymentPackage(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test package
	pkg := &dmsadapter.DeploymentPackage{
		ID:          "vnfd-test",
		Name:        "test-vnf",
		Version:     "1.0.0",
		PackageType: "onap-vnf",
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
	}{
		{
			name: "valid package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:        "my-vnf",
				Version:     "1.0.0",
				Description: "Test VNF package",
			},
			wantErr: false,
		},
		{
			name: "package with vnfd ID",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:    "my-vnf",
				Version: "1.0.0",
				Extensions: map[string]interface{}{
					"onap.vnfdId": "custom-vnfd-id",
				},
			},
			wantErr: false,
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
				assert.Equal(t, "onap-vnf", pkg.PackageType)
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

// TestListDeployments tests listing VNF instances.
func TestListDeployments(t *testing.T) {
	adp := createTestAdapter(t)

	// Add test deployments
	dep1 := &dmsadapter.Deployment{
		ID:     "vnf-1",
		Name:   "test-vnf-1",
		Status: dmsadapter.DeploymentStatusDeployed,
	}
	dep2 := &dmsadapter.Deployment{
		ID:     "vnf-2",
		Name:   "test-vnf-2",
		Status: dmsadapter.DeploymentStatusFailed,
	}
	adp.deployments["vnf-1"] = dep1
	adp.deployments["vnf-2"] = dep2

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
		ID:     "vnf-test",
		Name:   "test-vnf",
		Status: dmsadapter.DeploymentStatusDeployed,
	}
	adp.deployments["vnf-test"] = dep

	t.Run("deployment found", func(t *testing.T) {
		result, err := adp.GetDeployment(context.Background(), "vnf-test")
		require.NoError(t, err)
		assert.Equal(t, "vnf-test", result.ID)
	})

	t.Run("deployment not found", func(t *testing.T) {
		_, err := adp.GetDeployment(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestCreateDeployment tests creating VNF instances.
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
				Name:      "new-vnf",
				PackageID: "vnfd-1",
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
				PackageID: "vnfd-1",
			},
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name: "invalid name",
			request: &dmsadapter.DeploymentRequest{
				Name:      "INVALID",
				PackageID: "vnfd-1",
			},
			wantErr:     true,
			errContains: "DNS-1123",
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
			}
		})
	}
}

// TestUpdateDeployment tests updating VNF instances.
func TestUpdateDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["vnf-update"] = &dmsadapter.Deployment{
		ID:      "vnf-update",
		Name:    "test-vnf",
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
			deployID: "vnf-update",
			update: &dmsadapter.DeploymentUpdate{
				Description: "Updated description",
			},
			wantErr: false,
		},
		{
			name:        "nil update",
			deployID:    "vnf-update",
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

// TestDeleteDeployment tests deleting VNF instances.
func TestDeleteDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["vnf-delete"] = &dmsadapter.Deployment{
		ID:   "vnf-delete",
		Name: "test-vnf",
	}

	t.Run("delete existing", func(t *testing.T) {
		err := adp.DeleteDeployment(context.Background(), "vnf-delete")
		require.NoError(t, err)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := adp.DeleteDeployment(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})
}

// TestScaleDeployment tests scaling VNF instances.
func TestScaleDeployment(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["vnf-scale"] = &dmsadapter.Deployment{
		ID:   "vnf-scale",
		Name: "test-vnf",
	}

	t.Run("scale up", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "vnf-scale", 5)
		require.NoError(t, err)
	})

	t.Run("scale to zero", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "vnf-scale", 0)
		require.NoError(t, err)
	})

	t.Run("negative replicas", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "vnf-scale", -1)
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
		err := adp.RollbackDeployment(context.Background(), "any-vnf", 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOperationNotSupported)
	})

	t.Run("negative revision", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-vnf", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})
}

// TestGetDeploymentStatus tests getting deployment status.
func TestGetDeploymentStatus(t *testing.T) {
	adp := createTestAdapter(t)

	// Add a test deployment
	adp.deployments["vnf-status"] = &dmsadapter.Deployment{
		ID:     "vnf-status",
		Name:   "test-vnf",
		Status: dmsadapter.DeploymentStatusDeployed,
	}

	t.Run("get status", func(t *testing.T) {
		status, err := adp.GetDeploymentStatus(context.Background(), "vnf-status")
		require.NoError(t, err)
		assert.Equal(t, "vnf-status", status.DeploymentID)
		assert.Equal(t, dmsadapter.DeploymentStatusDeployed, status.Status)
		assert.NotEmpty(t, status.Conditions)
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
	adp.deployments["vnf-history"] = &dmsadapter.Deployment{
		ID:      "vnf-history",
		Name:    "test-vnf",
		Version: 3,
	}

	t.Run("get history", func(t *testing.T) {
		history, err := adp.GetDeploymentHistory(context.Background(), "vnf-history")
		require.NoError(t, err)
		assert.Equal(t, "vnf-history", history.DeploymentID)
		assert.NotEmpty(t, history.Revisions)
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
	adp.deployments["vnf-logs"] = &dmsadapter.Deployment{
		ID:   "vnf-logs",
		Name: "test-vnf",
	}

	t.Run("get logs", func(t *testing.T) {
		logs, err := adp.GetDeploymentLogs(context.Background(), "vnf-logs", nil)
		require.NoError(t, err)
		assert.NotEmpty(t, logs)
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
			SOEndpoint: server.URL,
		})
		require.NoError(t, err)

		err = adp.Health(context.Background())
		require.NoError(t, err)
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
		{"valid name", "my-vnf", false},
		{"valid with numbers", "vnf-123", false},
		{"valid single char", "a", false},
		{"empty", "", true},
		{"too long", "a23456789012345678901234567890123456789012345678901234567890123456789", true},
		{"uppercase", "MyVNF", true},
		{"starts with hyphen", "-vnf", true},
		{"ends with hyphen", "vnf-", true},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.calculateProgress(tt.status)
			assert.Equal(t, tt.want, got)
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
					PackageID: "vnfd-1",
				})
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
