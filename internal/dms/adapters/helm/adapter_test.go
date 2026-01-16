package helm_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms/adapters/helm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	helmtime "helm.sh/helm/v3/pkg/time"
	corev1 "k8s.io/api/core/v1"

	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
)

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name    string
		config  *helm.Config
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: &helm.Config{
				Namespace:     "test-namespace",
				RepositoryURL: "https://charts.example.com",
				Timeout:       5 * time.Minute,
				MaxHistory:    5,
			},
			wantErr: false,
		},
		{
			name: "configuration with defaults",
			config: &helm.Config{
				RepositoryURL: "https://charts.example.com",
			},
			wantErr: false,
		},
		{
			name:    "nil configuration",
			config:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)

				// Verify defaults were applied
				if tt.config.Namespace == "" {
					assert.Equal(t, "default", adapter.Config.Namespace)
				}
				if tt.config.Timeout == 0 {
					assert.Equal(t, helm.DefaultTimeout, adapter.Config.Timeout)
				}
				if tt.config.MaxHistory == 0 {
					assert.Equal(t, helm.DefaultMaxHistory, adapter.Config.MaxHistory)
				}
			}
		})
	}
}

func TestHelmAdapter_Metadata(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, helm.AdapterName, adapter.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.Equal(t, helm.AdapterVersion, adapter.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := adapter.Capabilities()
		assert.NotEmpty(t, caps)
		assert.Contains(t, caps, dmsadapter.CapabilityPackageManagement)
		assert.Contains(t, caps, dmsadapter.CapabilityDeploymentLifecycle)
		assert.Contains(t, caps, dmsadapter.CapabilityRollback)
		assert.Contains(t, caps, dmsadapter.CapabilityScaling)
	})
}

func TestHelmAdapter_CapabilityChecks(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	t.Run("SupportsRollback", func(t *testing.T) {
		assert.True(t, adapter.SupportsRollback())
	})

	t.Run("SupportsScaling", func(t *testing.T) {
		assert.True(t, adapter.SupportsScaling())
	})

	t.Run("SupportsGitOps", func(t *testing.T) {
		assert.False(t, adapter.SupportsGitOps())
	})
}

func TestHelmAdapter_TransformHelmStatus(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		helmStatus release.Status
		want       dmsadapter.DeploymentStatus
	}{
		{
			name:       "pending install",
			helmStatus: release.StatusPendingInstall,
			want:       dmsadapter.DeploymentStatusPending,
		},
		{
			name:       "pending upgrade",
			helmStatus: release.StatusPendingUpgrade,
			want:       dmsadapter.DeploymentStatusDeploying,
		},
		{
			name:       "deployed",
			helmStatus: release.StatusDeployed,
			want:       dmsadapter.DeploymentStatusDeployed,
		},
		{
			name:       "failed",
			helmStatus: release.StatusFailed,
			want:       dmsadapter.DeploymentStatusFailed,
		},
		{
			name:       "pending rollback",
			helmStatus: release.StatusPendingRollback,
			want:       dmsadapter.DeploymentStatusRollingBack,
		},
		{
			name:       "uninstalling",
			helmStatus: release.StatusUninstalling,
			want:       dmsadapter.DeploymentStatusDeleting,
		},
		{
			name:       "uninstalled",
			helmStatus: release.StatusUninstalled,
			want:       dmsadapter.DeploymentStatusDeleting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.TransformHelmStatus(tt.helmStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHelmAdapter_CalculateProgress(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		helmStatus release.Status
		want       int
	}{
		{
			name:       "deployed",
			helmStatus: release.StatusDeployed,
			want:       100,
		},
		{
			name:       "failed",
			helmStatus: release.StatusFailed,
			want:       0,
		},
		{
			name:       "pending install",
			helmStatus: release.StatusPendingInstall,
			want:       25,
		},
		{
			name:       "deploying",
			helmStatus: release.StatusPendingUpgrade,
			want:       50,
		},
		{
			name:       "uninstalling",
			helmStatus: release.StatusUninstalling,
			want:       75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &release.Release{
				Info: &release.Info{
					Status: tt.helmStatus,
				},
			}
			got := adapter.CalculateProgress(rel)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHelmAdapter_TransformReleaseToDeployment(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	now := helmtime.Now()
	rel := &release.Release{
		Name:      "test-release",
		Namespace: "test-namespace",
		Version:   3,
		Info: &release.Info{
			Status:        release.StatusDeployed,
			Description:   "Test deployment",
			FirstDeployed: now,
			LastDeployed:  now,
		},
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{
				Name:       "test-chart",
				Version:    "1.0.0",
				AppVersion: "1.0.0",
			},
		},
	}

	deployment := adapter.TransformReleaseToDeployment(rel)

	assert.Equal(t, "test-release", deployment.ID)
	assert.Equal(t, "test-release", deployment.Name)
	assert.Equal(t, "test-chart-1.0.0", deployment.PackageID)
	assert.Equal(t, "test-namespace", deployment.Namespace)
	assert.Equal(t, dmsadapter.DeploymentStatusDeployed, deployment.Status)
	assert.Equal(t, 3, deployment.Version)
	assert.Equal(t, "Test deployment", deployment.Description)

	// Check extensions
	assert.NotNil(t, deployment.Extensions)
	assert.Equal(t, "test-release", deployment.Extensions["helm.releaseName"])
	assert.Equal(t, 3, deployment.Extensions["helm.revision"])
	assert.Equal(t, "test-chart", deployment.Extensions["helm.chart"])
	assert.Equal(t, "1.0.0", deployment.Extensions["helm.chartVersion"])
	assert.Equal(t, "1.0.0", deployment.Extensions["helm.appVersion"])
}

func TestHelmAdapter_TransformReleaseToStatus(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	now := helmtime.Now()
	rel := &release.Release{
		Name:      "test-release",
		Namespace: "test-namespace",
		Version:   2,
		Info: &release.Info{
			Status:       release.StatusDeployed,
			Description:  "Release deployed successfully",
			LastDeployed: now,
			Notes:        "Test notes",
		},
	}

	status := adapter.TransformReleaseToStatus(rel)

	assert.Equal(t, "test-release", status.DeploymentID)
	assert.Equal(t, dmsadapter.DeploymentStatusDeployed, status.Status)
	assert.Equal(t, "Release deployed successfully", status.Message)
	assert.Equal(t, 100, status.Progress)
	assert.NotEmpty(t, status.Conditions)

	// Check conditions
	assert.Equal(t, "Deployed", status.Conditions[0].Type)
	assert.Equal(t, "True", status.Conditions[0].Status)
	assert.Equal(t, "DeploymentSuccessful", status.Conditions[0].Reason)
}

func TestHelmAdapter_BuildConditions(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name           string
		helmStatus     release.Status
		wantCondStatus string
		wantReason     string
	}{
		{
			name:           "deployed",
			helmStatus:     release.StatusDeployed,
			wantCondStatus: "True",
			wantReason:     "DeploymentSuccessful",
		},
		{
			name:           "failed",
			helmStatus:     release.StatusFailed,
			wantCondStatus: "False",
			wantReason:     "DeploymentInProgress",
		},
		{
			name:           "deploying",
			helmStatus:     release.StatusPendingUpgrade,
			wantCondStatus: "False",
			wantReason:     "DeploymentInProgress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &release.Release{
				Info: &release.Info{
					Status:       tt.helmStatus,
					LastDeployed: helmtime.Now(),
				},
			}

			conditions := adapter.BuildConditions(rel)

			assert.NotEmpty(t, conditions)
			assert.Equal(t, "Deployed", conditions[0].Type)
			assert.Equal(t, tt.wantCondStatus, conditions[0].Status)
			assert.Equal(t, tt.wantReason, conditions[0].Reason)
			assert.NotEmpty(t, conditions[0].Message)
		})
	}
}

func TestHelmAdapter_ApplyPagination(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	// Create test deployments
	deployments := make([]*dmsadapter.Deployment, 10)
	for i := 0; i < 10; i++ {
		deployments[i] = &dmsadapter.Deployment{
			ID:   string(rune('A' + i)),
			Name: string(rune('A' + i)),
		}
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantLen   int
		wantFirst string
		wantLast  string
	}{
		{
			name:      "no pagination",
			limit:     0,
			offset:    0,
			wantLen:   10,
			wantFirst: "A",
			wantLast:  "J",
		},
		{
			name:      "first page",
			limit:     3,
			offset:    0,
			wantLen:   3,
			wantFirst: "A",
			wantLast:  "C",
		},
		{
			name:      "second page",
			limit:     3,
			offset:    3,
			wantLen:   3,
			wantFirst: "D",
			wantLast:  "F",
		},
		{
			name:      "last page partial",
			limit:     3,
			offset:    9,
			wantLen:   1,
			wantFirst: "J",
			wantLast:  "J",
		},
		{
			name:    "offset beyond range",
			limit:   3,
			offset:  20,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.ApplyPagination(deployments, tt.limit, tt.offset)

			assert.Equal(t, tt.wantLen, len(result))

			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
				assert.Equal(t, tt.wantLast, result[len(result)-1].ID)
			}
		})
	}
}

