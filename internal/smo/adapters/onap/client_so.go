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

// SOClient provides a client for ONAP SO (Service Orchestrator) REST API.
// It handles service orchestration, workflow execution, and deployment lifecycle management.
type SOClient struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
	logger     *zap.Logger
	config     *Config
}

// NewSOClient creates a new SO client with the provided configuration.
func NewSOClient(config *Config, logger *zap.Logger) (*SOClient, error) {
	// Warn about insecure TLS configuration
	if config.TLSInsecureSkipVerify {
		logger.Warn("TLS certificate validation is disabled - this is insecure and should only be used in development/testing environments")
	}

	// Create TLS configuration
	tlsConfig, err := createTLSConfig(config, logger)
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

	return &SOClient{
		baseURL:    strings.TrimSuffix(config.SOURL, "/"),
		httpClient: httpClient,
		username:   config.Username,
		password:   config.Password,
		logger:     logger,
		config:     config,
	}, nil
}

// Health performs a health check on the SO service.
func (c *SOClient) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/manage/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
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

// CreateServiceInstance creates a new service instance via SO.
func (c *SOClient) CreateServiceInstance(
	ctx context.Context,
	request *ServiceInstanceRequest,
) (*ServiceInstanceResponse, error) {
	url := fmt.Sprintf("%s/onap/so/infra/serviceInstantiation/v7/serviceInstances", c.baseURL)

	return c.serviceInstanceOperation(ctx, http.MethodPost, url, request, "create service instance")
}

// DeleteServiceInstance deletes a service instance via SO.
func (c *SOClient) DeleteServiceInstance(
	ctx context.Context,
	serviceInstanceID string,
	request *ServiceInstanceRequest,
) (*ServiceInstanceResponse, error) {
	url := fmt.Sprintf("%s/onap/so/infra/serviceInstantiation/v7/serviceInstances/%s", c.baseURL, serviceInstanceID)

	return c.serviceInstanceOperation(ctx, http.MethodDelete, url, request, "delete service instance")
}

// GetOrchestrationStatus retrieves the status of an SO orchestration request.
func (c *SOClient) GetOrchestrationStatus(ctx context.Context, requestID string) (*OrchestrationStatus, error) {
	url := fmt.Sprintf("%s/onap/so/infra/orchestrationRequests/v7/%s", c.baseURL, requestID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("SO returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var orchestrationStatus OrchestrationStatus
	if err := json.NewDecoder(resp.Body).Decode(&orchestrationStatus); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Retrieved orchestration status",
		zap.String("requestId", requestID),
		zap.String("requestState", orchestrationStatus.RequestState),
		zap.Int("progress", orchestrationStatus.PercentProgress),
	)

	return &orchestrationStatus, nil
}

// CancelOrchestration cancels an ongoing SO orchestration request.
func (c *SOClient) CancelOrchestration(ctx context.Context, requestID string) error {
	url := fmt.Sprintf("%s/onap/so/infra/orchestrationRequests/v7/%s/cancel", c.baseURL, requestID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.Info("Successfully cancelled orchestration",
		zap.String("requestId", requestID),
	)

	return nil
}

// ExecuteWorkflow executes a Camunda BPMN workflow via SO.
func (c *SOClient) ExecuteWorkflow(
	ctx context.Context,
	workflowName string,
	parameters map[string]interface{},
) (string, error) {
	// Convert parameters to Camunda variable format
	variables := make(map[string]interface{})
	for key, value := range parameters {
		variables[key] = map[string]interface{}{
			"value": value,
			"type":  "String", // Simplified; in practice, infer type from value
		}
	}

	workflowRequest := &WorkflowExecutionRequest{
		ProcessDefinitionKey: workflowName,
		Variables:            variables,
		BusinessKey:          generateTransactionID(),
	}

	body, err := json.Marshal(workflowRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow request: %w", err)
	}

	// SO Camunda endpoint
	url := fmt.Sprintf("%s/engine-rest/process-definition/key/%s/start", c.baseURL, workflowName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var workflowResponse WorkflowExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&workflowResponse); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Info("Successfully started workflow execution",
		zap.String("workflowName", workflowName),
		zap.String("processInstanceId", workflowResponse.ProcessInstanceID),
	)

	return workflowResponse.ProcessInstanceID, nil
}

// RegisterServiceModel registers a service model with SO catalog.
func (c *SOClient) RegisterServiceModel(ctx context.Context, model *ServiceModel) error {
	url := fmt.Sprintf("%s/onap/so/infra/serviceModels", c.baseURL)

	body, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("failed to marshal service model: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.Info("Successfully registered service model",
		zap.String("modelId", model.ModelInvariantID),
		zap.String("modelName", model.ModelName),
	)

	return nil
}

// GetServiceModel retrieves a service model from SO catalog.
func (c *SOClient) GetServiceModel(ctx context.Context, modelID string) (*ServiceModel, error) {
	url := fmt.Sprintf("%s/onap/so/infra/serviceModels/%s", c.baseURL, modelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("SO returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var model ServiceModel
	if err := json.NewDecoder(resp.Body).Decode(&model); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &model, nil
}

// ListServiceModels retrieves all service models from SO catalog.
func (c *SOClient) ListServiceModels(ctx context.Context) ([]*ServiceModel, error) {
	url := fmt.Sprintf("%s/onap/so/infra/serviceModels", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("SO returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var models []*ServiceModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return models, nil
}

// serviceInstanceOperation is a helper for service instance CRUD operations.
func (c *SOClient) serviceInstanceOperation(
	ctx context.Context,
	method, url string,
	request *ServiceInstanceRequest,
	operation string,
) (*ServiceInstanceResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-FromAppId", "netweave")
	req.Header.Set("X-TransactionId", generateTransactionID())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SO returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response ServiceInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Info("SO service instance operation completed",
		zap.String("operation", operation),
		zap.String("requestId", response.RequestID),
		zap.String("serviceInstanceId", response.ServiceInstanceID),
	)

	return &response, nil
}

// Close closes the SO client and releases resources.
func (c *SOClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}
