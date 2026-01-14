//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// K8sResourceHelper provides utilities for creating and managing Kubernetes test resources.
type K8sResourceHelper struct {
	client kubernetes.Interface
}

// NewK8sResourceHelper creates a new Kubernetes resource helper.
func NewK8sResourceHelper(client kubernetes.Interface) *K8sResourceHelper {
	return &K8sResourceHelper{
		client: client,
	}
}

// CreateTestNamespace creates a test namespace for resource isolation.
func (h *K8sResourceHelper) CreateTestNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"test":    "e2e",
				"purpose": "subscription-testing",
			},
		},
	}

	created, err := h.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace %s: %w", name, err)
	}

	return created, nil
}

// DeleteNamespace deletes a namespace and waits for termination.
// Ignores NotFound errors (namespace already deleted).
func (h *K8sResourceHelper) DeleteNamespace(ctx context.Context, name string) error {
	err := h.client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace %s: %w", name, err)
	}

	// Wait for namespace to be deleted (up to 60 seconds)
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(pollCtx context.Context) (bool, error) {
		_, err := h.client.CoreV1().Namespaces().Get(pollCtx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			// Namespace has been successfully deleted
			return true, nil
		}
		if err != nil {
			// Other errors (network, permission, etc.) should be returned
			return false, err
		}
		// Namespace still exists
		return false, nil
	})
}

// CreateTestPod creates a simple test pod in the specified namespace.
func (h *K8sResourceHelper) CreateTestPod(ctx context.Context, namespace, name string, labels map[string]string) (*corev1.Pod, error) {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["test"] = "e2e"
	labels["app"] = name

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "pause",
					Image: "registry.k8s.io/pause:3.9",
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	created, err := h.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod %s/%s: %w", namespace, name, err)
	}

	// Wait for pod to be scheduled (Running or Pending)
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(pollCtx context.Context) (bool, error) {
		p, err := h.client.CoreV1().Pods(namespace).Get(pollCtx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending, nil
	})

	if err != nil {
		return nil, fmt.Errorf("pod %s/%s did not become ready: %w", namespace, name, err)
	}

	return created, nil
}

// UpdatePodLabels updates labels on an existing pod.
func (h *K8sResourceHelper) UpdatePodLabels(ctx context.Context, namespace, name string, labels map[string]string) (*corev1.Pod, error) {
	pod, err := h.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	for k, v := range labels {
		pod.Labels[k] = v
	}

	updated, err := h.client.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update pod %s/%s: %w", namespace, name, err)
	}

	return updated, nil
}

// DeletePod deletes a pod and waits for termination.
// Ignores NotFound errors (pod already deleted).
func (h *K8sResourceHelper) DeletePod(ctx context.Context, namespace, name string) error {
	err := h.client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod %s/%s: %w", namespace, name, err)
	}

	// Wait for pod to be deleted (up to 30 seconds)
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(pollCtx context.Context) (bool, error) {
		_, err := h.client.CoreV1().Pods(namespace).Get(pollCtx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			// Pod has been successfully deleted
			return true, nil
		}
		if err != nil {
			// Other errors (network, permission, etc.) should be returned
			return false, err
		}
		// Pod still exists
		return false, nil
	})
}

// WaitForEvent waits for a Kubernetes event to occur.
// This is a simple polling mechanism that checks for events matching the given criteria.
func (h *K8sResourceHelper) WaitForEvent(ctx context.Context, namespace, reason string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, timeout, true, func(pollCtx context.Context) (bool, error) {
		events, err := h.client.CoreV1().Events(namespace).List(pollCtx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		for _, event := range events.Items {
			if event.Reason == reason {
				return true, nil
			}
		}

		return false, nil
	})
}

// ListPods lists all pods in the specified namespace.
func (h *K8sResourceHelper) ListPods(ctx context.Context, namespace string) (*corev1.PodList, error) {
	pods, err := h.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}
	return pods, nil
}

// GetPod retrieves a specific pod.
func (h *K8sResourceHelper) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	pod, err := h.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}
	return pod, nil
}

// CleanupTestResources deletes all test resources in the given namespace.
// Ignores NotFound errors (resources already deleted).
func (h *K8sResourceHelper) CleanupTestResources(ctx context.Context, namespace string) error {
	// Delete all pods with test label
	err := h.client.CoreV1().Pods(namespace).DeleteCollection(
		ctx,
		metav1.DeleteOptions{},
		metav1.ListOptions{
			LabelSelector: "test=e2e",
		},
	)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to cleanup test pods in %s: %w", namespace, err)
	}

	return nil
}
