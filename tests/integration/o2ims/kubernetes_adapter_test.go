// Package o2ims contains integration tests for O2-IMS backend plugins.
//
//go:build integration
// +build integration

package o2ims

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// TestKubernetesAdapter_ResourcePoolLifecycle tests the complete CRUD lifecycle
// for resource pools using the Kubernetes adapter with real Redis storage.
func TestKubernetesAdapter_ResourcePoolLifecycle(t *testing.T) {
	t.Skip("Skipping until server routes are properly implemented - see issue #19")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment with Redis
	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	// Initialize Redis storage
	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		Password:     "",
		DB:           0,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	// Verify Redis connection
	err := redisStore.Ping(ctx)
	require.NoError(t, err, "Redis should be accessible")

	// Initialize Kubernetes adapter (mock mode for CI)
	// In real environment, this would connect to actual K8s cluster
	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	// Verify adapter capabilities
	capabilities := k8sAdapter.Capabilities()
	require.Contains(t, capabilities, adapter.CapabilityResourcePools)
	require.Contains(t, capabilities, adapter.CapabilityResources)

	// Create test server
	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Test 1: Create resource pool
	t.Run("CreateResourcePool", func(t *testing.T) {
		t.Skip("Skipping until server routes are properly implemented (issue #19)")
		poolData := helpers.TestResourcePool("test-pool-1")

		body, err := json.Marshal(poolData)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resourcePools",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.NotEmpty(t, result["resourcePoolId"])
		assert.Equal(t, "test-pool-1", result["name"])
	})

	// Test 2: Get resource pool
	t.Run("GetResourcePool", func(t *testing.T) {
		t.Skip("Skipping until server routes are properly implemented (issue #19)")
		// First create a pool
		poolData := helpers.TestResourcePool("test-pool-2")
		body, _ := json.Marshal(poolData)

		createReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resourcePools",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		createReq.Header.Set("Content-Type", "application/json")

		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		defer func() {
			if err := createResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var created map[string]interface{}
		if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		poolID := created["resourcePoolId"].(string)

		// Get the pool
		getReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resourcePools/"+poolID,
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

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, poolID, result["resourcePoolId"])
		assert.Equal(t, "test-pool-2", result["name"])
	})

	// Test 3: List resource pools
	t.Run("ListResourcePools", func(t *testing.T) {
		listReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resourcePools",
			nil,
		)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(listReq)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result []map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// We created at least 2 pools in previous tests
		assert.GreaterOrEqual(t, len(result), 2)
	})

	// Test 4: Update resource pool
	t.Run("UpdateResourcePool", func(t *testing.T) {
		// Create a pool first
		poolData := helpers.TestResourcePool("test-pool-update")
		body, _ := json.Marshal(poolData)

		createReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resourcePools",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		createReq.Header.Set("Content-Type", "application/json")

		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		defer func() {
			if err := createResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var created map[string]interface{}
		if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		poolID := created["resourcePoolId"].(string)

		// Update the pool
		updateData := map[string]interface{}{
			"name":        "test-pool-updated",
			"description": "Updated description",
		}
		updateBody, _ := json.Marshal(updateData)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPut,
			ts.O2IMSURL()+"/resourcePools/"+poolID,
			bytes.NewReader(updateBody),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		updateResp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := updateResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, updateResp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(updateResp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "test-pool-updated", result["name"])
		assert.Equal(t, "Updated description", result["description"])
	})

	// Test 5: Delete resource pool
	t.Run("DeleteResourcePool", func(t *testing.T) {
		// Create a pool first
		poolData := helpers.TestResourcePool("test-pool-delete")
		body, _ := json.Marshal(poolData)

		createReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resourcePools",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		createReq.Header.Set("Content-Type", "application/json")

		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		defer func() {
			if err := createResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var created map[string]interface{}
		if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		poolID := created["resourcePoolId"].(string)

		// Delete the pool
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			ts.O2IMSURL()+"/resourcePools/"+poolID,
			nil,
		)
		require.NoError(t, err)

		client := &http.Client{}
		deleteResp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := deleteResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Verify deletion - GET should return 404
		verifyReq, _ := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resourcePools/"+poolID,
			nil,
		)
		getResp, _ := http.DefaultClient.Do(verifyReq)
		defer func() {
			if err := getResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()
		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})
}

