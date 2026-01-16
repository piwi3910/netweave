// Package o2ims contains integration tests for O2-IMS multi-tenancy features.
//
//go:build integration
// +build integration

package o2ims_test

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
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// TestMultiTenancy_TenantIsolation verifies that tenants can only access their own resources.
func TestMultiTenancy_TenantIsolation(t *testing.T) {
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
		AllowInsecureCallbacks: true,
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	// Setup auth store for multi-tenancy
	authStore := auth.NewRedisStore(&auth.RedisConfig{
		Addr:        env.Redis.Addr(),
		MaxRetries:  3,
		DialTimeout: 5 * time.Second,
		ReadTimeout: 3 * time.Second,
		PoolSize:    10,
	})
	defer func() {
		if err := authStore.Close(); err != nil {
			t.Logf("Failed to close auth store: %v", err)
		}
	}()

	// Create two test tenants
	tenant1 := &auth.Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant 1",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions:     10,
			MaxResourcePools:     5,
			MaxDeployments:       20,
			MaxUsers:             50,
			MaxRequestsPerMinute: 1000,
		},
	}
	require.NoError(t, authStore.CreateTenant(ctx, tenant1))

	tenant2 := &auth.Tenant{
		ID:     "tenant-2",
		Name:   "Test Tenant 2",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions:     10,
			MaxResourcePools:     5,
			MaxDeployments:       20,
			MaxUsers:             50,
			MaxRequestsPerMinute: 1000,
		},
	}
	require.NoError(t, authStore.CreateTenant(ctx, tenant2))

	// Setup gateway server with auth
	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Setup webhook server for subscriptions
	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	// Test: Tenant 1 creates a subscription
	t.Run("tenant1_creates_subscription", func(t *testing.T) {
		subscriptionData := helpers.TestSubscription(webhookServer.URL())
		subBody, err := json.Marshal(subscriptionData)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Add tenant1 context (in real deployment, this comes from mTLS cert)
		// For testing, we simulate authenticated request
		req.Header.Set("X-Tenant-ID", "tenant-1")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdSub map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&createdSub))
		assert.NotEmpty(t, createdSub["subscriptionId"])
	})

	// Test: Tenant 2 cannot access tenant 1's subscription
	t.Run("tenant2_cannot_access_tenant1_subscription", func(t *testing.T) {
		// First, get tenant1's subscriptions to find subscription ID
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/subscriptions",
			nil,
		)
		require.NoError(t, err)
		req.Header.Set("X-Tenant-ID", "tenant-1")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var listResponse map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &listResponse))

		subs, ok := listResponse["subscriptions"].([]interface{})
		require.True(t, ok)
		require.NotEmpty(t, subs, "tenant1 should have subscriptions")

		sub1 := subs[0].(map[string]interface{})
		subID := sub1["subscriptionId"].(string)

		// Now try to access this subscription as tenant2
		req2, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/subscriptions/"+subID,
			nil,
		)
		require.NoError(t, err)
		req2.Header.Set("X-Tenant-ID", "tenant-2")

		resp2, err := client.Do(req2)
		require.NoError(t, err)
		defer resp2.Body.Close()

		// Should get 404 (resource not found from tenant2's perspective)
		assert.Equal(t, http.StatusNotFound, resp2.StatusCode)
	})

	// Test: Tenant 2 can only see their own subscriptions
	t.Run("tenant2_only_sees_own_subscriptions", func(t *testing.T) {
		// Create subscription for tenant2
		subscriptionData := helpers.TestSubscription(webhookServer.URL())
		subBody, err := json.Marshal(subscriptionData)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", "tenant-2")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// List subscriptions as tenant2
		req2, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/subscriptions",
			nil,
		)
		require.NoError(t, err)
		req2.Header.Set("X-Tenant-ID", "tenant-2")

		client2 := &http.Client{}
		resp2, err := client2.Do(req2)
		require.NoError(t, err)
		defer resp2.Body.Close()

		body, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)

		var listResponse map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &listResponse))

		subs, ok := listResponse["subscriptions"].([]interface{})
		require.True(t, ok)

		// Tenant2 should only see their own subscription, not tenant1's
		assert.Equal(t, 1, len(subs), "tenant2 should only see 1 subscription (their own)")
	})
}

