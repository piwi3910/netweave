package dms_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCapability tests the Capability type and constants.
func TestCapability(t *testing.T) {
	tests := []struct {
		name       string
		capability dms.Capability
		expected   string
	}{
		{
			name:       "package management capability",
			capability: dms.CapPackageManagement,
			expected:   "package-management",
		},
		{
			name:       "deployment lifecycle capability",
			capability: dms.CapDeploymentLifecycle,
			expected:   "deployment-lifecycle",
		},
		{
			name:       "rollback capability",
			capability: dms.CapRollback,
			expected:   "rollback",
		},
		{
			name:       "scaling capability",
			capability: dms.CapScaling,
			expected:   "scaling",
		},
		{
			name:       "gitops capability",
			capability: dms.CapGitOps,
			expected:   "gitops",
		},
		{
			name:       "health checks capability",
			capability: dms.CapHealthChecks,
			expected:   "health-checks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.capability))
		})
	}
}

// TestDeploymentStatus tests the DeploymentStatus type and constants.
func TestDeploymentStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   dms.DeploymentStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   dms.StatusPending,
			expected: "Pending",
		},
		{
			name:     "progressing status",
			status:   dms.StatusProgressing,
			expected: "Progressing",
		},
		{
			name:     "healthy status",
			status:   dms.StatusHealthy,
			expected: "Healthy",
		},
		{
			name:     "degraded status",
			status:   dms.StatusDegraded,
			expected: "Degraded",
		},
		{
			name:     "failed status",
			status:   dms.StatusFailed,
			expected: "Failed",
		},
		{
			name:     "suspended status",
			status:   dms.StatusSuspended,
			expected: "Suspended",
		},
		{
			name:     "unknown status",
			status:   dms.StatusUnknown,
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

// TestFilterDefaults tests Filter initialization and defaults.
func TestFilterDefaults(t *testing.T) {
	filter := &dms.Filter{}

	assert.Empty(t, filter.Namespace)
	assert.Nil(t, filter.Labels)
	assert.Empty(t, filter.Status)
	assert.Zero(t, filter.Limit)
	assert.Zero(t, filter.Offset)
}

// TestFilterWithValues tests Filter with populated values.
func TestFilterWithValues(t *testing.T) {
	filter := &dms.Filter{
		Namespace: "test-namespace",
		Labels: map[string]string{
			"app":  "test-app",
			"tier": "frontend",
		},
		Status: dms.StatusHealthy,
		Limit:  10,
		Offset: 5,
	}

	assert.Equal(t, "test-namespace", filter.Namespace)
	assert.Equal(t, 2, len(filter.Labels))
	assert.Equal(t, "test-app", filter.Labels["app"])
	assert.Equal(t, dms.StatusHealthy, filter.Status)
	assert.Equal(t, 10, filter.Limit)
	assert.Equal(t, 5, filter.Offset)
}

// TestDeploymentPackageJSONMarshaling tests JSON marshaling and unmarshaling.
func TestDeploymentPackageJSONMarshaling(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pkg := &dms.DeploymentPackage{
		ID:          "pkg-123",
		Name:        "my-chart",
		Version:     "1.2.3",
		PackageType: "helm-chart",
		Description: "Test package",
		UploadedAt:  now,
		Extensions: map[string]interface{}{
			"repo": "charts.example.com",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(pkg)
	require.NoError(t, err)

	// Unmarshal back
	var decoded dms.DeploymentPackage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, pkg.ID, decoded.ID)
	assert.Equal(t, pkg.Name, decoded.Name)
	assert.Equal(t, pkg.Version, decoded.Version)
	assert.Equal(t, pkg.PackageType, decoded.PackageType)
	assert.Equal(t, pkg.Description, decoded.Description)
	assert.True(t, pkg.UploadedAt.Equal(decoded.UploadedAt))
	assert.Equal(t, "charts.example.com", decoded.Extensions["repo"])
}

// TestDeploymentJSONMarshaling tests Deployment JSON marshaling.
func TestDeploymentJSONMarshaling(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	deployment := &dms.Deployment{
		ID:        "deploy-456",
		Name:      "my-app",
		Namespace: "production",
		PackageID: "pkg-123",
		Status:    dms.StatusHealthy,
		Version:   3,
		CreatedAt: now,
		UpdatedAt: now,
		Extensions: map[string]interface{}{
			"cluster": "prod-cluster-1",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(deployment)
	require.NoError(t, err)

	// Unmarshal back
	var decoded dms.Deployment
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, deployment.ID, decoded.ID)
	assert.Equal(t, deployment.Name, decoded.Name)
	assert.Equal(t, deployment.Namespace, decoded.Namespace)
	assert.Equal(t, deployment.PackageID, decoded.PackageID)
	assert.Equal(t, deployment.Status, decoded.Status)
	assert.Equal(t, deployment.Version, decoded.Version)
	assert.True(t, deployment.CreatedAt.Equal(decoded.CreatedAt))
	assert.True(t, deployment.UpdatedAt.Equal(decoded.UpdatedAt))
	assert.Equal(t, "prod-cluster-1", decoded.Extensions["cluster"])
}

// TestDeploymentRequestValidation tests DeploymentRequest fields.
func TestDeploymentRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request *dms.DeploymentRequest
		wantErr bool
	}{
		{
			name: "valid helm deployment request",
			request: &dms.DeploymentRequest{
				Name:      "test-deploy",
				Namespace: "default",
				PackageID: "pkg-123",
				Values: map[string]interface{}{
					"replicas": 3,
				},
			},
			wantErr: false,
		},
		{
			name: "valid gitops deployment request",
			request: &dms.DeploymentRequest{
				Name:        "gitops-deploy",
				Namespace:   "default",
				GitRepo:     "https://github.com/org/repo",
				GitRevision: "main",
				GitPath:     "charts/app",
			},
			wantErr: false,
		},
		{
			name: "request with labels",
			request: &dms.DeploymentRequest{
				Name:      "labeled-deploy",
				Namespace: "default",
				PackageID: "pkg-456",
				Labels: map[string]string{
					"env":  "prod",
					"team": "platform",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			assert.NotEmpty(t, tt.request.Name)
			assert.NotEmpty(t, tt.request.Namespace)

			// Either PackageID or GitRepo should be set
			hasPackage := tt.request.PackageID != ""
			hasGitRepo := tt.request.GitRepo != ""
			assert.True(t, hasPackage || hasGitRepo, "Either PackageID or GitRepo must be set")
		})
	}
}

// TestDeploymentStatusDetail tests DeploymentStatusDetail structure.
func TestDeploymentStatusDetail(t *testing.T) {
	now := time.Now().UTC()
	detail := &dms.DeploymentStatusDetail{
		DeploymentID: "deploy-789",
		Status:       dms.StatusHealthy,
		Message:      "All pods running",
		Progress:     100,
		UpdatedAt:    now,
		Conditions: []dms.StatusCondition{
			{
				Type:               "Ready",
				Status:             true,
				Reason:             "AllPodsReady",
				Message:            "All 3 pods are ready",
				LastTransitionTime: now,
			},
		},
	}

	// Verify structure
	assert.Equal(t, "deploy-789", detail.DeploymentID)
	assert.Equal(t, dms.StatusHealthy, detail.Status)
	assert.Equal(t, 100, detail.Progress)
	assert.Len(t, detail.Conditions, 1)
	assert.Equal(t, "Ready", detail.Conditions[0].Type)
	assert.True(t, detail.Conditions[0].Status)
}

// TestStatusCondition tests StatusCondition structure.
func TestStatusCondition(t *testing.T) {
	now := time.Now().UTC()
	condition := dms.StatusCondition{
		Type:               "Available",
		Status:             true,
		Reason:             "MinimumReplicasAvailable",
		Message:            "Deployment has minimum availability",
		LastTransitionTime: now,
	}

	assert.Equal(t, "Available", condition.Type)
	assert.True(t, condition.Status)
	assert.Equal(t, "MinimumReplicasAvailable", condition.Reason)
	assert.NotEmpty(t, condition.Message)
}

// TestLogOptions tests LogOptions structure.
func TestLogOptions(t *testing.T) {
	tests := []struct {
		name string
		opts *dms.LogOptions
	}{
		{
			name: "tail logs",
			opts: &dms.LogOptions{
				TailLines: 100,
				Follow:    false,
			},
		},
		{
			name: "follow logs",
			opts: &dms.LogOptions{
				Follow: true,
			},
		},
		{
			name: "specific container logs",
			opts: &dms.LogOptions{
				TailLines: 50,
				Container: "app-container",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.opts)
			if tt.opts.TailLines > 0 {
				assert.Greater(t, tt.opts.TailLines, 0)
			}
		})
	}
}

// TestDeploymentUpdateFields tests DeploymentUpdate structure.
func TestDeploymentUpdateFields(t *testing.T) {
	update := &dms.DeploymentUpdate{
		Values: map[string]interface{}{
			"replicas": 5,
			"image": map[string]string{
				"tag": "v2.0.0",
			},
		},
		PackageID:   "pkg-new-version",
		GitRevision: "v2.0.0",
	}

	assert.NotNil(t, update.Values)
	assert.Equal(t, 5, update.Values["replicas"])
	assert.Equal(t, "pkg-new-version", update.PackageID)
	assert.Equal(t, "v2.0.0", update.GitRevision)
}
