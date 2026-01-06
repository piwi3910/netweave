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

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// TestKubernetesAdapter_ResourcePoolLifecycle tests the complete CRUD lifecycle
// for resource pools using the Kubernetes adapter with real Redis storage.
func TestKubernetesAdapter_ResourcePoolLifecycle(t *testing.T) {
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
	defer redisStore.Close()

	// Verify Redis connection
	err := redisStore.Ping(ctx)
	require.NoError(t, err, "Redis should be accessible")

	// Initialize Kubernetes adapter (mock mode for CI)
	// In real environment, this would connect to actual K8s cluster
	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	// Verify adapter capabilities
	capabilities := k8sAdapter.Capabilities()
	require.Contains(t, capabilities, adapter.CapabilityResourcePools)
	require.Contains(t, capabilities, adapter.CapabilityResources)

	// Create test server
	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Test 1: Create resource pool
	t.Run("CreateResourcePool", func(t *testing.T) {
		poolData := helpers.TestResourcePool("test-pool-1")

		body, err := json.Marshal(poolData)
		require.NoError(t, err)

		resp, err := http.Post(
			ts.O2IMSURL()+"/resourcePools",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.NotEmpty(t, result["resourcePoolId"])
		assert.Equal(t, "test-pool-1", result["name"])
	})

	// Test 2: Get resource pool
	t.Run("GetResourcePool", func(t *testing.T) {
		// First create a pool
		poolData := helpers.TestResourcePool("test-pool-2")
		body, _ := json.Marshal(poolData)

		createResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer createResp.Body.Close()

		var created map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&created)
		poolID := created["resourcePoolId"].(string)

		// Get the pool
		getResp, err := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resourcePools/" + poolID,
		)
		require.NoError(t, err)
		defer getResp.Body.Close()

		assert.Equal(t, http.StatusOK, getResp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, poolID, result["resourcePoolId"])
		assert.Equal(t, "test-pool-2", result["name"])
	})

	// Test 3: List resource pools
	t.Run("ListResourcePools", func(t *testing.T) {
		resp, err := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resourcePools",
		)
		require.NoError(t, err)
		defer resp.Body.Close()

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

		createResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer createResp.Body.Close()

		var created map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&created)
		poolID := created["resourcePoolId"].(string)

		// Update the pool
		updateData := map[string]interface{}{
			"name":        "test-pool-updated",
			"description": "Updated description",
		}
		updateBody, _ := json.Marshal(updateData)

		req, _ := http.NewRequest(
			http.MethodPut,
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools/"+poolID,
			bytes.NewReader(updateBody),
		)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		updateResp, err := client.Do(req)
		require.NoError(t, err)
		defer updateResp.Body.Close()

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

		createResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer createResp.Body.Close()

		var created map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&created)
		poolID := created["resourcePoolId"].(string)

		// Delete the pool
		req, _ := http.NewRequest(
			http.MethodDelete,
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools/"+poolID,
			nil,
		)

		client := &http.Client{}
		deleteResp, err := client.Do(req)
		require.NoError(t, err)
		defer deleteResp.Body.Close()

		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Verify deletion - GET should return 404
		getResp, _ := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resourcePools/" + poolID,
		)
		defer getResp.Body.Close()
		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})
}

// TestKubernetesAdapter_ResourceLifecycle tests resource CRUD operations.
func TestKubernetesAdapter_ResourceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)
	ctx := env.Context()

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:       env.Redis.Addr(),
		MaxRetries: 3,
		PoolSize:   10,
	})
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	cfg := &config.Config{}
	srv := server.New(cfg)
	srv.SetAdapter(k8sAdapter)
	srv.SetStorage(redisStore)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// First create a resource pool and resource type
	poolData := helpers.TestResourcePool("resource-test-pool")
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

	// Test 1: Create resource
	t.Run("CreateResource", func(t *testing.T) {
		resourceData := helpers.TestResource(poolID, "compute-node")
		body, err := json.Marshal(resourceData)
		require.NoError(t, err)

		resp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resources",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

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

		createResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resources",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer createResp.Body.Close()

		var created map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&created)
		resourceID := created["resourceId"].(string)

		// Get the resource
		getResp, err := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resources/" + resourceID,
		)
		require.NoError(t, err)
		defer getResp.Body.Close()

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
			resp, _ := http.Post(
				ts.URL+"/o2ims-infrastructureInventory/v1/resources",
				"application/json",
				bytes.NewReader(body),
			)
			resp.Body.Close()
		}

		// List with pool filter
		resp, err := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resources?resourcePoolId=" + poolID,
		)
		require.NoError(t, err)
		defer resp.Body.Close()

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

		createResp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resources",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer createResp.Body.Close()

		var created map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&created)
		resourceID := created["resourceId"].(string)

		// Delete the resource
		req, _ := http.NewRequest(
			http.MethodDelete,
			ts.URL+"/o2ims-infrastructureInventory/v1/resources/"+resourceID,
			nil,
		)

		client := &http.Client{}
		deleteResp, err := client.Do(req)
		require.NoError(t, err)
		defer deleteResp.Body.Close()

		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Verify deletion
		getResp, _ := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resources/" + resourceID,
		)
		defer getResp.Body.Close()
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
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	cfg := &config.Config{}
	srv := server.New(cfg)
	srv.SetAdapter(k8sAdapter)
	srv.SetStorage(redisStore)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	t.Run("GetNonExistentResourcePool", func(t *testing.T) {
		resp, err := http.Get(
			ts.URL + "/o2ims-infrastructureInventory/v1/resourcePools/nonexistent-pool",
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("CreateResourcePoolWithInvalidData", func(t *testing.T) {
		invalidData := map[string]interface{}{
			"name": "", // Empty name should fail validation
		}
		body, _ := json.Marshal(invalidData)

		resp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
			"application/json",
			bytes.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("DeleteNonExistentResource", func(t *testing.T) {
		req, _ := http.NewRequest(
			http.MethodDelete,
			ts.URL+"/o2ims-infrastructureInventory/v1/resources/nonexistent-resource",
			nil,
		)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("InvalidJSONPayload", func(t *testing.T) {
		invalidJSON := []byte(`{"invalid": json}`)

		resp, err := http.Post(
			ts.URL+"/o2ims-infrastructureInventory/v1/resourcePools",
			"application/json",
			bytes.NewReader(invalidJSON),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
