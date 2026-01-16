package starlingx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

// Client is a StarlingX System Inventory (sysinv) API client.
type Client struct {
	endpoint   string
	authClient *AuthClient
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new StarlingX API client.
func NewClient(endpoint string, authClient *AuthClient, logger *zap.Logger) *Client {
	return &Client{
		endpoint:   endpoint,
		authClient: authClient,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

// doRequest executes an HTTP request with authentication.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	token, err := c.authClient.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth token: %w", err)
	}

	req, err := c.buildRequest(ctx, method, path, body, token)
	if err != nil {
		return err
	}

	respBody, statusCode, err := c.executeRequest(req)
	if err != nil {
		return err
	}

	if statusCode == http.StatusUnauthorized {
		return c.retryWithNewToken(ctx, req, result)
	}

	if err := c.handleErrorResponse(statusCode, respBody); err != nil {
		return err
	}

	return c.unmarshalResult(respBody, result)
}

func (c *Client) buildRequest(
	ctx context.Context,
	method, path string,
	body interface{},
	token string,
) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	reqURL := fmt.Sprintf("%s%s", c.endpoint, path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Auth-Token", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c.logger.Debug("executing starlingx api request",
		zap.String("method", method),
		zap.String("url", reqURL),
	)

	return req, nil
}

func (c *Client) executeRequest(req *http.Request) ([]byte, int, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

func (c *Client) retryWithNewToken(ctx context.Context, req *http.Request, result interface{}) error {
	c.logger.Warn("received 401 unauthorized, invalidating token and retrying")
	c.authClient.InvalidateToken()

	token, err := c.authClient.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh auth token: %w", err)
	}

	req.Header.Set("X-Auth-Token", token)
	respBody, statusCode, err := c.executeRequest(req)
	if err != nil {
		return err
	}

	if err := c.handleErrorResponse(statusCode, respBody); err != nil {
		return err
	}

	return c.unmarshalResult(respBody, result)
}

func (c *Client) handleErrorResponse(statusCode int, respBody []byte) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(respBody, &errResp); err == nil {
		return fmt.Errorf("starlingx api error (status %d): %s - %s", statusCode, errResp.Error, errResp.Detail)
	}
	return fmt.Errorf("starlingx api error (status %d): %s", statusCode, string(respBody))
}

func (c *Client) unmarshalResult(respBody []byte, result interface{}) error {
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}
	return nil
}

// ListHosts retrieves all hosts, optionally filtered by personality.
func (c *Client) ListHosts(ctx context.Context, personality string) ([]IHost, error) {
	path := "/v1/ihosts"
	if personality != "" {
		path = fmt.Sprintf("%s?personality=%s", path, url.QueryEscape(personality))
	}

	var hostList IHostList
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &hostList); err != nil {
		return nil, err
	}

	return hostList.IHosts, nil
}

// GetHost retrieves a specific host by UUID.
func (c *Client) GetHost(ctx context.Context, uuid string) (*IHost, error) {
	path := fmt.Sprintf("/v1/ihosts/%s", uuid)

	var host IHost
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &host); err != nil {
		return nil, err
	}

	return &host, nil
}

// CreateHost provisions a new host.
func (c *Client) CreateHost(ctx context.Context, req *CreateHostRequest) (*IHost, error) {
	var host IHost
	if err := c.doRequest(ctx, http.MethodPost, "/v1/ihosts", req, &host); err != nil {
		return nil, err
	}

	return &host, nil
}

// UpdateHost updates host configuration using JSON Patch.
func (c *Client) UpdateHost(ctx context.Context, uuid string, req *UpdateHostRequest) (*IHost, error) {
	path := fmt.Sprintf("/v1/ihosts/%s", uuid)

	var host IHost
	if err := c.doRequest(ctx, http.MethodPatch, path, req, &host); err != nil {
		return nil, err
	}

	return &host, nil
}

// DeleteHost deletes a host.
func (c *Client) DeleteHost(ctx context.Context, uuid string) error {
	path := fmt.Sprintf("/v1/ihosts/%s", uuid)
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}

// GetHostCPUs retrieves CPU information for a host.
func (c *Client) GetHostCPUs(ctx context.Context, hostUUID string) ([]ICPU, error) {
	path := fmt.Sprintf("/v1/ihosts/%s/icpus", hostUUID)

	var cpuList ICPUList
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &cpuList); err != nil {
		return nil, err
	}

	return cpuList.ICPUs, nil
}

// GetHostMemory retrieves memory information for a host.
func (c *Client) GetHostMemory(ctx context.Context, hostUUID string) ([]IMemory, error) {
	path := fmt.Sprintf("/v1/ihosts/%s/imemorys", hostUUID)

	var memList IMemoryList
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &memList); err != nil {
		return nil, err
	}

	return memList.IMemories, nil
}

// GetHostDisks retrieves disk information for a host.
func (c *Client) GetHostDisks(ctx context.Context, hostUUID string) ([]IDisk, error) {
	path := fmt.Sprintf("/v1/ihosts/%s/idisks", hostUUID)

	var diskList IDiskList
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &diskList); err != nil {
		return nil, err
	}

	return diskList.IDisks, nil
}

// ListLabels retrieves all labels.
func (c *Client) ListLabels(ctx context.Context) ([]Label, error) {
	var labelList LabelList
	if err := c.doRequest(ctx, http.MethodGet, "/v1/labels", nil, &labelList); err != nil {
		return nil, err
	}

	return labelList.Labels, nil
}

// GetLabel retrieves a specific label by UUID.
func (c *Client) GetLabel(ctx context.Context, uuid string) (*Label, error) {
	path := fmt.Sprintf("/v1/labels/%s", uuid)

	var label Label
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &label); err != nil {
		return nil, err
	}

	return &label, nil
}

// CreateLabel creates a new host label.
func (c *Client) CreateLabel(ctx context.Context, req *CreateLabelRequest) (*Label, error) {
	var label Label
	if err := c.doRequest(ctx, http.MethodPost, "/v1/labels", req, &label); err != nil {
		return nil, err
	}

	return &label, nil
}

// DeleteLabel deletes a label.
func (c *Client) DeleteLabel(ctx context.Context, uuid string) error {
	path := fmt.Sprintf("/v1/labels/%s", uuid)
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}

// ListSystems retrieves all systems.
func (c *Client) ListSystems(ctx context.Context) ([]ISystem, error) {
	var systemList ISystemList
	if err := c.doRequest(ctx, http.MethodGet, "/v1/isystems", nil, &systemList); err != nil {
		return nil, err
	}

	return systemList.ISystems, nil
}

// GetSystem retrieves a specific system by UUID.
func (c *Client) GetSystem(ctx context.Context, uuid string) (*ISystem, error) {
	path := fmt.Sprintf("/v1/isystems/%s", uuid)

	var system ISystem
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &system); err != nil {
		return nil, err
	}

	return &system, nil
}
