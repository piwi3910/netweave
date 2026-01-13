package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// setupTestAdapter creates a test adapter with a fake Kubernetes client.
func setupTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	return &Adapter{
		client: fake.NewClientset(),
		logger: zap.NewNop(),
	}
}

func TestGetNamespaceByID(t *testing.T) {
	testCases := map[string]struct {
		id           string
		existingNS   *corev1.Namespace
		expectedName string
		wantErr      bool
	}{
		"formatted ID with k8s-namespace prefix": {
			id:           "k8s-namespace-default",
			existingNS:   &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			expectedName: "default",
		},
		"direct namespace name": {
			id:           "kube-system",
			existingNS:   &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
			expectedName: "kube-system",
		},
		"non-existent namespace": {
			id:      "nonexistent",
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			adapter := setupTestAdapter(t)
			ctx := context.Background()

			if tc.existingNS != nil {
				_, createErr := adapter.client.CoreV1().Namespaces().Create(ctx, tc.existingNS, metav1.CreateOptions{})
				require.NoError(t, createErr)
			}

			result, err := adapter.getNamespaceByID(ctx, tc.id)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tc.expectedName, result.Name)
			}
		})
	}
}

func TestGetNodeByID(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		existingNode *corev1.Node
		expectedName string
		wantErr      bool
	}{
		{
			name: "formatted ID with k8s-node prefix",
			id:   "k8s-node-worker1",
			existingNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker1"},
			},
			expectedName: "worker1",
		},
		{
			name: "direct node name",
			id:   "master-1",
			existingNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "master-1"},
			},
			expectedName: "master-1",
		},
		{
			name:    "non-existent node",
			id:      "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			adapter := setupTestAdapter(t)

			if tt.existingNode != nil {
				_, err := adapter.client.CoreV1().Nodes().Create(
					context.Background(),
					tt.existingNode,
					metav1.CreateOptions{},
				)
				require.NoError(t, err)
			}

			node, err := adapter.getNodeByID(context.Background(), tt.id)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, node)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, node)
			assert.Equal(t, tt.expectedName, node.Name)
		})
	}
}
