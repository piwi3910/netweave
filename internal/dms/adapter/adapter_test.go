package adapter_test

import (
	"errors"
	"testing"

	"github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/stretchr/testify/assert"
)

// TestErrorDefinitions tests the predefined error constants.
func TestErrorDefinitions(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "deployment not found error",
			err:      adapter.ErrDeploymentNotFound,
			expected: "deployment not found",
		},
		{
			name:     "package not found error",
			err:      adapter.ErrPackageNotFound,
			expected: "deployment package not found",
		},
		{
			name:     "operation not supported error",
			err:      adapter.ErrOperationNotSupported,
			expected: "operation not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.expected)
		})
	}
}

// TestErrorIsComparison tests error comparison with errors.Is.
func TestErrorIsComparison(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		target   error
		expected bool
	}{
		{
			name:     "matching deployment not found",
			err:      adapter.ErrDeploymentNotFound,
			target:   adapter.ErrDeploymentNotFound,
			expected: true,
		},
		{
			name:     "non-matching errors",
			err:      adapter.ErrDeploymentNotFound,
			target:   adapter.ErrPackageNotFound,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errors.Is(tt.err, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCapabilityConstants tests Capability type and constants.
func TestCapabilityConstants(t *testing.T) {
	tests := []struct {
		name       string
		capability adapter.Capability
		expected   string
	}{
		{
			name:       "package management capability",
			capability: adapter.CapabilityPackageManagement,
			expected:   "package-management",
		},
		{
			name:       "deployment lifecycle capability",
			capability: adapter.CapabilityDeploymentLifecycle,
			expected:   "deployment-lifecycle",
		},
		{
			name:       "rollback capability",
			capability: adapter.CapabilityRollback,
			expected:   "rollback",
		},
		{
			name:       "scaling capability",
			capability: adapter.CapabilityScaling,
			expected:   "scaling",
		},
		{
			name:       "gitops capability",
			capability: adapter.CapabilityGitOps,
			expected:   "gitops",
		},
		{
			name:       "health checks capability",
			capability: adapter.CapabilityHealthChecks,
			expected:   "health-checks",
		},
		{
			name:       "metrics capability",
			capability: adapter.CapabilityMetrics,
			expected:   "metrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.capability))
		})
	}
}

// TestDeploymentStatusConstants tests DeploymentStatus type and constants.
func TestDeploymentStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		status   adapter.DeploymentStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   adapter.DeploymentStatusPending,
			expected: "pending",
		},
		{
			name:     "deploying status",
			status:   adapter.DeploymentStatusDeploying,
			expected: "deploying",
		},
		{
			name:     "deployed status",
			status:   adapter.DeploymentStatusDeployed,
			expected: "deployed",
		},
		{
			name:     "failed status",
			status:   adapter.DeploymentStatusFailed,
			expected: "failed",
		},
		{
			name:     "rolling back status",
			status:   adapter.DeploymentStatusRollingBack,
			expected: "rolling-back",
		},
		{
			name:     "deleting status",
			status:   adapter.DeploymentStatusDeleting,
			expected: "deleting",
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
	filter := &adapter.Filter{}

	assert.Empty(t, filter.Namespace)
	assert.Empty(t, filter.Status)
	assert.Nil(t, filter.Labels)
	assert.Nil(t, filter.Extensions)
	assert.Zero(t, filter.Limit)
	assert.Zero(t, filter.Offset)
}

// TestFilterWithValues tests Filter with populated values.
func TestFilterWithValues(t *testing.T) {
	filter := &adapter.Filter{
		Namespace: "production",
		Status:    adapter.DeploymentStatusDeployed,
		Labels: map[string]string{
			"app":     "frontend",
			"version": "v1.0.0",
		},
		Extensions: map[string]interface{}{
			"cluster": "prod-cluster-1",
		},
		Limit:  20,
		Offset: 10,
	}

	assert.Equal(t, "production", filter.Namespace)
	assert.Equal(t, adapter.DeploymentStatusDeployed, filter.Status)
	assert.Equal(t, 2, len(filter.Labels))
	assert.Equal(t, "frontend", filter.Labels["app"])
	assert.Equal(t, "v1.0.0", filter.Labels["version"])
	assert.Equal(t, 1, len(filter.Extensions))
	assert.Equal(t, "prod-cluster-1", filter.Extensions["cluster"])
	assert.Equal(t, 20, filter.Limit)
	assert.Equal(t, 10, filter.Offset)
}

// TestFilterPagination tests pagination fields.
func TestFilterPagination(t *testing.T) {
	tests := []struct {
		name   string
		filter *adapter.Filter
		limit  int
		offset int
	}{
		{
			name: "first page",
			filter: &adapter.Filter{
				Limit:  10,
				Offset: 0,
			},
			limit:  10,
			offset: 0,
		},
		{
			name: "second page",
			filter: &adapter.Filter{
				Limit:  10,
				Offset: 10,
			},
			limit:  10,
			offset: 10,
		},
		{
			name: "large limit",
			filter: &adapter.Filter{
				Limit:  100,
				Offset: 0,
			},
			limit:  100,
			offset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.limit, tt.filter.Limit)
			assert.Equal(t, tt.offset, tt.filter.Offset)
		})
	}
}

