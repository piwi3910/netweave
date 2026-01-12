// Package e2e provides end-to-end tests for the O2-IMS Gateway.
// These tests verify complete user workflows against a real Kubernetes cluster.
//
//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestInfrastructureDiscovery tests the complete infrastructure discovery workflow.
// This verifies that an SMO can discover resource pools and resources.
func TestInfrastructureDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := NewTestFramework(DefaultOptions())
	require.NoError(t, err, "Failed to create test framework")
	defer fw.Cleanup()

	t.Run("list resource pools", func(t *testing.T) {
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, fw.GatewayURL+APIPathResourcePools, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err, "Failed to send request")
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK")
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Failed to read response body")

		var pools []map[string]any
		err = json.Unmarshal(body, &pools)
		require.NoError(t, err, "Failed to parse JSON response")

		// We expect at least one resource pool (the default pool)
		assert.NotEmpty(t, pools, "Expected at least one resource pool")

		// Verify pool structure
		if len(pools) > 0 {
			pool := pools[0]
			assert.Contains(t, pool, "resourcePoolId")
			assert.Contains(t, pool, "name")
			assert.Contains(t, pool, "oCloudId")
		}

		fw.Logger.Info("Successfully listed resource pools",
			zap.Int("count", len(pools)),
		)
	})

	t.Run("get specific resource pool", func(t *testing.T) {
		// First, get the list of pools to find a valid ID
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, fw.GatewayURL+APIPathResourcePools, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var pools []map[string]any
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &pools)
		require.NoError(t, err)

		if len(pools) == 0 {
			t.Skip("No resource pools available for testing")
		}

		poolID, ok := pools[0]["resourcePoolId"].(string)
		require.True(t, ok, "resourcePoolId is not a string")
		require.NotEmpty(t, poolID, "resourcePoolId is empty")

		// Get specific pool
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcePoolByID, poolID)
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var pool map[string]any
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &pool)
		require.NoError(t, err)

		assert.Equal(t, poolID, pool["resourcePoolId"])
		assert.Contains(t, pool, "name")
		assert.Contains(t, pool, "description")

		fw.Logger.Info("Successfully retrieved resource pool",
			zap.String("poolId", poolID),
			zap.Any("name", pool["name"]),
		)
	})

	t.Run("list resources in pool", func(t *testing.T) {
		// Get a pool ID first
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, fw.GatewayURL+APIPathResourcePools, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var pools []map[string]any
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &pools)
		require.NoError(t, err)

		if len(pools) == 0 {
			t.Skip("No resource pools available")
		}

		poolID, ok := pools[0]["resourcePoolId"].(string)
		require.True(t, ok, "resourcePoolId is not a string")
		require.NotEmpty(t, poolID, "resourcePoolId is empty")

		// List resources in the pool
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID)
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var resources []map[string]any
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &resources)
		require.NoError(t, err)

		// May or may not have resources depending on cluster state
		fw.Logger.Info("Listed resources in pool",
			zap.String("poolId", poolID),
			zap.Int("count", len(resources)),
		)

		// Verify resource structure if any exist
		if len(resources) > 0 {
			resource := resources[0]
			assert.Contains(t, resource, "resourceId")
			assert.Contains(t, resource, "resourceType")
			assert.Contains(t, resource, "resourcePoolId")
		}
	})

	t.Run("get specific resource", func(t *testing.T) {
		// Get a pool ID and resource ID first
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, fw.GatewayURL+APIPathResourcePools, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var pools []map[string]any
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &pools)
		require.NoError(t, err)

		if len(pools) == 0 {
			t.Skip("No resource pools available")
		}

		poolID, ok := pools[0]["resourcePoolId"].(string)
		require.True(t, ok, "resourcePoolId is not a string")
		require.NotEmpty(t, poolID, "resourcePoolId is empty")

		// Get resources
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID)
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var resources []map[string]any
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &resources)
		require.NoError(t, err)

		if len(resources) == 0 {
			t.Skip("No resources available for testing")
		}

		resourceID, ok := resources[0]["resourceId"].(string)
		require.True(t, ok, "resourceId is not a string")
		require.NotEmpty(t, resourceID, "resourceId is empty")

		// Get specific resource
		url = fw.GatewayURL + fmt.Sprintf(APIPathResourceByID, poolID, resourceID)
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var resource map[string]any
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &resource)
		require.NoError(t, err)

		assert.Equal(t, resourceID, resource["resourceId"])
		assert.Equal(t, poolID, resource["resourcePoolId"])
		assert.Contains(t, resource, "resourceType")

		fw.Logger.Info("Successfully retrieved resource",
			zap.String("poolId", poolID),
			zap.String("resourceId", resourceID),
			zap.Any("resourceType", resource["resourceType"]),
		)
	})

	t.Run("filter resources by type", func(t *testing.T) {
		// Get a pool ID first
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, fw.GatewayURL+APIPathResourcePools, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var pools []map[string]any
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &pools)
		require.NoError(t, err)

		if len(pools) == 0 {
			t.Skip("No resource pools available")
		}

		poolID, ok := pools[0]["resourcePoolId"].(string)
		require.True(t, ok, "resourcePoolId is not a string")
		require.NotEmpty(t, poolID, "resourcePoolId is empty")

		// Filter resources by type (Node is common in K8s clusters)
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID) + "?filter=resourceType==Node"
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var resources []map[string]any
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &resources)
		require.NoError(t, err)

		// Verify all returned resources are of type Node
		for _, resource := range resources {
			assert.Equal(t, "Node", resource["resourceType"])
		}

		fw.Logger.Info("Successfully filtered resources by type",
			zap.String("poolId", poolID),
			zap.String("filter", "resourceType==Node"),
			zap.Int("count", len(resources)),
		)
	})

	t.Run("pagination support", func(t *testing.T) {
		// Get a pool ID first
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, fw.GatewayURL+APIPathResourcePools, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		var pools []map[string]any
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &pools)
		require.NoError(t, err)

		if len(pools) == 0 {
			t.Skip("No resource pools available")
		}

		poolID, ok := pools[0]["resourcePoolId"].(string)
		require.True(t, ok, "resourcePoolId is not a string")
		require.NotEmpty(t, poolID, "resourcePoolId is empty")

		// Request with limit
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID) + "?limit=5"
		req, err = http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err = fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var resources []map[string]any
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &resources)
		require.NoError(t, err)

		// Should not exceed the limit
		assert.LessOrEqual(t, len(resources), 5)

		fw.Logger.Info("Successfully tested pagination",
			zap.String("poolId", poolID),
			zap.Int("limit", 5),
			zap.Int("returned", len(resources)),
		)
	})
}

// TestErrorHandling tests error responses from the API.
func TestErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := NewTestFramework(DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	t.Run("get non-existent pool", func(t *testing.T) {
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcePoolByID, "non-existent-pool-id")
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var errorResp map[string]any
		err = json.Unmarshal(body, &errorResp)
		require.NoError(t, err)

		assert.Contains(t, errorResp, "error")

		fw.Logger.Info("Successfully handled non-existent resource pool")
	})

	t.Run("invalid filter syntax", func(t *testing.T) {
		// Use invalid filter syntax
		url := fw.GatewayURL + APIPathResourcePools + "?filter=invalid syntax here"
		req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
		require.NoError(t, err)

		resp, err := fw.APIClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Failed to close response body: %v", err)
			}
		}()

		// Should return 400 Bad Request for invalid filter
		assert.True(t, resp.StatusCode >= 400, "Expected error status code")

		fw.Logger.Info("Successfully handled invalid filter syntax",
			zap.Int("statusCode", resp.StatusCode),
		)
	})
}
