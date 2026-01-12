package crossplane_test

import (
	"context"
	"fmt"
	"testing"
	"time"

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
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Namespace: "default",
			},
			wantErr: false,
		},
		{
			name: "valid config with defaults",
			config: &Config{
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
			name: "config with composition ref",
			config: &Config{
				Namespace:             "crossplane-system",
				DefaultCompositionRef: "my-composition",
				Timeout:               5 * time.Minute,
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
				if tt.config.Namespace == "" {
					assert.Equal(t, DefaultNamespace, adp.config.Namespace)
				}
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
func createFakeAdapter(t *testing.T, objects ...runtime.Object) *Adapter {
	t.Helper()

	scheme := runtime.NewScheme()

	// Register Crossplane Configuration kinds
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   CrossplaneGroup,
			Version: CrossplaneVersion,
			Kind:    "Configuration",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   CrossplaneGroup,
			Version: CrossplaneVersion,
			Kind:    "ConfigurationList",
		},
		&unstructured.UnstructuredList{},
	)

	// Register Crossplane Provider kinds
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   CrossplaneGroup,
			Version: CrossplaneVersion,
			Kind:    "Provider",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   CrossplaneGroup,
			Version: CrossplaneVersion,
			Kind:    "ProviderList",
		},
		&unstructured.UnstructuredList{},
	)

	// Register Composition kinds
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "apiextensions.crossplane.io",
			Version: "v1",
			Kind:    "Composition",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "apiextensions.crossplane.io",
			Version: "v1",
			Kind:    "CompositionList",
		},
		&unstructured.UnstructuredList{},
	)

	// Create fake dynamic client
	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, err := NewAdapter(&Config{
		Namespace: "default",
	})
	require.NoError(t, err)

	// Set up fake client and mark as initialized
	adp.initOnce.Do(func() {
		adp.dynamicClient = client
	})

	return adp
}

// createTestConfiguration creates a test Crossplane Configuration.
func createTestConfiguration(name, packageRef string, healthy bool) *unstructured.Unstructured {
	healthyStatus := "True"
	if !healthy {
		healthyStatus = "False"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", CrossplaneGroup, CrossplaneVersion),
			"kind":       "Configuration",
			"metadata": map[string]interface{}{
				"name":              name,
				"creationTimestamp": time.Now().Format(time.RFC3339),
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "crossplane-adapter",
				},
			},
			"spec": map[string]interface{}{
				"package": packageRef,
			},
			"status": map[string]interface{}{
				"currentRevision": int64(1),
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Healthy",
						"status":  healthyStatus,
						"reason":  "HealthyPackageRevision",
						"message": "Package revision is healthy",
					},
					map[string]interface{}{
						"type":    "Installed",
						"status":  "True",
						"reason":  "ActivePackageRevision",
						"message": "Package revision is active",
					},
				},
			},
		},
	}
}

// createTestComposition creates a test Crossplane Composition.
func createTestComposition(name, compositeKind, apiVersion string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.crossplane.io/v1",
			"kind":       "Composition",
			"metadata": map[string]interface{}{
				"name":              name,
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"compositeTypeRef": map[string]interface{}{
					"kind":       compositeKind,
					"apiVersion": apiVersion,
				},
			},
		},
	}
}

// createTestProvider creates a test Crossplane Provider.
func createTestProvider(name, packageRef string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", CrossplaneGroup, CrossplaneVersion),
			"kind":       "Provider",
			"metadata": map[string]interface{}{
				"name":              name,
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"package": packageRef,
			},
		},
	}
}

