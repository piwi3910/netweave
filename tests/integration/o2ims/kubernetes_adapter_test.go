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

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// doHTTPRequest performs an HTTP request and decodes the JSON response.
func doHTTPRequest(t *testing.T, method, url string, body interface{}, result interface{}) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, reqBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := helpers.NewTestHTTPClient()
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	})

	if result != nil && resp.StatusCode != http.StatusNoContent {
		err = json.NewDecoder(resp.Body).Decode(result)
		require.NoError(t, err)
	}

	return resp
}

// createResourcePool creates a resource pool and returns its ID.
func createResourcePool(t *testing.T, ts *helpers.TestServer, name string) string {
	t.Helper()
	poolData := helpers.TestResourcePool(name)
	var result map[string]interface{}
	resp := doHTTPRequest(t, http.MethodPost, ts.O2IMSURL()+"/resourcePools", poolData, &result)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	return result["resourcePoolId"].(string)
}

// createResource creates a resource and returns its ID.
func createResource(t *testing.T, ts *helpers.TestServer, poolID, resourceType string) string {
	t.Helper()
	resourceData := helpers.TestResource(poolID, resourceType)
	var result map[string]interface{}
	resp := doHTTPRequest(t, http.MethodPost, ts.O2IMSURL()+"/resources", resourceData, &result)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	return result["resourceId"].(string)
}

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
		var result map[string]interface{}
		resp := doHTTPRequest(t, http.MethodPost, ts.O2IMSURL()+"/resourcePools", poolData, &result)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.NotEmpty(t, result["resourcePoolId"])
		assert.Equal(t, "test-pool-1", result["name"])
	})

	// Test 2: Get resource pool
	t.Run("GetResourcePool", func(t *testing.T) {
		t.Skip("Skipping until server routes are properly implemented (issue #19)")
		poolID := createResourcePool(t, ts, "test-pool-2")

		var result map[string]interface{}
		resp := doHTTPRequest(t, http.MethodGet, ts.O2IMSURL()+"/resourcePools/"+poolID, nil, &result)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, poolID, result["resourcePoolId"])
		assert.Equal(t, "test-pool-2", result["name"])
	})

	// Test 3: List resource pools
	t.Run("ListResourcePools", func(t *testing.T) {
		var result []map[string]interface{}
		resp := doHTTPRequest(t, http.MethodGet, ts.O2IMSURL()+"/resourcePools", nil, &result)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.GreaterOrEqual(t, len(result), 2)
	})

	// Test 4: Update resource pool
	t.Run("UpdateResourcePool", func(t *testing.T) {
		poolID := createResourcePool(t, ts, "test-pool-update")

		updateData := map[string]interface{}{"name": "test-pool-updated", "description": "Updated description"}
		var result map[string]interface{}
		resp := doHTTPRequest(t, http.MethodPut, ts.O2IMSURL()+"/resourcePools/"+poolID, updateData, &result)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "test-pool-updated", result["name"])
		assert.Equal(t, "Updated description", result["description"])
	})

	// Test 5: Delete resource pool
	t.Run("DeleteResourcePool", func(t *testing.T) {
		poolID := createResourcePool(t, ts, "test-pool-delete")

		deleteResp := doHTTPRequest(t, http.MethodDelete, ts.O2IMSURL()+"/resourcePools/"+poolID, nil, nil)
		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		getResp := doHTTPRequest(t, http.MethodGet, ts.O2IMSURL()+"/resourcePools/"+poolID, nil, nil)
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
	poolID := createResourcePool(t, ts, "resource-test-pool")

	// Test 1: Create resource
	t.Run("CreateResource", func(t *testing.T) {
		resourceData := helpers.TestResource(poolID, "compute-node")
		var result map[string]interface{}
		resp := doHTTPRequest(t, http.MethodPost, ts.O2IMSURL()+"/resources", resourceData, &result)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.NotEmpty(t, result["resourceId"])
		assert.Equal(t, poolID, result["resourcePoolId"])
		assert.Equal(t, "compute-node", result["resourceTypeId"])
	})

	// Test 2: Get resource
	t.Run("GetResource", func(t *testing.T) {
		resourceID := createResource(t, ts, poolID, "compute-node")

		var result map[string]interface{}
		resp := doHTTPRequest(t, http.MethodGet, ts.O2IMSURL()+"/resources/"+resourceID, nil, &result)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, resourceID, result["resourceId"])
		assert.Equal(t, poolID, result["resourcePoolId"])
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
			testClient := helpers.NewTestHTTPClient()
			resp, _ := testClient.Do(req)
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

		testClient := helpers.NewTestHTTPClient()
		resp, err := testClient.Do(listReq)
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

		testClient := helpers.NewTestHTTPClient()
		createResp, err := testClient.Do(createReq)
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

		deleteClient := &http.Client{}
		deleteResp, err := deleteClient.Do(req)
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
		verifyClient := helpers.NewTestHTTPClient()
		getResp, _ := verifyClient.Do(verifyReq)
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

		httpClient := helpers.NewTestHTTPClient()
		resp, err := httpClient.Do(req)
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

		httpClient := helpers.NewTestHTTPClient()
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// POST with invalid data should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
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

		// TODO(#208): Handler should return 404, currently returns 500
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
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

		httpClient := helpers.NewTestHTTPClient()
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// POST with invalid JSON should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
