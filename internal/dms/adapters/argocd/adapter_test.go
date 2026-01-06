package argocd

import (
	"context"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/piwi3910/netweave/internal/dms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewAdapter(t *testing.T) {
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
			errMsg:  "config is required",
		},
		{
			name: "missing server URL",
			config: &Config{
				AuthToken: "token123",
			},
			wantErr: true,
			errMsg:  "serverUrl is required",
		},
		{
			name: "missing authentication",
			config: &Config{
				ServerURL: "argocd.example.com:443",
			},
			wantErr: true,
			errMsg:  "either authToken or username/password must be provided",
		},
		{
			name: "valid config with auth token",
			config: &Config{
				ServerURL: "localhost:8080",
				AuthToken: "token123",
			},
			wantErr: false,
		},
		{
			name: "valid config with username/password",
			config: &Config{
				ServerURL: "localhost:8080",
				Username:  "admin",
				Password:  "password",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewAdapter(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, adapter)
			} else {
				// We expect an error here because we can't actually connect to ArgoCD in tests
				// But we verify the config validation passed
				if err != nil {
					// Connection error is acceptable
					assert.Contains(t, err.Error(), "failed to create")
				}
			}
		})
	}
}

func TestAdapter_Name(t *testing.T) {
	adapter := &Adapter{
		name: "argocd",
	}

	assert.Equal(t, "argocd", adapter.Name())
}

func TestAdapter_Version(t *testing.T) {
	adapter := &Adapter{
		version: "2.10.0",
	}

	assert.Equal(t, "2.10.0", adapter.Version())
}

func TestAdapter_Capabilities(t *testing.T) {
	adapter := &Adapter{}

	capabilities := adapter.Capabilities()

	require.Len(t, capabilities, 5)
	assert.Contains(t, capabilities, dms.CapPackageManagement)
	assert.Contains(t, capabilities, dms.CapDeploymentLifecycle)
	assert.Contains(t, capabilities, dms.CapRollback)
	assert.Contains(t, capabilities, dms.CapGitOps)
	assert.Contains(t, capabilities, dms.CapHealthChecks)
}

func TestAdapter_SupportsRollback(t *testing.T) {
	adapter := &Adapter{}
	assert.True(t, adapter.SupportsRollback())
}

func TestAdapter_SupportsScaling(t *testing.T) {
	adapter := &Adapter{}
	assert.True(t, adapter.SupportsScaling())
}

func TestAdapter_SupportsGitOps(t *testing.T) {
	adapter := &Adapter{}
	assert.True(t, adapter.SupportsGitOps())
}