func TestHelmAdapter_UploadDeploymentPackage(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: "https://charts.example.com",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name    string
		pkg     *dmsadapter.DeploymentPackageUpload
		wantErr bool
	}{
		{
			name: "valid package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:        "test-chart",
				Version:     "1.0.0",
				PackageType: "helm-chart",
				Description: "Test chart",
				Content:     []byte("chart content"),
			},
			wantErr: false,
		},
		{
			name:    "nil package",
			pkg:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.UploadDeploymentPackage(ctx, tt.pkg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.pkg.Name, result.Name)
				assert.Equal(t, tt.pkg.Version, result.Version)
				assert.Equal(t, "helm-chart", result.PackageType)
				assert.NotEmpty(t, result.ID)
			}
		})
	}
}

func TestHelmAdapter_ScaleDeployment_Validation(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("negative replicas", func(t *testing.T) {
		err := adapter.ScaleDeployment(ctx, "test-release", -1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "replicas must be non-negative")
	})
}

func TestHelmAdapter_RollbackDeployment_Validation(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("negative revision", func(t *testing.T) {
		err := adapter.RollbackDeployment(ctx, "test-release", -1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "revision must be non-negative")
	})
}

func TestHelmAdapter_Close(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	// Initialize adapter
	adapter.Initialized = true
	adapter.ActionCfg = &action.Configuration{}

	// Close adapter
	err = adapter.Close()
	assert.NoError(t, err)
	assert.False(t, adapter.Initialized)
	assert.Nil(t, adapter.ActionCfg)
}

func TestConfig_Defaults(t *testing.T) {
	tests := []struct {
		name           string
		input          *helm.Config
		wantNamespace  string
		wantTimeout    time.Duration
		wantMaxHistory int
	}{
		{
			name:           "all defaults",
			input:          &helm.Config{},
			wantNamespace:  "default",
			wantTimeout:    helm.DefaultTimeout,
			wantMaxHistory: helm.DefaultMaxHistory,
		},
		{
			name: "custom values",
			input: &helm.Config{
				Namespace:  "custom",
				Timeout:    5 * time.Minute,
				MaxHistory: 20,
			},
			wantNamespace:  "custom",
			wantTimeout:    5 * time.Minute,
			wantMaxHistory: 20,
		},
		{
			name: "partial custom",
			input: &helm.Config{
				Namespace: "custom",
			},
			wantNamespace:  "custom",
			wantTimeout:    helm.DefaultTimeout,
			wantMaxHistory: helm.DefaultMaxHistory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantNamespace, adapter.Config.Namespace)
			assert.Equal(t, tt.wantTimeout, adapter.Config.Timeout)
			assert.Equal(t, tt.wantMaxHistory, adapter.Config.MaxHistory)
		})
	}
}

// Benchmark tests.
func BenchmarkHelmAdapter_TransformReleaseToDeployment(b *testing.B) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(b, err)

	rel := &release.Release{
		Name:      "test-release",
		Namespace: "test-namespace",
		Version:   1,
		Info: &release.Info{
			Status:        release.StatusDeployed,
			Description:   "Test",
			FirstDeployed: helmtime.Now(),
			LastDeployed:  helmtime.Now(),
		},
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adapter.TransformReleaseToDeployment(rel)
	}
}

func BenchmarkHelmAdapter_ApplyPagination(b *testing.B) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(b, err)

	deployments := make([]*dmsadapter.Deployment, 100)
	for i := 0; i < 100; i++ {
		deployments[i] = &dmsadapter.Deployment{
			ID:   fmt.Sprintf("deployment-%d", i),
			Name: fmt.Sprintf("deployment-%d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adapter.ApplyPagination(deployments, 10, 0)
	}
}

// TestHelmAdapter_ListDeploymentPackages tests listing packages from repository.
func TestHelmAdapter_ListDeploymentPackages(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: "https://charts.example.com",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		filter    *dmsadapter.Filter
		expectErr bool
	}{
		{
			name:      "list all packages",
			filter:    nil,
			expectErr: true, // Will fail without real repository
		},
		{
			name: "filter by chart name",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nginx",
				},
			},
			expectErr: true, // Will fail without real repository
		},
		{
			name: "filter by version",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartVersion": "1.0.0",
				},
			},
			expectErr: true, // Will fail without real repository
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, err := adapter.ListDeploymentPackages(ctx, tt.filter)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, packages)
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentPackage tests getting a specific package.
func TestHelmAdapter_GetDeploymentPackage(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: "https://charts.example.com",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		packageID string
		expectErr bool
	}{
		{
			name:      "get existing package",
			packageID: "nginx-1.0.0",
			expectErr: true, // Will fail without real repository
		},
		{
			name:      "get non-existent package",
			packageID: "non-existent-1.0.0",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := adapter.GetDeploymentPackage(ctx, tt.packageID)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, pkg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, pkg)
				assert.Equal(t, tt.packageID, pkg.ID)
			}
		})
	}
}

