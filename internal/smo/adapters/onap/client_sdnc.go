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
	defer resp.Body.Close()

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
func (c *SDNCClient) DeleteNetwork(ctx context.Context, networkID string, networkType string) (*SDNCResponse, error) {
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
func (c *SDNCClient) ConfigureVNF(ctx context.Context, vnfID string, serviceInstanceID string, parameters map[string]interface{}) (*SDNCResponse, error) {
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
func (c *SDNCClient) ActivateService(ctx context.Context, serviceInstanceID string, serviceType string) (*SDNCResponse, error) {
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
func (c *SDNCClient) DeactivateService(ctx context.Context, serviceInstanceID string, serviceType string) (*SDNCResponse, error) {
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
func (c *SDNCClient) executeSDNCOperation(ctx context.Context, operation string, request *SDNCRequest) (*SDNCResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// SDNC operations go through the generic RESTCONF API
	url := fmt.Sprintf("%s/restconf/operations/SLI-API:execute-graph", c.baseURL)

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Debug("Retrying SDNC operation",
				zap.String("operation", operation),
				zap.Int("attempt", attempt),
			)
			// Exponential backoff
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.SetBasicAuth(c.username, c.password)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("SDNC returned status %d: %s", resp.StatusCode, string(bodyBytes))

			// Don't retry on client errors (4xx except 429)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				break
			}
			continue
		}

		var response SDNCResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			lastErr = fmt.Errorf("failed to decode response: %w", err)
			continue
		}

		// Check SDNC response code
		if response.Output.ResponseCode != "200" {
			lastErr = fmt.Errorf("SDNC operation failed: %s", response.Output.ResponseMessage)
			continue
		}

		c.logger.Info("SDNC operation completed successfully",
			zap.String("operation", operation),
			zap.String("responseCode", response.Output.ResponseCode),
		)

		return &response, nil
	}

	return nil, fmt.Errorf("failed to execute SDNC operation %s after %d attempts: %w",
		operation, c.config.MaxRetries+1, lastErr)
}

// Close closes the SDNC client and releases resources.
func (c *SDNCClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