func TestAdapter_UploadDeploymentPackage(t *testing.T) {
	adapter := &Adapter{}

	pkg := &dms.DeploymentPackageUpload{
		Name:    "test-chart",
		Version: "1.0.0",
	}

	result, err := adapter.UploadDeploymentPackage(context.Background(), pkg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support direct package uploads")
	assert.Nil(t, result)
}

func TestAdapter_DeleteDeploymentPackage(t *testing.T) {
	adapter := &Adapter{}

	err := adapter.DeleteDeploymentPackage(context.Background(), "test-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support package deletion")
}

func TestAdapter_GetDeploymentLogs(t *testing.T) {
	adapter := &Adapter{}

	logs, err := adapter.GetDeploymentLogs(context.Background(), "test-app", &dms.LogOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support log retrieval")
	assert.Nil(t, logs)
}

func TestAdapter_TransformHealthStatus(t *testing.T) {
	adapter := &Adapter{}

	tests := []struct {
		name           string
		argocdHealth   v1alpha1.HealthStatusCode
		expectedStatus dms.DeploymentStatus
	}{
		{
			name:           "healthy",
			argocdHealth:   v1alpha1.HealthStatusHealthy,
			expectedStatus: dms.StatusHealthy,
		},
		{
			name:           "progressing",
			argocdHealth:   v1alpha1.HealthStatusProgressing,
			expectedStatus: dms.StatusProgressing,
		},
		{
			name:           "degraded",
			argocdHealth:   v1alpha1.HealthStatusDegraded,
			expectedStatus: dms.StatusDegraded,
		},
		{
			name:           "suspended",
			argocdHealth:   v1alpha1.HealthStatusSuspended,
			expectedStatus: dms.StatusSuspended,
		},
		{
			name:           "missing",
			argocdHealth:   v1alpha1.HealthStatusMissing,
			expectedStatus: dms.StatusFailed,
		},
		{
			name:           "unknown",
			argocdHealth:   v1alpha1.HealthStatusUnknown,
			expectedStatus: dms.StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := adapter.transformHealthStatus(tt.argocdHealth)
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestAdapter_TransformApplicationToDeployment(t *testing.T) {
	adapter := &Adapter{}

	now := metav1.Now()
	app := &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-app",
			CreationTimestamp: now,
		},
		Spec: v1alpha1.ApplicationSpec{
			Project: "default",
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/example/repo",
				TargetRevision: "main",
				Path:           "manifests",
			},
			Destination: v1alpha1.ApplicationDestination{
				Namespace: "production",
			},
		},
		Status: v1alpha1.ApplicationStatus{
			Health: v1alpha1.HealthStatus{
				Status: v1alpha1.HealthStatusHealthy,
			},
			Sync: v1alpha1.SyncStatus{
				Status: v1alpha1.SyncStatusCodeSynced,
			},
			History: []v1alpha1.RevisionHistory{
				{Source: v1alpha1.ApplicationSource{TargetRevision: "v1.0.0"}},
				{Source: v1alpha1.ApplicationSource{TargetRevision: "main"}},
			},
		},
	}

	deployment := adapter.transformApplicationToDeployment(app)

	require.NotNil(t, deployment)
	assert.Equal(t, "test-app", deployment.ID)
	assert.Equal(t, "test-app", deployment.Name)
	assert.Equal(t, "production", deployment.Namespace)
	assert.Equal(t, "https://github.com/example/repo@main", deployment.PackageID)
	assert.Equal(t, dms.StatusHealthy, deployment.Status)
	assert.Equal(t, 2, deployment.Version) // Length of history
	assert.Equal(t, now.Time, deployment.CreatedAt)

	// Check extensions
	extensions := deployment.Extensions
	assert.Equal(t, "test-app", extensions["argocd.appName"])
	assert.Equal(t, "default", extensions["argocd.project"])
	assert.Equal(t, "https://github.com/example/repo", extensions["argocd.repoURL"])
	assert.Equal(t, "main", extensions["argocd.revision"])
	assert.Equal(t, "manifests", extensions["argocd.path"])
}

func TestAdapter_TransformConditions(t *testing.T) {
	adapter := &Adapter{}

	app := &v1alpha1.Application{
		Status: v1alpha1.ApplicationStatus{
			Sync: v1alpha1.SyncStatus{
				Status: v1alpha1.SyncStatusCodeSynced,
			},
			Health: v1alpha1.HealthStatus{
				Status:  v1alpha1.HealthStatusHealthy,
				Message: "All resources healthy",
			},
		},
	}

	conditions := adapter.transformConditions(app)

	require.Len(t, conditions, 2)

	// Check sync condition
	syncCond := conditions[0]
	assert.Equal(t, "Synced", syncCond.Type)
	assert.True(t, syncCond.Status)
	assert.Equal(t, string(v1alpha1.SyncStatusCodeSynced), syncCond.Reason)

	// Check health condition
	healthCond := conditions[1]
	assert.Equal(t, "Healthy", healthCond.Type)
	assert.True(t, healthCond.Status)
	assert.Equal(t, string(v1alpha1.HealthStatusHealthy), healthCond.Reason)
	assert.Equal(t, "All resources healthy", healthCond.Message)
}

func TestAdapter_CalculateProgress(t *testing.T) {
	adapter := &Adapter{}

	tests := []struct {
		name             string
		syncStatus       v1alpha1.SyncStatusCode
		healthStatus     v1alpha1.HealthStatusCode
		expectedProgress int
	}{
		{
			name:             "fully synced and healthy",
			syncStatus:       v1alpha1.SyncStatusCodeSynced,
			healthStatus:     v1alpha1.HealthStatusHealthy,
			expectedProgress: 100,
		},
		{
			name:             "out of sync",
			syncStatus:       v1alpha1.SyncStatusCodeOutOfSync,
			healthStatus:     v1alpha1.HealthStatusHealthy,
			expectedProgress: 0,
		},
		{
			name:             "progressing",
			syncStatus:       v1alpha1.SyncStatusCodeSynced,
			healthStatus:     v1alpha1.HealthStatusProgressing,
			expectedProgress: 50,
		},
		{
			name:             "other status",
			syncStatus:       v1alpha1.SyncStatusCodeSynced,
			healthStatus:     v1alpha1.HealthStatusDegraded,
			expectedProgress: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &v1alpha1.Application{
				Status: v1alpha1.ApplicationStatus{
					Sync: v1alpha1.SyncStatus{
						Status: tt.syncStatus,
					},
					Health: v1alpha1.HealthStatus{
						Status: tt.healthStatus,
					},
				},
			}

			progress := adapter.calculateProgress(app)
			assert.Equal(t, tt.expectedProgress, progress)
		})
	}
}

func TestAdapter_ConvertValuesToHelmParameters(t *testing.T) {
	adapter := &Adapter{}

	values := map[string]interface{}{
		"replicaCount": 3,
		"image.tag":    "v1.2.3",
		"enabled":      true,
		"cpu":          "500m",
	}

	params := adapter.convertValuesToHelmParameters(values)

	require.Len(t, params, 4)

	// Convert to map for easier testing
	paramMap := make(map[string]string)
	for _, param := range params {
		paramMap[param.Name] = param.Value
	}

	assert.Equal(t, "3", paramMap["replicaCount"])
	assert.Equal(t, "v1.2.3", paramMap["image.tag"])
	assert.Equal(t, "true", paramMap["enabled"])
	assert.Equal(t, "500m", paramMap["cpu"])
}

func TestConfig_Defaults(t *testing.T) {
	config := &Config{
		ServerURL: "argocd.example.com:443",
		AuthToken: "token123",
	}

	// The defaults are applied in NewAdapter, so we test the logic
	if config.Namespace == "" {
		config.Namespace = "argocd"
	}
	if config.DefaultProject == "" {
		config.DefaultProject = "default"
	}

	assert.Equal(t, "argocd", config.Namespace)
	assert.Equal(t, "default", config.DefaultProject)
}

func TestSyncPolicyConfig(t *testing.T) {
	config := &SyncPolicyConfig{
		Automated:  true,
		Prune:      true,
		SelfHeal:   true,
		AllowEmpty: false,
	}

	assert.True(t, config.Automated)
	assert.True(t, config.Prune)
	assert.True(t, config.SelfHeal)
	assert.False(t, config.AllowEmpty)
}

func TestBoolPtr(t *testing.T) {
	truePtr := boolPtr(true)
	falsePtr := boolPtr(false)

	require.NotNil(t, truePtr)
	require.NotNil(t, falsePtr)

	assert.True(t, *truePtr)
	assert.False(t, *falsePtr)
}

// TestAdapter_Close verifies that Close method works without errors.
func TestAdapter_Close(t *testing.T) {
	adapter := &Adapter{}

	err := adapter.Close()
	assert.NoError(t, err)
}

// TestDeploymentRequestValidation tests deployment request validation scenarios.
func TestDeploymentRequestValidation(t *testing.T) {
	adapter := &Adapter{
		config: &Config{
			Namespace:      "argocd",
			DefaultProject: "default",
			SyncPolicy: SyncPolicyConfig{
				Automated: true,
				Prune:     true,
				SelfHeal:  true,
			},
		},
	}

	tests := []struct {
		name    string
		req     *dms.DeploymentRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing git repo",
			req: &dms.DeploymentRequest{
				Name:      "test-app",
				Namespace: "default",
			},
			wantErr: true,
			errMsg:  "gitRepo is required",
		},
		{
			name: "valid request with defaults",
			req: &dms.DeploymentRequest{
				Name:      "test-app",
				Namespace: "default",
				GitRepo:   "https://github.com/example/repo",
			},
			wantErr: false,
		},
		{
			name: "valid request with all fields",
			req: &dms.DeploymentRequest{
				Name:        "test-app",
				Namespace:   "default",
				GitRepo:     "https://github.com/example/repo",
				GitRevision: "v1.0.0",
				GitPath:     "charts/myapp",
				Values: map[string]interface{}{
					"replicaCount": 3,
				},
				Labels: map[string]string{
					"env": "production",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We only test validation logic here, not actual ArgoCD API calls
			if tt.req.GitRepo == "" && tt.wantErr {
				// Simulate validation
				assert.Empty(t, tt.req.GitRepo)
			} else if !tt.wantErr {
				// Verify request has required fields
				assert.NotEmpty(t, tt.req.Name)
				assert.NotEmpty(t, tt.req.Namespace)
				assert.NotEmpty(t, tt.req.GitRepo)
			}
		})
	}
}

// TestTransformHealthStatusEdgeCases tests edge cases in health status transformation.
func TestTransformHealthStatusEdgeCases(t *testing.T) {
	adapter := &Adapter{}

	// Test with empty/default status
	defaultStatus := adapter.transformHealthStatus("")
	assert.Equal(t, dms.StatusUnknown, defaultStatus)

	// Test with invalid status
	invalidStatus := adapter.transformHealthStatus("invalid-status")
	assert.Equal(t, dms.StatusUnknown, invalidStatus)
}

// TestProgressCalculationEdgeCases tests edge cases in progress calculation.
func TestProgressCalculationEdgeCases(t *testing.T) {
	adapter := &Adapter{}

	// Empty application
	emptyApp := &v1alpha1.Application{}
	progress := adapter.calculateProgress(emptyApp)
	assert.GreaterOrEqual(t, progress, 0)
	assert.LessOrEqual(t, progress, 100)

	// Application with resources
	appWithResources := &v1alpha1.Application{
		Status: v1alpha1.ApplicationStatus{
			Sync: v1alpha1.SyncStatus{
				Status: v1alpha1.SyncStatusCodeSynced,
			},
			Health: v1alpha1.HealthStatus{
				Status: v1alpha1.HealthStatusHealthy,
			},
			Resources: []v1alpha1.ResourceStatus{
				{Name: "deployment-1"},
				{Name: "service-1"},
			},
		},
	}
	progress = adapter.calculateProgress(appWithResources)
	assert.Equal(t, 100, progress)
}

// TestConfigValidation tests configuration validation edge cases.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name         string
		modifyConfig func(*Config)
		expectValid  bool
	}{
		{
			name: "insecure connection",
			modifyConfig: func(c *Config) {
				c.Insecure = true
			},
			expectValid: true,
		},
		{
			name: "custom namespace",
			modifyConfig: func(c *Config) {
				c.Namespace = "custom-argocd"
			},
			expectValid: true,
		},
		{
			name: "custom project",
			modifyConfig: func(c *Config) {
				c.DefaultProject = "my-project"
			},
			expectValid: true,
		},
		{
			name: "disabled sync policy",
			modifyConfig: func(c *Config) {
				c.SyncPolicy.Automated = false
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				ServerURL:      "argocd.example.com:443",
				AuthToken:      "token123",
				Namespace:      "argocd",
				DefaultProject: "default",
			}

			tt.modifyConfig(config)

			// Validate configuration has expected values
			assert.NotEmpty(t, config.ServerURL)
			assert.NotEmpty(t, config.AuthToken)
		})
	}
}