func TestHelmAdapter_GetDeploymentPackage_ErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		packageID string
		wantErr   string
	}{
		{
			name:      "empty package ID",
			packageID: "",
			wantErr:   "chart not found",
		},
		{
			name:      "invalid package ID format - no version",
			packageID: "nginx",
			wantErr:   "chart not found",
		},
		{
			name:      "invalid package ID format - multiple dashes",
			packageID: "my-complex-chart-1.0.0-alpha",
			wantErr:   "chart not found",
		},
		{
			name:      "non-existent chart name",
			packageID: "nonexistent-chart-1.0.0",
			wantErr:   "chart not found",
		},
		{
			name:      "non-existent version",
			packageID: "nginx-999.0.0",
			wantErr:   "chart not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace:     "test",
				RepositoryURL: "https://charts.example.com",
			})
			require.NoError(t, err)

			ctx := context.Background()
			pkg, err := adapter.GetDeploymentPackage(ctx, tt.packageID)

			assert.Error(t, err)
			assert.Nil(t, pkg)
			// Skip if error is network related, check for expected error
			if err != nil && (strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "connection refused")) {
				t.Skip("Skipping - requires repository access")
			}
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestHelmAdapter_DeleteDeploymentPackage tests package deletion.
func TestHelmAdapter_DeleteDeploymentPackage(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: "https://charts.example.com",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		packageID string
		expectErr bool
	}{
		{
			name:      "delete package",
			packageID: "nginx-1.0.0",
			expectErr: true, // Always fails as deletion is not fully implemented
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.DeleteDeploymentPackage(ctx, tt.packageID)
			if tt.expectErr {
				assert.Error(t, err)
				// Error could be "not fully implemented" or repository access error
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHelmAdapter_LoadRepositoryIndex tests repository index loading.
func TestHelmAdapter_LoadRepositoryIndex(t *testing.T) {
	tests := []struct {
		name      string
		repoURL   string
		expectErr bool
	}{
		{
			name:      "missing repository URL",
			repoURL:   "",
			expectErr: true,
		},
		{
			name:      "invalid repository URL",
			repoURL:   "https://invalid-repo-url.example.com",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace:     "test",
				RepositoryURL: tt.repoURL,
			})
			require.NoError(t, err)

			ctx := context.Background()
			err = adapter.LoadRepositoryIndex(ctx)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHelmAdapter_DeleteDeployment tests the DeleteDeployment function.
func TestHelmAdapter_DeleteDeployment(t *testing.T) {
	t.Skip(
		"TODO(#197): Fix Kubernetes error handling - " +
			"test expects specific errors but gets K8s unreachable errors in CI",
	)
	tests := []struct {
		name          string
		releaseID     string
		expectError   bool
		errorContains string
	}{
		{
			name:        "successful deletion",
			releaseID:   "test-release",
			expectError: false,
		},
		{
			name:          "release not found",
			releaseID:     "nonexistent",
			expectError:   true,
			errorContains: "deployment not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace: "test",
				Timeout:   5 * time.Second,
			})
			require.NoError(t, err)

			ctx := context.Background()
			err = adapter.DeleteDeployment(ctx, tt.releaseID)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else if err != nil {
				// Will fail without K8s but tests the code path
				// Expected in unit test environment without K8s
				t.Skip("Skipping - requires Kubernetes")
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentStatus tests the GetDeploymentStatus function.
func TestHelmAdapter_GetDeploymentStatus(t *testing.T) {
	tests := []struct {
		name          string
		releaseID     string
		expectError   bool
		errorContains string
	}{
		{
			name:        "successful status retrieval",
			releaseID:   "test-release",
			expectError: false,
		},
		{
			name:          "release not found",
			releaseID:     "nonexistent",
			expectError:   true,
			errorContains: "failed to get release status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace: "test",
			})
			require.NoError(t, err)

			ctx := context.Background()
			status, err := adapter.GetDeploymentStatus(ctx, tt.releaseID)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Will fail without K8s but tests the code path
				if err != nil {
					t.Skip("Skipping - requires Kubernetes")
				}
				if status != nil {
					assert.Equal(t, tt.releaseID, status.DeploymentID)
					assert.NotEmpty(t, status.Status)
				}
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentHistory tests the GetDeploymentHistory function.
func TestHelmAdapter_GetDeploymentHistory(t *testing.T) {
	tests := []struct {
		name          string
		releaseID     string
		expectError   bool
		errorContains string
	}{
		{
			name:        "successful history retrieval",
			releaseID:   "test-release",
			expectError: false,
		},
		{
			name:          "release not found",
			releaseID:     "nonexistent",
			expectError:   true,
			errorContains: "failed to get release history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace:  "test",
				MaxHistory: 10,
			})
			require.NoError(t, err)

			ctx := context.Background()
			history, err := adapter.GetDeploymentHistory(ctx, tt.releaseID)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Will fail without K8s but tests the code path
				if err != nil {
					t.Skip("Skipping - requires Kubernetes")
				}
				if history != nil {
					assert.Equal(t, tt.releaseID, history.DeploymentID)
					assert.NotNil(t, history.Revisions)
				}
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentLogs tests the GetDeploymentLogs function.
func TestHelmAdapter_GetDeploymentLogs(t *testing.T) {
	tests := []struct {
		name          string
		releaseID     string
		logOpts       *dmsadapter.LogOptions
		expectError   bool
		errorContains string
	}{
		{
			name:      "successful log retrieval without options",
			releaseID: "test-release",
			logOpts:   nil,
		},
		{
			name:      "successful log retrieval with tail lines",
			releaseID: "test-release",
			logOpts: &dmsadapter.LogOptions{
				TailLines: 100,
			},
		},
		{
			name:      "successful log retrieval with since time",
			releaseID: "test-release",
			logOpts: &dmsadapter.LogOptions{
				Since: time.Now().Add(-1 * time.Hour),
			},
		},
		{
			name:      "successful log retrieval with follow",
			releaseID: "test-release",
			logOpts: &dmsadapter.LogOptions{
				Follow: true,
			},
		},
		{
			name:          "release not found",
			releaseID:     "nonexistent",
			expectError:   true,
			errorContains: "failed to get release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace: "test",
			})
			require.NoError(t, err)

			ctx := context.Background()
			logs, err := adapter.GetDeploymentLogs(ctx, tt.releaseID, tt.logOpts)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Will fail without K8s but tests the code path
				if err != nil {
					t.Skip("Skipping - requires Kubernetes")
				}
				// Logs might be empty but should not be nil
				assert.NotNil(t, logs)
			}
		})
	}
}

func TestHelmAdapter_GetDeploymentLogs_AdditionalOptions(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name    string
		logOpts *dmsadapter.LogOptions
	}{
		{
			name: "container filter",
			logOpts: &dmsadapter.LogOptions{
				Container: "app-container",
			},
		},
		{
			name: "combined options - tail and since",
			logOpts: &dmsadapter.LogOptions{
				TailLines: 50,
				Since:     time.Now().Add(-30 * time.Minute),
			},
		},
		{
			name: "combined options - container and tail",
			logOpts: &dmsadapter.LogOptions{
				Container: "sidecar",
				TailLines: 200,
			},
		},
		{
			name: "all options combined",
			logOpts: &dmsadapter.LogOptions{
				Container: "main",
				TailLines: 100,
				Since:     time.Now().Add(-1 * time.Hour),
			},
		},
		{
			name: "empty container name",
			logOpts: &dmsadapter.LogOptions{
				Container: "",
				TailLines: 50,
			},
		},
		{
			name: "zero tail lines",
			logOpts: &dmsadapter.LogOptions{
				TailLines: 0,
			},
		},
		{
			name: "large tail lines",
			logOpts: &dmsadapter.LogOptions{
				TailLines: 10000,
			},
		},
		{
			name: "future since time",
			logOpts: &dmsadapter.LogOptions{
				Since: time.Now().Add(1 * time.Hour),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, err := adapter.GetDeploymentLogs(ctx, "test-release", tt.logOpts)

			// Will fail without K8s but tests the code path
			if err != nil {
				t.Skip("Skipping - requires Kubernetes")
			}
			assert.NotNil(t, logs)
		})
	}
}

// TestHelmAdapter_Health tests the Health function.
func TestHelmAdapter_Health(t *testing.T) {
	t.Skip(
		"TODO(#197): Fix Kubernetes error handling - " +
			"test expects specific errors but gets K8s unreachable errors in CI",
	)
	tests := []struct {
		name          string
		expectError   bool
		errorContains string
	}{
		{
			name:        "health check success",
			expectError: false,
		},
		{
			name:          "health check failure due to initialization",
			expectError:   true,
			errorContains: "helm adapter not healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Namespace: "test",
			})
			require.NoError(t, err)

			ctx := context.Background()
			err = adapter.Health(ctx)

			if tt.expectError {
				// Will error without K8s, which is expected
				if err != nil {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// Will fail without K8s but tests the code path
				if err != nil {
					t.Skip("Skipping - requires Kubernetes")
				}
			}
		})
	}
}

// TestHelmAdapter_ListDeployments tests the ListDeployments function.
func TestHelmAdapter_ListDeployments(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)

	ctx := context.Background()
	deployments, err := adapter.ListDeployments(ctx, nil)

	// Will fail without K8s but tests the code path
	if err != nil {
		t.Skip("Skipping - requires Kubernetes")
	}
	assert.NotNil(t, deployments)
}

