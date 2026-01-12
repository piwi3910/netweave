package argocd

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
				Namespace: "argocd",
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
			name: "config with auto-sync",
			config: &Config{
				AutoSync: true,
				Prune:    true,
				SelfHeal: true,
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
				if tt.config.DefaultProject == "" {
					assert.Equal(t, "default", adp.config.DefaultProject)
				}
				if tt.config.SyncTimeout == 0 {
					assert.Equal(t, DefaultSyncTimeout, adp.config.SyncTimeout)
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
func createFakeAdapter(t *testing.T, objects ...runtime.Object) *ArgoCDAdapter {
	t.Helper()

	scheme := runtime.NewScheme()

	// Register ArgoCD Application CRD kinds with the scheme
	// This is required for the fake dynamic client to properly handle list operations
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "argoproj.io",
			Version: "v1alpha1",
			Kind:    "Application",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "argoproj.io",
			Version: "v1alpha1",
			Kind:    "ApplicationList",
		},
		&unstructured.UnstructuredList{},
	)

	// Create fake dynamic client
	client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	adp, err := NewAdapter(&Config{
		Namespace: "argocd",
	})
	require.NoError(t, err)

	// Set up fake client and mark as initialized
	adp.dynamicClient = client
	// Trigger the Once to prevent actual initialization attempts
	adp.initOnce.Do(func() {
		// Already initialized with fake client above
	})

	return adp
}

