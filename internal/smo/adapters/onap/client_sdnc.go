package onap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SDNCClient provides a client for ONAP SDNC (Software Defined Network Controller) REST API.
// It handles SDN configuration and network service operations.
type SDNCClient struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
	logger     *zap.Logger
	config     *Config
}

// NewSDNCClient creates a new SDNC client with the provided configuration.
func NewSDNCClient(config *Config, logger *zap.Logger) (*SDNCClient, error) {
	// Warn about insecure TLS configuration
	if config.TLSInsecureSkipVerify {
		logger.Warn(
			"TLS certificate validation is disabled - " +
				"this is insecure and should only be used in development/testing environments",
		)
	}

	// Create TLS configuration
	tlsConfig, err := createTLSConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: config.RequestTimeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &SDNCClient{
		baseURL:    strings.TrimSuffix(config.SDNCURL, "/"),
		httpClient: httpClient,
		username:   config.Username,
		password:   config.Password,
		logger:     logger,
		config:     config,
	}, nil
}

// Health performs a health check on the SDNC service.
func (c *SDNCClient) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/restconf/operations/SLI-API:healthcheck", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateNetwork creates a network via SDNC.
func (c *SDNCClient) CreateNetwork(ctx context.Context, networkInfo *NetworkInformation) (*SDNCResponse, error) {
	request := &SDNCRequest{
		Input: SDNCInput{
			RequestInformation: RequestInformation{
				RequestID:     generateTransactionID(),
				RequestAction: "CreateNetworkInstance",
				Source:        "netweave",
			},
			ServiceInformation: ServiceInformation{
				ServiceInstanceID: networkInfo.NetworkID,
				ServiceType:       networkInfo.NetworkType,
			},
			NetworkInformation: networkInfo,
		},
	}

	return c.executeSDNCOperation(ctx, "create network", request)
}

// DeleteNetwork deletes a network via SDNC.
func (c *SDNCClient) DeleteNetwork(ctx context.Context, networkID, networkType string) (*SDNCResponse, error) {
	request := &SDNCRequest{
		Input: SDNCInput{
			RequestInformation: RequestInformation{
				RequestID:     generateTransactionID(),
				RequestAction: "DeleteNetworkInstance",
				Source:        "netweave",
			},
			ServiceInformation: ServiceInformation{
				ServiceInstanceID: networkID,
				ServiceType:       networkType,
			},
			NetworkInformation: &NetworkInformation{
				NetworkID:   networkID,
				NetworkType: networkType,
			},
		},
	}

	return c.executeSDNCOperation(ctx, "delete network", request)
}

// ConfigureVNF configures a VNF via SDNC.
func (c *SDNCClient) ConfigureVNF(
	ctx context.Context,
	vnfID string,
	serviceInstanceID string,
	parameters map[string]interface{},
) (*SDNCResponse, error) {
	request := &SDNCRequest{
		Input: SDNCInput{
			RequestInformation: RequestInformation{
				RequestID:     generateTransactionID(),
				RequestAction: "ConfigureVNF",
				Source:        "netweave",
			},
			ServiceInformation: ServiceInformation{
				ServiceInstanceID: serviceInstanceID,
				ServiceType:       "vnf-service",
			},
			NetworkInformation: &NetworkInformation{
				NetworkID:   vnfID,
				NetworkType: "vnf",
				Parameters:  parameters,
			},
		},
	}

	return c.executeSDNCOperation(ctx, "configure VNF", request)
}

// ActivateService activates a service via SDNC.
func (c *SDNCClient) ActivateService(
	ctx context.Context,
	serviceInstanceID string,
	serviceType string,
) (*SDNCResponse, error) {
	request := &SDNCRequest{
		Input: SDNCInput{
			RequestInformation: RequestInformation{
				RequestID:     generateTransactionID(),
				RequestAction: "ActivateService",
				Source:        "netweave",
			},
			ServiceInformation: ServiceInformation{
				ServiceInstanceID: serviceInstanceID,
				ServiceType:       serviceType,
			},
		},
	}

	return c.executeSDNCOperation(ctx, "activate service", request)
}

// DeactivateService deactivates a service via SDNC.
func (c *SDNCClient) DeactivateService(
	ctx context.Context,
	serviceInstanceID string,
	serviceType string,
) (*SDNCResponse, error) {
	request := &SDNCRequest{
		Input: SDNCInput{
			RequestInformation: RequestInformation{
				RequestID:     generateTransactionID(),
				RequestAction: "DeactivateService",
				Source:        "netweave",
			},
			ServiceInformation: ServiceInformation{
				ServiceInstanceID: serviceInstanceID,
				ServiceType:       serviceType,
			},
		},
	}

	return c.executeSDNCOperation(ctx, "deactivate service", request)
}

// executeSDNCOperation is a helper method for executing SDNC operations.
func (c *SDNCClient) executeSDNCOperation(
	ctx context.Context,
	operation string,
	request *SDNCRequest,
) (*SDNCResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/restconf/operations/SLI-API:execute-graph", c.baseURL)
	return c.executeWithRetry(ctx, url, body, operation)
}

// executeWithRetry executes an SDNC operation with retry logic.
func (c *SDNCClient) executeWithRetry(
	ctx context.Context,
	url string,
	body []byte,
	operation string,
) (*SDNCResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.waitBeforeRetrySDNC(attempt, operation)
		}

		response, err := c.executeSDNCRequest(ctx, url, body, operation)
		if err == nil {
			return response, nil
		}
		lastErr = err

		if c.shouldStopRetryingSDNC(lastErr) {
			break
		}
	}

	return nil, fmt.Errorf(
		"failed to execute SDNC operation %s after %d attempts: %w",
		operation, c.config.MaxRetries+1, lastErr,
	)
}

// waitBeforeRetrySDNC implements exponential backoff for SDNC retries.
func (c *SDNCClient) waitBeforeRetrySDNC(attempt int, operation string) {
	c.logger.Debug("Retrying SDNC operation",
		zap.String("operation", operation),
		zap.Int("attempt", attempt),
	)
	time.Sleep(time.Duration(attempt) * time.Second)
}

// executeSDNCRequest executes a single SDNC request.
func (c *SDNCClient) executeSDNCRequest(
	ctx context.Context,
	url string,
	body []byte,
	operation string,
) (*SDNCResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	return c.processSDNCResponse(resp, operation)
}

// processSDNCResponse processes the SDNC response.
func (c *SDNCClient) processSDNCResponse(resp *http.Response, operation string) (*SDNCResponse, error) {
	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("SDNC returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("SDNC returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response SDNCResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode SDNC response: %w", err)
	}

	if response.Output.ResponseCode != "200" {
		return nil, fmt.Errorf("SDNC operation failed: %s", response.Output.ResponseMessage)
	}

	c.logger.Info("SDNC operation completed successfully",
		zap.String("operation", operation),
		zap.String("responseCode", response.Output.ResponseCode),
	)

	return &response, nil
}

// shouldStopRetryingSDNC determines if SDNC retries should be stopped.
func (c *SDNCClient) shouldStopRetryingSDNC(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "status 4") && !strings.Contains(errMsg, "status 429")
}

// Close closes the SDNC client and releases resources.
func (c *SDNCClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}