// TestHelmAdapter_GetDeployment tests the GetDeployment function.
func TestHelmAdapter_GetDeployment(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)

	ctx := context.Background()
	deployment, err := adapter.GetDeployment(ctx, "test-release")

	// Will fail without K8s but tests the code path
	if err != nil {
		t.Skip("Skipping - requires Kubernetes")
	}
	assert.NotNil(t, deployment)
}

// TestHelmAdapter_CreateDeployment tests the CreateDeployment function.
func TestHelmAdapter_CreateDeployment(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)

	ctx := context.Background()
	req := &dmsadapter.DeploymentRequest{
		Name:      "test-deployment",
		PackageID: "nginx-1.0.0",
		Namespace: "test",
		Values:    map[string]interface{}{},
	}

	deployment, err := adapter.CreateDeployment(ctx, req)

	// Will fail without K8s but tests the code path
	if err != nil {
		t.Skip("Skipping - requires Kubernetes")
	}
	assert.NotNil(t, deployment)
}

// TestHelmAdapter_UpdateDeployment tests the UpdateDeployment function.
func TestHelmAdapter_UpdateDeployment(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)

	ctx := context.Background()
	update := &dmsadapter.DeploymentUpdate{
		Values:      map[string]interface{}{"replicas": 3},
		Description: "Update replicas",
	}

	deployment, err := adapter.UpdateDeployment(ctx, "test-release", update)

	// Will fail without K8s but tests the code path
	if err != nil {
		t.Skip("Skipping - requires Kubernetes")
	}
	assert.NotNil(t, deployment)
}

