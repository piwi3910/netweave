// Package service provides Kubernetes service discovery and port-forwarding
// for connecting to in-cluster services (Vault, Keycloak, Gateway).
package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// Connector discovers and connects to Kubernetes services via port-forwarding.
type Connector struct {
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
	namespace  string
	forwards   []*portforward.PortForwarder
}

// NewConnector creates a Connector from the given kubeconfig and namespace.
func NewConnector(kubeconfig, namespace string) (*Connector, error) {
	restConfig, err := buildConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build k8s config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	return &Connector{
		clientset:  clientset,
		restConfig: restConfig,
		namespace:  namespace,
	}, nil
}

// Clientset returns the underlying Kubernetes client.
func (c *Connector) Clientset() *kubernetes.Clientset {
	return c.clientset
}

// RESTConfig returns the underlying REST config.
func (c *Connector) RESTConfig() *rest.Config {
	return c.restConfig
}

// Namespace returns the configured namespace.
func (c *Connector) Namespace() string {
	return c.namespace
}

// ForwardResult holds the result of a successful port-forward.
type ForwardResult struct {
	LocalPort int
	StopChan  chan struct{}
}

// PortForward creates a port-forward to the first ready pod matching the label
// selector on the given remote port. Returns the local port to connect to.
func (c *Connector) PortForward(
	ctx context.Context,
	labelSelector string,
	remotePort int,
) (*ForwardResult, error) {
	pods, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	podName := ""
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podName = pod.Name
			break
		}
	}
	if podName == "" {
		return nil, fmt.Errorf(
			"no running pod found for selector %q in namespace %q",
			labelSelector, c.namespace,
		)
	}

	return c.forwardPod(podName, remotePort)
}

func (c *Connector) forwardPod(podName string, remotePort int) (*ForwardResult, error) {
	reqURL := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(c.namespace).
		Name(podName).
		SubResource("portforward").
		URL()

	transport, upgrader, err := spdy.RoundTripperFor(c.restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create SPDY transport: %w", err)
	}

	dialer := spdy.NewDialer(
		upgrader,
		&http.Client{Transport: transport},
		http.MethodPost,
		reqURL,
	)

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})

	ports := []string{fmt.Sprintf("0:%d", remotePort)}
	fw, err := portforward.New(dialer, ports, stopChan, readyChan, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

	select {
	case <-readyChan:
		// Port forward is ready
	case err := <-errChan:
		return nil, fmt.Errorf("port forward failed: %w", err)
	}

	forwardedPorts, err := fw.GetPorts()
	if err != nil {
		close(stopChan)
		return nil, fmt.Errorf("failed to get forwarded ports: %w", err)
	}

	c.forwards = append(c.forwards, fw)

	return &ForwardResult{
		LocalPort: int(forwardedPorts[0].Local),
		StopChan:  stopChan,
	}, nil
}

// WaitForPod waits until a pod matching the selector is in Running phase.
func (c *Connector) WaitForPod(
	ctx context.Context,
	labelSelector string,
	timeout time.Duration,
) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled waiting for pod: %w", ctx.Err())
		case <-deadline:
			return fmt.Errorf(
				"timeout waiting for pod with selector %q", labelSelector,
			)
		case <-ticker.C:
			pods, err := c.clientset.CoreV1().Pods(c.namespace).List(
				ctx, metav1.ListOptions{LabelSelector: labelSelector},
			)
			if err != nil {
				continue
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					return nil
				}
			}
		}
	}
}

// Close stops all active port-forwards.
func (c *Connector) Close() {
	for _, fw := range c.forwards {
		fw.Close()
	}
	c.forwards = nil
}

func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from %s: %w", kubeconfigPath, err)
		}
		return cfg, nil
	}

	// Try KUBECONFIG env var
	if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", envPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from KUBECONFIG: %w", err)
		}
		return cfg, nil
	}

	// Try default location
	home, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(home, ".kube", "config")
		if _, statErr := os.Stat(defaultPath); statErr == nil {
			cfg, buildErr := clientcmd.BuildConfigFromFlags("", defaultPath)
			if buildErr != nil {
				return nil, fmt.Errorf("failed to build config from default path: %w", buildErr)
			}
			return cfg, nil
		}
	}

	// Try in-cluster config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build in-cluster config: %w", err)
	}
	return cfg, nil
}