// TestListDeployments tests listing Crossplane Configurations.
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
				createTestConfiguration("config1", "xpkg.upbound.io/example/config1:v1.0.0", true),
				createTestConfiguration("config2", "xpkg.upbound.io/example/config2:v1.0.0", true),
			},
			filter:    nil,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "filter by status - deployed",
			objects: []runtime.Object{
				createTestConfiguration("config1", "xpkg.upbound.io/example/config1:v1.0.0", true),
				createTestConfiguration("config2", "xpkg.upbound.io/example/config2:v1.0.0", false),
			},
			filter:    &dmsadapter.Filter{Status: dmsadapter.DeploymentStatusDeployed},
			wantCount: 1,
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
				createTestConfiguration("config1", "xpkg.upbound.io/example/config1:v1.0.0", true),
				createTestConfiguration("config2", "xpkg.upbound.io/example/config2:v1.0.0", true),
				createTestConfiguration("config3", "xpkg.upbound.io/example/config3:v1.0.0", true),
			},
			filter:    &dmsadapter.Filter{Limit: 2},
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

// TestGetDeployment tests retrieving a single Crossplane Configuration.
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
				createTestConfiguration("my-config", "xpkg.upbound.io/example/config:v1.0.0", true),
			},
			deployID: "my-config",
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
			}
		})
	}
}

// TestCreateDeployment tests creating Crossplane Configurations.
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
				Name:      "new-config",
				PackageID: "xpkg.upbound.io/example/config:v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "create with extensions",
			request: &dmsadapter.DeploymentRequest{
				Name: "new-config",
				Extensions: map[string]interface{}{
					"crossplane.package":                  "xpkg.upbound.io/example/config:v1.0.0",
					"crossplane.revisionActivationPolicy": "Automatic",
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
				PackageID: "xpkg.upbound.io/example/config:v1.0.0",
			},
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name: "invalid name - uppercase",
			request: &dmsadapter.DeploymentRequest{
				Name:      "MyConfig",
				PackageID: "xpkg.upbound.io/example/config:v1.0.0",
			},
			wantErr:     true,
			errContains: "DNS-1123",
		},
		{
			name: "missing package reference",
			request: &dmsadapter.DeploymentRequest{
				Name: "config",
			},
			wantErr:     true,
			errContains: "package reference is required",
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

// TestUpdateDeployment tests updating Crossplane Configurations.
func TestUpdateDeployment(t *testing.T) {
	existing := createTestConfiguration("existing-config", "xpkg.upbound.io/example/config:v1.0.0", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		update      *dmsadapter.DeploymentUpdate
		wantErr     bool
		errContains string
	}{
		{
			name:     "update package reference",
			objects:  []runtime.Object{existing},
			deployID: "existing-config",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"crossplane.package": "xpkg.upbound.io/example/config:v2.0.0",
				},
			},
			wantErr: false,
		},
		{
			name:     "update revision policy",
			objects:  []runtime.Object{existing},
			deployID: "existing-config",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"crossplane.revisionActivationPolicy": "Manual",
				},
			},
			wantErr: false,
		},
		{
			name:        "nil update",
			objects:     []runtime.Object{existing},
			deployID:    "existing-config",
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

// TestDeleteDeployment tests deleting Crossplane Configurations.
func TestDeleteDeployment(t *testing.T) {
	existing := createTestConfiguration("config-to-delete", "xpkg.upbound.io/example/config:v1.0.0", true)

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
			deployID: "config-to-delete",
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
		err := adp.ScaleDeployment(context.Background(), "any-config", 3)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOperationNotSupported)
	})

	t.Run("negative replicas", func(t *testing.T) {
		err := adp.ScaleDeployment(context.Background(), "any-config", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})
}

// TestRollbackDeployment tests that rollback is not supported.
func TestRollbackDeployment(t *testing.T) {
	adp := createFakeAdapter(t)

	t.Run("rollback not supported", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-config", 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOperationNotSupported)
	})

	t.Run("negative revision", func(t *testing.T) {
		err := adp.RollbackDeployment(context.Background(), "any-config", -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative")
	})
}

