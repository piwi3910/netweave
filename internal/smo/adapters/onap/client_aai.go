package onap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AAIClient provides a client for ONAP A&AI (Active & Available Inventory) REST API.
// It handles authentication, TLS configuration, and retry logic for A&AI operations.
type AAIClient struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
	logger     *zap.Logger
	config     *Config
}

// NewAAIClient creates a new A&AI client with the provided configuration.
func NewAAIClient(config *Config, logger *zap.Logger) (*AAIClient, error) {
	// Warn about insecure TLS configuration
	if config.TLSInsecureSkipVerify {
		logger.Warn("TLS certificate validation is disabled - this is insecure and should only be used in development/testing environments")
	}

	// Create TLS configuration
	tlsConfig, err := createTLSConfig(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}

	// Create HTTP client with timeouts and TLS
	httpClient := &http.Client{
		Timeout: config.RequestTimeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &AAIClient{
		baseURL:    strings.TrimSuffix(config.AAIURL, "/"),
		httpClient: httpClient,
		username:   config.Username,
		password:   config.Password,
		logger:     logger,
		config:     config,
	}, nil
}

// Health performs a health check on the A&AI service.
func (c *AAIClient) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/aai/util/echo", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateOrUpdateCloudRegion creates or updates a cloud region in A&AI.
func (c *AAIClient) CreateOrUpdateCloudRegion(ctx context.Context, cloudRegion *CloudRegion) error {
	url := fmt.Sprintf("%s/aai/v24/cloud-infrastructure/cloud-regions/cloud-region/%s/%s",
		c.baseURL, cloudRegion.CloudOwner, cloudRegion.CloudRegionID)

	return c.putResource(ctx, url, cloudRegion, "cloud region")
}

// CreateOrUpdateTenant creates or updates a tenant in A&AI.
func (c *AAIClient) CreateOrUpdateTenant(ctx context.Context, tenant *Tenant) error {
	url := fmt.Sprintf("%s/aai/v24/cloud-infrastructure/cloud-regions/cloud-region/%s/%s/tenants/tenant/%s",
		c.baseURL, tenant.CloudOwner, tenant.CloudRegionID, tenant.TenantID)

	return c.putResource(ctx, url, tenant, "tenant")
}

// CreateOrUpdatePNF creates or updates a PNF (Physical Network Function) in A&AI.
func (c *AAIClient) CreateOrUpdatePNF(ctx context.Context, pnf *PNF) error {
	url := fmt.Sprintf("%s/aai/v24/network/pnfs/pnf/%s", c.baseURL, pnf.PNFName)

	return c.putResource(ctx, url, pnf, "PNF")
}

// CreateOrUpdateVNF creates or updates a VNF (Virtual Network Function) in A&AI.
func (c *AAIClient) CreateOrUpdateVNF(ctx context.Context, vnf *VNF) error {
	url := fmt.Sprintf("%s/aai/v24/network/generic-vnfs/generic-vnf/%s", c.baseURL, vnf.VNFID)

	return c.putResource(ctx, url, vnf, "VNF")
}

// CreateOrUpdateServiceInstance creates or updates a service instance in A&AI.
func (c *AAIClient) CreateOrUpdateServiceInstance(ctx context.Context, serviceInstance *ServiceInstance) error {
	// Service instances are under customers, using a default customer for O2DMS
	customerID := "netweave-o2dms"
	serviceType := serviceInstance.ServiceType

	url := fmt.Sprintf(
		"%s/aai/v24/business/customers/customer/%s/"+
			"service-subscriptions/service-subscription/%s/"+
			"service-instances/service-instance/%s",
		c.baseURL, customerID, serviceType, serviceInstance.ServiceInstanceID,
	)

	return c.putResource(ctx, url, serviceInstance, "service instance")
}

// GetServiceInstance retrieves a service instance from A&AI.
func (c *AAIClient) GetServiceInstance(ctx context.Context, serviceInstanceID string) (*ServiceInstance, error) {
	// This is a simplified implementation; in practice, you'd need to know the customer and service type
	// or perform a search query

	customerID := "netweave-o2dms"
	serviceType := "netweave-deployment"

	url := fmt.Sprintf(
		"%s/aai/v24/business/customers/customer/%s/"+
			"service-subscriptions/service-subscription/%s/"+
			"service-instances/service-instance/%s",
		c.baseURL, customerID, serviceType, serviceInstanceID,
	)

	var serviceInstance ServiceInstance
	if err := c.getResource(ctx, url, &serviceInstance, "service instance"); err != nil {
		return nil, err
	}

	return &serviceInstance, nil
}

// putResource is a helper method to PUT a resource to A&AI with retry logic.
func (c *AAIClient) putResource(ctx context.Context, url string, resource interface{}, resourceType string) error {
	body, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", resourceType, err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.waitBeforeRetry(attempt, resourceType)
		}

		if err := c.executePutRequest(ctx, url, body, resourceType, &lastErr); err == nil {
			return nil
		}

		if c.shouldStopRetrying(lastErr) {
			break
		}
	}

	return fmt.Errorf("failed to create/update %s after %d attempts: %w",
		resourceType, c.config.MaxRetries+1, lastErr)
}

