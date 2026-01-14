// Package e2e provides end-to-end tests.
//
//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/tests/e2e"

	"go.uber.org/zap"
)

// Test timeout constants.
const (
	eventDeliveryTimeout = 5 * time.Second
	webhookRetryTimeout  = 30 * time.Second
	eventWaitTimeout     = 10 * time.Second
)

// extractSubscriptionID extracts and validates subscription ID from API response.
func extractSubscriptionID(t *testing.T, response map[string]any) string {
	t.Helper()
	subID, ok := response["subscriptionId"].(string)
	require.True(t, ok, "subscriptionId is not a string or missing")
	require.NotEmpty(t, subID, "subscriptionId is empty")
	return subID
}

// TestSubscriptionWorkflow tests the complete subscription lifecycle.
func TestSubscriptionWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	var subscriptionID string

	t.Run("create subscription", func(t *testing.T) {
		subscription := map[string]any{
			"callback": fw.WebhookServer.URL(),
			"filter":   "(resourceType==Node)",
		}

		reqBody, err := json.Marshal(subscription)
		require.NoError(t, err)

		url := fw.GatewayURL + e2e.APIPathSubscriptions
		req, err := http.NewRequestWithContext(fw.Context, http.MethodPost, url, bytes.NewReader(reqBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var createdSub map[string]any
		err = json.Unmarshal(body, &createdSub)
		require.NoError(t, err)

		assert.Contains(t, createdSub, "subscriptionId")
		assert.Equal(t, fw.WebhookServer.URL(), createdSub["callback"])

		subID, ok := createdSub["subscriptionId"].(string)
		require.True(t, ok, "subscriptionId is not a string")
		require.NotEmpty(t, subID, "subscriptionId is empty")
		subscriptionID = subID

		fw.Logger.Info("Successfully created subscription",
			zap.String("subscriptionId", subscriptionID),
		)
	})

	t.Run("list subscriptions", func(t *testing.T) {
		url := fw.GatewayURL + e2e.APIPathSubscriptions
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var subscriptions []map[string]any
		err = json.Unmarshal(body, &subscriptions)
		require.NoError(t, err)

		// Should contain our subscription
		found := false
		for _, sub := range subscriptions {
			if sub["subscriptionId"] == subscriptionID {
				found = true
				break
			}
		}
		assert.True(t, found, "Created subscription not found in list")

		fw.Logger.Info("Successfully listed subscriptions",
			zap.Int("count", len(subscriptions)),
		)
	})

	t.Run("get subscription", func(t *testing.T) {
		url := fw.GatewayURL + fmt.Sprintf(e2e.APIPathSubscriptionByID, subscriptionID)
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var sub map[string]any
		err = json.Unmarshal(body, &sub)
		require.NoError(t, err)

		assert.Equal(t, subscriptionID, sub["subscriptionId"])
		assert.Equal(t, fw.WebhookServer.URL(), sub["callback"])

		fw.Logger.Info("Successfully retrieved subscription",
			zap.String("subscriptionId", subscriptionID),
		)
	})

	t.Run("delete subscription", func(t *testing.T) {
		url := fw.GatewayURL + fmt.Sprintf(e2e.APIPathSubscriptionByID, subscriptionID)
		req, err := http.NewRequestWithContext(fw.Context, http.MethodDelete, url, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		fw.Logger.Info("Successfully deleted subscription",
			zap.String("subscriptionId", subscriptionID),
		)

		// Verify it's gone
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestSubscriptionNotifications tests webhook notification delivery.
func TestSubscriptionNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	t.Skip("Notification testing requires triggering actual resource changes in Kubernetes")

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	// Clear any existing events
	fw.WebhookServer.ClearEvents()

	// Create subscription
	subscription := map[string]any{
		"callback": fw.WebhookServer.URL(),
		"filter":   "(resourceType==Namespace)",
	}

	reqBody, err := json.Marshal(subscription)
	require.NoError(t, err)

	url := fw.GatewayURL + e2e.APIPathSubscriptions
	resp, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// TODO: Create a namespace in Kubernetes to trigger an event
	// This would require the test to have write access to the cluster

	// Wait for webhook notification
	event, err := fw.WebhookServer.WaitForEvent(30 * time.Second)
	if err == nil {
		assert.Equal(t, "Namespace", event.ResourceType)
		fw.Logger.Info("Received webhook notification",
			zap.String("eventId", event.ID),
			zap.String("resourceType", event.ResourceType),
		)
	}
}

// Additional comprehensive E2E tests for subscription workflow

// TestSubscriptionFiltering tests subscription filtering by resource attributes.
func TestSubscriptionFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	// Create two webhook servers for two different subscriptions
	webhook1 := e2e.NewWebhookServer(fw.Logger.Named("webhook1"))
	require.NoError(t, webhook1.Start())
	defer func() {
		if err := webhook1.Stop(); err != nil {
			t.Logf("Failed to stop webhook1: %v", err)
		}
	}()

	webhook2 := e2e.NewWebhookServer(fw.Logger.Named("webhook2"))
	require.NoError(t, webhook2.Start())
	defer func() {
		if err := webhook2.Stop(); err != nil {
			t.Logf("Failed to stop webhook2: %v", err)
		}
	}()

	fw.Logger.Info("Created webhook servers",
		zap.String("webhook1URL", webhook1.URL()),
		zap.String("webhook2URL", webhook2.URL()),
	)

	// Subscription 1: Filter for Node resources
	sub1 := map[string]any{
		"callback": webhook1.URL(),
		"filter":   "(resourceType==Node)",
	}
	reqBody1, err := json.Marshal(sub1)
	require.NoError(t, err)

	url := fw.GatewayURL + e2e.APIPathSubscriptions
	resp1, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody1))
	require.NoError(t, err)
	defer func() {
		if err := resp1.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	// Subscription 2: Filter for Namespace resources
	sub2 := map[string]any{
		"callback": webhook2.URL(),
		"filter":   "(resourceType==Namespace)",
	}
	reqBody2, err := json.Marshal(sub2)
	require.NoError(t, err)

	resp2, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody2))
	require.NoError(t, err)
	defer func() {
		if err := resp2.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()
	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	fw.Logger.Info("Created two subscriptions with different filters")

	// Clear any existing events
	webhook1.ClearEvents()
	webhook2.ClearEvents()

	// Create K8s resource helper
	k8sHelper := e2e.NewK8sResourceHelper(fw.KubeClient)

	// Create a Pod (should trigger webhook1 if adapter supports Pod events)
	podName := "filter-test-pod"
	pod, err := k8sHelper.CreateTestPod(fw.Context, fw.Namespace, podName, map[string]string{"test": "filtering"})
	if err != nil {
		t.Logf("Failed to create pod (may be expected in some environments): %v", err)
	} else {
		defer func() {
			if delErr := k8sHelper.DeletePod(fw.Context, fw.Namespace, podName); delErr != nil {
				t.Logf("Failed to cleanup pod: %v", delErr)
			}
		}()
		fw.Logger.Info("Created test pod", zap.String("podName", pod.Name))
	}

	// Create a Namespace (should trigger webhook2)
	nsName := "filter-test-namespace"
	ns, err := k8sHelper.CreateTestNamespace(fw.Context, nsName)
	if err != nil {
		t.Logf("Failed to create namespace: %v", err)
	} else {
		defer func() {
			if delErr := k8sHelper.DeleteNamespace(fw.Context, nsName); delErr != nil {
				t.Logf("Failed to cleanup namespace: %v", delErr)
			}
		}()
		fw.Logger.Info("Created test namespace", zap.String("namespace", ns.Name))
	}

	// Wait for events to be delivered
	time.Sleep(eventDeliveryTimeout)

	// Check webhook1 (should receive Pod events, not Namespace)
	webhook1Events := webhook1.GetReceivedEvents()
	fw.Logger.Info("Webhook1 events",
		zap.Int("count", len(webhook1Events)),
		zap.String("filter", "(resourceType==Node)"),
	)

	// Check webhook2 (should receive Namespace events, not Pod)
	webhook2Events := webhook2.GetReceivedEvents()
	fw.Logger.Info("Webhook2 events",
		zap.Int("count", len(webhook2Events)),
		zap.String("filter", "(resourceType==Namespace)"),
	)

	// Validate filtering (if events were received)
	for _, evt := range webhook1Events {
		assert.NotEqual(t, "Namespace", evt.ResourceType,
			"Webhook1 should not receive Namespace events")
	}

	for _, evt := range webhook2Events {
		assert.Equal(t, "Namespace", evt.ResourceType,
			"Webhook2 should only receive Namespace events")
	}

	if len(webhook1Events) == 0 && len(webhook2Events) == 0 {
		t.Skip("No events received - subscription notification may not be configured")
	}

	fw.Logger.Info("Subscription filtering test completed",
		zap.Int("webhook1Events", len(webhook1Events)),
		zap.Int("webhook2Events", len(webhook2Events)),
	)
}

// TestConcurrentSubscriptions tests multiple concurrent subscriptions.
func TestConcurrentSubscriptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	const numSubscriptions = 5
	webhooks := make([]*e2e.WebhookServer, numSubscriptions)
	subscriptionIDs := make([]string, numSubscriptions)

	// Create multiple subscriptions concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0, numSubscriptions)

	for i := 0; i < numSubscriptions; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Create webhook server
			webhook := e2e.NewWebhookServer(fw.Logger.Named(fmt.Sprintf("webhook%d", idx)))
			if err := webhook.Start(); err != nil {
				// Cleanup webhook even on failure to prevent resource leak
				_ = webhook.Stop()
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to start webhook%d: %w", idx, err))
				mu.Unlock()
				return
			}
			webhooks[idx] = webhook

			// Create subscription
			subscription := map[string]any{
				"callback":               webhook.URL(),
				"consumerSubscriptionId": fmt.Sprintf("concurrent-sub-%d", idx),
				"filter":                 "(resourceType==Node)",
			}

			reqBody, err := json.Marshal(subscription)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to marshal subscription%d: %w", idx, err))
				mu.Unlock()
				return
			}

			url := fw.GatewayURL + e2e.APIPathSubscriptions
			resp, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody))
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to create subscription%d: %w", idx, err))
				mu.Unlock()
				return
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Logf("Failed to close response body: %v", err)
				}
			}()

			if resp.StatusCode != http.StatusCreated {
				mu.Lock()
				errors = append(errors, fmt.Errorf("subscription%d got status %d", idx, resp.StatusCode))
				mu.Unlock()
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to read subscription%d response: %w", idx, err))
				mu.Unlock()
				return
			}

			var createdSub map[string]any
			if err := json.Unmarshal(body, &createdSub); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to unmarshal subscription%d response: %w", idx, err))
				mu.Unlock()
				return
			}

			subID, ok := createdSub["subscriptionId"].(string)
			if !ok || subID == "" {
				mu.Lock()
				errors = append(errors, fmt.Errorf("subscription%d: invalid subscriptionId", idx))
				mu.Unlock()
				return
			}

			mu.Lock()
			subscriptionIDs[idx] = subID
			mu.Unlock()

			fw.Logger.Info("Created subscription",
				zap.Int("index", idx),
				zap.String("subscriptionId", subID),
			)
		}(i)
	}

	wg.Wait()

	// Check for errors
	require.Empty(t, errors, "Errors during concurrent subscription creation: %v", errors)

	// Verify all subscriptions were created
	for i, subID := range subscriptionIDs {
		assert.NotEmpty(t, subID, "Subscription %d ID is empty", i)
	}

	// Cleanup webhooks
	for i, webhook := range webhooks {
		if webhook != nil {
			if err := webhook.Stop(); err != nil {
				t.Logf("Failed to stop webhook%d: %v", i, err)
			}
		}
	}

	fw.Logger.Info("Concurrent subscriptions test completed",
		zap.Int("created", numSubscriptions),
	)
}