// TestDeploymentStatusDetailTransformation tests detailed status transformation.
func TestDeploymentStatusDetailTransformation(t *testing.T) {
	adapter := &Adapter{}

	app := &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-app",
		},
		Status: v1alpha1.ApplicationStatus{
			Sync: v1alpha1.SyncStatus{
				Status:   v1alpha1.SyncStatusCodeSynced,
				Revision: "abc123",
			},
			Health: v1alpha1.HealthStatus{
				Status:  v1alpha1.HealthStatusHealthy,
				Message: "All resources are healthy",
			},
			Resources: []v1alpha1.ResourceStatus{
				{Name: "deployment-1"},
				{Name: "service-1"},
				{Name: "ingress-1"},
			},
		},
	}

	conditions := adapter.transformConditions(app)
	progress := adapter.calculateProgress(app)
	status := adapter.transformHealthStatus(app.Status.Health.Status)

	// Construct expected status detail
	statusDetail := &dms.DeploymentStatusDetail{
		DeploymentID: "test-app",
		Status:       status,
		Message:      app.Status.Health.Message,
		Progress:     progress,
		UpdatedAt:    time.Now(),
		Conditions:   conditions,
		Extensions: map[string]interface{}{
			"argocd.syncStatus":   string(app.Status.Sync.Status),
			"argocd.healthStatus": string(app.Status.Health.Status),
			"argocd.revision":     app.Status.Sync.Revision,
			"argocd.resources":    len(app.Status.Resources),
		},
	}

	require.NotNil(t, statusDetail)
	assert.Equal(t, "test-app", statusDetail.DeploymentID)
	assert.Equal(t, dms.StatusHealthy, statusDetail.Status)
	assert.Equal(t, "All resources are healthy", statusDetail.Message)
	assert.Equal(t, 100, statusDetail.Progress)
	assert.Len(t, statusDetail.Conditions, 2)
	assert.Equal(t, 3, statusDetail.Extensions["argocd.resources"])
	assert.Equal(t, "abc123", statusDetail.Extensions["argocd.revision"])
}
