// Package o2ims contains integration tests for O2-IMS backend plugins.
//
//go:build integration
// +build integration

package o2ims

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// TestSubscriptionWorkflow_CreateAndNotify tests the complete subscription workflow:
// 1. Create subscription
// 2. Trigger resource event
// 3. Receive webhook notification
func TestSubscriptionWorkflow_CreateAndNotify(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	// Setup Redis storage
	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer redisStore.Close()

	// Setup webhook server to receive notifications
	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	// Setup gateway server
	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	cfg := &config.Config{}
	srv := server.New(cfg)
	srv.SetAdapter(k8sAdapter)
	srv.SetStorage(redisStore)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Step 1: Create subscription
	t.Log("Step 1: Creating subscription...")
	subscriptionData := helpers.TestSubscription(webhookServer.URL())
	subBody, err := json.Marshal(subscriptionData)
	require.NoError(t, err)

	subResp, err := http.Post(
		ts.URL+"/o2ims-infrastructureInventory/v1/subscriptions",
		"application/json",
		bytes.NewReader(subBody),
	)
	require.NoError(t, err)
	defer subResp.Body.Close()

	assert.Equal(t, http.StatusCreated, subResp.StatusCode)

	var subscription map[string]interface{}
	err = json.NewDecoder(subResp.Body).Decode(&subscription)
	require.NoError(t, err)

	subscriptionID := subscription["subscriptionId"].(string)
	t.Logf("Created subscription: %s", subscriptionID)

	// Verify subscription was stored in Redis
	storedSub, err := redisStore.Get(ctx, subscriptionID)
	require.NoError(t, err)
	assert.Equal(t, subscriptionID, storedSub.ID)
	assert.Equal(t, webhookServer.URL(), storedSub.Callback)

	// Step 2: Create a resource pool (triggers event)
	t.Log("Step 2: Creating resource pool to trigger event...")
	poolData := helpers.TestResourcePool("subscription-test-pool")
	poolBody, err := json.Marshal(poolData)
	require.NoError(t, err)

	poolResp, err := http.Post(
		ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
		"application/json",
		bytes.NewReader(poolBody),
	)
	require.NoError(t, err)
	defer poolResp.Body.Close()

	assert.Equal(t, http.StatusCreated, poolResp.StatusCode)

	var pool map[string]interface{}
	json.NewDecoder(poolResp.Body).Decode(&pool)
	poolID := pool["resourcePoolId"].(string)

	// Step 3: Wait for webhook notification
	t.Log("Step 3: Waiting for webhook notification...")
	notification := webhookServer.WaitForNotification(5 * time.Second)

	// Verify webhook was received
	require.NotNil(t, notification, "Should receive webhook notification")
	assert.Equal(t, subscriptionID, notification.SubscriptionID)
	assert.Equal(t, "ResourcePoolCreated", notification.EventType)

	// Verify resource details in notification
	assert.NotNil(t, notification.Resource)
	assert.Equal(t, poolID, notification.Resource["resourcePoolId"])

	// Measure end-to-end latency
	latency := notification.ReceivedAt.Sub(notification.Timestamp)
	t.Logf("Webhook latency: %v", latency)
	assert.Less(t, latency, 2*time.Second, "Webhook should be delivered within 2s")
}

// TestSubscriptionWorkflow_WithFilters tests filtered subscriptions.
func TestSubscriptionWorkflow_WithFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:     env.Redis.Addr(),
		PoolSize: 10,
	})
	defer redisStore.Close()

	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	cfg := &config.Config{}
	srv := server.New(cfg)
	srv.SetAdapter(k8sAdapter)
	srv.SetStorage(redisStore)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create a resource pool first
	poolData := helpers.TestResourcePool("filter-test-pool")
	poolBody, _ := json.Marshal(poolData)
	poolResp, _ := http.Post(
		ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
		"application/json",
		bytes.NewReader(poolBody),
	)
	defer poolResp.Body.Close()

	var pool map[string]interface{}
	json.NewDecoder(poolResp.Body).Decode(&pool)
	poolID := pool["resourcePoolId"].(string)

	// Test 1: Subscription with pool filter (should match)
	t.Run("MatchingPoolFilter", func(t *testing.T) {
		webhookServer.Clear()

		// Create subscription with pool filter
		subData := helpers.TestSubscriptionWithFilter(webhookServer.URL(), poolID, "")
		subBody, _ := json.Marshal(subData)

		subResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/subscriptions",
			"application/json",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		defer subResp.Body.Close()

		var subscription map[string]interface{}
		json.NewDecoder(subResp.Body).Decode(&subscription)
		subscriptionID := subscription["subscriptionId"].(string)

		// Create resource in the matching pool
		resourceData := helpers.TestResource(poolID, "compute-node")
		resBody, _ := json.Marshal(resourceData)

		resResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resources",
			"application/json",
			bytes.NewReader(resBody),
		)
		require.NoError(t, err)
		defer resResp.Body.Close()

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

		subResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/subscriptions",
			"application/json",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		defer subResp.Body.Close()

		// Create resource in original pool (not matching subscription filter)
		resourceData := helpers.TestResource(poolID, "compute-node")
		resBody, _ := json.Marshal(resourceData)

		resResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resources",
			"application/json",
			bytes.NewReader(resBody),
		)
		require.NoError(t, err)
		defer resResp.Body.Close()

		// Should NOT receive notification
		notification := webhookServer.WaitForNotification(2 * time.Second)
		assert.Nil(t, notification, "Should not receive notification for non-matching pool")
	})
}

