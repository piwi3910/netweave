package kustomize_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms/adapters/kustomize"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
)

// TestNewAdapter tests adapter creation with various configurations.
func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name    string
		config  *kustomize.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &kustomize.Config{
				Namespace: "default",
			},
			wantErr: false,
		},
		{
			name: "valid config with defaults",
			config: &kustomize.Config{
				Kubeconfig: "/path/to/kubeconfig",
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "config with base URL",
			config: &kustomize.Config{
				Namespace: "custom-ns",
				BaseURL:   "https://github.com/example/repo",
				Timeout:   5 * time.Minute,
				Prune:     true,
				Force:     true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := kustomize.NewAdapter(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, adp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, adp)

				// Verify defaults are applied
				if tt.config.Namespace == "" {
					assert.Equal(t, kustomize.DefaultNamespace, adp.Config.Namespace)
				}
				if tt.config.Timeout == 0 {
					assert.Equal(t, kustomize.DefaultTimeout, adp.Config.Timeout)
				}
			}
		})
	}
}

// TestAdapterMetadata tests adapter metadata methods.
func TestAdapterMetadata(t *testing.T) {
	adp, err := kustomize.NewAdapter(&kustomize.Config{})
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, kustomize.AdapterName, adp.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.Equal(t, kustomize.AdapterVersion, adp.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := adp.Capabilities()
		require.NotEmpty(t, caps)
		assert.Contains(t, caps, dmsadapter.CapabilityDeploymentLifecycle)
		assert.Contains(t, caps, dmsadapter.CapabilityHealthChecks)
	})

	t.Run("SupportsRollback", func(t *testing.T) {
		assert.False(t, adp.SupportsRollback())
	})

	t.Run("SupportsScaling", func(t *testing.T) {
		assert.False(t, adp.SupportsScaling())
	})

	t.Run("SupportsGitOps", func(t *testing.T) {
		assert.True(t, adp.SupportsGitOps())
	})
}

// createFakeAdapter creates an adapter with a fake dynamic client for testing.
func createFakeAdapter(t *testing.T, objects ...runtime.Object) *kustomize.Adapter {
	t.Helper()

	scheme := runtime.NewScheme()

	// Register ConfigMap kinds with the scheme
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "ConfigMap",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "ConfigMapList",
		},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Namespace",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "NamespaceList",
		},
		&unstructured.UnstructuredList{},
	)

	// Create fake dynamic client
	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, err := kustomize.NewAdapter(&kustomize.Config{
		Namespace: "default",
		BaseURL:   "https://github.com/example/kustomize-repo",
	})
	require.NoError(t, err)

	// Set up fake client directly (InitOnce was already called in NewAdapter)
	adp.DynamicClient = client

	return adp
}

// createTestConfigMap creates a test ConfigMap for tracking deployments.
func createTestConfigMap(name, path string, version int) *unstructured.Unstructured {
	namespace := "default"
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":              fmt.Sprintf("kustomize-%s", name),
				"namespace":         namespace,
				"creationTimestamp": time.Now().Format(time.RFC3339),
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "kustomize-adapter",
					"app.kubernetes.io/name":       name,
				},
			},
			"data": map[string]interface{}{
				"name":        name,
				"packageId":   "kustomize-base",
				"path":        path,
				"status":      string(dmsadapter.DeploymentStatusDeployed),
				"version":     fmt.Sprintf("%d", version),
				"createdAt":   time.Now().Format(time.RFC3339),
				"updatedAt":   time.Now().Format(time.RFC3339),
				"description": "Test deployment",
			},
		},
	}
}

