package flux

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
				Namespace: "flux-system",
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
			name: "config with custom settings",
			config: &Config{
				Namespace:        "custom-ns",
				SourceNamespace:  "sources",
				ReconcileTimeout: 5 * time.Minute,
				Interval:         1 * time.Minute,
				Prune:            true,
				Force:            true,
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
				if tt.config.ReconcileTimeout == 0 {
					assert.Equal(t, DefaultReconcileTimeout, adp.config.ReconcileTimeout)
				}
				if tt.config.Interval == 0 {
					assert.Equal(t, DefaultInterval, adp.config.Interval)
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
		assert.Contains(t, caps, dmsadapter.CapabilityDeploymentLifecycle)
		assert.Contains(t, caps, dmsadapter.CapabilityGitOps)
		assert.Contains(t, caps, dmsadapter.CapabilityRollback)
		assert.Contains(t, caps, dmsadapter.CapabilityHealthChecks)
	})

	t.Run("SupportsRollback", func(t *testing.T) {
		assert.True(t, adp.SupportsRollback())
	})

	t.Run("SupportsScaling", func(t *testing.T) {
		assert.True(t, adp.SupportsScaling())
	})

	t.Run("SupportsGitOps", func(t *testing.T) {
		assert.True(t, adp.SupportsGitOps())
	})
}

// createFakeAdapter creates an adapter with a fake dynamic client for testing.
func createFakeAdapter(t *testing.T, objects ...runtime.Object) *Adapter {
	t.Helper()

	scheme := runtime.NewScheme()

	// Register Flux HelmRelease CRD kinds with the scheme
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   HelmReleaseGroup,
			Version: HelmReleaseVersion,
			Kind:    "HelmRelease",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   HelmReleaseGroup,
			Version: HelmReleaseVersion,
			Kind:    "HelmReleaseList",
		},
		&unstructured.UnstructuredList{},
	)

	// Register Flux Kustomization CRD kinds with the scheme
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   KustomizationGroup,
			Version: KustomizationVersion,
			Kind:    "Kustomization",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   KustomizationGroup,
			Version: KustomizationVersion,
			Kind:    "KustomizationList",
		},
		&unstructured.UnstructuredList{},
	)

	// Register Flux GitRepository CRD kinds with the scheme
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   GitRepositoryGroup,
			Version: GitRepositoryVersion,
			Kind:    "GitRepository",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   GitRepositoryGroup,
			Version: GitRepositoryVersion,
			Kind:    "GitRepositoryList",
		},
		&unstructured.UnstructuredList{},
	)

	// Register Flux HelmRepository CRD kinds with the scheme
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   GitRepositoryGroup,
			Version: GitRepositoryVersion,
			Kind:    "HelmRepository",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   GitRepositoryGroup,
			Version: GitRepositoryVersion,
			Kind:    "HelmRepositoryList",
		},
		&unstructured.UnstructuredList{},
	)

	// Create fake dynamic client
	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, err := NewAdapter(&Config{
		Namespace:       "flux-system",
		SourceNamespace: "flux-system",
	})
	require.NoError(t, err)

	// Use initOnce to set up fake client atomically to prevent race conditions.
	// Setting the client inside the Do() ensures thread-safe initialization.
	adp.initOnce.Do(func() {
		adp.dynamicClient = client
	})

	return adp
}

// createTestHelmRelease creates a test Flux HelmRelease unstructured object.
func createTestHelmRelease(name, chart, sourceRef string, ready bool) *unstructured.Unstructured {
	namespace := "flux-system"
	readyStatus := "True"
	reason := "ReconciliationSucceeded"
	message := "Release reconciliation succeeded"
	if !ready {
		readyStatus = "False"
		reason = "ReconciliationFailed"
		message = "Release reconciliation failed"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", HelmReleaseGroup, HelmReleaseVersion),
			"kind":       "HelmRelease",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"interval": "5m",
				"chart": map[string]interface{}{
					"spec": map[string]interface{}{
						"chart":   chart,
						"version": "1.0.0",
						"sourceRef": map[string]interface{}{
							"kind":      "HelmRepository",
							"name":      sourceRef,
							"namespace": namespace,
						},
					},
				},
				"targetNamespace": "default",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":               "Ready",
						"status":             readyStatus,
						"reason":             reason,
						"message":            message,
						"lastTransitionTime": time.Now().Format(time.RFC3339),
					},
				},
				"history": []interface{}{
					map[string]interface{}{
						"chartVersion": "1.0.0",
						"digest":       "sha256:abc123",
						"status":       "deployed",
					},
				},
			},
		},
	}
}