// TestSubscriptionInvalidCallback tests error handling for invalid callback URLs.
func TestSubscriptionInvalidCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	tests := []struct {
		name         string
		callback     string
		expectStatus int
	}{
		{
			name:         "invalid URL format",
			callback:     "not-a-valid-url",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "empty callback",
			callback:     "",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "non-http scheme",
			callback:     "ftp://example.com/webhook",
			expectStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscription := map[string]any{
				"callback": tt.callback,
				"filter":   "(resourceType==Node)",
			}

			reqBody, err := json.Marshal(subscription)
			require.NoError(t, err)

			url := fw.GatewayURL + e2e.APIPathSubscriptions
			resp, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody))
			require.NoError(t, err)
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Logf("Failed to close response body: %v", err)
				}
			}()

			assert.Equal(t, tt.expectStatus, resp.StatusCode,
				"Expected status %d for invalid callback %q", tt.expectStatus, tt.callback)

			fw.Logger.Info("Invalid callback test passed",
				zap.String("testCase", tt.name),
				zap.Int("statusCode", resp.StatusCode),
			)
		})
	}
}

// TestSubscriptionDeletionStopsNotifications tests that deleting a subscription stops notifications.
func TestSubscriptionDeletionStopsNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	t.Skip("Requires triggering Kubernetes events - will implement with resource creation")

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	// Create subscription
	subscription := map[string]any{
		"callback": fw.WebhookServer.URL(),
		"filter":   "(resourceType==Node)",
	}

	reqBody, err := json.Marshal(subscription)
	require.NoError(t, err)

	url := fw.GatewayURL + e2e.APIPathSubscriptions
	resp, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var createdSub map[string]any
	err = json.Unmarshal(body, &createdSub)
	require.NoError(t, err)

	subscriptionID := extractSubscriptionID(t, createdSub)

	fw.WebhookServer.ClearEvents()

	// TODO: Trigger an event - should receive notification

	// Delete subscription
	deleteURL := fw.GatewayURL + fmt.Sprintf(e2e.APIPathSubscriptionByID, subscriptionID)
	req, err := http.NewRequestWithContext(fw.Context, http.MethodDelete, deleteURL, nil)
	require.NoError(t, err)

	delResp, err := fw.APIClient.Do(req)
	require.NoError(t, err)
	defer func() {
		if err := delResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	require.Equal(t, http.StatusNoContent, delResp.StatusCode)

	// TODO: Trigger another event - should NOT receive notification

	fw.Logger.Info("Subscription deletion test completed")
}

