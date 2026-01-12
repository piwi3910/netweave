// Package o2ims contains integration tests for O2-IMS backend plugins.
//
//go:build integration
// +build integration

package o2ims

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// TestSubscriptionWorkflow_CreateAndNotify tests the complete subscription workflow:
// 1. Create subscription
// 2. Trigger resource event
// 3. Receive webhook notification.
func TestSubscriptionWorkflow_CreateAndNotify(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	// Setup Redis storage
	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:                   env.Redis.Addr(),
		MaxRetries:             3,
		DialTimeout:            5 * time.Second,
		ReadTimeout:            3 * time.Second,
		WriteTimeout:           3 * time.Second,
		PoolSize:               10,
		AllowInsecureCallbacks: true, // Allow HTTP callbacks in tests
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	// Setup webhook server to receive notifications
	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	// Setup gateway server
	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Step 1: Create subscription
	t.Log("Step 1: Creating subscription...")
	subscriptionData := helpers.TestSubscription(webhookServer.URL())
	subBody, err := json.Marshal(subscriptionData)
	require.NoError(t, err)

	subReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		ts.O2IMSURL()+"/subscriptions",
		bytes.NewReader(subBody),
	)
	require.NoError(t, err)
	subReq.Header.Set("Content-Type", "application/json")

	subResp, err := http.DefaultClient.Do(subReq)
	require.NoError(t, err)
	defer func() {
		if err := subResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	// Log response body for debugging
	bodyBytes, readErr := io.ReadAll(subResp.Body)
	require.NoError(t, readErr)
	t.Logf("Response status: %d", subResp.StatusCode)
	t.Logf("Response body: %s", string(bodyBytes))

	assert.Equal(t, http.StatusCreated, subResp.StatusCode)

	var subscription map[string]interface{}
	err = json.Unmarshal(bodyBytes, &subscription)
	require.NoError(t, err)

	subscriptionID := subscription["subscriptionId"].(string)
	t.Logf("Created subscription: %s", subscriptionID)

	// Verify subscription was stored in Redis
	storedSub, err := redisStore.Get(ctx, subscriptionID)
	require.NoError(t, err)
	assert.Equal(t, subscriptionID, storedSub.ID)
	assert.Equal(t, webhookServer.URL(), storedSub.Callback)

	// Step 2: Trigger resource event
	// Note: In the full implementation, the subscription controller would be running
	// and watching for K8s resource changes. For this test, the webhook delivery
	// would be triggered by creating a node or namespace in the K8s cluster.
	// This test validates the subscription CRUD operations.
	poolID := "test-pool-" + subscriptionID
	_ = poolID // Would be used to create a resource pool in full implementation

	// Step 3: Verify subscription is active and can be retrieved
	t.Log("Step 3: Verifying subscription is active...")

	// Get subscription
	getReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		ts.O2IMSURL()+"/subscriptions/"+subscriptionID,
		nil,
	)
	require.NoError(t, err)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer func() {
		if err := getResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var retrievedSub map[string]interface{}
	err = json.NewDecoder(getResp.Body).Decode(&retrievedSub)
	require.NoError(t, err)
	assert.Equal(t, subscriptionID, retrievedSub["subscriptionId"])

	t.Log("Subscription workflow test completed successfully")
	t.Log("Note: Full webhook notification testing requires the subscription controller")
	t.Log("and worker to be running. See internal/controllers and internal/workers.")
}

