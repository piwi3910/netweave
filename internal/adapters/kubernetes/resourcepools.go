package kubernetes

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResourcePools retrieves all Kubernetes namespaces and transforms them to O2-IMS Resource Pools.
// Namespaces in Kubernetes are logical groupings of resources, which map naturally to O2-IMS Resource Pools.
func (a *KubernetesAdapter) ListResourcePools(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.ResourcePool, error) {
	a.logger.Debug("ListResourcePools called",
		zap.Any("filter", filter))

	// List all namespaces
	namespaces, err := a.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		a.logger.Error("failed to list namespaces",
			zap.Error(err))
		return nil, fmt.Errorf("failed to list Kubernetes namespaces: %w", err)
	}

	a.logger.Debug("retrieved namespaces from Kubernetes",
		zap.Int("count", len(namespaces.Items)))

	// Transform Kubernetes namespaces to O2-IMS Resource Pools
	pools := make([]*adapter.ResourcePool, 0, len(namespaces.Items))
	for i := range namespaces.Items {
		pool := a.transformNamespaceToResourcePool(&namespaces.Items[i])

		// Apply filter
		location := ""
		if val, ok := namespaces.Items[i].Labels["topology.kubernetes.io/zone"]; ok {
			location = val
		}

		if adapter.MatchesFilter(filter, pool.ResourcePoolID, "", location, namespaces.Items[i].Labels) {
			pools = append(pools, pool)
		}
	}

	// Apply pagination
	if filter != nil {
		pools = adapter.ApplyPagination(pools, filter.Limit, filter.Offset)
	}

	a.logger.Info("listed resource pools",
		zap.Int("count", len(pools)))

	return pools, nil
}

// GetResourcePool retrieves a specific Kubernetes namespace by name and transforms it to O2-IMS Resource Pool.
func (a *KubernetesAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	a.logger.Debug("GetResourcePool called",
		zap.String("id", id))

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

	// Transform to O2-IMS Resource Pool
	pool := a.transformNamespaceToResourcePool(namespace)

	a.logger.Info("retrieved resource pool",
		zap.String("resourcePoolID", pool.ResourcePoolID),
		zap.String("name", pool.Name))

	return pool, nil
}

// CreateResourcePool creates a new Kubernetes namespace from an O2-IMS Resource Pool.
func (a *KubernetesAdapter) CreateResourcePool(
	ctx context.Context,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	a.logger.Debug("CreateResourcePool called",
		zap.String("name", pool.Name))

	// Create namespace specification
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: pool.Name,
			Labels: map[string]string{
				"o2ims.io/resource-pool-id": pool.ResourcePoolID,
				"o2ims.io/managed":          "true",
			},
		},
	}

	// Add description as annotation if provided
	if pool.Description != "" {
		namespace.Annotations = map[string]string{
			"o2ims.io/description": pool.Description,
		}
	}

	// Add location label if provided
	if pool.Location != "" {
		namespace.Labels["topology.kubernetes.io/zone"] = pool.Location
	}

	// Create the namespace
	created, err := a.client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil {
		a.logger.Error("failed to create namespace",
			zap.String("name", pool.Name),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create Kubernetes namespace: %w", err)
	}

	// Transform created namespace back to O2-IMS Resource Pool
	result := a.transformNamespaceToResourcePool(created)

	a.logger.Info("created resource pool",
		zap.String("resourcePoolID", result.ResourcePoolID),
		zap.String("name", result.Name))

	return result, nil
}

// UpdateResourcePool updates an existing Kubernetes namespace.
// Note: Namespace names are immutable, so only labels and annotations can be updated.
func (a *KubernetesAdapter) UpdateResourcePool(
	ctx context.Context,
	id string,
	pool *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	a.logger.Debug("UpdateResourcePool called",
		zap.String("id", id),
		zap.String("name", pool.Name))

	// Parse resource pool ID to extract namespace name
	var namespaceName string
	_, err := fmt.Sscanf(id, "k8s-namespace-%s", &namespaceName)
	if err != nil {
		namespaceName = id
	}

	// Get existing namespace
	namespace, err := a.client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		a.logger.Error("failed to get namespace for update",
			zap.String("namespace", namespaceName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get Kubernetes namespace %s: %w", namespaceName, err)
	}

	// Update mutable fields
	if namespace.Labels == nil {
		namespace.Labels = make(map[string]string)
	}
	if namespace.Annotations == nil {
		namespace.Annotations = make(map[string]string)
	}

	// Update description
	if pool.Description != "" {
		namespace.Annotations["o2ims.io/description"] = pool.Description
	}

	// Update location
	if pool.Location != "" {
		namespace.Labels["topology.kubernetes.io/zone"] = pool.Location
	}

	// Update the namespace
	updated, err := a.client.CoreV1().Namespaces().Update(ctx, namespace, metav1.UpdateOptions{})
	if err != nil {
		a.logger.Error("failed to update namespace",
			zap.String("namespace", namespaceName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to update Kubernetes namespace: %w", err)
	}

	// Transform updated namespace back to O2-IMS Resource Pool
	result := a.transformNamespaceToResourcePool(updated)

	a.logger.Info("updated resource pool",
		zap.String("resourcePoolID", result.ResourcePoolID),
		zap.String("name", result.Name))

	return result, nil
}

// DeleteResourcePool deletes a Kubernetes namespace by ID.
func (a *KubernetesAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	a.logger.Debug("DeleteResourcePool called",
		zap.String("id", id))

	// Parse resource pool ID to extract namespace name
	var namespaceName string
	_, err := fmt.Sscanf(id, "k8s-namespace-%s", &namespaceName)
	if err != nil {
		namespaceName = id
	}

	// Delete the namespace
	err = a.client.CoreV1().Namespaces().Delete(ctx, namespaceName, metav1.DeleteOptions{})
	if err != nil {
		a.logger.Error("failed to delete namespace",
			zap.String("namespace", namespaceName),
			zap.Error(err))
		return fmt.Errorf("failed to delete Kubernetes namespace %s: %w", namespaceName, err)
	}

	a.logger.Info("deleted resource pool",
		zap.String("namespace", namespaceName))

	return nil
}

// transformNamespaceToResourcePool converts a Kubernetes Namespace to an O2-IMS Resource Pool.
func (a *KubernetesAdapter) transformNamespaceToResourcePool(ns *corev1.Namespace) *adapter.ResourcePool {
	pool := &adapter.ResourcePool{
		ResourcePoolID: fmt.Sprintf("k8s-namespace-%s", ns.Name),
		Name:           ns.Name,
		OCloudID:       a.oCloudID,
		Extensions:     make(map[string]interface{}),
	}

	// Add description from annotation
	if desc, ok := ns.Annotations["o2ims.io/description"]; ok {
		pool.Description = desc
	}

	// Add location from zone label
	if zone, ok := ns.Labels["topology.kubernetes.io/zone"]; ok {
		pool.Location = zone
	}

	// Add Kubernetes-specific extensions
	pool.Extensions["kubernetes.io/namespace-uid"] = string(ns.UID)
	pool.Extensions["kubernetes.io/creation-timestamp"] = ns.CreationTimestamp.Time
	pool.Extensions["kubernetes.io/phase"] = string(ns.Status.Phase)

	// Add all labels as extensions
	if len(ns.Labels) > 0 {
		pool.Extensions["kubernetes.io/labels"] = ns.Labels
	}

	return pool
}
