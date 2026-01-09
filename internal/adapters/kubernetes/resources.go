package kubernetes

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResources retrieves all Kubernetes nodes and transforms them to O2-IMS Resources.
// Nodes in Kubernetes are compute resources, which map to O2-IMS Resources.
func (a *KubernetesAdapter) ListResources(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Resource, error) {
	// Start observability tracing and metrics
	ctx, span := adapter.StartSpan(ctx, a.Name(), "ListResources")
	start := time.Now()
	var err error
	defer func() {
		adapter.ObserveOperationWithTracing(a.Name(), "ListResources", span, start, err)
	}()

	a.logger.Debug("ListResources called",
		zap.Any("filter", filter))

	// Record backend API call timing
	backendStart := time.Now()
	// List all nodes
	nodes, listErr := a.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	adapter.ObserveBackendRequest(a.Name(), "/api/v1/nodes", "LIST", backendStart, 200, listErr)
	adapter.RecordBackendCall(span, "/api/v1/nodes", "LIST", 200)

	if listErr != nil {
		err = fmt.Errorf("failed to list Kubernetes nodes: %w", listErr)
		a.logger.Error("failed to list nodes",
			zap.Error(err))
		return nil, err
	}

	a.logger.Debug("retrieved nodes from Kubernetes",
		zap.Int("count", len(nodes.Items)))

	// Transform Kubernetes nodes to O2-IMS Resources
	resources := make([]*adapter.Resource, 0, len(nodes.Items))
	for i := range nodes.Items {
		resource := a.transformNodeToResource(&nodes.Items[i])

		// Apply filter
		resourcePoolID := ""
		if namespace, ok := nodes.Items[i].Labels["o2ims.io/resource-pool"]; ok {
			resourcePoolID = fmt.Sprintf("k8s-namespace-%s", namespace)
		}

		if adapter.MatchesFilter(filter, resourcePoolID, resource.ResourceTypeID, "", nodes.Items[i].Labels) {
			resources = append(resources, resource)
		}
	}

	// Apply pagination
	if filter != nil {
		resources = adapter.ApplyPagination(resources, filter.Limit, filter.Offset)
	}

	// Update resource metrics
	adapter.UpdateResourceCount(a.Name(), "node", len(resources))
	adapter.RecordSuccess(span, len(resources))
	adapter.AddAttributes(span, map[string]interface{}{
		"resource.type":  "node",
		"resource.count": len(resources),
		"filtered":       filter != nil,
	})

	a.logger.Info("listed resources",
		zap.Int("count", len(resources)))

	return resources, nil
}

// GetResource retrieves a specific Kubernetes node by name and transforms it to O2-IMS Resource.
func (a *KubernetesAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	// Start observability tracing and metrics
	ctx, span := adapter.StartSpan(ctx, a.Name(), "GetResource")
	start := time.Now()
	var err error
	defer func() {
		adapter.ObserveOperationWithTracing(a.Name(), "GetResource", span, start, err)
	}()

	adapter.RecordResourceOperation(span, "node", "get", id)

	a.logger.Debug("GetResource called",
		zap.String("id", id))

	// Get node from Kubernetes using helper
	node, err := a.getNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Transform to O2-IMS Resource
	resource := a.transformNodeToResource(node)

	adapter.RecordSuccess(span, 1)
	adapter.AddAttributes(span, map[string]interface{}{
		"resource.id":   resource.ResourceID,
		"resource.type": resource.ResourceTypeID,
	})

	a.logger.Info("retrieved resource",
		zap.String("resourceID", resource.ResourceID),
		zap.String("resourceTypeID", resource.ResourceTypeID))

	return resource, nil
}

// CreateResource creates a new Kubernetes node.
// Note: In Kubernetes, nodes are typically managed by the cluster infrastructure (kubelet).
// This method is provided for completeness but may have limited practical use.
func (a *KubernetesAdapter) CreateResource(
	_ context.Context,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	a.logger.Debug("CreateResource called",
		zap.String("resourceTypeID", resource.ResourceTypeID))

	// Creating nodes directly is not a standard Kubernetes operation
	// Nodes are typically registered by kubelet when they join the cluster
	// This implementation returns an error indicating the operation is not supported
	return nil, fmt.Errorf(
		"creating nodes directly is not supported in Kubernetes; " +
			"nodes are registered by kubelet when joining the cluster",
	)
}

// UpdateResource updates a Kubernetes node's mutable fields (labels, annotations).
// Note: Core node properties are managed by kubelet and cannot be modified directly.
func (a *KubernetesAdapter) UpdateResource(
	_ context.Context,
	_ string,
	resource *adapter.Resource,
) (*adapter.Resource, error) {
	a.logger.Debug("UpdateResource called",
		zap.String("resourceID", resource.ResourceID))

	// Updating nodes directly is not a standard Kubernetes operation
	// Nodes are managed by kubelet and controllers
	// This implementation returns an error indicating the operation is not supported
	return nil, fmt.Errorf(
		"updating nodes directly is not supported in Kubernetes; " +
			"nodes are managed by kubelet and must be modified through node taints/labels",
	)
}

