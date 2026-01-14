//go:build integration

package kubernetes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
)

// TestKubernetesAdapter_ListResourceTypes_Integration tests the ListResourceTypes method
// with a fake Kubernetes client to verify resource type discovery.
func TestKubernetesAdapter_ListResourceTypes_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup fake Kubernetes client with sample resources
	client := fake.NewSimpleClientset(
		// Node 1: Standard compute node
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.large",
					"topology.kubernetes.io/zone":       "us-east-1a",
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
			},
		},
		// Node 2: Different instance type
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "m5.xlarge",
					"topology.kubernetes.io/zone":       "us-east-1b",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("32Gi"),
				},
			},
		},
		// Storage Class 1: Fast SSD
		&storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fast-ssd",
			},
			Provisioner: "kubernetes.io/gce-pd",
			Parameters: map[string]string{
				"type": "pd-ssd",
			},
		},
		// Storage Class 2: Standard HDD
		&storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "standard-hdd",
			},
			Provisioner: "kubernetes.io/aws-ebs",
			Parameters: map[string]string{
				"type": "gp2",
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)
	require.NotNil(t, adp)

	t.Run("list_all_types", func(t *testing.T) {
		types, err := adp.ListResourceTypes(context.Background(), nil)
		require.NoError(t, err)
		assert.NotEmpty(t, types, "should discover resource types from nodes and storage classes")

		// Verify we got both compute and storage types
		var computeTypes, storageTypes int
		for _, rt := range types {
			// Verify required fields
			assert.NotEmpty(t, rt.ResourceTypeID)
			assert.NotEmpty(t, rt.Name)
			assert.NotNil(t, rt.Extensions)

			// Count resource classes
			if rt.ResourceClass == "compute" {
				computeTypes++
			} else if rt.ResourceClass == "storage" {
				storageTypes++
			}
		}

		assert.Greater(t, computeTypes, 0, "should have at least one compute type")
		assert.Greater(t, storageTypes, 0, "should have at least one storage type")
	})

	t.Run("list_with_pagination", func(t *testing.T) {
		filter := &adapter.Filter{
			Limit:  2,
			Offset: 0,
		}

		types, err := adp.ListResourceTypes(context.Background(), filter)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(types), 2, "should respect pagination limit")
	})

	t.Run("get_specific_type", func(t *testing.T) {
		// First list to get valid IDs
		types, err := adp.ListResourceTypes(context.Background(), nil)
		require.NoError(t, err)
		require.NotEmpty(t, types)

		// Get first type by ID
		typeID := types[0].ResourceTypeID
		retrieved, err := adp.GetResourceType(context.Background(), typeID)
		require.NoError(t, err)
		assert.Equal(t, typeID, retrieved.ResourceTypeID)
		assert.Equal(t, types[0].Name, retrieved.Name)
	})

	t.Run("get_nonexistent_type", func(t *testing.T) {
		_, err := adp.GetResourceType(context.Background(), "nonexistent-type")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestKubernetesAdapter_ResourceTypeFields_Integration verifies that resource type
// fields are properly populated with Kubernetes-specific information.
func TestKubernetesAdapter_ResourceTypeFields_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Labels: map[string]string{
					"node.kubernetes.io/instance-type": "t3.medium",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)

	types, err := adp.ListResourceTypes(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, types)

	// Verify first type has all expected fields
	rt := types[0]
	assert.NotEmpty(t, rt.ResourceTypeID, "ResourceTypeID is required")
	assert.NotEmpty(t, rt.Name, "Name is required")
	assert.NotEmpty(t, rt.ResourceClass, "ResourceClass should be set")
	assert.Contains(t, []string{"compute", "storage", "network"}, rt.ResourceClass)
	assert.NotNil(t, rt.Extensions, "Extensions should not be nil")

	// Verify extensions contain Kubernetes-specific data
	assert.NotEmpty(t, rt.Extensions, "Extensions should contain metadata")
}

// TestKubernetesAdapter_ResourceTypeConsistency_Integration verifies that
// resource types remain consistent across multiple calls.
func TestKubernetesAdapter_ResourceTypeConsistency_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
	)

	logger := zaptest.NewLogger(t)
	adp := kubernetes.NewForTesting(client, logger)

	// Call ListResourceTypes multiple times
	types1, err1 := adp.ListResourceTypes(context.Background(), nil)
	require.NoError(t, err1)

	types2, err2 := adp.ListResourceTypes(context.Background(), nil)
	require.NoError(t, err2)

	// Verify consistency
	assert.Equal(t, len(types1), len(types2), "should return same number of types")

	if len(types1) > 0 && len(types2) > 0 {
		assert.Equal(t, types1[0].ResourceTypeID, types2[0].ResourceTypeID,
			"resource type IDs should be consistent")
	}
}
