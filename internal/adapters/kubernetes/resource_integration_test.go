//go:build integration

package kubernetes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
)

// TestKubernetesAdapter_ListResources_Integration tests the ListResources method
// with a fake Kubernetes client to verify resource discovery.
func TestKubernetesAdapter_ListResources_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup fake Kubernetes client with sample nodes
	client := fake.NewClientset(
		// Node 1: Ready node
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				UID:  "node-1-uid",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.large",
					"topology.kubernetes.io/zone":      "us-east-1a",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3800m"),
					corev1.ResourceMemory: resource.MustParse("15Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		// Node 2: Ready node with different type
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				UID:  "node-2-uid",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.xlarge",
					"topology.kubernetes.io/zone":      "us-east-1b",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("32Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		// Node 3: Not ready node
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-3",
				UID:  "node-3-uid",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)
	require.NotNil(t, adp)

	t.Run("list_all_resources", func(t *testing.T) {
		resources, err := adp.ListResources(context.Background(), nil)
		require.NoError(t, err)
		assert.NotEmpty(t, resources, "should discover resources from nodes")

		// Verify node properties
		for _, res := range resources {
			assert.NotEmpty(t, res.ResourceID, "resource should have ID")
			assert.NotEmpty(t, res.ResourceTypeID, "resource should have type ID")
			assert.NotNil(t, res.Extensions, "resource should have extensions")

			// Verify Kubernetes-specific extensions
			assert.Contains(t, res.Extensions, "kubernetes.io/hostname", "should include node hostname")
		}
	})

	t.Run("list_with_pagination", func(t *testing.T) {
		filter := &adapter.Filter{
			Limit:  2,
			Offset: 0,
		}

		resources, err := adp.ListResources(context.Background(), filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resources), 2, "should respect pagination limit")
	})

	t.Run("get_specific_resource", func(t *testing.T) {
		// First list to get valid IDs
		resources, err := adp.ListResources(context.Background(), nil)
		require.NoError(t, err)
		require.NotEmpty(t, resources)

		// Get first resource by ID
		resourceID := resources[0].ResourceID
		retrieved, err := adp.GetResource(context.Background(), resourceID)
		require.NoError(t, err)
		assert.Equal(t, resourceID, retrieved.ResourceID)
		assert.Equal(t, resources[0].ResourceTypeID, retrieved.ResourceTypeID)
	})

	t.Run("get_nonexistent_resource", func(t *testing.T) {
		_, err := adp.GetResource(context.Background(), "nonexistent-resource")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestKubernetesAdapter_ResourceFields_Integration verifies that resource
// fields are properly populated with Kubernetes-specific information.
func TestKubernetesAdapter_ResourceFields_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := fake.NewClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				UID:  "test-node-uid",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "t3.medium",
					"topology.kubernetes.io/zone":      "us-west-2a",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)

	resources, err := adp.ListResources(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, resources)

	// Verify first resource has all expected fields
	res := resources[0]
	assert.NotEmpty(t, res.ResourceID, "ResourceID is required")
	assert.NotEmpty(t, res.ResourceTypeID, "ResourceTypeID is required")
	assert.NotNil(t, res.Extensions, "Extensions should not be nil")

	// Verify extensions contain Kubernetes-specific data
	assert.NotEmpty(t, res.Extensions, "Extensions should contain metadata")
	assert.Contains(t, res.Extensions, "kubernetes.io/hostname", "should include node hostname")
}

// TestKubernetesAdapter_ResourceConsistency_Integration verifies that
// resources remain consistent across multiple calls.
func TestKubernetesAdapter_ResourceConsistency_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := fake.NewClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				UID:  "node-1-uid",
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)

	// Call ListResources multiple times
	resources1, err1 := adp.ListResources(context.Background(), nil)
	require.NoError(t, err1)

	resources2, err2 := adp.ListResources(context.Background(), nil)
	require.NoError(t, err2)

	// Verify consistency
	assert.Equal(t, len(resources1), len(resources2), "should return same number of resources")

	if len(resources1) > 0 && len(resources2) > 0 {
		assert.Equal(t, resources1[0].ResourceID, resources2[0].ResourceID,
			"resource IDs should be consistent")
		assert.Equal(t, resources1[0].ResourceTypeID, resources2[0].ResourceTypeID,
			"resource type IDs should be consistent")
	}
}

// TestKubernetesAdapter_ResourceFiltering_Integration tests resource filtering capabilities.
func TestKubernetesAdapter_ResourceFiltering_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := fake.NewClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-type-a",
				UID:  "node-a-uid",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.large",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-type-b",
				UID:  "node-b-uid",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.xlarge",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)

	t.Run("filter_by_labels", func(t *testing.T) {
		filter := &adapter.Filter{
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": "m5.large",
			},
		}

		resources, err := adp.ListResources(context.Background(), filter)
		require.NoError(t, err)

		// Verify filtered results
		for _, res := range resources {
			if ext, ok := res.Extensions["labels"].(map[string]string); ok {
				assert.Equal(t, "m5.large", ext["node.kubernetes.io/instance-type"])
			}
		}
	})
}