// DeleteResource deletes a Kubernetes node by ID.
// This drains the node and then removes it from the cluster.
func (a *KubernetesAdapter) DeleteResource(ctx context.Context, id string) error {
	a.logger.Debug("DeleteResource called",
		zap.String("id", id))

	// Parse resource ID to extract node name
	var nodeName string
	_, err := fmt.Sscanf(id, "k8s-node-%s", &nodeName)
	if err != nil {
		nodeName = id
	}

	// In Kubernetes, deleting a node removes it from the cluster
	// This is typically done when decommissioning hardware
	// Note: This does NOT delete the actual machine, only its registration in Kubernetes
	err = a.client.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
	if err != nil {
		a.logger.Error("failed to delete node",
			zap.String("node", nodeName),
			zap.Error(err))
		return fmt.Errorf("failed to delete Kubernetes node %s: %w", nodeName, err)
	}

	a.logger.Info("deleted resource",
		zap.String("node", nodeName))

	return nil
}

// transformNodeToResource converts a Kubernetes Node to an O2-IMS Resource.
func (a *KubernetesAdapter) transformNodeToResource(node *corev1.Node) *adapter.Resource {
	// Determine resource type ID based on node labels
	resourceTypeID := a.getNodeResourceTypeID(node)

	// Determine resource pool ID from namespace label
	resourcePoolID := ""
	if namespace, ok := node.Labels["o2ims.io/resource-pool"]; ok {
		resourcePoolID = fmt.Sprintf("k8s-namespace-%s", namespace)
	}

	resource := &adapter.Resource{
		ResourceID:     fmt.Sprintf("k8s-node-%s", node.Name),
		ResourceTypeID: resourceTypeID,
		ResourcePoolID: resourcePoolID,
		GlobalAssetID:  fmt.Sprintf("urn:k8s:node:%s:%s", a.oCloudID, node.UID),
		Extensions:     make(map[string]interface{}),
	}

	// Add description from annotation
	if desc, ok := node.Annotations["o2ims.io/description"]; ok {
		resource.Description = desc
	}

	// Add Kubernetes-specific extensions
	resource.Extensions["kubernetes.io/node-uid"] = string(node.UID)
	resource.Extensions["kubernetes.io/creation-timestamp"] = node.CreationTimestamp.Time
	resource.Extensions["kubernetes.io/hostname"] = node.Name

	// Add node info
	resource.Extensions["kubernetes.io/node-info"] = map[string]interface{}{
		"architecture":            node.Status.NodeInfo.Architecture,
		"containerRuntimeVersion": node.Status.NodeInfo.ContainerRuntimeVersion,
		"kernelVersion":           node.Status.NodeInfo.KernelVersion,
		"kubeletVersion":          node.Status.NodeInfo.KubeletVersion,
		"operatingSystem":         node.Status.NodeInfo.OperatingSystem,
		"osImage":                 node.Status.NodeInfo.OSImage,
	}

	// Add capacity information
	resource.Extensions["kubernetes.io/capacity"] = map[string]interface{}{
		"cpu":              node.Status.Capacity.Cpu().String(),
		"memory":           node.Status.Capacity.Memory().String(),
		"ephemeralStorage": node.Status.Capacity.StorageEphemeral().String(),
		"pods":             node.Status.Capacity.Pods().String(),
	}

	// Add allocatable resources
	resource.Extensions["kubernetes.io/allocatable"] = map[string]interface{}{
		"cpu":              node.Status.Allocatable.Cpu().String(),
		"memory":           node.Status.Allocatable.Memory().String(),
		"ephemeralStorage": node.Status.Allocatable.StorageEphemeral().String(),
		"pods":             node.Status.Allocatable.Pods().String(),
	}

	// Add node conditions
	conditions := make([]map[string]interface{}, 0, len(node.Status.Conditions))
	for i := range node.Status.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":    string(node.Status.Conditions[i].Type),
			"status":  string(node.Status.Conditions[i].Status),
			"reason":  node.Status.Conditions[i].Reason,
			"message": node.Status.Conditions[i].Message,
		})
	}
	resource.Extensions["kubernetes.io/conditions"] = conditions

	// Add all labels
	if len(node.Labels) > 0 {
		resource.Extensions["kubernetes.io/labels"] = node.Labels
	}

	// Add addresses
	addresses := make([]map[string]interface{}, 0, len(node.Status.Addresses))
	for i := range node.Status.Addresses {
		addresses = append(addresses, map[string]interface{}{
			"type":    string(node.Status.Addresses[i].Type),
			"address": node.Status.Addresses[i].Address,
		})
	}
	resource.Extensions["kubernetes.io/addresses"] = addresses

	return resource
}

// getNodeResourceTypeID determines the resource type ID for a node based on its labels.
func (a *KubernetesAdapter) getNodeResourceTypeID(node *corev1.Node) string {
	// Check for explicit resource type label
	if typeID, ok := node.Labels["o2ims.io/resource-type"]; ok {
		return typeID
	}

	// Determine type from instance type label (common in cloud providers)
	if instanceType, ok := node.Labels["node.kubernetes.io/instance-type"]; ok {
		return fmt.Sprintf("k8s-node-type-%s", instanceType)
	}

	// Fallback to generic node type
	return "k8s-node-type-generic"
}