// createTestKustomization creates a test Flux Kustomization unstructured object.
func createTestKustomization(name, path, sourceRef string, ready bool) *unstructured.Unstructured {
	namespace := "flux-system"
	readyStatus := "True"
	reason := "ReconciliationSucceeded"
	message := "Applied revision: main/abc123"
	if !ready {
		readyStatus = "False"
		reason = "ReconciliationFailed"
		message = "Kustomization reconciliation failed"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", KustomizationGroup, KustomizationVersion),
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"interval": "5m",
				"path":     path,
				"sourceRef": map[string]interface{}{
					"kind":      "GitRepository",
					"name":      sourceRef,
					"namespace": namespace,
				},
				"targetNamespace": "default",
				"prune":           true,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":               "Ready",
						"status":             readyStatus,
						"reason":             reason,
						"message":            message,
						"lastTransitionTime": time.Now().Format(time.RFC3339),
					},
				},
				"lastAppliedRevision": "main/abc123",
			},
		},
	}
}

// createTestGitRepository creates a test Flux GitRepository unstructured object.
func createTestGitRepository(name, namespace, url, branch string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", GitRepositoryGroup, GitRepositoryVersion),
			"kind":       "GitRepository",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"url": url,
				"ref": map[string]interface{}{
					"branch": branch,
				},
				"interval": "1m",
			},
			"status": map[string]interface{}{
				"artifact": map[string]interface{}{
					"revision": fmt.Sprintf("%s/abc123", branch),
				},
			},
		},
	}
}

// createTestHelmRepository creates a test Flux HelmRepository unstructured object.
func createTestHelmRepository(name, namespace, url string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", GitRepositoryGroup, GitRepositoryVersion),
			"kind":       "HelmRepository",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"url":      url,
				"interval": "10m",
				"type":     "default",
			},
			"status": map[string]interface{}{
				"artifact": map[string]interface{}{
					"revision": "sha256:def456",
				},
			},
		},
	}
}