// waitBeforeRetry implements exponential backoff for retries.
func (c *AAIClient) waitBeforeRetry(attempt int, resourceType string) {
	c.logger.Debug("Retrying A&AI request",
		zap.String("resourceType", resourceType),
		zap.Int("attempt", attempt),
	)
	time.Sleep(time.Duration(attempt) * time.Second)
}

// executePutRequest executes a single PUT request to A&AI.
func (c *AAIClient) executePutRequest(
	ctx context.Context,
	url string,
	body []byte,
	resourceType string,
	lastErr *error,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		*lastErr = fmt.Errorf("failed to create request: %w", err)
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		*lastErr = fmt.Errorf("request failed: %w", err)
		return fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.handlePutResponse(resp, resourceType, lastErr)
}

// setRequestHeaders sets the required headers for A&AI requests.
func (c *AAIClient) setRequestHeaders(req *http.Request) {
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// handlePutResponse processes the response from a PUT request.
func (c *AAIClient) handlePutResponse(resp *http.Response, resourceType string, lastErr *error) error {
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		c.logger.Debug("Successfully created/updated resource in A&AI",
			zap.String("resourceType", resourceType),
			zap.Int("statusCode", resp.StatusCode),
		)
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	*lastErr = fmt.Errorf("A&AI returned status %d: %s", resp.StatusCode, string(bodyBytes))
	return *lastErr
}

// shouldStopRetrying determines if retries should be stopped based on the error.
func (c *AAIClient) shouldStopRetrying(err error) bool {
	if err == nil {
		return false
	}
	// Extract status code from error message and check if it's a non-retryable client error
	errMsg := err.Error()
	return strings.Contains(errMsg, "status 4") && !strings.Contains(errMsg, "status 429")
}

// getResource is a helper method to GET a resource from A&AI.
func (c *AAIClient) getResource(ctx context.Context, url string, result interface{}, _ string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("A&AI returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Close closes the A&AI client and releases resources.
func (c *AAIClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// createTLSConfig creates a TLS configuration from the plugin config.
// Returns nil when TLS is not enabled (uses default HTTP transport).
// WARNING: InsecureSkipVerify disables certificate validation and should only be used in development/testing.
func createTLSConfig(config *Config, logger *zap.Logger) (*tls.Config, error) {
	if !config.TLSEnabled {
		return &tls.Config{MinVersion: tls.VersionTLS12}, nil
	}

	// G402: InsecureSkipVerify is intentionally configurable for development/testing environments
	// Production deployments should always use proper certificate validation (InsecureSkipVerify=false)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.TLSInsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	// Load client certificate if provided
	if config.TLSCertFile != "" && config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided
	if config.TLSCAFile != "" {
		caCert, err := os.ReadFile(config.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// generateTransactionID generates a unique transaction ID for A&AI requests.
func generateTransactionID() string {
	return fmt.Sprintf("netweave-%d", time.Now().UnixNano())
}