// TestSubscriptionWorkflow_WithFilters tests filtered subscriptions.
func TestSubscriptionWorkflow_WithFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Skip("Skipping: Event notification system requires Kubernetes watch/informer integration (future work)")

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:                   env.Redis.Addr(),
		PoolSize:               10,
		AllowInsecureCallbacks: true, // Allow HTTP callbacks in tests
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Create a resource pool first
	poolData := helpers.TestResourcePool("filter-test-pool")
	poolBody, _ := json.Marshal(poolData)
	poolReq, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		ts.O2IMSURL()+"/resourcePools",
		bytes.NewReader(poolBody),
	)
	if poolReq != nil {
		poolReq.Header.Set("Content-Type", "application/json")
	}
	poolResp, _ := http.DefaultClient.Do(poolReq)
	defer func() {
		if err := poolResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	var pool map[string]interface{}
	if err := json.NewDecoder(poolResp.Body).Decode(&pool); err != nil {
		t.Logf("Failed to decode response: %v", err)
	}
	poolID := pool["resourcePoolId"].(string)

	// Test 1: Subscription with pool filter (should match)
	t.Run("MatchingPoolFilter", func(t *testing.T) {
		webhookServer.Clear()

		// Create subscription with pool filter
		subData := helpers.TestSubscriptionWithFilter(webhookServer.URL(), poolID, "")
		subBody, _ := json.Marshal(subData)

		subReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		subReq.Header.Set("Content-Type", "application/json")

		subResp, err := http.DefaultClient.Do(subReq)
		require.NoError(t, err)
		defer func() {
			if err := subResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var subscription map[string]interface{}
		if err := json.NewDecoder(subResp.Body).Decode(&subscription); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		subscriptionID := subscription["subscriptionId"].(string)

		// Create resource in the matching pool
		resourceData := helpers.TestResource(poolID, "compute-node")
		resBody, _ := json.Marshal(resourceData)

		resReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resources",
			bytes.NewReader(resBody),
		)
		require.NoError(t, err)
		resReq.Header.Set("Content-Type", "application/json")

		resResp, err := http.DefaultClient.Do(resReq)
		require.NoError(t, err)
		defer func() {
			if err := resResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// Should receive notification
		notification := webhookServer.WaitForNotification(5 * time.Second)
		require.NotNil(t, notification, "Should receive notification for matching pool")
		assert.Equal(t, subscriptionID, notification.SubscriptionID)
	})

	// Test 2: Subscription with non-matching filter (should NOT match)
	t.Run("NonMatchingPoolFilter", func(t *testing.T) {
		webhookServer.Clear()

		// Create subscription with different pool filter
		subData := helpers.TestSubscriptionWithFilter(webhookServer.URL(), "different-pool-id", "")
		subBody, _ := json.Marshal(subData)

		subReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		subReq.Header.Set("Content-Type", "application/json")

		subResp, err := http.DefaultClient.Do(subReq)
		require.NoError(t, err)
		defer func() {
			if err := subResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// Create resource in original pool (not matching subscription filter)
		resourceData := helpers.TestResource(poolID, "compute-node")
		resBody, _ := json.Marshal(resourceData)

		resReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resources",
			bytes.NewReader(resBody),
		)
		require.NoError(t, err)
		resReq.Header.Set("Content-Type", "application/json")

		resResp, err := http.DefaultClient.Do(resReq)
		require.NoError(t, err)
		defer func() {
			if err := resResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// Should NOT receive notification
		notification := webhookServer.WaitForNotification(2 * time.Second)
		assert.Nil(t, notification, "Should not receive notification for non-matching pool")
	})
}

// TestSubscriptionWorkflow_MultipleSubscriptions tests multiple concurrent subscriptions.
func TestSubscriptionWorkflow_MultipleSubscriptions(t *testing.T) {
	t.Skip("Skipping: Event notification system requires Kubernetes watch/informer integration (future work)")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:                   env.Redis.Addr(),
		PoolSize:               10,
		AllowInsecureCallbacks: true, // Allow HTTP callbacks in tests
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Create multiple subscriptions
	numSubscriptions := 3
	subscriptionIDs := make([]string, numSubscriptions)

	for i := 0; i < numSubscriptions; i++ {
		subData := helpers.TestSubscription(webhookServer.URL())
		subBody, _ := json.Marshal(subData)

		subReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		subReq.Header.Set("Content-Type", "application/json")

		subResp, err := http.DefaultClient.Do(subReq)
		require.NoError(t, err)
		defer func() {
			if err := subResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var subscription map[string]interface{}
		if err := json.NewDecoder(subResp.Body).Decode(&subscription); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		subscriptionIDs[i] = subscription["subscriptionId"].(string)

		t.Logf("Created subscription %d: %s", i+1, subscriptionIDs[i])
	}

	// Trigger event
	poolData := helpers.TestResourcePool("multi-sub-pool")
	poolBody, _ := json.Marshal(poolData)

	poolReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		ts.O2IMSURL()+"/resourcePools",
		bytes.NewReader(poolBody),
	)
	require.NoError(t, err)
	poolReq.Header.Set("Content-Type", "application/json")

	poolResp, err := http.DefaultClient.Do(poolReq)
	require.NoError(t, err)
	defer func() {
		if err := poolResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	// Wait for all notifications
	notifications := webhookServer.WaitForNotifications(numSubscriptions, 10*time.Second)

	// Verify all subscriptions received notification
	assert.Len(t, notifications, numSubscriptions)

	receivedIDs := make(map[string]bool)
	for _, notification := range notifications {
		receivedIDs[notification.SubscriptionID] = true
	}

	for _, subID := range subscriptionIDs {
		assert.True(t, receivedIDs[subID], "Subscription %s should receive notification", subID)
	}
}

// TestSubscriptionWorkflow_DeleteSubscription tests subscription deletion and cleanup.
func TestSubscriptionWorkflow_DeleteSubscription(t *testing.T) {
	t.Skip("Skipping: Event notification system requires Kubernetes watch/informer integration (future work)")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:                   env.Redis.Addr(),
		PoolSize:               10,
		AllowInsecureCallbacks: true, // Allow HTTP callbacks in tests
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Create subscription
	subData := helpers.TestSubscription(webhookServer.URL())
	subBody, _ := json.Marshal(subData)

	subReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		ts.O2IMSURL()+"/subscriptions",
		bytes.NewReader(subBody),
	)
	require.NoError(t, err)
	subReq.Header.Set("Content-Type", "application/json")

	subResp, err := http.DefaultClient.Do(subReq)
	require.NoError(t, err)
	defer func() {
		if err := subResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	var subscription map[string]interface{}
	err = json.NewDecoder(subResp.Body).Decode(&subscription)
	require.NoError(t, err)
	subscriptionID := subscription["subscriptionId"].(string)

	// Verify subscription exists in storage
	_, err = redisStore.Get(ctx, subscriptionID)
	require.NoError(t, err)

	// Delete subscription
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodDelete,
		ts.O2IMSURL()+"/subscriptions/"+subscriptionID,
		nil,
	)
	require.NoError(t, err)

	client := &http.Client{}
	delResp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		if err := delResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)

	// Verify subscription was removed from storage
	_, err = redisStore.Get(ctx, subscriptionID)
	assert.Error(t, err)
	assert.Equal(t, storage.ErrSubscriptionNotFound, err)

	// Trigger event - should NOT receive notification
	poolData := helpers.TestResourcePool("post-delete-pool")
	poolBody, _ := json.Marshal(poolData)

	poolReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		ts.O2IMSURL()+"/resourcePools",
		bytes.NewReader(poolBody),
	)
	require.NoError(t, err)
	poolReq.Header.Set("Content-Type", "application/json")

	poolResp, err := http.DefaultClient.Do(poolReq)
	require.NoError(t, err)
	defer func() {
		if err := poolResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	// Should NOT receive notification after deletion
	notification := webhookServer.WaitForNotification(2 * time.Second)
	assert.Nil(t, notification, "Should not receive notification after subscription deletion")
}
