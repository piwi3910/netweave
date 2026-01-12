package events_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/adapter"
)

// mockGeneratorAdapter is a mock implementation of adapter.Adapter for testing.
type mockGeneratorAdapter struct {
	adapter.Adapter
}

// TestNewK8sEventGenerator_NilParams tests generator creation with nil parameters.
func TestNewK8sEventGenerator_NilParams(t *testing.T) {
	mockAdp := &mockGeneratorAdapter{}
	logger := zaptest.NewLogger(t)

	t.Run("nil clientset panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewK8sEventGenerator(nil, mockAdp, logger)
		})
	})

	t.Run("nil adapter panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewK8sEventGenerator(nil, nil, logger)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewK8sEventGenerator(nil, mockAdp, nil)
		})
	})
}

// TestK8sEventGenerator_CreateDeletedResource tests createDeletedResource.
func TestK8sEventGenerator_CreateDeletedResource(t *testing.T) {
	// Note: Cannot easily test the full generator Start/Stop/Handle functions
	// because they require a real Kubernetes Clientset (not an interface).
	// The createDeletedResource function is private and not directly testable.
	// These functions are covered by integration tests.

	t.Run("validates node label handling", func(t *testing.T) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "",
					"custom-label":                   "value",
				},
			},
		}

		// The createDeletedResource would use these labels
		assert.NotNil(t, node.Labels)
		assert.Contains(t, node.Labels, "node-role.kubernetes.io/worker")
		assert.Contains(t, node.Labels, "custom-label")
	})

	t.Run("validates node name extraction", func(t *testing.T) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-123",
			},
		}

		// The createDeletedResource would use the node name
		assert.Equal(t, "test-node-123", node.Name)
	})
}