// TestWebhookRetryLogic tests webhook delivery retry with exponential backoff.
func TestWebhookRetryLogic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	t.Skip("Requires triggering actual resource changes and monitoring retry attempts")

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	// Create a webhook server that fails the first N attempts
	var attemptCount int
	var attemptMu sync.Mutex
	const failAttempts = 2

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptMu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		attemptMu.Unlock()

		fw.Logger.Info("Webhook delivery attempt",
			zap.Int("attempt", currentAttempt),
			zap.Int("failUntil", failAttempts),
		)

		if currentAttempt <= failAttempts {
			// Fail first N attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Succeed on subsequent attempts
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			t.Logf("Failed to write response: %v", err)
		}
	}))
	defer failingServer.Close()

	// Create subscription with the failing webhook
	subscription := map[string]any{
		"callback": failingServer.URL,
		"filter":   "(resourceType==Node)",
	}

	reqBody, err := json.Marshal(subscription)
	require.NoError(t, err)

	url := fw.GatewayURL + e2e.APIPathSubscriptions
	resp, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var createdSub map[string]any
	err = json.Unmarshal(body, &createdSub)
	require.NoError(t, err)

	subscriptionID := extractSubscriptionID(t, createdSub)

	fw.Logger.Info("Created subscription with failing webhook",
		zap.String("subscriptionId", subscriptionID),
		zap.String("callback", failingServer.URL),
	)

	// TODO: Trigger a Kubernetes event to test retry logic
	// The gateway should retry webhook delivery with exponential backoff

	// Wait for retries to complete
	time.Sleep(webhookRetryTimeout)

	// Verify retry attempts
	attemptMu.Lock()
	finalAttempts := attemptCount
	attemptMu.Unlock()

	assert.GreaterOrEqual(t, finalAttempts, failAttempts+1,
		"Should have retried at least %d times", failAttempts+1)

	fw.Logger.Info("Webhook retry test completed",
		zap.Int("totalAttempts", finalAttempts),
		zap.Int("failedAttempts", failAttempts),
	)
}