// TestListDeployments tests listing Kustomize deployments.
func TestListDeployments(t *testing.T) {
	tests := []struct {
		name        string
		objects     []runtime.Object
		filter      *dmsadapter.Filter
		wantCount   int
		wantErr     bool
		errContains string
	}{
		{
			name: "list all deployments",
			objects: []runtime.Object{
				createTestConfigMap("app1", "./apps/app1", 1),
				createTestConfigMap("app2", "./apps/app2", 2),
			},
			filter:    nil,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty list",
			objects:   []runtime.Object{},
			filter:    nil,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "pagination - limit",
			objects: []runtime.Object{
				createTestConfigMap("app1", "./apps/app1", 1),
				createTestConfigMap("app2", "./apps/app2", 1),
				createTestConfigMap("app3", "./apps/app3", 1),
			},
			filter:    &dmsadapter.Filter{Limit: 2},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "pagination - offset",
			objects: []runtime.Object{
				createTestConfigMap("app1", "./apps/app1", 1),
				createTestConfigMap("app2", "./apps/app2", 1),
				createTestConfigMap("app3", "./apps/app3", 1),
			},
			filter:    &dmsadapter.Filter{Offset: 1, Limit: 10},
			wantCount: 2,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			deployments, err := adp.ListDeployments(context.Background(), tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, deployments, tt.wantCount)
			}
		})
	}
}

// TestGetDeployment tests retrieving a single Kustomize deployment.
func TestGetDeployment(t *testing.T) {
	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name: "get existing deployment",
			objects: []runtime.Object{
				createTestConfigMap("my-app", "./apps/my-app", 1),
			},
			deployID: "my-app",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			deployment, err := adp.GetDeployment(context.Background(), tt.deployID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, deployment)
				assert.Equal(t, tt.deployID, deployment.ID)
				assert.Equal(t, tt.deployID, deployment.Name)
			}
		})
	}
}

// TestCreateDeployment tests creating Kustomize deployments.
func TestCreateDeployment(t *testing.T) {
	tests := []struct {
		name        string
		request     *dmsadapter.DeploymentRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "create valid deployment",
			request: &dmsadapter.DeploymentRequest{
				Name:      "new-app",
				Namespace: "production",
				Extensions: map[string]interface{}{
					"kustomize.path": "./apps/new-app",
				},
			},
			wantErr: false,
		},
		{
			name: "create deployment with defaults",
			request: &dmsadapter.DeploymentRequest{
				Name: "simple-app",
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
				Extensions: map[string]interface{}{
					"kustomize.path": "./apps",
				},
			},
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name: "invalid name - uppercase",
			request: &dmsadapter.DeploymentRequest{
				Name: "MyApp",
			},
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name: "invalid path - traversal",
			request: &dmsadapter.DeploymentRequest{
				Name: "app",
				Extensions: map[string]interface{}{
					"kustomize.path": "../../../etc/passwd",
				},
			},
			wantErr:     true,
			errContains: "cannot contain '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t)
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

// TestUpdateDeployment tests updating Kustomize deployments.
func TestUpdateDeployment(t *testing.T) {
	existing := createTestConfigMap("existing-app", "./apps/existing", 1)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		update      *dmsadapter.DeploymentUpdate
		wantErr     bool
		errContains string
	}{
		{
			name:     "update path",
			objects:  []runtime.Object{existing},
			deployID: "existing-app",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"kustomize.path": "./apps/v2",
				},
			},
			wantErr: false,
		},
		{
			name:     "update description",
			objects:  []runtime.Object{existing},
			deployID: "existing-app",
			update: &dmsadapter.DeploymentUpdate{
				Description: "Updated deployment",
			},
			wantErr: false,
		},
		{
			name:        "nil update",
			objects:     []runtime.Object{existing},
			deployID:    "existing-app",
			update:      nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name:     "deployment not found",
			objects:  []runtime.Object{},
			deployID: "nonexistent",
			update: &dmsadapter.DeploymentUpdate{
				Description: "Test",
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:     "invalid path update",
			objects:  []runtime.Object{existing},
			deployID: "existing-app",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"kustomize.path": "../../../secret",
				},
			},
			wantErr:     true,
			errContains: "cannot contain '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			deployment, err := adp.UpdateDeployment(context.Background(), tt.deployID, tt.update)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, deployment)
				assert.Equal(t, tt.deployID, deployment.ID)
			}
		})
	}
}