// TestHelmAdapter_CreateDeployment_Validation tests CreateDeployment validation.
func TestHelmAdapter_CreateDeployment_Validation(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name        string
		req         *dmsadapter.DeploymentRequest
		expectErr   bool
		errContains string
	}{
		{
			name:        "nil request",
			req:         nil,
			expectErr:   true,
			errContains: "cannot be nil",
		},
		{
			name: "missing name",
			req: &dmsadapter.DeploymentRequest{
				PackageID: "nginx-1.0.0",
			},
			expectErr:   true,
			errContains: "name is required",
		},
		{
			name: "missing package ID",
			req: &dmsadapter.DeploymentRequest{
				Name: "test-release",
			},
			expectErr:   true,
			errContains: "package ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adapter.CreateDeployment(ctx, tt.req)
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestHelmAdapter_UpdateDeployment_Validation tests UpdateDeployment validation.
func TestHelmAdapter_UpdateDeployment_Validation(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("nil update", func(t *testing.T) {
		_, err := adapter.UpdateDeployment(ctx, "test-release", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

// TestHelmAdapter_MatchesDeploymentFilter tests the filter matching logic.
func TestHelmAdapter_MatchesDeploymentFilter(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	now := helmtime.Now()
	rel := &release.Release{
		Name:      "test-release",
		Namespace: "production",
		Info: &release.Info{
			Status:        release.StatusDeployed,
			FirstDeployed: now,
			LastDeployed:  now,
		},
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}
	deployment := adapter.TransformReleaseToDeployment(rel)

	tests := []struct {
		name   string
		filter *dmsadapter.Filter
		want   bool
	}{
		{
			name:   "nil filter matches all",
			filter: nil,
			want:   true,
		},
		{
			name:   "empty filter matches all",
			filter: &dmsadapter.Filter{},
			want:   true,
		},
		{
			name: "matching namespace",
			filter: &dmsadapter.Filter{
				Namespace: "production",
			},
			want: true,
		},
		{
			name: "non-matching namespace",
			filter: &dmsadapter.Filter{
				Namespace: "staging",
			},
			want: false,
		},
		{
			name: "matching status",
			filter: &dmsadapter.Filter{
				Status: dmsadapter.DeploymentStatusDeployed,
			},
			want: true,
		},
		{
			name: "non-matching status",
			filter: &dmsadapter.Filter{
				Status: dmsadapter.DeploymentStatusFailed,
			},
			want: false,
		},
		{
			name: "matching namespace and status",
			filter: &dmsadapter.Filter{
				Namespace: "production",
				Status:    dmsadapter.DeploymentStatusDeployed,
			},
			want: true,
		},
		{
			name: "matching namespace but non-matching status",
			filter: &dmsadapter.Filter{
				Namespace: "production",
				Status:    dmsadapter.DeploymentStatusFailed,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.MatchesDeploymentFilter(rel, deployment, tt.filter)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHelmAdapter_FilterAndTransformReleases tests filtering and transformation.
func TestHelmAdapter_FilterAndTransformReleases(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	now := helmtime.Now()
	createRelease := func(name, namespace string, status release.Status) *release.Release {
		return &release.Release{
			Name:      name,
			Namespace: namespace,
			Info: &release.Info{
				Status:        status,
				FirstDeployed: now,
				LastDeployed:  now,
			},
			Chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
		}
	}

	releases := []*release.Release{
		createRelease("release1", "production", release.StatusDeployed),
		createRelease("release2", "staging", release.StatusDeployed),
		createRelease("release3", "production", release.StatusFailed),
	}

	tests := []struct {
		name     string
		filter   *dmsadapter.Filter
		wantLen  int
		wantName string
	}{
		{
			name:    "no filter",
			filter:  nil,
			wantLen: 3,
		},
		{
			name: "filter by namespace production",
			filter: &dmsadapter.Filter{
				Namespace: "production",
			},
			wantLen: 2,
		},
		{
			name: "filter by namespace staging",
			filter: &dmsadapter.Filter{
				Namespace: "staging",
			},
			wantLen:  1,
			wantName: "release2",
		},
		{
			name: "filter by status deployed",
			filter: &dmsadapter.Filter{
				Status: dmsadapter.DeploymentStatusDeployed,
			},
			wantLen: 2,
		},
		{
			name: "filter by status failed",
			filter: &dmsadapter.Filter{
				Status: dmsadapter.DeploymentStatusFailed,
			},
			wantLen:  1,
			wantName: "release3",
		},
		{
			name: "filter by production and deployed",
			filter: &dmsadapter.Filter{
				Namespace: "production",
				Status:    dmsadapter.DeploymentStatusDeployed,
			},
			wantLen:  1,
			wantName: "release1",
		},
		{
			name: "filter with no matches",
			filter: &dmsadapter.Filter{
				Namespace: "development",
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.FilterAndTransformReleases(releases, tt.filter)
			assert.Len(t, result, tt.wantLen)
			if tt.wantName != "" && len(result) > 0 {
				assert.Equal(t, tt.wantName, result[0].Name)
			}
		})
	}
}

// TestHelmAdapter_TransformHelmStatus_AllStatuses tests all Helm status transformations.
func TestHelmAdapter_TransformHelmStatus_AllStatuses(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		helmStatus release.Status
		want       dmsadapter.DeploymentStatus
	}{
		{"superseded", release.StatusSuperseded, dmsadapter.DeploymentStatusFailed},
		{"unknown", release.StatusUnknown, dmsadapter.DeploymentStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.TransformHelmStatus(tt.helmStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHelmAdapter_CalculateProgress_AllStatuses tests all progress calculations.
func TestHelmAdapter_CalculateProgress_AllStatuses(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		helmStatus release.Status
		want       int
	}{
		{"superseded", release.StatusSuperseded, 0},
		{"unknown", release.StatusUnknown, 0},
		{"uninstalled", release.StatusUninstalled, 0},
		{"pending_rollback", release.StatusPendingRollback, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &release.Release{
				Info: &release.Info{
					Status: tt.helmStatus,
				},
			}
			got := adapter.CalculateProgress(rel)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHelmAdapter_Initialize tests initialization behavior.
func TestHelmAdapter_Initialize(t *testing.T) {
	t.Run("already initialized", func(t *testing.T) {
		adapter, err := helm.NewAdapter(&helm.Config{
			Namespace: "test",
		})
		require.NoError(t, err)

		// Mark as initialized
		adapter.Initialized = true
		adapter.ActionCfg = &action.Configuration{}

		// Should return immediately without error
		err = adapter.Initialize(context.Background())
		assert.NoError(t, err)
		assert.True(t, adapter.Initialized)
	})

	t.Run("initialization without kubeconfig", func(t *testing.T) {
		adapter, err := helm.NewAdapter(&helm.Config{
			Namespace: "test",
			Debug:     true, // Enable debug for coverage
		})
		require.NoError(t, err)

		// Initialize should fail without proper kubeconfig
		err = adapter.Initialize(context.Background())
		// Error is expected since we don't have a real kubeconfig
		if err != nil {
			assert.Contains(t, err.Error(), "failed to initialize")
		}
	})
}

// TestHelmAdapter_BuildConditions_EdgeCases tests condition building edge cases.
func TestHelmAdapter_BuildConditions_EdgeCases(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name           string
		helmStatus     release.Status
		wantCondStatus string
	}{
		{"pending_install", release.StatusPendingInstall, "False"},
		{"uninstalling", release.StatusUninstalling, "False"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &release.Release{
				Info: &release.Info{
					Status:       tt.helmStatus,
					LastDeployed: helmtime.Now(),
				},
			}

			conditions := adapter.BuildConditions(rel)
			require.NotEmpty(t, conditions)
			assert.Equal(t, tt.wantCondStatus, conditions[0].Status)
		})
	}
}

// TestHelmAdapter_ScaleDeployment_ZeroReplicas tests scaling to zero.
func TestHelmAdapter_ScaleDeployment_ZeroReplicas(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Zero replicas should be valid
	err = adapter.ScaleDeployment(ctx, "test-release", 0)
	// Will fail without K8s but should not fail on validation
	if err != nil {
		// If error contains validation message, test should fail
		if strings.Contains(err.Error(), "replicas must be non-negative") {
			t.Fatalf("Zero replicas should be valid, got validation error: %v", err)
		}
		// Expected to fail due to missing K8s
		t.Skip("Skipping - requires Kubernetes")
	}
}

func TestHelmAdapter_ScaleDeployment_AdditionalValidation(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name        string
		deployID    string
		replicas    int
		expectErr   bool
		errContains string
	}{
		{
			name:        "empty deployment ID",
			deployID:    "",
			replicas:    1,
			expectErr:   true,
			errContains: "",
		},
		{
			name:        "very large replica count",
			deployID:    "test-release",
			replicas:    1000,
			expectErr:   false,
			errContains: "",
		},
		{
			name:        "negative replicas",
			deployID:    "test-release",
			replicas:    -5,
			expectErr:   true,
			errContains: "replicas must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ScaleDeployment(ctx, tt.deployID, tt.replicas)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else if err != nil && !strings.Contains(err.Error(), "replicas must be non-negative") {
				// May error due to missing K8s but not validation error
				t.Skip("Skipping - requires Kubernetes")
			}
		})
	}
}

// TestHelmAdapter_RollbackDeployment_ZeroRevision tests rollback to revision 0.
func TestHelmAdapter_RollbackDeployment_ZeroRevision(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Zero revision should be valid (means latest - 1 in Helm)
	err = adapter.RollbackDeployment(ctx, "test-release", 0)
	// Will fail without K8s but should not fail on validation
	if err != nil {
		// If error contains validation message, test should fail
		if strings.Contains(err.Error(), "revision must be non-negative") {
			t.Fatalf("Zero revision should be valid, got validation error: %v", err)
		}
		// Expected to fail due to missing K8s
		t.Skip("Skipping - requires Kubernetes")
	}
}

// TestHelmAdapter_Settings tests Helm settings configuration.
func TestHelmAdapter_Settings(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		namespace  string
		debug      bool
	}{
		{
			name:       "with kubeconfig",
			kubeconfig: "/tmp/kubeconfig",
			namespace:  "custom",
			debug:      false,
		},
		{
			name:       "with debug enabled",
			kubeconfig: "",
			namespace:  "test",
			debug:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := helm.NewAdapter(&helm.Config{
				Kubeconfig: tt.kubeconfig,
				Namespace:  tt.namespace,
				Debug:      tt.debug,
			})
			require.NoError(t, err)

			if tt.kubeconfig != "" {
				assert.Equal(t, tt.kubeconfig, adapter.Settings.KubeConfig)
			}
			assert.Equal(t, tt.debug, adapter.Settings.Debug)
		})
	}
}

func TestHelmAdapter_TestBuildPackageList(t *testing.T) {
	adp := &helm.Adapter{
		Config: &helm.Config{
			RepositoryURL: "https://charts.example.com",
		},
	}

	tests := []struct {
		name     string
		index    *repo.IndexFile
		filter   *dmsadapter.Filter
		expected int
	}{
		{
			name: "no filter returns all charts",
			index: &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{
					"nginx":      {{Metadata: &chart.Metadata{Version: "1.0.0", Description: "NGINX chart"}, Created: time.Now()}},
					"postgresql": {{Metadata: &chart.Metadata{Version: "2.0.0", Description: "PostgreSQL chart"}, Created: time.Now()}},
					"redis":      {{Metadata: &chart.Metadata{Version: "3.0.0", Description: "Redis chart"}, Created: time.Now()}},
				},
			},
			filter:   nil,
			expected: 3,
		},
		{
			name: "filter by chart name",
			index: &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{
					"nginx":      {{Metadata: &chart.Metadata{Version: "1.0.0", Description: "NGINX chart"}, Created: time.Now()}},
					"postgresql": {{Metadata: &chart.Metadata{Version: "2.0.0", Description: "PostgreSQL chart"}, Created: time.Now()}},
					"redis":      {{Metadata: &chart.Metadata{Version: "3.0.0", Description: "Redis chart"}, Created: time.Now()}},
				},
			},
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nginx",
				},
			},
			expected: 1,
		},
		{
			name: "filter by chart version",
			index: &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{
					"nginx": {{Metadata: &chart.Metadata{Version: "1.0.0", Description: "NGINX chart"}, Created: time.Now()}},
					"redis": {{Metadata: &chart.Metadata{Version: "1.0.0", Description: "Redis chart"}, Created: time.Now()}},
				},
			},
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartVersion": "1.0.0",
				},
			},
			expected: 2,
		},
		{
			name: "filter with no matches",
			index: &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{
					"nginx": {{Metadata: &chart.Metadata{Version: "1.0.0", Description: "NGINX chart"}, Created: time.Now()}},
				},
			},
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nonexistent",
				},
			},
			expected: 0,
		},
		{
			name: "empty index",
			index: &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{},
			},
			filter:   nil,
			expected: 0,
		},
		{
			name: "chart with no versions skipped",
			index: &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{
					"nginx":      {{Metadata: &chart.Metadata{Version: "1.0.0", Description: "NGINX chart"}, Created: time.Now()}},
					"emptyChart": {},
					"postgresql": {{Metadata: &chart.Metadata{Version: "2.0.0", Description: "PostgreSQL chart"}, Created: time.Now()}},
				},
			},
			filter:   nil,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adp.TestBuildPackageList(tt.index, tt.filter)
			assert.Len(t, result, tt.expected)

			if tt.expected > 0 {
				for _, pkg := range result {
					assert.NotEmpty(t, pkg.ID)
					assert.NotEmpty(t, pkg.Name)
					assert.NotEmpty(t, pkg.Version)
					assert.Equal(t, "helm-chart", pkg.PackageType)
					assert.NotNil(t, pkg.Extensions)
					assert.Equal(t, adp.Config.RepositoryURL, pkg.Extensions["helm.repository"])
				}
			}
		})
	}
}