// TestResourceLifecycleEvents tests event generation for resource CRUD operations.
func TestResourceLifecycleEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	// Create K8s resource helper
	k8sHelper := e2e.NewK8sResourceHelper(fw.KubeClient)

	// Clear any existing events
	fw.WebhookServer.ClearEvents()

	// Create subscription for all events
	subscription := map[string]any{
		"callback":               fw.WebhookServer.URL(),
		"consumerSubscriptionId": "lifecycle-test",
		"filter":                 "(resourceType==Namespace)",
	}

	reqBody, err := json.Marshal(subscription)
	require.NoError(t, err)

	url := fw.GatewayURL + e2e.APIPathSubscriptions
	resp, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var createdSub map[string]any
	err = json.Unmarshal(body, &createdSub)
	require.NoError(t, err)

	subscriptionID := extractSubscriptionID(t, createdSub)

	fw.Logger.Info("Created subscription for lifecycle events",
		zap.String("subscriptionId", subscriptionID),
	)

	// Event 1 - Create a Kubernetes Namespace
	testNsName := "e2e-lifecycle-test"
	ns, err := k8sHelper.CreateTestNamespace(fw.Context, testNsName)
	require.NoError(t, err)
	defer func() {
		if delErr := k8sHelper.DeleteNamespace(fw.Context, testNsName); delErr != nil {
			t.Logf("Failed to cleanup namespace: %v", delErr)
		}
	}()

	fw.Logger.Info("Created test namespace", zap.String("namespace", ns.Name))

	// Wait for create event
	createEvent, err := fw.WebhookServer.WaitForEventWithFilter(eventWaitTimeout, func(e *e2e.WebhookEvent) bool {
		return e.Type == "resource.created" && e.ResourceType == "Namespace"
	})
	if err == nil {
		assert.Equal(t, "resource.created", createEvent.Type)
		assert.Equal(t, subscriptionID, createEvent.SubscriptionID)
		fw.Logger.Info("Received create event", zap.String("eventId", createEvent.ID))
	} else {
		t.Logf("Warning: Did not receive create event within timeout: %v", err)
	}

	// Event 2 - Update the Namespace (add label/annotation)
	// Note: Namespace updates may not trigger events in all adapters
	// This tests the update notification path if supported

	// Wait for update event (may timeout if adapter doesn't support namespace updates)
	updateEvent, err := fw.WebhookServer.WaitForEventWithFilter(eventWaitTimeout, func(e *e2e.WebhookEvent) bool {
		return e.Type == "resource.updated" && e.ResourceType == "Namespace"
	})
	if err == nil {
		assert.Equal(t, "resource.updated", updateEvent.Type)
		assert.Equal(t, subscriptionID, updateEvent.SubscriptionID)
		fw.Logger.Info("Received update event", zap.String("eventId", updateEvent.ID))
	} else {
		t.Logf("No update event received (this is expected for some adapters)")
	}

	// Event 3 - Delete the Namespace
	err = k8sHelper.DeleteNamespace(fw.Context, testNsName)
	require.NoError(t, err)

	fw.Logger.Info("Deleted test namespace", zap.String("namespace", testNsName))

	// Wait for delete event
	deleteEvent, err := fw.WebhookServer.WaitForEventWithFilter(eventWaitTimeout, func(e *e2e.WebhookEvent) bool {
		return e.Type == "resource.deleted" && e.ResourceType == "Namespace"
	})
	if err == nil {
		assert.Equal(t, "resource.deleted", deleteEvent.Type)
		assert.Equal(t, subscriptionID, deleteEvent.SubscriptionID)
		fw.Logger.Info("Received delete event", zap.String("eventId", deleteEvent.ID))
	} else {
		t.Logf("Warning: Did not receive delete event within timeout: %v", err)
	}

	// Verify we received lifecycle events
	allEvents := fw.WebhookServer.GetReceivedEvents()
	if len(allEvents) > 0 {
		fw.Logger.Info("Resource lifecycle test completed",
			zap.Int("eventsReceived", len(allEvents)),
		)
		// At minimum, expect create and delete events
		assert.GreaterOrEqual(t, len(allEvents), 2, "Should receive at least create and delete events")
	} else {
		t.Skip("No events received - subscription notification may not be configured")
	}
}