// TestDeleteDeployment tests deleting Kustomize deployments.
func TestDeleteDeployment(t *testing.T) {
	existing := createTestConfigMap("app-to-delete", "./apps", 1)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "delete existing deployment",
			objects:  []runtime.Object{existing},
			deployID: "app-to-delete",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			err := adp.DeleteDeployment(context.Background(), tt.deployID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestScaleDeployment tests that scaling is not supported.
func TestScaleDeployment(t *testing.T) {
	adp := createFakeAdapter(t)

	t.Run("scaling not supported", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "any-app", 3)
		require.Error(t, err)
		assert.ErrorIs(t, err, kustomize.ErrOperationNotSupported)
	})

	t.Run("negative replicas", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "any-app", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})
}

// TestRollbackDeployment tests that rollback is not supported.
func TestRollbackDeployment(t *testing.T) {
	adp := createFakeAdapter(t)

	t.Run("rollback not supported", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-app", 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, kustomize.ErrOperationNotSupported)
	})

	t.Run("negative revision", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-app", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})
}

// TestGetDeploymentStatus tests retrieving deployment status.
func TestGetDeploymentStatus(t *testing.T) {
	existing := createTestConfigMap("status-app", "./apps", 1)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantStatus  dmsadapter.DeploymentStatus
		wantErr     bool
		errContains string
	}{
		{
			name:       "get status",
			objects:    []runtime.Object{existing},
			deployID:   "status-app",
			wantStatus: dmsadapter.DeploymentStatusDeployed,
			wantErr:    false,
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			status, err := adp.GetDeploymentStatus(context.Background(), tt.deployID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, status)
				assert.Equal(t, tt.deployID, status.DeploymentID)
				assert.Equal(t, tt.wantStatus, status.Status)
				assert.NotEmpty(t, status.Conditions)
			}
		})
	}
}

// TestGetDeploymentHistory tests retrieving deployment history.
func TestGetDeploymentHistory(t *testing.T) {
	existing := createTestConfigMap("history-app", "./apps", 5)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "get history",
			objects:  []runtime.Object{existing},
			deployID: "history-app",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			history, err := adp.GetDeploymentHistory(context.Background(), tt.deployID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, history)
				assert.Equal(t, tt.deployID, history.DeploymentID)
				assert.NotEmpty(t, history.Revisions)
			}
		})
	}
}

// TestGetDeploymentLogs tests retrieving deployment logs.
func TestGetDeploymentLogs(t *testing.T) {
	existing := createTestConfigMap("logs-app", "./apps", 1)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "get logs",
			objects:  []runtime.Object{existing},
			deployID: "logs-app",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			logs, err := adp.GetDeploymentLogs(context.Background(), tt.deployID, nil)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, logs)
			}
		})
	}
}

// TestListDeploymentPackages tests package listing functionality.
func TestListDeploymentPackages(t *testing.T) {
	adp := createFakeAdapter(t)

	packages, err := adp.ListDeploymentPackages(context.Background(), nil)
	require.NoError(t, err)

	// Should have at least one package (the configured base)
	assert.GreaterOrEqual(t, len(packages), 1)

	for _, pkg := range packages {
		assert.NotEmpty(t, pkg.ID)
		assert.Equal(t, "kustomize", pkg.PackageType)
	}
}

// TestGetDeploymentPackage tests getting a specific package.
func TestGetDeploymentPackage(t *testing.T) {
	adp := createFakeAdapter(t)

	t.Run("package found", func(t *testing.T) {
		expectedID := kustomize.GeneratePackageID("https://github.com/example/kustomize-repo")
		pkg, err := adp.GetDeploymentPackage(context.Background(), expectedID)
		require.NoError(t, err)
		require.NotNil(t, pkg)
		assert.Equal(t, "kustomize", pkg.PackageType)
	})

	t.Run("package not found", func(t *testing.T) {
		_, err := adp.GetDeploymentPackage(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, kustomize.ErrPackageNotFound)
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
				Name:    "my-kustomize-kustomize",
				Version: "v1.0.0",
				Extensions: map[string]interface{}{
					"kustomize.url": "https://github.com/example/repo",
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
		{
			name: "missing url",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:       "my-kustomize-kustomize",
				Extensions: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "kustomize.url extension is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t)
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
				assert.Equal(t, "kustomize", pkg.PackageType)
			}
		})
	}
}