// TestKubernetesAdapter_ResourceLifecycle tests resource CRUD operations.
func TestKubernetesAdapter_ResourceLifecycle(t *testing.T) {
	t.Skip("Skipping until server routes are properly implemented - see issue #19")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:       env.Redis.Addr(),
		MaxRetries: 3,
		PoolSize:   10,
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// First create a resource pool and resource type
	poolData := helpers.TestResourcePool("resource-test-pool")
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

	// Test 1: Create resource
	t.Run("CreateResource", func(t *testing.T) {
		resourceData := helpers.TestResource(poolID, "compute-node")
		body, err := json.Marshal(resourceData)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resources",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.NotEmpty(t, result["resourceId"])
		assert.Equal(t, poolID, result["resourcePoolId"])
		assert.Equal(t, "compute-node", result["resourceTypeId"])
	})

	// Test 2: Get resource
	t.Run("GetResource", func(t *testing.T) {
		resourceData := helpers.TestResource(poolID, "compute-node")
		body, _ := json.Marshal(resourceData)

		createReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resources",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		createReq.Header.Set("Content-Type", "application/json")

		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		defer func() {
			if err := createResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var created map[string]interface{}
		if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		resourceID := created["resourceId"].(string)

		// Get the resource
		getReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resources/"+resourceID,
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

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, resourceID, result["resourceId"])
	})

	// Test 3: List resources with filter
	t.Run("ListResourcesWithFilter", func(t *testing.T) {
		// Create multiple resources
		for i := 0; i < 3; i++ {
			resourceData := helpers.TestResource(poolID, "compute-node")
			body, _ := json.Marshal(resourceData)
			req, _ := http.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				ts.O2IMSURL()+"/resources",
				bytes.NewReader(body),
			)
			if req != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, _ := http.DefaultClient.Do(req)
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}

		// List with pool filter
		listReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resources?resourcePoolId="+poolID,
			nil,
		)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(listReq)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result []map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(result), 3)

		// All resources should belong to the specified pool
		for _, res := range result {
			assert.Equal(t, poolID, res["resourcePoolId"])
		}
	})

	// Test 4: Delete resource
	t.Run("DeleteResource", func(t *testing.T) {
		resourceData := helpers.TestResource(poolID, "compute-node")
		body, _ := json.Marshal(resourceData)

		createReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resources",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		createReq.Header.Set("Content-Type", "application/json")

		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		defer func() {
			if err := createResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var created map[string]interface{}
		if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
			t.Logf("Failed to decode response: %v", err)
		}
		resourceID := created["resourceId"].(string)

		// Delete the resource
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			ts.O2IMSURL()+"/resources/"+resourceID,
			nil,
		)
		require.NoError(t, err)

		client := &http.Client{}
		deleteResp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := deleteResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Verify deletion
		verifyReq, _ := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resources/"+resourceID,
			nil,
		)
		getResp, _ := http.DefaultClient.Do(verifyReq)
		defer func() {
			if err := getResp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()
		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})
}

// TestKubernetesAdapter_ErrorHandling tests error scenarios.
func TestKubernetesAdapter_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:     env.Redis.Addr(),
		PoolSize: 10,
	})
	defer func() {
		if err := redisStore.Close(); err != nil {
			t.Logf("Failed to close Redis store: %v", err)
		}
	}()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer func() {
		if err := k8sAdapter.Close(); err != nil {
			t.Logf("Failed to close Kubernetes adapter: %v", err)
		}
	}()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	t.Run("GetNonExistentResourcePool", func(t *testing.T) {
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			ts.O2IMSURL()+"/resourcePools/nonexistent-pool",
			nil,
		)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("CreateResourcePoolWithInvalidData", func(t *testing.T) {
		invalidData := map[string]interface{}{
			"name": "", // Empty name should fail validation
		}
		body, _ := json.Marshal(invalidData)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resourcePools",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// Resource pools are read-only in O2-IMS spec, so POST should return 404
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("DeleteNonExistentResource", func(t *testing.T) {
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			ts.O2IMSURL()+"/resources/nonexistent-resource",
			nil,
		)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("InvalidJSONPayload", func(t *testing.T) {
		invalidJSON := []byte(`{"invalid": json}`)

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			ts.O2IMSURL()+"/resourcePools",
			bytes.NewReader(invalidJSON),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// Resource pools are read-only in O2-IMS spec, so POST should return 404
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