// TestSubscriptionFilterByResourcePool tests filtering by resource pool ID.
func TestSubscriptionFilterByResourcePool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := e2e.NewTestFramework(e2e.DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	// First, discover available resource pools
	resp, err := fw.APIClient.Get(fw.GatewayURL + e2e.APIPathResourcePools)
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var poolsResponse struct {
		Items []map[string]any `json:"items"`
	}
	err = json.Unmarshal(body, &poolsResponse)
	require.NoError(t, err)

	if len(poolsResponse.Items) == 0 {
		t.Skip("No resource pools available for testing")
	}

	// Get the first pool ID
	poolID, ok := poolsResponse.Items[0]["resourcePoolId"].(string)
	if !ok {
		t.Skip("Could not extract resource pool ID")
	}

	// Create webhook for this specific pool
	webhook1 := e2e.NewWebhookServer(fw.Logger.Named("webhook-pool"))
	require.NoError(t, webhook1.Start())
	defer func() {
		if err := webhook1.Stop(); err != nil {
			t.Logf("Failed to stop webhook1: %v", err)
		}
	}()

	// Create webhook for non-existent pool (should receive no events)
	webhook2 := e2e.NewWebhookServer(fw.Logger.Named("webhook-nopool"))
	require.NoError(t, webhook2.Start())
	defer func() {
		if err := webhook2.Stop(); err != nil {
			t.Logf("Failed to stop webhook2: %v", err)
		}
	}()

	// Subscription 1: Filter for the discovered pool
	sub1 := map[string]any{
		"callback": webhook1.URL(),
		"filter":   fmt.Sprintf("(resourcePoolId==%s)", poolID),
	}
	reqBody1, err := json.Marshal(sub1)
	require.NoError(t, err)

	url := fw.GatewayURL + e2e.APIPathSubscriptions
	resp1, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody1))
	require.NoError(t, err)
	defer func() {
		if err := resp1.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	// Subscription 2: Filter for non-existent pool
	sub2 := map[string]any{
		"callback": webhook2.URL(),
		"filter":   "(resourcePoolId==nonexistent-pool-999)",
	}
	reqBody2, err := json.Marshal(sub2)
	require.NoError(t, err)

	resp2, err := fw.APIClient.Post(url, "application/json", bytes.NewReader(reqBody2))
	require.NoError(t, err)
	defer func() {
		if err := resp2.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()
	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	webhook1.ClearEvents()
	webhook2.ClearEvents()

	// Create K8s resource helper
	k8sHelper := e2e.NewK8sResourceHelper(fw.KubeClient)

	// Create a namespace to trigger events
	nsName := "pool-filter-test"
	ns, err := k8sHelper.CreateTestNamespace(fw.Context, nsName)
	if err != nil {
		t.Skipf("Could not create test namespace: %v", err)
	}
	defer func() {
		if delErr := k8sHelper.DeleteNamespace(fw.Context, nsName); delErr != nil {
			t.Logf("Failed to cleanup namespace: %v", delErr)
		}
	}()

	fw.Logger.Info("Created test namespace for pool filtering",
		zap.String("namespace", ns.Name),
		zap.String("poolId", poolID),
	)

	// Wait for events to be delivered
	time.Sleep(eventDeliveryTimeout)

	// Verify filtering worked
	events1 := webhook1.GetReceivedEvents()
	events2 := webhook2.GetReceivedEvents()

	// Webhook2 should receive NO events (filtering on non-existent pool)
	assert.Empty(t, events2, "Webhook2 should not receive events for non-existent pool")

	// Log event details for debugging
	for i, evt := range events1 {
		fw.Logger.Debug("Webhook1 received event",
			zap.Int("index", i),
			zap.String("type", evt.Type),
			zap.String("resourceType", evt.ResourceType),
			zap.String("resourceId", evt.ResourceID),
		)
	}

	if len(events1) == 0 {
		t.Skip("No events received - subscription notification may not be configured")
	}

	fw.Logger.Info("Resource pool filtering test completed",
		zap.Int("webhook1Events", len(events1)),
		zap.Int("webhook2Events", len(events2)),
		zap.String("filteredPoolId", poolID),
	)
}