// TestSubscriptionWorkflow_MultipleSubscriptions tests multiple concurrent subscriptions.
func TestSubscriptionWorkflow_MultipleSubscriptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:     env.Redis.Addr(),
		PoolSize: 10,
	})
	defer redisStore.Close()

	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	cfg := &config.Config{}
	srv := server.New(cfg)
	srv.SetAdapter(k8sAdapter)
	srv.SetStorage(redisStore)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create multiple subscriptions
	numSubscriptions := 3
	subscriptionIDs := make([]string, numSubscriptions)

	for i := 0; i < numSubscriptions; i++ {
		subData := helpers.TestSubscription(webhookServer.URL())
		subBody, _ := json.Marshal(subData)

		subResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/subscriptions",
			"application/json",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		defer subResp.Body.Close()

		var subscription map[string]interface{}
		json.NewDecoder(subResp.Body).Decode(&subscription)
		subscriptionIDs[i] = subscription["subscriptionId"].(string)

		t.Logf("Created subscription %d: %s", i+1, subscriptionIDs[i])
	}

	// Trigger event
	poolData := helpers.TestResourcePool("multi-sub-pool")
	poolBody, _ := json.Marshal(poolData)

	poolResp, err := http.Post(
		ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
		"application/json",
		bytes.NewReader(poolBody),
	)
	require.NoError(t, err)
	defer poolResp.Body.Close()

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
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:     env.Redis.Addr(),
		PoolSize: 10,
	})
	defer redisStore.Close()

	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	cfg := &config.Config{}
	srv := server.New(cfg)
	srv.SetAdapter(k8sAdapter)
	srv.SetStorage(redisStore)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create subscription
	subData := helpers.TestSubscription(webhookServer.URL())
	subBody, _ := json.Marshal(subData)

	subResp, err := http.Post(
		ts.URL+"/o2ims-infrastructureInventory/v1/subscriptions",
		"application/json",
		bytes.NewReader(subBody),
	)
	require.NoError(t, err)
	defer subResp.Body.Close()

	var subscription map[string]interface{}
	json.NewDecoder(subResp.Body).Decode(&subscription)
	subscriptionID := subscription["subscriptionId"].(string)

	// Verify subscription exists in storage
	_, err = redisStore.Get(ctx, subscriptionID)
	require.NoError(t, err)

	// Delete subscription
	req, _ := http.NewRequest(
		http.MethodDelete,
		ts.URL+"/o2ims-infrastructureInventory/v1/subscriptions/"+subscriptionID,
		nil,
	)

	client := &http.Client{}
	delResp, err := client.Do(req)
	require.NoError(t, err)
	defer delResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)

	// Verify subscription was removed from storage
	_, err = redisStore.Get(ctx, subscriptionID)
	assert.Error(t, err)
	assert.Equal(t, storage.ErrSubscriptionNotFound, err)

	// Trigger event - should NOT receive notification
	poolData := helpers.TestResourcePool("post-delete-pool")
	poolBody, _ := json.Marshal(poolData)

	poolResp, err := http.Post(
		ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
		"application/json",
		bytes.NewReader(poolBody),
	)
	require.NoError(t, err)
	defer poolResp.Body.Close()

	// Should NOT receive notification after deletion
	notification := webhookServer.WaitForNotification(2 * time.Second)
	assert.Nil(t, notification, "Should not receive notification after subscription deletion")
}
