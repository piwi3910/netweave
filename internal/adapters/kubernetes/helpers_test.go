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

func TestGetNamespaceByID(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		existingNS   *corev1.Namespace
		expectedName string
		wantErr      bool
	}{
		{
			name: "formatted ID with k8s-namespace prefix",
			id:   "k8s-namespace-default",
			existingNS: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			expectedName: "default",
			wantErr:      false,
		},
		{
			name: "direct namespace name",
			id:   "kube-system",
			existingNS: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
				},
			},
			expectedName: "kube-system",
			wantErr:      false,
		},
		{
			name:    "non-existent namespace",
			id:      "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset()
			logger := zap.NewNop()

			// Create namespace if provided
			if tt.existingNS != nil {
				_, err := client.CoreV1().Namespaces().Create(
					context.Background(),
					tt.existingNS,
					metav1.CreateOptions{},
				)
				require.NoError(t, err)
			}

			adapter := &Adapter{
				client: client,
				logger: logger,
			}

			// Execute
			ns, err := adapter.getNamespaceByID(context.Background(), tt.id)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, ns)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, ns)
				assert.Equal(t, tt.expectedName, ns.Name)
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
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker1",
				},
			},
			expectedName: "worker1",
			wantErr:      false,
		},
		{
			name: "direct node name",
			id:   "master-1",
			existingNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "master-1",
				},
			},
			expectedName: "master-1",
			wantErr:      false,
		},
		{
			name:    "non-existent node",
			id:      "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset()
			logger := zap.NewNop()

			// Create node if provided
			if tt.existingNode != nil {
				_, err := client.CoreV1().Nodes().Create(
					context.Background(),
					tt.existingNode,
					metav1.CreateOptions{},
				)
				require.NoError(t, err)
			}

			adapter := &Adapter{
				client: client,
				logger: logger,
			}

			// Execute
			node, err := adapter.getNodeByID(context.Background(), tt.id)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, node)
				assert.Equal(t, tt.expectedName, node.Name)
			}
		})
	}
}