// TestMultiTenancy_QuotaEnforcement verifies that tenant quotas are enforced.
func TestMultiTenancy_QuotaEnforcement(t *testing.T) {
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
		AllowInsecureCallbacks: true,
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	// Setup auth store
	authStore := auth.NewRedisStore(&auth.RedisConfig{
		Addr:        env.Redis.Addr(),
		MaxRetries:  3,
		DialTimeout: 5 * time.Second,
		ReadTimeout: 3 * time.Second,
		PoolSize:    10,
	})
	defer func() {
		if err := authStore.Close(); err != nil {
			t.Logf("Failed to close auth store: %v", err)
		}
	}()

	// Create tenant with low subscription quota
	tenant := &auth.Tenant{
		ID:     "tenant-quota-test",
		Name:   "Quota Test Tenant",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions:     2, // Low quota for testing
			MaxResourcePools:     5,
			MaxDeployments:       20,
			MaxUsers:             50,
			MaxRequestsPerMinute: 1000,
		},
	}
	require.NoError(t, authStore.CreateTenant(ctx, tenant))

	// Setup gateway server
	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Setup webhook server
	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	// Test: Create subscriptions up to quota
	var createdSubIDs []string
	for i := 0; i < 2; i++ {
		t.Run("create_subscription_within_quota", func(t *testing.T) {
			subscriptionData := helpers.TestSubscription(webhookServer.URL())
			subBody, err := json.Marshal(subscriptionData)
			require.NoError(t, err)

			req, err := http.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				ts.O2IMSURL()+"/subscriptions",
				bytes.NewReader(subBody),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-ID", "tenant-quota-test")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var createdSub map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&createdSub))
			subID := createdSub["subscriptionId"].(string)
			createdSubIDs = append(createdSubIDs, subID)
		})
	}

	// Test: Attempt to exceed quota
	t.Run("quota_exceeded", func(t *testing.T) {
		subscriptionData := helpers.TestSubscription(webhookServer.URL())
		subBody, err := json.Marshal(subscriptionData)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", "tenant-quota-test")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should get 429 Too Many Requests
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var errResponse map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &errResponse))
		assert.Equal(t, "QuotaExceeded", errResponse["error"])
	})

	// Test: Delete subscription and quota is freed
	t.Run("quota_freed_after_delete", func(t *testing.T) {
		// Delete one subscription
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			ts.O2IMSURL()+"/subscriptions/"+createdSubIDs[0],
			nil,
		)
		require.NoError(t, err)
		req.Header.Set("X-Tenant-ID", "tenant-quota-test")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		// Now create should succeed
		subscriptionData := helpers.TestSubscription(webhookServer.URL())
		subBody, err := json.Marshal(subscriptionData)
		require.NoError(t, err)

		req2, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Tenant-ID", "tenant-quota-test")

		client3 := &http.Client{}
		resp2, err := client3.Do(req2)
		require.NoError(t, err)
		defer resp2.Body.Close()

		assert.Equal(t, http.StatusCreated, resp2.StatusCode)
	})
}

// TestMultiTenancy_AuditLogging verifies that audit events include tenant context.
func TestMultiTenancy_AuditLogging(t *testing.T) {
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
		AllowInsecureCallbacks: true,
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	// Setup auth store
	authStore := auth.NewRedisStore(&auth.RedisConfig{
		Addr:        env.Redis.Addr(),
		MaxRetries:  3,
		DialTimeout: 5 * time.Second,
		ReadTimeout: 3 * time.Second,
		PoolSize:    10,
	})
	defer func() {
		if err := authStore.Close(); err != nil {
			t.Logf("Failed to close auth store: %v", err)
		}
	}()

	// Create test tenant
	tenant := &auth.Tenant{
		ID:     "tenant-audit-test",
		Name:   "Audit Test Tenant",
		Status: auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions:     10,
			MaxResourcePools:     5,
			MaxDeployments:       20,
			MaxUsers:             50,
			MaxRequestsPerMinute: 1000,
		},
	}
	require.NoError(t, authStore.CreateTenant(ctx, tenant))

	// Setup gateway server
	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Setup webhook server
	webhookServer := helpers.NewWebhookServer(t)
	defer webhookServer.Close()

	// Test: Create subscription and verify audit event
	t.Run("subscription_creation_audit", func(t *testing.T) {
		subscriptionData := helpers.TestSubscription(webhookServer.URL())
		subBody, err := json.Marshal(subscriptionData)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			ts.O2IMSURL()+"/subscriptions",
			bytes.NewReader(subBody),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", "tenant-audit-test")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdSub map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&createdSub))
		subID := createdSub["subscriptionId"].(string)

		// Wait briefly for audit event to be logged
		time.Sleep(100 * time.Millisecond)

		// Query audit events for this tenant
		events, err := authStore.ListEvents(ctx, "tenant-audit-test", 10, 0)
		require.NoError(t, err)
		require.NotEmpty(t, events, "audit events should be logged")

		// Find the subscription creation event
		var foundEvent *auth.AuditEvent
		for _, event := range events {
			if event.ResourceType == "subscription" &&
				event.ResourceID == subID &&
				event.Type == auth.AuditEventResourceCreated {
				foundEvent = event
				break
			}
		}

		require.NotNil(t, foundEvent, "subscription creation audit event should exist")
		assert.Equal(t, "tenant-audit-test", foundEvent.TenantID)
		assert.Equal(t, "subscription", foundEvent.ResourceType)
		assert.Equal(t, subID, foundEvent.ResourceID)
		assert.Equal(t, "subscription_created", foundEvent.Action)
	})
}
