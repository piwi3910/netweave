package kubernetes

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResourceTypes retrieves all unique resource types from Kubernetes nodes.
// Resource types are derived from node labels such as instance-type or node-type.
func (a *Adapter) ListResourceTypes(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.ResourceType, error) {
	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// List all nodes to discover resource types
	nodes, err := a.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		a.logger.Error("failed to list nodes",
			zap.Error(err))
		return nil, fmt.Errorf("failed to list Kubernetes nodes: %w", err)
	}

	// Collect unique resource types
	typeMap := make(map[string]*adapter.ResourceType)

	for i := range nodes.Items {
		node := &nodes.Items[i]
		resourceTypeID := a.getNodeResourceTypeID(node)

		// Skip if we've already seen this type
		if _, exists := typeMap[resourceTypeID]; exists {
			continue
		}

		// Create resource type from node information
		resourceType := a.createResourceTypeFromNode(node, resourceTypeID)
		typeMap[resourceTypeID] = resourceType
	}

	// Convert map to slice
	types := make([]*adapter.ResourceType, 0, len(typeMap))
	for _, rt := range typeMap {
		types = append(types, rt)
	}

	// Apply pagination
	if filter != nil {
		types = adapter.ApplyPagination(types, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource types",
		zap.Int("count", len(types)))

	return types, nil
}

// GetResourceType retrieves a specific resource type by ID.
// It finds a node with the matching type and derives the type information.
func (a *Adapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	a.logger.Debug("GetResourceType called",
		zap.String("id", id))

	// List all nodes to find one with this resource type
	nodes, err := a.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		a.logger.Error("failed to list nodes",
			zap.Error(err))
		return nil, fmt.Errorf("failed to list Kubernetes nodes: %w", err)
	}

	// Find a node with the matching resource type
	for i := range nodes.Items {
		node := &nodes.Items[i]
		resourceTypeID := a.getNodeResourceTypeID(node)

		if resourceTypeID == id {
			resourceType := a.createResourceTypeFromNode(node, resourceTypeID)

			a.logger.Info("retrieved resource type",
				zap.String("resourceTypeID", resourceType.ResourceTypeID),
				zap.String("name", resourceType.Name))

			return resourceType, nil
		}
	}

	// Resource type not found
	return nil, fmt.Errorf("resource type %s not found", id)
}

// createResourceTypeFromNode creates a ResourceType from a Kubernetes node.
func (a *Adapter) createResourceTypeFromNode(
	node *corev1.Node,
	resourceTypeID string,
) *adapter.ResourceType {
	resourceType := &adapter.ResourceType{
		ResourceTypeID: resourceTypeID,
		Name:           resourceTypeID,
		ResourceClass:  "compute",
		ResourceKind:   "virtual", // Kubernetes nodes are typically virtual
		Extensions:     make(map[string]interface{}),
	}

	// Extract vendor and model from node labels or annotations
	if vendor, ok := node.Labels["node.kubernetes.io/vendor"]; ok {
		resourceType.Vendor = vendor
	} else {
		// Try to derive from cloud provider
		provider := node.Spec.ProviderID
		if len(provider) > 0 {
			// ProviderID format examples:
			// aws:///us-east-1a/i-abc123
			// gce://project/zone/instance
			// azure:///subscriptions/.../resourceGroups/.../providers/Microsoft.Compute/virtualMachines/...
			if len(provider) > 6 {
				prefix := provider[:6]
				switch prefix {
				case "aws://":
					resourceType.Vendor = "AWS"
				case "gce://":
					resourceType.Vendor = "GCP"
				case "azure:":
					resourceType.Vendor = "Azure"
				default:
					resourceType.Vendor = "Unknown"
				}
			}
		}
	}

	// Extract model from instance type label
	if instanceType, ok := node.Labels["node.kubernetes.io/instance-type"]; ok {
		resourceType.Model = instanceType
	} else if instanceType, ok := node.Labels["beta.kubernetes.io/instance-type"]; ok {
		resourceType.Model = instanceType
	}

	// Extract version from kubelet version
	resourceType.Version = node.Status.NodeInfo.KubeletVersion

	// Add description
	resourceType.Description = fmt.Sprintf("Kubernetes node type: %s", resourceTypeID)

	// Add Kubernetes-specific extensions
	resourceType.Extensions["kubernetes.io/architecture"] = node.Status.NodeInfo.Architecture
	resourceType.Extensions["kubernetes.io/os"] = node.Status.NodeInfo.OperatingSystem
	resourceType.Extensions["kubernetes.io/os-image"] = node.Status.NodeInfo.OSImage
	resourceType.Extensions["kubernetes.io/kernel-version"] = node.Status.NodeInfo.KernelVersion
	resourceType.Extensions["kubernetes.io/container-runtime"] = node.Status.NodeInfo.ContainerRuntimeVersion

	// Add capacity as a reference (from a sample node with this type)
	resourceType.Extensions["kubernetes.io/typical-capacity"] = map[string]interface{}{
		"cpu":              node.Status.Capacity.Cpu().String(),
		"memory":           node.Status.Capacity.Memory().String(),
		"ephemeralStorage": node.Status.Capacity.StorageEphemeral().String(),
		"pods":             node.Status.Capacity.Pods().String(),
	}

	return resourceType
}