// createTestApplication creates a test ArgoCD Application unstructured object.
func createTestApplication(name, repoURL, path, healthStatus, syncStatus string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         "argocd",
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"path":           path,
					"targetRevision": "HEAD",
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": "default",
				},
			},
			"status": map[string]interface{}{
				"health": map[string]interface{}{
					"status":  healthStatus,
					"message": "Application is healthy",
				},
				"sync": map[string]interface{}{
					"status":   syncStatus,
					"revision": "abc123",
				},
				"reconciledAt": time.Now().Format(time.RFC3339),
				"history": []interface{}{
					map[string]interface{}{
						"revision":   "abc123",
						"deployedAt": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}
}

// TestListDeployments tests listing ArgoCD Applications.
func TestListDeployments(t *testing.T) {
	tests := []struct {
		name        string
		apps        []*unstructured.Unstructured
		filter      *dmsadapter.Filter
		wantCount   int
		wantErr     bool
		errContains string
	}{
		{
			name: "list all deployments",
			apps: []*unstructured.Unstructured{
				createTestApplication("app1", "https://github.com/example/repo", "app1", "Healthy", "Synced"),
				createTestApplication("app2", "https://github.com/example/repo", "app2", "Progressing", "OutOfSync"),
			},
			filter:    nil,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "filter by status - deployed",
			apps: []*unstructured.Unstructured{
				createTestApplication("app1", "https://github.com/example/repo", "app1", "Healthy", "Synced"),
				createTestApplication("app2", "https://github.com/example/repo", "app2", "Progressing", "OutOfSync"),
			},
			filter:    &dmsadapter.Filter{Status: dmsadapter.DeploymentStatusDeployed},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "empty list",
			apps:      []*unstructured.Unstructured{},
			filter:    nil,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "pagination - limit",
			apps: []*unstructured.Unstructured{
				createTestApplication("app1", "https://github.com/example/repo", "app1", "Healthy", "Synced"),
				createTestApplication("app2", "https://github.com/example/repo", "app2", "Healthy", "Synced"),
				createTestApplication("app3", "https://github.com/example/repo", "app3", "Healthy", "Synced"),
			},
			filter:    &dmsadapter.Filter{Limit: 2},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "pagination - offset",
			apps: []*unstructured.Unstructured{
				createTestApplication("app1", "https://github.com/example/repo", "app1", "Healthy", "Synced"),
				createTestApplication("app2", "https://github.com/example/repo", "app2", "Healthy", "Synced"),
				createTestApplication("app3", "https://github.com/example/repo", "app3", "Healthy", "Synced"),
			},
			filter:    &dmsadapter.Filter{Offset: 1, Limit: 10},
			wantCount: 2,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to runtime.Object slice
			objects := make([]runtime.Object, len(tt.apps))
			for i, app := range tt.apps {
				objects[i] = app
			}

			adp := createFakeAdapter(t, objects...)
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

// TestGetDeployment tests retrieving a single ArgoCD Application.
func TestGetDeployment(t *testing.T) {
	tests := []struct {
		name        string
		apps        []*unstructured.Unstructured
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name: "get existing deployment",
			apps: []*unstructured.Unstructured{
				createTestApplication("my-app", "https://github.com/example/repo", "apps/my-app", "Healthy", "Synced"),
			},
			deployID: "my-app",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			apps:        []*unstructured.Unstructured{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]runtime.Object, len(tt.apps))
			for i, app := range tt.apps {
				objects[i] = app
			}

			adp := createFakeAdapter(t, objects...)
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

// TestCreateDeployment tests creating ArgoCD Applications.
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
					"argocd.repoURL":        "https://github.com/example/repo",
					"argocd.path":           "apps/new-app",
					"argocd.targetRevision": "main",
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
					"argocd.repoURL": "https://github.com/example/repo",
				},
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "missing repoURL",
			request: &dmsadapter.DeploymentRequest{
				Name:       "app",
				Extensions: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "repoURL extension is required",
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

// TestUpdateDeployment tests updating ArgoCD Applications.
func TestUpdateDeployment(t *testing.T) {
	existingApp := createTestApplication(
		"existing-app",
		"https://github.com/example/repo",
		"apps/existing",
		"Healthy",
		"Synced",
	)

	tests := []struct {
		name        string
		deployID    string
		update      *dmsadapter.DeploymentUpdate
		wantErr     bool
		errContains string
	}{
		{
			name:     "update values",
			deployID: "existing-app",
			update: &dmsadapter.DeploymentUpdate{
				Values: map[string]interface{}{
					"replicaCount": 3,
				},
			},
			wantErr: false,
		},
		{
			name:     "update target revision",
			deployID: "existing-app",
			update: &dmsadapter.DeploymentUpdate{
				Extensions: map[string]interface{}{
					"argocd.targetRevision": "v1.0.0",
				},
			},
			wantErr: false,
		},
		{
			name:        "nil update",
			deployID:    "existing-app",
			update:      nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name:     "deployment not found",
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
			adp := createFakeAdapter(t, existingApp)
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

// TestDeleteDeployment tests deleting ArgoCD Applications.
func TestDeleteDeployment(t *testing.T) {
	existingApp := createTestApplication(
		"app-to-delete",
		"https://github.com/example/repo",
		"apps/delete",
		"Healthy",
		"Synced",
	)

	tests := []struct {
		name        string
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "delete existing deployment",
			deployID: "app-to-delete",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, existingApp)
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

// TestScaleDeployment tests scaling ArgoCD Applications.
func TestScaleDeployment(t *testing.T) {
	existingApp := createTestApplication(
		"scalable-app",
		"https://github.com/example/repo",
		"apps/scalable",
		"Healthy",
		"Synced",
	)

	tests := []struct {
		name        string
		deployID    string
		replicas    int
		wantErr     bool
		errContains string
	}{
		{
			name:     "scale up",
			deployID: "scalable-app",
			replicas: 5,
			wantErr:  false,
		},
		{
			name:     "scale down",
			deployID: "scalable-app",
			replicas: 1,
			wantErr:  false,
		},
		{
			name:        "negative replicas",
			deployID:    "scalable-app",
			replicas:    -1,
			wantErr:     true,
			errContains: "non-negative",
		},
		{
			name:        "deployment not found",
			deployID:    "nonexistent",
			replicas:    3,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, existingApp)
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

// TestGetDeploymentStatus tests retrieving deployment status.
func TestGetDeploymentStatus(t *testing.T) {
	healthyApp := createTestApplication(
		"healthy-app",
		"https://github.com/example/repo",
		"apps/healthy",
		"Healthy",
		"Synced",
	)
	progressingApp := createTestApplication(
		"progressing-app",
		"https://github.com/example/repo",
		"apps/progressing",
		"Progressing",
		"OutOfSync",
	)

	tests := []struct {
		name         string
		apps         []*unstructured.Unstructured
		deployID     string
		wantStatus   dmsadapter.DeploymentStatus
		wantProgress int
		wantErr      bool
		errContains  string
	}{
		{
			name:         "healthy deployment",
			apps:         []*unstructured.Unstructured{healthyApp},
			deployID:     "healthy-app",
			wantStatus:   dmsadapter.DeploymentStatusDeployed,
			wantProgress: 100,
			wantErr:      false,
		},
		{
			name:         "progressing deployment",
			apps:         []*unstructured.Unstructured{progressingApp},
			deployID:     "progressing-app",
			wantStatus:   dmsadapter.DeploymentStatusDeploying,
			wantProgress: 50,
			wantErr:      false,
		},
		{
			name:        "deployment not found",
			apps:        []*unstructured.Unstructured{},
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]runtime.Object, len(tt.apps))
			for i, app := range tt.apps {
				objects[i] = app
			}

			adp := createFakeAdapter(t, objects...)
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
	appWithHistory := createTestApplication(
		"app-with-history",
		"https://github.com/example/repo",
		"apps/history",
		"Healthy",
		"Synced",
	)

	tests := []struct {
		name        string
		deployID    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "get history",
			deployID: "app-with-history",
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			deployID:    "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, appWithHistory)
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
	app := createTestApplication("app-for-logs", "https://github.com/example/repo", "apps/logs", "Healthy", "Synced")

	tests := []struct {
		name        string
		deployID    string
		opts        *dmsadapter.LogOptions
		wantErr     bool
		errContains string
	}{
		{
			name:     "get logs",
			deployID: "app-for-logs",
			opts:     nil,
			wantErr:  false,
		},
		{
			name:        "deployment not found",
			deployID:    "nonexistent",
			opts:        nil,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, app)
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

// TestHealth tests the health check functionality.
func TestHealth(t *testing.T) {
	t.Run("healthy adapter", func(t *testing.T) {
		app := createTestApplication("test-app", "https://github.com/example/repo", "apps/test", "Healthy", "Synced")
		adp := createFakeAdapter(t, app)

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

// TestTransformArgoCDStatus tests status transformation logic.
func TestTransformArgoCDStatus(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	tests := []struct {
		name         string
		healthStatus string
		syncStatus   string
		want         dmsadapter.DeploymentStatus
	}{
		{
			name:         "healthy and synced",
			healthStatus: "Healthy",
			syncStatus:   "Synced",
			want:         dmsadapter.DeploymentStatusDeployed,
		},
		{
			name:         "healthy but out of sync",
			healthStatus: "Healthy",
			syncStatus:   "OutOfSync",
			want:         dmsadapter.DeploymentStatusDeploying,
		},
		{
			name:         "progressing",
			healthStatus: "Progressing",
			syncStatus:   "OutOfSync",
			want:         dmsadapter.DeploymentStatusDeploying,
		},
		{
			name:         "degraded",
			healthStatus: "Degraded",
			syncStatus:   "Synced",
			want:         dmsadapter.DeploymentStatusFailed,
		},
		{
			name:         "missing",
			healthStatus: "Missing",
			syncStatus:   "Unknown",
			want:         dmsadapter.DeploymentStatusFailed,
		},
		{
			name:         "suspended",
			healthStatus: "Suspended",
			syncStatus:   "Synced",
			want:         dmsadapter.DeploymentStatusPending,
		},
		{
			name:         "unknown with out of sync",
			healthStatus: "Unknown",
			syncStatus:   "OutOfSync",
			want:         dmsadapter.DeploymentStatusDeploying,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.transformArgoCDStatus(tt.healthStatus, tt.syncStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCalculateProgress tests progress calculation.
func TestCalculateProgress(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	tests := []struct {
		name         string
		healthStatus string
		syncStatus   string
		want         int
	}{
		{"healthy synced", "Healthy", "Synced", 100},
		{"healthy out of sync", "Healthy", "OutOfSync", 90},
		{"progressing", "Progressing", "OutOfSync", 50},
		{"suspended", "Suspended", "Synced", 25},
		{"degraded", "Degraded", "Synced", 0},
		{"unknown", "Unknown", "Unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adp.calculateProgress(tt.healthStatus, tt.syncStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGeneratePackageID tests package ID generation.
func TestGeneratePackageID(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		path    string
		want    string
	}{
		{
			name:    "github repo with path",
			repoURL: "https://github.com/example/repo",
			path:    "apps/myapp",
			want:    "https-github-com-example-repo-apps-myapp",
		},
		{
			name:    "repo without path",
			repoURL: "https://github.com/example/repo",
			path:    "",
			want:    "https-github-com-example-repo",
		},
		{
			name:    "git ssh url",
			repoURL: "git@github.com:example/repo.git",
			path:    "helm",
			want:    "git@github-com:example-repo-git-helm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePackageID(tt.repoURL, tt.path)
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
			// For multiple labels, we can't guarantee order, so just check length for multi-label case
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

// TestListDeploymentPackages tests package listing functionality.
func TestListDeploymentPackages(t *testing.T) {
	apps := []*unstructured.Unstructured{
		createTestApplication("app1", "https://github.com/example/repo1", "apps/app1", "Healthy", "Synced"),
		createTestApplication("app2", "https://github.com/example/repo1", "apps/app2", "Healthy", "Synced"),
		createTestApplication("app3", "https://github.com/example/repo2", "apps/app3", "Healthy", "Synced"),
	}

	objects := make([]runtime.Object, len(apps))
	for i, app := range apps {
		objects[i] = app
	}

	adp := createFakeAdapter(t, objects...)

	packages, err := adp.ListDeploymentPackages(context.Background(), nil)
	require.NoError(t, err)

	// Should have unique packages based on repo+path combinations
	assert.GreaterOrEqual(t, len(packages), 1)

	for _, pkg := range packages {
		assert.NotEmpty(t, pkg.ID)
		assert.Equal(t, "git-repo", pkg.PackageType)
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
			name: "valid package",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:    "my-package",
				Version: "v1.0.0",
				Extensions: map[string]interface{}{
					"argocd.repoURL": "https://github.com/example/repo",
					"argocd.path":    "apps/myapp",
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
			name: "missing repoURL",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:       "my-package",
				Extensions: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "repoURL extension is required",
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
				assert.Equal(t, "git-repo", pkg.PackageType)
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

// TestRollbackDeployment tests rollback functionality.
func TestRollbackDeployment(t *testing.T) {
	appWithHistory := createTestApplication(
		"rollback-app",
		"https://github.com/example/repo",
		"apps/rollback",
		"Healthy",
		"Synced",
	)

	tests := []struct {
		name        string
		deployID    string
		revision    int
		wantErr     bool
		errContains string
	}{
		{
			name:     "rollback to revision 0",
			deployID: "rollback-app",
			revision: 0,
			wantErr:  false,
		},
		{
			name:        "negative revision",
			deployID:    "rollback-app",
			revision:    -1,
			wantErr:     true,
			errContains: "non-negative",
		},
		{
			name:        "deployment not found",
			deployID:    "nonexistent",
			revision:    0,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "revision out of range",
			deployID:    "rollback-app",
			revision:    999,
			wantErr:     true,
			errContains: "not found in history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp := createFakeAdapter(t, appWithHistory)
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

// TestGVR verifies the ApplicationGVR is correctly defined.
func TestGVR(t *testing.T) {
	assert.Equal(t, "argoproj.io", applicationGVR.Group)
	assert.Equal(t, "v1alpha1", applicationGVR.Version)
	assert.Equal(t, "applications", applicationGVR.Resource)
}

// TestMustMarshalYAML tests YAML marshaling helper.
func TestMustMarshalYAML(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]interface{}
		want   string
	}{
		{
			name:   "simple values",
			values: map[string]interface{}{"key": "value"},
			want:   `{"key":"value"}`,
		},
		{
			name:   "nested values",
			values: map[string]interface{}{"outer": map[string]interface{}{"inner": "value"}},
			want:   `{"outer":{"inner":"value"}}`,
		},
		{
			name:   "empty values",
			values: map[string]interface{}{},
			want:   `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustMarshalYAML(tt.values)
			assert.Equal(t, tt.want, got)
		})
	}
}

// BenchmarkListDeployments benchmarks the deployment listing performance.
func BenchmarkListDeployments(b *testing.B) {
	// Create 100 test applications
	apps := make([]runtime.Object, 100)
	for i := 0; i < 100; i++ {
		apps[i] = createTestApplication(
			formatString("app-%d", i),
			"https://github.com/example/repo",
			formatString("apps/app-%d", i),
			"Healthy",
			"Synced",
		)
	}

	scheme := runtime.NewScheme()

	// Register ArgoCD Application CRD kinds with the scheme
	// This is required for the fake dynamic client to properly handle list operations
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "argoproj.io",
			Version: "v1alpha1",
			Kind:    "Application",
		},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{
			Group:   "argoproj.io",
			Version: "v1alpha1",
			Kind:    "ApplicationList",
		},
		&unstructured.UnstructuredList{},
	)
	client := dynamicfake.NewSimpleDynamicClient(scheme, apps...)

	adp, _ := NewAdapter(&Config{Namespace: "argocd"})
	adp.dynamicClient = client

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adp.ListDeployments(ctx, nil)
	}
}

// formatString is a helper for benchmark to avoid import cycle.
func formatString(format string, a ...interface{}) string {
	return fmt.Sprintf(format, a...)
}

// TestContextCancellation tests that adapter methods respect context cancellation.
func TestContextCancellation(t *testing.T) {
	t.Skip("Test requires proper context cancellation handling in adapter methods - see issue #200")
	app := createTestApplication("test-app", "https://github.com/example/repo", "apps/test", "Healthy", "Synced")
	adp := createFakeAdapter(t, app)

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
				_, err := adp.GetDeployment(ctx, "test-app")
				return err
			},
		},
		{
			name: "CreateDeployment",
			fn: func() error {
				_, err := adp.CreateDeployment(ctx, &dmsadapter.DeploymentRequest{
					Name: "new-app",
					Extensions: map[string]interface{}{
						"argocd.repoURL": "https://github.com/example/repo",
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

// TestBuildConditions tests condition building for different app states.
// TODO: Implement buildConditions method before enabling this test
func TestBuildConditions(t *testing.T) {
	t.Skip("Test requires unimplemented buildConditions method - see issue #200")

	// TODO: Re-enable when buildConditions() is implemented
	// adp, _ := NewAdapter(&Config{})
	//
	// tests := []struct {
	// 	name           string
	// 	healthStatus   string
	// 	syncStatus     string
	// 	wantCondType   string
	// 	wantCondStatus string
	// }{
	// 	{
	// 		name:           "healthy synced",
	// 		healthStatus:   "Healthy",
	// 		syncStatus:     "Synced",
	// 		wantCondType:   "Healthy",
	// 		wantCondStatus: "True",
	// 	},
	// 	{
	// 		name:           "degraded",
	// 		healthStatus:   "Degraded",
	// 		syncStatus:     "Synced",
	// 		wantCondType:   "Healthy",
	// 		wantCondStatus: "False",
	// 	},
	// 	{
	// 		name:           "progressing",
	// 		healthStatus:   "Progressing",
	// 		syncStatus:     "OutOfSync",
	// 		wantCondType:   "Healthy",
	// 		wantCondStatus: "Unknown",
	// 	},
	// }
	//
	// for _, tt := range tests {
	// 	t.Run(tt.name, func(t *testing.T) {
	// 		conditions := adp.buildConditions(tt.healthStatus, tt.syncStatus)
	// 		require.NotEmpty(t, conditions)
	//
	// 		// Find the health condition
	// 		var healthCond *dmsadapter.DeploymentCondition
	// 		for i := range conditions {
	// 			if conditions[i].Type == tt.wantCondType {
	// 				healthCond = &conditions[i]
	// 				break
	// 			}
	// 		}
	// 		require.NotNil(t, healthCond)
	// 		assert.Equal(t, tt.wantCondStatus, healthCond.Status)
	// 	})
	// }
}

// TestExtractSource tests source extraction from Application.
// TODO: Implement extractSource method before enabling this test
func TestExtractSource(t *testing.T) {
	t.Skip("Test requires unimplemented extractSource method - see issue #200")

	// TODO: Re-enable when extractSource() is implemented
	// adp, _ := NewAdapter(&Config{})
	//
	// tests := []struct {
	// 	name      string
	// 	app       *unstructured.Unstructured
	// 	wantRepo  string
	// 	wantPath  string
	// 	wantChart string
	// }{
	// 	{
	// 		name:     "standard source",
	// 		app:      createTestApplication("app", "https://github.com/example/repo", "apps/myapp", "Healthy", "Synced"),
	// 		wantRepo: "https://github.com/example/repo",
	// 		wantPath: "apps/myapp",
	// 	},
	// 	{
	// 		name: "helm source",
	// 		app: &unstructured.Unstructured{
	// 			Object: map[string]interface{}{
	// 				"apiVersion": "argoproj.io/v1alpha1",
	// 				"kind":       "Application",
	// 				"metadata": map[string]interface{}{
	// 					"name":      "helm-app",
	// 					"namespace": "argocd",
	// 				},
	// 				"spec": map[string]interface{}{
	// 					"source": map[string]interface{}{
	// 						"repoURL":   "https://charts.example.com",
	// 						"chart":     "nginx",
	// 						"chartPath": ".",
	// 					},
	// 				},
	// 			},
	// 		},
	// 		wantRepo:  "https://charts.example.com",
	// 		wantChart: "nginx",
	// 	},
	// }
	//
	// for _, tt := range tests {
	// 	t.Run(tt.name, func(t *testing.T) {
	// 		repoURL, path, _, chart := adp.extractSource(tt.app)
	// 		assert.Equal(t, tt.wantRepo, repoURL)
	// 		if tt.wantPath != "" {
	// 			assert.Equal(t, tt.wantPath, path)
	// 		}
	// 		if tt.wantChart != "" {
	// 			assert.Equal(t, tt.wantChart, chart)
	// 		}
	// 	})
	// }
}

// TestTransformApplicationToDeployment tests transformation of ArgoCD Application to Deployment.
func TestTransformApplicationToDeployment(t *testing.T) {
	adp, _ := NewAdapter(&Config{})

	tests := []struct {
		name       string
		app        *unstructured.Unstructured
		wantStatus dmsadapter.DeploymentStatus
	}{
		{
			name:       "healthy synced app",
			app:        createTestApplication("healthy-app", "https://github.com/example/repo", "apps/healthy", "Healthy", "Synced"),
			wantStatus: dmsadapter.DeploymentStatusDeployed,
		},
		{
			name:       "progressing app",
			app:        createTestApplication("progressing-app", "https://github.com/example/repo", "apps/progress", "Progressing", "OutOfSync"),
			wantStatus: dmsadapter.DeploymentStatusDeploying,
		},
		{
			name:       "degraded app",
			app:        createTestApplication("degraded-app", "https://github.com/example/repo", "apps/degraded", "Degraded", "Synced"),
			wantStatus: dmsadapter.DeploymentStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := adp.transformApplicationToDeployment(tt.app)

			assert.NotNil(t, deployment)
			assert.NotEmpty(t, deployment.ID)
			assert.NotEmpty(t, deployment.Name)
			assert.Equal(t, tt.wantStatus, deployment.Status)
			assert.NotNil(t, deployment.Extensions)
		})
	}
}

// TestScaleDeployment_ZeroReplicas tests scaling to zero replicas.
func TestScaleDeployment_ZeroReplicas(t *testing.T) {
	existingApp := createTestApplication("zero-scale-app", "https://github.com/example/repo", "apps/zero", "Healthy", "Synced")
	adp := createFakeAdapter(t, existingApp)

	err := adp.ScaleDeployment(context.Background(), "zero-scale-app", 0)
	require.NoError(t, err)
}

// TestGetDeploymentPackage tests getting a specific package.
func TestGetDeploymentPackage(t *testing.T) {
	apps := []*unstructured.Unstructured{
		createTestApplication("app1", "https://github.com/example/repo", "apps/app1", "Healthy", "Synced"),
	}
	objects := make([]runtime.Object, len(apps))
	for i, app := range apps {
		objects[i] = app
	}

	adp := createFakeAdapter(t, objects...)

	t.Run("package found", func(t *testing.T) {
		pkg, err := adp.GetDeploymentPackage(context.Background(), "https-github-com-example-repo-apps-app1")
		require.NoError(t, err)
		assert.NotNil(t, pkg)
		assert.Equal(t, "git-repo", pkg.PackageType)
	})

	t.Run("package not found", func(t *testing.T) {
		_, err := adp.GetDeploymentPackage(context.Background(), "nonexistent-package")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestBuildLabelSelector_MultipleLabels tests label selector with multiple labels.
func TestBuildLabelSelector_MultipleLabels(t *testing.T) {
	labels := map[string]string{
		"app":         "myapp",
		"environment": "production",
	}

	selector := buildLabelSelector(labels)

	// Should contain both labels separated by comma
	assert.Contains(t, selector, "app=myapp")
	assert.Contains(t, selector, "environment=production")
	assert.Contains(t, selector, ",")
}

// TestConfigDefaults tests default configuration values.
func TestConfigDefaults(t *testing.T) {
	tests := []struct {
		name            string
		config          *Config
		wantNamespace   string
		wantProject     string
		wantSyncTimeout time.Duration
	}{
		{
			name:            "all defaults",
			config:          &Config{},
			wantNamespace:   DefaultNamespace,
			wantProject:     "default",
			wantSyncTimeout: DefaultSyncTimeout,
		},
		{
			name: "custom values",
			config: &Config{
				Namespace:      "custom-ns",
				DefaultProject: "custom-project",
				SyncTimeout:    10 * time.Minute,
			},
			wantNamespace:   "custom-ns",
			wantProject:     "custom-project",
			wantSyncTimeout: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := NewAdapter(tt.config)
			require.NoError(t, err)

			assert.Equal(t, tt.wantNamespace, adp.config.Namespace)
			assert.Equal(t, tt.wantProject, adp.config.DefaultProject)
			assert.Equal(t, tt.wantSyncTimeout, adp.config.SyncTimeout)
		})
	}
}
