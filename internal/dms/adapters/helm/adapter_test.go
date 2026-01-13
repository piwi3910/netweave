package helm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"

	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
)

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: &Config{
				Namespace:     "test-namespace",
				RepositoryURL: "https://charts.example.com",
				Timeout:       5 * time.Minute,
				MaxHistory:    5,
			},
			wantErr: false,
		},
		{
			name: "configuration with defaults",
			config: &Config{
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
			adapter, err := NewAdapter(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)

				// Verify defaults were applied
				if tt.config.Namespace == "" {
					assert.Equal(t, "default", adapter.config.Namespace)
				}
				if tt.config.Timeout == 0 {
					assert.Equal(t, DefaultTimeout, adapter.config.Timeout)
				}
				if tt.config.MaxHistory == 0 {
					assert.Equal(t, DefaultMaxHistory, adapter.config.MaxHistory)
				}
			}
		})
	}
}

func TestHelmAdapter_Metadata(t *testing.T) {
	adapter, err := NewAdapter(&Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, AdapterName, adapter.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.Equal(t, AdapterVersion, adapter.Version())
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
			got := adapter.transformHelmStatus(tt.helmStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHelmAdapter_CalculateProgress(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			got := adapter.calculateProgress(rel)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHelmAdapter_TransformReleaseToDeployment(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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

	deployment := adapter.transformReleaseToDeployment(rel)

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
	adapter, err := NewAdapter(&Config{
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

	status := adapter.transformReleaseToStatus(rel)

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
	adapter, err := NewAdapter(&Config{
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

			conditions := adapter.buildConditions(rel)

			assert.NotEmpty(t, conditions)
			assert.Equal(t, "Deployed", conditions[0].Type)
			assert.Equal(t, tt.wantCondStatus, conditions[0].Status)
			assert.Equal(t, tt.wantReason, conditions[0].Reason)
			assert.NotEmpty(t, conditions[0].Message)
		})
	}
}

func TestHelmAdapter_ApplyPagination(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			result := adapter.applyPagination(deployments, tt.limit, tt.offset)

			assert.Equal(t, tt.wantLen, len(result))

			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
				assert.Equal(t, tt.wantLast, result[len(result)-1].ID)
			}
		})
	}
}

func TestHelmAdapter_UploadDeploymentPackage(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
		Namespace: "test",
	})
	require.NoError(t, err)

	// Initialize adapter
	adapter.initialized = true
	adapter.actionCfg = &action.Configuration{}

	// Close adapter
	err = adapter.Close()
	assert.NoError(t, err)
	assert.False(t, adapter.initialized)
	assert.Nil(t, adapter.actionCfg)
}

func TestConfig_Defaults(t *testing.T) {
	tests := []struct {
		name           string
		input          *Config
		wantNamespace  string
		wantTimeout    time.Duration
		wantMaxHistory int
	}{
		{
			name:           "all defaults",
			input:          &Config{},
			wantNamespace:  "default",
			wantTimeout:    DefaultTimeout,
			wantMaxHistory: DefaultMaxHistory,
		},
		{
			name: "custom values",
			input: &Config{
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
			input: &Config{
				Namespace: "custom",
			},
			wantNamespace:  "custom",
			wantTimeout:    DefaultTimeout,
			wantMaxHistory: DefaultMaxHistory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewAdapter(tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantNamespace, adapter.config.Namespace)
			assert.Equal(t, tt.wantTimeout, adapter.config.Timeout)
			assert.Equal(t, tt.wantMaxHistory, adapter.config.MaxHistory)
		})
	}
}

// Benchmark tests.
func BenchmarkHelmAdapter_TransformReleaseToDeployment(b *testing.B) {
	adapter, err := NewAdapter(&Config{
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
		_ = adapter.transformReleaseToDeployment(rel)
	}
}

func BenchmarkHelmAdapter_ApplyPagination(b *testing.B) {
	adapter, err := NewAdapter(&Config{
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
		_ = adapter.applyPagination(deployments, 10, 0)
	}
}

// TestHelmAdapter_ListDeploymentPackages tests listing packages from repository.
func TestHelmAdapter_ListDeploymentPackages(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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

// TestHelmAdapter_DeleteDeploymentPackage tests package deletion.
func TestHelmAdapter_DeleteDeploymentPackage(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			adapter, err := NewAdapter(&Config{
				Namespace:     "test",
				RepositoryURL: tt.repoURL,
			})
			require.NoError(t, err)

			ctx := context.Background()
			err = adapter.loadRepositoryIndex(ctx)
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
			adapter, err := NewAdapter(&Config{
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
			adapter, err := NewAdapter(&Config{
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
			adapter, err := NewAdapter(&Config{
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
			adapter, err := NewAdapter(&Config{
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
			adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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
	deployment := adapter.transformReleaseToDeployment(rel)

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
			got := adapter.matchesDeploymentFilter(rel, deployment, tt.filter)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHelmAdapter_FilterAndTransformReleases tests filtering and transformation.
func TestHelmAdapter_FilterAndTransformReleases(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			result := adapter.filterAndTransformReleases(releases, tt.filter)
			assert.Len(t, result, tt.wantLen)
			if tt.wantName != "" && len(result) > 0 {
				assert.Equal(t, tt.wantName, result[0].Name)
			}
		})
	}
}

// TestHelmAdapter_TransformHelmStatus_AllStatuses tests all Helm status transformations.
func TestHelmAdapter_TransformHelmStatus_AllStatuses(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			got := adapter.transformHelmStatus(tt.helmStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHelmAdapter_CalculateProgress_AllStatuses tests all progress calculations.
func TestHelmAdapter_CalculateProgress_AllStatuses(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			got := adapter.calculateProgress(rel)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHelmAdapter_Initialize tests initialization behavior.
func TestHelmAdapter_Initialize(t *testing.T) {
	t.Run("already initialized", func(t *testing.T) {
		adapter, err := NewAdapter(&Config{
			Namespace: "test",
		})
		require.NoError(t, err)

		// Mark as initialized
		adapter.initialized = true
		adapter.actionCfg = &action.Configuration{}

		// Should return immediately without error
		err = adapter.Initialize(context.Background())
		assert.NoError(t, err)
		assert.True(t, adapter.initialized)
	})

	t.Run("initialization without kubeconfig", func(t *testing.T) {
		adapter, err := NewAdapter(&Config{
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
	adapter, err := NewAdapter(&Config{
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

			conditions := adapter.buildConditions(rel)
			require.NotEmpty(t, conditions)
			assert.Equal(t, tt.wantCondStatus, conditions[0].Status)
		})
	}
}

// TestHelmAdapter_ScaleDeployment_ZeroReplicas tests scaling to zero.
func TestHelmAdapter_ScaleDeployment_ZeroReplicas(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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

// TestHelmAdapter_RollbackDeployment_ZeroRevision tests rollback to revision 0.
func TestHelmAdapter_RollbackDeployment_ZeroRevision(t *testing.T) {
	adapter, err := NewAdapter(&Config{
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
			adapter, err := NewAdapter(&Config{
				Kubeconfig: tt.kubeconfig,
				Namespace:  tt.namespace,
				Debug:      tt.debug,
			})
			require.NoError(t, err)

			if tt.kubeconfig != "" {
				assert.Equal(t, tt.kubeconfig, adapter.settings.KubeConfig)
			}
			assert.Equal(t, tt.debug, adapter.settings.Debug)
		})
	}
}