// TestFilterLabelMatching tests label-based filtering.
func TestFilterLabelMatching(t *testing.T) {
	filter := &adapter.Filter{
		Labels: map[string]string{
			"env":     "production",
			"team":    "platform",
			"version": "v2",
		},
	}

	// Verify all labels are present
	assert.Equal(t, 3, len(filter.Labels))
	assert.Equal(t, "production", filter.Labels["env"])
	assert.Equal(t, "platform", filter.Labels["team"])
	assert.Equal(t, "v2", filter.Labels["version"])
}

// TestFilterStatusFiltering tests status-based filtering.
func TestFilterStatusFiltering(t *testing.T) {
	tests := []struct {
		name   string
		status adapter.DeploymentStatus
	}{
		{
			name:   "filter pending deployments",
			status: adapter.DeploymentStatusPending,
		},
		{
			name:   "filter deployed deployments",
			status: adapter.DeploymentStatusDeployed,
		},
		{
			name:   "filter failed deployments",
			status: adapter.DeploymentStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &adapter.Filter{
				Status: tt.status,
			}
			assert.Equal(t, tt.status, filter.Status)
		})
	}
}

// TestFilterNamespaceFiltering tests namespace-based filtering.
func TestFilterNamespaceFiltering(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
	}{
		{
			name:      "default namespace",
			namespace: "default",
		},
		{
			name:      "production namespace",
			namespace: "production",
		},
		{
			name:      "kube-system namespace",
			namespace: "kube-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &adapter.Filter{
				Namespace: tt.namespace,
			}
			assert.Equal(t, tt.namespace, filter.Namespace)
		})
	}
}

// TestFilterExtensions tests vendor-specific extensions.
func TestFilterExtensions(t *testing.T) {
	filter := &adapter.Filter{
		Extensions: map[string]interface{}{
			"cluster":      "prod-cluster-1",
			"region":       "us-east-1",
			"cloud":        "aws",
			"customField":  42,
			"enabledFlag":  true,
			"complexField": map[string]string{"key": "value"},
		},
	}

	assert.Equal(t, 6, len(filter.Extensions))
	assert.Equal(t, "prod-cluster-1", filter.Extensions["cluster"])
	assert.Equal(t, "us-east-1", filter.Extensions["region"])
	assert.Equal(t, "aws", filter.Extensions["cloud"])
	assert.Equal(t, 42, filter.Extensions["customField"])
	assert.Equal(t, true, filter.Extensions["enabledFlag"])

	complexField, ok := filter.Extensions["complexField"].(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, "value", complexField["key"])
}
