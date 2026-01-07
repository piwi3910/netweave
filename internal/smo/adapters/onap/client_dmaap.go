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

// DMaaPClient provides a client for ONAP DMaaP (Data Movement as a Platform) message bus.
// It handles event publishing to DMaaP topics with batching and retry logic.
type DMaaPClient struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
	logger     *zap.Logger
	config     *Config
}

// NewDMaaPClient creates a new DMaaP client with the provided configuration.
func NewDMaaPClient(config *Config, logger *zap.Logger) (*DMaaPClient, error) {
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

	return &DMaaPClient{
		baseURL:    strings.TrimSuffix(config.DMaaPURL, "/"),
		httpClient: httpClient,
		username:   config.Username,
		password:   config.Password,
		logger:     logger,
		config:     config,
	}, nil
}

// Health performs a health check on the DMaaP service.
func (c *DMaaPClient) Health(ctx context.Context) error {
	// DMaaP health check endpoint
	url := fmt.Sprintf("%s/topics", c.baseURL)

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

// PublishEvent publishes a single event to a DMaaP topic.
func (c *DMaaPClient) PublishEvent(ctx context.Context, topic string, event *VESEvent) error {
	return c.PublishEvents(ctx, topic, []*VESEvent{event})
}

// PublishEvents publishes multiple events to a DMaaP topic (batch operation).
func (c *DMaaPClient) PublishEvents(ctx context.Context, topic string, events []*VESEvent) error {
	if len(events) == 0 {
		return nil
	}

	c.logger.Debug("Publishing events to DMaaP",
		zap.String("topic", topic),
		zap.Int("eventCount", len(events)),
	)

	body, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	url := fmt.Sprintf("%s/events/%s", c.baseURL, topic)
	return c.publishWithRetry(ctx, url, body, topic, len(events))
}

// publishWithRetry attempts to publish events with retry logic.
func (c *DMaaPClient) publishWithRetry(
	ctx context.Context,
	url string,
	body []byte,
	topic string,
	eventCount int,
) error {
	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.waitBeforeRetryDMaaP(attempt, topic)
		}

		if err := c.executePublishRequest(ctx, url, body, topic, eventCount, &lastErr); err == nil {
			return nil
		}

		if c.shouldStopRetryingDMaaP(lastErr) {
			break
		}
	}

	return fmt.Errorf("failed to publish events to DMaaP after %d attempts: %w",
		c.config.MaxRetries+1, lastErr)
}

// waitBeforeRetryDMaaP implements exponential backoff for DMaaP retries.
func (c *DMaaPClient) waitBeforeRetryDMaaP(attempt int, topic string) {
	c.logger.Debug("Retrying DMaaP publish",
		zap.String("topic", topic),
		zap.Int("attempt", attempt),
	)
	time.Sleep(time.Duration(attempt) * time.Second)
}

// executePublishRequest executes a single publish request to DMaaP.
func (c *DMaaPClient) executePublishRequest(
	ctx context.Context,
	url string,
	body []byte,
	topic string,
	eventCount int,
	lastErr *error,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		*lastErr = fmt.Errorf("failed to create request: %w", err)
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		*lastErr = fmt.Errorf("request failed: %w", err)
		return fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.handlePublishResponse(resp, topic, eventCount, lastErr)
}

// handlePublishResponse processes the response from a publish request.
func (c *DMaaPClient) handlePublishResponse(resp *http.Response, topic string, eventCount int, lastErr *error) error {
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		c.logger.Info("Successfully published events to DMaaP",
			zap.String("topic", topic),
			zap.Int("eventCount", eventCount),
			zap.Int("statusCode", resp.StatusCode),
		)
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	*lastErr = fmt.Errorf("DMaaP returned status %d: %s", resp.StatusCode, string(bodyBytes))
	return *lastErr
}

// shouldStopRetryingDMaaP determines if DMaaP retries should be stopped.
func (c *DMaaPClient) shouldStopRetryingDMaaP(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "status 4") && !strings.Contains(errMsg, "status 429")
}

// SubscribeTopic subscribes to a DMaaP topic for consuming messages.
// Note: This is for future use if netweave needs to consume events from ONAP.
func (c *DMaaPClient) SubscribeTopic(
	ctx context.Context,
	topic string,
	consumerGroup string,
	consumerID string,
) ([]json.RawMessage, error) {
	url := fmt.Sprintf("%s/events/%s/%s/%s", c.baseURL, topic, consumerGroup, consumerID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DMaaP returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var messages []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("failed to decode messages: %w", err)
	}

	c.logger.Debug("Consumed messages from DMaaP",
		zap.String("topic", topic),
		zap.Int("messageCount", len(messages)),
	)

	return messages, nil
}

// CreateTopic creates a new DMaaP topic.
// Note: Topic creation typically requires admin privileges and may not be needed for normal operations.
func (c *DMaaPClient) CreateTopic(ctx context.Context, topic string, partitions int, replicationFactor int) error {
	topicConfig := map[string]interface{}{
		"topicName":          topic,
		"topicDescription":   fmt.Sprintf("Topic for netweave O2-IMS/DMS events: %s", topic),
		"partitionCount":     partitions,
		"replicationCount":   replicationFactor,
		"transactionEnabled": false,
	}

	body, err := json.Marshal(topicConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal topic config: %w", err)
	}

	url := fmt.Sprintf("%s/topics/create", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DMaaP returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.Info("Successfully created DMaaP topic",
		zap.String("topic", topic),
		zap.Int("partitions", partitions),
		zap.Int("replicationFactor", replicationFactor),
	)

	return nil
}

// Close closes the DMaaP client and releases resources.
func (c *DMaaPClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}