// TestDeleteDeploymentPackage tests that package deletion is not supported.
func TestDeleteDeploymentPackage(t *testing.T) {
	adp := createFakeAdapter(t)
	err := adp.DeleteDeploymentPackage(context.Background(), "any-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, kustomize.ErrOperationNotSupported)
}

// TestHealth tests the health check functionality.
func TestHealth(t *testing.T) {
	t.Run("healthy adapter", func(t *testing.T) {
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "default",
				},
			},
		}
		adp := createFakeAdapter(t, ns)

		err := adp.Health(context.Background())
		require.NoError(t, err)
	})
}

// TestClose tests the adapter close functionality.
func TestClose(t *testing.T) {
	adp := createFakeAdapter(t)

	err := adp.Close()
	require.NoError(t, err)
	assert.Nil(t, adp.DynamicClient)
}

// TestValidateName tests name validation.
func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my-app", false},
		{"valid with numbers", "app-123", false},
		{"valid single char", "a", false},
		{"empty", "", true},
		{"too long", "a23456789012345678901234567890123456789012345678901234567890123456789", true},
		{"uppercase", "MyApp", true},
		{"starts with hyphen", "-app", true},
		{"ends with hyphen", "app-", true},
		{"underscore", "my_app", true},
		{"space", "my app", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := kustomize.ValidateName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, kustomize.ErrInvalidName)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidatePath tests path validation.
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid relative", "./apps", false},
		{"valid nested", "apps/production", false},
		{"empty allowed", "", false},
		{"path traversal", "../../../etc", true},
		{"traversal in middle", "apps/../secrets", true},
		{"absolute path", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := kustomize.ValidatePath(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, kustomize.ErrInvalidPath)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCalculateProgress tests progress calculation.
func TestCalculateProgress(t *testing.T) {
	adp, _ := kustomize.NewAdapter(&kustomize.Config{})

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
			got := adp.CalculateProgress(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestApplyPagination tests pagination logic.
func TestApplyPagination(t *testing.T) {
	adp, _ := kustomize.NewAdapter(&kustomize.Config{})

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
			result := adp.ApplyPagination(deployments, tt.limit, tt.offset)
			assert.Len(t, result, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
			}
		})
	}
}

// TestContextCancellation tests that adapter methods respect context cancellation.
func TestContextCancellation(t *testing.T) {
	adp := createFakeAdapter(t)

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
				return fmt.Errorf("test operation failed: %w", err)
			},
		},
		{
			name: "GetDeployment",
			fn: func() error {
				_, err := adp.GetDeployment(ctx, "test")
				return fmt.Errorf("test operation failed: %w", err)
			},
		},
		{
			name: "CreateDeployment",
			fn: func() error {
				_, err := adp.CreateDeployment(ctx, &dmsadapter.DeploymentRequest{Name: "test"})
				return fmt.Errorf("test operation failed: %w", err)
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

// TestGeneratePackageID tests package ID generation.
func TestGeneratePackageID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "github url",
			url:  "https://github.com/example/repo",
			want: "kustomize-https-github-com-example-repo",
		},
		{
			name: "simple url",
			url:  "http://example.com/kustomize-kustomize",
			want: "kustomize-http-example-com-kustomize-kustomize",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kustomize.GeneratePackageID(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

// BenchmarkListDeployments benchmarks the deployment listing performance.
func BenchmarkListDeployments(b *testing.B) {
	objects := make([]runtime.Object, 100)
	for i := 0; i < 100; i++ {
		objects[i] = createTestConfigMap(
			fmt.Sprintf("app-%d", i),
			fmt.Sprintf("./apps/app-%d", i),
			1,
		)
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMapList"},
		&unstructured.UnstructuredList{},
	)

	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, _ := kustomize.NewAdapter(&kustomize.Config{Namespace: "default"})
	adp.DynamicClient = client

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adp.ListDeployments(ctx, nil)
	}
}
