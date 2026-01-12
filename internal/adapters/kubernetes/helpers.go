package kubernetes

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getNamespaceByID retrieves a Kubernetes namespace by ID or name.
// It handles both formatted IDs (k8s-namespace-NAME) and direct namespace names.
// This helper function is used by both GetResourcePool and related methods to avoid code duplication.
func (a *Adapter) getNamespaceByID(ctx context.Context, id string) (*corev1.Namespace, error) {
	// Parse resource pool ID to extract namespace name
	var namespaceName string
	_, err := fmt.Sscanf(id, "k8s-namespace-%s", &namespaceName)
	if err != nil {
		// Try direct namespace name
		namespaceName = id
	}

	// Get namespace from Kubernetes
	namespace, err := a.client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		a.logger.Error("failed to get namespace",
			zap.String("namespace", namespaceName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get Kubernetes namespace %s: %w", namespaceName, err)
	}

	return namespace, nil
}

// getNodeByID retrieves a Kubernetes node by ID or name.
// It handles both formatted IDs (k8s-node-NAME) and direct node names.
// This helper function is used by GetResource and related methods to avoid code duplication.
func (a *Adapter) getNodeByID(ctx context.Context, id string) (*corev1.Node, error) {
	// Parse resource ID to extract node name
	var nodeName string
	_, err := fmt.Sscanf(id, "k8s-node-%s", &nodeName)
	if err != nil {
		// Try direct node name
		nodeName = id
	}

	// Get node from Kubernetes
	node, err := a.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		a.logger.Error("failed to get node",
			zap.String("node", nodeName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get Kubernetes node %s: %w", nodeName, err)
	}

	return node, nil
}