// TestListDeployments tests listing Flux deployments.
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
				createTestHelmRelease("hr1", "nginx", "bitnami", true),
				createTestHelmRelease("hr2", "redis", "bitnami", true),
				createTestKustomization("ks1", "./apps", "infra-repo", true),
			},
			filter:    nil,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "filter by status - deployed",
			objects: []runtime.Object{
				createTestHelmRelease("hr1", "nginx", "bitnami", true),
				createTestHelmRelease("hr2", "redis", "bitnami", false),
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
				createTestHelmRelease("hr1", "nginx", "bitnami", true),
				createTestHelmRelease("hr2", "redis", "bitnami", true),
				createTestHelmRelease("hr3", "postgres", "bitnami", true),
			},
			filter:    &dmsadapter.Filter{Limit: 2},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "pagination - offset",
			objects: []runtime.Object{
				createTestHelmRelease("hr1", "nginx", "bitnami", true),
				createTestHelmRelease("hr2", "redis", "bitnami", true),
				createTestHelmRelease("hr3", "postgres", "bitnami", true),
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

// TestGetDeployment tests retrieving a single Flux deployment.
func TestGetDeployment(t *testing.T) {
	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name: "get existing helmrelease",
			objects: []runtime.Object{
				createTestHelmRelease("my-release", "nginx", "bitnami", true),
			},
			deployID: "my-release",
			wantErr:  false,
		},
		{
			name: "get existing kustomization",
			objects: []runtime.Object{
				createTestKustomization("my-kustomization", "./apps", "infra-repo", true),
			},
			deployID: "my-kustomization",
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

// TestCreateDeployment tests creating Flux deployments.
func TestCreateDeployment(t *testing.T) {
	tests := []struct {
		name        string
		request     *dmsadapter.DeploymentRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "create valid helmrelease",
			request: &dmsadapter.DeploymentRequest{
				Name:      "new-release",
				Namespace: "production",
				Extensions: map[string]interface{}{
					"flux.type":      "helmrelease",
					"flux.chart":     "nginx",
					"flux.sourceRef": "bitnami",
				},
			},
			wantErr: false,
		},
		{
			name: "create valid kustomization",
			request: &dmsadapter.DeploymentRequest{
				Name:      "new-ks",
				Namespace: "production",
				Extensions: map[string]interface{}{
					"flux.type":      "kustomization",
					"flux.path":      "./apps",
					"flux.sourceRef": "infra-repo",
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
				Extensions: map[string]interface{}{
					"flux.chart": "nginx",
				},
			},
			wantErr:     true,
			errContains: "name cannot be empty",
		},
		{
			name: "helmrelease missing chart",
			request: &dmsadapter.DeploymentRequest{
				Name: "release",
				Extensions: map[string]interface{}{
					"flux.type":      "helmrelease",
					"flux.sourceRef": "bitnami",
				},
			},
			wantErr:     true,
			errContains: "flux.chart extension is required",
		},
		{
			name: "kustomization missing sourceRef",
			request: &dmsadapter.DeploymentRequest{
				Name: "ks",
				Extensions: map[string]interface{}{
					"flux.type": "kustomization",
					"flux.path": "./apps",
				},
			},
			wantErr:     true,
			errContains: "flux.sourceRef extension is required",
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
				assert.NotEmpty(t, deployment.PackageID)
			}
		})
	}
}

