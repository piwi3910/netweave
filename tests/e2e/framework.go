//go:build e2e

// Package e2e provides end-to-end testing framework for the O2-IMS Gateway.
// These tests verify complete user workflows by deploying the gateway to a
// Kind cluster and executing real API calls.
package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// API path constants for O2-IMS endpoints.
const (
	APIPathResourcePools    = "/o2ims/v1/resourcePools"
	APIPathResourcePoolByID = "/o2ims/v1/resourcePools/%s"
	APIPathResourcesInPool  = "/o2ims/v1/resourcePools/%s/resources"
	APIPathResourceByID     = "/o2ims/v1/resourcePools/%s/resources/%s"
	APIPathSubscriptions    = "/o2ims/v1/subscriptions"
	APIPathSubscriptionByID = "/o2ims/v1/subscriptions/%s"
	APIPathHealthCheck      = "/healthz"
)

// TestFramework provides infrastructure for E2E tests.
type TestFramework struct {
	// KubeClient is the Kubernetes client for cluster operations
	KubeClient kubernetes.Interface

	// APIClient is the HTTP client for API calls to the gateway
	APIClient *http.Client

	// GatewayURL is the base URL of the O2-IMS Gateway
	GatewayURL string

	// WebhookServer is the mock server for receiving subscription notifications
	WebhookServer *WebhookServer

	// Logger for test output
	Logger *zap.Logger

	// Context for test operations
	Context context.Context

	// Cancel function to stop operations
	Cancel context.CancelFunc

	// Namespace for test resources
	Namespace string

	// CleanupFuncs are called during cleanup
	CleanupFuncs []func() error
}

// FrameworkOptions configures the test framework.
type FrameworkOptions struct {
	// KubeconfigPath is the path to kubeconfig file
	KubeconfigPath string

	// GatewayURL is the gateway endpoint (default: detect from service)
	GatewayURL string

	// Namespace for test resources (default: netweave-e2e)
	Namespace string

	// UseTLS enables TLS for API calls
	UseTLS bool

	// TLSCertFile is the client certificate file (for mTLS)
	TLSCertFile string

	// TLSKeyFile is the client key file (for mTLS)
	TLSKeyFile string

	// TLSCAFile is the CA certificate file
	TLSCAFile string

	// Timeout for operations
	Timeout time.Duration
}

// DefaultOptions returns default framework options.
func DefaultOptions() *FrameworkOptions {
	return &FrameworkOptions{
		KubeconfigPath: os.Getenv("KUBECONFIG"),
		Namespace:      "netweave-e2e",
		UseTLS:         false,
		Timeout:        5 * time.Minute,
	}
}

// NewTestFramework creates a new E2E test framework.
func NewTestFramework(opts *FrameworkOptions) (*TestFramework, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	// Build Kubernetes client
	kubeClient, err := buildKubernetesClient(opts.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Build HTTP client
	httpClient, err := buildHTTPClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Resolve gateway URL
	gatewayURL := opts.GatewayURL
	if gatewayURL == "" {
		gatewayURL, err = detectGatewayURL(ctx, kubeClient, opts.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to detect gateway URL: %w", err)
		}
	}

	// Create webhook server for subscription notifications
	webhookServer := NewWebhookServer(logger)

	fw := &TestFramework{
		KubeClient:    kubeClient,
		APIClient:     httpClient,
		GatewayURL:    gatewayURL,
		WebhookServer: webhookServer,
		Logger:        logger,
		Context:       ctx,
		Cancel:        cancel,
		Namespace:     opts.Namespace,
		CleanupFuncs:  make([]func() error, 0),
	}

	// Start webhook server
	if err := webhookServer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start webhook server: %w", err)
	}
	fw.AddCleanup(webhookServer.Stop)

	logger.Info("Test framework initialized",
		zap.String("gatewayURL", gatewayURL),
		zap.String("namespace", opts.Namespace),
		zap.String("webhookURL", webhookServer.URL()),
	)

	return fw, nil
}

// AddCleanup adds a cleanup function to be called during framework teardown.
func (f *TestFramework) AddCleanup(fn func() error) {
	f.CleanupFuncs = append(f.CleanupFuncs, fn)
}

// Cleanup performs cleanup of test resources.
func (f *TestFramework) Cleanup() {
	f.Logger.Info("Cleaning up test framework")

	// Call cleanup functions in reverse order
	for i := len(f.CleanupFuncs) - 1; i >= 0; i-- {
		if err := f.CleanupFuncs[i](); err != nil {
			f.Logger.Error("Cleanup function failed", zap.Error(err))
		}
	}

	// Cancel context
	if f.Cancel != nil {
		f.Cancel()
	}

	// Sync logger
	// Note: Sync errors are ignored because they commonly occur when output
	// is redirected or during test cleanup. These are not critical failures.
	if err := f.Logger.Sync(); err != nil {
		_ = err
	}
}

// buildKubernetesClient creates a Kubernetes client from kubeconfig.
func buildKubernetesClient(kubeconfigPath string) (kubernetes.Interface, error) {
	if kubeconfigPath == "" {
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return client, nil
}

// buildHTTPClient creates an HTTP client with optional TLS configuration.
func buildHTTPClient(opts *FrameworkOptions) (*http.Client, error) {
	// Transport settings optimized for E2E testing:
	// - MaxIdleConns/MaxIdleConnsPerHost: Reuse connections for faster tests
	// - IdleConnTimeout: Keep connections alive between test cases
	// - Compression enabled for realistic testing
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: 5,
	}

	if opts.UseTLS {
		tlsConfig, err := buildTLSConfig(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", err)
		}
		transport.TLSClientConfig = tlsConfig
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	return client, nil
}

// buildTLSConfig creates TLS configuration for mTLS.
func buildTLSConfig(opts *FrameworkOptions) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Load CA certificate if provided
	if opts.TLSCAFile != "" {
		caCert, err := os.ReadFile(opts.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate if provided (for mTLS)
	if opts.TLSCertFile != "" && opts.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.TLSCertFile, opts.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// detectGatewayURL attempts to detect the gateway URL from the Kubernetes service.
// Currently returns a hardcoded localhost URL for port-forwarded E2E tests.
// Parameters are accepted for future enhancement when auto-detecting service URLs
// from the Kubernetes API (e.g., LoadBalancer external IPs, NodePort addresses).
func detectGatewayURL(_ context.Context, _ kubernetes.Interface, _ string) (string, error) {
	// For E2E tests, we assume port-forwarding or NodePort access.
	// The setup.sh script establishes port-forward to localhost:8080.
	return "http://localhost:8080", nil
}
