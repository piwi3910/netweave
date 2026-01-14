//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// K8sResourceHelper provides utilities for creating and managing Kubernetes test resources.
type K8sResourceHelper struct {
	client    kubernetes.Interface
	namespace string
	ctx       context.Context
}

// NewK8sResourceHelper creates a new Kubernetes resource helper.
func NewK8sResourceHelper(client kubernetes.Interface, namespace string, ctx context.Context) *K8sResourceHelper {
	return &K8sResourceHelper{
		client:    client,
		namespace: namespace,
		ctx:       ctx,
	}
}

// CreateTestNamespace creates a test namespace for resource isolation.
func (h *K8sResourceHelper) CreateTestNamespace(name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"test":    "e2e",
				"purpose": "subscription-testing",
			},
		},
	}

	created, err := h.client.CoreV1().Namespaces().Create(h.ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace %s: %w", name, err)
	}

	return created, nil
}

// DeleteNamespace deletes a namespace and waits for termination.
func (h *K8sResourceHelper) DeleteNamespace(name string) error {
	err := h.client.CoreV1().Namespaces().Delete(h.ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w", name, err)
	}

	// Wait for namespace to be deleted (up to 60 seconds)
	return wait.PollUntilContextTimeout(h.ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := h.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			// Namespace is gone
			return true, nil
		}
		return false, nil
	})
}

// CreateTestPod creates a simple test pod in the specified namespace.
func (h *K8sResourceHelper) CreateTestPod(namespace, name string, labels map[string]string) (*corev1.Pod, error) {
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

	created, err := h.client.CoreV1().Pods(namespace).Create(h.ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod %s/%s: %w", namespace, name, err)
	}

	// Wait for pod to be scheduled (Running or Pending)
	err = wait.PollUntilContextTimeout(h.ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		p, err := h.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
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
func (h *K8sResourceHelper) UpdatePodLabels(namespace, name string, labels map[string]string) (*corev1.Pod, error) {
	pod, err := h.client.CoreV1().Pods(namespace).Get(h.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	for k, v := range labels {
		pod.Labels[k] = v
	}

	updated, err := h.client.CoreV1().Pods(namespace).Update(h.ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update pod %s/%s: %w", namespace, name, err)
	}

	return updated, nil
}

// DeletePod deletes a pod and waits for termination.
func (h *K8sResourceHelper) DeletePod(namespace, name string) error {
	err := h.client.CoreV1().Pods(namespace).Delete(h.ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s/%s: %w", namespace, name, err)
	}

	// Wait for pod to be deleted (up to 30 seconds)
	return wait.PollUntilContextTimeout(h.ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := h.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			// Pod is gone
			return true, nil
		}
		return false, nil
	})
}

// WaitForEvent waits for a Kubernetes event to occur.
// This is a simple polling mechanism that checks for events matching the given criteria.
func (h *K8sResourceHelper) WaitForEvent(namespace, reason string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(h.ctx, 1*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		events, err := h.client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
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
func (h *K8sResourceHelper) ListPods(namespace string) (*corev1.PodList, error) {
	pods, err := h.client.CoreV1().Pods(namespace).List(h.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}
	return pods, nil
}

// GetPod retrieves a specific pod.
func (h *K8sResourceHelper) GetPod(namespace, name string) (*corev1.Pod, error) {
	pod, err := h.client.CoreV1().Pods(namespace).Get(h.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}
	return pod, nil
}

// CleanupTestResources deletes all test resources in the given namespace.
func (h *K8sResourceHelper) CleanupTestResources(namespace string) error {
	// Delete all pods with test label
	err := h.client.CoreV1().Pods(namespace).DeleteCollection(
		h.ctx,
		metav1.DeleteOptions{},
		metav1.ListOptions{
			LabelSelector: "test=e2e",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup test pods in %s: %w", namespace, err)
	}

	return nil
}