// TestGetDeploymentStatus tests retrieving deployment status.
func TestGetDeploymentStatus(t *testing.T) {
	existing := createTestConfiguration("status-config", "xpkg.upbound.io/example/config:v1.0.0", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantStatus  dmsadapter.DeploymentStatus
		wantErr     bool
		errContains string
	}{
		{
			name:       "get healthy status",
			objects:    []runtime.Object{existing},
			deployID:   "status-config",
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
	existing := createTestConfiguration("history-config", "xpkg.upbound.io/example/config:v1.0.0", true)

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
			deployID: "history-config",
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
	existing := createTestConfiguration("logs-config", "xpkg.upbound.io/example/config:v1.0.0", true)

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
			deployID: "logs-config",
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
	composition := createTestComposition("my-composition", "XDatabase", "example.org/v1")

	adp := createFakeAdapter(t, composition)

	packages, err := adp.ListDeploymentPackages(context.Background(), nil)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(packages), 1)

	for _, pkg := range packages {
		assert.NotEmpty(t, pkg.ID)
		assert.Equal(t, "crossplane-composition", pkg.PackageType)
	}
}

// TestGetDeploymentPackage tests getting a specific package.
func TestGetDeploymentPackage(t *testing.T) {
	composition := createTestComposition("my-composition", "XDatabase", "example.org/v1")

	t.Run("package found", func(t *testing.T) {
		adp := createFakeAdapter(t, composition)
		pkg, err := adp.GetDeploymentPackage(context.Background(), "my-composition")
		require.NoError(t, err)
		require.NotNil(t, pkg)
		assert.Equal(t, "crossplane-composition", pkg.PackageType)
	})

	t.Run("package not found", func(t *testing.T) {
		adp := createFakeAdapter(t)
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
				Name:    "my-composition",
				Version: "v1.0.0",
				Extensions: map[string]interface{}{
					"crossplane.compositionRef": "my-composition",
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
			name: "missing composition ref",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:       "my-composition",
				Extensions: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "compositionRef",
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
				assert.Equal(t, "crossplane-composition", pkg.PackageType)
			}
		})
	}
}

// TestDeleteDeploymentPackage tests that package deletion is not supported.
func TestDeleteDeploymentPackage(t *testing.T) {
	adp := createFakeAdapter(t)
	err := adp.DeleteDeploymentPackage(context.Background(), "any-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOperationNotSupported)
}

// TestHealth tests the health check functionality.
func TestHealth(t *testing.T) {
	t.Run("healthy adapter", func(t *testing.T) {
		provider := createTestProvider("provider-aws", "xpkg.upbound.io/upbound/provider-aws:v0.42.0")
		adp := createFakeAdapter(t, provider)

		err := adp.Health(context.Background())
		require.NoError(t, err)
	})
}

// TestClose tests the adapter close functionality.
func TestClose(t *testing.T) {
	adp := createFakeAdapter(t)

	err := adp.Close()
	require.NoError(t, err)
	assert.Nil(t, adp.dynamicClient)
}

// TestValidateName tests name validation.
func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my-config", false},
		{"valid with numbers", "config-123", false},
		{"valid single char", "a", false},
		{"empty", "", true},
		{"too long", "a23456789012345678901234567890123456789012345678901234567890123456789", true},
		{"uppercase", "MyConfig", true},
		{"starts with hyphen", "-config", true},
		{"ends with hyphen", "config-", true},
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
					PackageID: "xpkg.upbound.io/example/config:v1.0.0",
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

// TestBuildLabelSelector tests label selector building.
func TestBuildLabelSelector(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "single label",
			labels: map[string]string{"app": "myapp"},
			want:   "app=myapp",
		},
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLabelSelector(tt.labels)
			if len(tt.labels) <= 1 {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// BenchmarkListDeployments benchmarks the deployment listing performance.
func BenchmarkListDeployments(b *testing.B) {
	objects := make([]runtime.Object, 100)
	for i := 0; i < 100; i++ {
		objects[i] = createTestConfiguration(
			fmt.Sprintf("config-%d", i),
			fmt.Sprintf("xpkg.upbound.io/example/config%d:v1.0.0", i),
			true,
		)
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: CrossplaneGroup, Version: CrossplaneVersion, Kind: "Configuration"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: CrossplaneGroup, Version: CrossplaneVersion, Kind: "ConfigurationList"},
		&unstructured.UnstructuredList{},
	)

	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, _ := NewAdapter(&Config{Namespace: "default"})
	adp.dynamicClient = client

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adp.ListDeployments(ctx, nil)
	}
}