// TestUpdateDeployment tests updating Flux deployments.
func TestUpdateDeployment(t *testing.T) {
	existingHR := createTestHelmRelease("existing-hr", "nginx", "bitnami", true)
	existingKS := createTestKustomization("existing-ks", "./apps", "infra-repo", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		update      *dmsadapter.DeploymentUpdate
		wantErr     bool
		errContains string
	}{
		{
			name:     "update helmrelease values",
			objects:  []runtime.Object{existingHR},
			deployID: "existing-hr",
			update: &dmsadapter.DeploymentUpdate{
				Values: map[string]interface{}{
					"replicaCount": 3,
				},
			},
			wantErr: false,
		},
		{
			name:     "update helmrelease chart version",
			objects:  []runtime.Object{existingHR},
			deployID: "existing-hr",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"flux.chartVersion": "2.0.0",
				},
			},
			wantErr: false,
		},
		{
			name:     "update kustomization path",
			objects:  []runtime.Object{existingKS},
			deployID: "existing-ks",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"flux.path": "./apps/v2",
				},
			},
			wantErr: false,
		},
		{
			name:        "nil update",
			objects:     []runtime.Object{existingHR},
			deployID:    "existing-hr",
			update:      nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name:     "deployment not found",
			objects:  []runtime.Object{},
			deployID: "nonexistent",
			update: &dmsadapter.DeploymentUpdate{
				Values: map[string]interface{}{},
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

// TestDeleteDeployment tests deleting Flux deployments.
func TestDeleteDeployment(t *testing.T) {
	existingHR := createTestHelmRelease("hr-to-delete", "nginx", "bitnami", true)
	existingKS := createTestKustomization("ks-to-delete", "./apps", "infra-repo", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "delete existing helmrelease",
			objects:  []runtime.Object{existingHR},
			deployID: "hr-to-delete",
			wantErr:  false,
		},
		{
			name:     "delete existing kustomization",
			objects:  []runtime.Object{existingKS},
			deployID: "ks-to-delete",
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

// TestScaleDeployment tests scaling Flux deployments.
func TestScaleDeployment(t *testing.T) {
	existingHR := createTestHelmRelease("scalable-hr", "nginx", "bitnami", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		replicas    int
		wantErr     bool
		errContains string
	}{
		{
			name:     "scale up",
			objects:  []runtime.Object{existingHR},
			deployID: "scalable-hr",
			replicas: 5,
			wantErr:  false,
		},
		{
			name:     "scale down",
			objects:  []runtime.Object{existingHR},
			deployID: "scalable-hr",
			replicas: 1,
			wantErr:  false,
		},
		{
			name:        "negative replicas",
			objects:     []runtime.Object{existingHR},
			deployID:    "scalable-hr",
			replicas:    -1,
			wantErr:     true,
			errContains: "non-negative",
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			replicas:    3,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			err := adp.ScaleDeployment(context.Background(), tt.deployID, tt.replicas)

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

// TestRollbackDeployment tests rollback functionality.
func TestRollbackDeployment(t *testing.T) {
	hrWithHistory := createTestHelmRelease("rollback-hr", "nginx", "bitnami", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		revision    int
		wantErr     bool
		errContains string
	}{
		{
			name:     "rollback helmrelease",
			objects:  []runtime.Object{hrWithHistory},
			deployID: "rollback-hr",
			revision: 0,
			wantErr:  false,
		},
		{
			name:        "negative revision",
			objects:     []runtime.Object{hrWithHistory},
			deployID:    "rollback-hr",
			revision:    -1,
			wantErr:     true,
			errContains: "non-negative",
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			revision:    0,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "revision out of range",
			objects:     []runtime.Object{hrWithHistory},
			deployID:    "rollback-hr",
			revision:    999,
			wantErr:     true,
			errContains: "not found in history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			err := adp.RollbackDeployment(context.Background(), tt.deployID, tt.revision)

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

// TestGetDeploymentStatus tests retrieving deployment status.
func TestGetDeploymentStatus(t *testing.T) {
	healthyHR := createTestHelmRelease("healthy-hr", "nginx", "bitnami", true)
	failedHR := createTestHelmRelease("failed-hr", "nginx", "bitnami", false)
	healthyKS := createTestKustomization("healthy-ks", "./apps", "infra-repo", true)

	tests := []struct {
		name         string
		objects      []runtime.Object
		deployID     string
		wantStatus   dmsadapter.DeploymentStatus
		wantProgress int
		wantErr      bool
		errContains  string
	}{
		{
			name:         "healthy helmrelease",
			objects:      []runtime.Object{healthyHR},
			deployID:     "healthy-hr",
			wantStatus:   dmsadapter.DeploymentStatusDeployed,
			wantProgress: 100,
			wantErr:      false,
		},
		{
			name:         "failed helmrelease",
			objects:      []runtime.Object{failedHR},
			deployID:     "failed-hr",
			wantStatus:   dmsadapter.DeploymentStatusFailed,
			wantProgress: 0,
			wantErr:      false,
		},
		{
			name:         "healthy kustomization",
			objects:      []runtime.Object{healthyKS},
			deployID:     "healthy-ks",
			wantStatus:   dmsadapter.DeploymentStatusDeployed,
			wantProgress: 100,
			wantErr:      false,
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
				assert.Equal(t, tt.wantProgress, status.Progress)
				assert.NotEmpty(t, status.Conditions)
			}
		})
	}
}

// TestGetDeploymentHistory tests retrieving deployment history.
func TestGetDeploymentHistory(t *testing.T) {
	hrWithHistory := createTestHelmRelease("hr-with-history", "nginx", "bitnami", true)
	ksWithHistory := createTestKustomization("ks-with-history", "./apps", "infra-repo", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "get helmrelease history",
			objects:  []runtime.Object{hrWithHistory},
			deployID: "hr-with-history",
			wantErr:  false,
		},
		{
			name:     "get kustomization history",
			objects:  []runtime.Object{ksWithHistory},
			deployID: "ks-with-history",
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

// TestGetDeploymentLogs tests retrieving deployment logs/status.
func TestGetDeploymentLogs(t *testing.T) {
	hr := createTestHelmRelease("hr-for-logs", "nginx", "bitnami", true)
	ks := createTestKustomization("ks-for-logs", "./apps", "infra-repo", true)

	tests := []struct {
		name        string
		objects     []runtime.Object
		deployID    string
		opts        *dmsadapter.LogOptions
		wantErr     bool
		errContains string
	}{
		{
			name:     "get helmrelease logs",
			objects:  []runtime.Object{hr},
			deployID: "hr-for-logs",
			opts:     nil,
			wantErr:  false,
		},
		{
			name:     "get kustomization logs",
			objects:  []runtime.Object{ks},
			deployID: "ks-for-logs",
			opts:     nil,
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			objects:     []runtime.Object{},
			deployID:    "nonexistent",
			opts:        nil,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, tt.objects...)
			logs, err := adp.GetDeploymentLogs(context.Background(), tt.deployID, tt.opts)

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
	gitRepo := createTestGitRepository("infra-repo", "flux-system", "https://github.com/example/infra", "main")
	helmRepo := createTestHelmRepository("bitnami", "flux-system", "https://charts.bitnami.com/bitnami")

	objects := []runtime.Object{gitRepo, helmRepo}

	adp := createFakeAdapter(t, objects...)

	packages, err := adp.ListDeploymentPackages(context.Background(), nil)
	require.NoError(t, err)

	// Should have both git and helm repositories
	assert.GreaterOrEqual(t, len(packages), 1)

	for _, pkg := range packages {
		assert.NotEmpty(t, pkg.ID)
		assert.True(t, pkg.PackageType == "flux-git" || pkg.PackageType == "flux-helm")
		assert.NotNil(t, pkg.Extensions)
	}
}

// TestUploadDeploymentPackage tests package upload (reference creation).
func TestUploadDeploymentPackage(t *testing.T) {
	tests := []struct {
		name        string
		pkg         *dmsadapter.DeploymentPackageUpload
		wantErr     bool
		errContains string
	}{
		{
			name: "valid git package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:    "my-repo",
				Version: "main",
				Extensions: map[string]interface{}{
					"flux.url":  "https://github.com/example/repo",
					"flux.type": "git",
				},
			},
			wantErr: false,
		},
		{
			name: "valid helm package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:    "bitnami",
				Version: "latest",
				Extensions: map[string]interface{}{
					"flux.url":  "https://charts.bitnami.com/bitnami",
					"flux.type": "helm",
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
				Name:       "my-repo",
				Extensions: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "flux.url extension is required",
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
			}
		})
	}
}

// TestDeleteDeploymentPackage tests that package deletion is not supported.
func TestDeleteDeploymentPackage(t *testing.T) {
	adp := createFakeAdapter(t)
	err := adp.DeleteDeploymentPackage(context.Background(), "any-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support package deletion")
}

// TestHealth tests the health check functionality.
func TestHealth(t *testing.T) {
	t.Run("healthy adapter", func(t *testing.T) {
		hr := createTestHelmRelease("test-hr", "nginx", "bitnami", true)
		adp := createFakeAdapter(t, hr)

		err := adp.Health(context.Background())
		require.NoError(t, err)
	})

	t.Run("healthy with empty namespace", func(t *testing.T) {
		adp := createFakeAdapter(t)
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

// TestExtractFluxStatus tests status extraction from conditions.
func TestExtractFluxStatus(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	tests := []struct {
		name       string
		conditions []interface{}
		wantStatus dmsadapter.DeploymentStatus
	}{
		{
			name: "ready true",
			conditions: []interface{}{
				map[string]interface{}{
					"type":    "Ready",
					"status":  "True",
					"reason":  "ReconciliationSucceeded",
					"message": "Release reconciliation succeeded",
				},
			},
			wantStatus: dmsadapter.DeploymentStatusDeployed,
		},
		{
			name: "ready false - failed",
			conditions: []interface{}{
				map[string]interface{}{
					"type":    "Ready",
					"status":  "False",
					"reason":  "ReconciliationFailed",
					"message": "Release reconciliation failed",
				},
			},
			wantStatus: dmsadapter.DeploymentStatusFailed,
		},
		{
			name: "ready false - progressing",
			conditions: []interface{}{
				map[string]interface{}{
					"type":    "Ready",
					"status":  "False",
					"reason":  "Progressing",
					"message": "Reconciliation in progress",
				},
			},
			wantStatus: dmsadapter.DeploymentStatusDeploying,
		},
		{
			name:       "no conditions",
			conditions: []interface{}{},
			wantStatus: dmsadapter.DeploymentStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := adp.extractFluxStatus(tt.conditions)
			assert.Equal(t, tt.wantStatus, status)
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

// TestGeneratePackageID tests package ID generation.
func TestGeneratePackageID(t *testing.T) {
	tests := []struct {
		name    string
		pkgType string
		url     string
		want    string
	}{
		{
			name:    "git repo",
			pkgType: "git",
			url:     "https://github.com/example/repo",
			want:    "git-https-github-com-example-repo",
		},
		{
			name:    "helm repo",
			pkgType: "helm",
			url:     "https://charts.bitnami.com/bitnami",
			want:    "helm-https-charts-bitnami-com-bitnami",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePackageID(tt.pkgType, tt.url)
			assert.Equal(t, tt.want, got)
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
		{
			name:      "no pagination",
			limit:     0,
			offset:    0,
			wantCount: 5,
			wantFirst: "1",
		},
		{
			name:      "limit only",
			limit:     2,
			offset:    0,
			wantCount: 2,
			wantFirst: "1",
		},
		{
			name:      "offset only",
			limit:     10,
			offset:    2,
			wantCount: 3,
			wantFirst: "3",
		},
		{
			name:      "limit and offset",
			limit:     2,
			offset:    1,
			wantCount: 2,
			wantFirst: "2",
		},
		{
			name:      "offset beyond length",
			limit:     10,
			offset:    10,
			wantCount: 0,
			wantFirst: "",
		},
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

// TestGVRs verifies the GVRs are correctly defined.
func TestGVRs(t *testing.T) {
	t.Run("helmReleaseGVR", func(t *testing.T) {
		assert.Equal(t, "helm.toolkit.fluxcd.io", helmReleaseGVR.Group)
		assert.Equal(t, "v2", helmReleaseGVR.Version)
		assert.Equal(t, "helmreleases", helmReleaseGVR.Resource)
	})

	t.Run("kustomizationGVR", func(t *testing.T) {
		assert.Equal(t, "kustomize.toolkit.fluxcd.io", kustomizationGVR.Group)
		assert.Equal(t, "v1", kustomizationGVR.Version)
		assert.Equal(t, "kustomizations", kustomizationGVR.Resource)
	})

	t.Run("gitRepositoryGVR", func(t *testing.T) {
		assert.Equal(t, "source.toolkit.fluxcd.io", gitRepositoryGVR.Group)
		assert.Equal(t, "v1", gitRepositoryGVR.Version)
		assert.Equal(t, "gitrepositories", gitRepositoryGVR.Resource)
	})
}

// TestTransformHelmReleaseToDeployment tests HelmRelease transformation.
func TestTransformHelmReleaseToDeployment(t *testing.T) {
	adp := createFakeAdapter(t)
	hr := createTestHelmRelease("test-release", "nginx", "bitnami", true)

	deployment := adp.transformHelmReleaseToDeployment(hr)

	assert.Equal(t, "test-release", deployment.ID)
	assert.Equal(t, "test-release", deployment.Name)
	assert.Equal(t, dmsadapter.DeploymentStatusDeployed, deployment.Status)
	assert.NotEmpty(t, deployment.PackageID)
	assert.NotNil(t, deployment.Extensions)
	assert.Equal(t, "helmrelease", deployment.Extensions["flux.type"])
}

// TestTransformKustomizationToDeployment tests Kustomization transformation.
func TestTransformKustomizationToDeployment(t *testing.T) {
	adp := createFakeAdapter(t)
	ks := createTestKustomization("test-ks", "./apps", "infra-repo", true)

	deployment := adp.transformKustomizationToDeployment(ks)

	assert.Equal(t, "test-ks", deployment.ID)
	assert.Equal(t, "test-ks", deployment.Name)
	assert.Equal(t, dmsadapter.DeploymentStatusDeployed, deployment.Status)
	assert.NotEmpty(t, deployment.PackageID)
	assert.NotNil(t, deployment.Extensions)
	assert.Equal(t, "kustomization", deployment.Extensions["flux.type"])
}

// BenchmarkListDeployments benchmarks the deployment listing performance.
func BenchmarkListDeployments(b *testing.B) {
	// Create 100 test HelmReleases
	objects := make([]runtime.Object, 100)
	for i := 0; i < 100; i++ {
		objects[i] = createTestHelmRelease(
			fmt.Sprintf("hr-%d", i),
			"flux-system",
			"nginx",
			"bitnami",
			true,
		)
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   HelmReleaseGroup,
			Version: HelmReleaseVersion,
			Kind:    "HelmRelease",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   HelmReleaseGroup,
			Version: HelmReleaseVersion,
			Kind:    "HelmReleaseList",
		},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   KustomizationGroup,
			Version: KustomizationVersion,
			Kind:    "Kustomization",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   KustomizationGroup,
			Version: KustomizationVersion,
			Kind:    "KustomizationList",
		},
		&unstructured.UnstructuredList{},
	)

	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, _ := NewAdapter(&Config{Namespace: "flux-system"})
	adp.dynamicClient = client

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adp.ListDeployments(ctx, nil)
	}
}

// TestConcurrentInitialization tests that adapter initialization is thread-safe.
func TestConcurrentInitialization(t *testing.T) {
	adp := createFakeAdapter(t)

	// Run multiple goroutines that all try to initialize the adapter
	const numGoroutines = 100
	done := make(chan struct{})
	errors := make(chan error, numGoroutines)

	ctx := context.Background()

	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-done // Wait for all goroutines to be ready
			// Each goroutine calls a method that triggers initialization
			_, err := adp.ListDeployments(ctx, nil)
			errors <- err
		}()
	}

	// Release all goroutines at once
	close(done)

	// Collect all errors
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		require.NoError(t, err, "concurrent initialization should not cause errors")
	}

	// Verify the adapter is still usable after concurrent access
	deployments, err := adp.ListDeployments(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, deployments)
}

// TestContextCancellation tests that adapter methods respect context cancellation.
func TestContextCancellation(t *testing.T) {
	adp := createFakeAdapter(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

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
					Name: "test",
					Extensions: map[string]interface{}{
						"flux.chart":     "nginx",
						"flux.sourceRef": "bitnami",
					},
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

// TestGetDeploymentPackage tests getting a specific package.
func TestGetDeploymentPackage(t *testing.T) {
	gitRepo := createTestGitRepository("infra-repo", "flux-system", "https://github.com/example/infra", "main")
	helmRepo := createTestHelmRepository("bitnami", "flux-system", "https://charts.bitnami.com/bitnami")

	t.Run("git package found", func(t *testing.T) {
		adp := createFakeAdapter(t, gitRepo)
		pkg, err := adp.GetDeploymentPackage(context.Background(), "git-https-github-com-example-infra")
		require.NoError(t, err)
		require.NotNil(t, pkg)
		assert.Equal(t, "flux-git", pkg.PackageType)
	})

	t.Run("helm package found", func(t *testing.T) {
		adp := createFakeAdapter(t, helmRepo)
		pkg, err := adp.GetDeploymentPackage(context.Background(), "helm-https-charts-bitnami-com-bitnami")
		require.NoError(t, err)
		require.NotNil(t, pkg)
		assert.Equal(t, "flux-helm", pkg.PackageType)
	})

	t.Run("package not found with git prefix", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.GetDeploymentPackage(context.Background(), "git-nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("package not found with helm prefix", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.GetDeploymentPackage(context.Background(), "helm-nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("package not found without prefix", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.GetDeploymentPackage(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestValidateName tests the name validation function.
func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType error
	}{
		{
			name:    "valid simple name",
			input:   "my-app",
			wantErr: false,
		},
		{
			name:    "valid name with numbers",
			input:   "app-123",
			wantErr: false,
		},
		{
			name:    "valid single char",
			input:   "a",
			wantErr: false,
		},
		{
			name:    "valid max length",
			input:   "a23456789012345678901234567890123456789012345678901234567890123", // 63 chars
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   "",
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "name too long",
			input:   "a234567890123456789012345678901234567890123456789012345678901234", // 64 chars
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "uppercase letters",
			input:   "MyApp",
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "starts with hyphen",
			input:   "-myapp",
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "ends with hyphen",
			input:   "myapp-",
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "contains underscore",
			input:   "my_app",
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "contains space",
			input:   "my app",
			wantErr: true,
			errType: ErrInvalidName,
		},
		{
			name:    "contains special chars",
			input:   "my@app",
			wantErr: true,
			errType: ErrInvalidName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errType)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidatePath tests the path validation function.
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType error
	}{
		{
			name:    "valid relative path",
			input:   "./apps",
			wantErr: false,
		},
		{
			name:    "valid path without prefix",
			input:   "apps/production",
			wantErr: false,
		},
		{
			name:    "empty path is allowed",
			input:   "",
			wantErr: false,
		},
		{
			name:    "valid nested path",
			input:   "clusters/production/apps",
			wantErr: false,
		},
		{
			name:    "path traversal attack",
			input:   "../../../etc/passwd",
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "path traversal in middle",
			input:   "apps/../secrets",
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "absolute path",
			input:   "/etc/passwd",
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "double dot only",
			input:   "..",
			wantErr: true,
			errType: ErrInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errType)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestTypedErrors tests that typed errors work with errors.Is.
func TestTypedErrors(t *testing.T) {
	t.Run("ErrDeploymentNotFound", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.GetDeployment(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDeploymentNotFound)
	})

	t.Run("ErrPackageNotFound", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.GetDeploymentPackage(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})

	t.Run("ErrOperationNotSupported", func(t *testing.T) {
		adp := createFakeAdapter(t)
		err := adp.DeleteDeploymentPackage(context.Background(), "any-id")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOperationNotSupported)
	})

	t.Run("ErrInvalidName on CreateDeployment", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.CreateDeployment(context.Background(), &dmsadapter.DeploymentRequest{
			Name: "INVALID-NAME", // uppercase
			Extensions: map[string]interface{}{
				"flux.chart":     "nginx",
				"flux.sourceRef": "bitnami",
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidName)
	})

	t.Run("ErrInvalidPath on CreateKustomization", func(t *testing.T) {
		adp := createFakeAdapter(t)
		_, err := adp.CreateDeployment(context.Background(), &dmsadapter.DeploymentRequest{
			Name: "valid-name",
			Extensions: map[string]interface{}{
				"flux.type":      "kustomization",
				"flux.path":      "../../../etc/passwd",
				"flux.sourceRef": "infra-repo",
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPath)
	})
}

// TestCreateKustomizationPathValidation tests path validation in kustomization creation.
func TestCreateKustomizationPathValidation(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid path",
			path:    "./apps/production",
			wantErr: false,
		},
		{
			name:        "path traversal blocked",
			path:        "../../../secret",
			wantErr:     true,
			errContains: "cannot contain '..'",
		},
		{
			name:        "absolute path blocked",
			path:        "/etc/passwd",
			wantErr:     true,
			errContains: "absolute paths not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t)
			_, err := adp.CreateDeployment(context.Background(), &dmsadapter.DeploymentRequest{
				Name: "test-ks",
				Extensions: map[string]interface{}{
					"flux.type":      "kustomization",
					"flux.path":      tt.path,
					"flux.sourceRef": "infra-repo",
				},
			})

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

// TestUpdateKustomizationPathValidation tests path validation in kustomization updates.
func TestUpdateKustomizationPathValidation(t *testing.T) {
	existingKS := createTestKustomization("existing-ks", "./apps", "infra-repo", true)

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid path update",
			path:    "./apps/v2",
			wantErr: false,
		},
		{
			name:        "path traversal blocked on update",
			path:        "../../../secret",
			wantErr:     true,
			errContains: "cannot contain '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, existingKS)
			_, err := adp.UpdateDeployment(context.Background(), "existing-ks", &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"flux.path": tt.path,
				},
			})

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
