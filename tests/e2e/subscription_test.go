// Package e2e provides end-to-end tests.
//
//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestSubscriptionWorkflow tests the complete subscription lifecycle.
func TestSubscriptionWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := NewTestFramework(DefaultOptions())
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

		url := fw.GatewayURL + "/o2ims/v1/subscriptions"
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
		url := fw.GatewayURL + "/o2ims/v1/subscriptions"
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
		url := fmt.Sprintf("%s/o2ims/v1/subscriptions/%s", fw.GatewayURL, subscriptionID)
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
		url := fmt.Sprintf("%s/o2ims/v1/subscriptions/%s", fw.GatewayURL, subscriptionID)
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

	fw, err := NewTestFramework(DefaultOptions())
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

	url := fw.GatewayURL + "/o2ims/v1/subscriptions"
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