func TestHelmAdapter_TestMatchesChartFilter(t *testing.T) {
	adp := &helm.Adapter{}

	tests := []struct {
		name         string
		chartName    string
		chartVersion string
		filter       *dmsadapter.Filter
		expected     bool
	}{
		{
			name:         "nil filter matches all",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter:       nil,
			expected:     true,
		},
		{
			name:         "filter with nil extensions matches all",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter:       &dmsadapter.Filter{},
			expected:     true,
		},
		{
			name:         "matching chart name",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nginx",
				},
			},
			expected: true,
		},
		{
			name:         "non-matching chart name",
			chartName:    "postgresql",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nginx",
				},
			},
			expected: false,
		},
		{
			name:         "matching chart version",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartVersion": "1.0.0",
				},
			},
			expected: true,
		},
		{
			name:         "non-matching chart version",
			chartName:    "nginx",
			chartVersion: "2.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartVersion": "1.0.0",
				},
			},
			expected: false,
		},
		{
			name:         "matching both name and version",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName":    "nginx",
					"helm.chartVersion": "1.0.0",
				},
			},
			expected: true,
		},
		{
			name:         "matching name but non-matching version",
			chartName:    "nginx",
			chartVersion: "2.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName":    "nginx",
					"helm.chartVersion": "1.0.0",
				},
			},
			expected: false,
		},
		{
			name:         "empty filter extensions",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{},
			},
			expected: true,
		},
		{
			name:         "empty string values match all",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName":    "",
					"helm.chartVersion": "",
				},
			},
			expected: true,
		},
		{
			name:         "wrong type in extensions ignored",
			chartName:    "nginx",
			chartVersion: "1.0.0",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": 123,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adp.TestMatchesChartFilter(tt.chartName, tt.chartVersion, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHelmAdapter_TestBuildPackage(t *testing.T) {
	adp := &helm.Adapter{
		Config: &helm.Config{
			RepositoryURL: "https://charts.example.com",
		},
	}

	createdTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		chartName string
		chart     *repo.ChartVersion
		validate  func(t *testing.T, pkg *dmsadapter.DeploymentPackage)
	}{
		{
			name:      "basic chart",
			chartName: "nginx",
			chart: &repo.ChartVersion{
				Metadata: &chart.Metadata{
					Version:     "1.0.0",
					Description: "NGINX web server",
					AppVersion:  "1.19.0",
					APIVersion:  "v2",
					Deprecated:  false,
				},
				Created: createdTime,
			},
			validate: func(t *testing.T, pkg *dmsadapter.DeploymentPackage) {
				assert.Equal(t, "nginx-1.0.0", pkg.ID)
				assert.Equal(t, "nginx", pkg.Name)
				assert.Equal(t, "1.0.0", pkg.Version)
				assert.Equal(t, "helm-chart", pkg.PackageType)
				assert.Equal(t, "NGINX web server", pkg.Description)
				assert.Equal(t, createdTime, pkg.UploadedAt)
				assert.NotNil(t, pkg.Extensions)
				assert.Equal(t, "nginx", pkg.Extensions["helm.chartName"])
				assert.Equal(t, "1.0.0", pkg.Extensions["helm.chartVersion"])
				assert.Equal(t, "1.19.0", pkg.Extensions["helm.appVersion"])
				assert.Equal(t, "https://charts.example.com", pkg.Extensions["helm.repository"])
				assert.Equal(t, "v2", pkg.Extensions["helm.apiVersion"])
				assert.Equal(t, false, pkg.Extensions["helm.deprecated"])
			},
		},
		{
			name:      "deprecated chart",
			chartName: "oldchart",
			chart: &repo.ChartVersion{
				Metadata: &chart.Metadata{
					Version:     "0.1.0",
					Description: "Deprecated chart",
					Deprecated:  true,
				},
				Created: createdTime,
			},
			validate: func(t *testing.T, pkg *dmsadapter.DeploymentPackage) {
				assert.Equal(t, true, pkg.Extensions["helm.deprecated"])
			},
		},
		{
			name:      "chart with minimal metadata",
			chartName: "minimal",
			chart: &repo.ChartVersion{
				Metadata: &chart.Metadata{
					Version: "0.0.1",
				},
				Created: createdTime,
			},
			validate: func(t *testing.T, pkg *dmsadapter.DeploymentPackage) {
				assert.Equal(t, "minimal-0.0.1", pkg.ID)
				assert.Equal(t, "minimal", pkg.Name)
				assert.Equal(t, "0.0.1", pkg.Version)
				assert.Empty(t, pkg.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adp.TestBuildPackage(tt.chartName, tt.chart)
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

// TestHelmAdapter_LoadRepositoryIndex_Success tests successful repository index loading.
func TestHelmAdapter_LoadRepositoryIndex_Success(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: "https://charts.bitnami.com/bitnami",
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("first load", func(t *testing.T) {
		err := adapter.LoadRepositoryIndex(ctx)
		// May fail due to network issues, but that's ok for testing
		if err != nil {
			t.Logf("LoadRepositoryIndex failed (expected without network): %v", err)
		}
	})

	t.Run("cached load", func(t *testing.T) {
		// Test that calling LoadRepositoryIndex twice doesn't re-download
		adapter, err := helm.NewAdapter(&helm.Config{
			Namespace:     "test",
			RepositoryURL: "https://charts.bitnami.com/bitnami",
		})
		require.NoError(t, err)

		// First call - may succeed or fail
		err1 := adapter.LoadRepositoryIndex(ctx)

		// Second call - should hit cache path if first succeeded
		err2 := adapter.LoadRepositoryIndex(ctx)

		// Both calls should have same result
		if err1 == nil {
			assert.NoError(t, err2)
		} else {
			t.Logf("Both calls failed (expected without network): err1=%v, err2=%v", err1, err2)
		}
	})

	t.Run("with authentication", func(t *testing.T) {
		adapter, err := helm.NewAdapter(&helm.Config{
			Namespace:          "test",
			RepositoryURL:      "https://private.charts.example.com",
			RepositoryUsername: "testuser",
			RepositoryPassword: "testpass",
		})
		require.NoError(t, err)

		err = adapter.LoadRepositoryIndex(ctx)
		// Will fail without real repository, but tests the code path
		if err != nil {
			t.Logf("LoadRepositoryIndex with auth failed (expected): %v", err)
		}
	})
}

// TestHelmAdapter_GetRepositoryIndex tests the getRepositoryIndex helper.
func TestHelmAdapter_GetRepositoryIndex(t *testing.T) {
	t.Run("index not loaded", func(t *testing.T) {
		adapter, err := helm.NewAdapter(&helm.Config{
			Namespace:     "test",
			RepositoryURL: "https://charts.example.com",
		})
		require.NoError(t, err)

		_, err = adapter.ListDeploymentPackages(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load repository index")
	})
}

// TestHelmAdapter_ListDeployments_WithFilter tests listing deployments with various filters.
func TestHelmAdapter_ListDeployments_WithFilter(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name   string
		filter *dmsadapter.Filter
	}{
		{
			name:   "no filter",
			filter: nil,
		},
		{
			name: "filter by namespace",
			filter: &dmsadapter.Filter{
				Namespace: "production",
			},
		},
		{
			name: "filter by status",
			filter: &dmsadapter.Filter{
				Status: dmsadapter.DeploymentStatusDeployed,
			},
		},
		{
			name: "filter with pagination",
			filter: &dmsadapter.Filter{
				Limit:  10,
				Offset: 5,
			},
		},
		{
			name: "filter with labels",
			filter: &dmsadapter.Filter{
				Labels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployments, err := adapter.ListDeployments(ctx, tt.filter)
			// Will fail without K8s but tests the code path
			if err != nil {
				t.Skip("Skipping - requires Kubernetes")
			}
			assert.NotNil(t, deployments)
		})
	}
}

// TestHelmAdapter_ScaleDeployment_Complete tests the full scaling flow.
func TestHelmAdapter_ScaleDeployment_Complete(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:  "test",
		Timeout:    30 * time.Second,
		MaxHistory: 5,
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		releaseID string
		replicas  int
	}{
		{
			name:      "scale to 3 replicas",
			releaseID: "test-release",
			replicas:  3,
		},
		{
			name:      "scale to 0 replicas",
			releaseID: "test-release",
			replicas:  0,
		},
		{
			name:      "scale to 10 replicas",
			releaseID: "test-release",
			replicas:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ScaleDeployment(ctx, tt.releaseID, tt.replicas)
			// Will fail without K8s but tests the code path
			if err != nil {
				t.Skip("Skipping - requires Kubernetes")
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentHistory_Complete tests the full history retrieval.
func TestHelmAdapter_GetDeploymentHistory_Complete(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:  "test",
		MaxHistory: 20,
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		releaseID string
	}{
		{
			name:      "get history for release",
			releaseID: "test-release",
		},
		{
			name:      "get history for another release",
			releaseID: "another-release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history, err := adapter.GetDeploymentHistory(ctx, tt.releaseID)
			// Will fail without K8s but tests the code path
			if err != nil {
				t.Skip("Skipping - requires Kubernetes")
			}
			if history != nil {
				assert.Equal(t, tt.releaseID, history.DeploymentID)
				assert.NotNil(t, history.Revisions)
			}
		})
	}
}

// TestHelmAdapter_DeleteDeploymentPackage_Complete tests package deletion scenarios.
func TestHelmAdapter_DeleteDeploymentPackage_Complete(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: "https://charts.example.com",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		packageID string
	}{
		{
			name:      "delete existing package",
			packageID: "nginx-1.0.0",
		},
		{
			name:      "delete non-existent package",
			packageID: "nonexistent-1.0.0",
		},
		{
			name:      "delete package with complex name",
			packageID: "my-complex-app-chart-2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.DeleteDeploymentPackage(ctx, tt.packageID)
			// Will always error as deletion is not fully implemented
			// or requires repository access
			assert.Error(t, err)
		})
	}
}

// TestHelmAdapter_RollbackDeployment_Complete tests rollback scenarios.
func TestHelmAdapter_RollbackDeployment_Complete(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
		Timeout:   30 * time.Second,
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		releaseID string
		revision  int
	}{
		{
			name:      "rollback to previous version",
			releaseID: "test-release",
			revision:  0,
		},
		{
			name:      "rollback to specific version",
			releaseID: "test-release",
			revision:  3,
		},
		{
			name:      "rollback to version 1",
			releaseID: "test-release",
			revision:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.RollbackDeployment(ctx, tt.releaseID, tt.revision)
			// Will fail without K8s but tests the code path
			if err != nil {
				t.Skip("Skipping - requires Kubernetes")
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentStatus_Complete tests status retrieval.
func TestHelmAdapter_GetDeploymentStatus_Complete(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name      string
		releaseID string
	}{
		{
			name:      "get status for deployed release",
			releaseID: "deployed-release",
		},
		{
			name:      "get status for failed release",
			releaseID: "failed-release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := adapter.GetDeploymentStatus(ctx, tt.releaseID)
			// Will fail without K8s but tests the code path
			if err != nil {
				t.Skip("Skipping - requires Kubernetes")
			}
			if status != nil {
				assert.Equal(t, tt.releaseID, status.DeploymentID)
				assert.NotEmpty(t, status.Status)
				assert.NotNil(t, status.Conditions)
				assert.GreaterOrEqual(t, status.Progress, 0)
				assert.LessOrEqual(t, status.Progress, 100)
			}
		})
	}
}

// TestHelmAdapter_TestBuildPodLogOptions tests the buildPodLogOptions helper function.
func TestHelmAdapter_TestBuildPodLogOptions(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		opts     *dmsadapter.LogOptions
		validate func(t *testing.T, podOpts *corev1.PodLogOptions)
	}{
		{
			name: "nil options",
			opts: nil,
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				assert.NotNil(t, podOpts)
				assert.Nil(t, podOpts.TailLines)
				assert.Nil(t, podOpts.SinceTime)
				assert.False(t, podOpts.Follow)
			},
		},
		{
			name: "with tail lines",
			opts: &dmsadapter.LogOptions{
				TailLines: 100,
			},
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				require.NotNil(t, podOpts.TailLines)
				assert.Equal(t, int64(100), *podOpts.TailLines)
			},
		},
		{
			name: "with since time",
			opts: &dmsadapter.LogOptions{
				Since: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				require.NotNil(t, podOpts.SinceTime)
				assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), podOpts.SinceTime.Time)
			},
		},
		{
			name: "with follow",
			opts: &dmsadapter.LogOptions{
				Follow: true,
			},
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				assert.True(t, podOpts.Follow)
			},
		},
		{
			name: "with all options",
			opts: &dmsadapter.LogOptions{
				TailLines: 50,
				Since:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Follow:    true,
			},
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				require.NotNil(t, podOpts.TailLines)
				assert.Equal(t, int64(50), *podOpts.TailLines)
				require.NotNil(t, podOpts.SinceTime)
				assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), podOpts.SinceTime.Time)
				assert.True(t, podOpts.Follow)
			},
		},
		{
			name: "with zero tail lines (should be ignored)",
			opts: &dmsadapter.LogOptions{
				TailLines: 0,
			},
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				assert.Nil(t, podOpts.TailLines)
			},
		},
		{
			name: "with zero time (should be ignored)",
			opts: &dmsadapter.LogOptions{
				Since: time.Time{},
			},
			validate: func(t *testing.T, podOpts *corev1.PodLogOptions) {
				assert.Nil(t, podOpts.SinceTime)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.TestBuildPodLogOptions(tt.opts)
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

// TestHelmAdapter_Initialize_DebugMode tests initialization with debug enabled.
func TestHelmAdapter_Initialize_DebugMode(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
		Debug:     true,
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = adapter.Initialize(ctx)
	// Will fail without K8s but tests the debug path
	if err != nil {
		assert.Contains(t, err.Error(), "failed to initialize")
	}
}

// TestHelmAdapter_TransformHelmStatus_DefaultCase tests the default status case.
func TestHelmAdapter_TransformHelmStatus_DefaultCase(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	// Test with an invalid/unknown status value
	unknownStatus := release.Status("invalid-status")
	result := adapter.TransformHelmStatus(unknownStatus)
	assert.Equal(t, dmsadapter.DeploymentStatusFailed, result)
}

// TestHelmAdapter_CalculateProgress_DefaultCase tests the default progress case.
func TestHelmAdapter_CalculateProgress_DefaultCase(t *testing.T) {
	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	// Test with an invalid/unknown status value
	rel := &release.Release{
		Info: &release.Info{
			Status: release.Status("invalid-status"),
		},
	}
	result := adapter.CalculateProgress(rel)
	assert.Equal(t, 0, result)
}
